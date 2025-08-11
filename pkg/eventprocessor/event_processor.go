package eventprocessor

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"github.com/pyairtable-compose/go-services/pkg/eventstore"
	"go.uber.org/zap"
)

// EventProcessor processes unprocessed events and sends them to the event bus
type EventProcessor struct {
	eventStore    *eventstore.PostgresEventStore
	eventBus      shared.EventBus
	db            *sql.DB
	logger        *zap.Logger
	batchSize     int
	pollInterval  time.Duration
	maxRetries    int
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewEventProcessor creates a new event processor
func NewEventProcessor(
	eventStore *eventstore.PostgresEventStore,
	eventBus shared.EventBus,
	db *sql.DB,
	logger *zap.Logger,
) *EventProcessor {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &EventProcessor{
		eventStore:   eventStore,
		eventBus:     eventBus,
		db:           db,
		logger:       logger,
		batchSize:    100,
		pollInterval: 5 * time.Second,
		maxRetries:   3,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start begins processing events in a background goroutine
func (ep *EventProcessor) Start() {
	ep.logger.Info("Starting event processor", 
		zap.Int("batch_size", ep.batchSize),
		zap.Duration("poll_interval", ep.pollInterval),
	)

	go ep.processLoop()
}

// Stop stops the event processor
func (ep *EventProcessor) Stop() {
	ep.logger.Info("Stopping event processor")
	ep.cancel()
}

// processLoop is the main processing loop
func (ep *EventProcessor) processLoop() {
	ticker := time.NewTicker(ep.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ep.ctx.Done():
			ep.logger.Info("Event processor stopped")
			return
		case <-ticker.C:
			if err := ep.processBatch(); err != nil {
				ep.logger.Error("Error processing event batch", zap.Error(err))
			}
		}
	}
}

// processBatch processes a batch of unprocessed events
func (ep *EventProcessor) processBatch() error {
	// Get unprocessed events
	events, err := ep.eventStore.GetUnprocessedEvents(ep.batchSize)
	if err != nil {
		return fmt.Errorf("failed to get unprocessed events: %w", err)
	}

	if len(events) == 0 {
		return nil // No events to process
	}

	ep.logger.Info("Processing event batch", 
		zap.Int("event_count", len(events)),
	)

	successfulEventIDs := make([]string, 0, len(events))
	
	for _, event := range events {
		if err := ep.processEvent(event); err != nil {
			ep.logger.Error("Failed to process event", 
				zap.String("event_id", event.EventID()),
				zap.String("event_type", event.EventType()),
				zap.Error(err),
			)
			
			// Track failed event for retry logic
			if err := ep.trackEventFailure(event.EventID(), err); err != nil {
				ep.logger.Error("Failed to track event failure", 
					zap.String("event_id", event.EventID()),
					zap.Error(err),
				)
			}
		} else {
			successfulEventIDs = append(successfulEventIDs, event.EventID())
		}
	}

	// Mark successful events as processed
	if len(successfulEventIDs) > 0 {
		if err := ep.eventStore.MarkEventsAsProcessed(successfulEventIDs); err != nil {
			ep.logger.Error("Failed to mark events as processed", 
				zap.Int("event_count", len(successfulEventIDs)),
				zap.Error(err),
			)
			return err
		}

		ep.logger.Info("Successfully processed events", 
			zap.Int("processed_count", len(successfulEventIDs)),
			zap.Int("total_count", len(events)),
		)
	}

	return nil
}

// processEvent processes a single event by publishing it to the event bus
func (ep *EventProcessor) processEvent(event shared.DomainEvent) error {
	start := time.Now()
	
	err := ep.eventBus.Publish(event)
	
	duration := time.Since(start)
	
	if err != nil {
		ep.logger.Error("Event processing failed", 
			zap.String("event_id", event.EventID()),
			zap.String("event_type", event.EventType()),
			zap.String("aggregate_id", event.AggregateID()),
			zap.Duration("duration", duration),
			zap.Error(err),
		)
		return err
	}

	ep.logger.Debug("Event processed successfully", 
		zap.String("event_id", event.EventID()),
		zap.String("event_type", event.EventType()),
		zap.String("aggregate_id", event.AggregateID()),
		zap.Duration("duration", duration),
	)

	return nil
}

// trackEventFailure tracks failed events for retry logic
func (ep *EventProcessor) trackEventFailure(eventID string, processingError error) error {
	_, err := ep.db.Exec(`
		UPDATE domain_events 
		SET retry_count = retry_count + 1
		WHERE id = $1
	`, eventID)

	if err != nil {
		return fmt.Errorf("failed to update retry count: %w", err)
	}

	return nil
}

// ProcessEventsByType processes all events of a specific type (useful for replaying)
func (ep *EventProcessor) ProcessEventsByType(eventType string, from, to time.Time) error {
	events, err := ep.eventStore.GetEventsByType(eventType, from, to)
	if err != nil {
		return fmt.Errorf("failed to get events by type: %w", err)
	}

	ep.logger.Info("Processing events by type", 
		zap.String("event_type", eventType),
		zap.Int("event_count", len(events)),
		zap.Time("from", from),
		zap.Time("to", to),
	)

	for _, event := range events {
		if err := ep.processEvent(event); err != nil {
			ep.logger.Error("Failed to process event during replay", 
				zap.String("event_id", event.EventID()),
				zap.String("event_type", event.EventType()),
				zap.Error(err),
			)
			// Continue processing other events during replay
		}
	}

	return nil
}

// GetStats returns processing statistics
func (ep *EventProcessor) GetStats() (*ProcessingStats, error) {
	var stats ProcessingStats

	// Get total unprocessed events
	err := ep.db.QueryRow(`
		SELECT COUNT(*) FROM domain_events WHERE processed = false
	`).Scan(&stats.UnprocessedEvents)
	if err != nil {
		return nil, fmt.Errorf("failed to get unprocessed count: %w", err)
	}

	// Get total events
	err = ep.db.QueryRow(`
		SELECT COUNT(*) FROM domain_events
	`).Scan(&stats.TotalEvents)
	if err != nil {
		return nil, fmt.Errorf("failed to get total count: %w", err)
	}

	// Get events with high retry count
	err = ep.db.QueryRow(`
		SELECT COUNT(*) FROM domain_events WHERE retry_count >= $1
	`, ep.maxRetries).Scan(&stats.FailedEvents)
	if err != nil {
		return nil, fmt.Errorf("failed to get failed count: %w", err)
	}

	stats.ProcessedEvents = stats.TotalEvents - stats.UnprocessedEvents
	
	return &stats, nil
}

// ProcessingStats contains event processing statistics
type ProcessingStats struct {
	TotalEvents       int64 `json:"total_events"`
	ProcessedEvents   int64 `json:"processed_events"`
	UnprocessedEvents int64 `json:"unprocessed_events"`
	FailedEvents      int64 `json:"failed_events"`
}

// SetBatchSize sets the batch size for processing
func (ep *EventProcessor) SetBatchSize(size int) {
	ep.batchSize = size
}

// SetPollInterval sets the polling interval
func (ep *EventProcessor) SetPollInterval(interval time.Duration) {
	ep.pollInterval = interval
}

// SetMaxRetries sets the maximum retry count
func (ep *EventProcessor) SetMaxRetries(retries int) {
	ep.maxRetries = retries
}