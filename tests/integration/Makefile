# Integration Test Makefile
# Provides commands for running the comprehensive integration test suite

.PHONY: help test test-auth test-rbac test-cross-service test-data-consistency test-performance test-all clean setup logs

# Default target
help:
	@echo "PyAirtable Integration Test Suite"
	@echo ""
	@echo "Available targets:"
	@echo "  setup              - Set up test environment and dependencies"
	@echo "  test-all           - Run all integration tests"
	@echo "  test-auth          - Run authentication flow tests"
	@echo "  test-rbac          - Run RBAC system tests"
	@echo "  test-cross-service - Run cross-service communication tests"
	@echo "  test-data          - Run data consistency tests"
	@echo "  test-performance   - Run performance and reliability tests"
	@echo "  test-quick         - Run quick smoke tests"
	@echo "  clean              - Clean up test environment"
	@echo "  logs               - Show service logs"
	@echo "  status             - Show test environment status"
	@echo ""
	@echo "Environment variables:"
	@echo "  TEST_TIMEOUT       - Test timeout (default: 30m)"
	@echo "  TEST_PARALLEL      - Number of parallel tests (default: 1)"
	@echo "  TEST_VERBOSE       - Enable verbose output (default: false)"

# Test configuration
TEST_TIMEOUT ?= 30m
TEST_PARALLEL ?= 1
TEST_FLAGS = -timeout $(TEST_TIMEOUT) -p $(TEST_PARALLEL)

ifeq ($(TEST_VERBOSE),true)
	TEST_FLAGS += -v
endif

# Setup test environment
setup:
	@echo "Setting up integration test environment..."
	@go mod download
	@go mod tidy
	@docker --version >/dev/null 2>&1 || (echo "Docker is required but not installed. Please install Docker." && exit 1)
	@docker-compose --version >/dev/null 2>&1 || (echo "Docker Compose is required but not installed. Please install Docker Compose." && exit 1)
	@echo "Environment setup complete"

# Run all integration tests
test-all: setup
	@echo "Running full integration test suite..."
	@go test $(TEST_FLAGS) -run "TestAuthFlowTestSuite|TestRBACTestSuite|TestCrossServiceTestSuite|TestDataConsistencyTestSuite|TestPerformanceTestSuite" .
	@echo "All integration tests completed"

# Run authentication flow tests
test-auth: setup
	@echo "Running authentication flow tests..."
	@go test $(TEST_FLAGS) -run "TestAuthFlowTestSuite" .

# Run RBAC system tests
test-rbac: setup
	@echo "Running RBAC system tests..."
	@go test $(TEST_FLAGS) -run "TestRBACTestSuite" .

# Run cross-service communication tests
test-cross-service: setup
	@echo "Running cross-service communication tests..."
	@go test $(TEST_FLAGS) -run "TestCrossServiceTestSuite" .

# Run data consistency tests
test-data: setup
	@echo "Running data consistency tests..."
	@go test $(TEST_FLAGS) -run "TestDataConsistencyTestSuite" .

# Run performance and reliability tests
test-performance: setup
	@echo "Running performance and reliability tests..."
	@go test $(TEST_FLAGS) -run "TestPerformanceTestSuite" .

# Run quick smoke tests (subset of tests for faster feedback)
test-quick: setup
	@echo "Running quick smoke tests..."
	@go test $(TEST_FLAGS) -run "TestAuthFlowTestSuite/TestUserLoginFlow|TestRBACTestSuite/TestPermissionChecking|TestCrossServiceTestSuite/TestAPIGatewayRouting" .

# Clean up test environment
clean:
	@echo "Cleaning up test environment..."
	@docker-compose -f docker-compose.test.yml -p pyairtable-integration-test down -v --remove-orphans 2>/dev/null || true
	@docker system prune -f --volumes 2>/dev/null || true
	@echo "Cleanup complete"

# Show service logs
logs:
	@echo "Showing service logs..."
	@docker-compose -f docker-compose.test.yml -p pyairtable-integration-test logs --tail=100

# Show specific service logs
logs-%:
	@echo "Showing logs for service: $*"
	@docker-compose -f docker-compose.test.yml -p pyairtable-integration-test logs --tail=100 $*

# Show test environment status
status:
	@echo "Test environment status:"
	@docker-compose -f docker-compose.test.yml -p pyairtable-integration-test ps

# Start test environment without running tests (for debugging)
start-env:
	@echo "Starting test environment for debugging..."
	@docker-compose -f docker-compose.test.yml -p pyairtable-integration-test up -d
	@echo "Test environment started. Use 'make logs' to view logs and 'make stop-env' to stop."

# Stop test environment
stop-env:
	@echo "Stopping test environment..."
	@docker-compose -f docker-compose.test.yml -p pyairtable-integration-test down

# Run tests with coverage
test-coverage: setup
	@echo "Running tests with coverage..."
	@go test $(TEST_FLAGS) -coverprofile=coverage.out .
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run tests in CI mode (for automated pipelines)
test-ci: setup
	@echo "Running tests in CI mode..."
	@go test -timeout $(TEST_TIMEOUT) -v -json . > test-results.json
	@go test -timeout $(TEST_TIMEOUT) -v . | tee test-output.log

# Validate test environment configuration
validate:
	@echo "Validating test environment configuration..."
	@docker-compose -f docker-compose.test.yml config >/dev/null
	@echo "Docker Compose configuration is valid"
	@go mod verify
	@echo "Go module verification passed"
	@echo "Test environment configuration is valid"

# Generate test documentation
docs:
	@echo "Generating test documentation..."
	@go doc -all . > test-documentation.txt
	@echo "Test documentation generated: test-documentation.txt"

# Benchmark tests (performance tests with benchmarking)
benchmark: setup
	@echo "Running benchmark tests..."
	@go test -bench=. -benchmem .

# Run specific test by name
test-name-%: setup
	@echo "Running test: $*"
	@go test $(TEST_FLAGS) -run "$*" .

# Debug mode - start environment and wait
debug: start-env
	@echo "Debug mode: Test environment is running"
	@echo "Services available at:"
	@echo "  API Gateway:         http://localhost:8080"
	@echo "  Platform Services:   http://localhost:8081" 
	@echo "  Permission Service:  http://localhost:8085"
	@echo "  Automation Services: http://localhost:8082"
	@echo ""
	@echo "Press Ctrl+C to stop and clean up..."
	@trap 'make stop-env' INT; sleep infinity

# Check service health
health-check:
	@echo "Checking service health..."
	@curl -f -s http://localhost:8080/health >/dev/null && echo "✓ API Gateway: healthy" || echo "✗ API Gateway: unhealthy"
	@curl -f -s http://localhost:8081/health >/dev/null && echo "✓ Platform Services: healthy" || echo "✗ Platform Services: unhealthy"
	@curl -f -s http://localhost:8085/health >/dev/null && echo "✓ Permission Service: healthy" || echo "✗ Permission Service: unhealthy"
	@curl -f -s http://localhost:8082/health >/dev/null && echo "✓ Automation Services: healthy" || echo "✗ Automation Services: unhealthy"