package cqrs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"github.com/pyairtable-compose/go-services/domain/shared"
)

// UserProjectionHandler implements ProjectionHandler for user events
type UserProjectionHandler struct {
	readDB *sql.DB
}

// NewUserProjectionHandler creates a new user projection handler
func NewUserProjectionHandler(readDB *sql.DB) *UserProjectionHandler {
	return &UserProjectionHandler{
		readDB: readDB,
	}
}

// GetName returns the handler name
func (h *UserProjectionHandler) GetName() string {
	return "user_projection"
}

// GetEventTypes returns the event types this handler processes
func (h *UserProjectionHandler) GetEventTypes() []string {
	return []string{
		shared.EventTypeUserRegistered,
		shared.EventTypeUserActivated,
		shared.EventTypeUserDeactivated,
		shared.EventTypeUserUpdated,
		shared.EventTypeUserRoleChanged,
	}
}

// Handle processes user events and updates read model projections
func (h *UserProjectionHandler) Handle(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error {
	log.Printf("Processing user event: %s for aggregate: %s", event.EventType(), event.AggregateID())

	switch event.EventType() {
	case shared.EventTypeUserRegistered:
		return h.handleUserRegistered(ctx, event, tx)
	case shared.EventTypeUserActivated:
		return h.handleUserActivated(ctx, event, tx)
	case shared.EventTypeUserDeactivated:
		return h.handleUserDeactivated(ctx, event, tx)
	case shared.EventTypeUserUpdated:
		return h.handleUserUpdated(ctx, event, tx)
	case shared.EventTypeUserRoleChanged:
		return h.handleUserRoleChanged(ctx, event, tx)
	default:
		return fmt.Errorf("unsupported event type: %s", event.EventType())
	}
}

// GetCheckpoint retrieves the last processed checkpoint for this handler
func (h *UserProjectionHandler) GetCheckpoint(ctx context.Context, db *sql.DB) (string, error) {
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
func (h *UserProjectionHandler) UpdateCheckpoint(ctx context.Context, eventID string, tx *sql.Tx) error {
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

// handleUserRegistered creates a new user record in the read model
func (h *UserProjectionHandler) handleUserRegistered(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error {
	payload := event.Payload().(map[string]interface{})

	userID := payload["user_id"].(string)
	email := payload["email"].(string)
	firstName, _ := payload["first_name"].(string)
	lastName, _ := payload["last_name"].(string)
	tenantID := payload["tenant_id"].(string)

	displayName := fmt.Sprintf("%s %s", firstName, lastName)
	if firstName == "" && lastName == "" {
		displayName = email
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO read_schema.user_projections (
			id, email, first_name, last_name, display_name, 
			tenant_id, status, email_verified, is_active,
			created_at, updated_at, event_version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			email = EXCLUDED.email,
			first_name = EXCLUDED.first_name,
			last_name = EXCLUDED.last_name,
			display_name = EXCLUDED.display_name,
			updated_at = EXCLUDED.updated_at,
			event_version = EXCLUDED.event_version
		WHERE user_projections.event_version < EXCLUDED.event_version
	`,
		userID,
		email,
		firstName,
		lastName,
		displayName,
		tenantID,
		"pending",
		false,
		false,
		event.OccurredAt(),
		event.OccurredAt(),
		event.Version(),
	)

	if err != nil {
		return fmt.Errorf("failed to insert user projection: %w", err)
	}

	// Update tenant summary
	err = h.updateTenantUserCounts(ctx, tenantID, tx)
	if err != nil {
		log.Printf("Failed to update tenant user counts: %v", err)
		// Don't fail the entire operation for summary updates
	}

	log.Printf("User projection created: %s (%s)", userID, email)
	return nil
}

// handleUserActivated updates user status to active
func (h *UserProjectionHandler) handleUserActivated(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error {
	payload := event.Payload().(map[string]interface{})
	userID := payload["user_id"].(string)
	status, _ := payload["status"].(string)
	if status == "" {
		status = "active"
	}

	_, err := tx.ExecContext(ctx, `
		UPDATE read_schema.user_projections 
		SET status = $1, is_active = $2, updated_at = $3, event_version = $4
		WHERE id = $5 AND event_version < $4
	`,
		status,
		true,
		event.OccurredAt(),
		event.Version(),
		userID,
	)

	if err != nil {
		return fmt.Errorf("failed to update user activation: %w", err)
	}

	// Update tenant active user count
	var tenantID string
	err = tx.QueryRowContext(ctx, "SELECT tenant_id FROM read_schema.user_projections WHERE id = $1", userID).Scan(&tenantID)
	if err == nil {
		h.updateTenantUserCounts(ctx, tenantID, tx)
	}

	log.Printf("User projection activated: %s", userID)
	return nil
}

// handleUserDeactivated updates user status to deactivated
func (h *UserProjectionHandler) handleUserDeactivated(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error {
	payload := event.Payload().(map[string]interface{})
	userID := payload["user_id"].(string)
	status, _ := payload["status"].(string)
	if status == "" {
		status = "deactivated"
	}
	reason, _ := payload["reason"].(string)

	_, err := tx.ExecContext(ctx, `
		UPDATE read_schema.user_projections 
		SET status = $1, is_active = $2, deactivation_reason = $3, 
			updated_at = $4, event_version = $5
		WHERE id = $6 AND event_version < $5
	`,
		status,
		false,
		reason,
		event.OccurredAt(),
		event.Version(),
		userID,
	)

	if err != nil {
		return fmt.Errorf("failed to update user deactivation: %w", err)
	}

	// Update tenant active user count
	var tenantID string
	err = tx.QueryRowContext(ctx, "SELECT tenant_id FROM read_schema.user_projections WHERE id = $1", userID).Scan(&tenantID)
	if err == nil {
		h.updateTenantUserCounts(ctx, tenantID, tx)
	}

	log.Printf("User projection deactivated: %s (reason: %s)", userID, reason)
	return nil
}

// handleUserUpdated updates user profile information
func (h *UserProjectionHandler) handleUserUpdated(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error {
	payload := event.Payload().(map[string]interface{})
	userID := payload["user_id"].(string)

	// Build dynamic update query based on available fields
	updates := []string{"updated_at = $1", "event_version = $2"}
	args := []interface{}{event.OccurredAt(), event.Version()}
	argIndex := 3

	if firstName, ok := payload["first_name"].(string); ok {
		updates = append(updates, fmt.Sprintf("first_name = $%d", argIndex))
		args = append(args, firstName)
		argIndex++
	}

	if lastName, ok := payload["last_name"].(string); ok {
		updates = append(updates, fmt.Sprintf("last_name = $%d", argIndex))
		args = append(args, lastName)
		argIndex++
	}

	if avatar, ok := payload["avatar"].(string); ok {
		updates = append(updates, fmt.Sprintf("avatar = $%d", argIndex))
		args = append(args, avatar)
		argIndex++
	}

	if timezone, ok := payload["timezone"].(string); ok {
		updates = append(updates, fmt.Sprintf("timezone = $%d", argIndex))
		args = append(args, timezone)
		argIndex++
	}

	if language, ok := payload["language"].(string); ok {
		updates = append(updates, fmt.Sprintf("language = $%d", argIndex))
		args = append(args, language)
		argIndex++
	}

	// Update display_name if first_name or last_name changed
	if firstName, hasFirst := payload["first_name"].(string); hasFirst {
		if lastName, hasLast := payload["last_name"].(string); hasLast {
			updates = append(updates, fmt.Sprintf("display_name = $%d", argIndex))
			args = append(args, fmt.Sprintf("%s %s", firstName, lastName))
			argIndex++
		}
	}

	// Add WHERE conditions
	args = append(args, userID, event.Version())
	whereClause := fmt.Sprintf("WHERE id = $%d AND event_version < $%d", argIndex-1, argIndex)

	query := fmt.Sprintf(`
		UPDATE read_schema.user_projections 
		SET %s
		%s
	`, fmt.Sprintf("%s", updates[0]), whereClause)

	// Handle multiple updates properly
	if len(updates) > 1 {
		updateStr := ""
		for i, update := range updates {
			if i > 0 {
				updateStr += ", "
			}
			updateStr += update
		}
		query = fmt.Sprintf(`
			UPDATE read_schema.user_projections 
			SET %s
			%s
		`, updateStr, whereClause)
	}

	_, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update user projection: %w", err)
	}

	log.Printf("User projection updated: %s (%d fields)", userID, len(updates)-2)
	return nil
}

// handleUserRoleChanged updates user roles
func (h *UserProjectionHandler) handleUserRoleChanged(ctx context.Context, event shared.DomainEvent, tx *sql.Tx) error {
	payload := event.Payload().(map[string]interface{})
	userID := payload["user_id"].(string)
	role := payload["role"].(string)
	action := payload["action"].(string)

	// Get current roles
	var rolesJSON sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT roles FROM read_schema.user_projections WHERE id = $1
	`, userID).Scan(&rolesJSON)

	if err != nil {
		return fmt.Errorf("failed to get current roles: %w", err)
	}

	var roles []string
	if rolesJSON.Valid && rolesJSON.String != "" {
		if err := json.Unmarshal([]byte(rolesJSON.String), &roles); err != nil {
			log.Printf("Failed to unmarshal existing roles for user %s: %v", userID, err)
			roles = []string{}
		}
	}

	// Update roles based on action
	switch action {
	case "added":
		// Check if role already exists
		exists := false
		for _, r := range roles {
			if r == role {
				exists = true
				break
			}
		}
		if !exists {
			roles = append(roles, role)
		}
	case "removed":
		// Remove role from slice
		for i, r := range roles {
			if r == role {
				roles = append(roles[:i], roles[i+1:]...)
				break
			}
		}
	}

	// Marshal updated roles
	rolesBytes, err := json.Marshal(roles)
	if err != nil {
		return fmt.Errorf("failed to marshal roles: %w", err)
	}

	// Update database
	_, err = tx.ExecContext(ctx, `
		UPDATE read_schema.user_projections 
		SET roles = $1, updated_at = $2, event_version = $3
		WHERE id = $4 AND event_version < $3
	`,
		string(rolesBytes),
		event.OccurredAt(),
		event.Version(),
		userID,
	)

	if err != nil {
		return fmt.Errorf("failed to update user roles: %w", err)
	}

	log.Printf("User roles updated: %s - %s %s (total: %d)", userID, action, role, len(roles))
	return nil
}

// updateTenantUserCounts updates tenant summary with current user counts
func (h *UserProjectionHandler) updateTenantUserCounts(ctx context.Context, tenantID string, tx *sql.Tx) error {
	var totalUsers, activeUsers int
	err := tx.QueryRowContext(ctx, `
		SELECT 
			COUNT(*) as total_users,
			COUNT(*) FILTER (WHERE is_active = true) as active_users
		FROM read_schema.user_projections 
		WHERE tenant_id = $1
	`, tenantID).Scan(&totalUsers, &activeUsers)

	if err != nil {
		return fmt.Errorf("failed to count users for tenant: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO read_schema.tenant_summary_projections (
			tenant_id, name, total_users, active_users, created_at, updated_at
		) VALUES ($1, 'Unknown', $2, $3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (tenant_id) DO UPDATE SET
			total_users = EXCLUDED.total_users,
			active_users = EXCLUDED.active_users,
			updated_at = CURRENT_TIMESTAMP
	`, tenantID, totalUsers, activeUsers)

	return err
}