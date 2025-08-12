package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Event represents a WebSocket event
type Event struct {
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload"`
	Timestamp int64       `json:"timestamp"`
}

// IncomingMessage represents messages received from the server
type IncomingMessage struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// WebSocketTester simulates multiple WebSocket connections for testing
type WebSocketTester struct {
	connections []*websocket.Conn
	userIDs     []string
	tableID     string
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewWebSocketTester(numConnections int, tableID string) *WebSocketTester {
	ctx, cancel := context.WithCancel(context.Background())
	
	tester := &WebSocketTester{
		connections: make([]*websocket.Conn, numConnections),
		userIDs:     make([]string, numConnections),
		tableID:     tableID,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Generate user IDs
	for i := 0; i < numConnections; i++ {
		tester.userIDs[i] = fmt.Sprintf("testuser-%d", i+1)
	}

	return tester
}

func (wst *WebSocketTester) Connect() error {
	baseURL := "ws://localhost:8081/ws"
	
	for i := 0; i < len(wst.connections); i++ {
		userID := wst.userIDs[i]
		
		u, err := url.Parse(baseURL)
		if err != nil {
			return fmt.Errorf("failed to parse URL: %v", err)
		}
		
		// Add user ID to query parameters
		q := u.Query()
		q.Set("userId", userID)
		u.RawQuery = q.Encode()
		
		log.Printf("Connecting user %s to %s", userID, u.String())
		
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			return fmt.Errorf("failed to connect user %s: %v", userID, err)
		}
		
		wst.connections[i] = conn
		
		// Start message handler for this connection
		wst.wg.Add(1)
		go wst.handleMessages(conn, userID, i)
		
		// Join the test table
		joinMessage := map[string]interface{}{
			"type": "join_table",
			"payload": map[string]interface{}{
				"tableId": wst.tableID,
			},
		}
		
		if err := conn.WriteJSON(joinMessage); err != nil {
			log.Printf("Failed to join table for user %s: %v", userID, err)
		} else {
			log.Printf("User %s joined table %s", userID, wst.tableID)
		}
		
		// Small delay between connections
		time.Sleep(100 * time.Millisecond)
	}
	
	return nil
}

func (wst *WebSocketTester) handleMessages(conn *websocket.Conn, userID string, connIndex int) {
	defer wst.wg.Done()
	defer conn.Close()
	
	for {
		select {
		case <-wst.ctx.Done():
			return
		default:
			conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			
			var message IncomingMessage
			err := conn.ReadJSON(&message)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("User %s WebSocket error: %v", userID, err)
				}
				return
			}
			
			log.Printf("User %s received: %s", userID, message.Type)
			
			// Handle specific message types
			switch message.Type {
			case "presence_update":
				if users, ok := message.Payload["users"].([]interface{}); ok {
					log.Printf("User %s sees %d users in table", userID, len(users))
				}
			case "record:created":
				log.Printf("User %s notified of new record", userID)
			case "record:updated":
				log.Printf("User %s notified of record update", userID)
			case "record:deleted":
				log.Printf("User %s notified of record deletion", userID)
			case "user:joined":
				log.Printf("User %s notified that someone joined", userID)
			case "user:left":
				log.Printf("User %s notified that someone left", userID)
			}
		}
	}
}

func (wst *WebSocketTester) SimulateRecordOperations() {
	if len(wst.connections) == 0 {
		log.Println("No connections available for simulation")
		return
	}
	
	// Use the first connection to simulate operations
	conn := wst.connections[0]
	userID := wst.userIDs[0]
	
	log.Printf("Starting record operations simulation with user %s", userID)
	
	// Simulate record creation
	createEvent := Event{
		Type: "record:created",
		Payload: map[string]interface{}{
			"tableId":  wst.tableID,
			"recordId": fmt.Sprintf("rec%d", time.Now().Unix()),
			"data": map[string]interface{}{
				"id": fmt.Sprintf("rec%d", time.Now().Unix()),
				"fields": map[string]interface{}{
					"Name":   "Test Record",
					"Status": "Active",
				},
				"createdTime": time.Now().Format(time.RFC3339),
			},
		},
		Timestamp: time.Now().Unix(),
	}
	
	if err := conn.WriteJSON(createEvent); err != nil {
		log.Printf("Failed to send create event: %v", err)
	} else {
		log.Println("Sent record creation event")
	}
	
	time.Sleep(2 * time.Second)
	
	// Simulate record update
	updateEvent := Event{
		Type: "record:updated",
		Payload: map[string]interface{}{
			"tableId":  wst.tableID,
			"recordId": createEvent.Payload.(map[string]interface{})["recordId"],
			"data": map[string]interface{}{
				"fields": map[string]interface{}{
					"Status": "Updated",
				},
			},
		},
		Timestamp: time.Now().Unix(),
	}
	
	if err := conn.WriteJSON(updateEvent); err != nil {
		log.Printf("Failed to send update event: %v", err)
	} else {
		log.Println("Sent record update event")
	}
	
	time.Sleep(2 * time.Second)
	
	// Simulate record deletion
	deleteEvent := Event{
		Type: "record:deleted",
		Payload: map[string]interface{}{
			"tableId":  wst.tableID,
			"recordId": createEvent.Payload.(map[string]interface{})["recordId"],
		},
		Timestamp: time.Now().Unix(),
	}
	
	if err := conn.WriteJSON(deleteEvent); err != nil {
		log.Printf("Failed to send delete event: %v", err)
	} else {
		log.Println("Sent record deletion event")
	}
}

func (wst *WebSocketTester) TestConnectionStability() {
	log.Println("Testing connection stability...")
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for i := 0; i < 10; i++ {
		select {
		case <-wst.ctx.Done():
			return
		case <-ticker.C:
			// Send ping to all connections
			for j, conn := range wst.connections {
				if conn != nil {
					pingMessage := map[string]interface{}{
						"type":      "ping",
						"timestamp": time.Now().Unix(),
					}
					
					if err := conn.WriteJSON(pingMessage); err != nil {
						log.Printf("Failed to ping connection %d: %v", j, err)
					} else {
						log.Printf("Sent ping to connection %d", j)
					}
				}
			}
		}
	}
}

func (wst *WebSocketTester) Disconnect() {
	log.Println("Disconnecting all WebSocket connections...")
	
	wst.cancel()
	
	for i, conn := range wst.connections {
		if conn != nil {
			// Send leave table message
			leaveMessage := map[string]interface{}{
				"type": "leave_table",
				"payload": map[string]interface{}{
					"tableId": wst.tableID,
				},
			}
			conn.WriteJSON(leaveMessage)
			
			// Close connection
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			conn.Close()
			log.Printf("Disconnected connection %d", i)
		}
	}
	
	wst.wg.Wait()
	log.Println("All connections closed")
}

func main() {
	log.Println("Starting WebSocket functionality test...")
	
	// Test parameters
	numConnections := 3
	tableID := "test-table-001"
	
	// Create tester
	tester := NewWebSocketTester(numConnections, tableID)
	
	// Set up signal handling
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	
	// Connect to WebSocket server
	if err := tester.Connect(); err != nil {
		log.Fatalf("Failed to establish connections: %v", err)
	}
	
	log.Printf("Successfully established %d WebSocket connections", numConnections)
	
	// Wait a moment for all connections to be established
	time.Sleep(2 * time.Second)
	
	// Start test scenarios
	go func() {
		// Test 1: Record operations
		log.Println("\n=== Test 1: Record Operations ===")
		tester.SimulateRecordOperations()
		
		time.Sleep(3 * time.Second)
		
		// Test 2: Connection stability
		log.Println("\n=== Test 2: Connection Stability ===")
		tester.TestConnectionStability()
		
		log.Println("\n=== Tests completed ===")
		log.Println("Press Ctrl+C to exit or wait for manual testing...")
	}()
	
	// Wait for interrupt signal
	<-interrupt
	
	// Clean shutdown
	log.Println("\nShutting down...")
	tester.Disconnect()
	
	log.Println("WebSocket test completed successfully!")
}