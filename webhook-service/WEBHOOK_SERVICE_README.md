# PyAirtable Webhook Service

A production-ready webhook management system that provides reliable delivery, retry logic, dead letter queues, and comprehensive monitoring for PyAirtable platform events.

## Features

### Core Functionality
- **Webhook Registration & Management**: Create, update, delete, and manage webhook endpoints
- **Event Subscription System**: Subscribe to specific PyAirtable events with advanced filtering
- **Secure Delivery**: HMAC-SHA256 signature validation for payload integrity
- **Reliable Delivery**: At-least-once delivery semantics with 99.9% delivery guarantee
- **Retry Logic**: Exponential backoff retry mechanism with configurable max attempts
- **Dead Letter Queue**: Failed deliveries moved to DLQ with replay capability

### Reliability & Performance
- **Circuit Breaker Pattern**: Prevents cascade failures for consistently failing endpoints
- **Rate Limiting**: Per-webhook endpoint rate limiting (configurable RPS)
- **Idempotency**: Idempotency keys prevent duplicate deliveries
- **Connection Pooling**: Optimized HTTP client with connection reuse
- **Timeout Management**: Configurable per-webhook timeout settings

### Monitoring & Analytics
- **Delivery Metrics**: Success rates, latency, failure reasons
- **Prometheus Integration**: Comprehensive metrics collection
- **Health Monitoring**: Per-webhook health status tracking
- **Delivery Logs**: Detailed logging of all delivery attempts
- **Admin Dashboard**: Management interface for webhook operations

### Event Types Supported
- `table.created` - When a new table is created
- `table.updated` - When a table is modified
- `table.deleted` - When a table is deleted
- `record.created` - When a new record is added
- `record.updated` - When a record is modified
- `record.deleted` - When a record is deleted
- `workspace.created` - When a new workspace is created
- `workspace.updated` - When a workspace is modified
- `user.invited` - When a user is invited to a workspace
- `user.joined` - When a user joins a workspace

## API Endpoints

### Webhook Management
```
POST   /api/v1/webhooks                    # Create webhook
GET    /api/v1/webhooks                    # List webhooks
GET    /api/v1/webhooks/:id                # Get webhook details
PUT    /api/v1/webhooks/:id                # Update webhook
DELETE /api/v1/webhooks/:id                # Delete webhook
GET    /api/v1/webhooks/:id/deliveries     # Get delivery history
GET    /api/v1/webhooks/:id/metrics        # Get webhook metrics
POST   /api/v1/webhooks/:id/test           # Test webhook delivery
POST   /api/v1/webhooks/:id/regenerate-secret # Regenerate HMAC secret
```

### Subscription Management
```
POST   /api/v1/webhooks/:webhook_id/subscriptions     # Create subscription
GET    /api/v1/webhooks/:webhook_id/subscriptions     # List subscriptions
GET    /api/v1/webhooks/:webhook_id/subscriptions/:id # Get subscription
PUT    /api/v1/webhooks/:webhook_id/subscriptions/:id # Update subscription
DELETE /api/v1/webhooks/:webhook_id/subscriptions/:id # Delete subscription
```

### Dead Letter Queue
```
GET    /api/v1/webhooks/:webhook_id/dead-letters           # List dead letters
POST   /api/v1/webhooks/:webhook_id/dead-letters/:id/replay # Replay dead letter
POST   /api/v1/webhooks/:webhook_id/dead-letters/bulk-replay # Bulk replay
GET    /api/v1/dead-letters                                # List all dead letters
```

### System Information
```
GET    /api/v1/event-types    # List available event types
GET    /health                # Health check
GET    /ready                 # Readiness check
GET    /metrics               # Prometheus metrics
```

## Webhook Payload Format

All webhook deliveries include standardized headers and payload structure:

### Headers
```
Content-Type: application/json
User-Agent: PyAirtable-Webhooks/1.0
X-PyAirtable-Event-ID: unique-event-id
X-PyAirtable-Event-Type: record.created
X-PyAirtable-Signature: sha256=<hmac-signature>
X-PyAirtable-Timestamp: 1234567890
X-PyAirtable-Webhook-ID: 123
```

### Payload Structure
```json
{
  "id": "evt_1234567890",
  "type": "record.created",
  "resource_id": "rec123",
  "workspace_id": 456,
  "user_id": 789,
  "timestamp": "2024-01-01T12:00:00Z",
  "data": {
    "id": "rec123",
    "fields": {
      "name": "John Doe",
      "email": "john@example.com"
    }
  },
  "metadata": {
    "source": "api",
    "version": "v1"
  }
}
```

## HMAC Signature Verification

Webhook payloads are signed with HMAC-SHA256 for security:

```go
// Go example
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
)

func verifySignature(secret, payload, signature string, timestamp int64) bool {
    signedPayload := fmt.Sprintf("%d.%s", timestamp, payload)
    h := hmac.New(sha256.New, []byte(secret))
    h.Write([]byte(signedPayload))
    expectedSignature := "sha256=" + hex.EncodeToString(h.Sum(nil))
    return hmac.Equal([]byte(signature), []byte(expectedSignature))
}
```

```python
# Python example
import hmac
import hashlib

def verify_signature(secret, payload, signature, timestamp):
    signed_payload = f"{timestamp}.{payload}"
    expected_signature = "sha256=" + hmac.new(
        secret.encode(),
        signed_payload.encode(),
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(signature, expected_signature)
```

## Event Filtering

Subscriptions support advanced filtering capabilities:

### Field-based Filtering
```json
{
  "event_type": "record.updated",
  "filters": {
    "fields": {
      "status": "completed",
      "priority": "high"
    }
  }
}
```

### Conditional Filtering
```json
{
  "event_type": "record.created",
  "filters": {
    "conditions": {
      "operator": "and",
      "rules": [
        {
          "field": "amount",
          "operator": "greater_than",
          "value": 1000
        },
        {
          "field": "status",
          "operator": "equals",
          "value": "pending"
        }
      ]
    }
  }
}
```

### User ID Filtering
```json
{
  "event_type": "table.created",
  "filters": {
    "user_id": [123, 456, 789]
  }
}
```

### Time Range Filtering
```json
{
  "event_type": "record.updated",
  "filters": {
    "time_range": {
      "start": "2024-01-01T00:00:00Z",
      "end": "2024-01-31T23:59:59Z"
    }
  }
}
```

## Configuration

### Environment Variables
```bash
# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=password
DB_NAME=webhook_service_db

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

# Server
PORT=8086
HOST=0.0.0.0

# JWT
JWT_SECRET=your-jwt-secret
JWT_TTL=3600

# CORS
CORS_ALLOWED_ORIGINS=*

# Logging
LOG_LEVEL=info
```

### Webhook Configuration
```json
{
  "name": "My Webhook",
  "url": "https://myapp.com/webhook",
  "description": "Webhook for record updates",
  "max_retries": 3,
  "timeout_seconds": 30,
  "rate_limit_rps": 10,
  "headers": {
    "Authorization": "Bearer token",
    "X-Custom-Header": "value"
  },
  "metadata": {
    "environment": "production",
    "version": "1.0"
  }
}
```

## Database Schema

The service uses PostgreSQL with the following key tables:

### Webhooks
- `id` - Primary key
- `user_id` - Owner user ID
- `workspace_id` - Optional workspace scope
- `name` - Webhook name
- `url` - Endpoint URL
- `secret` - HMAC secret (encrypted)
- `status` - active/inactive/paused/failed
- `max_retries` - Retry attempts
- `timeout_seconds` - Request timeout
- `rate_limit_rps` - Rate limit
- Statistics and metadata fields

### Webhook Subscriptions
- `id` - Primary key
- `webhook_id` - Foreign key to webhooks
- `event_type` - Event type to subscribe to
- `resource_id` - Optional resource filter
- `is_active` - Active status
- `filters` - JSON filters

### Delivery Attempts
- `id` - Primary key
- `webhook_id` - Foreign key to webhooks
- `event_id` - Event identifier
- `payload` - Event payload
- `status` - pending/delivered/failed/retrying/abandoned
- `attempt_number` - Retry attempt number
- Response details and timing

### Dead Letters
- `id` - Primary key
- `webhook_id` - Foreign key to webhooks
- `event_id` - Failed event ID
- `payload` - Original payload
- `failure_reason` - Why it failed
- `attempt_count` - Total attempts made
- `is_processed` - Replay status

## Deployment

### Docker
```bash
# Build
docker build -t webhook-service .

# Run
docker run -d \
  --name webhook-service \
  -p 8086:8086 \
  -e DB_HOST=postgres \
  -e REDIS_HOST=redis \
  webhook-service
```

### Kubernetes
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: webhook-service
spec:
  replicas: 3
  selector:
    matchLabels:
      app: webhook-service
  template:
    metadata:
      labels:
        app: webhook-service
    spec:
      containers:
      - name: webhook-service
        image: webhook-service:latest
        ports:
        - containerPort: 8086
        env:
        - name: DB_HOST
          value: "postgres-service"
        - name: REDIS_HOST
          value: "redis-service"
        livenessProbe:
          httpGet:
            path: /health
            port: 8086
        readinessProbe:
          httpGet:
            path: /ready
            port: 8086
```

## Monitoring

The service exposes Prometheus metrics:

### Key Metrics
- `webhook_delivery_attempts_total` - Total delivery attempts
- `webhook_delivery_successes_total` - Successful deliveries
- `webhook_delivery_failures_total` - Failed deliveries
- `webhook_delivery_duration_seconds` - Delivery latency
- `webhook_active_total` - Active webhook count
- `webhook_dead_letters_total` - Dead letter count
- `webhook_circuit_breaker_state` - Circuit breaker states

### Grafana Dashboard
A pre-built Grafana dashboard is available at `monitoring/grafana/dashboards/webhook-service.json`

## Security Considerations

1. **HMAC Signatures**: All webhooks are signed with HMAC-SHA256
2. **URL Validation**: Webhook URLs are validated to prevent SSRF attacks
3. **Header Sanitization**: Custom headers are sanitized and validated
4. **Rate Limiting**: Built-in rate limiting prevents abuse
5. **Secret Management**: Webhook secrets are never exposed in API responses
6. **Private Network Protection**: Localhost and private IP ranges are blocked

## Performance Characteristics

### Throughput
- **Target**: 10,000 webhooks/second
- **Latency**: P95 < 100ms for delivery initiation
- **Retry Processing**: 1,000 retries/second

### Reliability
- **Uptime**: 99.9% availability target
- **Delivery Success**: 99.9% delivery success rate
- **Data Durability**: All events persisted before acknowledgment

### Scalability
- **Horizontal Scaling**: Stateless design supports multiple replicas
- **Database Sharding**: Ready for database partitioning
- **Queue Processing**: Distributed retry processing

## Troubleshooting

### Common Issues

**Webhook Not Receiving Events**
1. Check webhook status is "active"
2. Verify subscriptions are configured and active
3. Check event type matches subscription
4. Validate filters aren't excluding events

**Delivery Failures**
1. Check webhook URL is accessible
2. Verify HTTPS certificate is valid
3. Check response status codes
4. Review timeout settings

**High Retry Rate**
1. Monitor webhook endpoint performance
2. Check circuit breaker status
3. Review rate limiting settings
4. Validate payload size limits

### Debug Endpoints
```bash
# Check webhook status
curl -H "X-User-ID: 123" http://localhost:8086/api/v1/webhooks/1

# View delivery attempts
curl -H "X-User-ID: 123" http://localhost:8086/api/v1/webhooks/1/deliveries

# Check metrics
curl -H "X-User-ID: 123" http://localhost:8086/api/v1/webhooks/1/metrics

# Test webhook
curl -X POST -H "X-User-ID: 123" -H "Content-Type: application/json" \
  -d '{"test": true}' http://localhost:8086/api/v1/webhooks/1/test
```

## Development

### Running Locally
```bash
# Install dependencies
go mod download

# Run migrations
psql -h localhost -U postgres -d webhook_service_db -f migrations/001_create_webhook_tables.sql

# Start service
go run cmd/webhook-service/main.go
```

### Testing
```bash
# Unit tests
go test ./internal/...

# Integration tests
go test ./test/integration/...

# Load tests
go test -bench=. ./test/benchmarks/...
```

### Code Structure
```
├── cmd/webhook-service/          # Application entry point
├── internal/
│   ├── config/                   # Configuration management
│   ├── handlers/                 # HTTP handlers
│   ├── middleware/               # HTTP middleware
│   ├── models/                   # Data models
│   ├── repositories/             # Data access layer
│   └── services/                 # Business logic
├── pkg/                          # Shared packages
├── migrations/                   # Database migrations
├── test/                         # Test files
└── deployments/                  # Deployment configurations
```

## License

This webhook service is part of the PyAirtable platform and is proprietary software.

## Support

For issues and questions:
- GitHub Issues: [Repository Issues](https://github.com/your-org/pyairtable-webhook-service/issues)
- Documentation: [Wiki](https://github.com/your-org/pyairtable-webhook-service/wiki)
- Slack: #webhook-service channel