package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"log/slog"

	"github.com/Reg-Kris/pyairtable-web-bff/internal/services"
)

type Handlers struct {
	services *services.Services
	logger   *slog.Logger
}

func New(services *services.Services, logger *slog.Logger) *Handlers {
	return &Handlers{
		services: services,
		logger:   logger,
	}
}

// Ping handles health check requests
func (h *Handlers) Ping(c fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":    "ok",
		"service":   "web-bff",
		"timestamp": time.Now(),
	})
}

// Protected is an example protected endpoint
func (h *Handlers) Protected(c fiber.Ctx) error {
	userID := c.Locals("userID")
	
	return c.JSON(fiber.Map{
		"message": "This is a protected endpoint",
		"user_id": userID,
		"service": "web-bff",
	})
}