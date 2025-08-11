package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// DataConsistencyTestSuite tests data consistency and cascade operations
type DataConsistencyTestSuite struct {
	suite.Suite
	testEnv    *TestEnvironment
	alphaToken string
	betaToken  string
}

// SetupSuite runs before all tests in the suite
func (suite *DataConsistencyTestSuite) SetupSuite() {
	suite.testEnv = NewTestEnvironment(suite.T())
	suite.testEnv.Start()

	// Get authentication tokens
	suite.alphaToken = suite.getAuthToken("admin@alpha.test.com", "testpass123")
	suite.betaToken = suite.getAuthToken("admin@beta.test.com", "testpass123")
}

// TearDownSuite runs after all tests in the suite
func (suite *DataConsistencyTestSuite) TearDownSuite() {
	if suite.testEnv != nil {
		suite.testEnv.Stop()
	}
}

// SetupTest runs before each test
func (suite *DataConsistencyTestSuite) SetupTest() {
	suite.testEnv.ResetTestData()
}

// Helper function to get auth token
func (suite *DataConsistencyTestSuite) getAuthToken(email, password string) string {
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
func (suite *DataConsistencyTestSuite) makeAuthenticatedRequest(method, path string, data interface{}, token string) *http.Response {
	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}
	return suite.testEnv.MakeAPIRequest(method, path, data, headers)
}

// TestTenantDataIsolation tests that tenant data is properly isolated
func (suite *DataConsistencyTestSuite) TestTenantDataIsolation() {
	t := suite.T()

	// Alpha admin should only see Alpha tenant data
	alphaWorkspacesResp := suite.makeAuthenticatedRequest("GET", "/api/v1/workspaces", nil, suite.alphaToken)
	require.Equal(t, http.StatusOK, alphaWorkspacesResp.StatusCode)

	var alphaWorkspaces map[string]interface{}
	err := json.NewDecoder(alphaWorkspacesResp.Body).Decode(&alphaWorkspaces)
	require.NoError(t, err)

	alphaWsList := alphaWorkspaces["workspaces"].([]interface{})
	for _, ws := range alphaWsList {
		workspace := ws.(map[string]interface{})
		assert.Equal(t, TestTenantAlphaID, workspace["tenant_id"], 
			"Alpha admin should only see Alpha workspaces")
	}

	// Beta admin should only see Beta tenant data
	betaWorkspacesResp := suite.makeAuthenticatedRequest("GET", "/api/v1/workspaces", nil, suite.betaToken)
	require.Equal(t, http.StatusOK, betaWorkspacesResp.StatusCode)

	var betaWorkspaces map[string]interface{}
	err = json.NewDecoder(betaWorkspacesResp.Body).Decode(&betaWorkspaces)
	require.NoError(t, err)

	betaWsList := betaWorkspaces["workspaces"].([]interface{})
	for _, ws := range betaWsList {
		workspace := ws.(map[string]interface{})
		assert.Equal(t, TestTenantBetaID, workspace["tenant_id"], 
			"Beta admin should only see Beta workspaces")
	}

	// Verify cross-tenant data access is prevented
	alphaProjectsResp := suite.makeAuthenticatedRequest("GET", "/api/v1/projects", nil, suite.alphaToken)
	require.Equal(t, http.StatusOK, alphaProjectsResp.StatusCode)

	var alphaProjects map[string]interface{}
	err = json.NewDecoder(alphaProjectsResp.Body).Decode(&alphaProjects)
	require.NoError(t, err)

	alphaProjectsList := alphaProjects["projects"].([]interface{})
	for _, proj := range alphaProjectsList {
		project := proj.(map[string]interface{})
		assert.Equal(t, TestTenantAlphaID, project["tenant_id"], 
			"Alpha admin should only see Alpha projects")
	}
}

// TestCascadeDeletion tests that related resources are properly handled during deletion
func (suite *DataConsistencyTestSuite) TestCascadeDeletion() {
	t := suite.T()

	// Step 1: Create a complete hierarchy - Workspace -> Project -> Base
	// Create workspace
	workspaceData := map[string]interface{}{
		"name":        "Test Cascade Workspace",
		"description": "Workspace for testing cascade deletion",
	}

	workspaceResp := suite.makeAuthenticatedRequest("POST", "/api/v1/workspaces", workspaceData, suite.alphaToken)
	require.Equal(t, http.StatusCreated, workspaceResp.StatusCode)

	var workspace map[string]interface{}
	err := json.NewDecoder(workspaceResp.Body).Decode(&workspace)
	require.NoError(t, err)
	workspaceID := workspace["id"].(string)

	// Create project in workspace
	projectData := map[string]interface{}{
		"workspace_id": workspaceID,
		"name":         "Test Cascade Project",
		"description":  "Project for testing cascade deletion",
	}

	projectResp := suite.makeAuthenticatedRequest("POST", "/api/v1/projects", projectData, suite.alphaToken)
	require.Equal(t, http.StatusCreated, projectResp.StatusCode)

	var project map[string]interface{}
	err = json.NewDecoder(projectResp.Body).Decode(&project)
	require.NoError(t, err)
	projectID := project["id"].(string)

	// Create airtable base in project (if the endpoint exists)
	baseData := map[string]interface{}{
		"project_id":       projectID,
		"airtable_base_id": "appTestCascade001",
		"name":             "Test Cascade Base",
		"description":      "Base for testing cascade deletion",
	}

	baseResp := suite.makeAuthenticatedRequest("POST", "/api/v1/airtable/bases", baseData, suite.alphaToken)
	var baseID string
	if baseResp.StatusCode == http.StatusCreated {
		var base map[string]interface{}
		err = json.NewDecoder(baseResp.Body).Decode(&base)
		require.NoError(t, err)
		baseID = base["id"].(string)
	}

	// Step 2: Create user permissions on these resources
	// Grant project admin permission to another user
	user1Token := suite.getAuthToken("user1@alpha.test.com", "testpass123")
	
	roleAssignmentData := map[string]interface{}{
		"user_id":       TestUserAlphaUser1ID,
		"role_id":       "role-project-admin",
		"resource_type": "project",
		"resource_id":   projectID,
	}

	assignResp := suite.makeAuthenticatedRequest("POST", "/api/v1/roles/assign", roleAssignmentData, suite.alphaToken)
	require.Equal(t, http.StatusCreated, assignResp.StatusCode)

	// Verify user1 has access to the project
	verifyResp := suite.makeAuthenticatedRequest("GET", fmt.Sprintf("/api/v1/projects/%s", projectID), nil, user1Token)
	require.Equal(t, http.StatusOK, verifyResp.StatusCode)

	// Step 3: Delete the workspace and verify cascade behavior
	deleteResp := suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/workspaces/%s", workspaceID), nil, suite.alphaToken)
	
	// Should either succeed (cascade delete) or fail with dependency error
	if deleteResp.StatusCode == http.StatusOK || deleteResp.StatusCode == http.StatusNoContent {
		// Cascade deletion successful
		
		// Verify project no longer exists
		projectCheckResp := suite.makeAuthenticatedRequest("GET", fmt.Sprintf("/api/v1/projects/%s", projectID), nil, suite.alphaToken)
		assert.Equal(t, http.StatusNotFound, projectCheckResp.StatusCode, 
			"Project should be deleted when workspace is deleted")

		// Verify base no longer exists (if it was created)
		if baseID != "" {
			baseCheckResp := suite.makeAuthenticatedRequest("GET", fmt.Sprintf("/api/v1/airtable/bases/%s", baseID), nil, suite.alphaToken)
			assert.Equal(t, http.StatusNotFound, baseCheckResp.StatusCode, 
				"Base should be deleted when project is deleted")
		}

		// Verify user permissions are cleaned up
		permissionCheck := map[string]interface{}{
			"user_id":       TestUserAlphaUser1ID,
			"resource_type": "project",
			"resource_id":   projectID,
			"action":        "read",
			"tenant_id":     TestTenantAlphaID,
		}

		permResp := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", permissionCheck, user1Token)
		require.Equal(t, http.StatusOK, permResp.StatusCode)

		var permResult map[string]interface{}
		err = json.NewDecoder(permResp.Body).Decode(&permResult)
		require.NoError(t, err)

		assert.False(t, permResult["allowed"].(bool), 
			"User should not have permissions on deleted project")

	} else if deleteResp.StatusCode == http.StatusConflict || deleteResp.StatusCode == http.StatusBadRequest {
		// Dependency constraint - need to delete children first
		t.Log("Cascade deletion not implemented, testing manual deletion order")

		// Delete base first (if it exists)
		if baseID != "" {
			baseDeleteResp := suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/airtable/bases/%s", baseID), nil, suite.alphaToken)
			assert.True(t, baseDeleteResp.StatusCode == http.StatusOK || baseDeleteResp.StatusCode == http.StatusNoContent,
				"Base deletion should succeed")
		}

		// Delete project
		projectDeleteResp := suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/projects/%s", projectID), nil, suite.alphaToken)
		assert.True(t, projectDeleteResp.StatusCode == http.StatusOK || projectDeleteResp.StatusCode == http.StatusNoContent,
			"Project deletion should succeed after base deletion")

		// Now delete workspace
		workspaceDeleteResp := suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/workspaces/%s", workspaceID), nil, suite.alphaToken)
		assert.True(t, workspaceDeleteResp.StatusCode == http.StatusOK || workspaceDeleteResp.StatusCode == http.StatusNoContent,
			"Workspace deletion should succeed after children deletion")
	}
}

// TestUserDeletion tests that user deletion affects all related resources
func (suite *DataConsistencyTestSuite) TestUserDeletion() {
	t := suite.T()

	// Step 1: Create a new user for testing deletion
	userData := map[string]interface{}{
		"email":      "deleteme@alpha.test.com",
		"username":   "deleteme_user",
		"password":   "testpass123",
		"first_name": "Delete",
		"last_name":  "Me",
		"tenant_id":  TestTenantAlphaID,
	}

	userResp := suite.makeAuthenticatedRequest("POST", "/api/v1/auth/register", userData, nil)
	require.Equal(t, http.StatusCreated, userResp.StatusCode)

	var newUser map[string]interface{}
	err := json.NewDecoder(userResp.Body).Decode(&newUser)
	require.NoError(t, err)
	
	newUserID := newUser["user_id"].(string)
	newUserToken := newUser["access_token"].(string)

	// Step 2: Give the user some permissions and create resources
	// Assign a role
	roleAssignmentData := map[string]interface{}{
		"user_id":       newUserID,
		"role_id":       "role-member",
		"resource_type": "workspace",
		"resource_id":   TestWorkspaceAlphaMainID,
	}

	suite.makeAuthenticatedRequest("POST", "/api/v1/roles/assign", roleAssignmentData, suite.alphaToken)

	// Grant a direct permission
	permissionData := map[string]interface{}{
		"user_id":       newUserID,
		"resource_type": "project",
		"resource_id":   TestProjectAlpha1ID,
		"action":        "read",
		"permission":    "allow",
		"reason":        "Test permission for deletion test",
	}

	suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/grant", permissionData, suite.alphaToken)

	// Create a workspace owned by the user
	workspaceData := map[string]interface{}{
		"name":        "User Owned Workspace",
		"description": "Workspace owned by user to be deleted",
	}

	workspaceResp := suite.makeAuthenticatedRequest("POST", "/api/v1/workspaces", workspaceData, newUserToken)
	var ownedWorkspaceID string
	if workspaceResp.StatusCode == http.StatusCreated {
		var workspace map[string]interface{}
		err = json.NewDecoder(workspaceResp.Body).Decode(&workspace)
		require.NoError(t, err)
		ownedWorkspaceID = workspace["id"].(string)
	}

	// Step 3: Verify user has access before deletion
	permissionCheck := map[string]interface{}{
		"user_id":       newUserID,
		"resource_type": "workspace",
		"resource_id":   TestWorkspaceAlphaMainID,
		"action":        "read",
		"tenant_id":     TestTenantAlphaID,
	}

	permResp := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", permissionCheck, newUserToken)
	require.Equal(t, http.StatusOK, permResp.StatusCode)

	var permResult map[string]interface{}
	err = json.NewDecoder(permResp.Body).Decode(&permResult)
	require.NoError(t, err)
	assert.True(t, permResult["allowed"].(bool), "User should have access before deletion")

	// Step 4: Delete the user
	deleteUserResp := suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/users/%s", newUserID), nil, suite.alphaToken)
	
	if deleteUserResp.StatusCode == http.StatusOK || deleteUserResp.StatusCode == http.StatusNoContent {
		// User deletion successful
		
		// Verify user can no longer authenticate
		loginData := map[string]interface{}{
			"email":    userData["email"],
			"password": userData["password"],
		}

		loginResp := suite.testEnv.MakeAPIRequest("POST", "/api/v1/auth/login", loginData, nil)
		assert.Equal(t, http.StatusUnauthorized, loginResp.StatusCode, 
			"Deleted user should not be able to login")

		// Verify old token no longer works
		profileResp := suite.makeAuthenticatedRequest("GET", "/api/v1/auth/profile", nil, newUserToken)
		assert.Equal(t, http.StatusUnauthorized, profileResp.StatusCode, 
			"Deleted user's token should be invalid")

		// Verify permissions are cleaned up
		permResp2 := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", permissionCheck, suite.alphaToken)
		if permResp2.StatusCode == http.StatusOK {
			var permResult2 map[string]interface{}
			err = json.NewDecoder(permResp2.Body).Decode(&permResult2)
			require.NoError(t, err)
			assert.False(t, permResult2["allowed"].(bool), 
				"Deleted user should not have any permissions")
		}

		// Verify owned resources are handled appropriately
		if ownedWorkspaceID != "" {
			workspaceCheckResp := suite.makeAuthenticatedRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s", ownedWorkspaceID), nil, suite.alphaToken)
			// Should either be deleted or ownership transferred
			if workspaceCheckResp.StatusCode == http.StatusOK {
				var workspace map[string]interface{}
				err = json.NewDecoder(workspaceCheckResp.Body).Decode(&workspace)
				require.NoError(t, err)
				assert.NotEqual(t, newUserID, workspace["owner_id"], 
					"Workspace ownership should be transferred")
			} else {
				assert.Equal(t, http.StatusNotFound, workspaceCheckResp.StatusCode, 
					"Workspace should be deleted with user")
			}
		}

	} else {
		t.Logf("User deletion not implemented or failed with status: %d", deleteUserResp.StatusCode)
		
		// Clean up manually if deletion failed
		if ownedWorkspaceID != "" {
			suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/workspaces/%s", ownedWorkspaceID), newUserToken)
		}
	}
}

// TestDatabaseTransactionConsistency tests that multi-table operations are atomic
func (suite *DataConsistencyTestSuite) TestDatabaseTransactionConsistency() {
	t := suite.T()

	// Test creating a project with invalid workspace (should fail atomically)
	invalidProjectData := map[string]interface{}{
		"workspace_id": "00000000-0000-0000-0000-000000000000", // Invalid workspace
		"name":         "Invalid Project",
		"description":  "Project with invalid workspace reference",
	}

	invalidResp := suite.makeAuthenticatedRequest("POST", "/api/v1/projects", invalidProjectData, suite.alphaToken)
	assert.True(t, invalidResp.StatusCode >= 400, "Invalid project creation should fail")

	// Verify no partial data was created
	projectsResp := suite.makeAuthenticatedRequest("GET", "/api/v1/projects?name=Invalid Project", nil, suite.alphaToken)
	require.Equal(t, http.StatusOK, projectsResp.StatusCode)

	var projects map[string]interface{}
	err := json.NewDecoder(projectsResp.Body).Decode(&projects)
	require.NoError(t, err)

	projectsList := projects["projects"].([]interface{})
	
	// Should not find any project with this name
	for _, proj := range projectsList {
		project := proj.(map[string]interface{})
		assert.NotEqual(t, "Invalid Project", project["name"], 
			"No partial project should be created from failed transaction")
	}
}

// TestEventualConsistency tests eventual consistency in distributed operations
func (suite *DataConsistencyTestSuite) TestEventualConsistency() {
	t := suite.T()

	// Create a workspace and immediately try to create a project in it
	workspaceData := map[string]interface{}{
		"name":        "Eventual Consistency Test",
		"description": "Testing eventual consistency",
	}

	workspaceResp := suite.makeAuthenticatedRequest("POST", "/api/v1/workspaces", workspaceData, suite.alphaToken)
	require.Equal(t, http.StatusCreated, workspaceResp.StatusCode)

	var workspace map[string]interface{}
	err := json.NewDecoder(workspaceResp.Body).Decode(&workspace)
	require.NoError(t, err)
	workspaceID := workspace["id"].(string)

	// Immediately create a project (tests that workspace is immediately available)
	projectData := map[string]interface{}{
		"workspace_id": workspaceID,
		"name":         "Immediate Project",
		"description":  "Project created immediately after workspace",
	}

	projectResp := suite.makeAuthenticatedRequest("POST", "/api/v1/projects", projectData, suite.alphaToken)
	assert.Equal(t, http.StatusCreated, projectResp.StatusCode, 
		"Project creation should succeed immediately after workspace creation")

	if projectResp.StatusCode == http.StatusCreated {
		var project map[string]interface{}
		err = json.NewDecoder(projectResp.Body).Decode(&project)
		require.NoError(t, err)
		projectID := project["id"].(string)

		// Test permission propagation - admin should immediately have access to new project
		permissionCheck := map[string]interface{}{
			"user_id":       TestUserAlphaAdminID,
			"resource_type": "project",
			"resource_id":   projectID,
			"action":        "manage",
			"tenant_id":     TestTenantAlphaID,
		}

		permResp := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", permissionCheck, suite.alphaToken)
		require.Equal(t, http.StatusOK, permResp.StatusCode)

		var permResult map[string]interface{}
		err = json.NewDecoder(permResp.Body).Decode(&permResult)
		require.NoError(t, err)

		assert.True(t, permResult["allowed"].(bool), 
			"Admin should immediately have access to newly created project")

		// Clean up
		suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/projects/%s", projectID), nil, suite.alphaToken)
	}

	// Clean up workspace
	suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/workspaces/%s", workspaceID), nil, suite.alphaToken)
}

// TestDataValidationConsistency tests that data validation is consistent across services
func (suite *DataConsistencyTestSuite) TestDataValidationConsistency() {
	t := suite.T()

	// Test consistent validation rules across different endpoints
	validationTests := []struct {
		name        string
		endpoint    string
		method      string
		data        map[string]interface{}
		expectError bool
		description string
	}{
		{
			name:     "empty_workspace_name",
			endpoint: "/api/v1/workspaces",
			method:   "POST",
			data: map[string]interface{}{
				"name":        "",
				"description": "Valid description",
			},
			expectError: true,
			description: "Empty workspace name should be rejected",
		},
		{
			name:     "empty_project_name",
			endpoint: "/api/v1/projects",
			method:   "POST",
			data: map[string]interface{}{
				"workspace_id": TestWorkspaceAlphaMainID,
				"name":         "",
				"description":  "Valid description",
			},
			expectError: true,
			description: "Empty project name should be rejected",
		},
		{
			name:     "valid_workspace_data",
			endpoint: "/api/v1/workspaces",
			method:   "POST",
			data: map[string]interface{}{
				"name":        "Valid Workspace Name",
				"description": "Valid description",
			},
			expectError: false,
			description: "Valid workspace data should be accepted",
		},
		{
			name:     "invalid_uuid_reference",
			endpoint: "/api/v1/projects",
			method:   "POST",
			data: map[string]interface{}{
				"workspace_id": "invalid-uuid",
				"name":         "Valid Project Name",
				"description":  "Valid description",
			},
			expectError: true,
			description: "Invalid UUID should be rejected",
		},
	}

	for _, test := range validationTests {
		t.Run(test.name, func(t *testing.T) {
			resp := suite.makeAuthenticatedRequest(test.method, test.endpoint, test.data, suite.alphaToken)
			
			if test.expectError {
				assert.True(t, resp.StatusCode >= 400, test.description)
			} else {
				assert.True(t, resp.StatusCode >= 200 && resp.StatusCode < 300, test.description)
				
				// Clean up created resources
				if resp.StatusCode == http.StatusCreated {
					var created map[string]interface{}
					err := json.NewDecoder(resp.Body).Decode(&created)
					if err == nil {
						if id, exists := created["id"]; exists {
							deleteEndpoint := fmt.Sprintf("%s/%s", test.endpoint, id)
							suite.makeAuthenticatedRequest("DELETE", deleteEndpoint, nil, suite.alphaToken)
						}
					}
				}
			}
		})
	}
}

// TestAuditLogConsistency tests that audit logs are consistently recorded
func (suite *DataConsistencyTestSuite) TestAuditLogConsistency() {
	t := suite.T()

	// Get initial audit log count
	initialLogsResp := suite.makeAuthenticatedRequest("GET", "/api/v1/audit-logs?page=1&page_size=1", nil, suite.alphaToken)
	require.Equal(t, http.StatusOK, initialLogsResp.StatusCode)

	var initialLogs map[string]interface{}
	err := json.NewDecoder(initialLogsResp.Body).Decode(&initialLogs)
	require.NoError(t, err)

	initialTotal := int(initialLogs["total"].(float64))

	// Perform an auditable action
	permissionData := map[string]interface{}{
		"user_id":       TestUserAlphaUser1ID,
		"resource_type": "workspace",
		"resource_id":   TestWorkspaceAlphaMainID,
		"action":        "admin",
		"permission":    "allow",
		"reason":        "Audit log consistency test",
	}

	grantResp := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/grant", permissionData, suite.alphaToken)
	require.Equal(t, http.StatusCreated, grantResp.StatusCode)

	var grantedPermission map[string]interface{}
	err = json.NewDecoder(grantResp.Body).Decode(&grantedPermission)
	require.NoError(t, err)
	permissionID := grantedPermission["id"].(string)

	// Wait for audit log to be recorded (eventual consistency)
	suite.testEnv.AssertEventualConsistency(func() bool {
		logsResp := suite.makeAuthenticatedRequest("GET", "/api/v1/audit-logs?page=1&page_size=1", nil, suite.alphaToken)
		if logsResp.StatusCode != http.StatusOK {
			return false
		}

		var logs map[string]interface{}
		err := json.NewDecoder(logsResp.Body).Decode(&logs)
		if err != nil {
			return false
		}

		newTotal := int(logs["total"].(float64))
		return newTotal > initialTotal
	}, 10*time.Second)

	// Verify the audit log was created
	auditResp := suite.makeAuthenticatedRequest("GET", "/api/v1/audit-logs?action=grant_permission&page=1&page_size=5", nil, suite.alphaToken)
	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var auditData map[string]interface{}
	err = json.NewDecoder(auditResp.Body).Decode(&auditData)
	require.NoError(t, err)

	logs := auditData["logs"].([]interface{})
	assert.NotEmpty(t, logs, "Audit logs should be recorded")

	// Verify the audit log details
	if len(logs) > 0 {
		latestLog := logs[0].(map[string]interface{})
		assert.Equal(t, "grant_permission", latestLog["action"])
		assert.Equal(t, TestUserAlphaAdminID, latestLog["user_id"])
		assert.Equal(t, TestUserAlphaUser1ID, latestLog["target_user_id"])
		assert.Equal(t, TestTenantAlphaID, latestLog["tenant_id"])
	}

	// Clean up - revoke the permission
	suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/permissions/%s", permissionID), nil, suite.alphaToken)
}

// Run the test suite
func TestDataConsistencyTestSuite(t *testing.T) {
	suite.Run(t, new(DataConsistencyTestSuite))
}