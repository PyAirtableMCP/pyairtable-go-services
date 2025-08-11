// Package workspace contains the Workspace & Collaboration bounded context domain model
package workspace

import (
	"fmt"
	"strings"

	"github.com/pyairtable-compose/go-services/domain/shared"
)

// WorkspaceAggregate represents the workspace aggregate root
type WorkspaceAggregate struct {
	*shared.BaseAggregateRoot
	name         string
	description  string
	ownerID      shared.UserID
	status       WorkspaceStatus
	settings     WorkspaceSettings
	members      map[string]*WorkspaceMember
	projects     map[string]*Project
	invitations  map[string]*Invitation
	createdAt    shared.Timestamp
	updatedAt    shared.Timestamp
}

// WorkspaceStatus represents the status of a workspace
type WorkspaceStatus string

const (
	WorkspaceStatusActive     WorkspaceStatus = "active"
	WorkspaceStatusSuspended  WorkspaceStatus = "suspended"
	WorkspaceStatusArchived   WorkspaceStatus = "archived"
	WorkspaceStatusDeleted    WorkspaceStatus = "deleted"
)

// IsValid checks if the workspace status is valid
func (s WorkspaceStatus) IsValid() bool {
	switch s {
	case WorkspaceStatusActive, WorkspaceStatusSuspended, WorkspaceStatusArchived, WorkspaceStatusDeleted:
		return true
	default:
		return false
	}
}

// WorkspaceSettings represents workspace configuration
type WorkspaceSettings struct {
	IsPublic           bool
	AllowInvitations   bool
	DefaultMemberRole  MemberRole
	MaxMembers         int
	CustomFields       map[string]interface{}
	Integrations       map[string]IntegrationConfig
	UpdatedAt          shared.Timestamp
}

// IntegrationConfig represents external integration settings
type IntegrationConfig struct {
	Enabled    bool
	Settings   map[string]interface{}
	UpdatedAt  shared.Timestamp
}

// WorkspaceMember represents a member of a workspace
type WorkspaceMember struct {
	UserID      shared.UserID
	Role        MemberRole
	Permissions []Permission
	JoinedAt    shared.Timestamp
	UpdatedAt   shared.Timestamp
	AddedBy     shared.UserID
	Status      MemberStatus
}

// MemberRole represents workspace member roles
type MemberRole string

const (
	MemberRoleOwner   MemberRole = "owner"
	MemberRoleAdmin   MemberRole = "admin"
	MemberRoleMember  MemberRole = "member"
	MemberRoleViewer  MemberRole = "viewer"
	MemberRoleGuest   MemberRole = "guest"
)

// GetDefaultPermissions returns the default permissions for a role
func (r MemberRole) GetDefaultPermissions() []Permission {
	switch r {
	case MemberRoleOwner:
		return []Permission{
			PermissionManageWorkspace,
			PermissionManageMembers,
			PermissionManageProjects,
			PermissionViewProjects,
			PermissionEditProjects,
			PermissionDeleteProjects,
			PermissionManageIntegrations,
		}
	case MemberRoleAdmin:
		return []Permission{
			PermissionManageMembers,
			PermissionManageProjects,
			PermissionViewProjects,
			PermissionEditProjects,
			PermissionDeleteProjects,
			PermissionManageIntegrations,
		}
	case MemberRoleMember:
		return []Permission{
			PermissionViewProjects,
			PermissionEditProjects,
			PermissionCreateProjects,
		}
	case MemberRoleViewer:
		return []Permission{
			PermissionViewProjects,
		}
	case MemberRoleGuest:
		return []Permission{
			PermissionViewProjects,
		}
	default:
		return []Permission{}
	}
}

// HasPermission checks if the role has a specific permission by default
func (r MemberRole) HasPermission(permission Permission) bool {
	permissions := r.GetDefaultPermissions()
	for _, p := range permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// Permission represents workspace permissions
type Permission string

const (
	PermissionManageWorkspace    Permission = "manage_workspace"
	PermissionManageMembers      Permission = "manage_members"
	PermissionManageProjects     Permission = "manage_projects"
	PermissionViewProjects       Permission = "view_projects"
	PermissionEditProjects       Permission = "edit_projects"
	PermissionCreateProjects     Permission = "create_projects"
	PermissionDeleteProjects     Permission = "delete_projects"
	PermissionManageIntegrations Permission = "manage_integrations"
	PermissionInviteMembers      Permission = "invite_members"
)

// MemberStatus represents the status of a workspace member
type MemberStatus string

const (
	MemberStatusActive    MemberStatus = "active"
	MemberStatusInactive  MemberStatus = "inactive"
	MemberStatusSuspended MemberStatus = "suspended"
)

// Project represents a project within a workspace
type Project struct {
	ID          shared.ID
	Name        string
	Description string
	OwnerID     shared.UserID
	Status      ProjectStatus
	Settings    ProjectSettings
	Members     map[string]*ProjectMember
	CreatedAt   shared.Timestamp
	UpdatedAt   shared.Timestamp
}

// ProjectStatus represents the status of a project
type ProjectStatus string

const (
	ProjectStatusActive   ProjectStatus = "active"
	ProjectStatusArchived ProjectStatus = "archived"
	ProjectStatusDeleted  ProjectStatus = "deleted"
)

// ProjectSettings represents project configuration
type ProjectSettings struct {
	IsPublic    bool
	Visibility  ProjectVisibility
	Tags        []string
	CustomData  map[string]interface{}
	UpdatedAt   shared.Timestamp
}

// ProjectVisibility represents project visibility levels
type ProjectVisibility string

const (
	ProjectVisibilityPublic   ProjectVisibility = "public"
	ProjectVisibilityPrivate  ProjectVisibility = "private"
	ProjectVisibilityWorkspace ProjectVisibility = "workspace"
)

// ProjectMember represents a member of a project
type ProjectMember struct {
	UserID      shared.UserID
	Role        ProjectRole
	Permissions []Permission
	JoinedAt    shared.Timestamp
	AddedBy     shared.UserID
}

// ProjectRole represents project member roles
type ProjectRole string

const (
	ProjectRoleOwner       ProjectRole = "owner"
	ProjectRoleCollaborator ProjectRole = "collaborator"
	ProjectRoleViewer      ProjectRole = "viewer"
)

// Invitation represents a workspace invitation
type Invitation struct {
	ID          shared.ID
	Email       shared.Email
	Role        MemberRole
	InvitedBy   shared.UserID
	Status      InvitationStatus
	Message     string
	ExpiresAt   shared.Timestamp
	CreatedAt   shared.Timestamp
	AcceptedAt  *shared.Timestamp
	Token       string
}

// InvitationStatus represents the status of an invitation
type InvitationStatus string

const (
	InvitationStatusPending  InvitationStatus = "pending"
	InvitationStatusAccepted InvitationStatus = "accepted"
	InvitationStatusDeclined InvitationStatus = "declined"
	InvitationStatusExpired  InvitationStatus = "expired"
	InvitationStatusRevoked  InvitationStatus = "revoked"
)

// IsExpired checks if the invitation is expired
func (i *Invitation) IsExpired() bool {
	return shared.NewTimestamp().After(i.ExpiresAt)
}

// NewWorkspaceAggregate creates a new workspace aggregate
func NewWorkspaceAggregate(
	id shared.WorkspaceID,
	tenantID shared.TenantID,
	name string,
	description string,
	ownerID shared.UserID,
) (*WorkspaceAggregate, error) {
	
	// Validate inputs
	if name == "" {
		return nil, shared.NewDomainError(shared.ErrCodeValidation, "workspace name is required", shared.ErrEmptyValue)
	}
	
	if len(name) > 100 {
		return nil, shared.NewDomainError(shared.ErrCodeValidation, "workspace name too long", nil)
	}
	
	if ownerID.IsEmpty() {
		return nil, shared.NewDomainError(shared.ErrCodeValidation, "owner ID is required", shared.ErrEmptyValue)
	}

	now := shared.NewTimestamp()
	
	aggregate := &WorkspaceAggregate{
		BaseAggregateRoot: shared.NewBaseAggregateRoot(id.ID, tenantID),
		name:              strings.TrimSpace(name),
		description:       strings.TrimSpace(description),
		ownerID:           ownerID,
		status:            WorkspaceStatusActive,
		settings: WorkspaceSettings{
			IsPublic:           false,
			AllowInvitations:   true,
			DefaultMemberRole:  MemberRoleMember,
			MaxMembers:         100, // Default limit
			CustomFields:       make(map[string]interface{}),
			Integrations:       make(map[string]IntegrationConfig),
			UpdatedAt:          now,
		},
		members:     make(map[string]*WorkspaceMember),
		projects:    make(map[string]*Project),
		invitations: make(map[string]*Invitation),
		createdAt:   now,
		updatedAt:   now,
	}

	// Add owner as first member
	ownerMember := &WorkspaceMember{
		UserID:      ownerID,
		Role:        MemberRoleOwner,
		Permissions: MemberRoleOwner.GetDefaultPermissions(),
		JoinedAt:    now,
		UpdatedAt:   now,
		AddedBy:     ownerID, // Owner adds themselves
		Status:      MemberStatusActive,
	}
	
	aggregate.members[ownerID.String()] = ownerMember

	// Raise workspace created event
	aggregate.RaiseEvent(
		shared.EventTypeWorkspaceCreated,
		"WorkspaceAggregate",
		map[string]interface{}{
			"workspace_id": id.String(),
			"name":         name,
			"description":  description,
			"owner_id":     ownerID.String(),
			"tenant_id":    tenantID.String(),
		},
	)

	return aggregate, nil
}

// GetName returns the workspace name
func (w *WorkspaceAggregate) GetName() string {
	return w.name
}

// GetDescription returns the workspace description
func (w *WorkspaceAggregate) GetDescription() string {
	return w.description
}

// GetOwnerID returns the workspace owner ID
func (w *WorkspaceAggregate) GetOwnerID() shared.UserID {
	return w.ownerID
}

// GetStatus returns the workspace status
func (w *WorkspaceAggregate) GetStatus() WorkspaceStatus {
	return w.status
}

// GetSettings returns the workspace settings
func (w *WorkspaceAggregate) GetSettings() WorkspaceSettings {
	return w.settings
}

// GetMemberCount returns the number of active members
func (w *WorkspaceAggregate) GetMemberCount() int {
	count := 0
	for _, member := range w.members {
		if member.Status == MemberStatusActive {
			count++
		}
	}
	return count
}

// UpdateWorkspace updates workspace information
func (w *WorkspaceAggregate) UpdateWorkspace(name, description string, updatedBy shared.UserID) error {
	if !w.canManageWorkspace(updatedBy) {
		return shared.NewDomainError(shared.ErrCodeForbidden, "insufficient permissions to update workspace", nil)
	}

	if w.status != WorkspaceStatusActive {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "cannot update inactive workspace", nil)
	}

	hasChanges := false

	if name != "" && name != w.name {
		if len(name) > 100 {
			return shared.NewDomainError(shared.ErrCodeValidation, "workspace name too long", nil)
		}
		w.name = strings.TrimSpace(name)
		hasChanges = true
	}

	if description != w.description {
		w.description = strings.TrimSpace(description)
		hasChanges = true
	}

	if hasChanges {
		w.updatedAt = shared.NewTimestamp()

		w.RaiseEvent(
			shared.EventTypeWorkspaceUpdated,
			"WorkspaceAggregate",
			map[string]interface{}{
				"workspace_id": w.GetID().String(),
				"name":         w.name,
				"description":  w.description,
				"updated_by":   updatedBy.String(),
			},
		)
	}

	return nil
}

// AddMember adds a new member to the workspace
func (w *WorkspaceAggregate) AddMember(userID shared.UserID, role MemberRole, addedBy shared.UserID) error {
	if !w.canManageMembers(addedBy) {
		return shared.NewDomainError(shared.ErrCodeForbidden, "insufficient permissions to add members", nil)
	}

	if w.status != WorkspaceStatusActive {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "cannot add members to inactive workspace", nil)
	}

	// Check if user is already a member
	if _, exists := w.members[userID.String()]; exists {
		return shared.NewDomainError(shared.ErrCodeAlreadyExists, "user is already a member", nil)
	}

	// Check member limit
	if w.GetMemberCount() >= w.settings.MaxMembers {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "workspace member limit reached", nil)
	}

	// Cannot add another owner (business rule)
	if role == MemberRoleOwner {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "workspace can only have one owner", nil)
	}

	now := shared.NewTimestamp()
	member := &WorkspaceMember{
		UserID:      userID,
		Role:        role,
		Permissions: role.GetDefaultPermissions(),
		JoinedAt:    now,
		UpdatedAt:   now,
		AddedBy:     addedBy,
		Status:      MemberStatusActive,
	}

	w.members[userID.String()] = member
	w.updatedAt = shared.NewTimestamp()

	w.RaiseEvent(
		shared.EventTypeMemberAdded,
		"WorkspaceAggregate",
		map[string]interface{}{
			"workspace_id": w.GetID().String(),
			"user_id":      userID.String(),
			"role":         string(role),
			"added_by":     addedBy.String(),
		},
	)

	return nil
}

// RemoveMember removes a member from the workspace
func (w *WorkspaceAggregate) RemoveMember(userID shared.UserID, removedBy shared.UserID) error {
	if !w.canManageMembers(removedBy) {
		return shared.NewDomainError(shared.ErrCodeForbidden, "insufficient permissions to remove members", nil)
	}

	member, exists := w.members[userID.String()]
	if !exists {
		return shared.NewDomainError(shared.ErrCodeNotFound, "member not found", nil)
	}

	// Cannot remove workspace owner
	if member.Role == MemberRoleOwner {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "cannot remove workspace owner", nil)
	}

	// Members can remove themselves
	if userID.String() != removedBy.String() && !w.canManageMembers(removedBy) {
		return shared.NewDomainError(shared.ErrCodeForbidden, "insufficient permissions", nil)
	}

	delete(w.members, userID.String())
	w.updatedAt = shared.NewTimestamp()

	w.RaiseEvent(
		shared.EventTypeMemberRemoved,
		"WorkspaceAggregate",
		map[string]interface{}{
			"workspace_id": w.GetID().String(),
			"user_id":      userID.String(),
			"role":         string(member.Role),
			"removed_by":   removedBy.String(),
		},
	)

	return nil
}

// ChangeMemberRole changes a member's role
func (w *WorkspaceAggregate) ChangeMemberRole(userID shared.UserID, newRole MemberRole, changedBy shared.UserID) error {
	if !w.canManageMembers(changedBy) {
		return shared.NewDomainError(shared.ErrCodeForbidden, "insufficient permissions to change member roles", nil)
	}

	member, exists := w.members[userID.String()]
	if !exists {
		return shared.NewDomainError(shared.ErrCodeNotFound, "member not found", nil)
	}

	// Cannot change owner role
	if member.Role == MemberRoleOwner || newRole == MemberRoleOwner {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "cannot change owner role", nil)
	}

	if member.Role == newRole {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "member already has this role", nil)
	}

	oldRole := member.Role
	member.Role = newRole
	member.Permissions = newRole.GetDefaultPermissions()
	member.UpdatedAt = shared.NewTimestamp()
	w.updatedAt = shared.NewTimestamp()

	w.RaiseEvent(
		shared.EventTypeMemberRoleChanged,
		"WorkspaceAggregate",
		map[string]interface{}{
			"workspace_id": w.GetID().String(),
			"user_id":      userID.String(),
			"old_role":     string(oldRole),
			"new_role":     string(newRole),
			"changed_by":   changedBy.String(),
		},
	)

	return nil
}

// CreateProject creates a new project in the workspace
func (w *WorkspaceAggregate) CreateProject(
	projectID shared.ID,
	name string,
	description string,
	createdBy shared.UserID,
	visibility ProjectVisibility,
) error {
	if !w.canCreateProjects(createdBy) {
		return shared.NewDomainError(shared.ErrCodeForbidden, "insufficient permissions to create projects", nil)
	}

	if w.status != WorkspaceStatusActive {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "cannot create projects in inactive workspace", nil)
	}

	if name == "" {
		return shared.NewDomainError(shared.ErrCodeValidation, "project name is required", shared.ErrEmptyValue)
	}

	// Check if project already exists
	if _, exists := w.projects[projectID.String()]; exists {
		return shared.NewDomainError(shared.ErrCodeAlreadyExists, "project already exists", nil)
	}

	now := shared.NewTimestamp()
	project := &Project{
		ID:          projectID,
		Name:        strings.TrimSpace(name),
		Description: strings.TrimSpace(description),
		OwnerID:     createdBy,
		Status:      ProjectStatusActive,
		Settings: ProjectSettings{
			IsPublic:   visibility == ProjectVisibilityPublic,
			Visibility: visibility,
			Tags:       make([]string, 0),
			CustomData: make(map[string]interface{}),
			UpdatedAt:  now,
		},
		Members:   make(map[string]*ProjectMember),
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Add creator as project owner
	project.Members[createdBy.String()] = &ProjectMember{
		UserID:      createdBy,
		Role:        ProjectRoleOwner,
		Permissions: []Permission{PermissionManageProjects, PermissionEditProjects, PermissionViewProjects},
		JoinedAt:    now,
		AddedBy:     createdBy,
	}

	w.projects[projectID.String()] = project
	w.updatedAt = shared.NewTimestamp()

	return nil
}

// GetMember returns a workspace member by user ID
func (w *WorkspaceAggregate) GetMember(userID shared.UserID) (*WorkspaceMember, bool) {
	member, exists := w.members[userID.String()]
	return member, exists
}

// GetProject returns a project by ID
func (w *WorkspaceAggregate) GetProject(projectID shared.ID) (*Project, bool) {
	project, exists := w.projects[projectID.String()]
	return project, exists
}

// ListMembers returns all workspace members
func (w *WorkspaceAggregate) ListMembers() []*WorkspaceMember {
	members := make([]*WorkspaceMember, 0, len(w.members))
	for _, member := range w.members {
		if member.Status == MemberStatusActive {
			members = append(members, member)
		}
	}
	return members
}

// ListProjects returns all workspace projects
func (w *WorkspaceAggregate) ListProjects() []*Project {
	projects := make([]*Project, 0, len(w.projects))
	for _, project := range w.projects {
		if project.Status == ProjectStatusActive {
			projects = append(projects, project)
		}
	}
	return projects
}

// Archive archives the workspace
func (w *WorkspaceAggregate) Archive(archivedBy shared.UserID) error {
	if !w.canManageWorkspace(archivedBy) {
		return shared.NewDomainError(shared.ErrCodeForbidden, "insufficient permissions to archive workspace", nil)
	}

	if w.status == WorkspaceStatusArchived {
		return shared.NewDomainError(shared.ErrCodeBusinessRule, "workspace is already archived", nil)
	}

	w.status = WorkspaceStatusArchived
	w.updatedAt = shared.NewTimestamp()

	w.RaiseEvent(
		shared.EventTypeWorkspaceDeactivated,
		"WorkspaceAggregate",
		map[string]interface{}{
			"workspace_id": w.GetID().String(),
			"status":       string(w.status),
			"archived_by":  archivedBy.String(),
		},
	)

	return nil
}

// Helper methods for permission checking
func (w *WorkspaceAggregate) canManageWorkspace(userID shared.UserID) bool {
	member, exists := w.members[userID.String()]
	if !exists || member.Status != MemberStatusActive {
		return false
	}

	return member.Role == MemberRoleOwner || 
		   member.Role == MemberRoleAdmin ||
		   w.hasPermission(member, PermissionManageWorkspace)
}

func (w *WorkspaceAggregate) canManageMembers(userID shared.UserID) bool {
	member, exists := w.members[userID.String()]
	if !exists || member.Status != MemberStatusActive {
		return false
	}

	return member.Role == MemberRoleOwner || 
		   member.Role == MemberRoleAdmin ||
		   w.hasPermission(member, PermissionManageMembers)
}

func (w *WorkspaceAggregate) canCreateProjects(userID shared.UserID) bool {
	member, exists := w.members[userID.String()]
	if !exists || member.Status != MemberStatusActive {
		return false
	}

	return member.Role.HasPermission(PermissionCreateProjects) ||
		   w.hasPermission(member, PermissionCreateProjects)
}

func (w *WorkspaceAggregate) hasPermission(member *WorkspaceMember, permission Permission) bool {
	for _, p := range member.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// LoadFromHistory rebuilds the aggregate from events
func (w *WorkspaceAggregate) LoadFromHistory(events []shared.DomainEvent) error {
	if err := w.BaseAggregateRoot.LoadFromHistory(events); err != nil {
		return err
	}

	for _, event := range events {
		switch event.EventType() {
		case shared.EventTypeWorkspaceCreated:
			w.applyWorkspaceCreatedEvent(event)
		case shared.EventTypeWorkspaceUpdated:
			w.applyWorkspaceUpdatedEvent(event)
		case shared.EventTypeMemberAdded:
			w.applyMemberAddedEvent(event)
		case shared.EventTypeMemberRemoved:
			w.applyMemberRemovedEvent(event)
		case shared.EventTypeMemberRoleChanged:
			w.applyMemberRoleChangedEvent(event)
		case shared.EventTypeWorkspaceDeactivated:
			w.applyWorkspaceDeactivatedEvent(event)
		}
	}

	return nil
}

func (w *WorkspaceAggregate) applyWorkspaceCreatedEvent(event shared.DomainEvent) {
	payload := event.Payload().(map[string]interface{})
	
	w.name = payload["name"].(string)
	w.description = payload["description"].(string)
	w.ownerID = shared.NewUserIDFromString(payload["owner_id"].(string))
	w.status = WorkspaceStatusActive
}

func (w *WorkspaceAggregate) applyWorkspaceUpdatedEvent(event shared.DomainEvent) {
	payload := event.Payload().(map[string]interface{})
	
	if name, ok := payload["name"].(string); ok {
		w.name = name
	}
	if description, ok := payload["description"].(string); ok {
		w.description = description
	}
}

func (w *WorkspaceAggregate) applyMemberAddedEvent(event shared.DomainEvent) {
	payload := event.Payload().(map[string]interface{})
	
	userID := shared.NewUserIDFromString(payload["user_id"].(string))
	role := MemberRole(payload["role"].(string))
	addedBy := shared.NewUserIDFromString(payload["added_by"].(string))
	
	member := &WorkspaceMember{
		UserID:      userID,
		Role:        role,
		Permissions: role.GetDefaultPermissions(),
		JoinedAt:    shared.NewTimestampFromTime(event.OccurredAt()),
		UpdatedAt:   shared.NewTimestampFromTime(event.OccurredAt()),
		AddedBy:     addedBy,
		Status:      MemberStatusActive,
	}
	
	w.members[userID.String()] = member
}

func (w *WorkspaceAggregate) applyMemberRemovedEvent(event shared.DomainEvent) {
	payload := event.Payload().(map[string]interface{})
	userID := payload["user_id"].(string)
	
	delete(w.members, userID)
}

func (w *WorkspaceAggregate) applyMemberRoleChangedEvent(event shared.DomainEvent) {
	payload := event.Payload().(map[string]interface{})
	
	userID := payload["user_id"].(string)
	newRole := MemberRole(payload["new_role"].(string))
	
	if member, exists := w.members[userID]; exists {
		member.Role = newRole
		member.Permissions = newRole.GetDefaultPermissions()
		member.UpdatedAt = shared.NewTimestampFromTime(event.OccurredAt())
	}
}

func (w *WorkspaceAggregate) applyWorkspaceDeactivatedEvent(event shared.DomainEvent) {
	payload := event.Payload().(map[string]interface{})
	w.status = WorkspaceStatus(payload["status"].(string))
}