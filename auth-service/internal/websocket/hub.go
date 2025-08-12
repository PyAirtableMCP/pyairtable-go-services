package websocket

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// Hub maintains the set of active clients and broadcasts messages to the clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	broadcast chan []byte

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	// User presence tracking - maps userID to list of clients
	userPresence map[string][]*Client

	// Table presence tracking - maps tableID to list of clients
	tablePresence map[string][]*Client

	logger *zap.Logger
	mutex  sync.RWMutex
}

// NewHub creates a new WebSocket hub
func NewHub(logger *zap.Logger) *Hub {
	return &Hub{
		clients:       make(map[*Client]bool),
		broadcast:     make(chan []byte, 256),
		register:      make(chan *Client),
		unregister:    make(chan *Client),
		userPresence:  make(map[string][]*Client),
		tablePresence: make(map[string][]*Client),
		logger:        logger,
	}
}

// Run starts the hub's event loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case message := <-h.broadcast:
			h.broadcastMessage(message)
		}
	}
}

// registerClient adds a client to the hub
func (h *Hub) registerClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.clients[client] = true
	
	// Add to user presence
	if client.UserID != "" {
		h.userPresence[client.UserID] = append(h.userPresence[client.UserID], client)
	}

	h.logger.Info("Client registered", 
		zap.String("userID", client.UserID),
		zap.String("clientID", client.ID),
		zap.Int("totalClients", len(h.clients)))

	// Broadcast user joined event
	if client.UserID != "" {
		h.broadcastUserEvent("user:joined", client.UserID, client.TableID)
	}
}

// unregisterClient removes a client from the hub
func (h *Hub) unregisterClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)

		// Remove from user presence
		if client.UserID != "" {
			clients := h.userPresence[client.UserID]
			for i, c := range clients {
				if c == client {
					h.userPresence[client.UserID] = append(clients[:i], clients[i+1:]...)
					break
				}
			}
			if len(h.userPresence[client.UserID]) == 0 {
				delete(h.userPresence, client.UserID)
			}
		}

		// Remove from table presence
		if client.TableID != "" {
			clients := h.tablePresence[client.TableID]
			for i, c := range clients {
				if c == client {
					h.tablePresence[client.TableID] = append(clients[:i], clients[i+1:]...)
					break
				}
			}
			if len(h.tablePresence[client.TableID]) == 0 {
				delete(h.tablePresence, client.TableID)
			}
		}

		h.logger.Info("Client unregistered", 
			zap.String("userID", client.UserID),
			zap.String("clientID", client.ID),
			zap.Int("totalClients", len(h.clients)))

		// Broadcast user left event
		if client.UserID != "" {
			h.broadcastUserEvent("user:left", client.UserID, client.TableID)
		}
	}
}

// broadcastMessage sends a message to all registered clients
func (h *Hub) broadcastMessage(message []byte) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	for client := range h.clients {
		select {
		case client.send <- message:
		default:
			close(client.send)
			delete(h.clients, client)
		}
	}
}

// BroadcastToTable sends a message to all clients viewing a specific table
func (h *Hub) BroadcastToTable(tableID string, message []byte) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	clients, exists := h.tablePresence[tableID]
	if !exists {
		return
	}

	for _, client := range clients {
		select {
		case client.send <- message:
		default:
			// Client's send channel is full, remove it
			close(client.send)
			delete(h.clients, client)
		}
	}
}

// BroadcastToUser sends a message to all clients for a specific user
func (h *Hub) BroadcastToUser(userID string, message []byte) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	clients, exists := h.userPresence[userID]
	if !exists {
		return
	}

	for _, client := range clients {
		select {
		case client.send <- message:
		default:
			// Client's send channel is full, remove it
			close(client.send)
			delete(h.clients, client)
		}
	}
}

// AddClientToTable adds a client to table presence tracking
func (h *Hub) AddClientToTable(client *Client, tableID string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Remove from previous table if any
	if client.TableID != "" && client.TableID != tableID {
		h.removeClientFromTable(client, client.TableID)
	}

	client.TableID = tableID
	h.tablePresence[tableID] = append(h.tablePresence[tableID], client)

	h.logger.Info("Client joined table", 
		zap.String("userID", client.UserID),
		zap.String("tableID", tableID))
}

// removeClientFromTable removes a client from table presence tracking
func (h *Hub) removeClientFromTable(client *Client, tableID string) {
	clients := h.tablePresence[tableID]
	for i, c := range clients {
		if c == client {
			h.tablePresence[tableID] = append(clients[:i], clients[i+1:]...)
			break
		}
	}
	if len(h.tablePresence[tableID]) == 0 {
		delete(h.tablePresence, tableID)
	}
}

// GetTablePresence returns the list of users viewing a table
func (h *Hub) GetTablePresence(tableID string) []string {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	clients := h.tablePresence[tableID]
	userMap := make(map[string]bool)
	for _, client := range clients {
		if client.UserID != "" {
			userMap[client.UserID] = true
		}
	}

	users := make([]string, 0, len(userMap))
	for userID := range userMap {
		users = append(users, userID)
	}
	return users
}

// broadcastUserEvent sends user presence events
func (h *Hub) broadcastUserEvent(eventType, userID, tableID string) {
	event := Event{
		Type: eventType,
		Payload: UserPresencePayload{
			UserID:  userID,
			TableID: tableID,
		},
		Timestamp: getCurrentTimestamp(),
	}

	eventBytes, err := json.Marshal(event)
	if err != nil {
		h.logger.Error("Failed to marshal user event", zap.Error(err))
		return
	}

	// Broadcast to all clients in the same table
	if tableID != "" {
		h.BroadcastToTable(tableID, eventBytes)
	} else {
		// Broadcast to all clients
		select {
		case h.broadcast <- eventBytes:
		default:
			h.logger.Warn("Broadcast channel full, dropping message")
		}
	}
}

// BroadcastRecordEvent sends record-related events
func (h *Hub) BroadcastRecordEvent(eventType, tableID, recordID string, data interface{}) {
	event := Event{
		Type: eventType,
		Payload: RecordEventPayload{
			TableID:  tableID,
			RecordID: recordID,
			Data:     data,
		},
		Timestamp: getCurrentTimestamp(),
	}

	eventBytes, err := json.Marshal(event)
	if err != nil {
		h.logger.Error("Failed to marshal record event", zap.Error(err))
		return
	}

	h.BroadcastToTable(tableID, eventBytes)
}