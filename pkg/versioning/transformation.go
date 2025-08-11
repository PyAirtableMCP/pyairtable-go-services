package versioning

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
)

// TransformationRule defines how to transform data between versions
type TransformationRule struct {
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	SourceVersion  string                 `json:"source_version"`
	TargetVersion  string                 `json:"target_version"`
	ApplyToRequest bool                   `json:"apply_to_request"`
	ApplyToResponse bool                  `json:"apply_to_response"`
	PathPatterns   []string               `json:"path_patterns"`  // Regex patterns for paths
	Methods        []string               `json:"methods"`        // HTTP methods
	FieldMappings  []FieldMapping         `json:"field_mappings"`
	CustomTransform func(interface{}) interface{} `json:"-"`
}

// FieldMapping defines how to map fields between versions
type FieldMapping struct {
	Operation     MappingOperation `json:"operation"`
	SourcePath    string          `json:"source_path"`     // JSONPath or dot notation
	TargetPath    string          `json:"target_path"`     // JSONPath or dot notation
	DefaultValue  interface{}     `json:"default_value"`
	ValueMapping  map[string]interface{} `json:"value_mapping"`  // Map old values to new values
	Condition     string          `json:"condition"`       // Conditional transformation
	Transform     string          `json:"transform"`       // Transformation function name
}

// MappingOperation defines the type of field mapping
type MappingOperation string

const (
	OpCopy      MappingOperation = "copy"       // Copy field as-is
	OpRename    MappingOperation = "rename"     // Rename field
	OpTransform MappingOperation = "transform"  // Apply transformation
	OpRemove    MappingOperation = "remove"     // Remove field
	OpAdd       MappingOperation = "add"        // Add field with default value
	OpSplit     MappingOperation = "split"      // Split field into multiple
	OpMerge     MappingOperation = "merge"      // Merge multiple fields into one
	OpValidate  MappingOperation = "validate"   // Validate field value
)

// Transformer handles data transformation between API versions
type Transformer struct {
	rules     map[string][]TransformationRule // version -> rules
	functions map[string]TransformFunc        // Custom transformation functions
}

// TransformFunc is a custom transformation function
type TransformFunc func(input interface{}, context *TransformContext) interface{}

// TransformContext provides context for transformations
type TransformContext struct {
	SourceVersion string
	TargetVersion string
	RequestPath   string
	Method        string
	Headers       http.Header
	Metadata      map[string]interface{}
}

// NewTransformer creates a new data transformer
func NewTransformer() *Transformer {
	return &Transformer{
		rules:     make(map[string][]TransformationRule),
		functions: make(map[string]TransformFunc),
	}
}

// RegisterRule adds a transformation rule
func (t *Transformer) RegisterRule(rule TransformationRule) {
	key := fmt.Sprintf("%s->%s", rule.SourceVersion, rule.TargetVersion)
	t.rules[key] = append(t.rules[key], rule)
}

// RegisterFunction adds a custom transformation function
func (t *Transformer) RegisterFunction(name string, fn TransformFunc) {
	t.functions[name] = fn
}

// TransformRequest transforms request data from one version to another
func (t *Transformer) TransformRequest(data interface{}, context *TransformContext) (interface{}, error) {
	key := fmt.Sprintf("%s->%s", context.SourceVersion, context.TargetVersion)
	rules, exists := t.rules[key]
	if !exists {
		return data, nil // No transformation needed
	}
	
	for _, rule := range rules {
		if !rule.ApplyToRequest {
			continue
		}
		
		if !t.ruleApplies(rule, context) {
			continue
		}
		
		var err error
		data, err = t.applyRule(data, rule, context)
		if err != nil {
			return nil, fmt.Errorf("failed to apply rule %s: %v", rule.Name, err)
		}
	}
	
	return data, nil
}

// TransformResponse transforms response data from one version to another
func (t *Transformer) TransformResponse(data interface{}, context *TransformContext) (interface{}, error) {
	key := fmt.Sprintf("%s->%s", context.SourceVersion, context.TargetVersion)
	rules, exists := t.rules[key]
	if !exists {
		return data, nil // No transformation needed
	}
	
	for _, rule := range rules {
		if !rule.ApplyToResponse {
			continue
		}
		
		if !t.ruleApplies(rule, context) {
			continue
		}
		
		var err error
		data, err = t.applyRule(data, rule, context)
		if err != nil {
			return nil, fmt.Errorf("failed to apply rule %s: %v", rule.Name, err)
		}
	}
	
	return data, nil
}

// ruleApplies checks if a transformation rule applies to the current context
func (t *Transformer) ruleApplies(rule TransformationRule, context *TransformContext) bool {
	// Check HTTP methods
	if len(rule.Methods) > 0 {
		methodMatches := false
		for _, method := range rule.Methods {
			if strings.EqualFold(method, context.Method) {
				methodMatches = true
				break
			}
		}
		if !methodMatches {
			return false
		}
	}
	
	// Check path patterns
	if len(rule.PathPatterns) > 0 {
		pathMatches := false
		for _, pattern := range rule.PathPatterns {
			// Simple pattern matching (could be enhanced with regex)
			if strings.Contains(context.RequestPath, pattern) {
				pathMatches = true
				break
			}
		}
		if !pathMatches {
			return false
		}
	}
	
	return true
}

// applyRule applies a single transformation rule
func (t *Transformer) applyRule(data interface{}, rule TransformationRule, context *TransformContext) (interface{}, error) {
	// If there's a custom transform function, use it
	if rule.CustomTransform != nil {
		return rule.CustomTransform(data), nil
	}
	
	// Apply field mappings
	return t.applyFieldMappings(data, rule.FieldMappings, context)
}

// applyFieldMappings applies field mapping transformations
func (t *Transformer) applyFieldMappings(data interface{}, mappings []FieldMapping, context *TransformContext) (interface{}, error) {
	// Convert to map for manipulation
	dataMap, err := t.toMap(data)
	if err != nil {
		return nil, err
	}
	
	result := make(map[string]interface{})
	
	// Copy original data
	for k, v := range dataMap {
		result[k] = v
	}
	
	// Apply each mapping
	for _, mapping := range mappings {
		err := t.applyFieldMapping(result, mapping, context)
		if err != nil {
			return nil, fmt.Errorf("failed to apply mapping %s->%s: %v", 
				mapping.SourcePath, mapping.TargetPath, err)
		}
	}
	
	return result, nil
}

// applyFieldMapping applies a single field mapping
func (t *Transformer) applyFieldMapping(data map[string]interface{}, mapping FieldMapping, context *TransformContext) error {
	switch mapping.Operation {
	case OpCopy:
		return t.copyField(data, mapping.SourcePath, mapping.TargetPath)
	case OpRename:
		return t.renameField(data, mapping.SourcePath, mapping.TargetPath)
	case OpTransform:
		return t.transformField(data, mapping, context)
	case OpRemove:
		return t.removeField(data, mapping.SourcePath)
	case OpAdd:
		return t.addField(data, mapping.TargetPath, mapping.DefaultValue)
	case OpSplit:
		return t.splitField(data, mapping)
	case OpMerge:
		return t.mergeFields(data, mapping)
	case OpValidate:
		return t.validateField(data, mapping)
	default:
		return fmt.Errorf("unknown mapping operation: %s", mapping.Operation)
	}
}

// copyField copies a field from source to target path
func (t *Transformer) copyField(data map[string]interface{}, sourcePath, targetPath string) error {
	value, exists := t.getNestedValue(data, sourcePath)
	if !exists {
		return nil // Source field doesn't exist, skip
	}
	
	return t.setNestedValue(data, targetPath, value)
}

// renameField renames a field from source to target path
func (t *Transformer) renameField(data map[string]interface{}, sourcePath, targetPath string) error {
	value, exists := t.getNestedValue(data, sourcePath)
	if !exists {
		return nil // Source field doesn't exist, skip
	}
	
	// Set new field
	err := t.setNestedValue(data, targetPath, value)
	if err != nil {
		return err
	}
	
	// Remove old field
	return t.removeField(data, sourcePath)
}

// transformField applies transformation to a field
func (t *Transformer) transformField(data map[string]interface{}, mapping FieldMapping, context *TransformContext) error {
	value, exists := t.getNestedValue(data, mapping.SourcePath)
	if !exists {
		return nil
	}
	
	// Apply value mapping if configured
	if mapping.ValueMapping != nil {
		if mappedValue, hasMapped := mapping.ValueMapping[fmt.Sprintf("%v", value)]; hasMapped {
			value = mappedValue
		}
	}
	
	// Apply custom transformation function if specified
	if mapping.Transform != "" {
		if fn, exists := t.functions[mapping.Transform]; exists {
			value = fn(value, context)
		}
	}
	
	return t.setNestedValue(data, mapping.TargetPath, value)
}

// removeField removes a field from the data
func (t *Transformer) removeField(data map[string]interface{}, path string) error {
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		delete(data, parts[0])
		return nil
	}
	
	// Navigate to parent
	current := data
	for i := 0; i < len(parts)-1; i++ {
		if next, ok := current[parts[i]].(map[string]interface{}); ok {
			current = next
		} else {
			return nil // Path doesn't exist
		}
	}
	
	delete(current, parts[len(parts)-1])
	return nil
}

// addField adds a field with default value
func (t *Transformer) addField(data map[string]interface{}, path string, value interface{}) error {
	return t.setNestedValue(data, path, value)
}

// splitField splits a field into multiple fields
func (t *Transformer) splitField(data map[string]interface{}, mapping FieldMapping) error {
	value, exists := t.getNestedValue(data, mapping.SourcePath)
	if !exists {
		return nil
	}
	
	// Simple implementation: split string by delimiter
	if str, ok := value.(string); ok {
		parts := strings.Split(str, ",")
		for i, part := range parts {
			targetPath := fmt.Sprintf("%s_%d", mapping.TargetPath, i)
			err := t.setNestedValue(data, targetPath, strings.TrimSpace(part))
			if err != nil {
				return err
			}
		}
	}
	
	return nil
}

// mergeFields merges multiple fields into one
func (t *Transformer) mergeFields(data map[string]interface{}, mapping FieldMapping) error {
	// This would require more complex configuration to specify source fields
	// For now, implement a simple version
	return nil
}

// validateField validates a field value
func (t *Transformer) validateField(data map[string]interface{}, mapping FieldMapping) error {
	value, exists := t.getNestedValue(data, mapping.SourcePath)
	if !exists {
		return nil
	}
	
	// Simple validation - could be extended with more complex rules
	if mapping.Condition != "" {
		// Implement condition checking
	}
	
	return nil
}

// getNestedValue retrieves a nested value using dot notation
func (t *Transformer) getNestedValue(data map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	current := data
	
	for _, part := range parts[:len(parts)-1] {
		if next, ok := current[part].(map[string]interface{}); ok {
			current = next
		} else {
			return nil, false
		}
	}
	
	value, exists := current[parts[len(parts)-1]]
	return value, exists
}

// setNestedValue sets a nested value using dot notation
func (t *Transformer) setNestedValue(data map[string]interface{}, path string, value interface{}) error {
	parts := strings.Split(path, ".")
	current := data
	
	// Navigate/create path
	for _, part := range parts[:len(parts)-1] {
		if _, exists := current[part]; !exists {
			current[part] = make(map[string]interface{})
		}
		
		if next, ok := current[part].(map[string]interface{}); ok {
			current = next
		} else {
			return fmt.Errorf("cannot set nested value: path conflict at %s", part)
		}
	}
	
	current[parts[len(parts)-1]] = value
	return nil
}

// toMap converts interface{} to map[string]interface{}
func (t *Transformer) toMap(data interface{}) (map[string]interface{}, error) {
	if m, ok := data.(map[string]interface{}); ok {
		return m, nil
	}
	
	// Try JSON marshaling/unmarshaling
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	
	var result map[string]interface{}
	err = json.Unmarshal(jsonData, &result)
	return result, err
}

// TransformMiddleware provides HTTP middleware for data transformation
type TransformMiddleware struct {
	transformer *Transformer
}

// NewTransformMiddleware creates a new transform middleware
func NewTransformMiddleware(transformer *Transformer) *TransformMiddleware {
	return &TransformMiddleware{transformer: transformer}
}

// Middleware returns the HTTP middleware function
func (tm *TransformMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vctx, ok := GetVersionFromContext(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		
		// Create transform context
		context := &TransformContext{
			SourceVersion: vctx.Version.String(),
			TargetVersion: "v1", // Default target version
			RequestPath:   r.URL.Path,
			Method:        r.Method,
			Headers:       r.Header,
			Metadata:      make(map[string]interface{}),
		}
		
		// Transform request body if present
		if r.Body != nil && (r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH") {
			bodyBytes, err := io.ReadAll(r.Body)
			if err == nil && len(bodyBytes) > 0 {
				r.Body.Close()
				
				var requestData interface{}
				if json.Unmarshal(bodyBytes, &requestData) == nil {
					transformedData, err := tm.transformer.TransformRequest(requestData, context)
					if err == nil {
						if transformedBytes, marshalErr := json.Marshal(transformedData); marshalErr == nil {
							r.Body = io.NopCloser(bytes.NewBuffer(transformedBytes))
							r.ContentLength = int64(len(transformedBytes))
						}
					}
				} else {
					// Not JSON, restore original body
					r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				}
			}
		}
		
		// Wrap response writer for response transformation
		wrappedWriter := &transformResponseWriter{
			ResponseWriter: w,
			transformer:    tm.transformer,
			context:        context,
		}
		
		next.ServeHTTP(wrappedWriter, r)
	})
}

// transformResponseWriter wraps http.ResponseWriter to transform responses
type transformResponseWriter struct {
	http.ResponseWriter
	transformer *Transformer
	context     *TransformContext
	buffer      bytes.Buffer
	statusCode  int
}

// Write captures response data for transformation
func (w *transformResponseWriter) Write(data []byte) (int, error) {
	return w.buffer.Write(data)
}

// WriteHeader captures status code
func (w *transformResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

// Flush transforms and writes the response
func (w *transformResponseWriter) Flush() {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	
	// Write status code
	w.ResponseWriter.WriteHeader(w.statusCode)
	
	data := w.buffer.Bytes()
	if len(data) == 0 {
		return
	}
	
	// Try to transform response if it's JSON
	var responseData interface{}
	if json.Unmarshal(data, &responseData) == nil {
		transformedData, err := w.transformer.TransformResponse(responseData, w.context)
		if err == nil {
			if transformedBytes, marshalErr := json.Marshal(transformedData); marshalErr == nil {
				w.ResponseWriter.Write(transformedBytes)
				return
			}
		}
	}
	
	// Fall back to original data
	w.ResponseWriter.Write(data)
}

// Default transformation functions
func init() {
	// These would be registered with the transformer
}

// Common transformation functions
func StringToUppercase(input interface{}, context *TransformContext) interface{} {
	if str, ok := input.(string); ok {
		return strings.ToUpper(str)
	}
	return input
}

func StringToLowercase(input interface{}, context *TransformContext) interface{} {
	if str, ok := input.(string); ok {
		return strings.ToLower(str)
	}
	return input
}

func TimestampToISO(input interface{}, context *TransformContext) interface{} {
	// Convert various timestamp formats to ISO 8601
	return input // Placeholder implementation
}