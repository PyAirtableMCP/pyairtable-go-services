package versioning

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// GraphQLVersionManager manages GraphQL schema versioning
type GraphQLVersionManager struct {
	schemas           map[string]*GraphQLSchema       // version -> schema
	federationSupport *FederationSupport
	directives        map[string]*SchemaDirective
	transformations   map[string][]FieldTransformation
	introspection     *IntrospectionManager
	mu                sync.RWMutex
}

// GraphQLSchema represents a versioned GraphQL schema
type GraphQLSchema struct {
	Version      string                    `json:"version"`
	SDL          string                    `json:"sdl"`           // Schema Definition Language
	Status       VersionStatus             `json:"status"`
	ReleaseDate  time.Time                 `json:"release_date"`
	Deprecated   *time.Time                `json:"deprecated,omitempty"`
	Sunset       *time.Time                `json:"sunset,omitempty"`
	Types        map[string]*GraphQLType   `json:"types"`
	Queries      map[string]*GraphQLField  `json:"queries"`
	Mutations    map[string]*GraphQLField  `json:"mutations"`
	Subscriptions map[string]*GraphQLField `json:"subscriptions"`
	Directives   []string                  `json:"directives"`
	Dependencies []string                  `json:"dependencies"`   // Other schemas this depends on
	Federation   *FederationConfig         `json:"federation"`
	Metadata     map[string]interface{}    `json:"metadata"`
}

// GraphQLType represents a GraphQL type definition
type GraphQLType struct {
	Name        string                 `json:"name"`
	Kind        TypeKind               `json:"kind"`        // OBJECT, INTERFACE, UNION, ENUM, INPUT_OBJECT, SCALAR
	Description string                 `json:"description"`
	Fields      map[string]*GraphQLField `json:"fields"`
	Interfaces  []string               `json:"interfaces"`  // For OBJECT types
	PossibleTypes []string             `json:"possible_types"` // For INTERFACE/UNION types
	EnumValues  []string               `json:"enum_values"` // For ENUM types
	Deprecated  bool                   `json:"deprecated"`
	Reason      string                 `json:"reason"`      // Deprecation reason
	Directives  []DirectiveUsage       `json:"directives"`
}

// GraphQLField represents a GraphQL field definition
type GraphQLField struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`        // GraphQL type notation
	Description string            `json:"description"`
	Arguments   map[string]*GraphQLArgument `json:"arguments"`
	Deprecated  bool              `json:"deprecated"`
	Reason      string            `json:"reason"`      // Deprecation reason
	Resolver    string            `json:"resolver"`    // Resolver function name
	Directives  []DirectiveUsage  `json:"directives"`
}

// GraphQLArgument represents a GraphQL field argument
type GraphQLArgument struct {
	Name         string           `json:"name"`
	Type         string           `json:"type"`
	Description  string           `json:"description"`
	DefaultValue interface{}      `json:"default_value"`
	Deprecated   bool             `json:"deprecated"`
	Reason       string           `json:"reason"`
	Directives   []DirectiveUsage `json:"directives"`
}

// TypeKind represents GraphQL type kinds
type TypeKind string

const (
	TypeKindObject      TypeKind = "OBJECT"
	TypeKindInterface   TypeKind = "INTERFACE"
	TypeKindUnion       TypeKind = "UNION"
	TypeKindEnum        TypeKind = "ENUM"
	TypeKindInputObject TypeKind = "INPUT_OBJECT"
	TypeKindScalar      TypeKind = "SCALAR"
)

// DirectiveUsage represents usage of a directive
type DirectiveUsage struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// SchemaDirective defines a custom GraphQL directive
type SchemaDirective struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Locations   []DirectiveLocation `json:"locations"`
	Arguments   map[string]*GraphQLArgument `json:"arguments"`
	Handler     DirectiveHandler    `json:"-"`
}

// DirectiveLocation represents where a directive can be used
type DirectiveLocation string

const (
	DirectiveLocationQuery               DirectiveLocation = "QUERY"
	DirectiveLocationMutation            DirectiveLocation = "MUTATION"
	DirectiveLocationSubscription        DirectiveLocation = "SUBSCRIPTION"
	DirectiveLocationField               DirectiveLocation = "FIELD"
	DirectiveLocationFragmentDefinition  DirectiveLocation = "FRAGMENT_DEFINITION"
	DirectiveLocationFragmentSpread      DirectiveLocation = "FRAGMENT_SPREAD"
	DirectiveLocationInlineFragment      DirectiveLocation = "INLINE_FRAGMENT"
	DirectiveLocationVariableDefinition  DirectiveLocation = "VARIABLE_DEFINITION"
	DirectiveLocationSchema              DirectiveLocation = "SCHEMA"
	DirectiveLocationScalar              DirectiveLocation = "SCALAR"
	DirectiveLocationObject              DirectiveLocation = "OBJECT"
	DirectiveLocationFieldDefinition     DirectiveLocation = "FIELD_DEFINITION"
	DirectiveLocationArgumentDefinition  DirectiveLocation = "ARGUMENT_DEFINITION"
	DirectiveLocationInterface           DirectiveLocation = "INTERFACE"
	DirectiveLocationUnion               DirectiveLocation = "UNION"
	DirectiveLocationEnum                DirectiveLocation = "ENUM"
	DirectiveLocationEnumValue           DirectiveLocation = "ENUM_VALUE"
	DirectiveLocationInputObject         DirectiveLocation = "INPUT_OBJECT"
	DirectiveLocationInputFieldDefinition DirectiveLocation = "INPUT_FIELD_DEFINITION"
)

// DirectiveHandler processes directive logic
type DirectiveHandler interface {
	Process(ctx context.Context, args map[string]interface{}, next func() interface{}) (interface{}, error)
}

// FederationSupport handles Apollo Federation
type FederationSupport struct {
	Gateway    *FederationGateway
	Services   map[string]*FederatedService
	Composition *CompositionConfig
	mu         sync.RWMutex
}

// FederationGateway manages the federated gateway
type FederationGateway struct {
	Version     string                    `json:"version"`
	Services    []ServiceEndpoint         `json:"services"`
	Schema      string                    `json:"schema"`      // Composed schema
	Composition *CompositionResult        `json:"composition"`
	Health      map[string]ServiceHealth  `json:"health"`
}

// ServiceEndpoint represents a federated service endpoint
type ServiceEndpoint struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Version string `json:"version"`
	SDL     string `json:"sdl"`
}

// FederatedService represents a service in the federation
type FederatedService struct {
	Name     string                 `json:"name"`
	Versions map[string]*GraphQLSchema `json:"versions"`
	Current  string                 `json:"current"`
	Health   ServiceHealth          `json:"health"`
}

// ServiceHealth represents the health status of a federated service
type ServiceHealth struct {
	Status      HealthStatus `json:"status"`
	LastChecked time.Time    `json:"last_checked"`
	ResponseTime time.Duration `json:"response_time"`
	Error       string       `json:"error,omitempty"`
}

// HealthStatus represents service health status
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusDegraded  HealthStatus = "degraded"
)

// CompositionConfig configures schema composition
type CompositionConfig struct {
	ValidationRules []ValidationRule `json:"validation_rules"`
	TypeMerging     *TypeMergingConfig `json:"type_merging"`
	Federation      *FederationConfig  `json:"federation"`
}

// ValidationRule defines schema validation rules
type ValidationRule struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Severity    string `json:"severity"` // error, warning, info
}

// TypeMergingConfig configures how types are merged across services
type TypeMergingConfig struct {
	Strategy     MergingStrategy `json:"strategy"`
	ConflictMode ConflictMode    `json:"conflict_mode"`
	Rules        []MergingRule   `json:"rules"`
}

// MergingStrategy defines how to merge types
type MergingStrategy string

const (
	MergingStrategyUnion        MergingStrategy = "union"
	MergingStrategyIntersection MergingStrategy = "intersection"
	MergingStrategyOverride     MergingStrategy = "override"
)

// ConflictMode defines how to handle conflicts
type ConflictMode string

const (
	ConflictModeError   ConflictMode = "error"
	ConflictModeWarning ConflictMode = "warning"
	ConflictModeIgnore  ConflictMode = "ignore"
)

// MergingRule defines specific merging rules
type MergingRule struct {
	TypePattern   string          `json:"type_pattern"`
	FieldPattern  string          `json:"field_pattern"`
	Strategy      MergingStrategy `json:"strategy"`
	Priority      int             `json:"priority"`
}

// FederationConfig configures federation-specific settings
type FederationConfig struct {
	Enabled    bool     `json:"enabled"`
	Version    string   `json:"version"`   // Federation spec version
	Entities   []string `json:"entities"`  // Entity types
	KeyFields  map[string][]string `json:"key_fields"` // type -> key fields
	Provides   map[string][]string `json:"provides"`   // field -> provided fields
	Requires   map[string][]string `json:"requires"`   // field -> required fields
}

// CompositionResult represents the result of schema composition
type CompositionResult struct {
	Success     bool              `json:"success"`
	Schema      string            `json:"schema"`
	Errors      []CompositionError `json:"errors"`
	Warnings    []CompositionError `json:"warnings"`
	Metadata    map[string]interface{} `json:"metadata"`
	ComposedAt  time.Time         `json:"composed_at"`
}

// CompositionError represents an error during composition
type CompositionError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Type        string `json:"type"`
	Field       string `json:"field,omitempty"`
	Service     string `json:"service,omitempty"`
	Severity    string `json:"severity"`
}

// FieldTransformation defines how to transform GraphQL fields between versions
type FieldTransformation struct {
	TypeName      string            `json:"type_name"`
	FieldName     string            `json:"field_name"`
	SourceVersion string            `json:"source_version"`
	TargetVersion string            `json:"target_version"`
	Operation     TransformOperation `json:"operation"`
	Mapping       FieldMapping      `json:"mapping"`
	Condition     string            `json:"condition"`
}

// TransformOperation defines the type of transformation
type TransformOperation string

const (
	TransformOpRename     TransformOperation = "rename"
	TransformOpType       TransformOperation = "type_change"
	TransformOpDeprecate  TransformOperation = "deprecate"
	TransformOpRemove     TransformOperation = "remove"
	TransformOpAdd        TransformOperation = "add"
	TransformOpArgument   TransformOperation = "argument_change"
)

// IntrospectionManager manages GraphQL introspection for different versions
type IntrospectionManager struct {
	schemas map[string]*IntrospectionResult
	mu      sync.RWMutex
}

// IntrospectionResult represents the result of GraphQL introspection
type IntrospectionResult struct {
	Schema    *IntrospectionSchema `json:"__schema"`
	Version   string               `json:"version"`
	Generated time.Time            `json:"generated"`
}

// IntrospectionSchema represents the __schema type in GraphQL introspection
type IntrospectionSchema struct {
	QueryType        *IntrospectionType      `json:"queryType"`
	MutationType     *IntrospectionType      `json:"mutationType"`
	SubscriptionType *IntrospectionType      `json:"subscriptionType"`
	Types            []*IntrospectionType    `json:"types"`
	Directives       []*IntrospectionDirective `json:"directives"`
}

// IntrospectionType represents a type in GraphQL introspection
type IntrospectionType struct {
	Kind          string                 `json:"kind"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	Fields        []*IntrospectionField  `json:"fields"`
	Interfaces    []*IntrospectionType   `json:"interfaces"`
	PossibleTypes []*IntrospectionType   `json:"possibleTypes"`
	EnumValues    []*IntrospectionEnumValue `json:"enumValues"`
	InputFields   []*IntrospectionInputValue `json:"inputFields"`
	OfType        *IntrospectionType     `json:"ofType"`
}

// IntrospectionField represents a field in GraphQL introspection
type IntrospectionField struct {
	Name              string                    `json:"name"`
	Description       string                    `json:"description"`
	Args              []*IntrospectionInputValue `json:"args"`
	Type              *IntrospectionType        `json:"type"`
	IsDeprecated      bool                      `json:"isDeprecated"`
	DeprecationReason string                    `json:"deprecationReason"`
}

// IntrospectionInputValue represents an input value in GraphQL introspection
type IntrospectionInputValue struct {
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	Type         *IntrospectionType `json:"type"`
	DefaultValue string             `json:"defaultValue"`
}

// IntrospectionEnumValue represents an enum value in GraphQL introspection
type IntrospectionEnumValue struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	IsDeprecated      bool   `json:"isDeprecated"`
	DeprecationReason string `json:"deprecationReason"`
}

// IntrospectionDirective represents a directive in GraphQL introspection
type IntrospectionDirective struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Locations   []string                  `json:"locations"`
	Args        []*IntrospectionInputValue `json:"args"`
}

// NewGraphQLVersionManager creates a new GraphQL version manager
func NewGraphQLVersionManager() *GraphQLVersionManager {
	return &GraphQLVersionManager{
		schemas:         make(map[string]*GraphQLSchema),
		directives:      make(map[string]*SchemaDirective),
		transformations: make(map[string][]FieldTransformation),
		federationSupport: &FederationSupport{
			Services: make(map[string]*FederatedService),
			Gateway: &FederationGateway{
				Services: make([]ServiceEndpoint, 0),
				Health:   make(map[string]ServiceHealth),
			},
		},
		introspection: &IntrospectionManager{
			schemas: make(map[string]*IntrospectionResult),
		},
	}
}

// RegisterSchema registers a GraphQL schema version
func (gvm *GraphQLVersionManager) RegisterSchema(schema *GraphQLSchema) error {
	gvm.mu.Lock()
	defer gvm.mu.Unlock()
	
	// Validate schema
	if err := gvm.validateSchema(schema); err != nil {
		return fmt.Errorf("schema validation failed: %v", err)
	}
	
	// Parse SDL to extract types, fields, etc.
	if err := gvm.parseSchema(schema); err != nil {
		return fmt.Errorf("schema parsing failed: %v", err)
	}
	
	gvm.schemas[schema.Version] = schema
	
	// Generate introspection
	if err := gvm.generateIntrospection(schema); err != nil {
		return fmt.Errorf("introspection generation failed: %v", err)
	}
	
	return nil
}

// validateSchema validates a GraphQL schema
func (gvm *GraphQLVersionManager) validateSchema(schema *GraphQLSchema) error {
	if schema.Version == "" {
		return fmt.Errorf("schema version is required")
	}
	
	if schema.SDL == "" {
		return fmt.Errorf("schema SDL is required")
	}
	
	// Additional validation logic would go here
	return nil
}

// parseSchema parses SDL and extracts schema components
func (gvm *GraphQLVersionManager) parseSchema(schema *GraphQLSchema) error {
	// This would contain actual SDL parsing logic
	// For now, we'll implement a simplified version
	
	schema.Types = make(map[string]*GraphQLType)
	schema.Queries = make(map[string]*GraphQLField)
	schema.Mutations = make(map[string]*GraphQLField)
	schema.Subscriptions = make(map[string]*GraphQLField)
	
	// Parse types from SDL (simplified)
	typeRegex := regexp.MustCompile(`type\s+(\w+)\s*{([^}]+)}`)
	matches := typeRegex.FindAllStringSubmatch(schema.SDL, -1)
	
	for _, match := range matches {
		typeName := match[1]
		typeBody := match[2]
		
		graphqlType := &GraphQLType{
			Name:   typeName,
			Kind:   TypeKindObject,
			Fields: make(map[string]*GraphQLField),
		}
		
		// Parse fields (simplified)
		fieldRegex := regexp.MustCompile(`(\w+)\s*:\s*([^,\n]+)`)
		fieldMatches := fieldRegex.FindAllStringSubmatch(typeBody, -1)
		
		for _, fieldMatch := range fieldMatches {
			fieldName := fieldMatch[1]
			fieldType := strings.TrimSpace(fieldMatch[2])
			
			graphqlType.Fields[fieldName] = &GraphQLField{
				Name: fieldName,
				Type: fieldType,
			}
		}
		
		schema.Types[typeName] = graphqlType
	}
	
	return nil
}

// generateIntrospection generates introspection result for a schema
func (gvm *GraphQLVersionManager) generateIntrospection(schema *GraphQLSchema) error {
	introspectionTypes := make([]*IntrospectionType, 0)
	
	// Convert schema types to introspection types
	for _, graphqlType := range schema.Types {
		introspectionType := &IntrospectionType{
			Kind:        string(graphqlType.Kind),
			Name:        graphqlType.Name,
			Description: graphqlType.Description,
			Fields:      make([]*IntrospectionField, 0),
		}
		
		for _, field := range graphqlType.Fields {
			introspectionField := &IntrospectionField{
				Name:              field.Name,
				Description:       field.Description,
				IsDeprecated:      field.Deprecated,
				DeprecationReason: field.Reason,
				Type: &IntrospectionType{
					Name: field.Type,
				},
			}
			
			introspectionType.Fields = append(introspectionType.Fields, introspectionField)
		}
		
		introspectionTypes = append(introspectionTypes, introspectionType)
	}
	
	result := &IntrospectionResult{
		Schema: &IntrospectionSchema{
			Types: introspectionTypes,
		},
		Version:   schema.Version,
		Generated: time.Now(),
	}
	
	gvm.introspection.schemas[schema.Version] = result
	return nil
}

// GetSchema returns a schema by version
func (gvm *GraphQLVersionManager) GetSchema(version string) (*GraphQLSchema, error) {
	gvm.mu.RLock()
	defer gvm.mu.RUnlock()
	
	schema, exists := gvm.schemas[version]
	if !exists {
		return nil, fmt.Errorf("schema version %s not found", version)
	}
	
	return schema, nil
}

// GetIntrospection returns introspection result for a version
func (gvm *GraphQLVersionManager) GetIntrospection(version string) (*IntrospectionResult, error) {
	gvm.introspection.mu.RLock()
	defer gvm.introspection.mu.RUnlock()
	
	result, exists := gvm.introspection.schemas[version]
	if !exists {
		return nil, fmt.Errorf("introspection for version %s not found", version)
	}
	
	return result, nil
}

// RegisterDirective registers a custom GraphQL directive
func (gvm *GraphQLVersionManager) RegisterDirective(directive *SchemaDirective) {
	gvm.mu.Lock()
	defer gvm.mu.Unlock()
	
	gvm.directives[directive.Name] = directive
}

// RegisterFederatedService registers a service in the federation
func (gvm *GraphQLVersionManager) RegisterFederatedService(service *FederatedService) {
	gvm.federationSupport.mu.Lock()
	defer gvm.federationSupport.mu.Unlock()
	
	gvm.federationSupport.Services[service.Name] = service
}

// ComposeSchema composes schemas from federated services
func (gvm *GraphQLVersionManager) ComposeSchema(version string) (*CompositionResult, error) {
	gvm.federationSupport.mu.Lock()
	defer gvm.federationSupport.mu.Unlock()
	
	var services []ServiceEndpoint
	
	// Collect service schemas for the specified version
	for _, service := range gvm.federationSupport.Services {
		if schema, exists := service.Versions[version]; exists {
			services = append(services, ServiceEndpoint{
				Name:    service.Name,
				Version: version,
				SDL:     schema.SDL,
			})
		}
	}
	
	if len(services) == 0 {
		return &CompositionResult{
			Success: false,
			Errors: []CompositionError{
				{
					Code:     "NO_SERVICES",
					Message:  fmt.Sprintf("No services found for version %s", version),
					Severity: "error",
				},
			},
			ComposedAt: time.Now(),
		}, nil
	}
	
	// Compose schemas (simplified implementation)
	composedSDL := gvm.composeSchemas(services)
	
	result := &CompositionResult{
		Success:    true,
		Schema:     composedSDL,
		Errors:     make([]CompositionError, 0),
		Warnings:   make([]CompositionError, 0),
		ComposedAt: time.Now(),
	}
	
	return result, nil
}

// composeSchemas composes multiple service schemas into one
func (gvm *GraphQLVersionManager) composeSchemas(services []ServiceEndpoint) string {
	var composedSDL strings.Builder
	
	composedSDL.WriteString("# Composed schema from federated services\n\n")
	
	for _, service := range services {
		composedSDL.WriteString(fmt.Sprintf("# From service: %s\n", service.Name))
		composedSDL.WriteString(service.SDL)
		composedSDL.WriteString("\n\n")
	}
	
	return composedSDL.String()
}

// TransformField applies field transformations between versions
func (gvm *GraphQLVersionManager) TransformField(typeName, fieldName, sourceVersion, targetVersion string, value interface{}) (interface{}, error) {
	key := fmt.Sprintf("%s->%s", sourceVersion, targetVersion)
	transformations, exists := gvm.transformations[key]
	
	if !exists {
		return value, nil // No transformations defined
	}
	
	for _, transform := range transformations {
		if transform.TypeName == typeName && transform.FieldName == fieldName {
			return gvm.applyTransformation(transform, value)
		}
	}
	
	return value, nil
}

// applyTransformation applies a field transformation
func (gvm *GraphQLVersionManager) applyTransformation(transform FieldTransformation, value interface{}) (interface{}, error) {
	switch transform.Operation {
	case TransformOpRename:
		// Field renaming is handled at the schema level
		return value, nil
	case TransformOpType:
		// Type conversion
		return gvm.convertType(value, transform.Mapping)
	case TransformOpDeprecate:
		// Field is deprecated but still functional
		return value, nil
	case TransformOpRemove:
		// Field is removed, return nil
		return nil, nil
	case TransformOpAdd:
		// Field is added with default value
		if value == nil {
			return transform.Mapping.DefaultValue, nil
		}
		return value, nil
	default:
		return value, nil
	}
}

// convertType converts value between types
func (gvm *GraphQLVersionManager) convertType(value interface{}, mapping FieldMapping) (interface{}, error) {
	// Implement type conversion logic based on mapping
	return value, nil
}

// VersionedExecutor executes GraphQL queries with version-specific schema
type VersionedExecutor struct {
	manager *GraphQLVersionManager
}

// NewVersionedExecutor creates a new versioned GraphQL executor
func NewVersionedExecutor(manager *GraphQLVersionManager) *VersionedExecutor {
	return &VersionedExecutor{manager: manager}
}

// Execute executes a GraphQL query with version-specific schema
func (ve *VersionedExecutor) Execute(ctx context.Context, query string, variables map[string]interface{}, version string) (interface{}, error) {
	schema, err := ve.manager.GetSchema(version)
	if err != nil {
		return nil, err
	}
	
	// This would integrate with your GraphQL execution engine
	// For now, return a placeholder response
	return map[string]interface{}{
		"data": map[string]interface{}{
			"version": version,
			"schema":  schema.Version,
			"query":   query,
		},
	}, nil
}

// GraphQLVersionMiddleware provides GraphQL versioning middleware
func (gvm *GraphQLVersionManager) GraphQLVersionMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract version from request (similar to REST API versioning)
			version := gvm.extractVersionFromRequest(r)
			
			// Validate version exists
			_, err := gvm.GetSchema(version)
			if err != nil {
				http.Error(w, fmt.Sprintf("GraphQL schema version %s not found", version), http.StatusBadRequest)
				return
			}
			
			// Add version to context
			ctx := context.WithValue(r.Context(), "graphql_version", version)
			r = r.WithContext(ctx)
			
			// Add version headers
			w.Header().Set("GraphQL-Version", version)
			
			next.ServeHTTP(w, r)
		})
	}
}

// extractVersionFromRequest extracts GraphQL version from request
func (gvm *GraphQLVersionManager) extractVersionFromRequest(r *http.Request) string {
	// Check header
	if version := r.Header.Get("GraphQL-Version"); version != "" {
		return version
	}
	
	// Check query parameter
	if version := r.URL.Query().Get("gql_version"); version != "" {
		return version
	}
	
	// Default to latest stable version
	return gvm.getLatestStableVersion()
}

// getLatestStableVersion returns the latest stable GraphQL schema version
func (gvm *GraphQLVersionManager) getLatestStableVersion() string {
	gvm.mu.RLock()
	defer gvm.mu.RUnlock()
	
	var latestVersion string
	var latestTime time.Time
	
	for version, schema := range gvm.schemas {
		if schema.Status == StatusStable && schema.ReleaseDate.After(latestTime) {
			latestVersion = version
			latestTime = schema.ReleaseDate
		}
	}
	
	if latestVersion == "" {
		// Return first version if no stable version found
		for version := range gvm.schemas {
			return version
		}
	}
	
	return latestVersion
}

// SchemaInfoHandler provides HTTP endpoint for GraphQL schema information
func (gvm *GraphQLVersionManager) SchemaInfoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		gvm.mu.RLock()
		schemas := make(map[string]interface{})
		for version, schema := range gvm.schemas {
			schemas[version] = map[string]interface{}{
				"version":      schema.Version,
				"status":       schema.Status,
				"release_date": schema.ReleaseDate,
				"deprecated":   schema.Deprecated,
				"sunset":       schema.Sunset,
				"types":        len(schema.Types),
				"queries":      len(schema.Queries),
				"mutations":    len(schema.Mutations),
				"subscriptions": len(schema.Subscriptions),
			}
		}
		gvm.mu.RUnlock()
		
		response := map[string]interface{}{
			"schemas": schemas,
			"latest_stable": gvm.getLatestStableVersion(),
		}
		
		json.NewEncoder(w).Encode(response)
	})
}

// IntrospectionHandler provides version-specific GraphQL introspection
func (gvm *GraphQLVersionManager) IntrospectionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		version := r.URL.Query().Get("version")
		if version == "" {
			version = gvm.getLatestStableVersion()
		}
		
		introspection, err := gvm.GetIntrospection(version)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		
		json.NewEncoder(w).Encode(introspection)
	})
}