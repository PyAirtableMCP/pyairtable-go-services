package cdn

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"pyairtable-go/pkg/config"
)

// EdgeCacheManager manages CDN and edge caching functionality
type EdgeCacheManager struct {
	config         *CDNConfig
	providers      map[string]CDNProvider
	edgeLocations  map[string]*EdgeLocation
	metrics        *CDNMetrics
	purgeQueue     chan PurgeRequest
	warmupQueue    chan WarmupRequest
	healthChecker  *HealthChecker
	loadBalancer   *LoadBalancer
	ctx            context.Context
	cancel         context.CancelFunc
	mu             sync.RWMutex
}

// CDNConfig contains configuration for CDN and edge caching
type CDNConfig struct {
	Providers         map[string]ProviderConfig
	DefaultProvider   string
	EdgeLocations     []EdgeLocationConfig
	CacheRules        []CacheRule
	PurgeStrategy     PurgeStrategy
	WarmupStrategy    WarmupStrategy
	HealthCheckConfig HealthCheckConfig
	LoadBalancerConfig LoadBalancerConfig
	SecurityConfig    SecurityConfig
	CompressionConfig CompressionConfig
	EnableMetrics     bool
	EnableLogging     bool
}

// ProviderConfig contains configuration for a specific CDN provider
type ProviderConfig struct {
	Name        string
	Type        ProviderType
	Endpoints   []string
	APIKey      string
	SecretKey   string
	Region      string
	BucketName  string
	DistributionID string
	Enabled     bool
	Priority    int
	MaxRetries  int
	Timeout     time.Duration
}

// EdgeLocationConfig defines an edge cache location
type EdgeLocationConfig struct {
	ID          string
	Region      string
	Provider    string
	Endpoint    string
	Capacity    int64
	Enabled     bool
	Priority    int
	HealthCheck string
}

// CacheRule defines caching behavior for different content types
type CacheRule struct {
	PathPattern  string
	ContentType  string
	TTL          time.Duration
	EdgeTTL      time.Duration
	BrowserTTL   time.Duration
	Compression  bool
	Security     SecurityRule
	Headers      map[string]string
	VaryHeaders  []string
	Priority     int
	Enabled      bool
}

// SecurityRule defines security settings for cached content
type SecurityRule struct {
	EnableHTTPS      bool
	HSTSMaxAge       int
	ContentSecurityPolicy string
	ReferrerPolicy   string
	XFrameOptions    string
	XContentTypeOptions string
}

// CDNProvider interface for different CDN implementations
type CDNProvider interface {
	Name() string
	Purge(urls []string) error
	Warmup(urls []string) error
	GetStats() ProviderStats
	HealthCheck() error
	InvalidatePattern(pattern string) error
	PreloadContent(urls []string) error
}

// ProviderType represents different CDN provider types
type ProviderType string

const (
	CloudFlare ProviderType = "cloudflare"
	AWS        ProviderType = "aws"
	Fastly     ProviderType = "fastly"
	KeyCDN     ProviderType = "keycdn"
	MaxCDN     ProviderType = "maxcdn"
	Custom     ProviderType = "custom"
)

// EdgeLocation represents a single edge cache location
type EdgeLocation struct {
	Config      EdgeLocationConfig
	Cache       map[string]*CacheEntry
	Stats       LocationStats
	Health      HealthStatus
	LastUpdated time.Time
	mu          sync.RWMutex
}

// CacheEntry represents a cached item at edge location
type CacheEntry struct {
	Key          string
	Content      []byte
	ContentType  string
	ETag         string
	LastModified time.Time
	ExpiresAt    time.Time
	HitCount     int64
	Size         int64
	Compressed   bool
	Headers      http.Header
}

// LocationStats contains statistics for an edge location
type LocationStats struct {
	HitCount      int64
	MissCount     int64
	HitRatio      float64
	BytesServed   int64
	RequestCount  int64
	ErrorCount    int64
	ResponseTime  time.Duration
	CacheSize     int64
	CacheEntries  int64
}

// HealthStatus represents health status of an edge location
type HealthStatus struct {
	IsHealthy    bool
	LastCheck    time.Time
	ResponseTime time.Duration
	ErrorCount   int64
	StatusCode   int
	ErrorMessage string
}

// PurgeRequest represents a cache purge request
type PurgeRequest struct {
	URLs      []string
	Pattern   string
	Tags      []string
	Priority  int
	Providers []string
	Callback  func(error)
}

// WarmupRequest represents a cache warmup request
type WarmupRequest struct {
	URLs     []string
	Headers  map[string]string
	Priority int
	Callback func(error)
}

// PurgeStrategy defines cache purge behavior
type PurgeStrategy struct {
	BatchSize       int
	MaxConcurrency  int
	RetryAttempts   int
	RetryBackoff    time.Duration
	EnableAutomatic bool
	AutomaticRules  []AutoPurgeRule
}

// AutoPurgeRule defines automatic purge rules
type AutoPurgeRule struct {
	Pattern     string
	Event       string
	Delay       time.Duration
	Enabled     bool
}

// WarmupStrategy defines cache warmup behavior
type WarmupStrategy struct {
	BatchSize      int
	MaxConcurrency int
	Schedule       string
	Enabled        bool
	Rules          []WarmupRule
}

// WarmupRule defines cache warmup rules
type WarmupRule struct {
	URLs        []string
	Pattern     string
	Schedule    string
	Priority    int
	Enabled     bool
}

// CDNMetrics contains Prometheus metrics for CDN operations
type CDNMetrics struct {
	requestsTotal     *prometheus.CounterVec
	cacheHitRatio     *prometheus.GaugeVec
	responseTime      *prometheus.HistogramVec
	bandwidthUsage    *prometheus.CounterVec
	purgeOperations   *prometheus.CounterVec
	warmupOperations  *prometheus.CounterVec
	edgeHealth        *prometheus.GaugeVec
	cacheSize         *prometheus.GaugeVec
	errorRate         *prometheus.GaugeVec
}

// NewEdgeCacheManager creates a new edge cache manager
func NewEdgeCacheManager(cfg *config.Config, cdnConfig *CDNConfig) (*EdgeCacheManager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Set default configuration
	if cdnConfig == nil {
		cdnConfig = &CDNConfig{
			Providers:       make(map[string]ProviderConfig),
			DefaultProvider: "cloudflare",
			EdgeLocations:   []EdgeLocationConfig{},
			CacheRules:      getDefaultCacheRules(),
			EnableMetrics:   true,
			EnableLogging:   true,
		}
	}

	manager := &EdgeCacheManager{
		config:        cdnConfig,
		providers:     make(map[string]CDNProvider),
		edgeLocations: make(map[string]*EdgeLocation),
		purgeQueue:    make(chan PurgeRequest, 1000),
		warmupQueue:   make(chan WarmupRequest, 1000),
		ctx:           ctx,
		cancel:        cancel,
	}

	// Initialize metrics
	if cdnConfig.EnableMetrics {
		manager.metrics = NewCDNMetrics()
	}

	// Initialize providers
	if err := manager.initializeProviders(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize CDN providers: %w", err)
	}

	// Initialize edge locations
	if err := manager.initializeEdgeLocations(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize edge locations: %w", err)
	}

	// Initialize health checker
	manager.healthChecker = NewHealthChecker(cdnConfig.HealthCheckConfig)

	// Initialize load balancer
	manager.loadBalancer = NewLoadBalancer(cdnConfig.LoadBalancerConfig)

	// Start background processes
	go manager.processPurgeQueue()
	go manager.processWarmupQueue()
	go manager.startHealthChecking()
	go manager.startMetricsCollection()

	log.Println("Edge cache manager initialized successfully")
	return manager, nil
}

// ServeFromCache serves content from edge cache if available
func (ecm *EdgeCacheManager) ServeFromCache(w http.ResponseWriter, r *http.Request) bool {
	start := time.Now()
	cacheKey := ecm.generateCacheKey(r)
	
	// Find best edge location
	location := ecm.selectBestEdgeLocation(r)
	if location == nil {
		return false
	}

	// Try to serve from edge cache
	entry := location.Get(cacheKey)
	if entry == nil {
		ecm.recordCacheMiss(location.Config.ID, r.URL.Path)
		return false
	}

	// Check if entry is expired
	if time.Now().After(entry.ExpiresAt) {
		location.Delete(cacheKey)
		ecm.recordCacheMiss(location.Config.ID, r.URL.Path)
		return false
	}

	// Serve from cache
	ecm.serveCachedContent(w, r, entry)
	ecm.recordCacheHit(location.Config.ID, r.URL.Path, time.Since(start))
	
	return true
}

// CacheResponse caches a response at appropriate edge locations
func (ecm *EdgeCacheManager) CacheResponse(r *http.Request, resp *http.Response, content []byte) error {
	cacheKey := ecm.generateCacheKey(r)
	rule := ecm.findCacheRule(r.URL.Path, resp.Header.Get("Content-Type"))
	
	if rule == nil || rule.TTL == 0 {
		return nil // Not cacheable
	}

	// Create cache entry
	entry := &CacheEntry{
		Key:          cacheKey,
		Content:      content,
		ContentType:  resp.Header.Get("Content-Type"),
		ETag:         resp.Header.Get("ETag"),
		LastModified: time.Now(),
		ExpiresAt:    time.Now().Add(rule.EdgeTTL),
		Size:         int64(len(content)),
		Headers:      resp.Header.Clone(),
	}

	// Compress if enabled
	if rule.Compression && len(content) > 1024 {
		compressed, err := ecm.compressContent(content)
		if err == nil && len(compressed) < len(content) {
			entry.Content = compressed
			entry.Compressed = true
			entry.Size = int64(len(compressed))
		}
	}

	// Cache at selected edge locations
	locations := ecm.selectEdgeLocationsForCaching(r, rule)
	for _, location := range locations {
		location.Set(cacheKey, entry)
	}

	// Set cache headers
	ecm.setCacheHeaders(r.Header, rule)

	return nil
}

// PurgeCache purges cache entries
func (ecm *EdgeCacheManager) PurgeCache(urls []string, providers ...string) error {
	request := PurgeRequest{
		URLs:      urls,
		Priority:  1,
		Providers: providers,
	}

	select {
	case ecm.purgeQueue <- request:
		return nil
	default:
		return fmt.Errorf("purge queue is full")
	}
}

// PurgeCacheByPattern purges cache entries matching a pattern
func (ecm *EdgeCacheManager) PurgeCacheByPattern(pattern string, providers ...string) error {
	request := PurgeRequest{
		Pattern:   pattern,
		Priority:  1,
		Providers: providers,
	}

	select {
	case ecm.purgeQueue <- request:
		return nil
	default:
		return fmt.Errorf("purge queue is full")
	}
}

// WarmupCache warms up cache with specified URLs
func (ecm *EdgeCacheManager) WarmupCache(urls []string, headers map[string]string) error {
	request := WarmupRequest{
		URLs:     urls,
		Headers:  headers,
		Priority: 1,
	}

	select {
	case ecm.warmupQueue <- request:
		return nil
	default:
		return fmt.Errorf("warmup queue is full")
	}
}

// Edge Location Operations

// Get retrieves a cache entry from edge location
func (el *EdgeLocation) Get(key string) *CacheEntry {
	el.mu.RLock()
	defer el.mu.RUnlock()
	
	entry, exists := el.Cache[key]
	if !exists {
		el.Stats.MissCount++
		return nil
	}

	entry.HitCount++
	el.Stats.HitCount++
	el.Stats.BytesServed += entry.Size
	
	return entry
}

// Set stores a cache entry in edge location
func (el *EdgeLocation) Set(key string, entry *CacheEntry) {
	el.mu.Lock()
	defer el.mu.Unlock()

	// Check capacity
	if int64(len(el.Cache)) >= el.Config.Capacity {
		el.evictLRU()
	}

	el.Cache[key] = entry
	el.Stats.CacheEntries++
	el.Stats.CacheSize += entry.Size
	el.LastUpdated = time.Now()
}

// Delete removes a cache entry from edge location
func (el *EdgeLocation) Delete(key string) {
	el.mu.Lock()
	defer el.mu.Unlock()

	if entry, exists := el.Cache[key]; exists {
		delete(el.Cache, key)
		el.Stats.CacheEntries--
		el.Stats.CacheSize -= entry.Size
	}
}

// evictLRU evicts least recently used entries
func (el *EdgeLocation) evictLRU() {
	if len(el.Cache) == 0 {
		return
	}

	var oldestKey string
	var oldestTime time.Time

	for key, entry := range el.Cache {
		if oldestKey == "" || entry.LastModified.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.LastModified
		}
	}

	if oldestKey != "" {
		el.Delete(oldestKey)
	}
}

// Helper methods

func (ecm *EdgeCacheManager) generateCacheKey(r *http.Request) string {
	h := md5.New()
	h.Write([]byte(r.Method))
	h.Write([]byte(r.URL.Path))
	h.Write([]byte(r.URL.RawQuery))
	
	// Include relevant headers
	for _, header := range []string{"Accept", "Accept-Encoding", "Authorization"} {
		if value := r.Header.Get(header); value != "" {
			h.Write([]byte(header))
			h.Write([]byte(value))
		}
	}
	
	return hex.EncodeToString(h.Sum(nil))
}

func (ecm *EdgeCacheManager) selectBestEdgeLocation(r *http.Request) *EdgeLocation {
	// Implement edge location selection logic based on:
	// - Geographic proximity
	// - Health status
	// - Load
	// - Cache hit ratio
	
	ecm.mu.RLock()
	defer ecm.mu.RUnlock()

	var bestLocation *EdgeLocation
	var bestScore float64

	for _, location := range ecm.edgeLocations {
		if !location.Config.Enabled || !location.Health.IsHealthy {
			continue
		}

		score := ecm.calculateLocationScore(location, r)
		if bestLocation == nil || score > bestScore {
			bestLocation = location
			bestScore = score
		}
	}

	return bestLocation
}

func (ecm *EdgeCacheManager) calculateLocationScore(location *EdgeLocation, r *http.Request) float64 {
	score := float64(location.Config.Priority)
	
	// Factor in hit ratio
	hitRatio := float64(location.Stats.HitCount) / float64(location.Stats.HitCount + location.Stats.MissCount)
	score += hitRatio * 10
	
	// Factor in response time
	responseTime := location.Health.ResponseTime.Seconds()
	if responseTime > 0 {
		score -= responseTime * 100
	}
	
	// Factor in error rate
	errorRate := float64(location.Stats.ErrorCount) / float64(location.Stats.RequestCount)
	score -= errorRate * 50
	
	return score
}

func (ecm *EdgeCacheManager) findCacheRule(path, contentType string) *CacheRule {
	for _, rule := range ecm.config.CacheRules {
		if rule.Enabled && ecm.matchesPattern(path, rule.PathPattern) {
			if rule.ContentType == "" || strings.Contains(contentType, rule.ContentType) {
				return &rule
			}
		}
	}
	return nil
}

func (ecm *EdgeCacheManager) matchesPattern(path, pattern string) bool {
	// Simple pattern matching - in production, use more sophisticated matching
	return strings.Contains(path, pattern) || pattern == "*"
}

func (ecm *EdgeCacheManager) serveCachedContent(w http.ResponseWriter, r *http.Request, entry *CacheEntry) {
	// Set headers
	for key, values := range entry.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Set cache headers
	w.Header().Set("X-Cache", "HIT")
	w.Header().Set("X-Cache-Timestamp", entry.LastModified.Format(time.RFC3339))
	w.Header().Set("Age", strconv.Itoa(int(time.Since(entry.LastModified).Seconds())))

	// Handle compression
	content := entry.Content
	if entry.Compressed && !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		// Decompress for clients that don't support compression
		decompressed, err := ecm.decompressContent(content)
		if err == nil {
			content = decompressed
			w.Header().Del("Content-Encoding")
		}
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func (ecm *EdgeCacheManager) compressContent(data []byte) ([]byte, error) {
	// Implement gzip compression
	return data, nil // Placeholder
}

func (ecm *EdgeCacheManager) decompressContent(data []byte) ([]byte, error) {
	// Implement gzip decompression
	return data, nil // Placeholder
}

func (ecm *EdgeCacheManager) setCacheHeaders(headers http.Header, rule *CacheRule) {
	headers.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(rule.BrowserTTL.Seconds())))
	headers.Set("Expires", time.Now().Add(rule.BrowserTTL).Format(time.RFC1123))
	
	for key, value := range rule.Headers {
		headers.Set(key, value)
	}
}

func (ecm *EdgeCacheManager) selectEdgeLocationsForCaching(r *http.Request, rule *CacheRule) []*EdgeLocation {
	// Select appropriate edge locations for caching based on rule and request
	var locations []*EdgeLocation
	
	ecm.mu.RLock()
	defer ecm.mu.RUnlock()
	
	for _, location := range ecm.edgeLocations {
		if location.Config.Enabled && location.Health.IsHealthy {
			locations = append(locations, location)
		}
	}
	
	return locations
}

func (ecm *EdgeCacheManager) recordCacheHit(locationID, path string, responseTime time.Duration) {
	if ecm.metrics != nil {
		ecm.metrics.requestsTotal.WithLabelValues(locationID, "hit").Inc()
		ecm.metrics.responseTime.WithLabelValues(locationID, "hit").Observe(responseTime.Seconds())
	}
}

func (ecm *EdgeCacheManager) recordCacheMiss(locationID, path string) {
	if ecm.metrics != nil {
		ecm.metrics.requestsTotal.WithLabelValues(locationID, "miss").Inc()
	}
}

// Background processes

func (ecm *EdgeCacheManager) processPurgeQueue() {
	for {
		select {
		case request := <-ecm.purgeQueue:
			ecm.processPurgeRequest(request)
		case <-ecm.ctx.Done():
			return
		}
	}
}

func (ecm *EdgeCacheManager) processWarmupQueue() {
	for {
		select {
		case request := <-ecm.warmupQueue:
			ecm.processWarmupRequest(request)
		case <-ecm.ctx.Done():
			return
		}
	}
}

func (ecm *EdgeCacheManager) processPurgeRequest(request PurgeRequest) {
	// Process purge request with configured providers
	for _, provider := range ecm.providers {
		if len(request.Providers) == 0 || ecm.containsProvider(request.Providers, provider.Name()) {
			if err := provider.Purge(request.URLs); err != nil {
				log.Printf("Purge failed for provider %s: %v", provider.Name(), err)
			}
		}
	}
	
	if request.Callback != nil {
		request.Callback(nil)
	}
}

func (ecm *EdgeCacheManager) processWarmupRequest(request WarmupRequest) {
	// Process warmup request
	for _, url := range request.URLs {
		ecm.warmupURL(url, request.Headers)
	}
	
	if request.Callback != nil {
		request.Callback(nil)
	}
}

func (ecm *EdgeCacheManager) warmupURL(url string, headers map[string]string) {
	// Create HTTP request to warm up cache
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Failed to create warmup request for %s: %v", url, err)
		return
	}

	// Add headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Send request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Warmup request failed for %s: %v", url, err)
		return
	}
	defer resp.Body.Close()

	// Read response to trigger caching
	io.Copy(io.Discard, resp.Body)
}

func (ecm *EdgeCacheManager) startHealthChecking() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ecm.performHealthChecks()
		case <-ecm.ctx.Done():
			return
		}
	}
}

func (ecm *EdgeCacheManager) performHealthChecks() {
	ecm.mu.RLock()
	locations := make([]*EdgeLocation, 0, len(ecm.edgeLocations))
	for _, location := range ecm.edgeLocations {
		locations = append(locations, location)
	}
	ecm.mu.RUnlock()

	for _, location := range locations {
		go ecm.checkLocationHealth(location)
	}
}

func (ecm *EdgeCacheManager) checkLocationHealth(location *EdgeLocation) {
	start := time.Now()
	
	resp, err := http.Get(location.Config.HealthCheck)
	responseTime := time.Since(start)
	
	location.mu.Lock()
	location.Health.LastCheck = time.Now()
	location.Health.ResponseTime = responseTime
	
	if err != nil {
		location.Health.IsHealthy = false
		location.Health.ErrorCount++
		location.Health.ErrorMessage = err.Error()
	} else {
		location.Health.StatusCode = resp.StatusCode
		location.Health.IsHealthy = resp.StatusCode == http.StatusOK
		if resp.StatusCode != http.StatusOK {
			location.Health.ErrorCount++
		}
		resp.Body.Close()
	}
	location.mu.Unlock()

	// Record health metrics
	if ecm.metrics != nil {
		healthStatus := 0.0
		if location.Health.IsHealthy {
			healthStatus = 1.0
		}
		ecm.metrics.edgeHealth.WithLabelValues(location.Config.ID).Set(healthStatus)
	}
}

func (ecm *EdgeCacheManager) startMetricsCollection() {
	if ecm.metrics == nil {
		return
	}

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ecm.collectMetrics()
		case <-ecm.ctx.Done():
			return
		}
	}
}

func (ecm *EdgeCacheManager) collectMetrics() {
	ecm.mu.RLock()
	defer ecm.mu.RUnlock()

	for _, location := range ecm.edgeLocations {
		location.mu.RLock()
		
		// Calculate hit ratio
		totalRequests := location.Stats.HitCount + location.Stats.MissCount
		var hitRatio float64
		if totalRequests > 0 {
			hitRatio = float64(location.Stats.HitCount) / float64(totalRequests)
		}
		
		// Update metrics
		ecm.metrics.cacheHitRatio.WithLabelValues(location.Config.ID).Set(hitRatio)
		ecm.metrics.cacheSize.WithLabelValues(location.Config.ID).Set(float64(location.Stats.CacheSize))
		
		// Calculate error rate
		var errorRate float64
		if location.Stats.RequestCount > 0 {
			errorRate = float64(location.Stats.ErrorCount) / float64(location.Stats.RequestCount)
		}
		ecm.metrics.errorRate.WithLabelValues(location.Config.ID).Set(errorRate)
		
		location.mu.RUnlock()
	}
}

// Initialization methods

func (ecm *EdgeCacheManager) initializeProviders() error {
	for name, config := range ecm.config.Providers {
		if !config.Enabled {
			continue
		}

		var provider CDNProvider
		var err error

		switch config.Type {
		case CloudFlare:
			provider, err = NewCloudFlareProvider(config)
		case AWS:
			provider, err = NewAWSProvider(config)
		case Fastly:
			provider, err = NewFastlyProvider(config)
		default:
			err = fmt.Errorf("unsupported provider type: %s", config.Type)
		}

		if err != nil {
			return fmt.Errorf("failed to initialize provider %s: %w", name, err)
		}

		ecm.providers[name] = provider
	}

	return nil
}

func (ecm *EdgeCacheManager) initializeEdgeLocations() error {
	for _, config := range ecm.config.EdgeLocations {
		location := &EdgeLocation{
			Config:      config,
			Cache:       make(map[string]*CacheEntry),
			LastUpdated: time.Now(),
		}

		ecm.edgeLocations[config.ID] = location
	}

	return nil
}

// Utility functions

func (ecm *EdgeCacheManager) containsProvider(providers []string, name string) bool {
	for _, provider := range providers {
		if provider == name {
			return true
		}
	}
	return false
}

func getDefaultCacheRules() []CacheRule {
	return []CacheRule{
		{
			PathPattern: "/static/",
			TTL:         24 * time.Hour,
			EdgeTTL:     24 * time.Hour,
			BrowserTTL:  1 * time.Hour,
			Compression: true,
			Enabled:     true,
		},
		{
			PathPattern: "/api/",
			TTL:         5 * time.Minute,
			EdgeTTL:     5 * time.Minute,
			BrowserTTL:  1 * time.Minute,
			Compression: false,
			Enabled:     true,
		},
		{
			ContentType: "image/",
			TTL:         7 * 24 * time.Hour,
			EdgeTTL:     7 * 24 * time.Hour,
			BrowserTTL:  24 * time.Hour,
			Compression: false,
			Enabled:     true,
		},
	}
}

func NewCDNMetrics() *CDNMetrics {
	return &CDNMetrics{
		requestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cdn_requests_total",
				Help: "Total number of CDN requests",
			},
			[]string{"location", "status"},
		),
		cacheHitRatio: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cdn_cache_hit_ratio",
				Help: "Cache hit ratio by location",
			},
			[]string{"location"},
		),
		responseTime: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "cdn_response_time_seconds",
				Help:    "CDN response time in seconds",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 15),
			},
			[]string{"location", "status"},
		),
		bandwidthUsage: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cdn_bandwidth_bytes_total",
				Help: "Total bandwidth usage in bytes",
			},
			[]string{"location"},
		),
		purgeOperations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cdn_purge_operations_total",
				Help: "Total number of purge operations",
			},
			[]string{"provider", "status"},
		),
		warmupOperations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cdn_warmup_operations_total",
				Help: "Total number of warmup operations",
			},
			[]string{"status"},
		),
		edgeHealth: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cdn_edge_health",
				Help: "Health status of edge locations (1=healthy, 0=unhealthy)",
			},
			[]string{"location"},
		),
		cacheSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cdn_cache_size_bytes",
				Help: "Cache size in bytes by location",
			},
			[]string{"location"},
		),
		errorRate: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cdn_error_rate",
				Help: "Error rate by location",
			},
			[]string{"location"},
		),
	}
}

// Placeholder implementations for provider constructors
func NewCloudFlareProvider(config ProviderConfig) (CDNProvider, error) {
	return nil, fmt.Errorf("CloudFlare provider not implemented")
}

func NewAWSProvider(config ProviderConfig) (CDNProvider, error) {
	return nil, fmt.Errorf("AWS provider not implemented")
}

func NewFastlyProvider(config ProviderConfig) (CDNProvider, error) {
	return nil, fmt.Errorf("Fastly provider not implemented")
}

// Placeholder types for supporting components
type HealthChecker struct{}
type LoadBalancer struct{}
type HealthCheckConfig struct{}
type LoadBalancerConfig struct{}
type SecurityConfig struct{}
type CompressionConfig struct{}
type ProviderStats struct{}

func NewHealthChecker(config HealthCheckConfig) *HealthChecker {
	return &HealthChecker{}
}

func NewLoadBalancer(config LoadBalancerConfig) *LoadBalancer {
	return &LoadBalancer{}
}

// Close gracefully shuts down the edge cache manager
func (ecm *EdgeCacheManager) Close() error {
	log.Println("Shutting down edge cache manager...")
	ecm.cancel()
	close(ecm.purgeQueue)
	close(ecm.warmupQueue)
	log.Println("Edge cache manager shutdown complete")
	return nil
}