package handlers

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/Reg-Kris/pyairtable-permission-service/internal/models"
	"github.com/Reg-Kris/pyairtable-permission-service/internal/services"
	"go.uber.org/zap"
)

// PermissionHandler handles permission-related endpoints
type PermissionHandler struct {
	logger            *zap.Logger
	permissionService *services.PermissionService
	roleService       *services.RoleService
	auditService      *services.AuditService
}

// NewPermissionHandler creates a new permission handler
func NewPermissionHandler(
	logger *zap.Logger,
	permissionService *services.PermissionService,
	roleService *services.RoleService,
	auditService *services.AuditService,
) *PermissionHandler {
	return &PermissionHandler{
		logger:            logger,
		permissionService: permissionService,
		roleService:       roleService,
		auditService:      auditService,
	}
}

// Ping handles health check requests
func (h *PermissionHandler) Ping(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":    "ok",
		"service":   "permission-service",
		"timestamp": time.Now(),
	})
}

// CheckPermission handles POST /api/v1/permissions/check
func (h *PermissionHandler) CheckPermission(c *fiber.Ctx) error {
	var req models.CheckPermissionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate tenant access
	tenantID := c.Locals("tenantID").(string)
	if req.TenantID != tenantID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Access denied to this tenant",
		})
	}

	allowed, reason, err := h.permissionService.CheckPermission(c.Context(), &req)
	if err != nil {
		h.logger.Error("Permission check failed", zap.Error(err), zap.Any("request", req))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Permission check failed",
		})
	}

	response := models.CheckPermissionResponse{
		Allowed: allowed,
		Reason:  reason,
	}

	return c.JSON(response)
}

// BatchCheckPermissions handles POST /api/v1/permissions/batch-check
func (h *PermissionHandler) BatchCheckPermissions(c *fiber.Ctx) error {
	var req models.BatchCheckPermissionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate tenant access
	tenantID := c.Locals("tenantID").(string)
	if req.TenantID != tenantID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Access denied to this tenant",
		})
	}

	results, err := h.permissionService.BatchCheckPermissions(c.Context(), &req)
	if err != nil {
		h.logger.Error("Batch permission check failed", zap.Error(err), zap.Any("request", req))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Batch permission check failed",
		})
	}

	return c.JSON(models.BatchCheckPermissionResponse{
		Results: results,
	})
}

// GetUserPermissions handles GET /api/v1/users/:userId/permissions
func (h *PermissionHandler) GetUserPermissions(c *fiber.Ctx) error {
	userID := c.Params("userId")
	if userID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "User ID is required",
		})
	}

	tenantID := c.Locals("tenantID").(string)
	resourceType := c.Query("resource_type")
	resourceID := c.Query("resource_id")

	// Check if requesting user can view this user's permissions
	requesterID := c.Locals("userID").(string)
	canView, _, err := h.permissionService.CheckPermission(c.Context(), &models.CheckPermissionRequest{
		UserID:       requesterID,
		ResourceType: models.ResourceTypeUser,
		ResourceID:   userID,
		Action:       models.ActionView,
		TenantID:     tenantID,
	})
	if err != nil || !canView {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Access denied",
		})
	}

	permissions, err := h.permissionService.GetUserPermissions(c.Context(), userID, tenantID, resourceType, resourceID)
	if err != nil {
		h.logger.Error("Failed to get user permissions", zap.Error(err), zap.String("user_id", userID))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get user permissions",
		})
	}

	return c.JSON(permissions)
}

// GrantPermission handles POST /api/v1/permissions/grant
func (h *PermissionHandler) GrantPermission(c *fiber.Ctx) error {
	var req models.GrantPermissionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	tenantID := c.Locals("tenantID").(string)
	grantedBy := c.Locals("userID").(string)

	// Check if requesting user can grant permissions on this resource
	canGrant, _, err := h.permissionService.CheckPermission(c.Context(), &models.CheckPermissionRequest{
		UserID:       grantedBy,
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		Action:       models.ActionManage,
		TenantID:     tenantID,
	})
	if err != nil || !canGrant {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Access denied - insufficient permissions to grant access",
		})
	}

	permission, err := h.permissionService.GrantPermission(c.Context(), &req, tenantID, grantedBy)
	if err != nil {
		h.logger.Error("Failed to grant permission", zap.Error(err), zap.Any("request", req))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to grant permission",
		})
	}

	// Log audit event
	h.auditService.LogPermissionChange(c.Context(), &models.PermissionAuditLog{
		TenantID:     tenantID,
		UserID:       grantedBy,
		TargetUserID: req.UserID,
		Action:       "grant_permission",
		ResourceType: string(req.ResourceType),
		ResourceID:   req.ResourceID,
		Changes: models.JSONMap{
			"action":     req.Action,
			"permission": req.Permission,
			"reason":     req.Reason,
		},
		IPAddress: c.IP(),
		UserAgent: c.Get("User-Agent"),
	})

	return c.Status(fiber.StatusCreated).JSON(permission)
}

// RevokePermission handles DELETE /api/v1/permissions/:permissionId
func (h *PermissionHandler) RevokePermission(c *fiber.Ctx) error {
	permissionID := c.Params("permissionId")
	if permissionID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Permission ID is required",
		})
	}

	tenantID := c.Locals("tenantID").(string)
	revokedBy := c.Locals("userID").(string)

	// Get permission details first
	permission, err := h.permissionService.GetResourcePermission(c.Context(), permissionID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Permission not found",
		})
	}

	// Check if requesting user can revoke permissions on this resource
	canRevoke, _, err := h.permissionService.CheckPermission(c.Context(), &models.CheckPermissionRequest{
		UserID:       revokedBy,
		ResourceType: permission.ResourceType,
		ResourceID:   permission.ResourceID,
		Action:       models.ActionManage,
		TenantID:     tenantID,
	})
	if err != nil || !canRevoke {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Access denied - insufficient permissions to revoke access",
		})
	}

	if err := h.permissionService.RevokePermission(c.Context(), permissionID); err != nil {
		h.logger.Error("Failed to revoke permission", zap.Error(err), zap.String("permission_id", permissionID))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to revoke permission",
		})
	}

	// Log audit event
	h.auditService.LogPermissionChange(c.Context(), &models.PermissionAuditLog{
		TenantID:     tenantID,
		UserID:       revokedBy,
		TargetUserID: permission.UserID,
		Action:       "revoke_permission",
		ResourceType: string(permission.ResourceType),
		ResourceID:   permission.ResourceID,
		Changes: models.JSONMap{
			"permission_id": permissionID,
			"action":        permission.Action,
		},
		IPAddress: c.IP(),
		UserAgent: c.Get("User-Agent"),
	})

	return c.JSON(fiber.Map{
		"message": "Permission revoked successfully",
	})
}

// Role Management Endpoints

// GetRoles handles GET /api/v1/roles
func (h *PermissionHandler) GetRoles(c *fiber.Ctx) error {
	tenantID := c.Locals("tenantID").(string)
	requesterID := c.Locals("userID").(string)

	// Check if user can list roles
	canList, _, err := h.permissionService.CheckPermission(c.Context(), &models.CheckPermissionRequest{
		UserID:       requesterID,
		ResourceType: models.ResourceTypeAPI,
		ResourceID:   "roles",
		Action:       models.ActionList,
		TenantID:     tenantID,
	})
	if err != nil || !canList {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Access denied",
		})
	}

	// Parse query parameters
	filter := models.RoleFilter{
		TenantID:  tenantID,
		Search:    c.Query("search"),
		Page:      1,
		PageSize:  20,
		SortBy:    "name",
		SortOrder: "asc",
	}

	if page := c.Query("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil {
			filter.Page = p
		}
	}

	if pageSize := c.Query("page_size"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil && ps <= 100 {
			filter.PageSize = ps
		}
	}

	if isActive := c.Query("is_active"); isActive != "" {
		if active, err := strconv.ParseBool(isActive); err == nil {
			filter.IsActive = &active
		}
	}

	response, err := h.roleService.GetRoles(c.Context(), &filter)
	if err != nil {
		h.logger.Error("Failed to get roles", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get roles",
		})
	}

	return c.JSON(response)
}

// CreateRole handles POST /api/v1/roles
func (h *PermissionHandler) CreateRole(c *fiber.Ctx) error {
	var req models.CreateRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	tenantID := c.Locals("tenantID").(string)
	createdBy := c.Locals("userID").(string)

	// Check if user can create roles
	canCreate, _, err := h.permissionService.CheckPermission(c.Context(), &models.CheckPermissionRequest{
		UserID:       createdBy,
		ResourceType: models.ResourceTypeAPI,
		ResourceID:   "roles",
		Action:       models.ActionCreate,
		TenantID:     tenantID,
	})
	if err != nil || !canCreate {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Access denied",
		})
	}

	role, err := h.roleService.CreateRole(c.Context(), &req, tenantID)
	if err != nil {
		h.logger.Error("Failed to create role", zap.Error(err), zap.Any("request", req))
		if strings.Contains(err.Error(), "already exists") {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Role with this name already exists",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create role",
		})
	}

	// Log audit event
	h.auditService.LogPermissionChange(c.Context(), &models.PermissionAuditLog{
		TenantID:     tenantID,
		UserID:       createdBy,
		TargetUserID: "",
		Action:       "create_role",
		ResourceType: "role",
		ResourceID:   role.ID,
		Changes: models.JSONMap{
			"name":        role.Name,
			"description": role.Description,
			"permissions": req.Permissions,
		},
		IPAddress: c.IP(),
		UserAgent: c.Get("User-Agent"),
	})

	return c.Status(fiber.StatusCreated).JSON(role)
}

// AssignRole handles POST /api/v1/roles/assign
func (h *PermissionHandler) AssignRole(c *fiber.Ctx) error {
	var req models.AssignRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	tenantID := c.Locals("tenantID").(string)
	grantedBy := c.Locals("userID").(string)

	// Determine what resource we're checking permissions for
	resourceType := models.ResourceTypeUser
	resourceID := req.UserID
	if req.ResourceType != nil && req.ResourceID != nil {
		resourceType = models.ResourceType(*req.ResourceType)
		resourceID = *req.ResourceID
	}

	// Check if requesting user can assign roles for this resource
	canAssign, _, err := h.permissionService.CheckPermission(c.Context(), &models.CheckPermissionRequest{
		UserID:       grantedBy,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       models.ActionManage,
		TenantID:     tenantID,
	})
	if err != nil || !canAssign {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Access denied - insufficient permissions to assign roles",
		})
	}

	userRole, err := h.roleService.AssignRole(c.Context(), &req, tenantID, grantedBy)
	if err != nil {
		h.logger.Error("Failed to assign role", zap.Error(err), zap.Any("request", req))
		if strings.Contains(err.Error(), "already assigned") {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Role already assigned to user",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to assign role",
		})
	}

	// Log audit event
	h.auditService.LogPermissionChange(c.Context(), &models.PermissionAuditLog{
		TenantID:     tenantID,
		UserID:       grantedBy,
		TargetUserID: req.UserID,
		Action:       "assign_role",
		ResourceType: string(resourceType),
		ResourceID:   resourceID,
		Changes: models.JSONMap{
			"role_id":       req.RoleID,
			"resource_type": req.ResourceType,
			"resource_id":   req.ResourceID,
			"expires_at":    req.ExpiresAt,
		},
		IPAddress: c.IP(),
		UserAgent: c.Get("User-Agent"),
	})

	return c.Status(fiber.StatusCreated).JSON(userRole)
}

// GetAuditLogs handles GET /api/v1/audit-logs
func (h *PermissionHandler) GetAuditLogs(c *fiber.Ctx) error {
	tenantID := c.Locals("tenantID").(string)
	requesterID := c.Locals("userID").(string)

	// Check if user can view audit logs
	canView, _, err := h.permissionService.CheckPermission(c.Context(), &models.CheckPermissionRequest{
		UserID:       requesterID,
		ResourceType: models.ResourceTypeAPI,
		ResourceID:   "audit-logs",
		Action:       models.ActionRead,
		TenantID:     tenantID,
	})
	if err != nil || !canView {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Access denied",
		})
	}

	// Parse query parameters
	filter := models.AuditLogFilter{
		TenantID:     tenantID,
		UserID:       c.Query("user_id"),
		TargetUserID: c.Query("target_user_id"),
		Action:       c.Query("action"),
		ResourceType: c.Query("resource_type"),
		ResourceID:   c.Query("resource_id"),
		Page:         1,
		PageSize:     20,
		SortBy:       "created_at",
		SortOrder:    "desc",
	}

	if page := c.Query("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil {
			filter.Page = p
		}
	}

	if pageSize := c.Query("page_size"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil && ps <= 100 {
			filter.PageSize = ps
		}
	}

	response, err := h.auditService.GetAuditLogs(c.Context(), &filter)
	if err != nil {
		h.logger.Error("Failed to get audit logs", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get audit logs",
		})
	}

	return c.JSON(response)
}