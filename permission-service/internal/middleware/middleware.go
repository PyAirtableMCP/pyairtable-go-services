package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// AuthMiddleware validates tokens by calling the auth service
type AuthMiddleware struct {
	logger      *zap.Logger
	authBaseURL string
	client      *http.Client
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(logger *zap.Logger) *AuthMiddleware {
	authBaseURL := os.Getenv("AUTH_SERVICE_URL")
	if authBaseURL == "" {
		authBaseURL = "http://localhost:8082"
	}

	return &AuthMiddleware{
		logger:      logger,
		authBaseURL: authBaseURL,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// TokenValidationResponse represents auth service response
type TokenValidationResponse struct {
	UserID   string   `json:"user_id"`
	TenantID string   `json:"tenant_id"`
	Email    string   `json:"email"`
	Roles    []string `json:"roles,omitempty"`
}

// Authenticate validates token with auth service
func (m *AuthMiddleware) Authenticate() fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing authorization header",
			})
		}

		// Extract token from Bearer scheme
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid authorization header format",
			})
		}

		token := parts[1]
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Empty token",
			})
		}

		// Validate token with auth service
		userInfo, err := m.validateTokenWithAuthService(c.Context(), token)
		if err != nil {
			m.logger.Error("Token validation failed", 
				zap.Error(err),
				zap.String("ip", c.IP()),
				zap.String("user_agent", c.Get("User-Agent")),
			)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired token",
			})
		}

		// Set user information in context
		c.Locals("userID", userInfo.UserID)
		c.Locals("tenantID", userInfo.TenantID)
		c.Locals("email", userInfo.Email)
		c.Locals("roles", userInfo.Roles)
		c.Locals("token", token)

		m.logger.Debug("User authenticated", 
			zap.String("user_id", userInfo.UserID),
			zap.String("tenant_id", userInfo.TenantID),
			zap.String("email", userInfo.Email),
		)

		return c.Next()
	}
}

// validateTokenWithAuthService calls the auth service to validate token
func (m *AuthMiddleware) validateTokenWithAuthService(ctx context.Context, token string) (*TokenValidationResponse, error) {
	url := fmt.Sprintf("%s/api/v1/auth/validate", m.authBaseURL)
	
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "permission-service/1.0")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth service request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth service returned status %d", resp.StatusCode)
	}

	var userInfo TokenValidationResponse
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode auth response: %w", err)
	}

	return &userInfo, nil
}

// RequireTenant ensures user has access to the specified tenant
func RequireTenant() fiber.Handler {
	return func(c *fiber.Ctx) error {
		userTenantID := c.Locals("tenantID")
		if userTenantID == nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "No tenant access",
			})
		}

		// Check if tenant ID is provided in request (header, query, or path)
		requestTenantID := c.Get("X-Tenant-ID")
		if requestTenantID == "" {
			requestTenantID = c.Query("tenant_id")
		}
		if requestTenantID == "" {
			requestTenantID = c.Params("tenantId")
		}

		// If no tenant specified in request, use user's tenant
		if requestTenantID == "" {
			requestTenantID = userTenantID.(string)
		}

		// Verify user has access to this tenant
		if requestTenantID != userTenantID.(string) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Access denied to this tenant",
			})
		}

		// Set the validated tenant ID in context
		c.Locals("validatedTenantID", requestTenantID)

		return c.Next()
	}
}

// RequestLogger logs all requests
func RequestLogger(logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		
		err := c.Next()
		
		duration := time.Since(start)
		status := c.Response().StatusCode()
		
		logLevel := zap.InfoLevel
		if status >= 400 && status < 500 {
			logLevel = zap.WarnLevel
		} else if status >= 500 {
			logLevel = zap.ErrorLevel
		}
		
		logger.Log(logLevel, "HTTP Request",
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.String("query", c.OriginalURL()),
			zap.Int("status", status),
			zap.Duration("duration", duration),
			zap.String("ip", c.IP()),
			zap.String("user_agent", c.Get("User-Agent")),
			zap.String("user_id", getStringFromLocals(c, "userID")),
			zap.String("tenant_id", getStringFromLocals(c, "tenantID")),
		)
		
		return err
	}
}

// ErrorHandler provides centralized error handling
func ErrorHandler(logger *zap.Logger) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError

		var e *fiber.Error
		if errors.As(err, &e) {
			code = e.Code
		}

		logger.Error("Request error",
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.Int("status", code),
			zap.Error(err),
			zap.String("ip", c.IP()),
			zap.String("user_id", getStringFromLocals(c, "userID")),
		)

		return c.Status(code).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
}

// RateLimiter provides rate limiting
func RateLimiter() fiber.Handler {
	// Simple in-memory rate limiter for demonstration
	// In production, use Redis-based rate limiting
	return func(c *fiber.Ctx) error {
		// For now, just pass through
		// TODO: Implement proper rate limiting
		return c.Next()
	}
}

// CORS middleware
func CORS() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("Access-Control-Allow-Origin", "*")
		c.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Tenant-ID")

		if c.Method() == "OPTIONS" {
			return c.SendStatus(fiber.StatusNoContent)
		}

		return c.Next()
	}
}

// getStringFromLocals safely gets a string value from fiber locals
func getStringFromLocals(c *fiber.Ctx, key string) string {
	if val := c.Locals(key); val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}