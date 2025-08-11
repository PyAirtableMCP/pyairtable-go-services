package middleware

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pyairtable/api-gateway/internal/config"
	"go.uber.org/zap"
)

// Claims represents the JWT claims structure
type Claims struct {
	UserID      string   `json:"user_id"`
	Email       string   `json:"email"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	TenantID    string   `json:"tenant_id,omitempty"`
	SessionID   string   `json:"session_id,omitempty"`
	jwt.RegisteredClaims
}

// AuthMiddleware handles JWT token validation and API key authentication
type AuthMiddleware struct {
	config *config.Config
	logger *zap.Logger
}

func NewAuthMiddleware(cfg *config.Config, logger *zap.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		config: cfg,
		logger: logger,
	}
}

// JWT validates JWT tokens for requests
func (am *AuthMiddleware) JWT() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip authentication for health checks and metrics
		if am.shouldSkipAuth(c.Path()) {
			return c.Next()
		}

		// Get token from Authorization header
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			am.logger.Debug("Missing Authorization header", zap.String("path", c.Path()))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Authorization header required",
			})
		}

		// Extract Bearer token
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			am.logger.Debug("Invalid Authorization header format", zap.String("path", c.Path()))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Bearer token required",
			})
		}

		// Parse and validate token
		token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			// Validate signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(am.config.Auth.JWTSecret), nil
		})

		if err != nil {
			am.logger.Debug("Token validation failed", 
				zap.String("path", c.Path()),
				zap.Error(err))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid token",
			})
		}

		// Extract claims
		claims, ok := token.Claims.(*Claims)
		if !ok || !token.Valid {
			am.logger.Debug("Invalid token claims", zap.String("path", c.Path()))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid token claims",
			})
		}

		// Check token expiration
		if time.Now().After(claims.ExpiresAt.Time) {
			am.logger.Debug("Token expired", 
				zap.String("path", c.Path()),
				zap.Time("expired_at", claims.ExpiresAt.Time))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Token expired",
			})
		}

		// Store claims in context
		c.Locals("user", claims)
		c.Locals("user_id", claims.UserID)
		c.Locals("email", claims.Email)
		c.Locals("roles", claims.Roles)
		c.Locals("permissions", claims.Permissions)
		c.Locals("tenant_id", claims.TenantID)
		c.Locals("session_id", claims.SessionID)

		// Add user info to headers for backend services
		c.Set("X-User-ID", claims.UserID)
		c.Set("X-User-Email", claims.Email)
		c.Set("X-User-Roles", strings.Join(claims.Roles, ","))
		if claims.TenantID != "" {
			c.Set("X-Tenant-ID", claims.TenantID)
		}
		if claims.SessionID != "" {
			c.Set("X-Session-ID", claims.SessionID)
		}

		am.logger.Debug("JWT authentication successful",
			zap.String("user_id", claims.UserID),
			zap.String("path", c.Path()))

		return c.Next()
	}
}

// APIKey validates API keys for service-to-service communication
func (am *AuthMiddleware) APIKey() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip for paths that don't require auth
		if am.shouldSkipAuth(c.Path()) {
			return c.Next()
		}

		// Get API key from header
		apiKey := c.Get(am.config.Auth.APIKeyHeader)
		if apiKey == "" {
			am.logger.Debug("Missing API key", 
				zap.String("path", c.Path()),
				zap.String("header", am.config.Auth.APIKeyHeader))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "API key required",
			})
		}

		// In a real implementation, you would validate against a database
		// For now, we'll use a simple check against a configured key
		// TODO: Implement proper API key validation with database lookup
		if !am.isValidAPIKey(apiKey) {
			am.logger.Warn("Invalid API key", 
				zap.String("path", c.Path()),
				zap.String("api_key_prefix", apiKey[:min(len(apiKey), 8)]+"..."))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid API key",
			})
		}

		// Store API key info in context
		c.Locals("auth_type", "api_key")
		c.Locals("api_key", apiKey)

		am.logger.Debug("API key authentication successful", zap.String("path", c.Path()))
		return c.Next()
	}
}

// RequireRole ensures the user has one of the required roles
func (am *AuthMiddleware) RequireRole(roles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userRoles, ok := c.Locals("roles").([]string)
		if !ok {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "No roles found in token",
			})
		}

		// Check if user has any of the required roles
		for _, requiredRole := range roles {
			for _, userRole := range userRoles {
				if userRole == requiredRole {
					return c.Next()
				}
			}
		}

		am.logger.Warn("Insufficient permissions",
			zap.String("user_id", c.Locals("user_id").(string)),
			zap.Strings("user_roles", userRoles),
			zap.Strings("required_roles", roles),
			zap.String("path", c.Path()))

		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Insufficient permissions",
		})
	}
}

// RequirePermission ensures the user has a specific permission
func (am *AuthMiddleware) RequirePermission(permission string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userPermissions, ok := c.Locals("permissions").([]string)
		if !ok {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "No permissions found in token",
			})
		}

		// Check if user has the required permission
		for _, userPermission := range userPermissions {
			if userPermission == permission {
				return c.Next()
			}
		}

		am.logger.Warn("Missing required permission",
			zap.String("user_id", c.Locals("user_id").(string)),
			zap.Strings("user_permissions", userPermissions),
			zap.String("required_permission", permission),
			zap.String("path", c.Path()))

		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Missing required permission",
		})
	}
}

// TenantIsolation ensures users can only access their own tenant's data
func (am *AuthMiddleware) TenantIsolation() fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID := c.Locals("tenant_id")
		if tenantID == nil || tenantID == "" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Tenant isolation required",
			})
		}

		// Add tenant isolation header for backend services
		c.Set("X-Tenant-Isolation", "true")
		
		return c.Next()
	}
}

// shouldSkipAuth determines if authentication should be skipped for a path
func (am *AuthMiddleware) shouldSkipAuth(path string) bool {
	skipPaths := []string{
		"/health",
		"/ready",
		"/live",
		"/metrics",
		"/api/v1/auth/login",
		"/api/v1/auth/register",
		"/api/v1/auth/refresh",
	}

	for _, skipPath := range skipPaths {
		if strings.HasPrefix(path, skipPath) {
			return true
		}
	}

	return false
}

// isValidAPIKey validates an API key
// TODO: Implement proper validation with database lookup, rate limiting, etc.
func (am *AuthMiddleware) isValidAPIKey(apiKey string) bool {
	// This is a placeholder implementation
	// In production, you would:
	// 1. Hash the API key and look it up in a database
	// 2. Check if it's active and not expired
	// 3. Apply rate limiting per API key
	// 4. Log usage for audit purposes
	
	// For now, accept any key that starts with "gw_" and is at least 32 characters
	return strings.HasPrefix(apiKey, "gw_") && len(apiKey) >= 32
}

// GenerateToken generates a JWT token for testing purposes
func (am *AuthMiddleware) GenerateToken(userID, email string, roles, permissions []string, tenantID string) (string, error) {
	now := time.Now()
	claims := &Claims{
		UserID:      userID,
		Email:       email,
		Roles:       roles,
		Permissions: permissions,
		TenantID:    tenantID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(am.config.Auth.JWTExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "api-gateway",
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(am.config.Auth.JWTSecret))
}

// GetUserFromContext extracts user information from fiber context
func GetUserFromContext(c *fiber.Ctx) (*Claims, bool) {
	user, ok := c.Locals("user").(*Claims)
	return user, ok
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}