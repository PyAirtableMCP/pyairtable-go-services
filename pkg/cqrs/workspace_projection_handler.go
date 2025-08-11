package cqrs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"github.com/pyairtable-compose/go-services/domain/shared"
)

// WorkspaceProjectionHandler implements ProjectionHandler for workspace events
type WorkspaceProjectionHandler struct {
	readDB *sql.DB
}

// NewWorkspaceProjectionHandler creates a new workspace projection handler
func NewWorkspaceProjectionHandler(readDB *sql.DB) *WorkspaceProjectionHandler {
	return &WorkspaceProjectionHandler{
		readDB: readDB,
	}
}

// GetName returns the handler name
func (h *WorkspaceProjectionHandler) GetName() string {
	return "workspace_projection"
}

// GetEventTypes returns the event types this handler processes
func (h *WorkspaceProjectionHandler) GetEventTypes() []string {
	return []string{
		shared.EventTypeWorkspaceCreated,
		shared.EventTypeWorkspaceUpdated,
		shared.EventTypeWorkspaceDeleted,
		shared.EventTypeWorkspaceMemberAdded,
		shared.EventTypeWorkspaceMemberRemoved,
		shared.EventTypeWorkspaceMemberRoleChanged,
	}
}

// Handle processes workspace events and updates read model projections
func (h *WorkspaceProjectionHandler) Handle(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error {
	log.Printf("Processing workspace event: %s for aggregate: %s", event.EventType(), event.AggregateID())

	switch event.EventType() {
	case shared.EventTypeWorkspaceCreated:
		return h.handleWorkspaceCreated(ctx, event, tx)
	case shared.EventTypeWorkspaceUpdated:
		return h.handleWorkspaceUpdated(ctx, event, tx)
	case shared.EventTypeWorkspaceDeleted:
		return h.handleWorkspaceDeleted(ctx, event, tx)
	case shared.EventTypeWorkspaceMemberAdded:
		return h.handleWorkspaceMemberAdded(ctx, event, tx)
	case shared.EventTypeWorkspaceMemberRemoved:
		return h.handleWorkspaceMemberRemoved(ctx, event, tx)
	case shared.EventTypeWorkspaceMemberRoleChanged:
		return h.handleWorkspaceMemberRoleChanged(ctx, event, tx)
	default:
		return fmt.Errorf("unsupported event type: %s", event.EventType())
	}
}

// GetCheckpoint retrieves the last processed checkpoint for this handler
func (h *WorkspaceProjectionHandler) GetCheckpoint(ctx context.Context, db *sql.DB) (string, error) {
	var checkpoint sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT last_processed_event_id 
		FROM read_schema.projection_checkpoints 
		WHERE projection_name = $1
	`, h.GetName()).Scan(&checkpoint)

	if err == sql.ErrNoRows {
		return "", nil // No checkpoint yet
	}
	if err != nil {
		return "", fmt.Errorf("failed to get checkpoint: %w", err)
	}

	if checkpoint.Valid {
		return checkpoint.String, nil
	}
	return "", nil
}

// UpdateCheckpoint updates the checkpoint for this handler
func (h *WorkspaceProjectionHandler) UpdateCheckpoint(ctx context.Context, eventID string, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO read_schema.projection_checkpoints (
			projection_name, last_processed_event_id, last_processed_at, events_processed_count
		) VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = EXCLUDED.last_processed_event_id,
			last_processed_at = EXCLUDED.last_processed_at,
			events_processed_count = projection_checkpoints.events_processed_count + 1
	`, h.GetName(), eventID)

	if err != nil {
		return fmt.Errorf("failed to update checkpoint: %w", err)
	}

	return nil
}

// handleWorkspaceCreated creates a new workspace record in the read model
func (h *WorkspaceProjectionHandler) handleWorkspaceCreated(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error {
	payload := event.Payload().(map[string]interface{})

	workspaceID := payload["workspace_id"].(string)
	name := payload["name"].(string)
	description, _ := payload["description"].(string)
	tenantID := payload["tenant_id"].(string)
	ownerID := payload["owner_id"].(string)
	isPublic, _ := payload["is_public"].(bool)

	// Parse settings if provided
	settings := make(map[string]interface{})
	if settingsRaw, ok := payload["settings"]; ok {
		if settingsMap, ok := settingsRaw.(map[string]interface{}); ok {
			settings = settingsMap
		}
	}
	settingsJSON, _ := json.Marshal(settings)

	_, err := tx.ExecContext(ctx, `
		INSERT INTO read_schema.workspace_projections (
			id, name, description, tenant_id, owner_id, status, is_public,
			member_count, base_count, settings, created_at, updated_at, event_version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			owner_id = EXCLUDED.owner_id,
			is_public = EXCLUDED.is_public,
			settings = EXCLUDED.settings,
			updated_at = EXCLUDED.updated_at,
			event_version = EXCLUDED.event_version
		WHERE workspace_projections.event_version < EXCLUDED.event_version
	`,
		workspaceID,
		name,
		description,
		tenantID,
		ownerID,
		"active",
		isPublic,
		1, // Owner is the first member
		0, // No bases initially
		string(settingsJSON),
		event.OccurredAt(),
		event.OccurredAt(),
		event.Version(),
	)

	if err != nil {
		return fmt.Errorf("failed to insert workspace projection: %w", err)
	}

	// Add owner as a member
	err = h.addWorkspaceMember(ctx, workspaceID, ownerID, "owner", event, tx)
	if err != nil {
		log.Printf("Failed to add owner as workspace member: %v", err)
		// Don't fail the entire operation
	}

	// Update tenant summary
	err = h.updateTenantWorkspaceCounts(ctx, tenantID, tx)
	if err != nil {
		log.Printf("Failed to update tenant workspace counts: %v", err)
		// Don't fail the entire operation
	}

	log.Printf("Workspace projection created: %s (%s)", workspaceID, name)
	return nil
}

// handleWorkspaceUpdated updates workspace information
func (h *WorkspaceProjectionHandler) handleWorkspaceUpdated(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error {
	payload := event.Payload().(map[string]interface{})
	workspaceID := payload["workspace_id"].(string)

	// Build dynamic update query based on available fields
	updates := []string{"updated_at = $1", "event_version = $2"}
	args := []interface{}{event.OccurredAt(), event.Version()}
	argIndex := 3

	if name, ok := payload["name"].(string); ok {
		updates = append(updates, fmt.Sprintf("name = $%d", argIndex))
		args = append(args, name)
		argIndex++
	}

	if description, ok := payload["description"].(string); ok {
		updates = append(updates, fmt.Sprintf("description = $%d", argIndex))
		args = append(args, description)
		argIndex++
	}

	if isPublic, ok := payload["is_public"].(bool); ok {
		updates = append(updates, fmt.Sprintf("is_public = $%d", argIndex))
		args = append(args, isPublic)
		argIndex++
	}

	if settings, ok := payload["settings"]; ok {
		settingsJSON, _ := json.Marshal(settings)
		updates = append(updates, fmt.Sprintf("settings = $%d", argIndex))
		args = append(args, string(settingsJSON))
		argIndex++
	}

	if status, ok := payload["status"].(string); ok {
		updates = append(updates, fmt.Sprintf("status = $%d", argIndex))
		args = append(args, status)
		argIndex++
	}

	// Add WHERE conditions
	args = append(args, workspaceID, event.Version())
	whereClause := fmt.Sprintf("WHERE id = $%d AND event_version < $%d", argIndex-1, argIndex)

	// Build the complete query
	updateStr := ""
	for i, update := range updates {
		if i > 0 {
			updateStr += ", "
		}
		updateStr += update
	}

	query := fmt.Sprintf(`
		UPDATE read_schema.workspace_projections 
		SET %s
		%s
	`, updateStr, whereClause)

	_, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update workspace projection: %w", err)
	}

	log.Printf("Workspace projection updated: %s (%d fields)", workspaceID, len(updates)-2)
	return nil
}

// handleWorkspaceDeleted marks workspace as deleted
func (h *WorkspaceProjectionHandler) handleWorkspaceDeleted(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error {
	payload := event.Payload().(map[string]interface{})
	workspaceID := payload["workspace_id"].(string)

	_, err := tx.ExecContext(ctx, `
		UPDATE read_schema.workspace_projections 
		SET status = 'deleted', updated_at = $1, event_version = $2
		WHERE id = $3 AND event_version < $2
	`,
		event.OccurredAt(),
		event.Version(),
		workspaceID,
	)

	if err != nil {
		return fmt.Errorf("failed to mark workspace as deleted: %w", err)
	}

	// Update tenant workspace count
	var tenantID string
	err = tx.QueryRowContext(ctx, "SELECT tenant_id FROM read_schema.workspace_projections WHERE id = $1", workspaceID).Scan(&tenantID)
	if err == nil {
		h.updateTenantWorkspaceCounts(ctx, tenantID, tx)
	}

	log.Printf("Workspace projection deleted: %s", workspaceID)
	return nil
}

// handleWorkspaceMemberAdded adds a member to workspace
func (h *WorkspaceProjectionHandler) handleWorkspaceMemberAdded(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error {
	payload := event.Payload().(map[string]interface{})
	workspaceID := payload["workspace_id"].(string)
	userID := payload["user_id"].(string)
	role, _ := payload["role"].(string)
	if role == "" {
		role = "member"
	}

	err := h.addWorkspaceMember(ctx, workspaceID, userID, role, event, tx)
	if err != nil {
		return err
	}

	// Update member count
	err = h.updateWorkspaceMemberCount(ctx, workspaceID, tx)
	if err != nil {
		log.Printf("Failed to update workspace member count: %v", err)
	}

	log.Printf("Workspace member added: %s to %s as %s", userID, workspaceID, role)
	return nil
}

// handleWorkspaceMemberRemoved removes a member from workspace
func (h *WorkspaceProjectionHandler) handleWorkspaceMemberRemoved(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error {
	payload := event.Payload().(map[string]interface{})
	workspaceID := payload["workspace_id"].(string)
	userID := payload["user_id"].(string)

	_, err := tx.ExecContext(ctx, `
		DELETE FROM read_schema.workspace_member_projections 
		WHERE workspace_id = $1 AND user_id = $2
	`, workspaceID, userID)

	if err != nil {
		return fmt.Errorf("failed to remove workspace member: %w", err)
	}

	// Update member count
	err = h.updateWorkspaceMemberCount(ctx, workspaceID, tx)
	if err != nil {
		log.Printf("Failed to update workspace member count: %v", err)
	}

	log.Printf("Workspace member removed: %s from %s", userID, workspaceID)
	return nil
}

// handleWorkspaceMemberRoleChanged updates member role
func (h *WorkspaceProjectionHandler) handleWorkspaceMemberRoleChanged(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error {
	payload := event.Payload().(map[string]interface{})
	workspaceID := payload["workspace_id"].(string)
	userID := payload["user_id"].(string)
	newRole := payload["role"].(string)

	// Parse permissions if provided
	permissions := []string{}
	if permsRaw, ok := payload["permissions"]; ok {
		if permsSlice, ok := permsRaw.([]interface{}); ok {
			for _, perm := range permsSlice {
				if permStr, ok := perm.(string); ok {
					permissions = append(permissions, permStr)
				}
			}
		}
	}
	permissionsJSON, _ := json.Marshal(permissions)

	_, err := tx.ExecContext(ctx, `
		UPDATE read_schema.workspace_member_projections 
		SET role = $1, permissions = $2, updated_at = $3, event_version = $4
		WHERE workspace_id = $5 AND user_id = $6 AND event_version < $4
	`,
		newRole,
		string(permissionsJSON),
		event.OccurredAt(),
		event.Version(),
		workspaceID,
		userID,
	)

	if err != nil {
		return fmt.Errorf("failed to update workspace member role: %w", err)
	}

	log.Printf("Workspace member role changed: %s in %s to %s", userID, workspaceID, newRole)
	return nil
}

// addWorkspaceMember adds a member to workspace member projections
func (h *WorkspaceProjectionHandler) addWorkspaceMember(ctx context.Context, workspaceID, userID, role string, event shared.DomainEvent, tx *sql.Tx) error {
	permissions := []string{}
	permissionsJSON, _ := json.Marshal(permissions)

	_, err := tx.ExecContext(ctx, `
		INSERT INTO read_schema.workspace_member_projections (
			workspace_id, user_id, role, permissions, joined_at, updated_at, event_version
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET
			role = EXCLUDED.role,
			permissions = EXCLUDED.permissions,
			updated_at = EXCLUDED.updated_at,
			event_version = EXCLUDED.event_version
		WHERE workspace_member_projections.event_version < EXCLUDED.event_version
	`,
		workspaceID,
		userID,
		role,
		string(permissionsJSON),
		event.OccurredAt(),
		event.OccurredAt(),
		event.Version(),
	)

	return err
}

// updateWorkspaceMemberCount updates the member count for a workspace
func (h *WorkspaceProjectionHandler) updateWorkspaceMemberCount(ctx context.Context, workspaceID string, tx *sql.Tx) error {
	var memberCount int
	err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) 
		FROM read_schema.workspace_member_projections 
		WHERE workspace_id = $1
	`, workspaceID).Scan(&memberCount)

	if err != nil {
		return fmt.Errorf("failed to count workspace members: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE read_schema.workspace_projections 
		SET member_count = $1
		WHERE id = $2
	`, memberCount, workspaceID)

	return err
}

// updateTenantWorkspaceCounts updates tenant summary with current workspace counts
func (h *WorkspaceProjectionHandler) updateTenantWorkspaceCounts(ctx context.Context, tenantID string, tx *sql.Tx) error {
	var totalWorkspaces int
	err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) 
		FROM read_schema.workspace_projections 
		WHERE tenant_id = $1 AND status != 'deleted'
	`, tenantID).Scan(&totalWorkspaces)

	if err != nil {
		return fmt.Errorf("failed to count workspaces for tenant: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO read_schema.tenant_summary_projections (
			tenant_id, name, total_workspaces, created_at, updated_at
		) VALUES ($1, 'Unknown', $2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (tenant_id) DO UPDATE SET
			total_workspaces = EXCLUDED.total_workspaces,
			updated_at = CURRENT_TIMESTAMP
	`, tenantID, totalWorkspaces)

	return err
}