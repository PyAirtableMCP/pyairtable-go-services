package audit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// EventType represents the type of audit event
type EventType string

const (
	// Authentication events
	EventLogin              EventType = "LOGIN"
	EventLoginFailed        EventType = "LOGIN_FAILED"
	EventLogout             EventType = "LOGOUT"
	EventTokenRefresh       EventType = "TOKEN_REFRESH"
	EventTokenRevoke        EventType = "TOKEN_REVOKE"
	EventPasswordChange     EventType = "PASSWORD_CHANGE"
	
	// Authorization events
	EventAccessGranted      EventType = "ACCESS_GRANTED"
	EventAccessDenied       EventType = "ACCESS_DENIED"
	EventPermissionChange   EventType = "PERMISSION_CHANGE"
	EventRoleChange         EventType = "ROLE_CHANGE"
	
	// Data events
	EventDataAccess         EventType = "DATA_ACCESS"
	EventDataModify         EventType = "DATA_MODIFY"
	EventDataDelete         EventType = "DATA_DELETE"
	EventDataExport         EventType = "DATA_EXPORT"
	EventDataImport         EventType = "DATA_IMPORT"
	
	// System events
	EventSystemStart        EventType = "SYSTEM_START"
	EventSystemStop         EventType = "SYSTEM_STOP"
	EventConfigChange       EventType = "CONFIG_CHANGE"
	EventSecurityIncident   EventType = "SECURITY_INCIDENT"
	EventAPIRateLimit       EventType = "API_RATE_LIMIT"
)

// Severity levels for audit events
type Severity string

const (
	SeverityInfo     Severity = "INFO"
	SeverityWarning  Severity = "WARNING"
	SeverityError    Severity = "ERROR"
	SeverityCritical Severity = "CRITICAL"
)

// AuditEvent represents a security audit event
type AuditEvent struct {
	ID          string                 `json:"id" gorm:"primaryKey"`
	Timestamp   time.Time              `json:"timestamp" gorm:"index"`
	EventType   EventType              `json:"event_type" gorm:"index"`
	Severity    Severity               `json:"severity" gorm:"index"`
	UserID      string                 `json:"user_id,omitempty" gorm:"index"`
	TenantID    string                 `json:"tenant_id,omitempty" gorm:"index"`
	SessionID   string                 `json:"session_id,omitempty"`
	IPAddress   string                 `json:"ip_address,omitempty" gorm:"index"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	Resource    string                 `json:"resource,omitempty" gorm:"index"`
	Action      string                 `json:"action,omitempty"`
	Result      string                 `json:"result,omitempty"` // SUCCESS, FAILURE, DENIED
	Message     string                 `json:"message"`
	Details     json.RawMessage        `json:"details,omitempty" gorm:"type:jsonb"`
	ServiceName string                 `json:"service_name" gorm:"index"`
	RequestID   string                 `json:"request_id,omitempty"`
	Signature   string                 `json:"signature"` // HMAC signature for tamper protection
	CreatedAt   time.Time              `json:"created_at"`
}

// AuditLogger provides centralized security audit logging
type AuditLogger struct {
	db            *gorm.DB
	secretKey     []byte
	serviceName   string
	buffer        []*AuditEvent
	bufferSize    int
	flushInterval time.Duration
	mu            sync.RWMutex
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// NewAuditLogger creates a new audit logger with tamper protection
func NewAuditLogger(db *gorm.DB, secretKey, serviceName string) *AuditLogger {
	logger := &AuditLogger{
		db:            db,
		secretKey:     []byte(secretKey),
		serviceName:   serviceName,
		bufferSize:    100,
		flushInterval: 10 * time.Second,
		buffer:        make([]*AuditEvent, 0),
		stopCh:        make(chan struct{}),
	}

	// Auto-migrate audit events table
	if err := db.AutoMigrate(&AuditEvent{}); err != nil {
		log.Printf("Failed to migrate audit events table: %v", err)
	}

	// Start background flusher
	logger.startBackgroundFlusher()

	return logger
}

// LogEvent logs a security audit event with tamper protection
func (a *AuditLogger) LogEvent(ctx context.Context, event *AuditEvent) error {
	// Generate unique ID
	event.ID = generateEventID()
	event.Timestamp = time.Now().UTC()
	event.ServiceName = a.serviceName
	event.CreatedAt = event.Timestamp

	// Calculate HMAC signature for tamper protection
	signature, err := a.calculateSignature(event)
	if err != nil {
		return fmt.Errorf("failed to calculate signature: %w", err)
	}
	event.Signature = signature

	// Add to buffer for batch processing
	a.mu.Lock()
	a.buffer = append(a.buffer, event)
	shouldFlush := len(a.buffer) >= a.bufferSize
	a.mu.Unlock()

	// Immediate flush if buffer is full
	if shouldFlush {
		return a.flushBuffer()
	}

	// Also log to system logger for immediate visibility
	a.logToSystem(event)

	return nil
}

// LogAuthentication logs authentication-related events
func (a *AuditLogger) LogAuthentication(ctx context.Context, eventType EventType, userID, ipAddress, userAgent, result, message string, details map[string]interface{}) error {
	severity := SeverityInfo
	if eventType == EventLoginFailed {
		severity = SeverityWarning
	}

	detailsJSON, _ := json.Marshal(details)

	return a.LogEvent(ctx, &AuditEvent{
		EventType: eventType,
		Severity:  severity,
		UserID:    userID,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Result:    result,
		Message:   message,
		Details:   detailsJSON,
	})
}

// LogAuthorization logs authorization-related events
func (a *AuditLogger) LogAuthorization(ctx context.Context, eventType EventType, userID, resource, action, result, message string, details map[string]interface{}) error {
	severity := SeverityInfo
	if result == "DENIED" {
		severity = SeverityWarning
	}

	detailsJSON, _ := json.Marshal(details)

	return a.LogEvent(ctx, &AuditEvent{
		EventType: eventType,
		Severity:  severity,
		UserID:    userID,
		Resource:  resource,
		Action:    action,
		Result:    result,
		Message:   message,
		Details:   detailsJSON,
	})
}

// LogDataAccess logs data access events
func (a *AuditLogger) LogDataAccess(ctx context.Context, eventType EventType, userID, resource, action, message string, details map[string]interface{}) error {
	severity := SeverityInfo
	if eventType == EventDataDelete || eventType == EventDataExport {
		severity = SeverityWarning
	}

	detailsJSON, _ := json.Marshal(details)

	return a.LogEvent(ctx, &AuditEvent{
		EventType: eventType,
		Severity:  severity,
		UserID:    userID,
		Resource:  resource,
		Action:    action,
		Result:    "SUCCESS",
		Message:   message,
		Details:   detailsJSON,
	})
}

// LogSecurityIncident logs security incidents with critical severity
func (a *AuditLogger) LogSecurityIncident(ctx context.Context, userID, ipAddress, message string, details map[string]interface{}) error {
	detailsJSON, _ := json.Marshal(details)

	// Security incidents are always critical and flushed immediately
	event := &AuditEvent{
		EventType: EventSecurityIncident,
		Severity:  SeverityCritical,
		UserID:    userID,
		IPAddress: ipAddress,
		Result:    "INCIDENT",
		Message:   message,
		Details:   detailsJSON,
	}

	if err := a.LogEvent(ctx, event); err != nil {
		return err
	}

	// Force immediate flush for security incidents
	return a.flushBuffer()
}

// VerifyEventIntegrity verifies the integrity of an audit event using HMAC
func (a *AuditLogger) VerifyEventIntegrity(event *AuditEvent) (bool, error) {
	expectedSignature, err := a.calculateSignature(event)
	if err != nil {
		return false, err
	}

	return hmac.Equal([]byte(event.Signature), []byte(expectedSignature)), nil
}

// GetEvents retrieves audit events with filtering
func (a *AuditLogger) GetEvents(ctx context.Context, filters map[string]interface{}, limit, offset int) ([]*AuditEvent, error) {
	var events []*AuditEvent
	query := a.db.WithContext(ctx)

	// Apply filters
	for key, value := range filters {
		switch key {
		case "event_type":
			query = query.Where("event_type = ?", value)
		case "severity":
			query = query.Where("severity = ?", value)
		case "user_id":
			query = query.Where("user_id = ?", value)
		case "ip_address":
			query = query.Where("ip_address = ?", value)
		case "resource":
			query = query.Where("resource = ?", value)
		case "start_time":
			query = query.Where("timestamp >= ?", value)
		case "end_time":
			query = query.Where("timestamp <= ?", value)
		}
	}

	err := query.Order("timestamp DESC").
		Limit(limit).
		Offset(offset).
		Find(&events).Error

	return events, err
}

// calculateSignature creates HMAC signature for tamper protection
func (a *AuditLogger) calculateSignature(event *AuditEvent) (string, error) {
	// Create a copy without the signature field for calculation
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		event.Timestamp.Format(time.RFC3339Nano),
		event.EventType,
		event.Severity,
		event.UserID,
		event.IPAddress,
		event.Resource,
		event.Action,
		event.Result,
		event.Message,
		string(event.Details),
		event.ServiceName,
	)

	h := hmac.New(sha256.New, a.secretKey)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// flushBuffer writes buffered events to database
func (a *AuditLogger) flushBuffer() error {
	a.mu.Lock()
	if len(a.buffer) == 0 {
		a.mu.Unlock()
		return nil
	}

	events := make([]*AuditEvent, len(a.buffer))
	copy(events, a.buffer)
	a.buffer = a.buffer[:0] // Clear buffer
	a.mu.Unlock()

	// Batch insert events
	if err := a.db.CreateInBatches(events, 50).Error; err != nil {
		log.Printf("Failed to flush audit events to database: %v", err)
		return err
	}

	log.Printf("Flushed %d audit events to database", len(events))
	return nil
}

// startBackgroundFlusher starts the background goroutine for periodic flushing
func (a *AuditLogger) startBackgroundFlusher() {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(a.flushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := a.flushBuffer(); err != nil {
					log.Printf("Background flush failed: %v", err)
				}
			case <-a.stopCh:
				// Final flush before stopping
				a.flushBuffer()
				return
			}
		}
	}()
}

// logToSystem logs to system logger for immediate visibility
func (a *AuditLogger) logToSystem(event *AuditEvent) {
	logLine := fmt.Sprintf("[AUDIT] %s | %s | %s | %s | %s | %s",
		event.Timestamp.Format(time.RFC3339),
		event.EventType,
		event.Severity,
		event.UserID,
		event.IPAddress,
		event.Message,
	)

	// Log based on severity
	switch event.Severity {
	case SeverityCritical, SeverityError:
		log.Printf("ERROR: %s", logLine)
	case SeverityWarning:
		log.Printf("WARN: %s", logLine)
	default:
		log.Printf("INFO: %s", logLine)
	}
}

// Close stops the audit logger and flushes remaining events
func (a *AuditLogger) Close() error {
	close(a.stopCh)
	a.wg.Wait()
	return nil
}

// generateEventID creates a unique event ID
func generateEventID() string {
	return fmt.Sprintf("audit_%d_%s", time.Now().UnixNano(), randomString(8))
}

// randomString generates a random string of specified length
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// SIEM integration functions

// ExportToSIEM exports audit events in SIEM-compatible format (JSON)
func (a *AuditLogger) ExportToSIEM(ctx context.Context, filters map[string]interface{}) ([]byte, error) {
	events, err := a.GetEvents(ctx, filters, 1000, 0) // Export max 1000 events
	if err != nil {
		return nil, err
	}

	// Convert to SIEM format
	siemEvents := make([]map[string]interface{}, len(events))
	for i, event := range events {
		siemEvents[i] = map[string]interface{}{
			"@timestamp":    event.Timestamp.Format(time.RFC3339),
			"event_type":    event.EventType,
			"severity":      event.Severity,
			"user_id":       event.UserID,
			"tenant_id":     event.TenantID,
			"ip_address":    event.IPAddress,
			"user_agent":    event.UserAgent,
			"resource":      event.Resource,
			"action":        event.Action,
			"result":        event.Result,
			"message":       event.Message,
			"service_name":  event.ServiceName,
			"request_id":    event.RequestID,
			"details":       json.RawMessage(event.Details),
			"source":        "pyairtable-compose",
			"environment":   os.Getenv("ENVIRONMENT"),
		}
	}

	return json.Marshal(siemEvents)
}

// SendToSyslog sends audit events to syslog (for SIEM integration)
func (a *AuditLogger) SendToSyslog(event *AuditEvent) error {
	// Convert severity to syslog level
	var priority string
	switch event.Severity {
	case SeverityCritical:
		priority = "CRIT"
	case SeverityError:
		priority = "ERR"
	case SeverityWarning:
		priority = "WARNING"
	default:
		priority = "INFO"
	}

	syslogMsg := fmt.Sprintf("<%s> %s [AUDIT] %s | %s | %s | %s | %s | %s",
		priority,
		event.Timestamp.Format(time.RFC3339),
		event.EventType,
		event.UserID,
		event.IPAddress,
		event.Resource,
		event.Action,
		event.Message,
	)

	// In a real implementation, this would use a proper syslog client
	log.Printf("SYSLOG: %s", syslogMsg)
	return nil
}