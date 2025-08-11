package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pyairtable/go-services/file-processing-service/internal/models"
)

// ProgressTracker manages WebSocket connections and progress updates
type ProgressTracker struct {
	connections map[string]map[*websocket.Conn]bool // fileID -> connections
	mu          sync.RWMutex
	upgrader    websocket.Upgrader
	config      *models.WebSocketConfig
	hub         *Hub
}

// Hub manages WebSocket connections
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

// Client represents a WebSocket client
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	fileID string
	id     string
}

// Message represents a WebSocket message
type Message struct {
	Type      string                    `json:"type"`
	FileID    string                    `json:"file_id"`
	Timestamp time.Time                 `json:"timestamp"`
	Data      interface{}               `json:"data"`
	Progress  *models.ProcessingProgress `json:"progress,omitempty"`
	Error     string                    `json:"error,omitempty"`
}

// NewProgressTracker creates a new progress tracker
func NewProgressTracker(config *models.WebSocketConfig) *ProgressTracker {
	upgrader := websocket.Upgrader{
		ReadBufferSize:    config.ReadBufferSize,
		WriteBufferSize:   config.WriteBufferSize,
		HandshakeTimeout:  config.HandshakeTimeout,
		CheckOrigin: func(r *http.Request) bool {
			return !config.CheckOrigin || true // Implement proper origin checking
		},
		EnableCompression: config.EnableCompression,
	}

	hub := &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}

	tracker := &ProgressTracker{
		connections: make(map[string]map[*websocket.Conn]bool),
		upgrader:    upgrader,
		config:      config,
		hub:         hub,
	}

	// Start the hub
	go hub.run()

	return tracker
}

// HandleWebSocket handles WebSocket connections
func (pt *ProgressTracker) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	fileID := r.URL.Query().Get("file_id")
	if fileID == "" {
		http.Error(w, "file_id parameter is required", http.StatusBadRequest)
		return
	}

	conn, err := pt.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket connection: %v", err)
		return
	}

	client := &Client{
		hub:    pt.hub,
		conn:   conn,
		send:   make(chan []byte, 256),
		fileID: fileID,
		id:     generateClientID(),
	}

	// Register client
	pt.hub.register <- client

	// Start goroutines for this client
	go client.readPump()
	go client.writePump()

	// Send welcome message
	welcomeMsg := Message{
		Type:      "connected",
		FileID:    fileID,
		Timestamp: time.Now(),
		Data:      map[string]string{"message": "Connected to progress tracking"},
	}
	pt.sendToClient(client, welcomeMsg)
}

// UpdateProgress updates progress for a specific file
func (pt *ProgressTracker) UpdateProgress(fileID string, progress models.ProcessingProgress) {
	message := Message{
		Type:      "progress",
		FileID:    fileID,
		Timestamp: time.Now(),
		Progress:  &progress,
	}

	pt.broadcastToFile(fileID, message)
}

// NotifyError sends an error notification
func (pt *ProgressTracker) NotifyError(fileID string, err error) {
	message := Message{
		Type:      "error",
		FileID:    fileID,
		Timestamp: time.Now(),
		Error:     err.Error(),
	}

	pt.broadcastToFile(fileID, message)
}

// NotifyCompletion sends a completion notification
func (pt *ProgressTracker) NotifyCompletion(fileID string, result *models.ProcessingResult) {
	message := Message{
		Type:      "completed",
		FileID:    fileID,
		Timestamp: time.Now(),
		Data:      result,
	}

	pt.broadcastToFile(fileID, message)
}

// NotifyStatusChange sends a status change notification
func (pt *ProgressTracker) NotifyStatusChange(fileID string, oldStatus, newStatus models.ProcessingStatus) {
	message := Message{
		Type:      "status_change",
		FileID:    fileID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"old_status": oldStatus,
			"new_status": newStatus,
		},
	}

	pt.broadcastToFile(fileID, message)
}

// broadcastToFile sends a message to all clients tracking a specific file
func (pt *ProgressTracker) broadcastToFile(fileID string, message Message) {
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return
	}

	pt.hub.mu.RLock()
	defer pt.hub.mu.RUnlock()

	for client := range pt.hub.clients {
		if client.fileID == fileID {
			select {
			case client.send <- data:
			default:
				close(client.send)
				delete(pt.hub.clients, client)
			}
		}
	}
}

// sendToClient sends a message to a specific client
func (pt *ProgressTracker) sendToClient(client *Client, message Message) {
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return
	}

	select {
	case client.send <- data:
	default:
		close(client.send)
		delete(pt.hub.clients, client)
	}
}

// GetActiveConnections returns the number of active connections for a file
func (pt *ProgressTracker) GetActiveConnections(fileID string) int {
	pt.hub.mu.RLock()
	defer pt.hub.mu.RUnlock()

	count := 0
	for client := range pt.hub.clients {
		if client.fileID == fileID {
			count++
		}
	}
	return count
}

// GetTotalConnections returns the total number of active connections
func (pt *ProgressTracker) GetTotalConnections() int {
	pt.hub.mu.RLock()
	defer pt.hub.mu.RUnlock()
	return len(pt.hub.clients)
}

// Close gracefully closes all connections
func (pt *ProgressTracker) Close() error {
	pt.hub.mu.Lock()
	defer pt.hub.mu.Unlock()

	for client := range pt.hub.clients {
		client.conn.Close()
	}
	
	close(pt.hub.broadcast)
	close(pt.hub.register)
	close(pt.hub.unregister)
	
	return nil
}

// Hub methods

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("Client connected for file %s", client.fileID)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("Client disconnected for file %s", client.fileID)

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Client methods

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages to the current WebSocket message
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Helper functions

func generateClientID() string {
	return fmt.Sprintf("client_%d", time.Now().UnixNano())
}

// ProgressUpdate represents a progress update that can be sent via WebSocket
type ProgressUpdate struct {
	FileID      string    `json:"file_id"`
	Step        string    `json:"step"`
	Progress    float64   `json:"progress"`
	Message     string    `json:"message"`
	Timestamp   time.Time `json:"timestamp"`
	Details     map[string]interface{} `json:"details,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// BatchProgressUpdates groups multiple progress updates for efficient transmission
type BatchProgressUpdates struct {
	Updates []ProgressUpdate `json:"updates"`
	Count   int              `json:"count"`
}

// ProgressBufferedTracker provides buffered progress updates to reduce WebSocket traffic
type ProgressBufferedTracker struct {
	*ProgressTracker
	buffer     map[string][]ProgressUpdate
	bufferSize int
	flushTimer *time.Timer
	mu         sync.Mutex
}

// NewProgressBufferedTracker creates a buffered progress tracker
func NewProgressBufferedTracker(config *models.WebSocketConfig, bufferSize int, flushInterval time.Duration) *ProgressBufferedTracker {
	tracker := NewProgressTracker(config)
	
	buffered := &ProgressBufferedTracker{
		ProgressTracker: tracker,
		buffer:         make(map[string][]ProgressUpdate),
		bufferSize:     bufferSize,
	}
	
	// Start flush timer
	buffered.flushTimer = time.AfterFunc(flushInterval, buffered.flushBuffers)
	
	return buffered
}

// UpdateProgressBuffered adds a progress update to the buffer
func (pbt *ProgressBufferedTracker) UpdateProgressBuffered(fileID string, progress models.ProcessingProgress) {
	pbt.mu.Lock()
	defer pbt.mu.Unlock()
	
	update := ProgressUpdate{
		FileID:    fileID,
		Step:      string(progress.Step),
		Progress:  progress.Progress,
		Message:   progress.Message,
		Timestamp: time.Now(),
		Error:     progress.Error,
	}
	
	pbt.buffer[fileID] = append(pbt.buffer[fileID], update)
	
	// Flush if buffer is full
	if len(pbt.buffer[fileID]) >= pbt.bufferSize {
		pbt.flushBufferForFile(fileID)
	}
}

// flushBuffers flushes all buffered updates
func (pbt *ProgressBufferedTracker) flushBuffers() {
	pbt.mu.Lock()
	defer pbt.mu.Unlock()
	
	for fileID := range pbt.buffer {
		pbt.flushBufferForFile(fileID)
	}
	
	// Reset timer
	pbt.flushTimer.Reset(5 * time.Second)
}

// flushBufferForFile flushes buffered updates for a specific file
func (pbt *ProgressBufferedTracker) flushBufferForFile(fileID string) {
	updates := pbt.buffer[fileID]
	if len(updates) == 0 {
		return
	}
	
	batch := BatchProgressUpdates{
		Updates: updates,
		Count:   len(updates),
	}
	
	message := Message{
		Type:      "batch_progress",
		FileID:    fileID,
		Timestamp: time.Now(),
		Data:      batch,
	}
	
	pbt.broadcastToFile(fileID, message)
	
	// Clear buffer
	pbt.buffer[fileID] = nil
}