package models

import (
	"time"
	"encoding/json"
	"database/sql/driver"
	"fmt"

	"gorm.io/gorm"
)

// BaseModel contains common fields for all models
type BaseModel struct {
	ID        string         `gorm:"primarykey;type:uuid;default:gen_random_uuid()" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// JSONMap represents a JSON map field
type JSONMap map[string]interface{}

// Scan implements the sql.Scanner interface for JSONMap
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = make(JSONMap)
		return nil
	}
	
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, j)
	case string:
		return json.Unmarshal([]byte(v), j)
	default:
		return fmt.Errorf("cannot scan %T into JSONMap", value)
	}
}

// Value implements the driver.Valuer interface for JSONMap
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Resource Types
type ResourceType string

const (
	ResourceTypeWorkspace    ResourceType = "workspace"
	ResourceTypeProject      ResourceType = "project"
	ResourceTypeAirtableBase ResourceType = "airtable_base"
	ResourceTypeUser         ResourceType = "user"
	ResourceTypeFile         ResourceType = "file"
	ResourceTypeAPI          ResourceType = "api"
)

// Action Types
type ActionType string

const (
	// Read permissions
	ActionRead   ActionType = "read"
	ActionList   ActionType = "list"
	ActionView   ActionType = "view"
	
	// Write permissions
	ActionCreate ActionType = "create"
	ActionUpdate ActionType = "update"
	ActionDelete ActionType = "delete"
	
	// Management permissions
	ActionManage     ActionType = "manage"
	ActionAdmin      ActionType = "admin"
	ActionInvite     ActionType = "invite"
	ActionRemove     ActionType = "remove"
	
	// Special permissions
	ActionExecute    ActionType = "execute"
	ActionDownload   ActionType = "download"
	ActionUpload     ActionType = "upload"
	ActionShare      ActionType = "share"
)

// Role represents a role in the system
type Role struct {
	BaseModel
	Name         string    `gorm:"size:100;not null;uniqueIndex" json:"name"`
	Description  string    `gorm:"type:text" json:"description"`
	IsSystemRole bool      `gorm:"default:false" json:"is_system_role"`
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	TenantID     *string   `gorm:"size:255;index" json:"tenant_id,omitempty"` // null for system roles
	
	// Relationships
	Permissions   []*Permission     `gorm:"many2many:role_permissions;" json:"permissions,omitempty"`
	UserRoles     []*UserRole       `gorm:"foreignKey:RoleID;constraint:OnDelete:CASCADE" json:"user_roles,omitempty"`
}

// TableName sets the table name for Role
func (Role) TableName() string {
	return "roles"
}

// Permission represents a permission in the system
type Permission struct {
	BaseModel
	Name         string       `gorm:"size:200;not null;uniqueIndex" json:"name"`
	Description  string       `gorm:"type:text" json:"description"`
	ResourceType ResourceType `gorm:"size:50;not null;index" json:"resource_type"`
	Action       ActionType   `gorm:"size:50;not null;index" json:"action"`
	IsActive     bool         `gorm:"default:true" json:"is_active"`
	
	// Relationships
	Roles []*Role `gorm:"many2many:role_permissions;" json:"roles,omitempty"`
}

// TableName sets the table name for Permission
func (Permission) TableName() string {
	return "permissions"
}

// UserRole represents a user's role assignment
type UserRole struct {
	UserID       string     `gorm:"size:255;not null;primaryKey" json:"user_id"`
	RoleID       string     `gorm:"size:255;not null;primaryKey" json:"role_id"`
	ResourceType *string    `gorm:"size:50;index" json:"resource_type,omitempty"` // null for global roles
	ResourceID   *string    `gorm:"size:255;index" json:"resource_id,omitempty"`   // null for global roles
	TenantID     string     `gorm:"size:255;not null;index" json:"tenant_id"`
	GrantedBy    string     `gorm:"size:255;not null" json:"granted_by"`
	GrantedAt    time.Time  `gorm:"default:now()" json:"granted_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	IsActive     bool       `gorm:"default:true" json:"is_active"`
	
	// Relationships
	Role *Role `gorm:"foreignKey:RoleID" json:"role,omitempty"`
}

// TableName sets the table name for UserRole
func (UserRole) TableName() string {
	return "user_roles"
}

// ResourcePermission represents explicit permissions on resources
type ResourcePermission struct {
	BaseModel
	UserID       string       `gorm:"size:255;not null;index" json:"user_id"`
	ResourceType ResourceType `gorm:"size:50;not null;index" json:"resource_type"`
	ResourceID   string       `gorm:"size:255;not null;index" json:"resource_id"`
	Action       ActionType   `gorm:"size:50;not null;index" json:"action"`
	Permission   string       `gorm:"size:10;not null" json:"permission"` // allow, deny
	TenantID     string       `gorm:"size:255;not null;index" json:"tenant_id"`
	GrantedBy    string       `gorm:"size:255;not null" json:"granted_by"`
	GrantedAt    time.Time    `gorm:"default:now()" json:"granted_at"`
	ExpiresAt    *time.Time   `json:"expires_at,omitempty"`
	Reason       string       `gorm:"type:text" json:"reason,omitempty"`
}

// TableName sets the table name for ResourcePermission
func (ResourcePermission) TableName() string {
	return "resource_permissions"
}

// PermissionAuditLog represents audit log entries for permission changes
type PermissionAuditLog struct {
	ID           string    `gorm:"primarykey;type:uuid;default:gen_random_uuid()" json:"id"`
	TenantID     string    `gorm:"size:255;not null;index" json:"tenant_id"`
	UserID       string    `gorm:"size:255;not null;index" json:"user_id"`
	TargetUserID string    `gorm:"size:255;not null;index" json:"target_user_id"`
	Action       string    `gorm:"size:100;not null" json:"action"`
	ResourceType string    `gorm:"size:50;not null" json:"resource_type"`
	ResourceID   string    `gorm:"size:255" json:"resource_id"`
	Changes      JSONMap   `gorm:"type:jsonb" json:"changes"`
	IPAddress    string    `gorm:"size:45" json:"ip_address"`
	UserAgent    string    `gorm:"type:text" json:"user_agent"`
	CreatedAt    time.Time `gorm:"default:now()" json:"created_at"`
}

// TableName sets the table name for PermissionAuditLog
func (PermissionAuditLog) TableName() string {
	return "permission_audit_logs"
}

// Request and Response Models

// CheckPermissionRequest represents a permission check request
type CheckPermissionRequest struct {
	UserID       string       `json:"user_id" validate:"required"`
	ResourceType ResourceType `json:"resource_type" validate:"required"`
	ResourceID   string       `json:"resource_id"`
	Action       ActionType   `json:"action" validate:"required"`
	TenantID     string       `json:"tenant_id" validate:"required"`
}

// CheckPermissionResponse represents a permission check response
type CheckPermissionResponse struct {
	Allowed bool                   `json:"allowed"`
	Reason  string                 `json:"reason,omitempty"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// BatchCheckPermissionRequest represents a batch permission check request
type BatchCheckPermissionRequest struct {
	UserID   string                    `json:"user_id" validate:"required"`
	TenantID string                    `json:"tenant_id" validate:"required"`
	Checks   []PermissionCheckItem     `json:"checks" validate:"required,dive"`
}

type PermissionCheckItem struct {
	ResourceType ResourceType `json:"resource_type" validate:"required"`
	ResourceID   string       `json:"resource_id"`
	Action       ActionType   `json:"action" validate:"required"`
	Key          string       `json:"key"` // Optional key to identify this check in response
}

// BatchCheckPermissionResponse represents a batch permission check response
type BatchCheckPermissionResponse struct {
	Results map[string]CheckPermissionResponse `json:"results"`
}

// CreateRoleRequest represents a role creation request
type CreateRoleRequest struct {
	Name         string   `json:"name" validate:"required,min=1,max=100"`
	Description  string   `json:"description"`
	Permissions  []string `json:"permissions,omitempty"`
}

// UpdateRoleRequest represents a role update request
type UpdateRoleRequest struct {
	Name         *string  `json:"name,omitempty" validate:"omitempty,min=1,max=100"`
	Description  *string  `json:"description,omitempty"`
	IsActive     *bool    `json:"is_active,omitempty"`
	Permissions  []string `json:"permissions,omitempty"`
}

// AssignRoleRequest represents a role assignment request
type AssignRoleRequest struct {
	UserID       string     `json:"user_id" validate:"required"`
	RoleID       string     `json:"role_id" validate:"required"`
	ResourceType *string    `json:"resource_type,omitempty"`
	ResourceID   *string    `json:"resource_id,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

// GrantPermissionRequest represents a direct permission grant request
type GrantPermissionRequest struct {
	UserID       string       `json:"user_id" validate:"required"`
	ResourceType ResourceType `json:"resource_type" validate:"required"`
	ResourceID   string       `json:"resource_id" validate:"required"`
	Action       ActionType   `json:"action" validate:"required"`
	Permission   string       `json:"permission" validate:"required,oneof=allow deny"`
	ExpiresAt    *time.Time   `json:"expires_at,omitempty"`
	Reason       string       `json:"reason,omitempty"`
}

// GetUserPermissionsResponse represents user permissions response
type GetUserPermissionsResponse struct {
	UserID             string               `json:"user_id"`
	TenantID           string               `json:"tenant_id"`
	Roles              []*UserRole          `json:"roles"`
	DirectPermissions  []*ResourcePermission `json:"direct_permissions"`
	EffectivePermissions map[string][]string `json:"effective_permissions"`
}

// List Response Models

// RoleListResponse represents a paginated list of roles
type RoleListResponse struct {
	Roles      []*Role `json:"roles"`
	Total      int64   `json:"total"`
	Page       int     `json:"page"`
	PageSize   int     `json:"page_size"`
	TotalPages int     `json:"total_pages"`
}

// PermissionListResponse represents a paginated list of permissions
type PermissionListResponse struct {
	Permissions []*Permission `json:"permissions"`
	Total       int64         `json:"total"`
	Page        int           `json:"page"`
	PageSize    int           `json:"page_size"`
	TotalPages  int           `json:"total_pages"`
}

// UserRoleListResponse represents a paginated list of user roles
type UserRoleListResponse struct {
	UserRoles  []*UserRole `json:"user_roles"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

// AuditLogListResponse represents a paginated list of audit logs
type AuditLogListResponse struct {
	Logs       []*PermissionAuditLog `json:"logs"`
	Total      int64                 `json:"total"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"page_size"`
	TotalPages int                   `json:"total_pages"`
}

// Filter Models

// RoleFilter represents filters for listing roles
type RoleFilter struct {
	TenantID       string `query:"tenant_id"`
	Search         string `query:"search"`
	IsSystemRole   *bool  `query:"is_system_role"`
	IsActive       *bool  `query:"is_active"`
	Page           int    `query:"page"`
	PageSize       int    `query:"page_size"`
	SortBy         string `query:"sort_by"`
	SortOrder      string `query:"sort_order"`
}

// PermissionFilter represents filters for listing permissions
type PermissionFilter struct {
	ResourceType ResourceType `query:"resource_type"`
	Action       ActionType   `query:"action"`
	Search       string       `query:"search"`
	IsActive     *bool        `query:"is_active"`
	Page         int          `query:"page"`
	PageSize     int          `query:"page_size"`
	SortBy       string       `query:"sort_by"`
	SortOrder    string       `query:"sort_order"`
}

// UserRoleFilter represents filters for listing user roles
type UserRoleFilter struct {
	UserID       string `query:"user_id"`
	RoleID       string `query:"role_id"`
	ResourceType string `query:"resource_type"`
	ResourceID   string `query:"resource_id"`
	TenantID     string `query:"tenant_id"`
	IsActive     *bool  `query:"is_active"`
	Page         int    `query:"page"`
	PageSize     int    `query:"page_size"`
	SortBy       string `query:"sort_by"`
	SortOrder    string `query:"sort_order"`
}

// AuditLogFilter represents filters for listing audit logs
type AuditLogFilter struct {
	TenantID     string `query:"tenant_id"`
	UserID       string `query:"user_id"`
	TargetUserID string `query:"target_user_id"`
	Action       string `query:"action"`
	ResourceType string `query:"resource_type"`
	ResourceID   string `query:"resource_id"`
	Page         int    `query:"page"`
	PageSize     int    `query:"page_size"`
	SortBy       string `query:"sort_by"`
	SortOrder    string `query:"sort_order"`
}