package services

import (
	"context"
	"fmt"

	"github.com/Reg-Kris/pyairtable-webhook-service/internal/handlers"
	"github.com/Reg-Kris/pyairtable-webhook-service/internal/models"
	"github.com/Reg-Kris/pyairtable-webhook-service/internal/repositories"
	"go.uber.org/zap"
	"time"
)

// WebhookService handles webhook business logic
type WebhookService struct {
	repos    *repositories.Repositories
	security *SecurityService
	logger   *zap.Logger
}

// NewWebhookService creates a new webhook service
func NewWebhookService(repos *repositories.Repositories, security *SecurityService, logger *zap.Logger) *WebhookService {
	return &WebhookService{
		repos:    repos,
		security: security,
		logger:   logger,
	}
}

// CreateWebhook creates a new webhook
func (s *WebhookService) CreateWebhook(ctx context.Context, userID uint, req *handlers.CreateWebhookRequest) (*models.Webhook, error) {
	// Validate URL
	if err := s.security.ValidateWebhookURL(req.URL); err != nil {
		return nil, fmt.Errorf("invalid webhook URL: %w", err)
	}
	
	// Generate secret
	secret, err := s.security.GenerateSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}
	
	// Set defaults
	maxRetries := req.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	
	timeoutSeconds := req.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	
	rateLimitRPS := req.RateLimitRPS
	if rateLimitRPS <= 0 {
		rateLimitRPS = 10
	}
	
	// Create webhook
	webhook := &models.Webhook{
		UserID:         userID,
		WorkspaceID:    req.WorkspaceID,
		Name:           req.Name,
		URL:            req.URL,
		Secret:         secret,
		Status:         models.WebhookStatusActive,
		Description:    req.Description,
		MaxRetries:     maxRetries,
		TimeoutSeconds: timeoutSeconds,
		RateLimitRPS:   rateLimitRPS,
		Headers:        models.JSONMap(req.Headers),
		Metadata:       models.JSONMap(req.Metadata),
	}
	
	if err := s.repos.Webhook.Create(ctx, webhook); err != nil {
		return nil, fmt.Errorf("failed to create webhook: %w", err)
	}
	
	// Create subscriptions if provided
	if len(req.Subscriptions) > 0 {
		for _, subReq := range req.Subscriptions {
			isActive := true
			if subReq.IsActive != nil {
				isActive = *subReq.IsActive
			}
			
			subscription := &models.Subscription{
				WebhookID:  webhook.ID,
				EventType:  subReq.EventType,
				ResourceID: subReq.ResourceID,
				IsActive:   isActive,
				Filters:    models.JSONMap(subReq.Filters),
			}
			
			if err := s.repos.Subscription.Create(ctx, subscription); err != nil {
				s.logger.Error("Failed to create subscription", zap.Error(err), zap.Uint("webhook_id", webhook.ID))
				// Continue with other subscriptions
			}
		}
		
		// Reload webhook with subscriptions
		webhook, _ = s.repos.Webhook.GetByID(ctx, webhook.ID)
	}
	
	s.logger.Info("Created webhook", zap.Uint("webhook_id", webhook.ID), zap.Uint("user_id", userID))
	
	return webhook, nil
}

// GetWebhooks retrieves webhooks for a user
func (s *WebhookService) GetWebhooks(ctx context.Context, userID uint, workspaceID *uint, limit, offset int) ([]*models.Webhook, error) {
	if workspaceID != nil {
		return s.repos.Webhook.GetByWorkspaceID(ctx, *workspaceID, limit, offset)
	}
	return s.repos.Webhook.GetByUserID(ctx, userID, limit, offset)
}

// GetWebhook retrieves a webhook by ID
func (s *WebhookService) GetWebhook(ctx context.Context, webhookID, userID uint) (*models.Webhook, error) {
	webhook, err := s.repos.Webhook.GetByID(ctx, webhookID)
	if err != nil {
		return nil, err
	}
	
	if webhook == nil {
		return nil, nil
	}
	
	// Check ownership
	if webhook.UserID != userID {
		return nil, fmt.Errorf("webhook not found or access denied")
	}
	
	return webhook, nil
}

// UpdateWebhook updates a webhook
func (s *WebhookService) UpdateWebhook(ctx context.Context, webhookID, userID uint, req *handlers.UpdateWebhookRequest) (*models.Webhook, error) {
	webhook, err := s.GetWebhook(ctx, webhookID, userID)
	if err != nil {
		return nil, err
	}
	
	if webhook == nil {
		return nil, nil
	}
	
	// Update fields
	if req.Name != nil {
		webhook.Name = *req.Name
	}
	
	if req.URL != nil {
		if err := s.security.ValidateWebhookURL(*req.URL); err != nil {
			return nil, fmt.Errorf("invalid webhook URL: %w", err)
		}
		webhook.URL = *req.URL
	}
	
	if req.Description != nil {
		webhook.Description = *req.Description
	}
	
	if req.Status != nil {
		webhook.Status = *req.Status
	}
	
	if req.MaxRetries != nil {
		webhook.MaxRetries = *req.MaxRetries
	}
	
	if req.TimeoutSeconds != nil {
		webhook.TimeoutSeconds = *req.TimeoutSeconds
	}
	
	if req.RateLimitRPS != nil {
		webhook.RateLimitRPS = *req.RateLimitRPS
	}
	
	if req.Headers != nil {
		webhook.Headers = models.JSONMap(req.Headers)
	}
	
	if req.Metadata != nil {
		webhook.Metadata = models.JSONMap(req.Metadata)
	}
	
	if err := s.repos.Webhook.Update(ctx, webhook); err != nil {
		return nil, fmt.Errorf("failed to update webhook: %w", err)
	}
	
	s.logger.Info("Updated webhook", zap.Uint("webhook_id", webhookID), zap.Uint("user_id", userID))
	
	return webhook, nil
}

// DeleteWebhook deletes a webhook
func (s *WebhookService) DeleteWebhook(ctx context.Context, webhookID, userID uint) error {
	webhook, err := s.GetWebhook(ctx, webhookID, userID)
	if err != nil {
		return err
	}
	
	if webhook == nil {
		return fmt.Errorf("webhook not found")
	}
	
	if err := s.repos.Webhook.Delete(ctx, webhookID); err != nil {
		return fmt.Errorf("failed to delete webhook: %w", err)
	}
	
	s.logger.Info("Deleted webhook", zap.Uint("webhook_id", webhookID), zap.Uint("user_id", userID))
	
	return nil
}

// GetWebhookDeliveries retrieves delivery attempts for a webhook
func (s *WebhookService) GetWebhookDeliveries(ctx context.Context, webhookID, userID uint, limit, offset int) ([]*models.DeliveryAttempt, error) {
	// Verify ownership
	webhook, err := s.GetWebhook(ctx, webhookID, userID)
	if err != nil {
		return nil, err
	}
	
	if webhook == nil {
		return nil, fmt.Errorf("webhook not found")
	}
	
	return s.repos.DeliveryAttempt.GetByWebhookID(ctx, webhookID, limit, offset)
}

// GetWebhookMetrics retrieves metrics for a webhook
func (s *WebhookService) GetWebhookMetrics(ctx context.Context, webhookID, userID uint, days int) (*models.WebhookMetrics, error) {
	// Verify ownership
	webhook, err := s.GetWebhook(ctx, webhookID, userID)
	if err != nil {
		return nil, err
	}
	
	if webhook == nil {
		return nil, fmt.Errorf("webhook not found")
	}
	
	since := time.Now().AddDate(0, 0, -days)
	return s.repos.DeliveryAttempt.GetMetrics(ctx, webhookID, since)
}

// TestWebhook tests a webhook by sending a test payload
func (s *WebhookService) TestWebhook(ctx context.Context, webhookID, userID uint, payload map[string]interface{}) (*handlers.DeliveryResult, error) {
	webhook, err := s.GetWebhook(ctx, webhookID, userID)
	if err != nil {
		return nil, err
	}
	
	if webhook == nil {
		return nil, fmt.Errorf("webhook not found")
	}
	
	// Create test event
	testEvent := &models.WebhookEvent{
		ID:        fmt.Sprintf("test-%d-%d", webhookID, time.Now().Unix()),
		Type:      "test.webhook",
		ResourceID: "test",
		UserID:    userID,
		Timestamp: time.Now(),
		Data:      models.JSONMap(payload),
		Metadata: models.JSONMap{
			"test": true,
		},
	}
	
	// Use a test delivery service to avoid creating database records
	testDelivery := &DeliveryService{
		repos:    s.repos,
		security: s.security,
		logger:   s.logger,
	}
	
	result, err := testDelivery.attemptDelivery(ctx, webhook, testEvent, 1)
	if err != nil {
		return nil, fmt.Errorf("test delivery failed: %w", err)
	}
	
	// Convert to handler result type
	return &handlers.DeliveryResult{
		Success:         result.Success,
		StatusCode:      result.StatusCode,
		ResponseBody:    result.ResponseBody,
		ResponseHeaders: result.ResponseHeaders,
		ErrorMessage:    result.ErrorMessage,
		Duration:        result.Duration,
		AttemptNumber:   result.AttemptNumber,
	}, nil
}

// RegenerateSecret generates a new secret for a webhook
func (s *WebhookService) RegenerateSecret(ctx context.Context, webhookID, userID uint) (string, error) {
	webhook, err := s.GetWebhook(ctx, webhookID, userID)
	if err != nil {
		return "", err
	}
	
	if webhook == nil {
		return "", fmt.Errorf("webhook not found")
	}
	
	// Generate new secret
	newSecret, err := s.security.GenerateSecret()
	if err != nil {
		return "", fmt.Errorf("failed to generate new secret: %w", err)
	}
	
	// Update webhook
	webhook.Secret = newSecret
	if err := s.repos.Webhook.Update(ctx, webhook); err != nil {
		return "", fmt.Errorf("failed to update webhook with new secret: %w", err)
	}
	
	s.logger.Info("Regenerated webhook secret", zap.Uint("webhook_id", webhookID), zap.Uint("user_id", userID))
	
	return newSecret, nil
}