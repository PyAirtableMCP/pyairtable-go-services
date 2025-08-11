package middleware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"pyairtable-go/pkg/audit"
)

// AuditMiddleware logs all API requests for security monitoring
type AuditMiddleware struct {
	logger *audit.AuditLogger
}

// NewAuditMiddleware creates a new audit middleware
func NewAuditMiddleware(logger *audit.AuditLogger) *AuditMiddleware {
	return &AuditMiddleware{
		logger: logger,
	}
}

// Handler returns the fiber middleware handler
func (a *AuditMiddleware) Handler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Start time for performance tracking
		start := time.Now()
		
		// Extract request information
		method := c.Method()
		path := c.Path()
		userAgent := c.Get("User-Agent")
		ipAddress := c.IP()
		userID := extractUserID(c)
		tenantID := extractTenantID(c)
		requestID := c.Locals("requestId").(string)

		// Process the request
		err := c.Next()

		// Calculate response time
		duration := time.Since(start)
		statusCode := c.Response().StatusCode()

		// Determine event type and severity based on the request
		eventType, severity := categorizeRequest(method, path, statusCode)

		// Determine result
		result := "SUCCESS"
		if statusCode >= 400 {
			result = "FAILURE"
		}
		if statusCode == 401 || statusCode == 403 {
			result = "DENIED"
		}

		// Create audit event details
		details := map[string]interface{}{
			"method":        method,
			"path":          path,
			"status_code":   statusCode,
			"response_time": duration.Milliseconds(),
			"request_size":  len(c.Request().Body()),
			"response_size": len(c.Response().Body()),
		}

		// Add error details if there was an error
		if err != nil {
			details["error"] = err.Error()
		}

		// Create message
		message := fmt.Sprintf("%s %s - %d (%dms)", method, path, statusCode, duration.Milliseconds())

		// Log the audit event
		auditErr := a.logger.LogEvent(context.Background(), &audit.AuditEvent{
			EventType: eventType,
			Severity:  severity,
			UserID:    userID,
			TenantID:  tenantID,
			IPAddress: ipAddress,
			UserAgent: userAgent,
			Resource:  path,
			Action:    method,
			Result:    result,
			Message:   message,
			RequestID: requestID,
		})

		if auditErr != nil {
			// Log audit failure but don't fail the request
			fmt.Printf("Failed to log audit event: %v\n", auditErr)
		}

		// Log security incidents for suspicious activity
		if shouldLogSecurityIncident(method, path, statusCode, duration) {
			a.logSecurityIncident(c, userID, ipAddress, method, path, statusCode, duration)
		}

		return err
	}
}

// categorizeRequest determines the event type and severity based on request characteristics
func categorizeRequest(method, path string, statusCode int) (audit.EventType, audit.Severity) {
	// Authentication endpoints
	if strings.Contains(path, "/auth/login") {
		if statusCode == 200 {
			return audit.EventLogin, audit.SeverityInfo
		}
		return audit.EventLoginFailed, audit.SeverityWarning
	}
	
	if strings.Contains(path, "/auth/logout") {
		return audit.EventLogout, audit.SeverityInfo
	}
	
	if strings.Contains(path, "/auth/refresh") {
		return audit.EventTokenRefresh, audit.SeverityInfo
	}

	// Data access operations
	switch method {
	case "GET":
		if statusCode == 403 || statusCode == 401 {
			return audit.EventAccessDenied, audit.SeverityWarning
		}
		return audit.EventDataAccess, audit.SeverityInfo
	case "POST", "PUT", "PATCH":
		if statusCode == 403 || statusCode == 401 {
			return audit.EventAccessDenied, audit.SeverityWarning
		}
		return audit.EventDataModify, audit.SeverityInfo
	case "DELETE":
		if statusCode == 403 || statusCode == 401 {
			return audit.EventAccessDenied, audit.SeverityWarning
		}
		return audit.EventDataDelete, audit.SeverityWarning
	}

	// Default to access event
	if statusCode == 403 || statusCode == 401 {
		return audit.EventAccessDenied, audit.SeverityWarning
	}
	
	if statusCode >= 500 {
		return audit.EventDataAccess, audit.SeverityError
	}
	
	return audit.EventDataAccess, audit.SeverityInfo
}

// shouldLogSecurityIncident determines if a request indicates suspicious activity
func shouldLogSecurityIncident(method, path string, statusCode int, duration time.Duration) bool {
	// Multiple failed authentication attempts
	if strings.Contains(path, "/auth/login") && statusCode == 401 {
		return true
	}
	
	// Access denied to sensitive resources
	if statusCode == 403 && isSensitiveResource(path) {
		return true
	}
	
	// Unusually slow requests (potential DoS)
	if duration > 30*time.Second {
		return true
	}
	
	// SQL injection attempts (basic detection)
	if containsSQLInjectionPatterns(path) {
		return true
	}
	
	// Directory traversal attempts
	if strings.Contains(path, "..") || strings.Contains(path, "etc/passwd") {
		return true
	}
	
	return false
}

// isSensitiveResource checks if a resource is considered sensitive
func isSensitiveResource(path string) bool {
	sensitivePatterns := []string{
		"/admin",
		"/config",
		"/settings",
		"/users",
		"/permissions",
		"/keys",
		"/secrets",
	}
	
	for _, pattern := range sensitivePatterns {
		if strings.Contains(path, pattern) {
			return true
		}
	}
	
	return false
}

// containsSQLInjectionPatterns performs basic SQL injection detection
func containsSQLInjectionPatterns(path string) bool {
	patterns := []string{
		"union select",
		"drop table",
		"insert into",
		"delete from",
		"update set",
		"exec(",
		"<script",
		"javascript:",
		"onload=",
	}
	
	lowerPath := strings.ToLower(path)
	for _, pattern := range patterns {
		if strings.Contains(lowerPath, pattern) {
			return true
		}
	}
	
	return false
}

// logSecurityIncident logs a security incident
func (a *AuditMiddleware) logSecurityIncident(c *fiber.Ctx, userID, ipAddress, method, path string, statusCode int, duration time.Duration) {
	details := map[string]interface{}{
		"method":       method,
		"path":         path,
		"status_code":  statusCode,
		"duration_ms":  duration.Milliseconds(),
		"user_agent":   c.Get("User-Agent"),
		"referer":      c.Get("Referer"),
		"request_body": string(c.Request().Body()),
	}
	
	var message string
	if strings.Contains(path, "/auth/login") && statusCode == 401 {
		message = "Failed authentication attempt detected"
	} else if statusCode == 403 {
		message = "Unauthorized access attempt to sensitive resource"
	} else if duration > 30*time.Second {
		message = "Unusually slow request detected - potential DoS attempt"
	} else if containsSQLInjectionPatterns(path) {
		message = "Potential SQL injection attempt detected"
	} else if strings.Contains(path, "..") {
		message = "Directory traversal attempt detected"
	} else {
		message = "Suspicious activity detected"
	}
	
	a.logger.LogSecurityIncident(
		context.Background(),
		userID,
		ipAddress,
		message,
		details,
	)
}

// extractUserID extracts user ID from the request context
func extractUserID(c *fiber.Ctx) string {
	// Try to get from JWT claims in context
	if userID := c.Locals("user_id"); userID != nil {
		if id, ok := userID.(string); ok {
			return id
		}
	}
	
	// Try to get from custom header
	if userID := c.Get("X-User-ID"); userID != "" {
		return userID
	}
	
	return ""
}

// extractTenantID extracts tenant ID from the request context
func extractTenantID(c *fiber.Ctx) string {
	// Try to get from JWT claims in context
	if tenantID := c.Locals("tenant_id"); tenantID != nil {
		if id, ok := tenantID.(string); ok {
			return id
		}
	}
	
	// Try to get from custom header
	if tenantID := c.Get("X-Tenant-ID"); tenantID != "" {
		return tenantID
	}
	
	return ""
}