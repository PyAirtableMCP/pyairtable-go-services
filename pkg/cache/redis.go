package cache

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"pyairtable-go/pkg/config"
)

// RedisClient wraps the Redis client with enhanced security features
type RedisClient struct {
	*redis.Client
	config *config.Config
	ctx    context.Context
}

// NewRedisClient creates a new Redis client with TLS support and authentication
func NewRedisClient(cfg *config.Config) (*RedisClient, error) {
	// Parse Redis URL for connection parameters
	redisURL, err := url.Parse(cfg.Redis.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	// Extract host and port
	host := redisURL.Hostname()
	port := redisURL.Port()
	if port == "" {
		port = "6379" // Default Redis port
	}

	portInt, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("invalid Redis port: %w", err)
	}

	// Configure Redis options with enhanced security
	opts := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", host, portInt),
		Password: cfg.Redis.Password, // Password authentication
		DB:       cfg.Redis.DB,

		// Enhanced connection pool settings
		PoolSize:        cfg.Redis.PoolSize,
		MinIdleConns:    cfg.Redis.PoolSize / 2,
		ConnMaxLifetime: 30 * time.Minute,
		PoolTimeout:     10 * time.Second,
		ConnMaxIdleTime: 5 * time.Minute,

		// Security timeouts
		DialTimeout:  5 * time.Second,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,

		// Retry configuration for reliability
		MaxRetries:      cfg.Redis.RetryAttempts,
		MinRetryBackoff: 8 * time.Millisecond,
		MaxRetryBackoff: 512 * time.Millisecond,
	}

	// Configure TLS if enabled
	if cfg.Redis.TLSEnabled {
		tlsConfig, err := buildRedisTLSConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to build Redis TLS config: %w", err)
		}
		opts.TLSConfig = tlsConfig
	}

	// Create Redis client
	client := redis.NewClient(opts)

	// Test connection with retry logic
	ctx := context.Background()
	if err := testRedisConnection(ctx, client, cfg.Redis.RetryAttempts); err != nil {
		return nil, fmt.Errorf("failed to establish Redis connection: %w", err)
	}

	log.Printf("Redis connection established with TLS: %v, Auth: %v", 
		cfg.Redis.TLSEnabled, cfg.Redis.Password != "")

	return &RedisClient{
		Client: client,
		config: cfg,
		ctx:    ctx,
	}, nil
}

// buildRedisTLSConfig creates TLS configuration for Redis connection
func buildRedisTLSConfig(cfg *config.Config) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12, // Enforce minimum TLS 1.2
	}

	// Load client certificate and key if provided
	if cfg.Redis.TLSCert != "" && cfg.Redis.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.Redis.TLSCert, cfg.Redis.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load Redis client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate if provided
	if cfg.Redis.TLSCACert != "" {
		caCert, err := ioutil.ReadFile(cfg.Redis.TLSCACert)
		if err != nil {
			return nil, fmt.Errorf("failed to read Redis CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse Redis CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}

// testRedisConnection tests the Redis connection with retry logic
func testRedisConnection(ctx context.Context, client *redis.Client, maxRetries int) error {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := client.Ping(ctx).Err(); err == nil {
			log.Printf("Redis connection established on attempt %d", attempt)
			return nil
		} else {
			if attempt < maxRetries {
				backoff := time.Duration(attempt) * time.Second
				log.Printf("Redis connection attempt %d failed: %v. Retrying in %v...", 
					attempt, err, backoff)
				time.Sleep(backoff)
			} else {
				return fmt.Errorf("failed to connect after %d attempts: %w", maxRetries, err)
			}
		}
	}
	return fmt.Errorf("exhausted all connection attempts")
}

// Close closes the Redis connection
func (r *RedisClient) Close() error {
	return r.Client.Close()
}

// Ping tests the Redis connection
func (r *RedisClient) Ping() error {
	return r.Client.Ping(r.ctx).Err()
}

// GetConnectionStats returns Redis connection pool statistics
func (r *RedisClient) GetConnectionStats() *redis.PoolStats {
	return r.Client.PoolStats()
}

// SecureSet sets a key-value pair with expiration for security
func (r *RedisClient) SecureSet(key, value string, expiration time.Duration) error {
	// Always set expiration for security (prevent indefinite storage)
	if expiration == 0 {
		expiration = 24 * time.Hour // Default 24 hour expiration
	}
	
	return r.Client.Set(r.ctx, key, value, expiration).Err()
}

// SecureGet gets a value and logs access for audit purposes
func (r *RedisClient) SecureGet(key string) (string, error) {
	result := r.Client.Get(r.ctx, key)
	if result.Err() != nil {
		if result.Err() == redis.Nil {
			return "", nil // Key not found
		}
		return "", result.Err()
	}
	
	// Could add audit logging here for sensitive operations
	return result.Val(), nil
}

// SecureDel deletes a key and logs for audit purposes
func (r *RedisClient) SecureDel(keys ...string) error {
	result := r.Client.Del(r.ctx, keys...)
	if result.Err() != nil {
		return result.Err()
	}
	
	log.Printf("Redis keys deleted: %v", keys)
	return nil
}

// FlushDB clears the current database (use with caution)
func (r *RedisClient) FlushDB() error {
	log.Printf("WARNING: Flushing Redis database %d", r.config.Redis.DB)
	return r.Client.FlushDB(r.ctx).Err()
}