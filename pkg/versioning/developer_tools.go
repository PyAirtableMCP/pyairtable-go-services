package versioning

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// DeveloperTools provides tools for API version management
type DeveloperTools struct {
	versionManager    *VersionManager
	migrationService  *MigrationService
	compatibilityMatrix *CompatibilityMatrix
	documentationGen  *DocumentationGenerator
	sdkManager        *SDKManager
	changelogGen      *ChangelogGenerator
	mu                sync.RWMutex
}

// CompatibilityMatrix tracks compatibility between versions
type CompatibilityMatrix struct {
	Matrix     map[string]map[string]*CompatibilityInfo `json:"matrix"`     // from -> to -> info
	LastUpdated time.Time                              `json:"last_updated"`
	mu         sync.RWMutex
}

// CompatibilityInfo provides detailed compatibility information
type CompatibilityInfo struct {
	Compatible      bool               `json:"compatible"`
	BreakingChanges []BreakingChange   `json:"breaking_changes"`
	Warnings        []CompatibilityWarning `json:"warnings"`
	MigrationPath   []string           `json:"migration_path"`   // Intermediate versions for migration
	Effort          MigrationEffort    `json:"effort"`
	AutoMigration   bool               `json:"auto_migration"`
	Tools           []string           `json:"tools"`            // Available migration tools
	Documentation   string             `json:"documentation"`    // Link to migration docs
	EstimatedTime   time.Duration      `json:"estimated_time"`
	TestingRequired bool               `json:"testing_required"`
	DataMigration   bool               `json:"data_migration"`
	ConfigChanges   bool               `json:"config_changes"`
	LastChecked     time.Time          `json:"last_checked"`
}

// CompatibilityWarning represents a compatibility warning
type CompatibilityWarning struct {
	Type        WarningType `json:"type"`
	Message     string      `json:"message"`
	Severity    string      `json:"severity"`    // low, medium, high
	Component   string      `json:"component"`   // API endpoint, field, etc.
	Workaround  string      `json:"workaround"`
	MoreInfo    string      `json:"more_info"`   // Link to documentation
}

// WarningType defines types of compatibility warnings
type WarningType string

const (
	WarningDeprecation    WarningType = "deprecation"
	WarningBehaviorChange WarningType = "behavior_change"
	WarningPerformance    WarningType = "performance"
	WarningDataFormat     WarningType = "data_format"
	WarningAuthentication WarningType = "authentication"
	WarningRateLimit      WarningType = "rate_limit"
)

// MigrationEffort indicates the effort required for migration
type MigrationEffort string

const (
	EffortMinimal  MigrationEffort = "minimal"   // < 1 hour
	EffortLow      MigrationEffort = "low"       // 1-4 hours
	EffortMedium   MigrationEffort = "medium"    // 1-2 days
	EffortHigh     MigrationEffort = "high"      // 3-5 days
	EffortCritical MigrationEffort = "critical"  // > 1 week
)

// DocumentationGenerator generates version-specific documentation
type DocumentationGenerator struct {
	templates    map[string]*template.Template
	outputPath   string
	versions     map[string]*DocumentationSet
	mu           sync.RWMutex
}

// DocumentationSet contains all documentation for a version
type DocumentationSet struct {
	Version         string                    `json:"version"`
	APIReference    *APIReferenceDoc          `json:"api_reference"`
	GettingStarted  *GettingStartedDoc        `json:"getting_started"`
	MigrationGuides map[string]*MigrationGuide `json:"migration_guides"` // from_version -> guide
	Examples        *ExamplesDoc              `json:"examples"`
	Changelog       *ChangelogDoc             `json:"changelog"`
	SDKDocs         map[string]*SDKDoc        `json:"sdk_docs"`        // language -> docs
	GeneratedAt     time.Time                 `json:"generated_at"`
}

// APIReferenceDoc contains API reference documentation
type APIReferenceDoc struct {
	Introduction string                    `json:"introduction"`
	BaseURL      string                    `json:"base_url"`
	Authentication *AuthDoc               `json:"authentication"`
	Endpoints    map[string]*EndpointDoc   `json:"endpoints"`
	Models       map[string]*ModelDoc      `json:"models"`
	ErrorCodes   map[string]*ErrorDoc      `json:"error_codes"`
	RateLimit    *RateLimitDoc             `json:"rate_limit"`
	Examples     []APIExample              `json:"examples"`
}

// AuthDoc documents authentication
type AuthDoc struct {
	Type        string            `json:"type"`
	Description string            `json:"description"`
	Headers     map[string]string `json:"headers"`
	Examples    []AuthExample     `json:"examples"`
}

// EndpointDoc documents an API endpoint
type EndpointDoc struct {
	Path        string                   `json:"path"`
	Method      string                   `json:"method"`
	Summary     string                   `json:"summary"`
	Description string                   `json:"description"`
	Parameters  []ParameterDoc           `json:"parameters"`
	RequestBody *RequestBodyDoc          `json:"request_body"`
	Responses   map[string]*ResponseDoc  `json:"responses"`
	Examples    []EndpointExample        `json:"examples"`
	Notes       []string                 `json:"notes"`
	Deprecated  bool                     `json:"deprecated"`
	Since       string                   `json:"since"`        // Version introduced
	Changes     []EndpointChange         `json:"changes"`      // Version-specific changes
}

// ParameterDoc documents a parameter
type ParameterDoc struct {
	Name        string      `json:"name"`
	In          string      `json:"in"`          // query, header, path
	Type        string      `json:"type"`
	Required    bool        `json:"required"`
	Description string      `json:"description"`
	Default     interface{} `json:"default"`
	Example     interface{} `json:"example"`
	Enum        []string    `json:"enum"`
	Format      string      `json:"format"`
	Pattern     string      `json:"pattern"`
}

// RequestBodyDoc documents request body
type RequestBodyDoc struct {
	Description string                    `json:"description"`
	Required    bool                      `json:"required"`
	Content     map[string]*MediaTypeDoc  `json:"content"`
}

// ResponseDoc documents a response
type ResponseDoc struct {
	Description string                   `json:"description"`
	Headers     map[string]*HeaderDoc    `json:"headers"`
	Content     map[string]*MediaTypeDoc `json:"content"`
}

// MediaTypeDoc documents media type content
type MediaTypeDoc struct {
	Schema   *SchemaDoc `json:"schema"`
	Example  interface{} `json:"example"`
	Examples map[string]*ExampleDoc `json:"examples"`
}

// SchemaDoc documents a schema
type SchemaDoc struct {
	Type        string                  `json:"type"`
	Properties  map[string]*PropertyDoc `json:"properties"`
	Required    []string                `json:"required"`
	Description string                  `json:"description"`
	Example     interface{}             `json:"example"`
}

// PropertyDoc documents a property
type PropertyDoc struct {
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Example     interface{} `json:"example"`
	Enum        []string    `json:"enum"`
	Format      string      `json:"format"`
	Pattern     string      `json:"pattern"`
	Minimum     *float64    `json:"minimum"`
	Maximum     *float64    `json:"maximum"`
	Deprecated  bool        `json:"deprecated"`
}

// HeaderDoc documents a header
type HeaderDoc struct {
	Description string      `json:"description"`
	Type        string      `json:"type"`
	Example     interface{} `json:"example"`
}

// ExampleDoc documents an example
type ExampleDoc struct {
	Summary     string      `json:"summary"`
	Description string      `json:"description"`
	Value       interface{} `json:"value"`
}

// ModelDoc documents a data model
type ModelDoc struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Properties  map[string]*PropertyDoc `json:"properties"`
	Required    []string                `json:"required"`
	Example     interface{}             `json:"example"`
	Since       string                  `json:"since"`
	Changes     []ModelChange           `json:"changes"`
}

// ErrorDoc documents an error code
type ErrorDoc struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Description string `json:"description"`
	Resolution  string `json:"resolution"`
	Example     interface{} `json:"example"`
}

// RateLimitDoc documents rate limiting
type RateLimitDoc struct {
	Description string              `json:"description"`
	Limits      map[string]*Limit   `json:"limits"`
	Headers     []string            `json:"headers"`
	Handling    string              `json:"handling"`
}

// Limit documents a rate limit
type Limit struct {
	Requests int           `json:"requests"`
	Window   time.Duration `json:"window"`
	Scope    string        `json:"scope"`
}

// APIExample provides API usage examples
type APIExample struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Request     *ExampleRequest   `json:"request"`
	Response    *ExampleResponse  `json:"response"`
	Language    string            `json:"language"`
	Code        string            `json:"code"`
}

// ExampleRequest represents an example request
type ExampleRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    interface{}       `json:"body"`
}

// ExampleResponse represents an example response
type ExampleResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    interface{}       `json:"body"`
}

// AuthExample provides authentication examples
type AuthExample struct {
	Type        string            `json:"type"`
	Description string            `json:"description"`
	Headers     map[string]string `json:"headers"`
	Code        string            `json:"code"`
}

// EndpointExample provides endpoint-specific examples
type EndpointExample struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Request     *ExampleRequest  `json:"request"`
	Response    *ExampleResponse `json:"response"`
	Code        map[string]string `json:"code"` // language -> code
}

// EndpointChange documents changes to an endpoint
type EndpointChange struct {
	Version     string `json:"version"`
	Type        string `json:"type"`        // added, modified, deprecated, removed
	Description string `json:"description"`
	Migration   string `json:"migration"`
}

// ModelChange documents changes to a model
type ModelChange struct {
	Version     string `json:"version"`
	Type        string `json:"type"`
	Field       string `json:"field"`
	Description string `json:"description"`
	Migration   string `json:"migration"`
}

// GettingStartedDoc contains getting started documentation
type GettingStartedDoc struct {
	Introduction    string                 `json:"introduction"`
	Prerequisites   []string               `json:"prerequisites"`
	Installation    *InstallationDoc       `json:"installation"`
	QuickStart      *QuickStartDoc         `json:"quick_start"`
	Authentication  *AuthenticationDoc     `json:"authentication"`
	FirstSteps      []Step                 `json:"first_steps"`
	CommonPatterns  []Pattern              `json:"common_patterns"`
	Troubleshooting *TroubleshootingDoc    `json:"troubleshooting"`
	NextSteps       []string               `json:"next_steps"`
}

// InstallationDoc documents installation
type InstallationDoc struct {
	Methods     map[string]*InstallMethod `json:"methods"` // language/platform -> method
	Requirements []string                 `json:"requirements"`
}

// InstallMethod documents an installation method
type InstallMethod struct {
	Platform    string   `json:"platform"`
	Steps       []string `json:"steps"`
	Command     string   `json:"command"`
	Example     string   `json:"example"`
	Notes       []string `json:"notes"`
}

// QuickStartDoc provides quick start guide
type QuickStartDoc struct {
	Overview string              `json:"overview"`
	Steps    []QuickStartStep    `json:"steps"`
	Examples map[string]string   `json:"examples"` // language -> example code
}

// QuickStartStep represents a quick start step
type QuickStartStep struct {
	Order       int               `json:"order"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Code        map[string]string `json:"code"`     // language -> code
	Notes       []string          `json:"notes"`
}

// AuthenticationDoc documents authentication setup
type AuthenticationDoc struct {
	Overview string                      `json:"overview"`
	Methods  map[string]*AuthMethodDoc   `json:"methods"`
	Examples map[string]*AuthExampleDoc  `json:"examples"`
}

// AuthMethodDoc documents an authentication method
type AuthMethodDoc struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Setup       []string `json:"setup"`
	Usage       string   `json:"usage"`
	Security    []string `json:"security"`
}

// AuthExampleDoc provides authentication examples
type AuthExampleDoc struct {
	Language string `json:"language"`
	Code     string `json:"code"`
	Notes    []string `json:"notes"`
}

// Step represents a documentation step
type Step struct {
	Order       int               `json:"order"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Code        map[string]string `json:"code"`
	Links       []Link            `json:"links"`
}

// Pattern documents a common usage pattern
type Pattern struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	UseCase     string            `json:"use_case"`
	Code        map[string]string `json:"code"`
	Notes       []string          `json:"notes"`
}

// TroubleshootingDoc provides troubleshooting information
type TroubleshootingDoc struct {
	Common     []TroubleshootingItem `json:"common"`
	ByError    map[string]*ErrorSolution `json:"by_error"`
	FAQ        []FAQItem             `json:"faq"`
	Support    *SupportInfo          `json:"support"`
}

// TroubleshootingItem represents a troubleshooting item
type TroubleshootingItem struct {
	Problem    string   `json:"problem"`
	Cause      string   `json:"cause"`
	Solution   string   `json:"solution"`
	Code       string   `json:"code"`
	References []Link   `json:"references"`
}

// ErrorSolution provides solution for specific errors
type ErrorSolution struct {
	Error       string   `json:"error"`
	Description string   `json:"description"`
	Causes      []string `json:"causes"`
	Solutions   []string `json:"solutions"`
	Prevention  []string `json:"prevention"`
}

// FAQItem represents a frequently asked question
type FAQItem struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Tags     []string `json:"tags"`
	Related  []Link `json:"related"`
}

// SupportInfo provides support information
type SupportInfo struct {
	Channels []SupportChannel `json:"channels"`
	Response string           `json:"response"`
	Hours    string           `json:"hours"`
}

// SupportChannel represents a support channel
type SupportChannel struct {
	Type        string `json:"type"`
	Contact     string `json:"contact"`
	Description string `json:"description"`
}

// Link represents a documentation link
type Link struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Type  string `json:"type"`
}

// ExamplesDoc contains code examples
type ExamplesDoc struct {
	Introduction string                     `json:"introduction"`
	Languages    map[string]*LanguageExamples `json:"languages"`
	UseCases     map[string]*UseCaseExamples  `json:"use_cases"`
	Recipes      []CodeRecipe               `json:"recipes"`
}

// LanguageExamples contains examples for a specific language
type LanguageExamples struct {
	Language     string        `json:"language"`
	Setup        string        `json:"setup"`
	BasicUsage   []CodeExample `json:"basic_usage"`
	Advanced     []CodeExample `json:"advanced"`
	Integration  []CodeExample `json:"integration"`
	ErrorHandling []CodeExample `json:"error_handling"`
}

// UseCaseExamples contains examples for specific use cases
type UseCaseExamples struct {
	UseCase     string        `json:"use_case"`
	Description string        `json:"description"`
	Examples    []CodeExample `json:"examples"`
	Notes       []string      `json:"notes"`
}

// CodeRecipe provides a complete code recipe
type CodeRecipe struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Category    string            `json:"category"`
	Difficulty  string            `json:"difficulty"`
	Code        map[string]string `json:"code"`     // language -> code
	Explanation string            `json:"explanation"`
	Tips        []string          `json:"tips"`
	SeeAlso     []Link            `json:"see_also"`
}

// ChangelogDoc contains changelog information
type ChangelogDoc struct {
	Version     string          `json:"version"`
	ReleaseDate time.Time       `json:"release_date"`
	Summary     string          `json:"summary"`
	Added       []ChangeItem    `json:"added"`
	Changed     []ChangeItem    `json:"changed"`
	Deprecated  []ChangeItem    `json:"deprecated"`
	Removed     []ChangeItem    `json:"removed"`
	Fixed       []ChangeItem    `json:"fixed"`
	Security    []ChangeItem    `json:"security"`
	Migration   *MigrationInfo  `json:"migration"`
}

// ChangeItem represents a changelog item
type ChangeItem struct {
	Description string   `json:"description"`
	Component   string   `json:"component"`
	Breaking    bool     `json:"breaking"`
	References  []string `json:"references"` // Issue/PR numbers
	Migration   string   `json:"migration"`  // Migration instructions
}

// MigrationInfo provides migration information for a version
type MigrationInfo struct {
	Required    bool          `json:"required"`
	Effort      MigrationEffort `json:"effort"`
	Steps       []string      `json:"steps"`
	Tools       []string      `json:"tools"`
	TimeEstimate time.Duration `json:"time_estimate"`
	Guide       string        `json:"guide"`        // Link to migration guide
}

// SDKDoc contains SDK-specific documentation
type SDKDoc struct {
	Language    string            `json:"language"`
	Version     string            `json:"version"`
	Installation *InstallationDoc `json:"installation"`
	QuickStart  *QuickStartDoc    `json:"quick_start"`
	Reference   *SDKReference     `json:"reference"`
	Examples    *ExamplesDoc      `json:"examples"`
	Changelog   []ChangelogDoc    `json:"changelog"`
}

// SDKReference contains SDK reference documentation
type SDKReference struct {
	Classes   map[string]*ClassDoc    `json:"classes"`
	Functions map[string]*FunctionDoc `json:"functions"`
	Constants map[string]*ConstantDoc `json:"constants"`
	Types     map[string]*TypeDoc     `json:"types"`
}

// ClassDoc documents a class
type ClassDoc struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Constructor *FunctionDoc            `json:"constructor"`
	Methods     map[string]*FunctionDoc `json:"methods"`
	Properties  map[string]*PropertyDoc `json:"properties"`
	Examples    []CodeExample           `json:"examples"`
	Since       string                  `json:"since"`
}

// FunctionDoc documents a function
type FunctionDoc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  []ParameterDoc `json:"parameters"`
	Returns     *ReturnDoc     `json:"returns"`
	Throws      []ExceptionDoc `json:"throws"`
	Examples    []CodeExample  `json:"examples"`
	Since       string         `json:"since"`
	Deprecated  bool           `json:"deprecated"`
	Alternative string         `json:"alternative"`
}

// ReturnDoc documents a return value
type ReturnDoc struct {
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Example     interface{} `json:"example"`
}

// ExceptionDoc documents an exception
type ExceptionDoc struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	When        string `json:"when"`
}

// ConstantDoc documents a constant
type ConstantDoc struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Value       interface{} `json:"value"`
	Description string      `json:"description"`
	Since       string      `json:"since"`
}

// TypeDoc documents a type
type TypeDoc struct {
	Name        string                  `json:"name"`
	Kind        string                  `json:"kind"`        // class, interface, enum, etc.
	Description string                  `json:"description"`
	Properties  map[string]*PropertyDoc `json:"properties"`
	Methods     map[string]*FunctionDoc `json:"methods"`
	Since       string                  `json:"since"`
}

// SDKManager manages SDK generation and versioning
type SDKManager struct {
	generators map[string]SDKGenerator  // language -> generator
	versions   map[string]*SDKVersions  // language -> versions
	mu         sync.RWMutex
}

// SDKGenerator generates SDKs for different languages
type SDKGenerator interface {
	GetLanguage() string
	GenerateSDK(version string, schema interface{}) (*GeneratedSDK, error)
	GetTemplate() *SDKTemplate
	ValidateConfig(config *SDKConfig) error
}

// GeneratedSDK represents a generated SDK
type GeneratedSDK struct {
	Language    string                 `json:"language"`
	Version     string                 `json:"version"`
	Files       map[string]string      `json:"files"`       // filename -> content
	Package     *PackageInfo           `json:"package"`
	Docs        *SDKDoc                `json:"docs"`
	Tests       map[string]string      `json:"tests"`       // test file -> content
	Examples    map[string]string      `json:"examples"`    // example file -> content
	GeneratedAt time.Time              `json:"generated_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// PackageInfo contains package information
type PackageInfo struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Description  string            `json:"description"`
	Author       string            `json:"author"`
	License      string            `json:"license"`
	Repository   string            `json:"repository"`
	Homepage     string            `json:"homepage"`
	Keywords     []string          `json:"keywords"`
	Dependencies map[string]string `json:"dependencies"`
}

// SDKVersions tracks SDK versions for a language
type SDKVersions struct {
	Language string              `json:"language"`
	Versions map[string]*GeneratedSDK `json:"versions"`
	Latest   string              `json:"latest"`
	Stable   string              `json:"stable"`
}

// SDKTemplate defines the structure for SDK generation
type SDKTemplate struct {
	Language     string                  `json:"language"`
	Files        map[string]*FileTemplate `json:"files"`        // filename -> template
	Structure    []string                `json:"structure"`    // directory structure
	Dependencies []string                `json:"dependencies"`
	BuildConfig  *BuildConfig            `json:"build_config"`
}

// FileTemplate represents a file template
type FileTemplate struct {
	Name        string `json:"name"`
	Template    string `json:"template"`
	Required    bool   `json:"required"`
	Conditional string `json:"conditional"` // Condition for including file
}

// BuildConfig contains build configuration
type BuildConfig struct {
	Command     string            `json:"command"`
	Environment map[string]string `json:"environment"`
	Steps       []BuildStep       `json:"steps"`
}

// BuildStep represents a build step
type BuildStep struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Args    []string `json:"args"`
}

// SDKConfig configures SDK generation
type SDKConfig struct {
	Language     string                 `json:"language"`
	Version      string                 `json:"version"`
	PackageName  string                 `json:"package_name"`
	Namespace    string                 `json:"namespace"`
	Features     []string               `json:"features"`
	Options      map[string]interface{} `json:"options"`
	Templates    map[string]string      `json:"templates"`
}

// ChangelogGenerator generates changelogs
type ChangelogGenerator struct {
	versions  map[string]*ChangelogDoc
	templates map[string]*template.Template
	mu        sync.RWMutex
}

// NewDeveloperTools creates a new developer tools instance
func NewDeveloperTools(versionManager *VersionManager) *DeveloperTools {
	return &DeveloperTools{
		versionManager:   versionManager,
		migrationService: NewMigrationService(),
		compatibilityMatrix: &CompatibilityMatrix{
			Matrix: make(map[string]map[string]*CompatibilityInfo),
		},
		documentationGen: &DocumentationGenerator{
			templates: make(map[string]*template.Template),
			versions:  make(map[string]*DocumentationSet),
		},
		sdkManager: &SDKManager{
			generators: make(map[string]SDKGenerator),
			versions:   make(map[string]*SDKVersions),
		},
		changelogGen: &ChangelogGenerator{
			versions:  make(map[string]*ChangelogDoc),
			templates: make(map[string]*template.Template),
		},
	}
}

// BuildCompatibilityMatrix builds the version compatibility matrix
func (dt *DeveloperTools) BuildCompatibilityMatrix() {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	
	versions := dt.versionManager.registry.GetAllVersions()
	
	// Initialize matrix
	for fromVersion := range versions {
		if dt.compatibilityMatrix.Matrix[fromVersion] == nil {
			dt.compatibilityMatrix.Matrix[fromVersion] = make(map[string]*CompatibilityInfo)
		}
		
		for toVersion := range versions {
			info := dt.analyzeCompatibility(versions[fromVersion], versions[toVersion])
			dt.compatibilityMatrix.Matrix[fromVersion][toVersion] = info
		}
	}
	
	dt.compatibilityMatrix.LastUpdated = time.Now()
}

// analyzeCompatibility analyzes compatibility between two versions
func (dt *DeveloperTools) analyzeCompatibility(from, to *Version) *CompatibilityInfo {
	info := &CompatibilityInfo{
		Compatible:      from.IsCompatibleWith(to),
		BreakingChanges: make([]BreakingChange, 0),
		Warnings:        make([]CompatibilityWarning, 0),
		MigrationPath:   dt.findMigrationPath(from, to),
		AutoMigration:   true,
		Tools:           []string{"auto-migration-tool"},
		Documentation:   fmt.Sprintf("/docs/migration/%s-to-%s", from.String(), to.String()),
		EstimatedTime:   dt.estimateMigrationTime(from, to),
		TestingRequired: !from.IsCompatibleWith(to),
		DataMigration:   dt.requiresDataMigration(from, to),
		ConfigChanges:   dt.requiresConfigChanges(from, to),
		LastChecked:     time.Now(),
	}
	
	// Collect breaking changes
	if to.IsNewerThan(from) {
		for _, change := range to.BreakingChanges {
			info.BreakingChanges = append(info.BreakingChanges, change)
		}
	}
	
	// Add warnings based on version status
	if to.IsDeprecated() {
		info.Warnings = append(info.Warnings, CompatibilityWarning{
			Type:     WarningDeprecation,
			Message:  fmt.Sprintf("Target version %s is deprecated", to.String()),
			Severity: "high",
		})
	}
	
	// Determine effort level
	info.Effort = dt.calculateMigrationEffort(from, to, info)
	
	return info
}

// findMigrationPath finds the optimal migration path between versions
func (dt *DeveloperTools) findMigrationPath(from, to *Version) []string {
	if from.IsCompatibleWith(to) {
		return []string{from.String(), to.String()}
	}
	
	// For now, return direct path
	// In a real implementation, this would find the optimal path through intermediate versions
	return []string{from.String(), to.String()}
}

// estimateMigrationTime estimates the time required for migration
func (dt *DeveloperTools) estimateMigrationTime(from, to *Version) time.Duration {
	if from.IsCompatibleWith(to) {
		return 30 * time.Minute // Minimal effort for compatible versions
	}
	
	changeCount := len(to.BreakingChanges)
	
	switch {
	case changeCount == 0:
		return 1 * time.Hour
	case changeCount <= 3:
		return 4 * time.Hour
	case changeCount <= 10:
		return 2 * 24 * time.Hour // 2 days
	default:
		return 5 * 24 * time.Hour // 5 days
	}
}

// requiresDataMigration checks if data migration is required
func (dt *DeveloperTools) requiresDataMigration(from, to *Version) bool {
	// Check if any breaking changes affect data structures
	for _, change := range to.BreakingChanges {
		if change.MigrationType == MigrationField {
			return true
		}
	}
	return false
}

// requiresConfigChanges checks if configuration changes are required
func (dt *DeveloperTools) requiresConfigChanges(from, to *Version) bool {
	// Check if any breaking changes affect configuration
	for _, change := range to.BreakingChanges {
		if change.MigrationType == MigrationBehavior {
			return true
		}
	}
	return false
}

// calculateMigrationEffort calculates the migration effort
func (dt *DeveloperTools) calculateMigrationEffort(from, to *Version, info *CompatibilityInfo) MigrationEffort {
	if from.IsCompatibleWith(to) {
		return EffortMinimal
	}
	
	score := 0
	
	// Add points for breaking changes
	score += len(info.BreakingChanges) * 10
	
	// Add points for data migration
	if info.DataMigration {
		score += 20
	}
	
	// Add points for config changes
	if info.ConfigChanges {
		score += 15
	}
	
	// Add points for major version difference
	if to.Major > from.Major {
		score += 30
	}
	
	switch {
	case score <= 10:
		return EffortMinimal
	case score <= 30:
		return EffortLow
	case score <= 60:
		return EffortMedium
	case score <= 100:
		return EffortHigh
	default:
		return EffortCritical
	}
}

// GetCompatibilityMatrix returns the compatibility matrix
func (dt *DeveloperTools) GetCompatibilityMatrix() *CompatibilityMatrix {
	dt.compatibilityMatrix.mu.RLock()
	defer dt.compatibilityMatrix.mu.RUnlock()
	
	return dt.compatibilityMatrix
}

// GenerateDocumentation generates documentation for a version
func (dt *DeveloperTools) GenerateDocumentation(version string) (*DocumentationSet, error) {
	dt.documentationGen.mu.Lock()
	defer dt.documentationGen.mu.Unlock()
	
	versionObj, exists := dt.versionManager.registry.GetVersion(version)
	if !exists {
		return nil, fmt.Errorf("version %s not found", version)
	}
	
	docSet := &DocumentationSet{
		Version:         version,
		APIReference:    dt.generateAPIReference(versionObj),
		GettingStarted:  dt.generateGettingStarted(versionObj),
		MigrationGuides: dt.generateMigrationGuides(version),
		Examples:        dt.generateExamples(versionObj),
		Changelog:       dt.generateChangelog(versionObj),
		SDKDocs:         dt.generateSDKDocs(version),
		GeneratedAt:     time.Now(),
	}
	
	dt.documentationGen.versions[version] = docSet
	return docSet, nil
}

// generateAPIReference generates API reference documentation
func (dt *DeveloperTools) generateAPIReference(version *Version) *APIReferenceDoc {
	return &APIReferenceDoc{
		Introduction:   fmt.Sprintf("API Reference for version %s", version.String()),
		BaseURL:        "https://api.pyairtable.com",
		Authentication: &AuthDoc{
			Type:        "Bearer Token",
			Description: "Use OAuth2 bearer tokens for authentication",
			Headers:     map[string]string{"Authorization": "Bearer {token}"},
		},
		Endpoints:    make(map[string]*EndpointDoc),
		Models:       make(map[string]*ModelDoc),
		ErrorCodes:   make(map[string]*ErrorDoc),
		RateLimit:    &RateLimitDoc{
			Description: "Rate limits apply per user",
			Limits: map[string]*Limit{
				"default": {Requests: 1000, Window: time.Hour, Scope: "user"},
			},
		},
		Examples:     make([]APIExample, 0),
	}
}

// generateGettingStarted generates getting started documentation
func (dt *DeveloperTools) generateGettingStarted(version *Version) *GettingStartedDoc {
	return &GettingStartedDoc{
		Introduction:  fmt.Sprintf("Getting started with PyAirtable API %s", version.String()),
		Prerequisites: []string{"API key", "Basic HTTP knowledge"},
		Installation:  &InstallationDoc{
			Methods: map[string]*InstallMethod{
				"curl": {
					Platform: "curl",
					Command:  "curl",
					Example:  "curl -H 'Authorization: Bearer token' https://api.pyairtable.com/api/" + version.String(),
				},
			},
		},
		QuickStart: &QuickStartDoc{
			Overview: "Quick start guide for the API",
			Steps: []QuickStartStep{
				{
					Order:       1,
					Title:       "Get API Key",
					Description: "Obtain your API key from the dashboard",
				},
				{
					Order:       2,
					Title:       "Make First Request",
					Description: "Make your first API request",
					Code: map[string]string{
						"curl": fmt.Sprintf("curl -H 'Authorization: Bearer token' https://api.pyairtable.com/api/%s/users", version.String()),
					},
				},
			},
		},
	}
}

// generateMigrationGuides generates migration guides
func (dt *DeveloperTools) generateMigrationGuides(version string) map[string]*MigrationGuide {
	guides := make(map[string]*MigrationGuide)
	
	// Generate migration guides from all other versions to this version
	for fromVersion := range dt.versionManager.registry.GetAllVersions() {
		if fromVersion != version {
			guide := &MigrationGuide{
				FromVersion:   fromVersion,
				ToVersion:     version,
				Title:         fmt.Sprintf("Migration Guide: %s to %s", fromVersion, version),
				Description:   fmt.Sprintf("Guide for migrating from %s to %s", fromVersion, version),
				Complexity:    ComplexityMedium,
				EstimatedTime: 2 * time.Hour,
				Steps:         dt.generateMigrationSteps(fromVersion, version),
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
			guides[fromVersion] = guide
		}
	}
	
	return guides
}

// generateExamples generates code examples
func (dt *DeveloperTools) generateExamples(version *Version) *ExamplesDoc {
	return &ExamplesDoc{
		Introduction: fmt.Sprintf("Code examples for API version %s", version.String()),
		Languages: map[string]*LanguageExamples{
			"javascript": {
				Language: "javascript",
				Setup:    "npm install axios",
				BasicUsage: []CodeExample{
					{
						Title:       "Get Users",
						Description: "Retrieve a list of users",
						Before:      "",
						After: fmt.Sprintf(`const response = await fetch('https://api.pyairtable.com/api/%s/users', {
  headers: { 'Authorization': 'Bearer ' + token }
});`, version.String()),
						Explanation: "Basic user retrieval",
					},
				},
			},
		},
		UseCases: make(map[string]*UseCaseExamples),
		Recipes:  make([]CodeRecipe, 0),
	}
}

// generateChangelog generates changelog
func (dt *DeveloperTools) generateChangelog(version *Version) *ChangelogDoc {
	changelog := &ChangelogDoc{
		Version:     version.String(),
		ReleaseDate: version.ReleaseDate,
		Summary:     fmt.Sprintf("Release of API version %s", version.String()),
		Added:       make([]ChangeItem, 0),
		Changed:     make([]ChangeItem, 0),
		Deprecated:  make([]ChangeItem, 0),
		Removed:     make([]ChangeItem, 0),
		Fixed:       make([]ChangeItem, 0),
		Security:    make([]ChangeItem, 0),
	}
	
	// Add breaking changes to changelog
	for _, change := range version.BreakingChanges {
		item := ChangeItem{
			Description: change.Description,
			Breaking:    true,
			Migration:   change.MigrationGuide,
		}
		changelog.Changed = append(changelog.Changed, item)
	}
	
	return changelog
}

// generateSDKDocs generates SDK documentation
func (dt *DeveloperTools) generateSDKDocs(version string) map[string]*SDKDoc {
	docs := make(map[string]*SDKDoc)
	
	for language := range dt.sdkManager.generators {
		docs[language] = &SDKDoc{
			Language: language,
			Version:  version,
			Installation: &InstallationDoc{
				Methods: map[string]*InstallMethod{
					language: {
						Platform: language,
						Command:  fmt.Sprintf("install pyairtable-%s", language),
					},
				},
			},
		}
	}
	
	return docs
}

// CompatibilityMatrixHandler provides HTTP endpoint for compatibility matrix
func (dt *DeveloperTools) CompatibilityMatrixHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		matrix := dt.GetCompatibilityMatrix()
		json.NewEncoder(w).Encode(matrix)
	})
}

// DocumentationHandler provides HTTP endpoint for documentation
func (dt *DeveloperTools) DocumentationHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		version := r.URL.Query().Get("version")
		if version == "" {
			// Return available documentation versions
			dt.documentationGen.mu.RLock()
			versions := make([]string, 0, len(dt.documentationGen.versions))
			for v := range dt.documentationGen.versions {
				versions = append(versions, v)
			}
			dt.documentationGen.mu.RUnlock()
			
			sort.Strings(versions)
			
			response := map[string]interface{}{
				"available_versions": versions,
				"documentation_url":  "/api/developer-tools/documentation?version={version}",
			}
			
			json.NewEncoder(w).Encode(response)
			return
		}
		
		docSet, err := dt.GenerateDocumentation(version)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		
		json.NewEncoder(w).Encode(docSet)
	})
}

// MigrationToolsHandler provides HTTP endpoint for migration tools
func (dt *DeveloperTools) MigrationToolsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		fromVersion := r.URL.Query().Get("from")
		toVersion := r.URL.Query().Get("to")
		
		if fromVersion == "" || toVersion == "" {
			http.Error(w, "Both 'from' and 'to' parameters are required", http.StatusBadRequest)
			return
		}
		
		// Get compatibility info
		dt.compatibilityMatrix.mu.RLock()
		info, exists := dt.compatibilityMatrix.Matrix[fromVersion][toVersion]
		dt.compatibilityMatrix.mu.RUnlock()
		
		if !exists {
			http.Error(w, "Compatibility information not found", http.StatusNotFound)
			return
		}
		
		response := map[string]interface{}{
			"compatibility":     info,
			"migration_guide":   fmt.Sprintf("/docs/migration/%s-to-%s", fromVersion, toVersion),
			"automated_tools":   info.Tools,
			"estimated_effort":  info.Effort,
		}
		
		json.NewEncoder(w).Encode(response)
	})
}