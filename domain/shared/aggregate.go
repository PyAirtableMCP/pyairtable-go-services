package shared

import (
	"fmt"
)

// AggregateRoot defines the interface that all aggregate roots must implement
type AggregateRoot interface {
	// GetID returns the aggregate's unique identifier
	GetID() ID
	
	// GetVersion returns the current version of the aggregate
	GetVersion() Version
	
	// GetUncommittedEvents returns events that haven't been persisted yet
	GetUncommittedEvents() []DomainEvent
	
	// ClearUncommittedEvents clears the uncommitted events (called after persistence)
	ClearUncommittedEvents()
	
	// LoadFromHistory rebuilds the aggregate from historical events
	LoadFromHistory(events []DomainEvent) error
	
	// GetTenantID returns the tenant ID for multi-tenancy support
	GetTenantID() TenantID
}

// BaseAggregateRoot provides a base implementation for aggregate roots
type BaseAggregateRoot struct {
	id                ID
	version           Version
	tenantID          TenantID
	uncommittedEvents []DomainEvent
}

// NewBaseAggregateRoot creates a new base aggregate root
func NewBaseAggregateRoot(id ID, tenantID TenantID) *BaseAggregateRoot {
	return &BaseAggregateRoot{
		id:                id,
		version:           NewVersion(),
		tenantID:          tenantID,
		uncommittedEvents: make([]DomainEvent, 0),
	}
}

// GetID returns the aggregate's ID
func (a *BaseAggregateRoot) GetID() ID {
	return a.id
}

// GetVersion returns the current version
func (a *BaseAggregateRoot) GetVersion() Version {
	return a.version
}

// GetTenantID returns the tenant ID
func (a *BaseAggregateRoot) GetTenantID() TenantID {
	return a.tenantID
}

// GetUncommittedEvents returns uncommitted events
func (a *BaseAggregateRoot) GetUncommittedEvents() []DomainEvent {
	return a.uncommittedEvents
}

// ClearUncommittedEvents clears uncommitted events
func (a *BaseAggregateRoot) ClearUncommittedEvents() {
	a.uncommittedEvents = make([]DomainEvent, 0)
}

// ApplyEvent applies an event to the aggregate and adds it to uncommitted events
func (a *BaseAggregateRoot) ApplyEvent(event DomainEvent) {
	a.uncommittedEvents = append(a.uncommittedEvents, event)
	a.version = a.version.Next()
}

// LoadFromHistory rebuilds the aggregate from events (base implementation)
func (a *BaseAggregateRoot) LoadFromHistory(events []DomainEvent) error {
	for _, event := range events {
		if event.AggregateID() != a.id.String() {
			return fmt.Errorf("event aggregate ID %s does not match aggregate ID %s", 
				event.AggregateID(), a.id.String())
		}
		a.version = NewVersionFromInt(event.Version())
	}
	return nil
}

// RaiseEvent creates and applies a domain event
func (a *BaseAggregateRoot) RaiseEvent(
	eventType string,
	aggregateType string,
	payload map[string]interface{},
) {
	event := NewBaseDomainEvent(
		eventType,
		a.id.String(),
		aggregateType,
		a.version.Next().Int(),
		a.tenantID.String(),
		payload,
	)
	a.ApplyEvent(event)
}

// Repository defines the interface for aggregate repositories
type Repository interface {
	// Save persists an aggregate and its uncommitted events
	Save(aggregate AggregateRoot) error
	
	// GetByID retrieves an aggregate by its ID
	GetByID(id ID) (AggregateRoot, error)
	
	// GetVersion retrieves the current version of an aggregate
	GetVersion(id ID) (Version, error)
	
	// Exists checks if an aggregate exists
	Exists(id ID) (bool, error)
}

// Specification defines a business rule or constraint
type Specification interface {
	// IsSatisfiedBy checks if the specification is satisfied by the given aggregate
	IsSatisfiedBy(aggregate AggregateRoot) bool
	
	// GetErrorMessage returns an error message when the specification is not satisfied
	GetErrorMessage() string
}

// BaseSpecification provides a base implementation for specifications
type BaseSpecification struct {
	errorMessage string
}

// NewBaseSpecification creates a new base specification
func NewBaseSpecification(errorMessage string) *BaseSpecification {
	return &BaseSpecification{
		errorMessage: errorMessage,
	}
}

// GetErrorMessage returns the error message
func (s *BaseSpecification) GetErrorMessage() string {
	return s.errorMessage
}

// DomainService defines the interface for domain services
type DomainService interface {
	// GetName returns the service name
	GetName() string
}

// BaseDomainService provides a base implementation for domain services
type BaseDomainService struct {
	name string
}

// NewBaseDomainService creates a new base domain service
func NewBaseDomainService(name string) *BaseDomainService {
	return &BaseDomainService{name: name}
}

// GetName returns the service name
func (s *BaseDomainService) GetName() string {
	return s.name
}

// Factory defines the interface for aggregate factories
type Factory interface {
	// Create creates a new aggregate instance
	Create(params map[string]interface{}) (AggregateRoot, error)
}

// ValueObject defines the interface for value objects
type ValueObject interface {
	// Equals checks if two value objects are equal
	Equals(other ValueObject) bool
	
	// IsValid checks if the value object is in a valid state
	IsValid() error
}