package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Reg-Kris/pyairtable-webhook-service/internal/models"
	"github.com/Reg-Kris/pyairtable-webhook-service/internal/repositories"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// DeliveryService handles webhook delivery with retry logic and circuit breakers
type DeliveryService struct {
	repos           *repositories.Repositories
	security        *SecurityService
	redis           *redis.Client
	logger          *zap.Logger
	circuitBreakers sync.Map // map[uint]*gobreaker.CircuitBreaker
	rateLimiters    sync.Map // map[uint]*rate.Limiter
	httpClient      *http.Client
}

// NewDeliveryService creates a new delivery service
func NewDeliveryService(repos *repositories.Repositories, security *SecurityService, redis *redis.Client, logger *zap.Logger) *DeliveryService {
	return &DeliveryService{
		repos:    repos,
		security: security,
		redis:    redis,
		logger:   logger,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// DeliveryResult represents the result of a webhook delivery attempt
type DeliveryResult struct {
	Success        bool                   `json:"success"`
	StatusCode     int                    `json:"status_code"`
	ResponseBody   string                 `json:"response_body,omitempty"`
	ResponseHeaders map[string]interface{} `json:"response_headers,omitempty"`
	ErrorMessage   string                 `json:"error_message,omitempty"`
	Duration       time.Duration          `json:"duration"`
	AttemptNumber  int                    `json:"attempt_number"`
}

// DeliverWebhook delivers a webhook event to a specific webhook endpoint
func (s *DeliveryService) DeliverWebhook(ctx context.Context, webhook *models.Webhook, event *models.WebhookEvent) error {
	// Check circuit breaker
	cb := s.getCircuitBreaker(webhook.ID)
	if cb.State() == gobreaker.StateOpen {
		return fmt.Errorf("circuit breaker is open for webhook %d", webhook.ID)
	}
	
	// Apply rate limiting
	limiter := s.getRateLimiter(webhook.ID, webhook.RateLimitRPS)
	if !limiter.Allow() {
		return fmt.Errorf("rate limit exceeded for webhook %d", webhook.ID)
	}
	
	// Create delivery attempt record
	attempt := &models.DeliveryAttempt{
		WebhookID:      webhook.ID,
		EventID:        event.ID,
		EventType:      event.Type,
		Payload:        models.JSONMap(event.Data),
		Status:         models.DeliveryStatusPending,
		AttemptNumber:  1,
		ScheduledAt:    time.Now(),
		IdempotencyKey: fmt.Sprintf("%s-%d-%d", event.ID, webhook.ID, time.Now().UnixNano()),
	}
	
	if err := s.repos.DeliveryAttempt.Create(ctx, attempt); err != nil {
		return fmt.Errorf("failed to create delivery attempt: %w", err)
	}
	
	// Attempt delivery
	result, err := s.attemptDelivery(ctx, webhook, event, 1)
	if err != nil {
		s.logger.Error("Failed to attempt webhook delivery",
			zap.Error(err),
			zap.Uint("webhook_id", webhook.ID),
			zap.String("event_id", event.ID))
	}
	
	// Update delivery attempt with result
	s.updateDeliveryAttempt(ctx, attempt, result, err)
	
	// If delivery failed, schedule retry or move to dead letter
	if err != nil || !result.Success {
		return s.handleFailedDelivery(ctx, webhook, attempt, result, err)
	}
	
	// Update webhook success statistics
	s.repos.Webhook.UpdateStats(ctx, webhook.ID, true, time.Now())
	
	return nil
}

// attemptDelivery performs the actual HTTP request to deliver the webhook
func (s *DeliveryService) attemptDelivery(ctx context.Context, webhook *models.Webhook, event *models.WebhookEvent, attemptNumber int) (*DeliveryResult, error) {
	start := time.Now()
	
	// Prepare payload
	payload, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event payload: %w", err)
	}
	
	// Generate signature
	timestamp := time.Now()
	signature, err := s.security.GenerateSignature(webhook.Secret, string(payload), timestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to generate signature: %w", err)
	}
	
	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", webhook.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	
	// Set standard headers
	headers := s.security.CreateWebhookHeaders(event.ID, string(event.Type), signature, timestamp)
	headers["X-PyAirtable-Webhook-ID"] = strconv.FormatUint(uint64(webhook.ID), 10)
	
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	
	// Set custom headers
	if webhook.Headers != nil {
		sanitizedHeaders := s.security.SanitizeHeaders(webhook.Headers)
		for key, value := range sanitizedHeaders {
			req.Header.Set(key, value)
		}
	}
	
	// Set timeout based on webhook configuration
	if webhook.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(webhook.TimeoutSeconds)*time.Second)
		defer cancel()
		req = req.WithContext(ctx)
	}
	
	// Execute request through circuit breaker
	cb := s.getCircuitBreaker(webhook.ID)
	
	var resp *http.Response
	var reqErr error
	
	_, err = cb.Execute(func() (interface{}, error) {
		resp, reqErr = s.httpClient.Do(req)
		return nil, reqErr
	})
	
	duration := time.Since(start)
	
	if err != nil {
		return &DeliveryResult{
			Success:       false,
			ErrorMessage:  fmt.Sprintf("Circuit breaker error: %v", err),
			Duration:      duration,
			AttemptNumber: attemptNumber,
		}, err
	}
	
	if reqErr != nil {
		return &DeliveryResult{
			Success:       false,
			ErrorMessage:  fmt.Sprintf("HTTP request failed: %v", reqErr),
			Duration:      duration,
			AttemptNumber: attemptNumber,
		}, reqErr
	}
	
	defer resp.Body.Close()
	
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.logger.Warn("Failed to read response body", zap.Error(err))
		body = []byte("Failed to read response body")
	}
	
	// Limit response body size
	responseBody := string(body)
	if len(responseBody) > 10000 {
		responseBody = responseBody[:10000] + "... (truncated)"
	}
	
	// Convert response headers
	responseHeaders := make(map[string]interface{})
	for key, values := range resp.Header {
		if len(values) > 0 {
			responseHeaders[key] = values[0]
		}
	}
	
	// Determine success based on status code
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	
	result := &DeliveryResult{
		Success:         success,
		StatusCode:      resp.StatusCode,
		ResponseBody:    responseBody,
		ResponseHeaders: responseHeaders,
		Duration:        duration,
		AttemptNumber:   attemptNumber,
	}
	
	if !success {
		result.ErrorMessage = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	
	return result, nil
}

// handleFailedDelivery handles retry logic or moves to dead letter queue
func (s *DeliveryService) handleFailedDelivery(ctx context.Context, webhook *models.Webhook, attempt *models.DeliveryAttempt, result *DeliveryResult, err error) error {
	// Update webhook failure statistics
	s.repos.Webhook.UpdateStats(ctx, webhook.ID, false, time.Now())
	
	// Check if we should retry
	if attempt.AttemptNumber < webhook.MaxRetries {
		// Calculate next retry time using exponential backoff
		backoffSeconds := s.calculateBackoff(attempt.AttemptNumber)
		nextRetry := time.Now().Add(time.Duration(backoffSeconds) * time.Second)
		
		// Update attempt for retry
		attempt.Status = models.DeliveryStatusRetrying
		attempt.NextRetryAt = &nextRetry
		
		if err := s.repos.DeliveryAttempt.Update(ctx, attempt); err != nil {
			s.logger.Error("Failed to update delivery attempt for retry", zap.Error(err))
		}
		
		s.logger.Info("Scheduled webhook delivery retry",
			zap.Uint("webhook_id", webhook.ID),
			zap.String("event_id", attempt.EventID),
			zap.Int("attempt_number", attempt.AttemptNumber),
			zap.Time("next_retry", nextRetry))
		
		return nil
	}
	
	// Move to dead letter queue
	return s.moveToDeadLetter(ctx, webhook, attempt, result, err)
}

// moveToDeadLetter moves a failed delivery to the dead letter queue
func (s *DeliveryService) moveToDeadLetter(ctx context.Context, webhook *models.Webhook, attempt *models.DeliveryAttempt, result *DeliveryResult, err error) error {
	failureReason := "Maximum retry attempts exceeded"
	if err != nil {
		failureReason = err.Error()
	} else if result != nil && result.ErrorMessage != "" {
		failureReason = result.ErrorMessage
	}
	
	deadLetter := &models.DeadLetter{
		WebhookID:     webhook.ID,
		EventID:       attempt.EventID,
		EventType:     attempt.EventType,
		Payload:       attempt.Payload,
		FailureReason: failureReason,
		LastAttemptAt: time.Now(),
		AttemptCount:  attempt.AttemptNumber,
		Metadata: models.JSONMap{
			"last_status_code": result.StatusCode,
			"last_error":       failureReason,
		},
	}
	
	if err := s.repos.DeadLetter.Create(ctx, deadLetter); err != nil {
		return fmt.Errorf("failed to create dead letter: %w", err)
	}
	
	// Update delivery attempt status
	attempt.Status = models.DeliveryStatusAbandoned
	if err := s.repos.DeliveryAttempt.Update(ctx, attempt); err != nil {
		s.logger.Error("Failed to update delivery attempt status", zap.Error(err))
	}
	
	s.logger.Warn("Moved webhook delivery to dead letter queue",
		zap.Uint("webhook_id", webhook.ID),
		zap.String("event_id", attempt.EventID),
		zap.String("failure_reason", failureReason))
	
	return nil
}

// updateDeliveryAttempt updates the delivery attempt with the result
func (s *DeliveryService) updateDeliveryAttempt(ctx context.Context, attempt *models.DeliveryAttempt, result *DeliveryResult, err error) {
	now := time.Now()
	attempt.AttemptedAt = &now
	
	if result != nil {
		attempt.ResponseStatus = &result.StatusCode
		attempt.ResponseBody = &result.ResponseBody
		attempt.ResponseHeaders = result.ResponseHeaders
		durationMs := result.Duration.Milliseconds()
		attempt.DurationMs = &durationMs
		
		if result.Success {
			attempt.Status = models.DeliveryStatusDelivered
			attempt.DeliveredAt = &now
		} else {
			attempt.Status = models.DeliveryStatusFailed
			attempt.ErrorMessage = &result.ErrorMessage
		}
	} else if err != nil {
		attempt.Status = models.DeliveryStatusFailed
		errMsg := err.Error()
		attempt.ErrorMessage = &errMsg
	}
	
	if updateErr := s.repos.DeliveryAttempt.Update(ctx, attempt); updateErr != nil {
		s.logger.Error("Failed to update delivery attempt", zap.Error(updateErr))
	}
}

// calculateBackoff calculates exponential backoff delay
func (s *DeliveryService) calculateBackoff(attemptNumber int) int {
	// Base delay: 2^(attempt-1) seconds, with jitter
	// Attempt 1: 1s, Attempt 2: 2s, Attempt 3: 4s, etc.
	baseDelay := 1 << (attemptNumber - 1)
	
	// Cap at 5 minutes
	if baseDelay > 300 {
		baseDelay = 300
	}
	
	// Add jitter (±25%)
	jitter := baseDelay / 4
	if jitter < 1 {
		jitter = 1
	}
	
	return baseDelay + (int(time.Now().Unix()) % (jitter * 2)) - jitter
}

// getCircuitBreaker gets or creates a circuit breaker for a webhook
func (s *DeliveryService) getCircuitBreaker(webhookID uint) *gobreaker.CircuitBreaker {
	if cb, ok := s.circuitBreakers.Load(webhookID); ok {
		return cb.(*gobreaker.CircuitBreaker)
	}
	
	settings := gobreaker.Settings{
		Name:        fmt.Sprintf("webhook-%d", webhookID),
		MaxRequests: 3,
		Interval:    60 * time.Second,
		Timeout:     60 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.6
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			s.logger.Info("Circuit breaker state change",
				zap.String("name", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()))
		},
	}
	
	cb := gobreaker.NewCircuitBreaker(settings)
	s.circuitBreakers.Store(webhookID, cb)
	
	return cb
}

// getRateLimiter gets or creates a rate limiter for a webhook
func (s *DeliveryService) getRateLimiter(webhookID uint, rps int) *rate.Limiter {
	if limiter, ok := s.rateLimiters.Load(webhookID); ok {
		return limiter.(*rate.Limiter)
	}
	
	// Default to 10 RPS if not specified
	if rps <= 0 {
		rps = 10
	}
	
	limiter := rate.NewLimiter(rate.Limit(rps), rps*2) // burst = 2x rate
	s.rateLimiters.Store(webhookID, limiter)
	
	return limiter
}

// ProcessRetries processes delivery attempts that are ready for retry
func (s *DeliveryService) ProcessRetries(ctx context.Context) error {
	retries, err := s.repos.DeliveryAttempt.GetRetriesReady(ctx, 100)
	if err != nil {
		return fmt.Errorf("failed to get retries ready: %w", err)
	}
	
	for _, attempt := range retries {
		// Increment attempt number
		attempt.AttemptNumber++
		
		// Create event from attempt data
		event := &models.WebhookEvent{
			ID:        attempt.EventID,
			Type:      attempt.EventType,
			Data:      attempt.Payload,
			Timestamp: time.Now(),
		}
		
		// Attempt delivery
		result, err := s.attemptDelivery(ctx, &attempt.Webhook, event, attempt.AttemptNumber)
		
		// Update attempt with result
		s.updateDeliveryAttempt(ctx, attempt, result, err)
		
		// Handle failure if still failing
		if err != nil || !result.Success {
			if err := s.handleFailedDelivery(ctx, &attempt.Webhook, attempt, result, err); err != nil {
				s.logger.Error("Failed to handle failed retry", zap.Error(err))
			}
		} else {
			// Update webhook success statistics
			s.repos.Webhook.UpdateStats(ctx, attempt.WebhookID, true, time.Now())
		}
	}
	
	return nil
}