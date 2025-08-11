package middleware

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"github.com/golang/groupcache/lru"

	"pyairtable-go/pkg/config"
)

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	tokens    chan struct{}
	ticker    *time.Ticker
	rate      time.Duration
	capacity  int
	stop      chan bool
	mu        sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(requestsPerSecond int, burstCapacity int) *RateLimiter {
	rl := &RateLimiter{
		tokens:   make(chan struct{}, burstCapacity),
		rate:     time.Second / time.Duration(requestsPerSecond),
		capacity: burstCapacity,
		stop:     make(chan bool),
	}

	// Fill initial tokens
	for i := 0; i < burstCapacity; i++ {
		rl.tokens <- struct{}{}
	}

	// Start token refill goroutine
	rl.ticker = time.NewTicker(rl.rate)
	go rl.refillTokens()

	return rl
}

// refillTokens periodically adds tokens to the bucket
func (rl *RateLimiter) refillTokens() {
	for {
		select {
		case <-rl.ticker.C:
			select {
			case rl.tokens <- struct{}{}:
			default:
				// Bucket is full, ignore
			}
		case <-rl.stop:
			rl.ticker.Stop()
			return
		}
	}
}

// Allow checks if a request should be allowed
func (rl *RateLimiter) Allow() bool {
	select {
	case <-rl.tokens:
		return true
	default:
		return false
	}
}

// Close stops the rate limiter
func (rl *RateLimiter) Close() {
	close(rl.stop)
}

// RateLimitManager manages rate limiters per client
type RateLimitManager struct {
	limiters map[string]*RateLimiter
	mu       sync.RWMutex
	config   *config.Config
}

// NewRateLimitManager creates a new rate limit manager
func NewRateLimitManager(cfg *config.Config) *RateLimitManager {
	return &RateLimitManager{
		limiters: make(map[string]*RateLimiter),
		config:   cfg,
	}
}

// GetLimiter gets or creates a rate limiter for a client
func (rlm *RateLimitManager) GetLimiter(clientID string) *RateLimiter {
	rlm.mu.RLock()
	if limiter, exists := rlm.limiters[clientID]; exists {
		rlm.mu.RUnlock()
		return limiter
	}
	rlm.mu.RUnlock()

	rlm.mu.Lock()
	defer rlm.mu.Unlock()

	// Check again in case another goroutine created it
	if limiter, exists := rlm.limiters[clientID]; exists {
		return limiter
	}

	// Create new rate limiter with configured limits
	limiter := NewRateLimiter(rlm.config.RateLimit.RequestsPerSecond, rlm.config.RateLimit.BurstCapacity)
	rlm.limiters[clientID] = limiter
	return limiter
}

// Cleanup removes inactive rate limiters
func (rlm *RateLimitManager) Cleanup() {
	rlm.mu.Lock()
	defer rlm.mu.Unlock()

	// In a real implementation, you'd track last access time and clean up inactive limiters
	// For now, this is a placeholder
}

// Global rate limit manager
var globalRateLimitManager *RateLimitManager

// InitializeRateLimitManager initializes the global rate limit manager
func InitializeRateLimitManager(cfg *config.Config) {
	globalRateLimitManager = NewRateLimitManager(cfg)
}

// RateLimitUnaryInterceptor returns a gRPC unary interceptor with rate limiting
func RateLimitUnaryInterceptor(cfg *config.Config) grpc.UnaryServerInterceptor {
	if globalRateLimitManager == nil {
		InitializeRateLimitManager(cfg)
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Extract client ID from metadata (IP address or API key)
		clientID := extractClientID(ctx)
		
		limiter := globalRateLimitManager.GetLimiter(clientID)
		if !limiter.Allow() {
			return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded for client %s", clientID)
		}

		return handler(ctx, req)
	}
}

// RateLimitStreamInterceptor returns a gRPC stream interceptor with rate limiting
func RateLimitStreamInterceptor(cfg *config.Config) grpc.StreamServerInterceptor {
	if globalRateLimitManager == nil {
		InitializeRateLimitManager(cfg)
	}

	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		clientID := extractClientID(ss.Context())
		
		limiter := globalRateLimitManager.GetLimiter(clientID)
		if !limiter.Allow() {
			return status.Errorf(codes.ResourceExhausted, "rate limit exceeded for client %s", clientID)
		}

		return handler(srv, ss)
	}
}

// extractClientID extracts client identifier from context
func extractClientID(ctx context.Context) string {
	// Try to get client IP from metadata
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if ips := md.Get("x-forwarded-for"); len(ips) > 0 {
			return ips[0]
		}
		if ips := md.Get("x-real-ip"); len(ips) > 0 {
			return ips[0]
		}
		// Try to get API key
		if keys := md.Get("authorization"); len(keys) > 0 {
			return keys[0]
		}
	}
	return "unknown"
}

// TimeoutUnaryInterceptor returns a unary interceptor that enforces request timeouts
func TimeoutUnaryInterceptor(timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// TimeoutStreamInterceptor returns a stream interceptor that enforces request timeouts
func TimeoutStreamInterceptor(timeout time.Duration) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		_ = cancel // We can't defer cancel here as the stream might live longer
		return streamer(ctx, desc, cc, method, opts...)
	}
}

// RetryUnaryInterceptor returns a unary interceptor that implements retry logic
func RetryUnaryInterceptor(maxAttempts int) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var lastErr error
		
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			err := invoker(ctx, method, req, reply, cc, opts...)
			if err == nil {
				return nil
			}

			lastErr = err
			
			// Check if error is retryable
			if !isRetryableError(err) {
				return err
			}

			// Don't retry on the last attempt
			if attempt == maxAttempts {
				break
			}

			// Exponential backoff
			backoff := time.Duration(attempt) * time.Millisecond * 100
			time.Sleep(backoff)
		}

		return lastErr
	}
}

// isRetryableError checks if an error should trigger a retry
func isRetryableError(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}

	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Aborted:
		return true
	default:
		return false
	}
}

// LoggingUnaryInterceptor returns a unary interceptor that logs requests and responses
func LoggingUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		clientID := extractClientID(ctx)
		
		log.Printf("gRPC Request: method=%s, client=%s", info.FullMethod, clientID)
		
		resp, err := handler(ctx, req)
		
		duration := time.Since(start)
		status := "OK"
		if err != nil {
			status = "ERROR"
		}
		
		log.Printf("gRPC Response: method=%s, client=%s, status=%s, duration=%v", 
			info.FullMethod, clientID, status, duration)
		
		return resp, err
	}
}

// LoggingStreamInterceptor returns a stream interceptor that logs stream operations
func LoggingStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		clientID := extractClientID(ss.Context())
		
		log.Printf("gRPC Stream: method=%s, client=%s", info.FullMethod, clientID)
		
		err := handler(srv, ss)
		
		duration := time.Since(start)
		status := "OK"
		if err != nil {
			status = "ERROR"
		}
		
		log.Printf("gRPC Stream End: method=%s, client=%s, status=%s, duration=%v", 
			info.FullMethod, clientID, status, duration)
		
		return err
	}
}

// AuthUnaryInterceptor returns a unary interceptor that handles authentication
func AuthUnaryInterceptor(cfg *config.Config) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Skip auth for health checks
		if info.FullMethod == "/grpc.health.v1.Health/Check" {
			return handler(ctx, req)
		}

		// Extract and validate API key or JWT token
		if err := validateAuth(ctx, cfg); err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
		}

		return handler(ctx, req)
	}
}

// AuthStreamInterceptor returns a stream interceptor that handles authentication
func AuthStreamInterceptor(cfg *config.Config) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := validateAuth(ss.Context(), cfg); err != nil {
			return status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
		}

		return handler(srv, ss)
	}
}

// validateAuth validates authentication from context
func validateAuth(ctx context.Context, cfg *config.Config) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return fmt.Errorf("missing metadata")
	}

	// Check for API key
	if apiKeys := md.Get("api-key"); len(apiKeys) > 0 {
		if apiKeys[0] == cfg.Auth.APIKey {
			return nil
		}
		return fmt.Errorf("invalid API key")
	}

	// Check for Authorization header
	if authHeaders := md.Get("authorization"); len(authHeaders) > 0 {
		// In a real implementation, validate JWT token here
		return nil
	}

	return fmt.Errorf("missing authentication")
}

// RequestCacheInterceptor implements request-level caching
type RequestCacheInterceptor struct {
	cache *lru.Cache
	ttl   time.Duration
	mu    sync.RWMutex
}

type cacheEntry struct {
	response  interface{}
	timestamp time.Time
}

// NewRequestCacheInterceptor creates a new request cache interceptor
func NewRequestCacheInterceptor(maxEntries int, ttl time.Duration) *RequestCacheInterceptor {
	return &RequestCacheInterceptor{
		cache: lru.New(maxEntries),
		ttl:   ttl,
	}
}

// UnaryInterceptor returns a unary interceptor with caching
func (rci *RequestCacheInterceptor) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Generate cache key from method and request
		cacheKey := fmt.Sprintf("%s:%v", info.FullMethod, req)
		
		// Check cache
		rci.mu.RLock()
		if value, ok := rci.cache.Get(cacheKey); ok {
			entry := value.(*cacheEntry)
			if time.Since(entry.timestamp) < rci.ttl {
				rci.mu.RUnlock()
				return entry.response, nil
			}
		}
		rci.mu.RUnlock()

		// Execute request
		resp, err := handler(ctx, req)
		if err == nil {
			// Cache successful response
			rci.mu.Lock()
			rci.cache.Add(cacheKey, &cacheEntry{
				response:  resp,
				timestamp: time.Now(),
			})
			rci.mu.Unlock()
		}

		return resp, err
	}
}

// MemoryMonitoringInterceptor monitors memory usage
func MemoryMonitoringInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		var m1, m2 runtime.MemStats
		runtime.ReadMemStats(&m1)
		
		resp, err := handler(ctx, req)
		
		runtime.ReadMemStats(&m2)
		memUsed := m2.Alloc - m1.Alloc
		
		if memUsed > 1024*1024 { // Log if more than 1MB used
			log.Printf("High memory usage for %s: %d bytes", info.FullMethod, memUsed)
		}
		
		return resp, err
	}
}