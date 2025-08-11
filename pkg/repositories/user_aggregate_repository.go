package repositories

import (
	"database/sql"
	"fmt"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"github.com/pyairtable-compose/go-services/domain/user"
	"github.com/pyairtable-compose/go-services/pkg/eventstore"
)

// UserAggregateRepository implements the Repository interface for UserAggregate
type UserAggregateRepository struct {
	eventStore shared.EventStore
	db         *sql.DB
}

// NewUserAggregateRepository creates a new user aggregate repository
func NewUserAggregateRepository(eventStore shared.EventStore, db *sql.DB) *UserAggregateRepository {
	return &UserAggregateRepository{
		eventStore: eventStore,
		db:         db,
	}
}

// Save persists an aggregate and its uncommitted events
func (repo *UserAggregateRepository) Save(aggregate shared.AggregateRoot) error {
	userAggregate, ok := aggregate.(*user.UserAggregate)
	if !ok {
		return fmt.Errorf("invalid aggregate type: expected UserAggregate")
	}

	// Get uncommitted events
	events := userAggregate.GetUncommittedEvents()
	if len(events) == 0 {
		return nil // Nothing to save
	}

	// Save events to event store
	expectedVersion := userAggregate.GetVersion().Int() - int64(len(events))
	err := repo.eventStore.SaveEvents(
		userAggregate.GetID().String(),
		events,
		expectedVersion,
	)
	if err != nil {
		return fmt.Errorf("failed to save events: %w", err)
	}

	// Clear uncommitted events after successful save
	userAggregate.ClearUncommittedEvents()

	return nil
}

// GetByID retrieves an aggregate by its ID
func (repo *UserAggregateRepository) GetByID(id shared.ID) (shared.AggregateRoot, error) {
	// Get events from event store
	events, err := repo.eventStore.GetEvents(id.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	if len(events) == 0 {
		return nil, shared.NewDomainError(shared.ErrCodeNotFound, "user not found", nil)
	}

	// Create a new aggregate instance
	// We'll get basic info from the first event (UserRegistered)
	firstEvent := events[0]
	if firstEvent.EventType() != shared.EventTypeUserRegistered {
		return nil, fmt.Errorf("first event must be UserRegistered, got %s", firstEvent.EventType())
	}

	payload := firstEvent.Payload().(map[string]interface{})
	
	// Extract data from first event
	userID := shared.NewUserIDFromString(payload["user_id"].(string))
	tenantID := shared.NewTenantIDFromString(payload["tenant_id"].(string))
	email, _ := shared.NewEmail(payload["email"].(string))
	firstName := payload["first_name"].(string)
	lastName := payload["last_name"].(string)

	// Create aggregate with minimal data
	aggregate, err := user.NewUserAggregate(
		userID,
		tenantID,
		email,
		"placeholder_hash", // Password will be set from events
		firstName,
		lastName,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user aggregate: %w", err)
	}

	// Clear the creation event since we're rebuilding from history
	aggregate.ClearUncommittedEvents()

	// Load from history
	if err := aggregate.LoadFromHistory(events); err != nil {
		return nil, fmt.Errorf("failed to load aggregate from history: %w", err)
	}

	return aggregate, nil
}

// GetVersion retrieves the current version of an aggregate
func (repo *UserAggregateRepository) GetVersion(id shared.ID) (shared.Version, error) {
	events, err := repo.eventStore.GetEvents(id.String())
	if err != nil {
		return shared.NewVersion(), fmt.Errorf("failed to get events: %w", err)
	}

	if len(events) == 0 {
		return shared.NewVersion(), shared.NewDomainError(shared.ErrCodeNotFound, "user not found", nil)
	}

	// The version is the number of events
	return shared.NewVersionFromInt(int64(len(events))), nil
}

// Exists checks if an aggregate exists
func (repo *UserAggregateRepository) Exists(id shared.ID) (bool, error) {
	events, err := repo.eventStore.GetEvents(id.String())
	if err != nil {
		return false, fmt.Errorf("failed to check existence: %w", err)
	}

	return len(events) > 0, nil
}

// UserSpecificMethods - these methods are specific to UserAggregate

// GetByEmail retrieves a user aggregate by email
func (repo *UserAggregateRepository) GetByEmail(email shared.Email) (*user.UserAggregate, error) {
	// This is a simplified implementation. In a real system, you might:
	// 1. Maintain a projection/read model for email lookups
	// 2. Use a separate index table
	// 3. Query all UserRegistered events and filter by email
	
	// For now, we'll use a simple approach with a SQL query on the events table
	rows, err := repo.db.Query(`
		SELECT DISTINCT aggregate_id 
		FROM domain_events 
		WHERE event_type = $1 
		AND payload->>'email' = $2
	`, shared.EventTypeUserRegistered, email.String())
	
	if err != nil {
		return nil, fmt.Errorf("failed to query by email: %w", err)
	}
	defer rows.Close()

	var aggregateID string
	if !rows.Next() {
		return nil, shared.NewDomainError(shared.ErrCodeNotFound, "user not found", nil)
	}

	if err := rows.Scan(&aggregateID); err != nil {
		return nil, fmt.Errorf("failed to scan aggregate ID: %w", err)
	}

	// Get the full aggregate
	aggregate, err := repo.GetByID(shared.NewIDFromString(aggregateID))
	if err != nil {
		return nil, err
	}

	return aggregate.(*user.UserAggregate), nil
}

// ExistsByEmail checks if a user exists by email
func (repo *UserAggregateRepository) ExistsByEmail(email shared.Email) (bool, error) {
	var count int
	err := repo.db.QueryRow(`
		SELECT COUNT(DISTINCT aggregate_id) 
		FROM domain_events 
		WHERE event_type = $1 
		AND payload->>'email' = $2
	`, shared.EventTypeUserRegistered, email.String()).Scan(&count)
	
	if err != nil {
		return false, fmt.Errorf("failed to check email existence: %w", err)
	}

	return count > 0, nil
}

// GetByTenantID retrieves all users for a tenant
func (repo *UserAggregateRepository) GetByTenantID(tenantID shared.TenantID) ([]*user.UserAggregate, error) {
	rows, err := repo.db.Query(`
		SELECT DISTINCT aggregate_id 
		FROM domain_events 
		WHERE event_type = $1 
		AND tenant_id = $2
		ORDER BY occurred_at ASC
	`, shared.EventTypeUserRegistered, tenantID.String())
	
	if err != nil {
		return nil, fmt.Errorf("failed to query by tenant: %w", err)
	}
	defer rows.Close()

	var users []*user.UserAggregate
	for rows.Next() {
		var aggregateID string
		if err := rows.Scan(&aggregateID); err != nil {
			return nil, fmt.Errorf("failed to scan aggregate ID: %w", err)
		}

		aggregate, err := repo.GetByID(shared.NewIDFromString(aggregateID))
		if err != nil {
			return nil, err
		}

		users = append(users, aggregate.(*user.UserAggregate))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return users, nil
}