package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	"github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"pyairtable-go/pkg/config"
)

// DB wraps the GORM database connection with health monitoring
type DB struct {
	*gorm.DB
	config *config.Config
	healthTicker *time.Ticker
	context.CancelFunc
}

// New creates a new database connection with SSL and retry logic
func New(cfg *config.Config) (*DB, error) {
	// Configure SSL connection
	dsn, err := buildDSNWithSSL(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build DSN with SSL: %w", err)
	}

	// Configure GORM
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	if cfg.IsDevelopment() {
		gormConfig.Logger = logger.Default.LogMode(logger.Info)
	}

	// Connect with retry logic
	db, err := connectWithRetry(dsn, gormConfig, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database after retries: %w", err)
	}

	// Configure connection pool with enhanced settings
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// Enhanced connection pool configuration
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetime) * time.Second)
	sqlDB.SetConnMaxIdleTime(time.Duration(cfg.Database.ConnMaxIdleTime) * time.Second)

	// Create database wrapper with health monitoring
	ctx, cancel := context.WithCancel(context.Background())
	dbWrapper := &DB{
		DB:         db,
		config:     cfg,
		CancelFunc: cancel,
	}

	// Start health check monitoring
	dbWrapper.startHealthCheck(ctx)

	return dbWrapper, nil
}

// Close closes the database connection and stops health monitoring
func (db *DB) Close() error {
	// Stop health check monitoring
	if db.CancelFunc != nil {
		db.CancelFunc()
	}
	if db.healthTicker != nil {
		db.healthTicker.Stop()
	}

	// Close database connection
	sqlDB, err := db.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Ping checks if the database connection is alive
func (db *DB) Ping(ctx context.Context) error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Migrate runs database migrations
func (db *DB) Migrate() error {
	return db.AutoMigrate(
		&User{},
		&Session{},
		&WorkflowExecution{},
		&FileUpload{},
		&AnalyticsEvent{},
	)
}

// BeginTx starts a database transaction
func (db *DB) BeginTx(ctx context.Context) *gorm.DB {
	return db.WithContext(ctx).Begin()
}

// WithContext returns a new DB instance with the given context
func (db *DB) WithContext(ctx context.Context) *DB {
	return &DB{
		DB:         db.DB.WithContext(ctx),
		config:     db.config,
		CancelFunc: db.CancelFunc,
	}
}

// buildDSNWithSSL constructs a DSN with SSL configuration
func buildDSNWithSSL(cfg *config.Config) (string, error) {
	// If custom SSL certificates are provided, register TLS config
	if cfg.Database.SSLCert != "" && cfg.Database.SSLKey != "" {
		tlsConfig, err := buildTLSConfig(cfg)
		if err != nil {
			return "", fmt.Errorf("failed to build TLS config: %w", err)
		}
		
		// Register the custom TLS config
		err = pq.RegisterTLSConfig("custom", tlsConfig)
		if err != nil {
			return "", fmt.Errorf("failed to register TLS config: %w", err)
		}
		
		// Modify DSN to use custom TLS config
		return cfg.Database.URL + "&sslmode=" + cfg.Database.SSLMode + "&sslcert=" + cfg.Database.SSLCert + "&sslkey=" + cfg.Database.SSLKey + "&sslrootcert=" + cfg.Database.SSLRootCert, nil
	}
	
	// Use standard SSL mode
	return cfg.Database.URL, nil
}

// buildTLSConfig creates a TLS configuration from certificates
func buildTLSConfig(cfg *config.Config) (*tls.Config, error) {
	tlsConfig := &tls.Config{}
	
	// Load client certificate and key
	if cfg.Database.SSLCert != "" && cfg.Database.SSLKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.Database.SSLCert, cfg.Database.SSLKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	
	// Load CA certificate
	if cfg.Database.SSLRootCert != "" {
		caCert, err := ioutil.ReadFile(cfg.Database.SSLRootCert)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}
	
	return tlsConfig, nil
}

// connectWithRetry attempts to connect to the database with retry logic
func connectWithRetry(dsn string, gormConfig *gorm.Config, cfg *config.Config) (*gorm.DB, error) {
	var db *gorm.DB
	var err error
	
	for attempt := 1; attempt <= cfg.Database.RetryAttempts; attempt++ {
		db, err = gorm.Open(postgres.Open(dsn), gormConfig)
		if err == nil {
			// Test the connection
			sqlDB, testErr := db.DB()
			if testErr == nil {
				if pingErr := sqlDB.Ping(); pingErr == nil {
					log.Printf("Database connection established on attempt %d", attempt)
					return db, nil
				}
				err = pingErr
			} else {
				err = testErr
			}
		}
		
		if attempt < cfg.Database.RetryAttempts {
			backoffDuration := time.Duration(cfg.Database.RetryBackoff*attempt) * time.Second
			log.Printf("Database connection attempt %d failed: %v. Retrying in %v...", attempt, err, backoffDuration)
			time.Sleep(backoffDuration)
		}
	}
	
	return nil, fmt.Errorf("failed to connect after %d attempts: %w", cfg.Database.RetryAttempts, err)
}

// startHealthCheck starts periodic health checks for the database connection
func (db *DB) startHealthCheck(ctx context.Context) {
	interval := time.Duration(db.config.Database.HealthCheckInterval) * time.Second
	db.healthTicker = time.NewTicker(interval)
	
	go func() {
		defer db.healthTicker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				log.Println("Database health check stopped")
				return
			case <-db.healthTicker.C:
				if err := db.Ping(ctx); err != nil {
					log.Printf("Database health check failed: %v", err)
					// Could implement reconnection logic here
				}
			}
		}
	}()
	
	log.Printf("Database health check started with interval: %v", interval)
}

// GetConnectionStats returns database connection pool statistics
func (db *DB) GetConnectionStats() (*sql.DBStats, error) {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return nil, err
	}
	
	stats := sqlDB.Stats()
	return &stats, nil
}