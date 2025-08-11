package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"github.com/pyairtable-compose/go-services/domain/user"
	"github.com/pyairtable-compose/go-services/domain/workspace"
	"github.com/pyairtable-compose/go-services/domain/tenant"
)

// ServiceUnitOfWork provides a high-level interface for service layer operations
type ServiceUnitOfWork interface {
	// High-level operations
	ExecuteInTransaction(ctx context.Context, operation func(ServiceContext) error) error
	ExecuteWithRetry(ctx context.Context, operation func(ServiceContext) error, maxRetries int) error
	
	// Aggregate operations
	SaveUser(user *user.UserAggregate) error
	SaveWorkspace(workspace *workspace.WorkspaceAggregate) error
	SaveTenant(tenant *tenant.TenantAggregate) error
	
	GetUser(userID shared.UserID) (*user.UserAggregate, error)
	GetWorkspace(workspaceID shared.WorkspaceID) (*workspace.WorkspaceAggregate, error)
	GetTenant(tenantID shared.TenantID) (*tenant.TenantAggregate, error)
	
	// Batch operations
	SaveAggregates(aggregates ...shared.AggregateRoot) error
	LoadAggregates(ids ...shared.ID) ([]shared.AggregateRoot, error)
	
	// Event operations
	PublishEvents(events ...shared.DomainEvent) error
	CollectAndPublishEvents() error
	
	// Metrics and monitoring
	GetMetrics() ServiceMetrics
	LogMetrics()
}

// ServiceContext provides access to repositories and services within a transaction
type ServiceContext interface {
	// Repository access
	Users() UserRepository
	Workspaces() WorkspaceRepository
	Tenants() TenantRepository
	
	// Unit of Work access
	UnitOfWork() UnitOfWork
	
	// Context management
	Context() context.Context
	SetDeadline(deadline time.Time)
	SetTimeout(timeout time.Duration)
	
	// Aggregate registration
	Track(aggregate shared.AggregateRoot) error
	
	// Validation
	ValidateAll() error
	
	// Event management
	AddEvent(event shared.DomainEvent) error
	GetEvents() []shared.DomainEvent
}

// ServiceMetrics tracks service layer metrics
type ServiceMetrics struct {
	TotalTransactions       int64
	SuccessfulTransactions  int64
	FailedTransactions      int64
	RetryTransactions       int64
	AverageTransactionTime  time.Duration
	TotalAggregatesSaved    int64
	TotalEventsPublished    int64
	ConcurrencyConflicts    int64
	ValidationFailures      int64
}

// sqlServiceUnitOfWork implements ServiceUnitOfWork using SQL transactions
type sqlServiceUnitOfWork struct {
	uowFactory *UnitOfWorkFactory
	metrics    *ServiceMetrics
}

// NewServiceUnitOfWork creates a new service-layer Unit of Work
func NewServiceUnitOfWork(db *sql.DB, config UnitOfWorkConfig) ServiceUnitOfWork {
	factory := NewUnitOfWorkFactory(db, config)
	
	return &sqlServiceUnitOfWork{
		uowFactory: factory,
		metrics:    &ServiceMetrics{},
	}
}

// ExecuteInTransaction executes an operation within a transaction
func (suow *sqlServiceUnitOfWork) ExecuteInTransaction(ctx context.Context, operation func(ServiceContext) error) error {
	startTime := time.Now()
	
	defer func() {
		suow.metrics.TotalTransactions++
		duration := time.Since(startTime)
		suow.metrics.AverageTransactionTime = 
			(suow.metrics.AverageTransactionTime + duration) / 2
	}()
	
	// Create Unit of Work with context
	uow := suow.uowFactory.CreateWithContext(ctx)
	
	// Begin transaction
	if err := uow.Begin(); err != nil {
		suow.metrics.FailedTransactions++
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	// Ensure rollback on panic or error
	defer func() {
		if r := recover(); r != nil {
			uow.Rollback()
			suow.metrics.FailedTransactions++
			panic(r)
		}
	}()
	
	// Create service context
	serviceCtx := &sqlServiceContext{
		uow:     uow,
		ctx:     ctx,
		metrics: suow.metrics,
	}
	
	// Execute operation
	if err := operation(serviceCtx); err != nil {
		if rollbackErr := uow.Rollback(); rollbackErr != nil {
			suow.metrics.FailedTransactions++
			return fmt.Errorf("operation failed: %w, rollback failed: %v", err, rollbackErr)
		}
		suow.metrics.FailedTransactions++
		return err
	}
	
	// Commit transaction
	if err := uow.Commit(); err != nil {
		suow.metrics.FailedTransactions++
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	suow.metrics.SuccessfulTransactions++
	return nil
}

// ExecuteWithRetry executes an operation with retry logic
func (suow *sqlServiceUnitOfWork) ExecuteWithRetry(ctx context.Context, operation func(ServiceContext) error, maxRetries int) error {
	var lastErr error
	
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := suow.ExecuteInTransaction(ctx, operation)
		if err == nil {
			return nil // Success
		}
		
		lastErr = err
		suow.metrics.RetryTransactions++
		
		// Check if error is retryable
		if !isRetryableError(err) {
			return err
		}
		
		if attempt < maxRetries-1 {
			// Wait before retry with exponential backoff
			waitTime := time.Duration(1<<uint(attempt)) * time.Second
			log.Printf("Service operation attempt %d failed: %v, retrying in %v", 
				attempt+1, err, waitTime)
			
			select {
			case <-time.After(waitTime):
				// Continue with retry
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	
	return fmt.Errorf("service operation failed after %d attempts: %w", maxRetries, lastErr)
}

// SaveUser saves a user aggregate
func (suow *sqlServiceUnitOfWork) SaveUser(userAgg *user.UserAggregate) error {
	return suow.ExecuteInTransaction(context.Background(), func(ctx ServiceContext) error {
		if err := ctx.Track(userAgg); err != nil {
			return fmt.Errorf("failed to track user aggregate: %w", err)
		}
		
		return ctx.Users().Save(userAgg)
	})
}

// SaveWorkspace saves a workspace aggregate
func (suow *sqlServiceUnitOfWork) SaveWorkspace(workspaceAgg *workspace.WorkspaceAggregate) error {
	return suow.ExecuteInTransaction(context.Background(), func(ctx ServiceContext) error {
		if err := ctx.Track(workspaceAgg); err != nil {
			return fmt.Errorf("failed to track workspace aggregate: %w", err)
		}
		
		return ctx.Workspaces().Save(workspaceAgg)
	})
}

// SaveTenant saves a tenant aggregate
func (suow *sqlServiceUnitOfWork) SaveTenant(tenantAgg *tenant.TenantAggregate) error {
	return suow.ExecuteInTransaction(context.Background(), func(ctx ServiceContext) error {
		if err := ctx.Track(tenantAgg); err != nil {
			return fmt.Errorf("failed to track tenant aggregate: %w", err)
		}
		
		return ctx.Tenants().Save(tenantAgg)
	})
}

// GetUser retrieves a user aggregate
func (suow *sqlServiceUnitOfWork) GetUser(userID shared.UserID) (*user.UserAggregate, error) {
	var userAgg *user.UserAggregate
	
	err := suow.ExecuteInTransaction(context.Background(), func(ctx ServiceContext) error {
		aggregate, err := ctx.Users().GetByID(userID.ID)
		if err != nil {
			return err
		}
		
		var ok bool
		userAgg, ok = aggregate.(*user.UserAggregate)
		if !ok {
			return fmt.Errorf("expected UserAggregate, got %T", aggregate)
		}
		
		return ctx.Track(userAgg)
	})
	
	return userAgg, err
}

// GetWorkspace retrieves a workspace aggregate
func (suow *sqlServiceUnitOfWork) GetWorkspace(workspaceID shared.WorkspaceID) (*workspace.WorkspaceAggregate, error) {
	var workspaceAgg *workspace.WorkspaceAggregate
	
	err := suow.ExecuteInTransaction(context.Background(), func(ctx ServiceContext) error {
		aggregate, err := ctx.Workspaces().GetByID(workspaceID.ID)
		if err != nil {
			return err
		}
		
		var ok bool
		workspaceAgg, ok = aggregate.(*workspace.WorkspaceAggregate)
		if !ok {
			return fmt.Errorf("expected WorkspaceAggregate, got %T", aggregate)
		}
		
		return ctx.Track(workspaceAgg)
	})
	
	return workspaceAgg, err
}

// GetTenant retrieves a tenant aggregate
func (suow *sqlServiceUnitOfWork) GetTenant(tenantID shared.TenantID) (*tenant.TenantAggregate, error) {
	var tenantAgg *tenant.TenantAggregate
	
	err := suow.ExecuteInTransaction(context.Background(), func(ctx ServiceContext) error {
		aggregate, err := ctx.Tenants().GetByID(tenantID.ID)
		if err != nil {
			return err
		}
		
		var ok bool
		tenantAgg, ok = aggregate.(*tenant.TenantAggregate)
		if !ok {
			return fmt.Errorf("expected TenantAggregate, got %T", aggregate)
		}
		
		return ctx.Track(tenantAgg)
	})
	
	return tenantAgg, err
}

// SaveAggregates saves multiple aggregates in a single transaction
func (suow *sqlServiceUnitOfWork) SaveAggregates(aggregates ...shared.AggregateRoot) error {
	return suow.ExecuteInTransaction(context.Background(), func(ctx ServiceContext) error {
		for _, aggregate := range aggregates {
			if err := ctx.Track(aggregate); err != nil {
				return fmt.Errorf("failed to track aggregate %s: %w", 
					aggregate.GetID().String(), err)
			}
			
			// Save based on type
			switch agg := aggregate.(type) {
			case *user.UserAggregate:
				if err := ctx.Users().Save(agg); err != nil {
					return fmt.Errorf("failed to save user aggregate: %w", err)
				}
			case *workspace.WorkspaceAggregate:
				if err := ctx.Workspaces().Save(agg); err != nil {
					return fmt.Errorf("failed to save workspace aggregate: %w", err)
				}
			case *tenant.TenantAggregate:
				if err := ctx.Tenants().Save(agg); err != nil {
					return fmt.Errorf("failed to save tenant aggregate: %w", err)
				}
			default:
				return fmt.Errorf("unsupported aggregate type: %T", aggregate)
			}
			
			suow.metrics.TotalAggregatesSaved++
		}
		
		return nil
	})
}

// LoadAggregates loads multiple aggregates by ID
func (suow *sqlServiceUnitOfWork) LoadAggregates(ids ...shared.ID) ([]shared.AggregateRoot, error) {
	var aggregates []shared.AggregateRoot
	
	err := suow.ExecuteInTransaction(context.Background(), func(ctx ServiceContext) error {
		for _, id := range ids {
			// Try each repository type (this is simplified - in practice you'd have metadata about types)
			var aggregate shared.AggregateRoot
			var err error
			
			// Try user repository first
			aggregate, err = ctx.Users().GetByID(id)
			if err == nil {
				aggregates = append(aggregates, aggregate)
				ctx.Track(aggregate)
				continue
			}
			
			// Try workspace repository
			aggregate, err = ctx.Workspaces().GetByID(id)
			if err == nil {
				aggregates = append(aggregates, aggregate)
				ctx.Track(aggregate)
				continue
			}
			
			// Try tenant repository
			aggregate, err = ctx.Tenants().GetByID(id)
			if err == nil {
				aggregates = append(aggregates, aggregate)
				ctx.Track(aggregate)
				continue
			}
			
			// Not found in any repository
			return fmt.Errorf("aggregate with ID %s not found", id.String())
		}
		
		return nil
	})
	
	return aggregates, err
}

// PublishEvents publishes domain events
func (suow *sqlServiceUnitOfWork) PublishEvents(events ...shared.DomainEvent) error {
	return suow.ExecuteInTransaction(context.Background(), func(ctx ServiceContext) error {
		for _, event := range events {
			if err := ctx.AddEvent(event); err != nil {
				return fmt.Errorf("failed to add event: %w", err)
			}
		}
		
		suow.metrics.TotalEventsPublished += int64(len(events))
		return nil
	})
}

// CollectAndPublishEvents collects events from tracked aggregates and publishes them
func (suow *sqlServiceUnitOfWork) CollectAndPublishEvents() error {
	return suow.ExecuteInTransaction(context.Background(), func(ctx ServiceContext) error {
		events := ctx.GetEvents()
		suow.metrics.TotalEventsPublished += int64(len(events))
		return nil // Events are automatically published during commit
	})
}

// GetMetrics returns current service metrics
func (suow *sqlServiceUnitOfWork) GetMetrics() ServiceMetrics {
	return *suow.metrics
}

// LogMetrics logs current service metrics
func (suow *sqlServiceUnitOfWork) LogMetrics() {
	metrics := suow.GetMetrics()
	
	log.Printf("Service Unit of Work Metrics:")
	log.Printf("  Total Transactions: %d", metrics.TotalTransactions)
	log.Printf("  Successful Transactions: %d", metrics.SuccessfulTransactions)
	log.Printf("  Failed Transactions: %d", metrics.FailedTransactions)
	log.Printf("  Retry Transactions: %d", metrics.RetryTransactions)
	log.Printf("  Average Transaction Time: %v", metrics.AverageTransactionTime)
	log.Printf("  Total Aggregates Saved: %d", metrics.TotalAggregatesSaved)
	log.Printf("  Total Events Published: %d", metrics.TotalEventsPublished)
	log.Printf("  Concurrency Conflicts: %d", metrics.ConcurrencyConflicts)
	log.Printf("  Validation Failures: %d", metrics.ValidationFailures)
	
	if metrics.TotalTransactions > 0 {
		successRate := float64(metrics.SuccessfulTransactions) / float64(metrics.TotalTransactions) * 100
		log.Printf("  Success Rate: %.2f%%", successRate)
	}
}

// sqlServiceContext implements ServiceContext
type sqlServiceContext struct {
	uow     UnitOfWork
	ctx     context.Context
	metrics *ServiceMetrics
}

// Users returns the user repository
func (sc *sqlServiceContext) Users() UserRepository {
	return sc.uow.UserRepository()
}

// Workspaces returns the workspace repository
func (sc *sqlServiceContext) Workspaces() WorkspaceRepository {
	return sc.uow.WorkspaceRepository()
}

// Tenants returns the tenant repository
func (sc *sqlServiceContext) Tenants() TenantRepository {
	return sc.uow.TenantRepository()
}

// UnitOfWork returns the underlying unit of work
func (sc *sqlServiceContext) UnitOfWork() UnitOfWork {
	return sc.uow
}

// Context returns the current context
func (sc *sqlServiceContext) Context() context.Context {
	return sc.ctx
}

// SetDeadline sets a deadline on the context
func (sc *sqlServiceContext) SetDeadline(deadline time.Time) {
	ctx, cancel := context.WithDeadline(sc.ctx, deadline)
	defer cancel()
	sc.ctx = ctx
	sc.uow.SetContext(ctx)
}

// SetTimeout sets a timeout on the context
func (sc *sqlServiceContext) SetTimeout(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(sc.ctx, timeout)
	defer cancel()
	sc.ctx = ctx
	sc.uow.SetContext(ctx)
}

// Track registers an aggregate for tracking
func (sc *sqlServiceContext) Track(aggregate shared.AggregateRoot) error {
	return sc.uow.RegisterAggregate(aggregate)
}

// ValidateAll validates all tracked aggregates
func (sc *sqlServiceContext) ValidateAll() error {
	if err := sc.uow.ValidateAggregateConsistency(); err != nil {
		sc.metrics.ValidationFailures++
		return err
	}
	
	if err := sc.uow.ValidateBusinessRules(); err != nil {
		sc.metrics.ValidationFailures++
		return err
	}
	
	return nil
}

// AddEvent adds a domain event
func (sc *sqlServiceContext) AddEvent(event shared.DomainEvent) error {
	return sc.uow.AddDomainEvents([]shared.DomainEvent{event})
}

// GetEvents returns all domain events
func (sc *sqlServiceContext) GetEvents() []shared.DomainEvent {
	return sc.uow.GetDomainEvents()
}

// ServiceBuilder provides a fluent interface for building service operations
type ServiceBuilder struct {
	suow    ServiceUnitOfWork
	ctx     context.Context
	timeout time.Duration
	retries int
}

// NewServiceBuilder creates a new service builder
func NewServiceBuilder(suow ServiceUnitOfWork) *ServiceBuilder {
	return &ServiceBuilder{
		suow:    suow,
		ctx:     context.Background(),
		timeout: 30 * time.Second,
		retries: 1,
	}
}

// WithContext sets the context
func (sb *ServiceBuilder) WithContext(ctx context.Context) *ServiceBuilder {
	sb.ctx = ctx
	return sb
}

// WithTimeout sets the timeout
func (sb *ServiceBuilder) WithTimeout(timeout time.Duration) *ServiceBuilder {
	sb.timeout = timeout
	return sb
}

// WithRetries sets the number of retries
func (sb *ServiceBuilder) WithRetries(retries int) *ServiceBuilder {
	sb.retries = retries
	return sb
}

// Execute executes the operation with the configured options
func (sb *ServiceBuilder) Execute(operation func(ServiceContext) error) error {
	ctx, cancel := context.WithTimeout(sb.ctx, sb.timeout)
	defer cancel()
	
	if sb.retries > 1 {
		return sb.suow.ExecuteWithRetry(ctx, operation, sb.retries)
	}
	
	return sb.suow.ExecuteInTransaction(ctx, operation)
}

// Common service operations

// UserRegistrationService handles user registration with all related operations
type UserRegistrationService struct {
	suow ServiceUnitOfWork
}

// NewUserRegistrationService creates a new user registration service
func NewUserRegistrationService(suow ServiceUnitOfWork) *UserRegistrationService {
	return &UserRegistrationService{suow: suow}
}

// RegisterUser registers a new user with workspace creation
func (urs *UserRegistrationService) RegisterUser(
	userID shared.UserID,
	tenantID shared.TenantID,
	email shared.Email,
	hashedPassword string,
	firstName string,
	lastName string,
	createDefaultWorkspace bool,
) error {
	
	return urs.suow.ExecuteInTransaction(context.Background(), func(ctx ServiceContext) error {
		// Create user aggregate
		userAgg, err := user.NewUserAggregate(userID, tenantID, email, hashedPassword, firstName, lastName)
		if err != nil {
			return fmt.Errorf("failed to create user aggregate: %w", err)
		}
		
		// Track and save user
		if err := ctx.Track(userAgg); err != nil {
			return fmt.Errorf("failed to track user: %w", err)
		}
		
		if err := ctx.Users().Save(userAgg); err != nil {
			return fmt.Errorf("failed to save user: %w", err)
		}
		
		// Create default workspace if requested
		if createDefaultWorkspace {
			workspaceID := shared.NewWorkspaceID()
			workspaceAgg, err := workspace.NewWorkspaceAggregate(
				workspaceID,
				tenantID,
				fmt.Sprintf("%s's Workspace", firstName),
				"Default workspace",
				userID,
			)
			if err != nil {
				return fmt.Errorf("failed to create default workspace: %w", err)
			}
			
			if err := ctx.Track(workspaceAgg); err != nil {
				return fmt.Errorf("failed to track workspace: %w", err)
			}
			
			if err := ctx.Workspaces().Save(workspaceAgg); err != nil {
				return fmt.Errorf("failed to save default workspace: %w", err)
			}
		}
		
		return nil
	})
}

// BatchOperationService handles batch operations across multiple aggregates
type BatchOperationService struct {
	suow ServiceUnitOfWork
}

// NewBatchOperationService creates a new batch operation service
func NewBatchOperationService(suow ServiceUnitOfWork) *BatchOperationService {
	return &BatchOperationService{suow: suow}
}

// BulkUserUpdate updates multiple users in a single transaction
func (bos *BatchOperationService) BulkUserUpdate(updates map[shared.UserID]func(*user.UserAggregate) error) error {
	return bos.suow.ExecuteInTransaction(context.Background(), func(ctx ServiceContext) error {
		for userID, updateFunc := range updates {
			// Load user
			aggregate, err := ctx.Users().GetByID(userID.ID)
			if err != nil {
				return fmt.Errorf("failed to load user %s: %w", userID.String(), err)
			}
			
			userAgg, ok := aggregate.(*user.UserAggregate)
			if !ok {
				return fmt.Errorf("expected UserAggregate, got %T", aggregate)
			}
			
			// Track user
			if err := ctx.Track(userAgg); err != nil {
				return fmt.Errorf("failed to track user %s: %w", userID.String(), err)
			}
			
			// Apply update
			if err := updateFunc(userAgg); err != nil {
				return fmt.Errorf("update function failed for user %s: %w", userID.String(), err)
			}
			
			// Save user
			if err := ctx.Users().Save(userAgg); err != nil {
				return fmt.Errorf("failed to save user %s: %w", userID.String(), err)
			}
		}
		
		return nil
	})
}