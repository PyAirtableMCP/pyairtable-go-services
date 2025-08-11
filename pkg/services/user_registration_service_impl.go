package services

import (
	"database/sql"

	"github.com/pyairtable-compose/go-services/domain/services"
	"github.com/pyairtable-compose/go-services/domain/shared"
	"github.com/pyairtable-compose/go-services/domain/tenant"
	"github.com/pyairtable-compose/go-services/domain/user"
	"github.com/pyairtable-compose/go-services/domain/workspace"
	"github.com/pyairtable-compose/go-services/pkg/repositories"
)

// UserRegistrationServiceImpl implements the domain service using DDD repositories
type UserRegistrationServiceImpl struct {
	*services.UserRegistrationService
	userRepo      *repositories.UserAggregateRepository
	tenantRepo    *repositories.TenantAggregateRepository
	workspaceRepo *repositories.WorkspaceAggregateRepository
}

// NewUserRegistrationServiceImpl creates a new implementation of the user registration service
func NewUserRegistrationServiceImpl(
	eventStore shared.EventStore,
	eventBus shared.EventBus,
	db *sql.DB,
) *UserRegistrationServiceImpl {
	
	// Create repositories
	userRepo := repositories.NewUserAggregateRepository(eventStore, db)
	tenantRepo := repositories.NewTenantAggregateRepository(eventStore, db)
	workspaceRepo := repositories.NewWorkspaceAggregateRepository(eventStore, db)

	// Create the domain service
	domainService := services.NewUserRegistrationService(
		userRepo,
		tenantRepo,
		workspaceRepo,
		eventBus,
	)

	return &UserRegistrationServiceImpl{
		UserRegistrationService: domainService,
		userRepo:                userRepo,
		tenantRepo:              tenantRepo,
		workspaceRepo:           workspaceRepo,
	}
}

// RegisterUserWithEvents demonstrates the complete flow with event persistence
func (s *UserRegistrationServiceImpl) RegisterUserWithEvents(request services.UserRegistrationRequest) (*services.UserRegistrationResult, error) {
	// Use the domain service to register the user
	result, err := s.UserRegistrationService.RegisterUser(request)
	if err != nil {
		return nil, err
	}

	// The domain service already handles:
	// 1. Creating aggregates
	// 2. Raising domain events
	// 3. Saving aggregates (which persists events)
	// 4. Publishing events to the event bus

	return result, nil
}

// GetUserByIDFromEvents retrieves a user by reconstructing from events
func (s *UserRegistrationServiceImpl) GetUserByIDFromEvents(userID shared.UserID) (*user.UserAggregate, error) {
	aggregate, err := s.userRepo.GetByID(userID.ID)
	if err != nil {
		return nil, err
	}

	return aggregate.(*user.UserAggregate), nil
}

// GetUserByEmailFromEvents retrieves a user by email using event sourcing
func (s *UserRegistrationServiceImpl) GetUserByEmailFromEvents(email shared.Email) (*user.UserAggregate, error) {
	return s.userRepo.GetByEmail(email)
}

// ActivateUserWithEvents activates a user and persists the events
func (s *UserRegistrationServiceImpl) ActivateUserWithEvents(userID shared.UserID, activationToken string) error {
	// Get the user aggregate from events
	aggregate, err := s.userRepo.GetByID(userID.ID)
	if err != nil {
		return err
	}

	userAggregate := aggregate.(*user.UserAggregate)

	// Validate activation token (simplified validation)
	if len(activationToken) != 64 {
		return shared.NewDomainError(shared.ErrCodeValidation, "invalid activation token", nil)
	}

	// Activate the user (this raises events)
	if err := userAggregate.Activate(); err != nil {
		return err
	}

	// Verify email (this also raises an event)
	if err := userAggregate.VerifyEmail(); err != nil {
		return err
	}

	// Save the aggregate (this persists the events)
	if err := s.userRepo.Save(userAggregate); err != nil {
		return err
	}

	return nil
}

// UpdateUserProfileWithEvents updates a user profile and persists events
func (s *UserRegistrationServiceImpl) UpdateUserProfileWithEvents(
	userID shared.UserID,
	firstName, lastName, avatar, timezone, language string,
) error {
	// Get the user aggregate from events
	aggregate, err := s.userRepo.GetByID(userID.ID)
	if err != nil {
		return err
	}

	userAggregate := aggregate.(*user.UserAggregate)

	// Update the profile (this raises events)
	if err := userAggregate.UpdateProfile(firstName, lastName, avatar, timezone, language); err != nil {
		return err
	}

	// Save the aggregate (this persists the events)
	if err := s.userRepo.Save(userAggregate); err != nil {
		return err
	}

	return nil
}

// GetUsersByTenant retrieves all users for a tenant using event sourcing
func (s *UserRegistrationServiceImpl) GetUsersByTenant(tenantID shared.TenantID) ([]*user.UserAggregate, error) {
	return s.userRepo.GetByTenantID(tenantID)
}

// DemonstrateEventFlow demonstrates the complete event flow for user registration
func (s *UserRegistrationServiceImpl) DemonstrateEventFlow() (*DemoResult, error) {
	// 1. Create a user registration request
	email, _ := shared.NewEmail("demo@example.com")
	request := services.UserRegistrationRequest{
		Email:       email,
		Password:    "demo_password_123",
		FirstName:   "Demo",
		LastName:    "User",
		CompanyName: "Demo Company",
	}

	// 2. Register the user (this creates and persists events)
	result, err := s.RegisterUserWithEvents(request)
	if err != nil {
		return nil, err
	}

	// 3. Get the user back from events to verify event sourcing works
	reconstructedUser, err := s.GetUserByIDFromEvents(shared.NewUserIDFromString(result.User.GetID().String()))
	if err != nil {
		return nil, err
	}

	// 4. Activate the user (this creates more events)
	if err := s.ActivateUserWithEvents(
		shared.NewUserIDFromString(result.User.GetID().String()),
		"demo_activation_token_1234567890abcdef1234567890abcdef12345678",
	); err != nil {
		return nil, err
	}

	// 5. Update the user profile (more events)
	if err := s.UpdateUserProfileWithEvents(
		shared.NewUserIDFromString(result.User.GetID().String()),
		"Updated Demo",
		"Updated User",
		"https://example.com/avatar.jpg",
		"America/New_York",
		"en-US",
	); err != nil {
		return nil, err
	}

	// 6. Get the final state of the user
	finalUser, err := s.GetUserByIDFromEvents(shared.NewUserIDFromString(result.User.GetID().String()))
	if err != nil {
		return nil, err
	}

	return &DemoResult{
		InitialUser:       result.User,
		ReconstructedUser: reconstructedUser,
		FinalUser:         finalUser,
		Tenant:            result.Tenant,
		Workspace:         result.Workspace,
		IsNewTenant:       result.IsNewTenant,
	}, nil
}

// DemoResult contains the results of the event flow demonstration
type DemoResult struct {
	InitialUser       *user.UserAggregate       `json:"initial_user"`
	ReconstructedUser *user.UserAggregate       `json:"reconstructed_user"`
	FinalUser         *user.UserAggregate       `json:"final_user"`
	Tenant            *tenant.TenantAggregate   `json:"tenant"`
	Workspace         *workspace.WorkspaceAggregate `json:"workspace"`
	IsNewTenant       bool                      `json:"is_new_tenant"`
}