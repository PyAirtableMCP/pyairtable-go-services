package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test constants - these match the test data in fixtures/test-data.sql
const (
	// API Configuration
	TestAPIKey = "test-api-key-12345"
	
	// Service URLs
	TestAPIGatewayURL = "http://localhost:8080"
	
	// Test Tenant IDs
	TestTenantAlphaID = "550e8400-e29b-41d4-a716-446655440001"
	TestTenantBetaID  = "550e8400-e29b-41d4-a716-446655440002"
	TestTenantGammaID = "550e8400-e29b-41d4-a716-446655440003" // Suspended
	
	// Test User IDs
	TestUserAlphaAdminID = "660e8400-e29b-41d4-a716-446655440001"
	TestUserAlphaUser1ID = "660e8400-e29b-41d4-a716-446655440002"
	TestUserAlphaUser2ID = "660e8400-e29b-41d4-a716-446655440003"
	TestUserBetaAdminID  = "660e8400-e29b-41d4-a716-446655440004"
	TestUserBetaUser1ID  = "660e8400-e29b-41d4-a716-446655440005"
	TestUserGammaAdminID = "660e8400-e29b-41d4-a716-446655440006"
	
	// Test Workspace IDs
	TestWorkspaceAlphaMainID = "770e8400-e29b-41d4-a716-446655440001"
	TestWorkspaceAlphaDevID  = "770e8400-e29b-41d4-a716-446655440002"
	TestWorkspaceBetaID      = "770e8400-e29b-41d4-a716-446655440003"
	TestWorkspaceGammaID     = "770e8400-e29b-41d4-a716-446655440004"
	
	// Test Project IDs
	TestProjectAlpha1ID = "880e8400-e29b-41d4-a716-446655440001"
	TestProjectAlpha2ID = "880e8400-e29b-41d4-a716-446655440002"
	TestProjectAlpha3ID = "880e8400-e29b-41d4-a716-446655440003"
	TestProjectBeta1ID  = "880e8400-e29b-41d4-a716-446655440004"
	
	// Test Airtable Base IDs
	TestBaseAlpha1ID = "990e8400-e29b-41d4-a716-446655440001"
	TestBaseAlpha2ID = "990e8400-e29b-41d4-a716-446655440002"
	TestBaseBeta1ID  = "990e8400-e29b-41d4-a716-446655440003"
	
	// Test File IDs
	TestFileAlpha1ID = "file-001"
	TestFileAlpha2ID = "file-002"
	TestFileBeta1ID  = "file-003"
)

// TestEnvironment manages the test environment lifecycle
type TestEnvironment struct {
	t                *testing.T
	dockerComposeCmd *exec.Cmd
	httpClient       *http.Client
	running          bool
}

// NewTestEnvironment creates a new test environment
func NewTestEnvironment(t *testing.T) *TestEnvironment {
	return &TestEnvironment{
		t: t,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Start brings up the test environment
func (te *TestEnvironment) Start() {
	te.t.Log("Starting test environment...")
	
	// Clean up any existing containers
	te.cleanup()
	
	// Start the test environment
	te.startServices()
	
	// Wait for services to be ready
	te.waitForServices()
	
	te.running = true
	te.t.Log("Test environment is ready")
}

// Stop tears down the test environment
func (te *TestEnvironment) Stop() {
	if !te.running {
		return
	}
	
	te.t.Log("Stopping test environment...")
	te.cleanup()
	te.running = false
	te.t.Log("Test environment stopped")
}

// cleanup removes any existing test containers
func (te *TestEnvironment) cleanup() {
	ctx := context.Background()
	
	// Stop and remove containers
	cmd := exec.CommandContext(ctx, "docker-compose", 
		"-f", "docker-compose.test.yml", 
		"-p", "pyairtable-integration-test",
		"down", "-v", "--remove-orphans")
	cmd.Dir = "."
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		te.t.Logf("Cleanup warning: %v\nOutput: %s", err, string(output))
	}
}

// startServices starts all test services
func (te *TestEnvironment) startServices() {
	ctx := context.Background()
	
	// Build and start services
	cmd := exec.CommandContext(ctx, "docker-compose",
		"-f", "docker-compose.test.yml",
		"-p", "pyairtable-integration-test",
		"up", "-d", "--build")
	cmd.Dir = "."
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	err := cmd.Run()
	require.NoError(te.t, err, "Failed to start test services")
}

// waitForServices waits for all services to be healthy
func (te *TestEnvironment) waitForServices() {
	services := []struct {
		name string
		url  string
	}{
		{"API Gateway", TestAPIGatewayURL + "/health"},
		{"Platform Services", "http://localhost:8081/health"},
		{"Permission Service", "http://localhost:8085/health"},
		{"Automation Services", "http://localhost:8082/health"},
	}
	
	maxWait := 120 * time.Second
	checkInterval := 2 * time.Second
	timeout := time.After(maxWait)
	
	for _, service := range services {
		te.t.Logf("Waiting for %s to be ready...", service.name)
		
		for {
			select {
			case <-timeout:
				te.t.Fatalf("Timeout waiting for %s to be ready", service.name)
			default:
				if te.checkServiceHealth(service.url) {
					te.t.Logf("%s is ready", service.name)
					goto nextService
				}
				time.Sleep(checkInterval)
			}
		}
		nextService:
	}
	
	// Extra wait for services to fully initialize
	time.Sleep(5 * time.Second)
}

// checkServiceHealth checks if a service is healthy
func (te *TestEnvironment) checkServiceHealth(url string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}
	
	// Add API key for services that require it
	req.Header.Set("X-API-Key", TestAPIKey)
	
	resp, err := te.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	
	return resp.StatusCode == http.StatusOK
}

// ResetTestData resets the test database to a clean state
func (te *TestEnvironment) ResetTestData() {
	te.t.Log("Resetting test data...")
	
	// Execute the reset script in the postgres container
	cmd := exec.Command("docker", "exec", 
		"pyairtable-integration-test-postgres-test-1",
		"psql", "-U", "test_user", "-d", "pyairtable_test", 
		"-f", "/docker-entrypoint-initdb.d/99-test-data.sql")
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		te.t.Logf("Reset warning: %v\nOutput: %s", err, string(output))
	}
}

// MakeAPIRequest makes an HTTP request to the API Gateway
func (te *TestEnvironment) MakeAPIRequest(method, path string, data interface{}, headers map[string]string) *http.Response {
	var body io.Reader
	
	if data != nil {
		jsonData, err := json.Marshal(data)
		require.NoError(te.t, err)
		body = bytes.NewReader(jsonData)
	}
	
	url := TestAPIGatewayURL + path
	if !strings.HasPrefix(path, "/") {
		url = TestAPIGatewayURL + "/" + path
	}
	
	req, err := http.NewRequest(method, url, body)
	require.NoError(te.t, err)
	
	// Set default headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", TestAPIKey)
	
	// Add custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	
	resp, err := te.httpClient.Do(req)
	require.NoError(te.t, err)
	
	return resp
}

// MakeDirectServiceRequest makes a request directly to a service (bypassing API Gateway)
func (te *TestEnvironment) MakeDirectServiceRequest(serviceURL, method, path string, data interface{}, headers map[string]string) *http.Response {
	var body io.Reader
	
	if data != nil {
		jsonData, err := json.Marshal(data)
		require.NoError(te.t, err)
		body = bytes.NewReader(jsonData)
	}
	
	url := serviceURL + path
	if !strings.HasPrefix(path, "/") {
		url = serviceURL + "/" + path
	}
	
	req, err := http.NewRequest(method, url, body)
	require.NoError(te.t, err)
	
	// Set default headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", TestAPIKey)
	
	// Add custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	
	resp, err := te.httpClient.Do(req)
	require.NoError(te.t, err)
	
	return resp
}

// GetLogs retrieves logs from a specific service container
func (te *TestEnvironment) GetLogs(serviceName string) string {
	cmd := exec.Command("docker", "logs", 
		fmt.Sprintf("pyairtable-integration-test-%s-1", serviceName))
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		te.t.Logf("Error getting logs for %s: %v", serviceName, err)
		return ""
	}
	
	return string(output)
}

// ExecuteInContainer executes a command in a container
func (te *TestEnvironment) ExecuteInContainer(containerName string, command ...string) (string, error) {
	fullContainerName := fmt.Sprintf("pyairtable-integration-test-%s-1", containerName)
	
	args := append([]string{"exec", fullContainerName}, command...)
	cmd := exec.Command("docker", args...)
	
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// WaitForCondition waits for a condition to become true
func (te *TestEnvironment) WaitForCondition(condition func() bool, timeout time.Duration, message string) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	
	timeoutChan := time.After(timeout)
	
	for {
		select {
		case <-ticker.C:
			if condition() {
				return
			}
		case <-timeoutChan:
			te.t.Fatalf("Timeout waiting for condition: %s", message)
		}
	}
}

// GetServiceMetrics retrieves metrics from a service
func (te *TestEnvironment) GetServiceMetrics(serviceURL string) map[string]interface{} {
	resp := te.MakeDirectServiceRequest(serviceURL, "GET", "/metrics", nil, nil)
	require.Equal(te.t, http.StatusOK, resp.StatusCode)
	
	var metrics map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&metrics)
	require.NoError(te.t, err)
	
	return metrics
}

// ValidateJSONResponse validates that a response contains valid JSON
func (te *TestEnvironment) ValidateJSONResponse(resp *http.Response) map[string]interface{} {
	var result map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(te.t, err, "Response should contain valid JSON")
	return result
}

// CreateTestFile creates a test file for upload testing
func (te *TestEnvironment) CreateTestFile(filename, content string) *bytes.Buffer {
	buffer := &bytes.Buffer{}
	buffer.WriteString(content)
	return buffer
}

// AssertEventualConsistency waits for eventual consistency in distributed operations
func (te *TestEnvironment) AssertEventualConsistency(check func() bool, timeout time.Duration) {
	te.WaitForCondition(check, timeout, "eventual consistency check")
}