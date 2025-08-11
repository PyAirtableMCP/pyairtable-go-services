package ratelimit

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// LimitStrategy defines different rate limiting strategies
type LimitStrategy string

const (
	StrategyTokenBucket   LimitStrategy = "token_bucket"
	StrategySlidingWindow LimitStrategy = "sliding_window"
	StrategyFixedWindow   LimitStrategy = "fixed_window"
	StrategyLeakyBucket   LimitStrategy = "leaky_bucket"
)

// RateLimitConfig defines rate limiting configuration
type RateLimitConfig struct {
	Strategy         LimitStrategy     `json:"strategy"`
	RequestsPerUnit  int               `json:"requests_per_unit"`
	TimeUnit         time.Duration     `json:"time_unit"`
	BurstCapacity    int               `json:"burst_capacity"`
	KeyGenerator     func(*fiber.Ctx) string `json:"-"`
	SkipSuccessful   bool              `json:"skip_successful"`
	SkipFailedRequest bool             `json:"skip_failed_request"`
	Headers          HeaderConfig      `json:"headers"`
}

// HeaderConfig defines which headers to include in responses
type HeaderConfig struct {
	Enable         bool `json:"enable"`
	Remaining      bool `json:"remaining"`
	Reset          bool `json:"reset"`
	Total          bool `json:"total"`
	RetryAfter     bool `json:"retry_after"`
}

// QuotaConfig defines quota limits for users/tenants
type QuotaConfig struct {
	Daily   int64 `json:"daily"`
	Monthly int64 `json:"monthly"`
	Yearly  int64 `json:"yearly"`
}

// AdvancedRateLimiter provides multiple rate limiting strategies
type AdvancedRateLimiter struct {
	redis      *redis.Client
	config     *RateLimitConfig
	keyPrefix  string
	mu         sync.RWMutex
	localCache map[string]*LocalLimitState
}

// LocalLimitState maintains local state for in-memory limiting
type LocalLimitState struct {
	Tokens     float64   `json:"tokens"`
	LastRefill time.Time `json:"last_refill"`
	WindowStart time.Time `json:"window_start"`
	Count      int       `json:"count"`
}

// LimitResult contains the result of a rate limit check
type LimitResult struct {
	Allowed      bool          `json:"allowed"`
	Remaining    int           `json:"remaining"`
	ResetTime    time.Time     `json:"reset_time"`
	RetryAfter   time.Duration `json:"retry_after"`
	TotalHits    int           `json:"total_hits"`
}

// NewAdvancedRateLimiter creates a new advanced rate limiter
func NewAdvancedRateLimiter(redis *redis.Client, config *RateLimitConfig, keyPrefix string) *AdvancedRateLimiter {
	return &AdvancedRateLimiter{
		redis:      redis,
		config:     config,
		keyPrefix:  keyPrefix,
		localCache: make(map[string]*LocalLimitState),
	}
}

// Allow checks if a request should be allowed
func (a *AdvancedRateLimiter) Allow(c *fiber.Ctx) (*LimitResult, error) {
	key := a.generateKey(c)
	
	switch a.config.Strategy {
	case StrategyTokenBucket:
		return a.tokenBucketAllow(key)
	case StrategySlidingWindow:
		return a.slidingWindowAllow(key)
	case StrategyFixedWindow:
		return a.fixedWindowAllow(key)
	case StrategyLeakyBucket:
		return a.leakyBucketAllow(key)
	default:
		return a.tokenBucketAllow(key)
	}
}

// generateKey generates a unique key for rate limiting
func (a *AdvancedRateLimiter) generateKey(c *fiber.Ctx) string {
	if a.config.KeyGenerator != nil {
		return fmt.Sprintf("%s:%s", a.keyPrefix, a.config.KeyGenerator(c))
	}
	
	// Default key generation based on IP
	return fmt.Sprintf("%s:ip:%s", a.keyPrefix, c.IP())
}

// tokenBucketAllow implements token bucket algorithm
func (a *AdvancedRateLimiter) tokenBucketAllow(key string) (*LimitResult, error) {
	now := time.Now()
	
	// Try Redis first for distributed limiting
	if a.redis != nil {
		return a.distributedTokenBucket(key, now)
	}
	
	// Fallback to local implementation
	return a.localTokenBucket(key, now)
}

// distributedTokenBucket implements distributed token bucket using Redis
func (a *AdvancedRateLimiter) distributedTokenBucket(key string, now time.Time) (*LimitResult, error) {
	script := `
		local key = KEYS[1]
		local capacity = tonumber(ARGV[1])
		local refill_rate = tonumber(ARGV[2])
		local requested = tonumber(ARGV[3])
		local now = tonumber(ARGV[4])
		
		local bucket = redis.call('HMGET', key, 'tokens', 'last_refill')
		local tokens = tonumber(bucket[1]) or capacity
		local last_refill = tonumber(bucket[2]) or now
		
		-- Calculate tokens to add based on elapsed time
		local elapsed = now - last_refill
		local tokens_to_add = elapsed * refill_rate / 1000000000 -- Convert nanoseconds to seconds
		tokens = math.min(capacity, tokens + tokens_to_add)
		
		local allowed = tokens >= requested
		if allowed then
			tokens = tokens - requested
		end
		
		-- Update bucket state
		redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
		redis.call('EXPIRE', key, 3600) -- Expire in 1 hour
		
		return {allowed and 1 or 0, math.floor(tokens), math.ceil((capacity - tokens) / refill_rate * 1000000000)}
	`
	
	capacity := a.config.BurstCapacity
	if capacity <= 0 {
		capacity = a.config.RequestsPerUnit
	}
	
	refillRate := float64(a.config.RequestsPerUnit) / a.config.TimeUnit.Seconds()
	
	result, err := a.redis.Eval(context.Background(), script, []string{key}, 
		capacity, refillRate, 1, now.UnixNano()).Result()
	if err != nil {
		return nil, fmt.Errorf("token bucket script failed: %w", err)
	}
	
	resultSlice := result.([]interface{})
	allowed := resultSlice[0].(int64) == 1
	remaining := int(resultSlice[1].(int64))
	retryAfter := time.Duration(resultSlice[2].(int64))
	
	return &LimitResult{
		Allowed:    allowed,
		Remaining:  remaining,
		ResetTime:  now.Add(retryAfter),
		RetryAfter: retryAfter,
		TotalHits:  capacity - remaining,
	}, nil
}

// localTokenBucket implements local token bucket
func (a *AdvancedRateLimiter) localTokenBucket(key string, now time.Time) (*LimitResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	state, exists := a.localCache[key]
	if !exists {
		capacity := float64(a.config.BurstCapacity)
		if capacity <= 0 {
			capacity = float64(a.config.RequestsPerUnit)
		}
		
		state = &LocalLimitState{
			Tokens:     capacity,
			LastRefill: now,
		}
		a.localCache[key] = state
	}
	
	// Calculate tokens to add
	elapsed := now.Sub(state.LastRefill)
	refillRate := float64(a.config.RequestsPerUnit) / a.config.TimeUnit.Seconds()
	tokensToAdd := elapsed.Seconds() * refillRate
	
	capacity := float64(a.config.BurstCapacity)
	if capacity <= 0 {
		capacity = float64(a.config.RequestsPerUnit)
	}
	
	state.Tokens = math.Min(capacity, state.Tokens+tokensToAdd)
	state.LastRefill = now
	
	allowed := state.Tokens >= 1.0
	if allowed {
		state.Tokens -= 1.0
	}
	
	retryAfter := time.Duration(0)
	if !allowed {
		retryAfter = time.Duration((1.0-state.Tokens)/refillRate) * time.Second
	}
	
	return &LimitResult{
		Allowed:    allowed,
		Remaining:  int(state.Tokens),
		ResetTime:  now.Add(retryAfter),
		RetryAfter: retryAfter,
		TotalHits:  int(capacity - state.Tokens),
	}, nil
}

// slidingWindowAllow implements sliding window algorithm
func (a *AdvancedRateLimiter) slidingWindowAllow(key string) (*LimitResult, error) {
	now := time.Now()
	
	if a.redis != nil {
		return a.distributedSlidingWindow(key, now)
	}
	
	return a.localSlidingWindow(key, now)
}

// distributedSlidingWindow implements distributed sliding window using Redis
func (a *AdvancedRateLimiter) distributedSlidingWindow(key string, now time.Time) (*LimitResult, error) {
	script := `
		local key = KEYS[1]
		local window_size = tonumber(ARGV[1])
		local limit = tonumber(ARGV[2])
		local now = tonumber(ARGV[3])
		
		-- Remove expired entries
		redis.call('ZREMRANGEBYSCORE', key, '-inf', now - window_size * 1000)
		
		-- Count current requests
		local current = redis.call('ZCARD', key)
		
		if current < limit then
			-- Add current request
			redis.call('ZADD', key, now, now)
			redis.call('EXPIRE', key, math.ceil(window_size))
			return {1, limit - current - 1, window_size * 1000}
		else
			return {0, 0, window_size * 1000}
		end
	`
	
	windowSize := a.config.TimeUnit.Seconds()
	limit := a.config.RequestsPerUnit
	
	result, err := a.redis.Eval(context.Background(), script, []string{key}, 
		windowSize, limit, now.UnixMilli()).Result()
	if err != nil {
		return nil, fmt.Errorf("sliding window script failed: %w", err)
	}
	
	resultSlice := result.([]interface{})
	allowed := resultSlice[0].(int64) == 1
	remaining := int(resultSlice[1].(int64))
	resetAfter := time.Duration(resultSlice[2].(int64)) * time.Millisecond
	
	return &LimitResult{
		Allowed:    allowed,
		Remaining:  remaining,
		ResetTime:  now.Add(resetAfter),
		RetryAfter: resetAfter,
		TotalHits:  limit - remaining,
	}, nil
}

// localSlidingWindow implements local sliding window
func (a *AdvancedRateLimiter) localSlidingWindow(key string, now time.Time) (*LimitResult, error) {
	// Simplified local implementation
	// In production, you'd want a more sophisticated approach
	return a.localTokenBucket(key, now)
}

// fixedWindowAllow implements fixed window algorithm
func (a *AdvancedRateLimiter) fixedWindowAllow(key string) (*LimitResult, error) {
	now := time.Now()
	
	if a.redis != nil {
		return a.distributedFixedWindow(key, now)
	}
	
	return a.localFixedWindow(key, now)
}

// distributedFixedWindow implements distributed fixed window using Redis
func (a *AdvancedRateLimiter) distributedFixedWindow(key string, now time.Time) (*LimitResult, error) {
	windowStart := now.Truncate(a.config.TimeUnit)
	windowKey := fmt.Sprintf("%s:%d", key, windowStart.Unix())
	
	script := `
		local key = KEYS[1]
		local limit = tonumber(ARGV[1])
		local window_size = tonumber(ARGV[2])
		
		local current = redis.call('GET', key)
		if current == false then
			current = 0
		else
			current = tonumber(current)
		end
		
		if current < limit then
			local new_val = redis.call('INCR', key)
			redis.call('EXPIRE', key, window_size)
			return {1, limit - new_val, window_size}
		else
			return {0, 0, redis.call('TTL', key)}
		end
	`
	
	result, err := a.redis.Eval(context.Background(), script, []string{windowKey}, 
		a.config.RequestsPerUnit, int(a.config.TimeUnit.Seconds())).Result()
	if err != nil {
		return nil, fmt.Errorf("fixed window script failed: %w", err)
	}
	
	resultSlice := result.([]interface{})
	allowed := resultSlice[0].(int64) == 1
	remaining := int(resultSlice[1].(int64))
	ttl := time.Duration(resultSlice[2].(int64)) * time.Second
	
	return &LimitResult{
		Allowed:    allowed,
		Remaining:  remaining,
		ResetTime:  windowStart.Add(a.config.TimeUnit),
		RetryAfter: ttl,
		TotalHits:  a.config.RequestsPerUnit - remaining,
	}, nil
}

// localFixedWindow implements local fixed window
func (a *AdvancedRateLimiter) localFixedWindow(key string, now time.Time) (*LimitResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	windowStart := now.Truncate(a.config.TimeUnit)
	
	state, exists := a.localCache[key]
	if !exists || state.WindowStart.Before(windowStart) {
		state = &LocalLimitState{
			WindowStart: windowStart,
			Count:       0,
		}
		a.localCache[key] = state
	}
	
	allowed := state.Count < a.config.RequestsPerUnit
	if allowed {
		state.Count++
	}
	
	remaining := a.config.RequestsPerUnit - state.Count
	resetTime := windowStart.Add(a.config.TimeUnit)
	retryAfter := resetTime.Sub(now)
	
	return &LimitResult{
		Allowed:    allowed,
		Remaining:  remaining,
		ResetTime:  resetTime,
		RetryAfter: retryAfter,
		TotalHits:  state.Count,
	}, nil
}

// leakyBucketAllow implements leaky bucket algorithm
func (a *AdvancedRateLimiter) leakyBucketAllow(key string) (*LimitResult, error) {
	// Simplified implementation - leaky bucket is similar to token bucket
	// but with constant leak rate
	return a.tokenBucketAllow(key)
}

// SetHeaders sets rate limit headers on the response
func (a *AdvancedRateLimiter) SetHeaders(c *fiber.Ctx, result *LimitResult) {
	if !a.config.Headers.Enable {
		return
	}
	
	if a.config.Headers.Total {
		c.Set("X-RateLimit-Limit", fmt.Sprintf("%d", a.config.RequestsPerUnit))
	}
	
	if a.config.Headers.Remaining {
		c.Set("X-RateLimit-Remaining", fmt.Sprintf("%d", result.Remaining))
	}
	
	if a.config.Headers.Reset {
		c.Set("X-RateLimit-Reset", fmt.Sprintf("%d", result.ResetTime.Unix()))
	}
	
	if a.config.Headers.RetryAfter && !result.Allowed {
		c.Set("Retry-After", fmt.Sprintf("%.0f", result.RetryAfter.Seconds()))
	}
}

// QuotaManager manages user and tenant quotas
type QuotaManager struct {
	redis     *redis.Client
	keyPrefix string
}

// NewQuotaManager creates a new quota manager
func NewQuotaManager(redis *redis.Client, keyPrefix string) *QuotaManager {
	return &QuotaManager{
		redis:     redis,
		keyPrefix: keyPrefix,
	}
}

// CheckQuota checks if a user/tenant is within quota limits
func (q *QuotaManager) CheckQuota(userID, tenantID string, quotas *QuotaConfig) (*QuotaResult, error) {
	now := time.Now()
	
	// Check daily quota
	dailyKey := fmt.Sprintf("%s:quota:daily:%s:%s:%s", q.keyPrefix, tenantID, userID, now.Format("2006-01-02"))
	dailyUsage, err := q.getUsage(dailyKey)
	if err != nil {
		return nil, err
	}
	
	// Check monthly quota
	monthlyKey := fmt.Sprintf("%s:quota:monthly:%s:%s:%s", q.keyPrefix, tenantID, userID, now.Format("2006-01"))
	monthlyUsage, err := q.getUsage(monthlyKey)
	if err != nil {
		return nil, err
	}
	
	// Check yearly quota
	yearlyKey := fmt.Sprintf("%s:quota:yearly:%s:%s:%s", q.keyPrefix, tenantID, userID, now.Format("2006"))
	yearlyUsage, err := q.getUsage(yearlyKey)
	if err != nil {
		return nil, err
	}
	
	result := &QuotaResult{
		DailyUsage:    dailyUsage,
		MonthlyUsage:  monthlyUsage,
		YearlyUsage:   yearlyUsage,
		DailyLimit:    quotas.Daily,
		MonthlyLimit:  quotas.Monthly,
		YearlyLimit:   quotas.Yearly,
	}
	
	// Check if any quota is exceeded
	result.Allowed = (quotas.Daily == 0 || dailyUsage < quotas.Daily) &&
		(quotas.Monthly == 0 || monthlyUsage < quotas.Monthly) &&
		(quotas.Yearly == 0 || yearlyUsage < quotas.Yearly)
	
	return result, nil
}

// IncrementUsage increments usage counters
func (q *QuotaManager) IncrementUsage(userID, tenantID string) error {
	now := time.Now()
	
	// Increment daily usage
	dailyKey := fmt.Sprintf("%s:quota:daily:%s:%s:%s", q.keyPrefix, tenantID, userID, now.Format("2006-01-02"))
	err := q.redis.Incr(context.Background(), dailyKey).Err()
	if err != nil {
		return err
	}
	q.redis.Expire(context.Background(), dailyKey, 25*time.Hour) // Expire after day + buffer
	
	// Increment monthly usage
	monthlyKey := fmt.Sprintf("%s:quota:monthly:%s:%s:%s", q.keyPrefix, tenantID, userID, now.Format("2006-01"))
	err = q.redis.Incr(context.Background(), monthlyKey).Err()
	if err != nil {
		return err
	}
	q.redis.Expire(context.Background(), monthlyKey, 32*24*time.Hour) // Expire after month + buffer
	
	// Increment yearly usage
	yearlyKey := fmt.Sprintf("%s:quota:yearly:%s:%s:%s", q.keyPrefix, tenantID, userID, now.Format("2006"))
	err = q.redis.Incr(context.Background(), yearlyKey).Err()
	if err != nil {
		return err
	}
	q.redis.Expire(context.Background(), yearlyKey, 366*24*time.Hour) // Expire after year + buffer
	
	return nil
}

// getUsage gets current usage from Redis
func (q *QuotaManager) getUsage(key string) (int64, error) {
	result, err := q.redis.Get(context.Background(), key).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, err
	}
	
	var usage int64
	err = json.Unmarshal([]byte(result), &usage)
	if err != nil {
		// Try to parse as string
		usage = parseInt64(result)
	}
	
	return usage, nil
}

// QuotaResult contains quota check results
type QuotaResult struct {
	Allowed      bool  `json:"allowed"`
	DailyUsage   int64 `json:"daily_usage"`
	MonthlyUsage int64 `json:"monthly_usage"`
	YearlyUsage  int64 `json:"yearly_usage"`
	DailyLimit   int64 `json:"daily_limit"`
	MonthlyLimit int64 `json:"monthly_limit"`
	YearlyLimit  int64 `json:"yearly_limit"`
}

// Helper function for parsing int64
func parseInt64(s string) int64 {
	var result int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int64(c-'0')
		}
	}
	return result
}