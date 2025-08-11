package handlers

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"log/slog"

	"github.com/Reg-Kris/pyairtable-webhook-service/internal/services"
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
		"service":   "webhook-service",
		"timestamp": time.Now(),
	})
}

// Protected is an example protected endpoint
func (h *Handlers) Protected(c fiber.Ctx) error {
	userID := c.Locals("userID")
	
	return c.JSON(fiber.Map{
		"message": "This is a protected endpoint",
		"user_id": userID,
		"service": "webhook-service",
	})
}

// getUserIDFromContext extracts user ID from the request context
func (h *Handlers) getUserIDFromContext(c fiber.Ctx) (uint, error) {
	userIDStr := c.Get("X-User-ID")
	if userIDStr == "" {
		userIDVal := c.Locals("userID")
		if userIDVal != nil {
			if uid, ok := userIDVal.(uint); ok {
				return uid, nil
			}
			if uidStr, ok := userIDVal.(string); ok {
				userIDStr = uidStr
			}
		}
	}
	
	if userIDStr == "" {
		return 0, fiber.NewError(fiber.StatusUnauthorized, "User ID not found")
	}
	
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		return 0, fiber.NewError(fiber.StatusBadRequest, "Invalid user ID")
	}
	
	return uint(userID), nil
}

// getWorkspaceIDFromQuery extracts workspace ID from query parameters
func (h *Handlers) getWorkspaceIDFromQuery(c fiber.Ctx) (*uint, error) {
	workspaceIDStr := c.Query("workspace_id")
	if workspaceIDStr == "" {
		return nil, nil
	}
	
	workspaceID, err := strconv.ParseUint(workspaceIDStr, 10, 32)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "Invalid workspace ID")
	}
	
	workspaceIDUint := uint(workspaceID)
	return &workspaceIDUint, nil
}

// getPaginationParams extracts pagination parameters from query
func (h *Handlers) getPaginationParams(c fiber.Ctx) (limit, offset int) {
	limit = 20 // default
	offset = 0 // default
	
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}
	
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}
	
	return limit, offset
}