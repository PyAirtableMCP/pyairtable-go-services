package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// APIKey represents an API key with metadata
type APIKey struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	KeyHash     string            `json:"key_hash"`     // Hashed version of the key
	Prefix      string            `json:"prefix"`       // First 8 characters for identification
	TenantID    string            `json:"tenant_id"`
	UserID      string            `json:"user_id"`
	Scopes      []string          `json:"scopes"`       // Permissions/scopes
	RateLimit   *RateLimitInfo    `json:"rate_limit"`
	Quota       *QuotaInfo        `json:"quota"`
	Restrictions *Restrictions    `json:"restrictions"`
	CreatedAt   time.Time         `json:"created_at"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time        `json:"last_used_at,omitempty"`
	IsActive    bool              `json:"is_active"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// RateLimitInfo defines rate limiting for API keys
type RateLimitInfo struct {
	RequestsPerSecond int           `json:"requests_per_second"`
	RequestsPerMinute int           `json:"requests_per_minute"`
	RequestsPerHour   int           `json:"requests_per_hour"`
	BurstCapacity     int           `json:"burst_capacity"`
}

// QuotaInfo defines quota limits for API keys
type QuotaInfo struct {
	Daily   int64 `json:"daily"`
	Monthly int64 `json:"monthly"`
	Yearly  int64 `json:"yearly"`
}

// Restrictions defines access restrictions for API keys
type Restrictions struct {
	AllowedIPs    []string `json:"allowed_ips,omitempty"`
	BlockedIPs    []string `json:"blocked_ips,omitempty"`
	AllowedHosts  []string `json:"allowed_hosts,omitempty"`
	AllowedPaths  []string `json:"allowed_paths,omitempty"`
	TimeWindows   []TimeWindow `json:"time_windows,omitempty"`
}

// TimeWindow defines time-based access restrictions
type TimeWindow struct {
	StartTime string `json:"start_time"` // Format: "15:04"
	EndTime   string `json:"end_time"`   // Format: "15:04"
	Days      []string `json:"days"`     // ["monday", "tuesday", ...]
	Timezone  string `json:"timezone"`   // IANA timezone
}

// CreateAPIKeyRequest represents the request to create an API key
type CreateAPIKeyRequest struct {
	Name         string            `json:"name" validate:"required"`
	TenantID     string            `json:"tenant_id" validate:"required"`
	UserID       string            `json:"user_id" validate:"required"`
	Scopes       []string          `json:"scopes"`
	RateLimit    *RateLimitInfo    `json:"rate_limit"`
	Quota        *QuotaInfo        `json:"quota"`
	Restrictions *Restrictions     `json:"restrictions"`
	ExpiresIn    *time.Duration    `json:"expires_in,omitempty"` // Duration until expiry
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// APIKeyManager manages API keys
type APIKeyManager struct {
	redis     *redis.Client
	keyPrefix string
}

// NewAPIKeyManager creates a new API key manager
func NewAPIKeyManager(redis *redis.Client, keyPrefix string) *APIKeyManager {
	return &APIKeyManager{
		redis:     redis,
		keyPrefix: keyPrefix,
	}
}

// CreateAPIKey creates a new API key
func (m *APIKeyManager) CreateAPIKey(req *CreateAPIKeyRequest) (*APIKey, string, error) {
	// Generate the actual API key
	keyString, prefix, err := m.generateAPIKey()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate API key: %w", err)
	}
	
	// Hash the key for storage
	keyHash := m.hashAPIKey(keyString)
	
	// Create API key object
	apiKey := &APIKey{
		ID:          m.generateID(),
		Name:        req.Name,
		KeyHash:     keyHash,
		Prefix:      prefix,
		TenantID:    req.TenantID,
		UserID:      req.UserID,
		Scopes:      req.Scopes,
		RateLimit:   req.RateLimit,
		Quota:       req.Quota,
		Restrictions: req.Restrictions,
		CreatedAt:   time.Now(),
		IsActive:    true,
		Metadata:    req.Metadata,
	}
	
	// Set expiration if specified
	if req.ExpiresIn != nil {
		expiresAt := time.Now().Add(*req.ExpiresIn)
		apiKey.ExpiresAt = &expiresAt
	}
	
	// Store in Redis
	err = m.storeAPIKey(apiKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to store API key: %w", err)
	}
	
	return apiKey, keyString, nil
}

// ValidateAPIKey validates an API key and returns the associated metadata
func (m *APIKeyManager) ValidateAPIKey(keyString string) (*APIKey, error) {
	if keyString == "" {
		return nil, fmt.Errorf("empty API key")
	}
	
	// Extract prefix from key
	prefix := m.extractPrefix(keyString)
	if prefix == "" {
		return nil, fmt.Errorf("invalid API key format")
	}
	
	// Hash the key
	keyHash := m.hashAPIKey(keyString)
	
	// Find API key by hash
	apiKey, err := m.getAPIKeyByHash(keyHash)
	if err != nil {
		return nil, fmt.Errorf("API key not found: %w", err)
	}
	
	// Validate key is active
	if !apiKey.IsActive {
		return nil, fmt.Errorf("API key is inactive")
	}
	
	// Check expiration
	if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
		return nil, fmt.Errorf("API key has expired")
	}
	
	// Update last used timestamp
	now := time.Now()
	apiKey.LastUsedAt = &now
	m.updateLastUsed(apiKey.ID, now)
	
	return apiKey, nil
}

// GetAPIKey retrieves an API key by ID
func (m *APIKeyManager) GetAPIKey(keyID string) (*APIKey, error) {
	key := fmt.Sprintf("%s:apikey:%s", m.keyPrefix, keyID)
	data, err := m.redis.Get(context.Background(), key).Result()
	if err != nil {
		return nil, fmt.Errorf("API key not found: %w", err)
	}
	
	var apiKey APIKey
	err = json.Unmarshal([]byte(data), &apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal API key: %w", err)
	}
	
	return &apiKey, nil
}

// ListAPIKeys lists API keys for a tenant/user
func (m *APIKeyManager) ListAPIKeys(tenantID, userID string) ([]*APIKey, error) {
	pattern := fmt.Sprintf("%s:apikey:*", m.keyPrefix)
	keys, err := m.redis.Keys(context.Background(), pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}
	
	var apiKeys []*APIKey
	for _, key := range keys {
		data, err := m.redis.Get(context.Background(), key).Result()
		if err != nil {
			continue
		}
		
		var apiKey APIKey
		err = json.Unmarshal([]byte(data), &apiKey)
		if err != nil {
			continue
		}
		
		// Filter by tenant and user
		if (tenantID == "" || apiKey.TenantID == tenantID) &&
			(userID == "" || apiKey.UserID == userID) {
			apiKeys = append(apiKeys, &apiKey)
		}
	}
	
	return apiKeys, nil
}

// UpdateAPIKey updates an existing API key
func (m *APIKeyManager) UpdateAPIKey(keyID string, updates map[string]interface{}) (*APIKey, error) {
	apiKey, err := m.GetAPIKey(keyID)
	if err != nil {
		return nil, err
	}
	
	// Apply updates
	if name, ok := updates["name"].(string); ok {
		apiKey.Name = name
	}
	if scopes, ok := updates["scopes"].([]string); ok {
		apiKey.Scopes = scopes
	}
	if isActive, ok := updates["is_active"].(bool); ok {
		apiKey.IsActive = isActive
	}
	if rateLimit, ok := updates["rate_limit"].(*RateLimitInfo); ok {
		apiKey.RateLimit = rateLimit
	}
	if quota, ok := updates["quota"].(*QuotaInfo); ok {
		apiKey.Quota = quota
	}
	if restrictions, ok := updates["restrictions"].(*Restrictions); ok {
		apiKey.Restrictions = restrictions
	}
	if metadata, ok := updates["metadata"].(map[string]string); ok {
		apiKey.Metadata = metadata
	}
	
	// Store updated key
	err = m.storeAPIKey(apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to update API key: %w", err)
	}
	
	return apiKey, nil
}

// DeleteAPIKey deletes an API key
func (m *APIKeyManager) DeleteAPIKey(keyID string) error {
	// Get the API key first to clean up hash mapping
	apiKey, err := m.GetAPIKey(keyID)
	if err != nil {
		return err
	}
	
	// Delete the main key record
	key := fmt.Sprintf("%s:apikey:%s", m.keyPrefix, keyID)
	err = m.redis.Del(context.Background(), key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete API key: %w", err)
	}
	
	// Delete hash mapping
	hashKey := fmt.Sprintf("%s:apihash:%s", m.keyPrefix, apiKey.KeyHash)
	m.redis.Del(context.Background(), hashKey)
	
	return nil
}

// CheckRestrictions validates API key restrictions
func (m *APIKeyManager) CheckRestrictions(c *fiber.Ctx, apiKey *APIKey) error {
	if apiKey.Restrictions == nil {
		return nil
	}
	
	// Check IP restrictions
	clientIP := c.IP()
	if err := m.checkIPRestrictions(clientIP, apiKey.Restrictions); err != nil {
		return err
	}
	
	// Check host restrictions
	host := c.Hostname()
	if err := m.checkHostRestrictions(host, apiKey.Restrictions); err != nil {
		return err
	}
	
	// Check path restrictions
	path := c.Path()
	if err := m.checkPathRestrictions(path, apiKey.Restrictions); err != nil {
		return err
	}
	
	// Check time window restrictions
	if err := m.checkTimeRestrictions(apiKey.Restrictions); err != nil {
		return err
	}
	
	return nil
}

// generateAPIKey generates a new API key string
func (m *APIKeyManager) generateAPIKey() (string, string, error) {
	// Generate 32 random bytes
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", "", err
	}
	
	// Create key with prefix
	keyString := "pak_" + hex.EncodeToString(bytes)
	prefix := keyString[:8] // First 8 characters for identification
	
	return keyString, prefix, nil
}

// generateID generates a unique ID for API keys
func (m *APIKeyManager) generateID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// hashAPIKey creates a hash of the API key for secure storage
func (m *APIKeyManager) hashAPIKey(keyString string) string {
	hash := sha256.Sum256([]byte(keyString))
	return hex.EncodeToString(hash[:])
}

// extractPrefix extracts the prefix from an API key
func (m *APIKeyManager) extractPrefix(keyString string) string {
	if len(keyString) >= 8 {
		return keyString[:8]
	}
	return ""
}

// storeAPIKey stores an API key in Redis
func (m *APIKeyManager) storeAPIKey(apiKey *APIKey) error {
	data, err := json.Marshal(apiKey)
	if err != nil {
		return fmt.Errorf("failed to marshal API key: %w", err)
	}
	
	// Store main record
	key := fmt.Sprintf("%s:apikey:%s", m.keyPrefix, apiKey.ID)
	err = m.redis.Set(context.Background(), key, data, 0).Err()
	if err != nil {
		return fmt.Errorf("failed to store API key: %w", err)
	}
	
	// Store hash mapping for quick lookup
	hashKey := fmt.Sprintf("%s:apihash:%s", m.keyPrefix, apiKey.KeyHash)
	err = m.redis.Set(context.Background(), hashKey, apiKey.ID, 0).Err()
	if err != nil {
		return fmt.Errorf("failed to store API key hash mapping: %w", err)
	}
	
	return nil
}

// getAPIKeyByHash retrieves an API key by its hash
func (m *APIKeyManager) getAPIKeyByHash(keyHash string) (*APIKey, error) {
	// Get API key ID from hash mapping
	hashKey := fmt.Sprintf("%s:apihash:%s", m.keyPrefix, keyHash)
	keyID, err := m.redis.Get(context.Background(), hashKey).Result()
	if err != nil {
		return nil, fmt.Errorf("API key hash not found: %w", err)
	}
	
	// Get the actual API key
	return m.GetAPIKey(keyID)
}

// updateLastUsed updates the last used timestamp
func (m *APIKeyManager) updateLastUsed(keyID string, lastUsed time.Time) {
	key := fmt.Sprintf("%s:apikey:%s:lastused", m.keyPrefix, keyID)
	m.redis.Set(context.Background(), key, lastUsed.Unix(), 24*time.Hour)
}

// checkIPRestrictions validates IP restrictions
func (m *APIKeyManager) checkIPRestrictions(clientIP string, restrictions *Restrictions) error {
	// Check blocked IPs first
	for _, blockedIP := range restrictions.BlockedIPs {
		if m.matchIP(clientIP, blockedIP) {
			return fmt.Errorf("IP %s is blocked", clientIP)
		}
	}
	
	// Check allowed IPs
	if len(restrictions.AllowedIPs) > 0 {
		allowed := false
		for _, allowedIP := range restrictions.AllowedIPs {
			if m.matchIP(clientIP, allowedIP) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("IP %s is not allowed", clientIP)
		}
	}
	
	return nil
}

// matchIP checks if an IP matches a pattern (supports CIDR and wildcards)
func (m *APIKeyManager) matchIP(ip, pattern string) bool {
	// Simple implementation - in production, use proper CIDR matching
	if pattern == "*" {
		return true
	}
	return ip == pattern
}

// checkHostRestrictions validates host restrictions
func (m *APIKeyManager) checkHostRestrictions(host string, restrictions *Restrictions) error {
	if len(restrictions.AllowedHosts) == 0 {
		return nil
	}
	
	for _, allowedHost := range restrictions.AllowedHosts {
		if m.matchHost(host, allowedHost) {
			return nil
		}
	}
	
	return fmt.Errorf("host %s is not allowed", host)
}

// matchHost checks if a host matches a pattern (supports wildcards)
func (m *APIKeyManager) matchHost(host, pattern string) bool {
	if pattern == "*" {
		return true
	}
	
	// Simple wildcard matching
	if strings.HasPrefix(pattern, "*.") {
		domain := pattern[2:]
		return strings.HasSuffix(host, domain)
	}
	
	return host == pattern
}

// checkPathRestrictions validates path restrictions
func (m *APIKeyManager) checkPathRestrictions(path string, restrictions *Restrictions) error {
	if len(restrictions.AllowedPaths) == 0 {
		return nil
	}
	
	for _, allowedPath := range restrictions.AllowedPaths {
		if m.matchPath(path, allowedPath) {
			return nil
		}
	}
	
	return fmt.Errorf("path %s is not allowed", path)
}

// matchPath checks if a path matches a pattern (supports wildcards)
func (m *APIKeyManager) matchPath(path, pattern string) bool {
	if pattern == "*" {
		return true
	}
	
	// Simple prefix matching
	if strings.HasSuffix(pattern, "*") {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(path, prefix)
	}
	
	return path == pattern
}

// checkTimeRestrictions validates time window restrictions
func (m *APIKeyManager) checkTimeRestrictions(restrictions *Restrictions) error {
	if len(restrictions.TimeWindows) == 0 {
		return nil
	}
	
	now := time.Now()
	
	for _, window := range restrictions.TimeWindows {
		if m.isWithinTimeWindow(now, window) {
			return nil // Found a valid time window
		}
	}
	
	return fmt.Errorf("access not allowed at current time")
}

// isWithinTimeWindow checks if current time is within a time window
func (m *APIKeyManager) isWithinTimeWindow(now time.Time, window TimeWindow) bool {
	// Simple implementation - in production, handle timezone properly
	currentDay := strings.ToLower(now.Weekday().String())
	
	// Check if current day is allowed
	dayAllowed := false
	for _, day := range window.Days {
		if strings.ToLower(day) == currentDay {
			dayAllowed = true
			break
		}
	}
	
	if !dayAllowed {
		return false
	}
	
	// Check time range (simplified)
	currentTime := now.Format("15:04")
	return currentTime >= window.StartTime && currentTime <= window.EndTime
}