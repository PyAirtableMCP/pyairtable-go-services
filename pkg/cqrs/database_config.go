package cqrs

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

// DatabaseConfig holds configuration for CQRS database separation
type DatabaseConfig struct {
	WriteDB *sql.DB // Event store and command processing
	ReadDB  *sql.DB // Read model projections and queries
}

// DatabaseSeparationStrategy defines how to handle read/write separation
type DatabaseSeparationStrategy int

const (
	SingleDatabase DatabaseSeparationStrategy = iota // Same DB, different schemas
	SeparateDatabases                                 // Completely separate databases
	ReadReplicas                                      // Master-slave with read replicas
)

// CQRSConfig defines CQRS configuration
type CQRSConfig struct {
	Strategy           DatabaseSeparationStrategy
	WriteConnectionStr string
	ReadConnectionStr  string
	WriteSchema        string
	ReadSchema         string
	MaxWriteConns      int
	MaxReadConns       int
	ConnMaxLifetime    int // minutes
	ReadOnly           bool // For read-only replicas
}

// NewDatabaseConfig creates a new database configuration based on strategy
func NewDatabaseConfig(config CQRSConfig) (*DatabaseConfig, error) {
	switch config.Strategy {
	case SingleDatabase:
		return newSingleDatabaseConfig(config)
	case SeparateDatabases:
		return newSeparateDatabasesConfig(config)
	case ReadReplicas:
		return newReadReplicaConfig(config)
	default:
		return nil, fmt.Errorf("unsupported database strategy: %v", config.Strategy)
	}
}

// newSingleDatabaseConfig creates CQRS setup with single database, separate schemas
func newSingleDatabaseConfig(config CQRSConfig) (*DatabaseConfig, error) {
	// Open single database connection
	db, err := sql.Open("postgres", config.WriteConnectionStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool for mixed workload
	db.SetMaxOpenConns(config.MaxWriteConns + config.MaxReadConns)
	db.SetMaxIdleConns((config.MaxWriteConns + config.MaxReadConns) / 2)

	// Create separate schemas for write and read models
	if err := createSchemas(db, config.WriteSchema, config.ReadSchema); err != nil {
		return nil, fmt.Errorf("failed to create schemas: %w", err)
	}

	log.Printf("CQRS: Single database strategy with schemas write='%s', read='%s'", 
		config.WriteSchema, config.ReadSchema)

	return &DatabaseConfig{
		WriteDB: db,
		ReadDB:  db, // Same connection, different schemas via search_path
	}, nil
}

// newSeparateDatabasesConfig creates CQRS setup with completely separate databases
func newSeparateDatabasesConfig(config CQRSConfig) (*DatabaseConfig, error) {
	// Open write database
	writeDB, err := sql.Open("postgres", config.WriteConnectionStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open write database: %w", err)
	}

	// Configure write database for transaction-heavy workload
	writeDB.SetMaxOpenConns(config.MaxWriteConns)
	writeDB.SetMaxIdleConns(config.MaxWriteConns / 2)

	// Open read database
	readDB, err := sql.Open("postgres", config.ReadConnectionStr)
	if err != nil {
		writeDB.Close()
		return nil, fmt.Errorf("failed to open read database: %w", err)
	}

	// Configure read database for high-concurrency reads
	readDB.SetMaxOpenConns(config.MaxReadConns)
	readDB.SetMaxIdleConns(config.MaxReadConns / 2)

	log.Printf("CQRS: Separate databases strategy - Write: %s, Read: %s", 
		maskConnectionString(config.WriteConnectionStr),
		maskConnectionString(config.ReadConnectionStr))

	return &DatabaseConfig{
		WriteDB: writeDB,
		ReadDB:  readDB,
	}, nil
}

// newReadReplicaConfig creates CQRS setup with read replicas
func newReadReplicaConfig(config CQRSConfig) (*DatabaseConfig, error) {
	// Master database for writes
	writeDB, err := sql.Open("postgres", config.WriteConnectionStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open master database: %w", err)
	}

	writeDB.SetMaxOpenConns(config.MaxWriteConns)
	writeDB.SetMaxIdleConns(config.MaxWriteConns / 2)

	// Read replica for reads
	readDB, err := sql.Open("postgres", config.ReadConnectionStr)
	if err != nil {
		writeDB.Close()
		return nil, fmt.Errorf("failed to open read replica: %w", err)
	}

	// Configure read replica with read-only transactions
	readDB.SetMaxOpenConns(config.MaxReadConns)
	readDB.SetMaxIdleConns(config.MaxReadConns / 2)

	// Set read-only mode on read connections
	if config.ReadOnly {
		if err := setReadOnlyMode(readDB); err != nil {
			writeDB.Close()
			readDB.Close()
			return nil, fmt.Errorf("failed to set read-only mode: %w", err)
		}
	}

	log.Printf("CQRS: Read replica strategy - Master: %s, Replica: %s", 
		maskConnectionString(config.WriteConnectionStr),
		maskConnectionString(config.ReadConnectionStr))

	return &DatabaseConfig{
		WriteDB: writeDB,
		ReadDB:  readDB,
	}, nil
}

// createSchemas creates separate schemas for write and read models
func createSchemas(db *sql.DB, writeSchema, readSchema string) error {
	schemas := []string{writeSchema, readSchema}
	
	for _, schema := range schemas {
		query := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schema)
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to create schema %s: %w", schema, err)
		}
	}

	return nil
}

// setReadOnlyMode configures database connection for read-only access
func setReadOnlyMode(db *sql.DB) error {
	_, err := db.Exec("SET default_transaction_read_only = on")
	if err != nil {
		return fmt.Errorf("failed to set read-only mode: %w", err)
	}
	return nil
}

// maskConnectionString masks sensitive information in connection strings
func maskConnectionString(connStr string) string {
	// Simple masking - in production, use proper URL parsing
	return fmt.Sprintf("%s...", connStr[:20])
}

// Close closes database connections
func (dc *DatabaseConfig) Close() error {
	var err error
	
	if dc.WriteDB != nil {
		if closeErr := dc.WriteDB.Close(); closeErr != nil {
			err = closeErr
		}
	}
	
	// Only close ReadDB if it's different from WriteDB
	if dc.ReadDB != nil && dc.ReadDB != dc.WriteDB {
		if closeErr := dc.ReadDB.Close(); closeErr != nil {
			err = closeErr
		}
	}
	
	return err
}

// GetWriteDB returns the write database connection
func (dc *DatabaseConfig) GetWriteDB() *sql.DB {
	return dc.WriteDB
}

// GetReadDB returns the read database connection
func (dc *DatabaseConfig) GetReadDB() *sql.DB {
	return dc.ReadDB
}

// HealthCheck performs health checks on both databases
func (dc *DatabaseConfig) HealthCheck() error {
	if err := dc.WriteDB.Ping(); err != nil {
		return fmt.Errorf("write database health check failed: %w", err)
	}
	
	if dc.ReadDB != dc.WriteDB {
		if err := dc.ReadDB.Ping(); err != nil {
			return fmt.Errorf("read database health check failed: %w", err)
		}
	}
	
	return nil
}