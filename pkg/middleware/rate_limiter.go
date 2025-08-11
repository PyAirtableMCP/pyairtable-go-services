package middleware

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// RateLimiter provides comprehensive rate limiting capabilities
type RateLimiter struct {
	redis  *redis.Client
	config *RateLimitConfig
	logger *zap.Logger
}

// RateLimitConfig defines rate limiting rules
type RateLimitConfig struct {
	GlobalLimit    int                // requests per minute globally
	UserLimit      int                // requests per minute per user
	IPLimit        int                // requests per minute per IP
	EndpointLimits map[string]int     // specific endpoint limits
	WindowSize     time.Duration      // rate limit window (default: 1 minute)
	KeyPrefix      string             // Redis key prefix
	SkipPaths      []string           // paths to skip rate limiting
}

// NewRateLimiter creates a new rate limiter instance
func NewRateLimiter(redisClient *redis.Client, config *RateLimitConfig, logger *zap.Logger) *RateLimiter {
	if config.WindowSize == 0 {
		config.WindowSize = time.Minute
	}
	if config.KeyPrefix == "" {
		config.KeyPrefix = "ratelimit"
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &RateLimiter{
		redis:  redisClient,
		config: config,
		logger: logger,
	}
}

// DefaultRateLimitConfig returns a secure default configuration
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		GlobalLimit: 10000,           // 10k requests/minute globally
		UserLimit:   1000,            // 1k requests/minute per user
		IPLimit:     500,             // 500 requests/minute per IP
		WindowSize:  time.Minute,
		KeyPrefix:   "pyairtable:ratelimit",
		EndpointLimits: map[string]int{
			"/auth/login":    10,   // 10 login attempts per minute
			"/auth/register": 5,    // 5 registration attempts per minute
			"/api/upload":    100,  // 100 uploads per minute
			"/api/export":    20,   // 20 exports per minute
		},
		SkipPaths: []string{
			"/health",
			"/metrics",
			"/ready",
			"/ping",
		},
	}
}

// Middleware returns a Fiber middleware for rate limiting
func (rl *RateLimiter) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip rate limiting for certain paths
		path := c.Path()
		for _, skipPath := range rl.config.SkipPaths {
			if path == skipPath {
				return c.Next()
			}
		}

		ctx := context.Background()
		now := time.Now()
		
		// Get identifiers
		ip := c.IP()
		userID := rl.getUserID(c)
		endpoint := c.Route().Path

		// Check global rate limit
		if !rl.checkLimit(ctx, "global", "", rl.config.GlobalLimit, now) {
			rl.logger.Warn("Global rate limit exceeded",
				zap.String("ip", ip),
				zap.String("path", path),
			)
			return rl.rateLimitResponse(c, "global_rate_limit_exceeded", rl.config.WindowSize)
		}

		// Check IP rate limit
		if !rl.checkLimit(ctx, "ip", ip, rl.config.IPLimit, now) {
			rl.logger.Warn("IP rate limit exceeded",
				zap.String("ip", ip),
				zap.String("path", path),
			)
			return rl.rateLimitResponse(c, "ip_rate_limit_exceeded", rl.config.WindowSize)
		}

		// Check user rate limit (if authenticated)
		if userID != "" {
			if !rl.checkLimit(ctx, "user", userID, rl.config.UserLimit, now) {
				rl.logger.Warn("User rate limit exceeded",
					zap.String("user_id", userID),
					zap.String("ip", ip),
					zap.String("path", path),
				)
				return rl.rateLimitResponse(c, "user_rate_limit_exceeded", rl.config.WindowSize)
			}
		}

		// Check endpoint-specific limits
		if limit, exists := rl.config.EndpointLimits[endpoint]; exists {
			key := fmt.Sprintf("%s:%s", endpoint, ip)
			if !rl.checkLimit(ctx, "endpoint", key, limit, now) {
				rl.logger.Warn("Endpoint rate limit exceeded",
					zap.String("endpoint", endpoint),
					zap.String("ip", ip),
					zap.Int("limit", limit),
				)
				return rl.rateLimitResponse(c, "endpoint_rate_limit_exceeded", rl.config.WindowSize)
			}
		}

		return c.Next()
	}
}

// checkLimit checks if the rate limit is exceeded
func (rl *RateLimiter) checkLimit(ctx context.Context, limitType, identifier string, limit int, now time.Time) bool {
	if limit <= 0 {
		return true // No limit set
	}

	key := rl.buildKey(limitType, identifier)
	window := now.Truncate(rl.config.WindowSize).Unix()
	
	// Use Redis pipeline for atomic operations
	pipe := rl.redis.Pipeline()
	
	// Increment counter for current window
	counterKey := fmt.Sprintf("%s:%d", key, window)
	pipe.Incr(ctx, counterKey)
	pipe.Expire(ctx, counterKey, rl.config.WindowSize*2) // Keep for 2 windows
	
	// Execute pipeline
	results, err := pipe.Exec(ctx)
	if err != nil {
		rl.logger.Error("Redis pipeline error", zap.Error(err))
		return true // Allow request on Redis error (fail open)
	}
	
	// Get current count
	count, err := results[0].(*redis.IntCmd).Result()
	if err != nil {
		rl.logger.Error("Failed to get rate limit count", zap.Error(err))
		return true
	}

	allowed := count <= int64(limit)
	
	if !allowed {
		rl.logger.Debug("Rate limit check failed",
			zap.String("key", key),
			zap.Int64("count", count),
			zap.Int("limit", limit),
		)
	}

	return allowed
}

// buildKey builds a Redis key for rate limiting
func (rl *RateLimiter) buildKey(limitType, identifier string) string {
	if identifier == "" {
		return fmt.Sprintf("%s:%s", rl.config.KeyPrefix, limitType)
	}
	return fmt.Sprintf("%s:%s:%s", rl.config.KeyPrefix, limitType, identifier)
}

// getUserID extracts user ID from the request context
func (rl *RateLimiter) getUserID(c *fiber.Ctx) string {
	// Try to get user ID from JWT claims stored in context
	if userID := c.Locals("user_id"); userID != nil {
		if uid, ok := userID.(string); ok {
			return uid
		}
	}
	return ""
}

// rateLimitResponse returns a rate limit exceeded response
func (rl *RateLimiter) rateLimitResponse(c *fiber.Ctx, errorType string, retryAfter time.Duration) error {
	c.Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
	c.Set("X-RateLimit-Limit", strconv.Itoa(rl.getApplicableLimit(c)))
	c.Set("X-RateLimit-Remaining", "0")
	c.Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(retryAfter).Unix(), 10))
	
	return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
		"error":       errorType,
		"message":     "Rate limit exceeded. Please try again later.",
		"retry_after": int(retryAfter.Seconds()),
		"timestamp":   time.Now().ISO8601(),
	})
}

// getApplicableLimit returns the applicable rate limit for the current request
func (rl *RateLimiter) getApplicableLimit(c *fiber.Ctx) int {
	endpoint := c.Route().Path
	if limit, exists := rl.config.EndpointLimits[endpoint]; exists {
		return limit
	}
	
	if rl.getUserID(c) != "" {
		return rl.config.UserLimit
	}
	
	return rl.config.IPLimit
}

// GetStats returns current rate limiting statistics
func (rl *RateLimiter) GetStats(ctx context.Context) (*RateLimitStats, error) {
	stats := &RateLimitStats{
		WindowSize: rl.config.WindowSize,
		Timestamp:  time.Now(),
	}
	
	// Get global stats
	globalKey := rl.buildKey("global", "")
	window := time.Now().Truncate(rl.config.WindowSize).Unix()
	counterKey := fmt.Sprintf("%s:%d", globalKey, window)
	
	count, err := rl.redis.Get(ctx, counterKey).Int()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	
	stats.GlobalCount = count
	stats.GlobalLimit = rl.config.GlobalLimit
	
	return stats, nil
}

// RateLimitStats holds rate limiting statistics
type RateLimitStats struct {
	GlobalCount int           `json:"global_count"`
	GlobalLimit int           `json:"global_limit"`
	WindowSize  time.Duration `json:"window_size"`
	Timestamp   time.Time     `json:"timestamp"`
}

// Reset clears all rate limiting data (use with caution)
func (rl *RateLimiter) Reset(ctx context.Context) error {
	pattern := fmt.Sprintf("%s:*", rl.config.KeyPrefix)
	keys, err := rl.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}
	
	if len(keys) > 0 {
		return rl.redis.Del(ctx, keys...).Err()
	}
	
	return nil
}

// AdaptiveRateLimiter provides dynamic rate limiting based on system load
type AdaptiveRateLimiter struct {
	*RateLimiter
	baseConfig    *RateLimitConfig
	loadThreshold float64
}

// NewAdaptiveRateLimiter creates a rate limiter that adjusts limits based on system load
func NewAdaptiveRateLimiter(redisClient *redis.Client, config *RateLimitConfig, logger *zap.Logger) *AdaptiveRateLimiter {
	return &AdaptiveRateLimiter{
		RateLimiter:   NewRateLimiter(redisClient, config, logger),
		baseConfig:    config,
		loadThreshold: 0.8, // Reduce limits when system load > 80%
	}
}

// UpdateLimitsBasedOnLoad adjusts rate limits based on system metrics
func (arl *AdaptiveRateLimiter) UpdateLimitsBasedOnLoad(cpuUsage, memoryUsage float64) {
	if cpuUsage > arl.loadThreshold || memoryUsage > arl.loadThreshold {
		// Reduce limits by 50% during high load
		arl.config.GlobalLimit = arl.baseConfig.GlobalLimit / 2
		arl.config.UserLimit = arl.baseConfig.UserLimit / 2
		arl.config.IPLimit = arl.baseConfig.IPLimit / 2
		
		arl.logger.Warn("Rate limits reduced due to high system load",
			zap.Float64("cpu_usage", cpuUsage),
			zap.Float64("memory_usage", memoryUsage),
		)
	} else {
		// Restore normal limits
		arl.config.GlobalLimit = arl.baseConfig.GlobalLimit
		arl.config.UserLimit = arl.baseConfig.UserLimit
		arl.config.IPLimit = arl.baseConfig.IPLimit
	}
}