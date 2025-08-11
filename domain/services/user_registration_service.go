// Package services contains domain services that coordinate operations across multiple aggregates
package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"github.com/pyairtable-compose/go-services/domain/tenant"
	"github.com/pyairtable-compose/go-services/domain/user"
	"github.com/pyairtable-compose/go-services/domain/workspace"
)

// UserRegistrationService handles the complex process of user registration
// which involves creating a user, workspace, and setting up initial tenant relationships
type UserRegistrationService struct {
	*shared.BaseDomainService
	userRepo      UserRepository
	tenantRepo    TenantRepository
	workspaceRepo WorkspaceRepository
	eventBus      shared.EventBus
}

// UserRepository defines the interface for user persistence
type UserRepository interface {
	Save(aggregate *user.UserAggregate) error
	GetByID(id shared.UserID) (*user.UserAggregate, error)
	GetByEmail(email shared.Email) (*user.UserAggregate, error)
	ExistsByEmail(email shared.Email) (bool, error)
}

// TenantRepository defines the interface for tenant persistence
type TenantRepository interface {
	Save(aggregate *tenant.TenantAggregate) error
	GetByID(id shared.TenantID) (*tenant.TenantAggregate, error)
	GetBySlug(slug string) (*tenant.TenantAggregate, error)
	ExistsBySlug(slug string) (bool, error)
	ExistsByDomain(domain string) (bool, error)
}

// WorkspaceRepository defines the interface for workspace persistence
type WorkspaceRepository interface {
	Save(aggregate *workspace.WorkspaceAggregate) error
	GetByID(id shared.WorkspaceID) (*workspace.WorkspaceAggregate, error)
	GetByTenantID(tenantID shared.TenantID) ([]*workspace.WorkspaceAggregate, error)
}

// UserRegistrationRequest represents a user registration request
type UserRegistrationRequest struct {
	Email         shared.Email
	Password      string
	FirstName     string
	LastName      string
	CompanyName   string
	CompanyDomain string
	InvitationToken string // Optional - for invited users
}

// UserRegistrationResult represents the result of user registration
type UserRegistrationResult struct {
	User          *user.UserAggregate
	Tenant        *tenant.TenantAggregate
	Workspace     *workspace.WorkspaceAggregate
	IsNewTenant   bool
	ActivationToken string
}

// NewUserRegistrationService creates a new user registration service
func NewUserRegistrationService(
	userRepo UserRepository,
	tenantRepo TenantRepository,
	workspaceRepo WorkspaceRepository,
	eventBus shared.EventBus,
) *UserRegistrationService {
	return &UserRegistrationService{
		BaseDomainService: shared.NewBaseDomainService("UserRegistrationService"),
		userRepo:          userRepo,
		tenantRepo:        tenantRepo,
		workspaceRepo:     workspaceRepo,
		eventBus:          eventBus,
	}
}

// RegisterUser handles the complete user registration process
func (s *UserRegistrationService) RegisterUser(request UserRegistrationRequest) (*UserRegistrationResult, error) {
	// Validate request
	if err := s.validateRegistrationRequest(request); err != nil {
		return nil, err
	}

	// Check if user already exists
	exists, err := s.userRepo.ExistsByEmail(request.Email)
	if err != nil {
		return nil, shared.NewDomainError(shared.ErrCodeInternal, "failed to check user existence", err)
	}
	if exists {
		return nil, shared.NewDomainError(shared.ErrCodeAlreadyExists, "user already exists", nil)
	}

	// Generate secure password hash (in real implementation, use bcrypt or similar)
	hashedPassword, err := s.hashPassword(request.Password)
	if err != nil {
		return nil, shared.NewDomainError(shared.ErrCodeInternal, "failed to hash password", err)
	}

	// Determine tenant - either find existing or create new
	tenantAgg, isNewTenant, err := s.resolveTenant(request)
	if err != nil {
		return nil, err
	}

	// Create user aggregate
	userID := shared.NewUserID()
	userAgg, err := user.NewUserAggregate(
		userID,
		tenantAgg.GetTenantID(),
		request.Email,
		hashedPassword,
		request.FirstName,
		request.LastName,
	)
	if err != nil {
		return nil, err
	}

	// Generate activation token
	activationToken, err := s.generateActivationToken()
	if err != nil {
		return nil, shared.NewDomainError(shared.ErrCodeInternal, "failed to generate activation token", err)
	}

	// Create default workspace for new tenants
	var workspaceAgg *workspace.WorkspaceAggregate
	if isNewTenant {
		workspaceID := shared.NewWorkspaceID()
		workspaceName := request.CompanyName
		if workspaceName == "" {
			workspaceName = request.FirstName + "'s Workspace"
		}

		workspaceAgg, err = workspace.NewWorkspaceAggregate(
			workspaceID,
			tenantAgg.GetTenantID(),
			workspaceName,
			"Default workspace",
			userID,
		)
		if err != nil {
			return nil, err
		}
	}

	// Save aggregates (in transaction in real implementation)
	if err := s.saveAggregates(userAgg, tenantAgg, workspaceAgg, isNewTenant); err != nil {
		return nil, err
	}

	// Publish events
	if err := s.publishEvents(userAgg, tenantAgg, workspaceAgg); err != nil {
		// Log error but don't fail the registration
		// Events will eventually be published via outbox pattern
	}

	return &UserRegistrationResult{
		User:            userAgg,
		Tenant:          tenantAgg,
		Workspace:       workspaceAgg,
		IsNewTenant:     isNewTenant,
		ActivationToken: activationToken,
	}, nil
}

// ActivateUser activates a user account using an activation token
func (s *UserRegistrationService) ActivateUser(userID shared.UserID, activationToken string) error {
	// Retrieve user
	userAgg, err := s.userRepo.GetByID(userID)
	if err != nil {
		return shared.NewDomainError(shared.ErrCodeNotFound, "user not found", err)
	}

	// Validate activation token (in real implementation, store and validate tokens)
	if !s.validateActivationToken(activationToken) {
		return shared.NewDomainError(shared.ErrCodeValidation, "invalid activation token", nil)
	}

	// Activate user
	if err := userAgg.Activate(); err != nil {
		return err
	}

	// Verify email
	if err := userAgg.VerifyEmail(); err != nil {
		return err
	}

	// Save user
	if err := s.userRepo.Save(userAgg); err != nil {
		return shared.NewDomainError(shared.ErrCodeInternal, "failed to save user", err)
	}

	// Publish events
	for _, event := range userAgg.GetUncommittedEvents() {
		if err := s.eventBus.Publish(event); err != nil {
			// Log error but don't fail activation
		}
	}
	userAgg.ClearUncommittedEvents()

	return nil
}

// InviteUserToWorkspace invites a user to join an existing workspace
func (s *UserRegistrationService) InviteUserToWorkspace(
	workspaceID shared.WorkspaceID,
	email shared.Email,
	role workspace.MemberRole,
	invitedBy shared.UserID,
	message string,
) error {
	// Retrieve workspace
	workspaceAgg, err := s.workspaceRepo.GetByID(workspaceID)
	if err != nil {
		return shared.NewDomainError(shared.ErrCodeNotFound, "workspace not found", err)
	}

	// Check if user already exists
	existingUser, err := s.userRepo.GetByEmail(email)
	if err == nil && existingUser != nil {
		// User exists, add them directly to workspace
		return workspaceAgg.AddMember(shared.NewUserIDFromString(existingUser.GetID().String()), role, invitedBy)
	}

	// User doesn't exist, create invitation
	// This would involve creating an invitation aggregate or entity
	// For now, we'll just validate the invitation is allowed

	// Validate inviter has permission
	inviterMember, exists := workspaceAgg.GetMember(invitedBy)
	if !exists {
		return shared.NewDomainError(shared.ErrCodeForbidden, "inviter is not a workspace member", nil)
	}

	if !inviterMember.Role.HasPermission(workspace.PermissionInviteMembers) {
		return shared.NewDomainError(shared.ErrCodeForbidden, "insufficient permissions to invite members", nil)
	}

	// In a complete implementation, this would create an invitation
	// and send an email with a registration link that includes workspace context

	return nil
}

// validateRegistrationRequest validates the registration request
func (s *UserRegistrationService) validateRegistrationRequest(request UserRegistrationRequest) error {
	if request.Email.IsEmpty() {
		return shared.NewDomainError(shared.ErrCodeValidation, "email is required", nil)
	}

	if request.Password == "" {
		return shared.NewDomainError(shared.ErrCodeValidation, "password is required", nil)
	}

	if len(request.Password) < 8 {
		return shared.NewDomainError(shared.ErrCodeValidation, "password must be at least 8 characters", nil)
	}

	if request.FirstName == "" {
		return shared.NewDomainError(shared.ErrCodeValidation, "first name is required", nil)
	}

	return nil
}

// resolveTenant finds an existing tenant or creates a new one
func (s *UserRegistrationService) resolveTenant(request UserRegistrationRequest) (*tenant.TenantAggregate, bool, error) {
	// If company domain is provided, try to find existing tenant
	if request.CompanyDomain != "" {
		existingTenant, err := s.findTenantByDomain(request.CompanyDomain)
		if err == nil && existingTenant != nil {
			return existingTenant, false, nil
		}
	}

	// Create new tenant
	tenantID := shared.NewTenantID()
	slug := s.generateTenantSlug(request.CompanyName, request.FirstName)
	
	// Ensure slug is unique
	slug, err := s.ensureUniqueSlug(slug)
	if err != nil {
		return nil, false, err
	}

	tenantAgg, err := tenant.NewTenantAggregate(
		tenantID,
		request.CompanyName,
		slug,
		request.CompanyDomain,
		request.Email,
	)
	if err != nil {
		return nil, false, err
	}

	return tenantAgg, true, nil
}

// findTenantByDomain finds a tenant by domain
func (s *UserRegistrationService) findTenantByDomain(domain string) (*tenant.TenantAggregate, error) {
	// This would query tenants by domain
	// For now, return nil to indicate not found
	return nil, shared.NewDomainError(shared.ErrCodeNotFound, "tenant not found", nil)
}

// generateTenantSlug generates a tenant slug from company name or user name
func (s *UserRegistrationService) generateTenantSlug(companyName, firstName string) string {
	name := companyName
	if name == "" {
		name = firstName + "-org"
	}

	// Convert to slug format
	slug := shared.ToSlug(name)
	return slug
}

// ensureUniqueSlug ensures the slug is unique by appending numbers if needed
func (s *UserRegistrationService) ensureUniqueSlug(baseSlug string) (string, error) {
	slug := baseSlug
	counter := 1

	for {
		exists, err := s.tenantRepo.ExistsBySlug(slug)
		if err != nil {
			return "", err
		}
		if !exists {
			return slug, nil
		}
		
		slug = fmt.Sprintf("%s-%d", baseSlug, counter)
		counter++
		
		if counter > 100 {
			return "", shared.NewDomainError(shared.ErrCodeInternal, "unable to generate unique slug", nil)
		}
	}
}

// hashPassword hashes a password (placeholder implementation)
func (s *UserRegistrationService) hashPassword(password string) (string, error) {
	// In real implementation, use bcrypt
	return "hashed_" + password, nil
}

// generateActivationToken generates a secure activation token
func (s *UserRegistrationService) generateActivationToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// validateActivationToken validates an activation token
func (s *UserRegistrationService) validateActivationToken(token string) bool {
	// In real implementation, validate against stored tokens
	return len(token) == 64 // Simple validation for example
}

// saveAggregates saves all aggregates in a transaction
func (s *UserRegistrationService) saveAggregates(
	userAgg *user.UserAggregate,
	tenantAgg *tenant.TenantAggregate,
	workspaceAgg *workspace.WorkspaceAggregate,
	isNewTenant bool,
) error {
	// In real implementation, this would be in a database transaction
	
	if isNewTenant {
		if err := s.tenantRepo.Save(tenantAgg); err != nil {
			return shared.NewDomainError(shared.ErrCodeInternal, "failed to save tenant", err)
		}
	}

	if err := s.userRepo.Save(userAgg); err != nil {
		return shared.NewDomainError(shared.ErrCodeInternal, "failed to save user", err)
	}

	if workspaceAgg != nil {
		if err := s.workspaceRepo.Save(workspaceAgg); err != nil {
			return shared.NewDomainError(shared.ErrCodeInternal, "failed to save workspace", err)
		}
	}

	return nil
}

// publishEvents publishes domain events
func (s *UserRegistrationService) publishEvents(
	userAgg *user.UserAggregate,
	tenantAgg *tenant.TenantAggregate,
	workspaceAgg *workspace.WorkspaceAggregate,
) error {
	// Publish user events
	for _, event := range userAgg.GetUncommittedEvents() {
		if err := s.eventBus.Publish(event); err != nil {
			return err
		}
	}
	userAgg.ClearUncommittedEvents()

	// Publish tenant events
	for _, event := range tenantAgg.GetUncommittedEvents() {
		if err := s.eventBus.Publish(event); err != nil {
			return err
		}
	}
	tenantAgg.ClearUncommittedEvents()

	// Publish workspace events
	if workspaceAgg != nil {
		for _, event := range workspaceAgg.GetUncommittedEvents() {
			if err := s.eventBus.Publish(event); err != nil {
				return err
			}
		}
		workspaceAgg.ClearUncommittedEvents()
	}

	return nil
}