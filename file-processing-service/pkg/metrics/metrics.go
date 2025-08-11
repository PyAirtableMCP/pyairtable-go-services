package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/pyairtable/go-services/file-processing-service/internal/models"
)

// Manager handles application metrics
type Manager struct {
	registry *prometheus.Registry
	
	// HTTP metrics
	httpRequestsTotal    *prometheus.CounterVec
	httpRequestDuration  *prometheus.HistogramVec
	httpRequestsInFlight prometheus.Gauge
	
	// File processing metrics
	filesProcessedTotal     *prometheus.CounterVec
	fileProcessingDuration  *prometheus.HistogramVec
	fileProcessingErrors    *prometheus.CounterVec
	filesInQueue           prometheus.Gauge
	activeWorkers          prometheus.Gauge
	
	// Storage metrics
	storageOperationsTotal    *prometheus.CounterVec
	storageOperationDuration  *prometheus.HistogramVec
	storageErrors             *prometheus.CounterVec
	
	// WebSocket metrics
	websocketConnectionsActive prometheus.Gauge
	websocketMessagesTotal     *prometheus.CounterVec
	
	// System metrics
	memoryUsage          prometheus.Gauge
	cpuUsage            prometheus.Gauge
	goroutineCount      prometheus.Gauge
}

// New creates a new metrics manager
func New(config models.MonitoringConfig) *Manager {
	registry := prometheus.NewRegistry()
	
	m := &Manager{
		registry: registry,
		
		// HTTP metrics
		httpRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		
		httpRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "path", "status"},
		),
		
		httpRequestsInFlight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "http_requests_in_flight",
				Help: "Number of HTTP requests currently being served",
			},
		),
		
		// File processing metrics
		filesProcessedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "files_processed_total",
				Help: "Total number of files processed",
			},
			[]string{"file_type", "status"},
		),
		
		fileProcessingDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "file_processing_duration_seconds",
				Help:    "File processing duration in seconds",
				Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300},
			},
			[]string{"file_type"},
		),
		
		fileProcessingErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "file_processing_errors_total",
				Help: "Total number of file processing errors",
			},
			[]string{"file_type", "error_type"},
		),
		
		filesInQueue: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "files_in_queue",
				Help: "Number of files currently in processing queue",
			},
		),
		
		activeWorkers: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "active_workers",
				Help: "Number of active processing workers",
			},
		),
		
		// Storage metrics
		storageOperationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "storage_operations_total",
				Help: "Total number of storage operations",
			},
			[]string{"operation", "status"},
		),
		
		storageOperationDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "storage_operation_duration_seconds",
				Help:    "Storage operation duration in seconds",
				Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10},
			},
			[]string{"operation"},
		),
		
		storageErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "storage_errors_total",
				Help: "Total number of storage errors",
			},
			[]string{"operation", "error_type"},
		),
		
		// WebSocket metrics
		websocketConnectionsActive: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "websocket_connections_active",
				Help: "Number of active WebSocket connections",
			},
		),
		
		websocketMessagesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "websocket_messages_total",
				Help: "Total number of WebSocket messages sent",
			},
			[]string{"message_type"},
		),
		
		// System metrics
		memoryUsage: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "memory_usage_bytes",
				Help: "Current memory usage in bytes",
			},
		),
		
		cpuUsage: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "cpu_usage_percent",
				Help: "Current CPU usage percentage",
			},
		),
		
		goroutineCount: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "goroutine_count",
				Help: "Number of goroutines",
			},
		),
	}
	
	// Register all metrics
	m.registerMetrics()
	
	return m
}

// registerMetrics registers all metrics with the registry
func (m *Manager) registerMetrics() {
	metrics := []prometheus.Collector{
		m.httpRequestsTotal,
		m.httpRequestDuration,
		m.httpRequestsInFlight,
		m.filesProcessedTotal,
		m.fileProcessingDuration,
		m.fileProcessingErrors,
		m.filesInQueue,
		m.activeWorkers,
		m.storageOperationsTotal,
		m.storageOperationDuration,
		m.storageErrors,
		m.websocketConnectionsActive,
		m.websocketMessagesTotal,
		m.memoryUsage,
		m.cpuUsage,
		m.goroutineCount,
	}
	
	for _, metric := range metrics {
		m.registry.MustRegister(metric)
	}
}

// Handler returns the Prometheus metrics handler
func (m *Manager) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// HTTP Metrics

// RecordHTTPRequest records an HTTP request
func (m *Manager) RecordHTTPRequest(method, path string, statusCode int, duration time.Duration) {
	status := strconv.Itoa(statusCode)
	m.httpRequestsTotal.WithLabelValues(method, path, status).Inc()
	m.httpRequestDuration.WithLabelValues(method, path, status).Observe(duration.Seconds())
}

// IncHTTPRequestsInFlight increments in-flight HTTP requests
func (m *Manager) IncHTTPRequestsInFlight() {
	m.httpRequestsInFlight.Inc()
}

// DecHTTPRequestsInFlight decrements in-flight HTTP requests
func (m *Manager) DecHTTPRequestsInFlight() {
	m.httpRequestsInFlight.Dec()
}

// File Processing Metrics

// RecordFileProcessed records a processed file
func (m *Manager) RecordFileProcessed(fileType string, status string, duration time.Duration) {
	m.filesProcessedTotal.WithLabelValues(fileType, status).Inc()
	m.fileProcessingDuration.WithLabelValues(fileType).Observe(duration.Seconds())
}

// RecordFileProcessingError records a file processing error
func (m *Manager) RecordFileProcessingError(fileType, errorType string) {
	m.fileProcessingErrors.WithLabelValues(fileType, errorType).Inc()
}

// SetFilesInQueue sets the number of files in processing queue
func (m *Manager) SetFilesInQueue(count int) {
	m.filesInQueue.Set(float64(count))
}

// SetActiveWorkers sets the number of active workers
func (m *Manager) SetActiveWorkers(count int) {
	m.activeWorkers.Set(float64(count))
}

// Storage Metrics

// RecordStorageOperation records a storage operation
func (m *Manager) RecordStorageOperation(operation, status string, duration time.Duration) {
	m.storageOperationsTotal.WithLabelValues(operation, status).Inc()
	m.storageOperationDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordStorageError records a storage error
func (m *Manager) RecordStorageError(operation, errorType string) {
	m.storageErrors.WithLabelValues(operation, errorType).Inc()
}

// WebSocket Metrics

// SetWebSocketConnections sets the number of active WebSocket connections
func (m *Manager) SetWebSocketConnections(count int) {
	m.websocketConnectionsActive.Set(float64(count))
}

// RecordWebSocketMessage records a WebSocket message
func (m *Manager) RecordWebSocketMessage(messageType string) {
	m.websocketMessagesTotal.WithLabelValues(messageType).Inc()
}

// System Metrics

// SetMemoryUsage sets the current memory usage
func (m *Manager) SetMemoryUsage(bytes int64) {
	m.memoryUsage.Set(float64(bytes))
}

// SetCPUUsage sets the current CPU usage
func (m *Manager) SetCPUUsage(percent float64) {
	m.cpuUsage.Set(percent)
}

// SetGoroutineCount sets the number of goroutines
func (m *Manager) SetGoroutineCount(count int) {
	m.goroutineCount.Set(float64(count))
}

// GetRegistry returns the Prometheus registry
func (m *Manager) GetRegistry() *prometheus.Registry {
	return m.registry
}