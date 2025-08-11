package middleware

import (
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// APIKeyAuth validates API key authentication
func APIKeyAuth(apiKey string, required bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !required {
			return c.Next()
		}

		// Check for API key in header
		key := c.Get("X-API-Key")
		if key == "" {
			// Check for API key in query parameter
			key = c.Query("api_key")
		}

		if key == "" {
			return c.Status(401).JSON(fiber.Map{
				"error": "API key required",
			})
		}

		// Use constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) != 1 {
			return c.Status(401).JSON(fiber.Map{
				"error": "Invalid API key",
			})
		}

		return c.Next()
	}
}

// JWTAuth validates JWT tokens
func JWTAuth(secretKey string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Extract token from Authorization header
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(401).JSON(fiber.Map{
				"error": "Authorization header required",
			})
		}

		// Check for Bearer token format
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			return c.Status(401).JSON(fiber.Map{
				"error": "Invalid authorization header format",
			})
		}

		tokenString := tokenParts[1]

		// Parse and validate token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// Explicitly validate algorithm to prevent algorithm confusion attacks
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fiber.NewError(401, "Invalid signing method: only HS256 allowed")
			}
			return []byte(secretKey), nil
		})

		if err != nil {
			return c.Status(401).JSON(fiber.Map{
				"error": "Invalid token",
			})
		}

		if !token.Valid {
			return c.Status(401).JSON(fiber.Map{
				"error": "Token expired or invalid",
			})
		}

		// Extract claims
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			// Store user ID in context
			if userID, exists := claims["user_id"]; exists {
				c.Locals("user_id", userID)
			}
			// Store issuer in context
			if issuer, exists := claims["iss"]; exists {
				c.Locals("issuer", issuer)
			}
		}

		return c.Next()
	}
}

// OptionalJWTAuth validates JWT tokens but doesn't require them
func OptionalJWTAuth(secretKey string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Next()
		}

		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			return c.Next()
		}

		tokenString := tokenParts[1]

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// Explicitly validate algorithm to prevent algorithm confusion attacks
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fiber.NewError(401, "Invalid signing method: only HS256 allowed")
			}
			return []byte(secretKey), nil
		})

		if err == nil && token.Valid {
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				if userID, exists := claims["user_id"]; exists {
					c.Locals("user_id", userID)
				}
				if issuer, exists := claims["iss"]; exists {
					c.Locals("issuer", issuer)
				}
			}
		}

		return c.Next()
	}
}

// GetUserID extracts user ID from JWT claims stored in context
func GetUserID(c *fiber.Ctx) (uint, bool) {
	userID := c.Locals("user_id")
	if userID == nil {
		return 0, false
	}

	// Handle different number types from JWT claims
	switch v := userID.(type) {
	case float64:
		return uint(v), true
	case int:
		return uint(v), true
	case uint:
		return v, true
	default:
		return 0, false
	}
}

// RequireAuth is a helper that ensures user is authenticated
func RequireAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if _, exists := GetUserID(c); !exists {
			return c.Status(401).JSON(fiber.Map{
				"error": "Authentication required",
			})
		}
		return c.Next()
	}
}