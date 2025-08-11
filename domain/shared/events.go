package shared

import (
	"encoding/json"
	"time"
)

// DomainEvent represents a domain event that occurred in the system
type DomainEvent interface {
	// EventID returns the unique identifier for this event
	EventID() string
	
	// EventType returns the type of the event
	EventType() string
	
	// AggregateID returns the ID of the aggregate that produced this event
	AggregateID() string
	
	// AggregateType returns the type of the aggregate
	AggregateType() string
	
	// Version returns the version of the aggregate when this event was produced
	Version() int64
	
	// OccurredAt returns when the event occurred
	OccurredAt() time.Time
	
	// TenantID returns the tenant ID for multi-tenancy
	TenantID() string
	
	// Payload returns the event-specific data
	Payload() interface{}
	
	// ToJSON serializes the event to JSON
	ToJSON() ([]byte, error)
}

// BaseDomainEvent provides a base implementation of DomainEvent
type BaseDomainEvent struct {
	EventIDValue     string                 `json:"event_id"`
	EventTypeValue   string                 `json:"event_type"`
	AggregateIDValue string                 `json:"aggregate_id"`
	AggregateTypeValue string               `json:"aggregate_type"`
	VersionValue     int64                  `json:"version"`
	OccurredAtValue  time.Time              `json:"occurred_at"`
	TenantIDValue    string                 `json:"tenant_id"`
	PayloadValue     map[string]interface{} `json:"payload"`
}

// EventID returns the event ID
func (e *BaseDomainEvent) EventID() string {
	return e.EventIDValue
}

// EventType returns the event type
func (e *BaseDomainEvent) EventType() string {
	return e.EventTypeValue
}

// AggregateID returns the aggregate ID
func (e *BaseDomainEvent) AggregateID() string {
	return e.AggregateIDValue
}

// AggregateType returns the aggregate type
func (e *BaseDomainEvent) AggregateType() string {
	return e.AggregateTypeValue
}

// Version returns the version
func (e *BaseDomainEvent) Version() int64 {
	return e.VersionValue
}

// OccurredAt returns when the event occurred
func (e *BaseDomainEvent) OccurredAt() time.Time {
	return e.OccurredAtValue
}

// TenantID returns the tenant ID
func (e *BaseDomainEvent) TenantID() string {
	return e.TenantIDValue
}

// Payload returns the payload
func (e *BaseDomainEvent) Payload() interface{} {
	return e.PayloadValue
}

// ToJSON serializes the event to JSON
func (e *BaseDomainEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// NewBaseDomainEvent creates a new base domain event
func NewBaseDomainEvent(
	eventType string,
	aggregateID string,
	aggregateType string,
	version int64,
	tenantID string,
	payload map[string]interface{},
) *BaseDomainEvent {
	return &BaseDomainEvent{
		EventIDValue:       NewID().String(),
		EventTypeValue:     eventType,
		AggregateIDValue:   aggregateID,
		AggregateTypeValue: aggregateType,
		VersionValue:       version,
		OccurredAtValue:    time.Now().UTC(),
		TenantIDValue:      tenantID,
		PayloadValue:       payload,
	}
}

// EventHandler defines a handler for domain events
type EventHandler interface {
	// Handle processes a domain event
	Handle(event DomainEvent) error
	
	// CanHandle returns true if this handler can process the given event type
	CanHandle(eventType string) bool
}

// EventBus defines the interface for publishing and subscribing to domain events
type EventBus interface {
	// Publish publishes an event to the bus
	Publish(event DomainEvent) error
	
	// Subscribe registers an event handler for specific event types
	Subscribe(eventType string, handler EventHandler) error
	
	// Unsubscribe removes an event handler
	Unsubscribe(eventType string, handler EventHandler) error
}

// EventStore defines the interface for storing and retrieving events
type EventStore interface {
	// SaveEvents saves events for an aggregate
	SaveEvents(aggregateID string, events []DomainEvent, expectedVersion int64) error
	
	// GetEvents retrieves all events for an aggregate
	GetEvents(aggregateID string) ([]DomainEvent, error)
	
	// GetEventsFromVersion retrieves events from a specific version
	GetEventsFromVersion(aggregateID string, fromVersion int64) ([]DomainEvent, error)
	
	// GetEventsByType retrieves events by type within a time range
	GetEventsByType(eventType string, from, to time.Time) ([]DomainEvent, error)
}

// Common event types
const (
	// User events
	EventTypeUserRegistered   = "user.registered"
	EventTypeUserActivated    = "user.activated"
	EventTypeUserDeactivated  = "user.deactivated"
	EventTypeUserUpdated      = "user.updated"
	EventTypeUserRoleChanged  = "user.role_changed"
	
	// Workspace events
	EventTypeWorkspaceCreated      = "workspace.created"
	EventTypeWorkspaceUpdated      = "workspace.updated"
	EventTypeWorkspaceDeactivated  = "workspace.deactivated"
	EventTypeMemberAdded           = "workspace.member_added"
	EventTypeMemberRemoved         = "workspace.member_removed"
	EventTypeMemberRoleChanged     = "workspace.member_role_changed"
	
	// Tenant events
	EventTypeTenantCreated          = "tenant.created"
	EventTypeTenantUpdated          = "tenant.updated"
	EventTypeTenantPlanChanged      = "tenant.plan_changed"
	EventTypeTenantDeactivated      = "tenant.deactivated"
	EventTypeSubscriptionUpdated    = "tenant.subscription_updated"
)