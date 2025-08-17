package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

const (
	TraceIDHeader      = "X-Trace-ID"
	SpanIDHeader       = "X-Span-ID"
	ParentSpanIDHeader = "X-Parent-Span-ID"
	TracestateHeader   = "Tracestate"
	TraceparentHeader  = "Traceparent"
)

// TracingMiddleware handles distributed tracing with x-trace-id propagation
type TracingMiddleware struct {
	logger *zap.Logger
}

// NewTracingMiddleware creates a new tracing middleware instance
func NewTracingMiddleware(logger *zap.Logger) *TracingMiddleware {
	return &TracingMiddleware{
		logger: logger,
	}
}

// TraceID generates a new random trace ID
func generateTraceID() string {
	bytes := make([]byte, 16)
	_, err := rand.Read(bytes)
	if err != nil {
		// Fallback to timestamp-based ID
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000")))
	}
	return hex.EncodeToString(bytes)
}

// SpanID generates a new random span ID  
func generateSpanID() string {
	bytes := make([]byte, 8)
	_, err := rand.Read(bytes)
	if err != nil {
		// Fallback to timestamp-based ID
		return hex.EncodeToString([]byte(time.Now().Format("15040500")))
	}
	return hex.EncodeToString(bytes)
}

// Handler returns the tracing middleware handler
func (tm *TracingMiddleware) Handler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Extract or generate trace ID
		traceID := c.Get(TraceIDHeader)
		if traceID == "" {
			traceID = generateTraceID()
		}

		// Extract or generate span ID
		spanID := generateSpanID()
		
		// Get parent span ID if present
		parentSpanID := c.Get(SpanIDHeader)

		// Set trace headers in request context
		c.Set(TraceIDHeader, traceID)
		c.Set(SpanIDHeader, spanID)
		if parentSpanID != "" {
			c.Set(ParentSpanIDHeader, parentSpanID)
		}

		// Store trace context in locals for downstream use
		c.Locals("traceID", traceID)
		c.Locals("spanID", spanID)
		c.Locals("parentSpanID", parentSpanID)

		// Create structured trace context
		traceContext := map[string]interface{}{
			"trace_id":        traceID,
			"span_id":         spanID,
			"parent_span_id":  parentSpanID,
			"service":         "api-gateway",
			"operation":       c.Method() + " " + c.Path(),
			"start_time":      start,
		}

		// Add trace context to locals
		c.Locals("traceContext", traceContext)

		// Log request start with tracing context
		tm.logger.Info("Request started",
			zap.String("trace_id", traceID),
			zap.String("span_id", spanID),
			zap.String("parent_span_id", parentSpanID),
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.String("remote_addr", c.IP()),
			zap.String("user_agent", c.Get("User-Agent")),
		)

		// Continue processing request
		err := c.Next()

		// Calculate duration
		duration := time.Since(start)

		// Log request completion with tracing context
		tm.logger.Info("Request completed",
			zap.String("trace_id", traceID),
			zap.String("span_id", spanID),
			zap.String("parent_span_id", parentSpanID),
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.Int("status_code", c.Response().StatusCode()),
			zap.Duration("duration", duration),
			zap.Int("response_size", len(c.Response().Body())),
		)

		// Set response headers for trace propagation
		c.Set(TraceIDHeader, traceID)
		c.Set(SpanIDHeader, spanID)

		// Create OpenTelemetry compatible traceparent header
		// Format: version-trace_id-parent_id-trace_flags
		// We'll use sampling flag (01) for now
		traceparent := "00-" + traceID + "-" + spanID + "-01"
		c.Set(TraceparentHeader, traceparent)

		return err
	}
}

// PropagateHeaders ensures tracing headers are propagated to downstream services
func (tm *TracingMiddleware) PropagateHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get trace context from locals
		traceID, _ := c.Locals("traceID").(string)
		spanID, _ := c.Locals("spanID").(string)
		
		if traceID == "" {
			traceID = generateTraceID()
			c.Locals("traceID", traceID)
		}
		
		if spanID == "" {
			spanID = generateSpanID()
			c.Locals("spanID", spanID)
		}

		// Set headers for propagation to downstream services
		c.Request().Header.Set(TraceIDHeader, traceID)
		c.Request().Header.Set(SpanIDHeader, spanID)
		
		// Create traceparent for OpenTelemetry compatibility
		traceparent := "00-" + traceID + "-" + spanID + "-01"
		c.Request().Header.Set(TraceparentHeader, traceparent)

		tm.logger.Debug("Propagating trace headers",
			zap.String("trace_id", traceID),
			zap.String("span_id", spanID),
			zap.String("path", c.Path()),
		)

		return c.Next()
	}
}

// GetTraceContext extracts trace context from fiber context
func GetTraceContext(c *fiber.Ctx) map[string]interface{} {
	if traceContext, ok := c.Locals("traceContext").(map[string]interface{}); ok {
		return traceContext
	}
	
	// Fallback: create basic trace context
	traceID, _ := c.Locals("traceID").(string)
	spanID, _ := c.Locals("spanID").(string)
	
	if traceID == "" {
		traceID = "unknown"
	}
	if spanID == "" {
		spanID = "unknown"
	}
	
	return map[string]interface{}{
		"trace_id": traceID,
		"span_id":  spanID,
		"service":  "api-gateway",
	}
}

// InjectTempoHeaders adds headers needed for Tempo ingestion
func (tm *TracingMiddleware) InjectTempoHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get or create trace context
		traceID, _ := c.Locals("traceID").(string)
		spanID, _ := c.Locals("spanID").(string)
		
		if traceID == "" {
			traceID = generateTraceID()
			c.Locals("traceID", traceID)
		}
		
		if spanID == "" {
			spanID = generateSpanID()
			c.Locals("spanID", spanID)
		}

		// Set Tempo-compatible headers
		c.Request().Header.Set("X-Tempo-Trace-ID", traceID)
		c.Request().Header.Set("X-Tempo-Span-ID", spanID)
		
		// Set standard tracing headers
		c.Request().Header.Set(TraceIDHeader, traceID)
		c.Request().Header.Set(SpanIDHeader, spanID)
		
		// Add service name for Tempo
		c.Request().Header.Set("X-Service-Name", "api-gateway")
		c.Request().Header.Set("X-Operation-Name", c.Method()+" "+c.Path())

		return c.Next()
	}
}