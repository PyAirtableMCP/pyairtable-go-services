package middleware

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"go.uber.org/zap"
)

type CORSMiddleware struct {
	logger *zap.Logger
}

func NewCORSMiddleware(logger *zap.Logger) *CORSMiddleware {
	return &CORSMiddleware{
		logger: logger,
	}
}

func (cm *CORSMiddleware) GetCORSHandler() fiber.Handler {
	// Get CORS origins from environment variable, default to localhost:3000
	corsOrigins := os.Getenv("CORS_ORIGINS")
	if corsOrigins == "" {
		corsOrigins = "http://localhost:3000"
	}

	origins := strings.Split(corsOrigins, ",")
	for i, origin := range origins {
		origins[i] = strings.TrimSpace(origin)
	}

	cm.logger.Info("Configuring CORS", zap.Strings("allowed_origins", origins))

	return cors.New(cors.Config{
		AllowOrigins:     strings.Join(origins, ","),
		AllowCredentials: true,
		AllowHeaders:     "Content-Type, Authorization",
		AllowMethods:     "GET, POST, PUT, DELETE, OPTIONS",
	})
}

func (cm *CORSMiddleware) HandlePreflight() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Method() == "OPTIONS" {
			return c.SendStatus(fiber.StatusNoContent)
		}
		return c.Next()
	}
}