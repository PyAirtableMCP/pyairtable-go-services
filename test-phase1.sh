#!/bin/bash

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🧪 Testing Phase 1 Services..."
echo ""

# Base URLs
API_GATEWAY="http://localhost:8080"
AUTH_SERVICE="http://localhost:8001"
USER_SERVICE="http://localhost:8002"
AIRTABLE_GATEWAY="http://localhost:8003"

# Test health endpoints
echo "📡 Testing Health Endpoints..."
echo "------------------------"

# API Gateway health
echo -n "API Gateway: "
if curl -s "$API_GATEWAY/health" | grep -q "healthy"; then
    echo -e "${GREEN}✓ Healthy${NC}"
else
    echo -e "${RED}✗ Unhealthy${NC}"
fi

# Auth Service health
echo -n "Auth Service: "
if curl -s "$AUTH_SERVICE/health" | grep -q "healthy"; then
    echo -e "${GREEN}✓ Healthy${NC}"
else
    echo -e "${RED}✗ Unhealthy${NC}"
fi

# User Service health
echo -n "User Service: "
if curl -s "$USER_SERVICE/health" | grep -q "healthy"; then
    echo -e "${GREEN}✓ Healthy${NC}"
else
    echo -e "${RED}✗ Unhealthy${NC}"
fi

# Airtable Gateway health
echo -n "Airtable Gateway: "
if curl -s "$AIRTABLE_GATEWAY/health" | grep -q "healthy"; then
    echo -e "${GREEN}✓ Healthy${NC}"
else
    echo -e "${RED}✗ Unhealthy${NC}"
fi

echo ""
echo "🔐 Testing Auth Flow..."
echo "------------------------"

# Generate random email
RANDOM_EMAIL="test$(date +%s)@example.com"
PASSWORD="TestPassword123!"

# Test registration through API Gateway
echo -n "Registration: "
REGISTER_RESPONSE=$(curl -s -X POST "$API_GATEWAY/api/v1/auth/register" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$RANDOM_EMAIL\",\"password\":\"$PASSWORD\",\"first_name\":\"Test\",\"last_name\":\"User\"}")

if echo "$REGISTER_RESPONSE" | grep -q "id"; then
    echo -e "${GREEN}✓ Success${NC}"
    USER_ID=$(echo "$REGISTER_RESPONSE" | jq -r '.id' 2>/dev/null)
    echo "  User ID: $USER_ID"
else
    echo -e "${RED}✗ Failed${NC}"
    echo "  Response: $REGISTER_RESPONSE"
fi

# Test login
echo -n "Login: "
LOGIN_RESPONSE=$(curl -s -X POST "$API_GATEWAY/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$RANDOM_EMAIL\",\"password\":\"$PASSWORD\"}")

if echo "$LOGIN_RESPONSE" | grep -q "access_token"; then
    echo -e "${GREEN}✓ Success${NC}"
    ACCESS_TOKEN=$(echo "$LOGIN_RESPONSE" | jq -r '.access_token' 2>/dev/null)
    REFRESH_TOKEN=$(echo "$LOGIN_RESPONSE" | jq -r '.refresh_token' 2>/dev/null)
    echo "  Token acquired"
else
    echo -e "${RED}✗ Failed${NC}"
    echo "  Response: $LOGIN_RESPONSE"
fi

# Test token validation
if [ ! -z "$ACCESS_TOKEN" ]; then
    echo -n "Token Validation: "
    VALIDATE_RESPONSE=$(curl -s -X POST "$API_GATEWAY/api/v1/auth/validate" \
        -H "Authorization: Bearer $ACCESS_TOKEN")
    
    if echo "$VALIDATE_RESPONSE" | grep -q "user_id"; then
        echo -e "${GREEN}✓ Success${NC}"
    else
        echo -e "${RED}✗ Failed${NC}"
        echo "  Response: $VALIDATE_RESPONSE"
    fi
fi

echo ""
echo "🎯 Testing API Gateway Routing..."
echo "------------------------"

# Test protected endpoint
if [ ! -z "$ACCESS_TOKEN" ]; then
    echo -n "Protected Endpoint (/api/v1/users/me): "
    ME_RESPONSE=$(curl -s -X GET "$API_GATEWAY/api/v1/users/me" \
        -H "Authorization: Bearer $ACCESS_TOKEN")
    
    if echo "$ME_RESPONSE" | grep -q "user_id"; then
        echo -e "${GREEN}✓ Success${NC}"
    else
        echo -e "${YELLOW}⚠ Not Implemented${NC}"
    fi
fi

# Test Airtable Gateway (if PAT is available)
if [ ! -z "$AIRTABLE_PAT" ]; then
    echo ""
    echo "🗄️  Testing Airtable Gateway..."
    echo "------------------------"
    
    echo -n "List Bases: "
    BASES_RESPONSE=$(curl -s -X GET "$API_GATEWAY/api/v1/airtable/bases" \
        -H "Authorization: Bearer $ACCESS_TOKEN")
    
    if echo "$BASES_RESPONSE" | grep -q "bases"; then
        echo -e "${GREEN}✓ Success${NC}"
    else
        echo -e "${YELLOW}⚠ Check Airtable PAT${NC}"
    fi
fi

echo ""
echo "✅ Phase 1 Service Tests Complete!"