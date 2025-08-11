package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pyairtable/pyairtable-compose/go-services/pkg/versioning"
)

func main() {
	// Create the complete versioning system integration
	integration := versioning.NewExampleIntegration()
	
	// Create HTTP server with all versioning features
	handler := integration.GetHTTPHandler()
	server := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	// Start background services
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start monitoring services
	go startBackgroundServices(ctx, integration)
	
	// Start HTTP server
	go func() {
		fmt.Println("\n🚀 PyAirtable API Versioning System Demo")
		fmt.Println("=======================================")
		fmt.Printf("🌐 Server starting on http://localhost%s\n", server.Addr)
		fmt.Println("\n📖 Available endpoints:")
		fmt.Println("   • Demo page:              http://localhost:8080/")
		fmt.Println("   • Health check:           http://localhost:8080/health")
		fmt.Println("   • API v1:                 http://localhost:8080/api/v1/users")
		fmt.Println("   • API v2:                 http://localhost:8080/api/v2/users")
		fmt.Println("   • Version info:           http://localhost:8080/api/version-info")
		fmt.Println("   • Compatibility check:    http://localhost:8080/api/version-compatibility?source=v1&target=v2")
		fmt.Println("   • Migration tools:        http://localhost:8080/api/developer-tools/migration?from=v1&to=v2")
		fmt.Println("   • Analytics dashboard:    http://localhost:8080/api/analytics/dashboard")
		fmt.Println("   • GraphQL schema info:    http://localhost:8080/graphql/schema-info")
		fmt.Println("\n💡 Try different versioning strategies:")
		fmt.Println("   • URL path:    curl http://localhost:8080/api/v2/users")
		fmt.Println("   • Header:      curl -H 'API-Version: v2' http://localhost:8080/api/users")
		fmt.Println("   • Query param: curl http://localhost:8080/api/users?version=v2")
		fmt.Println("   • Content-type: curl -H 'Accept: application/vnd.pyairtable.v2+json' http://localhost:8080/api/users")
		fmt.Println("\n🎯 Features demonstrated:")
		fmt.Println("   ✅ Multi-strategy version detection")
		fmt.Println("   ✅ GraphQL schema versioning")
		fmt.Println("   ✅ Automated lifecycle management")
		fmt.Println("   ✅ Request/response transformation")
		fmt.Println("   ✅ Real-time analytics")
		fmt.Println("   ✅ Sunset monitoring")
		fmt.Println("   ✅ A/B testing")
		fmt.Println("   ✅ Developer tools")
		fmt.Println("   ✅ Migration guides")
		fmt.Println("   ✅ Compatibility matrix")
		fmt.Println("\n📚 Open http://localhost:8080/ in your browser for the interactive demo!")
		fmt.Println("\n🛑 Press Ctrl+C to stop the server")
		fmt.Println("=======================================\n")
		
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()
	
	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	
	fmt.Println("\n🛑 Shutting down server...")
	
	// Cancel background services
	cancel()
	
	// Shutdown server gracefully
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	} else {
		fmt.Println("✅ Server shutdown completed")
	}
}

// startBackgroundServices starts background monitoring and processing services
func startBackgroundServices(ctx context.Context, integration *versioning.ExampleIntegration) {
	// Start periodic tasks
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			fmt.Println("🛑 Stopping background services...")
			return
		case <-ticker.C:
			performPeriodicTasks(integration)
		}
	}
}

// performPeriodicTasks performs periodic maintenance tasks
func performPeriodicTasks(integration *versioning.ExampleIntegration) {
	// This would normally include:
	// - Processing scheduled lifecycle transitions
	// - Cleaning up old analytics data
	// - Sending deprecation notifications
	// - Updating client migration status
	// - Evaluating A/B test results
	
	// For demo purposes, just show that background tasks are running
	fmt.Printf("⏰ Background tasks executed at %s\n", time.Now().Format("15:04:05"))
}

// Demonstration functions that can be called from CLI or tests

// DemoVersionDetection demonstrates different version detection strategies
func DemoVersionDetection() {
	fmt.Println("\n🔍 Version Detection Strategies Demo")
	fmt.Println("===================================")
	
	integration := versioning.NewExampleIntegration()
	
	// Create sample requests with different version detection methods
	testCases := []struct {
		name    string
		method  string
		path    string
		headers map[string]string
	}{
		{
			name:   "URL Path Versioning",
			method: "GET",
			path:   "/api/v2/users",
			headers: map[string]string{},
		},
		{
			name:   "Header Versioning",
			method: "GET", 
			path:   "/api/users",
			headers: map[string]string{"API-Version": "v2"},
		},
		{
			name:   "Query Parameter Versioning",
			method: "GET",
			path:   "/api/users?version=v2",
			headers: map[string]string{},
		},
		{
			name:   "Content Type Versioning",
			method: "GET",
			path:   "/api/users",
			headers: map[string]string{"Accept": "application/vnd.pyairtable.v2+json"},
		},
	}
	
	for _, tc := range testCases {
		fmt.Printf("\n📍 %s:\n", tc.name)
		fmt.Printf("   Request: %s %s\n", tc.method, tc.path)
		
		for key, value := range tc.headers {
			fmt.Printf("   Header: %s: %s\n", key, value)
		}
		
		// In a real demo, you would create HTTP requests and show version detection
		fmt.Printf("   ✅ Would detect version: v2\n")
	}
}

// DemoTransformation demonstrates data transformation between versions
func DemoTransformation() {
	fmt.Println("\n🔄 Data Transformation Demo")
	fmt.Println("==========================")
	
	// Example v1 data
	v1Data := map[string]interface{}{
		"id":         123,
		"name":       "John Doe",
		"email":      "john@example.com",
		"created_at": "2024-01-01 10:00:00",
	}
	
	// Example v2 data (after transformation)
	v2Data := map[string]interface{}{
		"id":        "user-00000123-0000-0000-0000-000000000000",
		"firstName": "John",
		"lastName":  "Doe",
		"email":     "john@example.com",
		"createdAt": "2024-01-01T10:00:00Z",
		"profile": map[string]interface{}{
			"bio":      nil,
			"website":  nil,
			"location": nil,
		},
	}
	
	fmt.Printf("📥 V1 Data:\n")
	printJSON(v1Data)
	
	fmt.Printf("\n🔄 Transformation Rules Applied:\n")
	fmt.Printf("   • ID: integer → UUID format\n")
	fmt.Printf("   • name → firstName + lastName\n")
	fmt.Printf("   • created_at → createdAt (ISO format)\n")
	fmt.Printf("   • Added profile object with defaults\n")
	
	fmt.Printf("\n📤 V2 Data:\n")
	printJSON(v2Data)
}

// DemoLifecycleManagement demonstrates version lifecycle management
func DemoLifecycleManagement() {
	fmt.Println("\n⏳ Version Lifecycle Management Demo")
	fmt.Println("==================================")
	
	stages := []struct {
		stage       string
		description string
		duration    string
		actions     []string
	}{
		{
			stage:       "Development",
			description: "Version is being developed",
			duration:    "2-6 months",
			actions:     []string{"Feature development", "Internal testing", "API design"},
		},
		{
			stage:       "Pre-release",
			description: "Beta/alpha testing with select users",
			duration:    "2-4 weeks",
			actions:     []string{"Beta testing", "Documentation", "SDK updates"},
		},
		{
			stage:       "Stable",
			description: "Generally available for all users",
			duration:    "12-24 months",
			actions:     []string{"Full support", "Bug fixes", "Performance optimization"},
		},
		{
			stage:       "Deprecated",
			description: "Marked for removal, users should migrate",
			duration:    "3-6 months",
			actions:     []string{"Migration notifications", "Support migration", "Sunset warnings"},
		},
		{
			stage:       "Sunset",
			description: "No longer supported, blocked after grace period",
			duration:    "1-3 months grace period",
			actions:     []string{"Block new usage", "Final migration push", "Remove infrastructure"},
		},
	}
	
	for i, stage := range stages {
		fmt.Printf("\n%d. 📍 %s (%s)\n", i+1, stage.stage, stage.duration)
		fmt.Printf("   %s\n", stage.description)
		fmt.Printf("   Actions:\n")
		for _, action := range stage.actions {
			fmt.Printf("   • %s\n", action)
		}
	}
	
	fmt.Printf("\n⚠️  Automated Policies:\n")
	fmt.Printf("   • 30 days warning before deprecation\n")
	fmt.Printf("   • 90 days deprecated period\n")
	fmt.Printf("   • 30 days grace period after sunset\n")
	fmt.Printf("   • Automated notifications to clients\n")
	fmt.Printf("   • Migration guide generation\n")
}

// printJSON prints a map as formatted JSON
func printJSON(data map[string]interface{}) {
	// Simple JSON-like formatting for demo
	fmt.Printf("   {\n")
	for key, value := range data {
		switch v := value.(type) {
		case string:
			fmt.Printf("     \"%s\": \"%s\",\n", key, v)
		case int:
			fmt.Printf("     \"%s\": %d,\n", key, v)
		case map[string]interface{}:
			fmt.Printf("     \"%s\": {...},\n", key)
		case nil:
			fmt.Printf("     \"%s\": null,\n", key)
		default:
			fmt.Printf("     \"%s\": %v,\n", key, v)
		}
	}
	fmt.Printf("   }\n")
}

// init function runs when the package is initialized
func init() {
	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	
	// Check if this is a demo run
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "demo-detection":
			DemoVersionDetection()
			os.Exit(0)
		case "demo-transformation":
			DemoTransformation()
			os.Exit(0)
		case "demo-lifecycle":
			DemoLifecycleManagement()
			os.Exit(0)
		}
	}
}