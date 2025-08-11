package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// LogLevel represents different log levels
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
	LogLevelFatal LogLevel = "fatal"
)

// StructuredLogger provides structured logging with multiple outputs
type StructuredLogger struct {
	logger          *slog.Logger
	redis           *redis.Client
	config          *LogConfig
	requestIDHeader string
	tracer          trace.Tracer
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level              LogLevel `json:"level"`
	Format             string   `json:"format"` // json, text
	EnableConsole      bool     `json:"enable_console"`
	EnableFile         bool     `json:"enable_file"`
	EnableRedis        bool     `json:"enable_redis"`
	FilePath           string   `json:"file_path"`
	MaxFileSize        int      `json:"max_file_size"` // MB
	MaxFileAge         int      `json:"max_file_age"`  // days
	MaxBackups         int      `json:"max_backups"`
	CompressBackups    bool     `json:"compress_backups"`
	RedisKey           string   `json:"redis_key"`
	RedisTTL           time.Duration `json:"redis_ttl"`
	SamplingRate       float64  `json:"sampling_rate"` // 0.0 to 1.0
	EnableTracing      bool     `json:"enable_tracing"`
	IncludeStackTrace  bool     `json:"include_stack_trace"`
	MaskSensitiveData  bool     `json:"mask_sensitive_data"`
	SensitiveFields    []string `json:"sensitive_fields"`
}

// RequestLog represents a structured request log entry
type RequestLog struct {
	Timestamp        time.Time         `json:"timestamp"`
	RequestID        string            `json:"request_id"`
	TraceID          string            `json:"trace_id,omitempty"`
	SpanID           string            `json:"span_id,omitempty"`
	Method           string            `json:"method"`
	Path             string            `json:"path"`
	Query            string            `json:"query,omitempty"`
	StatusCode       int               `json:"status_code"`
	ResponseTime     float64           `json:"response_time_ms"`
	RequestSize      int               `json:"request_size_bytes"`
	ResponseSize     int               `json:"response_size_bytes"`
	UserAgent        string            `json:"user_agent"`
	RemoteIP         string            `json:"remote_ip"`
	ForwardedFor     string            `json:"forwarded_for,omitempty"`
	Referer          string            `json:"referer,omitempty"`
	UserID           string            `json:"user_id,omitempty"`
	TenantID         string            `json:"tenant_id,omitempty"`
	APIKey           string            `json:"api_key,omitempty"` // Masked
	Route            string            `json:"route,omitempty"`
	Upstream         string            `json:"upstream,omitempty"`
	Error            string            `json:"error,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	RequestBody      string            `json:"request_body,omitempty"`  // Masked sensitive data
	ResponseBody     string            `json:"response_body,omitempty"` // Masked sensitive data
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	Level            string            `json:"level"`
	Source           string            `json:"source"`
	Environment      string            `json:"environment"`
	Service          string            `json:"service"`
	Version          string            `json:"version"`
}

// ErrorLog represents a structured error log entry
type ErrorLog struct {
	Timestamp    time.Time              `json:"timestamp"`
	RequestID    string                 `json:"request_id"`
	TraceID      string                 `json:"trace_id,omitempty"`
	SpanID       string                 `json:"span_id,omitempty"`
	Level        string                 `json:"level"`
	Message      string                 `json:"message"`
	Error        string                 `json:"error"`
	ErrorType    string                 `json:"error_type"`
	StackTrace   string                 `json:"stack_trace,omitempty"`
	Context      map[string]interface{} `json:"context,omitempty"`
	Source       string                 `json:"source"`
	Function     string                 `json:"function,omitempty"`
	File         string                 `json:"file,omitempty"`
	Line         int                    `json:"line,omitempty"`
	Service      string                 `json:"service"`
	Environment  string                 `json:"environment"`
	Version      string                 `json:"version"`
}

// MetricLog represents a structured metric log entry
type MetricLog struct {
	Timestamp   time.Time              `json:"timestamp"`
	MetricName  string                 `json:"metric_name"`
	MetricType  string                 `json:"metric_type"` // counter, gauge, histogram, summary
	Value       float64                `json:"value"`
	Unit        string                 `json:"unit,omitempty"`
	Tags        map[string]string      `json:"tags,omitempty"`
	Labels      map[string]string      `json:"labels,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Service     string                 `json:"service"`
	Environment string                 `json:"environment"`
	Version     string                 `json:"version"`
}

// NewStructuredLogger creates a new structured logger
func NewStructuredLogger(config *LogConfig, redis *redis.Client) *StructuredLogger {
	// Create slog logger
	var handler slog.Handler
	
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	
	// Set log level
	switch config.Level {
	case LogLevelDebug:
		opts.Level = slog.LevelDebug
	case LogLevelInfo:
		opts.Level = slog.LevelInfo
	case LogLevelWarn:
		opts.Level = slog.LevelWarn
	case LogLevelError:
		opts.Level = slog.LevelError
	}
	
	// Create handler based on format
	if config.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	
	logger := slog.New(handler)
	
	// Initialize tracer
	tracer := otel.Tracer("api-gateway")
	
	return &StructuredLogger{
		logger:          logger,
		redis:           redis,
		config:          config,
		requestIDHeader: "X-Request-ID",
		tracer:          tracer,
	}
}

// RequestLoggingMiddleware creates a middleware for request logging
func (sl *StructuredLogger) RequestLoggingMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		
		// Generate request ID if not present
		requestID := c.Get(sl.requestIDHeader)
		if requestID == "" {
			requestID = sl.generateRequestID()
			c.Set(sl.requestIDHeader, requestID)
		}
		
		// Store request ID in locals for other middleware
		c.Locals("request_id", requestID)
		
		// Create span for tracing
		var span trace.Span
		var ctx context.Context
		if sl.config.EnableTracing {
			ctx, span = sl.tracer.Start(c.Context(), fmt.Sprintf("%s %s", c.Method(), c.Path()))
			span.SetAttributes(
				attribute.String("http.method", c.Method()),
				attribute.String("http.path", c.Path()),
				attribute.String("http.user_agent", c.Get("User-Agent")),
				attribute.String("http.remote_addr", c.IP()),
				attribute.String("request.id", requestID),
			)
			c.SetUserContext(ctx)
		}
		
		// Process request
		err := c.Next()
		
		// Calculate response time
		duration := time.Since(start)
		
		// Create request log
		requestLog := sl.createRequestLog(c, requestID, duration, err)
		
		// Add tracing information
		if span != nil {
			requestLog.TraceID = span.SpanContext().TraceID().String()
			requestLog.SpanID = span.SpanContext().SpanID().String()
			
			span.SetAttributes(
				attribute.Int("http.status_code", c.Response().StatusCode()),
				attribute.Float64("http.response_time_ms", duration.Seconds()*1000),
			)
			
			if err != nil {
				span.RecordError(err)
			}
			
			span.End()
		}
		
		// Log the request
		sl.logRequest(requestLog)
		
		return err
	}
}

// LogError logs an error with structured format
func (sl *StructuredLogger) LogError(ctx context.Context, err error, message string, fields ...interface{}) {
	requestID := sl.getRequestID(ctx)
	
	errorLog := &ErrorLog{
		Timestamp:   time.Now(),
		RequestID:   requestID,
		Level:       "error",
		Message:     message,
		Error:       err.Error(),
		Service:     "api-gateway",
		Environment: os.Getenv("ENVIRONMENT"),
		Version:     os.Getenv("VERSION"),
		Context:     make(map[string]interface{}),
	}
	
	// Add tracing information
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		errorLog.TraceID = span.SpanContext().TraceID().String()
		errorLog.SpanID = span.SpanContext().SpanID().String()
	}
	
	// Add additional fields
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			key := fmt.Sprintf("%v", fields[i])
			value := fields[i+1]
			errorLog.Context[key] = value
		}
	}
	
	// Log to slog
	sl.logger.Error(message,
		"request_id", requestID,
		"error", err.Error(),
		"context", errorLog.Context,
	)
	
	// Store in Redis for analytics
	if sl.config.EnableRedis && sl.redis != nil {
		sl.storeErrorLog(errorLog)
	}
}

// LogMetric logs a metric with structured format
func (sl *StructuredLogger) LogMetric(metricName, metricType string, value float64, tags map[string]string) {
	metricLog := &MetricLog{
		Timestamp:   time.Now(),
		MetricName:  metricName,
		MetricType:  metricType,
		Value:       value,
		Tags:        tags,
		Service:     "api-gateway",
		Environment: os.Getenv("ENVIRONMENT"),
		Version:     os.Getenv("VERSION"),
	}
	
	// Log to slog
	sl.logger.Info("metric",
		"metric_name", metricName,
		"metric_type", metricType,
		"value", value,
		"tags", tags,
	)
	
	// Store in Redis for analytics
	if sl.config.EnableRedis && sl.redis != nil {
		sl.storeMetricLog(metricLog)
	}
}

// createRequestLog creates a structured request log entry
func (sl *StructuredLogger) createRequestLog(c *fiber.Ctx, requestID string, duration time.Duration, err error) *RequestLog {
	requestLog := &RequestLog{
		Timestamp:    time.Now(),
		RequestID:    requestID,
		Method:       c.Method(),
		Path:         c.Path(),
		Query:        string(c.Request().URI().QueryString()),
		StatusCode:   c.Response().StatusCode(),
		ResponseTime: duration.Seconds() * 1000, // Convert to milliseconds
		RequestSize:  len(c.Body()),
		ResponseSize: len(c.Response().Body()),
		UserAgent:    c.Get("User-Agent"),
		RemoteIP:     c.IP(),
		ForwardedFor: c.Get("X-Forwarded-For"),
		Referer:      c.Get("Referer"),
		Level:        "info",
		Source:       "request",
		Service:      "api-gateway",
		Environment:  os.Getenv("ENVIRONMENT"),
		Version:      os.Getenv("VERSION"),
		Metadata:     make(map[string]interface{}),
	}
	
	// Add error information
	if err != nil {
		requestLog.Error = err.Error()
		requestLog.Level = "error"
	}
	
	// Extract user information from context
	if userClaims := c.Locals("user_claims"); userClaims != nil {
		if claims, ok := userClaims.(map[string]interface{}); ok {
			if userID, ok := claims["user_id"].(string); ok {
				requestLog.UserID = userID
			}
			if tenantID, ok := claims["tenant_id"].(string); ok {
				requestLog.TenantID = tenantID
			}
		}
	}
	
	// Mask API key
	if apiKey := c.Get("X-API-Key"); apiKey != "" {
		requestLog.APIKey = sl.maskSensitiveData(apiKey)
	}
	
	// Add route information
	if route := c.Route(); route != nil {
		requestLog.Route = route.Path
	}
	
	// Add upstream information
	if upstream := c.Locals("upstream"); upstream != nil {
		requestLog.Upstream = fmt.Sprintf("%v", upstream)
	}
	
	// Add headers (filtered)
	headers := make(map[string]string)
	c.Request().Header.VisitAll(func(key, value []byte) {
		headerName := string(key)
		headerValue := string(value)
		
		// Filter sensitive headers
		if sl.config.MaskSensitiveData && sl.isSensitiveHeader(headerName) {
			headerValue = sl.maskSensitiveData(headerValue)
		}
		
		headers[headerName] = headerValue
	})
	requestLog.Headers = headers
	
	// Add request/response bodies if configured (with masking)
	if sl.shouldLogBody(c) {
		requestBody := string(c.Body())
		responseBody := string(c.Response().Body())
		
		if sl.config.MaskSensitiveData {
			requestBody = sl.maskSensitiveDataInBody(requestBody)
			responseBody = sl.maskSensitiveDataInBody(responseBody)
		}
		
		requestLog.RequestBody = requestBody
		requestLog.ResponseBody = responseBody
	}
	
	return requestLog
}

// logRequest logs a request using multiple outputs
func (sl *StructuredLogger) logRequest(requestLog *RequestLog) {
	// Sample requests based on sampling rate
	if sl.config.SamplingRate < 1.0 && sl.shouldSample() {
		return
	}
	
	// Log to slog
	logLevel := slog.LevelInfo
	if requestLog.Level == "error" {
		logLevel = slog.LevelError
	}
	
	sl.logger.Log(context.Background(), logLevel, "request",
		"request_id", requestLog.RequestID,
		"method", requestLog.Method,
		"path", requestLog.Path,
		"status_code", requestLog.StatusCode,
		"response_time_ms", requestLog.ResponseTime,
		"remote_ip", requestLog.RemoteIP,
		"user_agent", requestLog.UserAgent,
		"error", requestLog.Error,
	)
	
	// Store in Redis for analytics
	if sl.config.EnableRedis && sl.redis != nil {
		sl.storeRequestLog(requestLog)
	}
}

// storeRequestLog stores request log in Redis
func (sl *StructuredLogger) storeRequestLog(requestLog *RequestLog) {
	key := fmt.Sprintf("%s:request:%s", sl.config.RedisKey, requestLog.RequestID)
	data, err := json.Marshal(requestLog)
	if err != nil {
		sl.logger.Error("Failed to marshal request log", "error", err)
		return
	}
	
	err = sl.redis.Set(context.Background(), key, data, sl.config.RedisTTL).Err()
	if err != nil {
		sl.logger.Error("Failed to store request log in Redis", "error", err)
	}
	
	// Also add to time-series for analytics
	timeKey := fmt.Sprintf("%s:requests:%s", sl.config.RedisKey, time.Now().Format("2006-01-02-15"))
	sl.redis.RPush(context.Background(), timeKey, data)
	sl.redis.Expire(context.Background(), timeKey, 24*time.Hour)
}

// storeErrorLog stores error log in Redis
func (sl *StructuredLogger) storeErrorLog(errorLog *ErrorLog) {
	key := fmt.Sprintf("%s:error:%s:%d", sl.config.RedisKey, errorLog.RequestID, time.Now().UnixNano())
	data, err := json.Marshal(errorLog)
	if err != nil {
		sl.logger.Error("Failed to marshal error log", "error", err)
		return
	}
	
	err = sl.redis.Set(context.Background(), key, data, sl.config.RedisTTL).Err()
	if err != nil {
		sl.logger.Error("Failed to store error log in Redis", "error", err)
	}
	
	// Also add to error time-series
	timeKey := fmt.Sprintf("%s:errors:%s", sl.config.RedisKey, time.Now().Format("2006-01-02-15"))
	sl.redis.RPush(context.Background(), timeKey, data)
	sl.redis.Expire(context.Background(), timeKey, 24*time.Hour)
}

// storeMetricLog stores metric log in Redis
func (sl *StructuredLogger) storeMetricLog(metricLog *MetricLog) {
	key := fmt.Sprintf("%s:metric:%s:%d", sl.config.RedisKey, metricLog.MetricName, time.Now().UnixNano())
	data, err := json.Marshal(metricLog)
	if err != nil {
		sl.logger.Error("Failed to marshal metric log", "error", err)
		return
	}
	
	err = sl.redis.Set(context.Background(), key, data, sl.config.RedisTTL).Err()
	if err != nil {
		sl.logger.Error("Failed to store metric log in Redis", "error", err)
	}
}

// Helper methods
func (sl *StructuredLogger) generateRequestID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().UnixNano()%1000)
}

func (sl *StructuredLogger) getRequestID(ctx context.Context) string {
	if requestID := ctx.Value("request_id"); requestID != nil {
		return fmt.Sprintf("%v", requestID)
	}
	return ""
}

func (sl *StructuredLogger) shouldSample() bool {
	// Simple sampling based on current time nanoseconds
	return float64(time.Now().UnixNano()%1000)/1000.0 < sl.config.SamplingRate
}

func (sl *StructuredLogger) shouldLogBody(c *fiber.Ctx) bool {
	// Don't log bodies for certain content types or large bodies
	contentType := c.Get("Content-Type")
	if strings.Contains(contentType, "multipart/form-data") ||
		strings.Contains(contentType, "application/octet-stream") {
		return false
	}
	
	// Don't log large bodies
	if len(c.Body()) > 10*1024 { // 10KB
		return false
	}
	
	return true
}

func (sl *StructuredLogger) isSensitiveHeader(headerName string) bool {
	sensitiveHeaders := map[string]bool{
		"authorization": true,
		"x-api-key":     true,
		"cookie":        true,
		"set-cookie":    true,
		"x-auth-token":  true,
	}
	
	return sensitiveHeaders[strings.ToLower(headerName)]
}

func (sl *StructuredLogger) maskSensitiveData(data string) string {
	if len(data) <= 8 {
		return "***"
	}
	return data[:4] + "***" + data[len(data)-4:]
}

func (sl *StructuredLogger) maskSensitiveDataInBody(body string) string {
	if sl.config.MaskSensitiveData {
		// Simple masking - in production, use more sophisticated JSON/XML parsing
		for _, field := range sl.config.SensitiveFields {
			body = strings.ReplaceAll(body, field, "***MASKED***")
		}
	}
	return body
}