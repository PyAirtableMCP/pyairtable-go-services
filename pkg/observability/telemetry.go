package observability

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace/embedded"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TracingConfig holds configuration for OpenTelemetry tracing
type TracingConfig struct {
	ServiceName     string
	ServiceVersion  string
	ServiceTier     string
	Environment     string
	OTLPEndpoint    string
	SamplingRatio   float64
	ResourceLabels  map[string]string
}

// NewConfig creates a default tracing configuration
func NewConfig(serviceName string) *TracingConfig {
	return &TracingConfig{
		ServiceName:    serviceName,
		ServiceVersion: getEnvOrDefault("SERVICE_VERSION", "1.0.0"),
		ServiceTier:    getEnvOrDefault("SERVICE_TIER", "application"),
		Environment:    getEnvOrDefault("ENVIRONMENT", "development"),
		OTLPEndpoint:   getEnvOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317"),
		SamplingRatio:  getSamplingRatio(),
		ResourceLabels: make(map[string]string),
	}
}

// TelemetryProvider wraps the OpenTelemetry tracer provider
type TelemetryProvider struct {
	provider *trace.TracerProvider
	logger   *zap.Logger
}

// InitializeTracing sets up OpenTelemetry tracing with comprehensive configuration
func InitializeTracing(config *TracingConfig, logger *zap.Logger) (*TelemetryProvider, error) {
	ctx := context.Background()

	// Create OTLP exporter
	conn, err := grpc.DialContext(ctx, config.OTLPEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection to collector: %w", err)
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			// Service attributes
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			semconv.DeploymentEnvironment(config.Environment),
			
			// Platform attributes
			attribute.String("platform.name", "pyairtable"),
			attribute.String("service.tier", config.ServiceTier),
			
			// Runtime attributes
			semconv.ProcessPID(os.Getpid()),
			semconv.ProcessExecutableName(os.Args[0]),
		),
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Add custom resource labels
	if len(config.ResourceLabels) > 0 {
		attrs := make([]attribute.KeyValue, 0, len(config.ResourceLabels))
		for k, v := range config.ResourceLabels {
			attrs = append(attrs, attribute.String(k, v))
		}
		res, _ = resource.Merge(res, resource.NewWithAttributes("", attrs...))
	}

	// Configure sampling
	var sampler trace.Sampler
	if config.Environment == "development" {
		sampler = trace.AlwaysSample() // Always sample in development
	} else {
		// Use parent-based sampling with ratio for production
		sampler = trace.ParentBased(trace.TraceIDRatioBased(config.SamplingRatio))
	}

	// Create tracer provider
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter,
			trace.WithBatchTimeout(2*time.Second),
			trace.WithMaxExportBatchSize(512),
			trace.WithMaxQueueSize(2048),
		),
		trace.WithResource(res),
		trace.WithSampler(sampler),
	)

	// Register as global tracer provider
	otel.SetTracerProvider(tp)

	// Set global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logger.Info("OpenTelemetry tracing initialized",
		zap.String("service", config.ServiceName),
		zap.String("environment", config.Environment),
		zap.String("endpoint", config.OTLPEndpoint),
		zap.Float64("sampling_ratio", config.SamplingRatio),
	)

	return &TelemetryProvider{
		provider: tp,
		logger:   logger,
	}, nil
}

// Shutdown gracefully shuts down the tracer provider
func (tp *TelemetryProvider) Shutdown(ctx context.Context) error {
	if tp.provider != nil {
		return tp.provider.Shutdown(ctx)
	}
	return nil
}

// TraceOptions holds options for creating spans
type TraceOptions struct {
	SpanKind   string
	Attributes map[string]interface{}
}

// StartSpan creates a new span with common attributes
func StartSpan(ctx context.Context, name string, opts ...*TraceOptions) (context.Context, func()) {
	tracer := otel.Tracer("pyairtable")
	
	spanCtx, span := tracer.Start(ctx, name)
	
	// Apply options
	for _, opt := range opts {
		if opt.Attributes != nil {
			for k, v := range opt.Attributes {
				switch val := v.(type) {
				case string:
					span.SetAttributes(attribute.String(k, val))
				case int:
					span.SetAttributes(attribute.Int(k, val))
				case int64:
					span.SetAttributes(attribute.Int64(k, val))
				case float64:
					span.SetAttributes(attribute.Float64(k, val))
				case bool:
					span.SetAttributes(attribute.Bool(k, val))
				}
			}
		}
	}
	
	return spanCtx, func() {
		span.End()
	}
}

// AddBusinessAttributes adds business context to the current span
func AddBusinessAttributes(ctx context.Context, userID, tenantID, correlationID string) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	
	if userID != "" {
		span.SetAttributes(attribute.String("user.id", userID))
	}
	if tenantID != "" {
		span.SetAttributes(attribute.String("tenant.id", tenantID))
	}
	if correlationID != "" {
		span.SetAttributes(attribute.String("correlation.id", correlationID))
	}
}

// AddDatabaseAttributes adds database operation context
func AddDatabaseAttributes(ctx context.Context, operation, table string, duration time.Duration) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	
	span.SetAttributes(
		attribute.String("db.operation", operation),
		attribute.String("db.table", table),
		attribute.Int64("db.duration_ms", duration.Milliseconds()),
		attribute.String("component", "database"),
	)
}

// AddHTTPAttributes adds HTTP request/response context
func AddHTTPAttributes(ctx context.Context, method, path string, statusCode int, duration time.Duration) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	
	span.SetAttributes(
		attribute.String("http.method", method),
		attribute.String("http.route", path),
		attribute.Int("http.status_code", statusCode),
		attribute.Int64("http.duration_ms", duration.Milliseconds()),
	)
}

// AddWorkflowAttributes adds workflow execution context
func AddWorkflowAttributes(ctx context.Context, workflowID, executionID, stepName string) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	
	span.SetAttributes(
		attribute.String("workflow.id", workflowID),
		attribute.String("workflow.execution_id", executionID),
		attribute.String("workflow.step", stepName),
		attribute.String("component", "workflow"),
	)
}

// AddCostAttributes adds cost tracking context
func AddCostAttributes(ctx context.Context, costCenter string, weight int) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	
	span.SetAttributes(
		attribute.String("cost.center", costCenter),
		attribute.Int("cost.weight", weight),
	)
}

// Helper functions
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getSamplingRatio() float64 {
	env := os.Getenv("ENVIRONMENT")
	switch env {
	case "development":
		return 1.0 // 100% sampling in development
	case "staging":
		return 0.5 // 50% sampling in staging
	default:
		return 0.1 // 10% sampling in production
	}
}