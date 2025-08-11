package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/pyairtable-compose/auth-service/internal/services"
	"go.uber.org/zap"
)

// AuthMiddleware provides JWT authentication middleware
type AuthMiddleware struct {
	logger      *zap.Logger
	authService *services.AuthService
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(logger *zap.Logger, authService *services.AuthService) *AuthMiddleware {
	return &AuthMiddleware{
		logger:      logger,
		authService: authService,
	}
}

// RequireAuth is middleware that validates JWT tokens
func (m *AuthMiddleware) RequireAuth(c *fiber.Ctx) error {
	// Get Authorization header
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		m.logger.Warn("Missing Authorization header", zap.String("path", c.Path()))
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing Authorization header",
		})
	}

	// Extract token from Bearer scheme
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		m.logger.Warn("Invalid Authorization header format", zap.String("path", c.Path()))
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid Authorization header format",
		})
	}

	tokenString := parts[1]

	// Validate token
	claims, err := m.authService.ValidateToken(tokenString)
	if err != nil {
		m.logger.Warn("Token validation failed", zap.Error(err), zap.String("path", c.Path()))
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid or expired token",
		})
	}

	// Store user information in context for downstream handlers
	c.Locals("userID", claims.UserID)
	c.Locals("userEmail", claims.Email)
	c.Locals("userRole", claims.Role)
	c.Locals("tenantID", claims.TenantID)
	c.Locals("claims", claims)

	return c.Next()
}

// RequireRole is middleware that validates user roles
func (m *AuthMiddleware) RequireRole(requiredRoles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userRole := c.Locals("userRole")
		if userRole == nil {
			m.logger.Error("User role not found in context - auth middleware not applied?")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Internal server error",
			})
		}

		role := userRole.(string)
		
		// Check if user has required role
		for _, requiredRole := range requiredRoles {
			if role == requiredRole {
				return c.Next()
			}
		}

		m.logger.Warn("Insufficient permissions", 
			zap.String("userRole", role),
			zap.Strings("requiredRoles", requiredRoles),
			zap.String("path", c.Path()))

		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Insufficient permissions",
		})
	}
}

// RequireAdmin is middleware that requires admin role
func (m *AuthMiddleware) RequireAdmin(c *fiber.Ctx) error {
	return m.RequireRole("admin")(c)
}

// OptionalAuth is middleware that validates JWT tokens but doesn't require them
func (m *AuthMiddleware) OptionalAuth(c *fiber.Ctx) error {
	// Get Authorization header
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		// No token provided, continue without setting user context
		return c.Next()
	}

	// Extract token from Bearer scheme
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		// Invalid format, continue without setting user context
		return c.Next()
	}

	tokenString := parts[1]

	// Validate token
	claims, err := m.authService.ValidateToken(tokenString)
	if err != nil {
		// Invalid token, continue without setting user context
		return c.Next()
	}

	// Store user information in context
	c.Locals("userID", claims.UserID)
	c.Locals("userEmail", claims.Email)
	c.Locals("userRole", claims.Role)
	c.Locals("tenantID", claims.TenantID)
	c.Locals("claims", claims)

	return c.Next()
}

// GetUserFromContext retrieves user information from context
func GetUserFromContext(c *fiber.Ctx) (*services.UserContext, bool) {
	userID := c.Locals("userID")
	if userID == nil {
		return nil, false
	}

	return &services.UserContext{
		UserID:   userID.(string),
		Email:    c.Locals("userEmail").(string),
		Role:     c.Locals("userRole").(string),
		TenantID: c.Locals("tenantID").(string),
	}, true
}