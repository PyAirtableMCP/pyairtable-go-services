package eventstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"github.com/lib/pq"
)

// PostgresEventStore implements the EventStore interface using PostgreSQL
type PostgresEventStore struct {
	db *sql.DB
}

// NewPostgresEventStore creates a new PostgreSQL event store
func NewPostgresEventStore(db *sql.DB) *PostgresEventStore {
	return &PostgresEventStore{
		db: db,
	}
}

// SaveEvents saves events for an aggregate
func (es *PostgresEventStore) SaveEvents(aggregateID string, events []shared.DomainEvent, expectedVersion int64) error {
	if len(events) == 0 {
		return nil
	}

	// Start transaction
	tx, err := es.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Check optimistic concurrency
	var currentVersion int64
	err = tx.QueryRow(`
		SELECT COALESCE(MAX(version), 0) 
		FROM domain_events 
		WHERE aggregate_id = $1
	`, aggregateID).Scan(&currentVersion)
	
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check current version: %w", err)
	}

	if currentVersion != expectedVersion {
		return shared.NewDomainError(
			shared.ErrCodeConflict, 
			fmt.Sprintf("concurrency conflict: expected version %d, got %d", expectedVersion, currentVersion),
			nil,
		)
	}

	// Insert events
	for _, event := range events {
		payloadBytes, err := json.Marshal(event.Payload())
		if err != nil {
			return fmt.Errorf("failed to marshal event payload: %w", err)
		}

		metadataBytes, err := json.Marshal(map[string]interface{}{
			"event_id": event.EventID(),
			"correlation_id": "", // Could be set from context
		})
		if err != nil {
			return fmt.Errorf("failed to marshal event metadata: %w", err)
		}

		_, err = tx.Exec(`
			INSERT INTO domain_events (
				id, event_type, aggregate_id, aggregate_type, version, 
				tenant_id, occurred_at, payload, metadata, processed
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`,
			event.EventID(),
			event.EventType(),
			event.AggregateID(),
			event.AggregateType(),
			event.Version(),
			event.TenantID(),
			event.OccurredAt(),
			payloadBytes,
			metadataBytes,
			false,
		)

		if err != nil {
			return fmt.Errorf("failed to insert event: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetEvents retrieves all events for an aggregate
func (es *PostgresEventStore) GetEvents(aggregateID string) ([]shared.DomainEvent, error) {
	return es.GetEventsFromVersion(aggregateID, 0)
}

// GetEventsFromVersion retrieves events from a specific version
func (es *PostgresEventStore) GetEventsFromVersion(aggregateID string, fromVersion int64) ([]shared.DomainEvent, error) {
	rows, err := es.db.Query(`
		SELECT id, event_type, aggregate_id, aggregate_type, version, 
			   tenant_id, occurred_at, payload
		FROM domain_events 
		WHERE aggregate_id = $1 AND version > $2
		ORDER BY version ASC
	`, aggregateID, fromVersion)

	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	var events []shared.DomainEvent
	for rows.Next() {
		var (
			id            string
			eventType     string
			aggID         string
			aggType       string
			version       int64
			tenantID      string
			occurredAt    time.Time
			payloadBytes  []byte
		)

		err := rows.Scan(&id, &eventType, &aggID, &aggType, &version, &tenantID, &occurredAt, &payloadBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event row: %w", err)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal event payload: %w", err)
		}

		event := &shared.BaseDomainEvent{
			EventIDValue:       id,
			EventTypeValue:     eventType,
			AggregateIDValue:   aggID,
			AggregateTypeValue: aggType,
			VersionValue:       version,
			TenantIDValue:      tenantID,
			OccurredAtValue:    occurredAt,
			PayloadValue:       payload,
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return events, nil
}

// GetEventsByType retrieves events by type within a time range
func (es *PostgresEventStore) GetEventsByType(eventType string, from, to time.Time) ([]shared.DomainEvent, error) {
	rows, err := es.db.Query(`
		SELECT id, event_type, aggregate_id, aggregate_type, version, 
			   tenant_id, occurred_at, payload
		FROM domain_events 
		WHERE event_type = $1 AND occurred_at >= $2 AND occurred_at <= $3
		ORDER BY occurred_at ASC
	`, eventType, from, to)

	if err != nil {
		return nil, fmt.Errorf("failed to query events by type: %w", err)
	}
	defer rows.Close()

	var events []shared.DomainEvent
	for rows.Next() {
		var (
			id            string
			evtType       string
			aggID         string
			aggType       string
			version       int64
			tenantID      string
			occurredAt    time.Time
			payloadBytes  []byte
		)

		err := rows.Scan(&id, &evtType, &aggID, &aggType, &version, &tenantID, &occurredAt, &payloadBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event row: %w", err)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal event payload: %w", err)
		}

		event := &shared.BaseDomainEvent{
			EventIDValue:       id,
			EventTypeValue:     evtType,
			AggregateIDValue:   aggID,
			AggregateTypeValue: aggType,
			VersionValue:       version,
			TenantIDValue:      tenantID,
			OccurredAtValue:    occurredAt,
			PayloadValue:       payload,
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return events, nil
}

// GetUnprocessedEvents retrieves events that haven't been processed yet
func (es *PostgresEventStore) GetUnprocessedEvents(limit int) ([]shared.DomainEvent, error) {
	rows, err := es.db.Query(`
		SELECT id, event_type, aggregate_id, aggregate_type, version, 
			   tenant_id, occurred_at, payload
		FROM domain_events 
		WHERE processed = false
		ORDER BY occurred_at ASC
		LIMIT $1
	`, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to query unprocessed events: %w", err)
	}
	defer rows.Close()

	var events []shared.DomainEvent
	for rows.Next() {
		var (
			id            string
			eventType     string
			aggID         string
			aggType       string
			version       int64
			tenantID      string
			occurredAt    time.Time
			payloadBytes  []byte
		)

		err := rows.Scan(&id, &eventType, &aggID, &aggType, &version, &tenantID, &occurredAt, &payloadBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event row: %w", err)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal event payload: %w", err)
		}

		event := &shared.BaseDomainEvent{
			EventIDValue:       id,
			EventTypeValue:     eventType,
			AggregateIDValue:   aggID,
			AggregateTypeValue: aggType,
			VersionValue:       version,
			TenantIDValue:      tenantID,
			OccurredAtValue:    occurredAt,
			PayloadValue:       payload,
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return events, nil
}

// MarkEventAsProcessed marks an event as processed
func (es *PostgresEventStore) MarkEventAsProcessed(eventID string) error {
	_, err := es.db.Exec(`
		UPDATE domain_events 
		SET processed = true, retry_count = retry_count + 1
		WHERE id = $1
	`, eventID)

	if err != nil {
		return fmt.Errorf("failed to mark event as processed: %w", err)
	}

	return nil
}

// MarkEventsAsProcessed marks multiple events as processed
func (es *PostgresEventStore) MarkEventsAsProcessed(eventIDs []string) error {
	if len(eventIDs) == 0 {
		return nil
	}

	_, err := es.db.Exec(`
		UPDATE domain_events 
		SET processed = true, retry_count = retry_count + 1
		WHERE id = ANY($1)
	`, pq.Array(eventIDs))

	if err != nil {
		return fmt.Errorf("failed to mark events as processed: %w", err)
	}

	return nil
}