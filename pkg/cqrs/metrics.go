package cqrs

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// CQRS performance metrics
var (
	// Query performance metrics
	queryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "pyairtable",
			Subsystem: "cqrs",
			Name:      "query_duration_seconds",
			Help:      "Duration of CQRS queries in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"query_type", "cache_hit"},
	)

	queryTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "pyairtable",
			Subsystem: "cqrs",
			Name:      "queries_total",
			Help:      "Total number of CQRS queries",
		},
		[]string{"query_type", "status"},
	)

	// Cache performance metrics
	cacheHitRatio = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "pyairtable",
			Subsystem: "cqrs",
			Name:      "cache_hit_ratio",
			Help:      "Cache hit ratio for CQRS queries",
		},
		[]string{"cache_level"},
	)

	cacheOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "pyairtable",
			Subsystem: "cqrs",
			Name:      "cache_operation_duration_seconds",
			Help:      "Duration of cache operations in seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		},
		[]string{"operation", "cache_level"},
	)

	// Projection performance metrics
	projectionEventProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "pyairtable",
			Subsystem: "cqrs",
			Name:      "projection_event_processing_duration_seconds",
			Help:      "Duration of projection event processing in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"projection_name", "event_type"},
	)

	projectionEventsProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "pyairtable",
			Subsystem: "cqrs",
			Name:      "projection_events_processed_total",
			Help:      "Total number of events processed by projections",
		},
		[]string{"projection_name", "event_type", "status"},
	)

	projectionLag = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "pyairtable",
			Subsystem: "cqrs",
			Name:      "projection_lag_seconds",
			Help:      "Lag between event occurrence and projection update in seconds",
		},
		[]string{"projection_name"},
	)

	// Database performance metrics
	databaseConnectionsActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "pyairtable",
			Subsystem: "cqrs",
			Name:      "database_connections_active",
			Help:      "Number of active database connections",
		},
		[]string{"database_type"}, // read, write
	)

	databaseQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "pyairtable",
			Subsystem: "cqrs",
			Name:      "database_query_duration_seconds",
			Help:      "Duration of database queries in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"database_type", "query_type"},
	)

	// Performance improvement metrics
	performanceImprovementRatio = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "pyairtable",
			Subsystem: "cqrs",
			Name:      "performance_improvement_ratio",
			Help:      "Performance improvement ratio compared to traditional queries",
		},
		[]string{"query_type"},
	)
)

// MetricsCollector provides methods to collect CQRS performance metrics
type MetricsCollector struct{}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{}
}

// RecordQueryDuration records the duration of a query
func (mc *MetricsCollector) RecordQueryDuration(queryType string, duration time.Duration, cacheHit bool) {
	cacheHitStr := "false"
	if cacheHit {
		cacheHitStr = "true"
	}
	queryDuration.WithLabelValues(queryType, cacheHitStr).Observe(duration.Seconds())
}

// RecordQueryTotal increments the total query counter
func (mc *MetricsCollector) RecordQueryTotal(queryType, status string) {
	queryTotal.WithLabelValues(queryType, status).Inc()
}

// UpdateCacheHitRatio updates the cache hit ratio
func (mc *MetricsCollector) UpdateCacheHitRatio(cacheLevel string, ratio float64) {
	cacheHitRatio.WithLabelValues(cacheLevel).Set(ratio)
}

// RecordCacheOperationDuration records cache operation duration
func (mc *MetricsCollector) RecordCacheOperationDuration(operation, cacheLevel string, duration time.Duration) {
	cacheOperationDuration.WithLabelValues(operation, cacheLevel).Observe(duration.Seconds())
}

// RecordProjectionEventProcessing records projection event processing metrics
func (mc *MetricsCollector) RecordProjectionEventProcessing(projectionName, eventType string, duration time.Duration, success bool) {
	projectionEventProcessingDuration.WithLabelValues(projectionName, eventType).Observe(duration.Seconds())
	
	status := "success"
	if !success {
		status = "error"
	}
	projectionEventsProcessedTotal.WithLabelValues(projectionName, eventType, status).Inc()
}

// UpdateProjectionLag updates the projection lag metric
func (mc *MetricsCollector) UpdateProjectionLag(projectionName string, lagSeconds float64) {
	projectionLag.WithLabelValues(projectionName).Set(lagSeconds)
}

// UpdateDatabaseConnections updates database connection metrics
func (mc *MetricsCollector) UpdateDatabaseConnections(databaseType string, activeConnections int) {
	databaseConnectionsActive.WithLabelValues(databaseType).Set(float64(activeConnections))
}

// RecordDatabaseQueryDuration records database query duration
func (mc *MetricsCollector) RecordDatabaseQueryDuration(databaseType, queryType string, duration time.Duration) {
	databaseQueryDuration.WithLabelValues(databaseType, queryType).Observe(duration.Seconds())
}

// UpdatePerformanceImprovement updates performance improvement metrics
func (mc *MetricsCollector) UpdatePerformanceImprovement(queryType string, improvementRatio float64) {
	performanceImprovementRatio.WithLabelValues(queryType).Set(improvementRatio)
}

// PerformanceTracker helps track query performance for comparison
type PerformanceTracker struct {
	metrics          *MetricsCollector
	baselineMetrics  map[string]time.Duration // Store baseline query times
}

// NewPerformanceTracker creates a new performance tracker
func NewPerformanceTracker() *PerformanceTracker {
	return &PerformanceTracker{
		metrics:         NewMetricsCollector(),
		baselineMetrics: make(map[string]time.Duration),
	}
}

// TrackQuery tracks a query execution and calculates performance improvement
func (pt *PerformanceTracker) TrackQuery(queryType string, execution func() error, cacheHit bool) error {
	startTime := time.Now()
	err := execution()
	duration := time.Since(startTime)

	// Record metrics
	status := "success"
	if err != nil {
		status = "error"
	}

	pt.metrics.RecordQueryDuration(queryType, duration, cacheHit)
	pt.metrics.RecordQueryTotal(queryType, status)

	// Calculate and record performance improvement if baseline exists
	if baseline, exists := pt.baselineMetrics[queryType]; exists && cacheHit {
		improvementRatio := float64(baseline.Nanoseconds()) / float64(duration.Nanoseconds())
		pt.metrics.UpdatePerformanceImprovement(queryType, improvementRatio)
	}

	return err
}

// SetBaseline sets a baseline query time for performance comparison
func (pt *PerformanceTracker) SetBaseline(queryType string, duration time.Duration) {
	pt.baselineMetrics[queryType] = duration
}

// TrackCacheOperation tracks cache operations
func (pt *PerformanceTracker) TrackCacheOperation(operation, cacheLevel string, execution func() error) error {
	startTime := time.Now()
	err := execution()
	duration := time.Since(startTime)

	pt.metrics.RecordCacheOperationDuration(operation, cacheLevel, duration)
	return err
}

// TrackProjectionProcessing tracks projection event processing
func (pt *PerformanceTracker) TrackProjectionProcessing(projectionName, eventType string, execution func() error) error {
	startTime := time.Now()
	err := execution()
	duration := time.Since(startTime)

	success := err == nil
	pt.metrics.RecordProjectionEventProcessing(projectionName, eventType, duration, success)
	return err
}

// TrackDatabaseQuery tracks database query performance
func (pt *PerformanceTracker) TrackDatabaseQuery(databaseType, queryType string, execution func() error) error {
	startTime := time.Now()
	err := execution()
	duration := time.Since(startTime)

	pt.metrics.RecordDatabaseQueryDuration(databaseType, queryType, duration)
	return err
}

// GetMetrics returns the metrics collector
func (pt *PerformanceTracker) GetMetrics() *MetricsCollector {
	return pt.metrics
}