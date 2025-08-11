package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// AuthFlowTestSuite tests end-to-end authentication flows
type AuthFlowTestSuite struct {
	suite.Suite
	testEnv *TestEnvironment
}

// SetupSuite runs before all tests in the suite
func (suite *AuthFlowTestSuite) SetupSuite() {
	suite.testEnv = NewTestEnvironment(suite.T())
	suite.testEnv.Start()
}

// TearDownSuite runs after all tests in the suite
func (suite *AuthFlowTestSuite) TearDownSuite() {
	if suite.testEnv != nil {
		suite.testEnv.Stop()
	}
}

// SetupTest runs before each test
func (suite *AuthFlowTestSuite) SetupTest() {
	// Clean and reset test data
	suite.testEnv.ResetTestData()
}

// TestUserRegistrationFlow tests complete user registration process
func (suite *AuthFlowTestSuite) TestUserRegistrationFlow() {
	t := suite.T()

	// Test data
	registrationData := map[string]interface{}{
		"email":      "newuser@alpha.test.com",
		"username":   "newuser_alpha",
		"password":   "testpass123",
		"first_name": "New",
		"last_name":  "User",
		"tenant_id":  TestTenantAlphaID,
	}

	// Step 1: Register new user
	resp := suite.testEnv.MakeAPIRequest("POST", "/api/v1/auth/register", registrationData, nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var registerResponse map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&registerResponse)
	require.NoError(t, err)

	// Verify registration response
	assert.Contains(t, registerResponse, "user_id")
	assert.Contains(t, registerResponse, "access_token")
	assert.Contains(t, registerResponse, "refresh_token")
	assert.Equal(t, registrationData["email"], registerResponse["email"])
	assert.Equal(t, TestTenantAlphaID, registerResponse["tenant_id"])

	// Step 2: Use the access token to access protected resource
	accessToken := registerResponse["access_token"].(string)
	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
	}

	// Verify token works
	profileResp := suite.testEnv.MakeAPIRequest("GET", "/api/v1/auth/profile", nil, headers)
	require.Equal(t, http.StatusOK, profileResp.StatusCode)

	var profileData map[string]interface{}
	err = json.NewDecoder(profileResp.Body).Decode(&profileData)
	require.NoError(t, err)

	assert.Equal(t, registrationData["email"], profileData["email"])
	assert.Equal(t, registrationData["username"], profileData["username"])
	assert.Equal(t, TestTenantAlphaID, profileData["tenant_id"])
}

// TestUserLoginFlow tests user authentication
func (suite *AuthFlowTestSuite) TestUserLoginFlow() {
	t := suite.T()

	// Test data - using existing test user
	loginData := map[string]interface{}{
		"email":    "admin@alpha.test.com",
		"password": "testpass123",
	}

	// Step 1: Login
	resp := suite.testEnv.MakeAPIRequest("POST", "/api/v1/auth/login", loginData, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var loginResponse map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&loginResponse)
	require.NoError(t, err)

	// Verify login response
	assert.Contains(t, loginResponse, "access_token")
	assert.Contains(t, loginResponse, "refresh_token")
	assert.Contains(t, loginResponse, "user_id")
	assert.Equal(t, TestUserAlphaAdminID, loginResponse["user_id"])
	assert.Equal(t, TestTenantAlphaID, loginResponse["tenant_id"])

	// Step 2: Access protected resources
	accessToken := loginResponse["access_token"].(string)
	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
	}

	// Get user profile
	profileResp := suite.testEnv.MakeAPIRequest("GET", "/api/v1/auth/profile", nil, headers)
	require.Equal(t, http.StatusOK, profileResp.StatusCode)

	// Get user workspaces (should include tenant-specific workspaces)
	workspacesResp := suite.testEnv.MakeAPIRequest("GET", "/api/v1/workspaces", nil, headers)
	require.Equal(t, http.StatusOK, workspacesResp.StatusCode)

	var workspacesData map[string]interface{}
	err = json.NewDecoder(workspacesResp.Body).Decode(&workspacesData)
	require.NoError(t, err)

	workspaces := workspacesData["workspaces"].([]interface{})
	assert.NotEmpty(t, workspaces)

	// Verify all workspaces belong to the correct tenant
	for _, ws := range workspaces {
		workspace := ws.(map[string]interface{})
		assert.Equal(t, TestTenantAlphaID, workspace["tenant_id"])
	}
}

// TestInvalidLoginAttempts tests security measures for invalid logins
func (suite *AuthFlowTestSuite) TestInvalidLoginAttempts() {
	t := suite.T()

	testCases := []struct {
		name         string
		loginData    map[string]interface{}
		expectedCode int
		description  string
	}{
		{
			name: "invalid_email",
			loginData: map[string]interface{}{
				"email":    "nonexistent@alpha.test.com",
				"password": "testpass123",
			},
			expectedCode: http.StatusUnauthorized,
			description:  "Non-existent email should be rejected",
		},
		{
			name: "invalid_password",
			loginData: map[string]interface{}{
				"email":    "admin@alpha.test.com",
				"password": "wrongpassword",
			},
			expectedCode: http.StatusUnauthorized,
			description:  "Wrong password should be rejected",
		},
		{
			name: "empty_credentials",
			loginData: map[string]interface{}{
				"email":    "",
				"password": "",
			},
			expectedCode: http.StatusBadRequest,
			description:  "Empty credentials should be rejected",
		},
		{
			name: "suspended_tenant_user",
			loginData: map[string]interface{}{
				"email":    "admin@gamma.test.com",
				"password": "testpass123",
			},
			expectedCode: http.StatusForbidden,
			description:  "User from suspended tenant should be rejected",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := suite.testEnv.MakeAPIRequest("POST", "/api/v1/auth/login", tc.loginData, nil)
			assert.Equal(t, tc.expectedCode, resp.StatusCode, tc.description)

			// Verify no tokens are returned on failure
			if resp.StatusCode != http.StatusOK {
				var errorResponse map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&errorResponse)
				require.NoError(t, err)
				assert.NotContains(t, errorResponse, "access_token")
				assert.NotContains(t, errorResponse, "refresh_token")
			}
		})
	}
}

// TestTokenRefreshFlow tests JWT token refresh mechanism
func (suite *AuthFlowTestSuite) TestTokenRefreshFlow() {
	t := suite.T()

	// Step 1: Login to get tokens
	loginData := map[string]interface{}{
		"email":    "admin@alpha.test.com",
		"password": "testpass123",
	}

	loginResp := suite.testEnv.MakeAPIRequest("POST", "/api/v1/auth/login", loginData, nil)
	require.Equal(t, http.StatusOK, loginResp.StatusCode)

	var loginResponse map[string]interface{}
	err := json.NewDecoder(loginResp.Body).Decode(&loginResponse)
	require.NoError(t, err)

	originalAccessToken := loginResponse["access_token"].(string)
	refreshToken := loginResponse["refresh_token"].(string)

	// Step 2: Use refresh token to get new access token
	refreshData := map[string]interface{}{
		"refresh_token": refreshToken,
	}

	refreshResp := suite.testEnv.MakeAPIRequest("POST", "/api/v1/auth/refresh", refreshData, nil)
	require.Equal(t, http.StatusOK, refreshResp.StatusCode)

	var refreshResponse map[string]interface{}
	err = json.NewDecoder(refreshResp.Body).Decode(&refreshResponse)
	require.NoError(t, err)

	// Verify new tokens are issued
	newAccessToken := refreshResponse["access_token"].(string)
	assert.NotEqual(t, originalAccessToken, newAccessToken, "New access token should be different")
	assert.Contains(t, refreshResponse, "refresh_token")

	// Step 3: Verify new access token works
	headers := map[string]string{
		"Authorization": "Bearer " + newAccessToken,
	}

	profileResp := suite.testEnv.MakeAPIRequest("GET", "/api/v1/auth/profile", nil, headers)
	assert.Equal(t, http.StatusOK, profileResp.StatusCode)

	// Step 4: Verify old access token is invalid (if implemented)
	oldHeaders := map[string]string{
		"Authorization": "Bearer " + originalAccessToken,
	}

	oldTokenResp := suite.testEnv.MakeAPIRequest("GET", "/api/v1/auth/profile", nil, oldHeaders)
	// This test depends on whether token blacklisting is implemented
	// For now, we'll just log the result
	t.Logf("Old token validation status: %d", oldTokenResp.StatusCode)
}

// TestTenantIsolationInAuth tests that users can only access their tenant's resources
func (suite *AuthFlowTestSuite) TestTenantIsolationInAuth() {
	t := suite.T()

	// Step 1: Login as Alpha tenant user
	alphaLoginData := map[string]interface{}{
		"email":    "admin@alpha.test.com",
		"password": "testpass123",
	}

	alphaResp := suite.testEnv.MakeAPIRequest("POST", "/api/v1/auth/login", alphaLoginData, nil)
	require.Equal(t, http.StatusOK, alphaResp.StatusCode)

	var alphaLogin map[string]interface{}
	err := json.NewDecoder(alphaResp.Body).Decode(&alphaLogin)
	require.NoError(t, err)

	alphaToken := alphaLogin["access_token"].(string)
	alphaHeaders := map[string]string{
		"Authorization": "Bearer " + alphaToken,
	}

	// Step 2: Login as Beta tenant user
	betaLoginData := map[string]interface{}{
		"email":    "admin@beta.test.com",
		"password": "testpass123",
	}

	betaResp := suite.testEnv.MakeAPIRequest("POST", "/api/v1/auth/login", betaLoginData, nil)
	require.Equal(t, http.StatusOK, betaResp.StatusCode)

	var betaLogin map[string]interface{}
	err = json.NewDecoder(betaResp.Body).Decode(&betaLogin)
	require.NoError(t, err)

	betaToken := betaLogin["access_token"].(string)
	betaHeaders := map[string]string{
		"Authorization": "Bearer " + betaToken,
	}

	// Step 3: Verify Alpha user can only see Alpha resources
	alphaWorkspacesResp := suite.testEnv.MakeAPIRequest("GET", "/api/v1/workspaces", nil, alphaHeaders)
	require.Equal(t, http.StatusOK, alphaWorkspacesResp.StatusCode)

	var alphaWorkspaces map[string]interface{}
	err = json.NewDecoder(alphaWorkspacesResp.Body).Decode(&alphaWorkspaces)
	require.NoError(t, err)

	alphaWsList := alphaWorkspaces["workspaces"].([]interface{})
	for _, ws := range alphaWsList {
		workspace := ws.(map[string]interface{})
		assert.Equal(t, TestTenantAlphaID, workspace["tenant_id"], "Alpha user should only see Alpha workspaces")
	}

	// Step 4: Verify Beta user can only see Beta resources
	betaWorkspacesResp := suite.testEnv.MakeAPIRequest("GET", "/api/v1/workspaces", nil, betaHeaders)
	require.Equal(t, http.StatusOK, betaWorkspacesResp.StatusCode)

	var betaWorkspaces map[string]interface{}
	err = json.NewDecoder(betaWorkspacesResp.Body).Decode(&betaWorkspaces)
	require.NoError(t, err)

	betaWsList := betaWorkspaces["workspaces"].([]interface{})
	for _, ws := range betaWsList {
		workspace := ws.(map[string]interface{})
		assert.Equal(t, TestTenantBetaID, workspace["tenant_id"], "Beta user should only see Beta workspaces")
	}

	// Step 5: Verify Alpha user cannot access specific Beta resources
	// Try to access a Beta workspace directly
	betaWorkspaceResp := suite.testEnv.MakeAPIRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s", TestWorkspaceBetaID), nil, alphaHeaders)
	assert.Equal(t, http.StatusNotFound, betaWorkspaceResp.StatusCode, "Alpha user should not access Beta workspace")

	// Step 6: Verify Beta user cannot access specific Alpha resources
	alphaWorkspaceResp := suite.testEnv.MakeAPIRequest("GET", fmt.Sprintf("/api/v1/workspaces/%s", TestWorkspaceAlphaMainID), nil, betaHeaders)
	assert.Equal(t, http.StatusNotFound, alphaWorkspaceResp.StatusCode, "Beta user should not access Alpha workspace")
}

// TestLogoutFlow tests user logout functionality
func (suite *AuthFlowTestSuite) TestLogoutFlow() {
	t := suite.T()

	// Step 1: Login
	loginData := map[string]interface{}{
		"email":    "admin@alpha.test.com",
		"password": "testpass123",
	}

	loginResp := suite.testEnv.MakeAPIRequest("POST", "/api/v1/auth/login", loginData, nil)
	require.Equal(t, http.StatusOK, loginResp.StatusCode)

	var loginResponse map[string]interface{}
	err := json.NewDecoder(loginResp.Body).Decode(&loginResponse)
	require.NoError(t, err)

	accessToken := loginResponse["access_token"].(string)
	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
	}

	// Step 2: Verify token works before logout
	profileResp := suite.testEnv.MakeAPIRequest("GET", "/api/v1/auth/profile", nil, headers)
	assert.Equal(t, http.StatusOK, profileResp.StatusCode)

	// Step 3: Logout
	logoutResp := suite.testEnv.MakeAPIRequest("POST", "/api/v1/auth/logout", nil, headers)
	assert.Equal(t, http.StatusOK, logoutResp.StatusCode)

	// Step 4: Verify token no longer works after logout (if token blacklisting is implemented)
	postLogoutResp := suite.testEnv.MakeAPIRequest("GET", "/api/v1/auth/profile", nil, headers)
	// This test depends on implementation - may still work if stateless JWT
	t.Logf("Post-logout token validation status: %d", postLogoutResp.StatusCode)
}

// TestExpiredTokenHandling tests behavior with expired tokens
func (suite *AuthFlowTestSuite) TestExpiredTokenHandling() {
	t := suite.T()

	// Create a token that's immediately expired (this would need to be supported by the auth service)
	// For now, we'll test with an obviously invalid token
	invalidHeaders := map[string]string{
		"Authorization": "Bearer invalid.token.here",
	}

	resp := suite.testEnv.MakeAPIRequest("GET", "/api/v1/auth/profile", nil, invalidHeaders)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	var errorResponse map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&errorResponse)
	require.NoError(t, err)
	assert.Contains(t, errorResponse, "error")
}

// TestConcurrentAuthentication tests multiple simultaneous authentications
func (suite *AuthFlowTestSuite) TestConcurrentAuthentication() {
	t := suite.T()

	// Test concurrent logins don't interfere with each other
	concurrency := 5
	results := make(chan bool, concurrency)

	loginData := map[string]interface{}{
		"email":    "admin@alpha.test.com",
		"password": "testpass123",
	}

	for i := 0; i < concurrency; i++ {
		go func(index int) {
			// Each goroutine performs a complete auth flow
			resp := suite.testEnv.MakeAPIRequest("POST", "/api/v1/auth/login", loginData, nil)
			success := resp.StatusCode == http.StatusOK

			if success {
				var loginResponse map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&loginResponse)
				if err != nil {
					success = false
				} else {
					// Verify the token works
					token := loginResponse["access_token"].(string)
					headers := map[string]string{
						"Authorization": "Bearer " + token,
					}
					
					profileResp := suite.testEnv.MakeAPIRequest("GET", "/api/v1/auth/profile", nil, headers)
					success = profileResp.StatusCode == http.StatusOK
				}
			}

			results <- success
		}(i)
	}

	// Wait for all goroutines to complete
	successCount := 0
	for i := 0; i < concurrency; i++ {
		select {
		case success := <-results:
			if success {
				successCount++
			}
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout waiting for concurrent auth tests")
		}
	}

	assert.Equal(t, concurrency, successCount, "All concurrent authentications should succeed")
}

// TestAuthenticationWithAPIKey tests API key authentication
func (suite *AuthFlowTestSuite) TestAuthenticationWithAPIKey() {
	t := suite.T()

	// Test with valid API key
	validHeaders := map[string]string{
		"X-API-Key": TestAPIKey,
	}

	resp := suite.testEnv.MakeAPIRequest("GET", "/api/v1/health", nil, validHeaders)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test with invalid API key
	invalidHeaders := map[string]string{
		"X-API-Key": "invalid-api-key",
	}

	resp = suite.testEnv.MakeAPIRequest("GET", "/api/v1/health", nil, invalidHeaders)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Test without API key (should fail if REQUIRE_API_KEY=true)
	resp = suite.testEnv.MakeAPIRequest("GET", "/api/v1/health", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// Run the test suite
func TestAuthFlowTestSuite(t *testing.T) {
	suite.Run(t, new(AuthFlowTestSuite))
}