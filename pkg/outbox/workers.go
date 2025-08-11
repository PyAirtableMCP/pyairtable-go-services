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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// WorkerConfig defines configuration for outbox workers
type WorkerConfig struct {
	WorkerID           string
	BatchSize          int
	PollInterval       time.Duration
	ProcessingTimeout  time.Duration
	MaxConcurrency     int
	RetryDelay         time.Duration
	DeadLetterDelay    time.Duration
	EnableMetrics      bool
	MetricsNamespace   string
}

// OutboxWorkerManager manages multiple outbox processing workers
type OutboxWorkerManager struct {
	db        *sql.DB
	config    WorkerConfig
	publisher EventPublisher
	
	// Worker management
	workers   []*OutboxWorker
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	
	// Metrics
	metrics   *WorkerMetrics
	
	// Coordination
	coordinator *WorkerCoordinator
}

// OutboxWorker represents a single worker processing outbox events
type OutboxWorker struct {
	id           string
	manager      *OutboxWorkerManager
	db           *sql.DB
	publisher    EventPublisher
	config       WorkerConfig
	
	// State
	ctx          context.Context
	cancel       context.CancelFunc
	isRunning    bool
	lastActivity time.Time
	
	// Metrics
	processedCount   int64
	errorCount       int64
	avgProcessingTime time.Duration
	
	mu sync.RWMutex
}

// WorkerMetrics contains Prometheus metrics for worker monitoring
type WorkerMetrics struct {
	WorkersActive      *prometheus.GaugeVec
	EventsProcessed    *prometheus.CounterVec
	ProcessingDuration *prometheus.HistogramVec
	BatchSize         *prometheus.HistogramVec
	WorkerErrors      *prometheus.CounterVec
	QueueDepth        *prometheus.GaugeVec
}

// WorkerCoordinator manages worker coordination and load balancing
type WorkerCoordinator struct {
	db              *sql.DB
	workers         []*OutboxWorker
	loadBalancer    *LoadBalancer
	healthChecker   *HealthChecker
	
	mu              sync.RWMutex
	leaderElection  *LeaderElection
	isLeader        bool
}

// LoadBalancer distributes work among workers
type LoadBalancer struct {
	strategy LoadBalanceStrategy
	mu       sync.RWMutex
}

// LoadBalanceStrategy defines how work is distributed
type LoadBalanceStrategy int

const (
	RoundRobin LoadBalanceStrategy = iota
	LeastBusy
	Random
)

// HealthChecker monitors worker health
type HealthChecker struct {
	checkInterval    time.Duration
	unhealthyThreshold time.Duration
	mu              sync.RWMutex
}

// LeaderElection manages leader election among workers
type LeaderElection struct {
	db           *sql.DB
	workerID     string
	leaseTimeout time.Duration
	renewInterval time.Duration
	isLeader     bool
	mu           sync.RWMutex
}

// NewOutboxWorkerManager creates a new worker manager
func NewOutboxWorkerManager(
	db *sql.DB,
	config WorkerConfig,
	publisher EventPublisher,
) *OutboxWorkerManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Initialize metrics if enabled
	var metrics *WorkerMetrics
	if config.EnableMetrics {
		metrics = initializeWorkerMetrics(config.MetricsNamespace)
	}
	
	manager := &OutboxWorkerManager{
		db:        db,
		config:    config,
		publisher: publisher,
		ctx:       ctx,
		cancel:    cancel,
		metrics:   metrics,
	}
	
	// Initialize coordinator
	manager.coordinator = NewWorkerCoordinator(db, config)
	
	return manager
}

// initializeWorkerMetrics creates Prometheus metrics for workers
func initializeWorkerMetrics(namespace string) *WorkerMetrics {
	return &WorkerMetrics{
		WorkersActive: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "outbox_workers_active",
				Help:      "Number of active outbox workers",
			},
			[]string{"worker_id", "status"},
		),
		EventsProcessed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "outbox_worker_events_processed_total",
				Help:      "Total events processed by workers",
			},
			[]string{"worker_id", "status"},
		),
		ProcessingDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "outbox_worker_processing_duration_seconds",
				Help:      "Time spent processing events",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"worker_id"},
		),
		BatchSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "outbox_worker_batch_size",
				Help:      "Number of events processed per batch",
				Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500},
			},
			[]string{"worker_id"},
		),
		WorkerErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "outbox_worker_errors_total",
				Help:      "Total worker errors",
			},
			[]string{"worker_id", "error_type"},
		),
		QueueDepth: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "outbox_queue_depth",
				Help:      "Number of pending events in outbox queue",
			},
			[]string{"status"},
		),
	}
}

// Start starts the worker manager and all workers
func (wm *OutboxWorkerManager) Start() error {
	log.Printf("Starting outbox worker manager with %d workers", wm.config.MaxConcurrency)
	
	// Start coordinator
	if err := wm.coordinator.Start(wm.ctx); err != nil {
		return fmt.Errorf("failed to start coordinator: %w", err)
	}
	
	// Start workers
	for i := 0; i < wm.config.MaxConcurrency; i++ {
		worker := NewOutboxWorker(
			fmt.Sprintf("%s-%d", wm.config.WorkerID, i),
			wm,
		)
		
		if err := worker.Start(); err != nil {
			return fmt.Errorf("failed to start worker %s: %w", worker.id, err)
		}
		
		wm.workers = append(wm.workers, worker)
	}
	
	// Start monitoring
	wm.wg.Add(1)
	go wm.monitoringLoop()
	
	log.Printf("Outbox worker manager started successfully")
	return nil
}

// Stop gracefully stops all workers
func (wm *OutboxWorkerManager) Stop() error {
	log.Println("Stopping outbox worker manager...")
	
	// Cancel context to signal workers to stop
	wm.cancel()
	
	// Wait for all workers to finish
	wm.wg.Wait()
	
	// Stop coordinator
	wm.coordinator.Stop()
	
	log.Println("Outbox worker manager stopped")
	return nil
}

// monitoringLoop periodically monitors worker health and metrics
func (wm *OutboxWorkerManager) monitoringLoop() {
	defer wm.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			wm.updateMetrics()
			wm.checkWorkerHealth()
		case <-wm.ctx.Done():
			return
		}
	}
}

// updateMetrics updates worker metrics
func (wm *OutboxWorkerManager) updateMetrics() {
	if wm.metrics == nil {
		return
	}
	
	// Update active workers count
	activeCount := 0
	for _, worker := range wm.workers {
		if worker.IsRunning() {
			activeCount++
			wm.metrics.WorkersActive.WithLabelValues(worker.id, "active").Set(1)
		} else {
			wm.metrics.WorkersActive.WithLabelValues(worker.id, "inactive").Set(1)
		}
	}
	
	// Update queue depth
	queueStats, err := wm.getQueueStatistics()
	if err != nil {
		log.Printf("Failed to get queue statistics: %v", err)
		return
	}
	
	for status, count := range queueStats {
		wm.metrics.QueueDepth.WithLabelValues(status).Set(float64(count))
	}
}

// checkWorkerHealth checks and restarts unhealthy workers
func (wm *OutboxWorkerManager) checkWorkerHealth() {
	for i, worker := range wm.workers {
		if !worker.IsHealthy() {
			log.Printf("Worker %s is unhealthy, restarting...", worker.id)
			
			// Stop unhealthy worker
			worker.Stop()
			
			// Create new worker
			newWorker := NewOutboxWorker(
				fmt.Sprintf("%s-%d", wm.config.WorkerID, i),
				wm,
			)
			
			if err := newWorker.Start(); err != nil {
				log.Printf("Failed to restart worker: %v", err)
				continue
			}
			
			wm.workers[i] = newWorker
		}
	}
}

// getQueueStatistics returns queue statistics by status
func (wm *OutboxWorkerManager) getQueueStatistics() (map[string]int, error) {
	query := `SELECT * FROM get_outbox_statistics()`
	rows, err := wm.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	stats := make(map[string]int)
	for rows.Next() {
		var status string
		var count int64
		var oldest, newest sql.NullTime
		
		if err := rows.Scan(&status, &count, &oldest, &newest); err != nil {
			continue
		}
		
		stats[status] = int(count)
	}
	
	return stats, nil
}

// NewOutboxWorker creates a new outbox worker
func NewOutboxWorker(id string, manager *OutboxWorkerManager) *OutboxWorker {
	ctx, cancel := context.WithCancel(manager.ctx)
	
	return &OutboxWorker{
		id:           id,
		manager:      manager,
		db:           manager.db,
		publisher:    manager.publisher,
		config:       manager.config,
		ctx:          ctx,
		cancel:       cancel,
		lastActivity: time.Now(),
	}
}

// Start starts the worker
func (w *OutboxWorker) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if w.isRunning {
		return fmt.Errorf("worker %s is already running", w.id)
	}
	
	w.isRunning = true
	w.manager.wg.Add(1)
	go w.processingLoop()
	
	log.Printf("Worker %s started", w.id)
	return nil
}

// Stop stops the worker
func (w *OutboxWorker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if !w.isRunning {
		return
	}
	
	w.cancel()
	w.isRunning = false
	
	log.Printf("Worker %s stopped", w.id)
}

// IsRunning returns whether the worker is running
func (w *OutboxWorker) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.isRunning
}

// IsHealthy returns whether the worker is healthy
func (w *OutboxWorker) IsHealthy() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	
	// Consider worker unhealthy if no activity for more than 5 minutes
	return w.isRunning && time.Since(w.lastActivity) < 5*time.Minute
}

// processingLoop is the main worker processing loop
func (w *OutboxWorker) processingLoop() {
	defer w.manager.wg.Done()
	
	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if err := w.processBatch(); err != nil {
				log.Printf("Worker %s: error processing batch: %v", w.id, err)
				w.recordError("batch_processing")
			}
		case <-w.ctx.Done():
			log.Printf("Worker %s: stopping processing loop", w.id)
			return
		}
	}
}

// processBatch processes a batch of outbox events
func (w *OutboxWorker) processBatch() error {
	startTime := time.Now()
	
	// Get batch of events using database function
	events, err := w.getNextBatch()
	if err != nil {
		return fmt.Errorf("failed to get next batch: %w", err)
	}
	
	if len(events) == 0 {
		return nil // No work to do
	}
	
	// Update activity timestamp
	w.mu.Lock()
	w.lastActivity = time.Now()
	w.mu.Unlock()
	
	// Mark events as processing
	eventIDs := make([]string, len(events))
	for i, event := range events {
		eventIDs[i] = event.ID
	}
	
	if err := w.markEventsAsProcessing(eventIDs); err != nil {
		return fmt.Errorf("failed to mark events as processing: %w", err)
	}
	
	// Process each event
	successfulIDs := make([]string, 0, len(events))
	failedEvents := make(map[string]string) // event ID -> error message
	
	for _, event := range events {
		if err := w.processEvent(event); err != nil {
			failedEvents[event.ID] = err.Error()
			w.recordError("event_processing")
		} else {
			successfulIDs = append(successfulIDs, event.ID)
			w.recordSuccess()
		}
	}
	
	// Update event statuses in database
	if len(successfulIDs) > 0 {
		if err := w.markEventsAsPublished(successfulIDs); err != nil {
			log.Printf("Worker %s: failed to mark events as published: %v", w.id, err)
		}
	}
	
	if len(failedEvents) > 0 {
		if err := w.markEventsAsFailed(failedEvents); err != nil {
			log.Printf("Worker %s: failed to mark events as failed: %v", w.id, err)
		}
	}
	
	// Update metrics
	processingTime := time.Since(startTime)
	w.updateMetrics(len(events), len(successfulIDs), len(failedEvents), processingTime)
	
	return nil
}

// getNextBatch gets the next batch of events to process
func (w *OutboxWorker) getNextBatch() ([]*OutboxEntry, error) {
	query := `SELECT * FROM get_next_outbox_batch($1, $2)`
	rows, err := w.db.Query(query, w.config.BatchSize, w.id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var events []*OutboxEntry
	for rows.Next() {
		event := &OutboxEntry{}
		
		err := rows.Scan(
			&event.ID,
			&event.AggregateID,
			&event.AggregateType,
			&event.EventType,
			&event.EventVersion,
			&event.Payload,
			&event.Metadata,
			&event.CreatedAt,
			&event.RetryCount,
			&event.TenantID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		
		events = append(events, event)
	}
	
	return events, nil
}

// processEvent processes a single outbox event
func (w *OutboxWorker) processEvent(event *OutboxEntry) error {
	ctx, cancel := context.WithTimeout(w.ctx, w.config.ProcessingTimeout)
	defer cancel()
	
	return w.publisher.Publish(ctx, event)
}

// markEventsAsProcessing marks events as processing
func (w *OutboxWorker) markEventsAsProcessing(eventIDs []string) error {
	if len(eventIDs) == 0 {
		return nil
	}
	
	query := `SELECT mark_outbox_events_processing($1, $2)`
	_, err := w.db.Exec(query, eventIDs, w.id)
	return err
}

// markEventsAsPublished marks events as published
func (w *OutboxWorker) markEventsAsPublished(eventIDs []string) error {
	if len(eventIDs) == 0 {
		return nil
	}
	
	query := `SELECT mark_outbox_events_published($1, $2)`
	_, err := w.db.Exec(query, eventIDs, w.id)
	return err
}

// markEventsAsFailed marks events as failed
func (w *OutboxWorker) markEventsAsFailed(failedEvents map[string]string) error {
	if len(failedEvents) == 0 {
		return nil
	}
	
	eventIDs := make([]string, 0, len(failedEvents))
	errorMessages := make([]string, 0, len(failedEvents))
	
	for id, msg := range failedEvents {
		eventIDs = append(eventIDs, id)
		errorMessages = append(errorMessages, msg)
	}
	
	query := `SELECT mark_outbox_events_failed($1, $2, $3, $4)`
	_, err := w.db.Exec(query, eventIDs, errorMessages, w.id, 60) // 60 second base retry delay
	return err
}

// recordSuccess records a successful event processing
func (w *OutboxWorker) recordSuccess() {
	w.mu.Lock()
	w.processedCount++
	w.mu.Unlock()
}

// recordError records an error during processing
func (w *OutboxWorker) recordError(errorType string) {
	w.mu.Lock()
	w.errorCount++
	w.mu.Unlock()
	
	if w.manager.metrics != nil {
		w.manager.metrics.WorkerErrors.WithLabelValues(w.id, errorType).Inc()
	}
}

// updateMetrics updates worker metrics
func (w *OutboxWorker) updateMetrics(batchSize, successful, failed int, duration time.Duration) {
	if w.manager.metrics == nil {
		return
	}
	
	w.manager.metrics.EventsProcessed.WithLabelValues(w.id, "success").Add(float64(successful))
	w.manager.metrics.EventsProcessed.WithLabelValues(w.id, "failed").Add(float64(failed))
	w.manager.metrics.ProcessingDuration.WithLabelValues(w.id).Observe(duration.Seconds())
	w.manager.metrics.BatchSize.WithLabelValues(w.id).Observe(float64(batchSize))
}

// GetStats returns worker statistics
func (w *OutboxWorker) GetStats() map[string]interface{} {
	w.mu.RLock()
	defer w.mu.RUnlock()
	
	return map[string]interface{}{
		"worker_id":       w.id,
		"is_running":      w.isRunning,
		"processed_count": w.processedCount,
		"error_count":     w.errorCount,
		"last_activity":   w.lastActivity,
	}
}

// NewWorkerCoordinator creates a new worker coordinator
func NewWorkerCoordinator(db *sql.DB, config WorkerConfig) *WorkerCoordinator {
	return &WorkerCoordinator{
		db:            db,
		loadBalancer:  &LoadBalancer{strategy: LeastBusy},
		healthChecker: &HealthChecker{
			checkInterval:      30 * time.Second,
			unhealthyThreshold: 2 * time.Minute,
		},
		leaderElection: &LeaderElection{
			db:            db,
			workerID:      config.WorkerID,
			leaseTimeout:  60 * time.Second,
			renewInterval: 30 * time.Second,
		},
	}
}

// Start starts the coordinator
func (wc *WorkerCoordinator) Start(ctx context.Context) error {
	// Start leader election
	go wc.leaderElection.Run(ctx)
	
	// Start health checking
	go wc.healthChecker.Run(ctx, wc.workers)
	
	return nil
}

// Stop stops the coordinator
func (wc *WorkerCoordinator) Stop() {
	// Cleanup resources
}

// Leader election implementation (simplified)
func (le *LeaderElection) Run(ctx context.Context) {
	ticker := time.NewTicker(le.renewInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			le.tryBecomeLeader()
		case <-ctx.Done():
			le.releaseLeadership()
			return
		}
	}
}

func (le *LeaderElection) tryBecomeLeader() {
	// Simplified leader election - in production use etcd or similar
	le.mu.Lock()
	defer le.mu.Unlock()
	
	// Try to acquire or renew leadership
	query := `
		INSERT INTO outbox_processors (id, processor_name, processor_type, is_active, heartbeat_at)
		VALUES ($1, $2, 'leader', true, NOW())
		ON CONFLICT (processor_name) 
		DO UPDATE SET heartbeat_at = NOW()
		WHERE outbox_processors.heartbeat_at < NOW() - INTERVAL '1 minute'
	`
	
	result, err := le.db.Exec(query, le.workerID, "leader-"+le.workerID)
	if err != nil {
		return
	}
	
	rowsAffected, _ := result.RowsAffected()
	le.isLeader = rowsAffected > 0
}

func (le *LeaderElection) releaseLeadership() {
	le.mu.Lock()
	defer le.mu.Unlock()
	
	if le.isLeader {
		query := `DELETE FROM outbox_processors WHERE id = $1`
		le.db.Exec(query, le.workerID)
		le.isLeader = false
	}
}

// Health checker implementation (simplified)
func (hc *HealthChecker) Run(ctx context.Context, workers []*OutboxWorker) {
	ticker := time.NewTicker(hc.checkInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			hc.checkHealth(workers)
		case <-ctx.Done():
			return
		}
	}
}

func (hc *HealthChecker) checkHealth(workers []*OutboxWorker) {
	// Health checking logic would go here
	// For now, this is a placeholder
}