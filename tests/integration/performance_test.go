package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// PerformanceTestSuite tests performance and reliability aspects
type PerformanceTestSuite struct {
	suite.Suite
	testEnv    *TestEnvironment
	alphaToken string
	betaToken  string
}

// SetupSuite runs before all tests in the suite
func (suite *PerformanceTestSuite) SetupSuite() {
	suite.testEnv = NewTestEnvironment(suite.T())
	suite.testEnv.Start()

	// Get authentication tokens
	suite.alphaToken = suite.getAuthToken("admin@alpha.test.com", "testpass123")
	suite.betaToken = suite.getAuthToken("admin@beta.test.com", "testpass123")
}

// TearDownSuite runs after all tests in the suite
func (suite *PerformanceTestSuite) TearDownSuite() {
	if suite.testEnv != nil {
		suite.testEnv.Stop()
	}
}

// SetupTest runs before each test
func (suite *PerformanceTestSuite) SetupTest() {
	suite.testEnv.ResetTestData()
}

// Helper function to get auth token
func (suite *PerformanceTestSuite) getAuthToken(email, password string) string {
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
func (suite *PerformanceTestSuite) makeAuthenticatedRequest(method, path string, data interface{}, token string) *http.Response {
	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}
	return suite.testEnv.MakeAPIRequest(method, path, data, headers)
}

// TestResponseTimes tests that API responses are within acceptable time limits
func (suite *PerformanceTestSuite) TestResponseTimes() {
	t := suite.T()

	endpoints := []struct {
		name        string
		method      string
		path        string
		data        interface{}
		maxDuration time.Duration
		description string
	}{
		{
			name:        "health_check",
			method:      "GET",
			path:        "/health",
			data:        nil,
			maxDuration: 100 * time.Millisecond,
			description: "Health check should be very fast",
		},
		{
			name:        "auth_profile",
			method:      "GET",
			path:        "/api/v1/auth/profile",
			data:        nil,
			maxDuration: 500 * time.Millisecond,
			description: "Profile fetch should be fast",
		},
		{
			name:        "workspace_list",
			method:      "GET",
			path:        "/api/v1/workspaces",
			data:        nil,
			maxDuration: 1 * time.Second,
			description: "Workspace listing should be responsive",
		},
		{
			name:        "permission_check",
			method:      "POST",
			path:        "/api/v1/permissions/check",
			data: map[string]interface{}{
				"user_id":       TestUserAlphaAdminID,
				"resource_type": "workspace",
				"resource_id":   TestWorkspaceAlphaMainID,
				"action":        "read",
				"tenant_id":     TestTenantAlphaID,
			},
			maxDuration: 300 * time.Millisecond,
			description: "Permission check should be fast (cached)",
		},
		{
			name:        "project_list",
			method:      "GET",
			path:        "/api/v1/projects",
			data:        nil,
			maxDuration: 1 * time.Second,
			description: "Project listing should be responsive",
		},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			start := time.Now()
			
			resp := suite.makeAuthenticatedRequest(endpoint.method, endpoint.path, endpoint.data, suite.alphaToken)
			
			duration := time.Since(start)
			
			assert.True(t, resp.StatusCode >= 200 && resp.StatusCode < 300,
				fmt.Sprintf("%s should return success status", endpoint.description))
			
			assert.True(t, duration <= endpoint.maxDuration,
				fmt.Sprintf("%s took %v, should be under %v", endpoint.description, duration, endpoint.maxDuration))
			
			t.Logf("%s response time: %v", endpoint.name, duration)
		})
	}
}

// TestConcurrentUsers tests system behavior under concurrent user load
func (suite *PerformanceTestSuite) TestConcurrentUsers() {
	t := suite.T()

	concurrency := 10
	requestsPerUser := 5
	
	var wg sync.WaitGroup
	results := make(chan map[string]interface{}, concurrency*requestsPerUser)
	
	// Each goroutine simulates a user making multiple requests
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(userIndex int) {
			defer wg.Done()
			
			// Get a fresh token for this user
			userToken := suite.alphaToken
			if userIndex%2 == 1 {
				userToken = suite.betaToken // Alternate between Alpha and Beta users
			}
			
			for j := 0; j < requestsPerUser; j++ {
				start := time.Now()
				
				// Make various types of requests
				var resp *http.Response
				var operation string
				
				switch j % 4 {
				case 0:
					resp = suite.makeAuthenticatedRequest("GET", "/api/v1/workspaces", nil, userToken)
					operation = "list_workspaces"
				case 1:
					resp = suite.makeAuthenticatedRequest("GET", "/api/v1/projects", nil, userToken)
					operation = "list_projects"
				case 2:
					permCheck := map[string]interface{}{
						"user_id":       TestUserAlphaAdminID,
						"resource_type": "workspace",
						"resource_id":   TestWorkspaceAlphaMainID,
						"action":        "read",
						"tenant_id":     TestTenantAlphaID,
					}
					resp = suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", permCheck, userToken)
					operation = "check_permission"
				case 3:
					resp = suite.makeAuthenticatedRequest("GET", "/api/v1/auth/profile", nil, userToken)
					operation = "get_profile"
				}
				
				duration := time.Since(start)
				
				results <- map[string]interface{}{
					"user":      userIndex,
					"request":   j,
					"operation": operation,
					"status":    resp.StatusCode,
					"duration":  duration,
					"success":   resp.StatusCode >= 200 && resp.StatusCode < 300,
				}
			}
		}(i)
	}
	
	wg.Wait()
	close(results)
	
	// Analyze results
	totalRequests := 0
	successfulRequests := 0
	var totalDuration time.Duration
	maxDuration := time.Duration(0)
	
	operationStats := make(map[string]struct {
		count    int
		duration time.Duration
		errors   int
	})
	
	for result := range results {
		totalRequests++
		duration := result["duration"].(time.Duration)
		operation := result["operation"].(string)
		success := result["success"].(bool)
		
		totalDuration += duration
		if duration > maxDuration {
			maxDuration = duration
		}
		
		if success {
			successfulRequests++
		}
		
		stats := operationStats[operation]
		stats.count++
		stats.duration += duration
		if !success {
			stats.errors++
		}
		operationStats[operation] = stats
	}
	
	// Assert performance requirements
	successRate := float64(successfulRequests) / float64(totalRequests)
	avgDuration := totalDuration / time.Duration(totalRequests)
	
	assert.True(t, successRate >= 0.95, 
		fmt.Sprintf("Success rate should be >= 95%%, got %.2f%%", successRate*100))
	
	assert.True(t, avgDuration <= 2*time.Second,
		fmt.Sprintf("Average response time should be <= 2s, got %v", avgDuration))
	
	assert.True(t, maxDuration <= 5*time.Second,
		fmt.Sprintf("Max response time should be <= 5s, got %v", maxDuration))
	
	// Log statistics
	t.Logf("Concurrent test results:")
	t.Logf("  Total requests: %d", totalRequests)
	t.Logf("  Success rate: %.2f%%", successRate*100)
	t.Logf("  Average duration: %v", avgDuration)
	t.Logf("  Max duration: %v", maxDuration)
	
	for operation, stats := range operationStats {
		avgOpDuration := stats.duration / time.Duration(stats.count)
		errorRate := float64(stats.errors) / float64(stats.count)
		t.Logf("  %s: %d requests, avg %v, %.1f%% errors", 
			operation, stats.count, avgOpDuration, errorRate*100)
	}
}

// TestRateLimiting tests that rate limiting works correctly
func (suite *PerformanceTestSuite) TestRateLimiting() {
	t := suite.T()

	// Test rate limiting by making many rapid requests
	rapidRequests := 50
	rapidInterval := 10 * time.Millisecond
	
	var responses []int
	var responseTimes []time.Duration
	
	for i := 0; i < rapidRequests; i++ {
		start := time.Now()
		resp := suite.makeAuthenticatedRequest("GET", "/api/v1/workspaces", nil, suite.alphaToken)
		duration := time.Since(start)
		
		responses = append(responses, resp.StatusCode)
		responseTimes = append(responseTimes, duration)
		
		time.Sleep(rapidInterval)
	}
	
	// Analyze rate limiting behavior
	successCount := 0
	rateLimitedCount := 0
	
	for _, status := range responses {
		if status == http.StatusOK {
			successCount++
		} else if status == http.StatusTooManyRequests {
			rateLimitedCount++
		}
	}
	
	t.Logf("Rate limiting test results:")
	t.Logf("  Total requests: %d", rapidRequests)
	t.Logf("  Successful: %d", successCount)
	t.Logf("  Rate limited: %d", rateLimitedCount)
	t.Logf("  Other errors: %d", rapidRequests-successCount-rateLimitedCount)
	
	// Rate limiting should kick in for rapid requests
	if rateLimitedCount > 0 {
		assert.True(t, rateLimitedCount < rapidRequests/2, 
			"Rate limiting should not block majority of reasonable requests")
		t.Log("Rate limiting is working correctly")
	} else {
		t.Log("No rate limiting detected - either not implemented or limits are very high")
	}
	
	// Test that normal usage after rate limiting recovers
	time.Sleep(2 * time.Second) // Wait for rate limit window to reset
	
	normalResp := suite.makeAuthenticatedRequest("GET", "/api/v1/workspaces", nil, suite.alphaToken)
	assert.Equal(t, http.StatusOK, normalResp.StatusCode, 
		"Normal requests should work after rate limit window")
}

// TestMemoryAndResourceUsage tests for memory leaks and resource usage
func (suite *PerformanceTestSuite) TestMemoryAndResourceUsage() {
	t := suite.T()

	// Get initial metrics
	initialMetrics := suite.getServiceMetrics()
	
	// Perform a series of operations that might cause memory leaks
	operations := 20
	
	for i := 0; i < operations; i++ {
		// Create and delete workspaces
		workspaceData := map[string]interface{}{
			"name":        fmt.Sprintf("Memory Test Workspace %d", i),
			"description": "Workspace for memory usage testing",
		}
		
		createResp := suite.makeAuthenticatedRequest("POST", "/api/v1/workspaces", workspaceData, suite.alphaToken)
		if createResp.StatusCode == http.StatusCreated {
			var workspace map[string]interface{}
			err := json.NewDecoder(createResp.Body).Decode(&workspace)
			if err == nil {
				workspaceID := workspace["id"].(string)
				
				// Create project in workspace
				projectData := map[string]interface{}{
					"workspace_id": workspaceID,
					"name":         fmt.Sprintf("Memory Test Project %d", i),
					"description":  "Project for memory testing",
				}
				
				projectResp := suite.makeAuthenticatedRequest("POST", "/api/v1/projects", projectData, suite.alphaToken)
				if projectResp.StatusCode == http.StatusCreated {
					var project map[string]interface{}
					err := json.NewDecoder(projectResp.Body).Decode(&project)
					if err == nil {
						projectID := project["id"].(string)
						suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/projects/%s", projectID), nil, suite.alphaToken)
					}
				}
				
				// Delete workspace
				suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/workspaces/%s", workspaceID), nil, suite.alphaToken)
			}
		}
		
		// Perform permission checks
		permissionCheck := map[string]interface{}{
			"user_id":       TestUserAlphaAdminID,
			"resource_type": "workspace",
			"resource_id":   TestWorkspaceAlphaMainID,
			"action":        "read",
			"tenant_id":     TestTenantAlphaID,
		}
		suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", permissionCheck, suite.alphaToken)
	}
	
	// Wait for garbage collection
	time.Sleep(5 * time.Second)
	
	// Get final metrics
	finalMetrics := suite.getServiceMetrics()
	
	// Compare metrics (implementation dependent)
	t.Logf("Memory usage test completed")
	t.Logf("Initial metrics: %+v", initialMetrics)
	t.Logf("Final metrics: %+v", finalMetrics)
	
	// Basic checks (these depend on metrics being implemented)
	if initialMetrics != nil && finalMetrics != nil {
		// Log any significant changes in metrics
		for key, initialValue := range initialMetrics {
			if finalValue, exists := finalMetrics[key]; exists {
				t.Logf("Metric %s: %v -> %v", key, initialValue, finalValue)
			}
		}
	}
}

// TestDatabaseConnectionPooling tests database connection handling under load
func (suite *PerformanceTestSuite) TestDatabaseConnectionPooling() {
	t := suite.T()

	concurrency := 20 // Higher than typical DB connection pool size
	var wg sync.WaitGroup
	results := make(chan bool, concurrency)
	
	// Simulate many concurrent database operations
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			
			// Perform database-heavy operations
			success := true
			
			// List workspaces (reads from database)
			resp1 := suite.makeAuthenticatedRequest("GET", "/api/v1/workspaces", nil, suite.alphaToken)
			if resp1.StatusCode != http.StatusOK {
				success = false
			}
			
			// List projects (reads from database)
			resp2 := suite.makeAuthenticatedRequest("GET", "/api/v1/projects", nil, suite.alphaToken)
			if resp2.StatusCode != http.StatusOK {
				success = false
			}
			
			// Check permissions (reads from database + cache)
			permissionCheck := map[string]interface{}{
				"user_id":       TestUserAlphaAdminID,
				"resource_type": "workspace",
				"resource_id":   TestWorkspaceAlphaMainID,
				"action":        "read",
				"tenant_id":     TestTenantAlphaID,
			}
			resp3 := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", permissionCheck, suite.alphaToken)
			if resp3.StatusCode != http.StatusOK {
				success = false
			}
			
			results <- success
		}(i)
	}
	
	wg.Wait()
	close(results)
	
	// Analyze results
	successCount := 0
	for success := range results {
		if success {
			successCount++
		}
	}
	
	successRate := float64(successCount) / float64(concurrency)
	
	assert.True(t, successRate >= 0.9, 
		fmt.Sprintf("Database connection pooling should handle concurrent requests, success rate: %.2f%%", successRate*100))
	
	t.Logf("Database connection test: %d/%d successful (%.1f%%)", 
		successCount, concurrency, successRate*100)
}

// TestCachePerformance tests caching behavior and performance
func (suite *PerformanceTestSuite) TestCachePerformance() {
	t := suite.T()

	permissionCheck := map[string]interface{}{
		"user_id":       TestUserAlphaAdminID,
		"resource_type": "workspace",
		"resource_id":   TestWorkspaceAlphaMainID,
		"action":        "read",
		"tenant_id":     TestTenantAlphaID,
	}
	
	// First request (cache miss)
	start1 := time.Now()
	resp1 := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", permissionCheck, suite.alphaToken)
	duration1 := time.Since(start1)
	
	require.Equal(t, http.StatusOK, resp1.StatusCode)
	
	// Second request (should be cached)
	start2 := time.Now()
	resp2 := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", permissionCheck, suite.alphaToken)
	duration2 := time.Since(start2)
	
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	
	// Third request (should still be cached)
	start3 := time.Now()
	resp3 := suite.makeAuthenticatedRequest("POST", "/api/v1/permissions/check", permissionCheck, suite.alphaToken)
	duration3 := time.Since(start3)
	
	require.Equal(t, http.StatusOK, resp3.StatusCode)
	
	// Verify responses are consistent
	var result1, result2, result3 map[string]interface{}
	err := json.NewDecoder(resp1.Body).Decode(&result1)
	require.NoError(t, err)
	err = json.NewDecoder(resp2.Body).Decode(&result2)
	require.NoError(t, err)
	err = json.NewDecoder(resp3.Body).Decode(&result3)
	require.NoError(t, err)
	
	assert.Equal(t, result1["allowed"], result2["allowed"], "Cached results should be consistent")
	assert.Equal(t, result1["allowed"], result3["allowed"], "Cached results should be consistent")
	
	// Performance analysis
	t.Logf("Cache performance test:")
	t.Logf("  First request (cache miss): %v", duration1)
	t.Logf("  Second request (cached): %v", duration2)
	t.Logf("  Third request (cached): %v", duration3)
	
	// Cached requests should be faster (if caching is implemented)
	if duration2 < duration1 && duration3 < duration1 {
		t.Log("Caching is working - subsequent requests are faster")
		
		// Cached requests should be significantly faster
		assert.True(t, duration2 < duration1/2 || duration2 < 50*time.Millisecond,
			"Cached requests should be much faster")
	} else {
		t.Log("No significant caching speedup detected - may not be implemented or data is already very fast")
	}
}

// TestErrorRecovery tests system recovery from errors and failures
func (suite *PerformanceTestSuite) TestErrorRecovery() {
	t := suite.T()

	// Test that the system recovers from various error conditions
	
	// 1. Test recovery from invalid requests
	invalidData := map[string]interface{}{
		"invalid_field": "invalid_value",
	}
	
	// Send several invalid requests
	for i := 0; i < 5; i++ {
		resp := suite.makeAuthenticatedRequest("POST", "/api/v1/workspaces", invalidData, suite.alphaToken)
		assert.True(t, resp.StatusCode >= 400, "Invalid requests should return error status")
	}
	
	// Verify system still works normally after invalid requests
	validData := map[string]interface{}{
		"name":        "Recovery Test Workspace",
		"description": "Testing recovery after errors",
	}
	
	recoveryResp := suite.makeAuthenticatedRequest("POST", "/api/v1/workspaces", validData, suite.alphaToken)
	assert.Equal(t, http.StatusCreated, recoveryResp.StatusCode, 
		"System should recover and handle valid requests after errors")
	
	// Clean up
	if recoveryResp.StatusCode == http.StatusCreated {
		var workspace map[string]interface{}
		err := json.NewDecoder(recoveryResp.Body).Decode(&workspace)
		if err == nil {
			workspaceID := workspace["id"].(string)
			suite.makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/workspaces/%s", workspaceID), nil, suite.alphaToken)
		}
	}
	
	// 2. Test recovery from authentication errors
	invalidToken := "invalid.jwt.token"
	invalidHeaders := map[string]string{
		"Authorization": "Bearer " + invalidToken,
	}
	
	// Send requests with invalid token
	for i := 0; i < 3; i++ {
		resp := suite.testEnv.MakeAPIRequest("GET", "/api/v1/workspaces", nil, invalidHeaders)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, 
			"Invalid token should return unauthorized")
	}
	
	// Verify valid authentication still works
	validResp := suite.makeAuthenticatedRequest("GET", "/api/v1/workspaces", nil, suite.alphaToken)
	assert.Equal(t, http.StatusOK, validResp.StatusCode, 
		"Valid authentication should work after invalid attempts")
}

// Helper function to get service metrics (if available)
func (suite *PerformanceTestSuite) getServiceMetrics() map[string]interface{} {
	// Try to get metrics from API Gateway
	resp := suite.testEnv.MakeDirectServiceRequest(TestAPIGatewayURL, "GET", "/metrics", nil, nil)
	if resp.StatusCode == http.StatusOK {
		var metrics map[string]interface{}
		err := json.NewDecoder(resp.Body).Decode(&metrics)
		if err == nil {
			return metrics
		}
	}
	return nil
}

// Run the test suite
func TestPerformanceTestSuite(t *testing.T) {
	suite.Run(t, new(PerformanceTestSuite))
}