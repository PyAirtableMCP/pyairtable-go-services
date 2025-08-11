package versioning

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"
)

// VersionManager coordinates all versioning functionality
type VersionManager struct {
	registry    *VersionRegistry
	detector    *VersionDetector
	transformer *Transformer
	analytics   *VersionAnalytics
	config      *ManagerConfig
	mu          sync.RWMutex
}

// ManagerConfig configures the version manager
type ManagerConfig struct {
	DefaultStrategy    VersioningStrategy        `json:"default_strategy"`
	FallbackStrategies []VersioningStrategy      `json:"fallback_strategies"`
	AutoDetectStrategy bool                      `json:"auto_detect_strategy"`
	EnableAnalytics    bool                      `json:"enable_analytics"`
	EnableTransforms   bool                      `json:"enable_transforms"`
	DeprecationPolicy  *DeprecationPolicy        `json:"deprecation_policy"`
	SunsetPolicy       *SunsetPolicy             `json:"sunset_policy"`
	ClientSupport      map[string]ClientConfig   `json:"client_support"`
}

// DeprecationPolicy defines how versions are deprecated
type DeprecationPolicy struct {
	WarningPeriod      time.Duration `json:"warning_period"`       // How long to warn before deprecation
	DeprecationPeriod  time.Duration `json:"deprecation_period"`   // How long deprecated versions are supported
	NotificationMethods []string     `json:"notification_methods"` // email, webhook, etc.
	AutomaticDeprecation bool        `json:"automatic_deprecation"`
}

// SunsetPolicy defines how versions are sunset
type SunsetPolicy struct {
	GracePeriod        time.Duration `json:"grace_period"`         // Grace period after sunset
	ForceUpgrade       bool          `json:"force_upgrade"`        // Force clients to upgrade
	MaintenanceMode    bool          `json:"maintenance_mode"`     // Maintenance mode for deprecated versions
	RedirectToNewest   bool          `json:"redirect_to_newest"`   // Redirect to newest version
}

// ClientConfig defines per-client version support
type ClientConfig struct {
	Name              string   `json:"name"`
	SupportedVersions []string `json:"supported_versions"`
	DefaultVersion    string   `json:"default_version"`
	MinimumVersion    string   `json:"minimum_version"`
	AutoUpgrade       bool     `json:"auto_upgrade"`
}

// VersionAnalytics tracks version usage and performance
type VersionAnalytics struct {
	UsageStats     map[string]*UsageStats     `json:"usage_stats"`
	PerformanceStats map[string]*PerformanceStats `json:"performance_stats"`
	ErrorStats     map[string]*ErrorStats     `json:"error_stats"`
	ClientStats    map[string]*ClientStats    `json:"client_stats"`
	mu             sync.RWMutex
}

// UsageStats tracks version usage statistics
type UsageStats struct {
	Version      string    `json:"version"`
	RequestCount int64     `json:"request_count"`
	UniqueUsers  int64     `json:"unique_users"`
	LastUsed     time.Time `json:"last_used"`
	DailyUsage   map[string]int64 `json:"daily_usage"` // date -> count
}

// PerformanceStats tracks version performance metrics
type PerformanceStats struct {
	Version         string        `json:"version"`
	AvgResponseTime time.Duration `json:"avg_response_time"`
	P95ResponseTime time.Duration `json:"p95_response_time"`
	P99ResponseTime time.Duration `json:"p99_response_time"`
	ThroughputRPS   float64       `json:"throughput_rps"`
}

// ErrorStats tracks version error rates
type ErrorStats struct {
	Version    string  `json:"version"`
	ErrorRate  float64 `json:"error_rate"`
	ErrorCount int64   `json:"error_count"`
	TotalRequests int64 `json:"total_requests"`
}

// ClientStats tracks per-client version usage
type ClientStats struct {
	ClientID     string            `json:"client_id"`
	VersionUsage map[string]int64  `json:"version_usage"`
	LastSeen     time.Time         `json:"last_seen"`
}

// NewVersionManager creates a new version manager
func NewVersionManager(config *ManagerConfig) *VersionManager {
	if config == nil {
		config = &ManagerConfig{
			DefaultStrategy:    StrategyPath,
			FallbackStrategies: []VersioningStrategy{StrategyHeader, StrategyQuery},
			AutoDetectStrategy: true,
			EnableAnalytics:    true,
			EnableTransforms:   true,
		}
	}
	
	registry := NewVersionRegistry("v1")
	analytics := &VersionAnalytics{
		UsageStats:       make(map[string]*UsageStats),
		PerformanceStats: make(map[string]*PerformanceStats),
		ErrorStats:       make(map[string]*ErrorStats),
		ClientStats:      make(map[string]*ClientStats),
	}
	
	return &VersionManager{
		registry:    registry,
		analytics:   analytics,
		transformer: NewTransformer(),
		config:      config,
	}
}

// Initialize sets up the version manager with initial versions and rules
func (vm *VersionManager) Initialize() error {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	// Register default versions
	vm.registerDefaultVersions()
	
	// Set up version detector
	detectorConfig := &VersionConfig{
		DefaultVersion:    vm.registry.defaultVersion,
		SupportedVersions: vm.getSupportedVersionStrings(),
		Strategy:          vm.config.DefaultStrategy,
		PathPrefix:        "/api",
		HeaderName:        "API-Version",
		QueryParam:        "version",
		MediaTypePrefix:   "application/vnd.pyairtable",
		DeprecationWarning: true,
		StrictVersioning:   false,
		VersionRegistry:    vm.registry.GetAllVersions(),
	}
	
	vm.detector = NewVersionDetector(detectorConfig)
	
	// Register default transformation rules
	vm.registerDefaultTransformRules()
	
	return nil
}

// registerDefaultVersions registers the default API versions
func (vm *VersionManager) registerDefaultVersions() {
	// V1 - Initial stable version
	v1 := &Version{
		Major:       1,
		Minor:       0,
		Patch:       0,
		ReleaseDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Status:      StatusStable,
		Features: []string{
			"user_management",
			"workspace_management",
			"basic_auth",
			"pagination",
		},
	}
	vm.registry.RegisterVersion(v1)
	
	// V1.1 - Minor update with new features
	v11 := &Version{
		Major:       1,
		Minor:       1,
		Patch:       0,
		ReleaseDate: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		Status:      StatusStable,
		Features: []string{
			"user_management",
			"workspace_management",
			"basic_auth",
			"pagination",
			"filtering",
			"sorting",
			"bulk_operations",
		},
	}
	vm.registry.RegisterVersion(v11)
	
	// V2 - Major version with breaking changes
	v2 := &Version{
		Major:       2,
		Minor:       0,
		Patch:       0,
		ReleaseDate: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Status:      StatusStable,
		Features: []string{
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
		BreakingChanges: []BreakingChange{
			{
				Description:   "Authentication changed from basic auth to OAuth2",
				AffectedPaths: []string{"/api/v2/auth/*", "/api/v2/login"},
				MigrationType: MigrationBehavior,
				MigrationGuide: "Update your authentication to use OAuth2 instead of basic auth",
				AutoMigration: false,
			},
			{
				Description:   "User ID format changed from integer to UUID",
				AffectedPaths: []string{"/api/v2/users/*"},
				MigrationType: MigrationField,
				MigrationGuide: "Update your code to handle UUID format for user IDs",
				AutoMigration: false,
			},
		},
	}
	vm.registry.RegisterVersion(v2)
	
	// V2.1 - Current latest version
	v21 := &Version{
		Major:       2,
		Minor:       1,
		Patch:       0,
		ReleaseDate: time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC),
		Status:      StatusStable,
		Features: []string{
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
	}
	vm.registry.RegisterVersion(v21)
	
	// V3 - Beta version
	v3 := &Version{
		Major:       3,
		Minor:       0,
		Patch:       0,
		Label:       "beta",
		ReleaseDate: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
		Status:      StatusPreRelease,
		Features: []string{
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
			"microservices_architecture",
			"event_sourcing",
			"cqrs",
		},
	}
	vm.registry.RegisterVersion(v3)
}

// registerDefaultTransformRules registers transformation rules between versions
func (vm *VersionManager) registerDefaultTransformRules() {
	// V1 to V1.1 transformations (backward compatible)
	vm.transformer.RegisterRule(TransformationRule{
		Name:            "v1_to_v1.1_response",
		Description:     "Add default values for new fields in v1.1",
		SourceVersion:   "v1",
		TargetVersion:   "v1.1",
		ApplyToResponse: true,
		FieldMappings: []FieldMapping{
			{
				Operation:    OpAdd,
				TargetPath:   "filters_enabled",
				DefaultValue: false,
			},
			{
				Operation:    OpAdd,
				TargetPath:   "sort_enabled",
				DefaultValue: false,
			},
		},
	})
	
	// V1.1 to V2 transformations (breaking changes)
	vm.transformer.RegisterRule(TransformationRule{
		Name:           "v1.1_to_v2_user_id",
		Description:    "Transform integer user IDs to UUID format",
		SourceVersion:  "v1.1",
		TargetVersion:  "v2",
		ApplyToRequest: true,
		ApplyToResponse: true,
		PathPatterns:   []string{"/users/", "/workspaces/"},
		FieldMappings: []FieldMapping{
			{
				Operation:  OpTransform,
				SourcePath: "user_id",
				TargetPath: "user_id",
				Transform:  "integer_to_uuid",
			},
			{
				Operation:  OpTransform,
				SourcePath: "id",
				TargetPath: "id",
				Transform:  "integer_to_uuid",
			},
		},
	})
	
	// Register transformation functions
	vm.transformer.RegisterFunction("integer_to_uuid", func(input interface{}, context *TransformContext) interface{} {
		if id, ok := input.(float64); ok {
			// Convert integer ID to UUID format (this is a placeholder implementation)
			return fmt.Sprintf("user-%08d-0000-0000-0000-000000000000", int(id))
		}
		return input
	})
}

// DetectVersion detects the API version from an HTTP request
func (vm *VersionManager) DetectVersion(r *http.Request) (*Version, VersioningStrategy, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	// Try primary strategy first
	version, err := vm.detector.DetectVersion(r)
	if err == nil && version != nil {
		return version, vm.config.DefaultStrategy, nil
	}
	
	// Try fallback strategies if auto-detect is enabled
	if vm.config.AutoDetectStrategy {
		originalStrategy := vm.detector.config.Strategy
		
		for _, strategy := range vm.config.FallbackStrategies {
			vm.detector.config.Strategy = strategy
			version, err := vm.detector.DetectVersion(r)
			if err == nil && version != nil {
				vm.detector.config.Strategy = originalStrategy
				return version, strategy, nil
			}
		}
		
		vm.detector.config.Strategy = originalStrategy
	}
	
	// Return default version if all strategies fail
	defaultVersion, exists := vm.registry.GetDefaultVersion()
	if !exists {
		return nil, "", fmt.Errorf("no default version configured")
	}
	
	return defaultVersion, vm.config.DefaultStrategy, nil
}

// GetMiddleware returns the complete versioning middleware stack
func (vm *VersionManager) GetMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Detect version
			version, strategy, err := vm.DetectVersion(r)
			if err != nil {
				vm.writeError(w, http.StatusBadRequest, "VERSION_DETECTION_FAILED", err.Error())
				return
			}
			
			// Add version context
			versionContext := &VersionContext{
				Version:    version,
				DetectedBy: strategy,
				RequestID:  generateRequestID(),
			}
			
			ctx := WithVersionContext(r.Context(), versionContext)
			r = r.WithContext(ctx)
			
			// Add response headers
			vm.detector.AddResponseHeaders(w, version)
			
			// Track analytics
			if vm.config.EnableAnalytics {
				vm.trackRequest(r, version, strategy)
			}
			
			// Apply transformations if enabled
			if vm.config.EnableTransforms {
				transformMiddleware := NewTransformMiddleware(vm.transformer)
				transformMiddleware.Middleware(next).ServeHTTP(w, r)
			} else {
				next.ServeHTTP(w, r)
			}
		})
	}
}

// trackRequest tracks version usage for analytics
func (vm *VersionManager) trackRequest(r *http.Request, version *Version, strategy VersioningStrategy) {
	vm.analytics.mu.Lock()
	defer vm.analytics.mu.Unlock()
	
	versionStr := version.String()
	
	// Update usage stats
	if stats, exists := vm.analytics.UsageStats[versionStr]; exists {
		stats.RequestCount++
		stats.LastUsed = time.Now()
	} else {
		vm.analytics.UsageStats[versionStr] = &UsageStats{
			Version:      versionStr,
			RequestCount: 1,
			UniqueUsers:  1,
			LastUsed:     time.Now(),
			DailyUsage:   make(map[string]int64),
		}
	}
	
	// Update daily usage
	today := time.Now().Format("2006-01-02")
	vm.analytics.UsageStats[versionStr].DailyUsage[today]++
	
	// Track client stats
	clientID := vm.getClientID(r)
	if clientID != "" {
		if stats, exists := vm.analytics.ClientStats[clientID]; exists {
			stats.VersionUsage[versionStr]++
			stats.LastSeen = time.Now()
		} else {
			vm.analytics.ClientStats[clientID] = &ClientStats{
				ClientID:     clientID,
				VersionUsage: map[string]int64{versionStr: 1},
				LastSeen:     time.Now(),
			}
		}
	}
}

// getClientID extracts client ID from request
func (vm *VersionManager) getClientID(r *http.Request) string {
	// Try various methods to identify the client
	if clientID := r.Header.Get("X-Client-ID"); clientID != "" {
		return clientID
	}
	
	if userAgent := r.Header.Get("User-Agent"); userAgent != "" {
		return userAgent
	}
	
	return ""
}

// GetVersionInfo returns version information for discovery
func (vm *VersionManager) GetVersionInfo(currentVersion string) *VersionInfo {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	return vm.registry.BuildVersionInfo(currentVersion, vm.config.DefaultStrategy)
}

// GetAnalytics returns version usage analytics
func (vm *VersionManager) GetAnalytics() *VersionAnalytics {
	vm.analytics.mu.RLock()
	defer vm.analytics.mu.RUnlock()
	
	// Return a copy to avoid race conditions
	analytics := &VersionAnalytics{
		UsageStats:       make(map[string]*UsageStats),
		PerformanceStats: make(map[string]*PerformanceStats),
		ErrorStats:       make(map[string]*ErrorStats),
		ClientStats:      make(map[string]*ClientStats),
	}
	
	for k, v := range vm.analytics.UsageStats {
		analytics.UsageStats[k] = v
	}
	
	for k, v := range vm.analytics.PerformanceStats {
		analytics.PerformanceStats[k] = v
	}
	
	for k, v := range vm.analytics.ErrorStats {
		analytics.ErrorStats[k] = v
	}
	
	for k, v := range vm.analytics.ClientStats {
		analytics.ClientStats[k] = v
	}
	
	return analytics
}

// getSupportedVersionStrings returns supported version strings
func (vm *VersionManager) getSupportedVersionStrings() []string {
	supported := vm.registry.GetSupportedVersions()
	versions := make([]string, 0, len(supported))
	
	for versionStr := range supported {
		versions = append(versions, versionStr)
	}
	
	sort.Strings(versions)
	return versions
}

// writeError writes an error response
func (vm *VersionManager) writeError(w http.ResponseWriter, statusCode int, errorCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	errorResponse := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    errorCode,
			"message": message,
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	
	json.NewEncoder(w).Encode(errorResponse)
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}

// RegisterVersion registers a new API version
func (vm *VersionManager) RegisterVersion(version *Version) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	vm.registry.RegisterVersion(version)
	
	// Update detector configuration
	vm.detector.config.SupportedVersions = vm.getSupportedVersionStrings()
	vm.detector.config.VersionRegistry = vm.registry.GetAllVersions()
}

// DeprecateVersion marks a version as deprecated
func (vm *VersionManager) DeprecateVersion(versionStr string, sunsetDate time.Time) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	version, exists := vm.registry.GetVersion(versionStr)
	if !exists {
		return fmt.Errorf("version %s not found", versionStr)
	}
	
	now := time.Now()
	version.Status = StatusDeprecated
	version.DeprecatedDate = &now
	version.SunsetDate = &sunsetDate
	
	return nil
}

// SunsetVersion marks a version as sunset
func (vm *VersionManager) SunsetVersion(versionStr string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	version, exists := vm.registry.GetVersion(versionStr)
	if !exists {
		return fmt.Errorf("version %s not found", versionStr)
	}
	
	version.Status = StatusSunset
	
	return nil
}