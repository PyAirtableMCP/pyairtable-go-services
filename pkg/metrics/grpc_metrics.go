package metrics

import (
	"context"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// GRPCMetrics contains Prometheus metrics for gRPC operations
type GRPCMetrics struct {
	requestsTotal        *prometheus.CounterVec
	requestDuration      *prometheus.HistogramVec
	requestsInFlight     prometheus.Gauge
	requestSizeBytes     *prometheus.HistogramVec
	responseSizeBytes    *prometheus.HistogramVec
	connectionTotal      prometheus.Counter
	connectionCurrent    prometheus.Gauge
	streamMessagesTotal  *prometheus.CounterVec
	circuitBreakerState  *prometheus.GaugeVec
	rateLimitDropped     *prometheus.CounterVec
}

// NewGRPCMetrics creates a new GRPCMetrics instance
func NewGRPCMetrics() *GRPCMetrics {
	return &GRPCMetrics{
		requestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "grpc_requests_total",
				Help: "Total number of gRPC requests",
			},
			[]string{"service", "method", "status_code"},
		),
		requestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "grpc_request_duration_seconds",
				Help:    "Duration of gRPC requests in seconds",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~32s
			},
			[]string{"service", "method", "status_code"},
		),
		requestsInFlight: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "grpc_requests_in_flight",
				Help: "Current number of gRPC requests being processed",
			},
		),
		requestSizeBytes: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "grpc_request_size_bytes",
				Help:    "Size of gRPC requests in bytes",
				Buckets: prometheus.ExponentialBuckets(64, 2, 16), // 64B to ~4MB
			},
			[]string{"service", "method"},
		),
		responseSizeBytes: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "grpc_response_size_bytes",
				Help:    "Size of gRPC responses in bytes",
				Buckets: prometheus.ExponentialBuckets(64, 2, 16), // 64B to ~4MB
			},
			[]string{"service", "method"},
		),
		connectionTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "grpc_connections_total",
				Help: "Total number of gRPC connections established",
			},
		),
		connectionCurrent: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "grpc_connections_current",
				Help: "Current number of active gRPC connections",
			},
		),
		streamMessagesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "grpc_stream_messages_total",
				Help: "Total number of gRPC stream messages",
			},
			[]string{"service", "method", "direction"},
		),
		circuitBreakerState: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "grpc_circuit_breaker_state",
				Help: "Circuit breaker state (0=closed, 1=half-open, 2=open)",
			},
			[]string{"service", "method"},
		),
		rateLimitDropped: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "grpc_rate_limit_dropped_total",
				Help: "Total number of requests dropped due to rate limiting",
			},
			[]string{"service", "method", "client_id"},
		),
	}
}

// UnaryServerInterceptor returns a gRPC unary server interceptor that collects metrics
func (m *GRPCMetrics) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		service, method := splitFullMethodName(info.FullMethod)
		
		// Track in-flight requests
		m.requestsInFlight.Inc()
		defer m.requestsInFlight.Dec()

		// Record request size (approximation based on interface{} size)
		reqSize := estimateSize(req)
		m.requestSizeBytes.WithLabelValues(service, method).Observe(float64(reqSize))

		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start).Seconds()

		// Get status code
		statusCode := getStatusCode(err)
		
		// Record metrics
		m.requestsTotal.WithLabelValues(service, method, statusCode).Inc()
		m.requestDuration.WithLabelValues(service, method, statusCode).Observe(duration)

		// Record response size
		if resp != nil {
			respSize := estimateSize(resp)
			m.responseSizeBytes.WithLabelValues(service, method).Observe(float64(respSize))
		}

		return resp, err
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor that collects metrics
func (m *GRPCMetrics) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		service, method := splitFullMethodName(info.FullMethod)
		
		// Track in-flight requests
		m.requestsInFlight.Inc()
		defer m.requestsInFlight.Dec()

		start := time.Now()
		
		// Wrap the server stream to count messages
		wrappedStream := &monitoredServerStream{
			ServerStream: ss,
			metrics:      m,
			service:      service,
			method:       method,
		}

		err := handler(srv, wrappedStream)
		duration := time.Since(start).Seconds()

		// Get status code
		statusCode := getStatusCode(err)
		
		// Record metrics
		m.requestsTotal.WithLabelValues(service, method, statusCode).Inc()
		m.requestDuration.WithLabelValues(service, method, statusCode).Observe(duration)

		return err
	}
}

// UnaryClientInterceptor returns a gRPC unary client interceptor that collects metrics
func (m *GRPCMetrics) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		service, methodName := splitFullMethodName(method)
		
		// Record connection
		m.connectionCurrent.Inc()
		defer m.connectionCurrent.Dec()

		// Record request size
		reqSize := estimateSize(req)
		m.requestSizeBytes.WithLabelValues(service, methodName).Observe(float64(reqSize))

		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start).Seconds()

		// Get status code
		statusCode := getStatusCode(err)
		
		// Record metrics
		m.requestsTotal.WithLabelValues(service, methodName, statusCode).Inc()
		m.requestDuration.WithLabelValues(service, methodName, statusCode).Observe(duration)

		// Record response size
		if reply != nil && err == nil {
			respSize := estimateSize(reply)
			m.responseSizeBytes.WithLabelValues(service, methodName).Observe(float64(respSize))
		}

		return err
	}
}

// StreamClientInterceptor returns a gRPC stream client interceptor that collects metrics
func (m *GRPCMetrics) StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		service, methodName := splitFullMethodName(method)
		
		// Record connection
		m.connectionCurrent.Inc()
		
		start := time.Now()
		stream, err := streamer(ctx, desc, cc, method, opts...)
		
		if err != nil {
			m.connectionCurrent.Dec()
			statusCode := getStatusCode(err)
			m.requestsTotal.WithLabelValues(service, methodName, statusCode).Inc()
			return nil, err
		}

		// Wrap the client stream to count messages and handle cleanup
		wrappedStream := &monitoredClientStream{
			ClientStream: stream,
			metrics:      m,
			service:      service,
			method:       methodName,
			startTime:    start,
		}

		return wrappedStream, nil
	}
}

// RecordCircuitBreakerState records the current state of a circuit breaker
func (m *GRPCMetrics) RecordCircuitBreakerState(service, method string, state int) {
	m.circuitBreakerState.WithLabelValues(service, method).Set(float64(state))
}

// RecordRateLimitDrop records a request dropped due to rate limiting
func (m *GRPCMetrics) RecordRateLimitDrop(service, method, clientID string) {
	m.rateLimitDropped.WithLabelValues(service, method, clientID).Inc()
}

// monitoredServerStream wraps a server stream to collect metrics
type monitoredServerStream struct {
	grpc.ServerStream
	metrics *GRPCMetrics
	service string
	method  string
}

func (m *monitoredServerStream) SendMsg(msg interface{}) error {
	err := m.ServerStream.SendMsg(msg)
	if err == nil {
		m.metrics.streamMessagesTotal.WithLabelValues(m.service, m.method, "sent").Inc()
	}
	return err
}

func (m *monitoredServerStream) RecvMsg(msg interface{}) error {
	err := m.ServerStream.RecvMsg(msg)
	if err == nil {
		m.metrics.streamMessagesTotal.WithLabelValues(m.service, m.method, "received").Inc()
	}
	return err
}

// monitoredClientStream wraps a client stream to collect metrics
type monitoredClientStream struct {
	grpc.ClientStream
	metrics   *GRPCMetrics
	service   string
	method    string
	startTime time.Time
	closed    bool
}

func (m *monitoredClientStream) SendMsg(msg interface{}) error {
	err := m.ClientStream.SendMsg(msg)
	if err == nil {
		m.metrics.streamMessagesTotal.WithLabelValues(m.service, m.method, "sent").Inc()
	}
	return err
}

func (m *monitoredClientStream) RecvMsg(msg interface{}) error {
	err := m.ClientStream.RecvMsg(msg)
	if err == nil {
		m.metrics.streamMessagesTotal.WithLabelValues(m.service, m.method, "received").Inc()
	}
	return err
}

func (m *monitoredClientStream) CloseSend() error {
	err := m.ClientStream.CloseSend()
	m.recordStreamCompletion(err)
	return err
}

func (m *monitoredClientStream) recordStreamCompletion(err error) {
	if m.closed {
		return
	}
	m.closed = true

	// Record stream completion metrics
	duration := time.Since(m.startTime).Seconds()
	statusCode := getStatusCode(err)
	
	m.metrics.requestsTotal.WithLabelValues(m.service, m.method, statusCode).Inc()
	m.metrics.requestDuration.WithLabelValues(m.service, m.method, statusCode).Observe(duration)
	m.metrics.connectionCurrent.Dec()
}

// Helper functions

// splitFullMethodName splits a full gRPC method name into service and method
func splitFullMethodName(fullMethod string) (service, method string) {
	// fullMethod format: /package.service/method
	if len(fullMethod) > 0 && fullMethod[0] == '/' {
		fullMethod = fullMethod[1:]
	}
	
	parts := strings.SplitN(fullMethod, "/", 2)
	if len(parts) == 2 {
		// Split service from package if needed
		serviceParts := strings.Split(parts[0], ".")
		if len(serviceParts) > 0 {
			service = serviceParts[len(serviceParts)-1]
		} else {
			service = parts[0]
		}
		method = parts[1]
	} else {
		service = "unknown"
		method = fullMethod
	}
	
	return service, method
}

// getStatusCode extracts the status code from an error
func getStatusCode(err error) string {
	if err == nil {
		return "OK"
	}
	
	st, ok := status.FromError(err)
	if !ok {
		return "UNKNOWN"
	}
	
	return st.Code().String()
}

// estimateSize provides a rough estimate of the size of an interface{}
func estimateSize(v interface{}) int {
	if v == nil {
		return 0
	}
	
	// This is a very rough estimation
	// In a real implementation, you might want to use reflection
	// or serialization for more accurate size measurement
	switch val := v.(type) {
	case string:
		return len(val)
	case []byte:
		return len(val)
	default:
		// Rough estimate based on typical message sizes
		return 256
	}
}

// Additional metrics for database performance
type DatabaseMetrics struct {
	connectionsTotal       prometheus.Counter
	connectionsCurrent     prometheus.Gauge
	queryDuration          *prometheus.HistogramVec
	queriesTotal           *prometheus.CounterVec
	transactionsTotal      *prometheus.CounterVec
	connectionPoolStats    *prometheus.GaugeVec
	slowQueries            prometheus.Counter
	cacheHits              prometheus.Counter
	cacheMisses            prometheus.Counter
}

// NewDatabaseMetrics creates a new DatabaseMetrics instance
func NewDatabaseMetrics() *DatabaseMetrics {
	return &DatabaseMetrics{
		connectionsTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "database_connections_total",
				Help: "Total number of database connections established",
			},
		),
		connectionsCurrent: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "database_connections_current",
				Help: "Current number of active database connections",
			},
		),
		queryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "database_query_duration_seconds",
				Help:    "Duration of database queries in seconds",
				Buckets: prometheus.ExponentialBuckets(0.0001, 2, 15), // 0.1ms to ~3s
			},
			[]string{"query_type", "table", "status"},
		),
		queriesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "database_queries_total",
				Help: "Total number of database queries",
			},
			[]string{"query_type", "table", "status"},
		),
		transactionsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "database_transactions_total",
				Help: "Total number of database transactions",
			},
			[]string{"status"},
		),
		connectionPoolStats: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "database_connection_pool",
				Help: "Database connection pool statistics",
			},
			[]string{"stat_type"},
		),
		slowQueries: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "database_slow_queries_total",
				Help: "Total number of slow database queries",
			},
		),
		cacheHits: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "database_cache_hits_total",
				Help: "Total number of database cache hits",
			},
		),
		cacheMisses: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "database_cache_misses_total",
				Help: "Total number of database cache misses",
			},
		),
	}
}