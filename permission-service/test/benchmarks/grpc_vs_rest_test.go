package benchmarks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"

	permissionv1 "github.com/pyairtable/pyairtable-protos/generated/go/pyairtable/permission/v1"
	commonv1 "github.com/pyairtable/pyairtable-protos/generated/go/pyairtable/common/v1"
	"github.com/pyairtable/pyairtable-permission-service/pkg/client"
)

// BenchmarkConfig holds configuration for benchmarks
type BenchmarkConfig struct {
	GRPCAddress string
	RESTAddress string
	UserID      string
	ResourceID  string
	TenantID    string
}

// Default benchmark configuration
var defaultConfig = &BenchmarkConfig{
	GRPCAddress: "localhost:50051",
	RESTAddress: "http://localhost:8085",
	UserID:      "test-user-1",
	ResourceID:  "test-resource-1",
	TenantID:    "test-tenant-1",
}

// REST API request/response structures
type RESTCheckPermissionRequest struct {
	UserID       string            `json:"user_id"`
	ResourceType string            `json:"resource_type"`
	ResourceID   string            `json:"resource_id"`
	Action       string            `json:"action"`
	Context      map[string]string `json:"context,omitempty"`
}

type RESTCheckPermissionResponse struct {
	Allowed  bool              `json:"allowed"`
	Level    int               `json:"level"`
	Reasons  []string          `json:"reasons,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// setupGRPCClient creates a gRPC client for benchmarking
func setupGRPCClient(b *testing.B, config *BenchmarkConfig) *client.PermissionClient {
	logger, _ := zap.NewDevelopment()
	clientConfig := client.DefaultClientConfig(config.GRPCAddress)
	clientConfig.MaxRetries = 1 // Reduce retries for benchmarking
	
	grpcClient, err := client.NewPermissionClient(clientConfig, logger)
	if err != nil {
		b.Fatalf("Failed to create gRPC client: %v", err)
	}
	
	b.Cleanup(func() {
		grpcClient.Close()
	})
	
	return grpcClient
}

// setupRESTClient creates an HTTP client for benchmarking
func setupRESTClient(b *testing.B) *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

// BenchmarkGRPCCheckPermission benchmarks gRPC CheckPermission
func BenchmarkGRPCCheckPermission(b *testing.B) {
	grpcClient := setupGRPCClient(b, defaultConfig)
	
	req := &permissionv1.CheckPermissionRequest{
		RequestMetadata: &commonv1.RequestMetadata{
			RequestId: "bench-request",
			UserContext: &commonv1.UserContext{
				UserId: defaultConfig.UserID,
				Email:  "test@example.com",
				TenantContext: &commonv1.TenantContext{
					TenantId:    defaultConfig.TenantID,
					WorkspaceId: "test-workspace",
				},
			},
			Timestamp: timestamppb.Now(),
		},
		UserId:       defaultConfig.UserID,
		ResourceType: permissionv1.ResourceType_RESOURCE_TYPE_TABLE,
		ResourceId:   defaultConfig.ResourceID,
		Action:       "read",
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := grpcClient.CheckPermission(ctx, req)
		cancel()
		
		if err != nil {
			b.Errorf("gRPC CheckPermission failed: %v", err)
		}
	}
}

// BenchmarkRESTCheckPermission benchmarks REST CheckPermission
func BenchmarkRESTCheckPermission(b *testing.B) {
	httpClient := setupRESTClient(b)
	
	reqBody := RESTCheckPermissionRequest{
		UserID:       defaultConfig.UserID,
		ResourceType: "table",
		ResourceID:   defaultConfig.ResourceID,
		Action:       "read",
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		jsonBody, _ := json.Marshal(reqBody)
		
		req, err := http.NewRequest("POST", defaultConfig.RESTAddress+"/api/v1/permissions/check", bytes.NewBuffer(jsonBody))
		if err != nil {
			b.Errorf("Failed to create HTTP request: %v", err)
			continue
		}
		
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("X-User-ID", defaultConfig.UserID)
		req.Header.Set("X-Tenant-ID", defaultConfig.TenantID)
		
		resp, err := httpClient.Do(req)
		if err != nil {
			b.Errorf("REST CheckPermission failed: %v", err)
			continue
		}
		
		resp.Body.Close()
	}
}

// BenchmarkGRPCBatchCheckPermission benchmarks gRPC BatchCheckPermission
func BenchmarkGRPCBatchCheckPermission(b *testing.B) {
	grpcClient := setupGRPCClient(b, defaultConfig)
	
	// Create batch request with 10 permission checks
	checks := make([]*permissionv1.PermissionCheck, 10)
	for i := 0; i < 10; i++ {
		checks[i] = &permissionv1.PermissionCheck{
			CheckId:      fmt.Sprintf("check-%d", i),
			ResourceType: permissionv1.ResourceType_RESOURCE_TYPE_TABLE,
			ResourceId:   fmt.Sprintf("%s-%d", defaultConfig.ResourceID, i),
			Action:       "read",
		}
	}
	
	req := &permissionv1.BatchCheckPermissionRequest{
		RequestMetadata: &commonv1.RequestMetadata{
			RequestId: "bench-batch-request",
			UserContext: &commonv1.UserContext{
				UserId: defaultConfig.UserID,
				Email:  "test@example.com",
				TenantContext: &commonv1.TenantContext{
					TenantId:    defaultConfig.TenantID,
					WorkspaceId: "test-workspace",
				},
			},
			Timestamp: timestamppb.Now(),
		},
		UserId: defaultConfig.UserID,
		Checks: checks,
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := grpcClient.BatchCheckPermission(ctx, req)
		cancel()
		
		if err != nil {
			b.Errorf("gRPC BatchCheckPermission failed: %v", err)
		}
	}
}

// BenchmarkRESTBatchCheckPermission benchmarks REST BatchCheckPermission
func BenchmarkRESTBatchCheckPermission(b *testing.B) {
	httpClient := setupRESTClient(b)
	
	// Create batch request structure (simplified for REST)
	checks := make([]map[string]interface{}, 10)
	for i := 0; i < 10; i++ {
		checks[i] = map[string]interface{}{
			"check_id":      fmt.Sprintf("check-%d", i),
			"resource_type": "table",
			"resource_id":   fmt.Sprintf("%s-%d", defaultConfig.ResourceID, i),
			"action":        "read",
		}
	}
	
	reqBody := map[string]interface{}{
		"user_id": defaultConfig.UserID,
		"checks":  checks,
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		jsonBody, _ := json.Marshal(reqBody)
		
		req, err := http.NewRequest("POST", defaultConfig.RESTAddress+"/api/v1/permissions/batch-check", bytes.NewBuffer(jsonBody))
		if err != nil {
			b.Errorf("Failed to create HTTP request: %v", err)
			continue
		}
		
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("X-User-ID", defaultConfig.UserID)
		req.Header.Set("X-Tenant-ID", defaultConfig.TenantID)
		
		resp, err := httpClient.Do(req)
		if err != nil {
			b.Errorf("REST BatchCheckPermission failed: %v", err)
			continue
		}
		
		resp.Body.Close()
	}
}

// BenchmarkGRPCConnectionSetup benchmarks gRPC connection setup time
func BenchmarkGRPCConnectionSetup(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		clientConfig := client.DefaultClientConfig(defaultConfig.GRPCAddress)
		grpcClient, err := client.NewPermissionClient(clientConfig, logger)
		if err != nil {
			b.Errorf("Failed to create gRPC client: %v", err)
			continue
		}
		grpcClient.Close()
	}
}

// BenchmarkRESTConnectionSetup benchmarks REST connection setup time
func BenchmarkRESTConnectionSetup(b *testing.B) {
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		client := &http.Client{
			Timeout: 30 * time.Second,
		}
		
		// Make a simple request to establish connection
		req, _ := http.NewRequest("GET", defaultConfig.RESTAddress+"/health", nil)
		resp, err := client.Do(req)
		if err != nil {
			b.Errorf("Failed to connect via REST: %v", err)
			continue
		}
		resp.Body.Close()
		client.CloseIdleConnections()
	}
}

// BenchmarkGRPCConcurrentRequests benchmarks concurrent gRPC requests
func BenchmarkGRPCConcurrentRequests(b *testing.B) {
	grpcClient := setupGRPCClient(b, defaultConfig)
	
	req := &permissionv1.CheckPermissionRequest{
		RequestMetadata: &commonv1.RequestMetadata{
			RequestId: "bench-concurrent-request",
			UserContext: &commonv1.UserContext{
				UserId: defaultConfig.UserID,
				TenantContext: &commonv1.TenantContext{
					TenantId: defaultConfig.TenantID,
				},
			},
			Timestamp: timestamppb.Now(),
		},
		UserId:       defaultConfig.UserID,
		ResourceType: permissionv1.ResourceType_RESOURCE_TYPE_TABLE,
		ResourceId:   defaultConfig.ResourceID,
		Action:       "read",
	}
	
	b.ResetTimer()
	
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err := grpcClient.CheckPermission(ctx, req)
			cancel()
			
			if err != nil {
				b.Errorf("Concurrent gRPC CheckPermission failed: %v", err)
			}
		}
	})
}

// BenchmarkRESTConcurrentRequests benchmarks concurrent REST requests
func BenchmarkRESTConcurrentRequests(b *testing.B) {
	httpClient := setupRESTClient(b)
	
	reqBody := RESTCheckPermissionRequest{
		UserID:       defaultConfig.UserID,
		ResourceType: "table",
		ResourceID:   defaultConfig.ResourceID,
		Action:       "read",
	}
	
	jsonBody, _ := json.Marshal(reqBody)
	
	b.ResetTimer()
	
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req, err := http.NewRequest("POST", defaultConfig.RESTAddress+"/api/v1/permissions/check", bytes.NewBuffer(jsonBody))
			if err != nil {
				b.Errorf("Failed to create HTTP request: %v", err)
				continue
			}
			
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("X-User-ID", defaultConfig.UserID)
			req.Header.Set("X-Tenant-ID", defaultConfig.TenantID)
			
			resp, err := httpClient.Do(req)
			if err != nil {
				b.Errorf("Concurrent REST CheckPermission failed: %v", err)
				continue
			}
			
			resp.Body.Close()
		}
	})
}

// Helper functions for running comprehensive benchmarks

// RunComprehensiveBenchmarks runs all benchmarks and provides comparison
func RunComprehensiveBenchmarks(b *testing.B) {
	b.Run("gRPC/CheckPermission", BenchmarkGRPCCheckPermission)
	b.Run("REST/CheckPermission", BenchmarkRESTCheckPermission)
	
	b.Run("gRPC/BatchCheckPermission", BenchmarkGRPCBatchCheckPermission)
	b.Run("REST/BatchCheckPermission", BenchmarkRESTBatchCheckPermission)
	
	b.Run("gRPC/ConnectionSetup", BenchmarkGRPCConnectionSetup)
	b.Run("REST/ConnectionSetup", BenchmarkRESTConnectionSetup)
	
	b.Run("gRPC/ConcurrentRequests", BenchmarkGRPCConcurrentRequests)
	b.Run("REST/ConcurrentRequests", BenchmarkRESTConcurrentRequests)
}

// Example test function
func TestBenchmarkComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping benchmark comparison in short mode")
	}
	
	// This would typically be run with: go test -bench=. -benchmem
	t.Log("Run benchmarks with: go test -bench=. -benchmem")
}