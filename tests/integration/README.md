# PyAirtable Integration Test Suite

Comprehensive integration testing framework for the PyAirtable platform, designed to validate end-to-end functionality across all microservices and ensure production readiness.

## Overview

This test suite provides comprehensive coverage of:

### 🔐 **Authentication & Authorization**
- End-to-end user registration and login flows
- JWT token lifecycle management (issue, refresh, revoke)
- Tenant isolation validation
- Cross-tenant access prevention
- API key authentication
- Session management and security

### 🛡️ **RBAC System Validation**
- Hierarchical permission inheritance (Workspace → Project → Base)
- Role-based access control across all resource types
- Direct permission grants and denials
- Permission caching and performance
- Audit logging for all permission changes
- Batch permission checking
- Cross-service permission validation

### 🔄 **Cross-Service Communication**
- API Gateway routing and load balancing
- Service-to-service authentication
- Request/response validation across service boundaries
- Error handling and propagation
- Circuit breaker and retry mechanisms
- Service health monitoring

### 📊 **Data Consistency & Integrity**
- Tenant data isolation enforcement
- Cascade deletion workflows
- Database transaction atomicity
- Eventual consistency validation
- Data validation consistency across services
- Audit trail completeness

### ⚡ **Performance & Reliability**
- Response time benchmarking
- Concurrent user load testing
- Rate limiting validation
- Memory usage and leak detection
- Database connection pooling
- Cache performance optimization
- Error recovery mechanisms

## Architecture

### Test Environment
```
┌─────────────────────┐    ┌─────────────────────┐
│   API Gateway       │    │  Permission Service │
│   (Port 8080)       │    │   (Port 8085)       │
└─────────────────────┘    └─────────────────────┘
           │                           │
           └───────────────┬───────────┘
                          │
┌─────────────────────┐    │    ┌─────────────────────┐
│ Platform Services   │    │    │ Automation Services │
│   (Port 8081)       │    │    │   (Port 8082)       │
└─────────────────────┘    │    └─────────────────────┘
                          │
           ┌──────────────────────────────┐
           │     Infrastructure           │
           │  • PostgreSQL (isolated DBs) │
           │  • Redis (with DB separation)│
           │  • Mock Python Services      │
           └──────────────────────────────┘
```

### Test Data Strategy
- **Fixture-based**: Predictable test data loaded from SQL fixtures
- **Tenant Isolation**: Alpha, Beta, and Gamma (suspended) tenants
- **User Hierarchy**: Admins, project leads, members, and viewers
- **Resource Tree**: Workspaces → Projects → Airtable Bases
- **Permission Matrix**: Comprehensive role and permission assignments

## Quick Start

### Prerequisites
- Docker and Docker Compose
- Go 1.21+
- Make (optional, for convenience commands)

### Run All Tests
```bash
# Using the test runner script
./run_tests.sh

# Or using Make
make test-all

# Or using Go directly
go test -timeout 30m .
```

### Run Specific Test Suites
```bash
# Authentication flow tests
./run_tests.sh auth

# RBAC system tests
./run_tests.sh rbac

# Cross-service communication tests
./run_tests.sh cross-service

# Data consistency tests
./run_tests.sh data

# Performance and reliability tests
./run_tests.sh performance

# Quick smoke tests (for CI/PR validation)
./run_tests.sh quick
```

## Test Suites

### 1. Authentication Flow Tests (`auth_flow_test.go`)

**Coverage:**
- ✅ User registration with tenant assignment
- ✅ Login with email/password validation
- ✅ JWT token generation and validation
- ✅ Token refresh mechanisms
- ✅ Logout and token invalidation
- ✅ Tenant isolation enforcement
- ✅ Invalid login attempt handling
- ✅ Concurrent authentication safety
- ✅ API key authentication
- ✅ Expired token handling

**Key Test Cases:**
```go
TestUserRegistrationFlow()     // Complete registration process
TestUserLoginFlow()           // Standard authentication
TestTenantIsolationInAuth()   // Cross-tenant access prevention
TestTokenRefreshFlow()        // JWT lifecycle management
TestConcurrentAuthentication() // Race condition safety
```

### 2. RBAC System Tests (`rbac_test.go`)

**Coverage:**
- ✅ Permission checking (single and batch)
- ✅ Hierarchical permission inheritance
- ✅ Role creation and assignment
- ✅ Direct permission grants/denials
- ✅ Permission audit logging
- ✅ Role listing and filtering
- ✅ Cross-service permission validation
- ✅ Permission caching behavior

**Key Test Cases:**
```go
TestPermissionChecking()          // Core permission validation
TestBatchPermissionChecking()     // Efficient bulk operations
TestHierarchicalPermissions()     // Inheritance validation
TestRoleManagement()              // Role lifecycle
TestDirectPermissions()           // Explicit grants/denials
TestPermissionAuditLogs()         // Audit trail verification
```

### 3. Cross-Service Tests (`cross_service_test.go`)

**Coverage:**
- ✅ API Gateway routing validation
- ✅ Service-to-service authentication
- ✅ Permission integration across services
- ✅ Complete workflow testing
- ✅ Error handling and propagation
- ✅ Service health monitoring
- ✅ Concurrent cross-service operations
- ✅ CORS and security headers

**Key Test Cases:**
```go
TestAPIGatewayRouting()               // Request routing validation
TestServiceToServiceAuthentication()  // Internal auth verification
TestPermissionIntegrationAcrossServices() // End-to-end permission flow
TestWorkflowIntegration()             // Complete business workflows
TestErrorHandlingAcrossServices()     // Error propagation
```

### 4. Data Consistency Tests (`data_consistency_test.go`)

**Coverage:**
- ✅ Tenant data isolation
- ✅ Cascade deletion workflows
- ✅ User deletion impact analysis
- ✅ Database transaction atomicity
- ✅ Eventual consistency validation
- ✅ Data validation consistency
- ✅ Audit log consistency

**Key Test Cases:**
```go
TestTenantDataIsolation()         // Cross-tenant data protection
TestCascadeDeletion()             // Resource cleanup workflows
TestUserDeletion()                // User removal impact
TestDatabaseTransactionConsistency() // Atomic operations
TestEventualConsistency()         // Distributed system behavior
TestAuditLogConsistency()         // Complete audit trails
```

### 5. Performance Tests (`performance_test.go`)

**Coverage:**
- ✅ Response time benchmarking
- ✅ Concurrent user load testing
- ✅ Rate limiting validation
- ✅ Memory usage monitoring
- ✅ Database connection pooling
- ✅ Cache performance analysis
- ✅ Error recovery testing

**Key Test Cases:**
```go
TestResponseTimes()               // Performance benchmarking
TestConcurrentUsers()             // Load testing
TestRateLimiting()                // Rate limit validation
TestMemoryAndResourceUsage()      // Resource leak detection
TestDatabaseConnectionPooling()   // Connection management
TestCachePerformance()            // Cache efficiency
TestErrorRecovery()               // Resilience testing
```

## Configuration

### Environment Variables
```bash
# Test execution
TEST_TIMEOUT=30m          # Maximum test execution time
TEST_PARALLEL=1           # Number of parallel test processes
VERBOSE=false             # Enable verbose output
CLEANUP=true              # Clean up after tests

# Test environment
TEST_API_KEY=test-api-key-12345
JWT_SECRET=test-jwt-secret-for-testing-only
POSTGRES_USER=test_user
POSTGRES_PASSWORD=test_password
POSTGRES_DB=pyairtable_test
REDIS_PASSWORD=testpassword
```

### Test Constants
All test UUIDs and identifiers are defined in `test_environment.go`:
```go
// Test tenant IDs
TestTenantAlphaID = "550e8400-e29b-41d4-a716-446655440001"
TestTenantBetaID  = "550e8400-e29b-41d4-a716-446655440002"

// Test user IDs
TestUserAlphaAdminID = "660e8400-e29b-41d4-a716-446655440001"
TestUserAlphaUser1ID = "660e8400-e29b-41d4-a716-446655440002"
// ... etc
```

## Advanced Usage

### Debug Mode
Start the test environment without running tests for debugging:
```bash
./run_tests.sh --debug
# Services will be available at:
# - API Gateway: http://localhost:8080
# - Platform Services: http://localhost:8081
# - Permission Service: http://localhost:8085
# - Automation Services: http://localhost:8082
```

### Custom Test Execution
```bash
# Run with custom timeout and parallelism
./run_tests.sh --timeout 60m --parallel 4 all

# Run with verbose output
./run_tests.sh --verbose auth

# Run in CI mode with structured output
./run_tests.sh --ci all > test-results.json

# Skip cleanup for debugging
./run_tests.sh --no-cleanup rbac
```

### Manual Test Environment Management
```bash
# Start environment
make start-env

# Check service health
make health-check

# View logs
make logs                    # All services
make logs-api-gateway       # Specific service

# Stop environment
make stop-env

# Clean up completely
make clean
```

## CI/CD Integration

### GitHub Actions
The test suite includes a comprehensive GitHub Actions workflow (`.github/workflows/integration-tests.yml`) that:

- ✅ Runs on push/PR to main branches
- ✅ Executes daily scheduled runs
- ✅ Supports manual workflow dispatch
- ✅ Provides matrix testing for different suites
- ✅ Collects artifacts and logs
- ✅ Includes security scanning
- ✅ Sends notifications on failures

### Integration with Other CI Systems
```bash
# GitLab CI
script:
  - cd go-services/tests/integration
  - ./run_tests.sh --ci all

# Jenkins
sh '''
    cd go-services/tests/integration
    ./run_tests.sh --ci --timeout 45m all
'''

# Azure DevOps
- script: |
    cd go-services/tests/integration
    ./run_tests.sh --ci all
  displayName: 'Run Integration Tests'
```

## Test Data Management

### Fixture Structure
```sql
-- fixtures/test-data.sql
INSERT INTO tenants (id, name, domain, status, plan_type) VALUES
('550e8400-e29b-41d4-a716-446655440001', 'Test Tenant Alpha', 'alpha.test.com', 'active', 'enterprise'),
('550e8400-e29b-41d4-a716-446655440002', 'Test Tenant Beta', 'beta.test.com', 'active', 'pro');

INSERT INTO users (id, tenant_id, email, username, password_hash) VALUES
('660e8400-e29b-41d4-a716-446655440001', '550e8400-e29b-41d4-a716-446655440001', 'admin@alpha.test.com', 'alpha_admin', '$2b$12$...');
-- ... more test data
```

### Data Reset Strategy
- Each test suite runs with fresh data loaded from fixtures
- `ResetTestData()` function reloads SQL fixtures between tests
- Isolated test databases prevent cross-test contamination
- Predictable test data ensures consistent test behavior

## Monitoring and Observability

### Test Metrics
The test suite collects and reports:
- ✅ Response time percentiles (p50, p90, p95, p99)
- ✅ Error rates by service and endpoint
- ✅ Concurrent user performance
- ✅ Memory and resource usage
- ✅ Cache hit/miss ratios
- ✅ Database connection pool usage

### Logging and Debugging
```bash
# View real-time logs during test execution
tail -f test-results/integration-tests.log

# Get service-specific logs
make logs-permission-service

# Debug failed tests
./run_tests.sh --debug
# Then manually test endpoints:
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/workspaces
```

## Best Practices

### Test Organization
- ✅ **Suite-based**: Related tests grouped in test suites
- ✅ **Isolation**: Each test runs with clean state
- ✅ **Deterministic**: Predictable test data and behavior
- ✅ **Fast feedback**: Quick smoke tests for PR validation
- ✅ **Comprehensive**: Full test coverage for production confidence

### Performance Considerations
- ✅ **Parallel execution**: Tests run concurrently where safe
- ✅ **Resource cleanup**: Automatic cleanup prevents resource leaks
- ✅ **Caching**: Docker layer caching speeds up CI builds
- ✅ **Efficient fixtures**: Minimal test data for faster setup

### Error Handling
- ✅ **Graceful failures**: Tests fail with clear error messages
- ✅ **Artifact collection**: Logs and reports saved on failure
- ✅ **Debug modes**: Easy debugging with --debug flag
- ✅ **Retry logic**: Eventual consistency handled appropriately

## Troubleshooting

### Common Issues

**Tests fail with "service not ready"**
```bash
# Check service health
make health-check

# View service logs
make logs

# Increase timeout
./run_tests.sh --timeout 60m all
```

**Port conflicts**
```bash
# Clean up existing containers
make clean

# Check for port usage
lsof -i :8080,8081,8085,8082
```

**Docker issues**
```bash
# Restart Docker daemon
sudo systemctl restart docker

# Clean Docker resources
docker system prune -f --volumes
```

**Permission denied errors**
```bash
# Make script executable
chmod +x run_tests.sh

# Check Docker permissions
sudo usermod -aG docker $USER
```

### Debug Workflow
1. Start debug mode: `./run_tests.sh --debug`
2. Test services manually: `curl -H "X-API-Key: test-api-key-12345" http://localhost:8080/health`
3. Check logs: `make logs`
4. Fix issues and re-run specific tests: `./run_tests.sh auth`

## Contributing

### Adding New Tests
1. Create test functions following the pattern: `TestFeatureName()`
2. Add to appropriate test suite or create new suite
3. Update fixtures if new test data needed
4. Run locally: `./run_tests.sh --verbose your-new-test`
5. Update documentation

### Test Suite Guidelines
- ✅ Use table-driven tests for multiple scenarios
- ✅ Include both positive and negative test cases
- ✅ Test error conditions and edge cases
- ✅ Validate response structure and data
- ✅ Clean up created resources
- ✅ Use descriptive test names and error messages

### Performance Test Guidelines
- ✅ Set realistic performance thresholds
- ✅ Test with concurrent users
- ✅ Monitor resource usage
- ✅ Validate under different load conditions
- ✅ Include cache warming where appropriate

## Support

For issues with the integration test suite:

1. **Check logs**: Start with `make logs` to see service output
2. **Validate environment**: Run `./run_tests.sh --validate-only`
3. **Debug mode**: Use `./run_tests.sh --debug` for interactive debugging
4. **Clean restart**: Try `make clean` followed by fresh test run
5. **Report issues**: Include test logs and system information

**Test Results Location**: `./test-results/`
**Service Logs**: `make logs`
**Health Check**: `make health-check`

---

This integration test suite provides comprehensive validation of the PyAirtable platform, ensuring production readiness through automated testing of all critical functionality across authentication, authorization, cross-service communication, data consistency, and performance requirements.