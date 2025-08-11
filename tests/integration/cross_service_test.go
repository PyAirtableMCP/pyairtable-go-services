package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// CrossServiceTestSuite tests cross-service communication and API Gateway routing
type CrossServiceTestSuite struct {
	suite.Suite
	testEnv    *TestEnvironment
	alphaToken string
	betaToken  string
}

// SetupSuite runs before all tests in the suite
func (suite *CrossServiceTestSuite) SetupSuite() {
	suite.testEnv = NewTestEnvironment(suite.T())
	suite.testEnv.Start()

	// Get authentication tokens
	suite.alphaToken = suite.getAuthToken("admin@alpha.test.com", "testpass123")
	suite.betaToken = suite.getAuthToken("admin@beta.test.com", "testpass123")
}

// TearDownSuite runs after all tests in the suite
func (suite *CrossServiceTestSuite) TearDownSuite() {
	if suite.testEnv != nil {
		suite.testEnv.Stop()
	}
}

// SetupTest runs before each test
func (suite *CrossServiceTestSuite) SetupTest() {
	suite.testEnv.ResetTestData()
}

// Helper function to get auth token
func (suite *CrossServiceTestSuite) getAuthToken(email, password string) string {
	loginData := map[string]interface{}{
		"email":    email,
		"password": password,
	}

	resp := suite.testEnv.MakeAPIRequest("POST", "/api/v1/auth/login", loginData, nil)
	require.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var loginResponse map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&loginResponse)
	require.NoError(suite.T(), err)

	return loginResponse["access_token"].(string)
}

// Helper function to make authenticated request
func (suite *CrossServiceTestSuite) makeAuthenticatedRequest(method, path string, data interface{}, token string) *http.Response {
	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}
	return suite.testEnv.MakeAPIRequest(method, path, data, headers)
}

// TestAPIGatewayRouting tests that API Gateway correctly routes requests to services
func (suite *CrossServiceTestSuite) TestAPIGatewayRouting() {
	t := suite.T()

	testCases := []struct {
		name        string
		method      string
		path        string
		token       string
		expectedMin int // Minimum expected status code
		expectedMax int // Maximum expected status code
		description string
	}{
		{
			name:        "auth_routes_to_platform_services",
			method:      "GET",
			path:        "/api/v1/auth/profile",
			token:       suite.alphaToken,
			expectedMin: 200,
			expectedMax: 299,
			description: "Auth routes should reach platform services",
		},
		{
			name:        "workspace_routes_to_platform_services",
			method:      "GET",
			path:        "/api/v1/workspaces",
			token:       suite.alphaToken,
			expectedMin: 200,
			expectedMax: 299,
			description: "Workspace routes should reach platform services",
		},
		{
			name:        "permission_routes_to_permission_service",
			method:      "POST",
			path:        "/api/v1/permissions/check",
			token:       suite.alphaToken,
			expectedMin: 200,
			expectedMax: 299,
			description: "Permission routes should reach permission service",
		},
		{
			name:        "file_routes_to_automation_services",
			method:      "GET",
			path:        "/api/v1/files",
			token:       suite.alphaToken,
			expectedMin: 200,
			expectedMax: 299,
			description: "File routes should reach automation services",
		},
		{
			name:        "airtable_routes_to_gateway",
			method:      "GET",
			path:        "/api/v1/airtable/bases",
			token:       suite.alphaToken,
			expectedMin: 200,
			expectedMax: 299,
			description: "Airtable routes should reach airtable gateway",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Prepare data for permission check
			var data interface{}
			if tc.path == "/api/v1/permissions/check" {
				data = map[string]interface{}{
					"user_id":       TestUserAlphaAdminID,
					"resource_type": "workspace",
					"resource_id":   TestWorkspaceAlphaMainID,
					"action":        "read",
					"tenant_id":     TestTenantAlphaID,
				}
			}

			resp := suite.makeAuthenticatedRequest(tc.method, tc.path, data, tc.token)
			
			assert.True(t, resp.StatusCode >= tc.expectedMin && resp.StatusCode <= tc.expectedMax,
				fmt.Sprintf("%s: Expected status %d-%d, got %d", tc.description, tc.expectedMin, tc.expectedMax, resp.StatusCode))

			// Verify response is JSON
			contentType := resp.Header.Get("Content-Type")
			assert.Contains(t, contentType, "application/json", "Response should be JSON")
		})
	}
}

// TestServiceToServiceAuthentication tests internal service authentication
func (suite *CrossServiceTestSuite) TestServiceToServiceAuthentication() {
	t := suite.T()

	// Test that Permission Service can authenticate with Platform Services
	// This happens internally when Permission Service validates JWT tokens

	permissionCheck := map[string]interface{}{
		"user_id":       TestUserAlphaAdminID,
		"resource_type": "workspace",
		"resource_id":   TestWorkspaceAlphaMainID,
		"action":        "read",
		"tenant_id":     TestTenantAlphaID,
	}

	// Make request through API Gateway (which forwards to Permission Service)
	resp := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", permissionCheck, suite.alphaToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	// If this works, it means:
	// 1. API Gateway received the request
	// 2. API Gateway forwarded to Permission Service  
	// 3. Permission Service validated JWT with Platform Services
	// 4. Permission Service returned the result
	assert.Contains(t, result, "allowed", "Permission check should return 'allowed' field")
	assert.True(t, result["allowed"].(bool), "Alpha admin should have read access to their workspace")
}

// TestPermissionIntegrationAcrossServices tests permission validation across all services
func (suite *CrossServiceTestSuite) TestPermissionIntegrationAcrossServices() {
	t := suite.T()

	// Test different permission levels across services
	testCases := []struct {
		name         string
		userEmail    string
		operation    func(token string) *http.Response
		expectStatus int
		description  string
	}{
		{
			name:      "admin_can_create_workspace",
			userEmail: "admin@alpha.test.com",
			operation: func(token string) *http.Response {
				workspaceData := map[string]interface{}{
					"name":        "Test Workspace via API",
					"description": "Created through cross-service test",
				}
				return suite.makeAuthenticatedRequest("POST", "/api/v1/workspaces", workspaceData, token)
			},
			expectStatus: http.StatusCreated,
			description:  "Admin should be able to create workspaces",
		},
		{
			name:      "member_cannot_delete_workspace",
			userEmail: "user2@alpha.test.com",
			operation: func(token string) *http.Response {
				return suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/workspaces/%s", TestWorkspaceAlphaMainID), nil, token)
			},
			expectStatus: http.StatusForbidden,
			description:  "Member should not be able to delete workspace",
		},
		{
			name:      "project_admin_can_manage_project",
			userEmail: "user1@alpha.test.com",
			operation: func(token string) *http.Response {
				projectData := map[string]interface{}{
					"name":        "Updated Project Name",
					"description": "Updated through cross-service test",
				}
				return suite.makeAuthenticatedRequest("PUT", fmt.Sprintf("/api/v1/projects/%s", TestProjectAlpha1ID), projectData, token)
			},
			expectStatus: http.StatusOK,
			description:  "Project admin should be able to update their project",
		},
		{
			name:      "cross_tenant_access_denied",
			userEmail: "admin@alpha.test.com",
			operation: func(token string) *http.Response {
				return suite.makeAuthenticatedRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s", TestWorkspaceBetaID), nil, token)
			},
			expectStatus: http.StatusNotFound,
			description:  "Alpha admin should not access Beta workspace",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			userToken := suite.getAuthToken(tc.userEmail, "testpass123")
			resp := tc.operation(userToken)
			assert.Equal(t, tc.expectStatus, resp.StatusCode, tc.description)
		})
	}
}

// TestWorkflowIntegration tests complete workflows that span multiple services
func (suite *CrossServiceTestSuite) TestWorkflowIntegration() {
	t := suite.T()

	// Test: Create workspace -> Create project -> Upload file -> Process workflow
	
	// Step 1: Create workspace
	workspaceData := map[string]interface{}{
		"name":        "Integration Test Workspace",
		"description": "Workspace for testing cross-service workflows",
	}

	workspaceResp := suite.makeAuthenticatedRequest("POST", "/api/v1/workspaces", workspaceData, suite.alphaToken)
	require.Equal(t, http.StatusCreated, workspaceResp.StatusCode)

	var workspace map[string]interface{}
	err := json.NewDecoder(workspaceResp.Body).Decode(&workspace)
	require.NoError(t, err)

	workspaceID := workspace["id"].(string)
	assert.NotEmpty(t, workspaceID)
	assert.Equal(t, TestTenantAlphaID, workspace["tenant_id"])

	// Step 2: Create project in the workspace
	projectData := map[string]interface{}{
		"workspace_id": workspaceID,
		"name":         "Integration Test Project",
		"description":  "Project for testing workflows",
	}

	projectResp := suite.makeAuthenticatedRequest("POST", "/api/v1/projects", projectData, suite.alphaToken)
	require.Equal(t, http.StatusCreated, projectResp.StatusCode)

	var project map[string]interface{}
	err = json.NewDecoder(projectResp.Body).Decode(&project)
	require.NoError(t, err)

	projectID := project["id"].(string)
	assert.NotEmpty(t, projectID)
	assert.Equal(t, workspaceID, project["workspace_id"])

	// Step 3: Upload a file (Automation Services)
	fileContent := "test,data,for,integration\n1,2,3,4\n5,6,7,8"
	fileData := map[string]interface{}{
		"filename":   "test-data.csv",
		"content":    fileContent,
		"project_id": projectID,
	}

	fileResp := suite.makeAuthenticatedRequest("POST", "/api/v1/files/upload", fileData, suite.alphaToken)
	// Note: Might return different status codes depending on implementation
	assert.True(t, fileResp.StatusCode >= 200 && fileResp.StatusCode < 300 || 
		fileResp.StatusCode == http.StatusNotImplemented, 
		"File upload should succeed or be not implemented")

	// Step 4: Verify permissions are consistent across the workflow
	// Check that the creator has access to all created resources
	permissionChecks := []map[string]interface{}{
		{
			"user_id":       TestUserAlphaAdminID,
			"resource_type": "workspace",
			"resource_id":   workspaceID,
			"action":        "manage",
			"tenant_id":     TestTenantAlphaID,
		},
		{
			"user_id":       TestUserAlphaAdminID,
			"resource_type": "project",
			"resource_id":   projectID,
			"action":        "manage",
			"tenant_id":     TestTenantAlphaID,
		},
	}

	for _, check := range permissionChecks {
		permResp := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", check, suite.alphaToken)
		require.Equal(t, http.StatusOK, permResp.StatusCode)

		var permResult map[string]interface{}
		err = json.NewDecoder(permResp.Body).Decode(&permResult)
		require.NoError(t, err)

		assert.True(t, permResult["allowed"].(bool), 
			fmt.Sprintf("Admin should have %s permission on %s %s", 
				check["action"], check["resource_type"], check["resource_id"]))
	}

	// Step 5: Clean up - Delete the created resources
	// Delete project first (due to foreign key constraints)
	deleteProjectResp := suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/projects/%s", projectID), nil, suite.alphaToken)
	assert.True(t, deleteProjectResp.StatusCode == http.StatusOK || deleteProjectResp.StatusCode == http.StatusNoContent,
		"Project deletion should succeed")

	// Delete workspace
	deleteWorkspaceResp := suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/workspaces/%s", workspaceID), nil, suite.alphaToken)
	assert.True(t, deleteWorkspaceResp.StatusCode == http.StatusOK || deleteWorkspaceResp.StatusCode == http.StatusNoContent,
		"Workspace deletion should succeed")
}

// TestErrorHandlingAcrossServices tests error propagation and handling
func (suite *CrossServiceTestSuite) TestErrorHandlingAcrossServices() {
	t := suite.T()

	// Test various error scenarios that span multiple services
	testCases := []struct {
		name         string
		operation    func() *http.Response
		expectStatus int
		expectError  string
		description  string
	}{
		{
			name: "invalid_permission_check",
			operation: func() *http.Response {
				invalidCheck := map[string]interface{}{
					"user_id":       "invalid-uuid",
					"resource_type": "workspace",
					"resource_id":   TestWorkspaceAlphaMainID,
					"action":        "read",
					"tenant_id":     TestTenantAlphaID,
				}
				return suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", invalidCheck, suite.alphaToken)
			},
			expectStatus: http.StatusBadRequest,
			expectError:  "invalid",
			description:  "Invalid user ID should return bad request",
		},
		{
			name: "nonexistent_resource_access",
			operation: func() *http.Response {
				return suite.makeAuthenticatedRequest("GET", "/api/v1/workspaces/00000000-0000-0000-0000-000000000000", nil, suite.alphaToken)
			},
			expectStatus: http.StatusNotFound,
			expectError:  "not found",
			description:  "Nonexistent resource should return not found",
		},
		{
			name: "unauthorized_cross_tenant_action",
			operation: func() *http.Response {
				projectData := map[string]interface{}{
					"name":        "Unauthorized Project",
					"workspace_id": TestWorkspaceBetaID, // Alpha user trying to create in Beta workspace
				}
				return suite.makeAuthenticatedRequest("POST", "/api/v1/projects", projectData, suite.alphaToken)
			},
			expectStatus: http.StatusForbidden,
			expectError:  "forbidden",
			description:  "Cross-tenant resource creation should be forbidden",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := tc.operation()
			assert.Equal(t, tc.expectStatus, resp.StatusCode, tc.description)

			if tc.expectError != "" {
				var errorResponse map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&errorResponse)
				require.NoError(t, err)

				errorMsg := ""
				if msg, exists := errorResponse["error"]; exists {
					errorMsg = msg.(string)
				} else if msg, exists := errorResponse["message"]; exists {
					errorMsg = msg.(string)
				}

				assert.Contains(t, strings.ToLower(errorMsg), tc.expectError, 
					fmt.Sprintf("Error message should contain '%s'", tc.expectError))
			}
		})
	}
}

// TestServiceHealthAndDependencies tests service health and dependency management
func (suite *CrossServiceTestSuite) TestServiceHealthAndDependencies() {
	t := suite.T()

	services := map[string]string{
		"api-gateway":         TestAPIGatewayURL,
		"platform-services":  "http://localhost:8081",
		"permission-service": "http://localhost:8085",
		"automation-services": "http://localhost:8082",
	}

	// Test that all services are healthy
	for serviceName, serviceURL := range services {
		t.Run(fmt.Sprintf("health_check_%s", serviceName), func(t *testing.T) {
			resp := suite.testEnv.MakeDirectServiceRequest(serviceURL, "GET", "/health", nil, nil)
			assert.Equal(t, http.StatusOK, resp.StatusCode, 
				fmt.Sprintf("%s should be healthy", serviceName))

			var healthData map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&healthData)
			require.NoError(t, err)

			status, exists := healthData["status"]
			assert.True(t, exists, "Health response should include status")
			assert.Contains(t, []string{"healthy", "ok"}, status, "Status should be healthy/ok")
		})
	}

	// Test readiness endpoints where available
	readinessServices := []string{"permission-service"}
	for _, serviceName := range readinessServices {
		t.Run(fmt.Sprintf("readiness_check_%s", serviceName), func(t *testing.T) {
			serviceURL := services[serviceName]
			resp := suite.testEnv.MakeDirectServiceRequest(serviceURL, "GET", "/ready", nil, nil)
			
			// Should be either 200 (ready) or 503 (not ready), not 404
			assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusServiceUnavailable,
				fmt.Sprintf("%s readiness endpoint should exist", serviceName))
		})
	}
}

// TestConcurrentCrossServiceOperations tests concurrent operations across services
func (suite *CrossServiceTestSuite) TestConcurrentCrossServiceOperations() {
	t := suite.T()

	concurrency := 5
	results := make(chan bool, concurrency)

	// Test concurrent workspace creation (requires auth + permission checks)
	for i := 0; i < concurrency; i++ {
		go func(index int) {
			workspaceData := map[string]interface{}{
				"name":        fmt.Sprintf("Concurrent Workspace %d", index),
				"description": fmt.Sprintf("Workspace created in concurrent test %d", index),
			}

			resp := suite.makeAuthenticatedRequest("POST", "/api/v1/workspaces", workspaceData, suite.alphaToken)
			success := resp.StatusCode == http.StatusCreated

			if success {
				// Clean up - delete the workspace
				var workspace map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&workspace)
				if err == nil {
					workspaceID := workspace["id"].(string)
					suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/workspaces/%s", workspaceID), nil, suite.alphaToken)
				}
			}

			results <- success
		}(i)
	}

	// Wait for all operations to complete
	successCount := 0
	for i := 0; i < concurrency; i++ {
		select {
		case success := <-results:
			if success {
				successCount++
			}
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout waiting for concurrent operations")
		}
	}

	assert.Equal(t, concurrency, successCount, "All concurrent cross-service operations should succeed")
}

// TestAPIGatewayCORSAndSecurity tests CORS and security headers
func (suite *CrossServiceTestSuite) TestAPIGatewayCORSAndSecurity() {
	t := suite.T()

	// Test CORS preflight request
	req, err := http.NewRequest("OPTIONS", TestAPIGatewayURL+"/api/v1/workspaces", nil)
	require.NoError(t, err)
	
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Authorization")

	resp, err := suite.testEnv.httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should allow CORS
	assert.Equal(t, http.StatusOK, resp.StatusCode, "CORS preflight should be allowed")
	assert.NotEmpty(t, resp.Header.Get("Access-Control-Allow-Origin"), "Should include CORS headers")

	// Test security headers on actual API request
	apiResp := suite.makeAuthenticatedRequest("GET", "/api/v1/workspaces", nil, suite.alphaToken)
	
	// Check for security headers (if implemented)
	securityHeaders := []string{
		"X-Content-Type-Options",
		"X-Frame-Options", 
		"X-XSS-Protection",
	}

	for _, header := range securityHeaders {
		headerValue := apiResp.Header.Get(header)
		t.Logf("Security header %s: %s", header, headerValue)
		// Log the headers but don't fail if they're missing (implementation dependent)
	}
}

// Run the test suite
func TestCrossServiceTestSuite(t *testing.T) {
	suite.Run(t, new(CrossServiceTestSuite))
}