package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pyairtable-compose/go-services/domain/shared"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// EnhancedOutboxConfig extends the basic config with monitoring and reliability features
type EnhancedOutboxConfig struct {
	OutboxConfig
	// Database settings
	ProcessorID          string
	ProcessorName        string  
	ProcessorType        string
	HeartbeatInterval    time.Duration
	CleanupInterval      time.Duration
	RetentionDays        int
	
	// Performance settings
	DeadLetterThreshold  int
	CircuitBreakerConfig CircuitBreakerConfig
	
	// Monitoring settings
	EnableMetrics        bool
	MetricsNamespace     string
	
	// Delivery tracking
	EnableDeliveryLog    bool
	LogSuccessfulDeliveries bool
}

// CircuitBreakerConfig configures circuit breaker behavior
type CircuitBreakerConfig struct {
	MaxFailures     int
	ResetTimeout    time.Duration
	FailureThreshold float64
}

// EnhancedTransactionalOutbox extends the basic outbox with monitoring and reliability
type EnhancedTransactionalOutbox struct {
	*TransactionalOutbox
	config   EnhancedOutboxConfig
	
	// Monitoring
	metrics  *OutboxMetrics
	
	// Circuit breaker state
	circuitBreaker *CircuitBreaker
	
	// Additional state
	processorID    string
	mu            sync.RWMutex
	lastHeartbeat time.Time
}

// OutboxMetrics contains Prometheus metrics for outbox monitoring
type OutboxMetrics struct {
	EventsTotal         *prometheus.CounterVec
	EventsProcessed     *prometheus.CounterVec
	ProcessingDuration  *prometheus.HistogramVec
	QueueSize          *prometheus.GaugeVec
	RetryAttempts      *prometheus.CounterVec
	CircuitBreakerState *prometheus.GaugeVec
	ProcessorHeartbeat  *prometheus.GaugeVec
}

// CircuitBreaker implements circuit breaker pattern for outbox publishing
type CircuitBreaker struct {
	maxFailures     int
	resetTimeout    time.Duration
	failureThreshold float64
	
	mu              sync.RWMutex
	failures        int
	successes       int
	state          CircuitState
	lastFailureTime time.Time
}

// NewEnhancedTransactionalOutbox creates an enhanced outbox with monitoring
func NewEnhancedTransactionalOutbox(
	db *sql.DB, 
	config EnhancedOutboxConfig, 
	publisher EventPublisher,
) *EnhancedTransactionalOutbox {
	
	processorID := config.ProcessorID
	if processorID == "" {
		processorID = uuid.New().String()
	}
	
	// Create base outbox
	baseOutbox := NewTransactionalOutbox(db, config.OutboxConfig, publisher)
	
	// Initialize metrics if enabled
	var metrics *OutboxMetrics
	if config.EnableMetrics {
		metrics = initializeMetrics(config.MetricsNamespace, config.ProcessorName)
	}
	
	// Initialize circuit breaker
	circuitBreaker := &CircuitBreaker{
		maxFailures:      config.CircuitBreakerConfig.MaxFailures,
		resetTimeout:     config.CircuitBreakerConfig.ResetTimeout,
		failureThreshold: config.CircuitBreakerConfig.FailureThreshold,
		state:           CircuitClosed,
	}
	
	enhanced := &EnhancedTransactionalOutbox{
		TransactionalOutbox: baseOutbox,
		config:             config,
		metrics:            metrics,
		circuitBreaker:     circuitBreaker,
		processorID:        processorID,
		lastHeartbeat:      time.Now(),
	}
	
	return enhanced
}

// initializeMetrics creates Prometheus metrics for outbox monitoring
func initializeMetrics(namespace, processorName string) *OutboxMetrics {
	labels := []string{"processor", "status", "event_type"}
	
	return &OutboxMetrics{
		EventsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "outbox_events_total",
				Help:      "Total number of outbox events by status",
			},
			labels,
		),
		EventsProcessed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "outbox_events_processed_total", 
				Help:      "Total number of processed outbox events",
			},
			labels,
		),
		ProcessingDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "outbox_processing_duration_seconds",
				Help:      "Time spent processing outbox events",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"processor", "event_type"},
		),
		QueueSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "outbox_queue_size",
				Help:      "Number of events in outbox queue by status",
			},
			[]string{"processor", "status"},
		),
		RetryAttempts: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "outbox_retry_attempts_total",
				Help:      "Total number of retry attempts",
			},
			[]string{"processor", "event_type"},
		),
		CircuitBreakerState: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "outbox_circuit_breaker_state",
				Help:      "Circuit breaker state (0=closed, 1=half-open, 2=open)",
			},
			[]string{"processor", "publisher"},
		),
		ProcessorHeartbeat: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "outbox_processor_heartbeat_timestamp",
				Help:      "Timestamp of last processor heartbeat",
			},
			[]string{"processor"},
		),
	}
}

// Start starts the enhanced outbox with additional monitoring workers
func (eo *EnhancedTransactionalOutbox) Start() error {
	// Register processor in database
	if err := eo.registerProcessor(); err != nil {
		return fmt.Errorf("failed to register processor: %w", err)
	}
	
	// Start base outbox
	if err := eo.TransactionalOutbox.Start(); err != nil {
		return err
	}
	
	// Start heartbeat worker
	eo.wg.Add(1)
	go eo.heartbeatWorker()
	
	// Start cleanup worker  
	eo.wg.Add(1)
	go eo.cleanupWorker()
	
	// Start metrics worker if enabled
	if eo.config.EnableMetrics {
		eo.wg.Add(1)
		go eo.metricsWorker()
	}
	
	log.Printf("Enhanced outbox started for processor %s (%s)", 
		eo.config.ProcessorName, eo.processorID)
	
	return nil
}

// registerProcessor registers the processor in the database
func (eo *EnhancedTransactionalOutbox) registerProcessor() error {
	configJSON, err := json.Marshal(eo.config)
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}
	
	query := `
		INSERT INTO outbox_processors (
			id, processor_name, processor_type, configuration, is_active, heartbeat_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (processor_name) 
		DO UPDATE SET 
			id = EXCLUDED.id,
			processor_type = EXCLUDED.processor_type,
			configuration = EXCLUDED.configuration,
			is_active = EXCLUDED.is_active,
			heartbeat_at = EXCLUDED.heartbeat_at,
			updated_at = NOW()
	`
	
	_, err = eo.db.Exec(query,
		eo.processorID,
		eo.config.ProcessorName,
		eo.config.ProcessorType,
		configJSON,
		true,
		time.Now(),
	)
	
	return err
}

// heartbeatWorker periodically updates processor heartbeat
func (eo *EnhancedTransactionalOutbox) heartbeatWorker() {
	defer eo.wg.Done()
	
	ticker := time.NewTicker(eo.config.HeartbeatInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if err := eo.updateHeartbeat(); err != nil {
				log.Printf("Failed to update heartbeat: %v", err)
			}
		case <-eo.ctx.Done():
			// Mark processor as inactive
			eo.deactivateProcessor()
			return
		}
	}
}

// updateHeartbeat updates the processor heartbeat in database
func (eo *EnhancedTransactionalOutbox) updateHeartbeat() error {
	query := `SELECT update_processor_heartbeat($1, $2)`
	_, err := eo.db.Exec(query, eo.processorID, nil)
	
	if err == nil {
		eo.mu.Lock()
		eo.lastHeartbeat = time.Now()
		eo.mu.Unlock()
		
		if eo.metrics != nil {
			eo.metrics.ProcessorHeartbeat.WithLabelValues(eo.config.ProcessorName).SetToCurrentTime()
		}
	}
	
	return err
}

// deactivateProcessor marks processor as inactive
func (eo *EnhancedTransactionalOutbox) deactivateProcessor() {
	query := `
		UPDATE outbox_processors 
		SET is_active = false, updated_at = NOW() 
		WHERE id = $1
	`
	eo.db.Exec(query, eo.processorID)
}

// cleanupWorker periodically cleans up old processed events
func (eo *EnhancedTransactionalOutbox) cleanupWorker() {
	defer eo.wg.Done()
	
	ticker := time.NewTicker(eo.config.CleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if err := eo.cleanupProcessedEvents(); err != nil {
				log.Printf("Failed to cleanup processed events: %v", err)
			}
		case <-eo.ctx.Done():
			return
		}
	}
}

// cleanupProcessedEvents removes old processed events
func (eo *EnhancedTransactionalOutbox) cleanupProcessedEvents() error {
	query := `SELECT cleanup_processed_outbox_events($1)`
	var deletedCount int
	err := eo.db.QueryRow(query, eo.config.RetentionDays).Scan(&deletedCount)
	
	if err == nil && deletedCount > 0 {
		log.Printf("Cleaned up %d processed outbox events", deletedCount)
	}
	
	return err
}

// metricsWorker periodically updates metrics
func (eo *EnhancedTransactionalOutbox) metricsWorker() {
	defer eo.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second) // Update metrics every 30 seconds
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			eo.updateQueueMetrics()
		case <-eo.ctx.Done():
			return
		}
	}
}

// updateQueueMetrics updates queue size metrics
func (eo *EnhancedTransactionalOutbox) updateQueueMetrics() {
	if eo.metrics == nil {
		return
	}
	
	query := `SELECT * FROM get_outbox_statistics()`
	rows, err := eo.db.Query(query)
	if err != nil {
		log.Printf("Failed to get outbox statistics: %v", err)
		return
	}
	defer rows.Close()
	
	for rows.Next() {
		var status string
		var count int64
		var oldest, newest sql.NullTime
		
		if err := rows.Scan(&status, &count, &oldest, &newest); err != nil {
			log.Printf("Failed to scan statistics: %v", err)
			continue
		}
		
		eo.metrics.QueueSize.WithLabelValues(eo.config.ProcessorName, status).Set(float64(count))
	}
}

// AddEventWithDeliveryTracking adds an event with enhanced tracking
func (eo *EnhancedTransactionalOutbox) AddEventWithDeliveryTracking(
	tx *sql.Tx, 
	event shared.DomainEvent,
) error {
	// Add to base outbox
	if err := eo.AddEvent(tx, event); err != nil {
		return err
	}
	
	// Update metrics
	if eo.metrics != nil {
		eo.metrics.EventsTotal.WithLabelValues(
			eo.config.ProcessorName, 
			"added", 
			event.EventType(),
		).Inc()
	}
	
	return nil
}

// processEntryWithTracking processes an entry with enhanced monitoring
func (eo *EnhancedTransactionalOutbox) processEntryWithTracking(entry *OutboxEntry) error {
	startTime := time.Now()
	
	// Create delivery log entry if enabled
	var deliveryLogID string
	if eo.config.EnableDeliveryLog {
		deliveryLogID = uuid.New().String()
		if err := eo.createDeliveryLogEntry(deliveryLogID, entry, 1); err != nil {
			log.Printf("Failed to create delivery log entry: %v", err)
		}
	}
	
	// Check circuit breaker
	if !eo.circuitBreaker.CanProcess() {
		eo.updateMetricsForFailure(entry, "circuit_breaker_open")
		return fmt.Errorf("circuit breaker is open for processor %s", eo.config.ProcessorName)
	}
	
	// Mark as processing in database
	if err := eo.markAsProcessing(entry.ID); err != nil {
		return fmt.Errorf("failed to mark entry as processing: %w", err)
	}
	
	// Attempt to publish
	ctx, cancel := context.WithTimeout(eo.ctx, eo.config.PublishTimeout)
	defer cancel()
	
	err := eo.publisher.Publish(ctx, entry)
	duration := time.Since(startTime)
	
	if err != nil {
		// Handle failure
		eo.circuitBreaker.RecordFailure()
		eo.updateMetricsForFailure(entry, "publish_failed")
		
		if eo.config.EnableDeliveryLog && deliveryLogID != "" {
			eo.updateDeliveryLogEntry(deliveryLogID, "failure", duration, err.Error())
		}
		
		return eo.handlePublishFailure(entry, err)
	}
	
	// Handle success
	eo.circuitBreaker.RecordSuccess()
	eo.updateMetricsForSuccess(entry, duration)
	
	if eo.config.EnableDeliveryLog && deliveryLogID != "" && eo.config.LogSuccessfulDeliveries {
		eo.updateDeliveryLogEntry(deliveryLogID, "success", duration, "")
	}
	
	// Mark as published
	now := time.Now()
	entry.ProcessedAt = &now
	if err := eo.markAsPublished(entry.ID); err != nil {
		return fmt.Errorf("failed to mark entry as published: %w", err)
	}
	
	return nil
}

// createDeliveryLogEntry creates a delivery log entry
func (eo *EnhancedTransactionalOutbox) createDeliveryLogEntry(
	logID string, 
	entry *OutboxEntry, 
	attemptNumber int,
) error {
	query := `
		INSERT INTO outbox_delivery_log (
			id, event_id, processor_id, attempt_number, status, started_at
		) VALUES ($1, $2, $3, $4, $5, $6)
	`
	
	_, err := eo.db.Exec(query, logID, entry.ID, eo.processorID, attemptNumber, "started", time.Now())
	return err
}

// updateDeliveryLogEntry updates a delivery log entry
func (eo *EnhancedTransactionalOutbox) updateDeliveryLogEntry(
	logID string, 
	status string, 
	duration time.Duration, 
	errorMessage string,
) error {
	query := `
		UPDATE outbox_delivery_log 
		SET status = $1, completed_at = $2, duration_ms = $3, error_message = $4
		WHERE id = $5
	`
	
	durationMs := int(duration.Milliseconds())
	_, err := eo.db.Exec(query, status, time.Now(), durationMs, errorMessage, logID)
	return err
}

// markAsProcessing marks an entry as processing in database
func (eo *EnhancedTransactionalOutbox) markAsProcessing(eventID string) error {
	query := `SELECT mark_outbox_events_processing($1, $2)`
	_, err := eo.db.Exec(query, []string{eventID}, eo.processorID)
	return err
}

// markAsPublished marks an entry as published in database
func (eo *EnhancedTransactionalOutbox) markAsPublished(eventID string) error {
	query := `SELECT mark_outbox_events_published($1, $2)`
	_, err := eo.db.Exec(query, []string{eventID}, eo.processorID)
	return err
}

// updateMetricsForSuccess updates metrics for successful processing
func (eo *EnhancedTransactionalOutbox) updateMetricsForSuccess(entry *OutboxEntry, duration time.Duration) {
	if eo.metrics == nil {
		return
	}
	
	labels := []string{eo.config.ProcessorName, "success", entry.EventType}
	eo.metrics.EventsProcessed.WithLabelValues(labels...).Inc()
	eo.metrics.ProcessingDuration.WithLabelValues(eo.config.ProcessorName, entry.EventType).Observe(duration.Seconds())
}

// updateMetricsForFailure updates metrics for failed processing
func (eo *EnhancedTransactionalOutbox) updateMetricsForFailure(entry *OutboxEntry, reason string) {
	if eo.metrics == nil {
		return
	}
	
	labels := []string{eo.config.ProcessorName, reason, entry.EventType}
	eo.metrics.EventsProcessed.WithLabelValues(labels...).Inc()
	
	if entry.RetryCount > 0 {
		eo.metrics.RetryAttempts.WithLabelValues(eo.config.ProcessorName, entry.EventType).Inc()
	}
}

// Circuit breaker methods
func (cb *CircuitBreaker) CanProcess() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	
	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		return time.Since(cb.lastFailureTime) > cb.resetTimeout
	case CircuitHalfOpen:
		return true
	default:
		return false
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.successes++
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitClosed
		cb.failures = 0
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.failures++
	cb.lastFailureTime = time.Now()
	
	if cb.failures >= cb.maxFailures {
		cb.state = CircuitOpen
	}
}

// GetProcessorStatus returns current processor status
func (eo *EnhancedTransactionalOutbox) GetProcessorStatus() map[string]interface{} {
	eo.mu.RLock()
	defer eo.mu.RUnlock()
	
	return map[string]interface{}{
		"processor_id":      eo.processorID,
		"processor_name":    eo.config.ProcessorName,
		"processor_type":    eo.config.ProcessorType,
		"is_active":         true,
		"last_heartbeat":    eo.lastHeartbeat,
		"circuit_breaker":   eo.circuitBreaker.GetState(),
		"stats":            eo.GetStats(),
	}
}

// GetState returns current circuit breaker state
func (cb *CircuitBreaker) GetState() map[string]interface{} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	
	return map[string]interface{}{
		"state":             cb.state,
		"failures":          cb.failures,
		"successes":         cb.successes,
		"last_failure_time": cb.lastFailureTime,
	}
}