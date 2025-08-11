package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// OutboxMonitor provides comprehensive monitoring for the outbox pattern
type OutboxMonitor struct {
	db                *sql.DB
	metrics          *MonitoringMetrics
	alertThresholds  AlertThresholds
	healthChecker    *OutboxHealthChecker
	performanceTracker *PerformanceTracker
}

// MonitoringMetrics contains all Prometheus metrics for outbox monitoring
type MonitoringMetrics struct {
	// Event metrics
	EventsInQueue       *prometheus.GaugeVec
	EventsProcessedRate *prometheus.CounterVec
	EventLatency        *prometheus.HistogramVec
	EventErrors         *prometheus.CounterVec
	
	// Publisher metrics
	PublisherHealth     *prometheus.GaugeVec
	PublisherLatency    *prometheus.HistogramVec
	PublisherErrors     *prometheus.CounterVec
	
	// Worker metrics
	WorkerHealth        *prometheus.GaugeVec
	WorkerUtilization   *prometheus.GaugeVec
	WorkerThroughput    *prometheus.GaugeVec
	
	// System metrics
	DatabaseConnections *prometheus.GaugeVec
	MemoryUsage        *prometheus.GaugeVec
	CPUUsage           *prometheus.GaugeVec
	
	// Business metrics
	EventTypeDistribution *prometheus.CounterVec
	TenantDistribution    *prometheus.CounterVec
	RetryDistribution     *prometheus.HistogramVec
	
	// SLA metrics
	SLACompliance       *prometheus.GaugeVec
	SLAViolations       *prometheus.CounterVec
}

// AlertThresholds defines thresholds for various alerts
type AlertThresholds struct {
	QueueDepthWarning      int
	QueueDepthCritical     int
	ProcessingLatencyP95   time.Duration
	ProcessingLatencyP99   time.Duration
	ErrorRateWarning       float64
	ErrorRateCritical      float64
	WorkerHealthThreshold  float64
	SLAComplianceThreshold float64
}

// OutboxHealthChecker monitors the overall health of the outbox system
type OutboxHealthChecker struct {
	db                *sql.DB
	checkInterval     time.Duration
	healthStatus      map[string]HealthStatus
	lastHealthCheck   time.Time
}

// HealthStatus represents the health status of a component
type HealthStatus struct {
	Component    string    `json:"component"`
	Status       string    `json:"status"` // healthy, warning, critical, unknown
	Message      string    `json:"message"`
	LastChecked  time.Time `json:"last_checked"`
	ResponseTime time.Duration `json:"response_time"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// PerformanceTracker tracks performance metrics and trends
type PerformanceTracker struct {
	db              *sql.DB
	metrics         *MonitoringMetrics
	samplingRate    float64
	trendWindow     time.Duration
	performanceData map[string]*PerformanceData
}

// PerformanceData holds performance data for analysis
type PerformanceData struct {
	Timestamps    []time.Time
	Values        []float64
	TrendSlope    float64
	LastUpdated   time.Time
}

// NewOutboxMonitor creates a new outbox monitor
func NewOutboxMonitor(db *sql.DB, namespace string) *OutboxMonitor {
	metrics := initializeMonitoringMetrics(namespace)
	
	return &OutboxMonitor{
		db:      db,
		metrics: metrics,
		alertThresholds: AlertThresholds{
			QueueDepthWarning:      1000,
			QueueDepthCritical:     5000,
			ProcessingLatencyP95:   30 * time.Second,
			ProcessingLatencyP99:   60 * time.Second,
			ErrorRateWarning:       0.05, // 5%
			ErrorRateCritical:      0.10, // 10%
			WorkerHealthThreshold:  0.80, // 80%
			SLAComplianceThreshold: 0.95, // 95%
		},
		healthChecker: &OutboxHealthChecker{
			db:            db,
			checkInterval: 30 * time.Second,
			healthStatus:  make(map[string]HealthStatus),
		},
		performanceTracker: &PerformanceTracker{
			db:              db,
			metrics:         metrics,
			samplingRate:    0.1, // 10% sampling
			trendWindow:     1 * time.Hour,
			performanceData: make(map[string]*PerformanceData),
		},
	}
}

// initializeMonitoringMetrics creates all Prometheus metrics
func initializeMonitoringMetrics(namespace string) *MonitoringMetrics {
	return &MonitoringMetrics{
		EventsInQueue: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "outbox_events_in_queue",
				Help:      "Number of events in outbox queue by status",
			},
			[]string{"status", "tenant_id"},
		),
		EventsProcessedRate: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "outbox_events_processed_total",
				Help:      "Total number of events processed",
			},
			[]string{"status", "event_type", "tenant_id"},
		),
		EventLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "outbox_event_processing_duration_seconds",
				Help:      "Time from event creation to successful publishing",
				Buckets:   []float64{0.1, 0.5, 1, 5, 10, 30, 60, 300},
			},
			[]string{"event_type", "publisher_type"},
		),
		EventErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "outbox_event_errors_total",
				Help:      "Total number of event processing errors",
			},
			[]string{"error_type", "event_type", "publisher_type"},
		),
		PublisherHealth: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "outbox_publisher_health",
				Help:      "Health status of publishers (0=unhealthy, 1=healthy)",
			},
			[]string{"publisher_type", "publisher_id"},
		),
		PublisherLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "outbox_publisher_latency_seconds",
				Help:      "Publisher response time",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"publisher_type", "publisher_id"},
		),
		PublisherErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "outbox_publisher_errors_total",
				Help:      "Total publisher errors",
			},
			[]string{"publisher_type", "error_code"},
		),
		WorkerHealth: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "outbox_worker_health",
				Help:      "Health status of workers (0=unhealthy, 1=healthy)",
			},
			[]string{"worker_id", "processor_id"},
		),
		WorkerUtilization: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "outbox_worker_utilization",
				Help:      "Worker utilization percentage",
			},
			[]string{"worker_id", "processor_id"},
		),
		WorkerThroughput: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "outbox_worker_throughput",
				Help:      "Worker throughput (events per second)",
			},
			[]string{"worker_id", "processor_id"},
		),
		DatabaseConnections: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "outbox_database_connections",
				Help:      "Number of database connections",
			},
			[]string{"state"},
		),
		MemoryUsage: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "outbox_memory_usage_bytes",
				Help:      "Memory usage in bytes",
			},
			[]string{"component"},
		),
		CPUUsage: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "outbox_cpu_usage_percent",
				Help:      "CPU usage percentage",
			},
			[]string{"component"},
		),
		EventTypeDistribution: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "outbox_event_type_distribution",
				Help:      "Distribution of event types",
			},
			[]string{"event_type", "aggregate_type"},
		),
		TenantDistribution: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "outbox_tenant_distribution",
				Help:      "Distribution of events by tenant",
			},
			[]string{"tenant_id"},
		),
		RetryDistribution: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "outbox_retry_attempts",
				Help:      "Distribution of retry attempts",
				Buckets:   []float64{0, 1, 2, 3, 4, 5, 10},
			},
			[]string{"event_type"},
		),
		SLACompliance: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "outbox_sla_compliance",
				Help:      "SLA compliance percentage",
			},
			[]string{"sla_type", "time_window"},
		),
		SLAViolations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "outbox_sla_violations_total",
				Help:      "Total SLA violations",
			},
			[]string{"sla_type", "violation_type"},
		),
	}
}

// Start starts the monitoring system
func (om *OutboxMonitor) Start(ctx context.Context) error {
	log.Println("Starting outbox monitoring system")
	
	// Start health checker
	go om.healthChecker.Run(ctx)
	
	// Start performance tracker
	go om.performanceTracker.Run(ctx)
	
	// Start metrics collector
	go om.metricsCollectionLoop(ctx)
	
	log.Println("Outbox monitoring system started")
	return nil
}

// metricsCollectionLoop periodically collects and updates metrics
func (om *OutboxMonitor) metricsCollectionLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second) // Collect metrics every 10 seconds
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			om.collectMetrics()
		case <-ctx.Done():
			return
		}
	}
}

// collectMetrics collects various metrics from the database and system
func (om *OutboxMonitor) collectMetrics() {
	// Collect queue metrics
	om.collectQueueMetrics()
	
	// Collect processing metrics
	om.collectProcessingMetrics()
	
	// Collect worker metrics
	om.collectWorkerMetrics()
	
	// Collect SLA metrics
	om.collectSLAMetrics()
}

// collectQueueMetrics collects outbox queue metrics
func (om *OutboxMonitor) collectQueueMetrics() {
	query := `
		SELECT status, tenant_id, COUNT(*) as count
		FROM event_outbox 
		GROUP BY status, tenant_id
	`
	
	rows, err := om.db.Query(query)
	if err != nil {
		log.Printf("Failed to collect queue metrics: %v", err)
		return
	}
	defer rows.Close()
	
	for rows.Next() {
		var status, tenantID string
		var count int64
		
		if err := rows.Scan(&status, &tenantID, &count); err != nil {
			continue
		}
		
		om.metrics.EventsInQueue.WithLabelValues(status, tenantID).Set(float64(count))
		
		// Check alert thresholds
		if status == "pending" {
			if count > int64(om.alertThresholds.QueueDepthCritical) {
				log.Printf("CRITICAL: Queue depth critical for tenant %s: %d events", tenantID, count)
			} else if count > int64(om.alertThresholds.QueueDepthWarning) {
				log.Printf("WARNING: Queue depth warning for tenant %s: %d events", tenantID, count)
			}
		}
	}
}

// collectProcessingMetrics collects event processing metrics
func (om *OutboxMonitor) collectProcessingMetrics() {
	query := `
		SELECT 
			event_type,
			COUNT(*) as total_events,
			AVG(EXTRACT(EPOCH FROM (processed_at - created_at))) as avg_latency_seconds,
			COUNT(CASE WHEN status = 'failed' THEN 1 END) as failed_events
		FROM event_outbox 
		WHERE created_at > NOW() - INTERVAL '1 hour'
		GROUP BY event_type
	`
	
	rows, err := om.db.Query(query)
	if err != nil {
		log.Printf("Failed to collect processing metrics: %v", err)
		return
	}
	defer rows.Close()
	
	for rows.Next() {
		var eventType string
		var totalEvents, failedEvents int64
		var avgLatency sql.NullFloat64
		
		if err := rows.Scan(&eventType, &totalEvents, &avgLatency, &failedEvents); err != nil {
			continue
		}
		
		// Update latency metrics
		if avgLatency.Valid {
			om.metrics.EventLatency.WithLabelValues(eventType, "outbox").Observe(avgLatency.Float64)
		}
		
		// Update error rate
		if totalEvents > 0 {
			errorRate := float64(failedEvents) / float64(totalEvents)
			if errorRate > om.alertThresholds.ErrorRateCritical {
				log.Printf("CRITICAL: High error rate for event type %s: %.2f%%", eventType, errorRate*100)
			} else if errorRate > om.alertThresholds.ErrorRateWarning {
				log.Printf("WARNING: Elevated error rate for event type %s: %.2f%%", eventType, errorRate*100)
			}
		}
	}
}

// collectWorkerMetrics collects worker performance metrics
func (om *OutboxMonitor) collectWorkerMetrics() {
	query := `
		SELECT 
			id,
			processor_name,
			is_active,
			heartbeat_at,
			EXTRACT(EPOCH FROM (NOW() - heartbeat_at)) as seconds_since_heartbeat
		FROM outbox_processors
	`
	
	rows, err := om.db.Query(query)
	if err != nil {
		log.Printf("Failed to collect worker metrics: %v", err)
		return
	}
	defer rows.Close()
	
	for rows.Next() {
		var id, processorName string
		var isActive bool
		var heartbeat time.Time
		var secondsSinceHeartbeat float64
		
		if err := rows.Scan(&id, &processorName, &isActive, &heartbeat, &secondsSinceHeartbeat); err != nil {
			continue
		}
		
		// Update worker health
		health := 0.0
		if isActive && secondsSinceHeartbeat < 120 { // Consider healthy if heartbeat within 2 minutes
			health = 1.0
		}
		
		om.metrics.WorkerHealth.WithLabelValues(id, processorName).Set(health)
		
		// Alert on unhealthy workers
		if health < om.alertThresholds.WorkerHealthThreshold {
			log.Printf("WARNING: Worker %s (%s) appears unhealthy - last heartbeat %.0f seconds ago", 
				processorName, id, secondsSinceHeartbeat)
		}
	}
}

// collectSLAMetrics collects SLA compliance metrics
func (om *OutboxMonitor) collectSLAMetrics() {
	// SLA: 95% of events should be processed within 30 seconds
	query := `
		SELECT 
			COUNT(*) as total_events,
			COUNT(CASE 
				WHEN processed_at IS NOT NULL 
				AND EXTRACT(EPOCH FROM (processed_at - created_at)) <= 30 
				THEN 1 
			END) as sla_compliant_events
		FROM event_outbox 
		WHERE created_at > NOW() - INTERVAL '1 hour'
		AND status IN ('published', 'failed')
	`
	
	var totalEvents, slaCompliantEvents int64
	err := om.db.QueryRow(query).Scan(&totalEvents, &slaCompliantEvents)
	if err != nil {
		log.Printf("Failed to collect SLA metrics: %v", err)
		return
	}
	
	if totalEvents > 0 {
		compliance := float64(slaCompliantEvents) / float64(totalEvents)
		om.metrics.SLACompliance.WithLabelValues("processing_latency", "1h").Set(compliance)
		
		if compliance < om.alertThresholds.SLAComplianceThreshold {
			violationCount := totalEvents - slaCompliantEvents
			om.metrics.SLAViolations.WithLabelValues("processing_latency", "threshold_exceeded").Add(float64(violationCount))
			log.Printf("WARNING: SLA compliance below threshold: %.2f%% (%d violations)", 
				compliance*100, violationCount)
		}
	}
}

// Run starts the health checker
func (hc *OutboxHealthChecker) Run(ctx context.Context) {
	ticker := time.NewTicker(hc.checkInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			hc.performHealthChecks()
		case <-ctx.Done():
			return
		}
	}
}

// performHealthChecks performs various health checks
func (hc *OutboxHealthChecker) performHealthChecks() {
	hc.lastHealthCheck = time.Now()
	
	// Check database connectivity
	hc.checkDatabaseHealth()
	
	// Check outbox table health
	hc.checkOutboxTableHealth()
	
	// Check processor health
	hc.checkProcessorHealth()
}

// checkDatabaseHealth checks database connectivity and performance
func (hc *OutboxHealthChecker) checkDatabaseHealth() {
	start := time.Now()
	
	var result int
	err := hc.db.QueryRow("SELECT 1").Scan(&result)
	
	responseTime := time.Since(start)
	
	status := HealthStatus{
		Component:    "database",
		LastChecked:  time.Now(),
		ResponseTime: responseTime,
		Metadata: map[string]interface{}{
			"query": "SELECT 1",
		},
	}
	
	if err != nil {
		status.Status = "critical"
		status.Message = fmt.Sprintf("Database connectivity failed: %v", err)
	} else if responseTime > 5*time.Second {
		status.Status = "warning"
		status.Message = fmt.Sprintf("Database response slow: %v", responseTime)
	} else {
		status.Status = "healthy"
		status.Message = "Database connectivity normal"
	}
	
	hc.healthStatus["database"] = status
}

// checkOutboxTableHealth checks outbox table health
func (hc *OutboxHealthChecker) checkOutboxTableHealth() {
	start := time.Now()
	
	var pendingCount, processingCount int
	query := `
		SELECT 
			COUNT(CASE WHEN status = 'pending' THEN 1 END) as pending,
			COUNT(CASE WHEN status = 'processing' THEN 1 END) as processing
		FROM event_outbox
	`
	
	err := hc.db.QueryRow(query).Scan(&pendingCount, &processingCount)
	responseTime := time.Since(start)
	
	status := HealthStatus{
		Component:    "outbox_table",
		LastChecked:  time.Now(),
		ResponseTime: responseTime,
		Metadata: map[string]interface{}{
			"pending_events":    pendingCount,
			"processing_events": processingCount,
		},
	}
	
	if err != nil {
		status.Status = "critical"
		status.Message = fmt.Sprintf("Failed to query outbox table: %v", err)
	} else if pendingCount > 10000 {
		status.Status = "critical"
		status.Message = fmt.Sprintf("High number of pending events: %d", pendingCount)
	} else if pendingCount > 1000 {
		status.Status = "warning"
		status.Message = fmt.Sprintf("Elevated number of pending events: %d", pendingCount)
	} else {
		status.Status = "healthy"
		status.Message = "Outbox table health normal"
	}
	
	hc.healthStatus["outbox_table"] = status
}

// checkProcessorHealth checks processor health
func (hc *OutboxHealthChecker) checkProcessorHealth() {
	query := `
		SELECT 
			COUNT(*) as total_processors,
			COUNT(CASE WHEN is_active = true THEN 1 END) as active_processors,
			COUNT(CASE WHEN heartbeat_at > NOW() - INTERVAL '2 minutes' THEN 1 END) as healthy_processors
		FROM outbox_processors
	`
	
	var totalProcessors, activeProcessors, healthyProcessors int
	err := hc.db.QueryRow(query).Scan(&totalProcessors, &activeProcessors, &healthyProcessors)
	
	status := HealthStatus{
		Component:   "processors",
		LastChecked: time.Now(),
		Metadata: map[string]interface{}{
			"total_processors":   totalProcessors,
			"active_processors":  activeProcessors,
			"healthy_processors": healthyProcessors,
		},
	}
	
	if err != nil {
		status.Status = "critical"
		status.Message = fmt.Sprintf("Failed to query processors: %v", err)
	} else if healthyProcessors == 0 {
		status.Status = "critical"
		status.Message = "No healthy processors found"
	} else if float64(healthyProcessors)/float64(totalProcessors) < 0.5 {
		status.Status = "warning"
		status.Message = fmt.Sprintf("Low processor health: %d/%d healthy", healthyProcessors, totalProcessors)
	} else {
		status.Status = "healthy"
		status.Message = "Processors healthy"
	}
	
	hc.healthStatus["processors"] = status
}

// GetHealthStatus returns the current health status
func (hc *OutboxHealthChecker) GetHealthStatus() map[string]HealthStatus {
	return hc.healthStatus
}

// Run starts the performance tracker
func (pt *PerformanceTracker) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute) // Track performance every minute
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			pt.trackPerformance()
		case <-ctx.Done():
			return
		}
	}
}

// trackPerformance tracks performance metrics and trends
func (pt *PerformanceTracker) trackPerformance() {
	// Track throughput trend
	pt.trackThroughputTrend()
	
	// Track latency trend  
	pt.trackLatencyTrend()
	
	// Track error rate trend
	pt.trackErrorRateTrend()
}

// trackThroughputTrend tracks event processing throughput trend
func (pt *PerformanceTracker) trackThroughputTrend() {
	query := `
		SELECT COUNT(*) 
		FROM event_outbox 
		WHERE processed_at > NOW() - INTERVAL '1 minute'
		AND status = 'published'
	`
	
	var throughput int64
	err := pt.db.QueryRow(query).Scan(&throughput)
	if err != nil {
		return
	}
	
	pt.updatePerformanceData("throughput", float64(throughput))
}

// trackLatencyTrend tracks processing latency trend
func (pt *PerformanceTracker) trackLatencyTrend() {
	query := `
		SELECT AVG(EXTRACT(EPOCH FROM (processed_at - created_at)))
		FROM event_outbox 
		WHERE processed_at > NOW() - INTERVAL '1 minute'
		AND status = 'published'
	`
	
	var avgLatency sql.NullFloat64
	err := pt.db.QueryRow(query).Scan(&avgLatency)
	if err != nil || !avgLatency.Valid {
		return
	}
	
	pt.updatePerformanceData("latency", avgLatency.Float64)
}

// trackErrorRateTrend tracks error rate trend
func (pt *PerformanceTracker) trackErrorRateTrend() {
	query := `
		SELECT 
			COUNT(*) as total,
			COUNT(CASE WHEN status = 'failed' THEN 1 END) as failed
		FROM event_outbox 
		WHERE created_at > NOW() - INTERVAL '1 minute'
	`
	
	var total, failed int64
	err := pt.db.QueryRow(query).Scan(&total, &failed)
	if err != nil || total == 0 {
		return
	}
	
	errorRate := float64(failed) / float64(total)
	pt.updatePerformanceData("error_rate", errorRate)
}

// updatePerformanceData updates performance data and calculates trends
func (pt *PerformanceTracker) updatePerformanceData(metric string, value float64) {
	now := time.Now()
	
	data, exists := pt.performanceData[metric]
	if !exists {
		data = &PerformanceData{
			Timestamps: make([]time.Time, 0),
			Values:     make([]float64, 0),
		}
		pt.performanceData[metric] = data
	}
	
	// Add new data point
	data.Timestamps = append(data.Timestamps, now)
	data.Values = append(data.Values, value)
	data.LastUpdated = now
	
	// Keep only data within trend window
	cutoff := now.Add(-pt.trendWindow)
	for i, ts := range data.Timestamps {
		if ts.After(cutoff) {
			data.Timestamps = data.Timestamps[i:]
			data.Values = data.Values[i:]
			break
		}
	}
	
	// Calculate trend slope (simplified linear regression)
	if len(data.Values) >= 2 {
		data.TrendSlope = pt.calculateTrendSlope(data.Values)
	}
}

// calculateTrendSlope calculates the trend slope using simple linear regression
func (pt *PerformanceTracker) calculateTrendSlope(values []float64) float64 {
	n := len(values)
	if n < 2 {
		return 0
	}
	
	// Create x values (time indices)
	var sumX, sumY, sumXY, sumX2 float64
	for i, y := range values {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}
	
	// Calculate slope: (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	denominator := float64(n)*sumX2 - sumX*sumX
	if denominator == 0 {
		return 0
	}
	
	return (float64(n)*sumXY - sumX*sumY) / denominator
}

// GetMonitoringDashboard returns a comprehensive monitoring dashboard
func (om *OutboxMonitor) GetMonitoringDashboard() map[string]interface{} {
	return map[string]interface{}{
		"health_status":      om.healthChecker.GetHealthStatus(),
		"performance_data":   om.performanceTracker.performanceData,
		"alert_thresholds":   om.alertThresholds,
		"last_health_check":  om.healthChecker.lastHealthCheck,
		"monitoring_status":  "active",
	}
}