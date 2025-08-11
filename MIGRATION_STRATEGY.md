# PyAirtable Platform Migration Strategy

## Overview

This document outlines the strategy for migrating from separate microservices (user-service, workspace-service, tenant-service, notification-service) to a consolidated pyairtable-platform service.

## Services Being Consolidated

### Source Services
- **user-service**: User management and authentication
- **workspace-service**: Workspace and membership management  
- **tenant-service**: Tenant/organization management
- **notification-service**: User notification system (partial)

### Target Service
- **pyairtable-platform**: Consolidated service containing all above functionality

## Migration Phases

### Phase 1: Preparation (Pre-Migration)
1. **Backup existing data**
   ```bash
   # Database backup
   pg_dump pyairtable > backup_$(date +%Y%m%d_%H%M%S).sql
   
   # Service configuration backup
   cp -r go-services/ go-services-backup-$(date +%Y%m%d_%H%M%S)/
   ```

2. **Run consolidation script**
   ```bash
   cd go-services
   ./consolidate-platform-services.sh
   ```

3. **Verify consolidated service**
   ```bash
   cd pyairtable-platform
   go mod tidy
   go build cmd/server/main.go
   ```

### Phase 2: Data Migration
1. **Schema Migration**
   - The consolidated service uses the same database schema
   - Auto-migration will handle any new fields
   - Existing data remains compatible

2. **Configuration Migration**
   ```bash
   # Copy environment variables from existing services
   # Update connection strings to point to consolidated service
   ```

### Phase 3: Service Replacement (Blue-Green Deployment)

#### Option A: Gradual Migration
1. **Deploy consolidated service on new port**
   ```bash
   cd pyairtable-platform
   docker-compose up -d
   # Service runs on port 8080
   ```

2. **Update API Gateway routing**
   ```yaml
   # In API Gateway configuration
   - path: /api/v1/users/*
     upstream: http://pyairtable-platform:8080
   - path: /api/v1/workspaces/*
     upstream: http://pyairtable-platform:8080
   - path: /api/v1/tenants/*
     upstream: http://pyairtable-platform:8080
   ```

3. **Monitor and validate**
   - Check health endpoints
   - Verify API responses
   - Monitor logs for errors

4. **Gradually shift traffic**
   - 10% traffic to new service
   - 50% traffic if no issues
   - 100% traffic after validation

#### Option B: Complete Cutover
1. **Stop existing services**
   ```bash
   docker-compose -f docker-compose.yml stop user-service workspace-service tenant-service notification-service
   ```

2. **Start consolidated service**
   ```bash
   cd pyairtable-platform
   docker-compose up -d
   ```

3. **Update all service references**

### Phase 4: Cleanup
1. **Remove old services** (after 48h of successful operation)
   ```bash
   rm -rf go-services/user-service
   rm -rf go-services/workspace-service  
   rm -rf go-services/tenant-service
   rm -rf go-services/notification-service
   ```

2. **Update documentation and configs**

## Dependency Management

### Shared Dependencies
The consolidated service uses these key dependencies:
```go
require (
    github.com/gofiber/fiber/v3 v3.0.0-beta.2
    github.com/golang-jwt/jwt/v5 v5.0.0
    golang.org/x/crypto v0.19.0
    gorm.io/driver/postgres v1.5.2
    gorm.io/gorm v1.30.0
)
```

### Breaking Dependencies
- Remove individual service go.mod files
- Consolidate into single module: `github.com/Reg-Kris/pyairtable-platform`
- Update import paths in any external consumers

## API Compatibility

### Endpoint Mapping
| Old Service | Old Endpoint | New Endpoint |
|-------------|--------------|--------------|
| user-service | `/health` | `/api/v1/users/*` |
| workspace-service | `/health` | `/api/v1/workspaces/*` |
| tenant-service | `/health` | `/api/v1/tenants/*` |
| notification-service | `/health` | `/api/v1/notifications/*` |

### Authentication
- JWT tokens remain compatible
- Same secret key configuration
- Token validation logic consolidated

### Response Formats
- Maintain existing JSON response structures
- Add consistent error handling
- Preserve pagination patterns

## Rollback Strategy

### Quick Rollback (< 1 hour)
1. **Revert API Gateway configuration**
   ```bash
   # Restore original routing to individual services
   kubectl apply -f api-gateway-original-config.yaml
   ```

2. **Restart original services**
   ```bash
   docker-compose -f docker-compose.original.yml up -d
   ```

### Full Rollback (if issues persist)
1. **Restore from backup**
   ```bash
   # Restore service directories
   rm -rf go-services/pyairtable-platform
   cp -r go-services-backup-TIMESTAMP/* go-services/
   
   # Restore database if needed
   psql pyairtable < backup_TIMESTAMP.sql
   ```

2. **Restart all original services**
   ```bash
   cd go-services
   docker-compose up -d
   ```

## Testing Strategy

### Pre-Migration Testing
1. **Unit Tests**
   ```bash
   cd pyairtable-platform
   go test ./...
   ```

2. **Integration Tests**
   ```bash
   # Start test database
   docker-compose -f docker-compose.test.yml up -d postgres
   
   # Run integration tests
   go test -tags=integration ./test/integration/...
   ```

3. **Load Testing**
   ```bash
   # Use existing load test scripts
   ./scripts/load-test.sh http://localhost:8080
   ```

### Post-Migration Validation
1. **Health Checks**
   ```bash
   curl http://localhost:8080/health
   ```

2. **API Endpoint Tests**
   ```bash
   # Test each major endpoint
   curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/users
   curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/workspaces
   ```

3. **Database Integrity**
   ```sql
   -- Verify data consistency
   SELECT COUNT(*) FROM users;
   SELECT COUNT(*) FROM workspaces;
   SELECT COUNT(*) FROM tenants;
   ```

## Monitoring and Observability

### Metrics to Monitor
- Request latency (should be similar to individual services)
- Error rates (should not increase)
- Memory usage (may be higher due to consolidation)
- CPU usage
- Database connection pool usage

### Logging
- Structured JSON logging with slog
- Request tracing across consolidated handlers
- Error tracking and alerting

### Health Checks
```bash
# Consolidated health endpoint
curl http://localhost:8080/health

# Individual component health (internal)
curl http://localhost:8080/internal/health/users
curl http://localhost:8080/internal/health/workspaces
```

## Performance Considerations

### Expected Improvements
- **Reduced network latency**: No inter-service communication
- **Shared connection pools**: More efficient database usage
- **Reduced memory overhead**: Single Go runtime instead of 4

### Potential Concerns
- **Single point of failure**: One service instead of distributed
- **Larger memory footprint**: All services in one process
- **Deployment complexity**: All components must be deployed together

### Mitigation Strategies
- **Multiple replicas**: Run 3+ instances for redundancy
- **Circuit breakers**: Isolate failures within the service
- **Resource limits**: Set appropriate memory/CPU limits
- **Monitoring**: Enhanced observability for early issue detection

## Security Considerations

### Authentication & Authorization
- Same JWT validation logic
- Consistent middleware across all routes
- Tenant isolation maintained

### Network Security
- Same firewall rules apply
- Internal API endpoints secured
- Rate limiting preserved

### Data Privacy
- Tenant data isolation enforced
- Audit logging maintained
- GDPR compliance preserved

## Configuration Management

### Environment Variables
```bash
# Required for consolidated service
PORT=8080
DATABASE_URL=postgres://postgres:postgres@localhost:5432/pyairtable?sslmode=disable
JWT_SECRET=your-secret-key-change-in-production
REDIS_URL=redis://localhost:6379
LOG_LEVEL=info
```

### Docker Compose Updates
```yaml
# Update main docker-compose.yml
services:
  pyairtable-platform:
    build: ./pyairtable-platform
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=${DATABASE_URL}
      - JWT_SECRET=${JWT_SECRET}
    depends_on:
      - postgres
      - redis
```

## Timeline

### Recommended Timeline
- **Week 1**: Preparation and testing
  - Run consolidation script
  - Unit and integration testing
  - Load testing in staging
  
- **Week 2**: Staging deployment
  - Deploy to staging environment
  - End-to-end testing
  - Performance validation
  
- **Week 3**: Production migration
  - Blue-green deployment
  - Traffic shifting
  - Monitoring and validation
  
- **Week 4**: Cleanup and optimization
  - Remove old services
  - Performance tuning
  - Documentation updates

### Critical Success Factors
1. **Thorough testing** in staging environment
2. **Gradual traffic shifting** to minimize risk
3. **Real-time monitoring** during migration
4. **Quick rollback capability** if issues arise
5. **Team readiness** for 24/7 support during migration

## Support and Troubleshooting

### Common Issues and Solutions

#### Issue: Service won't start
```bash
# Check configuration
go run cmd/server/main.go --check-config

# Check database connectivity
psql $DATABASE_URL -c "SELECT 1"

# Check logs
docker logs pyairtable-platform
```

#### Issue: High memory usage
```bash
# Monitor memory
docker stats pyairtable-platform

# Adjust limits in docker-compose.yml
deploy:
  resources:
    limits:
      memory: 512M
```

#### Issue: Slow response times
```bash
# Check database connections
SELECT * FROM pg_stat_activity WHERE application_name = 'pyairtable-platform';

# Enable query logging
LOG_LEVEL=debug go run cmd/server/main.go
```

### Emergency Contacts
- Platform Team: platform-team@company.com
- Database Team: dba-team@company.com
- DevOps Team: devops-team@company.com

## Post-Migration Optimization

### Performance Tuning
1. **Database optimization**
   - Connection pool tuning
   - Query optimization
   - Index analysis

2. **Memory optimization**
   - Profile memory usage
   - Optimize struct layouts
   - Tune garbage collection

3. **CPU optimization**
   - Profile CPU usage
   - Optimize hot paths
   - Consider Go routine optimization

### Scaling Considerations
- **Horizontal scaling**: Multiple service instances
- **Database scaling**: Read replicas, connection pooling
- **Caching**: Redis for frequently accessed data
- **Load balancing**: Distribute traffic across instances

This migration strategy provides a comprehensive approach to consolidating your microservices while minimizing risk and maintaining system reliability.