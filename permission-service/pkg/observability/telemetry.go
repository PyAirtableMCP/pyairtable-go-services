package observability

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// TelemetryConfig holds configuration for telemetry
type TelemetryConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	MetricsPort    string
	EnableTracing  bool
	EnableMetrics  bool
}

// DefaultTelemetryConfig returns default telemetry configuration
func DefaultTelemetryConfig() *TelemetryConfig {
	return &TelemetryConfig{
		ServiceName:    "permission-service",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		MetricsPort:    "9090",
		EnableTracing:  true,
		EnableMetrics:  true,
	}
}

// TelemetryManager manages observability components
type TelemetryManager struct {
	config           *TelemetryConfig
	logger           *zap.Logger
	meterProvider    *metric.MeterProvider
	tracerProvider   trace.TracerProvider
	meter            metric.Meter
	tracer           trace.Tracer
	prometheusServer *http.Server
	
	// Metrics
	requestCounter   metric.Int64Counter
	requestDuration  metric.Float64Histogram
	activeRequests   metric.Int64UpDownCounter
	errorCounter     metric.Int64Counter
	dbConnections    metric.Int64UpDownCounter
	cacheHits        metric.Int64Counter
	cacheMisses      metric.Int64Counter
}

// NewTelemetryManager creates a new telemetry manager
func NewTelemetryManager(config *TelemetryConfig, logger *zap.Logger) (*TelemetryManager, error) {
	tm := &TelemetryManager{
		config: config,
		logger: logger,
	}

	if err := tm.initializeMetrics(); err != nil {
		return nil, fmt.Errorf("failed to initialize metrics: %w", err)
	}

	if err := tm.initializeTracing(); err != nil {
		return nil, fmt.Errorf("failed to initialize tracing: %w", err)
	}

	if err := tm.createMetrics(); err != nil {
		return nil, fmt.Errorf("failed to create metrics: %w", err)
	}

	return tm, nil
}

// initializeMetrics sets up the metrics provider
func (tm *TelemetryManager) initializeMetrics() error {
	if !tm.config.EnableMetrics {
		return nil
	}

	// Create resource
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			attribute.String("service.name", tm.config.ServiceName),
			attribute.String("service.version", tm.config.ServiceVersion),
			attribute.String("environment", tm.config.Environment),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	// Create Prometheus exporter
	exporter, err := prometheus.New()
	if err != nil {
		return fmt.Errorf("failed to create prometheus exporter: %w", err)
	}

	// Create meter provider
	tm.meterProvider = metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(exporter),
	)

	// Set global meter provider
	otel.SetMeterProvider(tm.meterProvider)

	// Create meter
	tm.meter = tm.meterProvider.Meter(tm.config.ServiceName)

	// Start Prometheus metrics server
	if err := tm.startPrometheusServer(); err != nil {
		return fmt.Errorf("failed to start prometheus server: %w", err)
	}

	return nil
}

// initializeTracing sets up the tracing provider
func (tm *TelemetryManager) initializeTracing() error {
	if !tm.config.EnableTracing {
		return nil
	}

	// For now, use a no-op tracer provider
	// In production, you would configure Jaeger, Zipkin, or another tracing backend
	tm.tracerProvider = otel.GetTracerProvider()
	tm.tracer = tm.tracerProvider.Tracer(tm.config.ServiceName)

	// Set global tracer provider
	otel.SetTracerProvider(tm.tracerProvider)

	// Set global text map propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return nil
}

// createMetrics creates all the metrics instruments
func (tm *TelemetryManager) createMetrics() error {
	if !tm.config.EnableMetrics {
		return nil
	}

	var err error

	// Request counter
	tm.requestCounter, err = tm.meter.Int64Counter(
		"requests_total",
		metric.WithDescription("Total number of requests"),
	)
	if err != nil {
		return fmt.Errorf("failed to create request counter: %w", err)
	}

	// Request duration
	tm.requestDuration, err = tm.meter.Float64Histogram(
		"request_duration_seconds",
		metric.WithDescription("Duration of requests in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return fmt.Errorf("failed to create request duration histogram: %w", err)
	}

	// Active requests
	tm.activeRequests, err = tm.meter.Int64UpDownCounter(
		"active_requests",
		metric.WithDescription("Number of active requests"),
	)
	if err != nil {
		return fmt.Errorf("failed to create active requests counter: %w", err)
	}

	// Error counter
	tm.errorCounter, err = tm.meter.Int64Counter(
		"errors_total",
		metric.WithDescription("Total number of errors"),
	)
	if err != nil {
		return fmt.Errorf("failed to create error counter: %w", err)
	}

	// Database connections
	tm.dbConnections, err = tm.meter.Int64UpDownCounter(
		"db_connections",
		metric.WithDescription("Number of database connections"),
	)
	if err != nil {
		return fmt.Errorf("failed to create db connections counter: %w", err)
	}

	// Cache hits
	tm.cacheHits, err = tm.meter.Int64Counter(
		"cache_hits_total",
		metric.WithDescription("Total number of cache hits"),
	)
	if err != nil {
		return fmt.Errorf("failed to create cache hits counter: %w", err)
	}

	// Cache misses
	tm.cacheMisses, err = tm.meter.Int64Counter(
		"cache_misses_total",
		metric.WithDescription("Total number of cache misses"),
	)
	if err != nil {
		return fmt.Errorf("failed to create cache misses counter: %w", err)
	}

	return nil
}

// startPrometheusServer starts the Prometheus metrics HTTP server
func (tm *TelemetryManager) startPrometheusServer() error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	tm.prometheusServer = &http.Server{
		Addr:    ":" + tm.config.MetricsPort,
		Handler: mux,
	}

	go func() {
		tm.logger.Info("Starting Prometheus metrics server", zap.String("port", tm.config.MetricsPort))
		if err := tm.prometheusServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			tm.logger.Error("Prometheus server error", zap.Error(err))
		}
	}()

	return nil
}

// Metric recording methods

// RecordRequest records a request with its duration
func (tm *TelemetryManager) RecordRequest(ctx context.Context, method, protocol string, duration time.Duration, success bool) {
	if !tm.config.EnableMetrics {
		return
	}

	attributes := []attribute.KeyValue{
		attribute.String("method", method),
		attribute.String("protocol", protocol),
		attribute.Bool("success", success),
	}

	tm.requestCounter.Add(ctx, 1, metric.WithAttributes(attributes...))
	tm.requestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attributes...))

	if !success {
		tm.errorCounter.Add(ctx, 1, metric.WithAttributes(attributes...))
	}
}

// IncrementActiveRequests increments the active requests counter
func (tm *TelemetryManager) IncrementActiveRequests(ctx context.Context, protocol string) {
	if !tm.config.EnableMetrics {
		return
	}

	tm.activeRequests.Add(ctx, 1, metric.WithAttributes(
		attribute.String("protocol", protocol),
	))
}

// DecrementActiveRequests decrements the active requests counter
func (tm *TelemetryManager) DecrementActiveRequests(ctx context.Context, protocol string) {
	if !tm.config.EnableMetrics {
		return
	}

	tm.activeRequests.Add(ctx, -1, metric.WithAttributes(
		attribute.String("protocol", protocol),
	))
}

// RecordDBConnection records database connection changes
func (tm *TelemetryManager) RecordDBConnection(ctx context.Context, delta int64) {
	if !tm.config.EnableMetrics {
		return
	}

	tm.dbConnections.Add(ctx, delta)
}

// RecordCacheHit records a cache hit
func (tm *TelemetryManager) RecordCacheHit(ctx context.Context, cacheType string) {
	if !tm.config.EnableMetrics {
		return
	}

	tm.cacheHits.Add(ctx, 1, metric.WithAttributes(
		attribute.String("cache_type", cacheType),
	))
}

// RecordCacheMiss records a cache miss
func (tm *TelemetryManager) RecordCacheMiss(ctx context.Context, cacheType string) {
	if !tm.config.EnableMetrics {
		return
	}

	tm.cacheMisses.Add(ctx, 1, metric.WithAttributes(
		attribute.String("cache_type", cacheType),
	))
}

// Tracing methods

// StartSpan starts a new tracing span
func (tm *TelemetryManager) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if !tm.config.EnableTracing {
		return ctx, trace.SpanFromContext(ctx)
	}

	return tm.tracer.Start(ctx, name, opts...)
}

// AddSpanAttributes adds attributes to the current span
func (tm *TelemetryManager) AddSpanAttributes(span trace.Span, attrs ...attribute.KeyValue) {
	if !tm.config.EnableTracing {
		return
	}

	span.SetAttributes(attrs...)
}

// RecordSpanError records an error in the current span
func (tm *TelemetryManager) RecordSpanError(span trace.Span, err error) {
	if !tm.config.EnableTracing {
		return
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// GetMeter returns the OpenTelemetry meter
func (tm *TelemetryManager) GetMeter() metric.Meter {
	return tm.meter
}

// GetTracer returns the OpenTelemetry tracer
func (tm *TelemetryManager) GetTracer() trace.Tracer {
	return tm.tracer
}

// Shutdown gracefully shuts down the telemetry manager
func (tm *TelemetryManager) Shutdown(ctx context.Context) error {
	var errors []error

	// Shutdown Prometheus server
	if tm.prometheusServer != nil {
		if err := tm.prometheusServer.Shutdown(ctx); err != nil {
			errors = append(errors, fmt.Errorf("failed to shutdown prometheus server: %w", err))
		}
	}

	// Shutdown meter provider
	if tm.meterProvider != nil {
		if err := tm.meterProvider.Shutdown(ctx); err != nil {
			errors = append(errors, fmt.Errorf("failed to shutdown meter provider: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("telemetry shutdown errors: %v", errors)
	}

	tm.logger.Info("Telemetry manager shut down successfully")
	return nil
}

// RequestTracker helps track request metrics automatically
type RequestTracker struct {
	tm        *TelemetryManager
	ctx       context.Context
	span      trace.Span
	method    string
	protocol  string
	startTime time.Time
}

// NewRequestTracker creates a new request tracker
func (tm *TelemetryManager) NewRequestTracker(ctx context.Context, method, protocol string) *RequestTracker {
	// Start span
	spanCtx, span := tm.StartSpan(ctx, method)
	
	// Increment active requests
	tm.IncrementActiveRequests(spanCtx, protocol)

	return &RequestTracker{
		tm:        tm,
		ctx:       spanCtx,
		span:      span,
		method:    method,
		protocol:  protocol,
		startTime: time.Now(),
	}
}

// AddAttributes adds attributes to the request span
func (rt *RequestTracker) AddAttributes(attrs ...attribute.KeyValue) {
	rt.tm.AddSpanAttributes(rt.span, attrs...)
}

// RecordError records an error in the request
func (rt *RequestTracker) RecordError(err error) {
	rt.tm.RecordSpanError(rt.span, err)
}

// Finish finishes the request tracking
func (rt *RequestTracker) Finish(success bool) {
	duration := time.Since(rt.startTime)
	
	// Record metrics
	rt.tm.RecordRequest(rt.ctx, rt.method, rt.protocol, duration, success)
	rt.tm.DecrementActiveRequests(rt.ctx, rt.protocol)
	
	// End span
	rt.span.End()
}