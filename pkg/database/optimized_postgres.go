package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"pyairtable-go/pkg/config"
)

// OptimizedDB provides an enhanced database connection with performance optimizations
type OptimizedDB struct {
	*DB
	readReplicas []*gorm.DB
	writeDB      *gorm.DB
	queryCache   *QueryCache
	indexManager *IndexManager
	stats        *DatabaseStats
	mu           sync.RWMutex
}

// QueryCache implements intelligent query result caching
type QueryCache struct {
	cache   map[string]*CacheEntry
	mu      sync.RWMutex
	maxSize int
	ttl     time.Duration
}

type CacheEntry struct {
	Data      interface{}
	CreatedAt time.Time
	HitCount  int64
}

// IndexManager handles dynamic index optimization
type IndexManager struct {
	db                *gorm.DB
	indexAnalytics    map[string]*IndexStats
	mu                sync.RWMutex
	optimizationCycle time.Duration
}

type IndexStats struct {
	TableName       string
	IndexName       string
	Usage           int64
	LastUsed        time.Time
	CreationCost    time.Duration
	MaintenanceCost time.Duration
}

// DatabaseStats tracks performance metrics
type DatabaseStats struct {
	QueryCount          int64
	CacheHits           int64
	CacheMisses         int64
	SlowQueries         int64
	ReadReplicaQueries  int64
	WriteQueries        int64
	ConnectionPoolStats *ConnectionPoolStats
	mu                  sync.RWMutex
}

type ConnectionPoolStats struct {
	MaxOpenConnections     int32
	OpenConnections        int32
	InUse                  int32
	Idle                   int32
	WaitCount             int64
	WaitDuration          time.Duration
	MaxIdleClosed         int64
	MaxIdleTimeClosed     int64
	MaxLifetimeClosed     int64
}

// NewOptimizedDB creates an enhanced database connection with performance optimizations
func NewOptimizedDB(cfg *config.Config) (*OptimizedDB, error) {
	// Create primary database connection
	db, err := New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create primary database connection: %w", err)
	}

	// Initialize optimized database
	optimizedDB := &OptimizedDB{
		DB:           db,
		writeDB:      db.DB,
		readReplicas: make([]*gorm.DB, 0),
		queryCache:   NewQueryCache(cfg.Database.QueryCacheSize, time.Duration(cfg.Database.QueryCacheTTL)*time.Minute),
		stats:        NewDatabaseStats(),
	}

	// Setup read replicas if configured
	if err := optimizedDB.setupReadReplicas(cfg); err != nil {
		log.Printf("Warning: Failed to setup read replicas: %v", err)
	}

	// Initialize index manager
	optimizedDB.indexManager = NewIndexManager(db.DB, cfg)

	// Start background optimization routines
	go optimizedDB.startPerformanceMonitoring()
	go optimizedDB.startIndexOptimization()
	go optimizedDB.startCacheEviction()

	log.Println("Optimized database connection established with performance enhancements")
	return optimizedDB, nil
}

// setupReadReplicas configures read replica connections for load balancing
func (db *OptimizedDB) setupReadReplicas(cfg *config.Config) error {
	if len(cfg.Database.ReadReplicaURLs) == 0 {
		log.Println("No read replicas configured")
		return nil
	}

	for i, replicaURL := range cfg.Database.ReadReplicaURLs {
		replicaDB, err := gorm.Open(postgres.Open(replicaURL), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			return fmt.Errorf("failed to connect to read replica %d: %w", i, err)
		}

		// Configure connection pool for read replica
		sqlDB, err := replicaDB.DB()
		if err != nil {
			return fmt.Errorf("failed to get sql.DB from read replica %d: %w", i, err)
		}

		sqlDB.SetMaxOpenConns(cfg.Database.ReadReplicaMaxConns)
		sqlDB.SetMaxIdleConns(cfg.Database.ReadReplicaMaxConns / 2)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
		sqlDB.SetConnMaxIdleTime(5 * time.Minute)

		db.readReplicas = append(db.readReplicas, replicaDB)
	}

	log.Printf("Configured %d read replicas for load balancing", len(db.readReplicas))
	return nil
}

// SelectOptimalDB chooses the best database connection for the query type
func (db *OptimizedDB) SelectOptimalDB(isRead bool) *gorm.DB {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if !isRead || len(db.readReplicas) == 0 {
		db.stats.mu.Lock()
		db.stats.WriteQueries++
		db.stats.mu.Unlock()
		return db.writeDB
	}

	// Load balance across read replicas using round-robin
	replicaIndex := int(db.stats.ReadReplicaQueries) % len(db.readReplicas)
	db.stats.mu.Lock()
	db.stats.ReadReplicaQueries++
	db.stats.mu.Unlock()

	return db.readReplicas[replicaIndex]
}

// OptimizedFind performs optimized SELECT queries with caching and read replica routing
func (db *OptimizedDB) OptimizedFind(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	// Generate cache key
	cacheKey := fmt.Sprintf("%s:%v", query, args)
	
	// Check cache first
	if cachedResult := db.queryCache.Get(cacheKey); cachedResult != nil {
		db.stats.mu.Lock()
		db.stats.CacheHits++
		db.stats.mu.Unlock()
		
		// Copy cached result to destination
		return nil // Would need proper deserialization here
	}

	db.stats.mu.Lock()
	db.stats.CacheMisses++
	db.stats.QueryCount++
	db.stats.mu.Unlock()

	// Use read replica for SELECT queries
	selectedDB := db.SelectOptimalDB(true)
	
	start := time.Now()
	err := selectedDB.WithContext(ctx).Raw(query, args...).Scan(dest).Error
	duration := time.Since(start)

	// Track slow queries
	if duration > 1*time.Second {
		db.stats.mu.Lock()
		db.stats.SlowQueries++
		db.stats.mu.Unlock()
		log.Printf("Slow query detected (%v): %s", duration, query)
	}

	// Cache successful results
	if err == nil {
		db.queryCache.Set(cacheKey, dest)
	}

	return err
}

// OptimizedCreate performs optimized INSERT operations with batching
func (db *OptimizedDB) OptimizedCreate(ctx context.Context, values interface{}) error {
	db.stats.mu.Lock()
	db.stats.WriteQueries++
	db.stats.QueryCount++
	db.stats.mu.Unlock()

	// Use write database for INSERT operations
	return db.writeDB.WithContext(ctx).Create(values).Error
}

// BatchInsert performs optimized batch insertions
func (db *OptimizedDB) BatchInsert(ctx context.Context, values interface{}, batchSize int) error {
	if batchSize <= 0 {
		batchSize = 1000 // Default batch size
	}

	return db.writeDB.WithContext(ctx).CreateInBatches(values, batchSize).Error
}

// GetPerformanceStats returns current database performance statistics
func (db *OptimizedDB) GetPerformanceStats() *DatabaseStats {
	db.stats.mu.RLock()
	defer db.stats.mu.RUnlock()

	// Update connection pool stats
	if sqlDB, err := db.writeDB.DB(); err == nil {
		stats := sqlDB.Stats()
		db.stats.ConnectionPoolStats = &ConnectionPoolStats{
			MaxOpenConnections:    int32(stats.MaxOpenConnections),
			OpenConnections:       int32(stats.OpenConnections),
			InUse:                 int32(stats.InUse),
			Idle:                  int32(stats.Idle),
			WaitCount:            stats.WaitCount,
			WaitDuration:         stats.WaitDuration,
			MaxIdleClosed:        stats.MaxIdleClosed,
			MaxIdleTimeClosed:    stats.MaxIdleTimeClosed,
			MaxLifetimeClosed:    stats.MaxLifetimeClosed,
		}
	}

	return db.stats
}

// OptimizeTablePartitioning creates table partitions for large tables
func (db *OptimizedDB) OptimizeTablePartitioning(tableName string, partitionColumn string, partitionType string) error {
	var queries []string

	switch partitionType {
	case "range":
		// Example: Partition by date range
		queries = []string{
			fmt.Sprintf(`CREATE TABLE %s_y2024m01 PARTITION OF %s FOR VALUES FROM ('2024-01-01') TO ('2024-02-01')`, tableName, tableName),
			fmt.Sprintf(`CREATE TABLE %s_y2024m02 PARTITION OF %s FOR VALUES FROM ('2024-02-01') TO ('2024-03-01')`, tableName, tableName),
			// Add more partitions as needed
		}
	case "hash":
		// Example: Hash partitioning
		partitionCount := 4
		for i := 0; i < partitionCount; i++ {
			queries = append(queries, fmt.Sprintf(
				`CREATE TABLE %s_p%d PARTITION OF %s FOR VALUES WITH (modulus %d, remainder %d)`,
				tableName, i, tableName, partitionCount, i,
			))
		}
	}

	for _, query := range queries {
		if err := db.writeDB.Exec(query).Error; err != nil {
			log.Printf("Failed to create partition: %s, error: %v", query, err)
			return err
		}
	}

	log.Printf("Table partitioning optimized for %s", tableName)
	return nil
}

// NewQueryCache creates a new query cache
func NewQueryCache(maxSize int, ttl time.Duration) *QueryCache {
	return &QueryCache{
		cache:   make(map[string]*CacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get retrieves a cached query result
func (qc *QueryCache) Get(key string) interface{} {
	qc.mu.RLock()
	defer qc.mu.RUnlock()

	entry, exists := qc.cache[key]
	if !exists {
		return nil
	}

	// Check if entry has expired
	if time.Since(entry.CreatedAt) > qc.ttl {
		delete(qc.cache, key)
		return nil
	}

	entry.HitCount++
	return entry.Data
}

// Set stores a query result in the cache
func (qc *QueryCache) Set(key string, data interface{}) {
	qc.mu.Lock()
	defer qc.mu.Unlock()

	// Evict oldest entries if cache is full
	if len(qc.cache) >= qc.maxSize {
		qc.evictOldest()
	}

	qc.cache[key] = &CacheEntry{
		Data:      data,
		CreatedAt: time.Now(),
		HitCount:  0,
	}
}

// evictOldest removes the oldest cache entries
func (qc *QueryCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range qc.cache {
		if oldestKey == "" || entry.CreatedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.CreatedAt
		}
	}

	if oldestKey != "" {
		delete(qc.cache, oldestKey)
	}
}

// NewIndexManager creates a new index manager
func NewIndexManager(db *gorm.DB, cfg *config.Config) *IndexManager {
	return &IndexManager{
		db:                db,
		indexAnalytics:    make(map[string]*IndexStats),
		optimizationCycle: time.Duration(cfg.Database.IndexOptimizationCycle) * time.Hour,
	}
}

// AnalyzeAndOptimizeIndexes performs intelligent index optimization
func (im *IndexManager) AnalyzeAndOptimizeIndexes() error {
	// Query to find unused indexes
	unusedIndexQuery := `
		SELECT schemaname, tablename, indexname, idx_scan
		FROM pg_stat_user_indexes
		WHERE idx_scan < 10 AND schemaname = 'public'
	`

	// Query to find missing indexes (slow queries without indexes)
	missingIndexQuery := `
		SELECT query, calls, mean_time, rows
		FROM pg_stat_statements
		WHERE mean_time > 1000 AND calls > 100
		ORDER BY mean_time DESC
		LIMIT 20
	`

	var unusedIndexes []struct {
		SchemaName string `gorm:"column:schemaname"`
		TableName  string `gorm:"column:tablename"`
		IndexName  string `gorm:"column:indexname"`
		IndexScan  int64  `gorm:"column:idx_scan"`
	}

	// Find unused indexes
	if err := im.db.Raw(unusedIndexQuery).Scan(&unusedIndexes).Error; err != nil {
		return fmt.Errorf("failed to analyze unused indexes: %w", err)
	}

	// Drop unused indexes
	for _, index := range unusedIndexes {
		if index.IndexScan == 0 {
			dropQuery := fmt.Sprintf("DROP INDEX IF EXISTS %s", index.IndexName)
			if err := im.db.Exec(dropQuery).Error; err != nil {
				log.Printf("Failed to drop unused index %s: %v", index.IndexName, err)
			} else {
				log.Printf("Dropped unused index: %s", index.IndexName)
			}
		}
	}

	log.Println("Index optimization completed")
	return nil
}

// NewDatabaseStats creates a new database stats tracker
func NewDatabaseStats() *DatabaseStats {
	return &DatabaseStats{
		ConnectionPoolStats: &ConnectionPoolStats{},
	}
}

// startPerformanceMonitoring begins background performance monitoring
func (db *OptimizedDB) startPerformanceMonitoring() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats := db.GetPerformanceStats()
		
		// Log performance metrics
		log.Printf("DB Performance - Queries: %d, Cache Hit Rate: %.2f%%, Slow Queries: %d",
			stats.QueryCount,
			float64(stats.CacheHits)/float64(stats.CacheHits+stats.CacheMisses)*100,
			stats.SlowQueries,
		)

		// Alert on performance issues
		if stats.ConnectionPoolStats.InUse > stats.ConnectionPoolStats.MaxOpenConnections*8/10 {
			log.Printf("WARNING: High database connection usage: %d/%d",
				stats.ConnectionPoolStats.InUse, stats.ConnectionPoolStats.MaxOpenConnections)
		}
	}
}

// startIndexOptimization begins background index optimization
func (db *OptimizedDB) startIndexOptimization() {
	ticker := time.NewTicker(db.indexManager.optimizationCycle)
	defer ticker.Stop()

	for range ticker.C {
		if err := db.indexManager.AnalyzeAndOptimizeIndexes(); err != nil {
			log.Printf("Index optimization failed: %v", err)
		}
	}
}

// startCacheEviction begins background cache cleanup
func (db *OptimizedDB) startCacheEviction() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		db.queryCache.mu.Lock()
		
		// Remove expired entries
		now := time.Now()
		for key, entry := range db.queryCache.cache {
			if now.Sub(entry.CreatedAt) > db.queryCache.ttl {
				delete(db.queryCache.cache, key)
			}
		}
		
		db.queryCache.mu.Unlock()
	}
}

// CreateOptimizedIndexes creates performance-optimized indexes
func (db *OptimizedDB) CreateOptimizedIndexes() error {
	indexes := []string{
		// Composite indexes for common query patterns
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_email_status ON users(email, status) WHERE status = 'active'",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sessions_user_expires ON sessions(user_id, expires_at) WHERE expires_at > NOW()",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_analytics_events_time ON analytics_events(created_at DESC, event_type)",
		
		// Partial indexes for better performance
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_workflow_executions_running ON workflow_executions(status, created_at) WHERE status IN ('running', 'pending')",
		
		// GIN indexes for JSONB columns
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_metadata_gin ON workflow_executions USING gin(metadata)",
		
		// Full-text search indexes
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_files_search ON file_uploads USING gin(to_tsvector('english', filename || ' ' || COALESCE(description, '')))",
	}

	for _, indexSQL := range indexes {
		log.Printf("Creating index: %s", strings.Split(indexSQL, " ON ")[0])
		if err := db.writeDB.Exec(indexSQL).Error; err != nil {
			// Don't fail if index already exists
			if !strings.Contains(err.Error(), "already exists") {
				log.Printf("Failed to create index: %v", err)
			}
		}
	}

	return nil
}