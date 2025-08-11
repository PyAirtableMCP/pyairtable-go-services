#!/bin/bash

# PyAirtable Platform Service Endpoint Test Script

set -euo pipefail

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PLATFORM_SERVICE_URL="http://localhost:8007"
API_GATEWAY_URL="http://localhost:8080"

print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Test function
test_endpoint() {
    local method="$1"
    local url="$2"
    local description="$3"
    local expected_status="${4:-200}"
    
    print_status "Testing: $description"
    echo "  Method: $method"
    echo "  URL: $url"
    
    # Make the request
    response=$(curl -s -w "\nHTTP_STATUS:%{http_code}" -X "$method" "$url" || echo "CURL_ERROR")
    
    if [[ "$response" == "CURL_ERROR" ]]; then
        print_error "  Failed to connect to $url"
        return 1
    fi
    
    # Extract status code
    status_code=$(echo "$response" | grep "HTTP_STATUS:" | cut -d: -f2)
    response_body=$(echo "$response" | sed '/HTTP_STATUS:/d')
    
    echo "  Status: $status_code"
    echo "  Response: $response_body"
    
    if [[ "$status_code" == "$expected_status" ]]; then
        print_success "  ✓ Test passed"
        return 0
    else
        print_error "  ✗ Expected status $expected_status, got $status_code"
        return 1
    fi
    echo ""
}

# Wait for service to be ready
wait_for_service() {
    local url="$1"
    local service_name="$2"
    local timeout=30
    local count=0
    
    print_status "Waiting for $service_name to be ready..."
    
    while [ $count -lt $timeout ]; do
        if curl -s -f "$url/health" > /dev/null 2>&1; then
            print_success "$service_name is ready!"
            return 0
        fi
        
        count=$((count + 1))
        echo -n "."
        sleep 1
    done
    
    print_error "$service_name failed to start within $timeout seconds"
    return 1
}

main() {
    print_status "Starting PyAirtable Platform Service Endpoint Tests"
    echo ""
    
    # Test if services are running
    print_status "Checking if services are running..."
    
    if wait_for_service "$PLATFORM_SERVICE_URL" "Platform Service"; then
        echo ""
        
        # Test direct platform service endpoints
        print_status "Testing Platform Service Direct Endpoints:"
        echo ""
        
        test_endpoint "GET" "$PLATFORM_SERVICE_URL/health" "Health check" 200
        echo ""
        
        # Test public endpoints (no auth required)
        test_endpoint "GET" "$PLATFORM_SERVICE_URL/api/v1/public/tenants/validate-slug?slug=test" "Tenant slug validation" 200
        echo ""
        
        # Test auth endpoint (should return error without credentials)
        test_endpoint "GET" "$PLATFORM_SERVICE_URL/api/v1/users" "User list (no auth - should fail)" 401
        echo ""
        
        print_success "Platform Service tests completed!"
    else
        print_error "Platform Service is not running. Please start it with:"
        echo "cd /Users/kg/IdeaProjects/pyairtable-compose/go-services/pyairtable-platform"
        echo "docker-compose up platform-service"
        exit 1
    fi
    
    # Test API Gateway routing (if available)
    if curl -s -f "$API_GATEWAY_URL/health" > /dev/null 2>&1; then
        echo ""
        print_status "Testing API Gateway Routing:"
        echo ""
        
        test_endpoint "GET" "$API_GATEWAY_URL/health" "API Gateway health check" 200
        echo ""
        
        # These would need auth, but we can test that they route correctly by checking the auth error
        test_endpoint "GET" "$API_GATEWAY_URL/api/v1/users" "User list via gateway (should route to platform service)" 401
        echo ""
        
        print_success "API Gateway tests completed!"
    else
        print_warning "API Gateway is not running - skipping gateway tests"
    fi
    
    print_success "All endpoint tests completed successfully!"
    echo ""
    print_status "Summary:"
    echo "✓ Platform Service is running on port 8007"
    echo "✓ Health endpoints are accessible"
    echo "✓ Authentication is working (returns 401 for protected endpoints)"
    echo "✓ Public endpoints are accessible"
    if curl -s -f "$API_GATEWAY_URL/health" > /dev/null 2>&1; then
        echo "✓ API Gateway routing is configured correctly"
    fi
}

main "$@"