package versioning

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// VersionMiddleware handles API versioning for HTTP requests
type VersionMiddleware struct {
	detector *VersionDetector
	registry *VersionRegistry
	config   *MiddlewareConfig
}

// MiddlewareConfig configures the version middleware
type MiddlewareConfig struct {
	ErrorOnUnsupported bool                      `json:"error_on_unsupported"`
	LogVersionUsage    bool                      `json:"log_version_usage"`
	TrackAnalytics     bool                      `json:"track_analytics"`
	ResponseHeaders    bool                      `json:"response_headers"`
	VersionRouting     map[string]http.Handler   `json:"-"` // Version-specific handlers
	DefaultHandler     http.Handler              `json:"-"` // Default handler
	FeatureFlags       map[string]map[string]bool `json:"feature_flags"` // version -> feature -> enabled
}

// NewVersionMiddleware creates a new version middleware
func NewVersionMiddleware(detector *VersionDetector, registry *VersionRegistry, config *MiddlewareConfig) *VersionMiddleware {
	if config == nil {
		config = &MiddlewareConfig{
			ErrorOnUnsupported: false,
			LogVersionUsage:    true,
			TrackAnalytics:     true,
			ResponseHeaders:    true,
			VersionRouting:     make(map[string]http.Handler),
			FeatureFlags:       make(map[string]map[string]bool),
		}
	}
	
	return &VersionMiddleware{
		detector: detector,
		registry: registry,
		config:   config,
	}
}

// Middleware returns the HTTP middleware function
func (vm *VersionMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Generate request ID for tracking
		requestID := uuid.New().String()
		
		// Detect version from request
		version, err := vm.detector.DetectVersion(r)
		if err != nil {
			if vm.config.ErrorOnUnsupported {
				vm.writeErrorResponse(w, http.StatusBadRequest, "UNSUPPORTED_VERSION", err.Error())
				return
			}
			// Fall back to default version
			defaultVersion, exists := vm.registry.GetDefaultVersion()
			if !exists {
				vm.writeErrorResponse(w, http.StatusInternalServerError, "NO_DEFAULT_VERSION", "No default version configured")
				return
			}
			version = defaultVersion
		}
		
		// Check if version is sunset
		if version.IsSunset() {
			vm.writeErrorResponse(w, http.StatusGone, "VERSION_SUNSET", fmt.Sprintf("API version %s is no longer supported", version.String()))
			return
		}
		
		// Add version context to request
		versionContext := &VersionContext{
			Version:    version,
			DetectedBy: vm.detector.config.Strategy,
			RequestID:  requestID,
		}
		
		ctx := WithVersionContext(r.Context(), versionContext)
		r = r.WithContext(ctx)
		
		// Add response headers
		if vm.config.ResponseHeaders {
			vm.detector.AddResponseHeaders(w, version)
		}
		
		// Log version usage
		if vm.config.LogVersionUsage {
			vm.logVersionUsage(r, version, requestID)
		}
		
		// Track analytics
		if vm.config.TrackAnalytics {
			vm.trackVersionAnalytics(r, version, requestID)
		}
		
		// Route to version-specific handler if available
		if handler, exists := vm.config.VersionRouting[version.String()]; exists {
			handler.ServeHTTP(w, r)
			return
		}
		
		// Use default handler
		if vm.config.DefaultHandler != nil {
			vm.config.DefaultHandler.ServeHTTP(w, r)
			return
		}
		
		// Continue with next middleware
		next.ServeHTTP(w, r)
	})
}

// writeErrorResponse writes a standardized error response
func (vm *VersionMiddleware) writeErrorResponse(w http.ResponseWriter, statusCode int, errorCode, message string) {
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

// logVersionUsage logs version usage for monitoring
func (vm *VersionMiddleware) logVersionUsage(r *http.Request, version *Version, requestID string) {
	// This would integrate with your logging system
	fmt.Printf("[API-VERSION] RequestID=%s Version=%s Method=%s Path=%s UserAgent=%s IP=%s\n",
		requestID,
		version.String(),
		r.Method,
		r.URL.Path,
		r.UserAgent(),
		getClientIP(r),
	)
}

// trackVersionAnalytics tracks version usage for analytics
func (vm *VersionMiddleware) trackVersionAnalytics(r *http.Request, version *Version, requestID string) {
	// This would integrate with your analytics system
	analytics := map[string]interface{}{
		"event":      "api_version_used",
		"request_id": requestID,
		"version":    version.String(),
		"method":     r.Method,
		"path":       r.URL.Path,
		"user_agent": r.UserAgent(),
		"client_ip":  getClientIP(r),
		"timestamp":  time.Now().UTC(),
		"features":   version.Features,
		"status":     version.Status,
	}
	
	// Send to analytics service (placeholder)
	fmt.Printf("[ANALYTICS] %+v\n", analytics)
}

// getClientIP extracts client IP from request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(forwarded, ",")
		return strings.TrimSpace(ips[0])
	}
	
	// Check X-Real-IP header  
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}
	
	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// VersionDiscoveryHandler provides version discovery endpoint
func (vm *VersionMiddleware) VersionDiscoveryHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		// Get current version from context or use default
		currentVersion := vm.detector.config.DefaultVersion
		if vctx, ok := GetVersionFromContext(r.Context()); ok {
			currentVersion = vctx.Version.String()  
		}
		
		versionInfo := vm.registry.BuildVersionInfo(currentVersion, vm.detector.config.Strategy)
		
		json.NewEncoder(w).Encode(versionInfo)
	})
}

// VersionCompatibilityHandler checks version compatibility
func (vm *VersionMiddleware) VersionCompatibilityHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		sourceVersion := r.URL.Query().Get("source")
		targetVersion := r.URL.Query().Get("target")
		
		if sourceVersion == "" || targetVersion == "" {
			vm.writeErrorResponse(w, http.StatusBadRequest, "MISSING_PARAMETERS", "Both source and target versions are required")
			return
		}
		
		source, sourceExists := vm.registry.GetVersion(sourceVersion)
		target, targetExists := vm.registry.GetVersion(targetVersion)
		
		if !sourceExists {
			vm.writeErrorResponse(w, http.StatusNotFound, "SOURCE_VERSION_NOT_FOUND", fmt.Sprintf("Source version %s not found", sourceVersion))
			return
		}
		
		if !targetExists {
			vm.writeErrorResponse(w, http.StatusNotFound, "TARGET_VERSION_NOT_FOUND", fmt.Sprintf("Target version %s not found", targetVersion))
			return
		}
		
		compatibility := map[string]interface{}{
			"source_version":      sourceVersion,
			"target_version":      targetVersion,
			"compatible":          source.IsCompatibleWith(target),
			"breaking_changes":    vm.getBreakingChangesBetween(source, target),
			"migration_required":  !source.IsCompatibleWith(target),
			"migration_guide":     vm.getMigrationGuide(source, target),
		}
		
		json.NewEncoder(w).Encode(compatibility)
	})
}

// getBreakingChangesBetween returns breaking changes between two versions
func (vm *VersionMiddleware) getBreakingChangesBetween(source, target *Version) []BreakingChange {
	var changes []BreakingChange
	
	// If target is newer, collect breaking changes from source to target
	if target.IsNewerThan(source) {
		for _, version := range vm.registry.GetAllVersions() {
			if version.IsNewerThan(source) && !version.IsNewerThan(target) {
				changes = append(changes, version.BreakingChanges...)
			}
		}
	}
	
	return changes
}

// getMigrationGuide generates a migration guide between versions
func (vm *VersionMiddleware) getMigrationGuide(source, target *Version) string {
	if source.IsCompatibleWith(target) {
		return "No migration required - versions are compatible"
	}
	
	return fmt.Sprintf("Migration required from %s to %s. Please refer to the API documentation for detailed migration steps.", 
		source.String(), target.String())
}

// FeatureFlagMiddleware adds feature flag support per version
func (vm *VersionMiddleware) FeatureFlagMiddleware(feature string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			vctx, ok := GetVersionFromContext(r.Context())
			if !ok {
				vm.writeErrorResponse(w, http.StatusInternalServerError, "NO_VERSION_CONTEXT", "Version context not found")
				return
			}
			
			version := vctx.Version.String()
			if versionFlags, exists := vm.config.FeatureFlags[version]; exists {
				if enabled, flagExists := versionFlags[feature]; flagExists && !enabled {
					vm.writeErrorResponse(w, http.StatusNotFound, "FEATURE_NOT_AVAILABLE", 
						fmt.Sprintf("Feature %s is not available in version %s", feature, version))
					return
				}
			}
			
			next.ServeHTTP(w, r)
		})
	}
}

// ABTestMiddleware enables A/B testing across versions
func (vm *VersionMiddleware) ABTestMiddleware(testName string, variants map[string]string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			vctx, ok := GetVersionFromContext(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			
			// Simple hash-based variant selection
			userID := r.Header.Get("X-User-ID")
			if userID == "" {
				userID = getClientIP(r)
			}
			
			variant := vm.selectVariant(userID, testName, variants)
			
			// Add variant to response headers
			w.Header().Set("X-AB-Test", testName)
			w.Header().Set("X-AB-Variant", variant)
			
			// Track A/B test analytics
			if vm.config.TrackAnalytics {
				vm.trackABTest(r, vctx.Version, testName, variant, vctx.RequestID)
			}
			
			next.ServeHTTP(w, r)
		})
	}
}

// selectVariant selects an A/B test variant based on user ID
func (vm *VersionMiddleware) selectVariant(userID, testName string, variants map[string]string) string {
	if len(variants) == 0 {
		return "control"
	}
	
	// Simple hash-based selection
	hash := 0
	for _, c := range userID + testName {
		hash = hash*31 + int(c)
	}
	
	if hash < 0 {
		hash = -hash
	}
	
	variantNames := make([]string, 0, len(variants))
	for name := range variants {
		variantNames = append(variantNames, name)
	}
	
	return variantNames[hash%len(variantNames)]
}

// trackABTest tracks A/B test participation
func (vm *VersionMiddleware) trackABTest(r *http.Request, version *Version, testName, variant, requestID string) {
	analytics := map[string]interface{}{
		"event":      "ab_test_participation",
		"request_id": requestID,
		"version":    version.String(),
		"test_name":  testName,
		"variant":    variant,
		"user_id":    r.Header.Get("X-User-ID"),
		"client_ip":  getClientIP(r),
		"timestamp":  time.Now().UTC(),
	}
	
	fmt.Printf("[AB-TEST] %+v\n", analytics)
}

// VersionMetricsHandler provides version usage metrics
func (vm *VersionMiddleware) VersionMetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		// This would integrate with your metrics system
		metrics := map[string]interface{}{
			"total_requests": 0,
			"version_usage": map[string]int{},
			"deprecated_usage": map[string]int{},
			"error_rates": map[string]float64{},
			"response_times": map[string]float64{},
		}
		
		json.NewEncoder(w).Encode(metrics)
	})
}