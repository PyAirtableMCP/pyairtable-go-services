package services

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// SecurityService handles webhook security operations
type SecurityService struct{}

// NewSecurityService creates a new security service
func NewSecurityService() *SecurityService {
	return &SecurityService{}
}

// GenerateSecret generates a cryptographically secure random secret
func (s *SecurityService) GenerateSecret() (string, error) {
	// Generate 32 bytes (256 bits) of random data
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random secret: %w", err)
	}
	
	return hex.EncodeToString(bytes), nil
}

// GenerateSignature generates an HMAC-SHA256 signature for the payload
func (s *SecurityService) GenerateSignature(secret, payload string, timestamp time.Time) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("secret cannot be empty")
	}
	
	// Create the signed payload: timestamp.payload
	signedPayload := fmt.Sprintf("%d.%s", timestamp.Unix(), payload)
	
	// Create HMAC-SHA256 hash
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(signedPayload))
	
	// Return signature in format: sha256=<hex_digest>
	signature := hex.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("sha256=%s", signature), nil
}

// ValidateSignature validates an HMAC-SHA256 signature
func (s *SecurityService) ValidateSignature(secret, payload, signature string, timestamp time.Time, toleranceSeconds int) error {
	if secret == "" {
		return fmt.Errorf("secret cannot be empty")
	}
	
	if signature == "" {
		return fmt.Errorf("signature cannot be empty")
	}
	
	// Check if signature has the correct format
	if !strings.HasPrefix(signature, "sha256=") {
		return fmt.Errorf("invalid signature format, expected 'sha256=<hex_digest>'")
	}
	
	// Extract the hex digest
	receivedDigest := strings.TrimPrefix(signature, "sha256=")
	
	// Validate timestamp (prevent replay attacks)
	now := time.Now()
	if toleranceSeconds > 0 {
		minTime := now.Add(-time.Duration(toleranceSeconds) * time.Second)
		maxTime := now.Add(time.Duration(toleranceSeconds) * time.Second)
		
		if timestamp.Before(minTime) || timestamp.After(maxTime) {
			return fmt.Errorf("timestamp out of tolerance range")
		}
	}
	
	// Generate expected signature
	expectedSignature, err := s.GenerateSignature(secret, payload, timestamp)
	if err != nil {
		return fmt.Errorf("failed to generate expected signature: %w", err)
	}
	
	expectedDigest := strings.TrimPrefix(expectedSignature, "sha256=")
	
	// Compare signatures using constant-time comparison to prevent timing attacks
	if !hmac.Equal([]byte(receivedDigest), []byte(expectedDigest)) {
		return fmt.Errorf("signature validation failed")
	}
	
	return nil
}

// CreateWebhookHeaders creates the standard webhook headers
func (s *SecurityService) CreateWebhookHeaders(eventID, eventType, signature string, timestamp time.Time) map[string]string {
	return map[string]string{
		"Content-Type":              "application/json",
		"User-Agent":               "PyAirtable-Webhooks/1.0",
		"X-PyAirtable-Event-ID":    eventID,
		"X-PyAirtable-Event-Type":  eventType,
		"X-PyAirtable-Signature":   signature,
		"X-PyAirtable-Timestamp":   fmt.Sprintf("%d", timestamp.Unix()),
		"X-PyAirtable-Webhook-ID":  "", // Will be set by caller
	}
}

// ValidateWebhookURL validates that a webhook URL is safe to call
func (s *SecurityService) ValidateWebhookURL(url string) error {
	if url == "" {
		return fmt.Errorf("URL cannot be empty")
	}
	
	// Check URL scheme
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return fmt.Errorf("URL must use HTTP or HTTPS scheme")
	}
	
	// Prevent localhost and private IP ranges in production
	lowercaseURL := strings.ToLower(url)
	
	// Block localhost
	if strings.Contains(lowercaseURL, "localhost") || 
	   strings.Contains(lowercaseURL, "127.0.0.1") ||
	   strings.Contains(lowercaseURL, "::1") {
		return fmt.Errorf("localhost URLs are not allowed")
	}
	
	// Block private IP ranges (basic check)
	privateRanges := []string{
		"10.", "172.16.", "172.17.", "172.18.", "172.19.", "172.20.",
		"172.21.", "172.22.", "172.23.", "172.24.", "172.25.", "172.26.",
		"172.27.", "172.28.", "172.29.", "172.30.", "172.31.", "192.168.",
		"169.254.", // Link-local
	}
	
	for _, privateRange := range privateRanges {
		if strings.Contains(lowercaseURL, "://"+privateRange) ||
		   strings.Contains(lowercaseURL, "://www."+privateRange) {
			return fmt.Errorf("private IP ranges are not allowed")
		}
	}
	
	return nil
}

// SanitizeHeaders sanitizes custom headers to prevent injection attacks
func (s *SecurityService) SanitizeHeaders(headers map[string]interface{}) map[string]string {
	if headers == nil {
		return nil
	}
	
	sanitized := make(map[string]string)
	
	// List of forbidden headers that could be dangerous
	forbiddenHeaders := map[string]bool{
		"authorization":       true,
		"cookie":             true,
		"set-cookie":         true,
		"x-forwarded-for":    true,
		"x-real-ip":          true,
		"x-forwarded-proto":  true,
		"host":               true,
		"connection":         true,
		"upgrade":            true,
		"proxy-authorization": true,
	}
	
	for key, value := range headers {
		keyLower := strings.ToLower(strings.TrimSpace(key))
		
		// Skip forbidden headers
		if forbiddenHeaders[keyLower] {
			continue
		}
		
		// Skip headers that start with x-pyairtable- (reserved)
		if strings.HasPrefix(keyLower, "x-pyairtable-") {
			continue
		}
		
		// Convert value to string and sanitize
		strValue := fmt.Sprintf("%v", value)
		strValue = strings.TrimSpace(strValue)
		
		// Skip empty values
		if strValue == "" {
			continue
		}
		
		// Limit header value length
		if len(strValue) > 1000 {
			strValue = strValue[:1000]
		}
		
		// Remove control characters
		var cleaned strings.Builder
		for _, r := range strValue {
			if r >= 32 && r < 127 || r == '\t' {
				cleaned.WriteRune(r)
			}
		}
		
		if cleaned.Len() > 0 {
			sanitized[key] = cleaned.String()
		}
	}
	
	return sanitized
}