package websocket

import (
	"time"
)

// IncomingMessage represents messages received from clients
type IncomingMessage struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// OutgoingMessage represents messages sent to clients
type OutgoingMessage struct {
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

// Event represents a real-time event
type Event struct {
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload"`
	Timestamp int64       `json:"timestamp"`
}

// RecordEventPayload represents record-related events
type RecordEventPayload struct {
	TableID  string      `json:"tableId"`
	RecordID string      `json:"recordId"`
	Data     interface{} `json:"data,omitempty"`
}

// UserPresencePayload represents user presence events
type UserPresencePayload struct {
	UserID  string `json:"userId"`
	TableID string `json:"tableId,omitempty"`
}

// PresenceUpdatePayload represents presence update events
type PresenceUpdatePayload struct {
	TableID string   `json:"tableId"`
	Users   []string `json:"users"`
}

// CursorPositionPayload represents cursor position updates (optional feature)
type CursorPositionPayload struct {
	UserID   string  `json:"userId"`
	TableID  string  `json:"tableId"`
	RecordID string  `json:"recordId,omitempty"`
	FieldID  string  `json:"fieldId,omitempty"`
	X        float64 `json:"x,omitempty"`
	Y        float64 `json:"y,omitempty"`
}

// ConnectionStats represents WebSocket connection statistics
type ConnectionStats struct {
	TotalConnections  int                       `json:"totalConnections"`
	UserConnections   map[string]int            `json:"userConnections"`
	TableConnections  map[string]int            `json:"tableConnections"`
	ConnectionsByUser map[string][]string       `json:"connectionsByUser"`
	Uptime           time.Duration             `json:"uptime"`
	MessageStats     MessageStats              `json:"messageStats"`
}

// MessageStats represents message statistics
type MessageStats struct {
	TotalMessages    int64 `json:"totalMessages"`
	MessagesPerType  map[string]int64 `json:"messagesPerType"`
	ErrorCount       int64 `json:"errorCount"`
	LastMessageTime  time.Time `json:"lastMessageTime"`
}

// AuthPayload represents authentication information
type AuthPayload struct {
	UserID    string `json:"userId"`
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"`
}

// ErrorPayload represents error messages
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// getCurrentTimestamp returns the current Unix timestamp
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}

// Event types constants
const (
	EventTypeRecordCreated = "record:created"
	EventTypeRecordUpdated = "record:updated"
	EventTypeRecordDeleted = "record:deleted"
	EventTypeUserJoined    = "user:joined"
	EventTypeUserLeft      = "user:left"
	EventTypeCursorMoved   = "cursor:moved"
	EventTypePresenceUpdate = "presence:update"
	EventTypeError         = "error"
	EventTypeAuth          = "auth"
	EventTypePing          = "ping"
	EventTypePong          = "pong"
)

// Message types constants
const (
	MessageTypeJoinTable    = "join_table"
	MessageTypeLeaveTable   = "leave_table"
	MessageTypeAuth         = "auth"
	MessageTypePing         = "ping"
	MessageTypePong         = "pong"
	MessageTypeCursorMove   = "cursor_move"
	MessageTypeSubscribe    = "subscribe"
	MessageTypeUnsubscribe  = "unsubscribe"
)