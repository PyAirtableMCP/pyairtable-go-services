package cache

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"

	"pyairtable-go/pkg/config"
)

// CacheLevel represents different cache levels
type CacheLevel int

const (
	L1Cache CacheLevel = iota // In-memory cache
	L2Cache                   // Local BigCache
	L3Cache                   // Redis cache
	L4Cache                   // Database cache
)

// MultiLevelCache implements a sophisticated multi-level caching system
type MultiLevelCache struct {
	l1Cache          map[string]*CacheEntry
	l1Mutex          sync.RWMutex
	l2Cache          *bigcache.BigCache
	l3Cache          *redis.Client
	config           *CacheConfig
	metrics          *CacheMetrics
	invalidator      *CacheInvalidator
	warmer           *CacheWarmer
	compressor       *CacheCompressor
	serializer       *CacheSerializer
	ctx              context.Context
	cancel           context.CancelFunc
}

// CacheConfig contains configuration for the multi-level cache
type CacheConfig struct {
	L1MaxEntries      int
	L1TTL             time.Duration
	L2MaxSizeMB       int
	L2TTL             time.Duration
	L3TTL             time.Duration
	CompressionLevel  int
	EnableCompression bool
	EnableMetrics     bool
	WarmupEnabled     bool
	InvalidationTTL   time.Duration
	RedisConfig       *RedisConfig
}

// CacheEntry represents a cache entry with metadata
type CacheEntry struct {
	Data         interface{}
	CreatedAt    time.Time
	LastAccessed time.Time
	AccessCount  int64
	TTL          time.Duration
	Tags         []string
	Size         int64
	Compressed   bool
}

// CacheMetrics contains Prometheus metrics for cache operations
type CacheMetrics struct {
	hits           *prometheus.CounterVec
	misses         *prometheus.CounterVec
	evictions      *prometheus.CounterVec
	compressionRatio *prometheus.GaugeVec
	latency        *prometheus.HistogramVec
	memoryUsage    *prometheus.GaugeVec
	hitRate        *prometheus.GaugeVec
}

// CacheInvalidator handles intelligent cache invalidation
type CacheInvalidator struct {
	patterns        map[string][]string
	dependencyGraph map[string][]string
	mutex           sync.RWMutex
}

// CacheWarmer handles cache warming strategies
type CacheWarmer struct {
	strategies map[string]WarmupStrategy
	scheduler  *time.Ticker
	mutex      sync.RWMutex
}

// WarmupStrategy defines a cache warming strategy
type WarmupStrategy struct {
	Name        string
	KeyPattern  string
	DataLoader  func(key string) (interface{}, error)
	Schedule    time.Duration
	Priority    int
	Enabled     bool
}

// CacheCompressor handles data compression for cache entries
type CacheCompressor struct {
	level     int
	threshold int64 // Only compress data larger than this size
}

// CacheSerializer handles serialization/deserialization of cache data
type CacheSerializer struct {
	enableBinary bool
}

// NewMultiLevelCache creates a new multi-level cache system
func NewMultiLevelCache(cfg *config.Config, cacheConfig *CacheConfig) (*MultiLevelCache, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Set default configuration
	if cacheConfig == nil {
		cacheConfig = &CacheConfig{
			L1MaxEntries:      10000,
			L1TTL:             5 * time.Minute,
			L2MaxSizeMB:       256,
			L2TTL:             15 * time.Minute,
			L3TTL:             60 * time.Minute,
			CompressionLevel:  6,
			EnableCompression: true,
			EnableMetrics:     true,
			WarmupEnabled:     true,
			InvalidationTTL:   30 * time.Second,
		}
	}

	// Initialize L2 cache (BigCache)
	l2Config := bigcache.DefaultConfig(cacheConfig.L2TTL)
	l2Config.HardMaxCacheSize = cacheConfig.L2MaxSizeMB
	l2Config.MaxEntrySize = 1024 * 1024 // 1MB max entry size
	l2Config.Verbose = false
	l2Config.CleanWindow = 5 * time.Minute

	l2Cache, err := bigcache.NewBigCache(l2Config)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create L2 cache: %w", err)
	}

	// Initialize L3 cache (Redis)
	l3Cache, err := NewRedisClient(cfg)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create L3 cache: %w", err)
	}

	// Initialize metrics
	var metrics *CacheMetrics
	if cacheConfig.EnableMetrics {
		metrics = NewCacheMetrics()
	}

	// Initialize components
	invalidator := NewCacheInvalidator()
	warmer := NewCacheWarmer()
	compressor := NewCacheCompressor(cacheConfig.CompressionLevel, 1024) // Compress data > 1KB
	serializer := NewCacheSerializer(true)

	cache := &MultiLevelCache{
		l1Cache:     make(map[string]*CacheEntry),
		l2Cache:     l2Cache,
		l3Cache:     l3Cache.Client,
		config:      cacheConfig,
		metrics:     metrics,
		invalidator: invalidator,
		warmer:      warmer,
		compressor:  compressor,
		serializer:  serializer,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start background processes
	go cache.startMaintenanceRoutines()
	go cache.startWarmupScheduler()

	log.Println("Multi-level cache system initialized successfully")
	return cache, nil
}

// Get retrieves data from the cache using the multi-level strategy
func (c *MultiLevelCache) Get(key string) (interface{}, bool) {
	start := time.Now()
	var level CacheLevel
	var data interface{}
	var found bool

	// Try L1 cache first (in-memory)
	if data, found = c.getFromL1(key); found {
		level = L1Cache
	} else if data, found = c.getFromL2(key); found {
		level = L2Cache
		// Promote to L1
		c.setToL1(key, data, c.config.L1TTL)
	} else if data, found = c.getFromL3(key); found {
		level = L3Cache
		// Promote to L2 and L1
		c.setToL2(key, data, c.config.L2TTL)
		c.setToL1(key, data, c.config.L1TTL)
	}

	// Record metrics
	if c.metrics != nil {
		latency := time.Since(start).Seconds()
		if found {
			c.metrics.hits.WithLabelValues(string(rune(level))).Inc()
		} else {
			c.metrics.misses.WithLabelValues("all").Inc()
		}
		c.metrics.latency.WithLabelValues("get", string(rune(level))).Observe(latency)
	}

	return data, found
}

// Set stores data in all cache levels with intelligent placement
func (c *MultiLevelCache) Set(key string, data interface{}, ttl time.Duration, tags ...string) error {
	start := time.Now()

	// Determine cache placement strategy based on data size and access patterns
	serializedData, err := c.serializer.Serialize(data)
	if err != nil {
		return fmt.Errorf("serialization failed: %w", err)
	}

	dataSize := int64(len(serializedData))
	
	// Create cache entry
	entry := &CacheEntry{
		Data:         data,
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
		AccessCount:  1,
		TTL:          ttl,
		Tags:         tags,
		Size:         dataSize,
		Compressed:   false,
	}

	// Compress if enabled and data is large enough
	if c.config.EnableCompression && dataSize > int64(c.compressor.threshold) {
		compressedData, err := c.compressor.Compress(serializedData)
		if err == nil && len(compressedData) < len(serializedData) {
			serializedData = compressedData
			entry.Compressed = true
			
			// Record compression metrics
			if c.metrics != nil {
				ratio := float64(len(compressedData)) / float64(dataSize)
				c.metrics.compressionRatio.WithLabelValues(key).Set(ratio)
			}
		}
	}

	// Store in appropriate cache levels
	if dataSize < 1024*10 { // Small data goes to L1
		c.setToL1(key, data, ttl)
	}
	
	if dataSize < 1024*100 { // Medium data goes to L2
		c.setToL2(key, data, ttl)
	}
	
	// All data goes to L3 (Redis)
	c.setToL3(key, serializedData, ttl)

	// Record metrics
	if c.metrics != nil {
		c.metrics.latency.WithLabelValues("set", "all").Observe(time.Since(start).Seconds())
	}

	return nil
}

// SetWithStrategy stores data using a specific caching strategy
func (c *MultiLevelCache) SetWithStrategy(key string, data interface{}, strategy CacheStrategy) error {
	switch strategy {
	case WriteThrough:
		return c.setWriteThrough(key, data)
	case WriteBack:
		return c.setWriteBack(key, data)
	case WriteAround:
		return c.setWriteAround(key, data)
	default:
		return c.Set(key, data, c.config.L1TTL)
	}
}

// Delete removes data from all cache levels
func (c *MultiLevelCache) Delete(key string) error {
	c.deleteFromL1(key)
	c.deleteFromL2(key)
	c.deleteFromL3(key)
	
	// Record metrics
	if c.metrics != nil {
		c.metrics.evictions.WithLabelValues("manual").Inc()
	}
	
	return nil
}

// InvalidateByTags invalidates all cache entries with specific tags
func (c *MultiLevelCache) InvalidateByTags(tags ...string) error {
	return c.invalidator.InvalidateByTags(c, tags...)
}

// InvalidateByPattern invalidates cache entries matching a pattern
func (c *MultiLevelCache) InvalidateByPattern(pattern string) error {
	return c.invalidator.InvalidateByPattern(c, pattern)
}

// Warmup warms the cache using configured strategies
func (c *MultiLevelCache) Warmup(strategyName string) error {
	return c.warmer.ExecuteStrategy(c, strategyName)
}

// L1 Cache Operations (In-Memory)
func (c *MultiLevelCache) getFromL1(key string) (interface{}, bool) {
	c.l1Mutex.RLock()
	defer c.l1Mutex.RUnlock()

	entry, exists := c.l1Cache[key]
	if !exists {
		return nil, false
	}

	// Check TTL
	if time.Since(entry.CreatedAt) > entry.TTL {
		go c.deleteFromL1(key) // Async cleanup
		return nil, false
	}

	// Update access metadata
	entry.LastAccessed = time.Now()
	entry.AccessCount++

	return entry.Data, true
}

func (c *MultiLevelCache) setToL1(key string, data interface{}, ttl time.Duration) {
	c.l1Mutex.Lock()
	defer c.l1Mutex.Unlock()

	// Check if we need to evict entries
	if len(c.l1Cache) >= c.config.L1MaxEntries {
		c.evictLRUFromL1()
	}

	c.l1Cache[key] = &CacheEntry{
		Data:         data,
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
		AccessCount:  1,
		TTL:          ttl,
	}
}

func (c *MultiLevelCache) deleteFromL1(key string) {
	c.l1Mutex.Lock()
	defer c.l1Mutex.Unlock()
	delete(c.l1Cache, key)
}

func (c *MultiLevelCache) evictLRUFromL1() {
	if len(c.l1Cache) == 0 {
		return
	}

	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.l1Cache {
		if oldestKey == "" || entry.LastAccessed.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.LastAccessed
		}
	}

	if oldestKey != "" {
		delete(c.l1Cache, oldestKey)
		if c.metrics != nil {
			c.metrics.evictions.WithLabelValues("l1_lru").Inc()
		}
	}
}

// L2 Cache Operations (BigCache)
func (c *MultiLevelCache) getFromL2(key string) (interface{}, bool) {
	data, err := c.l2Cache.Get(key)
	if err != nil {
		return nil, false
	}

	// Deserialize data
	var result interface{}
	if err := c.serializer.Deserialize(data, &result); err != nil {
		log.Printf("L2 cache deserialization error: %v", err)
		return nil, false
	}

	return result, true
}

func (c *MultiLevelCache) setToL2(key string, data interface{}, ttl time.Duration) {
	serializedData, err := c.serializer.Serialize(data)
	if err != nil {
		log.Printf("L2 cache serialization error: %v", err)
		return
	}

	if err := c.l2Cache.Set(key, serializedData); err != nil {
		log.Printf("L2 cache set error: %v", err)
	}
}

func (c *MultiLevelCache) deleteFromL2(key string) {
	c.l2Cache.Delete(key)
}

// L3 Cache Operations (Redis)
func (c *MultiLevelCache) getFromL3(key string) (interface{}, bool) {
	data, err := c.l3Cache.Get(c.ctx, key).Bytes()
	if err != nil {
		return nil, false
	}

	// Check if data is compressed
	var finalData []byte
	if c.isCompressed(data) {
		decompressed, err := c.compressor.Decompress(data)
		if err != nil {
			log.Printf("L3 cache decompression error: %v", err)
			return nil, false
		}
		finalData = decompressed
	} else {
		finalData = data
	}

	// Deserialize data
	var result interface{}
	if err := c.serializer.Deserialize(finalData, &result); err != nil {
		log.Printf("L3 cache deserialization error: %v", err)
		return nil, false
	}

	return result, true
}

func (c *MultiLevelCache) setToL3(key string, data []byte, ttl time.Duration) {
	if err := c.l3Cache.Set(c.ctx, key, data, ttl).Err(); err != nil {
		log.Printf("L3 cache set error: %v", err)
	}
}

func (c *MultiLevelCache) deleteFromL3(key string) {
	c.l3Cache.Del(c.ctx, key)
}

// Cache Strategy Types
type CacheStrategy int

const (
	WriteThrough CacheStrategy = iota
	WriteBack
	WriteAround
)

func (c *MultiLevelCache) setWriteThrough(key string, data interface{}) error {
	// Write to cache and database simultaneously
	if err := c.Set(key, data, c.config.L1TTL); err != nil {
		return err
	}
	// TODO: Write to database
	return nil
}

func (c *MultiLevelCache) setWriteBack(key string, data interface{}) error {
	// Write to cache immediately, database later
	if err := c.Set(key, data, c.config.L1TTL); err != nil {
		return err
	}
	// TODO: Schedule database write
	return nil
}

func (c *MultiLevelCache) setWriteAround(key string, data interface{}) error {
	// Write directly to database, bypass cache
	// TODO: Write to database
	return nil
}

// Utility functions
func (c *MultiLevelCache) isCompressed(data []byte) bool {
	// Simple magic number check - in production, use proper headers
	return len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b
}

func (c *MultiLevelCache) generateCacheKey(prefix string, params ...interface{}) string {
	h := md5.New()
	h.Write([]byte(prefix))
	for _, param := range params {
		h.Write([]byte(fmt.Sprintf("%v", param)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Background maintenance routines
func (c *MultiLevelCache) startMaintenanceRoutines() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.performMaintenance()
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *MultiLevelCache) performMaintenance() {
	// Clean expired L1 entries
	c.cleanExpiredL1Entries()
	
	// Update memory usage metrics
	c.updateMemoryMetrics()
	
	// Calculate and update hit rates
	c.updateHitRateMetrics()
}

func (c *MultiLevelCache) cleanExpiredL1Entries() {
	c.l1Mutex.Lock()
	defer c.l1Mutex.Unlock()

	now := time.Now()
	for key, entry := range c.l1Cache {
		if now.Sub(entry.CreatedAt) > entry.TTL {
			delete(c.l1Cache, key)
		}
	}
}

func (c *MultiLevelCache) updateMemoryMetrics() {
	if c.metrics == nil {
		return
	}

	// L1 memory usage
	c.l1Mutex.RLock()
	l1Size := len(c.l1Cache)
	var l1Memory int64
	for _, entry := range c.l1Cache {
		l1Memory += entry.Size
	}
	c.l1Mutex.RUnlock()

	c.metrics.memoryUsage.WithLabelValues("l1").Set(float64(l1Memory))

	// L2 memory usage
	l2Stats := c.l2Cache.Stats()
	c.metrics.memoryUsage.WithLabelValues("l2").Set(float64(l2Stats.EntriesCount))
}

func (c *MultiLevelCache) updateHitRateMetrics() {
	// This would calculate hit rates based on accumulated metrics
	// Implementation depends on your specific metrics collection
}

func (c *MultiLevelCache) startWarmupScheduler() {
	if !c.config.WarmupEnabled {
		return
	}

	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.warmer.ExecuteScheduledStrategies(c)
		case <-c.ctx.Done():
			return
		}
	}
}

// Component constructors
func NewCacheInvalidator() *CacheInvalidator {
	return &CacheInvalidator{
		patterns:        make(map[string][]string),
		dependencyGraph: make(map[string][]string),
	}
}

func (ci *CacheInvalidator) InvalidateByTags(cache *MultiLevelCache, tags ...string) error {
	// Implementation for tag-based invalidation
	return nil
}

func (ci *CacheInvalidator) InvalidateByPattern(cache *MultiLevelCache, pattern string) error {
	// Implementation for pattern-based invalidation
	return nil
}

func NewCacheWarmer() *CacheWarmer {
	return &CacheWarmer{
		strategies: make(map[string]WarmupStrategy),
	}
}

func (cw *CacheWarmer) ExecuteStrategy(cache *MultiLevelCache, strategyName string) error {
	// Implementation for cache warming
	return nil
}

func (cw *CacheWarmer) ExecuteScheduledStrategies(cache *MultiLevelCache) {
	// Implementation for scheduled cache warming
}

func NewCacheCompressor(level int, threshold int64) *CacheCompressor {
	return &CacheCompressor{
		level:     level,
		threshold: threshold,
	}
}

func (cc *CacheCompressor) Compress(data []byte) ([]byte, error) {
	// Implementation for data compression
	return data, nil // Placeholder
}

func (cc *CacheCompressor) Decompress(data []byte) ([]byte, error) {
	// Implementation for data decompression
	return data, nil // Placeholder
}

func NewCacheSerializer(enableBinary bool) *CacheSerializer {
	return &CacheSerializer{
		enableBinary: enableBinary,
	}
}

func (cs *CacheSerializer) Serialize(data interface{}) ([]byte, error) {
	return json.Marshal(data)
}

func (cs *CacheSerializer) Deserialize(data []byte, result interface{}) error {
	return json.Unmarshal(data, result)
}

func NewCacheMetrics() *CacheMetrics {
	return &CacheMetrics{
		hits: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cache_hits_total",
				Help: "Total number of cache hits",
			},
			[]string{"level"},
		),
		misses: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cache_misses_total",
				Help: "Total number of cache misses",
			},
			[]string{"level"},
		),
		evictions: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cache_evictions_total",
				Help: "Total number of cache evictions",
			},
			[]string{"reason"},
		),
		compressionRatio: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cache_compression_ratio",
				Help: "Cache compression ratio",
			},
			[]string{"key"},
		),
		latency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "cache_operation_duration_seconds",
				Help:    "Duration of cache operations",
				Buckets: prometheus.ExponentialBuckets(0.0001, 2, 15),
			},
			[]string{"operation", "level"},
		),
		memoryUsage: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cache_memory_usage_bytes",
				Help: "Memory usage of cache levels",
			},
			[]string{"level"},
		),
		hitRate: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cache_hit_rate",
				Help: "Cache hit rate percentage",
			},
			[]string{"level"},
		),
	}
}

// Close gracefully shuts down the multi-level cache
func (c *MultiLevelCache) Close() error {
	log.Println("Shutting down multi-level cache...")
	
	c.cancel()
	
	if c.l2Cache != nil {
		c.l2Cache.Close()
	}
	
	if c.l3Cache != nil {
		c.l3Cache.Close()
	}
	
	log.Println("Multi-level cache shutdown complete")
	return nil
}