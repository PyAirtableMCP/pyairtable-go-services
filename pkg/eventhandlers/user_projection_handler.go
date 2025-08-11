package eventhandlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"go.uber.org/zap"
)

// UserProjectionHandler handles user events and updates read model projections
type UserProjectionHandler struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewUserProjectionHandler creates a new user projection handler
func NewUserProjectionHandler(db *sql.DB, logger *zap.Logger) *UserProjectionHandler {
	return &UserProjectionHandler{
		db:     db,
		logger: logger,
	}
}

// Handle processes user events and updates projections
func (h *UserProjectionHandler) Handle(event shared.DomainEvent) error {
	h.logger.Info("Processing user event for projection", 
		zap.String("event_type", event.EventType()),
		zap.String("event_id", event.EventID()),
		zap.String("aggregate_id", event.AggregateID()),
	)

	switch event.EventType() {
	case shared.EventTypeUserRegistered:
		return h.handleUserRegistered(event)
	case shared.EventTypeUserActivated:
		return h.handleUserActivated(event)
	case shared.EventTypeUserDeactivated:
		return h.handleUserDeactivated(event)
	case shared.EventTypeUserUpdated:
		return h.handleUserUpdated(event)
	case shared.EventTypeUserRoleChanged:
		return h.handleUserRoleChanged(event)
	default:
		h.logger.Debug("Unhandled event type for user projection", 
			zap.String("event_type", event.EventType()),
		)
		return nil
	}
}

// CanHandle returns true if this handler can process the given event type
func (h *UserProjectionHandler) CanHandle(eventType string) bool {
	switch eventType {
	case shared.EventTypeUserRegistered,
		 shared.EventTypeUserActivated,
		 shared.EventTypeUserDeactivated,
		 shared.EventTypeUserUpdated,
		 shared.EventTypeUserRoleChanged:
		return true
	default:
		return false
	}
}

// handleUserRegistered creates a new user record in the read model
func (h *UserProjectionHandler) handleUserRegistered(event shared.DomainEvent) error {
	payload := event.Payload().(map[string]interface{})
	
	userID := payload["user_id"].(string)
	email := payload["email"].(string)
	firstName := payload["first_name"].(string)
	lastName := payload["last_name"].(string)
	tenantID := payload["tenant_id"].(string)

	// Create user read model record
	_, err := h.db.Exec(`
		INSERT INTO user_projections (
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
		fmt.Sprintf("%s %s", firstName, lastName),
		tenantID,
		"pending", // UserStatus
		false,     // email_verified
		false,     // is_active
		event.OccurredAt(),
		event.OccurredAt(),
		event.Version(),
	)

	if err != nil {
		return fmt.Errorf("failed to insert user projection: %w", err)
	}

	h.logger.Info("User projection created", 
		zap.String("user_id", userID),
		zap.String("email", email),
	)

	return nil
}

// handleUserActivated updates user status to active
func (h *UserProjectionHandler) handleUserActivated(event shared.DomainEvent) error {
	payload := event.Payload().(map[string]interface{})
	userID := payload["user_id"].(string)
	status := payload["status"].(string)

	_, err := h.db.Exec(`
		UPDATE user_projections 
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

	h.logger.Info("User projection activated", 
		zap.String("user_id", userID),
	)

	return nil
}

// handleUserDeactivated updates user status to deactivated
func (h *UserProjectionHandler) handleUserDeactivated(event shared.DomainEvent) error {
	payload := event.Payload().(map[string]interface{})
	userID := payload["user_id"].(string)
	status := payload["status"].(string)
	reason := ""
	if r, ok := payload["reason"].(string); ok {
		reason = r
	}

	_, err := h.db.Exec(`
		UPDATE user_projections 
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

	h.logger.Info("User projection deactivated", 
		zap.String("user_id", userID),
		zap.String("reason", reason),
	)

	return nil
}

// handleUserUpdated updates user profile information
func (h *UserProjectionHandler) handleUserUpdated(event shared.DomainEvent) error {
	payload := event.Payload().(map[string]interface{})
	userID := payload["user_id"].(string)

	// Build dynamic update query based on available fields
	updates := []string{}
	args := []interface{}{}
	argIndex := 1

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

	if len(updates) == 0 {
		return nil // Nothing to update
	}

	// Always update display_name if first_name or last_name changed
	if firstName, hasFirst := payload["first_name"].(string); hasFirst {
		if lastName, hasLast := payload["last_name"].(string); hasLast {
			updates = append(updates, fmt.Sprintf("display_name = $%d", argIndex))
			args = append(args, fmt.Sprintf("%s %s", firstName, lastName))
			argIndex++
		}
	}

	// Add standard update fields
	updates = append(updates, fmt.Sprintf("updated_at = $%d", argIndex))
	args = append(args, event.OccurredAt())
	argIndex++

	updates = append(updates, fmt.Sprintf("event_version = $%d", argIndex))
	args = append(args, event.Version())
	argIndex++

	// Add WHERE conditions
	args = append(args, userID, event.Version())

	query := fmt.Sprintf(`
		UPDATE user_projections 
		SET %s
		WHERE id = $%d AND event_version < $%d
	`, 
		fmt.Sprintf("%s", updates[0]),
		argIndex-1,
		argIndex,
	)

	// Handle multiple updates
	if len(updates) > 1 {
		query = fmt.Sprintf(`
			UPDATE user_projections 
			SET %s
			WHERE id = $%d AND event_version < $%d
		`, 
			fmt.Sprintf("%s", updates[0])+", "+fmt.Sprintf("%s", updates[1:]),
			argIndex-1,
			argIndex,
		)
	}

	_, err := h.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update user projection: %w", err)
	}

	h.logger.Info("User projection updated", 
		zap.String("user_id", userID),
		zap.Int("fields_updated", len(updates)-2), // Exclude updated_at and event_version
	)

	return nil
}

// handleUserRoleChanged updates user roles
func (h *UserProjectionHandler) handleUserRoleChanged(event shared.DomainEvent) error {
	payload := event.Payload().(map[string]interface{})
	userID := payload["user_id"].(string)
	role := payload["role"].(string)
	action := payload["action"].(string)

	// Get current roles
	var rolesJSON sql.NullString
	err := h.db.QueryRow(`
		SELECT roles FROM user_projections WHERE id = $1
	`, userID).Scan(&rolesJSON)

	if err != nil {
		return fmt.Errorf("failed to get current roles: %w", err)
	}

	var roles []string
	if rolesJSON.Valid && rolesJSON.String != "" {
		if err := json.Unmarshal([]byte(rolesJSON.String), &roles); err != nil {
			h.logger.Warn("Failed to unmarshal existing roles, starting fresh", 
				zap.Error(err),
				zap.String("user_id", userID),
			)
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
	_, err = h.db.Exec(`
		UPDATE user_projections 
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

	h.logger.Info("User roles updated", 
		zap.String("user_id", userID),
		zap.String("role", role),
		zap.String("action", action),
		zap.Int("total_roles", len(roles)),
	)

	return nil
}