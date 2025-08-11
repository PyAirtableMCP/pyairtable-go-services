package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

// ConnectionPoolConfig defines configuration for database connection pooling
type ConnectionPoolConfig struct {
	// Basic connection settings
	MaxOpenConns    int           // Maximum number of open connections
	MaxIdleConns    int           // Maximum number of idle connections
	ConnMaxLifetime time.Duration // Maximum lifetime of a connection
	ConnMaxIdleTime time.Duration // Maximum idle time for a connection
	
	// Pool behavior settings
	MaxRetries       int           // Maximum number of retry attempts
	RetryDelay       time.Duration // Delay between retry attempts
	HealthCheckInterval time.Duration // Interval for connection health checks
	
	// Workload-specific settings
	ReadPoolSize     int // Connections dedicated to read operations
	WritePoolSize    int // Connections dedicated to write operations
	LongRunningPool  int // Connections for long-running operations
	
	// Monitoring settings
	EnableMetrics    bool          // Enable connection pool metrics
	SlowQueryThreshold time.Duration // Threshold for slow query logging
}

// ConnectionPool manages database connections with workload separation
type ConnectionPool struct {
	config     ConnectionPoolConfig
	readDB     *sql.DB
	writeDB    *sql.DB
	longRunningDB *sql.DB
	
	// Metrics
	metrics *PoolMetrics
	mu      sync.RWMutex
	
	// Health monitoring
	healthTicker *time.Ticker
	ctx          context.Context
	cancel       context.CancelFunc
}

// PoolMetrics tracks connection pool performance
type PoolMetrics struct {
	TotalConnections     int64
	ActiveConnections    int64
	IdleConnections      int64
	ConnectionsCreated   int64
	ConnectionsClosed    int64
	SlowQueries          int64
	FailedConnections    int64
	AverageResponseTime  time.Duration
	LastHealthCheck      time.Time
	HealthCheckErrors    int64
}

// WorkloadType defines different types of database workloads
type WorkloadType int

const (
	ReadWorkload WorkloadType = iota
	WriteWorkload
	LongRunningWorkload
	AnalyticsWorkload
)

// NewConnectionPool creates a new connection pool with workload separation
func NewConnectionPool(connectionString string, config ConnectionPoolConfig) (*ConnectionPool, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	pool := &ConnectionPool{
		config:  config,
		metrics: &PoolMetrics{},
		ctx:     ctx,
		cancel:  cancel,
	}
	
	// Create separate connection pools for different workloads
	if err := pool.initializeConnectionPools(connectionString); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize connection pools: %w", err)
	}
	
	// Start health monitoring if enabled
	if config.HealthCheckInterval > 0 {
		pool.startHealthMonitoring()
	}
	
	log.Printf("Connection pool initialized with read: %d, write: %d, long-running: %d connections",
		config.ReadPoolSize, config.WritePoolSize, config.LongRunningPool)
	
	return pool, nil
}

// initializeConnectionPools creates separate database connections for different workloads
func (cp *ConnectionPool) initializeConnectionPools(connectionString string) error {
	// Read connection pool - optimized for high concurrency reads
	readDB, err := sql.Open("postgres", connectionString)
	if err != nil {
		return fmt.Errorf("failed to open read database: %w", err)
	}
	
	readDB.SetMaxOpenConns(cp.config.ReadPoolSize)
	readDB.SetMaxIdleConns(cp.config.ReadPoolSize / 2)
	readDB.SetConnMaxLifetime(cp.config.ConnMaxLifetime)
	readDB.SetConnMaxIdleTime(cp.config.ConnMaxIdleTime)
	
	// Test read connection
	if err := readDB.Ping(); err != nil {
		readDB.Close()
		return fmt.Errorf("failed to ping read database: %w", err)
	}
	
	cp.readDB = readDB
	
	// Write connection pool - optimized for transactions
	writeDB, err := sql.Open("postgres", connectionString)
	if err != nil {
		readDB.Close()
		return fmt.Errorf("failed to open write database: %w", err)
	}
	
	writeDB.SetMaxOpenConns(cp.config.WritePoolSize)
	writeDB.SetMaxIdleConns(cp.config.WritePoolSize / 2)
	writeDB.SetConnMaxLifetime(cp.config.ConnMaxLifetime)
	writeDB.SetConnMaxIdleTime(cp.config.ConnMaxIdleTime)
	
	// Test write connection
	if err := writeDB.Ping(); err != nil {
		readDB.Close()
		writeDB.Close()
		return fmt.Errorf("failed to ping write database: %w", err)
	}
	
	cp.writeDB = writeDB
	
	// Long-running connection pool - for analytics and migrations
	longRunningDB, err := sql.Open("postgres", connectionString)
	if err != nil {
		readDB.Close()
		writeDB.Close()
		return fmt.Errorf("failed to open long-running database: %w", err)
	}
	
	longRunningDB.SetMaxOpenConns(cp.config.LongRunningPool)
	longRunningDB.SetMaxIdleConns(1) // Keep minimal idle connections
	longRunningDB.SetConnMaxLifetime(time.Hour) // Longer lifetime for long operations
	longRunningDB.SetConnMaxIdleTime(time.Minute * 5)
	
	// Test long-running connection
	if err := longRunningDB.Ping(); err != nil {
		readDB.Close()
		writeDB.Close()
		longRunningDB.Close()
		return fmt.Errorf("failed to ping long-running database: %w", err)
	}
	
	cp.longRunningDB = longRunningDB
	
	return nil
}

// GetConnection returns a database connection for the specified workload
func (cp *ConnectionPool) GetConnection(workload WorkloadType) *sql.DB {
	switch workload {
	case ReadWorkload:
		return cp.readDB
	case WriteWorkload:
		return cp.writeDB
	case LongRunningWorkload, AnalyticsWorkload:
		return cp.longRunningDB
	default:
		// Default to read pool
		return cp.readDB
	}
}

// WithConnection executes a function with a connection appropriate for the workload
func (cp *ConnectionPool) WithConnection(workload WorkloadType, fn func(*sql.DB) error) error {
	db := cp.GetConnection(workload)
	
	start := time.Now()
	err := fn(db)
	duration := time.Since(start)
	
	// Update metrics
	cp.updateMetrics(workload, duration, err)
	
	// Log slow queries
	if cp.config.EnableMetrics && duration > cp.config.SlowQueryThreshold {
		cp.mu.Lock()
		cp.metrics.SlowQueries++
		cp.mu.Unlock()
		log.Printf("Slow query detected: %v duration for %s workload", duration, cp.workloadString(workload))
	}
	
	return err
}

// WithTransaction executes a function within a transaction for the specified workload
func (cp *ConnectionPool) WithTransaction(workload WorkloadType, fn func(*sql.Tx) error) error {
	db := cp.GetConnection(workload)
	
	start := time.Now()
	
	tx, err := db.BeginTx(cp.ctx, &sql.TxOptions{
		Isolation: sql.LevelReadCommitted, // Can be made configurable
		ReadOnly:  workload == ReadWorkload,
	})
	if err != nil {
		cp.updateMetrics(workload, time.Since(start), err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()
	
	if err := fn(tx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			log.Printf("Failed to rollback transaction: %v", rollbackErr)
		}
		cp.updateMetrics(workload, time.Since(start), err)
		return err
	}
	
	if err := tx.Commit(); err != nil {
		cp.updateMetrics(workload, time.Since(start), err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	cp.updateMetrics(workload, time.Since(start), nil)
	return nil
}

// WithRetryableTransaction executes a transaction with retry logic
func (cp *ConnectionPool) WithRetryableTransaction(workload WorkloadType, fn func(*sql.Tx) error) error {
	var lastErr error
	
	for attempt := 0; attempt < cp.config.MaxRetries; attempt++ {
		err := cp.WithTransaction(workload, fn)
		if err == nil {
			return nil // Success
		}
		
		lastErr = err
		
		// Check if error is retryable
		if !isRetryableError(err) {
			return err
		}
		
		if attempt < cp.config.MaxRetries-1 {
			log.Printf("Transaction attempt %d failed: %v, retrying in %v", 
				attempt+1, err, cp.config.RetryDelay)
			time.Sleep(cp.config.RetryDelay * time.Duration(attempt+1))
		}
	}
	
	return fmt.Errorf("transaction failed after %d attempts: %w", cp.config.MaxRetries, lastErr)
}

// updateMetrics updates connection pool metrics
func (cp *ConnectionPool) updateMetrics(workload WorkloadType, duration time.Duration, err error) {
	if !cp.config.EnableMetrics {
		return
	}
	
	cp.mu.Lock()
	defer cp.mu.Unlock()
	
	if err != nil {
		cp.metrics.FailedConnections++
	}
	
	// Update average response time (simple moving average)
	if cp.metrics.AverageResponseTime == 0 {
		cp.metrics.AverageResponseTime = duration
	} else {
		cp.metrics.AverageResponseTime = (cp.metrics.AverageResponseTime + duration) / 2
	}
}

// startHealthMonitoring starts periodic health checks
func (cp *ConnectionPool) startHealthMonitoring() {
	cp.healthTicker = time.NewTicker(cp.config.HealthCheckInterval)
	
	go func() {
		defer cp.healthTicker.Stop()
		
		for {
			select {
			case <-cp.healthTicker.C:
				cp.performHealthCheck()
			case <-cp.ctx.Done():
				return
			}
		}
	}()
}

// performHealthCheck performs health checks on all connection pools
func (cp *ConnectionPool) performHealthCheck() {
	cp.mu.Lock()
	cp.metrics.LastHealthCheck = time.Now()
	cp.mu.Unlock()
	
	pools := map[string]*sql.DB{
		"read":         cp.readDB,
		"write":        cp.writeDB,
		"long_running": cp.longRunningDB,
	}
	
	for name, db := range pools {
		if err := cp.checkPoolHealth(name, db); err != nil {
			cp.mu.Lock()
			cp.metrics.HealthCheckErrors++
			cp.mu.Unlock()
			log.Printf("Health check failed for %s pool: %v", name, err)
		}
	}
	
	// Update connection statistics
	cp.updateConnectionStats()
}

// checkPoolHealth checks the health of a specific connection pool
func (cp *ConnectionPool) checkPoolHealth(poolName string, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(cp.ctx, 5*time.Second)
	defer cancel()
	
	// Simple ping test
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	
	// Test query execution
	var result int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result); err != nil {
		return fmt.Errorf("test query failed: %w", err)
	}
	
	return nil
}

// updateConnectionStats updates connection pool statistics
func (cp *ConnectionPool) updateConnectionStats() {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	
	// Get stats from all pools
	readStats := cp.readDB.Stats()
	writeStats := cp.writeDB.Stats()
	longRunningStats := cp.longRunningDB.Stats()
	
	// Aggregate statistics
	cp.metrics.TotalConnections = int64(readStats.OpenConnections + writeStats.OpenConnections + longRunningStats.OpenConnections)
	cp.metrics.ActiveConnections = int64(readStats.InUse + writeStats.InUse + longRunningStats.InUse)
	cp.metrics.IdleConnections = int64(readStats.Idle + writeStats.Idle + longRunningStats.Idle)
	
	// Log detailed statistics periodically
	if time.Now().Unix()%300 == 0 { // Every 5 minutes
		log.Printf("Connection Pool Stats - Total: %d, Active: %d, Idle: %d, Avg Response: %v",
			cp.metrics.TotalConnections,
			cp.metrics.ActiveConnections,
			cp.metrics.IdleConnections,
			cp.metrics.AverageResponseTime)
	}
}

// GetMetrics returns current connection pool metrics
func (cp *ConnectionPool) GetMetrics() PoolMetrics {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return *cp.metrics
}

// GetDetailedStats returns detailed statistics for all connection pools
func (cp *ConnectionPool) GetDetailedStats() map[string]sql.DBStats {
	return map[string]sql.DBStats{
		"read":         cp.readDB.Stats(),
		"write":        cp.writeDB.Stats(),
		"long_running": cp.longRunningDB.Stats(),
	}
}

// workloadString returns a string representation of the workload type
func (cp *ConnectionPool) workloadString(workload WorkloadType) string {
	switch workload {
	case ReadWorkload:
		return "read"
	case WriteWorkload:
		return "write"
	case LongRunningWorkload:
		return "long_running"
	case AnalyticsWorkload:
		return "analytics"
	default:
		return "unknown"
	}
}

// Close closes all connection pools
func (cp *ConnectionPool) Close() error {
	log.Println("Closing connection pools...")
	
	cp.cancel()
	
	if cp.healthTicker != nil {
		cp.healthTicker.Stop()
	}
	
	var errors []error
	
	if cp.readDB != nil {
		if err := cp.readDB.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close read pool: %w", err))
		}
	}
	
	if cp.writeDB != nil && cp.writeDB != cp.readDB {
		if err := cp.writeDB.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close write pool: %w", err))
		}
	}
	
	if cp.longRunningDB != nil && cp.longRunningDB != cp.readDB && cp.longRunningDB != cp.writeDB {
		if err := cp.longRunningDB.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close long-running pool: %w", err))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("errors closing connection pools: %v", errors)
	}
	
	log.Println("Connection pools closed successfully")
	return nil
}

// ConnectionPoolManager manages multiple connection pools for different databases
type ConnectionPoolManager struct {
	pools   map[string]*ConnectionPool
	mu      sync.RWMutex
	configs map[string]ConnectionPoolConfig
}

// NewConnectionPoolManager creates a new connection pool manager
func NewConnectionPoolManager() *ConnectionPoolManager {
	return &ConnectionPoolManager{
		pools:   make(map[string]*ConnectionPool),
		configs: make(map[string]ConnectionPoolConfig),
	}
}

// AddPool adds a new connection pool
func (cpm *ConnectionPoolManager) AddPool(name, connectionString string, config ConnectionPoolConfig) error {
	cpm.mu.Lock()
	defer cpm.mu.Unlock()
	
	pool, err := NewConnectionPool(connectionString, config)
	if err != nil {
		return fmt.Errorf("failed to create pool %s: %w", name, err)
	}
	
	// Close existing pool if it exists
	if existingPool, exists := cpm.pools[name]; exists {
		existingPool.Close()
	}
	
	cpm.pools[name] = pool
	cpm.configs[name] = config
	
	return nil
}

// GetPool returns a connection pool by name
func (cpm *ConnectionPoolManager) GetPool(name string) (*ConnectionPool, error) {
	cpm.mu.RLock()
	defer cpm.mu.RUnlock()
	
	pool, exists := cpm.pools[name]
	if !exists {
		return nil, fmt.Errorf("connection pool %s not found", name)
	}
	
	return pool, nil
}

// CloseAll closes all connection pools
func (cpm *ConnectionPoolManager) CloseAll() error {
	cpm.mu.Lock()
	defer cpm.mu.Unlock()
	
	var errors []error
	
	for name, pool := range cpm.pools {
		if err := pool.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close pool %s: %w", name, err))
		}
	}
	
	cpm.pools = make(map[string]*ConnectionPool)
	
	if len(errors) > 0 {
		return fmt.Errorf("errors closing pools: %v", errors)
	}
	
	return nil
}

// GetAllMetrics returns metrics for all connection pools
func (cpm *ConnectionPoolManager) GetAllMetrics() map[string]PoolMetrics {
	cpm.mu.RLock()
	defer cpm.mu.RUnlock()
	
	metrics := make(map[string]PoolMetrics)
	for name, pool := range cpm.pools {
		metrics[name] = pool.GetMetrics()
	}
	
	return metrics
}