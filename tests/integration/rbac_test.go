package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// RBACTestSuite tests the Role-Based Access Control system
type RBACTestSuite struct {
	suite.Suite
	testEnv    *TestEnvironment
	alphaToken string
	betaToken  string
}

// SetupSuite runs before all tests in the suite
func (suite *RBACTestSuite) SetupSuite() {
	suite.testEnv = NewTestEnvironment(suite.T())
	suite.testEnv.Start()

	// Login as Alpha admin to get token
	suite.alphaToken = suite.getAuthToken("admin@alpha.test.com", "testpass123")
	
	// Login as Beta admin to get token  
	suite.betaToken = suite.getAuthToken("admin@beta.test.com", "testpass123")
}

// TearDownSuite runs after all tests in the suite
func (suite *RBACTestSuite) TearDownSuite() {
	if suite.testEnv != nil {
		suite.testEnv.Stop()
	}
}

// SetupTest runs before each test
func (suite *RBACTestSuite) SetupTest() {
	suite.testEnv.ResetTestData()
}

// Helper function to get auth token for a user
func (suite *RBACTestSuite) getAuthToken(email, password string) string {
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
func (suite *RBACTestSuite) makeAuthenticatedRequest(method, path string, data interface{}, token string) *http.Response {
	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}
	return suite.testEnv.MakeAPIRequest(method, path, data, headers)
}

// TestPermissionChecking tests the core permission checking functionality
func (suite *RBACTestSuite) TestPermissionChecking() {
	t := suite.T()

	testCases := []struct {
		name             string
		userEmail        string
		permissionCheck  map[string]interface{}
		expectedAllowed  bool
		expectedReason   string
		description      string
	}{
		{
			name:      "workspace_admin_can_manage_workspace",
			userEmail: "admin@alpha.test.com",
			permissionCheck: map[string]interface{}{
				"user_id":       TestUserAlphaAdminID,
				"resource_type": "workspace",
				"resource_id":   TestWorkspaceAlphaMainID,
				"action":        "manage",
				"tenant_id":     TestTenantAlphaID,
			},
			expectedAllowed: true,
			expectedReason:  "role permission",
			description:     "Workspace admin should be able to manage their workspace",
		},
		{
			name:      "project_admin_can_update_project",
			userEmail: "user1@alpha.test.com",
			permissionCheck: map[string]interface{}{
				"user_id":       TestUserAlphaUser1ID,
				"resource_type": "project",
				"resource_id":   TestProjectAlpha1ID,
				"action":        "update",
				"tenant_id":     TestTenantAlphaID,
			},
			expectedAllowed: true,
			expectedReason:  "role permission",
			description:     "Project admin should be able to update their project",
		},
		{
			name:      "member_can_read_workspace",
			userEmail: "user2@alpha.test.com",
			permissionCheck: map[string]interface{}{
				"user_id":       TestUserAlphaUser2ID,
				"resource_type": "workspace",
				"resource_id":   TestWorkspaceAlphaMainID,
				"action":        "read",
				"tenant_id":     TestTenantAlphaID,
			},
			expectedAllowed: true,
			expectedReason:  "role permission",
			description:     "Member should be able to read workspace",
		},
		{
			name:      "member_cannot_delete_workspace",
			userEmail: "user2@alpha.test.com",
			permissionCheck: map[string]interface{}{
				"user_id":       TestUserAlphaUser2ID,
				"resource_type": "workspace",
				"resource_id":   TestWorkspaceAlphaMainID,
				"action":        "delete",
				"tenant_id":     TestTenantAlphaID,
			},
			expectedAllowed: false,
			expectedReason:  "no matching permissions",
			description:     "Member should not be able to delete workspace",
		},
		{
			name:      "cross_tenant_access_denied",
			userEmail: "admin@alpha.test.com",
			permissionCheck: map[string]interface{}{
				"user_id":       TestUserAlphaAdminID,
				"resource_type": "workspace",
				"resource_id":   TestWorkspaceBetaID,
				"action":        "read",
				"tenant_id":     TestTenantBetaID,
			},
			expectedAllowed: false,
			expectedReason:  "no matching permissions",
			description:     "Alpha admin should not access Beta workspace",
		},
		{
			name:      "direct_permission_allows_action",
			userEmail: "user2@alpha.test.com",
			permissionCheck: map[string]interface{}{
				"user_id":       TestUserAlphaUser2ID,
				"resource_type": "project",
				"resource_id":   TestProjectAlpha2ID,
				"action":        "delete",
				"tenant_id":     TestTenantAlphaID,
			},
			expectedAllowed: true,
			expectedReason:  "direct permission",
			description:     "Direct permission should override role permissions",
		},
		{
			name:      "direct_permission_denies_action",
			userEmail: "user1@beta.test.com",
			permissionCheck: map[string]interface{}{
				"user_id":       TestUserBetaUser1ID,
				"resource_type": "airtable_base",
				"resource_id":   TestBaseBeta1ID,
				"action":        "read",
				"tenant_id":     TestTenantBetaID,
			},
			expectedAllowed: false,
			expectedReason:  "direct permission: deny",
			description:     "Direct deny permission should block access",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Get token for the user
			token := suite.getAuthToken(tc.userEmail, "testpass123")

			// Make permission check request
			resp := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", tc.permissionCheck, token)
			require.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			// Verify permission result
			assert.Equal(t, tc.expectedAllowed, result["allowed"], tc.description)
			if tc.expectedReason != "" {
				reason := result["reason"].(string)
				assert.Contains(t, reason, tc.expectedReason, "Permission reason should match expected pattern")
			}
		})
	}
}

// TestBatchPermissionChecking tests batch permission checking for efficiency
func (suite *RBACTestSuite) TestBatchPermissionChecking() {
	t := suite.T()

	// Test batch permission check for Alpha admin
	batchCheckData := map[string]interface{}{
		"user_id":   TestUserAlphaAdminID,
		"tenant_id": TestTenantAlphaID,
		"checks": []map[string]interface{}{
			{
				"resource_type": "workspace",
				"resource_id":   TestWorkspaceAlphaMainID,
				"action":        "read",
				"key":           "workspace_read",
			},
			{
				"resource_type": "workspace",
				"resource_id":   TestWorkspaceAlphaMainID,
				"action":        "manage",
				"key":           "workspace_manage",
			},
			{
				"resource_type": "project",
				"resource_id":   TestProjectAlpha1ID,
				"action":        "create",
				"key":           "project_create",
			},
			{
				"resource_type": "project",
				"resource_id":   TestProjectAlpha1ID,
				"action":        "delete",
				"key":           "project_delete",
			},
			{
				"resource_type": "workspace",
				"resource_id":   TestWorkspaceBetaID, // Cross-tenant access
				"action":        "read",
				"key":           "beta_workspace_read",
			},
		},
	}

	resp := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/batch-check", batchCheckData, suite.alphaToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var batchResult map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&batchResult)
	require.NoError(t, err)

	results := batchResult["results"].(map[string]interface{})

	// Verify workspace admin permissions
	workspaceRead := results["workspace_read"].(map[string]interface{})
	assert.True(t, workspaceRead["allowed"].(bool), "Admin should read workspace")

	workspaceManage := results["workspace_manage"].(map[string]interface{})
	assert.True(t, workspaceManage["allowed"].(bool), "Admin should manage workspace")

	projectCreate := results["project_create"].(map[string]interface{})
	assert.True(t, projectCreate["allowed"].(bool), "Admin should create projects")

	projectDelete := results["project_delete"].(map[string]interface{})
	assert.True(t, projectDelete["allowed"].(bool), "Admin should delete projects")

	// Verify cross-tenant access is denied
	betaWorkspaceRead := results["beta_workspace_read"].(map[string]interface{})
	assert.False(t, betaWorkspaceRead["allowed"].(bool), "Alpha admin should not access Beta workspace")
}

// TestHierarchicalPermissions tests permission inheritance in the hierarchy
func (suite *RBACTestSuite) TestHierarchicalPermissions() {
	t := suite.T()

	// Test that workspace-level permissions apply to projects within that workspace
	testCases := []struct {
		name        string
		userID      string
		token       string
		checks      []map[string]interface{}
		description string
	}{
		{
			name:   "workspace_admin_inherits_to_projects",
			userID: TestUserAlphaAdminID,
			token:  suite.alphaToken,
			checks: []map[string]interface{}{
				{
					"resource_type": "workspace",
					"resource_id":   TestWorkspaceAlphaMainID,
					"action":        "manage",
					"expected":      true,
				},
				{
					"resource_type": "project",
					"resource_id":   TestProjectAlpha1ID,
					"action":        "manage",
					"expected":      true, // Should inherit from workspace
				},
				{
					"resource_type": "project",
					"resource_id":   TestProjectAlpha2ID,
					"action":        "manage",
					"expected":      true, // Should inherit from workspace
				},
			},
			description: "Workspace admin should have management access to all projects in workspace",
		},
		{
			name:   "project_admin_limited_to_specific_project",
			userID: TestUserAlphaUser1ID,
			token:  suite.getAuthToken("user1@alpha.test.com", "testpass123"),
			checks: []map[string]interface{}{
				{
					"resource_type": "project",
					"resource_id":   TestProjectAlpha1ID,
					"action":        "manage",
					"expected":      true, // Has project admin role
				},
				{
					"resource_type": "project",
					"resource_id":   TestProjectAlpha2ID,
					"action":        "manage",
					"expected":      false, // No role on this project
				},
				{
					"resource_type": "workspace",
					"resource_id":   TestWorkspaceAlphaMainID,
					"action":        "manage",
					"expected":      false, // No workspace admin role
				},
			},
			description: "Project admin should only manage their specific project",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for i, check := range tc.checks {
				permissionCheck := map[string]interface{}{
					"user_id":       tc.userID,
					"resource_type": check["resource_type"],
					"resource_id":   check["resource_id"],
					"action":        check["action"],
					"tenant_id":     TestTenantAlphaID,
				}

				resp := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", permissionCheck, tc.token)
				require.Equal(t, http.StatusOK, resp.StatusCode)

				var result map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&result)
				require.NoError(t, err)

				expected := check["expected"].(bool)
				actual := result["allowed"].(bool)
				assert.Equal(t, expected, actual, 
					fmt.Sprintf("%s - Check %d: %s %s on %s", 
						tc.description, i+1, check["action"], check["resource_type"], check["resource_id"]))
			}
		})
	}
}

// TestRoleManagement tests creating, assigning, and revoking roles
func (suite *RBACTestSuite) TestRoleManagement() {
	t := suite.T()

	// Step 1: Create a custom role
	roleData := map[string]interface{}{
		"name":        "test_custom_role",
		"description": "Custom role for integration testing",
		"permissions": []string{
			"project:read",
			"project:update",
			"airtable_base:read",
		},
	}

	createResp := suite.makeAuthenticatedRequest("POST", "/api/v1/roles", roleData, suite.alphaToken)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var createdRole map[string]interface{}
	err := json.NewDecoder(createResp.Body).Decode(&createdRole)
	require.NoError(t, err)

	roleID := createdRole["id"].(string)
	assert.Equal(t, roleData["name"], createdRole["name"])
	assert.Equal(t, TestTenantAlphaID, createdRole["tenant_id"])
	assert.False(t, createdRole["is_system_role"].(bool))

	// Step 2: Assign the role to a user
	assignmentData := map[string]interface{}{
		"user_id":       TestUserAlphaUser2ID,
		"role_id":       roleID,
		"resource_type": "project",
		"resource_id":   TestProjectAlpha1ID,
		"expires_at":    nil,
	}

	assignResp := suite.makeAuthenticatedRequest("POST", "/api/v1/roles/assign", assignmentData, suite.alphaToken)
	require.Equal(t, http.StatusCreated, assignResp.StatusCode)

	var assignment map[string]interface{}
	err = json.NewDecoder(assignResp.Body).Decode(&assignment)
	require.NoError(t, err)

	assert.Equal(t, TestUserAlphaUser2ID, assignment["user_id"])
	assert.Equal(t, roleID, assignment["role_id"])
	assert.True(t, assignment["is_active"].(bool))

	// Step 3: Verify the user now has the role permissions
	user2Token := suite.getAuthToken("user2@alpha.test.com", "testpass123")

	permissionCheck := map[string]interface{}{
		"user_id":       TestUserAlphaUser2ID,
		"resource_type": "project",
		"resource_id":   TestProjectAlpha1ID,
		"action":        "update",
		"tenant_id":     TestTenantAlphaID,
	}

	checkResp := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", permissionCheck, user2Token)
	require.Equal(t, http.StatusOK, checkResp.StatusCode)

	var checkResult map[string]interface{}
	err = json.NewDecoder(checkResp.Body).Decode(&checkResult)
	require.NoError(t, err)

	assert.True(t, checkResult["allowed"].(bool), "User should have update permission from custom role")

	// Step 4: Test that user doesn't have permissions not included in the role
	deleteCheck := map[string]interface{}{
		"user_id":       TestUserAlphaUser2ID,
		"resource_type": "project",
		"resource_id":   TestProjectAlpha1ID,
		"action":        "delete",
		"tenant_id":     TestTenantAlphaID,
	}

	deleteCheckResp := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", deleteCheck, user2Token)
	require.Equal(t, http.StatusOK, deleteCheckResp.StatusCode)

	var deleteResult map[string]interface{}
	err = json.NewDecoder(deleteCheckResp.Body).Decode(&deleteResult)
	require.NoError(t, err)

	// Should be allowed due to direct permission from test data, not the custom role
	// This tests that direct permissions take precedence
	assert.True(t, deleteResult["allowed"].(bool), "User should have delete permission from direct permission")
	assert.Contains(t, deleteResult["reason"].(string), "direct permission", "Should be allowed by direct permission")
}

// TestDirectPermissions tests direct permission grants and denials
func (suite *RBACTestSuite) TestDirectPermissions() {
	t := suite.T()

	// Step 1: Grant a direct permission
	permissionData := map[string]interface{}{
		"user_id":       TestUserAlphaUser1ID,
		"resource_type": "airtable_base",
		"resource_id":   TestBaseAlpha1ID,
		"action":        "delete",
		"permission":    "allow",
		"expires_at":    "2025-12-31T23:59:59Z",
		"reason":        "Temporary deletion access for testing",
	}

	grantResp := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/grant", permissionData, suite.alphaToken)
	require.Equal(t, http.StatusCreated, grantResp.StatusCode)

	var grantedPermission map[string]interface{}
	err := json.NewDecoder(grantResp.Body).Decode(&grantedPermission)
	require.NoError(t, err)

	permissionID := grantedPermission["id"].(string)
	assert.Equal(t, permissionData["user_id"], grantedPermission["user_id"])
	assert.Equal(t, permissionData["action"], grantedPermission["action"])
	assert.Equal(t, "allow", grantedPermission["permission"])

	// Step 2: Verify the direct permission works
	user1Token := suite.getAuthToken("user1@alpha.test.com", "testpass123")

	permissionCheck := map[string]interface{}{
		"user_id":       TestUserAlphaUser1ID,
		"resource_type": "airtable_base",
		"resource_id":   TestBaseAlpha1ID,
		"action":        "delete",
		"tenant_id":     TestTenantAlphaID,
	}

	checkResp := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", permissionCheck, user1Token)
	require.Equal(t, http.StatusOK, checkResp.StatusCode)

	var checkResult map[string]interface{}
	err = json.NewDecoder(checkResp.Body).Decode(&checkResult)
	require.NoError(t, err)

	assert.True(t, checkResult["allowed"].(bool), "Direct permission should allow access")
	assert.Contains(t, checkResult["reason"].(string), "direct permission", "Should be allowed by direct permission")

	// Step 3: Revoke the permission
	revokeResp := suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/permissions/%s", permissionID), nil, suite.alphaToken)
	require.Equal(t, http.StatusOK, revokeResp.StatusCode)

	// Step 4: Verify the permission is revoked
	checkResp2 := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", permissionCheck, user1Token)
	require.Equal(t, http.StatusOK, checkResp2.StatusCode)

	var checkResult2 map[string]interface{}
	err = json.NewDecoder(checkResp2.Body).Decode(&checkResult2)
	require.NoError(t, err)

	// Should now fall back to role-based permissions (project admin should still have some access)
	if checkResult2["allowed"].(bool) {
		assert.Contains(t, checkResult2["reason"].(string), "role permission", "Should fall back to role permissions")
	}
}

// TestPermissionAuditLogs tests that permission changes are properly audited
func (suite *RBACTestSuite) TestPermissionAuditLogs() {
	t := suite.T()

	// Step 1: Perform a permission change that should be audited
	permissionData := map[string]interface{}{
		"user_id":       TestUserAlphaUser1ID,
		"resource_type": "project",
		"resource_id":   TestProjectAlpha2ID,
		"action":        "admin",
		"permission":    "allow",
		"reason":        "Audit test permission",
	}

	suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/grant", permissionData, suite.alphaToken)

	// Step 2: Check audit logs
	auditResp := suite.makeAuthenticatedRequest("GET", "/api/v1/audit-logs?action=grant_permission&page=1&page_size=10", nil, suite.alphaToken)
	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var auditData map[string]interface{}
	err := json.NewDecoder(auditResp.Body).Decode(&auditData)
	require.NoError(t, err)

	logs := auditData["logs"].([]interface{})
	assert.NotEmpty(t, logs, "Should have audit logs")

	// Verify the most recent log entry
	if len(logs) > 0 {
		latestLog := logs[0].(map[string]interface{})
		assert.Equal(t, "grant_permission", latestLog["action"])
		assert.Equal(t, TestUserAlphaAdminID, latestLog["user_id"]) // The granter
		assert.Equal(t, TestUserAlphaUser1ID, latestLog["target_user_id"]) // The grantee
		assert.Equal(t, TestTenantAlphaID, latestLog["tenant_id"])

		changes := latestLog["changes"].(map[string]interface{})
		assert.Equal(t, "admin", changes["action"])
		assert.Equal(t, "allow", changes["permission"])
	}
}

// TestRoleListingAndFiltering tests role listing with various filters
func (suite *RBACTestSuite) TestRoleListingAndFiltering() {
	t := suite.T()

	// Test basic role listing
	listResp := suite.makeAuthenticatedRequest("GET", "/api/v1/roles", nil, suite.alphaToken)
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var roleList map[string]interface{}
	err := json.NewDecoder(listResp.Body).Decode(&roleList)
	require.NoError(t, err)

	roles := roleList["roles"].([]interface{})
	assert.NotEmpty(t, roles, "Should have roles")

	// Test filtering by system roles
	systemRolesResp := suite.makeAuthenticatedRequest("GET", "/api/v1/roles?is_system_role=true", nil, suite.alphaToken)
	require.Equal(t, http.StatusOK, systemRolesResp.StatusCode)

	var systemRoleList map[string]interface{}
	err = json.NewDecoder(systemRolesResp.Body).Decode(&systemRoleList)
	require.NoError(t, err)

	systemRoles := systemRoleList["roles"].([]interface{})
	for _, role := range systemRoles {
		roleData := role.(map[string]interface{})
		assert.True(t, roleData["is_system_role"].(bool), "All returned roles should be system roles")
	}

	// Test search functionality
	searchResp := suite.makeAuthenticatedRequest("GET", "/api/v1/roles?search=admin", nil, suite.alphaToken)
	require.Equal(t, http.StatusOK, searchResp.StatusCode)

	var searchResults map[string]interface{}
	err = json.NewDecoder(searchResp.Body).Decode(&searchResults)
	require.NoError(t, err)

	searchRoles := searchResults["roles"].([]interface{})
	for _, role := range searchRoles {
		roleData := role.(map[string]interface{})
		roleName := roleData["name"].(string)
		assert.Contains(t, roleName, "admin", "Searched roles should contain 'admin' in name")
	}
}

// TestCrossServicePermissionValidation tests that permission checks work across services
func (suite *RBACTestSuite) TestCrossServicePermissionValidation() {
	t := suite.T()

	// Test that API Gateway validates permissions for protected routes
	// This requires the API Gateway to be configured to check permissions

	user2Token := suite.getAuthToken("user2@alpha.test.com", "testpass123")

	// User2 is a member, should be able to read but not manage workspaces
	readResp := suite.makeAuthenticatedRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s", TestWorkspaceAlphaMainID), nil, user2Token)
	assert.Equal(t, http.StatusOK, readResp.StatusCode, "Member should be able to read workspace")

	// Try to delete workspace (should be denied)
	deleteResp := suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/workspaces/%s", TestWorkspaceAlphaMainID), nil, user2Token)
	assert.Equal(t, http.StatusForbidden, deleteResp.StatusCode, "Member should not be able to delete workspace")

	// Try to access Beta tenant workspace (should be denied)
	crossTenantResp := suite.makeAuthenticatedRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s", TestWorkspaceBetaID), nil, user2Token)
	assert.Equal(t, http.StatusNotFound, crossTenantResp.StatusCode, "Alpha user should not access Beta workspace")
}

// Run the test suite
func TestRBACTestSuite(t *testing.T) {
	suite.Run(t, new(RBACTestSuite))
}