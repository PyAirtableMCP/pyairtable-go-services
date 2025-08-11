# PyAirtable Outbox Pattern Implementation

This package implements the **Transactional Outbox Pattern** for PyAirtable's Go services, ensuring reliable message delivery and event publishing in a distributed microservices architecture.

## Overview

The outbox pattern solves the dual-write problem by storing domain events in the same database transaction as the business data changes, then asynchronously publishing these events to external systems. This guarantees consistency between business state changes and event publishing.

## Key Features

### 🚀 Core Functionality
- **Transactional Safety**: Events are stored in the same transaction as business data
- **Reliable Delivery**: Automatic retry logic with exponential backoff
- **Multiple Publishers**: Support for Kafka, Redis Streams, Webhooks, and Event Bus
- **Circuit Breaker**: Automatic failure handling and recovery
- **Dead Letter Queue**: Failed events after max retries are preserved for investigation

### 📊 Monitoring & Observability
- **Prometheus Metrics**: Comprehensive metrics for monitoring performance
- **Health Checks**: Automated health monitoring for all components
- **Grafana Dashboard**: Ready-to-use monitoring dashboard
- **Alerting Rules**: Pre-configured Prometheus alerts
- **Performance Tracking**: Trend analysis and SLA monitoring

### ⚡ Performance & Scalability
- **Worker Pool**: Configurable number of background workers
- **Batch Processing**: Efficient batch processing of events
- **Load Balancing**: Automatic load distribution among workers
- **Connection Pooling**: Efficient database connection management
- **Cleanup Jobs**: Automatic cleanup of processed events

### 🔧 Configuration & Integration
- **Environment-based Config**: Easy configuration via environment variables
- **Service-specific Setup**: Pre-configured setups for each service
- **Repository Integration**: Seamless integration with existing repositories
- **Event Store Compatibility**: Works with existing event sourcing infrastructure

## Architecture

```mermaid
graph TB
    A[Business Logic] --> B[Repository]
    B --> C[Outbox Repository Wrapper]
    C --> D[Database Transaction]
    D --> E[Business Data]
    D --> F[Outbox Table]
    
    G[Background Workers] --> F
    G --> H[Event Publishers]
    H --> I[Kafka]
    H --> J[Redis Streams]
    H --> K[Webhooks]
    H --> L[Event Bus]
    
    M[Monitoring] --> G
    M --> N[Prometheus Metrics]
    M --> O[Health Checks]
    M --> P[Grafana Dashboard]
```

## Quick Start

### 1. Database Setup

Run the migration to create outbox tables:

```bash
# Apply the outbox migration
psql -d your_database -f migrations/004_create_outbox_tables.sql
```

### 2. Service Integration

```go
package main

import (
    "database/sql"
    "log"
    
    "github.com/pyairtable-compose/go-services/pkg/outbox"
)

func main() {
    // Connect to database
    db, err := sql.Open("postgres", "your_connection_string")
    if err != nil {
        log.Fatal(err)
    }
    
    // Setup outbox for user service
    outboxService, err := outbox.SetupForUserService(db)
    if err != nil {
        log.Fatal(err)
    }
    
    // Start outbox processing
    if err := outboxService.Start(); err != nil {
        log.Fatal(err)
    }
    defer outboxService.Stop()
    
    // Your service is now ready with outbox pattern!
}
```

### 3. Repository Integration

```go
// Wrap your existing repository with outbox functionality
baseRepo := &YourUserRepository{db: db}
outboxEnabledRepo := outbox.NewOutboxEnabledAggregateRepository(
    baseRepo, 
    outboxService.Wrapper,
)

// Now saving aggregates automatically publishes events
err := outboxEnabledRepo.Save(ctx, userAggregate)
```

### 4. Manual Event Publishing

```go
// For custom operations that produce events
operation := outbox.EventProducingOperation{
    Operation: func(tx *sql.Tx) error {
        // Your business logic here
        return updateDatabase(tx, data)
    },
    Events: []shared.DomainEvent{
        userRegisteredEvent,
        welcomeEmailEvent,
    },
}

err := outboxService.Wrapper.ExecuteWithOutbox(ctx, operation)
```

## Configuration

### Environment Variables

```bash
# Database
DATABASE_URL=postgres://user:pass@localhost:5432/pyairtable

# Outbox Settings
OUTBOX_BATCH_SIZE=50
OUTBOX_POLL_INTERVAL=5s
OUTBOX_MAX_RETRIES=3
OUTBOX_WORKER_COUNT=2

# Publisher Configuration
OUTBOX_PUBLISHER_TYPE=kafka  # kafka, redis_stream, webhook, event_bus, composite

# Monitoring
OUTBOX_ENABLE_METRICS=true
OUTBOX_RETENTION_DAYS=7

# Service Identity
SERVICE_NAME=user-service
```

### Configuration Object

```go
config := outbox.OutboxServiceConfig{
    ServiceName:         "user-service",
    DatabaseURL:         "postgres://...",
    PublisherType:       "kafka",
    BatchSize:          50,
    WorkerCount:        2,
    EnableMetrics:      true,
    RetentionDays:      7,
    // ... other settings
}
```

## Publishers

### Kafka Publisher

```go
config.PublisherType = "kafka"
config.PublisherConfig = map[string]interface{}{
    "brokers": []string{"localhost:9092"},
    "topic":   "pyairtable-events",
}
```

### Redis Streams Publisher

```go
config.PublisherType = "redis_stream"
config.PublisherConfig = map[string]interface{}{
    "redis_url": "redis://localhost:6379",
    "stream":    "pyairtable:events",
}
```

### Webhook Publisher

```go
config.PublisherType = "webhook"
config.PublisherConfig = map[string]interface{}{
    "url": "https://api.example.com/webhooks/events",
    "headers": map[string]string{
        "Authorization": "Bearer token",
    },
}
```

### Composite Publisher (Multiple Targets)

```go
config.PublisherType = "composite"
config.PublisherConfig = map[string]interface{}{
    "strategy": "primary", // all, any, primary
    "kafka": map[string]interface{}{
        "brokers": []string{"localhost:9092"},
        "topic":   "pyairtable-events",
    },
}
```

## Monitoring

### Metrics

The outbox pattern exposes comprehensive Prometheus metrics:

- `pyairtable_outbox_events_in_queue` - Number of events by status
- `pyairtable_outbox_events_processed_total` - Total processed events
- `pyairtable_outbox_event_processing_duration_seconds` - Processing latency
- `pyairtable_outbox_worker_health` - Worker health status
- `pyairtable_outbox_sla_compliance` - SLA compliance percentage

### Grafana Dashboard

Import the provided Grafana dashboard:

```bash
# Import dashboard
curl -X POST http://grafana:3000/api/dashboards/db \
  -H "Content-Type: application/json" \
  -d @monitoring/grafana/dashboards/outbox/outbox-monitoring.json
```

### Alerts

Configure Prometheus alerts:

```yaml
# Add to prometheus.yml
rule_files:
  - "rules/outbox-alerts.yml"
```

### Health Checks

```go
// Check overall health
status := outboxService.GetStatus()

// Check specific component health
healthStatus := outboxService.Outbox.GetProcessorStatus()
```

## Database Schema

### Outbox Tables

```sql
-- Main outbox table
event_outbox (
    id, aggregate_id, aggregate_type, event_type, event_version,
    payload, metadata, status, created_at, processed_at,
    retry_count, max_retries, next_retry_at, error_message,
    tenant_id, correlation_id, causation_id
)

-- Processor tracking
outbox_processors (
    id, processor_name, processor_type, configuration,
    is_active, heartbeat_at, last_processed_at, metrics
)

-- Delivery tracking
outbox_delivery_log (
    id, event_id, processor_id, attempt_number, status,
    started_at, completed_at, duration_ms, error_message
)

-- Performance metrics
outbox_metrics (
    id, processor_id, metric_type, time_window,
    value, unit, metadata
)
```

### Indexes

Optimized indexes for performance:

- `idx_outbox_status` - Query pending events
- `idx_outbox_pending_events` - Partial index for pending events
- `idx_outbox_retry_ready` - Failed events ready for retry
- `idx_outbox_aggregate` - Query by aggregate
- `idx_outbox_tenant_id` - Multi-tenant queries

## Advanced Usage

### Batch Operations

```go
// Process multiple operations in a single transaction
operations := []outbox.EventProducingOperation{
    {
        Operation: func(tx *sql.Tx) error {
            return createUser(tx, user1)
        },
        Events: []shared.DomainEvent{userCreatedEvent1},
    },
    {
        Operation: func(tx *sql.Tx) error {
            return createUser(tx, user2)
        },
        Events: []shared.DomainEvent{userCreatedEvent2},
    },
}

batchOp := outbox.CombineOperations(operations...)
err := wrapper.ExecuteBatch(ctx, batchOp)
```

### SAGA Integration

```go
sagaManager := outbox.NewOutboxSagaManager(wrapper)

err := sagaManager.ExecuteSagaStep(
    ctx,
    sagaID,
    func(tx *sql.Tx) error {
        // SAGA step logic
        return executePayment(tx, paymentData)
    },
    compensationEvents,
    resultEvents,
)
```

### Circuit Breaker Configuration

```go
config.CircuitBreakerEnabled = true
config.CircuitBreakerMaxFailures = 5
config.CircuitBreakerResetTimeout = 60 * time.Second
```

### Custom Publishers

```go
type CustomPublisher struct {
    // Your implementation
}

func (p *CustomPublisher) Publish(ctx context.Context, entry *outbox.OutboxEntry) error {
    // Custom publishing logic
    return nil
}

func (p *CustomPublisher) GetName() string {
    return "custom_publisher"
}
```

## Best Practices

### 1. Event Design

- Keep events immutable and self-contained
- Include all necessary data in the event payload
- Use correlation IDs for tracing
- Version your event schemas

### 2. Performance Optimization

- Configure appropriate batch sizes (50-100 events)
- Use multiple workers for high throughput
- Monitor queue depth and processing latency
- Set up proper database connection pooling

### 3. Error Handling

- Configure retry policies based on your SLA
- Monitor dead letter queue for failed events
- Set up alerts for high error rates
- Implement idempotent event handlers

### 4. Monitoring

- Monitor all key metrics continuously
- Set up appropriate alerting thresholds
- Use distributed tracing for debugging
- Regular health checks and capacity planning

### 5. Testing

```go
// Test outbox integration
func TestUserRegistrationWithOutbox(t *testing.T) {
    // Setup test database and outbox
    db := setupTestDB(t)
    outboxService := setupTestOutbox(t, db)
    
    // Execute operation
    user, err := userService.RegisterUser(ctx, "test@example.com", "Test User", "tenant-1")
    require.NoError(t, err)
    
    // Verify event in outbox
    events := getOutboxEvents(t, db, user.ID)
    assert.Len(t, events, 1)
    assert.Equal(t, "user.registered", events[0].EventType)
}
```

## Troubleshooting

### Common Issues

1. **High Queue Depth**
   - Increase worker count
   - Check publisher health
   - Verify database performance

2. **Processing Errors**
   - Check publisher configuration
   - Verify network connectivity
   - Review error logs

3. **Performance Issues**
   - Monitor database indexes
   - Check connection pool settings
   - Review batch sizes

### Debug Commands

```go
// Get detailed status
status := outboxService.GetStatus()

// Check worker health
workers := outboxService.WorkerManager.GetWorkerStats()

// Monitor queue depth
stats := outboxService.Outbox.GetStats()
```

## Migration Guide

### From Direct Publishing

1. Install outbox tables
2. Wrap repositories with outbox
3. Update event publishing code
4. Monitor and validate

### Performance Considerations

- Database I/O increases due to outbox writes
- Background processing adds CPU/memory overhead
- Network traffic to publishers
- Storage requirements for event retention

## Contributing

See the main PyAirtable contributing guidelines. For outbox-specific changes:

1. Update tests for any new functionality
2. Update monitoring dashboards and alerts
3. Update documentation and examples
4. Consider backward compatibility

## License

This implementation is part of PyAirtable and follows the same license terms.