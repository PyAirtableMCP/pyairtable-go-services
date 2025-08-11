package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// BaseModel contains common fields for all models
type BaseModel struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// WebhookStatus represents the status of a webhook
type WebhookStatus string

const (
	WebhookStatusActive   WebhookStatus = "active"
	WebhookStatusInactive WebhookStatus = "inactive"
	WebhookStatusPaused   WebhookStatus = "paused"
	WebhookStatusFailed   WebhookStatus = "failed"
)

// EventType represents supported event types
type EventType string

const (
	EventTypeTableCreated    EventType = "table.created"
	EventTypeTableUpdated    EventType = "table.updated"
	EventTypeTableDeleted    EventType = "table.deleted"
	EventTypeRecordCreated   EventType = "record.created"
	EventTypeRecordUpdated   EventType = "record.updated"
	EventTypeRecordDeleted   EventType = "record.deleted"
	EventTypeWorkspaceCreated EventType = "workspace.created"
	EventTypeWorkspaceUpdated EventType = "workspace.updated"
	EventTypeUserInvited     EventType = "user.invited"
	EventTypeUserJoined      EventType = "user.joined"
)

// DeliveryStatus represents the status of a webhook delivery attempt
type DeliveryStatus string

const (
	DeliveryStatusPending   DeliveryStatus = "pending"
	DeliveryStatusDelivered DeliveryStatus = "delivered"
	DeliveryStatusFailed    DeliveryStatus = "failed"
	DeliveryStatusRetrying  DeliveryStatus = "retrying"
	DeliveryStatusAbandoned DeliveryStatus = "abandoned"
)

// JSONMap represents a JSON object stored in the database
type JSONMap map[string]interface{}

// Value implements the driver.Valuer interface
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements the sql.Scanner interface
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into JSONMap", value)
	}

	return json.Unmarshal(bytes, j)
}

// Webhook represents a webhook endpoint configuration
type Webhook struct {
	BaseModel
	UserID          uint          `gorm:"not null;index" json:"user_id"`
	WorkspaceID     *uint         `gorm:"index" json:"workspace_id,omitempty"`
	Name            string        `gorm:"size:255;not null" json:"name"`
	URL             string        `gorm:"size:2048;not null" json:"url"`
	Secret          string        `gorm:"size:255;not null" json:"-"` // HMAC secret, never returned in API
	Status          WebhookStatus `gorm:"size:50;not null;default:'active'" json:"status"`
	Description     string        `gorm:"type:text" json:"description"`
	MaxRetries      int           `gorm:"default:3" json:"max_retries"`
	TimeoutSeconds  int           `gorm:"default:30" json:"timeout_seconds"`
	RateLimitRPS    int           `gorm:"default:10" json:"rate_limit_rps"`
	LastDeliveryAt  *time.Time    `json:"last_delivery_at"`
	LastFailureAt   *time.Time    `json:"last_failure_at"`
	FailureCount    int           `gorm:"default:0" json:"failure_count"`
	SuccessCount    int           `gorm:"default:0" json:"success_count"`
	Headers         JSONMap       `gorm:"type:jsonb" json:"headers"`
	Metadata        JSONMap       `gorm:"type:jsonb" json:"metadata"`
	
	// Relationships
	Subscriptions   []Subscription   `gorm:"foreignKey:WebhookID;constraint:OnDelete:CASCADE" json:"subscriptions,omitempty"`
	DeliveryAttempts []DeliveryAttempt `gorm:"foreignKey:WebhookID;constraint:OnDelete:CASCADE" json:"-"`
}

// TableName sets the table name for Webhook
func (Webhook) TableName() string {
	return "webhooks"
}

// Subscription represents an event subscription for a webhook
type Subscription struct {
	BaseModel
	WebhookID   uint      `gorm:"not null;index" json:"webhook_id"`
	EventType   EventType `gorm:"size:100;not null;index" json:"event_type"`
	ResourceID  *string   `gorm:"size:255;index" json:"resource_id,omitempty"` // Optional filter by specific resource
	IsActive    bool      `gorm:"default:true" json:"is_active"`
	Filters     JSONMap   `gorm:"type:jsonb" json:"filters"` // Additional filtering conditions
	
	// Relationships
	Webhook     Webhook   `gorm:"constraint:OnDelete:CASCADE" json:"-"`
}

// TableName sets the table name for Subscription
func (Subscription) TableName() string {
	return "webhook_subscriptions"
}

// DeliveryAttempt represents a webhook delivery attempt
type DeliveryAttempt struct {
	BaseModel
	WebhookID       uint           `gorm:"not null;index" json:"webhook_id"`
	EventID         string         `gorm:"size:255;not null;index" json:"event_id"`
	EventType       EventType      `gorm:"size:100;not null;index" json:"event_type"`
	Payload         JSONMap        `gorm:"type:jsonb;not null" json:"payload"`
	Status          DeliveryStatus `gorm:"size:50;not null;default:'pending';index" json:"status"`
	AttemptNumber   int            `gorm:"default:1" json:"attempt_number"`
	ScheduledAt     time.Time      `gorm:"not null;index" json:"scheduled_at"`
	AttemptedAt     *time.Time     `json:"attempted_at"`
	DeliveredAt     *time.Time     `json:"delivered_at"`
	ResponseStatus  *int           `json:"response_status"`
	ResponseHeaders JSONMap        `gorm:"type:jsonb" json:"response_headers"`
	ResponseBody    *string        `gorm:"type:text" json:"response_body"`
	ErrorMessage    *string        `gorm:"type:text" json:"error_message"`
	DurationMs      *int64         `json:"duration_ms"`
	Signature       string         `gorm:"size:255" json:"-"` // HMAC signature sent
	IdempotencyKey  string         `gorm:"size:255;not null;unique" json:"idempotency_key"`
	NextRetryAt     *time.Time     `json:"next_retry_at"`
	
	// Relationships
	Webhook         Webhook        `gorm:"constraint:OnDelete:CASCADE" json:"-"`
}

// TableName sets the table name for DeliveryAttempt
func (DeliveryAttempt) TableName() string {
	return "webhook_delivery_attempts"
}

// DeadLetter represents a failed webhook delivery that couldn't be retried
type DeadLetter struct {
	BaseModel
	WebhookID       uint      `gorm:"not null;index" json:"webhook_id"`
	EventID         string    `gorm:"size:255;not null;index" json:"event_id"`
	EventType       EventType `gorm:"size:100;not null;index" json:"event_type"`
	Payload         JSONMap   `gorm:"type:jsonb;not null" json:"payload"`
	FailureReason   string    `gorm:"type:text;not null" json:"failure_reason"`
	LastAttemptAt   time.Time `gorm:"not null" json:"last_attempt_at"`
	AttemptCount    int       `gorm:"not null" json:"attempt_count"`
	IsProcessed     bool      `gorm:"default:false;index" json:"is_processed"`
	ProcessedAt     *time.Time `json:"processed_at"`
	OriginalHeaders JSONMap   `gorm:"type:jsonb" json:"original_headers"`
	Metadata        JSONMap   `gorm:"type:jsonb" json:"metadata"`
	
	// Relationships
	Webhook         Webhook   `gorm:"constraint:OnDelete:CASCADE" json:"-"`
}

// TableName sets the table name for DeadLetter
func (DeadLetter) TableName() string {
	return "webhook_dead_letters"
}

// WebhookEvent represents an event that triggers webhook deliveries
type WebhookEvent struct {
	ID          string    `json:"id"`
	Type        EventType `json:"type"`
	ResourceID  string    `json:"resource_id"`
	WorkspaceID *uint     `json:"workspace_id,omitempty"`
	UserID      uint      `json:"user_id"`
	Timestamp   time.Time `json:"timestamp"`
	Data        JSONMap   `json:"data"`
	Metadata    JSONMap   `json:"metadata,omitempty"`
}

// WebhookMetrics represents aggregated webhook metrics
type WebhookMetrics struct {
	WebhookID         uint    `json:"webhook_id"`
	TotalDeliveries   int64   `json:"total_deliveries"`
	SuccessfulDeliveries int64 `json:"successful_deliveries"`
	FailedDeliveries  int64   `json:"failed_deliveries"`
	AverageLatencyMs  float64 `json:"average_latency_ms"`
	LastDeliveryAt    *time.Time `json:"last_delivery_at"`
	SuccessRate       float64 `json:"success_rate"`
}

// CircuitBreakerState represents the state of a circuit breaker for a webhook
type CircuitBreakerState struct {
	WebhookID     uint      `json:"webhook_id"`
	State         string    `json:"state"` // closed, open, half-open
	FailureCount  int       `json:"failure_count"`
	LastFailureAt time.Time `json:"last_failure_at"`
	NextRetryAt   time.Time `json:"next_retry_at"`
}