package handlers

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/fiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/go-playground/validator/v10"

	"github.com/pyairtable/go-services/plugin-service/internal/models"
	"github.com/pyairtable/go-services/plugin-service/internal/services"
)

// PluginHandler handles plugin-related HTTP requests
type PluginHandler struct {
	pluginService *services.PluginService
	validator     *validator.Validate
}

// NewPluginHandler creates a new plugin handler
func NewPluginHandler(pluginService *services.PluginService) *PluginHandler {
	return &PluginHandler{
		pluginService: pluginService,
		validator:     validator.New(),
	}
}

// RegisterRoutes registers plugin routes
func (h *PluginHandler) RegisterRoutes(app *fiber.App) {
	v1 := app.Group("/api/v1")
	
	// Plugin management routes
	plugins := v1.Group("/plugins")
	plugins.Get("/", h.ListPlugins)
	plugins.Post("/", h.CreatePlugin)
	plugins.Get("/:plugin_id", h.GetPlugin)
	plugins.Put("/:plugin_id", h.UpdatePlugin)
	plugins.Delete("/:plugin_id", h.DeletePlugin)
	plugins.Post("/:plugin_id/validate", h.ValidatePlugin)
	
	// Plugin installation routes
	installations := v1.Group("/installations")
	installations.Get("/", h.ListInstallations)
	installations.Post("/", h.InstallPlugin)
	installations.Get("/:installation_id", h.GetInstallation)
	installations.Put("/:installation_id", h.UpdateInstallation)
	installations.Delete("/:installation_id", h.UninstallPlugin)
	installations.Post("/:installation_id/execute", h.ExecutePlugin)
	installations.Get("/:installation_id/logs", h.GetExecutionLogs)
	
	// Registry routes
	registry := v1.Group("/registry")
	registry.Get("/", h.SearchRegistry)
	registry.Get("/:plugin_name", h.GetRegistryPlugin)
	registry.Post("/sync", h.SyncRegistry)
	
	// Developer routes
	developers := v1.Group("/developers")
	developers.Get("/", h.ListDevelopers)
	developers.Post("/", h.RegisterDeveloper)
	developers.Get("/:developer_id", h.GetDeveloper)
	developers.Put("/:developer_id", h.UpdateDeveloper)
	
	// Management routes
	admin := v1.Group("/admin")
	admin.Get("/stats", h.GetStats)
	admin.Get("/health", h.HealthCheck)
	admin.Get("/metrics", h.GetMetrics)
	admin.Post("/quarantine/:plugin_id", h.QuarantinePlugin)
	admin.Delete("/quarantine/:plugin_id", h.ReleaseFromQuarantine)
}

// ListPlugins lists plugins with filtering and pagination
func (h *PluginHandler) ListPlugins(c *fiber.Ctx) error {
	req := &models.ListPluginsRequest{
		Page:     1,
		PageSize: 20,
		SortBy:   "created_at",
		SortOrder: "desc",
	}

	// Parse query parameters
	if page := c.Query("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil && p > 0 {
			req.Page = int32(p)
		}
	}
	
	if pageSize := c.Query("page_size"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil && ps > 0 && ps <= 100 {
			req.PageSize = int32(ps)
		}
	}

	if pluginType := c.Query("type"); pluginType != "" {
		pType := models.PluginType(pluginType)
		req.Type = &pType
	}

	if status := c.Query("status"); status != "" {
		pStatus := models.PluginStatus(status)
		req.Status = &pStatus
	}

	if workspaceID := c.Query("workspace_id"); workspaceID != "" {
		if wID, err := uuid.Parse(workspaceID); err == nil {
			req.WorkspaceID = &wID
		}
	}

	if category := c.Query("category"); category != "" {
		req.Category = &category
	}

	if tag := c.Query("tag"); tag != "" {
		req.Tag = &tag
	}

	if search := c.Query("search"); search != "" {
		req.Search = &search
	}

	if sortBy := c.Query("sort_by"); sortBy != "" {
		req.SortBy = sortBy
	}

	if sortOrder := c.Query("sort_order"); sortOrder != "" {
		req.SortOrder = sortOrder
	}

	// Validate request
	if err := h.validator.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{
			Code:      "INVALID_REQUEST",
			Message:   "Invalid request parameters",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	// Get user context
	userID, workspaceID := h.getUserContext(c)

	// List plugins
	response, err := h.pluginService.ListPlugins(c.Context(), req, userID, workspaceID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(response)
}

// CreatePlugin creates a new plugin
func (h *PluginHandler) CreatePlugin(c *fiber.Ctx) error {
	var plugin models.Plugin
	if err := c.BodyParser(&plugin); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{
			Code:      "INVALID_JSON",
			Message:   "Invalid JSON in request body",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	// Validate plugin
	if err := h.validator.Struct(plugin); err != nil {
		return h.handleValidationError(c, err)
	}

	// Get user context
	userID, workspaceID := h.getUserContext(c)
	plugin.DeveloperID = userID
	plugin.WorkspaceID = &workspaceID

	// Create plugin
	createdPlugin, err := h.pluginService.CreatePlugin(c.Context(), &plugin, userID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(createdPlugin)
}

// GetPlugin retrieves a specific plugin
func (h *PluginHandler) GetPlugin(c *fiber.Ctx) error {
	pluginID, err := uuid.Parse(c.Params("plugin_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{
			Code:      "INVALID_UUID",
			Message:   "Invalid plugin ID format",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	userID, workspaceID := h.getUserContext(c)

	plugin, err := h.pluginService.GetPlugin(c.Context(), pluginID, userID, workspaceID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(plugin)
}

// UpdatePlugin updates an existing plugin
func (h *PluginHandler) UpdatePlugin(c *fiber.Ctx) error {
	pluginID, err := uuid.Parse(c.Params("plugin_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{
			Code:      "INVALID_UUID",
			Message:   "Invalid plugin ID format",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	var updates models.Plugin
	if err := c.BodyParser(&updates); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{
			Code:      "INVALID_JSON",
			Message:   "Invalid JSON in request body",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	userID, workspaceID := h.getUserContext(c)

	updatedPlugin, err := h.pluginService.UpdatePlugin(c.Context(), pluginID, &updates, userID, workspaceID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(updatedPlugin)
}

// DeletePlugin deletes a plugin
func (h *PluginHandler) DeletePlugin(c *fiber.Ctx) error {
	pluginID, err := uuid.Parse(c.Params("plugin_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{
			Code:      "INVALID_UUID",
			Message:   "Invalid plugin ID format",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	userID, workspaceID := h.getUserContext(c)

	if err := h.pluginService.DeletePlugin(c.Context(), pluginID, userID, workspaceID); err != nil {
		return h.handleError(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// ValidatePlugin validates a plugin
func (h *PluginHandler) ValidatePlugin(c *fiber.Ctx) error {
	pluginID, err := uuid.Parse(c.Params("plugin_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{
			Code:      "INVALID_UUID",
			Message:   "Invalid plugin ID format",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	userID, workspaceID := h.getUserContext(c)

	result, err := h.pluginService.ValidatePlugin(c.Context(), pluginID, userID, workspaceID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(result)
}

// InstallPlugin installs a plugin
func (h *PluginHandler) InstallPlugin(c *fiber.Ctx) error {
	var req struct {
		PluginID    uuid.UUID       `json:"plugin_id" validate:"required"`
		Version     string          `json:"version" validate:"required"`
		Config      json.RawMessage `json:"config,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{
			Code:      "INVALID_JSON",
			Message:   "Invalid JSON in request body",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	if err := h.validator.Struct(req); err != nil {
		return h.handleValidationError(c, err)
	}

	userID, workspaceID := h.getUserContext(c)

	installation, err := h.pluginService.InstallPlugin(c.Context(), req.PluginID, req.Version, req.Config, userID, workspaceID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(installation)
}

// ExecutePlugin executes a plugin function
func (h *PluginHandler) ExecutePlugin(c *fiber.Ctx) error {
	installationID, err := uuid.Parse(c.Params("installation_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{
			Code:      "INVALID_UUID",
			Message:   "Invalid installation ID format",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	var req struct {
		Function string          `json:"function" validate:"required"`
		Input    json.RawMessage `json:"input,omitempty"`
		Metadata map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{
			Code:      "INVALID_JSON",
			Message:   "Invalid JSON in request body",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	if err := h.validator.Struct(req); err != nil {
		return h.handleValidationError(c, err)
	}

	userID, workspaceID := h.getUserContext(c)

	result, err := h.pluginService.ExecutePlugin(c.Context(), installationID, req.Function, req.Input, req.Metadata, userID, workspaceID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(result)
}

// GetStats returns plugin service statistics
func (h *PluginHandler) GetStats(c *fiber.Ctx) error {
	userID, workspaceID := h.getUserContext(c)

	stats, err := h.pluginService.GetStats(c.Context(), userID, workspaceID)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(stats)
}

// HealthCheck returns service health status
func (h *PluginHandler) HealthCheck(c *fiber.Ctx) error {
	health := h.pluginService.HealthCheck(c.Context())
	
	status := fiber.StatusOK
	if health.Status != "healthy" {
		status = fiber.StatusServiceUnavailable
	}

	return c.Status(status).JSON(health)
}

// GetMetrics returns service metrics
func (h *PluginHandler) GetMetrics(c *fiber.Ctx) error {
	// This would typically be handled by Prometheus middleware
	// but we can provide additional plugin-specific metrics here
	metrics, err := h.pluginService.GetMetrics(c.Context())
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(metrics)
}

// QuarantinePlugin quarantines a plugin
func (h *PluginHandler) QuarantinePlugin(c *fiber.Ctx) error {
	pluginID, err := uuid.Parse(c.Params("plugin_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{
			Code:      "INVALID_UUID",
			Message:   "Invalid plugin ID format",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	var req struct {
		Reason   string                 `json:"reason" validate:"required"`
		Metadata map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{
			Code:      "INVALID_JSON",
			Message:   "Invalid JSON in request body",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	if err := h.validator.Struct(req); err != nil {
		return h.handleValidationError(c, err)
	}

	userID, _ := h.getUserContext(c)

	if err := h.pluginService.QuarantinePlugin(c.Context(), pluginID, req.Reason, req.Metadata, userID); err != nil {
		return h.handleError(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// Helper methods

// getUserContext extracts user and workspace context from request
func (h *PluginHandler) getUserContext(c *fiber.Ctx) (userID, workspaceID uuid.UUID) {
	// Extract from JWT token or headers
	if userIDStr := c.Get("X-User-ID"); userIDStr != "" {
		if uid, err := uuid.Parse(userIDStr); err == nil {
			userID = uid
		}
	}
	
	if workspaceIDStr := c.Get("X-Workspace-ID"); workspaceIDStr != "" {
		if wid, err := uuid.Parse(workspaceIDStr); err == nil {
			workspaceID = wid
		}
	}

	return userID, workspaceID
}

// handleError handles service errors and returns appropriate HTTP responses
func (h *PluginHandler) handleError(c *fiber.Ctx, err error) error {
	// Map service errors to HTTP status codes
	switch err {
	case services.ErrPluginNotFound:
		return c.Status(fiber.StatusNotFound).JSON(models.APIError{
			Code:      "PLUGIN_NOT_FOUND",
			Message:   "Plugin not found",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	case services.ErrInstallationNotFound:
		return c.Status(fiber.StatusNotFound).JSON(models.APIError{
			Code:      "INSTALLATION_NOT_FOUND",
			Message:   "Plugin installation not found",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	case services.ErrUnauthorized:
		return c.Status(fiber.StatusUnauthorized).JSON(models.APIError{
			Code:      "UNAUTHORIZED",
			Message:   "Unauthorized access",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	case services.ErrForbidden:
		return c.Status(fiber.StatusForbidden).JSON(models.APIError{
			Code:      "FORBIDDEN",
			Message:   "Access forbidden",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	case services.ErrValidationFailed:
		return c.Status(fiber.StatusBadRequest).JSON(models.APIError{
			Code:      "VALIDATION_FAILED",
			Message:   "Validation failed",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(models.APIError{
			Code:      "INTERNAL_ERROR",
			Message:   "Internal server error",
			Details:   err.Error(),
			Timestamp: time.Now(),
		})
	}
}

// handleValidationError handles validation errors
func (h *PluginHandler) handleValidationError(c *fiber.Ctx, err error) error {
	var validationErrors []models.ValidationError
	
	if validationErrs, ok := err.(validator.ValidationErrors); ok {
		for _, validationErr := range validationErrs {
			validationErrors = append(validationErrors, models.ValidationError{
				Field:   validationErr.Field(),
				Message: validationErr.Tag(),
				Code:    validationErr.Tag(),
			})
		}
	}

	return c.Status(fiber.StatusBadRequest).JSON(models.APIError{
		Code:       "VALIDATION_ERROR",
		Message:    "Request validation failed",
		Details:    err.Error(),
		Validation: validationErrors,
		Timestamp:  time.Now(),
	})
}

// Additional handler methods for registry, installations, etc. would follow similar patterns...