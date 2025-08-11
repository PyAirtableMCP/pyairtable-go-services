package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

// AnalyticsEngine provides comprehensive API analytics and metrics
type AnalyticsEngine struct {
	redis         *redis.Client
	keyPrefix     string
	metrics       *MetricsCollector
	config        *AnalyticsConfig
	eventBuffer   chan *AnalyticsEvent
	bufferSize    int
	flushInterval time.Duration
	mu            sync.RWMutex
}

// AnalyticsConfig holds analytics configuration
type AnalyticsConfig struct {
	Enabled              bool          `json:"enabled"`
	BufferSize           int           `json:"buffer_size"`
	FlushInterval        time.Duration `json:"flush_interval"`
	RetentionPeriod      time.Duration `json:"retention_period"`
	EnableUserTracking   bool          `json:"enable_user_tracking"`
	EnablePathAnalytics  bool          `json:"enable_path_analytics"`
	EnableErrorTracking  bool          `json:"enable_error_tracking"`
	EnablePerformanceMetrics bool      `json:"enable_performance_metrics"`
	SamplingRate         float64       `json:"sampling_rate"`
	MaxEventsPerFlush    int           `json:"max_events_per_flush"`
}

// AnalyticsEvent represents an analytics event
type AnalyticsEvent struct {
	ID            string                 `json:"id"`
	Timestamp     time.Time              `json:"timestamp"`
	Type          EventType              `json:"type"`
	RequestID     string                 `json:"request_id"`
	UserID        string                 `json:"user_id,omitempty"`
	TenantID      string                 `json:"tenant_id,omitempty"`
	APIKey        string                 `json:"api_key,omitempty"`
	Method        string                 `json:"method"`
	Path          string                 `json:"path"`
	StatusCode    int                    `json:"status_code"`
	ResponseTime  float64                `json:"response_time_ms"`
	RequestSize   int                    `json:"request_size_bytes"`
	ResponseSize  int                    `json:"response_size_bytes"`
	UserAgent     string                 `json:"user_agent"`
	RemoteIP      string                 `json:"remote_ip"`
	Country       string                 `json:"country,omitempty"`
	Route         string                 `json:"route,omitempty"`
	Upstream      string                 `json:"upstream,omitempty"`
	ErrorType     string                 `json:"error_type,omitempty"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	CustomFields  map[string]interface{} `json:"custom_fields,omitempty"`
}

// EventType defines different types of analytics events
type EventType string

const (
	EventTypeRequest     EventType = "request"
	EventTypeError       EventType = "error"
	EventTypeAuth        EventType = "auth"
	EventTypeRateLimit   EventType = "rate_limit"
	EventTypeSecurity    EventType = "security"
	EventTypePerformance EventType = "performance"
)

// MetricsCollector handles Prometheus metrics
type MetricsCollector struct {
	// Request metrics
	requestsTotal       *prometheus.CounterVec
	requestDuration     *prometheus.HistogramVec
	requestSize         *prometheus.HistogramVec
	responseSize        *prometheus.HistogramVec
	
	// Error metrics
	errorsTotal         *prometheus.CounterVec
	
	// Rate limiting metrics
	rateLimitHits       *prometheus.CounterVec
	rateLimitBlocked    *prometheus.CounterVec
	
	// Security metrics
	securityThreats     *prometheus.CounterVec
	ipBlocked           *prometheus.CounterVec
	
	// Performance metrics
	upstreamLatency     *prometheus.HistogramVec
	connectionPool      *prometheus.GaugeVec
	
	// Business metrics
	activeUsers         *prometheus.GaugeVec
	apiKeyUsage         *prometheus.CounterVec
	tenantRequests      *prometheus.CounterVec
}

// UsageStats represents usage statistics
type UsageStats struct {
	TotalRequests    int64                  `json:"total_requests"`
	TotalErrors      int64                  `json:"total_errors"`
	AvgResponseTime  float64                `json:"avg_response_time_ms"`
	RequestsByStatus map[int]int64          `json:"requests_by_status"`
	RequestsByMethod map[string]int64       `json:"requests_by_method"`
	RequestsByPath   map[string]int64       `json:"requests_by_path"`
	TopUsers         []UserStats            `json:"top_users"`
	TopAPIKeys       []APIKeyStats          `json:"top_api_keys"`
	ErrorsByType     map[string]int64       `json:"errors_by_type"`
	GeographicData   map[string]int64       `json:"geographic_data"`
	TimeSeriesData   []TimeSeriesPoint      `json:"time_series_data"`
}

// UserStats represents user-specific statistics
type UserStats struct {
	UserID       string  `json:"user_id"`
	TenantID     string  `json:"tenant_id"`
	RequestCount int64   `json:"request_count"`
	ErrorCount   int64   `json:"error_count"`
	AvgLatency   float64 `json:"avg_latency_ms"`
	LastSeen     time.Time `json:"last_seen"`
}

// APIKeyStats represents API key usage statistics
type APIKeyStats struct {
	APIKey       string    `json:"api_key"` // Masked
	RequestCount int64     `json:"request_count"`
	ErrorCount   int64     `json:"error_count"`
	LastUsed     time.Time `json:"last_used"`
	QuotaUsed    int64     `json:"quota_used"`
	QuotaLimit   int64     `json:"quota_limit"`
}

// TimeSeriesPoint represents a point in time series data
type TimeSeriesPoint struct {
	Timestamp    time.Time `json:"timestamp"`
	RequestCount int64     `json:"request_count"`
	ErrorCount   int64     `json:"error_count"`
	AvgLatency   float64   `json:"avg_latency_ms"`
}

// NewAnalyticsEngine creates a new analytics engine
func NewAnalyticsEngine(redis *redis.Client, config *AnalyticsConfig, keyPrefix string) *AnalyticsEngine {
	metrics := NewMetricsCollector()
	
	engine := &AnalyticsEngine{
		redis:         redis,
		keyPrefix:     keyPrefix,
		metrics:       metrics,
		config:        config,
		bufferSize:    config.BufferSize,
		flushInterval: config.FlushInterval,
		eventBuffer:   make(chan *AnalyticsEvent, config.BufferSize),
	}
	
	// Start background processing
	go engine.processEvents()
	
	return engine
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "api_gateway_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "path", "status_code", "tenant_id"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "api_gateway_request_duration_seconds",
				Help:    "HTTP request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "path", "tenant_id"},
		),
		requestSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "api_gateway_request_size_bytes",
				Help:    "HTTP request size in bytes",
				Buckets: []float64{100, 1000, 10000, 100000, 1000000},
			},
			[]string{"method", "path"},
		),
		responseSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "api_gateway_response_size_bytes",
				Help:    "HTTP response size in bytes",
				Buckets: []float64{100, 1000, 10000, 100000, 1000000},
			},
			[]string{"method", "path"},
		),
		errorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "api_gateway_errors_total",
				Help: "Total number of errors",
			},
			[]string{"error_type", "path", "tenant_id"},
		),
		rateLimitHits: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "api_gateway_rate_limit_hits_total",
				Help: "Total number of rate limit hits",
			},
			[]string{"limit_type", "tenant_id", "user_id"},
		),
		rateLimitBlocked: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "api_gateway_rate_limit_blocked_total",
				Help: "Total number of rate limit blocks",
			},
			[]string{"limit_type", "tenant_id", "user_id"},
		),
		securityThreats: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "api_gateway_security_threats_total",
				Help: "Total number of security threats detected",
			},
			[]string{"threat_type", "severity", "action"},
		),
		ipBlocked: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "api_gateway_ip_blocked_total",
				Help: "Total number of blocked IP addresses",
			},
			[]string{"reason", "country"},
		),
		upstreamLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "api_gateway_upstream_latency_seconds",
				Help:    "Upstream service latency in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"upstream", "method"},
		),
		connectionPool: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "api_gateway_connection_pool",
				Help: "Connection pool statistics",
			},
			[]string{"pool_name", "state"},
		),
		activeUsers: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "api_gateway_active_users",
				Help: "Number of active users",
			},
			[]string{"tenant_id", "time_window"},
		),
		apiKeyUsage: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "api_gateway_api_key_usage_total",
				Help: "Total API key usage",
			},
			[]string{"api_key_hash", "tenant_id"},
		),
		tenantRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "api_gateway_tenant_requests_total",
				Help: "Total requests per tenant",
			},
			[]string{"tenant_id", "method", "path"},
		),
	}
}

// RegisterMetrics registers all metrics with Prometheus
func (mc *MetricsCollector) RegisterMetrics(registry *prometheus.Registry) {
	registry.MustRegister(
		mc.requestsTotal,
		mc.requestDuration,
		mc.requestSize,
		mc.responseSize,
		mc.errorsTotal,
		mc.rateLimitHits,
		mc.rateLimitBlocked,
		mc.securityThreats,
		mc.ipBlocked,
		mc.upstreamLatency,
		mc.connectionPool,
		mc.activeUsers,
		mc.apiKeyUsage,
		mc.tenantRequests,
	)
}

// Middleware creates a Fiber middleware for analytics
func (ae *AnalyticsEngine) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !ae.config.Enabled {
			return c.Next()
		}
		
		start := time.Now()
		
		// Process request
		err := c.Next()
		
		// Create analytics event
		event := ae.createAnalyticsEvent(c, start, err)
		
		// Sample events based on sampling rate
		if ae.shouldSample() {
			// Send to buffer (non-blocking)
			select {
			case ae.eventBuffer <- event:
			default:
				// Buffer full, skip this event
			}
		}
		
		// Update Prometheus metrics
		ae.updateMetrics(event)
		
		return err
	}
}

// TrackEvent tracks a custom analytics event
func (ae *AnalyticsEngine) TrackEvent(event *AnalyticsEvent) {
	if !ae.config.Enabled {
		return
	}
	
	select {
	case ae.eventBuffer <- event:
	default:
		// Buffer full, skip this event
	}
}

// GetUsageStats retrieves usage statistics for a time period
func (ae *AnalyticsEngine) GetUsageStats(startTime, endTime time.Time, tenantID string) (*UsageStats, error) {
	stats := &UsageStats{
		RequestsByStatus: make(map[int]int64),
		RequestsByMethod: make(map[string]int64),
		RequestsByPath:   make(map[string]int64),
		ErrorsByType:     make(map[string]int64),
		GeographicData:   make(map[string]int64),
		TimeSeriesData:   []TimeSeriesPoint{},
	}
	
	// Query Redis for analytics data
	pattern := fmt.Sprintf("%s:events:*", ae.keyPrefix)
	keys, err := ae.redis.Keys(context.Background(), pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get analytics keys: %w", err)
	}
	
	var totalLatency float64
	var latencyCount int64
	
	for _, key := range keys {
		data, err := ae.redis.Get(context.Background(), key).Result()
		if err != nil {
			continue
		}
		
		var event AnalyticsEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		
		// Filter by time range and tenant
		if event.Timestamp.Before(startTime) || event.Timestamp.After(endTime) {
			continue
		}
		if tenantID != "" && event.TenantID != tenantID {
			continue
		}
		
		// Update statistics
		stats.TotalRequests++
		stats.RequestsByStatus[event.StatusCode]++
		stats.RequestsByMethod[event.Method]++
		stats.RequestsByPath[event.Path]++
		
		if event.Country != "" {
			stats.GeographicData[event.Country]++
		}
		
		if event.ResponseTime > 0 {
			totalLatency += event.ResponseTime
			latencyCount++
		}
		
		if event.Type == EventTypeError {
			stats.TotalErrors++
			stats.ErrorsByType[event.ErrorType]++
		}
	}
	
	// Calculate average response time
	if latencyCount > 0 {
		stats.AvgResponseTime = totalLatency / float64(latencyCount)
	}
	
	// Get top users and API keys
	stats.TopUsers = ae.getTopUsers(startTime, endTime, tenantID)
	stats.TopAPIKeys = ae.getTopAPIKeys(startTime, endTime, tenantID)
	
	// Generate time series data
	stats.TimeSeriesData = ae.getTimeSeriesData(startTime, endTime, tenantID)
	
	return stats, nil
}

// createAnalyticsEvent creates an analytics event from a request
func (ae *AnalyticsEngine) createAnalyticsEvent(c *fiber.Ctx, startTime time.Time, err error) *AnalyticsEvent {
	event := &AnalyticsEvent{
		ID:           fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().UnixNano()%1000),
		Timestamp:    time.Now(),
		Type:         EventTypeRequest,
		RequestID:    c.Get("X-Request-ID"),
		Method:       c.Method(),
		Path:         c.Path(),
		StatusCode:   c.Response().StatusCode(),
		ResponseTime: float64(time.Since(startTime).Nanoseconds()) / 1e6, // Convert to milliseconds
		RequestSize:  len(c.Body()),
		ResponseSize: len(c.Response().Body()),
		UserAgent:    c.Get("User-Agent"),
		RemoteIP:     c.IP(),
		CustomFields: make(map[string]interface{}),
	}
	
	// Add route information
	if route := c.Route(); route != nil {
		event.Route = route.Path
	}
	
	// Add user information
	if userClaims := c.Locals("user_claims"); userClaims != nil {
		if claims, ok := userClaims.(map[string]interface{}); ok {
			if userID, ok := claims["user_id"].(string); ok {
				event.UserID = userID
			}
			if tenantID, ok := claims["tenant_id"].(string); ok {
				event.TenantID = tenantID
			}
		}
	}
	
	// Add API key information (masked)
	if apiKey := c.Get("X-API-Key"); apiKey != "" {
		event.APIKey = maskAPIKey(apiKey)
	}
	
	// Add upstream information
	if upstream := c.Locals("upstream"); upstream != nil {
		event.Upstream = fmt.Sprintf("%v", upstream)
	}
	
	// Add error information
	if err != nil {
		event.Type = EventTypeError
		event.ErrorMessage = err.Error()
		event.ErrorType = getErrorType(err)
	}
	
	return event
}

// processEvents processes analytics events in the background
func (ae *AnalyticsEngine) processEvents() {
	ticker := time.NewTicker(ae.flushInterval)
	defer ticker.Stop()
	
	events := make([]*AnalyticsEvent, 0, ae.config.MaxEventsPerFlush)
	
	for {
		select {
		case event := <-ae.eventBuffer:
			events = append(events, event)
			
			// Flush if buffer is full
			if len(events) >= ae.config.MaxEventsPerFlush {
				ae.flushEvents(events)
				events = events[:0] // Reset slice
			}
			
		case <-ticker.C:
			// Flush events periodically
			if len(events) > 0 {
				ae.flushEvents(events)
				events = events[:0] // Reset slice
			}
		}
	}
}

// flushEvents flushes events to Redis
func (ae *AnalyticsEngine) flushEvents(events []*AnalyticsEvent) {
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		
		// Store individual event
		key := fmt.Sprintf("%s:events:%s", ae.keyPrefix, event.ID)
		ae.redis.Set(context.Background(), key, data, ae.config.RetentionPeriod)
		
		// Add to time-based index
		timeKey := fmt.Sprintf("%s:events:by_time:%s", ae.keyPrefix, event.Timestamp.Format("2006-01-02-15"))
		ae.redis.RPush(context.Background(), timeKey, event.ID)
		ae.redis.Expire(context.Background(), timeKey, ae.config.RetentionPeriod)
		
		// Add to tenant-based index if tenant ID exists
		if event.TenantID != "" {
			tenantKey := fmt.Sprintf("%s:events:by_tenant:%s", ae.keyPrefix, event.TenantID)
			ae.redis.RPush(context.Background(), tenantKey, event.ID)
			ae.redis.Expire(context.Background(), tenantKey, ae.config.RetentionPeriod)
		}
	}
}

// updateMetrics updates Prometheus metrics
func (ae *AnalyticsEngine) updateMetrics(event *AnalyticsEvent) {
	// Update request metrics
	ae.metrics.requestsTotal.WithLabelValues(
		event.Method,
		event.Path,
		fmt.Sprintf("%d", event.StatusCode),
		event.TenantID,
	).Inc()
	
	ae.metrics.requestDuration.WithLabelValues(
		event.Method,
		event.Path,
		event.TenantID,
	).Observe(event.ResponseTime / 1000) // Convert to seconds
	
	ae.metrics.requestSize.WithLabelValues(
		event.Method,
		event.Path,
	).Observe(float64(event.RequestSize))
	
	ae.metrics.responseSize.WithLabelValues(
		event.Method,
		event.Path,
	).Observe(float64(event.ResponseSize))
	
	// Update error metrics
	if event.Type == EventTypeError {
		ae.metrics.errorsTotal.WithLabelValues(
			event.ErrorType,
			event.Path,
			event.TenantID,
		).Inc()
	}
	
	// Update tenant-specific metrics
	if event.TenantID != "" {
		ae.metrics.tenantRequests.WithLabelValues(
			event.TenantID,
			event.Method,
			event.Path,
		).Inc()
	}
	
	// Update API key metrics
	if event.APIKey != "" {
		ae.metrics.apiKeyUsage.WithLabelValues(
			event.APIKey,
			event.TenantID,
		).Inc()
	}
}

// Helper methods
func (ae *AnalyticsEngine) shouldSample() bool {
	return float64(time.Now().UnixNano()%1000)/1000.0 < ae.config.SamplingRate
}

func (ae *AnalyticsEngine) getTopUsers(startTime, endTime time.Time, tenantID string) []UserStats {
	// Implementation would query Redis for top users
	return []UserStats{}
}

func (ae *AnalyticsEngine) getTopAPIKeys(startTime, endTime time.Time, tenantID string) []APIKeyStats {
	// Implementation would query Redis for top API keys
	return []APIKeyStats{}
}

func (ae *AnalyticsEngine) getTimeSeriesData(startTime, endTime time.Time, tenantID string) []TimeSeriesPoint {
	// Implementation would generate time series data
	return []TimeSeriesPoint{}
}

func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "***"
	}
	return apiKey[:4] + "***" + apiKey[len(apiKey)-4:]
}

func getErrorType(err error) string {
	// Simple error type classification
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "timeout"):
		return "timeout"
	case strings.Contains(errStr, "connection"):
		return "connection"
	case strings.Contains(errStr, "unauthorized"):
		return "auth"
	case strings.Contains(errStr, "rate limit"):
		return "rate_limit"
	default:
		return "unknown"
	}
}