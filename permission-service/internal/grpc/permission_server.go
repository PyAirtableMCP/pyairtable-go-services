package grpc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	tracecodes "go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	permissionv1 "github.com/pyairtable/pyairtable-protos/generated/go/pyairtable/permission/v1"
	commonv1 "github.com/pyairtable/pyairtable-protos/generated/go/pyairtable/common/v1"
	"github.com/pyairtable/pyairtable-permission-service/internal/services"
)

// PermissionServer implements the gRPC PermissionService
type PermissionServer struct {
	permissionv1.UnimplementedPermissionServiceServer
	logger           *zap.Logger
	permissionSvc    *services.PermissionService
	roleSvc          *services.RoleService
	auditSvc         *services.AuditService
	tracer           trace.Tracer
	requestCounter   metric.Int64Counter
	requestDuration  metric.Float64Histogram
	errorCounter     metric.Int64Counter
}

// NewPermissionServer creates a new gRPC permission server
func NewPermissionServer(
	logger *zap.Logger,
	permissionSvc *services.PermissionService,
	roleSvc *services.RoleService,
	auditSvc *services.AuditService,
	meter metric.Meter,
) (*PermissionServer, error) {
	tracer := otel.Tracer("permission-service")

	// Create metrics
	requestCounter, err := meter.Int64Counter(
		"grpc_requests_total",
		metric.WithDescription("Total number of gRPC requests"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request counter: %w", err)
	}

	requestDuration, err := meter.Float64Histogram(
		"grpc_request_duration_seconds",
		metric.WithDescription("Duration of gRPC requests in seconds"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request duration histogram: %w", err)
	}

	errorCounter, err := meter.Int64Counter(
		"grpc_errors_total",
		metric.WithDescription("Total number of gRPC errors"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create error counter: %w", err)
	}

	return &PermissionServer{
		logger:          logger,
		permissionSvc:   permissionSvc,
		roleSvc:         roleSvc,
		auditSvc:        auditSvc,
		tracer:          tracer,
		requestCounter:  requestCounter,
		requestDuration: requestDuration,
		errorCounter:    errorCounter,
	}, nil
}

// instrumentRequest adds observability to gRPC requests
func (s *PermissionServer) instrumentRequest(ctx context.Context, method string, fn func(context.Context) error) error {
	start := time.Now()
	
	// Start tracing span
	ctx, span := s.tracer.Start(ctx, method)
	defer span.End()

	// Increment request counter
	s.requestCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("method", method),
	))

	// Execute the function
	err := fn(ctx)
	
	// Record duration
	duration := time.Since(start).Seconds()
	s.requestDuration.Record(ctx, duration, metric.WithAttributes(
		attribute.String("method", method),
	))

	// Handle errors
	if err != nil {
		span.RecordError(err)
		span.SetStatus(tracecodes.Error, err.Error())
		s.errorCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("method", method),
			attribute.String("error", err.Error()),
		))
	} else {
		span.SetStatus(tracecodes.Ok, "success")
	}

	return err
}

// CheckPermission checks if a user has permission for a specific action
func (s *PermissionServer) CheckPermission(ctx context.Context, req *permissionv1.CheckPermissionRequest) (*permissionv1.CheckPermissionResponse, error) {
	var response *permissionv1.CheckPermissionResponse
	
	err := s.instrumentRequest(ctx, "CheckPermission", func(ctx context.Context) error {
		s.logger.Debug("CheckPermission called",
			zap.String("user_id", req.UserId),
			zap.String("resource_type", req.ResourceType.String()),
			zap.String("resource_id", req.ResourceId),
			zap.String("action", req.Action),
		)

		// Validate request
		if req.UserId == "" {
			return status.Error(codes.InvalidArgument, "user_id is required")
		}
		if req.ResourceType == permissionv1.ResourceType_RESOURCE_TYPE_UNSPECIFIED {
			return status.Error(codes.InvalidArgument, "resource_type is required")
		}
		if req.ResourceId == "" {
			return status.Error(codes.InvalidArgument, "resource_id is required")
		}
		if req.Action == "" {
			return status.Error(codes.InvalidArgument, "action is required")
		}

		// Check permission using the service layer
		result, err := s.permissionSvc.CheckPermission(ctx, services.PermissionCheckRequest{
			UserID:       req.UserId,
			ResourceType: req.ResourceType.String(),
			ResourceID:   req.ResourceId,
			Action:       req.Action,
			Context:      req.Context,
		})
		if err != nil {
			s.logger.Error("Failed to check permission", zap.Error(err))
			return status.Error(codes.Internal, "failed to check permission")
		}

		response = &permissionv1.CheckPermissionResponse{
			Allowed: result.Allowed,
			Level:   permissionv1.PermissionLevel(result.Level),
			Reasons: result.Reasons,
			Metadata: result.Metadata,
		}

		return nil
	})

	return response, err
}

// BatchCheckPermission checks multiple permissions in a single request
func (s *PermissionServer) BatchCheckPermission(ctx context.Context, req *permissionv1.BatchCheckPermissionRequest) (*permissionv1.BatchCheckPermissionResponse, error) {
	var response *permissionv1.BatchCheckPermissionResponse
	
	err := s.instrumentRequest(ctx, "BatchCheckPermission", func(ctx context.Context) error {
		s.logger.Debug("BatchCheckPermission called",
			zap.String("user_id", req.UserId),
			zap.Int("check_count", len(req.Checks)),
		)

		// Validate request
		if req.UserId == "" {
			return status.Error(codes.InvalidArgument, "user_id is required")
		}
		if len(req.Checks) == 0 {
			return status.Error(codes.InvalidArgument, "at least one check is required")
		}
		if len(req.Checks) > 100 {
			return status.Error(codes.InvalidArgument, "maximum 100 checks allowed per batch")
		}

		// Convert to service requests
		checks := make([]services.PermissionCheckRequest, len(req.Checks))
		for i, check := range req.Checks {
			checks[i] = services.PermissionCheckRequest{
				ID:           check.CheckId,
				UserID:       req.UserId,
				ResourceType: check.ResourceType.String(),
				ResourceID:   check.ResourceId,
				Action:       check.Action,
				Context:      check.Context,
			}
		}

		// Perform batch check
		results, err := s.permissionSvc.BatchCheckPermissions(ctx, checks)
		if err != nil {
			s.logger.Error("Failed to batch check permissions", zap.Error(err))
			return status.Error(codes.Internal, "failed to batch check permissions")
		}

		// Convert results
		responseResults := make(map[string]*permissionv1.CheckPermissionResponse)
		for id, result := range results {
			responseResults[id] = &permissionv1.CheckPermissionResponse{
				Allowed:  result.Allowed,
				Level:    permissionv1.PermissionLevel(result.Level),
				Reasons:  result.Reasons,
				Metadata: result.Metadata,
			}
		}

		response = &permissionv1.BatchCheckPermissionResponse{
			Results: responseResults,
		}

		return nil
	})

	return response, err
}

// GrantPermission grants a permission to a user
func (s *PermissionServer) GrantPermission(ctx context.Context, req *permissionv1.GrantPermissionRequest) (*permissionv1.GrantPermissionResponse, error) {
	var response *permissionv1.GrantPermissionResponse
	
	err := s.instrumentRequest(ctx, "GrantPermission", func(ctx context.Context) error {
		s.logger.Debug("GrantPermission called",
			zap.String("user_id", req.UserId),
			zap.String("resource_type", req.ResourceType.String()),
			zap.String("resource_id", req.ResourceId),
			zap.String("level", req.Level.String()),
		)

		// Validate request
		if req.UserId == "" {
			return status.Error(codes.InvalidArgument, "user_id is required")
		}
		if req.ResourceType == permissionv1.ResourceType_RESOURCE_TYPE_UNSPECIFIED {
			return status.Error(codes.InvalidArgument, "resource_type is required")
		}
		if req.ResourceId == "" {
			return status.Error(codes.InvalidArgument, "resource_id is required")
		}
		if req.Level == permissionv1.PermissionLevel_PERMISSION_LEVEL_UNSPECIFIED {
			return status.Error(codes.InvalidArgument, "level is required")
		}

		// Get current user from context for audit
		requestUserID := extractUserIDFromContext(ctx)
		if requestUserID == "" {
			return status.Error(codes.Unauthenticated, "authentication required")
		}

		// Grant permission using service layer
		permission, err := s.permissionSvc.GrantPermission(ctx, services.GrantPermissionRequest{
			UserID:       req.UserId,
			ResourceType: req.ResourceType.String(),
			ResourceID:   req.ResourceId,
			Level:        int(req.Level),
			ExpiresAt:    req.ExpiresAt.AsTime(),
			Attributes:   req.Attributes,
			GrantedBy:    requestUserID,
		})
		if err != nil {
			s.logger.Error("Failed to grant permission", zap.Error(err))
			if strings.Contains(err.Error(), "already exists") {
				return status.Error(codes.AlreadyExists, "permission already exists")
			}
			if strings.Contains(err.Error(), "not authorized") {
				return status.Error(codes.PermissionDenied, "not authorized to grant permission")
			}
			return status.Error(codes.Internal, "failed to grant permission")
		}

		// Convert to protobuf response
		response = &permissionv1.GrantPermissionResponse{
			Permission: &permissionv1.Permission{
				Metadata: &commonv1.BaseMetadata{
					Id:        permission.ID,
					CreatedAt: timestamppb.New(permission.CreatedAt),
					UpdatedAt: timestamppb.New(permission.UpdatedAt),
					CreatedBy: &permission.CreatedBy,
					UpdatedBy: &permission.UpdatedBy,
					Version:   permission.Version,
				},
				UserId:       permission.UserID,
				ResourceType: permissionv1.ResourceType(permission.ResourceType),
				ResourceId:   permission.ResourceID,
				Level:        permissionv1.PermissionLevel(permission.Level),
				Scope:        permissionv1.PermissionScope(permission.Scope),
				Attributes:   permission.Attributes,
				Inherited:    permission.Inherited,
			},
		}

		if permission.ExpiresAt != nil {
			response.Permission.ExpiresAt = timestamppb.New(*permission.ExpiresAt)
		}
		if permission.InheritedFrom != nil {
			response.Permission.InheritedFrom = permission.InheritedFrom
		}

		return nil
	})

	return response, err
}

// RevokePermission revokes a permission from a user
func (s *PermissionServer) RevokePermission(ctx context.Context, req *permissionv1.RevokePermissionRequest) (*permissionv1.RevokePermissionResponse, error) {
	var response *permissionv1.RevokePermissionResponse
	
	err := s.instrumentRequest(ctx, "RevokePermission", func(ctx context.Context) error {
		s.logger.Debug("RevokePermission called",
			zap.String("user_id", req.UserId),
			zap.String("resource_type", req.ResourceType.String()),
			zap.String("resource_id", req.ResourceId),
		)

		// Validate request
		if req.UserId == "" {
			return status.Error(codes.InvalidArgument, "user_id is required")
		}
		if req.ResourceType == permissionv1.ResourceType_RESOURCE_TYPE_UNSPECIFIED {
			return status.Error(codes.InvalidArgument, "resource_type is required")
		}
		if req.ResourceId == "" {
			return status.Error(codes.InvalidArgument, "resource_id is required")
		}

		// Get current user from context for audit
		requestUserID := extractUserIDFromContext(ctx)
		if requestUserID == "" {
			return status.Error(codes.Unauthenticated, "authentication required")
		}

		// Revoke permission using service layer
		level := 0
		if req.Level != nil {
			level = int(*req.Level)
		}

		err := s.permissionSvc.RevokePermission(ctx, services.RevokePermissionRequest{
			UserID:       req.UserId,
			ResourceType: req.ResourceType.String(),
			ResourceID:   req.ResourceId,
			Level:        level,
			RevokedBy:    requestUserID,
		})
		if err != nil {
			s.logger.Error("Failed to revoke permission", zap.Error(err))
			if strings.Contains(err.Error(), "not found") {
				return status.Error(codes.NotFound, "permission not found")
			}
			if strings.Contains(err.Error(), "not authorized") {
				return status.Error(codes.PermissionDenied, "not authorized to revoke permission")
			}
			return status.Error(codes.Internal, "failed to revoke permission")
		}

		response = &permissionv1.RevokePermissionResponse{
			Success: true,
			Message: "Permission successfully revoked",
		}

		return nil
	})

	return response, err
}

// ListPermissions lists permissions with optional filtering
func (s *PermissionServer) ListPermissions(ctx context.Context, req *permissionv1.ListPermissionsRequest) (*permissionv1.ListPermissionsResponse, error) {
	var response *permissionv1.ListPermissionsResponse
	
	err := s.instrumentRequest(ctx, "ListPermissions", func(ctx context.Context) error {
		s.logger.Debug("ListPermissions called",
			zap.String("user_id", req.UserId),
			zap.String("resource_type", req.ResourceType.String()),
			zap.String("resource_id", req.ResourceId),
		)

		// Convert pagination
		page := int(req.Pagination.Page)
		if page <= 0 {
			page = 1
		}
		pageSize := int(req.Pagination.PageSize)
		if pageSize <= 0 {
			pageSize = 20
		}
		if pageSize > 100 {
			pageSize = 100
		}

		// List permissions using service layer
		permissions, pagination, err := s.permissionSvc.ListPermissions(ctx, services.ListPermissionsRequest{
			UserID:       req.UserId,
			ResourceType: req.ResourceType.String(),
			ResourceID:   req.ResourceId,
			Page:         page,
			PageSize:     pageSize,
			Filters:      convertFilters(req.Filters),
		})
		if err != nil {
			s.logger.Error("Failed to list permissions", zap.Error(err))
			return status.Error(codes.Internal, "failed to list permissions")
		}

		// Convert to protobuf response
		pbPermissions := make([]*permissionv1.Permission, len(permissions))
		for i, perm := range permissions {
			pbPermissions[i] = &permissionv1.Permission{
				Metadata: &commonv1.BaseMetadata{
					Id:        perm.ID,
					CreatedAt: timestamppb.New(perm.CreatedAt),
					UpdatedAt: timestamppb.New(perm.UpdatedAt),
					CreatedBy: &perm.CreatedBy,
					UpdatedBy: &perm.UpdatedBy,
					Version:   perm.Version,
				},
				UserId:       perm.UserID,
				ResourceType: permissionv1.ResourceType(perm.ResourceType),
				ResourceId:   perm.ResourceID,
				Level:        permissionv1.PermissionLevel(perm.Level),
				Scope:        permissionv1.PermissionScope(perm.Scope),
				Attributes:   perm.Attributes,
				Inherited:    perm.Inherited,
			}

			if perm.ExpiresAt != nil {
				pbPermissions[i].ExpiresAt = timestamppb.New(*perm.ExpiresAt)
			}
			if perm.InheritedFrom != nil {
				pbPermissions[i].InheritedFrom = perm.InheritedFrom
			}
		}

		response = &permissionv1.ListPermissionsResponse{
			Permissions: pbPermissions,
			Pagination: &commonv1.PaginationResponse{
				CurrentPage: int32(pagination.CurrentPage),
				PageSize:    int32(pagination.PageSize),
				TotalCount:  pagination.TotalCount,
				TotalPages:  int32(pagination.TotalPages),
				HasNext:     pagination.HasNext,
				HasPrev:     pagination.HasPrev,
			},
		}

		return nil
	})

	return response, err
}

// CreateRole creates a new role
func (s *PermissionServer) CreateRole(ctx context.Context, req *permissionv1.CreateRoleRequest) (*permissionv1.CreateRoleResponse, error) {
	var response *permissionv1.CreateRoleResponse
	
	err := s.instrumentRequest(ctx, "CreateRole", func(ctx context.Context) error {
		s.logger.Debug("CreateRole called",
			zap.String("name", req.Name),
			zap.String("description", req.Description),
			zap.Strings("permissions", req.Permissions),
		)

		// Validate request
		if req.Name == "" {
			return status.Error(codes.InvalidArgument, "name is required")
		}
		if req.Scope == permissionv1.PermissionScope_PERMISSION_SCOPE_UNSPECIFIED {
			return status.Error(codes.InvalidArgument, "scope is required")
		}

		// Get current user from context for audit
		requestUserID := extractUserIDFromContext(ctx)
		if requestUserID == "" {
			return status.Error(codes.Unauthenticated, "authentication required")
		}

		// Create role using service layer
		role, err := s.roleSvc.CreateRole(ctx, services.CreateRoleRequest{
			Name:        req.Name,
			Description: req.Description,
			Permissions: req.Permissions,
			Scope:       int(req.Scope),
			Attributes:  req.Attributes,
			CreatedBy:   requestUserID,
		})
		if err != nil {
			s.logger.Error("Failed to create role", zap.Error(err))
			if strings.Contains(err.Error(), "already exists") {
				return status.Error(codes.AlreadyExists, "role with this name already exists")
			}
			return status.Error(codes.Internal, "failed to create role")
		}

		// Convert to protobuf response
		response = &permissionv1.CreateRoleResponse{
			Role: &permissionv1.Role{
				Metadata: &commonv1.BaseMetadata{
					Id:        role.ID,
					CreatedAt: timestamppb.New(role.CreatedAt),
					UpdatedAt: timestamppb.New(role.UpdatedAt),
					CreatedBy: &role.CreatedBy,
					UpdatedBy: &role.UpdatedBy,
					Version:   role.Version,
				},
				Name:        role.Name,
				Description: role.Description,
				Permissions: role.Permissions,
				Scope:       permissionv1.PermissionScope(role.Scope),
				SystemRole:  role.SystemRole,
				Attributes:  role.Attributes,
			},
		}

		return nil
	})

	return response, err
}

// AssignRole assigns a role to a user
func (s *PermissionServer) AssignRole(ctx context.Context, req *permissionv1.AssignRoleRequest) (*permissionv1.AssignRoleResponse, error) {
	var response *permissionv1.AssignRoleResponse
	
	err := s.instrumentRequest(ctx, "AssignRole", func(ctx context.Context) error {
		s.logger.Debug("AssignRole called",
			zap.String("user_id", req.UserId),
			zap.String("role_id", req.RoleId),
			zap.String("resource_type", req.ResourceType.String()),
			zap.String("resource_id", req.ResourceId),
		)

		// Validate request
		if req.UserId == "" {
			return status.Error(codes.InvalidArgument, "user_id is required")
		}
		if req.RoleId == "" {
			return status.Error(codes.InvalidArgument, "role_id is required")
		}
		if req.ResourceType == permissionv1.ResourceType_RESOURCE_TYPE_UNSPECIFIED {
			return status.Error(codes.InvalidArgument, "resource_type is required")
		}
		if req.ResourceId == "" {
			return status.Error(codes.InvalidArgument, "resource_id is required")
		}

		// Get current user from context for audit
		requestUserID := extractUserIDFromContext(ctx)
		if requestUserID == "" {
			return status.Error(codes.Unauthenticated, "authentication required")
		}

		// Assign role using service layer
		userRole, err := s.roleSvc.AssignRole(ctx, services.AssignRoleRequest{
			UserID:       req.UserId,
			RoleID:       req.RoleId,
			ResourceType: req.ResourceType.String(),
			ResourceID:   req.ResourceId,
			ExpiresAt:    req.ExpiresAt.AsTime(),
			AssignedBy:   requestUserID,
		})
		if err != nil {
			s.logger.Error("Failed to assign role", zap.Error(err))
			if strings.Contains(err.Error(), "already assigned") {
				return status.Error(codes.AlreadyExists, "role already assigned to user")
			}
			if strings.Contains(err.Error(), "not found") {
				return status.Error(codes.NotFound, "role not found")
			}
			return status.Error(codes.Internal, "failed to assign role")
		}

		// Convert to protobuf response
		response = &permissionv1.AssignRoleResponse{
			UserRole: &permissionv1.UserRole{
				Metadata: &commonv1.BaseMetadata{
					Id:        userRole.ID,
					CreatedAt: timestamppb.New(userRole.CreatedAt),
					UpdatedAt: timestamppb.New(userRole.UpdatedAt),
					CreatedBy: &userRole.CreatedBy,
					UpdatedBy: &userRole.UpdatedBy,
					Version:   userRole.Version,
				},
				UserId:       userRole.UserID,
				RoleId:       userRole.RoleID,
				ResourceType: permissionv1.ResourceType(userRole.ResourceType),
				ResourceId:   userRole.ResourceID,
				AssignedBy:   userRole.AssignedBy,
			},
		}

		if userRole.ExpiresAt != nil {
			response.UserRole.ExpiresAt = timestamppb.New(*userRole.ExpiresAt)
		}

		return nil
	})

	return response, err
}

// ListUserRoles lists roles assigned to a user
func (s *PermissionServer) ListUserRoles(ctx context.Context, req *permissionv1.ListUserRolesRequest) (*permissionv1.ListUserRolesResponse, error) {
	var response *permissionv1.ListUserRolesResponse
	
	err := s.instrumentRequest(ctx, "ListUserRoles", func(ctx context.Context) error {
		s.logger.Debug("ListUserRoles called",
			zap.String("user_id", req.UserId),
			zap.String("resource_type", req.ResourceType.String()),
			zap.String("resource_id", req.ResourceId),
		)

		// Validate request
		if req.UserId == "" {
			return status.Error(codes.InvalidArgument, "user_id is required")
		}

		// Convert pagination
		page := int(req.Pagination.Page)
		if page <= 0 {
			page = 1
		}
		pageSize := int(req.Pagination.PageSize)
		if pageSize <= 0 {
			pageSize = 20
		}
		if pageSize > 100 {
			pageSize = 100
		}

		// List user roles using service layer
		userRoles, pagination, err := s.roleSvc.ListUserRoles(ctx, services.ListUserRolesRequest{
			UserID:       req.UserId,
			ResourceType: req.ResourceType.String(),
			ResourceID:   req.ResourceId,
			Page:         page,
			PageSize:     pageSize,
		})
		if err != nil {
			s.logger.Error("Failed to list user roles", zap.Error(err))
			return status.Error(codes.Internal, "failed to list user roles")
		}

		// Convert to protobuf response
		pbUserRoles := make([]*permissionv1.UserRole, len(userRoles))
		for i, userRole := range userRoles {
			pbUserRoles[i] = &permissionv1.UserRole{
				Metadata: &commonv1.BaseMetadata{
					Id:        userRole.ID,
					CreatedAt: timestamppb.New(userRole.CreatedAt),
					UpdatedAt: timestamppb.New(userRole.UpdatedAt),
					CreatedBy: &userRole.CreatedBy,
					UpdatedBy: &userRole.UpdatedBy,
					Version:   userRole.Version,
				},
				UserId:       userRole.UserID,
				RoleId:       userRole.RoleID,
				ResourceType: permissionv1.ResourceType(userRole.ResourceType),
				ResourceId:   userRole.ResourceID,
				AssignedBy:   userRole.AssignedBy,
			}

			if userRole.ExpiresAt != nil {
				pbUserRoles[i].ExpiresAt = timestamppb.New(*userRole.ExpiresAt)
			}
		}

		response = &permissionv1.ListUserRolesResponse{
			UserRoles: pbUserRoles,
			Pagination: &commonv1.PaginationResponse{
				CurrentPage: int32(pagination.CurrentPage),
				PageSize:    int32(pagination.PageSize),
				TotalCount:  pagination.TotalCount,
				TotalPages:  int32(pagination.TotalPages),
				HasNext:     pagination.HasNext,
				HasPrev:     pagination.HasPrev,
			},
		}

		return nil
	})

	return response, err
}

// HealthCheck implements health check for the gRPC service
func (s *PermissionServer) HealthCheck(ctx context.Context, req *emptypb.Empty) (*commonv1.HealthCheckResponse, error) {
	var response *commonv1.HealthCheckResponse
	
	err := s.instrumentRequest(ctx, "HealthCheck", func(ctx context.Context) error {
		s.logger.Debug("HealthCheck called")

		// Check service health
		healthy, services, details := s.permissionSvc.HealthCheck(ctx)
		
		status := commonv1.HealthStatus_HEALTH_STATUS_SERVING
		if !healthy {
			status = commonv1.HealthStatus_HEALTH_STATUS_NOT_SERVING
		}

		response = &commonv1.HealthCheckResponse{
			Status:   status,
			Services: services,
			Details:  details,
		}

		return nil
	})

	return response, err
}

// Helper functions

func extractUserIDFromContext(ctx context.Context) string {
	// Implementation would extract user ID from gRPC metadata or JWT token
	// This is a placeholder implementation
	if userID, ok := ctx.Value("user_id").(string); ok {
		return userID
	}
	return ""
}

func convertFilters(filters []*commonv1.Filter) []services.Filter {
	result := make([]services.Filter, len(filters))
	for i, filter := range filters {
		result[i] = services.Filter{
			Field:    filter.Field,
			Operator: filter.Operator.String(),
			Values:   filter.Values,
		}
	}
	return result
}