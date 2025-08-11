// Package telemetry provides OpenTelemetry instrumentation for PyAirtable Go services
package telemetry

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Config holds telemetry configuration
type Config struct {
	ServiceName      string
	ServiceVersion   string
	ServiceTier      string
	Environment      string
	OTLPEndpoint     string
	SamplingRatio    float64
	EnableDebug      bool
	ResourceAttributes map[string]string
}

// TracingProvider manages OpenTelemetry tracing
type TracingProvider struct {
	provider *trace.TracerProvider
	config   *Config
	logger   *zap.Logger
}

// NewConfig creates a new telemetry configuration with defaults
func NewConfig(serviceName string) *Config {
	return &Config{
		ServiceName:      serviceName,
		ServiceVersion:   getEnv("SERVICE_VERSION", "1.0.0"),
		ServiceTier:      getEnv("SERVICE_TIER", "application"),
		Environment:      getEnv("ENVIRONMENT", "development"),
		OTLPEndpoint:     getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317"),
		SamplingRatio:    getEnvFloat("OTEL_SAMPLING_RATIO", 0.1), // 10% default sampling
		EnableDebug:      getEnv("OTEL_DEBUG", "false") == "true",
		ResourceAttributes: make(map[string]string),
	}
}

// InitializeTracing sets up OpenTelemetry tracing
func InitializeTracing(config *Config, logger *zap.Logger) (*TracingProvider, error) {
	// Create resource with service information
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			// Standard OpenTelemetry attributes
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			semconv.DeploymentEnvironment(config.Environment),
			
			// PyAirtable specific attributes
			attribute.String("platform.name", "pyairtable"),
			attribute.String("platform.tier", config.ServiceTier),
			attribute.String("platform.cluster", "pyairtable-platform"),
		),
		resource.WithHost(),
		resource.WithProcess(),
		resource.WithContainer(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Add custom resource attributes
	attrs := res.Attributes()
	for key, value := range config.ResourceAttributes {
		attrs = append(attrs, attribute.String(key, value))
	}

	// Create OTLP exporter
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, config.OTLPEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		logger.Warn("Failed to connect to OTLP collector, using noop tracer",
			zap.String("endpoint", config.OTLPEndpoint),
			zap.Error(err))
		return &TracingProvider{
			provider: trace.NewTracerProvider(), // noop provider
			config:   config,
			logger:   logger,
		}, nil
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Configure sampling
	var sampler trace.Sampler
	if config.Environment == "production" {
		// Use parent-based sampling in production
		sampler = trace.ParentBased(trace.TraceIDRatioBased(config.SamplingRatio))
	} else {
		// Always sample in development
		sampler = trace.AlwaysSample()
	}

	// Create tracer provider
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter,
			trace.WithBatchTimeout(time.Second),
			trace.WithMaxExportBatchSize(512),
		),
		trace.WithResource(res),
		trace.WithSampler(sampler),
	)

	// Set global tracer provider
	otel.SetTracerProvider(tp)

	// Set global propagator for context propagation
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logger.Info("OpenTelemetry tracing initialized",
		zap.String("service", config.ServiceName),
		zap.String("endpoint", config.OTLPEndpoint),
		zap.Float64("sampling_ratio", config.SamplingRatio),
		zap.String("environment", config.Environment),
	)

	return &TracingProvider{
		provider: tp,
		config:   config,
		logger:   logger,
	}, nil
}

// Shutdown gracefully shuts down the tracer provider
func (tp *TracingProvider) Shutdown(ctx context.Context) error {
	if tp.provider == nil {
		return nil
	}

	tp.logger.Info("Shutting down OpenTelemetry tracer provider")
	return tp.provider.Shutdown(ctx)
}

// GetTracer returns a tracer for the given name
func (tp *TracingProvider) GetTracer(name string) otel.Tracer {
	return otel.Tracer(name)
}

// AddCustomAttributes adds custom attributes to spans
func AddCustomAttributes(span otel.Span, attrs map[string]interface{}) {
	for key, value := range attrs {
		switch v := value.(type) {
		case string:
			span.SetAttributes(attribute.String(key, v))
		case int:
			span.SetAttributes(attribute.Int(key, v))
		case int64:
			span.SetAttributes(attribute.Int64(key, v))
		case float64:
			span.SetAttributes(attribute.Float64(key, v))
		case bool:
			span.SetAttributes(attribute.Bool(key, v))
		default:
			span.SetAttributes(attribute.String(key, fmt.Sprintf("%v", v)))
		}
	}
}

// TraceHTTPRequest adds HTTP request attributes to span
func TraceHTTPRequest(span otel.Span, method, url, userAgent string, statusCode int) {
	span.SetAttributes(
		semconv.HTTPMethod(method),
		semconv.HTTPURL(url),
		semconv.HTTPStatusCode(statusCode),
		semconv.HTTPUserAgent(userAgent),
	)

	// Set span status based on HTTP status code
	if statusCode >= 400 {
		span.SetAttributes(attribute.Bool("error", true))
		if statusCode >= 500 {
			span.SetName(fmt.Sprintf("HTTP %s (ERROR)", method))
		} else {
			span.SetName(fmt.Sprintf("HTTP %s (CLIENT_ERROR)", method))
		}
	}
}

// TraceDatabase adds database operation attributes to span
func TraceDatabase(span otel.Span, operation, table, query string, rowsAffected int64) {
	span.SetAttributes(
		semconv.DBSystem("postgresql"),
		semconv.DBOperation(operation),
		semconv.DBSQLTable(table),
		attribute.Int64("db.rows_affected", rowsAffected),
	)

	// Only include query in development
	if os.Getenv("ENVIRONMENT") != "production" {
		span.SetAttributes(semconv.DBStatement(query))
	}
}

// TraceRedis adds Redis operation attributes to span
func TraceRedis(span otel.Span, operation, key string, ttl time.Duration) {
	span.SetAttributes(
		semconv.DBSystem("redis"),
		semconv.DBOperation(operation),
		attribute.String("redis.key", key),
	)

	if ttl > 0 {
		span.SetAttributes(attribute.String("redis.ttl", ttl.String()))
	}
}

// TraceAIRequest adds AI/ML request attributes to span
func TraceAIRequest(span otel.Span, provider, model string, inputTokens, outputTokens int, cost float64) {
	span.SetAttributes(
		attribute.String("ai.provider", provider),
		attribute.String("ai.model", model),
		attribute.Int("ai.input_tokens", inputTokens),
		attribute.Int("ai.output_tokens", outputTokens),
		attribute.Float64("ai.cost_usd", cost),
		attribute.String("ai.tier", "llm"),
	)
}

// TraceWorkflow adds workflow execution attributes to span
func TraceWorkflow(span otel.Span, workflowID, workflowType, status string, duration time.Duration) {
	span.SetAttributes(
		attribute.String("workflow.id", workflowID),
		attribute.String("workflow.type", workflowType),
		attribute.String("workflow.status", status),
		attribute.String("workflow.duration", duration.String()),
	)
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}