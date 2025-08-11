package versioning

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Version represents an API version
type Version struct {
	Major          int                    `json:"major"`
	Minor          int                    `json:"minor"`
	Patch          int                    `json:"patch"`
	Label          string                 `json:"label,omitempty"`          // e.g., "beta", "alpha"
	ReleaseDate    time.Time              `json:"release_date"`
	DeprecatedDate *time.Time             `json:"deprecated_date,omitempty"`
	SunsetDate     *time.Time             `json:"sunset_date,omitempty"`
	Status         VersionStatus          `json:"status"`
	Features       []string               `json:"features"`
	BreakingChanges []BreakingChange      `json:"breaking_changes"`
	Metadata       map[string]interface{} `json:"metadata"`
}

// VersionStatus represents the lifecycle status of a version
type VersionStatus string

const (
	StatusDevelopment VersionStatus = "development"
	StatusPreRelease  VersionStatus = "pre-release"
	StatusStable      VersionStatus = "stable"    
	StatusDeprecated  VersionStatus = "deprecated"
	StatusSunset      VersionStatus = "sunset"
)

// BreakingChange describes a breaking change in an API version
type BreakingChange struct {
	Description    string            `json:"description"`
	AffectedPaths  []string          `json:"affected_paths"`
	MigrationType  MigrationType     `json:"migration_type"`
	MigrationGuide string            `json:"migration_guide"`
	AutoMigration  bool              `json:"auto_migration"`
	Metadata       map[string]string `json:"metadata"`
}

// MigrationType describes the type of migration required
type MigrationType string

const (
	MigrationField      MigrationType = "field_change"
	MigrationEndpoint   MigrationType = "endpoint_change"
	MigrationBehavior   MigrationType = "behavior_change"
	MigrationRemoval    MigrationType = "removal"
	MigrationAddition   MigrationType = "addition"
)

// VersioningStrategy defines how versions are detected from requests
type VersioningStrategy string

const (
	StrategyPath           VersioningStrategy = "path"
	StrategyHeader         VersioningStrategy = "header" 
	StrategyQuery          VersioningStrategy = "query"
	StrategyContentType    VersioningStrategy = "content_type"
	StrategyAcceptHeader   VersioningStrategy = "accept_header"
	StrategyCombined       VersioningStrategy = "combined"
)

// VersionConfig holds configuration for version detection
type VersionConfig struct {
	DefaultVersion     string                    `json:"default_version"`
	SupportedVersions  []string                  `json:"supported_versions"`
	Strategy           VersioningStrategy        `json:"strategy"`
	PathPrefix         string                    `json:"path_prefix"`          // e.g., "/api"
	HeaderName         string                    `json:"header_name"`          // e.g., "API-Version"
	QueryParam         string                    `json:"query_param"`          // e.g., "version"
	MediaTypePrefix    string                    `json:"media_type_prefix"`    // e.g., "application/vnd.pyairtable"
	DeprecationWarning bool                      `json:"deprecation_warning"`
	StrictVersioning   bool                      `json:"strict_versioning"`    // Reject unsupported versions
	VersionRegistry    map[string]*Version       `json:"version_registry"`
}

// VersionDetector extracts version information from HTTP requests
type VersionDetector struct {
	config *VersionConfig
}

// NewVersionDetector creates a new version detector
func NewVersionDetector(config *VersionConfig) *VersionDetector {
	if config.VersionRegistry == nil {
		config.VersionRegistry = make(map[string]*Version)
	}
	return &VersionDetector{config: config}
}

// DetectVersion extracts version from HTTP request using configured strategy
func (vd *VersionDetector) DetectVersion(r *http.Request) (*Version, error) {
	var versionStr string
	
	switch vd.config.Strategy {
	case StrategyPath:
		versionStr = vd.detectFromPath(r.URL.Path)
	case StrategyHeader:
		versionStr = r.Header.Get(vd.config.HeaderName)
	case StrategyQuery:
		versionStr = r.URL.Query().Get(vd.config.QueryParam)
	case StrategyContentType:
		versionStr = vd.detectFromContentType(r.Header.Get("Content-Type"))
	case StrategyAcceptHeader:
		versionStr = vd.detectFromAcceptHeader(r.Header.Get("Accept"))
	case StrategyCombined:
		versionStr = vd.detectCombined(r)
	default:
		versionStr = vd.config.DefaultVersion
	}
	
	if versionStr == "" {
		versionStr = vd.config.DefaultVersion
	}
	
	version, exists := vd.config.VersionRegistry[versionStr]
	if !exists {
		if vd.config.StrictVersioning {
			return nil, fmt.Errorf("unsupported API version: %s", versionStr)
		}
		// Return default version if strict versioning is disabled
		version = vd.config.VersionRegistry[vd.config.DefaultVersion]
	}
	
	return version, nil
}

// detectFromPath extracts version from URL path
func (vd *VersionDetector) detectFromPath(path string) string {
	// Match patterns like /api/v1, /api/v2.1, /api/v1-beta
	pattern := fmt.Sprintf(`%s/v(\d+(?:\.\d+)?(?:-\w+)?)`, regexp.QuoteMeta(vd.config.PathPrefix))
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(path)
	if len(matches) > 1 {
		return "v" + matches[1]
	}
	return ""
}

// detectFromContentType extracts version from content type
func (vd *VersionDetector) detectFromContentType(contentType string) string {
	// Match patterns like application/vnd.pyairtable.v1+json
	pattern := fmt.Sprintf(`%s\.v(\d+(?:\.\d+)?(?:-\w+)?)\+`, regexp.QuoteMeta(vd.config.MediaTypePrefix))
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(contentType)
	if len(matches) > 1 {
		return "v" + matches[1]
	}
	return ""
}

// detectFromAcceptHeader extracts version from Accept header
func (vd *VersionDetector) detectFromAcceptHeader(accept string) string {
	return vd.detectFromContentType(accept)
}

// detectCombined uses multiple strategies in order of preference
func (vd *VersionDetector) detectCombined(r *http.Request) string {
	// Try strategies in order of preference
	strategies := []VersioningStrategy{
		StrategyPath,
		StrategyHeader,
		StrategyAcceptHeader,
		StrategyContentType,
		StrategyQuery,
	}
	
	for _, strategy := range strategies {
		origStrategy := vd.config.Strategy
		vd.config.Strategy = strategy
		version := ""
		
		switch strategy {
		case StrategyPath:
			version = vd.detectFromPath(r.URL.Path)
		case StrategyHeader:
			version = r.Header.Get(vd.config.HeaderName)
		case StrategyQuery:
			version = r.URL.Query().Get(vd.config.QueryParam)
		case StrategyContentType:
			version = vd.detectFromContentType(r.Header.Get("Content-Type"))
		case StrategyAcceptHeader:
			version = vd.detectFromAcceptHeader(r.Header.Get("Accept"))
		}
		
		vd.config.Strategy = origStrategy
		
		if version != "" {
			return version
		}
	}
	
	return ""
}

// AddResponseHeaders adds version-related headers to HTTP response
func (vd *VersionDetector) AddResponseHeaders(w http.ResponseWriter, version *Version) {
	if version == nil {
		return
	}
	
	versionStr := version.String()
	w.Header().Set("API-Version", versionStr)
	w.Header().Set("API-Version-Status", string(version.Status))
	
	// Add deprecation warnings
	if version.Status == StatusDeprecated && version.SunsetDate != nil {
		w.Header().Set("Deprecation", version.DeprecatedDate.Format(http.TimeFormat))
		w.Header().Set("Sunset", version.SunsetDate.Format(http.TimeFormat))
		w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="successor-version"`, vd.getSuccessorVersion(version)))
	}
	
	// Add supported versions
	supportedVersions := strings.Join(vd.config.SupportedVersions, ", ")
	w.Header().Set("API-Supported-Versions", supportedVersions)
}

// String returns string representation of version
func (v *Version) String() string {
	versionStr := fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Label != "" {
		versionStr += "-" + v.Label
	}
	return versionStr
}

// IsCompatibleWith checks if this version is compatible with another
func (v *Version) IsCompatibleWith(other *Version) bool {
	// Same major version indicates compatibility
	return v.Major == other.Major
}

// IsNewerThan checks if this version is newer than another
func (v *Version) IsNewerThan(other *Version) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor > other.Minor
	}
	return v.Patch > other.Patch
}

// IsDeprecated checks if version is deprecated
func (v *Version) IsDeprecated() bool {
	return v.Status == StatusDeprecated
}

// IsSunset checks if version is sunset
func (v *Version) IsSunset() bool {
	return v.Status == StatusSunset
}

// GetFeatures returns features available in this version
func (v *Version) GetFeatures() []string {
	return v.Features
}

// GetBreakingChanges returns breaking changes in this version
func (v *Version) GetBreakingChanges() []BreakingChange {
	return v.BreakingChanges
}

// getSuccessorVersion finds the next stable version
func (vd *VersionDetector) getSuccessorVersion(version *Version) string {
	var candidates []*Version
	
	for _, v := range vd.config.VersionRegistry {
		if v.Status == StatusStable && v.IsNewerThan(version) {
			candidates = append(candidates, v)
		}
	}
	
	if len(candidates) == 0 {
		return ""
	}
	
	// Sort by version and return the earliest successor
	sort.Slice(candidates, func(i, j int) bool {
		return !candidates[i].IsNewerThan(candidates[j])
	})
	
	return candidates[0].String()
}

// VersionContext holds version information in request context
type VersionContext struct {
	Version    *Version
	DetectedBy VersioningStrategy
	RequestID  string
}

// GetVersionFromContext extracts version information from context
func GetVersionFromContext(ctx context.Context) (*VersionContext, bool) {
	vctx, ok := ctx.Value("version").(*VersionContext)
	return vctx, ok
}

// WithVersionContext adds version information to context
func WithVersionContext(ctx context.Context, vctx *VersionContext) context.Context {
	return context.WithValue(ctx, "version", vctx)
}

// ParseVersionString parses version string like "v1.2.3-beta" into Version struct
func ParseVersionString(versionStr string) (*Version, error) {
	if !strings.HasPrefix(versionStr, "v") {
		return nil, fmt.Errorf("version must start with 'v'")
	}
	
	versionStr = strings.TrimPrefix(versionStr, "v")
	
	// Handle labels like "1.2.3-beta"
	parts := strings.Split(versionStr, "-")
	versionPart := parts[0]
	var label string
	if len(parts) > 1 {
		label = parts[1]
	}
	
	// Parse version numbers
	versionNumbers := strings.Split(versionPart, ".")
	if len(versionNumbers) < 1 || len(versionNumbers) > 3 {
		return nil, fmt.Errorf("invalid version format")
	}
	
	major, err := strconv.Atoi(versionNumbers[0])
	if err != nil {
		return nil, fmt.Errorf("invalid major version: %v", err)
	}
	
	var minor, patch int
	if len(versionNumbers) > 1 {
		minor, err = strconv.Atoi(versionNumbers[1])
		if err != nil {
			return nil, fmt.Errorf("invalid minor version: %v", err)
		}
	}
	
	if len(versionNumbers) > 2 {
		patch, err = strconv.Atoi(versionNumbers[2])
		if err != nil {
			return nil, fmt.Errorf("invalid patch version: %v", err)
		}
	}
	
	return &Version{
		Major: major,
		Minor: minor,
		Patch: patch,
		Label: label,
	}, nil
}

// VersionRegistry manages all available API versions
type VersionRegistry struct {
	versions     map[string]*Version
	defaultVersion string
}

// NewVersionRegistry creates a new version registry
func NewVersionRegistry(defaultVersion string) *VersionRegistry {
	return &VersionRegistry{
		versions:     make(map[string]*Version),
		defaultVersion: defaultVersion,
	}
}

// RegisterVersion adds a version to the registry
func (vr *VersionRegistry) RegisterVersion(version *Version) {
	vr.versions[version.String()] = version
}

// GetVersion retrieves a version from the registry
func (vr *VersionRegistry) GetVersion(versionStr string) (*Version, bool) {
	version, exists := vr.versions[versionStr]
	return version, exists
}

// GetAllVersions returns all registered versions
func (vr *VersionRegistry) GetAllVersions() map[string]*Version {
	return vr.versions
}

// GetSupportedVersions returns only supported (non-sunset) versions
func (vr *VersionRegistry) GetSupportedVersions() map[string]*Version {
	supported := make(map[string]*Version)
	for k, v := range vr.versions {
		if v.Status != StatusSunset {
			supported[k] = v
		}
	}
	return supported
}

// GetDefaultVersion returns the default version
func (vr *VersionRegistry) GetDefaultVersion() (*Version, bool) {
	return vr.GetVersion(vr.defaultVersion)
}

// VersionInfo provides information about API versions for discovery
type VersionInfo struct {
	CurrentVersion    string              `json:"current_version"`
	DefaultVersion    string              `json:"default_version"`
	SupportedVersions []VersionSummary    `json:"supported_versions"`
	DeprecatedVersions []VersionSummary   `json:"deprecated_versions"`
	VersioningStrategy VersioningStrategy  `json:"versioning_strategy"`
	DiscoveryEndpoint string              `json:"discovery_endpoint"`
}

// VersionSummary provides basic version information
type VersionSummary struct {
	Version        string     `json:"version"`
	Status         VersionStatus `json:"status"`
	ReleaseDate    time.Time  `json:"release_date"`
	DeprecatedDate *time.Time `json:"deprecated_date,omitempty"`
	SunsetDate     *time.Time `json:"sunset_date,omitempty"`
	Features       []string   `json:"features"`
}

// BuildVersionInfo creates version information for API discovery
func (vr *VersionRegistry) BuildVersionInfo(currentVersion string, strategy VersioningStrategy) *VersionInfo {
	info := &VersionInfo{
		CurrentVersion:    currentVersion,
		DefaultVersion:    vr.defaultVersion,
		VersioningStrategy: strategy,
		DiscoveryEndpoint: "/api/version-info",
	}
	
	for _, version := range vr.versions {
		summary := VersionSummary{
			Version:        version.String(),
			Status:         version.Status,
			ReleaseDate:    version.ReleaseDate,
			DeprecatedDate: version.DeprecatedDate,
			SunsetDate:     version.SunsetDate,
			Features:       version.Features,
		}
		
		if version.Status == StatusDeprecated {
			info.DeprecatedVersions = append(info.DeprecatedVersions, summary)
		} else if version.Status != StatusSunset {
			info.SupportedVersions = append(info.SupportedVersions, summary)
		}
	}
	
	return info
}