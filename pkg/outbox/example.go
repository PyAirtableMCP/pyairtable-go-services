package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/pyairtable-compose/go-services/domain/shared"
)

// This file demonstrates end-to-end usage of the outbox pattern
// in a realistic PyAirtable scenario

// ExampleUserService demonstrates outbox integration in the user service
type ExampleUserService struct {
	db            *sql.DB
	outboxService *OutboxService
	userRepo      *ExampleUserRepository
}

// ExampleUser represents a user aggregate
type ExampleUser struct {
	shared.BaseAggregate
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	TenantID string `json:"tenant_id"`
	Status   string `json:"status"`
}

// ExampleUserRepository demonstrates outbox-enabled repository
type ExampleUserRepository struct {
	outboxRepo *OutboxEnabledAggregateRepository
}

// UserRegisteredEvent represents a user registration event
type UserRegisteredEvent struct {
	*shared.BaseDomainEvent
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	TenantID string `json:"tenant_id"`
}

// UserActivatedEvent represents a user activation event
type UserActivatedEvent struct {
	*shared.BaseDomainEvent
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	TenantID string `json:"tenant_id"`
}

// SetupExampleUserService demonstrates complete outbox setup for user service
func SetupExampleUserService(db *sql.DB) (*ExampleUserService, error) {
	log.Println("Setting up example user service with outbox pattern...")
	
	// 1. Initialize outbox service
	outboxService, err := SetupForUserService(db)
	if err != nil {
		return nil, fmt.Errorf("failed to setup outbox service: %w", err)
	}
	
	// 2. Start outbox service
	if err := outboxService.Start(); err != nil {
		return nil, fmt.Errorf("failed to start outbox service: %w", err)
	}
	
	// 3. Create outbox-enabled repository
	baseUserRepo := &BaseUserRepository{db: db}
	outboxEnabledRepo := NewOutboxEnabledAggregateRepository(baseUserRepo, outboxService.Wrapper)
	userRepo := &ExampleUserRepository{outboxRepo: outboxEnabledRepo}
	
	service := &ExampleUserService{
		db:            db,
		outboxService: outboxService,
		userRepo:      userRepo,
	}
	
	log.Println("Example user service setup completed successfully")
	return service, nil
}

// RegisterUser demonstrates user registration with automatic event publishing via outbox
func (s *ExampleUserService) RegisterUser(ctx context.Context, email, name, tenantID string) (*ExampleUser, error) {
	log.Printf("Registering user: %s (%s) for tenant %s", name, email, tenantID)
	
	// 1. Create user aggregate
	user := &ExampleUser{
		ID:       uuid.New().String(),
		Email:    email,
		Name:     name,
		TenantID: tenantID,
		Status:   "registered",
	}
	
	// 2. Initialize base aggregate
	user.BaseAggregate = *shared.NewBaseAggregate(user.ID, "user", tenantID)
	
	// 3. Create domain event
	registeredEvent := &UserRegisteredEvent{
		BaseDomainEvent: shared.NewBaseDomainEvent(
			shared.EventTypeUserRegistered,
			user.ID,
			"user",
			user.GetVersion()+1,
			tenantID,
			map[string]interface{}{
				"user_id":  user.ID,
				"email":    user.Email,
				"name":     user.Name,
				"tenant_id": user.TenantID,
				"status":   user.Status,
			},
		),
		UserID:   user.ID,
		Email:    user.Email,
		Name:     user.Name,
		TenantID: user.TenantID,
	}
	
	// 4. Add event to aggregate
	user.AddEvent(registeredEvent)
	
	// 5. Save user (this will automatically add events to outbox)
	if err := s.userRepo.Save(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to save user: %w", err)
	}
	
	log.Printf("User %s registered successfully. Event will be published via outbox.", user.ID)
	return user, nil
}

// ActivateUser demonstrates user activation with event publishing
func (s *ExampleUserService) ActivateUser(ctx context.Context, userID string) error {
	log.Printf("Activating user: %s", userID)
	
	// 1. Load user aggregate
	userAggregate, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to load user: %w", err)
	}
	
	user, ok := userAggregate.(*ExampleUser)
	if !ok {
		return fmt.Errorf("invalid user aggregate type")
	}
	
	// 2. Update user status
	user.Status = "active"
	
	// 3. Create activation event
	activatedEvent := &UserActivatedEvent{
		BaseDomainEvent: shared.NewBaseDomainEvent(
			shared.EventTypeUserActivated,
			user.ID,
			"user",
			user.GetVersion()+1,
			user.TenantID,
			map[string]interface{}{
				"user_id":  user.ID,
				"email":    user.Email,
				"tenant_id": user.TenantID,
				"activated_at": time.Now(),
			},
		),
		UserID:   user.ID,
		Email:    user.Email,
		TenantID: user.TenantID,
	}
	
	// 4. Add event to aggregate
	user.AddEvent(activatedEvent)
	
	// 5. Save user (events automatically go to outbox)
	if err := s.userRepo.Save(ctx, user); err != nil {
		return fmt.Errorf("failed to save activated user: %w", err)
	}
	
	log.Printf("User %s activated successfully. Event will be published via outbox.", userID)
	return nil
}

// BatchProcessUsers demonstrates batch operations with outbox
func (s *ExampleUserService) BatchProcessUsers(ctx context.Context, userUpdates []UserUpdate) error {
	log.Printf("Processing batch of %d user updates", len(userUpdates))
	
	// Prepare batch operations
	operations := make([]EventProducingOperation, len(userUpdates))
	
	for i, update := range userUpdates {
		userID := update.UserID
		
		operations[i] = EventProducingOperation{
			Operation: func(tx *sql.Tx) error {
				// Update user in database
				query := `UPDATE users SET name = $1, updated_at = NOW() WHERE id = $2`
				_, err := tx.Exec(query, update.NewName, userID)
				return err
			},
			Events: []shared.DomainEvent{
				&shared.BaseDomainEvent{
					EventIDValue:       uuid.New().String(),
					EventTypeValue:     shared.EventTypeUserUpdated,
					AggregateIDValue:   userID,
					AggregateTypeValue: "user",
					VersionValue:       1, // In real scenario, get from aggregate
					TenantIDValue:      update.TenantID,
					OccurredAtValue:    time.Now(),
					PayloadValue: map[string]interface{}{
						"user_id":  userID,
						"old_name": update.OldName,
						"new_name": update.NewName,
						"tenant_id": update.TenantID,
					},
				},
			},
		}
	}
	
	// Execute batch with outbox
	batchOp := CombineOperations(operations...)
	if err := s.outboxService.Wrapper.ExecuteBatch(ctx, batchOp); err != nil {
		return fmt.Errorf("failed to execute batch operations: %w", err)
	}
	
	log.Printf("Batch processed successfully. %d events added to outbox.", len(userUpdates))
	return nil
}

// UserUpdate represents a user update operation
type UserUpdate struct {
	UserID   string
	TenantID string
	OldName  string
	NewName  string
}

// GetOutboxStatus demonstrates how to monitor outbox status
func (s *ExampleUserService) GetOutboxStatus() map[string]interface{} {
	return s.outboxService.GetStatus()
}

// BaseUserRepository implements basic user repository operations
type BaseUserRepository struct {
	db *sql.DB
}

// Save saves a user aggregate
func (r *BaseUserRepository) Save(ctx context.Context, aggregate shared.AggregateRoot) error {
	user, ok := aggregate.(*ExampleUser)
	if !ok {
		return fmt.Errorf("invalid aggregate type")
	}
	
	// In a real implementation, this would be more sophisticated
	query := `
		INSERT INTO users (id, email, name, tenant_id, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		ON CONFLICT (id) 
		DO UPDATE SET 
			name = EXCLUDED.name,
			status = EXCLUDED.status,
			version = EXCLUDED.version,
			updated_at = NOW()
	`
	
	_, err := r.db.ExecContext(ctx, query,
		user.ID, user.Email, user.Name, user.TenantID, user.Status, user.GetVersion())
	
	return err
}

// GetByID loads a user by ID
func (r *BaseUserRepository) GetByID(ctx context.Context, id string) (shared.AggregateRoot, error) {
	query := `SELECT id, email, name, tenant_id, status, version FROM users WHERE id = $1`
	
	user := &ExampleUser{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Email, &user.Name, &user.TenantID, &user.Status, &user.BaseAggregate.Version)
	
	if err != nil {
		return nil, err
	}
	
	user.BaseAggregate = *shared.NewBaseAggregate(user.ID, "user", user.TenantID)
	user.BaseAggregate.Version = user.BaseAggregate.Version // Set loaded version
	
	return user, nil
}

// Save saves a user via outbox-enabled repository
func (r *ExampleUserRepository) Save(ctx context.Context, user shared.AggregateRoot) error {
	return r.outboxRepo.Save(ctx, user)
}

// GetByID gets a user by ID
func (r *ExampleUserRepository) GetByID(ctx context.Context, id string) (shared.AggregateRoot, error) {
	return r.outboxRepo.GetByID(ctx, id)
}

// ExampleEventHandler demonstrates event handling for outbox events
type ExampleEventHandler struct {
	name string
}

// NewExampleEventHandler creates a new event handler
func NewExampleEventHandler(name string) *ExampleEventHandler {
	return &ExampleEventHandler{name: name}
}

// Handle processes a domain event
func (h *ExampleEventHandler) Handle(event shared.DomainEvent) error {
	log.Printf("Handler %s processing event %s (type: %s, aggregate: %s)",
		h.name, event.EventID(), event.EventType(), event.AggregateID())
	
	switch event.EventType() {
	case shared.EventTypeUserRegistered:
		return h.handleUserRegistered(event)
	case shared.EventTypeUserActivated:
		return h.handleUserActivated(event)
	case shared.EventTypeUserUpdated:
		return h.handleUserUpdated(event)
	default:
		log.Printf("Handler %s: unknown event type %s", h.name, event.EventType())
		return nil
	}
}

// CanHandle returns true if this handler can process the given event type
func (h *ExampleEventHandler) CanHandle(eventType string) bool {
	return eventType == shared.EventTypeUserRegistered ||
		eventType == shared.EventTypeUserActivated ||
		eventType == shared.EventTypeUserUpdated
}

// handleUserRegistered handles user registration events
func (h *ExampleEventHandler) handleUserRegistered(event shared.DomainEvent) error {
	payload := event.Payload().(map[string]interface{})
	
	userID := payload["user_id"].(string)
	email := payload["email"].(string)
	name := payload["name"].(string)
	tenantID := payload["tenant_id"].(string)
	
	log.Printf("Handler %s: User registered - ID: %s, Email: %s, Name: %s, Tenant: %s",
		h.name, userID, email, name, tenantID)
	
	// In a real scenario, this might:
	// - Send welcome email
	// - Create user profile in read model
	// - Update analytics
	// - Trigger onboarding workflow
	
	return nil
}

// handleUserActivated handles user activation events
func (h *ExampleEventHandler) handleUserActivated(event shared.DomainEvent) error {
	payload := event.Payload().(map[string]interface{})
	
	userID := payload["user_id"].(string)
	email := payload["email"].(string)
	tenantID := payload["tenant_id"].(string)
	
	log.Printf("Handler %s: User activated - ID: %s, Email: %s, Tenant: %s",
		h.name, userID, email, tenantID)
	
	// In a real scenario, this might:
	// - Send activation confirmation email
	// - Update user permissions
	// - Enable features
	// - Update billing
	
	return nil
}

// handleUserUpdated handles user update events
func (h *ExampleEventHandler) handleUserUpdated(event shared.DomainEvent) error {
	payload := event.Payload().(map[string]interface{})
	
	userID := payload["user_id"].(string)
	oldName := payload["old_name"].(string)
	newName := payload["new_name"].(string)
	
	log.Printf("Handler %s: User updated - ID: %s, Name changed from '%s' to '%s'",
		h.name, userID, oldName, newName)
	
	// In a real scenario, this might:
	// - Update search indexes
	// - Update user profile views
	// - Audit log entry
	// - Notification to team members
	
	return nil
}

// RunExampleDemo demonstrates the complete outbox pattern in action
func RunExampleDemo(db *sql.DB) error {
	log.Println("=== Starting PyAirtable Outbox Pattern Demo ===")
	
	// 1. Setup user service with outbox
	userService, err := SetupExampleUserService(db)
	if err != nil {
		return fmt.Errorf("failed to setup user service: %w", err)
	}
	defer userService.outboxService.Stop()
	
	// 2. Setup event handlers (these would typically be in other services)
	notificationHandler := NewExampleEventHandler("notification-service")
	analyticsHandler := NewExampleEventHandler("analytics-service")
	auditHandler := NewExampleEventHandler("audit-service")
	
	// 3. Register event handlers with the event bus (if using event bus publisher)
	// In a real scenario, these would be separate services consuming from Kafka, etc.
	
	ctx := context.Background()
	
	// 4. Demonstrate user registration
	log.Println("\n--- Demonstrating User Registration ---")
	user1, err := userService.RegisterUser(ctx, "john.doe@example.com", "John Doe", "tenant-1")
	if err != nil {
		return fmt.Errorf("failed to register user: %w", err)
	}
	
	user2, err := userService.RegisterUser(ctx, "jane.smith@example.com", "Jane Smith", "tenant-1")
	if err != nil {
		return fmt.Errorf("failed to register user: %w", err)
	}
	
	// 5. Demonstrate user activation
	log.Println("\n--- Demonstrating User Activation ---")
	if err := userService.ActivateUser(ctx, user1.ID); err != nil {
		return fmt.Errorf("failed to activate user: %w", err)
	}
	
	// 6. Demonstrate batch operations
	log.Println("\n--- Demonstrating Batch Operations ---")
	updates := []UserUpdate{
		{UserID: user1.ID, TenantID: "tenant-1", OldName: "John Doe", NewName: "John D. Doe"},
		{UserID: user2.ID, TenantID: "tenant-1", OldName: "Jane Smith", NewName: "Jane S. Smith"},
	}
	
	if err := userService.BatchProcessUsers(ctx, updates); err != nil {
		return fmt.Errorf("failed to process batch updates: %w", err)
	}
	
	// 7. Check outbox status
	log.Println("\n--- Outbox Status ---")
	status := userService.GetOutboxStatus()
	statusJSON, _ := fmt.Sprintf("%+v", status)
	log.Printf("Outbox Status: %s", statusJSON)
	
	// 8. Simulate event processing by handlers
	log.Println("\n--- Simulating Event Processing ---")
	
	// In a real scenario, these events would be consumed from the outbox by workers
	// and delivered to actual event handlers. For demo purposes, we'll simulate this.
	
	sampleEvents := []shared.DomainEvent{
		&shared.BaseDomainEvent{
			EventIDValue:       uuid.New().String(),
			EventTypeValue:     shared.EventTypeUserRegistered,
			AggregateIDValue:   user1.ID,
			AggregateTypeValue: "user",
			VersionValue:       1,
			TenantIDValue:      "tenant-1",
			OccurredAtValue:    time.Now(),
			PayloadValue: map[string]interface{}{
				"user_id":  user1.ID,
				"email":    user1.Email,
				"name":     user1.Name,
				"tenant_id": user1.TenantID,
			},
		},
		&shared.BaseDomainEvent{
			EventIDValue:       uuid.New().String(),
			EventTypeValue:     shared.EventTypeUserActivated,
			AggregateIDValue:   user1.ID,
			AggregateTypeValue: "user",
			VersionValue:       2,
			TenantIDValue:      "tenant-1",
			OccurredAtValue:    time.Now(),
			PayloadValue: map[string]interface{}{
				"user_id":  user1.ID,
				"email":    user1.Email,
				"tenant_id": user1.TenantID,
			},
		},
	}
	
	for _, event := range sampleEvents {
		log.Printf("\nProcessing event: %s", event.EventType())
		
		// Process event with each handler
		if notificationHandler.CanHandle(event.EventType()) {
			notificationHandler.Handle(event)
		}
		if analyticsHandler.CanHandle(event.EventType()) {
			analyticsHandler.Handle(event)
		}
		if auditHandler.CanHandle(event.EventType()) {
			auditHandler.Handle(event)
		}
	}
	
	// 9. Wait a bit for background processing
	log.Println("\n--- Waiting for background processing ---")
	time.Sleep(5 * time.Second)
	
	// 10. Final status check
	log.Println("\n--- Final Status Check ---")
	finalStatus := userService.GetOutboxStatus()
	finalStatusJSON, _ := fmt.Sprintf("%+v", finalStatus)
	log.Printf("Final Outbox Status: %s", finalStatusJSON)
	
	log.Println("\n=== PyAirtable Outbox Pattern Demo Completed ===")
	return nil
}

// Example SQL for creating the users table (for demo purposes)
const CreateUsersTableSQL = `
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(255) PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    tenant_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'registered',
    version BIGINT DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
`