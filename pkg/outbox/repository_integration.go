package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/pyairtable-compose/go-services/domain/shared"
)

// OutboxRepositoryWrapper wraps repository operations with automatic outbox event publishing
type OutboxRepositoryWrapper struct {
	db     *sql.DB
	outbox *EnhancedTransactionalOutbox
}

// NewOutboxRepositoryWrapper creates a new repository wrapper with outbox integration
func NewOutboxRepositoryWrapper(db *sql.DB, outbox *EnhancedTransactionalOutbox) *OutboxRepositoryWrapper {
	return &OutboxRepositoryWrapper{
		db:     db,
		outbox: outbox,
	}
}

// TransactionalOperation represents an operation that should be executed within a transaction
type TransactionalOperation func(tx *sql.Tx) error

// EventProducingOperation represents an operation that produces domain events
type EventProducingOperation struct {
	Operation TransactionalOperation
	Events    []shared.DomainEvent
}

// ExecuteWithOutbox executes an operation within a transaction and adds events to outbox
func (w *OutboxRepositoryWrapper) ExecuteWithOutbox(
	ctx context.Context,
	operation EventProducingOperation,
) error {
	// Start transaction
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute the business operation
	if err := operation.Operation(tx); err != nil {
		return fmt.Errorf("business operation failed: %w", err)
	}

	// Add events to outbox within the same transaction
	if len(operation.Events) > 0 {
		for _, event := range operation.Events {
			if err := w.outbox.AddEventWithDeliveryTracking(tx, event); err != nil {
				return fmt.Errorf("failed to add event to outbox: %w", err)
			}
		}
	}

	// Commit transaction (both business data and outbox events)
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully executed operation with %d events added to outbox", len(operation.Events))
	return nil
}

// AggregateRepository interface for repositories that work with aggregates
type AggregateRepository interface {
	Save(ctx context.Context, aggregate shared.AggregateRoot) error
	GetByID(ctx context.Context, id string) (shared.AggregateRoot, error)
}

// OutboxEnabledAggregateRepository wraps aggregate repositories with outbox support
type OutboxEnabledAggregateRepository struct {
	repo    AggregateRepository
	wrapper *OutboxRepositoryWrapper
}

// NewOutboxEnabledAggregateRepository creates a new outbox-enabled aggregate repository
func NewOutboxEnabledAggregateRepository(
	repo AggregateRepository,
	wrapper *OutboxRepositoryWrapper,
) *OutboxEnabledAggregateRepository {
	return &OutboxEnabledAggregateRepository{
		repo:    repo,
		wrapper: wrapper,
	}
}

// Save saves an aggregate and publishes its events via outbox
func (r *OutboxEnabledAggregateRepository) Save(ctx context.Context, aggregate shared.AggregateRoot) error {
	// Get uncommitted events from aggregate
	events := aggregate.GetUncommittedEvents()
	
	// Create operation that saves aggregate and its events
	operation := EventProducingOperation{
		Operation: func(tx *sql.Tx) error {
			// Create a context with the transaction for the repository
			// Note: This assumes the underlying repository can work with transactions
			// You might need to adapt this based on your repository implementation
			return r.saveAggregateInTransaction(ctx, tx, aggregate)
		},
		Events: events,
	}

	// Execute with outbox
	if err := r.wrapper.ExecuteWithOutbox(ctx, operation); err != nil {
		return err
	}

	// Mark events as committed in aggregate
	aggregate.MarkEventsAsCommitted()
	
	return nil
}

// GetByID delegates to the underlying repository
func (r *OutboxEnabledAggregateRepository) GetByID(ctx context.Context, id string) (shared.AggregateRoot, error) {
	return r.repo.GetByID(ctx, id)
}

// saveAggregateInTransaction saves the aggregate within a transaction
// This is a helper method that adapts the repository interface to work with transactions
func (r *OutboxEnabledAggregateRepository) saveAggregateInTransaction(
	ctx context.Context,
	tx *sql.Tx,
	aggregate shared.AggregateRoot,
) error {
	// If the underlying repository supports transactions, use it directly
	if txRepo, ok := r.repo.(TransactionalRepository); ok {
		return txRepo.SaveInTransaction(ctx, tx, aggregate)
	}
	
	// Otherwise, you'll need to implement transaction support specific to your repositories
	// This is a placeholder - implement based on your specific repository implementations
	return fmt.Errorf("repository does not support transactions")
}

// TransactionalRepository interface for repositories that support explicit transactions
type TransactionalRepository interface {
	SaveInTransaction(ctx context.Context, tx *sql.Tx, aggregate shared.AggregateRoot) error
}

// BatchEventOperation allows batching multiple operations with their events
type BatchEventOperation struct {
	Operations []EventProducingOperation
}

// ExecuteBatch executes multiple operations within a single transaction
func (w *OutboxRepositoryWrapper) ExecuteBatch(
	ctx context.Context,
	batchOp BatchEventOperation,
) error {
	// Start transaction
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	var allEvents []shared.DomainEvent

	// Execute all operations
	for i, operation := range batchOp.Operations {
		if err := operation.Operation(tx); err != nil {
			return fmt.Errorf("batch operation %d failed: %w", i, err)
		}
		allEvents = append(allEvents, operation.Events...)
	}

	// Add all events to outbox
	for _, event := range allEvents {
		if err := w.outbox.AddEventWithDeliveryTracking(tx, event); err != nil {
			return fmt.Errorf("failed to add event to outbox: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit batch transaction: %w", err)
	}

	log.Printf("Successfully executed batch with %d operations and %d events", 
		len(batchOp.Operations), len(allEvents))
	return nil
}

// OutboxEventStore wraps the event store with outbox integration
type OutboxEventStore struct {
	eventStore shared.EventStore
	wrapper    *OutboxRepositoryWrapper
}

// NewOutboxEventStore creates a new outbox-enabled event store
func NewOutboxEventStore(
	eventStore shared.EventStore,
	wrapper *OutboxRepositoryWrapper,
) *OutboxEventStore {
	return &OutboxEventStore{
		eventStore: eventStore,
		wrapper:    wrapper,
	}
}

// SaveEvents saves events to the event store and outbox transactionally
func (es *OutboxEventStore) SaveEvents(
	aggregateID string,
	events []shared.DomainEvent,
	expectedVersion int64,
) error {
	ctx := context.Background()
	
	operation := EventProducingOperation{
		Operation: func(tx *sql.Tx) error {
			// If the event store supports transactions, use it
			if txEventStore, ok := es.eventStore.(TransactionalEventStore); ok {
				return txEventStore.SaveEventsInTransaction(tx, aggregateID, events, expectedVersion)
			}
			
			// Otherwise, delegate to the regular event store
			// Note: This loses transactional guarantees - consider refactoring your event store
			return es.eventStore.SaveEvents(aggregateID, events, expectedVersion)
		},
		Events: events,
	}

	return es.wrapper.ExecuteWithOutbox(ctx, operation)
}

// GetEvents delegates to the underlying event store
func (es *OutboxEventStore) GetEvents(aggregateID string) ([]shared.DomainEvent, error) {
	return es.eventStore.GetEvents(aggregateID)
}

// GetEventsFromVersion delegates to the underlying event store
func (es *OutboxEventStore) GetEventsFromVersion(aggregateID string, fromVersion int64) ([]shared.DomainEvent, error) {
	return es.eventStore.GetEventsFromVersion(aggregateID, fromVersion)
}

// GetEventsByType delegates to the underlying event store
func (es *OutboxEventStore) GetEventsByType(eventType string, from, to time.Time) ([]shared.DomainEvent, error) {
	return es.eventStore.GetEventsByType(eventType, from, to)
}

// TransactionalEventStore interface for event stores that support transactions
type TransactionalEventStore interface {
	SaveEventsInTransaction(
		tx *sql.Tx,
		aggregateID string,
		events []shared.DomainEvent,
		expectedVersion int64,
	) error
}

// SAGA integration with outbox
type OutboxSagaManager struct {
	wrapper *OutboxRepositoryWrapper
}

// NewOutboxSagaManager creates a new SAGA manager with outbox integration
func NewOutboxSagaManager(wrapper *OutboxRepositoryWrapper) *OutboxSagaManager {
	return &OutboxSagaManager{
		wrapper: wrapper,
	}
}

// ExecuteSagaStep executes a SAGA step with outbox event publishing
func (sm *OutboxSagaManager) ExecuteSagaStep(
	ctx context.Context,
	sagaID string,
	stepOperation func(tx *sql.Tx) error,
	compensationEvents []shared.DomainEvent,
	resultEvents []shared.DomainEvent,
) error {
	operation := EventProducingOperation{
		Operation: func(tx *sql.Tx) error {
			// Update SAGA step status
			if err := sm.updateSagaStepStatus(tx, sagaID, "running"); err != nil {
				return fmt.Errorf("failed to update saga step status: %w", err)
			}
			
			// Execute the step operation
			if err := stepOperation(tx); err != nil {
				// Mark step as failed and add compensation events
				if updateErr := sm.updateSagaStepStatus(tx, sagaID, "failed"); updateErr != nil {
					log.Printf("Failed to update saga step status to failed: %v", updateErr)
				}
				return err
			}
			
			// Mark step as completed
			return sm.updateSagaStepStatus(tx, sagaID, "completed")
		},
		Events: resultEvents,
	}

	return sm.wrapper.ExecuteWithOutbox(ctx, operation)
}

// updateSagaStepStatus updates the status of a SAGA step
func (sm *OutboxSagaManager) updateSagaStepStatus(tx *sql.Tx, sagaID string, status string) error {
	query := `
		UPDATE saga_steps 
		SET status = $1, completed_at = CASE WHEN $1 IN ('completed', 'failed') THEN NOW() ELSE completed_at END
		WHERE saga_id = $2 AND status = 'running'
	`
	
	_, err := tx.Exec(query, status, sagaID)
	return err
}

// Helper functions for common patterns

// WithOutboxEvents wraps a function to produce events for outbox
func WithOutboxEvents(
	operation func(tx *sql.Tx) error,
	events []shared.DomainEvent,
) EventProducingOperation {
	return EventProducingOperation{
		Operation: operation,
		Events:    events,
	}
}

// EmptyOperation creates an operation with no business logic but events to publish
func EmptyOperation(events []shared.DomainEvent) EventProducingOperation {
	return EventProducingOperation{
		Operation: func(tx *sql.Tx) error { return nil },
		Events:    events,
	}
}

// CombineOperations combines multiple operations into a batch
func CombineOperations(operations ...EventProducingOperation) BatchEventOperation {
	return BatchEventOperation{
		Operations: operations,
	}
}