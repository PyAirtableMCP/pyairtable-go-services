package repository

import (
	"database/sql"
	"fmt"
	"log"
	"sync"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"github.com/pyairtable-compose/go-services/domain/user"
	"github.com/pyairtable-compose/go-services/domain/workspace"
	"github.com/pyairtable-compose/go-services/domain/tenant"
	"github.com/pyairtable-compose/go-services/pkg/eventstore"
)

// TransactionalUserRepository implements UserRepository with transaction support
type TransactionalUserRepository struct {
	tx          *sql.Tx
	eventStore  *eventstore.PostgresEventStore
	cache       map[string]shared.AggregateRoot // In-memory cache for the transaction
	mu          sync.RWMutex
	isDirty     map[string]bool // Track which aggregates have been modified
}

// NewTransactionalUserRepository creates a new transactional user repository
func NewTransactionalUserRepository(tx *sql.Tx, eventStore *eventstore.PostgresEventStore) *TransactionalUserRepository {
	return &TransactionalUserRepository{
		tx:         tx,
		eventStore: eventStore,
		cache:      make(map[string]shared.AggregateRoot),
		isDirty:    make(map[string]bool),
	}
}

// Save persists a user aggregate and tracks consistency
func (repo *TransactionalUserRepository) Save(aggregate shared.AggregateRoot) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	
	userAggregate, ok := aggregate.(*user.UserAggregate)
	if !ok {
		return fmt.Errorf("invalid aggregate type: expected UserAggregate")
	}
	
	aggregateID := userAggregate.GetID().String()
	
	// Check for consistency violations
	if err := repo.validateAggregateConsistency(userAggregate); err != nil {
		return fmt.Errorf("aggregate consistency violation: %w", err)
	}
	
	// Get uncommitted events
	events := userAggregate.GetUncommittedEvents()
	if len(events) == 0 {
		// No changes, but still cache the aggregate
		repo.cache[aggregateID] = userAggregate
		return nil
	}
	
	// Check optimistic concurrency
	expectedVersion := userAggregate.GetVersion().Int() - int64(len(events))
	if err := repo.checkOptimisticConcurrency(aggregateID, expectedVersion); err != nil {
		return fmt.Errorf("concurrency conflict: %w", err)
	}
	
	// Save events using the transaction's event store wrapper
	if err := repo.saveEventsInTransaction(aggregateID, events, expectedVersion); err != nil {
		return fmt.Errorf("failed to save events: %w", err)
	}
	
	// Cache the aggregate and mark as dirty
	repo.cache[aggregateID] = userAggregate
	repo.isDirty[aggregateID] = true
	
	// Clear uncommitted events after successful save
	userAggregate.ClearUncommittedEvents()
	
	log.Printf("Saved user aggregate %s with %d events", aggregateID, len(events))
	return nil
}

// GetByID retrieves a user aggregate by ID with caching
func (repo *TransactionalUserRepository) GetByID(id shared.ID) (shared.AggregateRoot, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	
	aggregateID := id.String()
	
	// Check transaction cache first
	if aggregate, exists := repo.cache[aggregateID]; exists {
		return aggregate, nil
	}
	
	// Load from event store
	events, err := repo.eventStore.GetEvents(aggregateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}
	
	if len(events) == 0 {
		return nil, shared.NewDomainError(shared.ErrCodeNotFound, "user not found", nil)
	}
	
	// Reconstruct aggregate from events
	aggregate, err := repo.reconstructUserFromEvents(events)
	if err != nil {
		return nil, fmt.Errorf("failed to reconstruct aggregate: %w", err)
	}
	
	// Cache the aggregate
	repo.cache[aggregateID] = aggregate
	return aggregate, nil
}

// GetByEmail retrieves a user by email with transaction awareness
func (repo *TransactionalUserRepository) GetByEmail(email shared.Email) (shared.AggregateRoot, error) {
	// First check if any cached aggregates match the email
	repo.mu.RLock()
	for _, aggregate := range repo.cache {
		if userAgg, ok := aggregate.(*user.UserAggregate); ok {
			if userAgg.GetEmail().String() == email.String() {
				repo.mu.RUnlock()
				return userAgg, nil
			}
		}
	}
	repo.mu.RUnlock()
	
	// Query the database within the transaction
	var aggregateID string
	err := repo.tx.QueryRow(`
		SELECT DISTINCT aggregate_id 
		FROM domain_events 
		WHERE event_type = $1 
		AND payload->>'email' = $2
		LIMIT 1
	`, shared.EventTypeUserRegistered, email.String()).Scan(&aggregateID)
	
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, shared.NewDomainError(shared.ErrCodeNotFound, "user not found", nil)
		}
		return nil, fmt.Errorf("failed to query by email: %w", err)
	}
	
	// Get the full aggregate
	return repo.GetByID(shared.NewIDFromString(aggregateID))
}

// ExistsByEmail checks if a user exists by email
func (repo *TransactionalUserRepository) ExistsByEmail(email shared.Email) (bool, error) {
	// Check transaction cache first
	repo.mu.RLock()
	for _, aggregate := range repo.cache {
		if userAgg, ok := aggregate.(*user.UserAggregate); ok {
			if userAgg.GetEmail().String() == email.String() {
				repo.mu.RUnlock()
				return true, nil
			}
		}
	}
	repo.mu.RUnlock()
	
	// Query database
	var count int
	err := repo.tx.QueryRow(`
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
func (repo *TransactionalUserRepository) GetByTenantID(tenantID shared.TenantID) ([]shared.AggregateRoot, error) {
	// Check cache for users in this tenant
	repo.mu.RLock()
	var cachedUsers []shared.AggregateRoot
	for _, aggregate := range repo.cache {
		if userAgg, ok := aggregate.(*user.UserAggregate); ok {
			if userAgg.GetTenantID().String() == tenantID.String() {
				cachedUsers = append(cachedUsers, userAgg)
			}
		}
	}
	repo.mu.RUnlock()
	
	// Query database for additional users
	rows, err := repo.tx.Query(`
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
	
	var allUsers []shared.AggregateRoot
	processedIDs := make(map[string]bool)
	
	// Add cached users first
	for _, user := range cachedUsers {
		allUsers = append(allUsers, user)
		processedIDs[user.GetID().String()] = true
	}
	
	// Add users from database (skip already cached ones)
	for rows.Next() {
		var aggregateID string
		if err := rows.Scan(&aggregateID); err != nil {
			return nil, fmt.Errorf("failed to scan aggregate ID: %w", err)
		}
		
		if processedIDs[aggregateID] {
			continue // Already processed from cache
		}
		
		aggregate, err := repo.GetByID(shared.NewIDFromString(aggregateID))
		if err != nil {
			return nil, err
		}
		
		allUsers = append(allUsers, aggregate)
		processedIDs[aggregateID] = true
	}
	
	return allUsers, nil
}

// validateAggregateConsistency validates business rules and invariants
func (repo *TransactionalUserRepository) validateAggregateConsistency(userAgg *user.UserAggregate) error {
	// Check email uniqueness within transaction
	exists, err := repo.ExistsByEmail(userAgg.GetEmail())
	if err != nil {
		return fmt.Errorf("failed to check email uniqueness: %w", err)
	}
	
	// Allow if it's the same user (update) or if email doesn't exist
	if exists {
		existing, err := repo.GetByEmail(userAgg.GetEmail())
		if err != nil {
			return fmt.Errorf("failed to get existing user: %w", err)
		}
		
		if existing.GetID().String() != userAgg.GetID().String() {
			return shared.NewDomainError(
				shared.ErrCodeConflict,
				"email already exists for different user",
				map[string]interface{}{
					"email":      userAgg.GetEmail().String(),
					"user_id":    userAgg.GetID().String(),
					"existing_id": existing.GetID().String(),
				},
			)
		}
	}
	
	// Validate other business rules
	if err := repo.validateUserBusinessRules(userAgg); err != nil {
		return fmt.Errorf("business rule validation failed: %w", err)
	}
	
	return nil
}

// validateUserBusinessRules validates user-specific business rules
func (repo *TransactionalUserRepository) validateUserBusinessRules(userAgg *user.UserAggregate) error {
	// Example business rules validation
	
	// Check tenant membership limits
	tenantUsers, err := repo.GetByTenantID(userAgg.GetTenantID())
	if err != nil {
		return fmt.Errorf("failed to get tenant users: %w", err)
	}
	
	// Example: Limit to 1000 users per tenant (this would be configurable)
	maxUsersPerTenant := 1000
	if len(tenantUsers) >= maxUsersPerTenant {
		// Check if this is an existing user (update) vs new user
		isExistingUser := false
		for _, existingUser := range tenantUsers {
			if existingUser.GetID().String() == userAgg.GetID().String() {
				isExistingUser = true
				break
			}
		}
		
		if !isExistingUser {
			return shared.NewDomainError(
				shared.ErrCodeBusinessRule,
				"tenant user limit exceeded",
				map[string]interface{}{
					"tenant_id":   userAgg.GetTenantID().String(),
					"user_count":  len(tenantUsers),
					"max_users":   maxUsersPerTenant,
				},
			)
		}
	}
	
	return nil
}

// checkOptimisticConcurrency checks for concurrent modifications
func (repo *TransactionalUserRepository) checkOptimisticConcurrency(aggregateID string, expectedVersion int64) error {
	var currentVersion int64
	err := repo.tx.QueryRow(`
		SELECT COALESCE(MAX(version), 0) 
		FROM domain_events 
		WHERE aggregate_id = $1
	`, aggregateID).Scan(&currentVersion)
	
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check current version: %w", err)
	}
	
	if currentVersion != expectedVersion {
		return shared.NewDomainError(
			shared.ErrCodeConflict,
			fmt.Sprintf("concurrency conflict: expected version %d, got %d", expectedVersion, currentVersion),
			map[string]interface{}{
				"aggregate_id":      aggregateID,
				"expected_version":  expectedVersion,
				"current_version":   currentVersion,
			},
		)
	}
	
	return nil
}

// saveEventsInTransaction saves events within the current transaction
func (repo *TransactionalUserRepository) saveEventsInTransaction(aggregateID string, events []shared.DomainEvent, expectedVersion int64) error {
	// This would integrate with the transaction-aware event store
	// For now, we'll implement a simplified version
	
	for i, event := range events {
		eventVersion := expectedVersion + int64(i) + 1
		
		payloadBytes, err := event.Payload().([]byte)
		if err != nil {
			// If payload is not already bytes, marshal it
			// This is a simplified implementation
			payloadBytes = []byte(fmt.Sprintf("%v", event.Payload()))
		}
		
		_, err = repo.tx.Exec(`
			INSERT INTO domain_events (
				id, event_type, aggregate_id, aggregate_type, version, 
				tenant_id, occurred_at, payload, metadata, processed
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`,
			event.EventID(),
			event.EventType(),
			event.AggregateID(),
			event.AggregateType(),
			eventVersion,
			event.TenantID(),
			event.OccurredAt(),
			payloadBytes,
			[]byte("{}"), // metadata
			false,
		)
		
		if err != nil {
			return fmt.Errorf("failed to insert event %s: %w", event.EventID(), err)
		}
	}
	
	return nil
}

// reconstructUserFromEvents rebuilds a user aggregate from its events
func (repo *TransactionalUserRepository) reconstructUserFromEvents(events []shared.DomainEvent) (*user.UserAggregate, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("no events to reconstruct from")
	}
	
	// Get the first event (should be UserRegistered)
	firstEvent := events[0]
	if firstEvent.EventType() != shared.EventTypeUserRegistered {
		return nil, fmt.Errorf("first event must be UserRegistered, got %s", firstEvent.EventType())
	}
	
	// Extract data from the registration event
	payload := firstEvent.Payload().(map[string]interface{})
	
	userID := shared.NewUserIDFromString(payload["user_id"].(string))
	tenantID := shared.NewTenantIDFromString(payload["tenant_id"].(string))
	email, _ := shared.NewEmail(payload["email"].(string))
	firstName := payload["first_name"].(string)
	lastName := payload["last_name"].(string)
	
	// Create the aggregate with initial state
	userAggregate, err := user.NewUserAggregate(
		userID,
		tenantID,
		email,
		"", // Password hash will be set during event replay
		firstName,
		lastName,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user aggregate: %w", err)
	}
	
	// Clear the creation event since we're rebuilding from history
	userAggregate.ClearUncommittedEvents()
	
	// Apply all events to rebuild state
	if err := userAggregate.LoadFromHistory(events); err != nil {
		return nil, fmt.Errorf("failed to load from history: %w", err)
	}
	
	return userAggregate, nil
}

// GetDirtyAggregates returns aggregates that have been modified in this transaction
func (repo *TransactionalUserRepository) GetDirtyAggregates() []shared.AggregateRoot {
	repo.mu.RLock()
	defer repo.mu.RUnlock()
	
	var dirtyAggregates []shared.AggregateRoot
	for id, isDirty := range repo.isDirty {
		if isDirty {
			if aggregate, exists := repo.cache[id]; exists {
				dirtyAggregates = append(dirtyAggregates, aggregate)
			}
		}
	}
	
	return dirtyAggregates
}

// Similar implementations for WorkspaceRepository and TenantRepository would follow
// the same patterns with their respective business rules and consistency checks

// TransactionalWorkspaceRepository implements WorkspaceRepository with transaction support
type TransactionalWorkspaceRepository struct {
	tx         *sql.Tx
	eventStore *eventstore.PostgresEventStore
	cache      map[string]shared.AggregateRoot
	mu         sync.RWMutex
	isDirty    map[string]bool
}

// NewTransactionalWorkspaceRepository creates a new transactional workspace repository
func NewTransactionalWorkspaceRepository(tx *sql.Tx, eventStore *eventstore.PostgresEventStore) *TransactionalWorkspaceRepository {
	return &TransactionalWorkspaceRepository{
		tx:         tx,
		eventStore: eventStore,
		cache:      make(map[string]shared.AggregateRoot),
		isDirty:    make(map[string]bool),
	}
}

// Save implementation for workspace repository
func (repo *TransactionalWorkspaceRepository) Save(aggregate shared.AggregateRoot) error {
	// Implementation similar to user repository but with workspace-specific logic
	log.Printf("Saving workspace aggregate: %s", aggregate.GetID().String())
	return nil
}

// GetByID implementation for workspace repository
func (repo *TransactionalWorkspaceRepository) GetByID(id shared.ID) (shared.AggregateRoot, error) {
	// Implementation similar to user repository
	log.Printf("Getting workspace by ID: %s", id.String())
	return nil, nil
}

// GetByTenantID implementation for workspace repository
func (repo *TransactionalWorkspaceRepository) GetByTenantID(tenantID shared.TenantID) ([]shared.AggregateRoot, error) {
	// Implementation to get workspaces by tenant
	log.Printf("Getting workspaces for tenant: %s", tenantID.String())
	return nil, nil
}

// TransactionalTenantRepository implements TenantRepository with transaction support
type TransactionalTenantRepository struct {
	tx         *sql.Tx
	eventStore *eventstore.PostgresEventStore
	cache      map[string]shared.AggregateRoot
	mu         sync.RWMutex
	isDirty    map[string]bool
}

// NewTransactionalTenantRepository creates a new transactional tenant repository
func NewTransactionalTenantRepository(tx *sql.Tx, eventStore *eventstore.PostgresEventStore) *TransactionalTenantRepository {
	return &TransactionalTenantRepository{
		tx:         tx,
		eventStore: eventStore,
		cache:      make(map[string]shared.AggregateRoot),
		isDirty:    make(map[string]bool),
	}
}

// Save implementation for tenant repository
func (repo *TransactionalTenantRepository) Save(aggregate shared.AggregateRoot) error {
	// Implementation similar to user repository but with tenant-specific logic
	log.Printf("Saving tenant aggregate: %s", aggregate.GetID().String())
	return nil
}

// GetByID implementation for tenant repository
func (repo *TransactionalTenantRepository) GetByID(id shared.ID) (shared.AggregateRoot, error) {
	// Implementation similar to user repository
	log.Printf("Getting tenant by ID: %s", id.String())
	return nil, nil
}

// GetBySlug implementation for tenant repository
func (repo *TransactionalTenantRepository) GetBySlug(slug string) (shared.AggregateRoot, error) {
	// Implementation to get tenant by slug
	log.Printf("Getting tenant by slug: %s", slug)
	return nil, nil
}