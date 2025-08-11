# Unit of Work Pattern Implementation

A comprehensive implementation of the Unit of Work pattern for PyAirtable Go services, designed to ensure transactional consistency across aggregate operations while integrating seamlessly with Domain-Driven Design (DDD) architecture.

## Overview

This implementation provides:

- **Transactional Consistency**: Ensures all aggregate changes are committed atomically
- **Event Collection**: Automatically collects and processes domain events from multiple aggregates
- **Optimistic Concurrency Control**: Prevents data corruption in concurrent scenarios
- **Outbox Pattern Integration**: Reliable event publishing with guaranteed delivery
- **Service Layer Integration**: Clean, fluent APIs for service layer usage
- **Comprehensive Validation**: Business rule and aggregate consistency validation
- **Metrics and Monitoring**: Built-in performance and health monitoring

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Service       │    │  Unit of Work   │    │   Repository    │
│   Layer         │───▶│     Core        │───▶│     Layer       │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                              │                        │
                              ▼                        ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│    Event        │    │  Concurrency    │    │   Event Store   │
│  Collector      │    │   Control       │    │   & Outbox      │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Core Components

### 1. Unit of Work Interface

```go
type UnitOfWork interface {
    // Transaction management
    Begin() error
    Commit() error
    Rollback() error
    IsActive() bool
    
    // Repository access
    UserRepository() UserRepository
    WorkspaceRepository() WorkspaceRepository
    TenantRepository() TenantRepository
    
    // Aggregate tracking and event management
    RegisterAggregate(aggregate shared.AggregateRoot) error
    GetTrackedAggregates() []shared.AggregateRoot
    CollectUncommittedEvents() []shared.DomainEvent
    
    // Validation and consistency
    ValidateAggregateConsistency() error
    ValidateBusinessRules() error
    
    // Event collector and concurrency control access
    GetEventCollector() *AggregateEventCollector
    GetConcurrencyManager() *ConcurrencyControlManager
}
```

### 2. Service Layer Integration

```go
type ServiceUnitOfWork interface {
    // High-level operations
    ExecuteInTransaction(ctx context.Context, operation func(ServiceContext) error) error
    ExecuteWithRetry(ctx context.Context, operation func(ServiceContext) error, maxRetries int) error
    
    // Aggregate operations
    SaveUser(user *user.UserAggregate) error
    SaveWorkspace(workspace *workspace.WorkspaceAggregate) error
    GetUser(userID shared.UserID) (*user.UserAggregate, error)
    
    // Batch operations
    SaveAggregates(aggregates ...shared.AggregateRoot) error
    LoadAggregates(ids ...shared.ID) ([]shared.AggregateRoot, error)
}
```

### 3. Event Collection System

The event collector automatically processes domain events with support for:

- **Event Handlers**: Process events as they're collected
- **Event Filters**: Control which events are processed
- **Event Transformers**: Modify events before storage
- **Event Interceptors**: Validate or block events

### 4. Optimistic Concurrency Control

Prevents data corruption through:

- **Version Tracking**: Automatic version management for aggregates
- **Conflict Detection**: Identifies concurrent modifications
- **Conflict Resolution**: Configurable strategies for handling conflicts
- **Lock Management**: Optimistic locking with timeout support

## Quick Start

### Basic Usage

```go
// Configure Unit of Work
config := UnitOfWorkConfig{
    TransactionTimeout: 30 * time.Second,
    IsolationLevel:     sql.LevelReadCommitted,
    EnableOutbox:       true,
    EnableEventStore:   true,
    MaxRetries:         3,
}

// Create Unit of Work
uow := NewSqlUnitOfWork(db, config)

// Execute transaction
err := uow.WithTransaction(func(uow UnitOfWork) error {
    // Create user aggregate
    userAgg, err := user.NewUserAggregate(userID, tenantID, email, password, firstName, lastName)
    if err != nil {
        return err
    }
    
    // Register for tracking
    if err := uow.RegisterAggregate(userAgg); err != nil {
        return err
    }
    
    // Save user
    return uow.UserRepository().Save(userAgg)
})
```

### Service Layer Usage

```go
// Create service-level Unit of Work
serviceUow := NewServiceUnitOfWork(db, config)

// Use fluent builder
builder := NewServiceBuilder(serviceUow).
    WithTimeout(60 * time.Second).
    WithRetries(3)

err := builder.Execute(func(ctx ServiceContext) error {
    // Create and save user
    userAgg, err := user.NewUserAggregate(...)
    if err != nil {
        return err
    }
    
    if err := ctx.Track(userAgg); err != nil {
        return err
    }
    
    return ctx.Users().Save(userAgg)
})
```

### Batch Operations

```go
batchService := NewBatchOperationService(serviceUow)

// Bulk update multiple users
updates := map[shared.UserID]func(*user.UserAggregate) error{
    userID1: func(user *user.UserAggregate) error {
        return user.UpdateProfile("New", "Name", "", "", "")
    },
    userID2: func(user *user.UserAggregate) error {
        return user.Activate()
    },
}

err := batchService.BulkUserUpdate(updates)
```

## Configuration

### Unit of Work Configuration

```go
type UnitOfWorkConfig struct {
    TransactionTimeout time.Duration    // Transaction timeout
    IsolationLevel     sql.IsolationLevel // SQL isolation level
    EnableOutbox       bool              // Enable outbox pattern
    EnableEventStore   bool              // Enable event store
    MaxRetries         int               // Maximum retry attempts
}
```

### Concurrency Control Configuration

```go
// Configure conflict resolution
fallbackResolver := NewRetryOnConflictResolver(3)
smartResolver := NewSmartConflictResolver(fallbackResolver)

// Register type-specific strategies
smartResolver.RegisterStrategy("*user.UserAggregate", NewRetryOnConflictResolver(5))
smartResolver.RegisterStrategy("*workspace.WorkspaceAggregate", NewAbortOnConflictResolver())
```

### Event Collection Configuration

```go
eventCollector := uow.GetEventCollector()

// Add event handlers
eventCollector.RegisterEventHandler(shared.EventTypeUserRegistered, &UserEventHandler{})
eventCollector.RegisterEventHandler("*", &LoggingEventHandler{})

// Add filters
eventCollector.RegisterEventFilter(NewTenantIsolationFilter(tenantID))
eventCollector.RegisterEventFilter(NewEventTypeFilter([]string{"user.*", "workspace.*"}))

// Add transformers
eventCollector.RegisterEventTransformer(NewMetadataEnrichmentTransformer(enrichmentFunc))
```

## Advanced Features

### Event Handling

```go
type CustomEventHandler struct{}

func (h *CustomEventHandler) HandleEvent(event shared.DomainEvent, aggregate shared.AggregateRoot) error {
    // Process event
    log.Printf("Processing event: %s", event.EventType())
    return nil
}

func (h *CustomEventHandler) CanHandle(eventType string) bool {
    return strings.HasPrefix(eventType, "user.")
}
```

### Custom Validation Rules

```go
type CustomBusinessRule struct{}

func (r *CustomBusinessRule) ValidateRule(aggregates map[string]shared.AggregateRoot) error {
    // Implement custom business logic
    return nil
}

// Register the rule
validator := uow.GetValidator()
validator.RegisterBusinessRule(&CustomBusinessRule{})
```

### Concurrency Conflict Resolution

```go
type CustomConflictResolver struct{}

func (r *CustomConflictResolver) ResolveConflict(
    aggregate shared.AggregateRoot, 
    expectedVersion, actualVersion shared.Version,
) (ResolveAction, error) {
    // Custom conflict resolution logic
    if actualVersion.Int() - expectedVersion.Int() == 1 {
        return ResolveActionRetry, nil
    }
    return ResolveActionAbort, nil
}
```

## Examples

The implementation includes comprehensive examples covering:

1. **Basic Unit of Work Usage** - Simple transaction management
2. **Service Layer Integration** - High-level service operations
3. **Optimistic Concurrency Control** - Conflict detection and resolution
4. **Event Collection and Processing** - Automatic event handling
5. **Batch Operations** - Bulk aggregate operations
6. **Error Handling and Retries** - Resilient operation patterns
7. **Complex Business Scenarios** - Multi-aggregate transactions
8. **Monitoring and Metrics** - Performance tracking

To run all examples:

```go
import "github.com/pyairtable-compose/go-services/pkg/repository"

err := repository.RunAllExamples(db)
if err != nil {
    log.Fatal(err)
}
```

## Monitoring and Metrics

### Service Metrics

```go
metrics := serviceUow.GetMetrics()
fmt.Printf("Success Rate: %.2f%%", 
    float64(metrics.SuccessfulTransactions) / float64(metrics.TotalTransactions) * 100)
```

### Event Collection Metrics

```go
eventCollector.LogMetrics()
// Output:
// Event Collector Metrics:
//   Total Events Collected: 150
//   Events Processed: 145
//   Events Filtered: 5
//   Events Transformed: 20
```

### Concurrency Control Metrics

```go
concurrencyManager.LogMetrics()
// Output:
// Concurrency Control Metrics:
//   Total Checks: 100
//   Conflicts Detected: 5
//   Conflicts Resolved: 4
//   Conflict Rate: 5.00%
```

## Integration with Existing Systems

### Event Store Integration

The Unit of Work seamlessly integrates with the existing PostgreSQL event store:

- Events are automatically saved to the `domain_events` table
- Optimistic concurrency control uses event versions
- Event replay is supported for aggregate reconstruction

### Outbox Pattern Integration

Events are reliably published using the transactional outbox pattern:

- Events are saved to the `outbox_events` table within the same transaction
- Background workers process outbox entries
- Retry logic handles temporary failures
- Duplicate detection prevents event reprocessing

### Repository Integration

Works with existing transactional repositories:

- Repositories operate within the Unit of Work transaction
- Aggregate tracking is automatic
- Business rule validation is enforced
- Version management is transparent

## Best Practices

### 1. Always Use Aggregate Registration

```go
// ✅ Good
if err := uow.RegisterAggregate(userAgg); err != nil {
    return err
}

// ❌ Bad - aggregate won't be tracked for events/validation
uow.UserRepository().Save(userAgg)
```

### 2. Handle Concurrency Conflicts Gracefully

```go
// ✅ Good - use retry logic
err := serviceUow.ExecuteWithRetry(ctx, operation, 3)

// ❌ Bad - conflicts will cause immediate failure
err := serviceUow.ExecuteInTransaction(ctx, operation)
```

### 3. Use Service Layer for Business Operations

```go
// ✅ Good - use service layer
err := userRegistrationService.RegisterUser(...)

// ❌ Bad - low-level Unit of Work for business logic
err := uow.WithTransaction(func(uow UnitOfWork) error {
    // Complex business logic here
})
```

### 4. Configure Appropriate Timeouts

```go
// ✅ Good - different timeouts for different scenarios
builder.WithTimeout(10 * time.Second)  // Simple operations
builder.WithTimeout(60 * time.Second)  // Batch operations
builder.WithTimeout(5 * time.Minute)   // Data migration
```

### 5. Monitor Performance

```go
// ✅ Good - regular metrics logging
go func() {
    ticker := time.NewTicker(1 * time.Minute)
    for range ticker.C {
        serviceUow.LogMetrics()
        eventCollector.LogMetrics()
        concurrencyManager.LogMetrics()
    }
}()
```

## Testing

### Unit Testing

```go
func TestUserRegistration(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()
    
    config := UnitOfWorkConfig{...}
    serviceUow := NewServiceUnitOfWork(db, config)
    
    err := serviceUow.SaveUser(userAgg)
    assert.NoError(t, err)
    
    // Verify user was saved
    savedUser, err := serviceUow.GetUser(userID)
    assert.NoError(t, err)
    assert.Equal(t, userAgg.GetEmail(), savedUser.GetEmail())
}
```

### Integration Testing

```go
func TestConcurrencyConflict(t *testing.T) {
    // Test concurrent modifications
    var wg sync.WaitGroup
    errors := make(chan error, 2)
    
    for i := 0; i < 2; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            err := serviceUow.ExecuteInTransaction(ctx, func(ctx ServiceContext) error {
                // Concurrent modification
                user, err := ctx.Users().GetByID(userID)
                if err != nil {
                    return err
                }
                return ctx.Users().Save(user)
            })
            errors <- err
        }()
    }
    
    wg.Wait()
    close(errors)
    
    // One should succeed, one should have concurrency conflict
    var successCount, conflictCount int
    for err := range errors {
        if err == nil {
            successCount++
        } else if strings.Contains(err.Error(), "concurrency conflict") {
            conflictCount++
        }
    }
    
    assert.Equal(t, 1, successCount)
    assert.Equal(t, 1, conflictCount)
}
```

## Performance Considerations

### Transaction Scope

- Keep transactions as short as possible
- Avoid long-running operations within transactions
- Use appropriate isolation levels
- Consider read-only operations outside transactions

### Batch Operations

- Use batch operations for bulk updates
- Configure longer timeouts for batch operations
- Monitor memory usage with large batches
- Consider pagination for very large datasets

### Event Processing

- Configure appropriate batch sizes for event processing
- Use filters to reduce unnecessary event processing
- Monitor event processing performance
- Consider async processing for non-critical events

### Concurrency Control

- Use optimistic locking judiciously
- Configure appropriate retry strategies
- Monitor conflict rates
- Consider pessimistic locking for high-conflict scenarios

## Troubleshooting

### Common Issues

1. **Transaction Timeout**
   ```
   Error: transaction timeout exceeded
   Solution: Increase TransactionTimeout or optimize operations
   ```

2. **Concurrency Conflicts**
   ```
   Error: optimistic concurrency conflict detected
   Solution: Implement retry logic or review conflict resolution strategy
   ```

3. **Memory Usage**
   ```
   Error: out of memory during batch operation
   Solution: Reduce batch size or use pagination
   ```

4. **Event Processing Delays**
   ```
   Error: events not processed timely
   Solution: Check outbox worker performance and configuration
   ```

### Debugging

Enable debug logging:

```go
import "log"

// Set debug level
log.SetFlags(log.LstdFlags | log.Lshortfile)

// Monitor Unit of Work operations
uow.GetEventCollector().LogMetrics()
uow.GetConcurrencyManager().LogMetrics()
```

## Contributing

When extending the Unit of Work implementation:

1. Follow existing patterns and interfaces
2. Add comprehensive tests
3. Update documentation
4. Consider backward compatibility
5. Add examples for new features

## Files Overview

- `unit_of_work.go` - Core Unit of Work implementation
- `aggregate_event_collector.go` - Event collection and processing system
- `optimistic_concurrency.go` - Concurrency control management
- `service_integration.go` - Service layer integration utilities
- `examples_and_usage.go` - Comprehensive usage examples
- `transactional_repositories.go` - Repository implementations
- `README.md` - This documentation

The Unit of Work pattern implementation provides a robust, scalable foundation for managing complex domain operations while maintaining data consistency and reliability in the PyAirtable system.