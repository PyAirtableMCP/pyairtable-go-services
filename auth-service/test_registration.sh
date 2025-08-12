#!/bin/bash

# Test Registration Flow for Auth Service
# This script demonstrates the user registration functionality

echo "=== Auth Service Registration Test ==="
echo ""

# Service endpoint
BASE_URL="http://localhost:8001"

echo "Testing registration endpoint..."
echo ""

# Test 1: Valid registration
echo "1. Testing valid user registration:"
curl -X POST "$BASE_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "john.doe@example.com",
    "password": "SecurePassword123",
    "first_name": "John",
    "last_name": "Doe"
  }' | jq '.'

echo ""
echo ""

# Test 2: Login with registered user
echo "2. Testing login with registered user:"
TOKEN_RESPONSE=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "john.doe@example.com",
    "password": "SecurePassword123"
  }')

echo "$TOKEN_RESPONSE" | jq '.'

# Extract token for further requests
TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.access_token')
echo ""
echo "Extracted token: $TOKEN"
echo ""

# Test 3: Duplicate email registration (should fail)
echo "3. Testing duplicate email registration (should fail):"
curl -X POST "$BASE_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "john.doe@example.com",
    "password": "AnotherPassword123",
    "first_name": "Jane",
    "last_name": "Smith"
  }' | jq '.'

echo ""
echo ""

# Test 4: Invalid password (too short)
echo "4. Testing invalid password (too short, should fail):"
curl -X POST "$BASE_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "jane.smith@example.com",
    "password": "123",
    "first_name": "Jane",
    "last_name": "Smith"
  }' | jq '.'

echo ""
echo ""

# Test 5: Missing required fields
echo "5. Testing missing required fields (should fail):"
curl -X POST "$BASE_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "incomplete@example.com",
    "password": "ValidPassword123"
  }' | jq '.'

echo ""
echo ""

# Test 6: Get user profile (if login was successful)
if [ "$TOKEN" != "null" ] && [ -n "$TOKEN" ]; then
    echo "6. Testing user profile retrieval:"
    curl -X GET "$BASE_URL/auth/me" \
      -H "Authorization: Bearer $TOKEN" | jq '.'
else
    echo "6. Skipping profile test - no valid token"
fi

echo ""
echo "=== Test Complete ==="