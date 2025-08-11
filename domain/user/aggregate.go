// Package user contains the User & Authentication bounded context domain model
package user

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/pyairtable-compose/go-services/domain/shared"
)

// UserAggregate represents the user aggregate root
type UserAggregate struct {
	*shared.BaseAggregateRoot
	email           shared.Email
	hashedPassword  string
	firstName       string
	lastName        string
	status          UserStatus
	profile         *UserProfile
	roles           []UserRole
	sessions        map[string]*AuthSession
	lastLoginAt     *shared.Timestamp
	emailVerified   bool
	emailVerifiedAt *shared.Timestamp
	createdAt       shared.Timestamp
	updatedAt       shared.Timestamp
}

// UserStatus represents the status of a user
type UserStatus string

const (
	UserStatusPending    UserStatus = "pending"
	UserStatusActive     UserStatus = "active"
	UserStatusSuspended  UserStatus = "suspended"
	UserStatusDeactivated UserStatus = "deactivated"
)

// IsValid checks if the user status is valid
func (s UserStatus) IsValid() bool {
	switch s {
	case UserStatusPending, UserStatusActive, UserStatusSuspended, UserStatusDeactivated:
		return true
	default:
		return false
	}
}

// UserRole represents a role assigned to a user
type UserRole struct {
	Role        Role
	AssignedAt  shared.Timestamp
	AssignedBy  shared.UserID
	ExpiresAt   *shared.Timestamp
}

// Role represents available system roles
type Role string

const (
	RoleUser         Role = "user"
	RoleAdmin        Role = "admin"
	RoleSuperAdmin   Role = "super_admin"
	RoleWorkspaceOwner Role = "workspace_owner"
	RoleWorkspaceAdmin Role = "workspace_admin"
	RoleWorkspaceMember Role = "workspace_member"
	RoleWorkspaceViewer Role = "workspace_viewer"
)

// HasPermission checks if the role has a specific permission
func (r Role) HasPermission(permission Permission) bool {
	rolePermissions := map[Role][]Permission{
		RoleUser:            {PermissionReadOwnProfile, PermissionUpdateOwnProfile},
		RoleWorkspaceViewer: {PermissionReadOwnProfile, PermissionUpdateOwnProfile, PermissionViewWorkspace},
		RoleWorkspaceMember: {PermissionReadOwnProfile, PermissionUpdateOwnProfile, PermissionViewWorkspace, PermissionEditWorkspace},
		RoleWorkspaceAdmin:  {PermissionReadOwnProfile, PermissionUpdateOwnProfile, PermissionViewWorkspace, PermissionEditWorkspace, PermissionManageWorkspace},
		RoleWorkspaceOwner:  {PermissionReadOwnProfile, PermissionUpdateOwnProfile, PermissionViewWorkspace, PermissionEditWorkspace, PermissionManageWorkspace, PermissionDeleteWorkspace},
		RoleAdmin:           {PermissionReadOwnProfile, PermissionUpdateOwnProfile, PermissionViewWorkspace, PermissionEditWorkspace, PermissionManageWorkspace, PermissionDeleteWorkspace, PermissionManageUsers},
		RoleSuperAdmin:      {PermissionAll},
	}

	permissions, exists := rolePermissions[r]
	if !exists {
		return false
	}

	for _, p := range permissions {
		if p == permission || p == PermissionAll {
			return true
		}
	}
	return false
}

// Permission represents system permissions
type Permission string

const (
	PermissionAll                Permission = "all"
	PermissionReadOwnProfile     Permission = "read_own_profile"
	PermissionUpdateOwnProfile   Permission = "update_own_profile"
	PermissionViewWorkspace      Permission = "view_workspace"
	PermissionEditWorkspace      Permission = "edit_workspace"
	PermissionManageWorkspace    Permission = "manage_workspace"
	PermissionDeleteWorkspace    Permission = "delete_workspace"
	PermissionManageUsers        Permission = "manage_users"
	PermissionManageTenant       Permission = "manage_tenant"
)

// UserProfile represents user profile information
type UserProfile struct {
	Avatar      string
	Timezone    string
	Language    string
	Preferences map[string]interface{}
	UpdatedAt   shared.Timestamp
}

// AuthSession represents a user authentication session
type AuthSession struct {
	ID          shared.ID
	Token       string
	DeviceInfo  string
	IPAddress   string
	UserAgent   string
	CreatedAt   shared.Timestamp
	ExpiresAt   shared.Timestamp
	LastUsedAt  shared.Timestamp
	IsActive    bool
}

// IsExpired checks if the session is expired
func (s *AuthSession) IsExpired() bool {
	return shared.NewTimestamp().After(s.ExpiresAt)
}

// Refresh extends the session expiration
func (s *AuthSession) Refresh(duration time.Duration) {
	s.ExpiresAt = shared.NewTimestamp().AddDuration(duration)
	s.LastUsedAt = shared.NewTimestamp()
}

// NewUserAggregate creates a new user aggregate
func NewUserAggregate(
	id shared.UserID,
	tenantID shared.TenantID,
	email shared.Email,
	hashedPassword string,
	firstName string,
	lastName string,
) (*UserAggregate, error) {
	
	// Validate inputs
	if email.IsEmpty() {
		return nil, shared.NewDomainError(shared.ErrCodeValidation, "email is required", shared.ErrEmptyValue)
	}
	
	if hashedPassword == "" {
		return nil, shared.NewDomainError(shared.ErrCodeValidation, "password is required", shared.ErrEmptyValue)
	}
	
	if firstName == "" {
		return nil, shared.NewDomainError(shared.ErrCodeValidation, "first name is required", shared.ErrEmptyValue)
	}

	aggregate := &UserAggregate{
		BaseAggregateRoot: shared.NewBaseAggregateRoot(id.ID, tenantID),
		email:             email,
		hashedPassword:    hashedPassword,
		firstName:         firstName,
		lastName:          lastName,
		status:            UserStatusPending,
		profile:           &UserProfile{
			Timezone:    "UTC",
			Language:    "en",
			Preferences: make(map[string]interface{}),
			UpdatedAt:   shared.NewTimestamp(),
		},
		roles:           []UserRole{{Role: RoleUser, AssignedAt: shared.NewTimestamp()}},
		sessions:        make(map[string]*AuthSession),
		emailVerified:   false,
		createdAt:       shared.NewTimestamp(),
		updatedAt:       shared.NewTimestamp(),
	}

	// Raise user registered event
	aggregate.RaiseEvent(
		shared.EventTypeUserRegistered,
		"UserAggregate",
		map[string]interface{}{
			"user_id":    id.String(),
			"email":      email.String(),
			"first_name": firstName,
			"last_name":  lastName,
			"tenant_id":  tenantID.String(),
		},
	)

	return aggregate, nil
}

// GetEmail returns the user's email
func (u *UserAggregate) GetEmail() shared.Email {
	return u.email
}

// GetFullName returns the user's full name
func (u *UserAggregate) GetFullName() string {
	return strings.TrimSpace(u.firstName + " " + u.lastName)
}

// GetStatus returns the user's status
func (u *UserAggregate) GetStatus() UserStatus {
	return u.status
}

// GetProfile returns the user's profile
func (u *UserAggregate) GetProfile() *UserProfile {
	return u.profile
}

// Activate activates the user account
func (u *UserAggregate) Activate() error {
	if u.status == UserStatusActive {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "user is already active", nil)
	}

	u.status = UserStatusActive
	u.updatedAt = shared.NewTimestamp()

	u.RaiseEvent(
		shared.EventTypeUserActivated,
		"UserAggregate",
		map[string]interface{}{
			"user_id": u.GetID().String(),
			"status":  string(u.status),
		},
	)

	return nil
}

// Deactivate deactivates the user account
func (u *UserAggregate) Deactivate(reason string) error {
	if u.status == UserStatusDeactivated {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "user is already deactivated", nil)
	}

	u.status = UserStatusDeactivated
	u.updatedAt = shared.NewTimestamp()

	// Deactivate all sessions
	for _, session := range u.sessions {
		session.IsActive = false
	}

	u.RaiseEvent(
		shared.EventTypeUserDeactivated,
		"UserAggregate",
		map[string]interface{}{
			"user_id": u.GetID().String(),
			"status":  string(u.status),
			"reason":  reason,
		},
	)

	return nil
}

// UpdateProfile updates the user's profile
func (u *UserAggregate) UpdateProfile(firstName, lastName, avatar, timezone, language string) error {
	if u.status != UserStatusActive {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "cannot update profile of inactive user", nil)
	}

	hasChanges := false

	if firstName != "" && firstName != u.firstName {
		u.firstName = firstName
		hasChanges = true
	}

	if lastName != "" && lastName != u.lastName {
		u.lastName = lastName
		hasChanges = true
	}

	if avatar != u.profile.Avatar {
		u.profile.Avatar = avatar
		hasChanges = true
	}

	if timezone != "" && timezone != u.profile.Timezone {
		u.profile.Timezone = timezone
		hasChanges = true
	}

	if language != "" && language != u.profile.Language {
		u.profile.Language = language
		hasChanges = true
	}

	if hasChanges {
		u.profile.UpdatedAt = shared.NewTimestamp()
		u.updatedAt = shared.NewTimestamp()

		u.RaiseEvent(
			shared.EventTypeUserUpdated,
			"UserAggregate",
			map[string]interface{}{
				"user_id":    u.GetID().String(),
				"first_name": u.firstName,
				"last_name":  u.lastName,
				"avatar":     u.profile.Avatar,
				"timezone":   u.profile.Timezone,
				"language":   u.profile.Language,
			},
		)
	}

	return nil
}

// AddRole adds a role to the user
func (u *UserAggregate) AddRole(role Role, assignedBy shared.UserID, expiresAt *shared.Timestamp) error {
	if u.status != UserStatusActive {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "cannot add role to inactive user", nil)
	}

	// Check if role already exists
	for _, userRole := range u.roles {
		if userRole.Role == role {
			return shared.NewDomainError(shared.ErrCodeBusinessRule, "user already has this role", nil)
		}
	}

	newRole := UserRole{
		Role:       role,
		AssignedAt: shared.NewTimestamp(),
		AssignedBy: assignedBy,
		ExpiresAt:  expiresAt,
	}

	u.roles = append(u.roles, newRole)
	u.updatedAt = shared.NewTimestamp()

	u.RaiseEvent(
		shared.EventTypeUserRoleChanged,
		"UserAggregate",
		map[string]interface{}{
			"user_id":     u.GetID().String(),
			"role":        string(role),
			"action":      "added",
			"assigned_by": assignedBy.String(),
		},
	)

	return nil
}

// RemoveRole removes a role from the user
func (u *UserAggregate) RemoveRole(role Role, removedBy shared.UserID) error {
	if u.status != UserStatusActive {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "cannot remove role from inactive user", nil)
	}

	// Cannot remove the base user role
	if role == RoleUser {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "cannot remove base user role", nil)
	}

	for i, userRole := range u.roles {
		if userRole.Role == role {
			// Remove the role
			u.roles = append(u.roles[:i], u.roles[i+1:]...)
			u.updatedAt = shared.NewTimestamp()

			u.RaiseEvent(
				shared.EventTypeUserRoleChanged,
				"UserAggregate",
				map[string]interface{}{
					"user_id":    u.GetID().String(),
					"role":       string(role),
					"action":     "removed",
					"removed_by": removedBy.String(),
				},
			)

			return nil
		}
	}

	return shared.NewDomainError(shared.ErrCodeNotFound, "role not found", nil)
}

// HasRole checks if the user has a specific role
func (u *UserAggregate) HasRole(role Role) bool {
	for _, userRole := range u.roles {
		if userRole.Role == role {
			// Check if role is expired
			if userRole.ExpiresAt != nil && shared.NewTimestamp().After(*userRole.ExpiresAt) {
				continue
			}
			return true
		}
	}
	return false
}

// HasPermission checks if the user has a specific permission
func (u *UserAggregate) HasPermission(permission Permission) bool {
	for _, userRole := range u.roles {
		// Check if role is expired
		if userRole.ExpiresAt != nil && shared.NewTimestamp().After(*userRole.ExpiresAt) {
			continue
		}
		
		if userRole.Role.HasPermission(permission) {
			return true
		}
	}
	return false
}

// CreateSession creates a new authentication session
func (u *UserAggregate) CreateSession(deviceInfo, ipAddress, userAgent string, duration time.Duration) (*AuthSession, error) {
	if u.status != UserStatusActive {
		return nil, shared.NewDomainError(shared.ErrCodeBusinessRule, "cannot create session for inactive user", nil)
	}

	// Generate secure token
	token, err := generateSecureToken()
	if err != nil {
		return nil, shared.NewDomainError(shared.ErrCodeInternal, "failed to generate session token", err)
	}

	session := &AuthSession{
		ID:         shared.NewID(),
		Token:      token,
		DeviceInfo: deviceInfo,
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
		CreatedAt:  shared.NewTimestamp(),
		ExpiresAt:  shared.NewTimestamp().AddDuration(duration),
		LastUsedAt: shared.NewTimestamp(),
		IsActive:   true,
	}

	u.sessions[session.ID.String()] = session
	u.lastLoginAt = &session.CreatedAt
	u.updatedAt = shared.NewTimestamp()

	return session, nil
}

// InvalidateSession invalidates a specific session
func (u *UserAggregate) InvalidateSession(sessionID shared.ID) error {
	session, exists := u.sessions[sessionID.String()]
	if !exists {
		return shared.NewDomainError(shared.ErrCodeNotFound, "session not found", nil)
	}

	session.IsActive = false
	u.updatedAt = shared.NewTimestamp()

	return nil
}

// InvalidateAllSessions invalidates all user sessions
func (u *UserAggregate) InvalidateAllSessions() {
	for _, session := range u.sessions {
		session.IsActive = false
	}
	u.updatedAt = shared.NewTimestamp()
}

// VerifyEmail marks the user's email as verified
func (u *UserAggregate) VerifyEmail() error {
	if u.emailVerified {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "email is already verified", nil)
	}

	u.emailVerified = true
	now := shared.NewTimestamp()
	u.emailVerifiedAt = &now
	u.updatedAt = shared.NewTimestamp()

	return nil
}

// generateSecureToken generates a cryptographically secure token
func generateSecureToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// LoadFromHistory rebuilds the aggregate from events
func (u *UserAggregate) LoadFromHistory(events []shared.DomainEvent) error {
	if err := u.BaseAggregateRoot.LoadFromHistory(events); err != nil {
		return err
	}

	for _, event := range events {
		switch event.EventType() {
		case shared.EventTypeUserRegistered:
			u.applyUserRegisteredEvent(event)
		case shared.EventTypeUserActivated:
			u.applyUserActivatedEvent(event)
		case shared.EventTypeUserDeactivated:
			u.applyUserDeactivatedEvent(event)
		case shared.EventTypeUserUpdated:
			u.applyUserUpdatedEvent(event)
		case shared.EventTypeUserRoleChanged:
			u.applyUserRoleChangedEvent(event)
		}
	}

	return nil
}

func (u *UserAggregate) applyUserRegisteredEvent(event shared.DomainEvent) {
	payload := event.Payload().(map[string]interface{})
	
	email, _ := shared.NewEmail(payload["email"].(string))
	u.email = email
	u.firstName = payload["first_name"].(string)
	u.lastName = payload["last_name"].(string)
	u.status = UserStatusPending
}

func (u *UserAggregate) applyUserActivatedEvent(event shared.DomainEvent) {
	u.status = UserStatusActive
}

func (u *UserAggregate) applyUserDeactivatedEvent(event shared.DomainEvent) {
	u.status = UserStatusDeactivated
}

func (u *UserAggregate) applyUserUpdatedEvent(event shared.DomainEvent) {
	payload := event.Payload().(map[string]interface{})
	
	if firstName, ok := payload["first_name"].(string); ok {
		u.firstName = firstName
	}
	if lastName, ok := payload["last_name"].(string); ok {
		u.lastName = lastName
	}
	if avatar, ok := payload["avatar"].(string); ok {
		u.profile.Avatar = avatar
	}
	if timezone, ok := payload["timezone"].(string); ok {
		u.profile.Timezone = timezone
	}
	if language, ok := payload["language"].(string); ok {
		u.profile.Language = language
	}
}

func (u *UserAggregate) applyUserRoleChangedEvent(event shared.DomainEvent) {
	payload := event.Payload().(map[string]interface{})
	role := Role(payload["role"].(string))
	action := payload["action"].(string)
	
	if action == "added" {
		userRole := UserRole{
			Role:       role,
			AssignedAt: shared.NewTimestampFromTime(event.OccurredAt()),
		}
		u.roles = append(u.roles, userRole)
	} else if action == "removed" {
		for i, userRole := range u.roles {
			if userRole.Role == role {
				u.roles = append(u.roles[:i], u.roles[i+1:]...)
				break
			}
		}
	}
}