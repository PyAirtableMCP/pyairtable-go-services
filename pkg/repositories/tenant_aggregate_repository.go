package repositories

import (
	"database/sql"
	"fmt"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"github.com/pyairtable-compose/go-services/domain/tenant"
)

// TenantAggregateRepository implements the Repository interface for TenantAggregate
type TenantAggregateRepository struct {
	eventStore shared.EventStore
	db         *sql.DB
}

// NewTenantAggregateRepository creates a new tenant aggregate repository
func NewTenantAggregateRepository(eventStore shared.EventStore, db *sql.DB) *TenantAggregateRepository {
	return &TenantAggregateRepository{
		eventStore: eventStore,
		db:         db,
	}
}

// Save persists an aggregate and its uncommitted events
func (repo *TenantAggregateRepository) Save(aggregate shared.AggregateRoot) error {
	tenantAggregate, ok := aggregate.(*tenant.TenantAggregate)
	if !ok {
		return fmt.Errorf("invalid aggregate type: expected TenantAggregate")
	}

	// Get uncommitted events
	events := tenantAggregate.GetUncommittedEvents()
	if len(events) == 0 {
		return nil // Nothing to save
	}

	// Save events to event store
	expectedVersion := tenantAggregate.GetVersion().Int() - int64(len(events))
	err := repo.eventStore.SaveEvents(
		tenantAggregate.GetID().String(),
		events,
		expectedVersion,
	)
	if err != nil {
		return fmt.Errorf("failed to save events: %w", err)
	}

	// Clear uncommitted events after successful save
	tenantAggregate.ClearUncommittedEvents()

	return nil
}

// GetByID retrieves an aggregate by its ID
func (repo *TenantAggregateRepository) GetByID(id shared.ID) (shared.AggregateRoot, error) {
	// Get events from event store
	events, err := repo.eventStore.GetEvents(id.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	if len(events) == 0 {
		return nil, shared.NewDomainError(shared.ErrCodeNotFound, "tenant not found", nil)
	}

	// Create a new aggregate instance from first event
	firstEvent := events[0]
	if firstEvent.EventType() != shared.EventTypeTenantCreated {
		return nil, fmt.Errorf("first event must be TenantCreated, got %s", firstEvent.EventType())
	}

	payload := firstEvent.Payload().(map[string]interface{})
	
	// Extract data from first event
	tenantID := shared.NewTenantIDFromString(payload["tenant_id"].(string))
	name := payload["name"].(string)
	slug := payload["slug"].(string)
	domain := payload["domain"].(string)
	ownerEmail, _ := shared.NewEmail(payload["owner_email"].(string))

	// Create aggregate with data from first event
	aggregate, err := tenant.NewTenantAggregate(
		tenantID,
		name,
		slug,
		domain,
		ownerEmail,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create tenant aggregate: %w", err)
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
func (repo *TenantAggregateRepository) GetVersion(id shared.ID) (shared.Version, error) {
	events, err := repo.eventStore.GetEvents(id.String())
	if err != nil {
		return shared.NewVersion(), fmt.Errorf("failed to get events: %w", err)
	}

	if len(events) == 0 {
		return shared.NewVersion(), shared.NewDomainError(shared.ErrCodeNotFound, "tenant not found", nil)
	}

	// The version is the number of events
	return shared.NewVersionFromInt(int64(len(events))), nil
}

// Exists checks if an aggregate exists
func (repo *TenantAggregateRepository) Exists(id shared.ID) (bool, error) {
	events, err := repo.eventStore.GetEvents(id.String())
	if err != nil {
		return false, fmt.Errorf("failed to check existence: %w", err)
	}

	return len(events) > 0, nil
}

// Tenant-specific methods

// GetBySlug retrieves a tenant by slug
func (repo *TenantAggregateRepository) GetBySlug(slug string) (*tenant.TenantAggregate, error) {
	// Query events table for TenantCreated events with the specified slug
	rows, err := repo.db.Query(`
		SELECT DISTINCT aggregate_id 
		FROM domain_events 
		WHERE event_type = $1 
		AND payload->>'slug' = $2
	`, shared.EventTypeTenantCreated, slug)
	
	if err != nil {
		return nil, fmt.Errorf("failed to query by slug: %w", err)
	}
	defer rows.Close()

	var aggregateID string
	if !rows.Next() {
		return nil, shared.NewDomainError(shared.ErrCodeNotFound, "tenant not found", nil)
	}

	if err := rows.Scan(&aggregateID); err != nil {
		return nil, fmt.Errorf("failed to scan aggregate ID: %w", err)
	}

	// Get the full aggregate
	aggregate, err := repo.GetByID(shared.NewIDFromString(aggregateID))
	if err != nil {
		return nil, err
	}

	return aggregate.(*tenant.TenantAggregate), nil
}

// ExistsBySlug checks if a tenant exists by slug
func (repo *TenantAggregateRepository) ExistsBySlug(slug string) (bool, error) {
	var count int
	err := repo.db.QueryRow(`
		SELECT COUNT(DISTINCT aggregate_id) 
		FROM domain_events 
		WHERE event_type = $1 
		AND payload->>'slug' = $2
	`, shared.EventTypeTenantCreated, slug).Scan(&count)
	
	if err != nil {
		return false, fmt.Errorf("failed to check slug existence: %w", err)
	}

	return count > 0, nil
}

// ExistsByDomain checks if a tenant exists by domain
func (repo *TenantAggregateRepository) ExistsByDomain(domain string) (bool, error) {
	var count int
	err := repo.db.QueryRow(`
		SELECT COUNT(DISTINCT aggregate_id) 
		FROM domain_events 
		WHERE event_type = $1 
		AND payload->>'domain' = $2
	`, shared.EventTypeTenantCreated, domain).Scan(&count)
	
	if err != nil {
		return false, fmt.Errorf("failed to check domain existence: %w", err)
	}

	return count > 0, nil
}

// ListAll retrieves all tenants
func (repo *TenantAggregateRepository) ListAll() ([]*tenant.TenantAggregate, error) {
	rows, err := repo.db.Query(`
		SELECT DISTINCT aggregate_id 
		FROM domain_events 
		WHERE event_type = $1
		ORDER BY occurred_at ASC
	`, shared.EventTypeTenantCreated)
	
	if err != nil {
		return nil, fmt.Errorf("failed to query all tenants: %w", err)
	}
	defer rows.Close()

	var tenants []*tenant.TenantAggregate
	for rows.Next() {
		var aggregateID string
		if err := rows.Scan(&aggregateID); err != nil {
			return nil, fmt.Errorf("failed to scan aggregate ID: %w", err)
		}

		aggregate, err := repo.GetByID(shared.NewIDFromString(aggregateID))
		if err != nil {
			return nil, err
		}

		tenants = append(tenants, aggregate.(*tenant.TenantAggregate))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return tenants, nil
}