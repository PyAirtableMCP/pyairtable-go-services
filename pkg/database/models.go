package database

import (
	"time"

	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Email     string         `gorm:"unique;not null" json:"email"`
	Password  string         `gorm:"not null" json:"-"` // Never serialize password
	FirstName string         `json:"first_name"`
	LastName  string         `json:"last_name"`
	IsActive  bool           `gorm:"default:true" json:"is_active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName returns the table name for User
func (User) TableName() string {
	return "users"
}

// Session represents a user session
type Session struct {
	ID        string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	UserID    uint      `gorm:"not null;index" json:"user_id"`
	User      User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Token     string    `gorm:"unique;not null" json:"-"` // JWT token hash
	ExpiresAt time.Time `gorm:"not null;index" json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName returns the table name for Session
func (Session) TableName() string {
	return "sessions"
}

// IsExpired checks if the session is expired
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// WorkflowExecution represents a workflow execution
type WorkflowExecution struct {
	ID          string                 `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	WorkflowID  string                 `gorm:"not null;index" json:"workflow_id"`
	UserID      uint                   `gorm:"not null;index" json:"user_id"`
	User        User                   `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Status      string                 `gorm:"not null;default:'pending'" json:"status"` // pending, running, completed, failed
	Input       map[string]interface{} `gorm:"type:jsonb" json:"input"`
	Output      map[string]interface{} `gorm:"type:jsonb" json:"output"`
	Error       string                 `json:"error,omitempty"`
	StartedAt   *time.Time             `json:"started_at"`
	CompletedAt *time.Time             `json:"completed_at"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// TableName returns the table name for WorkflowExecution
func (WorkflowExecution) TableName() string {
	return "workflow_executions"
}

// Duration returns the execution duration
func (w *WorkflowExecution) Duration() time.Duration {
	if w.StartedAt == nil {
		return 0
	}
	if w.CompletedAt == nil {
		return time.Since(*w.StartedAt)
	}
	return w.CompletedAt.Sub(*w.StartedAt)
}

// FileUpload represents an uploaded file
type FileUpload struct {
	ID           string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	UserID       uint      `gorm:"not null;index" json:"user_id"`
	User         User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	OriginalName string    `gorm:"not null" json:"original_name"`
	FileName     string    `gorm:"unique;not null" json:"file_name"`
	ContentType  string    `gorm:"not null" json:"content_type"`
	Size         int64     `gorm:"not null" json:"size"`
	Path         string    `gorm:"not null" json:"path"`
	Status       string    `gorm:"not null;default:'uploaded'" json:"status"` // uploaded, processing, completed, failed
	Metadata     map[string]interface{} `gorm:"type:jsonb" json:"metadata"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName returns the table name for FileUpload
func (FileUpload) TableName() string {
	return "file_uploads"
}

// AnalyticsEvent represents an analytics event
type AnalyticsEvent struct {
	ID         string                 `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	UserID     *uint                  `gorm:"index" json:"user_id"`
	User       *User                  `gorm:"foreignKey:UserID" json:"user,omitempty"`
	EventName  string                 `gorm:"not null;index" json:"event_name"`
	Properties map[string]interface{} `gorm:"type:jsonb" json:"properties"`
	Timestamp  time.Time              `gorm:"not null;index" json:"timestamp"`
	SessionID  string                 `gorm:"index" json:"session_id"`
	IPAddress  string                 `json:"ip_address"`
	UserAgent  string                 `json:"user_agent"`
	CreatedAt  time.Time              `json:"created_at"`
}

// TableName returns the table name for AnalyticsEvent
func (AnalyticsEvent) TableName() string {
	return "analytics_events"
}

// Workflow represents a workflow definition
type Workflow struct {
	ID          string                 `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Name        string                 `gorm:"not null" json:"name"`
	Description string                 `json:"description"`
	UserID      uint                   `gorm:"not null;index" json:"user_id"`
	User        User                   `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Definition  map[string]interface{} `gorm:"type:jsonb;not null" json:"definition"`
	IsActive    bool                   `gorm:"default:true" json:"is_active"`
	Schedule    string                 `json:"schedule"` // cron expression
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	DeletedAt   gorm.DeletedAt         `gorm:"index" json:"-"`
}

// TableName returns the table name for Workflow
func (Workflow) TableName() string {
	return "workflows"
}

// APIKey represents an API key for external access
type APIKey struct {
	ID        string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	UserID    uint           `gorm:"not null;index" json:"user_id"`
	User      User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Name      string         `gorm:"not null" json:"name"`
	Key       string         `gorm:"unique;not null" json:"-"` // Hashed key
	LastUsed  *time.Time     `json:"last_used"`
	IsActive  bool           `gorm:"default:true" json:"is_active"`
	ExpiresAt *time.Time     `json:"expires_at"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName returns the table name for APIKey
func (APIKey) TableName() string {
	return "api_keys"
}

// IsExpired checks if the API key is expired
func (a *APIKey) IsExpired() bool {
	if a.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*a.ExpiresAt)
}