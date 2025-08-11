package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pyairtable-compose/go-services/domain/shared"
)

// OutboxStatus represents the status of an outbox entry
type OutboxStatus string

const (
	OutboxStatusPending    OutboxStatus = "pending"
	OutboxStatusProcessing OutboxStatus = "processing"
	OutboxStatusPublished  OutboxStatus = "published"
	OutboxStatusFailed     OutboxStatus = "failed"
	OutboxStatusSkipped    OutboxStatus = "skipped"
)

// OutboxEntry represents an entry in the transactional outbox
type OutboxEntry struct {
	ID            string       `json:"id"`
	AggregateID   string       `json:"aggregate_id"`
	AggregateType string       `json:"aggregate_type"`
	EventType     string       `json:"event_type"`
	EventVersion  int64        `json:"event_version"`
	Payload       []byte       `json:"payload"`
	Metadata      []byte       `json:"metadata"`
	Status        OutboxStatus `json:"status"`
	CreatedAt     time.Time    `json:"created_at"`
	ProcessedAt   *time.Time   `json:"processed_at,omitempty"`
	RetryCount    int          `json:"retry_count"`
	MaxRetries    int          `json:"max_retries"`
	NextRetryAt   *time.Time   `json:"next_retry_at,omitempty"`
	ErrorMessage  string       `json:"error_message,omitempty"`
	TenantID      string       `json:"tenant_id"`
}

// OutboxConfig defines configuration for the outbox pattern
type OutboxConfig struct {
	TableName           string
	BatchSize           int
	PollInterval        time.Duration
	MaxRetries          int
	RetryBackoffBase    time.Duration
	RetryBackoffMax     time.Duration
	WorkerCount         int
	EnableDeduplication bool
	PublishTimeout      time.Duration
}

// TransactionalOutbox implements the transactional outbox pattern
type TransactionalOutbox struct {
	db       *sql.DB
	config   OutboxConfig
	publisher EventPublisher
	
	// Control channels
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	
	// Metrics
	stats OutboxStats
	mu    sync.RWMutex
}

// OutboxStats tracks outbox performance metrics
type OutboxStats struct {
	TotalEvents     int64
	PublishedEvents int64
	FailedEvents    int64
	RetryEvents     int64
	ProcessingTime  time.Duration
	LastProcessedAt time.Time
}

// EventPublisher interface for publishing events to external systems
type EventPublisher interface {
	Publish(ctx context.Context, event *OutboxEntry) error
	GetName() string
}

// NewTransactionalOutbox creates a new transactional outbox
func NewTransactionalOutbox(db *sql.DB, config OutboxConfig, publisher EventPublisher) *TransactionalOutbox {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &TransactionalOutbox{
		db:        db,
		config:    config,
		publisher: publisher,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start starts the outbox processing workers
func (to *TransactionalOutbox) Start() error {
	log.Printf("Starting transactional outbox with %d workers", to.config.WorkerCount)
	
	// Create outbox table if it doesn't exist
	if err := to.createOutboxTable(); err != nil {
		return fmt.Errorf("failed to create outbox table: %w", err)
	}
	
	// Start worker pool
	for i := 0; i < to.config.WorkerCount; i++ {
		to.wg.Add(1)
		go to.worker(i)
	}
	
	// Start metrics collector
	to.wg.Add(1)
	go to.metricsCollector()
	
	log.Printf("Transactional outbox started successfully")
	return nil
}

// Stop gracefully stops the outbox processing
func (to *TransactionalOutbox) Stop() error {
	log.Println("Stopping transactional outbox...")
	
	to.cancel()
	to.wg.Wait()
	
	log.Println("Transactional outbox stopped")
	return nil
}

// AddEvent adds an event to the outbox within a transaction
func (to *TransactionalOutbox) AddEvent(tx *sql.Tx, event shared.DomainEvent) error {
	// Serialize payload and metadata
	payloadBytes, err := json.Marshal(event.Payload())
	if err != nil {
		return fmt.Errorf("failed to serialize event payload: %w", err)
	}
	
	metadata := map[string]interface{}{
		"correlation_id": uuid.New().String(),
		"created_by":     "transactional_outbox",
		"source":         "domain_events",
	}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to serialize metadata: %w", err)
	}
	
	// Create outbox entry
	entry := &OutboxEntry{
		ID:            uuid.New().String(),
		AggregateID:   event.AggregateID(),
		AggregateType: event.AggregateType(),
		EventType:     event.EventType(),
		EventVersion:  event.Version(),
		Payload:       payloadBytes,
		Metadata:      metadataBytes,
		Status:        OutboxStatusPending,
		CreatedAt:     time.Now(),
		RetryCount:    0,
		MaxRetries:    to.config.MaxRetries,
		TenantID:      event.TenantID(),
	}
	
	// Insert into outbox table using the provided transaction
	query := fmt.Sprintf(`
		INSERT INTO %s (
			id, aggregate_id, aggregate_type, event_type, event_version,
			payload, metadata, status, created_at, retry_count, max_retries, tenant_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, to.config.TableName)
	
	_, err = tx.Exec(query,
		entry.ID, entry.AggregateID, entry.AggregateType, entry.EventType, entry.EventVersion,
		entry.Payload, entry.Metadata, entry.Status, entry.CreatedAt,
		entry.RetryCount, entry.MaxRetries, entry.TenantID,
	)
	
	if err != nil {
		return fmt.Errorf("failed to insert outbox entry: %w", err)
	}
	
	to.mu.Lock()
	to.stats.TotalEvents++
	to.mu.Unlock()
	
	return nil
}

// AddEvents adds multiple events to the outbox within a transaction
func (to *TransactionalOutbox) AddEvents(tx *sql.Tx, events []shared.DomainEvent) error {
	for _, event := range events {
		if err := to.AddEvent(tx, event); err != nil {
			return fmt.Errorf("failed to add event %s to outbox: %w", event.EventID(), err)
		}
	}
	return nil
}

// worker processes outbox entries
func (to *TransactionalOutbox) worker(workerID int) {
	defer to.wg.Done()
	
	ticker := time.NewTicker(to.config.PollInterval)
	defer ticker.Stop()
	
	log.Printf("Outbox worker %d started", workerID)
	
	for {
		select {
		case <-ticker.C:
			if err := to.processBatch(); err != nil {
				log.Printf("Worker %d: error processing batch: %v", workerID, err)
			}
		case <-to.ctx.Done():
			log.Printf("Outbox worker %d stopping", workerID)
			return
		}
	}
}

// processBatch processes a batch of pending outbox entries
func (to *TransactionalOutbox) processBatch() error {
	// Get pending entries
	entries, err := to.getPendingEntries()
	if err != nil {
		return fmt.Errorf("failed to get pending entries: %w", err)
	}
	
	if len(entries) == 0 {
		return nil // No work to do
	}
	
	startTime := time.Now()
	successCount := 0
	failureCount := 0
	
	for _, entry := range entries {
		if err := to.processEntry(entry); err != nil {
			log.Printf("Failed to process outbox entry %s: %v", entry.ID, err)
			failureCount++
		} else {
			successCount++
		}
	}
	
	processingTime := time.Since(startTime)
	
	to.mu.Lock()
	to.stats.PublishedEvents += int64(successCount)
	to.stats.FailedEvents += int64(failureCount)
	to.stats.ProcessingTime += processingTime
	to.stats.LastProcessedAt = time.Now()
	to.mu.Unlock()
	
	if len(entries) > 0 {
		log.Printf("Processed batch: %d success, %d failed, took %v", 
			successCount, failureCount, processingTime)
	}
	
	return nil
}

// getPendingEntries retrieves pending outbox entries
func (to *TransactionalOutbox) getPendingEntries() ([]*OutboxEntry, error) {
	query := fmt.Sprintf(`
		SELECT id, aggregate_id, aggregate_type, event_type, event_version,
			   payload, metadata, status, created_at, retry_count, max_retries,
			   next_retry_at, error_message, tenant_id
		FROM %s 
		WHERE status = $1 OR (status = $2 AND next_retry_at <= NOW())
		ORDER BY created_at ASC
		LIMIT $3
	`, to.config.TableName)
	
	rows, err := to.db.Query(query, OutboxStatusPending, OutboxStatusFailed, to.config.BatchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending entries: %w", err)
	}
	defer rows.Close()
	
	var entries []*OutboxEntry
	for rows.Next() {
		entry := &OutboxEntry{}
		var nextRetryAt sql.NullTime
		
		err := rows.Scan(
			&entry.ID, &entry.AggregateID, &entry.AggregateType, &entry.EventType, &entry.EventVersion,
			&entry.Payload, &entry.Metadata, &entry.Status, &entry.CreatedAt,
			&entry.RetryCount, &entry.MaxRetries, &nextRetryAt, &entry.ErrorMessage, &entry.TenantID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan outbox entry: %w", err)
		}
		
		if nextRetryAt.Valid {
			entry.NextRetryAt = &nextRetryAt.Time
		}
		
		entries = append(entries, entry)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}
	
	return entries, nil
}

// processEntry processes a single outbox entry
func (to *TransactionalOutbox) processEntry(entry *OutboxEntry) error {
	// Mark as processing
	if err := to.updateEntryStatus(entry.ID, OutboxStatusProcessing, ""); err != nil {
		return fmt.Errorf("failed to mark entry as processing: %w", err)
	}
	
	// Create timeout context
	ctx, cancel := context.WithTimeout(to.ctx, to.config.PublishTimeout)
	defer cancel()
	
	// Attempt to publish
	err := to.publisher.Publish(ctx, entry)
	if err != nil {
		// Handle failure
		return to.handlePublishFailure(entry, err)
	}
	
	// Mark as published
	now := time.Now()
	entry.ProcessedAt = &now
	if err := to.updateEntryStatus(entry.ID, OutboxStatusPublished, ""); err != nil {
		return fmt.Errorf("failed to mark entry as published: %w", err)
	}
	
	return nil
}

// handlePublishFailure handles publication failures with retry logic
func (to *TransactionalOutbox) handlePublishFailure(entry *OutboxEntry, publishErr error) error {
	entry.RetryCount++
	entry.ErrorMessage = publishErr.Error()
	
	if entry.RetryCount >= entry.MaxRetries {
		// Max retries exceeded, mark as failed
		if err := to.updateEntryStatus(entry.ID, OutboxStatusFailed, publishErr.Error()); err != nil {
			return fmt.Errorf("failed to mark entry as failed: %w", err)
		}
		return fmt.Errorf("max retries exceeded for entry %s: %w", entry.ID, publishErr)
	}
	
	// Calculate next retry time with exponential backoff
	backoff := time.Duration(1<<uint(entry.RetryCount-1)) * to.config.RetryBackoffBase
	if backoff > to.config.RetryBackoffMax {
		backoff = to.config.RetryBackoffMax
	}
	
	nextRetry := time.Now().Add(backoff)
	entry.NextRetryAt = &nextRetry
	
	// Update for retry
	if err := to.updateEntryForRetry(entry); err != nil {
		return fmt.Errorf("failed to update entry for retry: %w", err)
	}
	
	to.mu.Lock()
	to.stats.RetryEvents++
	to.mu.Unlock()
	
	return fmt.Errorf("entry %s scheduled for retry at %v: %w", entry.ID, nextRetry, publishErr)
}

// updateEntryStatus updates the status of an outbox entry
func (to *TransactionalOutbox) updateEntryStatus(entryID string, status OutboxStatus, errorMsg string) error {
	query := fmt.Sprintf(`
		UPDATE %s 
		SET status = $1, error_message = $2, processed_at = CASE WHEN $1 = 'published' THEN NOW() ELSE processed_at END
		WHERE id = $3
	`, to.config.TableName)
	
	_, err := to.db.Exec(query, status, errorMsg, entryID)
	if err != nil {
		return fmt.Errorf("failed to update entry status: %w", err)
	}
	
	return nil
}

// updateEntryForRetry updates an entry for retry
func (to *TransactionalOutbox) updateEntryForRetry(entry *OutboxEntry) error {
	query := fmt.Sprintf(`
		UPDATE %s 
		SET status = $1, retry_count = $2, next_retry_at = $3, error_message = $4
		WHERE id = $5
	`, to.config.TableName)
	
	_, err := to.db.Exec(query, OutboxStatusFailed, entry.RetryCount, entry.NextRetryAt, entry.ErrorMessage, entry.ID)
	if err != nil {
		return fmt.Errorf("failed to update entry for retry: %w", err)
	}
	
	return nil
}

// createOutboxTable creates the outbox table if it doesn't exist
func (to *TransactionalOutbox) createOutboxTable() error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id VARCHAR(255) PRIMARY KEY,
			aggregate_id VARCHAR(255) NOT NULL,
			aggregate_type VARCHAR(255) NOT NULL,
			event_type VARCHAR(255) NOT NULL,
			event_version BIGINT NOT NULL,
			payload JSONB NOT NULL,
			metadata JSONB DEFAULT '{}',
			status VARCHAR(50) NOT NULL DEFAULT 'pending',
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			processed_at TIMESTAMP WITH TIME ZONE,
			retry_count INTEGER DEFAULT 0,
			max_retries INTEGER DEFAULT 3,
			next_retry_at TIMESTAMP WITH TIME ZONE,
			error_message TEXT,
			tenant_id VARCHAR(255)
		)
	`, to.config.TableName)
	
	if _, err := to.db.Exec(query); err != nil {
		return fmt.Errorf("failed to create outbox table: %w", err)
	}
	
	// Create indexes
	indexes := []string{
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_status ON %s(status)", to.config.TableName, to.config.TableName),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_created_at ON %s(created_at)", to.config.TableName, to.config.TableName),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_next_retry ON %s(next_retry_at)", to.config.TableName, to.config.TableName),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_aggregate ON %s(aggregate_id, aggregate_type)", to.config.TableName, to.config.TableName),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_tenant ON %s(tenant_id)", to.config.TableName, to.config.TableName),
	}
	
	for _, indexQuery := range indexes {
		if _, err := to.db.Exec(indexQuery); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}
	
	return nil
}

// metricsCollector periodically collects and logs metrics
func (to *TransactionalOutbox) metricsCollector() {
	defer to.wg.Done()
	
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			to.logMetrics()
		case <-to.ctx.Done():
			return
		}
	}
}

// logMetrics logs current outbox metrics
func (to *TransactionalOutbox) logMetrics() {
	to.mu.RLock()
	stats := to.stats
	to.mu.RUnlock()
	
	// Get current counts from database
	counts, err := to.getStatusCounts()
	if err != nil {
		log.Printf("Failed to get outbox status counts: %v", err)
		return
	}
	
	log.Printf("Outbox Metrics - Total: %d, Published: %d, Failed: %d, Retries: %d, Pending: %d, Processing: %d",
		stats.TotalEvents, stats.PublishedEvents, stats.FailedEvents, stats.RetryEvents,
		counts[OutboxStatusPending], counts[OutboxStatusProcessing])
}

// getStatusCounts returns count of entries by status
func (to *TransactionalOutbox) getStatusCounts() (map[OutboxStatus]int, error) {
	query := fmt.Sprintf(`
		SELECT status, COUNT(*) 
		FROM %s 
		GROUP BY status
	`, to.config.TableName)
	
	rows, err := to.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	counts := make(map[OutboxStatus]int)
	for rows.Next() {
		var status OutboxStatus
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	
	return counts, nil
}

// GetStats returns current outbox statistics
func (to *TransactionalOutbox) GetStats() OutboxStats {
	to.mu.RLock()
	defer to.mu.RUnlock()
	return to.stats
}

// CleanupProcessedEntries removes old processed entries to prevent table bloat
func (to *TransactionalOutbox) CleanupProcessedEntries(olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	
	query := fmt.Sprintf(`
		DELETE FROM %s 
		WHERE status = $1 AND processed_at < $2
	`, to.config.TableName)
	
	result, err := to.db.Exec(query, OutboxStatusPublished, cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup processed entries: %w", err)
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		log.Printf("Cleaned up %d processed outbox entries", rowsAffected)
	}
	
	return nil
}