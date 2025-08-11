package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"github.com/pyairtable-compose/go-services/domain/user"
	"github.com/pyairtable-compose/go-services/domain/workspace"
	"github.com/pyairtable-compose/go-services/domain/tenant"
	"github.com/pyairtable-compose/go-services/pkg/eventstore"
	"github.com/pyairtable-compose/go-services/pkg/outbox"
)

// UnitOfWork defines the interface for transaction boundary management
type UnitOfWork interface {
	// Transaction management
	Begin() error
	Commit() error
	Rollback() error
	IsActive() bool
	
	// Repository access
	UserRepository() UserRepository
	WorkspaceRepository() WorkspaceRepository
	TenantRepository() TenantRepository
	
	// Aggregate tracking and event management
	RegisterAggregate(aggregate shared.AggregateRoot) error
	GetTrackedAggregates() []shared.AggregateRoot
	CollectUncommittedEvents() []shared.DomainEvent
	AddDomainEvents(events []shared.DomainEvent) error
	GetDomainEvents() []shared.DomainEvent
	ClearDomainEvents()
	
	// Validation and consistency
	ValidateAggregateConsistency() error
	ValidateBusinessRules() error
	
	// Context management
	SetContext(ctx context.Context)
	GetContext() context.Context
	
	// Event collector access
	GetEventCollector() *AggregateEventCollector
	
	// Concurrency control access
	GetConcurrencyManager() *ConcurrencyControlManager
}

// UserRepository interface for user aggregate operations
type UserRepository interface {
	Save(aggregate shared.AggregateRoot) error
	GetByID(id shared.ID) (shared.AggregateRoot, error)
	GetByEmail(email shared.Email) (shared.AggregateRoot, error)
	ExistsByEmail(email shared.Email) (bool, error)
	GetByTenantID(tenantID shared.TenantID) ([]shared.AggregateRoot, error)
}

// WorkspaceRepository interface for workspace aggregate operations
type WorkspaceRepository interface {
	Save(aggregate shared.AggregateRoot) error
	GetByID(id shared.ID) (shared.AggregateRoot, error)
	GetByTenantID(tenantID shared.TenantID) ([]shared.AggregateRoot, error)
}

// TenantRepository interface for tenant aggregate operations
type TenantRepository interface {
	Save(aggregate shared.AggregateRoot) error
	GetByID(id shared.ID) (shared.AggregateRoot, error)
	GetBySlug(slug string) (shared.AggregateRoot, error)
}

// UnitOfWorkConfig defines configuration for Unit of Work
type UnitOfWorkConfig struct {
	TransactionTimeout time.Duration
	IsolationLevel     sql.IsolationLevel
	EnableOutbox       bool
	EnableEventStore   bool
	MaxRetries         int
}

// SqlUnitOfWork implements UnitOfWork using SQL transactions
type SqlUnitOfWork struct {
	db       *sql.DB
	tx       *sql.Tx
	ctx      context.Context
	config   UnitOfWorkConfig
	isActive bool
	mu       sync.RWMutex
	
	// Aggregate tracking
	trackedAggregates map[string]shared.AggregateRoot
	aggregateVersions map[string]shared.Version
	
	// Domain events collected during the transaction
	domainEvents []shared.DomainEvent
	
	// Repositories
	userRepo      UserRepository
	workspaceRepo WorkspaceRepository
	tenantRepo    TenantRepository
	
	// Infrastructure
	eventStore         *eventstore.PostgresEventStore
	outbox             *outbox.TransactionalOutbox
	eventCollector     *AggregateEventCollector
	concurrencyManager *ConcurrencyControlManager
	
	// Validation
	validator *AggregateValidator
}

// NewSqlUnitOfWork creates a new SQL-based Unit of Work
func NewSqlUnitOfWork(db *sql.DB, config UnitOfWorkConfig) *SqlUnitOfWork {
	validator := NewAggregateValidator()
	// Register default validators
	validator.RegisterValidator(&UserAggregateValidator{})
	validator.RegisterValidator(&WorkspaceAggregateValidator{})
	// Register default business rules
	validator.RegisterBusinessRule(&UniqueEmailBusinessRule{})
	
	// Set up event collector with default handlers
	eventCollector := NewAggregateEventCollector()
	eventCollector.RegisterEventHandler("*", NewLoggingEventHandler("INFO"))
	
	// Set up concurrency control with smart resolver
	fallbackResolver := NewRetryOnConflictResolver(3)
	smartResolver := NewSmartConflictResolver(fallbackResolver)
	concurrencyManager := NewConcurrencyControlManager(smartResolver)
	
	uow := &SqlUnitOfWork{
		db:                 db,
		config:             config,
		trackedAggregates:  make(map[string]shared.AggregateRoot),
		aggregateVersions:  make(map[string]shared.Version),
		domainEvents:       make([]shared.DomainEvent, 0),
		validator:          validator,
		eventCollector:     eventCollector,
		concurrencyManager: concurrencyManager,
	}
	
	// Initialize infrastructure components
	if config.EnableEventStore {
		uow.eventStore = eventstore.NewPostgresEventStore(db)
	}
	
	if config.EnableOutbox {
		outboxConfig := outbox.OutboxConfig{
			TableName:           "outbox_events",
			BatchSize:           100,
			PollInterval:        5 * time.Second,
			MaxRetries:          3,
			RetryBackoffBase:    time.Second,
			RetryBackoffMax:     30 * time.Second,
			WorkerCount:         2,
			EnableDeduplication: true,
			PublishTimeout:      30 * time.Second,
		}
		// Note: Publisher would be injected in real implementation
		uow.outbox = outbox.NewTransactionalOutbox(db, outboxConfig, nil)
	}
	
	return uow
}

// Begin starts a new transaction
func (uow *SqlUnitOfWork) Begin() error {
	uow.mu.Lock()
	defer uow.mu.Unlock()
	
	if uow.isActive {
		return fmt.Errorf("transaction already active")
	}
	
	// Set up context with timeout
	ctx := uow.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	
	if uow.config.TransactionTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, uow.config.TransactionTimeout)
		defer cancel()
	}
	
	// Begin transaction with isolation level
	txOptions := &sql.TxOptions{
		Isolation: uow.config.IsolationLevel,
		ReadOnly:  false,
	}
	
	tx, err := uow.db.BeginTx(ctx, txOptions)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	uow.tx = tx
	uow.ctx = ctx
	uow.isActive = true
	
	// Initialize repositories with the transaction
	uow.initializeRepositories()
	
	log.Printf("Unit of Work transaction started with isolation level: %v", uow.config.IsolationLevel)
	return nil
}

// Commit commits the transaction and publishes domain events
func (uow *SqlUnitOfWork) Commit() error {
	uow.mu.Lock()
	defer uow.mu.Unlock()
	
	if !uow.isActive {
		return fmt.Errorf("no active transaction to commit")
	}
	
	defer func() {
		uow.isActive = false
		uow.tx = nil
		// Clear tracking after transaction
		uow.trackedAggregates = make(map[string]shared.AggregateRoot)
		uow.aggregateVersions = make(map[string]shared.Version)
	}()
	
	// Validate optimistic concurrency control before anything else
	if err := uow.validateOptimisticConcurrency(); err != nil {
		uow.tx.Rollback()
		return fmt.Errorf("optimistic concurrency validation failed: %w", err)
	}
	
	// Validate business rules and consistency before committing
	if err := uow.ValidateBusinessRules(); err != nil {
		uow.tx.Rollback()
		return fmt.Errorf("business rule validation failed: %w", err)
	}
	
	if err := uow.ValidateAggregateConsistency(); err != nil {
		uow.tx.Rollback()
		return fmt.Errorf("aggregate consistency validation failed: %w", err)
	}
	
	// Collect all uncommitted events from tracked aggregates
	if err := uow.collectAllUncommittedEvents(); err != nil {
		uow.tx.Rollback()
		return fmt.Errorf("failed to collect uncommitted events: %w", err)
	}
	
	// Process domain events before committing
	if err := uow.processDomainEvents(); err != nil {
		uow.tx.Rollback()
		return fmt.Errorf("failed to process domain events: %w", err)
	}
	
	// Commit the transaction
	if err := uow.tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	// Update version tracking after successful commit
	uow.updateVersionsAfterSave()
	
	// Clear uncommitted events from all tracked aggregates after successful commit
	uow.clearAggregateEvents()
	
	log.Printf("Unit of Work transaction committed successfully with %d domain events", len(uow.domainEvents))
	
	// Clear domain events after successful commit
	uow.domainEvents = nil
	
	return nil
}

// Rollback rolls back the transaction
func (uow *SqlUnitOfWork) Rollback() error {
	uow.mu.Lock()
	defer uow.mu.Unlock()
	
	if !uow.isActive {
		return fmt.Errorf("no active transaction to rollback")
	}
	
	defer func() {
		uow.isActive = false
		uow.tx = nil
	}()
	
	if err := uow.tx.Rollback(); err != nil {
		return fmt.Errorf("failed to rollback transaction: %w", err)
	}
	
	log.Printf("Unit of Work transaction rolled back, clearing %d domain events", len(uow.domainEvents))
	
	// Clear domain events after rollback
	uow.domainEvents = nil
	
	return nil
}

// IsActive returns whether a transaction is currently active
func (uow *SqlUnitOfWork) IsActive() bool {
	uow.mu.RLock()
	defer uow.mu.RUnlock()
	return uow.isActive
}

// SetContext sets the context for the Unit of Work
func (uow *SqlUnitOfWork) SetContext(ctx context.Context) {
	uow.mu.Lock()
	defer uow.mu.Unlock()
	uow.ctx = ctx
}

// GetContext returns the current context
func (uow *SqlUnitOfWork) GetContext() context.Context {
	uow.mu.RLock()
	defer uow.mu.RUnlock()
	return uow.ctx
}

// GetEventCollector returns the event collector for customization
func (uow *SqlUnitOfWork) GetEventCollector() *AggregateEventCollector {
	uow.mu.RLock()
	defer uow.mu.RUnlock()
	return uow.eventCollector
}

// GetConcurrencyManager returns the concurrency control manager for customization
func (uow *SqlUnitOfWork) GetConcurrencyManager() *ConcurrencyControlManager {
	uow.mu.RLock()
	defer uow.mu.RUnlock()
	return uow.concurrencyManager
}

// UserRepository returns the user repository
func (uow *SqlUnitOfWork) UserRepository() UserRepository {
	uow.mu.RLock()
	defer uow.mu.RUnlock()
	return uow.userRepo
}

// WorkspaceRepository returns the workspace repository
func (uow *SqlUnitOfWork) WorkspaceRepository() WorkspaceRepository {
	uow.mu.RLock()
	defer uow.mu.RUnlock()
	return uow.workspaceRepo
}

// TenantRepository returns the tenant repository
func (uow *SqlUnitOfWork) TenantRepository() TenantRepository {
	uow.mu.RLock()
	defer uow.mu.RUnlock()
	return uow.tenantRepo
}

// AddDomainEvents adds domain events to be processed during commit
func (uow *SqlUnitOfWork) AddDomainEvents(events []shared.DomainEvent) error {
	uow.mu.Lock()
	defer uow.mu.Unlock()
	
	if !uow.isActive {
		return fmt.Errorf("no active transaction")
	}
	
	uow.domainEvents = append(uow.domainEvents, events...)
	return nil
}

// GetDomainEvents returns the current domain events
func (uow *SqlUnitOfWork) GetDomainEvents() []shared.DomainEvent {
	uow.mu.RLock()
	defer uow.mu.RUnlock()
	
	// Return a copy to prevent external modification
	events := make([]shared.DomainEvent, len(uow.domainEvents))
	copy(events, uow.domainEvents)
	return events
}

// ClearDomainEvents clears the domain events
func (uow *SqlUnitOfWork) ClearDomainEvents() {
	uow.mu.Lock()
	defer uow.mu.Unlock()
	uow.domainEvents = nil
}

// RegisterAggregate registers an aggregate for tracking within the transaction
func (uow *SqlUnitOfWork) RegisterAggregate(aggregate shared.AggregateRoot) error {
	uow.mu.Lock()
	defer uow.mu.Unlock()
	
	if !uow.isActive {
		return fmt.Errorf("no active transaction")
	}
	
	aggregateID := aggregate.GetID().String()
	
	// Check if already tracked
	if existing, exists := uow.trackedAggregates[aggregateID]; exists {
		// Verify it's the same aggregate type
		if existing.GetTenantID() != aggregate.GetTenantID() {
			return fmt.Errorf("aggregate %s already tracked with different tenant", aggregateID)
		}
	}
	
	// Store the original version for optimistic concurrency control
	uow.aggregateVersions[aggregateID] = aggregate.GetVersion()
	uow.trackedAggregates[aggregateID] = aggregate
	
	// Register with concurrency manager
	if uow.concurrencyManager != nil {
		uow.concurrencyManager.RegisterAggregateVersion(aggregateID, aggregate.GetVersion())
	}
	
	log.Printf("Registered aggregate %s (type: %T, version: %d) for tracking", 
		aggregateID, aggregate, aggregate.GetVersion().Int())
	
	return nil
}

// GetTrackedAggregates returns all currently tracked aggregates
func (uow *SqlUnitOfWork) GetTrackedAggregates() []shared.AggregateRoot {
	uow.mu.RLock()
	defer uow.mu.RUnlock()
	
	aggregates := make([]shared.AggregateRoot, 0, len(uow.trackedAggregates))
	for _, aggregate := range uow.trackedAggregates {
		aggregates = append(aggregates, aggregate)
	}
	
	return aggregates
}

// CollectUncommittedEvents collects all uncommitted events from tracked aggregates
func (uow *SqlUnitOfWork) CollectUncommittedEvents() []shared.DomainEvent {
	uow.mu.RLock()
	defer uow.mu.RUnlock()
	
	var allEvents []shared.DomainEvent
	
	for _, aggregate := range uow.trackedAggregates {
		events := aggregate.GetUncommittedEvents()
		allEvents = append(allEvents, events...)
	}
	
	return allEvents
}

// ValidateAggregateConsistency validates consistency across tracked aggregates
func (uow *SqlUnitOfWork) ValidateAggregateConsistency() error {
	uow.mu.RLock()
	defer uow.mu.RUnlock()
	
	if uow.validator == nil {
		return nil // No validator configured
	}
	
	// Validate each tracked aggregate
	for _, aggregate := range uow.trackedAggregates {
		if err := uow.validator.ValidateAggregate(aggregate); err != nil {
			return fmt.Errorf("aggregate validation failed for %s: %w", 
				aggregate.GetID().String(), err)
		}
	}
	
	// Validate cross-aggregate consistency
	if err := uow.validator.ValidateCrossAggregateConsistency(uow.trackedAggregates); err != nil {
		return fmt.Errorf("cross-aggregate consistency validation failed: %w", err)
	}
	
	return nil
}

// ValidateBusinessRules validates business rules across the unit of work
func (uow *SqlUnitOfWork) ValidateBusinessRules() error {
	uow.mu.RLock()
	defer uow.mu.RUnlock()
	
	if uow.validator == nil {
		return nil // No validator configured
	}
	
	// Validate business rules across all tracked aggregates
	return uow.validator.ValidateBusinessRules(uow.trackedAggregates)
}

// collectAllUncommittedEvents collects events from all tracked aggregates and adds them to domain events
func (uow *SqlUnitOfWork) collectAllUncommittedEvents() error {
	aggregates := make([]shared.AggregateRoot, 0, len(uow.trackedAggregates))
	for _, aggregate := range uow.trackedAggregates {
		aggregates = append(aggregates, aggregate)
	}
	
	// Use the event collector to process events
	if uow.eventCollector != nil {
		processedEvents, err := uow.eventCollector.CollectEventsFromAggregates(aggregates)
		if err != nil {
			return fmt.Errorf("event collector failed: %w", err)
		}
		
		if len(processedEvents) > 0 {
			// Add to the unit of work's domain events
			uow.domainEvents = append(uow.domainEvents, processedEvents...)
			log.Printf("Collected %d processed events from %d tracked aggregates", 
				len(processedEvents), len(uow.trackedAggregates))
		}
	} else {
		// Fallback to basic collection
		collectedEvents := uow.CollectUncommittedEvents()
		
		if len(collectedEvents) > 0 {
			// Add to the unit of work's domain events
			uow.domainEvents = append(uow.domainEvents, collectedEvents...)
			log.Printf("Collected %d uncommitted events from %d tracked aggregates", 
				len(collectedEvents), len(uow.trackedAggregates))
		}
	}
	
	return nil
}

// clearAggregateEvents clears uncommitted events from all tracked aggregates
func (uow *SqlUnitOfWork) clearAggregateEvents() {
	for _, aggregate := range uow.trackedAggregates {
		aggregate.ClearUncommittedEvents()
	}
}

// validateOptimisticConcurrency validates concurrency control for all tracked aggregates
func (uow *SqlUnitOfWork) validateOptimisticConcurrency() error {
	if uow.concurrencyManager == nil {
		return nil // No concurrency manager configured
	}
	
	for _, aggregate := range uow.trackedAggregates {
		result, err := uow.concurrencyManager.CheckOptimisticConcurrency(uow.tx, aggregate)
		if err != nil {
			return fmt.Errorf("concurrency check failed for aggregate %s: %w", 
				aggregate.GetID().String(), err)
		}
		
		if !result.IsValid {
			if result.Error != nil {
				return result.Error
			}
			
			return shared.NewDomainError(
				shared.ErrCodeConflict,
				"optimistic concurrency conflict detected",
				map[string]interface{}{
					"aggregate_id":     aggregate.GetID().String(),
					"expected_version": result.ExpectedVersion.Int(),
					"actual_version":   result.ActualVersion.Int(),
					"resolve_action":   string(result.ResolveAction),
				},
			)
		}
	}
	
	return nil
}

// updateVersionsAfterSave updates version tracking after successful save
func (uow *SqlUnitOfWork) updateVersionsAfterSave() {
	if uow.concurrencyManager == nil {
		return
	}
	
	for _, aggregate := range uow.trackedAggregates {
		aggregateID := aggregate.GetID().String()
		newVersion := aggregate.GetVersion()
		uow.concurrencyManager.UpdateVersionAfterSave(aggregateID, newVersion)
	}
}

// processDomainEvents processes domain events within the transaction
func (uow *SqlUnitOfWork) processDomainEvents() error {
	if len(uow.domainEvents) == 0 {
		return nil
	}
	
	// Store events in event store if enabled
	if uow.config.EnableEventStore && uow.eventStore != nil {
		for _, event := range uow.domainEvents {
			// Group events by aggregate for batch saving
			if err := uow.saveEventToStore(event); err != nil {
				return fmt.Errorf("failed to save event to store: %w", err)
			}
		}
	}
	
	// Add events to outbox if enabled
	if uow.config.EnableOutbox && uow.outbox != nil {
		if err := uow.outbox.AddEvents(uow.tx, uow.domainEvents); err != nil {
			return fmt.Errorf("failed to add events to outbox: %w", err)
		}
	}
	
	return nil
}

// saveEventToStore saves an event to the event store
func (uow *SqlUnitOfWork) saveEventToStore(event shared.DomainEvent) error {
	// This would typically batch events by aggregate
	// For simplicity, we'll save individually
	return uow.eventStore.SaveEvents(
		event.AggregateID(),
		[]shared.DomainEvent{event},
		event.Version()-1, // Expected version is current version - 1
	)
}

// initializeRepositories creates repository instances with the current transaction
func (uow *SqlUnitOfWork) initializeRepositories() {
	// In a real implementation, these would be actual repository implementations
	// that use the transaction (uow.tx) for database operations
	uow.userRepo = NewTransactionalUserRepository(uow.tx, uow.eventStore)
	uow.workspaceRepo = NewTransactionalWorkspaceRepository(uow.tx, uow.eventStore)
	uow.tenantRepo = NewTransactionalTenantRepository(uow.tx, uow.eventStore)
}

// WithTransaction executes a function within a Unit of Work transaction
func (uow *SqlUnitOfWork) WithTransaction(fn func(UnitOfWork) error) error {
	// Begin transaction
	if err := uow.Begin(); err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	// Ensure rollback on panic or error
	defer func() {
		if r := recover(); r != nil {
			uow.Rollback()
			panic(r)
		}
	}()
	
	// Execute function
	if err := fn(uow); err != nil {
		if rollbackErr := uow.Rollback(); rollbackErr != nil {
			return fmt.Errorf("operation failed: %w, rollback failed: %v", err, rollbackErr)
		}
		return err
	}
	
	// Commit transaction
	if err := uow.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	return nil
}

// RetryableUnitOfWork wraps a Unit of Work with retry logic
type RetryableUnitOfWork struct {
	uow        UnitOfWork
	maxRetries int
	backoff    time.Duration
}

// NewRetryableUnitOfWork creates a new retryable Unit of Work
func NewRetryableUnitOfWork(uow UnitOfWork, maxRetries int, backoff time.Duration) *RetryableUnitOfWork {
	return &RetryableUnitOfWork{
		uow:        uow,
		maxRetries: maxRetries,
		backoff:    backoff,
	}
}

// WithRetryableTransaction executes a function with retry logic
func (ruow *RetryableUnitOfWork) WithRetryableTransaction(fn func(UnitOfWork) error) error {
	var lastErr error
	
	for attempt := 0; attempt < ruow.maxRetries; attempt++ {
		err := ruow.executeWithRetry(fn)
		if err == nil {
			return nil // Success
		}
		
		lastErr = err
		
		// Check if error is retryable
		if !isRetryableError(err) {
			return err
		}
		
		if attempt < ruow.maxRetries-1 {
			log.Printf("Transaction attempt %d failed: %v, retrying in %v", 
				attempt+1, err, ruow.backoff)
			time.Sleep(ruow.backoff * time.Duration(attempt+1))
		}
	}
	
	return fmt.Errorf("transaction failed after %d attempts: %w", ruow.maxRetries, lastErr)
}

// executeWithRetry executes the function once with proper error handling
func (ruow *RetryableUnitOfWork) executeWithRetry(fn func(UnitOfWork) error) error {
	if err := ruow.uow.Begin(); err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	defer func() {
		if r := recover(); r != nil {
			ruow.uow.Rollback()
			panic(r)
		}
	}()
	
	if err := fn(ruow.uow); err != nil {
		if rollbackErr := ruow.uow.Rollback(); rollbackErr != nil {
			return fmt.Errorf("operation failed: %w, rollback failed: %v", err, rollbackErr)
		}
		return err
	}
	
	if err := ruow.uow.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	return nil
}

// isRetryableError determines if an error is retryable
func isRetryableError(err error) bool {
	// Check for common retryable database errors
	errStr := err.Error()
	
	retryablePatterns := []string{
		"connection reset",
		"connection refused",
		"timeout",
		"deadlock",
		"serialization failure",
		"could not serialize access",
	}
	
	for _, pattern := range retryablePatterns {
		if contains(errStr, pattern) {
			return true
		}
	}
	
	return false
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || 
		(len(s) > len(substr) && (s[:len(substr)] == substr || 
		s[len(s)-len(substr):] == substr || 
		containsAt(s, substr))))
}

// containsAt checks if substr is contained within s
func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// UnitOfWorkFactory creates Unit of Work instances
type UnitOfWorkFactory struct {
	db     *sql.DB
	config UnitOfWorkConfig
}

// NewUnitOfWorkFactory creates a new Unit of Work factory
func NewUnitOfWorkFactory(db *sql.DB, config UnitOfWorkConfig) *UnitOfWorkFactory {
	return &UnitOfWorkFactory{
		db:     db,
		config: config,
	}
}

// Create creates a new Unit of Work instance
func (factory *UnitOfWorkFactory) Create() UnitOfWork {
	return NewSqlUnitOfWork(factory.db, factory.config)
}

// CreateWithContext creates a new Unit of Work with context
func (factory *UnitOfWorkFactory) CreateWithContext(ctx context.Context) UnitOfWork {
	uow := NewSqlUnitOfWork(factory.db, factory.config)
	uow.SetContext(ctx)
	return uow
}

// CreateRetryable creates a new retryable Unit of Work
func (factory *UnitOfWorkFactory) CreateRetryable(maxRetries int, backoff time.Duration) *RetryableUnitOfWork {
	uow := factory.Create()
	return NewRetryableUnitOfWork(uow, maxRetries, backoff)
}

// AggregateValidator provides validation for aggregates within a Unit of Work
type AggregateValidator struct {
	validators map[string]AggregateTypeValidator
	rules      []BusinessRule
}

// AggregateTypeValidator validates specific aggregate types
type AggregateTypeValidator interface {
	ValidateAggregate(aggregate shared.AggregateRoot) error
	GetAggregateType() string
}

// BusinessRule defines a business rule that can be validated across aggregates
type BusinessRule interface {
	ValidateRule(aggregates map[string]shared.AggregateRoot) error
	GetRuleName() string
	GetDescription() string
}

// NewAggregateValidator creates a new aggregate validator
func NewAggregateValidator() *AggregateValidator {
	return &AggregateValidator{
		validators: make(map[string]AggregateTypeValidator),
		rules:      make([]BusinessRule, 0),
	}
}

// RegisterValidator registers a validator for a specific aggregate type
func (v *AggregateValidator) RegisterValidator(validator AggregateTypeValidator) {
	v.validators[validator.GetAggregateType()] = validator
}

// RegisterBusinessRule registers a business rule for validation
func (v *AggregateValidator) RegisterBusinessRule(rule BusinessRule) {
	v.rules = append(v.rules, rule)
}

// ValidateAggregate validates a single aggregate using registered validators
func (v *AggregateValidator) ValidateAggregate(aggregate shared.AggregateRoot) error {
	aggregateType := getAggregateTypeName(aggregate)
	
	if validator, exists := v.validators[aggregateType]; exists {
		if err := validator.ValidateAggregate(aggregate); err != nil {
			return fmt.Errorf("validation failed for %s: %w", aggregateType, err)
		}
	}
	
	// Default validations that apply to all aggregates
	if err := v.validateDefaultRules(aggregate); err != nil {
		return fmt.Errorf("default validation failed: %w", err)
	}
	
	return nil
}

// ValidateCrossAggregateConsistency validates consistency across multiple aggregates
func (v *AggregateValidator) ValidateCrossAggregateConsistency(aggregates map[string]shared.AggregateRoot) error {
	// Check for duplicate aggregate IDs across different types
	seenIDs := make(map[string]string)
	
	for id, aggregate := range aggregates {
		aggregateType := getAggregateTypeName(aggregate)
		
		if existingType, exists := seenIDs[id]; exists && existingType != aggregateType {
			return shared.NewDomainError(
				shared.ErrCodeConflict,
				"duplicate aggregate ID across different types",
				map[string]interface{}{
					"aggregate_id":   id,
					"existing_type":  existingType,
					"conflicting_type": aggregateType,
				},
			)
		}
		
		seenIDs[id] = aggregateType
	}
	
	// Validate tenant consistency
	if err := v.validateTenantConsistency(aggregates); err != nil {
		return fmt.Errorf("tenant consistency validation failed: %w", err)
	}
	
	return nil
}

// ValidateBusinessRules validates business rules across all tracked aggregates
func (v *AggregateValidator) ValidateBusinessRules(aggregates map[string]shared.AggregateRoot) error {
	for _, rule := range v.rules {
		if err := rule.ValidateRule(aggregates); err != nil {
			return fmt.Errorf("business rule '%s' failed: %w", rule.GetRuleName(), err)
		}
	}
	
	return nil
}

// validateDefaultRules validates default rules that apply to all aggregates
func (v *AggregateValidator) validateDefaultRules(aggregate shared.AggregateRoot) error {
	// Validate aggregate ID is not empty
	if aggregate.GetID().IsEmpty() {
		return shared.NewDomainError(
			shared.ErrCodeValidation,
			"aggregate ID cannot be empty",
			nil,
		)
	}
	
	// Validate tenant ID is not empty
	if aggregate.GetTenantID().IsEmpty() {
		return shared.NewDomainError(
			shared.ErrCodeValidation,
			"tenant ID cannot be empty",
			nil,
		)
	}
	
	// Validate version is non-negative
	if aggregate.GetVersion().Int() < 0 {
		return shared.NewDomainError(
			shared.ErrCodeValidation,
			"aggregate version cannot be negative",
			map[string]interface{}{
				"version": aggregate.GetVersion().Int(),
			},
		)
	}
	
	return nil
}

// validateTenantConsistency ensures all aggregates belong to consistent tenants
func (v *AggregateValidator) validateTenantConsistency(aggregates map[string]shared.AggregateRoot) error {
	var primaryTenantID shared.TenantID
	tenantSet := false
	
	for _, aggregate := range aggregates {
		tenantID := aggregate.GetTenantID()
		
		if !tenantSet {
			primaryTenantID = tenantID
			tenantSet = true
		} else if primaryTenantID.String() != tenantID.String() {
			return shared.NewDomainError(
				shared.ErrCodeConflict,
				"aggregates must belong to the same tenant within a transaction",
				map[string]interface{}{
					"primary_tenant_id": primaryTenantID.String(),
					"conflicting_tenant_id": tenantID.String(),
					"aggregate_id": aggregate.GetID().String(),
				},
			)
		}
	}
	
	return nil
}

// getAggregateTypeName extracts the type name from an aggregate
func getAggregateTypeName(aggregate shared.AggregateRoot) string {
	switch aggregate.(type) {
	case *user.UserAggregate:
		return "UserAggregate"
	case *workspace.WorkspaceAggregate:
		return "WorkspaceAggregate"
	case *tenant.TenantAggregate:
		return "TenantAggregate"
	default:
		return fmt.Sprintf("%T", aggregate)
	}
}

// UserAggregateValidator validates user aggregates
type UserAggregateValidator struct{}

// ValidateAggregate validates a user aggregate
func (v *UserAggregateValidator) ValidateAggregate(aggregate shared.AggregateRoot) error {
	userAgg, ok := aggregate.(*user.UserAggregate)
	if !ok {
		return fmt.Errorf("expected UserAggregate, got %T", aggregate)
	}
	
	// Validate email format
	if userAgg.GetEmail().IsEmpty() {
		return shared.NewDomainError(
			shared.ErrCodeValidation,
			"user email cannot be empty",
			nil,
		)
	}
	
	// Validate user status
	status := userAgg.GetStatus()
	if !isValidUserStatus(status) {
		return shared.NewDomainError(
			shared.ErrCodeValidation,
			"invalid user status",
			map[string]interface{}{
				"status": string(status),
			},
		)
	}
	
	return nil
}

// GetAggregateType returns the aggregate type name
func (v *UserAggregateValidator) GetAggregateType() string {
	return "UserAggregate"
}

// isValidUserStatus checks if a user status is valid
func isValidUserStatus(status user.UserStatus) bool {
	return status.IsValid()
}

// WorkspaceAggregateValidator validates workspace aggregates
type WorkspaceAggregateValidator struct{}

// ValidateAggregate validates a workspace aggregate
func (v *WorkspaceAggregateValidator) ValidateAggregate(aggregate shared.AggregateRoot) error {
	workspaceAgg, ok := aggregate.(*workspace.WorkspaceAggregate)
	if !ok {
		return fmt.Errorf("expected WorkspaceAggregate, got %T", aggregate)
	}
	
	// Validate workspace name
	if workspaceAgg.GetName() == "" {
		return shared.NewDomainError(
			shared.ErrCodeValidation,
			"workspace name cannot be empty",
			nil,
		)
	}
	
	// Validate owner ID
	if workspaceAgg.GetOwnerID().IsEmpty() {
		return shared.NewDomainError(
			shared.ErrCodeValidation,
			"workspace owner ID cannot be empty",
			nil,
		)
	}
	
	// Validate workspace status
	status := workspaceAgg.GetStatus()
	if !isValidWorkspaceStatus(status) {
		return shared.NewDomainError(
			shared.ErrCodeValidation,
			"invalid workspace status",
			map[string]interface{}{
				"status": string(status),
			},
		)
	}
	
	return nil
}

// GetAggregateType returns the aggregate type name
func (v *WorkspaceAggregateValidator) GetAggregateType() string {
	return "WorkspaceAggregate"
}

// isValidWorkspaceStatus checks if a workspace status is valid
func isValidWorkspaceStatus(status workspace.WorkspaceStatus) bool {
	return status.IsValid()
}

// UniqueEmailBusinessRule ensures email uniqueness across user aggregates
type UniqueEmailBusinessRule struct{}

// ValidateRule validates the unique email business rule
func (r *UniqueEmailBusinessRule) ValidateRule(aggregates map[string]shared.AggregateRoot) error {
	emailSet := make(map[string]string) // email -> aggregate_id
	
	for id, aggregate := range aggregates {
		if userAgg, ok := aggregate.(*user.UserAggregate); ok {
			email := userAgg.GetEmail().String()
			
			if existingID, exists := emailSet[email]; exists && existingID != id {
				return shared.NewDomainError(
					shared.ErrCodeConflict,
					"email already exists for different user",
					map[string]interface{}{
						"email":         email,
						"user_id":       id,
						"existing_id":   existingID,
					},
				)
			}
			
			emailSet[email] = id
		}
	}
	
	return nil
}

// GetRuleName returns the rule name
func (r *UniqueEmailBusinessRule) GetRuleName() string {
	return "UniqueEmailRule"
}

// GetDescription returns the rule description
func (r *UniqueEmailBusinessRule) GetDescription() string {
	return "Ensures email addresses are unique across all user aggregates in the transaction"
}