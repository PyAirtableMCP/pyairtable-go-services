package cqrs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"github.com/pyairtable-compose/go-services/pkg/cache"
)

// ProjectionStrategy defines how projections are updated
type ProjectionStrategy int

const (
	SynchronousProjection ProjectionStrategy = iota // Update immediately
	AsynchronousProjection                          // Update via background workers
	BatchProjection                                 // Update in batches
	EventuallyConsistent                            // Allow delayed consistency
)

// ProjectionConfig defines projection update configuration
type ProjectionConfig struct {
	Strategy        ProjectionStrategy
	BatchSize       int
	BatchTimeout    time.Duration
	WorkerCount     int
	RetryAttempts   int
	RetryBackoff    time.Duration
	EnableCaching   bool
	CacheExpiration time.Duration
}

// ProjectionManager handles updating read model projections from events
type ProjectionManager struct {
	dbConfig    *DatabaseConfig
	cache       *cache.RedisClient
	config      ProjectionConfig
	projections map[string]ProjectionHandler
	mu          sync.RWMutex
	
	// Channels for async processing
	eventChan   chan shared.DomainEvent
	batchChan   chan []shared.DomainEvent
	workerGroup sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
}

// ProjectionHandler interface for specific projection implementations
type ProjectionHandler interface {
	GetName() string
	GetEventTypes() []string
	Handle(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error
	GetCheckpoint(ctx context.Context, db *sql.DB) (string, error)
	UpdateCheckpoint(ctx context.Context, eventID string, tx *sql.Tx) error
}

// NewProjectionManager creates a new projection manager
func NewProjectionManager(dbConfig *DatabaseConfig, cache *cache.RedisClient, config ProjectionConfig) *ProjectionManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	pm := &ProjectionManager{
		dbConfig:    dbConfig,
		cache:       cache,
		config:      config,
		projections: make(map[string]ProjectionHandler),
		eventChan:   make(chan shared.DomainEvent, 1000),
		batchChan:   make(chan []shared.DomainEvent, 100),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start workers based on strategy
	switch config.Strategy {
	case AsynchronousProjection:
		pm.startAsyncWorkers()
	case BatchProjection:
		pm.startBatchWorkers()
	}

	return pm
}

// RegisterProjection registers a projection handler
func (pm *ProjectionManager) RegisterProjection(handler ProjectionHandler) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.projections[handler.GetName()] = handler
	log.Printf("Registered projection handler: %s for events: %v", 
		handler.GetName(), handler.GetEventTypes())
}

// UpdateProjections updates all relevant projections for an event
func (pm *ProjectionManager) UpdateProjections(event shared.DomainEvent) error {
	switch pm.config.Strategy {
	case SynchronousProjection:
		return pm.updateSynchronously(event)
	case AsynchronousProjection:
		return pm.updateAsynchronously(event)
	case BatchProjection:
		return pm.updateInBatch(event)
	case EventuallyConsistent:
		return pm.updateEventually(event)
	default:
		return fmt.Errorf("unsupported projection strategy: %v", pm.config.Strategy)
	}
}

// updateSynchronously updates projections immediately in the same transaction
func (pm *ProjectionManager) updateSynchronously(event shared.DomainEvent) error {
	tx, err := pm.dbConfig.GetReadDB().BeginTx(pm.ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	pm.mu.RLock()
	handlers := pm.getHandlersForEvent(event)
	pm.mu.RUnlock()

	for _, handler := range handlers {
		if err := pm.handleWithRetry(handler, event, tx); err != nil {
			return fmt.Errorf("failed to update projection %s: %w", handler.GetName(), err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit projection updates: %w", err)
	}

	// Invalidate cache if enabled
	if pm.config.EnableCaching {
		pm.invalidateCache(event)
	}

	return nil
}

// updateAsynchronously queues event for background processing
func (pm *ProjectionManager) updateAsynchronously(event shared.DomainEvent) error {
	select {
	case pm.eventChan <- event:
		return nil
	default:
		return fmt.Errorf("event channel full, dropping event: %s", event.EventID())
	}
}

// updateInBatch adds event to batch for processing
func (pm *ProjectionManager) updateInBatch(event shared.DomainEvent) error {
	// For simplicity, we'll process immediately in batches
	// In production, implement proper batching logic
	return pm.updateAsynchronously(event)
}

// updateEventually processes with eventual consistency guarantees
func (pm *ProjectionManager) updateEventually(event shared.DomainEvent) error {
	// Store event for eventual processing
	// Could use message queue or dedicated table
	go func() {
		time.Sleep(100 * time.Millisecond) // Simulate delay
		pm.updateAsynchronously(event)
	}()
	return nil
}

// startAsyncWorkers starts background workers for async processing
func (pm *ProjectionManager) startAsyncWorkers() {
	for i := 0; i < pm.config.WorkerCount; i++ {
		pm.workerGroup.Add(1)
		go pm.asyncWorker(i)
	}
	log.Printf("Started %d async projection workers", pm.config.WorkerCount)
}

// asyncWorker processes events asynchronously
func (pm *ProjectionManager) asyncWorker(workerID int) {
	defer pm.workerGroup.Done()
	
	for {
		select {
		case event := <-pm.eventChan:
			if err := pm.processEventAsync(event); err != nil {
				log.Printf("Worker %d failed to process event %s: %v", 
					workerID, event.EventID(), err)
			}
		case <-pm.ctx.Done():
			log.Printf("Async worker %d shutting down", workerID)
			return
		}
	}
}

// processEventAsync processes a single event asynchronously
func (pm *ProjectionManager) processEventAsync(event shared.DomainEvent) error {
	tx, err := pm.dbConfig.GetReadDB().BeginTx(pm.ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	pm.mu.RLock()
	handlers := pm.getHandlersForEvent(event)
	pm.mu.RUnlock()

	for _, handler := range handlers {
		if err := pm.handleWithRetry(handler, event, tx); err != nil {
			return fmt.Errorf("failed to update projection %s: %w", handler.GetName(), err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit projection updates: %w", err)
	}

	// Invalidate cache if enabled
	if pm.config.EnableCaching {
		pm.invalidateCache(event)
	}

	return nil
}

// startBatchWorkers starts workers for batch processing
func (pm *ProjectionManager) startBatchWorkers() {
	// Batch collector
	go pm.batchCollector()
	
	// Batch processors
	for i := 0; i < pm.config.WorkerCount; i++ {
		pm.workerGroup.Add(1)
		go pm.batchWorker(i)
	}
	log.Printf("Started batch projection processing with %d workers", pm.config.WorkerCount)
}

// batchCollector collects events into batches
func (pm *ProjectionManager) batchCollector() {
	batch := make([]shared.DomainEvent, 0, pm.config.BatchSize)
	timer := time.NewTimer(pm.config.BatchTimeout)
	
	for {
		select {
		case event := <-pm.eventChan:
			batch = append(batch, event)
			if len(batch) >= pm.config.BatchSize {
				pm.sendBatch(batch)
				batch = make([]shared.DomainEvent, 0, pm.config.BatchSize)
				timer.Reset(pm.config.BatchTimeout)
			}
		case <-timer.C:
			if len(batch) > 0 {
				pm.sendBatch(batch)
				batch = make([]shared.DomainEvent, 0, pm.config.BatchSize)
			}
			timer.Reset(pm.config.BatchTimeout)
		case <-pm.ctx.Done():
			if len(batch) > 0 {
				pm.sendBatch(batch)
			}
			return
		}
	}
}

// sendBatch sends a batch for processing
func (pm *ProjectionManager) sendBatch(batch []shared.DomainEvent) {
	batchCopy := make([]shared.DomainEvent, len(batch))
	copy(batchCopy, batch)
	
	select {
	case pm.batchChan <- batchCopy:
	default:
		log.Printf("Batch channel full, dropping batch of %d events", len(batch))
	}
}

// batchWorker processes event batches
func (pm *ProjectionManager) batchWorker(workerID int) {
	defer pm.workerGroup.Done()
	
	for {
		select {
		case batch := <-pm.batchChan:
			if err := pm.processBatch(batch); err != nil {
				log.Printf("Batch worker %d failed to process batch: %v", workerID, err)
			}
		case <-pm.ctx.Done():
			log.Printf("Batch worker %d shutting down", workerID)
			return
		}
	}
}

// processBatch processes a batch of events
func (pm *ProjectionManager) processBatch(events []shared.DomainEvent) error {
	tx, err := pm.dbConfig.GetReadDB().BeginTx(pm.ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin batch transaction: %w", err)
	}
	defer tx.Rollback()

	for _, event := range events {
		pm.mu.RLock()
		handlers := pm.getHandlersForEvent(event)
		pm.mu.RUnlock()

		for _, handler := range handlers {
			if err := pm.handleWithRetry(handler, event, tx); err != nil {
				return fmt.Errorf("failed to process event %s in batch: %w", event.EventID(), err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit batch: %w", err)
	}

	log.Printf("Successfully processed batch of %d events", len(events))
	return nil
}

// getHandlersForEvent returns handlers that can process the given event
func (pm *ProjectionManager) getHandlersForEvent(event shared.DomainEvent) []ProjectionHandler {
	var handlers []ProjectionHandler
	
	for _, handler := range pm.projections {
		for _, eventType := range handler.GetEventTypes() {
			if eventType == event.EventType() {
				handlers = append(handlers, handler)
				break
			}
		}
	}
	
	return handlers
}

// handleWithRetry handles an event with retry logic
func (pm *ProjectionManager) handleWithRetry(handler ProjectionHandler, event shared.DomainEvent, tx *sql.Tx) error {
	var lastErr error
	
	for attempt := 0; attempt < pm.config.RetryAttempts; attempt++ {
		if err := handler.Handle(pm.ctx, event, tx); err != nil {
			lastErr = err
			time.Sleep(pm.config.RetryBackoff * time.Duration(attempt+1))
			continue
		}
		
		// Update checkpoint on success
		if err := handler.UpdateCheckpoint(pm.ctx, event.EventID(), tx); err != nil {
			return fmt.Errorf("failed to update checkpoint: %w", err)
		}
		
		return nil
	}
	
	return fmt.Errorf("failed after %d attempts: %w", pm.config.RetryAttempts, lastErr)
}

// invalidateCache invalidates relevant cache entries for an event
func (pm *ProjectionManager) invalidateCache(event shared.DomainEvent) {
	if pm.cache == nil {
		return
	}

	// Cache invalidation patterns based on event type
	patterns := pm.getCacheInvalidationPatterns(event)
	
	for _, pattern := range patterns {
		if err := pm.cache.SecureDel(pattern); err != nil {
			log.Printf("Failed to invalidate cache pattern %s: %v", pattern, err)
		}
	}
}

// getCacheInvalidationPatterns returns cache keys to invalidate for an event
func (pm *ProjectionManager) getCacheInvalidationPatterns(event shared.DomainEvent) []string {
	var patterns []string
	
	switch event.EventType() {
	case shared.EventTypeUserRegistered, shared.EventTypeUserUpdated:
		patterns = append(patterns, 
			fmt.Sprintf("user:%s", event.AggregateID()),
			fmt.Sprintf("users:tenant:%s", event.TenantID()),
		)
	case shared.EventTypeWorkspaceCreated, shared.EventTypeWorkspaceUpdated:
		patterns = append(patterns,
			fmt.Sprintf("workspace:%s", event.AggregateID()),
			fmt.Sprintf("workspaces:tenant:%s", event.TenantID()),
		)
	}
	
	return patterns
}

// Shutdown gracefully shuts down the projection manager
func (pm *ProjectionManager) Shutdown() error {
	log.Println("Shutting down projection manager...")
	
	pm.cancel()
	pm.workerGroup.Wait()
	
	close(pm.eventChan)
	close(pm.batchChan)
	
	log.Println("Projection manager shutdown complete")
	return nil
}

// GetProjectionStatus returns status of all projections
func (pm *ProjectionManager) GetProjectionStatus() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	status := make(map[string]interface{})
	
	for name, handler := range pm.projections {
		checkpoint, err := handler.GetCheckpoint(pm.ctx, pm.dbConfig.GetReadDB())
		if err != nil {
			checkpoint = "error: " + err.Error()
		}
		
		status[name] = map[string]interface{}{
			"last_checkpoint": checkpoint,
			"event_types":     handler.GetEventTypes(),
		}
	}
	
	return status
}