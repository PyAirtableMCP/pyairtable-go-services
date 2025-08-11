package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pyairtable/api-gateway/internal/config"
	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

// Engine handles high-performance request routing to backend services
type Engine struct {
	logger          *zap.Logger
	registry        *ServiceRegistry
	balancer        LoadBalancer
	client          *http.Client
	circuitBreakers map[string]*gobreaker.CircuitBreaker
	cbMutex         sync.RWMutex
	config          *config.Config
	stickySession   *StickySessionManager
}

// RouteContext contains information about the current request routing
type RouteContext struct {
	ServiceName    string
	Instance       *ServiceInstance
	StartTime      time.Time
	TraceID        string
	RequestID      string
	RetryAttempt   int
	CircuitBreaker *gobreaker.CircuitBreaker
}

func NewEngine(logger *zap.Logger, registry *ServiceRegistry, cfg *config.Config) (*Engine, error) {
	// Create load balancer
	balancer, err := NewLoadBalancer(cfg.Performance.LoadBalancing.Algorithm)
	if err != nil {
		return nil, fmt.Errorf("failed to create load balancer: %w", err)
	}

	// Create HTTP client optimized for high performance
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  false,
			WriteBufferSize:     32 * 1024,
			ReadBufferSize:      32 * 1024,
		},
	}

	engine := &Engine{
		logger:          logger,
		registry:        registry,
		balancer:        balancer,
		client:          client,
		circuitBreakers: make(map[string]*gobreaker.CircuitBreaker),
		config:          cfg,
	}

	// Initialize sticky session manager if enabled
	if cfg.Performance.LoadBalancing.StickySession {
		engine.stickySession = NewStickySessionManager(logger)
	}

	return engine, nil
}

func (e *Engine) RouteRequest(c *fiber.Ctx, serviceName string) error {
	ctx := &RouteContext{
		ServiceName: serviceName,
		StartTime:   time.Now(),
		TraceID:     c.Get("X-Trace-ID"),
		RequestID:   c.Get("X-Request-ID"),
	}

	// Generate request ID if not present
	if ctx.RequestID == "" {
		ctx.RequestID = generateRequestID()
	}

	// Select service instance
	instance, err := e.selectServiceInstance(serviceName, c)
	if err != nil {
		e.logger.Error("Failed to select service instance",
			zap.String("service", serviceName),
			zap.String("trace_id", ctx.TraceID),
			zap.Error(err))
		return fiber.NewError(fiber.StatusServiceUnavailable, "Service temporarily unavailable")
	}

	ctx.Instance = instance

	// Get or create circuit breaker for this instance
	cb := e.getCircuitBreaker(instance)
	ctx.CircuitBreaker = cb

	// Execute request with circuit breaker
	result, err := cb.Execute(func() (interface{}, error) {
		return nil, e.executeRequest(c, ctx)
	})

	// Handle circuit breaker result
	if err != nil {
		if err == gobreaker.ErrOpenState {
			e.logger.Warn("Circuit breaker is open",
				zap.String("service", serviceName),
				zap.String("instance", instance.ID),
				zap.String("trace_id", ctx.TraceID))
			return fiber.NewError(fiber.StatusServiceUnavailable, "Service circuit breaker is open")
		}
		return err
	}

	_ = result // Circuit breaker returns interface{}, but we don't need it
	return nil
}

func (e *Engine) selectServiceInstance(serviceName string, c *fiber.Ctx) (*ServiceInstance, error) {
	// Check for sticky session
	if e.stickySession != nil {
		sessionID := e.extractSessionID(c)
		if sessionID != "" {
			if instance := e.stickySession.GetInstance(sessionID, serviceName); instance != nil {
				if instance.IsHealthy() {
					return instance, nil
				}
				// Remove unhealthy sticky session
				e.stickySession.RemoveSession(sessionID, serviceName)
			}
		}
	}

	// Get healthy instances
	instances := e.registry.GetHealthyInstances(serviceName)
	if len(instances) == 0 {
		return nil, fmt.Errorf("no healthy instances available for service: %s", serviceName)
	}

	// Use load balancer to select instance
	instance, err := e.balancer.SelectInstance(instances)
	if err != nil {
		return nil, err
	}

	// Set sticky session if enabled
	if e.stickySession != nil {
		sessionID := e.extractSessionID(c)
		if sessionID != "" {
			e.stickySession.SetInstance(sessionID, serviceName, instance)
		}
	}

	return instance, nil
}

func (e *Engine) executeRequest(c *fiber.Ctx, ctx *RouteContext) error {
	maxRetries := ctx.Instance.RetryCount
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		ctx.RetryAttempt = attempt
		
		if attempt > 0 {
			// Wait before retry
			time.Sleep(ctx.Instance.RetryDelay)
			e.logger.Debug("Retrying request",
				zap.String("service", ctx.ServiceName),
				zap.String("instance", ctx.Instance.ID),
				zap.Int("attempt", attempt),
				zap.String("trace_id", ctx.TraceID))
		}

		err := e.performRequest(c, ctx)
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Don't retry on client errors (4xx)
		if fiberErr, ok := err.(*fiber.Error); ok {
			if fiberErr.Code >= 400 && fiberErr.Code < 500 {
				break
			}
		}
	}

	// All retries failed
	ctx.Instance.IncrementErrors()
	return lastErr
}

func (e *Engine) performRequest(c *fiber.Ctx, ctx *RouteContext) error {
	// Build target URL
	targetURL := ctx.Instance.URL + c.Path()

	// Create request
	var body io.Reader
	if c.Body() != nil {
		body = bytes.NewReader(c.Body())
	}

	reqCtx, cancel := context.WithTimeout(c.Context(), ctx.Instance.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, c.Method(), targetURL, body)
	if err != nil {
		e.logger.Error("Failed to create request",
			zap.String("service", ctx.ServiceName),
			zap.String("url", targetURL),
			zap.String("trace_id", ctx.TraceID),
			zap.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create request")
	}

	// Copy headers from original request
	e.copyHeaders(c, req, ctx)

	// Add query parameters
	if queryString := string(c.Request().URI().QueryString()); queryString != "" {
		req.URL.RawQuery = queryString
	}

	// Record start time for metrics
	startTime := time.Now()

	// Execute request
	resp, err := e.client.Do(req)
	duration := time.Since(startTime)

	// Update instance metrics
	ctx.Instance.UpdateResponseTime(duration)
	ctx.Instance.IncrementRequests()

	if err != nil {
		e.logger.Error("Request execution failed",
			zap.String("service", ctx.ServiceName),
			zap.String("url", targetURL),
			zap.Duration("duration", duration),
			zap.String("trace_id", ctx.TraceID),
			zap.Error(err))
		ctx.Instance.IncrementErrors()
		return fiber.NewError(fiber.StatusBadGateway, "Upstream service error")
	}
	defer resp.Body.Close()

	// Log successful request
	e.logger.Debug("Request completed",
		zap.String("service", ctx.ServiceName),
		zap.String("instance", ctx.Instance.ID),
		zap.String("method", c.Method()),
		zap.String("path", c.Path()),
		zap.Int("status", resp.StatusCode),
		zap.Duration("duration", duration),
		zap.String("trace_id", ctx.TraceID))

	// Copy response
	return e.copyResponse(c, resp, ctx)
}

func (e *Engine) copyHeaders(c *fiber.Ctx, req *http.Request, ctx *RouteContext) {
	// Copy original headers
	c.Request().Header.VisitAll(func(key, value []byte) {
		keyStr := string(key)
		// Skip hop-by-hop headers
		if !isHopByHopHeader(keyStr) {
			req.Header.Set(keyStr, string(value))
		}
	})

	// Add routing headers
	req.Header.Set("X-Gateway-Service", ctx.ServiceName)
	req.Header.Set("X-Gateway-Instance", ctx.Instance.ID)
	req.Header.Set("X-Request-ID", ctx.RequestID)
	
	if ctx.TraceID != "" {
		req.Header.Set("X-Trace-ID", ctx.TraceID)
	}
	
	if ctx.RetryAttempt > 0 {
		req.Header.Set("X-Retry-Attempt", fmt.Sprintf("%d", ctx.RetryAttempt))
	}

	// Set forwarded headers
	req.Header.Set("X-Forwarded-For", c.IP())
	req.Header.Set("X-Forwarded-Proto", c.Protocol())
	req.Header.Set("X-Forwarded-Host", c.Hostname())
}

func (e *Engine) copyResponse(c *fiber.Ctx, resp *http.Response, ctx *RouteContext) error {
	// Copy response headers
	for key, values := range resp.Header {
		if !isHopByHopHeader(key) {
			for _, value := range values {
				c.Response().Header.Add(key, value)
			}
		}
	}

	// Add gateway headers
	c.Set("X-Gateway-Service", ctx.ServiceName)
	c.Set("X-Gateway-Instance", ctx.Instance.ID)
	c.Set("X-Request-ID", ctx.RequestID)
	c.Set("X-Response-Time", time.Since(ctx.StartTime).String())

	// Set status code
	c.Status(resp.StatusCode)

	// Handle different content types efficiently
	contentType := resp.Header.Get("Content-Type")
	
	// For JSON responses, we can optimize
	if strings.Contains(contentType, "application/json") {
		return e.copyJSONResponse(c, resp)
	}

	// For other content types, stream the response
	return e.streamResponse(c, resp)
}

func (e *Engine) copyJSONResponse(c *fiber.Ctx, resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to read response body")
	}

	// Validate JSON if in development mode
	if e.config.Server.Environment == "development" {
		var js json.RawMessage
		if err := json.Unmarshal(body, &js); err != nil {
			e.logger.Warn("Invalid JSON response from upstream", zap.Error(err))
		}
	}

	return c.Send(body)
}

func (e *Engine) streamResponse(c *fiber.Ctx, resp *http.Response) error {
	// Use io.Copy for efficient streaming
	_, err := io.Copy(c.Response().BodyWriter(), resp.Body)
	return err
}

func (e *Engine) getCircuitBreaker(instance *ServiceInstance) *gobreaker.CircuitBreaker {
	e.cbMutex.RLock()
	cb, exists := e.circuitBreakers[instance.ID]
	e.cbMutex.RUnlock()

	if exists {
		return cb
	}

	e.cbMutex.Lock()
	defer e.cbMutex.Unlock()

	// Double-check after acquiring write lock
	if cb, exists := e.circuitBreakers[instance.ID]; exists {
		return cb
	}

	// Create new circuit breaker
	settings := gobreaker.Settings{
		Name:        fmt.Sprintf("%s-%s", instance.Name, instance.ID),
		MaxRequests: e.config.Performance.CircuitBreaker.MaxRequests,
		Interval:    e.config.Performance.CircuitBreaker.Interval,
		Timeout:     e.config.Performance.CircuitBreaker.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 3 && failureRatio >= 0.6
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			e.logger.Info("Circuit breaker state changed",
				zap.String("name", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()))
		},
	}

	cb = gobreaker.NewCircuitBreaker(settings)
	e.circuitBreakers[instance.ID] = cb

	return cb
}

func (e *Engine) extractSessionID(c *fiber.Ctx) string {
	// Extract session ID from various sources
	if sessionID := c.Get("X-Session-ID"); sessionID != "" {
		return sessionID
	}
	
	if sessionID := c.Cookies("session_id"); sessionID != "" {
		return sessionID
	}
	
	// Could also extract from JWT token if needed
	return ""
}

// Helper functions

func isHopByHopHeader(header string) bool {
	hopByHopHeaders := []string{
		"connection", "keep-alive", "proxy-authenticate",
		"proxy-authorization", "te", "trailers", "transfer-encoding", "upgrade",
	}
	
	headerLower := strings.ToLower(header)
	for _, hopByHop := range hopByHopHeaders {
		if headerLower == hopByHop {
			return true
		}
	}
	return false
}

func generateRequestID() string {
	// Simple request ID generation - in production, consider using UUID
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}

// GetStats returns routing engine statistics
func (e *Engine) GetStats() map[string]interface{} {
	e.cbMutex.RLock()
	defer e.cbMutex.RUnlock()

	stats := map[string]interface{}{
		"load_balancer":     e.balancer.Name(),
		"circuit_breakers":  len(e.circuitBreakers),
		"sticky_sessions":   e.stickySession != nil,
	}

	// Add circuit breaker stats
	cbStats := make(map[string]interface{})
	for instanceID, cb := range e.circuitBreakers {
		cbStats[instanceID] = map[string]interface{}{
			"state": cb.State().String(),
			"counts": cb.Counts(),
		}
	}
	stats["circuit_breaker_states"] = cbStats

	return stats
}
