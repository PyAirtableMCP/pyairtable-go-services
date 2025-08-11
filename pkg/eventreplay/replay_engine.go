package eventreplay

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"github.com/pyairtable-compose/go-services/pkg/eventstore"
)

// ReplayStrategy defines different replay strategies
type ReplayStrategy int

const (
	SequentialReplay ReplayStrategy = iota // Process events one by one
	ParallelReplay                         // Process events in parallel by aggregate
	BatchReplay                            // Process events in batches
	StreamingReplay                        // Stream events for real-time processing
)

// ReplayConfig defines configuration for event replay
type ReplayConfig struct {
	Strategy           ReplayStrategy
	BatchSize          int
	WorkerCount        int
	SnapshotInterval   int64         // Create snapshots every N events
	MaxRetries         int
	RetryBackoff       time.Duration
	EnableSnapshots    bool
	EnableCompression  bool
	EnableMetrics      bool
	ReplayTimeout      time.Duration
	ParallelismLevel   int // How many aggregates to process in parallel
}

// ReplayEngine manages event replay operations
type ReplayEngine struct {
	eventStore *eventstore.PostgresEventStore
	db         *sql.DB
	config     ReplayConfig
	
	// State management
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	
	// Metrics and monitoring
	metrics    *ReplayMetrics
	mu         sync.RWMutex
	
	// Replay handlers
	handlers   map[string]ReplayHandler
	handlersMu sync.RWMutex
	
	// Snapshot management
	snapshotStore SnapshotStore
}

// ReplayMetrics tracks replay performance
type ReplayMetrics struct {
	TotalEvents      int64
	ProcessedEvents  int64
	FailedEvents     int64
	SkippedEvents    int64
	ReplayStartTime  time.Time
	ReplayEndTime    time.Time
	ThroughputEPS    float64 // Events per second
	AggregatesRebuilt int64
	SnapshotsCreated int64
	ErrorCount       int64
	LastError        string
	LastErrorTime    time.Time
}

// ReplayHandler interface for handling replayed events
type ReplayHandler interface {
	GetName() string
	GetEventTypes() []string
	Handle(ctx context.Context, event shared.DomainEvent) error
	OnReplayStart(ctx context.Context) error
	OnReplayComplete(ctx context.Context, metrics ReplayMetrics) error
	OnReplayError(ctx context.Context, event shared.DomainEvent, err error) error
}

// SnapshotStore interface for managing aggregate snapshots
type SnapshotStore interface {
	SaveSnapshot(aggregateID string, aggregateType string, version int64, data []byte) error
	LoadSnapshot(aggregateID string) (*AggregateSnapshot, error)
	DeleteSnapshot(aggregateID string) error
	ListSnapshots(aggregateType string, limit int) ([]*AggregateSnapshot, error)
}

// AggregateSnapshot represents a snapshot of an aggregate's state
type AggregateSnapshot struct {
	ID            string    `json:"id"`
	AggregateID   string    `json:"aggregate_id"`
	AggregateType string    `json:"aggregate_type"`
	Version       int64     `json:"version"`
	Data          []byte    `json:"data"`
	CreatedAt     time.Time `json:"created_at"`
	TenantID      string    `json:"tenant_id"`
}

// ReplayRequest defines parameters for an event replay operation
type ReplayRequest struct {
	// Filters
	AggregateID   string    `json:"aggregate_id,omitempty"`
	AggregateType string    `json:"aggregate_type,omitempty"`
	EventType     string    `json:"event_type,omitempty"`
	TenantID      string    `json:"tenant_id,omitempty"`
	
	// Time range
	FromTime      time.Time `json:"from_time,omitempty"`
	ToTime        time.Time `json:"to_time,omitempty"`
	
	// Version range
	FromVersion   int64     `json:"from_version,omitempty"`
	ToVersion     int64     `json:"to_version,omitempty"`
	
	// Replay options
	Strategy      ReplayStrategy `json:"strategy"`
	UseSnapshots  bool          `json:"use_snapshots"`
	CreateSnapshot bool         `json:"create_snapshot"`
	DryRun        bool          `json:"dry_run"`
	
	// Performance options
	BatchSize     int           `json:"batch_size,omitempty"`
	WorkerCount   int           `json:"worker_count,omitempty"`
	Timeout       time.Duration `json:"timeout,omitempty"`
}

// ReplayResult contains the results of a replay operation
type ReplayResult struct {
	RequestID       string        `json:"request_id"`
	Status          string        `json:"status"`
	TotalEvents     int64         `json:"total_events"`
	ProcessedEvents int64         `json:"processed_events"`
	FailedEvents    int64         `json:"failed_events"`
	SkippedEvents   int64         `json:"skipped_events"`
	Duration        time.Duration `json:"duration"`
	ThroughputEPS   float64       `json:"throughput_eps"`
	Errors          []string      `json:"errors,omitempty"`
	AggregatesRebuilt int64       `json:"aggregates_rebuilt"`
	SnapshotsCreated  int64       `json:"snapshots_created"`
}

// NewReplayEngine creates a new event replay engine
func NewReplayEngine(eventStore *eventstore.PostgresEventStore, db *sql.DB, config ReplayConfig) *ReplayEngine {
	ctx, cancel := context.WithCancel(context.Background())
	
	engine := &ReplayEngine{
		eventStore:    eventStore,
		db:           db,
		config:       config,
		ctx:          ctx,
		cancel:       cancel,
		handlers:     make(map[string]ReplayHandler),
		metrics:      &ReplayMetrics{},
		snapshotStore: NewPostgresSnapshotStore(db),
	}
	
	return engine
}

// RegisterHandler registers a replay handler
func (re *ReplayEngine) RegisterHandler(handler ReplayHandler) {
	re.handlersMu.Lock()
	defer re.handlersMu.Unlock()
	
	re.handlers[handler.GetName()] = handler
	log.Printf("Registered replay handler: %s for events: %v", 
		handler.GetName(), handler.GetEventTypes())
}

// ReplayEvents replays events based on the provided request
func (re *ReplayEngine) ReplayEvents(request ReplayRequest) (*ReplayResult, error) {
	startTime := time.Now()
	
	// Initialize metrics
	re.mu.Lock()
	re.metrics = &ReplayMetrics{
		ReplayStartTime: startTime,
	}
	re.mu.Unlock()
	
	// Validate request
	if err := re.validateRequest(request); err != nil {
		return nil, fmt.Errorf("invalid replay request: %w", err)
	}
	
	// Get events to replay
	events, err := re.getEventsForReplay(request)
	if err != nil {
		return nil, fmt.Errorf("failed to get events for replay: %w", err)
	}
	
	if len(events) == 0 {
		return &ReplayResult{
			Status:        "completed",
			TotalEvents:   0,
			Duration:      time.Since(startTime),
		}, nil
	}
	
	log.Printf("Starting replay of %d events using %s strategy", 
		len(events), re.strategyString(request.Strategy))
	
	// Notify handlers of replay start
	if err := re.notifyHandlersStart(); err != nil {
		return nil, fmt.Errorf("failed to notify handlers of replay start: %w", err)
	}
	
	// Execute replay based on strategy
	var replayErr error
	switch request.Strategy {
	case SequentialReplay:
		replayErr = re.replaySequential(events, request)
	case ParallelReplay:
		replayErr = re.replayParallel(events, request)
	case BatchReplay:
		replayErr = re.replayBatch(events, request)
	case StreamingReplay:
		replayErr = re.replayStreaming(events, request)
	default:
		replayErr = fmt.Errorf("unsupported replay strategy: %v", request.Strategy)
	}
	
	// Finalize metrics
	re.mu.Lock()
	re.metrics.ReplayEndTime = time.Now()
	duration := re.metrics.ReplayEndTime.Sub(re.metrics.ReplayStartTime)
	if duration.Seconds() > 0 {
		re.metrics.ThroughputEPS = float64(re.metrics.ProcessedEvents) / duration.Seconds()
	}
	finalMetrics := *re.metrics
	re.mu.Unlock()
	
	// Notify handlers of replay completion
	re.notifyHandlersComplete(finalMetrics)
	
	// Create result
	result := &ReplayResult{
		RequestID:         fmt.Sprintf("replay_%d", startTime.Unix()),
		TotalEvents:       finalMetrics.TotalEvents,
		ProcessedEvents:   finalMetrics.ProcessedEvents,
		FailedEvents:      finalMetrics.FailedEvents,
		SkippedEvents:     finalMetrics.SkippedEvents,
		Duration:          duration,
		ThroughputEPS:     finalMetrics.ThroughputEPS,
		AggregatesRebuilt: finalMetrics.AggregatesRebuilt,
		SnapshotsCreated:  finalMetrics.SnapshotsCreated,
	}
	
	if replayErr != nil {
		result.Status = "failed"
		result.Errors = []string{replayErr.Error()}
		return result, replayErr
	}
	
	result.Status = "completed"
	log.Printf("Replay completed: %d events processed in %v (%.2f EPS)", 
		result.ProcessedEvents, result.Duration, result.ThroughputEPS)
	
	return result, nil
}

// getEventsForReplay retrieves events based on the replay request filters
func (re *ReplayEngine) getEventsForReplay(request ReplayRequest) ([]shared.DomainEvent, error) {
	var events []shared.DomainEvent
	var err error
	
	if request.AggregateID != "" {
		// Get events for specific aggregate
		if request.FromVersion > 0 {
			events, err = re.eventStore.GetEventsFromVersion(request.AggregateID, request.FromVersion)
		} else {
			events, err = re.eventStore.GetEvents(request.AggregateID)
		}
	} else if request.EventType != "" && !request.FromTime.IsZero() && !request.ToTime.IsZero() {
		// Get events by type and time range
		events, err = re.eventStore.GetEventsByType(request.EventType, request.FromTime, request.ToTime)
	} else {
		// Get all unprocessed events or events by other criteria
		// This would require extending the event store interface
		events, err = re.getEventsByCustomCriteria(request)
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve events: %w", err)
	}
	
	// Apply additional filters
	filteredEvents := re.applyFilters(events, request)
	
	re.mu.Lock()
	re.metrics.TotalEvents = int64(len(filteredEvents))
	re.mu.Unlock()
	
	return filteredEvents, nil
}

// replaySequential replays events one by one in sequence
func (re *ReplayEngine) replaySequential(events []shared.DomainEvent, request ReplayRequest) error {
	for _, event := range events {
		if err := re.processEvent(event, request); err != nil {
			log.Printf("Failed to process event %s: %v", event.EventID(), err)
			atomic.AddInt64(&re.metrics.FailedEvents, 1)
			
			if !re.shouldContinueOnError(err) {
				return fmt.Errorf("stopping replay due to critical error: %w", err)
			}
		} else {
			atomic.AddInt64(&re.metrics.ProcessedEvents, 1)
		}
	}
	
	return nil
}

// replayParallel replays events in parallel by aggregate
func (re *ReplayEngine) replayParallel(events []shared.DomainEvent, request ReplayRequest) error {
	// Group events by aggregate ID
	eventsByAggregate := make(map[string][]shared.DomainEvent)
	for _, event := range events {
		aggregateID := event.AggregateID()
		eventsByAggregate[aggregateID] = append(eventsByAggregate[aggregateID], event)
	}
	
	// Create worker pool
	workChan := make(chan []shared.DomainEvent, len(eventsByAggregate))
	errorChan := make(chan error, len(eventsByAggregate))
	
	// Start workers
	workerCount := request.WorkerCount
	if workerCount == 0 {
		workerCount = re.config.WorkerCount
	}
	
	for i := 0; i < workerCount; i++ {
		re.wg.Add(1)
		go func() {
			defer re.wg.Done()
			for aggregateEvents := range workChan {
				if err := re.replayAggregateEvents(aggregateEvents, request); err != nil {
					errorChan <- err
				}
			}
		}()
	}
	
	// Send work to workers
	go func() {
		defer close(workChan)
		for _, aggregateEvents := range eventsByAggregate {
			workChan <- aggregateEvents
		}
	}()
	
	// Wait for completion and collect errors
	go func() {
		re.wg.Wait()
		close(errorChan)
	}()
	
	var errors []error
	for err := range errorChan {
		errors = append(errors, err)
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("parallel replay failed with %d errors: %v", len(errors), errors[0])
	}
	
	return nil
}

// replayBatch replays events in batches
func (re *ReplayEngine) replayBatch(events []shared.DomainEvent, request ReplayRequest) error {
	batchSize := request.BatchSize
	if batchSize == 0 {
		batchSize = re.config.BatchSize
	}
	
	for i := 0; i < len(events); i += batchSize {
		end := i + batchSize
		if end > len(events) {
			end = len(events)
		}
		
		batch := events[i:end]
		if err := re.processBatch(batch, request); err != nil {
			return fmt.Errorf("failed to process batch %d-%d: %w", i, end-1, err)
		}
		
		log.Printf("Processed batch %d-%d (%d events)", i, end-1, len(batch))
	}
	
	return nil
}

// replayStreaming replays events in a streaming fashion
func (re *ReplayEngine) replayStreaming(events []shared.DomainEvent, request ReplayRequest) error {
	// Create a buffered channel for streaming
	eventChan := make(chan shared.DomainEvent, 1000)
	
	// Start consumer
	re.wg.Add(1)
	go func() {
		defer re.wg.Done()
		for event := range eventChan {
			if err := re.processEvent(event, request); err != nil {
				log.Printf("Streaming replay error for event %s: %v", event.EventID(), err)
				atomic.AddInt64(&re.metrics.FailedEvents, 1)
			} else {
				atomic.AddInt64(&re.metrics.ProcessedEvents, 1)
			}
		}
	}()
	
	// Stream events
	for _, event := range events {
		select {
		case eventChan <- event:
		case <-re.ctx.Done():
			close(eventChan)
			return re.ctx.Err()
		}
	}
	
	close(eventChan)
	re.wg.Wait()
	
	return nil
}

// processEvent processes a single event
func (re *ReplayEngine) processEvent(event shared.DomainEvent, request ReplayRequest) error {
	if request.DryRun {
		// In dry run mode, just validate without actually processing
		return re.validateEvent(event)
	}
	
	// Get handlers for this event type
	handlers := re.getHandlersForEvent(event)
	
	for _, handler := range handlers {
		if err := handler.Handle(re.ctx, event); err != nil {
			re.recordError(event, handler.GetName(), err)
			return fmt.Errorf("handler %s failed: %w", handler.GetName(), err)
		}
	}
	
	return nil
}

// replayAggregateEvents replays all events for a specific aggregate
func (re *ReplayEngine) replayAggregateEvents(events []shared.DomainEvent, request ReplayRequest) error {
	if len(events) == 0 {
		return nil
	}
	
	aggregateID := events[0].AggregateID()
	
	// Check if we can use a snapshot
	var startIndex int
	if request.UseSnapshots && re.config.EnableSnapshots {
		snapshot, err := re.snapshotStore.LoadSnapshot(aggregateID)
		if err == nil && snapshot != nil {
			// Skip events up to the snapshot version
			for i, event := range events {
				if event.Version() > snapshot.Version {
					startIndex = i
					break
				}
			}
			log.Printf("Using snapshot for aggregate %s, skipping %d events", aggregateID, startIndex)
		}
	}
	
	// Process events from the start index
	for i := startIndex; i < len(events); i++ {
		if err := re.processEvent(events[i], request); err != nil {
			return fmt.Errorf("failed to process event %d for aggregate %s: %w", i, aggregateID, err)
		}
	}
	
	// Create snapshot if requested
	if request.CreateSnapshot && re.config.EnableSnapshots && len(events) > 0 {
		if err := re.createSnapshot(events[len(events)-1]); err != nil {
			log.Printf("Failed to create snapshot for aggregate %s: %v", aggregateID, err)
		} else {
			atomic.AddInt64(&re.metrics.SnapshotsCreated, 1)
		}
	}
	
	atomic.AddInt64(&re.metrics.AggregatesRebuilt, 1)
	return nil
}

// Additional helper methods...

// validateRequest validates a replay request
func (re *ReplayEngine) validateRequest(request ReplayRequest) error {
	if !request.FromTime.IsZero() && !request.ToTime.IsZero() && request.FromTime.After(request.ToTime) {
		return fmt.Errorf("from_time cannot be after to_time")
	}
	
	if request.FromVersion > request.ToVersion && request.ToVersion > 0 {
		return fmt.Errorf("from_version cannot be greater than to_version")
	}
	
	if request.BatchSize < 0 || request.WorkerCount < 0 {
		return fmt.Errorf("batch_size and worker_count must be positive")
	}
	
	return nil
}

// getEventsByCustomCriteria retrieves events by custom criteria
func (re *ReplayEngine) getEventsByCustomCriteria(request ReplayRequest) ([]shared.DomainEvent, error) {
	// This would require a more sophisticated query builder
	// For now, return unprocessed events
	return re.eventStore.GetUnprocessedEvents(10000)
}

// applyFilters applies additional filters to events
func (re *ReplayEngine) applyFilters(events []shared.DomainEvent, request ReplayRequest) []shared.DomainEvent {
	var filtered []shared.DomainEvent
	
	for _, event := range events {
		if re.eventMatchesFilters(event, request) {
			filtered = append(filtered, event)
		}
	}
	
	return filtered
}

// eventMatchesFilters checks if an event matches the request filters
func (re *ReplayEngine) eventMatchesFilters(event shared.DomainEvent, request ReplayRequest) bool {
	if request.TenantID != "" && event.TenantID() != request.TenantID {
		return false
	}
	
	if request.AggregateType != "" && event.AggregateType() != request.AggregateType {
		return false
	}
	
	if request.EventType != "" && event.EventType() != request.EventType {
		return false
	}
	
	if !request.FromTime.IsZero() && event.OccurredAt().Before(request.FromTime) {
		return false
	}
	
	if !request.ToTime.IsZero() && event.OccurredAt().After(request.ToTime) {
		return false
	}
	
	if request.FromVersion > 0 && event.Version() < request.FromVersion {
		return false
	}
	
	if request.ToVersion > 0 && event.Version() > request.ToVersion {
		return false
	}
	
	return true
}

// processBatch processes a batch of events
func (re *ReplayEngine) processBatch(events []shared.DomainEvent, request ReplayRequest) error {
	for _, event := range events {
		if err := re.processEvent(event, request); err != nil {
			return err
		}
	}
	return nil
}

// validateEvent validates an event without processing it
func (re *ReplayEngine) validateEvent(event shared.DomainEvent) error {
	// Perform validation checks
	if event.EventID() == "" {
		return fmt.Errorf("event ID is required")
	}
	
	if event.AggregateID() == "" {
		return fmt.Errorf("aggregate ID is required")
	}
	
	if event.EventType() == "" {
		return fmt.Errorf("event type is required")
	}
	
	return nil
}

// getHandlersForEvent returns handlers that can process the given event
func (re *ReplayEngine) getHandlersForEvent(event shared.DomainEvent) []ReplayHandler {
	re.handlersMu.RLock()
	defer re.handlersMu.RUnlock()
	
	var handlers []ReplayHandler
	
	for _, handler := range re.handlers {
		for _, eventType := range handler.GetEventTypes() {
			if eventType == event.EventType() || eventType == "*" {
				handlers = append(handlers, handler)
				break
			}
		}
	}
	
	return handlers
}

// createSnapshot creates a snapshot for an aggregate based on its last event
func (re *ReplayEngine) createSnapshot(lastEvent shared.DomainEvent) error {
	// This would require rebuilding the aggregate and serializing its state
	// For now, we'll create a minimal snapshot
	snapshotData := []byte(fmt.Sprintf(`{"aggregate_id":"%s","version":%d}`, 
		lastEvent.AggregateID(), lastEvent.Version()))
	
	return re.snapshotStore.SaveSnapshot(
		lastEvent.AggregateID(),
		lastEvent.AggregateType(),
		lastEvent.Version(),
		snapshotData,
	)
}

// recordError records an error in the metrics
func (re *ReplayEngine) recordError(event shared.DomainEvent, handlerName string, err error) {
	re.mu.Lock()
	defer re.mu.Unlock()
	
	re.metrics.ErrorCount++
	re.metrics.LastError = fmt.Sprintf("Handler %s failed for event %s: %v", handlerName, event.EventID(), err)
	re.metrics.LastErrorTime = time.Now()
}

// shouldContinueOnError determines if replay should continue after an error
func (re *ReplayEngine) shouldContinueOnError(err error) bool {
	// Define critical errors that should stop replay
	criticalErrors := []string{
		"context canceled",
		"database connection lost",
		"out of memory",
	}
	
	errStr := err.Error()
	for _, critical := range criticalErrors {
		if contains(errStr, critical) {
			return false
		}
	}
	
	return true
}

// notifyHandlersStart notifies all handlers that replay is starting
func (re *ReplayEngine) notifyHandlersStart() error {
	re.handlersMu.RLock()
	defer re.handlersMu.RUnlock()
	
	for _, handler := range re.handlers {
		if err := handler.OnReplayStart(re.ctx); err != nil {
			return fmt.Errorf("handler %s failed to start: %w", handler.GetName(), err)
		}
	}
	
	return nil
}

// notifyHandlersComplete notifies all handlers that replay is complete
func (re *ReplayEngine) notifyHandlersComplete(metrics ReplayMetrics) {
	re.handlersMu.RLock()
	defer re.handlersMu.RUnlock()
	
	for _, handler := range re.handlers {
		if err := handler.OnReplayComplete(re.ctx, metrics); err != nil {
			log.Printf("Handler %s failed to handle replay completion: %v", handler.GetName(), err)
		}
	}
}

// strategyString returns a string representation of the replay strategy
func (re *ReplayEngine) strategyString(strategy ReplayStrategy) string {
	switch strategy {
	case SequentialReplay:
		return "sequential"
	case ParallelReplay:
		return "parallel"
	case BatchReplay:
		return "batch"
	case StreamingReplay:
		return "streaming"
	default:
		return "unknown"
	}
}

// GetMetrics returns current replay metrics
func (re *ReplayEngine) GetMetrics() ReplayMetrics {
	re.mu.RLock()
	defer re.mu.RUnlock()
	return *re.metrics
}

// Shutdown gracefully shuts down the replay engine
func (re *ReplayEngine) Shutdown() error {
	log.Println("Shutting down replay engine...")
	
	re.cancel()
	re.wg.Wait()
	
	log.Println("Replay engine shutdown complete")
	return nil
}