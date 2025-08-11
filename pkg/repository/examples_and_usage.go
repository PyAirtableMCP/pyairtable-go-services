package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"github.com/pyairtable-compose/go-services/domain/user"
	"github.com/pyairtable-compose/go-services/domain/workspace"
	"github.com/pyairtable-compose/go-services/domain/tenant"
)

// Example 1: Basic Unit of Work Usage
func ExampleBasicUnitOfWorkUsage(db *sql.DB) error {
	log.Println("=== Example 1: Basic Unit of Work Usage ===")
	
	// Configure Unit of Work
	config := UnitOfWorkConfig{
		TransactionTimeout: 30 * time.Second,
		IsolationLevel:     sql.LevelReadCommitted,
		EnableOutbox:       true,
		EnableEventStore:   true,
		MaxRetries:         3,
	}
	
	// Create Unit of Work
	uow := NewSqlUnitOfWork(db, config)
	
	// Example operation
	return uow.WithTransaction(func(uow UnitOfWork) error {
		// Create a new user
		userID := shared.NewUserID()
		tenantID := shared.NewTenantID()
		email, _ := shared.NewEmail("john.doe@example.com")
		
		userAgg, err := user.NewUserAggregate(
			userID, tenantID, email, 
			"hashedpassword", "John", "Doe",
		)
		if err != nil {
			return fmt.Errorf("failed to create user aggregate: %w", err)
		}
		
		// Register aggregate for tracking
		if err := uow.RegisterAggregate(userAgg); err != nil {
			return fmt.Errorf("failed to register aggregate: %w", err)
		}
		
		// Save user
		if err := uow.UserRepository().Save(userAgg); err != nil {
			return fmt.Errorf("failed to save user: %w", err)
		}
		
		log.Printf("Created user: %s", userID.String())
		return nil
	})
}

// Example 2: Service Layer Integration
func ExampleServiceLayerIntegration(db *sql.DB) error {
	log.Println("=== Example 2: Service Layer Integration ===")
	
	config := UnitOfWorkConfig{
		TransactionTimeout: 30 * time.Second,
		IsolationLevel:     sql.LevelReadCommitted,
		EnableOutbox:       true,
		EnableEventStore:   true,
		MaxRetries:         3,
	}
	
	// Create service-level Unit of Work
	serviceUow := NewServiceUnitOfWork(db, config)
	
	// Register a user with default workspace
	userRegistrationService := NewUserRegistrationService(serviceUow)
	
	userID := shared.NewUserID()
	tenantID := shared.NewTenantID()
	email, _ := shared.NewEmail("jane.smith@example.com")
	
	err := userRegistrationService.RegisterUser(
		userID, tenantID, email,
		"hashedpassword", "Jane", "Smith",
		true, // Create default workspace
	)
	
	if err != nil {
		return fmt.Errorf("failed to register user: %w", err)
	}
	
	log.Printf("Successfully registered user %s with default workspace", userID.String())
	return nil
}

// Example 3: Optimistic Concurrency Control
func ExampleOptimisticConcurrencyControl(db *sql.DB) error {
	log.Println("=== Example 3: Optimistic Concurrency Control ===")
	
	config := UnitOfWorkConfig{
		TransactionTimeout: 30 * time.Second,
		IsolationLevel:     sql.LevelReadCommitted,
		EnableOutbox:       true,
		EnableEventStore:   true,
		MaxRetries:         3,
	}
	
	uow := NewSqlUnitOfWork(db, config)
	
	// Configure concurrency control
	concurrencyManager := uow.GetConcurrencyManager()
	
	// Setup a retry resolver for handling conflicts
	retryResolver := NewRetryOnConflictResolver(3)
	smartResolver := NewSmartConflictResolver(retryResolver)
	
	// Register user-specific strategy
	smartResolver.RegisterStrategy("*user.UserAggregate", retryResolver)
	
	return uow.WithTransaction(func(uow UnitOfWork) error {
		userID := shared.NewUserID()
		tenantID := shared.NewTenantID()
		email, _ := shared.NewEmail("concurrent.user@example.com")
		
		// Create user
		userAgg, err := user.NewUserAggregate(
			userID, tenantID, email,
			"hashedpassword", "Concurrent", "User",
		)
		if err != nil {
			return err
		}
		
		// Register and acquire optimistic lock
		if err := uow.RegisterAggregate(userAgg); err != nil {
			return err
		}
		
		// Acquire lock for 10 seconds
		if err := concurrencyManager.AcquireOptimisticLock(
			userID.String(), 
			userAgg.GetVersion(), 
			10*time.Second,
		); err != nil {
			return fmt.Errorf("failed to acquire lock: %w", err)
		}
		
		defer concurrencyManager.ReleaseOptimisticLock(userID.String())
		
		// Update user
		if err := userAgg.UpdateProfile("Updated", "Name", "", "", ""); err != nil {
			return err
		}
		
		// Save with concurrency checking
		if err := uow.UserRepository().Save(userAgg); err != nil {
			return err
		}
		
		log.Printf("Successfully updated user with concurrency control")
		return nil
	})
}

// Example 4: Event Collection and Processing
func ExampleEventCollectionAndProcessing(db *sql.DB) error {
	log.Println("=== Example 4: Event Collection and Processing ===")
	
	config := UnitOfWorkConfig{
		TransactionTimeout: 30 * time.Second,
		IsolationLevel:     sql.LevelReadCommitted,
		EnableOutbox:       true,
		EnableEventStore:   true,
		MaxRetries:         3,
	}
	
	uow := NewSqlUnitOfWork(db, config)
	
	// Configure event collector
	eventCollector := uow.GetEventCollector()
	
	// Add custom event handlers
	eventCollector.RegisterEventHandler(shared.EventTypeUserRegistered, &CustomUserEventHandler{})
	eventCollector.RegisterEventHandler(shared.EventTypeWorkspaceCreated, &CustomWorkspaceEventHandler{})
	
	// Add event filters
	tenantFilter := NewTenantIsolationFilter("tenant-123")
	eventCollector.RegisterEventFilter(tenantFilter)
	
	// Add event transformers
	enrichmentTransformer := NewMetadataEnrichmentTransformer(func(event shared.DomainEvent, aggregate shared.AggregateRoot) map[string]interface{} {
		return map[string]interface{}{
			"source":     "example_service",
			"processed_at": time.Now().UTC(),
			"aggregate_type": fmt.Sprintf("%T", aggregate),
		}
	})
	eventCollector.RegisterEventTransformer(enrichmentTransformer)
	
	return uow.WithTransaction(func(uow UnitOfWork) error {
		// Create multiple aggregates to generate events
		userID := shared.NewUserID()
		tenantID := shared.NewTenantIDFromString("tenant-123")
		email, _ := shared.NewEmail("event.user@example.com")
		
		// Create user
		userAgg, err := user.NewUserAggregate(
			userID, tenantID, email,
			"hashedpassword", "Event", "User",
		)
		if err != nil {
			return err
		}
		
		// Create workspace
		workspaceID := shared.NewWorkspaceID()
		workspaceAgg, err := workspace.NewWorkspaceAggregate(
			workspaceID, tenantID,
			"Event Workspace", "Test workspace for events",
			userID,
		)
		if err != nil {
			return err
		}
		
		// Register aggregates
		if err := uow.RegisterAggregate(userAgg); err != nil {
			return err
		}
		if err := uow.RegisterAggregate(workspaceAgg); err != nil {
			return err
		}
		
		// Save aggregates (this will trigger event collection)
		if err := uow.UserRepository().Save(userAgg); err != nil {
			return err
		}
		if err := uow.WorkspaceRepository().Save(workspaceAgg); err != nil {
			return err
		}
		
		// Events will be automatically collected and processed during commit
		log.Printf("Events will be collected and processed automatically")
		return nil
	})
}

// Example 5: Batch Operations
func ExampleBatchOperations(db *sql.DB) error {
	log.Println("=== Example 5: Batch Operations ===")
	
	config := UnitOfWorkConfig{
		TransactionTimeout: 60 * time.Second, // Longer timeout for batch operations
		IsolationLevel:     sql.LevelReadCommitted,
		EnableOutbox:       true,
		EnableEventStore:   true,
		MaxRetries:         3,
	}
	
	serviceUow := NewServiceUnitOfWork(db, config)
	batchService := NewBatchOperationService(serviceUow)
	
	// Create multiple users first
	users := make(map[shared.UserID]*user.UserAggregate)
	tenantID := shared.NewTenantID()
	
	for i := 0; i < 5; i++ {
		userID := shared.NewUserID()
		email, _ := shared.NewEmail(fmt.Sprintf("batch.user%d@example.com", i))
		
		userAgg, err := user.NewUserAggregate(
			userID, tenantID, email,
			"hashedpassword", fmt.Sprintf("Batch%d", i), "User",
		)
		if err != nil {
			return err
		}
		
		users[userID] = userAgg
	}
	
	// Save all users in batch
	var aggregates []shared.AggregateRoot
	for _, userAgg := range users {
		aggregates = append(aggregates, userAgg)
	}
	
	if err := serviceUow.SaveAggregates(aggregates...); err != nil {
		return fmt.Errorf("failed to save users in batch: %w", err)
	}
	
	// Now update all users in batch
	updates := make(map[shared.UserID]func(*user.UserAggregate) error)
	for userID := range users {
		updates[userID] = func(userAgg *user.UserAggregate) error {
			return userAgg.UpdateProfile("Updated", "", "", "", "")
		}
	}
	
	if err := batchService.BulkUserUpdate(updates); err != nil {
		return fmt.Errorf("failed to update users in batch: %w", err)
	}
	
	log.Printf("Successfully processed %d users in batch operations", len(users))
	return nil
}

// Example 6: Error Handling and Retries
func ExampleErrorHandlingAndRetries(db *sql.DB) error {
	log.Println("=== Example 6: Error Handling and Retries ===")
	
	config := UnitOfWorkConfig{
		TransactionTimeout: 30 * time.Second,
		IsolationLevel:     sql.LevelReadCommitted,
		EnableOutbox:       true,
		EnableEventStore:   true,
		MaxRetries:         3,
	}
	
	serviceUow := NewServiceUnitOfWork(db, config)
	
	// Use service builder for fluent configuration
	builder := NewServiceBuilder(serviceUow).
		WithTimeout(60 * time.Second).
		WithRetries(5).
		WithContext(context.Background())
	
	// Simulate an operation that might fail and need retries
	attemptCount := 0
	err := builder.Execute(func(ctx ServiceContext) error {
		attemptCount++
		log.Printf("Attempt %d", attemptCount)
		
		// Simulate failure on first two attempts
		if attemptCount < 3 {
			return fmt.Errorf("simulated temporary failure")
		}
		
		// Create user on third attempt
		userID := shared.NewUserID()
		tenantID := shared.NewTenantID()
		email, _ := shared.NewEmail("retry.user@example.com")
		
		userAgg, err := user.NewUserAggregate(
			userID, tenantID, email,
			"hashedpassword", "Retry", "User",
		)
		if err != nil {
			return err
		}
		
		if err := ctx.Track(userAgg); err != nil {
			return err
		}
		
		if err := ctx.Users().Save(userAgg); err != nil {
			return err
		}
		
		log.Printf("Successfully created user after %d attempts", attemptCount)
		return nil
	})
	
	if err != nil {
		return fmt.Errorf("operation failed even with retries: %w", err)
	}
	
	return nil
}

// Example 7: Complex Business Scenario
func ExampleComplexBusinessScenario(db *sql.DB) error {
	log.Println("=== Example 7: Complex Business Scenario ===")
	
	config := UnitOfWorkConfig{
		TransactionTimeout: 60 * time.Second,
		IsolationLevel:     sql.LevelReadCommitted,
		EnableOutbox:       true,
		EnableEventStore:   true,
		MaxRetries:         3,
	}
	
	serviceUow := NewServiceUnitOfWork(db, config)
	
	// Scenario: User invites another user to their workspace
	return serviceUow.ExecuteInTransaction(context.Background(), func(ctx ServiceContext) error {
		// Step 1: Load existing user and workspace
		ownerID := shared.NewUserID()
		tenantID := shared.NewTenantID()
		ownerEmail, _ := shared.NewEmail("owner@example.com")
		
		// Create owner user
		ownerAgg, err := user.NewUserAggregate(
			ownerID, tenantID, ownerEmail,
			"hashedpassword", "Owner", "User",
		)
		if err != nil {
			return err
		}
		
		if err := ctx.Track(ownerAgg); err != nil {
			return err
		}
		if err := ctx.Users().Save(ownerAgg); err != nil {
			return err
		}
		
		// Create workspace
		workspaceID := shared.NewWorkspaceID()
		workspaceAgg, err := workspace.NewWorkspaceAggregate(
			workspaceID, tenantID,
			"Collaboration Workspace", "Workspace for collaboration",
			ownerID,
		)
		if err != nil {
			return err
		}
		
		if err := ctx.Track(workspaceAgg); err != nil {
			return err
		}
		if err := ctx.Users().Save(workspaceAgg); err != nil {
			return err
		}
		
		// Step 2: Create invited user
		invitedID := shared.NewUserID()
		invitedEmail, _ := shared.NewEmail("invited@example.com")
		
		invitedAgg, err := user.NewUserAggregate(
			invitedID, tenantID, invitedEmail,
			"hashedpassword", "Invited", "User",
		)
		if err != nil {
			return err
		}
		
		if err := ctx.Track(invitedAgg); err != nil {
			return err
		}
		if err := ctx.Users().Save(invitedAgg); err != nil {
			return err
		}
		
		// Step 3: Add invited user to workspace
		if err := workspaceAgg.AddMember(invitedID, workspace.MemberRoleMember, ownerID); err != nil {
			return err
		}
		
		if err := ctx.Workspaces().Save(workspaceAgg); err != nil {
			return err
		}
		
		// Step 4: Validate all business rules
		if err := ctx.ValidateAll(); err != nil {
			return fmt.Errorf("business rule validation failed: %w", err)
		}
		
		log.Printf("Successfully completed user invitation scenario")
		return nil
	})
}

// Example 8: Monitoring and Metrics
func ExampleMonitoringAndMetrics(db *sql.DB) error {
	log.Println("=== Example 8: Monitoring and Metrics ===")
	
	config := UnitOfWorkConfig{
		TransactionTimeout: 30 * time.Second,
		IsolationLevel:     sql.LevelReadCommitted,
		EnableOutbox:       true,
		EnableEventStore:   true,
		MaxRetries:         3,
	}
	
	uow := NewSqlUnitOfWork(db, config)
	serviceUow := NewServiceUnitOfWork(db, config)
	
	// Perform several operations to generate metrics
	for i := 0; i < 10; i++ {
		userID := shared.NewUserID()
		tenantID := shared.NewTenantID()
		email, _ := shared.NewEmail(fmt.Sprintf("metrics.user%d@example.com", i))
		
		err := serviceUow.ExecuteInTransaction(context.Background(), func(ctx ServiceContext) error {
			userAgg, err := user.NewUserAggregate(
				userID, tenantID, email,
				"hashedpassword", fmt.Sprintf("Metrics%d", i), "User",
			)
			if err != nil {
				return err
			}
			
			if err := ctx.Track(userAgg); err != nil {
				return err
			}
			
			return ctx.Users().Save(userAgg)
		})
		
		if err != nil {
			log.Printf("Failed to create user %d: %v", i, err)
		}
	}
	
	// Log metrics
	log.Println("\n--- Service Metrics ---")
	serviceUow.LogMetrics()
	
	log.Println("\n--- Event Collector Metrics ---")
	uow.GetEventCollector().LogMetrics()
	
	log.Println("\n--- Concurrency Control Metrics ---")
	uow.GetConcurrencyManager().LogMetrics()
	
	return nil
}

// Custom Event Handlers for Examples

type CustomUserEventHandler struct{}

func (h *CustomUserEventHandler) HandleEvent(event shared.DomainEvent, aggregate shared.AggregateRoot) error {
	log.Printf("Custom User Event Handler: %s for aggregate %s", 
		event.EventType(), aggregate.GetID().String())
	return nil
}

func (h *CustomUserEventHandler) GetHandlerName() string {
	return "CustomUserEventHandler"
}

func (h *CustomUserEventHandler) CanHandle(eventType string) bool {
	return eventType == shared.EventTypeUserRegistered || 
		   eventType == shared.EventTypeUserActivated ||
		   eventType == shared.EventTypeUserUpdated
}

type CustomWorkspaceEventHandler struct{}

func (h *CustomWorkspaceEventHandler) HandleEvent(event shared.DomainEvent, aggregate shared.AggregateRoot) error {
	log.Printf("Custom Workspace Event Handler: %s for aggregate %s", 
		event.EventType(), aggregate.GetID().String())
	return nil
}

func (h *CustomWorkspaceEventHandler) GetHandlerName() string {
	return "CustomWorkspaceEventHandler"
}

func (h *CustomWorkspaceEventHandler) CanHandle(eventType string) bool {
	return eventType == shared.EventTypeWorkspaceCreated || 
		   eventType == shared.EventTypeWorkspaceUpdated ||
		   eventType == shared.EventTypeMemberAdded
}

// RunAllExamples runs all the examples
func RunAllExamples(db *sql.DB) error {
	examples := []struct {
		name string
		fn   func(*sql.DB) error
	}{
		{"Basic Unit of Work Usage", ExampleBasicUnitOfWorkUsage},
		{"Service Layer Integration", ExampleServiceLayerIntegration},
		{"Optimistic Concurrency Control", ExampleOptimisticConcurrencyControl},
		{"Event Collection and Processing", ExampleEventCollectionAndProcessing},
		{"Batch Operations", ExampleBatchOperations},
		{"Error Handling and Retries", ExampleErrorHandlingAndRetries},
		{"Complex Business Scenario", ExampleComplexBusinessScenario},
		{"Monitoring and Metrics", ExampleMonitoringAndMetrics},
	}
	
	log.Println("Running Unit of Work Pattern Examples...")
	
	for _, example := range examples {
		log.Printf("\n" + strings.Repeat("=", 60))
		log.Printf("Running: %s", example.name)
		log.Printf(strings.Repeat("=", 60))
		
		if err := example.fn(db); err != nil {
			log.Printf("Example '%s' failed: %v", example.name, err)
			return err
		}
		
		log.Printf("Example '%s' completed successfully\n", example.name)
		time.Sleep(100 * time.Millisecond) // Brief pause between examples
	}
	
	log.Println("\nAll examples completed successfully!")
	return nil
}