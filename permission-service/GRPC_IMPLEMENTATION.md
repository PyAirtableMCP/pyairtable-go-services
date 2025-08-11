# Permission Service gRPC Implementation

This document describes the gRPC implementation for the Permission Service, which serves as a reference implementation for other services in the pyairtable ecosystem.

## Overview

The Permission Service now supports both REST and gRPC protocols, providing high-performance permission checking and management capabilities. This implementation includes:

- Dual protocol support (REST + gRPC)
- Connection pooling and retry logic
- Circuit breaker pattern
- Comprehensive observability
- Performance benchmarking
- Client libraries for Go and Python

## Architecture

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│             │    │             │    │             │
│   Clients   │◄──►│  Load       │◄──►│  Permission │
│             │    │  Balancer   │    │  Service    │
│             │    │             │    │             │
└─────────────┘    └─────────────┘    └─────────────┘
      │                                       │
      │            gRPC (50051)              │
      │            REST (8085)               │
      │            Metrics (9090)            │
      │                                       │
      ▼                                       ▼
┌─────────────┐                    ┌─────────────┐
│             │                    │             │
│ Observability│                    │  Database   │
│   Stack     │                    │   & Cache   │
│             │                    │             │
└─────────────┘                    └─────────────┘
```

## Features

### 1. Dual Protocol Support

The service runs both REST and gRPC servers simultaneously:

- **REST API**: Port 8085 (existing functionality)
- **gRPC API**: Port 50051 (new high-performance interface)
- **Metrics**: Port 9090 (Prometheus metrics)

### 2. gRPC Service Methods

```protobuf
service PermissionService {
  // Core permission operations
  rpc CheckPermission(CheckPermissionRequest) returns (CheckPermissionResponse);
  rpc BatchCheckPermission(BatchCheckPermissionRequest) returns (BatchCheckPermissionResponse);
  rpc GrantPermission(GrantPermissionRequest) returns (GrantPermissionResponse);
  rpc RevokePermission(RevokePermissionRequest) returns (RevokePermissionResponse);
  rpc ListPermissions(ListPermissionsRequest) returns (ListPermissionsResponse);
  
  // Role management
  rpc CreateRole(CreateRoleRequest) returns (CreateRoleResponse);
  rpc AssignRole(AssignRoleRequest) returns (AssignRoleResponse);
  rpc ListUserRoles(ListUserRolesRequest) returns (ListUserRolesResponse);
  
  // Health check
  rpc HealthCheck(google.protobuf.Empty) returns (HealthCheckResponse);
}
```

### 3. Client Libraries

#### Go Client

```go
package main

import (
    "context"
    "log"
    
    "github.com/pyairtable/pyairtable-permission-service/pkg/client"
    permissionv1 "github.com/pyairtable/pyairtable-protos/generated/go/pyairtable/permission/v1"
)

func main() {
    config := client.DefaultClientConfig("localhost:50051")
    client, err := client.NewPermissionClient(config, logger)
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Check permission
    resp, err := client.CheckPermission(context.Background(), &permissionv1.CheckPermissionRequest{
        UserId:       "user-123",
        ResourceType: permissionv1.ResourceType_RESOURCE_TYPE_TABLE,
        ResourceId:   "table-456",
        Action:       "read",
    })
    
    if err != nil {
        log.Printf("Permission check failed: %v", err)
        return
    }
    
    log.Printf("Permission allowed: %v", resp.Allowed)
}
```

#### Python Client

```python
import asyncio
from permission_client import PermissionClientContext, create_client_config

async def main():
    config = create_client_config("localhost:50051")
    
    async with PermissionClientContext(config) as client:
        # Health check
        await client.health_check()
        
        # Check permission
        response = await client.check_permission(request)
        print(f"Permission allowed: {response.allowed}")

if __name__ == "__main__":
    asyncio.run(main())
```

### 4. Observability

#### Metrics

The service exposes Prometheus metrics including:

- Request count and duration
- Active requests
- Error rates
- Database connection pool status
- Cache hit/miss rates
- gRPC-specific metrics

#### Tracing

OpenTelemetry tracing is integrated for:

- End-to-end request tracing
- Database query tracing
- Cache operation tracing
- Cross-service correlation

#### Logging

Structured logging with:

- Request/response logging
- Error logging with context
- Performance logging
- gRPC interceptor logging

### 5. Resilience Features

#### Connection Pooling

- Configurable pool size (default: 5 connections)
- Round-robin load balancing
- Automatic connection health checking
- Connection recovery

#### Retry Logic

- Exponential backoff with jitter
- Configurable retry attempts
- Retryable error detection
- Circuit breaker integration

#### Circuit Breaker

- Configurable failure threshold
- Automatic recovery detection
- Half-open state testing
- Metrics reporting

## Performance

### Benchmarks

Run performance comparisons between gRPC and REST:

```bash
cd test/benchmarks
go test -bench=. -benchmem -benchtime=10s
```

Expected performance improvements with gRPC:

- **Latency**: 20-40% lower than REST
- **Throughput**: 30-50% higher than REST
- **Memory**: 15-25% lower memory usage
- **CPU**: 10-20% lower CPU usage

### Load Testing

```bash
# gRPC load test
ghz --insecure \
    --proto ../../proto/pyairtable/permission/v1/permission.proto \
    --call pyairtable.permission.v1.PermissionService.CheckPermission \
    --data '{"user_id":"test","resource_type":"RESOURCE_TYPE_TABLE","resource_id":"test","action":"read"}' \
    --connections=50 \
    --concurrency=100 \
    --total=10000 \
    localhost:50051

# REST load test
hey -n 10000 -c 100 -m POST \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer test-token" \
    -d '{"user_id":"test","resource_type":"table","resource_id":"test","action":"read"}' \
    http://localhost:8085/api/v1/permissions/check
```

## Deployment

### Docker Compose

```bash
# Start with gRPC support
docker-compose -f docker-compose.grpc.yml up -d

# View logs
docker-compose -f docker-compose.grpc.yml logs -f permission-service

# Health check
grpc_health_probe -addr=localhost:50051
curl http://localhost:8085/health
```

### Kubernetes

```yaml
apiVersion: v1
kind: Service
metadata:
  name: permission-service
spec:
  ports:
  - name: http
    port: 8085
    targetPort: 8085
  - name: grpc
    port: 50051
    targetPort: 50051
  - name: metrics
    port: 9090
    targetPort: 9090
  selector:
    app: permission-service
```

## Monitoring

### Grafana Dashboards

Import the provided Grafana dashboard for comprehensive monitoring:

1. gRPC request metrics
2. REST request metrics
3. Database performance
4. Cache performance
5. Error rates and alerts

### Alerts

Key alerts to configure:

```yaml
groups:
- name: permission-service
  rules:
  - alert: HighErrorRate
    expr: rate(errors_total[5m]) > 0.1
    labels:
      severity: warning
    annotations:
      summary: High error rate in permission service

  - alert: HighLatency
    expr: histogram_quantile(0.95, rate(request_duration_seconds_bucket[5m])) > 1
    labels:
      severity: warning
    annotations:
      summary: High latency in permission service
```

## Migration Guide

### From REST to gRPC

1. **Phase 1**: Deploy dual-protocol service
2. **Phase 2**: Update clients to use gRPC
3. **Phase 3**: Monitor performance improvements
4. **Phase 4**: Optionally deprecate REST endpoints

### Client Migration

```go
// Before (REST)
resp, err := http.Post("http://service/api/v1/permissions/check", ...)

// After (gRPC)
resp, err := grpcClient.CheckPermission(ctx, req)
```

## Security

### Authentication

- JWT token validation
- mTLS support (optional)
- Rate limiting
- Request validation

### Authorization

- User context extraction
- Tenant isolation
- Resource-level permissions
- Audit logging

## Troubleshooting

### Common Issues

1. **Connection Refused**
   ```bash
   # Check if gRPC port is listening
   netstat -an | grep 50051
   
   # Check container logs
   docker logs permission-service-grpc
   ```

2. **High Latency**
   ```bash
   # Check connection pool stats
   curl http://localhost:8085/debug/grpc/stats
   
   # Monitor Prometheus metrics
   curl http://localhost:9090/metrics | grep request_duration
   ```

3. **Memory Leaks**
   ```bash
   # Check for connection leaks
   curl http://localhost:8085/debug/grpc/connections
   
   # Monitor memory usage
   docker stats permission-service-grpc
   ```

### Debug Mode

Enable debug logging:

```bash
export LOG_LEVEL=debug
export GRPC_VERBOSITY=debug
```

## Future Enhancements

1. **Streaming Support**: Implement streaming for bulk operations
2. **Load Balancing**: Add client-side load balancing
3. **Service Mesh**: Integration with Istio/Envoy
4. **gRPC-Web**: Support for browser clients
5. **Advanced Retry**: Implement hedged requests

## Contributing

When adding new gRPC methods:

1. Update the protobuf definition
2. Regenerate Go code
3. Implement server method
4. Add client wrapper
5. Write tests and benchmarks
6. Update documentation

For questions or issues, please refer to the main project repository or create an issue.