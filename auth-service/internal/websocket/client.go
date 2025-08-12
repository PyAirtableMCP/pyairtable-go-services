package websocket

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// In production, you should check the origin
		return true
	},
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	// The hub.
	hub *Hub

	// User ID of the connected user
	UserID string

	// Table ID the user is currently viewing
	TableID string

	// Unique client ID
	ID string

	logger *zap.Logger
}

// NewClient creates a new WebSocket client
func NewClient(conn *websocket.Conn, hub *Hub, userID string, logger *zap.Logger) *Client {
	return &Client{
		conn:    conn,
		send:    make(chan []byte, 256),
		hub:     hub,
		UserID:  userID,
		ID:      uuid.New().String(),
		logger:  logger,
	}
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, messageBytes, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Error("WebSocket error", zap.Error(err))
			}
			break
		}

		// Handle incoming messages
		c.handleMessage(messageBytes)
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message.
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes incoming WebSocket messages
func (c *Client) handleMessage(messageBytes []byte) {
	var message IncomingMessage
	if err := json.Unmarshal(messageBytes, &message); err != nil {
		c.logger.Error("Failed to unmarshal message", zap.Error(err))
		return
	}

	switch message.Type {
	case "join_table":
		if tableID, ok := message.Payload["tableId"].(string); ok {
			c.hub.AddClientToTable(c, tableID)
			c.sendPresenceUpdate(tableID)
		}

	case "leave_table":
		if c.TableID != "" {
			c.hub.removeClientFromTable(c, c.TableID)
			c.TableID = ""
		}

	case "ping":
		c.sendMessage(OutgoingMessage{
			Type:      "pong",
			Timestamp: getCurrentTimestamp(),
		})

	default:
		c.logger.Warn("Unknown message type", zap.String("type", message.Type))
	}
}

// sendMessage sends a message to the client
func (c *Client) sendMessage(message OutgoingMessage) {
	messageBytes, err := json.Marshal(message)
	if err != nil {
		c.logger.Error("Failed to marshal message", zap.Error(err))
		return
	}

	select {
	case c.send <- messageBytes:
	default:
		c.logger.Warn("Client send channel full, dropping message")
	}
}

// sendPresenceUpdate sends the current presence information for a table
func (c *Client) sendPresenceUpdate(tableID string) {
	users := c.hub.GetTablePresence(tableID)
	message := OutgoingMessage{
		Type: "presence_update",
		Payload: PresenceUpdatePayload{
			TableID: tableID,
			Users:   users,
		},
		Timestamp: getCurrentTimestamp(),
	}
	c.sendMessage(message)
}

// ServeWS handles websocket requests from the peer.
func ServeWS(hub *Hub, w http.ResponseWriter, r *http.Request, userID string, logger *zap.Logger) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}

	client := NewClient(conn, hub, userID, logger)
	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}