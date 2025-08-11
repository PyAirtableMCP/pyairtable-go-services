package observability

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// FiberTracingConfig holds configuration for Fiber tracing middleware
type FiberTracingConfig struct {
	ServiceName    string
	ServiceTier    string
	SkipPaths      []string
	CaptureHeaders []string
	CaptureBody    bool
	MaxBodySize    int
}

// DefaultFiberTracingConfig creates a default configuration for Fiber tracing
func DefaultFiberTracingConfig(serviceName, tier string) *FiberTracingConfig {
	return &FiberTracingConfig{
		ServiceName: serviceName,
		ServiceTier: tier,
		SkipPaths: []string{
			"/health",
			"/metrics",
			"/favicon.ico",
		},
		CaptureHeaders: []string{
			"User-Agent",
			"Content-Type",
			"Accept",
			"X-Forwarded-For",
			"X-Real-IP",
		},
		CaptureBody: false,
		MaxBodySize: 1024,
	}
}

// FiberTracing creates a Fiber middleware for OpenTelemetry tracing
func FiberTracing(config *FiberTracingConfig) fiber.Handler {
	tracer := otel.Tracer("pyairtable-fiber")
	propagator := otel.GetTextMapPropagator()

	return func(c *fiber.Ctx) error {
		// Skip certain paths
		path := c.Path()
		for _, skipPath := range config.SkipPaths {
			if path == skipPath {
				return c.Next()
			}
		}

		// Extract or generate correlation ID
		correlationID := c.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = uuid.New().String()
			c.Set("X-Correlation-ID", correlationID)
		}

		// Extract trace context from headers
		ctx := propagator.Extract(c.Context(), &fiberCarrier{c: c})

		// Create span
		spanName := fmt.Sprintf("%s %s", c.Method(), c.Route().Path)
		if spanName == " " {
			spanName = fmt.Sprintf("%s %s", c.Method(), path)
		}

		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				// HTTP attributes
				attribute.String("http.method", c.Method()),
				attribute.String("http.url", c.OriginalURL()),
				attribute.String("http.scheme", c.Protocol()),
				attribute.String("http.route", c.Route().Path),
				attribute.String("http.target", c.Path()),
				attribute.String("http.user_agent", c.Get("User-Agent")),
				attribute.String("http.remote_addr", c.IP()),
				
				// Service attributes
				attribute.String("service.name", config.ServiceName),
				attribute.String("service.tier", config.ServiceTier),
				
				// Correlation attributes
				attribute.String("correlation.id", correlationID),
				attribute.String("request.id", correlationID),
				
				// Network attributes
				attribute.String("net.peer.ip", c.IP()),
				attribute.String("net.host.name", c.Hostname()),
			),
		)

		// Add captured headers as attributes
		for _, header := range config.CaptureHeaders {
			if value := c.Get(header); value != "" {
				span.SetAttributes(attribute.String(fmt.Sprintf("http.request.header.%s", header), value))
			}
		}

		// Add query parameters if present
		if queryString := c.Context().QueryArgs().String(); queryString != "" {
			span.SetAttributes(attribute.String("http.query", queryString))
		}

		// Capture request body if configured
		if config.CaptureBody && len(c.Body()) > 0 && len(c.Body()) <= config.MaxBodySize {
			span.SetAttributes(attribute.String("http.request.body", string(c.Body())))
		}

		// Store context and span in Fiber context
		c.SetUserContext(ctx)
		c.Locals("trace_span", span)
		c.Locals("correlation_id", correlationID)

		// Record start time
		startTime := time.Now()

		// Process request
		err := c.Next()

		// Calculate duration
		duration := time.Since(startTime)

		// Set response attributes
		statusCode := c.Response().StatusCode()
		span.SetAttributes(
			attribute.Int("http.status_code", statusCode),
			attribute.Int64("http.request.duration_ms", duration.Milliseconds()),
			attribute.Int("http.response.size", len(c.Response().Body())),
			attribute.Int("http.request.size", len(c.Request().Body())),
		)

		// Set response headers for tracing
		c.Set("X-Trace-ID", span.SpanContext().TraceID().String())
		c.Set("X-Span-ID", span.SpanContext().SpanID().String())

		// Set span status based on HTTP status code
		if statusCode >= 400 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", statusCode))
			
			if err != nil {
				span.RecordError(err)
				span.SetAttributes(attribute.String("error.message", err.Error()))
			}
		} else {
			span.SetStatus(codes.Ok, "")
		}

		// Add business context if available in locals
		if userID, ok := c.Locals("user_id").(string); ok && userID != "" {
			span.SetAttributes(attribute.String("user.id", userID))
		}
		if tenantID, ok := c.Locals("tenant_id").(string); ok && tenantID != "" {
			span.SetAttributes(attribute.String("tenant.id", tenantID))
		}

		// End span
		span.End()

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
	keys := make([]string, 0)
	fc.c.Request().Header.VisitAll(func(key, value []byte) {
		keys = append(keys, string(key))
	})
	return keys
}

// RequestLogger creates a structured logger with trace context
func RequestLogger(c *fiber.Ctx, logger *Logger) *Logger {
	correlationID, _ := c.Locals("correlation_id").(string)
	if correlationID == "" {
		correlationID = "unknown"
	}

	return &Logger{
		Logger: logger.Logger.With(
			"correlation_id", correlationID,
			"method", c.Method(),
			"path", c.Path(),
			"ip", c.IP(),
			"user_agent", c.Get("User-Agent"),
		),
	}
}

// AddSpanEvent adds an event to the current span
func AddSpanEvent(c *fiber.Ctx, name string, attributes map[string]interface{}) {
	if span, ok := c.Locals("trace_span").(trace.Span); ok && span.IsRecording() {
		attrs := make([]attribute.KeyValue, 0, len(attributes))
		for k, v := range attributes {
			switch val := v.(type) {
			case string:
				attrs = append(attrs, attribute.String(k, val))
			case int:
				attrs = append(attrs, attribute.Int(k, val))
			case int64:
				attrs = append(attrs, attribute.Int64(k, val))
			case float64:
				attrs = append(attrs, attribute.Float64(k, val))
			case bool:
				attrs = append(attrs, attribute.Bool(k, val))
			}
		}
		span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

// RecordError records an error in the current span
func RecordError(c *fiber.Ctx, err error, attributes map[string]interface{}) {
	if span, ok := c.Locals("trace_span").(trace.Span); ok && span.IsRecording() {
		attrs := make([]attribute.KeyValue, 0, len(attributes))
		for k, v := range attributes {
			switch val := v.(type) {
			case string:
				attrs = append(attrs, attribute.String(k, val))
			case int:
				attrs = append(attrs, attribute.Int(k, val))
			case int64:
				attrs = append(attrs, attribute.Int64(k, val))
			case float64:
				attrs = append(attrs, attribute.Float64(k, val))
			case bool:
				attrs = append(attrs, attribute.Bool(k, val))
			}
		}
		span.RecordError(err, trace.WithAttributes(attrs...))
		span.SetStatus(codes.Error, err.Error())
	}
}

// GetTraceID returns the trace ID from the current request
func GetTraceID(c *fiber.Ctx) string {
	if span, ok := c.Locals("trace_span").(trace.Span); ok {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// GetSpanID returns the span ID from the current request
func GetSpanID(c *fiber.Ctx) string {
	if span, ok := c.Locals("trace_span").(trace.Span); ok {
		return span.SpanContext().SpanID().String()
	}
	return ""
}

// GetCorrelationID returns the correlation ID from the current request
func GetCorrelationID(c *fiber.Ctx) string {
	if correlationID, ok := c.Locals("correlation_id").(string); ok {
		return correlationID
	}
	return ""
}