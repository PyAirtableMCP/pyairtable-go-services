package versioning

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"
)

// VersionAnalyticsService provides comprehensive analytics for API versions
type VersionAnalyticsService struct {
	storage       AnalyticsStorage
	aggregator    *MetricsAggregator
	alerting      *AlertingService
	dashboard     *AnalyticsDashboard
	config        *AnalyticsConfig
	mu            sync.RWMutex
}

// AnalyticsStorage interface for storing analytics data
type AnalyticsStorage interface {
	StoreEvent(ctx context.Context, event *AnalyticsEvent) error
	GetMetrics(ctx context.Context, query *MetricsQuery) (*MetricsResult, error)
	GetTimeSeries(ctx context.Context, query *TimeSeriesQuery) (*TimeSeriesResult, error)
	GetTopN(ctx context.Context, query *TopNQuery) (*TopNResult, error)
	Cleanup(ctx context.Context, before time.Time) error
}

// AnalyticsConfig configures analytics behavior
type AnalyticsConfig struct {
	Enabled             bool          `json:"enabled"`
	SamplingRate        float64       `json:"sampling_rate"`        // 0.0 to 1.0
	RetentionPeriod     time.Duration `json:"retention_period"`
	AggregationInterval time.Duration `json:"aggregation_interval"`
	BatchSize           int           `json:"batch_size"`
	FlushInterval       time.Duration `json:"flush_interval"`
	AlertThresholds     *AlertThresholds `json:"alert_thresholds"`
}

// AlertThresholds defines thresholds for alerts
type AlertThresholds struct {
	ErrorRateThreshold      float64       `json:"error_rate_threshold"`       // e.g., 0.05 for 5%
	ResponseTimeP99         time.Duration `json:"response_time_p99"`
	LowUsageThreshold       int64         `json:"low_usage_threshold"`        // requests per day
	HighUsageGrowth         float64       `json:"high_usage_growth"`          // e.g., 2.0 for 200% growth
	DeprecatedUsageWarning  int64         `json:"deprecated_usage_warning"`   // requests per day for deprecated versions
	ClientDistributionSkew  float64       `json:"client_distribution_skew"`   // percentage of traffic from single client
}

// AnalyticsEvent represents an analytics event
type AnalyticsEvent struct {
	ID          string                 `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	Type        EventType              `json:"type"`
	Version     string                 `json:"version"`
	ClientID    string                 `json:"client_id"`
	UserID      string                 `json:"user_id"`
	IP          string                 `json:"ip"`
	UserAgent   string                 `json:"user_agent"`
	Method      string                 `json:"method"`
	Path        string                 `json:"path"`
	StatusCode  int                    `json:"status_code"`
	ResponseTime time.Duration         `json:"response_time"`
	RequestSize int64                  `json:"request_size"`
	ResponseSize int64                 `json:"response_size"`
	Error       string                 `json:"error,omitempty"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// EventType defines types of analytics events
type EventType string

const (
	EventAPIRequest    EventType = "api_request"
	EventVersionSwitch EventType = "version_switch"
	EventError         EventType = "error"
	EventDeprecation   EventType = "deprecation_warning"
	EventSunset        EventType = "sunset_attempt"
	EventClientOnboard EventType = "client_onboard"
	EventMigration     EventType = "migration"
)

// MetricsQuery defines a query for metrics
type MetricsQuery struct {
	Versions   []string           `json:"versions"`
	StartTime  time.Time          `json:"start_time"`
	EndTime    time.Time          `json:"end_time"`
	GroupBy    []string           `json:"group_by"`    // version, client, user, path, etc.
	Filters    map[string]interface{} `json:"filters"`
	Metrics    []MetricType       `json:"metrics"`
}

// MetricType defines types of metrics to calculate
type MetricType string

const (
	MetricRequestCount    MetricType = "request_count"
	MetricUniqueUsers     MetricType = "unique_users"
	MetricUniqueClients   MetricType = "unique_clients"
	MetricErrorRate       MetricType = "error_rate"
	MetricAvgResponseTime MetricType = "avg_response_time"
	MetricP95ResponseTime MetricType = "p95_response_time"
	MetricP99ResponseTime MetricType = "p99_response_time"
	MetricThroughput      MetricType = "throughput"
	MetricDataTransfer    MetricType = "data_transfer"
)

// MetricsResult contains the result of a metrics query
type MetricsResult struct {
	Query   *MetricsQuery              `json:"query"`
	Data    map[string]*MetricValue    `json:"data"`     // dimension -> metric values
	Summary *MetricsSummary            `json:"summary"`
	GeneratedAt time.Time              `json:"generated_at"`
}

// MetricValue contains values for different metrics
type MetricValue struct {
	RequestCount    int64         `json:"request_count"`
	UniqueUsers     int64         `json:"unique_users"`
	UniqueClients   int64         `json:"unique_clients"`
	ErrorRate       float64       `json:"error_rate"`
	AvgResponseTime time.Duration `json:"avg_response_time"`
	P95ResponseTime time.Duration `json:"p95_response_time"`
	P99ResponseTime time.Duration `json:"p99_response_time"`
	Throughput      float64       `json:"throughput"`      // requests per second
	DataTransfer    int64         `json:"data_transfer"`   // bytes
}

// MetricsSummary provides overall summary
type MetricsSummary struct {
	TotalRequests   int64         `json:"total_requests"`
	TotalUsers      int64         `json:"total_users"`
	TotalClients    int64         `json:"total_clients"`
	OverallErrorRate float64      `json:"overall_error_rate"`
	AvgResponseTime time.Duration `json:"avg_response_time"`
	PeakThroughput  float64       `json:"peak_throughput"`
	TopVersions     []VersionUsage `json:"top_versions"`
	TopClients      []ClientUsage  `json:"top_clients"`
}

// VersionUsage tracks usage per version
type VersionUsage struct {
	Version      string  `json:"version"`
	RequestCount int64   `json:"request_count"`
	Percentage   float64 `json:"percentage"`
}

// ClientUsage tracks usage per client
type ClientUsage struct {
	ClientID     string  `json:"client_id"`
	RequestCount int64   `json:"request_count"`
	Percentage   float64 `json:"percentage"`
}

// TimeSeriesQuery defines a time series query
type TimeSeriesQuery struct {
	Versions   []string           `json:"versions"`
	StartTime  time.Time          `json:"start_time"`
	EndTime    time.Time          `json:"end_time"`
	Interval   time.Duration      `json:"interval"`     // aggregation interval
	Metric     MetricType         `json:"metric"`
	GroupBy    string             `json:"group_by"`     // dimension to group by
	Filters    map[string]interface{} `json:"filters"`
}

// TimeSeriesResult contains time series data
type TimeSeriesResult struct {
	Query     *TimeSeriesQuery    `json:"query"`
	Series    map[string]*TimeSeries `json:"series"`    // dimension -> time series
	GeneratedAt time.Time         `json:"generated_at"`
}

// TimeSeries represents a time series
type TimeSeries struct {
	Name   string            `json:"name"`
	Points []TimeSeriesPoint `json:"points"`
}

// TimeSeriesPoint represents a point in time series
type TimeSeriesPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// TopNQuery defines a top N query
type TopNQuery struct {
	Versions   []string           `json:"versions"`
	StartTime  time.Time          `json:"start_time"`
	EndTime    time.Time          `json:"end_time"`
	Dimension  string             `json:"dimension"`    // client, user, path, etc.
	Metric     MetricType         `json:"metric"`
	Limit      int                `json:"limit"`
	Filters    map[string]interface{} `json:"filters"`
	OrderBy    string             `json:"order_by"`     // asc, desc
}

// TopNResult contains top N results
type TopNResult struct {
	Query   *TopNQuery      `json:"query"`
	Results []TopNItem      `json:"results"`
	GeneratedAt time.Time   `json:"generated_at"`
}

// TopNItem represents a top N item
type TopNItem struct {
	Dimension string  `json:"dimension"`
	Value     string  `json:"value"`
	Count     int64   `json:"count"`
	Metric    float64 `json:"metric"`
}

// MetricsAggregator aggregates metrics in real-time
type MetricsAggregator struct {
	windows    map[string]*AggregationWindow
	config     *AggregationConfig
	mu         sync.RWMutex
}

// AggregationConfig configures metrics aggregation
type AggregationConfig struct {
	WindowSize     time.Duration `json:"window_size"`
	RetentionCount int           `json:"retention_count"`  // Number of windows to retain
	Dimensions     []string      `json:"dimensions"`       // Dimensions to aggregate by
	Metrics        []MetricType  `json:"metrics"`
}

// AggregationWindow represents a time window for aggregation
type AggregationWindow struct {
	StartTime   time.Time                        `json:"start_time"`
	EndTime     time.Time                        `json:"end_time"`
	Aggregates  map[string]*WindowAggregate      `json:"aggregates"`  // dimension -> aggregate
	EventCount  int64                            `json:"event_count"`
	Completed   bool                             `json:"completed"`
}

// WindowAggregate contains aggregated metrics for a dimension
type WindowAggregate struct {
	Dimension       string        `json:"dimension"`
	RequestCount    int64         `json:"request_count"`
	ErrorCount      int64         `json:"error_count"`
	UniqueUsers     map[string]bool `json:"-"`           // Set of unique users
	UniqueClients   map[string]bool `json:"-"`           // Set of unique clients
	ResponseTimes   []time.Duration `json:"-"`           // Response times for percentile calculation
	TotalDataTransfer int64        `json:"total_data_transfer"`
}

// AlertingService manages alerts based on metrics
type AlertingService struct {
	rules     map[string]*AlertRule
	channels  map[string]AlertChannel
	history   []Alert
	config    *AlertingConfig
	mu        sync.RWMutex
}

// AlertingConfig configures alerting
type AlertingConfig struct {
	Enabled           bool          `json:"enabled"`
	EvaluationInterval time.Duration `json:"evaluation_interval"`
	HistoryRetention  time.Duration `json:"history_retention"`
	DefaultChannels   []string      `json:"default_channels"`
}

// AlertRule defines an alerting rule
type AlertRule struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Query       *MetricsQuery          `json:"query"`
	Condition   *AlertCondition        `json:"condition"`
	Channels    []string               `json:"channels"`
	Frequency   time.Duration          `json:"frequency"`
	Enabled     bool                   `json:"enabled"`
	Labels      map[string]string      `json:"labels"`
	LastEval    time.Time              `json:"last_eval"`
	LastAlert   *time.Time             `json:"last_alert,omitempty"`
}

// AlertCondition defines when to trigger an alert
type AlertCondition struct {
	Metric    MetricType      `json:"metric"`
	Operator  string          `json:"operator"`     // gt, lt, eq, ne, gte, lte
	Threshold float64         `json:"threshold"`
	Duration  time.Duration   `json:"duration"`     // How long condition must be true
	Severity  AlertSeverity   `json:"severity"`
}

// Alert represents a triggered alert
type Alert struct {
	ID          string                 `json:"id"`
	RuleID      string                 `json:"rule_id"`
	RuleName    string                 `json:"rule_name"`
	Severity    AlertSeverity          `json:"severity"`
	Message     string                 `json:"message"`
	Details     map[string]interface{} `json:"details"`
	TriggeredAt time.Time              `json:"triggered_at"`
	ResolvedAt  *time.Time             `json:"resolved_at,omitempty"`
	Status      AlertStatus            `json:"status"`
	Labels      map[string]string      `json:"labels"`
}

// AlertStatus represents alert status
type AlertStatus string

const (
	AlertStatusFiring   AlertStatus = "firing"
	AlertStatusResolved AlertStatus = "resolved"
	AlertStatusSilenced AlertStatus = "silenced"
)

// AlertChannel interface for sending alerts
type AlertChannel interface {
	Send(ctx context.Context, alert *Alert) error
	GetType() string
}

// AnalyticsDashboard provides web dashboard for analytics
type AnalyticsDashboard struct {
	widgets    map[string]*DashboardWidget
	layouts    map[string]*DashboardLayout
	templates  map[string]*DashboardTemplate
	mu         sync.RWMutex
}

// DashboardWidget represents a dashboard widget
type DashboardWidget struct {
	ID          string                 `json:"id"`
	Type        WidgetType             `json:"type"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Query       interface{}            `json:"query"`       // MetricsQuery, TimeSeriesQuery, etc.
	Config      map[string]interface{} `json:"config"`
	Position    *WidgetPosition        `json:"position"`
	Refresh     time.Duration          `json:"refresh"`
}

// WidgetType defines types of dashboard widgets
type WidgetType string

const (
	WidgetTypeMetric     WidgetType = "metric"
	WidgetTypeChart      WidgetType = "chart"
	WidgetTypeTable      WidgetType = "table"
	WidgetTypeHeatmap    WidgetType = "heatmap"
	WidgetTypeTopN       WidgetType = "topn"
	WidgetTypeAlert      WidgetType = "alert"
)

// WidgetPosition defines widget position on dashboard
type WidgetPosition struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// DashboardLayout defines a dashboard layout
type DashboardLayout struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Widgets     []string  `json:"widgets"`    // Widget IDs
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DashboardTemplate provides predefined dashboard templates
type DashboardTemplate struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	Widgets     []*DashboardWidget     `json:"widgets"`
	Layout      *DashboardLayout       `json:"layout"`
	Variables   map[string]interface{} `json:"variables"`
}

// NewVersionAnalyticsService creates a new version analytics service
func NewVersionAnalyticsService(storage AnalyticsStorage, config *AnalyticsConfig) *VersionAnalyticsService {
	if config == nil {
		config = &AnalyticsConfig{
			Enabled:             true,
			SamplingRate:        1.0,
			RetentionPeriod:     30 * 24 * time.Hour,
			AggregationInterval: 5 * time.Minute,
			BatchSize:           1000,
			FlushInterval:       10 * time.Second,
			AlertThresholds: &AlertThresholds{
				ErrorRateThreshold:      0.05,
				ResponseTimeP99:         2 * time.Second,
				LowUsageThreshold:       100,
				HighUsageGrowth:         2.0,
				DeprecatedUsageWarning:  1000,
				ClientDistributionSkew:  0.8,
			},
		}
	}
	
	return &VersionAnalyticsService{
		storage:    storage,
		aggregator: NewMetricsAggregator(&AggregationConfig{
			WindowSize:     config.AggregationInterval,
			RetentionCount: 288, // 24 hours worth of 5-minute windows
			Dimensions:     []string{"version", "client", "path", "status"},
			Metrics:        []MetricType{MetricRequestCount, MetricErrorRate, MetricAvgResponseTime},
		}),
		alerting: NewAlertingService(&AlertingConfig{
			Enabled:            true,
			EvaluationInterval: 1 * time.Minute,
			HistoryRetention:   7 * 24 * time.Hour,
		}),
		dashboard: NewAnalyticsDashboard(),
		config:    config,
	}
}

// NewMetricsAggregator creates a new metrics aggregator
func NewMetricsAggregator(config *AggregationConfig) *MetricsAggregator {
	return &MetricsAggregator{
		windows: make(map[string]*AggregationWindow),
		config:  config,
	}
}

// NewAlertingService creates a new alerting service
func NewAlertingService(config *AlertingConfig) *AlertingService {
	return &AlertingService{
		rules:    make(map[string]*AlertRule),
		channels: make(map[string]AlertChannel),
		history:  make([]Alert, 0),
		config:   config,
	}
}

// NewAnalyticsDashboard creates a new analytics dashboard
func NewAnalyticsDashboard() *AnalyticsDashboard {
	dashboard := &AnalyticsDashboard{
		widgets:   make(map[string]*DashboardWidget),
		layouts:   make(map[string]*DashboardLayout),
		templates: make(map[string]*DashboardTemplate),
	}
	
	// Create default templates
	dashboard.createDefaultTemplates()
	
	return dashboard
}

// TrackEvent tracks an analytics event
func (vas *VersionAnalyticsService) TrackEvent(ctx context.Context, event *AnalyticsEvent) error {
	if !vas.config.Enabled {
		return nil
	}
	
	// Apply sampling
	if vas.config.SamplingRate < 1.0 {
		// Simple sampling implementation
		if time.Now().UnixNano()%100 >= int64(vas.config.SamplingRate*100) {
			return nil
		}
	}
	
	// Store event
	if err := vas.storage.StoreEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to store event: %v", err)
	}
	
	// Update real-time aggregation
	vas.aggregator.ProcessEvent(event)
	
	return nil
}

// GetMetrics retrieves metrics based on query
func (vas *VersionAnalyticsService) GetMetrics(ctx context.Context, query *MetricsQuery) (*MetricsResult, error) {
	vas.mu.RLock()
	defer vas.mu.RUnlock()
	
	return vas.storage.GetMetrics(ctx, query)
}

// GetTimeSeries retrieves time series data
func (vas *VersionAnalyticsService) GetTimeSeries(ctx context.Context, query *TimeSeriesQuery) (*TimeSeriesResult, error) {
	vas.mu.RLock()
	defer vas.mu.RUnlock()
	
	return vas.storage.GetTimeSeries(ctx, query)
}

// GetTopN retrieves top N results
func (vas *VersionAnalyticsService) GetTopN(ctx context.Context, query *TopNQuery) (*TopNResult, error) {
	vas.mu.RLock()
	defer vas.mu.RUnlock()
	
	return vas.storage.GetTopN(ctx, query)
}

// ProcessEvent processes an event for aggregation
func (ma *MetricsAggregator) ProcessEvent(event *AnalyticsEvent) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	
	windowKey := ma.getWindowKey(event.Timestamp)
	window := ma.getOrCreateWindow(windowKey, event.Timestamp)
	
	// Update aggregates for each dimension
	for _, dimension := range ma.config.Dimensions {
		dimensionValue := ma.getDimensionValue(event, dimension)
		key := fmt.Sprintf("%s:%s", dimension, dimensionValue)
		
		aggregate := window.Aggregates[key]
		if aggregate == nil {
			aggregate = &WindowAggregate{
				Dimension:     key,
				UniqueUsers:   make(map[string]bool),
				UniqueClients: make(map[string]bool),
				ResponseTimes: make([]time.Duration, 0),
			}
			window.Aggregates[key] = aggregate
		}
		
		// Update aggregate
		aggregate.RequestCount++
		if event.StatusCode >= 400 {
			aggregate.ErrorCount++
		}
		aggregate.UniqueUsers[event.UserID] = true
		aggregate.UniqueClients[event.ClientID] = true
		aggregate.ResponseTimes = append(aggregate.ResponseTimes, event.ResponseTime)
		aggregate.TotalDataTransfer += event.RequestSize + event.ResponseSize
	}
	
	window.EventCount++
}

// getWindowKey generates a window key for the timestamp
func (ma *MetricsAggregator) getWindowKey(timestamp time.Time) string {
	windowStart := timestamp.Truncate(ma.config.WindowSize)
	return windowStart.Format(time.RFC3339)
}

// getOrCreateWindow gets or creates an aggregation window
func (ma *MetricsAggregator) getOrCreateWindow(key string, timestamp time.Time) *AggregationWindow {
	window, exists := ma.windows[key]
	if exists {
		return window
	}
	
	windowStart := timestamp.Truncate(ma.config.WindowSize)
	window = &AggregationWindow{
		StartTime:  windowStart,
		EndTime:    windowStart.Add(ma.config.WindowSize),
		Aggregates: make(map[string]*WindowAggregate),
		Completed:  false,
	}
	
	ma.windows[key] = window
	
	// Clean up old windows
	ma.cleanupOldWindows()
	
	return window
}

// getDimensionValue extracts dimension value from event
func (ma *MetricsAggregator) getDimensionValue(event *AnalyticsEvent, dimension string) string {
	switch dimension {
	case "version":
		return event.Version
	case "client":
		return event.ClientID
	case "user":
		return event.UserID
	case "path":
		return event.Path
	case "status":
		return fmt.Sprintf("%d", event.StatusCode/100*100) // Group by status class (2xx, 4xx, etc.)
	default:
		return "unknown"
	}
}

// cleanupOldWindows removes old aggregation windows
func (ma *MetricsAggregator) cleanupOldWindows() {
	if len(ma.windows) <= ma.config.RetentionCount {
		return
	}
	
	// Sort windows by start time
	keys := make([]string, 0, len(ma.windows))
	for key := range ma.windows {
		keys = append(keys, key)
	}
	
	sort.Strings(keys)
	
	// Remove oldest windows
	toRemove := len(keys) - ma.config.RetentionCount
	for i := 0; i < toRemove; i++ {
		delete(ma.windows, keys[i])
	}
}

// createDefaultTemplates creates default dashboard templates
func (ad *AnalyticsDashboard) createDefaultTemplates() {
	// Version Overview Template
	versionOverview := &DashboardTemplate{
		ID:          "version-overview",
		Name:        "Version Overview",
		Description: "Overview of API version usage and performance",
		Category:    "overview",
		Widgets: []*DashboardWidget{
			{
				ID:    "version-usage",
				Type:  WidgetTypeChart,
				Title: "Version Usage Distribution",
				Query: &MetricsQuery{
					GroupBy: []string{"version"},
					Metrics: []MetricType{MetricRequestCount},
				},
				Position: &WidgetPosition{X: 0, Y: 0, Width: 6, Height: 4},
			},
			{
				ID:    "error-rates",
				Type:  WidgetTypeChart,
				Title: "Error Rates by Version",
				Query: &MetricsQuery{
					GroupBy: []string{"version"},
					Metrics: []MetricType{MetricErrorRate},
				},
				Position: &WidgetPosition{X: 6, Y: 0, Width: 6, Height: 4},
			},
			{
				ID:    "response-times",
				Type:  WidgetTypeChart,
				Title: "Response Times by Version",
				Query: &TimeSeriesQuery{
					Metric:   MetricP95ResponseTime,
					GroupBy:  "version",
					Interval: 5 * time.Minute,
				},
				Position: &WidgetPosition{X: 0, Y: 4, Width: 12, Height: 4},
			},
		},
	}
	
	ad.templates["version-overview"] = versionOverview
	
	// Deprecation Monitoring Template
	deprecationMonitoring := &DashboardTemplate{
		ID:          "deprecation-monitoring",
		Name:        "Deprecation Monitoring",
		Description: "Monitor deprecated version usage and migration progress",
		Category:    "lifecycle",
		Widgets: []*DashboardWidget{
			{
				ID:    "deprecated-usage",
				Type:  WidgetTypeMetric,
				Title: "Deprecated Version Usage",
				Query: &MetricsQuery{
					Filters: map[string]interface{}{"status": "deprecated"},
					Metrics: []MetricType{MetricRequestCount},
				},
				Position: &WidgetPosition{X: 0, Y: 0, Width: 3, Height: 2},
			},
			{
				ID:    "migration-progress",
				Type:  WidgetTypeChart,
				Title: "Migration Progress",
				Query: &TimeSeriesQuery{
					Metric:   MetricRequestCount,
					GroupBy:  "version",
					Interval: 1 * time.Hour,
				},
				Position: &WidgetPosition{X: 3, Y: 0, Width: 9, Height: 4},
			},
		},
	}
	
	ad.templates["deprecation-monitoring"] = deprecationMonitoring
}

// AnalyticsHandler provides HTTP endpoints for analytics
func (vas *VersionAnalyticsService) AnalyticsHandler() http.Handler {
	mux := http.NewServeMux()
	
	mux.HandleFunc("/metrics", vas.handleMetrics)
	mux.HandleFunc("/timeseries", vas.handleTimeSeries)
	mux.HandleFunc("/topn", vas.handleTopN)
	mux.HandleFunc("/dashboard", vas.handleDashboard)
	mux.HandleFunc("/alerts", vas.handleAlerts)
	
	return mux
}

// handleMetrics handles metrics queries
func (vas *VersionAnalyticsService) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	var query MetricsQuery
	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
		http.Error(w, "Invalid query", http.StatusBadRequest)
		return
	}
	
	result, err := vas.GetMetrics(r.Context(), &query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	json.NewEncoder(w).Encode(result)
}

// handleTimeSeries handles time series queries
func (vas *VersionAnalyticsService) handleTimeSeries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	var query TimeSeriesQuery
	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
		http.Error(w, "Invalid query", http.StatusBadRequest)
		return
	}
	
	result, err := vas.GetTimeSeries(r.Context(), &query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	json.NewEncoder(w).Encode(result)
}

// handleTopN handles top N queries
func (vas *VersionAnalyticsService) handleTopN(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	var query TopNQuery
	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
		http.Error(w, "Invalid query", http.StatusBadRequest)
		return
	}
	
	result, err := vas.GetTopN(r.Context(), &query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	json.NewEncoder(w).Encode(result)
}

// handleDashboard handles dashboard requests
func (vas *VersionAnalyticsService) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	templateID := r.URL.Query().Get("template")
	if templateID == "" {
		// Return available templates
		templates := make([]string, 0)
		for id := range vas.dashboard.templates {
			templates = append(templates, id)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"available_templates": templates,
		})
		return
	}
	
	template, exists := vas.dashboard.templates[templateID]
	if !exists {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}
	
	json.NewEncoder(w).Encode(template)
}

// handleAlerts handles alert queries
func (vas *VersionAnalyticsService) handleAlerts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	alerts := vas.alerting.GetActiveAlerts()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"alerts": alerts,
		"count":  len(alerts),
	})
}

// GetActiveAlerts returns currently active alerts
func (as *AlertingService) GetActiveAlerts() []Alert {
	as.mu.RLock()
	defer as.mu.RUnlock()
	
	active := make([]Alert, 0)
	for _, alert := range as.history {
		if alert.Status == AlertStatusFiring {
			active = append(active, alert)
		}
	}
	
	return active
}