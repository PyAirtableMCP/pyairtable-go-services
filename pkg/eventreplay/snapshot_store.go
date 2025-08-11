package eventreplay

import (
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PostgresSnapshotStore implements SnapshotStore using PostgreSQL
type PostgresSnapshotStore struct {
	db             *sql.DB
	tableName      string
	enableCompress bool
}

// NewPostgresSnapshotStore creates a new PostgreSQL snapshot store
func NewPostgresSnapshotStore(db *sql.DB) *PostgresSnapshotStore {
	store := &PostgresSnapshotStore{
		db:             db,
		tableName:      "aggregate_snapshots",
		enableCompress: true,
	}
	
	// Ensure snapshot table exists
	if err := store.createSnapshotTable(); err != nil {
		log.Printf("Warning: Failed to create snapshot table: %v", err)
	}
	
	return store
}

// SaveSnapshot saves an aggregate snapshot
func (pss *PostgresSnapshotStore) SaveSnapshot(aggregateID string, aggregateType string, version int64, data []byte) error {
	// Compress data if enabled
	var finalData []byte
	var err error
	
	if pss.enableCompress {
		finalData, err = pss.compressData(data)
		if err != nil {
			log.Printf("Failed to compress snapshot data: %v, saving uncompressed", err)
			finalData = data
		}
	} else {
		finalData = data
	}
	
	// Generate snapshot ID
	snapshotID := uuid.New().String()
	
	// Insert or update snapshot
	query := fmt.Sprintf(`
		INSERT INTO %s (id, aggregate_id, aggregate_type, version, snapshot_data, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (aggregate_id) 
		DO UPDATE SET 
			version = EXCLUDED.version,
			snapshot_data = EXCLUDED.snapshot_data,
			created_at = EXCLUDED.created_at
		WHERE %s.version < EXCLUDED.version
	`, pss.tableName, pss.tableName)
	
	_, err = pss.db.Exec(query,
		snapshotID,
		aggregateID,
		aggregateType,
		version,
		finalData,
		time.Now(),
	)
	
	if err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}
	
	log.Printf("Saved snapshot for aggregate %s at version %d", aggregateID, version)
	return nil
}

// LoadSnapshot loads an aggregate snapshot
func (pss *PostgresSnapshotStore) LoadSnapshot(aggregateID string) (*AggregateSnapshot, error) {
	query := fmt.Sprintf(`
		SELECT id, aggregate_id, aggregate_type, version, snapshot_data, created_at, tenant_id
		FROM %s 
		WHERE aggregate_id = $1
	`, pss.tableName)
	
	var snapshot AggregateSnapshot
	var tenantID sql.NullString
	
	err := pss.db.QueryRow(query, aggregateID).Scan(
		&snapshot.ID,
		&snapshot.AggregateID,
		&snapshot.AggregateType,
		&snapshot.Version,
		&snapshot.Data,
		&snapshot.CreatedAt,
		&tenantID,
	)
	
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No snapshot found
		}
		return nil, fmt.Errorf("failed to load snapshot: %w", err)
	}
	
	if tenantID.Valid {
		snapshot.TenantID = tenantID.String
	}
	
	// Decompress data if needed
	if pss.enableCompress {
		decompressedData, err := pss.decompressData(snapshot.Data)
		if err != nil {
			log.Printf("Failed to decompress snapshot data: %v, using raw data", err)
		} else {
			snapshot.Data = decompressedData
		}
	}
	
	return &snapshot, nil
}

// DeleteSnapshot deletes an aggregate snapshot
func (pss *PostgresSnapshotStore) DeleteSnapshot(aggregateID string) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE aggregate_id = $1`, pss.tableName)
	
	result, err := pss.db.Exec(query, aggregateID)
	if err != nil {
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}
	
	rowsAffected, _ := result.RowsAffected()
	log.Printf("Deleted %d snapshot(s) for aggregate %s", rowsAffected, aggregateID)
	
	return nil
}

// ListSnapshots lists snapshots for an aggregate type
func (pss *PostgresSnapshotStore) ListSnapshots(aggregateType string, limit int) ([]*AggregateSnapshot, error) {
	query := fmt.Sprintf(`
		SELECT id, aggregate_id, aggregate_type, version, snapshot_data, created_at, tenant_id
		FROM %s 
		WHERE aggregate_type = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, pss.tableName)
	
	rows, err := pss.db.Query(query, aggregateType, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots: %w", err)
	}
	defer rows.Close()
	
	var snapshots []*AggregateSnapshot
	
	for rows.Next() {
		var snapshot AggregateSnapshot
		var tenantID sql.NullString
		
		err := rows.Scan(
			&snapshot.ID,
			&snapshot.AggregateID,
			&snapshot.AggregateType,
			&snapshot.Version,
			&snapshot.Data,
			&snapshot.CreatedAt,
			&tenantID,
		)
		
		if err != nil {
			return nil, fmt.Errorf("failed to scan snapshot: %w", err)
		}
		
		if tenantID.Valid {
			snapshot.TenantID = tenantID.String
		}
		
		// Decompress data if needed
		if pss.enableCompress {
			decompressedData, err := pss.decompressData(snapshot.Data)
			if err != nil {
				log.Printf("Failed to decompress snapshot data: %v, using raw data", err)
			} else {
				snapshot.Data = decompressedData
			}
		}
		
		snapshots = append(snapshots, &snapshot)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}
	
	return snapshots, nil
}

// createSnapshotTable creates the snapshot table if it doesn't exist
func (pss *PostgresSnapshotStore) createSnapshotTable() error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id VARCHAR(255) PRIMARY KEY,
			aggregate_id VARCHAR(255) NOT NULL UNIQUE,
			aggregate_type VARCHAR(255) NOT NULL,
			version BIGINT NOT NULL,
			snapshot_data BYTEA NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			tenant_id VARCHAR(255)
		)
	`, pss.tableName)
	
	if _, err := pss.db.Exec(query); err != nil {
		return fmt.Errorf("failed to create snapshot table: %w", err)
	}
	
	// Create indexes
	indexes := []string{
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_aggregate_type ON %s(aggregate_type)", pss.tableName, pss.tableName),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_created_at ON %s(created_at)", pss.tableName, pss.tableName),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_tenant_id ON %s(tenant_id)", pss.tableName, pss.tableName),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_version ON %s(aggregate_id, version)", pss.tableName, pss.tableName),
	}
	
	for _, indexQuery := range indexes {
		if _, err := pss.db.Exec(indexQuery); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}
	
	return nil
}

// compressData compresses data using gzip
func (pss *PostgresSnapshotStore) compressData(data []byte) ([]byte, error) {
	var buf strings.Builder
	gz := gzip.NewWriter(&buf)
	
	if _, err := gz.Write(data); err != nil {
		return nil, err
	}
	
	if err := gz.Close(); err != nil {
		return nil, err
	}
	
	return []byte(buf.String()), nil
}

// decompressData decompresses gzip data
func (pss *PostgresSnapshotStore) decompressData(data []byte) ([]byte, error) {
	reader := strings.NewReader(string(data))
	gz, err := gzip.NewReader(reader)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	
	var buf strings.Builder
	if _, err := io.Copy(&buf, gz); err != nil {
		return nil, err
	}
	
	return []byte(buf.String()), nil
}

// CleanupOldSnapshots removes old snapshots to prevent storage bloat
func (pss *PostgresSnapshotStore) CleanupOldSnapshots(olderThan time.Duration, keepLatest int) error {
	cutoff := time.Now().Add(-olderThan)
	
	// Keep the latest N snapshots for each aggregate type, delete older ones
	query := fmt.Sprintf(`
		DELETE FROM %s
		WHERE id IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (
					PARTITION BY aggregate_type 
					ORDER BY created_at DESC
				) as rn
				FROM %s
				WHERE created_at < $1
			) ranked
			WHERE rn > $2
		)
	`, pss.tableName, pss.tableName)
	
	result, err := pss.db.Exec(query, cutoff, keepLatest)
	if err != nil {
		return fmt.Errorf("failed to cleanup old snapshots: %w", err)
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		log.Printf("Cleaned up %d old snapshots", rowsAffected)
	}
	
	return nil
}

// GetSnapshotStats returns statistics about stored snapshots
func (pss *PostgresSnapshotStore) GetSnapshotStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Total snapshots
	var totalSnapshots int
	err := pss.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", pss.tableName)).Scan(&totalSnapshots)
	if err != nil {
		return nil, fmt.Errorf("failed to get total snapshots: %w", err)
	}
	stats["total_snapshots"] = totalSnapshots
	
	// Snapshots by aggregate type
	typeQuery := fmt.Sprintf(`
		SELECT aggregate_type, COUNT(*) as count
		FROM %s 
		GROUP BY aggregate_type
		ORDER BY count DESC
	`, pss.tableName)
	
	rows, err := pss.db.Query(typeQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshots by type: %w", err)
	}
	defer rows.Close()
	
	typeStats := make(map[string]int)
	for rows.Next() {
		var aggregateType string
		var count int
		if err := rows.Scan(&aggregateType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan type stats: %w", err)
		}
		typeStats[aggregateType] = count
	}
	stats["by_aggregate_type"] = typeStats
	
	// Average snapshot size
	var avgSize sql.NullFloat64
	err = pss.db.QueryRow(fmt.Sprintf("SELECT AVG(LENGTH(snapshot_data)) FROM %s", pss.tableName)).Scan(&avgSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get average size: %w", err)
	}
	if avgSize.Valid {
		stats["average_size_bytes"] = avgSize.Float64
	}
	
	// Oldest and newest snapshots
	var oldestDate, newestDate sql.NullTime
	err = pss.db.QueryRow(fmt.Sprintf("SELECT MIN(created_at), MAX(created_at) FROM %s", pss.tableName)).Scan(&oldestDate, &newestDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get date range: %w", err)
	}
	if oldestDate.Valid {
		stats["oldest_snapshot"] = oldestDate.Time
	}
	if newestDate.Valid {
		stats["newest_snapshot"] = newestDate.Time
	}
	
	return stats, nil
}

// ReplayHandler implementations for different use cases

// ProjectionReplayHandler rebuilds read model projections
type ProjectionReplayHandler struct {
	name        string
	eventTypes  []string
	db          *sql.DB
	projections map[string]ProjectionBuilder
}

// ProjectionBuilder interface for building specific projections
type ProjectionBuilder interface {
	GetProjectionName() string
	BuildFromEvent(event map[string]interface{}) error
	Reset() error
}

// NewProjectionReplayHandler creates a new projection replay handler
func NewProjectionReplayHandler(name string, eventTypes []string, db *sql.DB) *ProjectionReplayHandler {
	return &ProjectionReplayHandler{
		name:        name,
		eventTypes:  eventTypes,
		db:          db,
		projections: make(map[string]ProjectionBuilder),
	}
}

// GetName returns the handler name
func (prh *ProjectionReplayHandler) GetName() string {
	return prh.name
}

// GetEventTypes returns the event types this handler processes
func (prh *ProjectionReplayHandler) GetEventTypes() []string {
	return prh.eventTypes
}

// Handle processes an event to rebuild projections
func (prh *ProjectionReplayHandler) Handle(ctx context.Context, event interface{}) error {
	// Convert event to map for projection building
	eventBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}
	
	var eventMap map[string]interface{}
	if err := json.Unmarshal(eventBytes, &eventMap); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}
	
	// Process with each projection builder
	for _, builder := range prh.projections {
		if err := builder.BuildFromEvent(eventMap); err != nil {
			return fmt.Errorf("projection %s failed: %w", builder.GetProjectionName(), err)
		}
	}
	
	return nil
}

// OnReplayStart prepares for replay
func (prh *ProjectionReplayHandler) OnReplayStart(ctx context.Context) error {
	log.Printf("Starting projection replay for handler: %s", prh.name)
	
	// Reset all projections
	for _, builder := range prh.projections {
		if err := builder.Reset(); err != nil {
			return fmt.Errorf("failed to reset projection %s: %w", builder.GetProjectionName(), err)
		}
	}
	
	return nil
}

// OnReplayComplete handles replay completion
func (prh *ProjectionReplayHandler) OnReplayComplete(ctx context.Context, metrics ReplayMetrics) error {
	log.Printf("Projection replay completed for handler %s: %d events processed", 
		prh.name, metrics.ProcessedEvents)
	return nil
}

// OnReplayError handles replay errors
func (prh *ProjectionReplayHandler) OnReplayError(ctx context.Context, event interface{}, err error) error {
	log.Printf("Projection replay error in handler %s: %v", prh.name, err)
	return nil // Continue on errors
}

// RegisterProjection registers a projection builder
func (prh *ProjectionReplayHandler) RegisterProjection(builder ProjectionBuilder) {
	prh.projections[builder.GetProjectionName()] = builder
}

// AggregateReplayHandler rebuilds aggregates from events
type AggregateReplayHandler struct {
	name       string
	eventTypes []string
	cache      map[string]interface{} // Cache for rebuilt aggregates
}

// NewAggregateReplayHandler creates a new aggregate replay handler
func NewAggregateReplayHandler(name string, eventTypes []string) *AggregateReplayHandler {
	return &AggregateReplayHandler{
		name:       name,
		eventTypes: eventTypes,
		cache:      make(map[string]interface{}),
	}
}

// GetName returns the handler name
func (arh *AggregateReplayHandler) GetName() string {
	return arh.name
}

// GetEventTypes returns the event types this handler processes
func (arh *AggregateReplayHandler) GetEventTypes() []string {
	return arh.eventTypes
}

// Handle processes an event to rebuild aggregates
func (arh *AggregateReplayHandler) Handle(ctx context.Context, event interface{}) error {
	// This would contain the logic to rebuild aggregates from events
	// For now, just cache the event
	if eventMap, ok := event.(map[string]interface{}); ok {
		if aggregateID, exists := eventMap["aggregate_id"]; exists {
			arh.cache[aggregateID.(string)] = eventMap
		}
	}
	
	return nil
}

// OnReplayStart prepares for replay
func (arh *AggregateReplayHandler) OnReplayStart(ctx context.Context) error {
	log.Printf("Starting aggregate replay for handler: %s", arh.name)
	arh.cache = make(map[string]interface{}) // Clear cache
	return nil
}

// OnReplayComplete handles replay completion
func (arh *AggregateReplayHandler) OnReplayComplete(ctx context.Context, metrics ReplayMetrics) error {
	log.Printf("Aggregate replay completed for handler %s: %d aggregates rebuilt", 
		arh.name, len(arh.cache))
	return nil
}

// OnReplayError handles replay errors
func (arh *AggregateReplayHandler) OnReplayError(ctx context.Context, event interface{}, err error) error {
	log.Printf("Aggregate replay error in handler %s: %v", arh.name, err)
	return nil // Continue on errors
}

// GetRebuiltAggregates returns the cache of rebuilt aggregates
func (arh *AggregateReplayHandler) GetRebuiltAggregates() map[string]interface{} {
	return arh.cache
}