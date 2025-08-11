package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

func New(cfg RedisConfig, logger *zap.Logger) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
		
		// Connection pool settings optimized for permission service
		PoolSize:        10,              // Max connections in pool
		MinIdleConns:    5,               // Min idle connections
		ConnMaxLifetime: 30 * time.Minute, // Max age of connection
		PoolTimeout:     10 * time.Second, // Pool timeout
		ConnMaxIdleTime: 5 * time.Minute,  // Idle timeout
		
		// Timeouts
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	// Test connection with authentication
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis (check password): %w", err)
	}

	logger.Info("Redis connection established with authentication", 
		zap.String("addr", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)),
		zap.Int("db", cfg.DB),
		zap.Bool("auth_enabled", cfg.Password != ""))

	return client, nil
}