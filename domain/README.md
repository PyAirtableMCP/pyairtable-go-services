# Domain Layer - PyAirtable Compose

This directory contains the domain layer implementation following Domain-Driven Design (DDD) principles. The domain layer encapsulates the core business logic and rules of the PyAirtable Compose system.

## Architecture Overview

```
domain/
├── shared/           # Shared domain infrastructure and primitives
├── user/            # User & Authentication bounded context
├── workspace/       # Workspace & Collaboration bounded context  
├── tenant/          # Tenant Management bounded context
├── services/        # Domain services for cross-aggregate operations
└── BOUNDED_CONTEXTS.md  # Detailed context documentation
```

## Core Design Principles

### 1. Aggregate Roots
Each bounded context contains aggregate roots that maintain consistency boundaries:

- **UserAggregate**: Manages user identity, authentication, and authorization
- **WorkspaceAggregate**: Manages workspace lifecycle, members, and projects  
- **TenantAggregate**: Manages tenant subscriptions, quotas, and billing

### 2. Domain Events
All significant business events are captured as domain events:

```go
// Example: User registration triggers multiple events
user.RegisterUser() → UserRegistered
tenant.Create() → TenantCreated  
workspace.Create() → WorkspaceCreated
```

### 3. Business Rules Enforcement
Business invariants are enforced within aggregates:

```go
// Example: Workspace member limits
func (w *WorkspaceAggregate) AddMember(userID UserID, role MemberRole) error {
    if w.GetMemberCount() >= w.settings.MaxMembers {
        return ErrMemberLimitReached
    }
    // ... rest of implementation
}
```

### 4. Value Objects
Complex domain concepts are modeled as value objects:

```go
// Examples of value objects
type Email struct { value string }
type Money struct { Amount int64; Currency string }
type PhoneNumber struct { Number string; CountryCode string }
```

## Bounded Contexts

### User & Authentication Context
**Purpose**: Manages user identity, authentication, and role-based access control.

**Key Aggregates**:
- `UserAggregate`: Complete user lifecycle management
- `AuthSessionAggregate`: Session and token management

**Key Business Rules**:
- Users must have unique email addresses
- Authentication sessions expire after configured time
- Role changes require appropriate permissions
- Users can only access resources within their tenant scope

### Workspace & Collaboration Context  
**Purpose**: Manages workspaces, teams, projects, and collaboration features.

**Key Aggregates**:
- `WorkspaceAggregate`: Workspace lifecycle and membership management
- `ProjectAggregate`: Project management within workspaces

**Key Business Rules**:
- Workspaces must belong to a tenant
- Users can be members of multiple workspaces with different roles
- Workspace owners can manage all aspects of their workspace
- Project visibility is controlled by workspace settings

### Tenant Management Context
**Purpose**: Manages tenant organizations, subscriptions, and resource quotas.

**Key Aggregates**:
- `TenantAggregate`: Complete tenant lifecycle with subscription management

**Key Business Rules**:
- Each tenant has resource quotas based on subscription plan
- Plan changes require payment method validation for paid plans
- Usage must not exceed plan limits
- Trial periods automatically expire and require action

## Domain Services

Domain services handle complex operations that span multiple aggregates:

### UserRegistrationService
Coordinates user registration across multiple bounded contexts:

```go
func (s *UserRegistrationService) RegisterUser(request UserRegistrationRequest) (*UserRegistrationResult, error) {
    // 1. Validate request
    // 2. Create or find tenant
    // 3. Create user aggregate
    // 4. Create default workspace (if new tenant)
    // 5. Save all aggregates in transaction
    // 6. Publish domain events
}
```

### TenantManagementService
Handles complex tenant operations:

```go
func (s *TenantManagementService) ChangeSubscription(request SubscriptionChangeRequest) error {
    // 1. Validate subscription change
    // 2. Check usage fits new plan limits
    // 3. Process billing changes
    // 4. Update tenant quotas
    // 5. Handle proration
}
```

## Key Patterns Implemented

### 1. Aggregate Root Pattern
```go
type UserAggregate struct {
    *shared.BaseAggregateRoot
    // ... domain fields
}

func (u *UserAggregate) SomeBusinessOperation() error {
    // Business logic
    u.RaiseEvent("UserSomethingHappened", payload)
    return nil
}
```

### 2. Domain Events Pattern
```go
// Events are raised within aggregates
u.RaiseEvent(shared.EventTypeUserActivated, "UserAggregate", payload)

// Events are published after successful persistence
for _, event := range aggregate.GetUncommittedEvents() {
    eventBus.Publish(event)
}
aggregate.ClearUncommittedEvents()
```

### 3. Specification Pattern
```go
type UniqueEmailSpecification struct {
    userRepo UserRepository
}

func (s *UniqueEmailSpecification) IsSatisfiedBy(user *UserAggregate) bool {
    existing, _ := s.userRepo.GetByEmail(user.GetEmail())
    return existing == nil
}
```

### 4. Repository Pattern
```go
type UserRepository interface {
    Save(aggregate *user.UserAggregate) error
    GetByID(id shared.UserID) (*user.UserAggregate, error)
    GetByEmail(email shared.Email) (*user.UserAggregate, error)
}
```

### 5. Value Object Pattern
```go
type Email struct {
    value string
}

func NewEmail(email string) (Email, error) {
    if !isValidEmail(email) {
        return Email{}, ErrInvalidEmail
    }
    return Email{value: email}, nil
}
```

## Event Sourcing Considerations

While not fully implemented, the domain layer is designed to support event sourcing:

```go
// Aggregates can be rebuilt from events
func (u *UserAggregate) LoadFromHistory(events []shared.DomainEvent) error {
    for _, event := range events {
        switch event.EventType() {
        case shared.EventTypeUserRegistered:
            u.applyUserRegisteredEvent(event)
        case shared.EventTypeUserActivated:
            u.applyUserActivatedEvent(event)
        }
    }
    return nil
}
```

## Integration with Infrastructure

The domain layer defines interfaces that infrastructure must implement:

```go
// Domain defines the interface
type UserRepository interface {
    Save(aggregate *user.UserAggregate) error
    GetByID(id shared.UserID) (*user.UserAggregate, error)
}

// Infrastructure provides the implementation
type PostgresUserRepository struct {
    db *sql.DB
}

func (r *PostgresUserRepository) Save(aggregate *user.UserAggregate) error {
    // Database-specific implementation
}
```

## Testing Strategy

### Unit Testing
Each aggregate and domain service should have comprehensive unit tests:

```go
func TestUserAggregate_Activate(t *testing.T) {
    // Arrange
    user := createTestUser()
    
    // Act
    err := user.Activate()
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, UserStatusActive, user.GetStatus())
    assert.Len(t, user.GetUncommittedEvents(), 1)
}
```

### Integration Testing
Test domain services with real repository implementations:

```go
func TestUserRegistrationService_RegisterUser(t *testing.T) {
    // Use test database
    service := NewUserRegistrationService(testUserRepo, testTenantRepo, testWorkspaceRepo, testEventBus)
    
    // Test the complete registration flow
    result, err := service.RegisterUser(testRequest)
    
    // Verify all aggregates were created correctly
}
```

## Best Practices

### 1. Keep Aggregates Small
- Each aggregate should have a single responsibility
- Avoid creating "god aggregates" that do everything

### 2. Use Domain Events for Integration
- Don't make direct calls between aggregates
- Use events for eventual consistency

### 3. Protect Invariants
- All business rules must be enforced within aggregates
- Don't allow invalid state transitions

### 4. Be Explicit About Language
- Use domain terminology consistently
- Avoid technical jargon in domain code

### 5. Design for Testability
- Make behavior explicit and testable
- Use dependency injection for external dependencies

## Future Enhancements

1. **Event Sourcing**: Full event sourcing implementation with snapshots
2. **CQRS**: Separate read and write models for better performance
3. **Saga Pattern**: Long-running business processes
4. **Domain Event Versioning**: Support for evolving event schemas
5. **Advanced Specifications**: More complex business rule validation

This domain layer provides a solid foundation for building a scalable, maintainable system that clearly expresses business intent and enforces business rules.