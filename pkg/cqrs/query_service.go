package cqrs

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/pyairtable-compose/go-services/pkg/cache"
)

// QueryService provides optimized read operations with multi-level caching
type QueryService struct {
	readDB       *sql.DB
	cacheManager *CQRSCacheManager
	optimizer    *QueryOptimizer
	tracker      *PerformanceTracker
}

// NewQueryService creates a new query service
func NewQueryService(readDB *sql.DB, cacheManager *CQRSCacheManager, optimizer *QueryOptimizer) *QueryService {
	return &QueryService{
		readDB:       readDB,
		cacheManager: cacheManager,
		optimizer:    optimizer,
		tracker:      NewPerformanceTracker(),
	}
}

// UserProjection represents a user read model
type UserProjection struct {
	ID                  string                 `json:"id" db:"id"`
	Email               string                 `json:"email" db:"email"`
	FirstName           string                 `json:"first_name" db:"first_name"`
	LastName            string                 `json:"last_name" db:"last_name"`
	DisplayName         string                 `json:"display_name" db:"display_name"`
	Avatar              *string                `json:"avatar" db:"avatar"`
	TenantID            string                 `json:"tenant_id" db:"tenant_id"`
	Status              string                 `json:"status" db:"status"`
	EmailVerified       bool                   `json:"email_verified" db:"email_verified"`
	IsActive            bool                   `json:"is_active" db:"is_active"`
	Timezone            *string                `json:"timezone" db:"timezone"`
	Language            string                 `json:"language" db:"language"`
	Roles               []string               `json:"roles"`
	DeactivationReason  *string                `json:"deactivation_reason" db:"deactivation_reason"`
	LastLoginAt         *time.Time             `json:"last_login_at" db:"last_login_at"`
	LoginCount          int                    `json:"login_count" db:"login_count"`
	CreatedAt           time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at" db:"updated_at"`
	EventVersion        int64                  `json:"event_version" db:"event_version"`
}

// WorkspaceProjection represents a workspace read model
type WorkspaceProjection struct {
	ID          string                 `json:"id" db:"id"`
	Name        string                 `json:"name" db:"name"`
	Description *string                `json:"description" db:"description"`
	TenantID    string                 `json:"tenant_id" db:"tenant_id"`
	OwnerID     string                 `json:"owner_id" db:"owner_id"`
	Status      string                 `json:"status" db:"status"`
	IsPublic    bool                   `json:"is_public" db:"is_public"`
	MemberCount int                    `json:"member_count" db:"member_count"`
	BaseCount   int                    `json:"base_count" db:"base_count"`
	Settings    map[string]interface{} `json:"settings"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
	EventVersion int64                 `json:"event_version" db:"event_version"`
}

// WorkspaceMemberProjection represents a workspace member read model
type WorkspaceMemberProjection struct {
	ID          string    `json:"id" db:"id"`
	WorkspaceID string    `json:"workspace_id" db:"workspace_id"`
	UserID      string    `json:"user_id" db:"user_id"`
	Role        string    `json:"role" db:"role"`
	Permissions []string  `json:"permissions"`
	JoinedAt    time.Time `json:"joined_at" db:"joined_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	EventVersion int64    `json:"event_version" db:"event_version"`
}

// TenantSummaryProjection represents a tenant summary read model
type TenantSummaryProjection struct {
	TenantID         string    `json:"tenant_id" db:"tenant_id"`
	Name             string    `json:"name" db:"name"`
	TotalUsers       int       `json:"total_users" db:"total_users"`
	ActiveUsers      int       `json:"active_users" db:"active_users"`
	TotalWorkspaces  int       `json:"total_workspaces" db:"total_workspaces"`
	TotalBases       int       `json:"total_bases" db:"total_bases"`
	PlanType         string    `json:"plan_type" db:"plan_type"`
	StorageUsedBytes int64     `json:"storage_used_bytes" db:"storage_used_bytes"`
	APICallsMonth    int       `json:"api_calls_month" db:"api_calls_month"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
	EventVersion     int64     `json:"event_version" db:"event_version"`
}

// GetUserByID retrieves a user by ID with caching
func (qs *QueryService) GetUserByID(ctx context.Context, userID, tenantID string) (*UserProjection, error) {
	var user UserProjection
	var cacheHit bool
	
	err := qs.tracker.TrackQuery("get_user_by_id", func() error {
		// Try cache first
		cacheKey := CacheKey{
			Prefix:   "user",
			EntityID: userID,
			TenantID: tenantID,
			Version:  "v1",
		}

		found, err := qs.tracker.TrackCacheOperation("get", "L2", func() error {
			var cacheErr error
			found, cacheErr := qs.cacheManager.GetProjection(ctx, cacheKey, &user)
			if cacheErr != nil {
				log.Printf("Cache error for user %s: %v", userID, cacheErr)
				return cacheErr
			}
			cacheHit = found
			return nil
		})
		
		if err != nil {
			log.Printf("Cache error for user %s: %v", userID, err)
			// Continue to database on cache error
		} else if found {
			log.Printf("Cache hit for user: %s", userID)
			return nil // Success via cache
		}

		// Cache miss - query database
		log.Printf("Cache miss for user: %s, querying database", userID)
		
		return qs.tracker.TrackDatabaseQuery("read", "get_user_by_id", func() error {
			row := qs.readDB.QueryRowContext(ctx, `
				SELECT id, email, first_name, last_name, display_name, avatar, 
					   tenant_id, status, email_verified, is_active, timezone, language,
					   COALESCE(roles, '[]'::jsonb) as roles, deactivation_reason,
					   last_login_at, login_count, created_at, updated_at, event_version
				FROM read_schema.user_projections 
				WHERE id = $1 AND tenant_id = $2
			`, userID, tenantID)

			var rolesJSON string
			dbErr := row.Scan(
				&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.DisplayName,
				&user.Avatar, &user.TenantID, &user.Status, &user.EmailVerified, &user.IsActive,
				&user.Timezone, &user.Language, &rolesJSON, &user.DeactivationReason,
				&user.LastLoginAt, &user.LoginCount, &user.CreatedAt, &user.UpdatedAt, &user.EventVersion,
			)

			if dbErr != nil {
				if dbErr == sql.ErrNoRows {
					return fmt.Errorf("user not found: %s", userID)
				}
				return fmt.Errorf("failed to query user: %w", dbErr)
			}

			// Parse roles JSON
			if parseErr := parseJSONArray(rolesJSON, &user.Roles); parseErr != nil {
				log.Printf("Failed to parse roles for user %s: %v", userID, parseErr)
				user.Roles = []string{}
			}

			// Cache the result
			if cacheErr := qs.cacheManager.SetProjection(ctx, cacheKey, user, 15*time.Minute); cacheErr != nil {
				log.Printf("Failed to cache user %s: %v", userID, cacheErr)
			}

			return nil
		})
	}, cacheHit)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

// GetUsersByTenant retrieves users for a tenant with pagination and caching
func (qs *QueryService) GetUsersByTenant(ctx context.Context, tenantID string, limit, offset int, activeOnly bool) ([]UserProjection, error) {
	// Create cache key for this query
	cacheKey := CacheKey{
		Prefix:   "users_tenant",
		EntityID: tenantID,
		Version:  "v1",
		Params: map[string]string{
			"limit":      fmt.Sprintf("%d", limit),
			"offset":     fmt.Sprintf("%d", offset),
			"active_only": fmt.Sprintf("%t", activeOnly),
		},
	}

	var users []UserProjection
	found, err := qs.cacheManager.GetProjection(ctx, cacheKey, &users)
	if err != nil {
		log.Printf("Cache error for users tenant %s: %v", tenantID, err)
	} else if found {
		log.Printf("Cache hit for users tenant: %s", tenantID)
		return users, nil
	}

	// Build query based on filters
	query := `
		SELECT id, email, first_name, last_name, display_name, avatar, 
			   tenant_id, status, email_verified, is_active, timezone, language,
			   COALESCE(roles, '[]'::jsonb) as roles, deactivation_reason,
			   last_login_at, login_count, created_at, updated_at, event_version
		FROM read_schema.user_projections 
		WHERE tenant_id = $1
	`
	args := []interface{}{tenantID}
	argIndex := 2

	if activeOnly {
		query += fmt.Sprintf(" AND is_active = $%d", argIndex)
		args = append(args, true)
		argIndex++
	}

	query += " ORDER BY created_at DESC"
	
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, limit)
		argIndex++
	}
	
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIndex)
		args = append(args, offset)
	}

	rows, err := qs.readDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	users = make([]UserProjection, 0)
	for rows.Next() {
		var user UserProjection
		var rolesJSON string

		err := rows.Scan(
			&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.DisplayName,
			&user.Avatar, &user.TenantID, &user.Status, &user.EmailVerified, &user.IsActive,
			&user.Timezone, &user.Language, &rolesJSON, &user.DeactivationReason,
			&user.LastLoginAt, &user.LoginCount, &user.CreatedAt, &user.UpdatedAt, &user.EventVersion,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user row: %w", err)
		}

		// Parse roles JSON
		if err := parseJSONArray(rolesJSON, &user.Roles); err != nil {
			log.Printf("Failed to parse roles for user %s: %v", user.ID, err)
			user.Roles = []string{}
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user rows: %w", err)
	}

	// Cache the result (shorter TTL for list queries)
	if err := qs.cacheManager.SetProjection(ctx, cacheKey, users, 5*time.Minute); err != nil {
		log.Printf("Failed to cache users for tenant %s: %v", tenantID, err)
	}

	return users, nil
}

// GetWorkspaceByID retrieves a workspace by ID with caching
func (qs *QueryService) GetWorkspaceByID(ctx context.Context, workspaceID, tenantID string) (*WorkspaceProjection, error) {
	cacheKey := CacheKey{
		Prefix:   "workspace",
		EntityID: workspaceID,
		TenantID: tenantID,
		Version:  "v1",
	}

	var workspace WorkspaceProjection
	found, err := qs.cacheManager.GetProjection(ctx, cacheKey, &workspace)
	if err != nil {
		log.Printf("Cache error for workspace %s: %v", workspaceID, err)
	} else if found {
		log.Printf("Cache hit for workspace: %s", workspaceID)
		return &workspace, nil
	}

	row := qs.readDB.QueryRowContext(ctx, `
		SELECT id, name, description, tenant_id, owner_id, status, is_public,
			   member_count, base_count, COALESCE(settings, '{}'::jsonb) as settings,
			   created_at, updated_at, event_version
		FROM read_schema.workspace_projections 
		WHERE id = $1 AND tenant_id = $2 AND status != 'deleted'
	`, workspaceID, tenantID)

	var settingsJSON string
	err = row.Scan(
		&workspace.ID, &workspace.Name, &workspace.Description, &workspace.TenantID,
		&workspace.OwnerID, &workspace.Status, &workspace.IsPublic, &workspace.MemberCount,
		&workspace.BaseCount, &settingsJSON, &workspace.CreatedAt, &workspace.UpdatedAt,
		&workspace.EventVersion,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("workspace not found: %s", workspaceID)
		}
		return nil, fmt.Errorf("failed to query workspace: %w", err)
	}

	// Parse settings JSON
	if err := parseJSONObject(settingsJSON, &workspace.Settings); err != nil {
		log.Printf("Failed to parse settings for workspace %s: %v", workspaceID, err)
		workspace.Settings = make(map[string]interface{})
	}

	// Cache the result
	if err := qs.cacheManager.SetProjection(ctx, cacheKey, workspace, 15*time.Minute); err != nil {
		log.Printf("Failed to cache workspace %s: %v", workspaceID, err)
	}

	return &workspace, nil
}

// GetWorkspacesByTenant retrieves workspaces for a tenant with caching
func (qs *QueryService) GetWorkspacesByTenant(ctx context.Context, tenantID string, limit, offset int) ([]WorkspaceProjection, error) {
	cacheKey := CacheKey{
		Prefix:   "workspaces_tenant",
		EntityID: tenantID,
		Version:  "v1",
		Params: map[string]string{
			"limit":  fmt.Sprintf("%d", limit),
			"offset": fmt.Sprintf("%d", offset),
		},
	}

	var workspaces []WorkspaceProjection
	found, err := qs.cacheManager.GetProjection(ctx, cacheKey, &workspaces)
	if err != nil {
		log.Printf("Cache error for workspaces tenant %s: %v", tenantID, err)
	} else if found {
		log.Printf("Cache hit for workspaces tenant: %s", tenantID)
		return workspaces, nil
	}

	query := `
		SELECT id, name, description, tenant_id, owner_id, status, is_public,
			   member_count, base_count, COALESCE(settings, '{}'::jsonb) as settings,
			   created_at, updated_at, event_version
		FROM read_schema.workspace_projections 
		WHERE tenant_id = $1 AND status != 'deleted'
		ORDER BY created_at DESC
	`
	args := []interface{}{tenantID}

	if limit > 0 {
		query += " LIMIT $2"
		args = append(args, limit)
		if offset > 0 {
			query += " OFFSET $3"
			args = append(args, offset)
		}
	}

	rows, err := qs.readDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query workspaces: %w", err)
	}
	defer rows.Close()

	workspaces = make([]WorkspaceProjection, 0)
	for rows.Next() {
		var workspace WorkspaceProjection
		var settingsJSON string

		err := rows.Scan(
			&workspace.ID, &workspace.Name, &workspace.Description, &workspace.TenantID,
			&workspace.OwnerID, &workspace.Status, &workspace.IsPublic, &workspace.MemberCount,
			&workspace.BaseCount, &settingsJSON, &workspace.CreatedAt, &workspace.UpdatedAt,
			&workspace.EventVersion,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan workspace row: %w", err)
		}

		// Parse settings JSON
		if err := parseJSONObject(settingsJSON, &workspace.Settings); err != nil {
			log.Printf("Failed to parse settings for workspace %s: %v", workspace.ID, err)
			workspace.Settings = make(map[string]interface{})
		}

		workspaces = append(workspaces, workspace)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating workspace rows: %w", err)
	}

	// Cache the result
	if err := qs.cacheManager.SetProjection(ctx, cacheKey, workspaces, 5*time.Minute); err != nil {
		log.Printf("Failed to cache workspaces for tenant %s: %v", tenantID, err)
	}

	return workspaces, nil
}

// GetWorkspaceMembers retrieves workspace members with caching
func (qs *QueryService) GetWorkspaceMembers(ctx context.Context, workspaceID string, limit, offset int) ([]WorkspaceMemberProjection, error) {
	cacheKey := CacheKey{
		Prefix:   "workspace_members",
		EntityID: workspaceID,
		Version:  "v1",
		Params: map[string]string{
			"limit":  fmt.Sprintf("%d", limit),
			"offset": fmt.Sprintf("%d", offset),
		},
	}

	var members []WorkspaceMemberProjection
	found, err := qs.cacheManager.GetProjection(ctx, cacheKey, &members)
	if err != nil {
		log.Printf("Cache error for workspace members %s: %v", workspaceID, err)
	} else if found {
		log.Printf("Cache hit for workspace members: %s", workspaceID)
		return members, nil
	}

	query := `
		SELECT id, workspace_id, user_id, role, 
			   COALESCE(permissions, '[]'::jsonb) as permissions,
			   joined_at, updated_at, event_version
		FROM read_schema.workspace_member_projections 
		WHERE workspace_id = $1
		ORDER BY joined_at ASC
	`
	args := []interface{}{workspaceID}

	if limit > 0 {
		query += " LIMIT $2"
		args = append(args, limit)
		if offset > 0 {
			query += " OFFSET $3"
			args = append(args, offset)
		}
	}

	rows, err := qs.readDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query workspace members: %w", err)
	}
	defer rows.Close()

	members = make([]WorkspaceMemberProjection, 0)
	for rows.Next() {
		var member WorkspaceMemberProjection
		var permissionsJSON string

		err := rows.Scan(
			&member.ID, &member.WorkspaceID, &member.UserID, &member.Role,
			&permissionsJSON, &member.JoinedAt, &member.UpdatedAt, &member.EventVersion,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan member row: %w", err)
		}

		// Parse permissions JSON
		if err := parseJSONArray(permissionsJSON, &member.Permissions); err != nil {
			log.Printf("Failed to parse permissions for member %s: %v", member.ID, err)
			member.Permissions = []string{}
		}

		members = append(members, member)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating member rows: %w", err)
	}

	// Cache the result
	if err := qs.cacheManager.SetProjection(ctx, cacheKey, members, 10*time.Minute); err != nil {
		log.Printf("Failed to cache workspace members %s: %v", workspaceID, err)
	}

	return members, nil
}

// GetTenantSummary retrieves tenant summary with caching
func (qs *QueryService) GetTenantSummary(ctx context.Context, tenantID string) (*TenantSummaryProjection, error) {
	cacheKey := CacheKey{
		Prefix:   "tenant_summary",
		EntityID: tenantID,
		Version:  "v1",
	}

	var summary TenantSummaryProjection
	found, err := qs.cacheManager.GetProjection(ctx, cacheKey, &summary)
	if err != nil {
		log.Printf("Cache error for tenant summary %s: %v", tenantID, err)
	} else if found {
		log.Printf("Cache hit for tenant summary: %s", tenantID)
		return &summary, nil
	}

	row := qs.readDB.QueryRowContext(ctx, `
		SELECT tenant_id, name, total_users, active_users, total_workspaces,
			   total_bases, plan_type, storage_used_bytes, api_calls_month,
			   created_at, updated_at, event_version
		FROM read_schema.tenant_summary_projections 
		WHERE tenant_id = $1
	`, tenantID)

	err = row.Scan(
		&summary.TenantID, &summary.Name, &summary.TotalUsers, &summary.ActiveUsers,
		&summary.TotalWorkspaces, &summary.TotalBases, &summary.PlanType,
		&summary.StorageUsedBytes, &summary.APICallsMonth, &summary.CreatedAt,
		&summary.UpdatedAt, &summary.EventVersion,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tenant summary not found: %s", tenantID)
		}
		return nil, fmt.Errorf("failed to query tenant summary: %w", err)
	}

	// Cache the result (longer TTL for summary data)
	if err := qs.cacheManager.SetProjection(ctx, cacheKey, summary, 30*time.Minute); err != nil {
		log.Printf("Failed to cache tenant summary %s: %v", tenantID, err)
	}

	return &summary, nil
}

// InvalidateUserCache invalidates cache for a user
func (qs *QueryService) InvalidateUserCache(ctx context.Context, userID, tenantID string) error {
	keys := []CacheKey{
		{Prefix: "user", EntityID: userID, TenantID: tenantID, Version: "v1"},
		{Prefix: "users_tenant", EntityID: tenantID, Version: "v1"},
	}

	for _, key := range keys {
		if err := qs.cacheManager.InvalidateProjection(ctx, key); err != nil {
			log.Printf("Failed to invalidate cache key %s: %v", key.String(), err)
		}
	}
	return nil
}

// InvalidateWorkspaceCache invalidates cache for a workspace
func (qs *QueryService) InvalidateWorkspaceCache(ctx context.Context, workspaceID, tenantID string) error {
	keys := []CacheKey{
		{Prefix: "workspace", EntityID: workspaceID, TenantID: tenantID, Version: "v1"},
		{Prefix: "workspaces_tenant", EntityID: tenantID, Version: "v1"},
		{Prefix: "workspace_members", EntityID: workspaceID, Version: "v1"},
	}

	for _, key := range keys {
		if err := qs.cacheManager.InvalidateProjection(ctx, key); err != nil {
			log.Printf("Failed to invalidate cache key %s: %v", key.String(), err)
		}
	}
	return nil
}