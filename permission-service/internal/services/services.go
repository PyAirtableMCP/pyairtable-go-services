package services

import (
	"context"
	"fmt"
	"time"

	"github.com/Reg-Kris/pyairtable-permission-service/internal/models"
	"github.com/Reg-Kris/pyairtable-permission-service/internal/repositories"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// PermissionService handles permission checks and management
type PermissionService struct {
	logger     *zap.Logger
	permRepo   repositories.PermissionRepository
	roleRepo   repositories.RoleRepository
	userRoleRepo repositories.UserRoleRepository
	resPermRepo repositories.ResourcePermissionRepository
	cacheRepo  repositories.CacheRepository
}

// NewPermissionService creates a new permission service
func NewPermissionService(
	logger *zap.Logger,
	permRepo repositories.PermissionRepository,
	roleRepo repositories.RoleRepository,
	userRoleRepo repositories.UserRoleRepository,
	resPermRepo repositories.ResourcePermissionRepository,
	cacheRepo repositories.CacheRepository,
) *PermissionService {
	return &PermissionService{
		logger:      logger,
		permRepo:    permRepo,
		roleRepo:    roleRepo,
		userRoleRepo: userRoleRepo,
		resPermRepo: resPermRepo,
		cacheRepo:   cacheRepo,
	}
}

// CheckPermission checks if a user has permission to perform an action on a resource
func (s *PermissionService) CheckPermission(ctx context.Context, req *models.CheckPermissionRequest) (bool, string, error) {
	// Create cache key for this permission check
	cacheKey := fmt.Sprintf("perm:%s:%s:%s:%s:%s", req.TenantID, req.UserID, req.ResourceType, req.ResourceID, req.Action)
	
	// Try to get from cache first
	if result, err := s.cacheRepo.Get(ctx, cacheKey); err == nil {
		if result == "allow" {
			return true, "cached allow", nil
		} else if result == "deny" {
			return false, "cached deny", nil
		}
	}

	// 1. Check direct resource permissions first (most specific)
	allowed, reason := s.checkDirectPermission(ctx, req)
	if allowed != nil {
		// Cache the result
		result := "deny"
		if *allowed {
			result = "allow"
		}
		s.cacheRepo.Set(ctx, cacheKey, result, 5*time.Minute)
		return *allowed, reason, nil
	}

	// 2. Check role-based permissions (hierarchical)
	allowed, reason = s.checkRoleBasedPermission(ctx, req)
	if allowed != nil {
		// Cache the result
		result := "deny"
		if *allowed {
			result = "allow"
		}
		s.cacheRepo.Set(ctx, cacheKey, result, 5*time.Minute)
		return *allowed, reason, nil
	}

	// 3. Default deny
	s.cacheRepo.Set(ctx, cacheKey, "deny", 5*time.Minute)
	return false, "no matching permissions found", nil
}

// checkDirectPermission checks explicit resource permissions
func (s *PermissionService) checkDirectPermission(ctx context.Context, req *models.CheckPermissionRequest) (*bool, string) {
	permissions, err := s.resPermRepo.GetUserResourcePermissions(ctx, req.UserID, req.TenantID, string(req.ResourceType), req.ResourceID)
	if err != nil {
		s.logger.Error("Failed to get resource permissions", zap.Error(err))
		return nil, ""
	}

	for _, perm := range permissions {
		if perm.Action == req.Action {
			// Check if permission is still valid
		if perm.ExpiresAt != nil && perm.ExpiresAt.Before(time.Now()) {
				continue
			}
			
			if perm.Permission == "allow" {
				allowed := true
				return &allowed, "direct permission: allow"
			} else if perm.Permission == "deny" {
				allowed := false
				return &allowed, "direct permission: deny"
			}
		}
	}

	return nil, ""
}

// checkRoleBasedPermission checks permissions through role assignments
func (s *PermissionService) checkRoleBasedPermission(ctx context.Context, req *models.CheckPermissionRequest) (*bool, string) {
	// Get user roles for this tenant and resource context
	userRoles, err := s.getUserEffectiveRoles(ctx, req.UserID, req.TenantID, string(req.ResourceType), req.ResourceID)
	if err != nil {
		s.logger.Error("Failed to get user roles", zap.Error(err))
		return nil, ""
	}

	// Check each role for the required permission
	for _, userRole := range userRoles {
		// Check if role assignment is still valid
		if userRole.ExpiresAt != nil && userRole.ExpiresAt.Before(time.Now()) {
			continue
		}
		
		if !userRole.IsActive {
			continue
		}

		// Get role permissions
		role, err := s.roleRepo.GetByID(ctx, userRole.RoleID)
		if err != nil || !role.IsActive {
			continue
		}

		// Check if role has the required permission
		for _, permission := range role.Permissions {
			if permission.ResourceType == req.ResourceType && permission.Action == req.Action && permission.IsActive {
				allowed := true
				return &allowed, fmt.Sprintf("role permission: %s", role.Name)
			}
		}
	}

	return nil, ""
}

// getUserEffectiveRoles gets all effective roles for a user in a given context
func (s *PermissionService) getUserEffectiveRoles(ctx context.Context, userID, tenantID, resourceType, resourceID string) ([]*models.UserRole, error) {
	allRoles := []*models.UserRole{}

	// 1. Get global tenant roles (no resource specified)
	globalRoles, err := s.userRoleRepo.GetUserRoles(ctx, userID, tenantID, "", "")
	if err != nil {
		return nil, err
	}
	allRoles = append(allRoles, globalRoles...)

	// 2. Get resource-type specific roles
	if resourceType != "" {
		typeRoles, err := s.userRoleRepo.GetUserRoles(ctx, userID, tenantID, resourceType, "")
		if err != nil {
			return nil, err
		}
		allRoles = append(allRoles, typeRoles...)
	}

	// 3. Get resource-specific roles
	if resourceType != "" && resourceID != "" {
		resourceRoles, err := s.userRoleRepo.GetUserRoles(ctx, userID, tenantID, resourceType, resourceID)
		if err != nil {
			return nil, err
		}
		allRoles = append(allRoles, resourceRoles...)
	}

	// 4. Check hierarchical inheritance (workspace -> project -> base)
	if resourceType != "" && resourceID != "" {
		hierarchicalRoles, err := s.getHierarchicalRoles(ctx, userID, tenantID, resourceType, resourceID)
		if err != nil {
			s.logger.Warn("Failed to get hierarchical roles", zap.Error(err))
		} else {
			allRoles = append(allRoles, hierarchicalRoles...)
		}
	}

	return allRoles, nil
}

// getHierarchicalRoles implements permission inheritance (workspace -> project -> base)
func (s *PermissionService) getHierarchicalRoles(ctx context.Context, userID, tenantID, resourceType, resourceID string) ([]*models.UserRole, error) {
	roles := []*models.UserRole{}

	switch models.ResourceType(resourceType) {
	case models.ResourceTypeProject:
		// For projects, check workspace permissions
		// This would require workspace service integration to get workspace ID
		// For now, we'll skip this implementation
		return roles, nil
		
	case models.ResourceTypeAirtableBase:
		// For Airtable bases, check project and workspace permissions
		// This would require project service integration
		// For now, we'll skip this implementation
		return roles, nil
		
	default:
		return roles, nil
	}
}

// BatchCheckPermissions performs multiple permission checks efficiently
func (s *PermissionService) BatchCheckPermissions(ctx context.Context, req *models.BatchCheckPermissionRequest) (map[string]models.CheckPermissionResponse, error) {
	results := make(map[string]models.CheckPermissionResponse)

	for i, check := range req.Checks {
		key := check.Key
		if key == "" {
			key = fmt.Sprintf("check_%d", i)
		}

		permReq := &models.CheckPermissionRequest{
			UserID:       req.UserID,
			ResourceType: check.ResourceType,
			ResourceID:   check.ResourceID,
			Action:       check.Action,
			TenantID:     req.TenantID,
		}

		allowed, reason, err := s.CheckPermission(ctx, permReq)
		if err != nil {
			s.logger.Error("Batch permission check failed", zap.Error(err), zap.String("key", key))
			results[key] = models.CheckPermissionResponse{
				Allowed: false,
				Reason:  "check failed: " + err.Error(),
			}
		} else {
			results[key] = models.CheckPermissionResponse{
				Allowed: allowed,
				Reason:  reason,
			}
		}
	}

	return results, nil
}

// GetUserPermissions retrieves all permissions for a user
func (s *PermissionService) GetUserPermissions(ctx context.Context, userID, tenantID, resourceType, resourceID string) (*models.GetUserPermissionsResponse, error) {
	// Get user roles
	userRoles, err := s.getUserEffectiveRoles(ctx, userID, tenantID, resourceType, resourceID)
	if err != nil {
		return nil, err
	}

	// Get direct permissions
	directPermissions, err := s.resPermRepo.GetUserResourcePermissions(ctx, userID, tenantID, resourceType, resourceID)
	if err != nil {
		return nil, err
	}

	// Build effective permissions map
	effectivePerms := make(map[string][]string)
	
	// Add permissions from roles
	for _, userRole := range userRoles {
		role, err := s.roleRepo.GetByID(ctx, userRole.RoleID)
		if err != nil || !role.IsActive {
			continue
		}

		for _, perm := range role.Permissions {
			if perm.IsActive {
				resource := fmt.Sprintf("%s:%s", perm.ResourceType, "*")
				if userRole.ResourceType != nil && userRole.ResourceID != nil {
					resource = fmt.Sprintf("%s:%s", *userRole.ResourceType, *userRole.ResourceID)
				}
				effectivePerms[resource] = append(effectivePerms[resource], string(perm.Action))
			}
		}
	}

	// Add direct permissions
	for _, perm := range directPermissions {
		if perm.Permission == "allow" {
			resource := fmt.Sprintf("%s:%s", perm.ResourceType, perm.ResourceID)
			effectivePerms[resource] = append(effectivePerms[resource], string(perm.Action))
		}
	}

	return &models.GetUserPermissionsResponse{
		UserID:               userID,
		TenantID:             tenantID,
		Roles:                userRoles,
		DirectPermissions:    directPermissions,
		EffectivePermissions: effectivePerms,
	}, nil
}

// GrantPermission grants a direct permission to a user
func (s *PermissionService) GrantPermission(ctx context.Context, req *models.GrantPermissionRequest, tenantID, grantedBy string) (*models.ResourcePermission, error) {
	permission := &models.ResourcePermission{
		UserID:       req.UserID,
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		Action:       req.Action,
		Permission:   req.Permission,
		TenantID:     tenantID,
		GrantedBy:    grantedBy,
		GrantedAt:    time.Now(),
		ExpiresAt:    req.ExpiresAt,
		Reason:       req.Reason,
	}

	if err := s.resPermRepo.Create(ctx, permission); err != nil {
		return nil, err
	}

	// Invalidate cache for this user's permissions
	s.invalidateUserPermissionCache(ctx, req.UserID, tenantID)

	return permission, nil
}

// RevokePermission revokes a direct permission
func (s *PermissionService) RevokePermission(ctx context.Context, permissionID string) error {
	permission, err := s.resPermRepo.GetByID(ctx, permissionID)
	if err != nil {
		return err
	}

	if err := s.resPermRepo.Delete(ctx, permissionID); err != nil {
		return err
	}

	// Invalidate cache for this user's permissions
	s.invalidateUserPermissionCache(ctx, permission.UserID, permission.TenantID)

	return nil
}

// GetResourcePermission gets a resource permission by ID
func (s *PermissionService) GetResourcePermission(ctx context.Context, permissionID string) (*models.ResourcePermission, error) {
	return s.resPermRepo.GetByID(ctx, permissionID)
}

// invalidateUserPermissionCache clears cache for user permissions
func (s *PermissionService) invalidateUserPermissionCache(ctx context.Context, userID, tenantID string) {
	// Clear all cached permissions for this user
	pattern := fmt.Sprintf("perm:%s:%s:*", tenantID, userID)
	if err := s.cacheRepo.DeletePattern(ctx, pattern); err != nil {
		s.logger.Warn("Failed to invalidate permission cache", zap.Error(err), zap.String("pattern", pattern))
	}
}

// RoleService handles role management
type RoleService struct {
	logger       *zap.Logger
	roleRepo     repositories.RoleRepository
	permRepo     repositories.PermissionRepository
	userRoleRepo repositories.UserRoleRepository
	cacheRepo    repositories.CacheRepository
}

// NewRoleService creates a new role service
func NewRoleService(
	logger *zap.Logger,
	roleRepo repositories.RoleRepository,
	permRepo repositories.PermissionRepository,
	userRoleRepo repositories.UserRoleRepository,
	cacheRepo repositories.CacheRepository,
) *RoleService {
	return &RoleService{
		logger:       logger,
		roleRepo:     roleRepo,
		permRepo:     permRepo,
		userRoleRepo: userRoleRepo,
		cacheRepo:    cacheRepo,
	}
}

// GetRoles retrieves roles with filtering
func (s *RoleService) GetRoles(ctx context.Context, filter *models.RoleFilter) (*models.RoleListResponse, error) {
	roles, total, err := s.roleRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	totalPages := int((total + int64(filter.PageSize) - 1) / int64(filter.PageSize))

	return &models.RoleListResponse{
		Roles:      roles,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

// CreateRole creates a new role
func (s *RoleService) CreateRole(ctx context.Context, req *models.CreateRoleRequest, tenantID string) (*models.Role, error) {
	// Check if role name already exists in this tenant
	existing, err := s.roleRepo.GetByName(ctx, req.Name, tenantID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("role with name '%s' already exists", req.Name)
	}

	role := &models.Role{
		Name:         req.Name,
		Description:  req.Description,
		IsSystemRole: false,
		IsActive:     true,
		TenantID:     &tenantID,
	}

	// Create role
	if err := s.roleRepo.Create(ctx, role); err != nil {
		return nil, err
	}

	// Assign permissions if provided
	if len(req.Permissions) > 0 {
		permissions, err := s.permRepo.GetByNames(ctx, req.Permissions)
		if err != nil {
			s.logger.Error("Failed to get permissions for role", zap.Error(err))
			return role, nil // Return role but log the error
		}
		
		if err := s.roleRepo.AssignPermissions(ctx, role.ID, permissions); err != nil {
			s.logger.Error("Failed to assign permissions to role", zap.Error(err))
		}
	}

	return role, nil
}

// AssignRole assigns a role to a user
func (s *RoleService) AssignRole(ctx context.Context, req *models.AssignRoleRequest, tenantID, grantedBy string) (*models.UserRole, error) {
	// Check if role exists and is active
	role, err := s.roleRepo.GetByID(ctx, req.RoleID)
	if err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}
	if !role.IsActive {
		return nil, fmt.Errorf("role is not active")
	}

	// Check if assignment already exists
	existing, err := s.userRoleRepo.GetUserRole(ctx, req.UserID, req.RoleID, tenantID, req.ResourceType, req.ResourceID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("role already assigned to user")
	}

	userRole := &models.UserRole{
		UserID:       req.UserID,
		RoleID:       req.RoleID,
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		TenantID:     tenantID,
		GrantedBy:    grantedBy,
		GrantedAt:    time.Now(),
		ExpiresAt:    req.ExpiresAt,
		IsActive:     true,
	}

	if err := s.userRoleRepo.Create(ctx, userRole); err != nil {
		return nil, err
	}

	// Invalidate user's permission cache
	pattern := fmt.Sprintf("perm:%s:%s:*", tenantID, req.UserID)
	s.cacheRepo.DeletePattern(ctx, pattern)

	return userRole, nil
}

// AuditService handles audit logging
type AuditService struct {
	logger    *zap.Logger
	auditRepo repositories.AuditRepository
}

// NewAuditService creates a new audit service
func NewAuditService(logger *zap.Logger, auditRepo repositories.AuditRepository) *AuditService {
	return &AuditService{
		logger:    logger,
		auditRepo: auditRepo,
	}
}

// LogPermissionChange logs a permission change event
func (s *AuditService) LogPermissionChange(ctx context.Context, log *models.PermissionAuditLog) error {
	return s.auditRepo.Create(ctx, log)
}

// GetAuditLogs retrieves audit logs with filtering
func (s *AuditService) GetAuditLogs(ctx context.Context, filter *models.AuditLogFilter) (*models.AuditLogListResponse, error) {
	logs, total, err := s.auditRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	totalPages := int((total + int64(filter.PageSize) - 1) / int64(filter.PageSize))

	return &models.AuditLogListResponse{
		Logs:       logs,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}