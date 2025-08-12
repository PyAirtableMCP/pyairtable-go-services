package websocket

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// Handler handles WebSocket-related HTTP requests
type Handler struct {
	hub    *Hub
	logger *zap.Logger
}

// NewHandler creates a new WebSocket handler
func NewHandler(hub *Hub, logger *zap.Logger) *Handler {
	return &Handler{
		hub:    hub,
		logger: logger,
	}
}

// HandleWebSocketUpgrade handles WebSocket upgrade requests
func (h *Handler) HandleWebSocketUpgrade(c *fiber.Ctx) error {
	// Extract user ID from JWT token or query params
	userID := h.extractUserID(c)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Authentication required",
		})
	}

	// Convert Fiber context to standard HTTP
	return c.Status(fiber.StatusUpgradeRequired).JSON(fiber.Map{
		"error": "WebSocket upgrade required",
		"upgrade_url": "/ws",
	})
}

// HandleWebSocketConnection handles the actual WebSocket connection
func (h *Handler) HandleWebSocketConnection(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from query parameters or headers
	userID := h.extractUserIDFromHTTP(r)
	if userID == "" {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	h.logger.Info("WebSocket connection attempt", zap.String("userID", userID))

	// Upgrade the connection and create client
	ServeWS(h.hub, w, r, userID, h.logger)
}

// GetConnectionStats returns current connection statistics
func (h *Handler) GetConnectionStats(c *fiber.Ctx) error {
	h.hub.mutex.RLock()
	defer h.hub.mutex.RUnlock()

	stats := ConnectionStats{
		TotalConnections:  len(h.hub.clients),
		UserConnections:   make(map[string]int),
		TableConnections:  make(map[string]int),
		ConnectionsByUser: make(map[string][]string),
	}

	// Count connections per user
	for client := range h.hub.clients {
		if client.UserID != "" {
			stats.UserConnections[client.UserID]++
			stats.ConnectionsByUser[client.UserID] = append(
				stats.ConnectionsByUser[client.UserID], 
				client.ID,
			)
		}
	}

	// Count connections per table
	for tableID, clients := range h.hub.tablePresence {
		stats.TableConnections[tableID] = len(clients)
	}

	return c.JSON(stats)
}

// BroadcastMessage broadcasts a message to specific targets
func (h *Handler) BroadcastMessage(c *fiber.Ctx) error {
	var request struct {
		Type     string      `json:"type"`
		Target   string      `json:"target"`   // "all", "table", "user"
		TargetID string      `json:"targetId"` // tableID or userID
		Payload  interface{} `json:"payload"`
	}

	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	event := Event{
		Type:      request.Type,
		Payload:   request.Payload,
		Timestamp: getCurrentTimestamp(),
	}

	eventBytes, err := json.Marshal(event)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to marshal event",
		})
	}

	switch request.Target {
	case "all":
		select {
		case h.hub.broadcast <- eventBytes:
		default:
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "Broadcast queue full",
			})
		}
	case "table":
		if request.TargetID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "tableId required for table broadcast",
			})
		}
		h.hub.BroadcastToTable(request.TargetID, eventBytes)
	case "user":
		if request.TargetID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "userId required for user broadcast",
			})
		}
		h.hub.BroadcastToUser(request.TargetID, eventBytes)
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid target type",
		})
	}

	return c.JSON(fiber.Map{
		"status": "Message broadcasted successfully",
	})
}

// GetTablePresence returns users currently viewing a table
func (h *Handler) GetTablePresence(c *fiber.Ctx) error {
	tableID := c.Params("tableId")
	if tableID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "tableId is required",
		})
	}

	users := h.hub.GetTablePresence(tableID)
	
	return c.JSON(fiber.Map{
		"tableId": tableID,
		"users":   users,
		"count":   len(users),
	})
}

// extractUserID extracts user ID from Fiber context
func (h *Handler) extractUserID(c *fiber.Ctx) string {
	// Try to get from query params first
	userID := c.Query("userId")
	if userID != "" {
		return userID
	}

	// Try to extract from JWT token in Authorization header
	authHeader := c.Get("Authorization")
	if authHeader != "" {
		// This is a simplified extraction - in production, properly validate JWT
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token != authHeader {
			// For now, return a placeholder - in production, decode JWT
			return h.extractUserIDFromToken(token)
		}
	}

	return ""
}

// extractUserIDFromHTTP extracts user ID from standard HTTP request
func (h *Handler) extractUserIDFromHTTP(r *http.Request) string {
	// Try query params first
	userID := r.URL.Query().Get("userId")
	if userID != "" {
		return userID
	}

	// Try Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token != authHeader {
			return h.extractUserIDFromToken(token)
		}
	}

	return ""
}

// extractUserIDFromToken extracts user ID from JWT token
func (h *Handler) extractUserIDFromToken(token string) string {
	// TODO: Implement proper JWT validation and user ID extraction
	// For now, this is a placeholder that should be replaced with actual JWT validation
	// using the same JWT secret as the auth service
	
	h.logger.Warn("JWT validation not implemented in WebSocket handler",
		zap.String("token_prefix", token[:min(len(token), 20)]))
	
	// Return empty string to force authentication failure
	return ""
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}