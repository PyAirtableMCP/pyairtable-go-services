#!/bin/bash

# Test script for DDD Event System
# This script tests the complete event flow in PyAirtable

set -e

echo "🚀 Testing DDD Event System for PyAirtable"
echo "========================================="

# Configuration
USER_SERVICE_URL="http://localhost:8080"
EVENT_ENDPOINT="$USER_SERVICE_URL/events"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

# Function to test HTTP endpoint
test_endpoint() {
    local method=$1
    local url=$2
    local description=$3
    local expected_status=${4:-200}
    
    echo -n "Testing $description... "
    
    response=$(curl -s -w "\n%{http_code}" -X "$method" "$url" || echo "000")
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n -1)
    
    if [ "$http_code" = "$expected_status" ]; then
        print_status "OK ($http_code)"
        if [ ! -z "$body" ]; then
            echo "Response: $(echo "$body" | jq -c '.' 2>/dev/null || echo "$body")"
        fi
        echo
        return 0
    else
        print_error "FAILED ($http_code)"
        echo "Response: $body"
        echo
        return 1
    fi
}

# Wait for service to be ready
wait_for_service() {
    echo "Waiting for user service to be ready..."
    local max_attempts=30
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        if curl -s "$USER_SERVICE_URL/health" > /dev/null 2>&1; then
            print_status "User service is ready"
            return 0
        fi
        
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done
    
    print_error "User service is not responding after $max_attempts attempts"
    exit 1
}

# Check if dependencies are available
check_dependencies() {
    echo "Checking dependencies..."
    
    if ! command -v curl &> /dev/null; then
        print_error "curl is required but not installed"
        exit 1
    fi
    
    if ! command -v jq &> /dev/null; then
        print_warning "jq is not installed - JSON output will not be formatted"
    fi
    
    print_status "Dependencies checked"
    echo
}

# Test basic service health
test_basic_health() {
    echo "=== Testing Basic Service Health ==="
    test_endpoint "GET" "$USER_SERVICE_URL/health" "User service health check"
    test_endpoint "GET" "$EVENT_ENDPOINT/health" "Event system health check"
}

# Test event system endpoints
test_event_endpoints() {
    echo "=== Testing Event System Endpoints ==="
    
    # Test event statistics
    test_endpoint "GET" "$EVENT_ENDPOINT/stats" "Event processing statistics"
    
    # Test event list
    test_endpoint "GET" "$EVENT_ENDPOINT/list" "Recent events list"
    
    # Test user projections
    test_endpoint "GET" "$EVENT_ENDPOINT/projections/users" "User projections read model"
}

# Test complete event flow
test_event_flow() {
    echo "=== Testing Complete Event Flow ==="
    
    # Run the demo event flow
    echo "Running complete event flow demonstration..."
    test_endpoint "POST" "$EVENT_ENDPOINT/demo" "Complete event flow demo"
    
    # Wait a moment for event processing
    echo "Waiting for event processing..."
    sleep 3
    
    # Check stats after demo
    echo "Checking statistics after demo..."
    test_endpoint "GET" "$EVENT_ENDPOINT/stats" "Post-demo statistics"
    
    # Check projections after demo
    echo "Checking projections after demo..."
    test_endpoint "GET" "$EVENT_ENDPOINT/projections/users" "Post-demo user projections"
}

# Test event replay
test_event_replay() {
    echo "=== Testing Event Replay ==="
    
    # Test replaying user.registered events
    test_endpoint "POST" "$EVENT_ENDPOINT/replay?type=user.registered&hours=1" "Replay user.registered events"
    
    # Wait for replay processing
    sleep 2
    
    # Check stats after replay
    test_endpoint "GET" "$EVENT_ENDPOINT/stats" "Post-replay statistics"
}

# Main test execution
main() {
    echo "Starting DDD Event System Tests"
    echo "Time: $(date)"
    echo
    
    check_dependencies
    wait_for_service
    
    echo "Starting tests..."
    echo
    
    # Run all test suites
    test_basic_health
    echo
    
    test_event_endpoints
    echo
    
    test_event_flow
    echo
    
    test_event_replay
    echo
    
    print_status "All tests completed successfully!"
    echo
    echo "🎉 DDD Event System is working correctly!"
    echo
    echo "You can now:"
    echo "  • View events: curl $EVENT_ENDPOINT/list"
    echo "  • Check stats: curl $EVENT_ENDPOINT/stats"
    echo "  • Run demo: curl -X POST $EVENT_ENDPOINT/demo"
    echo "  • View projections: curl $EVENT_ENDPOINT/projections/users"
    echo
}

# Handle script interruption
trap 'echo; print_error "Tests interrupted"; exit 1' INT

# Run main function
main "$@"