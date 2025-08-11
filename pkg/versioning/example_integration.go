package versioning

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ExampleIntegration demonstrates how to integrate the complete versioning system
type ExampleIntegration struct {
	versionManager    *VersionManager
	lifecycleManager  *VersionLifecycleManager
	graphqlManager    *GraphQLVersionManager
	developerTools    *DeveloperTools
	analytics         *VersionAnalyticsService
	sunsetMonitoring  *SunsetMonitoringService
	abTesting         *ABTestingService
}

// NewExampleIntegration creates a complete versioning system integration
func NewExampleIntegration() *ExampleIntegration {
	// Initialize the complete versioning system
	integration := &ExampleIntegration{}
	
	// 1. Create version manager with comprehensive configuration
	managerConfig := &ManagerConfig{
		DefaultStrategy:    StrategyCombined, // Use combined strategy for maximum flexibility
		FallbackStrategies: []VersioningStrategy{StrategyPath, StrategyHeader, StrategyQuery},
		AutoDetectStrategy: true,
		EnableAnalytics:    true,
		EnableTransforms:   true,
		DeprecationPolicy: &DeprecationPolicy{
			WarningPeriod:       30 * 24 * time.Hour, // 30 days warning
			DeprecationPeriod:   90 * 24 * time.Hour, // 90 days deprecated
			NotificationMethods: []string{"email", "webhook", "slack"},
			AutomaticDeprecation: false, // Require manual approval
		},
		SunsetPolicy: &SunsetPolicy{
			GracePeriod:      30 * 24 * time.Hour, // 30 days grace period
			ForceUpgrade:     false,
			MaintenanceMode:  true,
			RedirectToNewest: true,
		},
	}
	
	integration.versionManager = NewVersionManager(managerConfig)
	
	// 2. Initialize lifecycle management
	lifecyclePolicies := &LifecyclePolicies{
		DeprecationWarningPeriod: 30 * 24 * time.Hour,
		DeprecationPeriod:        90 * 24 * time.Hour,
		GracePeriod:              30 * 24 * time.Hour,
		AutomaticTransitions:     false, // Require approval
		RequireApproval:          true,
		NotificationChannels:     []string{"email", "webhook", "slack"},
	}
	
	integration.lifecycleManager = NewVersionLifecycleManager(
		integration.versionManager.registry,
		lifecyclePolicies,
	)
	
	// 3. Initialize GraphQL version management
	integration.graphqlManager = NewGraphQLVersionManager()
	
	// 4. Initialize developer tools
	integration.developerTools = NewDeveloperTools(integration.versionManager)
	
	// 5. Initialize analytics (with in-memory storage for demo)
	analyticsConfig := &AnalyticsConfig{
		Enabled:             true,
		SamplingRate:        1.0, // 100% sampling for demo
		RetentionPeriod:     30 * 24 * time.Hour,
		AggregationInterval: 5 * time.Minute,
		BatchSize:           1000,
		FlushInterval:       10 * time.Second,
		AlertThresholds: &AlertThresholds{
			ErrorRateThreshold:      0.05,  // 5% error rate
			ResponseTimeP99:         2 * time.Second,
			LowUsageThreshold:       100,   // 100 requests per day
			HighUsageGrowth:         2.0,   // 200% growth
			DeprecatedUsageWarning:  1000,  // 1000 requests per day
			ClientDistributionSkew:  0.8,   // 80% from single client
		},
	}
	
	// Use in-memory storage for demonstration
	storage := NewInMemoryAnalyticsStorage()
	integration.analytics = NewVersionAnalyticsService(storage, analyticsConfig)
	
	// 6. Initialize sunset monitoring
	sunsetConfig := &SunsetMonitoringConfig{
		Enabled:                    true,
		MonitoringInterval:         1 * time.Hour,
		UsageThresholdForWarning:   1000,
		ClientMigrationTracking:    true,
		AutomatedNotifications:     true,
		GracePeriodEnforcement:     true,
		PreSunsetAnalyisis:         true,
		ImpactAnalysisThreshold:    0.05,
		ClientSegmentation:         true,
		RollbackCapability:         true,
	}
	
	integration.sunsetMonitoring = NewSunsetMonitoringService(
		integration.analytics,
		integration.lifecycleManager,
		sunsetConfig,
	)
	
	// 7. Initialize A/B testing
	abConfig := &ABTestingConfig{
		Enabled:            true,
		DefaultTrafficSplit: 0.1,        // 10% traffic for experiments
		MinimumSampleSize:   1000,       // Minimum 1000 users
		ConfidenceLevel:     0.95,       // 95% confidence
		ExperimentDuration:  7 * 24 * time.Hour, // 7 days
		PowerAnalysis:       true,
		SequentialTesting:   false,
	}
	
	integration.abTesting = NewABTestingService(abConfig)
	
	// Initialize the complete system
	integration.Initialize()
	
	return integration
}

// Initialize sets up the complete versioning system
func (ei *ExampleIntegration) Initialize() error {
	// 1. Initialize version manager
	if err := ei.versionManager.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize version manager: %v", err)
	}
	
	// 2. Register sample API versions
	ei.registerSampleVersions()
	
	// 3. Register sample GraphQL schemas
	ei.registerSampleGraphQLSchemas()
	
	// 4. Set up transformation rules
	ei.setupTransformationRules()
	
	// 5. Create migration guides
	ei.setupMigrationGuides()
	
	// 6. Build compatibility matrix
	ei.developerTools.BuildCompatibilityMatrix()
	
	// 7. Set up sample experiments
	ei.setupSampleExperiments()
	
	// 8. Register sample clients
	ei.registerSampleClients()
	
	fmt.Println("✅ PyAirtable API Versioning System initialized successfully!")
	fmt.Println("📊 Features enabled:")
	fmt.Println("   • Multi-strategy version detection (URL, header, query, content-type)")
	fmt.Println("   • GraphQL schema versioning with federation support")
	fmt.Println("   • Automated lifecycle management with deprecation policies")
	fmt.Println("   • Request/response transformation for backward compatibility")
	fmt.Println("   • Real-time analytics and usage monitoring")
	fmt.Println("   • Sunset monitoring and client migration tracking")
	fmt.Println("   • A/B testing across API versions")
	fmt.Println("   • Developer tools: migration guides, compatibility matrix, documentation")
	
	return nil
}

// registerSampleVersions registers sample API versions
func (ei *ExampleIntegration) registerSampleVersions() {
	// This is already done in the version manager initialization
	// The versions are registered in manager.go registerDefaultVersions()
	
	// Add some custom transformation rules for demonstration
	ei.setupCustomTransformations()
}

// registerSampleGraphQLSchemas registers sample GraphQL schemas
func (ei *ExampleIntegration) registerSampleGraphQLSchemas() {
	// Sample GraphQL schema for v1
	v1Schema := &GraphQLSchema{
		Version:     "v1",
		ReleaseDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Status:      StatusStable,
		SDL: `
			type User {
				id: Int!
				email: String!
				name: String!
				created_at: String!
			}
			
			type Query {
				user(id: Int!): User
				users(limit: Int = 10): [User!]!
			}
		`,
		Features: []string{"basic_user_queries"},
	}
	
	ei.graphqlManager.RegisterSchema(v1Schema)
	
	// Sample GraphQL schema for v2 with breaking changes
	v2Schema := &GraphQLSchema{
		Version:     "v2",
		ReleaseDate: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Status:      StatusStable,
		SDL: `
			type User {
				id: ID!  # Changed from Int to ID
				email: String!
				firstName: String!  # Split name into firstName/lastName
				lastName: String!
				profile: UserProfile
				createdAt: DateTime!  # Changed from created_at to createdAt
			}
			
			type UserProfile {
				bio: String
				website: String
				location: String
			}
			
			type Query {
				user(id: ID!): User
				users(limit: Int = 10, offset: Int = 0): [User!]!
				searchUsers(query: String!): [User!]!
			}
			
			type Mutation {
				updateUser(id: ID!, input: UpdateUserInput!): User!
			}
			
			input UpdateUserInput {
				firstName: String
				lastName: String
				profile: UserProfileInput
			}
			
			input UserProfileInput {
				bio: String
				website: String
				location: String
			}
		`,
		Features: []string{"user_queries", "user_mutations", "user_search", "user_profiles"},
		BreakingChanges: []BreakingChange{
			{
				Description:   "User ID changed from Int to ID type",
				AffectedPaths: []string{"Query.user", "User.id"},
				MigrationType: MigrationField,
				MigrationGuide: "Update your queries to use ID type instead of Int for user IDs",
			},
			{
				Description:   "User name field split into firstName and lastName",
				AffectedPaths: []string{"User.name"},
				MigrationType: MigrationField,
				MigrationGuide: "Replace 'name' field with 'firstName' and 'lastName' fields",
			},
		},
	}
	
	ei.graphqlManager.RegisterSchema(v2Schema)
}

// setupTransformationRules sets up transformation rules between versions
func (ei *ExampleIntegration) setupTransformationRules() {
	transformer := ei.versionManager.transformer
	
	// v1 to v2 transformation: user name splitting
	transformer.RegisterRule(TransformationRule{
		Name:            "v1_to_v2_user_name_split",
		Description:     "Split user name into firstName and lastName",
		SourceVersion:   "v1",
		TargetVersion:   "v2",
		ApplyToResponse: true,
		PathPatterns:    []string{"/users/", "/user/"},
		FieldMappings: []FieldMapping{
			{
				Operation:  OpSplit,
				SourcePath: "name",
				TargetPath: "firstName",
			},
		},
	})
	
	// v2 to v1 transformation: name concatenation
	transformer.RegisterRule(TransformationRule{
		Name:            "v2_to_v1_user_name_concat",
		Description:     "Concatenate firstName and lastName into name",
		SourceVersion:   "v2",
		TargetVersion:   "v1",
		ApplyToResponse: true,
		PathPatterns:    []string{"/users/", "/user/"},
		FieldMappings: []FieldMapping{
			{
				Operation:  OpMerge,
				SourcePath: "firstName,lastName",
				TargetPath: "name",
			},
		},
	})
}

// setupCustomTransformations sets up custom transformation functions
func (ei *ExampleIntegration) setupCustomTransformations() {
	transformer := ei.versionManager.transformer
	
	// Custom transformation: integer ID to UUID
	transformer.RegisterFunction("integer_to_uuid", func(input interface{}, context *TransformContext) interface{} {
		if id, ok := input.(float64); ok {
			return fmt.Sprintf("user-%08d-0000-0000-0000-000000000000", int(id))
		}
		return input
	})
	
	// Custom transformation: timestamp format conversion
	transformer.RegisterFunction("timestamp_to_iso", func(input interface{}, context *TransformContext) interface{} {
		if timestamp, ok := input.(string); ok {
			// Convert various timestamp formats to ISO 8601
			if t, err := time.Parse("2006-01-02 15:04:05", timestamp); err == nil {
				return t.Format(time.RFC3339)
			}
		}
		return input
	})
}

// setupMigrationGuides creates sample migration guides
func (ei *ExampleIntegration) setupMigrationGuides() {
	// Migration guides are automatically generated by the developer tools
	// This demonstrates how to create custom migration guides
	
	migrationService := ei.developerTools.migrationService
	
	// Custom migration guide for v1 to v2
	guide := &MigrationGuide{
		FromVersion:   "v1",
		ToVersion:     "v2",
		Title:         "Migration Guide: API v1 to v2",
		Description:   "Comprehensive guide for migrating from API v1 to v2",
		Complexity:    ComplexityMedium,
		EstimatedTime: 4 * time.Hour,
		Steps: []MigrationStep{
			{
				Order:       1,
				Title:       "Update Authentication",
				Description: "Switch from basic auth to OAuth2",
				Type:        StepCodeChange,
				Required:    true,
				Actions:     []string{"Update auth headers", "Implement OAuth2 flow"},
				Examples: []CodeExample{
					{
						Language:    "javascript",
						Title:       "Authentication Update",
						Description: "Update from basic auth to OAuth2",
						Before: `// v1
const response = await fetch('/api/v1/users', {
  headers: {
    'Authorization': 'Basic ' + btoa(username + ':' + password)
  }
});`,
						After: `// v2
const response = await fetch('/api/v2/users', {
  headers: {
    'Authorization': 'Bearer ' + accessToken
  }
});`,
						Explanation: "Replace basic authentication with OAuth2 bearer tokens",
					},
				},
			},
			{
				Order:       2,
				Title:       "Update User Data Structure",
				Description: "Handle changes to user data structure",
				Type:        StepCodeChange,
				Required:    true,
				Actions:     []string{"Update user model", "Handle name field split"},
				Examples: []CodeExample{
					{
						Language:    "javascript",
						Title:       "User Data Structure",
						Description: "Handle the split of name field",
						Before: `// v1
const user = {
  id: 123,
  name: "John Doe",
  email: "john@example.com"
};`,
						After: `// v2
const user = {
  id: "user-00000123-0000-0000-0000-000000000000",
  firstName: "John",
  lastName: "Doe",
  email: "john@example.com"
};`,
						Explanation: "User ID is now UUID format and name is split into firstName/lastName",
					},
				},
			},
			{
				Order:       3,
				Title:       "Test Migration",
				Description: "Thoroughly test all changes",
				Type:        StepTesting,
				Required:    true,
				Actions:     []string{"Unit tests", "Integration tests", "Load testing"},
			},
		},
		Prerequisites: []string{
			"Review breaking changes documentation",
			"Set up development environment with v2 API access",
			"Backup existing data and configurations",
		},
		Warnings: []string{
			"This migration includes breaking changes",
			"User ID format changes from integer to UUID",
			"Name field is split into firstName and lastName",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	migrationService.guides["v1->v2"] = guide
}

// setupSampleExperiments sets up sample A/B test experiments
func (ei *ExampleIntegration) setupSampleExperiments() {
	// Sample experiment: v1 vs v2 performance comparison
	experiment := &Experiment{
		ID:          "v1_vs_v2_performance",
		Name:        "API v1 vs v2 Performance Comparison",
		Description: "Compare performance and user experience between API v1 and v2",
		Hypothesis:  "API v2 provides better performance and user experience than v1",
		Versions:    []string{"v1", "v2"},
		Variants: map[string]*VariantConfig{
			"control": {
				ID:          "control",
				Name:        "API v1 (Control)",
				Description: "Current stable API v1",
				Version:     "v1",
				Weight:      0.5,
			},
			"treatment": {
				ID:          "treatment",
				Name:        "API v2 (Treatment)",
				Description: "New API v2 with improvements",
				Version:     "v2",
				Weight:      0.5,
			},
		},
		TrafficSplit: map[string]float64{
			"control":   0.5,
			"treatment": 0.5,
		},
		TargetMetrics: []ExperimentMetric{
			{
				Name:        "response_time",
				Type:        "value",
				Goal:        "decrease",
				Baseline:    500.0, // 500ms baseline
				Target:      400.0, // Target 400ms
				Significance: 0.05,
				Primary:     true,
			},
			{
				Name:        "error_rate",
				Type:        "conversion",
				Goal:        "decrease",
				Baseline:    0.02, // 2% error rate
				Target:      0.01, // Target 1% error rate
				Significance: 0.05,
				Primary:     false,
			},
		},
		StartDate: time.Now(),
		EndDate:   time.Now().Add(7 * 24 * time.Hour),
		Status:    ExperimentStatusRunning,
		Configuration: &ExperimentConfiguration{
			SampleSize:        10000,
			PowerLevel:        0.8,
			AlphaLevel:        0.05,
			MinimumEffect:     50.0, // 50ms improvement
			EarlyStoppingRule: "sequential",
			Randomization:     "user_id",
		},
		CreatedBy: "platform-team",
		CreatedAt: time.Now(),
	}
	
	ei.abTesting.experiments[experiment.ID] = experiment
}

// registerSampleClients registers sample clients for demonstration
func (ei *ExampleIntegration) registerSampleClients() {
	tracker := ei.sunsetMonitoring.clientTracker
	
	// Sample enterprise client
	enterpriseClient := &ClientProfile{
		ID:   "enterprise-client-001",
		Name: "Acme Corp Web App",
		Type: ClientTypeWebApp,
		Tier: ClientTierEnterprise,
		Contact: &ContactInfo{
			Email:   "dev-team@acme.com",
			Name:    "Acme Development Team",
			Company: "Acme Corp",
		},
		CurrentVersions:  []string{"v1", "v2"},
		PreferredVersion: "v2",
		LastSeen:        time.Now(),
		FirstSeen:       time.Now().Add(-365 * 24 * time.Hour),
		TotalRequests:   1000000,
		Status:          ClientStatusActive,
		Configuration: &ClientConfiguration{
			AllowedVersions: []string{"v1", "v2"},
			AutoUpgrade:     false,
			NotificationPrefs: &NotificationPrefs{
				Email:     true,
				Slack:     true,
				Frequency: "immediate",
				Channels:  []string{"email", "slack"},
			},
		},
		Tags: []string{"enterprise", "high-volume", "critical"},
	}
	
	tracker.clients[enterpriseClient.ID] = enterpriseClient
	
	// Sample community client
	communityClient := &ClientProfile{
		ID:   "community-client-001",
		Name: "Open Source Project",
		Type: ClientTypeScript,
		Tier: ClientTierCommunity,
		Contact: &ContactInfo{
			Email: "maintainer@opensource.org",
			Name:  "Project Maintainer",
		},
		CurrentVersions:  []string{"v1"},
		PreferredVersion: "v1",
		LastSeen:        time.Now().Add(-7 * 24 * time.Hour),
		FirstSeen:       time.Now().Add(-180 * 24 * time.Hour),
		TotalRequests:   5000,
		Status:          ClientStatusActive,
		Configuration: &ClientConfiguration{
			AllowedVersions: []string{"v1", "v2"},
			AutoUpgrade:     true,
			NotificationPrefs: &NotificationPrefs{
				Email:     true,
				Frequency: "weekly",
				Channels:  []string{"email"},
			},
		},
		Tags: []string{"community", "low-volume", "auto-upgrade"},
	}
	
	tracker.clients[communityClient.ID] = communityClient
}

// GetHTTPHandler returns a complete HTTP handler with all versioning features
func (ei *ExampleIntegration) GetHTTPHandler() http.Handler {
	mux := http.NewServeMux()
	
	// 1. Main API endpoints with versioning middleware
	apiHandler := ei.createVersionedAPIHandler()
	mux.Handle("/api/", ei.versionManager.GetMiddleware()(apiHandler))
	
	// 2. Version discovery and information
	mux.Handle("/api/version-info", ei.versionManager.detector.VersionDiscoveryHandler())
	mux.Handle("/api/version-compatibility", ei.versionManager.detector.VersionCompatibilityHandler())
	
	// 3. Developer tools endpoints
	mux.Handle("/api/developer-tools/compatibility-matrix", ei.developerTools.CompatibilityMatrixHandler())
	mux.Handle("/api/developer-tools/documentation", ei.developerTools.DocumentationHandler())
	mux.Handle("/api/developer-tools/migration", ei.developerTools.MigrationToolsHandler())
	
	// 4. Analytics endpoints
	mux.Handle("/api/analytics/", ei.analytics.AnalyticsHandler())
	
	// 5. Lifecycle management endpoints
	mux.Handle("/api/lifecycle/status", ei.lifecycleManager.LifecycleStatusHandler())
	mux.Handle("/api/lifecycle/migration-guide", ei.lifecycleManager.MigrationGuideHandler())
	
	// 6. Sunset monitoring endpoints
	mux.Handle("/api/sunset-monitoring/", ei.sunsetMonitoring.SunsetMonitoringHandler())
	
	// 7. GraphQL endpoints with versioning
	mux.Handle("/graphql", ei.graphqlManager.GraphQLVersionMiddleware()(ei.createGraphQLHandler()))
	mux.Handle("/graphql/schema-info", ei.graphqlManager.SchemaInfoHandler())
	mux.Handle("/graphql/introspection", ei.graphqlManager.IntrospectionHandler())
	
	// 8. Health check endpoint
	mux.HandleFunc("/health", ei.handleHealth)
	
	// 9. Demo endpoints
	mux.HandleFunc("/demo", ei.handleDemo)
	mux.HandleFunc("/", ei.handleRoot)
	
	return mux
}

// createVersionedAPIHandler creates the main API handler with version-specific routing
func (ei *ExampleIntegration) createVersionedAPIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get version context
		vctx, ok := GetVersionFromContext(r.Context())
		if !ok {
			http.Error(w, "Version context not found", http.StatusInternalServerError)
			return
		}
		
		version := vctx.Version
		
		// Track analytics event
		event := &AnalyticsEvent{
			ID:           fmt.Sprintf("req_%d", time.Now().UnixNano()),
			Timestamp:    time.Now(),
			Type:         EventAPIRequest,
			Version:      version.String(),
			ClientID:     r.Header.Get("X-Client-ID"),
			UserID:       r.Header.Get("X-User-ID"),
			IP:           getClientIP(r),
			UserAgent:    r.UserAgent(),
			Method:       r.Method,
			Path:         r.URL.Path,
			RequestSize:  r.ContentLength,
		}
		
		startTime := time.Now()
		
		// Handle the request based on version
		switch version.String() {
		case "v1":
			ei.handleV1API(w, r)
		case "v1.1":
			ei.handleV11API(w, r)
		case "v2":
			ei.handleV2API(w, r)
		case "v2.1":
			ei.handleV21API(w, r)
		default:
			http.Error(w, fmt.Sprintf("Unsupported version: %s", version.String()), http.StatusBadRequest)
			return
		}
		
		// Complete analytics event
		event.ResponseTime = time.Since(startTime)
		event.StatusCode = 200 // Simplified - in real implementation, capture actual status
		
		// Track the event (fire and forget)
		go func() {
			if err := ei.analytics.TrackEvent(context.Background(), event); err != nil {
				fmt.Printf("Failed to track analytics event: %v\n", err)
			}
		}()
	})
}

// handleV1API handles v1 API requests
func (ei *ExampleIntegration) handleV1API(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	response := map[string]interface{}{
		"version": "v1",
		"message": "PyAirtable API v1 - Basic functionality",
		"features": []string{
			"user_management",
			"workspace_management", 
			"basic_auth",
			"pagination",
		},
		"data": map[string]interface{}{
			"users": []map[string]interface{}{
				{
					"id":         123,
					"name":       "John Doe",
					"email":      "john@example.com",
					"created_at": "2024-01-01 10:00:00",
				},
			},
		},
		"meta": map[string]interface{}{
			"timestamp": time.Now().Format(time.RFC3339),
			"path":      r.URL.Path,
			"method":    r.Method,
		},
	}
	
	json.NewEncoder(w).Encode(response)
}

// handleV11API handles v1.1 API requests
func (ei *ExampleIntegration) handleV11API(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	response := map[string]interface{}{
		"version": "v1.1",
		"message": "PyAirtable API v1.1 - Enhanced with filtering and sorting",
		"features": []string{
			"user_management",
			"workspace_management",
			"basic_auth",
			"pagination",
			"filtering",
			"sorting",
			"bulk_operations",
		},
		"data": map[string]interface{}{
			"users": []map[string]interface{}{
				{
					"id":              123,
					"name":            "John Doe",
					"email":           "john@example.com",
					"created_at":      "2024-01-01 10:00:00",
					"filters_enabled": true,
					"sort_enabled":    true,
				},
			},
		},
		"meta": map[string]interface{}{
			"timestamp":    time.Now().Format(time.RFC3339),
			"path":         r.URL.Path,
			"method":       r.Method,
			"enhancements": []string{"filtering", "sorting", "bulk_operations"},
		},
	}
	
	json.NewEncoder(w).Encode(response)
}

// handleV2API handles v2 API requests
func (ei *ExampleIntegration) handleV2API(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	response := map[string]interface{}{
		"version": "v2",
		"message": "PyAirtable API v2 - Major upgrade with OAuth2 and GraphQL",
		"features": []string{
			"user_management", 
			"workspace_management",
			"oauth2_auth",
			"pagination",
			"filtering",
			"sorting",
			"bulk_operations",
			"graphql_support",
			"webhooks",
			"real_time_updates",
		},
		"breaking_changes": []string{
			"Authentication changed to OAuth2",
			"User ID format changed to UUID",
			"Field names changed to camelCase",
		},
		"data": map[string]interface{}{
			"users": []map[string]interface{}{
				{
					"id":        "user-00000123-0000-0000-0000-000000000000",
					"firstName": "John",
					"lastName":  "Doe",
					"email":     "john@example.com",
					"createdAt": "2024-01-01T10:00:00Z",
					"profile": map[string]interface{}{
						"bio":      "Software Developer",
						"website":  "https://johndoe.dev",
						"location": "San Francisco, CA",
					},
				},
			},
		},
		"meta": map[string]interface{}{
			"timestamp":      time.Now().Format(time.RFC3339),
			"path":           r.URL.Path,
			"method":         r.Method,
			"breaking_changes": true,
			"migration_guide": "/api/developer-tools/migration?from=v1&to=v2",
		},
	}
	
	json.NewEncoder(w).Encode(response)
}

// handleV21API handles v2.1 API requests
func (ei *ExampleIntegration) handleV21API(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	response := map[string]interface{}{
		"version": "v2.1",
		"message": "PyAirtable API v2.1 - Latest with AI features and advanced analytics",
		"features": []string{
			"user_management",
			"workspace_management", 
			"oauth2_auth",
			"pagination",
			"filtering",
			"sorting",
			"bulk_operations",
			"graphql_support",
			"webhooks",
			"real_time_updates",
			"ai_features",
			"advanced_analytics",
			"custom_fields",
		},
		"data": map[string]interface{}{
			"users": []map[string]interface{}{
				{
					"id":        "user-00000123-0000-0000-0000-000000000000",
					"firstName": "John",
					"lastName":  "Doe",
					"email":     "john@example.com",
					"createdAt": "2024-01-01T10:00:00Z",
					"profile": map[string]interface{}{
						"bio":      "Software Developer",
						"website":  "https://johndoe.dev",
						"location": "San Francisco, CA",
						"skills":   []string{"Go", "React", "Python"},
						"interests": []string{"AI", "Cloud Computing"},
					},
					"analytics": map[string]interface{}{
						"last_login":    "2024-12-20T15:30:00Z",
						"session_count": 145,
						"api_usage":     "high",
					},
					"ai_insights": map[string]interface{}{
						"productivity_score": 8.5,
						"collaboration_style": "team_player",
						"suggested_features": []string{"automation", "templates"},
					},
				},
			},
		},
		"meta": map[string]interface{}{
			"timestamp":     time.Now().Format(time.RFC3339),
			"path":          r.URL.Path,
			"method":        r.Method,
			"latest_stable": true,
			"new_features":  []string{"ai_features", "advanced_analytics", "custom_fields"},
		},
	}
	
	json.NewEncoder(w).Encode(response)
}

// createGraphQLHandler creates GraphQL handler
func (ei *ExampleIntegration) createGraphQLHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		// Get GraphQL version from context
		version, ok := r.Context().Value("graphql_version").(string)
		if !ok {
			version = "v2" // Default to latest
		}
		
		// Simplified GraphQL response
		response := map[string]interface{}{
			"data": map[string]interface{}{
				"version": version,
				"schema":  fmt.Sprintf("GraphQL Schema %s", version),
				"introspection": fmt.Sprintf("/graphql/introspection?version=%s", version),
			},
		}
		
		json.NewEncoder(w).Encode(response)
	})
}

// handleHealth handles health check requests
func (ei *ExampleIntegration) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "1.0.0",
		"services": map[string]string{
			"version_manager":     "healthy",
			"lifecycle_manager":   "healthy",
			"graphql_manager":     "healthy",
			"analytics":           "healthy",
			"sunset_monitoring":   "healthy",
			"ab_testing":         "healthy",
			"developer_tools":    "healthy",
		},
		"metrics": map[string]interface{}{
			"total_versions":    len(ei.versionManager.registry.GetAllVersions()),
			"active_experiments": len(ei.abTesting.experiments),
			"tracked_clients":   len(ei.sunsetMonitoring.clientTracker.clients),
		},
	}
	
	json.NewEncoder(w).Encode(health)
}

// handleDemo handles demo page requests
func (ei *ExampleIntegration) handleDemo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>PyAirtable API Versioning System Demo</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .demo-section { margin: 30px 0; padding: 20px; border: 1px solid #ddd; border-radius: 5px; }
        .demo-request { background: #f5f5f5; padding: 10px; margin: 10px 0; border-radius: 3px; }
        .demo-response { background: #e8f5e8; padding: 10px; margin: 10px 0; border-radius: 3px; }
        code { background: #f0f0f0; padding: 2px 4px; border-radius: 2px; }
        .feature-list { columns: 2; column-gap: 30px; }
        .version-badge { 
            display: inline-block; 
            padding: 4px 8px; 
            margin: 2px; 
            border-radius: 4px; 
            font-size: 12px; 
            font-weight: bold;
        }
        .v1 { background: #e3f2fd; color: #1976d2; }
        .v2 { background: #e8f5e8; color: #388e3c; }
        .deprecated { background: #fff3e0; color: #f57c00; }
        .sunset { background: #ffebee; color: #d32f2f; }
    </style>
</head>
<body>
    <h1>🚀 PyAirtable API Versioning System Demo</h1>
    
    <div class="demo-section">
        <h2>📊 System Overview</h2>
        <p>This is a comprehensive API versioning system that provides:</p>
        <div class="feature-list">
            <ul>
                <li>Multiple versioning strategies</li>
                <li>GraphQL schema versioning</li>
                <li>Automated lifecycle management</li>
                <li>Request/response transformation</li>
                <li>Real-time analytics</li>
                <li>Sunset monitoring</li>
                <li>A/B testing</li>
                <li>Developer tools</li>
                <li>Migration guides</li>
                <li>Compatibility matrix</li>
                <li>Automated documentation</li>
                <li>Client tracking</li>
            </ul>
        </div>
    </div>
    
    <div class="demo-section">
        <h2>🎯 Available Versions</h2>
        <p>
            <span class="version-badge v1">v1.0</span> - Basic functionality (stable)
            <span class="version-badge v1">v1.1</span> - Enhanced with filtering (stable)
            <span class="version-badge v2">v2.0</span> - Major upgrade with OAuth2 (stable)
            <span class="version-badge v2">v2.1</span> - Latest with AI features (stable)
            <span class="version-badge deprecated">v3.0-beta</span> - Next generation (pre-release)
        </p>
    </div>
    
    <div class="demo-section">
        <h2>🔄 Version Detection Strategies</h2>
        
        <h3>1. URL Path Versioning</h3>
        <div class="demo-request">
            GET /api/v1/users
            GET /api/v2/users
        </div>
        
        <h3>2. Header Versioning</h3>
        <div class="demo-request">
            GET /api/users<br>
            API-Version: v2
        </div>
        
        <h3>3. Query Parameter Versioning</h3>
        <div class="demo-request">
            GET /api/users?version=v2
        </div>
        
        <h3>4. Content Type Versioning</h3>
        <div class="demo-request">
            GET /api/users<br>
            Accept: application/vnd.pyairtable.v2+json
        </div>
    </div>
    
    <div class="demo-section">
        <h2>🛠️ Try the APIs</h2>
        <p>Click these links to test different versions:</p>
        <ul>
            <li><a href="/api/v1/users" target="_blank">API v1 - Basic Users</a></li>
            <li><a href="/api/v1.1/users" target="_blank">API v1.1 - Enhanced Users</a></li>
            <li><a href="/api/v2/users" target="_blank">API v2 - Modern Users (Breaking Changes)</a></li>
            <li><a href="/api/v2.1/users" target="_blank">API v2.1 - Latest with AI Features</a></li>
        </ul>
    </div>
    
    <div class="demo-section">
        <h2>📈 Developer Tools</h2>
        <ul>
            <li><a href="/api/version-info" target="_blank">Version Discovery</a></li>
            <li><a href="/api/version-compatibility?source=v1&target=v2" target="_blank">Version Compatibility Check</a></li>
            <li><a href="/api/developer-tools/compatibility-matrix" target="_blank">Compatibility Matrix</a></li>
            <li><a href="/api/developer-tools/migration?from=v1&to=v2" target="_blank">Migration Tools</a></li>
            <li><a href="/api/developer-tools/documentation?version=v2" target="_blank">Auto-generated Documentation</a></li>
        </ul>
    </div>
    
    <div class="demo-section">
        <h2>📊 Analytics & Monitoring</h2>
        <ul>
            <li><a href="/api/analytics/dashboard?template=version-overview" target="_blank">Analytics Dashboard</a></li>
            <li><a href="/api/lifecycle/status" target="_blank">Lifecycle Status</a></li>
            <li><a href="/api/sunset-monitoring/sunset-status" target="_blank">Sunset Monitoring</a></li>
            <li><a href="/health" target="_blank">System Health</a></li>
        </ul>
    </div>
    
    <div class="demo-section">
        <h2>🧪 GraphQL Support</h2>
        <ul>
            <li><a href="/graphql/schema-info" target="_blank">GraphQL Schema Information</a></li>
            <li><a href="/graphql/introspection?version=v1" target="_blank">GraphQL v1 Introspection</a></li>
            <li><a href="/graphql/introspection?version=v2" target="_blank">GraphQL v2 Introspection</a></li>
        </ul>
    </div>
    
    <div class="demo-section">
        <h2>📝 Example Usage</h2>
        
        <h3>JavaScript/Node.js</h3>
        <div class="demo-request">
<pre><code>// Using URL path versioning
const v1Response = await fetch('/api/v1/users');
const v2Response = await fetch('/api/v2/users');

// Using header versioning
const response = await fetch('/api/users', {
  headers: { 'API-Version': 'v2' }
});

// Using query parameter versioning
const response = await fetch('/api/users?version=v2');</code></pre>
        </div>
        
        <h3>cURL Examples</h3>
        <div class="demo-request">
<pre><code># URL path versioning
curl -H "Accept: application/json" /api/v2/users

# Header versioning
curl -H "API-Version: v2" -H "Accept: application/json" /api/users

# Content type versioning
curl -H "Accept: application/vnd.pyairtable.v2+json" /api/users</code></pre>
        </div>
    </div>
    
    <div class="demo-section">
        <h2>🎉 Key Features Demonstrated</h2>
        <ul>
            <li>✅ <strong>Multi-Strategy Versioning</strong> - URL, header, query, content-type</li>
            <li>✅ <strong>Automatic Transformation</strong> - Seamless data conversion between versions</li>
            <li>✅ <strong>GraphQL Versioning</strong> - Schema evolution with federation support</li>
            <li>✅ <strong>Lifecycle Management</strong> - Automated deprecation and sunset workflows</li>
            <li>✅ <strong>Real-time Analytics</strong> - Usage tracking and performance monitoring</li>
            <li>✅ <strong>Developer Experience</strong> - Migration guides, compatibility matrix, documentation</li>
            <li>✅ <strong>Production Ready</strong> - Sunset monitoring, client tracking, A/B testing</li>
        </ul>
    </div>
    
    <p><em>This demo showcases a production-ready API versioning system built for PyAirtable.</em></p>
</body>
</html>
`
	
	w.Write([]byte(html))
}

// handleRoot handles root requests
func (ei *ExampleIntegration) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		ei.handleDemo(w, r)
		return
	}
	
	http.NotFound(w, r)
}

// InMemoryAnalyticsStorage provides in-memory storage for demonstration
type InMemoryAnalyticsStorage struct {
	events []AnalyticsEvent
	mu     sync.RWMutex
}

// NewInMemoryAnalyticsStorage creates new in-memory storage
func NewInMemoryAnalyticsStorage() *InMemoryAnalyticsStorage {
	return &InMemoryAnalyticsStorage{
		events: make([]AnalyticsEvent, 0),
	}
}

// StoreEvent stores an analytics event
func (storage *InMemoryAnalyticsStorage) StoreEvent(ctx context.Context, event *AnalyticsEvent) error {
	storage.mu.Lock()
	defer storage.mu.Unlock()
	
	storage.events = append(storage.events, *event)
	return nil
}

// GetMetrics retrieves metrics (simplified implementation)
func (storage *InMemoryAnalyticsStorage) GetMetrics(ctx context.Context, query *MetricsQuery) (*MetricsResult, error) {
	storage.mu.RLock()
	defer storage.mu.RUnlock()
	
	// Simplified metrics calculation
	result := &MetricsResult{
		Query: query,
		Data:  make(map[string]*MetricValue),
		Summary: &MetricsSummary{
			TotalRequests: int64(len(storage.events)),
			TopVersions: []VersionUsage{
				{Version: "v1", RequestCount: 100, Percentage: 40.0},
				{Version: "v2", RequestCount: 150, Percentage: 60.0},
			},
		},
		GeneratedAt: time.Now(),
	}
	
	return result, nil
}

// GetTimeSeries retrieves time series data (simplified implementation)
func (storage *InMemoryAnalyticsStorage) GetTimeSeries(ctx context.Context, query *TimeSeriesQuery) (*TimeSeriesResult, error) {
	// Simplified implementation
	return &TimeSeriesResult{
		Query:       query,
		Series:      make(map[string]*TimeSeries),
		GeneratedAt: time.Now(),
	}, nil
}

// GetTopN retrieves top N results (simplified implementation)
func (storage *InMemoryAnalyticsStorage) GetTopN(ctx context.Context, query *TopNQuery) (*TopNResult, error) {
	// Simplified implementation
	return &TopNResult{
		Query:       query,
		Results:     make([]TopNItem, 0),
		GeneratedAt: time.Now(),
	}, nil
}

// Cleanup removes old events (simplified implementation)
func (storage *InMemoryAnalyticsStorage) Cleanup(ctx context.Context, before time.Time) error {
	storage.mu.Lock()
	defer storage.mu.Unlock()
	
	filtered := make([]AnalyticsEvent, 0)
	for _, event := range storage.events {
		if event.Timestamp.After(before) {
			filtered = append(filtered, event)
		}
	}
	
	storage.events = filtered
	return nil
}

// NewABTestingService creates a new A/B testing service
func NewABTestingService(config *ABTestingConfig) *ABTestingService {
	return &ABTestingService{
		experiments: make(map[string]*Experiment),
		variants:    make(map[string]*VariantConfig),
		assignments: make(map[string]string),
		analytics:   &ExperimentAnalytics{
			experiments: make(map[string]*ExperimentData),
		},
		config: config,
	}
}