package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"

	"github.com/Reg-Kris/pyairtable-file-service/pkg/metrics"
)

// ErrorHandler provides centralized error handling
func ErrorHandler(logger *slog.Logger) fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError

		var e *fiber.Error
		if errors.As(err, &e) {
			code = e.Code
		}

		logger.Error("Request error",
			"method", c.Method(),
			"path", c.Path(),
			"status", code,
			"error", err.Error(),
		)

		return c.Status(code).JSON(fiber.Map{
			"error":   true,
			"message": err.Error(),
		})
	}
}

// JWT middleware for authentication
func JWT(secret string) fiber.Handler {
	return func(c fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   true,
				"message": "Missing authorization header",
			})
		}

		tokenString := strings.Replace(authHeader, "Bearer ", "", 1)
		
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   true,
				"message": "Invalid token",
			})
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   true,
				"message": "Invalid token claims",
			})
		}

		c.Locals("userID", claims["user_id"])
		c.Locals("claims", claims)

		return c.Next()
	}
}

// Metrics middleware for Prometheus metrics
func Metrics(registry *metrics.Registry) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		duration := time.Since(start)
		status := c.Response().StatusCode()

		registry.HTTPRequestsTotal.WithLabelValues(
			c.Method(),
			c.Path(),
			fmt.Sprintf("%d", status),
		).Inc()

		registry.HTTPRequestDuration.WithLabelValues(
			c.Method(),
			c.Path(),
		).Observe(duration.Seconds())

		return err
	}
}