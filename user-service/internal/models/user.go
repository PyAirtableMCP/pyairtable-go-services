package models

import (
	"time"
)

// User represents a user profile in the system
type User struct {
	ID             string                 `json:"id" db:"id"`
	Email          string                 `json:"email" db:"email"`
	FirstName      string                 `json:"first_name" db:"first_name"`
	LastName       string                 `json:"last_name" db:"last_name"`
	DisplayName    string                 `json:"display_name" db:"display_name"`
	Avatar         string                 `json:"avatar,omitempty" db:"avatar"`
	Bio            string                 `json:"bio,omitempty" db:"bio"`
	Phone          string                 `json:"phone,omitempty" db:"phone"`
	TenantID       string                 `json:"tenant_id" db:"tenant_id"`
	Role           string                 `json:"role" db:"role"`
	IsActive       bool                   `json:"is_active" db:"is_active"`
	EmailVerified  bool                   `json:"email_verified" db:"email_verified"`
	PhoneVerified  bool                   `json:"phone_verified" db:"phone_verified"`
	Preferences    map[string]interface{} `json:"preferences,omitempty" db:"preferences"`
	Metadata       map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
	LastLoginAt    *time.Time             `json:"last_login_at,omitempty" db:"last_login_at"`
	CreatedAt      time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at" db:"updated_at"`
	DeletedAt      *time.Time             `json:"deleted_at,omitempty" db:"deleted_at"`
}

// UpdateUserRequest represents a user update request
type UpdateUserRequest struct {
	FirstName   *string                `json:"first_name,omitempty"`
	LastName    *string                `json:"last_name,omitempty"`
	DisplayName *string                `json:"display_name,omitempty"`
	Avatar      *string                `json:"avatar,omitempty"`
	Bio         *string                `json:"bio,omitempty"`
	Phone       *string                `json:"phone,omitempty"`
	Preferences map[string]interface{} `json:"preferences,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CreateUserRequest represents a user creation request (internal use)
type CreateUserRequest struct {
	ID            string                 `json:"id" validate:"required"`
	Email         string                 `json:"email" validate:"required,email"`
	FirstName     string                 `json:"first_name" validate:"required"`
	LastName      string                 `json:"last_name" validate:"required"`
	TenantID      string                 `json:"tenant_id"`
	Role          string                 `json:"role"`
	IsActive      bool                   `json:"is_active"`
	EmailVerified bool                   `json:"email_verified"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// UserListResponse represents a paginated list of users
type UserListResponse struct {
	Users      []*User `json:"users"`
	Total      int64   `json:"total"`
	Page       int     `json:"page"`
	PageSize   int     `json:"page_size"`
	TotalPages int     `json:"total_pages"`
}

// UserFilter represents filters for listing users
type UserFilter struct {
	TenantID      string   `query:"tenant_id"`
	Role          string   `query:"role"`
	IsActive      *bool    `query:"is_active"`
	EmailVerified *bool    `query:"email_verified"`
	Search        string   `query:"search"`
	Page          int      `query:"page"`
	PageSize      int      `query:"page_size"`
	SortBy        string   `query:"sort_by"`
	SortOrder     string   `query:"sort_order"`
	IncludeDeleted bool    `query:"include_deleted"`
}

// UserStats represents user statistics
type UserStats struct {
	TotalUsers         int64              `json:"total_users"`
	ActiveUsers        int64              `json:"active_users"`
	VerifiedUsers      int64              `json:"verified_users"`
	UsersByRole        map[string]int64   `json:"users_by_role"`
	UsersByTenant      map[string]int64   `json:"users_by_tenant"`
	RecentSignups      int64              `json:"recent_signups"`
	LastUpdated        time.Time          `json:"last_updated"`
}