// Package telemetry provides Fiber middleware for OpenTelemetry instrumentation
package telemetry

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// FiberTracingConfig holds configuration for Fiber tracing middleware
type FiberTracingConfig struct {
	Tracer          trace.Tracer
	SpanNameFunc    func(c *fiber.Ctx) string
	FilterFunc      func(c *fiber.Ctx) bool
	AttributeFunc   func(c *fiber.Ctx) []attribute.KeyValue
	SkipPaths       []string
	ServiceName     string
	ServiceTier     string
}

// DefaultFiberTracingConfig returns default configuration for Fiber tracing
func DefaultFiberTracingConfig(serviceName, serviceTier string) FiberTracingConfig {
	return FiberTracingConfig{
		Tracer: otel.Tracer(serviceName),
		SpanNameFunc: func(c *fiber.Ctx) string {
			return fmt.Sprintf("%s %s", c.Method(), c.Route().Path)
		},
		FilterFunc: nil,
		AttributeFunc: func(c *fiber.Ctx) []attribute.KeyValue {
			return []attribute.KeyValue{
				semconv.HTTPMethod(c.Method()),
				semconv.HTTPURL(c.OriginalURL()),
				semconv.HTTPRoute(c.Route().Path),
				semconv.HTTPScheme(c.Protocol()),
				semconv.HTTPHost(c.Hostname()),
				semconv.HTTPUserAgent(c.Get("User-Agent")),
				semconv.NetHostPort(c.Port()),
				attribute.String("service.tier", serviceTier),
				attribute.String("platform.name", "pyairtable"),
			}
		},
		SkipPaths: []string{
			"/health",
			"/metrics",
			"/ready",
		},
		ServiceName: serviceName,
		ServiceTier: serviceTier,
	}
}

// FiberTracing returns a Fiber middleware for OpenTelemetry tracing
func FiberTracing(config FiberTracingConfig) fiber.Handler {
	if config.SpanNameFunc == nil {
		config.SpanNameFunc = DefaultFiberTracingConfig("", "").SpanNameFunc
	}
	if config.AttributeFunc == nil {
		config.AttributeFunc = DefaultFiberTracingConfig("", "").AttributeFunc
	}

	return func(c *fiber.Ctx) error {
		// Skip tracing for certain paths
		for _, skipPath := range config.SkipPaths {
			if c.Path() == skipPath {
				return c.Next()
			}
		}

		// Apply filter if provided
		if config.FilterFunc != nil && !config.FilterFunc(c) {
			return c.Next()
		}

		// Extract trace context from headers
		ctx := otel.GetTextMapPropagator().Extract(c.Context(), &fiberCarrier{c: c})

		// Start span
		spanName := config.SpanNameFunc(c)
		ctx, span := config.Tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(config.AttributeFunc(c)...),
		)
		defer span.End()

		// Add span context to Fiber context
		c.SetUserContext(ctx)

		// Add trace ID to response headers for correlation
		if span.SpanContext().IsValid() {
			c.Set("X-Trace-ID", span.SpanContext().TraceID().String())
			c.Set("X-Span-ID", span.SpanContext().SpanID().String())
		}

		// Record request start time
		startTime := time.Now()

		// Continue with request
		err := c.Next()

		// Record response information
		duration := time.Since(startTime)
		statusCode := c.Response().StatusCode()

		// Set span attributes based on response
		span.SetAttributes(
			semconv.HTTPStatusCode(statusCode),
			semconv.HTTPResponseSize(int(c.Response().Header.ContentLength())),
			attribute.String("http.request_id", c.Get("X-Request-ID")),
			attribute.Float64("http.duration_ms", float64(duration.Nanoseconds())/1000000),
		)

		// Set span status based on HTTP status code
		if statusCode >= 400 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", statusCode))
			if statusCode >= 500 {
				span.SetAttributes(attribute.Bool("error", true))
			}
		} else {
			span.SetStatus(codes.Ok, "")
		}

		// Record error if present
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.SetAttributes(attribute.Bool("error", true))
		}

		// Add business metrics
		recordBusinessMetrics(span, c, duration, statusCode)

		return err
	}
}

// fiberCarrier implements TextMapCarrier for Fiber context
type fiberCarrier struct {
	c *fiber.Ctx
}

func (fc *fiberCarrier) Get(key string) string {
	return fc.c.Get(key)
}

func (fc *fiberCarrier) Set(key, value string) {
	fc.c.Set(key, value)
}

func (fc *fiberCarrier) Keys() []string {
	var keys []string
	fc.c.Request().Header.VisitAll(func(key, value []byte) {
		keys = append(keys, string(key))
	})
	return keys
}

// recordBusinessMetrics adds business-specific metrics to spans
func recordBusinessMetrics(span trace.Span, c *fiber.Ctx, duration time.Duration, statusCode int) {
	// Add cost tracking attributes
	span.SetAttributes(
		attribute.String("cost.service", extractServiceFromPath(c.Path())),
		attribute.Float64("cost.duration_seconds", duration.Seconds()),
	)

	// Add user context if available
	if userID := c.Get("X-User-ID"); userID != "" {
		span.SetAttributes(attribute.String("user.id", userID))
	}

	if tenantID := c.Get("X-Tenant-ID"); tenantID != "" {
		span.SetAttributes(attribute.String("tenant.id", tenantID))
	}

	// Add API key context for rate limiting insights
	if apiKey := c.Get("X-API-Key"); apiKey != "" {
		// Hash the API key for privacy
		span.SetAttributes(attribute.String("api.key_hash", hashAPIKey(apiKey)))
	}

	// Add workflow context if available
	if workflowID := c.Get("X-Workflow-ID"); workflowID != "" {
		span.SetAttributes(attribute.String("workflow.id", workflowID))
	}

	// Record performance buckets for alerting
	performanceBucket := getPerformanceBucket(duration)
	span.SetAttributes(attribute.String("performance.bucket", performanceBucket))
}

// extractServiceFromPath determines the service type from the request path
func extractServiceFromPath(path string) string {
	if len(path) < 2 {
		return "unknown"
	}

	switch {
	case containsAny(path, []string{"/ai", "/llm", "/chat", "/generate"}):
		return "ai-ml"
	case containsAny(path, []string{"/auth", "/login", "/register", "/token"}):
		return "auth"
	case containsAny(path, []string{"/workflow", "/automation", "/file"}):
		return "automation"
	case containsAny(path, []string{"/airtable", "/table", "/record"}):
		return "airtable"
	case containsAny(path, []string{"/analytics", "/metrics", "/reports"}):
		return "analytics"
	default:
		return "platform"
	}
}

// getPerformanceBucket categorizes request duration for monitoring
func getPerformanceBucket(duration time.Duration) string {
	switch {
	case duration < 100*time.Millisecond:
		return "fast"
	case duration < 500*time.Millisecond:
		return "normal"
	case duration < 2*time.Second:
		return "slow"
	case duration < 10*time.Second:
		return "very_slow"
	default:
		return "timeout_risk"
	}
}

// hashAPIKey creates a simple hash of the API key for tracking without exposing the key
func hashAPIKey(apiKey string) string {
	if len(apiKey) < 8 {
		return "short_key"
	}
	return fmt.Sprintf("%s***%s", apiKey[:3], apiKey[len(apiKey)-3:])
}

// containsAny checks if string contains any of the substrings
func containsAny(s string, substrings []string) bool {
	for _, substring := range substrings {
		if contains(s, substring) {
			return true
		}
	}
	return false
}

// contains checks if string contains substring (simple implementation)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) != -1
}

// findSubstring finds substring in string (simple implementation)
func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// DatabaseSpan creates a database operation span
func DatabaseSpan(ctx context.Context, tracer trace.Tracer, operation, table string) (context.Context, trace.Span) {
	spanName := fmt.Sprintf("db.%s %s", operation, table)
	ctx, span := tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.DBSystem("postgresql"),
			semconv.DBOperation(operation),
			semconv.DBSQLTable(table),
		),
	)
	return ctx, span
}

// RedisSpan creates a Redis operation span
func RedisSpan(ctx context.Context, tracer trace.Tracer, operation, key string) (context.Context, trace.Span) {
	spanName := fmt.Sprintf("redis.%s", operation)
	ctx, span := tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.DBSystem("redis"),
			semconv.DBOperation(operation),
			attribute.String("redis.key", key),
		),
	)
	return ctx, span
}

// HTTPClientSpan creates an HTTP client span
func HTTPClientSpan(ctx context.Context, tracer trace.Tracer, method, url string) (context.Context, trace.Span) {
	spanName := fmt.Sprintf("%s %s", method, extractHostFromURL(url))
	ctx, span := tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.HTTPMethod(method),
			semconv.HTTPURL(url),
		),
	)
	return ctx, span
}

// extractHostFromURL extracts hostname from URL for span naming
func extractHostFromURL(url string) string {
	// Simple URL parsing for hostname extraction
	if len(url) > 8 && url[:8] == "https://" {
		rest := url[8:]
		if idx := findSubstring(rest, "/"); idx != -1 {
			return rest[:idx]
		}
		return rest
	}
	if len(url) > 7 && url[:7] == "http://" {
		rest := url[7:]
		if idx := findSubstring(rest, "/"); idx != -1 {
			return rest[:idx]
		}
		return rest
	}
	return "unknown"
}