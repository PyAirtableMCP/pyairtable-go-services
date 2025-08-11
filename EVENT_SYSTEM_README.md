# DDD Event System - PyAirtable

This document explains how to use the Domain-Driven Design (DDD) event system that has been activated in the PyAirtable Go services.

## Overview

The DDD event system provides:
- **Event Sourcing**: All changes to aggregates are stored as events
- **Event Store**: PostgreSQL-based persistent event storage
- **Event Bus**: In-memory event publishing and subscription
- **Projections**: Read models automatically updated from events
- **Event Processing**: Automatic processing of unprocessed events

## Architecture

```
User Action -> Domain Aggregate -> Domain Events -> Event Store
                                                       ↓
Event Processor -> Event Bus -> Event Handlers -> Read Model Projections
```

## Components

### 1. Domain Aggregates
- **UserAggregate**: User registration, activation, profile updates
- **WorkspaceAggregate**: Workspace creation, member management
- **TenantAggregate**: Tenant management, subscription handling

### 2. Event Infrastructure
- **PostgresEventStore**: Stores events with optimistic concurrency control
- **InMemoryEventBus**: Publishes events to registered handlers
- **EventProcessor**: Processes unprocessed events in batches
- **UserProjectionHandler**: Updates user read models from events

### 3. Database Tables

#### Event Storage
- `domain_events`: Main event store table
- `event_store`: Alternative event storage (legacy)
- `saga_instances` & `saga_steps`: SAGA orchestration support

#### Read Model Projections
- `user_projections`: Optimized user read model
- `workspace_projections`: Workspace read model
- `workspace_member_projections`: Workspace membership
- `tenant_projections`: Tenant information

## Quick Start

### 1. Run Database Migrations

```bash
# Apply event store migrations
psql -d pyairtable -f /go-services/migrations/001_create_event_store.sql
psql -d pyairtable -f /go-services/migrations/002_create_user_projections.sql
```

### 2. Start the User Service

```bash
cd /go-services/user-service
go run cmd/user-service/main.go
```

### 3. Test the Event Flow

#### Demo the Complete Event Flow
```bash
curl -X POST http://localhost:8080/events/demo
```

This will:
1. Create a new user (raises `user.registered` event)
2. Activate the user (raises `user.activated` event)
3. Update user profile (raises `user.updated` event)
4. Show event sourcing by reconstructing user from events

#### Check Event Statistics
```bash
curl http://localhost:8080/events/stats
```

#### View Recent Events
```bash
curl http://localhost:8080/events/list
```

#### View User Read Model Projections
```bash
curl http://localhost:8080/events/projections/users
```

#### Health Check
```bash
curl http://localhost:8080/events/health
```

## Event Types

### User Events
- `user.registered`: User account created
- `user.activated`: User account activated
- `user.deactivated`: User account deactivated
- `user.updated`: User profile updated
- `user.role_changed`: User role added/removed

### Workspace Events
- `workspace.created`: Workspace created
- `workspace.updated`: Workspace updated
- `workspace.deactivated`: Workspace archived
- `workspace.member_added`: Member added to workspace
- `workspace.member_removed`: Member removed from workspace
- `workspace.member_role_changed`: Member role changed

### Tenant Events
- `tenant.created`: Tenant created
- `tenant.updated`: Tenant information updated
- `tenant.plan_changed`: Subscription plan changed
- `tenant.deactivated`: Tenant suspended/deactivated

## Usage Examples

### 1. Creating a User with Events

```go
// Create user registration request
email, _ := shared.NewEmail("user@example.com")
request := services.UserRegistrationRequest{
    Email:       email,
    Password:    "secure_password",
    FirstName:   "John",
    LastName:    "Doe",
    CompanyName: "Example Corp",
}

// Register user (this creates and persists events)
result, err := userService.RegisterUserWithEvents(request)
if err != nil {
    return err
}

// Events created:
// 1. tenant.created (if new tenant)
// 2. user.registered
// 3. workspace.created (if new tenant)
```

### 2. Retrieving User from Events

```go
// Get user by reconstructing from events
userID := shared.NewUserIDFromString("user-uuid")
user, err := userService.GetUserByIDFromEvents(userID)
if err != nil {
    return err
}

// The user aggregate is rebuilt from all its events
```

### 3. Updating User Profile

```go
// Update user profile (creates events)
err := userService.UpdateUserProfileWithEvents(
    userID,
    "Jane",           // firstName
    "Smith",          // lastName
    "avatar.jpg",     // avatar
    "UTC",           // timezone
    "en",            // language
)

// Events created:
// 1. user.updated
```

## Event Processing

### Automatic Processing
The event processor runs automatically and:
- Polls for unprocessed events every 5 seconds
- Processes events in batches of 100
- Publishes events to the event bus
- Updates read model projections
- Marks events as processed

### Manual Replay
You can replay events for specific types:

```bash
# Replay all user.registered events from the last 24 hours
curl -X POST "http://localhost:8080/events/replay?type=user.registered&hours=24"
```

## Read Model Projections

The system automatically maintains optimized read models:

### User Projections
- Optimized for user queries
- Includes computed fields like `display_name`
- Indexed for fast lookups by email, tenant, status
- Updated automatically from user events

### Performance Benefits
- Fast queries without event replay
- Optimized indexes for common query patterns
- Eventual consistency model
- Horizontal scaling support

## Monitoring

### Event Statistics
- Total events: All events in the system
- Processed events: Successfully processed events
- Unprocessed events: Events waiting for processing
- Failed events: Events that failed processing

### Event Handler Tracking
- Last processed event ID per handler
- Error counts and last error details
- Processing timestamps

## Configuration

### Event Processor Settings
```go
processor.SetBatchSize(100)         // Events per batch
processor.SetPollInterval(5 * time.Second)  // Polling frequency
processor.SetMaxRetries(3)          // Max retry attempts
```

### Event Bus Settings
- In-memory event bus for single instance
- Can be replaced with distributed event bus (Redis, Kafka, etc.)

## Best Practices

### 1. Event Design
- Events are immutable facts about what happened
- Use past tense for event names (e.g., `UserRegistered`)
- Include all necessary data in event payload
- Keep events focused and atomic

### 2. Aggregate Design
- Aggregates should be transaction boundaries
- Validate business rules before raising events
- Use optimistic concurrency control
- Keep aggregates small and focused

### 3. Projection Design
- Design projections for specific query patterns
- Use event versioning for breaking changes
- Handle out-of-order events gracefully
- Monitor projection lag

### 4. Error Handling
- Implement idempotent event handlers
- Use retry mechanisms with exponential backoff
- Implement dead letter queues for failed events
- Monitor event processing health

## Troubleshooting

### Common Issues

1. **Events not processing**: Check event processor health
2. **Projection lag**: Monitor event processing statistics
3. **Concurrency conflicts**: Review aggregate version handling
4. **Handler errors**: Check event handler logs

### Debugging Tools

1. **Event List**: View recent events and payloads
2. **Event Stats**: Monitor processing health
3. **Projection Viewer**: Compare events vs. projections
4. **Event Replay**: Reprocess events for debugging

## Extension Points

### Adding New Event Types
1. Define event constants in `shared/events.go`
2. Raise events from aggregates
3. Create event handlers
4. Subscribe handlers to event bus

### Custom Projections
1. Create projection table
2. Implement event handler
3. Subscribe to relevant events
4. Handle event replay

### Distributed Event Bus
Replace `InMemoryEventBus` with:
- Redis Pub/Sub
- Apache Kafka
- RabbitMQ
- AWS EventBridge

## Next Steps

1. **Add More Aggregates**: Implement remaining domain aggregates
2. **Distributed Events**: Replace in-memory event bus
3. **SAGA Support**: Implement distributed transactions
4. **Event Streaming**: Add real-time event streaming
5. **Event Replay UI**: Build web interface for event management

## API Documentation

### Event Demo Endpoints

- `GET /events/health` - Health check
- `POST /events/demo` - Run complete event flow demonstration
- `GET /events/stats` - Get event processing statistics
- `GET /events/list` - List recent events
- `GET /events/projections/users` - View user projections
- `POST /events/replay?type={eventType}&hours={hours}` - Replay events

This event system provides a solid foundation for building scalable, maintainable microservices with strong consistency guarantees and excellent auditability.