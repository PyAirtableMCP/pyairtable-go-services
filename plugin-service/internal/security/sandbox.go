package security

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	
	"github.com/pyairtable/go-services/plugin-service/internal/models"
)

// SecurityManager manages plugin security operations
type SecurityManager struct {
	config          *SecurityConfig
	permissions     *PermissionManager
	codeVerifier    *CodeVerifier  
	quarantine      *QuarantineManager
	auditLogger     AuditLogger
	mu              sync.RWMutex
}

// SecurityConfig contains security configuration
type SecurityConfig struct {
	CodeSigningEnabled     bool
	TrustedSigners         []string
	SandboxEnabled         bool
	PermissionCheckEnabled bool
	MaxPluginSize          int64
	AllowedFileTypes       []string
	QuarantineEnabled      bool
	QuarantineTime         time.Duration
}

// PermissionManager manages plugin permissions
type PermissionManager struct {
	permissions map[uuid.UUID][]models.Permission
	mu          sync.RWMutex
}

// CodeVerifier handles code signing verification
type CodeVerifier struct {
	trustedKeys map[string]*rsa.PublicKey
	mu          sync.RWMutex
}

// QuarantineManager manages plugin quarantine
type QuarantineManager struct {
	quarantined map[uuid.UUID]*QuarantineEntry
	mu          sync.RWMutex
}

// QuarantineEntry represents a quarantined plugin
type QuarantineEntry struct {
	PluginID      uuid.UUID
	Reason        string
	QuarantinedAt time.Time
	ReleaseAt     time.Time
	Metadata      map[string]interface{}
}

// AuditLogger interface for security audit logging
type AuditLogger interface {
	LogSecurityEvent(event SecurityEvent) error
}

// SecurityEvent represents a security-related event
type SecurityEvent struct {
	Type        string                 `json:"type"`
	PluginID    uuid.UUID             `json:"plugin_id"`
	UserID      uuid.UUID             `json:"user_id"`
	WorkspaceID uuid.UUID             `json:"workspace_id"`
	Timestamp   time.Time             `json:"timestamp"`
	Severity    string                `json:"severity"`
	Message     string                `json:"message"`
	Details     map[string]interface{} `json:"details"`
	IPAddress   string                `json:"ip_address,omitempty"`
	UserAgent   string                `json:"user_agent,omitempty"`
}

// SecurityCheckResult represents the result of security checks
type SecurityCheckResult struct {
	Passed       bool     `json:"passed"`
	Violations   []string `json:"violations,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
	Quarantined  bool     `json:"quarantined"`
	Hash         string   `json:"hash"`
}

// NewSecurityManager creates a new security manager
func NewSecurityManager(config *SecurityConfig, auditLogger AuditLogger) *SecurityManager {
	return &SecurityManager{
		config:       config,
		permissions:  NewPermissionManager(),
		codeVerifier: NewCodeVerifier(config.TrustedSigners),
		quarantine:   NewQuarantineManager(),
		auditLogger:  auditLogger,
	}
}

// ValidatePlugin performs comprehensive security validation on a plugin
func (sm *SecurityManager) ValidatePlugin(ctx context.Context, plugin *models.Plugin, userID, workspaceID uuid.UUID) (*SecurityCheckResult, error) {
	result := &SecurityCheckResult{
		Passed: true,
		Hash:   sm.calculateHash(plugin.WasmBinary),
	}

	// Check if plugin is quarantined
	if sm.config.QuarantineEnabled && sm.quarantine.IsQuarantined(plugin.ID) {
		result.Passed = false
		result.Quarantined = true
		result.Violations = append(result.Violations, "Plugin is quarantined")
		
		sm.auditLogger.LogSecurityEvent(SecurityEvent{
			Type:        "validation.quarantine_blocked",
			PluginID:    plugin.ID,
			UserID:      userID,
			WorkspaceID: workspaceID,
			Timestamp:   time.Now(),
			Severity:    "high",
			Message:     "Attempted to install quarantined plugin",
		})
		
		return result, nil
	}

	// Validate plugin size
	if int64(len(plugin.WasmBinary)) > sm.config.MaxPluginSize {
		result.Passed = false
		result.Violations = append(result.Violations, fmt.Sprintf("Plugin size exceeds limit: %d > %d", 
			len(plugin.WasmBinary), sm.config.MaxPluginSize))
	}

	// Validate code signature
	if sm.config.CodeSigningEnabled {
		if err := sm.codeVerifier.VerifySignature(plugin); err != nil {
			result.Passed = false
			result.Violations = append(result.Violations, fmt.Sprintf("Code signature verification failed: %v", err))
			
			sm.auditLogger.LogSecurityEvent(SecurityEvent{
				Type:        "validation.signature_failed",
				PluginID:    plugin.ID,
				UserID:      userID,
				WorkspaceID: workspaceID,
				Timestamp:   time.Now(),
				Severity:    "high",
				Message:     "Plugin signature verification failed",
				Details: map[string]interface{}{
					"error": err.Error(),
				},
			})
		}
	}

	// Validate permissions
	if sm.config.PermissionCheckEnabled {
		violations := sm.permissions.ValidatePermissions(plugin.Permissions, workspaceID)
		if len(violations) > 0 {
			result.Passed = false
			result.Violations = append(result.Violations, violations...)
		}
	}

	// Perform static analysis
	staticResult := sm.performStaticAnalysis(plugin.WasmBinary)
	result.Violations = append(result.Violations, staticResult.Violations...)
	result.Warnings = append(result.Warnings, staticResult.Warnings...)
	
	if len(staticResult.Violations) > 0 {
		result.Passed = false
	}

	// Log validation result
	sm.auditLogger.LogSecurityEvent(SecurityEvent{
		Type:        "validation.completed",
		PluginID:    plugin.ID,
		UserID:      userID,
		WorkspaceID: workspaceID,
		Timestamp:   time.Now(),
		Severity:    "info",
		Message:     "Plugin security validation completed",
		Details: map[string]interface{}{
			"passed":      result.Passed,
			"violations":  len(result.Violations),
			"warnings":    len(result.Warnings),
			"hash":        result.Hash,
		},
	})

	return result, nil
}

// CheckPermission checks if a plugin has permission to perform an action
func (sm *SecurityManager) CheckPermission(pluginID uuid.UUID, resource, action string, context map[string]interface{}) (bool, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if !sm.config.PermissionCheckEnabled {
		return true, nil
	}

	return sm.permissions.CheckPermission(pluginID, resource, action, context)
}

// QuarantinePlugin quarantines a plugin
func (sm *SecurityManager) QuarantinePlugin(pluginID uuid.UUID, reason string, metadata map[string]interface{}) error {
	if !sm.config.QuarantineEnabled {
		return fmt.Errorf("quarantine is disabled")
	}

	return sm.quarantine.QuarantinePlugin(pluginID, reason, sm.config.QuarantineTime, metadata)
}

// ReleaseFromQuarantine releases a plugin from quarantine
func (sm *SecurityManager) ReleaseFromQuarantine(pluginID uuid.UUID) error {
	return sm.quarantine.ReleasePlugin(pluginID)
}

// calculateHash calculates SHA-256 hash of plugin binary
func (sm *SecurityManager) calculateHash(binary []byte) string {
	hash := sha256.Sum256(binary)
	return hex.EncodeToString(hash[:])
}

// performStaticAnalysis performs static analysis on WASM binary
func (sm *SecurityManager) performStaticAnalysis(binary []byte) *SecurityCheckResult {
	result := &SecurityCheckResult{
		Passed: true,
	}

	// Check for suspicious patterns
	suspiciousPatterns := []struct {
		pattern string
		message string
	}{
		{"\x00\x61\x73\x6d", "WASM magic number found"}, // Not actually suspicious, just checking
		// Add more patterns for actual suspicious content
	}

	for _, pattern := range suspiciousPatterns {
		// Simplified pattern matching - in reality you'd use proper WASM parsing
		if len(binary) > 4 && string(binary[:4]) == pattern.pattern {
			result.Warnings = append(result.Warnings, pattern.message)
		}
	}

	// Check binary size
	if len(binary) > 1024*1024 { // 1MB
		result.Warnings = append(result.Warnings, "Large binary size may indicate embedded resources")
	}

	// TODO: Add more sophisticated static analysis
	// - Import/export analysis
	// - Control flow analysis
	// - Resource usage pattern detection

	return result
}

// NewPermissionManager creates a new permission manager
func NewPermissionManager() *PermissionManager {
	return &PermissionManager{
		permissions: make(map[uuid.UUID][]models.Permission),
	}
}

// ValidatePermissions validates plugin permissions
func (pm *PermissionManager) ValidatePermissions(permissions []models.Permission, workspaceID uuid.UUID) []string {
	var violations []string

	// Define allowed resources and actions
	allowedResources := map[string][]string{
		"records":     {"read", "write", "delete"},
		"fields":      {"read", "write"},
		"tables":      {"read", "list"},
		"bases":       {"read", "list"},
		"attachments": {"read", "write"},
		"comments":    {"read", "write"},
		"webhooks":    {"create", "delete"},
		"formulas":    {"execute"},
		"ui":          {"render", "update"},
		"network":     {"http_request"},
		"storage":     {"read", "write"},
	}

	for _, permission := range permissions {
		allowed, exists := allowedResources[permission.Resource]
		if !exists {
			violations = append(violations, fmt.Sprintf("Unknown resource: %s", permission.Resource))
			continue
		}

		for _, action := range permission.Actions {
			found := false
			for _, allowedAction := range allowed {
				if action == allowedAction {
					found = true
					break
				}
			}
			if !found {
				violations = append(violations, fmt.Sprintf("Invalid action '%s' for resource '%s'", action, permission.Resource))
			}
		}
	}

	return violations
}

// CheckPermission checks if a plugin has permission to perform an action
func (pm *PermissionManager) CheckPermission(pluginID uuid.UUID, resource, action string, context map[string]interface{}) (bool, error) {
	pm.mu.RLock()
	permissions, exists := pm.permissions[pluginID]
	pm.mu.RUnlock()

	if !exists {
		return false, fmt.Errorf("no permissions found for plugin %s", pluginID)
	}

	for _, permission := range permissions {
		if permission.Resource == resource {
			for _, allowedAction := range permission.Actions {
				if allowedAction == action {
					// Check scope if specified
					if permission.Scope != "" {
						return pm.checkScope(permission.Scope, context), nil
					}
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// checkScope checks if the context matches the permission scope
func (pm *PermissionManager) checkScope(scope string, context map[string]interface{}) bool {
	// Simplified scope checking - in reality you'd have more sophisticated logic
	switch scope {
	case "own":
		// Plugin can only access its own resources
		return context["owner_id"] == context["plugin_id"]
	case "workspace":
		// Plugin can access workspace resources
		return context["workspace_id"] != nil
	default:
		return true
	}
}

// SetPluginPermissions sets permissions for a plugin
func (pm *PermissionManager) SetPluginPermissions(pluginID uuid.UUID, permissions []models.Permission) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.permissions[pluginID] = permissions
}

// NewCodeVerifier creates a new code verifier
func NewCodeVerifier(trustedSigners []string) *CodeVerifier {
	cv := &CodeVerifier{
		trustedKeys: make(map[string]*rsa.PublicKey),
	}

	// Load trusted public keys
	for _, signerPEM := range trustedSigners {
		if key, err := cv.parsePublicKey(signerPEM); err == nil {
			// Use key fingerprint as identifier
			fingerprint := cv.getKeyFingerprint(key)
			cv.trustedKeys[fingerprint] = key
		}
	}

	return cv
}

// VerifySignature verifies the plugin's code signature
func (cv *CodeVerifier) VerifySignature(plugin *models.Plugin) error {
	if plugin.Signature == "" {
		return fmt.Errorf("plugin has no signature")
	}

	if plugin.SignedBy == "" {
		return fmt.Errorf("plugin has no signer information")
	}

	cv.mu.RLock()
	publicKey, exists := cv.trustedKeys[plugin.SignedBy]
	cv.mu.RUnlock()

	if !exists {
		return fmt.Errorf("untrusted signer: %s", plugin.SignedBy)
	}

	// Decode signature
	signature, err := hex.DecodeString(plugin.Signature)
	if err != nil {
		return fmt.Errorf("invalid signature format: %w", err)
	}

	// Create hash of plugin binary
	hash := sha256.Sum256(plugin.WasmBinary)

	// Verify signature
	err = rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash[:], signature)
	if err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	return nil
}

// parsePublicKey parses a PEM-encoded public key
func (cv *CodeVerifier) parsePublicKey(pemKey string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}

	return rsaPub, nil
}

// getKeyFingerprint generates a fingerprint for a public key
func (cv *CodeVerifier) getKeyFingerprint(key *rsa.PublicKey) string {
	keyBytes, _ := x509.MarshalPKIXPublicKey(key)
	hash := sha256.Sum256(keyBytes)
	return hex.EncodeToString(hash[:])
}

// NewQuarantineManager creates a new quarantine manager
func NewQuarantineManager() *QuarantineManager {
	return &QuarantineManager{
		quarantined: make(map[uuid.UUID]*QuarantineEntry),
	}
}

// QuarantinePlugin adds a plugin to quarantine
func (qm *QuarantineManager) QuarantinePlugin(pluginID uuid.UUID, reason string, duration time.Duration, metadata map[string]interface{}) error {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	now := time.Now()
	qm.quarantined[pluginID] = &QuarantineEntry{
		PluginID:      pluginID,
		Reason:        reason,
		QuarantinedAt: now,
		ReleaseAt:     now.Add(duration),
		Metadata:      metadata,
	}

	return nil
}

// IsQuarantined checks if a plugin is quarantined
func (qm *QuarantineManager) IsQuarantined(pluginID uuid.UUID) bool {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	entry, exists := qm.quarantined[pluginID]
	if !exists {
		return false
	}

	// Check if quarantine has expired
	if time.Now().After(entry.ReleaseAt) {
		// Auto-release expired quarantine
		delete(qm.quarantined, pluginID)
		return false
	}

	return true
}

// ReleasePlugin releases a plugin from quarantine
func (qm *QuarantineManager) ReleasePlugin(pluginID uuid.UUID) error {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	delete(qm.quarantined, pluginID)
	return nil
}

// GetQuarantinedPlugins returns all quarantined plugins
func (qm *QuarantineManager) GetQuarantinedPlugins() map[uuid.UUID]*QuarantineEntry {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	result := make(map[uuid.UUID]*QuarantineEntry)
	for id, entry := range qm.quarantined {
		result[id] = entry
	}
	return result
}

