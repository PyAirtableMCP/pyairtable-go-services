#!/bin/bash

# PyAirtable Integration Test Runner
# Comprehensive script for running integration tests with detailed reporting

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
TEST_TIMEOUT="${TEST_TIMEOUT:-30m}"
TEST_PARALLEL="${TEST_PARALLEL:-1}"
VERBOSE="${VERBOSE:-false}"
CLEANUP="${CLEANUP:-true}"
OUTPUT_DIR="${OUTPUT_DIR:-${SCRIPT_DIR}/test-results}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_debug() {
    if [[ "${VERBOSE}" == "true" ]]; then
        echo -e "${PURPLE}[DEBUG]${NC} $1"
    fi
}

# Print usage information
usage() {
    cat << EOF
PyAirtable Integration Test Runner

Usage: $0 [OPTIONS] [TEST_SUITE]

TEST_SUITE options:
    all            Run all test suites (default)
    auth           Run authentication flow tests
    rbac           Run RBAC system tests
    cross-service  Run cross-service communication tests
    data           Run data consistency tests
    performance    Run performance and reliability tests
    quick          Run quick smoke tests

OPTIONS:
    -h, --help          Show this help message
    -v, --verbose       Enable verbose output
    -t, --timeout       Test timeout (default: 30m)
    -p, --parallel      Number of parallel tests (default: 1)
    -o, --output-dir    Output directory for test results
    --no-cleanup        Skip cleanup after tests
    --ci                Run in CI mode with structured output
    --debug             Start test environment and wait (for debugging)
    --validate-only     Only validate configuration, don't run tests

Examples:
    $0                                    # Run all tests
    $0 auth                              # Run only auth tests
    $0 --verbose --timeout 60m all      # Run all tests with verbose output and 60m timeout
    $0 --debug                           # Start environment for debugging
    $0 --ci > test-results.json         # Run in CI mode with JSON output

Environment Variables:
    TEST_TIMEOUT    Test timeout (default: 30m)
    TEST_PARALLEL   Number of parallel tests (default: 1)
    VERBOSE         Enable verbose output (true/false)
    CLEANUP         Clean up after tests (true/false)
    OUTPUT_DIR      Output directory for results

EOF
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                usage
                exit 0
                ;;
            -v|--verbose)
                VERBOSE="true"
                shift
                ;;
            -t|--timeout)
                TEST_TIMEOUT="$2"
                shift 2
                ;;
            -p|--parallel)
                TEST_PARALLEL="$2"
                shift 2
                ;;
            -o|--output-dir)
                OUTPUT_DIR="$2"
                shift 2
                ;;
            --no-cleanup)
                CLEANUP="false"
                shift
                ;;
            --ci)
                CI_MODE="true"
                shift
                ;;
            --debug)
                DEBUG_MODE="true"
                shift
                ;;
            --validate-only)
                VALIDATE_ONLY="true"
                shift
                ;;
            auth|rbac|cross-service|data|performance|quick|all)
                TEST_SUITE="$1"
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done
}

# Initialize variables
TEST_SUITE="${TEST_SUITE:-all}"
CI_MODE="${CI_MODE:-false}"
DEBUG_MODE="${DEBUG_MODE:-false}"
VALIDATE_ONLY="${VALIDATE_ONLY:-false}"

# Validate environment
validate_environment() {
    log_info "Validating environment..."
    
    # Check required tools
    local required_tools=("docker" "docker-compose" "go" "curl")
    for tool in "${required_tools[@]}"; do
        if ! command -v "$tool" &> /dev/null; then
            log_error "$tool is required but not installed"
            exit 1
        fi
        log_debug "$tool is available"
    done
    
    # Check Docker daemon
    if ! docker info &> /dev/null; then
        log_error "Docker daemon is not running"
        exit 1
    fi
    
    # Check Go version
    local go_version
    go_version=$(go version | awk '{print $3}' | sed 's/go//')
    log_debug "Go version: $go_version"
    
    # Validate Docker Compose configuration
    cd "$SCRIPT_DIR"
    if ! docker-compose -f docker-compose.test.yml config &> /dev/null; then
        log_error "Docker Compose configuration is invalid"
        exit 1
    fi
    
    # Validate Go module
    if ! go mod verify &> /dev/null; then
        log_error "Go module verification failed"
        exit 1
    fi
    
    log_success "Environment validation passed"
}

# Setup test environment
setup_environment() {
    log_info "Setting up test environment..."
    
    # Create output directory
    mkdir -p "$OUTPUT_DIR"
    
    # Download Go dependencies
    cd "$SCRIPT_DIR"
    go mod download
    go mod tidy
    
    log_success "Test environment setup completed"
}

# Start test services
start_services() {
    log_info "Starting test services..."
    
    cd "$SCRIPT_DIR"
    
    # Clean up any existing containers
    docker-compose -f docker-compose.test.yml -p pyairtable-integration-test down -v --remove-orphans &> /dev/null || true
    
    # Start services
    if [[ "${VERBOSE}" == "true" ]]; then
        docker-compose -f docker-compose.test.yml -p pyairtable-integration-test up -d --build
    else
        docker-compose -f docker-compose.test.yml -p pyairtable-integration-test up -d --build &> /dev/null
    fi
    
    log_success "Test services started"
}

# Wait for services to be ready
wait_for_services() {
    log_info "Waiting for services to be ready..."
    
    local services=(
        "API Gateway:http://localhost:8080/health"
        "Platform Services:http://localhost:8081/health"
        "Permission Service:http://localhost:8085/health"
        "Automation Services:http://localhost:8082/health"
    )
    
    local max_wait=120
    local wait_interval=2
    local elapsed=0
    
    for service_info in "${services[@]}"; do
        local service_name="${service_info%%:*}"
        local health_url="${service_info##*:}"
        
        log_info "Waiting for $service_name..."
        
        while (( elapsed < max_wait )); do
            if curl -f -s "$health_url" -H "X-API-Key: test-api-key-12345" &> /dev/null; then
                log_success "$service_name is ready"
                break
            fi
            
            sleep $wait_interval
            ((elapsed += wait_interval))
            
            if (( elapsed >= max_wait )); then
                log_error "$service_name failed to become ready within ${max_wait}s"
                show_service_logs
                exit 1
            fi
        done
        
        elapsed=0
    done
    
    # Extra wait for full initialization
    log_info "Waiting for services to fully initialize..."
    sleep 5
    
    log_success "All services are ready"
}

# Show service logs for debugging
show_service_logs() {
    log_info "Service logs:"
    cd "$SCRIPT_DIR"
    docker-compose -f docker-compose.test.yml -p pyairtable-integration-test logs --tail=50
}

# Run specific test suite
run_test_suite() {
    local suite="$1"
    local test_pattern=""
    
    case "$suite" in
        auth)
            test_pattern="TestAuthFlowTestSuite"
            ;;
        rbac)
            test_pattern="TestRBACTestSuite"
            ;;
        cross-service)
            test_pattern="TestCrossServiceTestSuite"
            ;;
        data)
            test_pattern="TestDataConsistencyTestSuite"
            ;;
        performance)
            test_pattern="TestPerformanceTestSuite"
            ;;
        quick)
            test_pattern="TestAuthFlowTestSuite/TestUserLoginFlow|TestRBACTestSuite/TestPermissionChecking|TestCrossServiceTestSuite/TestAPIGatewayRouting"
            ;;
        all)
            test_pattern="TestAuthFlowTestSuite|TestRBACTestSuite|TestCrossServiceTestSuite|TestDataConsistencyTestSuite|TestPerformanceTestSuite"
            ;;
        *)
            log_error "Unknown test suite: $suite"
            exit 1
            ;;
    esac
    
    log_info "Running test suite: $suite"
    
    cd "$SCRIPT_DIR"
    
    local test_flags="-timeout ${TEST_TIMEOUT} -p ${TEST_PARALLEL}"
    if [[ "${VERBOSE}" == "true" ]]; then
        test_flags+=" -v"
    fi
    
    local output_file="${OUTPUT_DIR}/test-${suite}-$(date +%Y%m%d-%H%M%S).log"
    
    if [[ "${CI_MODE}" == "true" ]]; then
        # CI mode with structured output
        go test $test_flags -json -run "$test_pattern" . | tee "${output_file}"
    else
        # Regular mode
        if go test $test_flags -run "$test_pattern" . | tee "${output_file}"; then
            log_success "Test suite '$suite' passed"
            return 0
        else
            log_error "Test suite '$suite' failed"
            return 1
        fi
    fi
}

# Cleanup test environment
cleanup_environment() {
    if [[ "${CLEANUP}" == "true" ]]; then
        log_info "Cleaning up test environment..."
        
        cd "$SCRIPT_DIR"
        docker-compose -f docker-compose.test.yml -p pyairtable-integration-test down -v --remove-orphans &> /dev/null || true
        
        log_success "Cleanup completed"
    else
        log_info "Cleanup skipped (use --no-cleanup to disable cleanup)"
    fi
}

# Generate test report
generate_report() {
    local suite="$1"
    local exit_code="$2"
    
    log_info "Generating test report..."
    
    local report_file="${OUTPUT_DIR}/test-report-${suite}-$(date +%Y%m%d-%H%M%S).md"
    
    cat > "$report_file" << EOF
# PyAirtable Integration Test Report

## Test Execution Summary

- **Test Suite**: $suite
- **Date**: $(date)
- **Exit Code**: $exit_code
- **Status**: $([ $exit_code -eq 0 ] && echo "PASSED" || echo "FAILED")
- **Timeout**: $TEST_TIMEOUT
- **Parallel Tests**: $TEST_PARALLEL

## Environment Information

- **Go Version**: $(go version)
- **Docker Version**: $(docker --version)
- **Docker Compose Version**: $(docker-compose --version)

## Service Status

EOF
    
    # Add service status to report
    cd "$SCRIPT_DIR"
    if docker-compose -f docker-compose.test.yml -p pyairtable-integration-test ps &> /dev/null; then
        echo "### Running Services" >> "$report_file"
        echo '```' >> "$report_file"
        docker-compose -f docker-compose.test.yml -p pyairtable-integration-test ps >> "$report_file"
        echo '```' >> "$report_file"
    fi
    
    log_success "Test report generated: $report_file"
}

# Debug mode - start environment and wait
debug_mode() {
    log_info "Starting debug mode..."
    
    start_services
    wait_for_services
    
    cat << EOF

${GREEN}Debug Mode: Test environment is running${NC}

Services available at:
  ${CYAN}API Gateway:${NC}         http://localhost:8080
  ${CYAN}Platform Services:${NC}   http://localhost:8081
  ${CYAN}Permission Service:${NC}  http://localhost:8085
  ${CYAN}Automation Services:${NC} http://localhost:8082

${YELLOW}Useful commands:${NC}
  make logs                   # Show all service logs
  make logs-api-gateway       # Show specific service logs
  make health-check           # Check service health
  curl -H "X-API-Key: test-api-key-12345" http://localhost:8080/health

${YELLOW}Press Ctrl+C to stop and clean up...${NC}

EOF
    
    # Wait for interrupt signal
    trap cleanup_environment INT
    sleep infinity
}

# Main execution function
main() {
    log_info "Starting PyAirtable Integration Test Runner"
    
    # Parse command line arguments
    parse_args "$@"
    
    # Handle special modes
    if [[ "${DEBUG_MODE}" == "true" ]]; then
        validate_environment
        debug_mode
        exit 0
    fi
    
    if [[ "${VALIDATE_ONLY}" == "true" ]]; then
        validate_environment
        setup_environment
        log_success "Validation completed successfully"
        exit 0
    fi
    
    local exit_code=0
    
    # Main test execution
    {
        validate_environment
        setup_environment
        start_services
        wait_for_services
        
        if ! run_test_suite "$TEST_SUITE"; then
            exit_code=1
        fi
        
    } || {
        exit_code=1
        log_error "Test execution failed"
        show_service_logs
    }
    
    # Generate report
    generate_report "$TEST_SUITE" "$exit_code"
    
    # Cleanup
    cleanup_environment
    
    if [[ $exit_code -eq 0 ]]; then
        log_success "All tests completed successfully!"
    else
        log_error "Some tests failed. Check the logs for details."
    fi
    
    exit $exit_code
}

# Run main function with all arguments
main "$@"