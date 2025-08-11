package cqrs

import (
	"time"

	"github.com/pyairtable-compose/go-services/pkg/cache"
)

// CQRSServiceConfig represents the complete CQRS configuration
type CQRSServiceConfig struct {
	Database   CQRSConfig
	Projection ProjectionConfig
	Cache      CacheConfig
}

// DefaultCQRSServiceConfig returns default CQRS configuration
func DefaultCQRSServiceConfig() CQRSServiceConfig {
	return CQRSServiceConfig{
		Database: CQRSConfig{
			Strategy:           SingleDatabase,
			WriteConnectionStr: "",
			ReadConnectionStr:  "",
			WriteSchema:        "write_schema",
			ReadSchema:         "read_schema",
			MaxWriteConns:      10,
			MaxReadConns:       50,
			ConnMaxLifetime:    30,
			ReadOnly:           false,
		},
		Projection: ProjectionConfig{
			Strategy:        AsynchronousProjection,
			BatchSize:       100,
			BatchTimeout:    5 * time.Second,
			WorkerCount:     4,
			RetryAttempts:   3,
			RetryBackoff:    1 * time.Second,
			EnableCaching:   true,
			CacheExpiration: 15 * time.Minute,
		},
		Cache: CacheConfig{
			Strategy:           ReadThrough,
			DefaultExpiration:  15 * time.Minute,
			MaxMemorySize:      1024, // 1GB
			EvictionPolicy:     "LRU",
			PrefetchEnabled:    true,
			CompressionEnabled: false,
			EncryptionEnabled:  false,
		},
	}
}

// CQRSService represents the complete CQRS implementation
type CQRSService struct {
	DatabaseConfig     *DatabaseConfig
	ProjectionManager  *ProjectionManager
	CacheManager      *CQRSCacheManager
	QueryOptimizer    *QueryOptimizer
	Config            CQRSServiceConfig
}

// NewCQRSService creates a new CQRS service with all components wired up
func NewCQRSService(config CQRSServiceConfig, redisClient *cache.RedisClient) (*CQRSService, error) {
	// Initialize database configuration
	dbConfig, err := NewDatabaseConfig(config.Database)
	if err != nil {
		return nil, err
	}

	// Initialize cache manager
	cacheManager := NewCQRSCacheManager(redisClient, config.Cache)

	// Initialize projection manager
	projectionManager := NewProjectionManager(dbConfig, redisClient, config.Projection)

	// Initialize query optimizer
	queryOptimizer := NewQueryOptimizer(cacheManager)

	return &CQRSService{
		DatabaseConfig:    dbConfig,
		ProjectionManager: projectionManager,
		CacheManager:     cacheManager,
		QueryOptimizer:   queryOptimizer,
		Config:           config,
	}, nil
}

// Close shuts down all CQRS components gracefully
func (cs *CQRSService) Close() error {
	if err := cs.ProjectionManager.Shutdown(); err != nil {
		return err
	}
	
	return cs.DatabaseConfig.Close()
}

// HealthCheck verifies the health of all CQRS components
func (cs *CQRSService) HealthCheck() map[string]interface{} {
	health := make(map[string]interface{})
	
	// Database health
	if err := cs.DatabaseConfig.HealthCheck(); err != nil {
		health["database"] = map[string]interface{}{
			"status": "unhealthy",
			"error":  err.Error(),
		}
	} else {
		health["database"] = map[string]interface{}{
			"status": "healthy",
		}
	}
	
	// Cache health
	health["cache"] = map[string]interface{}{
		"status":   "healthy",
		"hit_ratio": cs.CacheManager.GetHitRatio(),
		"stats":    cs.CacheManager.GetStats(),
	}
	
	// Projections health
	health["projections"] = cs.ProjectionManager.GetProjectionStatus()
	
	return health
}