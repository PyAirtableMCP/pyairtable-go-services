package repositories

import (
	"database/sql"
	"fmt"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"github.com/pyairtable-compose/go-services/domain/workspace"
)

// WorkspaceAggregateRepository implements the Repository interface for WorkspaceAggregate
type WorkspaceAggregateRepository struct {
	eventStore shared.EventStore
	db         *sql.DB
}

// NewWorkspaceAggregateRepository creates a new workspace aggregate repository
func NewWorkspaceAggregateRepository(eventStore shared.EventStore, db *sql.DB) *WorkspaceAggregateRepository {
	return &WorkspaceAggregateRepository{
		eventStore: eventStore,
		db:         db,
	}
}

// Save persists an aggregate and its uncommitted events
func (repo *WorkspaceAggregateRepository) Save(aggregate shared.AggregateRoot) error {
	workspaceAggregate, ok := aggregate.(*workspace.WorkspaceAggregate)
	if !ok {
		return fmt.Errorf("invalid aggregate type: expected WorkspaceAggregate")
	}

	// Get uncommitted events
	events := workspaceAggregate.GetUncommittedEvents()
	if len(events) == 0 {
		return nil // Nothing to save
	}

	// Save events to event store
	expectedVersion := workspaceAggregate.GetVersion().Int() - int64(len(events))
	err := repo.eventStore.SaveEvents(
		workspaceAggregate.GetID().String(),
		events,
		expectedVersion,
	)
	if err != nil {
		return fmt.Errorf("failed to save events: %w", err)
	}

	// Clear uncommitted events after successful save
	workspaceAggregate.ClearUncommittedEvents()

	return nil
}

// GetByID retrieves an aggregate by its ID
func (repo *WorkspaceAggregateRepository) GetByID(id shared.ID) (shared.AggregateRoot, error) {
	// Get events from event store
	events, err := repo.eventStore.GetEvents(id.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	if len(events) == 0 {
		return nil, shared.NewDomainError(shared.ErrCodeNotFound, "workspace not found", nil)
	}

	// Create a new aggregate instance from first event
	firstEvent := events[0]
	if firstEvent.EventType() != shared.EventTypeWorkspaceCreated {
		return nil, fmt.Errorf("first event must be WorkspaceCreated, got %s", firstEvent.EventType())
	}

	payload := firstEvent.Payload().(map[string]interface{})
	
	// Extract data from first event
	workspaceID := shared.NewWorkspaceIDFromString(payload["workspace_id"].(string))
	tenantID := shared.NewTenantIDFromString(payload["tenant_id"].(string))
	name := payload["name"].(string)
	description := payload["description"].(string)
	ownerID := shared.NewUserIDFromString(payload["owner_id"].(string))

	// Create aggregate with data from first event
	aggregate, err := workspace.NewWorkspaceAggregate(
		workspaceID,
		tenantID,
		name,
		description,
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace aggregate: %w", err)
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
func (repo *WorkspaceAggregateRepository) GetVersion(id shared.ID) (shared.Version, error) {
	events, err := repo.eventStore.GetEvents(id.String())
	if err != nil {
		return shared.NewVersion(), fmt.Errorf("failed to get events: %w", err)
	}

	if len(events) == 0 {
		return shared.NewVersion(), shared.NewDomainError(shared.ErrCodeNotFound, "workspace not found", nil)
	}

	// The version is the number of events
	return shared.NewVersionFromInt(int64(len(events))), nil
}

// Exists checks if an aggregate exists
func (repo *WorkspaceAggregateRepository) Exists(id shared.ID) (bool, error) {
	events, err := repo.eventStore.GetEvents(id.String())
	if err != nil {
		return false, fmt.Errorf("failed to check existence: %w", err)
	}

	return len(events) > 0, nil
}

// Workspace-specific methods

// GetByTenantID retrieves all workspaces for a tenant
func (repo *WorkspaceAggregateRepository) GetByTenantID(tenantID shared.TenantID) ([]*workspace.WorkspaceAggregate, error) {
	rows, err := repo.db.Query(`
		SELECT DISTINCT aggregate_id 
		FROM domain_events 
		WHERE event_type = $1 
		AND tenant_id = $2
		ORDER BY occurred_at ASC
	`, shared.EventTypeWorkspaceCreated, tenantID.String())
	
	if err != nil {
		return nil, fmt.Errorf("failed to query by tenant: %w", err)
	}
	defer rows.Close()

	var workspaces []*workspace.WorkspaceAggregate
	for rows.Next() {
		var aggregateID string
		if err := rows.Scan(&aggregateID); err != nil {
			return nil, fmt.Errorf("failed to scan aggregate ID: %w", err)
		}

		aggregate, err := repo.GetByID(shared.NewIDFromString(aggregateID))
		if err != nil {
			return nil, err
		}

		workspaces = append(workspaces, aggregate.(*workspace.WorkspaceAggregate))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return workspaces, nil
}

// GetByOwnerID retrieves all workspaces owned by a user
func (repo *WorkspaceAggregateRepository) GetByOwnerID(ownerID shared.UserID) ([]*workspace.WorkspaceAggregate, error) {
	rows, err := repo.db.Query(`
		SELECT DISTINCT aggregate_id 
		FROM domain_events 
		WHERE event_type = $1 
		AND payload->>'owner_id' = $2
		ORDER BY occurred_at ASC
	`, shared.EventTypeWorkspaceCreated, ownerID.String())
	
	if err != nil {
		return nil, fmt.Errorf("failed to query by owner: %w", err)
	}
	defer rows.Close()

	var workspaces []*workspace.WorkspaceAggregate
	for rows.Next() {
		var aggregateID string
		if err := rows.Scan(&aggregateID); err != nil {
			return nil, fmt.Errorf("failed to scan aggregate ID: %w", err)
		}

		aggregate, err := repo.GetByID(shared.NewIDFromString(aggregateID))
		if err != nil {
			return nil, err
		}

		workspaces = append(workspaces, aggregate.(*workspace.WorkspaceAggregate))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return workspaces, nil
}

// GetWorkspacesByUserMembership retrieves workspaces where a user is a member
func (repo *WorkspaceAggregateRepository) GetWorkspacesByUserMembership(userID shared.UserID) ([]*workspace.WorkspaceAggregate, error) {
	// This is more complex as we need to check MemberAdded events
	rows, err := repo.db.Query(`
		SELECT DISTINCT aggregate_id 
		FROM domain_events 
		WHERE event_type = $1 
		AND payload->>'user_id' = $2
		ORDER BY occurred_at ASC
	`, shared.EventTypeMemberAdded, userID.String())
	
	if err != nil {
		return nil, fmt.Errorf("failed to query workspaces by membership: %w", err)
	}
	defer rows.Close()

	var workspaces []*workspace.WorkspaceAggregate
	for rows.Next() {
		var aggregateID string
		if err := rows.Scan(&aggregateID); err != nil {
			return nil, fmt.Errorf("failed to scan aggregate ID: %w", err)
		}

		// Check if user is still a member (not removed)
		aggregate, err := repo.GetByID(shared.NewIDFromString(aggregateID))
		if err != nil {
			continue // Skip if we can't load the workspace
		}

		workspaceAggregate := aggregate.(*workspace.WorkspaceAggregate)
		if _, isMember := workspaceAggregate.GetMember(userID); isMember {
			workspaces = append(workspaces, workspaceAggregate)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return workspaces, nil
}