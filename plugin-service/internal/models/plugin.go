package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PluginStatus represents the current status of a plugin
type PluginStatus string

const (
	PluginStatusPending    PluginStatus = "pending"
	PluginStatusInstalled  PluginStatus = "installed"
	PluginStatusActive     PluginStatus = "active"
	PluginStatusSuspended  PluginStatus = "suspended"
	PluginStatusDeprecated PluginStatus = "deprecated"
	PluginStatusError      PluginStatus = "error"
)

// PluginType defines the type of plugin
type PluginType string

const (
	PluginTypeFormula    PluginType = "formula"
	PluginTypeUI         PluginType = "ui"
	PluginTypeWebhook    PluginType = "webhook"
	PluginTypeAutomation PluginType = "automation"
	PluginTypeConnector  PluginType = "connector"
	PluginTypeView       PluginType = "view"
)

// Plugin represents a plugin in the system
type Plugin struct {
	ID              uuid.UUID            `json:"id" db:"id"`
	Name            string               `json:"name" db:"name" validate:"required,min=3,max=100"`
	Version         string               `json:"version" db:"version" validate:"required,semver"`
	Type            PluginType           `json:"type" db:"type" validate:"required"`
	Status          PluginStatus         `json:"status" db:"status"`
	DeveloperID     uuid.UUID            `json:"developer_id" db:"developer_id"`
	WorkspaceID     *uuid.UUID           `json:"workspace_id,omitempty" db:"workspace_id"`
	Description     string               `json:"description" db:"description" validate:"max=1000"`
	LongDescription string               `json:"long_description" db:"long_description"`
	IconURL         string               `json:"icon_url" db:"icon_url"`
	Tags            []string             `json:"tags" db:"tags"`
	Categories      []string             `json:"categories" db:"categories"`
	
	// Technical details
	EntryPoint      string               `json:"entry_point" db:"entry_point" validate:"required"`
	WasmBinary      []byte               `json:"-" db:"wasm_binary"`
	WasmHash        string               `json:"wasm_hash" db:"wasm_hash"`
	SourceCode      string               `json:"-" db:"source_code"`
	Dependencies    []PluginDependency   `json:"dependencies" db:"dependencies"`
	
	// Permissions and security
	Permissions     []Permission         `json:"permissions" db:"permissions"`
	Signature       string               `json:"signature" db:"signature"`
	SignedBy        string               `json:"signed_by" db:"signed_by"`
	
	// Resource limits
	ResourceLimits  ResourceLimits       `json:"resource_limits" db:"resource_limits"`
	
	// Configuration
	Config          PluginConfig         `json:"config" db:"config"`
	UserConfig      json.RawMessage      `json:"user_config,omitempty" db:"user_config"`
	
	// Metadata
	Downloads       int64                `json:"downloads" db:"downloads"`
	Rating          float64              `json:"rating" db:"rating"`
	ReviewCount     int64                `json:"review_count" db:"review_count"`
	
	// Timestamps
	CreatedAt       time.Time            `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at" db:"updated_at"`
	InstalledAt     *time.Time           `json:"installed_at,omitempty" db:"installed_at"`
	LastActiveAt    *time.Time           `json:"last_active_at,omitempty" db:"last_active_at"`
}

// PluginDependency represents a plugin dependency
type PluginDependency struct {
	Name    string `json:"name" validate:"required"`
	Version string `json:"version" validate:"required,semver"`
	Type    string `json:"type" validate:"required"`
}

// Permission represents a plugin permission
type Permission struct {
	Resource string   `json:"resource" validate:"required"`
	Actions  []string `json:"actions" validate:"required"`
	Scope    string   `json:"scope,omitempty"`
}

// ResourceLimits defines resource constraints for plugin execution
type ResourceLimits struct {
	MaxMemoryMB     int32         `json:"max_memory_mb" validate:"min=1,max=512"`
	MaxCPUPercent   int32         `json:"max_cpu_percent" validate:"min=1,max=100"`
	MaxExecutionMs  int32         `json:"max_execution_ms" validate:"min=100,max=30000"`
	MaxStorageMB    int32         `json:"max_storage_mb" validate:"min=0,max=100"`
	MaxNetworkReqs  int32         `json:"max_network_reqs" validate:"min=0,max=1000"`
}

// PluginConfig defines plugin configuration schema
type PluginConfig struct {
	Schema      json.RawMessage `json:"schema,omitempty"`
	UISchema    json.RawMessage `json:"ui_schema,omitempty"`
	DefaultData json.RawMessage `json:"default_data,omitempty"`
}

// PluginManifest represents the plugin manifest file
type PluginManifest struct {
	Name            string               `yaml:"name" json:"name" validate:"required"`
	Version         string               `yaml:"version" json:"version" validate:"required,semver"`
	Type            PluginType           `yaml:"type" json:"type" validate:"required"`
	Description     string               `yaml:"description" json:"description" validate:"required"`
	Author          string               `yaml:"author" json:"author" validate:"required"`
	License         string               `yaml:"license" json:"license"`
	Homepage        string               `yaml:"homepage" json:"homepage"`
	Repository      string               `yaml:"repository" json:"repository"`
	
	EntryPoint      string               `yaml:"entry_point" json:"entry_point" validate:"required"`
	Dependencies    []PluginDependency   `yaml:"dependencies" json:"dependencies"`
	Permissions     []Permission         `yaml:"permissions" json:"permissions"`
	ResourceLimits  ResourceLimits       `yaml:"resource_limits" json:"resource_limits"`
	Config          PluginConfig         `yaml:"config" json:"config"`
	
	Tags            []string             `yaml:"tags" json:"tags"`
	Categories      []string             `yaml:"categories" json:"categories"`
	
	// API compatibility
	MinAPIVersion   string               `yaml:"min_api_version" json:"min_api_version"`
	MaxAPIVersion   string               `yaml:"max_api_version" json:"max_api_version"`
}

// PluginInstallation represents an installed plugin instance
type PluginInstallation struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	PluginID     uuid.UUID       `json:"plugin_id" db:"plugin_id"`
	WorkspaceID  uuid.UUID       `json:"workspace_id" db:"workspace_id"`
	UserID       uuid.UUID       `json:"user_id" db:"user_id"`
	Version      string          `json:"version" db:"version"`
	Status       PluginStatus    `json:"status" db:"status"`
	Config       json.RawMessage `json:"config" db:"config"`
	
	// Runtime state
	LastError    *string         `json:"last_error,omitempty" db:"last_error"`
	RestartCount int32           `json:"restart_count" db:"restart_count"`
	
	// Timestamps
	InstalledAt  time.Time       `json:"installed_at" db:"installed_at"`
	UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`
	LastActiveAt *time.Time      `json:"last_active_at,omitempty" db:"last_active_at"`
}

// PluginExecution represents a plugin execution instance
type PluginExecution struct {
	ID               uuid.UUID   `json:"id" db:"id"`
	PluginID         uuid.UUID   `json:"plugin_id" db:"plugin_id"`
	InstallationID   uuid.UUID   `json:"installation_id" db:"installation_id"`
	WorkspaceID      uuid.UUID   `json:"workspace_id" db:"workspace_id"`
	UserID           uuid.UUID   `json:"user_id" db:"user_id"`
	
	// Execution context
	TriggerType      string      `json:"trigger_type" db:"trigger_type"`
	TriggerData      json.RawMessage `json:"trigger_data" db:"trigger_data"`
	
	// Resource usage
	MemoryUsedMB     int32       `json:"memory_used_mb" db:"memory_used_mb"`
	CPUTimeMs        int32       `json:"cpu_time_ms" db:"cpu_time_ms"`
	ExecutionTimeMs  int32       `json:"execution_time_ms" db:"execution_time_ms"`
	
	// Results
	Status           string      `json:"status" db:"status"`
	Result           json.RawMessage `json:"result,omitempty" db:"result"`
	Error            *string     `json:"error,omitempty" db:"error"`
	
	// Timestamps
	StartedAt        time.Time   `json:"started_at" db:"started_at"`
	CompletedAt      *time.Time  `json:"completed_at,omitempty" db:"completed_at"`
}

// PluginHook represents event hooks that plugins can register for
type PluginHook struct {
	ID             uuid.UUID `json:"id" db:"id"`
	PluginID       uuid.UUID `json:"plugin_id" db:"plugin_id"`
	InstallationID uuid.UUID `json:"installation_id" db:"installation_id"`
	EventType      string    `json:"event_type" db:"event_type"`
	Priority       int32     `json:"priority" db:"priority"`
	Enabled        bool      `json:"enabled" db:"enabled"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// PluginRegistry represents a plugin registry entry
type PluginRegistry struct {
	ID          uuid.UUID `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	URL         string    `json:"url" db:"url"`
	Type        string    `json:"type" db:"type"` // official, community, private
	Enabled     bool      `json:"enabled" db:"enabled"`
	TrustedKeys []string  `json:"trusted_keys" db:"trusted_keys"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// Developer represents a plugin developer
type Developer struct {
	ID              uuid.UUID `json:"id" db:"id"`
	Name            string    `json:"name" db:"name" validate:"required"`
	Email           string    `json:"email" db:"email" validate:"required,email"`
	Website         string    `json:"website" db:"website"`
	Verified        bool      `json:"verified" db:"verified"`
	PublicKey       string    `json:"public_key" db:"public_key"`
	TotalDownloads  int64     `json:"total_downloads" db:"total_downloads"`
	AverageRating   float64   `json:"average_rating" db:"average_rating"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

// ValidationError represents a plugin validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// APIError represents a standardized API error response
type APIError struct {
	Code       string             `json:"code"`
	Message    string             `json:"message"`
	Details    string             `json:"details,omitempty"`
	Validation []ValidationError  `json:"validation_errors,omitempty"`
	Timestamp  time.Time          `json:"timestamp"`
	RequestID  string             `json:"request_id,omitempty"`
}

// ListPluginsRequest represents request parameters for listing plugins
type ListPluginsRequest struct {
	WorkspaceID *uuid.UUID   `query:"workspace_id"`
	Type        *PluginType  `query:"type"`
	Status      *PluginStatus `query:"status"`
	Category    *string      `query:"category"`
	Tag         *string      `query:"tag"`
	Search      *string      `query:"search"`
	DeveloperID *uuid.UUID   `query:"developer_id"`
	
	// Pagination
	Page     int32 `query:"page" validate:"min=1"`
	PageSize int32 `query:"page_size" validate:"min=1,max=100"`
	
	// Sorting
	SortBy    string `query:"sort_by" validate:"oneof=name created_at updated_at downloads rating"`
	SortOrder string `query:"sort_order" validate:"oneof=asc desc"`
}

// ListPluginsResponse represents the response for listing plugins
type ListPluginsResponse struct {
	Plugins    []Plugin          `json:"plugins"`
	Total      int64             `json:"total"`
	Page       int32             `json:"page"`
	PageSize   int32             `json:"page_size"`
	TotalPages int32             `json:"total_pages"`
}