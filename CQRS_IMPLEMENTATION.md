# CQRS Implementation for PyAirtable

This document describes the complete CQRS (Command Query Responsibility Segregation) implementation for PyAirtable, designed to achieve 10x query performance improvement through read/write separation, multi-level caching, and optimized projections.

## Architecture Overview

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Commands      │    │   Events         │    │   Queries       │
│   (Write Side)  │───▶│   (Event Store)  │───▶│   (Read Side)   │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│  Write Schema   │    │ Projection       │    │  Read Schema    │
│  - Events       │    │ Manager          │    │  - User Views   │
│  - Aggregates   │    │ - Async Workers  │    │  - Workspace    │
│  - Commands     │    │ - Retry Logic    │    │  - Summaries    │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                                │
                                ▼
                       ┌──────────────────┐
                       │  Multi-Level     │
                       │  Cache (L1/L2)   │
                       │  - Redis         │
                       │  - Compression   │
                       │  - TTL           │
                       └──────────────────┘
```

## Key Components

### 1. Database Separation

**Write Schema (`write_schema`)**
- Domain events storage
- Command processing
- Event sourcing
- Optimistic concurrency control

**Read Schema (`read_schema`)**
- User projections
- Workspace projections  
- Tenant summaries
- Optimized for queries

### 2. Projection System

**Asynchronous Projection Updates**
- Background workers process events
- Automatic retry with exponential backoff
- Checkpoint tracking for reliability
- Event ordering guarantees

**Projection Handlers**
- `UserProjectionHandler` - User events
- `WorkspaceProjectionHandler` - Workspace events
- Extensible for new projections

### 3. Multi-Level Caching

**L1 Cache (Application)**
- In-memory caching (planned)
- Fastest access
- Limited capacity

**L2 Cache (Redis)**
- Distributed caching
- Compression support
- Encryption for sensitive data
- TTL-based expiration

**L3 Cache (CDN)**
- Edge caching (planned)
- Geographic distribution
- Static content caching

### 4. Performance Monitoring

**Metrics Collected**
- Query duration by type
- Cache hit ratios
- Database connection pools
- Projection lag
- Performance improvement ratios

## Setup Instructions

### 1. Database Setup

Run the CQRS schema migration:

```bash
psql -d pyairtable -f migrations/003_create_cqrs_schemas.sql
```

This creates:
- `write_schema` with domain events table
- `read_schema` with projection tables
- Proper indexes for performance
- Checkpoint tracking

### 2. CQRS Service Configuration

```go
// Initialize CQRS configuration
cqrsConfig := cqrs.DefaultCQRSServiceConfig()
cqrsConfig.Database.WriteConnectionStr = "postgres://write-db-url"
cqrsConfig.Database.ReadConnectionStr = "postgres://read-db-url"
cqrsConfig.Database.WriteSchema = "write_schema"
cqrsConfig.Database.ReadSchema = "read_schema"

// Projection settings
cqrsConfig.Projection.Strategy = cqrs.AsynchronousProjection
cqrsConfig.Projection.WorkerCount = 4
cqrsConfig.Projection.BatchSize = 100

// Cache settings
cqrsConfig.Cache.Strategy = cqrs.ReadThrough
cqrsConfig.Cache.DefaultExpiration = 15 * time.Minute
```

### 3. Service Integration

```go
// Initialize CQRS service
cqrsService, err := cqrs.NewCQRSService(cqrsConfig, redisClient)
if err != nil {
    log.Fatal("Failed to initialize CQRS:", err)
}

// Register projection handlers
userHandler := cqrs.NewUserProjectionHandler(cqrsService.DatabaseConfig.GetReadDB())
workspaceHandler := cqrs.NewWorkspaceProjectionHandler(cqrsService.DatabaseConfig.GetReadDB())

cqrsService.ProjectionManager.RegisterProjection(userHandler)
cqrsService.ProjectionManager.RegisterProjection(workspaceHandler)

// Initialize query service
queryService := cqrs.NewQueryService(
    cqrsService.DatabaseConfig.GetReadDB(),
    cqrsService.CacheManager,
    cqrsService.QueryOptimizer,
)
```

## API Usage Examples

### User Queries

```go
// Get user by ID (with caching)
user, err := queryService.GetUserByID(ctx, userID, tenantID)
if err != nil {
    return err
}

// Get users by tenant (paginated)
users, err := queryService.GetUsersByTenant(ctx, tenantID, 50, 0, true)
if err != nil {
    return err
}
```

### Workspace Queries

```go
// Get workspace with members
workspace, err := queryService.GetWorkspaceByID(ctx, workspaceID, tenantID)
if err != nil {
    return err
}

// Get workspace members
members, err := queryService.GetWorkspaceMembers(ctx, workspaceID, 100, 0)
if err != nil {
    return err
}
```

### Dashboard Queries

```go
// Get tenant summary (cached for 30 minutes)
summary, err := queryService.GetTenantSummary(ctx, tenantID)
if err != nil {
    return err
}
```

### HTTP Endpoints

The CQRS implementation provides optimized REST endpoints:

```
GET /api/v1/users/profile                    # Current user profile
GET /api/v1/users/{userId}                   # User by ID
GET /api/v1/users/tenant/{tenantId}          # Users in tenant
GET /api/v1/workspaces/{workspaceId}         # Workspace details
GET /api/v1/workspaces/tenant/{tenantId}     # Tenant workspaces
GET /api/v1/workspaces/{id}/members          # Workspace members
GET /api/v1/tenants/{tenantId}/summary       # Tenant summary
GET /api/v1/tenants/{tenantId}/dashboard     # Complete dashboard
```

## Performance Features

### Query Optimization

1. **Index-Optimized Queries**
   - Database indexes on frequently queried fields
   - Composite indexes for complex queries
   - Covering indexes to avoid table lookups

2. **Cache-First Strategy**
   - Check cache before database
   - Warm cache on startup
   - Intelligent cache invalidation

3. **Connection Pooling**
   - Separate pools for read/write
   - Optimized pool sizes
   - Connection health monitoring

### Caching Strategy

1. **Cache Keys**
   ```
   user:{userId}:tenant:{tenantId}:v:1
   users_tenant:{tenantId}:limit:50:offset:0:v:1
   workspace:{workspaceId}:tenant:{tenantId}:v:1
   tenant_summary:{tenantId}:v:1
   ```

2. **TTL Configuration**
   - User data: 15 minutes
   - Workspace data: 15 minutes
   - Tenant summaries: 30 minutes
   - List queries: 5 minutes

3. **Cache Invalidation**
   - Event-driven invalidation
   - Pattern-based invalidation
   - Manual cache warming

## Performance Monitoring

### Metrics Dashboard

Access performance metrics via:

```
GET /api/v1/admin/cqrs/projections/status    # Projection health
GET /api/v1/admin/cqrs/cache/stats           # Cache performance
POST /api/v1/admin/cqrs/cache/invalidate     # Manual invalidation
POST /api/v1/admin/cqrs/cache/warmup         # Cache warming
```

### Key Metrics

1. **Query Performance**
   - Average query duration
   - Cache hit ratio
   - Database connection usage
   - Performance improvement ratio

2. **Projection Health**
   - Event processing lag
   - Projection checkpoint status
   - Error rates and retries

3. **Cache Performance**
   - Hit/miss ratios by cache level
   - Memory usage
   - Eviction rates

## Expected Performance Improvements

### Target: 10x Performance Improvement

| Query Type | Traditional | CQRS (Cached) | Improvement |
|------------|-------------|---------------|-------------|
| Single User | 50ms | 5ms | 10x |
| Tenant Users | 200ms | 15ms | 13x |
| Workspace Details | 80ms | 8ms | 10x |
| Dashboard | 500ms | 25ms | 20x |

### Cache Hit Ratio Targets

- **Cold Cache**: 0% hit ratio (first queries)
- **Warm Cache**: 80%+ hit ratio (typical operation)  
- **Hot Cache**: 95%+ hit ratio (peak performance)

## Event Processing

### Supported Events

**User Events**
- `user.registered`
- `user.activated`
- `user.deactivated`
- `user.updated`
- `user.role_changed`

**Workspace Events**
- `workspace.created`
- `workspace.updated`
- `workspace.deleted`
- `workspace.member_added`
- `workspace.member_removed`
- `workspace.member_role_changed`

### Event Processing Flow

1. **Command Execution**
   - Validate command
   - Apply to aggregate
   - Generate domain events
   - Save to event store

2. **Projection Updates**
   - Event bus notification
   - Async projection workers
   - Update read models
   - Cache invalidation

3. **Error Handling**
   - Automatic retry with backoff
   - Dead letter queue
   - Manual replay capability

## Troubleshooting

### Common Issues

1. **Cache Misses**
   ```bash
   # Check Redis connectivity
   redis-cli ping
   
   # Monitor cache hit ratio
   curl /api/v1/admin/cqrs/cache/stats
   ```

2. **Projection Lag**
   ```bash
   # Check projection status
   curl /api/v1/admin/cqrs/projections/status
   
   # Manual cache warmup
   curl -X POST /api/v1/admin/cqrs/cache/warmup
   ```

3. **Database Connection Issues**
   ```bash
   # Check database health
   curl /health/cqrs
   
   # Monitor connection pools
   curl /api/v1/admin/cqrs/cache/stats
   ```

### Performance Tuning

1. **Cache Configuration**
   - Adjust TTL based on data volatility
   - Increase Redis memory allocation
   - Enable compression for large objects

2. **Database Optimization**
   - Analyze slow queries
   - Update table statistics
   - Consider additional indexes

3. **Projection Workers**
   - Adjust worker count based on event volume
   - Monitor event processing lag
   - Optimize batch sizes

## Testing

### Performance Testing

Run the performance benchmark suite:

```go
go test -v ./pkg/cqrs/performance_test.go
```

This will:
- Setup test data (1000 users across 5 tenants)
- Measure baseline query performance
- Measure CQRS query performance  
- Verify 10x improvement target
- Generate performance report

### Integration Testing

```bash
# Run CQRS integration tests
go test -v ./pkg/cqrs/...

# Run end-to-end performance tests
go test -v -tags=integration ./tests/integration/cqrs_test.go
```

## Security Considerations

1. **Data Isolation**
   - Tenant-based data separation
   - Row-level security in projections
   - Secure cache key patterns

2. **Cache Security**
   - Optional encryption for sensitive data
   - TTL-based expiration
   - Secure Redis configuration

3. **Access Control**
   - JWT-based authentication
   - Role-based authorization
   - Admin-only management endpoints

## Deployment

### Production Checklist

- [ ] Database schemas created
- [ ] Redis cluster configured
- [ ] Monitoring dashboards setup
- [ ] Performance baselines established
- [ ] Cache warming strategy configured
- [ ] Backup and recovery procedures tested
- [ ] Security audit completed

### Scaling Considerations

1. **Horizontal Scaling**
   - Multiple projection workers
   - Redis clustering
   - Read replica databases

2. **Vertical Scaling**
   - Increased connection pools
   - More Redis memory
   - Faster storage for databases

This CQRS implementation provides the foundation for achieving 10x query performance improvements while maintaining data consistency and system reliability.