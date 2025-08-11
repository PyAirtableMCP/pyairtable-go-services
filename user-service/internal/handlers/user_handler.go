package handlers

import (
	"fmt"
	"strconv"
	
	"github.com/Reg-Kris/pyairtable-user-service/internal/models"
	"github.com/Reg-Kris/pyairtable-user-service/internal/services"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type UserHandler struct {
	logger      *zap.Logger
	userService *services.UserService
}

func NewUserHandler(logger *zap.Logger, userService *services.UserService) *UserHandler {
	return &UserHandler{
		logger:      logger,
		userService: userService,
	}
}

// GetCurrentUser returns the authenticated user's profile
func (h *UserHandler) GetCurrentUser(c *fiber.Ctx) error {
	userID := c.Get("X-User-ID")
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User ID not found in request",
		})
	}
	
	user, err := h.userService.GetUser(c.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get user", zap.Error(err), zap.String("user_id", userID))
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}
	
	return c.JSON(user)
}

// UpdateCurrentUser updates the authenticated user's profile
func (h *UserHandler) UpdateCurrentUser(c *fiber.Ctx) error {
	userID := c.Get("X-User-ID")
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User ID not found in request",
		})
	}
	
	var req models.UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}
	
	user, err := h.userService.UpdateUser(c.Context(), userID, &req)
	if err != nil {
		h.logger.Error("Failed to update user", zap.Error(err), zap.String("user_id", userID))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update user",
		})
	}
	
	return c.JSON(user)
}

// GetUser returns a user by ID
func (h *UserHandler) GetUser(c *fiber.Ctx) error {
	userID := c.Params("id")
	
	user, err := h.userService.GetUser(c.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get user", zap.Error(err), zap.String("user_id", userID))
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}
	
	// Check if requester has permission to view this user
	requesterID := c.Get("X-User-ID")
	requesterTenantID := c.Get("X-Tenant-ID")
	
	// Users can view their own profile or users in the same tenant
	if requesterID != user.ID && requesterTenantID != user.TenantID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Access denied",
		})
	}
	
	return c.JSON(user)
}

// UpdateUser updates a user by ID
func (h *UserHandler) UpdateUser(c *fiber.Ctx) error {
	userID := c.Params("id")
	
	var req models.UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}
	
	// Check permissions
	requesterTenantID := c.Get("X-Tenant-ID")
	user, err := h.userService.GetUser(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}
	
	if requesterTenantID != user.TenantID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Access denied",
		})
	}
	
	updatedUser, err := h.userService.UpdateUser(c.Context(), userID, &req)
	if err != nil {
		h.logger.Error("Failed to update user", zap.Error(err), zap.String("user_id", userID))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update user",
		})
	}
	
	return c.JSON(updatedUser)
}

// DeleteUser soft deletes a user
func (h *UserHandler) DeleteUser(c *fiber.Ctx) error {
	userID := c.Params("id")
	
	// Check permissions
	requesterTenantID := c.Get("X-Tenant-ID")
	user, err := h.userService.GetUser(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}
	
	if requesterTenantID != user.TenantID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Access denied",
		})
	}
	
	if err := h.userService.DeleteUser(c.Context(), userID); err != nil {
		h.logger.Error("Failed to delete user", zap.Error(err), zap.String("user_id", userID))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete user",
		})
	}
	
	return c.JSON(fiber.Map{
		"message": "User deleted successfully",
	})
}

// ListUsers returns a paginated list of users
func (h *UserHandler) ListUsers(c *fiber.Ctx) error {
	filter := &models.UserFilter{
		TenantID:      c.Query("tenant_id"),
		Role:          c.Query("role"),
		Search:        c.Query("search"),
		SortBy:        c.Query("sort_by", "created_at"),
		SortOrder:     c.Query("sort_order", "desc"),
	}
	
	// Parse boolean filters
	if isActiveStr := c.Query("is_active"); isActiveStr != "" {
		isActive, _ := strconv.ParseBool(isActiveStr)
		filter.IsActive = &isActive
	}
	
	if emailVerifiedStr := c.Query("email_verified"); emailVerifiedStr != "" {
		emailVerified, _ := strconv.ParseBool(emailVerifiedStr)
		filter.EmailVerified = &emailVerified
	}
	
	// Parse pagination
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("page_size", "20"))
	
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	
	filter.Page = page
	filter.PageSize = pageSize
	
	// Filter by requester's tenant unless they have global access
	requesterTenantID := c.Get("X-Tenant-ID")
	if filter.TenantID == "" && requesterTenantID != "" {
		filter.TenantID = requesterTenantID
	}
	
	response, err := h.userService.ListUsers(c.Context(), filter)
	if err != nil {
		h.logger.Error("Failed to list users", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to list users",
		})
	}
	
	return c.JSON(response)
}

// GetUserStats returns user statistics
func (h *UserHandler) GetUserStats(c *fiber.Ctx) error {
	tenantID := c.Query("tenant_id")
	
	// Filter by requester's tenant unless they have global access
	requesterTenantID := c.Get("X-Tenant-ID")
	if tenantID == "" && requesterTenantID != "" {
		tenantID = requesterTenantID
	}
	
	stats, err := h.userService.GetUserStats(c.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to get user stats", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get user statistics",
		})
	}
	
	return c.JSON(stats)
}

// CreateUser creates a new user (usually called by auth service)
func (h *UserHandler) CreateUser(c *fiber.Ctx) error {
	// This endpoint should only be called by internal services
	// Check for internal service token or API key
	apiKey := c.Get("X-API-Key")
	if apiKey == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing API key",
		})
	}
	
	var req models.CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}
	
	user, err := h.userService.CreateUser(c.Context(), &req)
	if err != nil {
		h.logger.Error("Failed to create user", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create user",
		})
	}
	
	return c.Status(fiber.StatusCreated).JSON(user)
}

// BulkCreateUsers creates multiple users
func (h *UserHandler) BulkCreateUsers(c *fiber.Ctx) error {
	// This endpoint should only be called by internal services
	apiKey := c.Get("X-API-Key")
	if apiKey == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing API key",
		})
	}
	
	var users []*models.User
	if err := c.BodyParser(&users); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}
	
	if err := h.userService.BulkCreateUsers(c.Context(), users); err != nil {
		h.logger.Error("Failed to bulk create users", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create users",
		})
	}
	
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": fmt.Sprintf("Successfully created %d users", len(users)),
	})
}