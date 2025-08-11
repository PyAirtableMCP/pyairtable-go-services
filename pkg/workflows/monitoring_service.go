package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// WorkflowMonitoringService provides comprehensive monitoring and observability for workflows
type WorkflowMonitoringService struct {
	metricsCollector   MetricsCollector
	alertManager       AlertManager
	dashboardService   DashboardService
	logAggregator      LogAggregator
	tracingService     TracingService
	healthChecker      HealthChecker
	performanceAnalyzer PerformanceAnalyzer
	eventStore         EventStore
	mu                 sync.RWMutex
}

// MetricsCollector interface for collecting workflow metrics
type MetricsCollector interface {
	CollectMetric(ctx context.Context, metric *WorkflowMetric) error
	GetMetrics(ctx context.Context, filter MetricsFilter) ([]*WorkflowMetric, error)
	AggregateMetrics(ctx context.Context, aggregation MetricsAggregation) (*AggregatedMetrics, error)
	CreateCustomMetric(ctx context.Context, definition *CustomMetricDefinition) error
	GetMetricsHistory(ctx context.Context, workflowID uuid.UUID, timeRange TimeRange) (*MetricsHistory, error)
	ExportMetrics(ctx context.Context, format string, filter MetricsFilter) ([]byte, error)
}

// DashboardService interface for creating and managing dashboards
type DashboardService interface {
	CreateDashboard(ctx context.Context, dashboard *Dashboard) error
	UpdateDashboard(ctx context.Context, dashboard *Dashboard) error
	GetDashboard(ctx context.Context, dashboardID uuid.UUID) (*Dashboard, error)
	ListDashboards(ctx context.Context, filter DashboardFilter) ([]*Dashboard, error)
	GenerateReport(ctx context.Context, dashboardID uuid.UUID, format string) (*DashboardReport, error)
	SubscribeToUpdates(ctx context.Context, dashboardID uuid.UUID, callback func(*DashboardUpdate)) error
}

// LogAggregator interface for log aggregation and analysis
type LogAggregator interface {
	CollectLog(ctx context.Context, log *WorkflowLog) error
	QueryLogs(ctx context.Context, query LogQuery) ([]*WorkflowLog, error)
	AggregateLogsByLevel(ctx context.Context, workflowID uuid.UUID, timeRange TimeRange) (*LogAggregation, error)
	SearchLogs(ctx context.Context, searchQuery string, filter LogFilter) ([]*WorkflowLog, error)
	CreateLogAlert(ctx context.Context, alert *LogAlert) error
	ArchiveLogs(ctx context.Context, archivePolicy ArchivePolicy) error
}

// TracingService interface for distributed tracing
type TracingService interface {
	StartTrace(ctx context.Context, workflowID uuid.UUID, operation string) (*TraceContext, error)
	EndTrace(ctx context.Context, traceID uuid.UUID, result TraceResult) error
	AddSpan(ctx context.Context, traceID uuid.UUID, span *TraceSpan) error
	GetTrace(ctx context.Context, traceID uuid.UUID) (*Trace, error)
	QueryTraces(ctx context.Context, query TraceQuery) ([]*Trace, error)
	AnalyzePerformance(ctx context.Context, traceID uuid.UUID) (*PerformanceAnalysis, error)
}

// HealthChecker interface for health monitoring
type HealthChecker interface {
	CheckWorkflowHealth(ctx context.Context, workflowID uuid.UUID) (*HealthStatus, error)
	GetSystemHealth(ctx context.Context) (*SystemHealth, error)
	RegisterHealthCheck(ctx context.Context, check *HealthCheck) error
	RunHealthChecks(ctx context.Context, workflowID uuid.UUID) ([]*HealthCheckResult, error)
	CreateHealthAlert(ctx context.Context, alert *HealthAlert) error
}

// PerformanceAnalyzer interface for performance analysis
type PerformanceAnalyzer interface {
	AnalyzeWorkflowPerformance(ctx context.Context, workflowID uuid.UUID, timeRange TimeRange) (*PerformanceReport, error)
	DetectBottlenecks(ctx context.Context, workflowID uuid.UUID) ([]*Bottleneck, error)
	GenerateOptimizationSuggestions(ctx context.Context, workflowID uuid.UUID) ([]*OptimizationSuggestion, error)
	ComparePerformance(ctx context.Context, workflowID1, workflowID2 uuid.UUID, timeRange TimeRange) (*PerformanceComparison, error)
	CreatePerformanceBaseline(ctx context.Context, workflowID uuid.UUID) (*PerformanceBaseline, error)
}

// Domain entities for monitoring
type WorkflowMetric struct {
	ID            uuid.UUID                `json:"id"`
	WorkflowID    uuid.UUID                `json:"workflow_id"`
	WorkflowType  string                   `json:"workflow_type"`
	MetricName    string                   `json:"metric_name"`
	MetricType    string                   `json:"metric_type"` // counter, gauge, histogram, summary
	Value         float64                  `json:"value"`
	Unit          string                   `json:"unit"`
	Tags          map[string]string        `json:"tags"`
	Dimensions    map[string]interface{}   `json:"dimensions"`
	Timestamp     time.Time                `json:"timestamp"`
	TenantID      uuid.UUID                `json:"tenant_id"`
	UserID        uuid.UUID                `json:"user_id"`
}

type MetricsFilter struct {
	WorkflowIDs   []uuid.UUID   `json:"workflow_ids,omitempty"`
	MetricNames   []string      `json:"metric_names,omitempty"`
	MetricTypes   []string      `json:"metric_types,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
	TimeRange     *TimeRange    `json:"time_range,omitempty"`
	TenantID      *uuid.UUID    `json:"tenant_id,omitempty"`
	Limit         int           `json:"limit,omitempty"`
	Offset        int           `json:"offset,omitempty"`
}

type MetricsAggregation struct {
	GroupBy       []string      `json:"group_by"`
	Aggregations  []string      `json:"aggregations"` // sum, avg, min, max, count
	Interval      time.Duration `json:"interval"`
	TimeRange     TimeRange     `json:"time_range"`
	Filter        MetricsFilter `json:"filter"`
}

type AggregatedMetrics struct {
	Groups        []*MetricGroup           `json:"groups"`
	Aggregations  map[string]float64       `json:"aggregations"`
	TimeRange     TimeRange                `json:"time_range"`
	Interval      time.Duration            `json:"interval"`
	GeneratedAt   time.Time                `json:"generated_at"`
}

type MetricGroup struct {
	GroupKey      map[string]interface{}   `json:"group_key"`
	Metrics       []*WorkflowMetric        `json:"metrics"`
	Aggregations  map[string]float64       `json:"aggregations"`
	Count         int64                    `json:"count"`
}

type CustomMetricDefinition struct {
	ID            uuid.UUID                `json:"id"`
	Name          string                   `json:"name"`
	Description   string                   `json:"description"`
	Type          string                   `json:"type"`
	Unit          string                   `json:"unit"`
	Expression    string                   `json:"expression"`
	Tags          []string                 `json:"tags"`
	Dimensions    []string                 `json:"dimensions"`
	Enabled       bool                     `json:"enabled"`
	CreatedBy     uuid.UUID                `json:"created_by"`
	CreatedAt     time.Time                `json:"created_at"`
}

type MetricsHistory struct {
	WorkflowID    uuid.UUID                `json:"workflow_id"`
	TimeRange     TimeRange                `json:"time_range"`
	DataPoints    []*MetricDataPoint       `json:"data_points"`
	Statistics    *MetricStatistics        `json:"statistics"`
	Trends        []*MetricTrend           `json:"trends"`
}

type MetricDataPoint struct {
	Timestamp     time.Time                `json:"timestamp"`
	Value         float64                  `json:"value"`
	Tags          map[string]string        `json:"tags"`
}

type MetricStatistics struct {
	Count         int64                    `json:"count"`
	Sum           float64                  `json:"sum"`
	Average       float64                  `json:"average"`
	Min           float64                  `json:"min"`
	Max           float64                  `json:"max"`
	StdDev        float64                  `json:"std_dev"`
	Percentiles   map[string]float64       `json:"percentiles"`
}

type MetricTrend struct {
	Period        string                   `json:"period"`
	Direction     string                   `json:"direction"` // increasing, decreasing, stable
	Slope         float64                  `json:"slope"`
	Confidence    float64                  `json:"confidence"`
	StartValue    float64                  `json:"start_value"`
	EndValue      float64                  `json:"end_value"`
}

type Dashboard struct {
	ID            uuid.UUID                `json:"id"`
	Name          string                   `json:"name"`
	Description   string                   `json:"description"`
	Type          string                   `json:"type"` // workflow, system, custom
	WorkflowIDs   []uuid.UUID              `json:"workflow_ids"`
	Widgets       []*DashboardWidget       `json:"widgets"`
	Layout        *DashboardLayout         `json:"layout"`
	RefreshRate   time.Duration            `json:"refresh_rate"`
	IsPublic      bool                     `json:"is_public"`
	Tags          []string                 `json:"tags"`
	CreatedBy     uuid.UUID                `json:"created_by"`
	CreatedAt     time.Time                `json:"created_at"`
	UpdatedAt     time.Time                `json:"updated_at"`
}

type DashboardWidget struct {
	ID            uuid.UUID                `json:"id"`
	Type          string                   `json:"type"` // chart, table, metric, log, alert
	Title         string                   `json:"title"`
	Configuration *WidgetConfiguration     `json:"configuration"`
	Position      *WidgetPosition          `json:"position"`
	Size          *WidgetSize              `json:"size"`
	DataSource    *WidgetDataSource        `json:"data_source"`
	Filters       map[string]interface{}   `json:"filters"`
	RefreshRate   time.Duration            `json:"refresh_rate"`
}

type WidgetConfiguration struct {
	ChartType     string                   `json:"chart_type,omitempty"`
	Axes          map[string]interface{}   `json:"axes,omitempty"`
	Colors        []string                 `json:"colors,omitempty"`
	ShowLegend    bool                     `json:"show_legend"`
	ShowGrid      bool                     `json:"show_grid"`
	Thresholds    []*Threshold             `json:"thresholds,omitempty"`
	Format        string                   `json:"format,omitempty"`
}

type WidgetSize struct {
	Width         int                      `json:"width"`
	Height        int                      `json:"height"`
}

type WidgetDataSource struct {
	Type          string                   `json:"type"` // metrics, logs, traces, health
	Query         string                   `json:"query"`
	Aggregation   string                   `json:"aggregation,omitempty"`
	GroupBy       []string                 `json:"group_by,omitempty"`
	TimeRange     *TimeRange               `json:"time_range,omitempty"`
}

type Threshold struct {
	Value         float64                  `json:"value"`
	Operator      string                   `json:"operator"` // gt, gte, lt, lte, eq
	Color         string                   `json:"color"`
	Label         string                   `json:"label"`
}

type DashboardLayout struct {
	Type          string                   `json:"type"` // grid, flexible
	Columns       int                      `json:"columns"`
	RowHeight     int                      `json:"row_height"`
	Margin        map[string]int           `json:"margin"`
	Padding       map[string]int           `json:"padding"`
}

type DashboardFilter struct {
	Type          string                   `json:"type,omitempty"`
	WorkflowIDs   []uuid.UUID              `json:"workflow_ids,omitempty"`
	CreatedBy     *uuid.UUID               `json:"created_by,omitempty"`
	Tags          []string                 `json:"tags,omitempty"`
	Search        string                   `json:"search,omitempty"`
	Limit         int                      `json:"limit,omitempty"`
	Offset        int                      `json:"offset,omitempty"`
}

type DashboardReport struct {
	DashboardID   uuid.UUID                `json:"dashboard_id"`
	Format        string                   `json:"format"`
	Data          []byte                   `json:"data"`
	GeneratedAt   time.Time                `json:"generated_at"`
	TimeRange     TimeRange                `json:"time_range"`
	Metadata      map[string]interface{}   `json:"metadata"`
}

type DashboardUpdate struct {
	DashboardID   uuid.UUID                `json:"dashboard_id"`
	UpdateType    string                   `json:"update_type"`
	Data          interface{}              `json:"data"`
	Timestamp     time.Time                `json:"timestamp"`
}

type WorkflowLog struct {
	ID            uuid.UUID                `json:"id"`
	WorkflowID    uuid.UUID                `json:"workflow_id"`
	ExecutionID   *uuid.UUID               `json:"execution_id,omitempty"`
	StepID        *uuid.UUID               `json:"step_id,omitempty"`
	Level         string                   `json:"level"` // debug, info, warn, error, fatal
	Message       string                   `json:"message"`
	Data          map[string]interface{}   `json:"data"`
	Tags          map[string]string        `json:"tags"`
	Source        string                   `json:"source"`
	Timestamp     time.Time                `json:"timestamp"`
	TenantID      uuid.UUID                `json:"tenant_id"`
	UserID        uuid.UUID                `json:"user_id"`
	TraceID       *uuid.UUID               `json:"trace_id,omitempty"`
	SpanID        *uuid.UUID               `json:"span_id,omitempty"`
}

type LogQuery struct {
	WorkflowIDs   []uuid.UUID              `json:"workflow_ids,omitempty"`
	ExecutionIDs  []uuid.UUID              `json:"execution_ids,omitempty"`
	Levels        []string                 `json:"levels,omitempty"`
	TimeRange     *TimeRange               `json:"time_range,omitempty"`
	Query         string                   `json:"query,omitempty"`
	Tags          map[string]string        `json:"tags,omitempty"`
	Limit         int                      `json:"limit,omitempty"`
	Offset        int                      `json:"offset,omitempty"`
	OrderBy       string                   `json:"order_by,omitempty"`
	OrderDir      string                   `json:"order_dir,omitempty"`
}

type LogFilter struct {
	WorkflowIDs   []uuid.UUID              `json:"workflow_ids,omitempty"`
	Levels        []string                 `json:"levels,omitempty"`
	Sources       []string                 `json:"sources,omitempty"`
	Tags          map[string]string        `json:"tags,omitempty"`
	TimeRange     *TimeRange               `json:"time_range,omitempty"`
}

type LogAggregation struct {
	WorkflowID    uuid.UUID                `json:"workflow_id"`
	TimeRange     TimeRange                `json:"time_range"`
	LevelCounts   map[string]int64         `json:"level_counts"`
	SourceCounts  map[string]int64         `json:"source_counts"`
	TotalLogs     int64                    `json:"total_logs"`
	ErrorRate     float64                  `json:"error_rate"`
	Timeline      []*LogTimelinePoint      `json:"timeline"`
}

type LogTimelinePoint struct {
	Timestamp     time.Time                `json:"timestamp"`
	LevelCounts   map[string]int64         `json:"level_counts"`
	TotalCount    int64                    `json:"total_count"`
}

type LogAlert struct {
	ID            uuid.UUID                `json:"id"`
	Name          string                   `json:"name"`
	Query         string                   `json:"query"`
	Condition     string                   `json:"condition"`
	Threshold     int64                    `json:"threshold"`
	TimeWindow    time.Duration            `json:"time_window"`
	Severity      string                   `json:"severity"`
	Recipients    []string                 `json:"recipients"`
	Actions       []AlertAction            `json:"actions"`
	Enabled       bool                     `json:"enabled"`
	CreatedAt     time.Time                `json:"created_at"`
}

type ArchivePolicy struct {
	RetentionPeriod time.Duration          `json:"retention_period"`
	ArchiveAfter    time.Duration          `json:"archive_after"`
	Compression     bool                   `json:"compression"`
	Storage         string                 `json:"storage"`
	DeleteAfter     *time.Duration         `json:"delete_after,omitempty"`
}

type TraceContext struct {
	TraceID       uuid.UUID                `json:"trace_id"`
	WorkflowID    uuid.UUID                `json:"workflow_id"`
	Operation     string                   `json:"operation"`
	StartTime     time.Time                `json:"start_time"`
	Tags          map[string]string        `json:"tags"`
	Baggage       map[string]interface{}   `json:"baggage"`
}

type Trace struct {
	ID            uuid.UUID                `json:"id"`
	WorkflowID    uuid.UUID                `json:"workflow_id"`
	Operation     string                   `json:"operation"`
	StartTime     time.Time                `json:"start_time"`
	EndTime       *time.Time               `json:"end_time,omitempty"`
	Duration      *time.Duration           `json:"duration,omitempty"`
	Status        string                   `json:"status"`
	Tags          map[string]string        `json:"tags"`
	Spans         []*TraceSpan             `json:"spans"`
	Errors        []*TraceError            `json:"errors"`
}

type TraceSpan struct {
	ID            uuid.UUID                `json:"id"`
	TraceID       uuid.UUID                `json:"trace_id"`
	ParentSpanID  *uuid.UUID               `json:"parent_span_id,omitempty"`
	Operation     string                   `json:"operation"`
	StartTime     time.Time                `json:"start_time"`
	EndTime       *time.Time               `json:"end_time,omitempty"`
	Duration      *time.Duration           `json:"duration,omitempty"`
	Tags          map[string]string        `json:"tags"`
	Logs          []*SpanLog               `json:"logs"`
	Status        string                   `json:"status"`
}

type SpanLog struct {
	Timestamp     time.Time                `json:"timestamp"`
	Level         string                   `json:"level"`
	Message       string                   `json:"message"`
	Fields        map[string]interface{}   `json:"fields"`
}

type TraceError struct {
	SpanID        uuid.UUID                `json:"span_id"`
	Type          string                   `json:"type"`
	Message       string                   `json:"message"`
	Stack         string                   `json:"stack"`
	Timestamp     time.Time                `json:"timestamp"`
}

type TraceResult struct {
	Status        string                   `json:"status"` // success, error, timeout
	Error         *TraceError              `json:"error,omitempty"`
	Metadata      map[string]interface{}   `json:"metadata"`
}

type TraceQuery struct {
	WorkflowIDs   []uuid.UUID              `json:"workflow_ids,omitempty"`
	Operations    []string                 `json:"operations,omitempty"`
	Status        []string                 `json:"status,omitempty"`
	MinDuration   *time.Duration           `json:"min_duration,omitempty"`
	MaxDuration   *time.Duration           `json:"max_duration,omitempty"`
	TimeRange     *TimeRange               `json:"time_range,omitempty"`
	Tags          map[string]string        `json:"tags,omitempty"`
	HasErrors     *bool                    `json:"has_errors,omitempty"`
	Limit         int                      `json:"limit,omitempty"`
	Offset        int                      `json:"offset,omitempty"`
}

type PerformanceAnalysis struct {
	TraceID       uuid.UUID                `json:"trace_id"`
	TotalDuration time.Duration            `json:"total_duration"`
	SpanCount     int                      `json:"span_count"`
	ErrorCount    int                      `json:"error_count"`
	Bottlenecks   []*SpanBottleneck        `json:"bottlenecks"`
	HotPaths      []*HotPath               `json:"hot_paths"`
	Recommendations []*PerformanceRecommendation `json:"recommendations"`
}

type SpanBottleneck struct {
	SpanID        uuid.UUID                `json:"span_id"`
	Operation     string                   `json:"operation"`
	Duration      time.Duration            `json:"duration"`
	Percentage    float64                  `json:"percentage"`
	Type          string                   `json:"type"` // cpu, io, network, wait
}

type HotPath struct {
	Path          []string                 `json:"path"`
	Duration      time.Duration            `json:"duration"`
	Frequency     int                      `json:"frequency"`
	Percentage    float64                  `json:"percentage"`
}

type PerformanceRecommendation struct {
	Type          string                   `json:"type"`
	Priority      string                   `json:"priority"`
	Description   string                   `json:"description"`
	Impact        string                   `json:"impact"`
	Effort        string                   `json:"effort"`
	SpanIDs       []uuid.UUID              `json:"span_ids,omitempty"`
}

type HealthStatus struct {
	WorkflowID    uuid.UUID                `json:"workflow_id"`
	Status        string                   `json:"status"` // healthy, warning, critical, unknown
	Score         float64                  `json:"score"` // 0-100
	Components    []*ComponentHealth       `json:"components"`
	Issues        []*HealthIssue           `json:"issues"`
	LastChecked   time.Time                `json:"last_checked"`
	NextCheck     time.Time                `json:"next_check"`
}

type ComponentHealth struct {
	Name          string                   `json:"name"`
	Status        string                   `json:"status"`
	Score         float64                  `json:"score"`
	Message       string                   `json:"message"`
	LastChecked   time.Time                `json:"last_checked"`
	Metadata      map[string]interface{}   `json:"metadata"`
}

type HealthIssue struct {
	Type          string                   `json:"type"`
	Severity      string                   `json:"severity"`
	Component     string                   `json:"component"`
	Description   string                   `json:"description"`
	Resolution    string                   `json:"resolution,omitempty"`
	DetectedAt    time.Time                `json:"detected_at"`
}

type SystemHealth struct {
	Status        string                   `json:"status"`
	Score         float64                  `json:"score"`
	Services      []*ServiceHealth         `json:"services"`
	Infrastructure []*InfrastructureHealth `json:"infrastructure"`
	Workflows     []*WorkflowHealthSummary `json:"workflows"`
	LastChecked   time.Time                `json:"last_checked"`
}

type ServiceHealth struct {
	Name          string                   `json:"name"`
	Status        string                   `json:"status"`
	Score         float64                  `json:"score"`
	Version       string                   `json:"version"`
	Uptime        time.Duration            `json:"uptime"`
	ResponseTime  time.Duration            `json:"response_time"`
	ErrorRate     float64                  `json:"error_rate"`
	LastChecked   time.Time                `json:"last_checked"`
}

type InfrastructureHealth struct {
	Component     string                   `json:"component"`
	Status        string                   `json:"status"`
	Score         float64                  `json:"score"`
	Metrics       map[string]float64       `json:"metrics"`
	Thresholds    map[string]float64       `json:"thresholds"`
	LastChecked   time.Time                `json:"last_checked"`
}

type WorkflowHealthSummary struct {
	WorkflowID    uuid.UUID                `json:"workflow_id"`
	WorkflowType  string                   `json:"workflow_type"`
	Status        string                   `json:"status"`
	Score         float64                  `json:"score"`
	ExecutionCount int64                   `json:"execution_count"`
	SuccessRate   float64                  `json:"success_rate"`
	AvgDuration   time.Duration            `json:"avg_duration"`
	LastExecution *time.Time               `json:"last_execution,omitempty"`
}

type HealthCheck struct {
	ID            uuid.UUID                `json:"id"`
	Name          string                   `json:"name"`
	Type          string                   `json:"type"`
	Target        string                   `json:"target"`
	Interval      time.Duration            `json:"interval"`
	Timeout       time.Duration            `json:"timeout"`
	Retries       int                      `json:"retries"`
	Thresholds    map[string]float64       `json:"thresholds"`
	Enabled       bool                     `json:"enabled"`
	CreatedAt     time.Time                `json:"created_at"`
}

type HealthCheckResult struct {
	CheckID       uuid.UUID                `json:"check_id"`
	Status        string                   `json:"status"`
	Score         float64                  `json:"score"`
	Duration      time.Duration            `json:"duration"`
	Message       string                   `json:"message"`
	Metadata      map[string]interface{}   `json:"metadata"`
	Timestamp     time.Time                `json:"timestamp"`
}

type HealthAlert struct {
	ID            uuid.UUID                `json:"id"`
	Name          string                   `json:"name"`
	Condition     string                   `json:"condition"`
	Threshold     float64                  `json:"threshold"`
	Components    []string                 `json:"components"`
	Severity      string                   `json:"severity"`
	Recipients    []string                 `json:"recipients"`
	Actions       []AlertAction            `json:"actions"`
	Enabled       bool                     `json:"enabled"`
	CreatedAt     time.Time                `json:"created_at"`
}

type PerformanceReport struct {
	WorkflowID    uuid.UUID                `json:"workflow_id"`
	TimeRange     TimeRange                `json:"time_range"`
	Overview      *PerformanceOverview     `json:"overview"`
	Trends        []*PerformanceTrend      `json:"trends"`
	Bottlenecks   []*Bottleneck            `json:"bottlenecks"`
	Comparisons   []*PerformanceComparison `json:"comparisons"`
	Recommendations []*OptimizationSuggestion `json:"recommendations"`
	GeneratedAt   time.Time                `json:"generated_at"`
}

type PerformanceOverview struct {
	ExecutionCount    int64                `json:"execution_count"`
	SuccessRate       float64              `json:"success_rate"`
	AvgDuration       time.Duration        `json:"avg_duration"`
	MinDuration       time.Duration        `json:"min_duration"`
	MaxDuration       time.Duration        `json:"max_duration"`
	Throughput        float64              `json:"throughput"`
	ErrorRate         float64              `json:"error_rate"`
	ResourceUtilization map[string]float64 `json:"resource_utilization"`
}

type PerformanceTrend struct {
	Metric        string                   `json:"metric"`
	Period        string                   `json:"period"`
	Direction     string                   `json:"direction"`
	Change        float64                  `json:"change"`
	ChangePercent float64                  `json:"change_percent"`
	Significance  string                   `json:"significance"`
	DataPoints    []*TrendDataPoint        `json:"data_points"`
}

type TrendDataPoint struct {
	Timestamp     time.Time                `json:"timestamp"`
	Value         float64                  `json:"value"`
}

type Bottleneck struct {
	ID            uuid.UUID                `json:"id"`
	Type          string                   `json:"type"`
	Component     string                   `json:"component"`
	Description   string                   `json:"description"`
	Impact        string                   `json:"impact"`
	Severity      string                   `json:"severity"`
	Duration      time.Duration            `json:"duration"`
	Frequency     int                      `json:"frequency"`
	AffectedSteps []uuid.UUID              `json:"affected_steps"`
	DetectedAt    time.Time                `json:"detected_at"`
}

type OptimizationSuggestion struct {
	ID            uuid.UUID                `json:"id"`
	Type          string                   `json:"type"`
	Category      string                   `json:"category"`
	Title         string                   `json:"title"`
	Description   string                   `json:"description"`
	Priority      string                   `json:"priority"`
	Impact        string                   `json:"impact"`
	Effort        string                   `json:"effort"`
	ExpectedGain  string                   `json:"expected_gain"`
	Implementation string                  `json:"implementation"`
	RelatedBottlenecks []uuid.UUID          `json:"related_bottlenecks"`
}

type PerformanceComparison struct {
	BaselineID    uuid.UUID                `json:"baseline_id"`
	ComparisonID  uuid.UUID                `json:"comparison_id"`
	Metrics       map[string]*MetricComparison `json:"metrics"`
	Summary       string                   `json:"summary"`
	Improvement   float64                  `json:"improvement"`
	Regression    float64                  `json:"regression"`
}

type MetricComparison struct {
	BaselineValue float64                  `json:"baseline_value"`
	ComparisonValue float64                `json:"comparison_value"`
	Change        float64                  `json:"change"`
	ChangePercent float64                  `json:"change_percent"`
	Significance  string                   `json:"significance"`
}

type PerformanceBaseline struct {
	ID            uuid.UUID                `json:"id"`
	WorkflowID    uuid.UUID                `json:"workflow_id"`
	Name          string                   `json:"name"`
	Description   string                   `json:"description"`
	Metrics       map[string]float64       `json:"metrics"`
	TimeRange     TimeRange                `json:"time_range"`
	CreatedAt     time.Time                `json:"created_at"`
	IsActive      bool                     `json:"is_active"`
}

// NewWorkflowMonitoringService creates a new workflow monitoring service
func NewWorkflowMonitoringService(
	metricsCollector MetricsCollector,
	alertManager AlertManager,
	dashboardService DashboardService,
	logAggregator LogAggregator,
	tracingService TracingService,
	healthChecker HealthChecker,
	performanceAnalyzer PerformanceAnalyzer,
	eventStore EventStore,
) *WorkflowMonitoringService {
	return &WorkflowMonitoringService{
		metricsCollector:    metricsCollector,
		alertManager:        alertManager,
		dashboardService:    dashboardService,
		logAggregator:       logAggregator,
		tracingService:      tracingService,
		healthChecker:       healthChecker,
		performanceAnalyzer: performanceAnalyzer,
		eventStore:          eventStore,
	}
}

// StartMonitoring initializes monitoring for a workflow
func (s *WorkflowMonitoringService) StartMonitoring(ctx context.Context, workflowID uuid.UUID, config *MonitoringConfiguration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create default dashboard for the workflow
	dashboard := &Dashboard{
		ID:          uuid.New(),
		Name:        fmt.Sprintf("Workflow %s Dashboard", workflowID.String()[:8]),
		Description: "Auto-generated dashboard for workflow monitoring",
		Type:        "workflow",
		WorkflowIDs: []uuid.UUID{workflowID},
		Widgets:     s.createDefaultWidgets(workflowID),
		Layout: &DashboardLayout{
			Type:      "grid",
			Columns:   3,
			RowHeight: 200,
			Margin:    map[string]int{"top": 10, "bottom": 10, "left": 10, "right": 10},
		},
		RefreshRate: 30 * time.Second,
		IsPublic:    false,
		Tags:        []string{"auto-generated", "workflow"},
		CreatedBy:   config.CreatedBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.dashboardService.CreateDashboard(ctx, dashboard); err != nil {
		return errors.Wrap(err, "failed to create monitoring dashboard")
	}

	// Set up default health checks
	healthChecks := s.createDefaultHealthChecks(workflowID)
	for _, check := range healthChecks {
		if err := s.healthChecker.RegisterHealthCheck(ctx, check); err != nil {
			log.Printf("Failed to register health check %s: %v", check.Name, err)
		}
	}

	// Set up default alerts
	alerts := s.createDefaultAlerts(workflowID, config)
	for _, alert := range alerts {
		if err := s.alertManager.CreateAlert(ctx, alert); err != nil {
			log.Printf("Failed to create alert %s: %v", alert.Name, err)
		}
	}

	// Emit monitoring started event
	s.emitMonitoringEvent(ctx, "monitoring.started", map[string]interface{}{
		"workflow_id":    workflowID,
		"dashboard_id":   dashboard.ID,
		"health_checks":  len(healthChecks),
		"alerts":         len(alerts),
	})

	return nil
}

// CollectWorkflowMetrics collects metrics for a specific workflow execution
func (s *WorkflowMonitoringService) CollectWorkflowMetrics(ctx context.Context, workflowID uuid.UUID, executionID uuid.UUID, metrics map[string]float64) error {
	timestamp := time.Now()

	for metricName, value := range metrics {
		metric := &WorkflowMetric{
			ID:           uuid.New(),
			WorkflowID:   workflowID,
			MetricName:   metricName,
			MetricType:   s.determineMetricType(metricName),
			Value:        value,
			Unit:         s.determineMetricUnit(metricName),
			Tags: map[string]string{
				"execution_id": executionID.String(),
				"source":       "workflow_execution",
			},
			Timestamp: timestamp,
		}

		if err := s.metricsCollector.CollectMetric(ctx, metric); err != nil {
			log.Printf("Failed to collect metric %s: %v", metricName, err)
		}
	}

	// Emit metrics collected event
	s.emitMonitoringEvent(ctx, "metrics.collected", map[string]interface{}{
		"workflow_id":  workflowID,
		"execution_id": executionID,
		"metric_count": len(metrics),
	})

	return nil
}

// LogWorkflowEvent logs an event for workflow monitoring
func (s *WorkflowMonitoringService) LogWorkflowEvent(ctx context.Context, workflowID uuid.UUID, level string, message string, data map[string]interface{}) error {
	log := &WorkflowLog{
		ID:         uuid.New(),
		WorkflowID: workflowID,
		Level:      level,
		Message:    message,
		Data:       data,
		Source:     "workflow_monitoring",
		Timestamp:  time.Now(),
	}

	if executionID, exists := data["execution_id"]; exists {
		if id, ok := executionID.(uuid.UUID); ok {
			log.ExecutionID = &id
		}
	}

	if stepID, exists := data["step_id"]; exists {
		if id, ok := stepID.(uuid.UUID); ok {
			log.StepID = &id
		}
	}

	return s.logAggregator.CollectLog(ctx, log)
}

// StartTrace starts a distributed trace for a workflow operation
func (s *WorkflowMonitoringService) StartTrace(ctx context.Context, workflowID uuid.UUID, operation string) (*TraceContext, error) {
	return s.tracingService.StartTrace(ctx, workflowID, operation)
}

// EndTrace ends a distributed trace
func (s *WorkflowMonitoringService) EndTrace(ctx context.Context, traceID uuid.UUID, result TraceResult) error {
	return s.tracingService.EndTrace(ctx, traceID, result)
}

// GetWorkflowHealth returns the current health status of a workflow
func (s *WorkflowMonitoringService) GetWorkflowHealth(ctx context.Context, workflowID uuid.UUID) (*HealthStatus, error) {
	return s.healthChecker.CheckWorkflowHealth(ctx, workflowID)
}

// GeneratePerformanceReport generates a comprehensive performance report
func (s *WorkflowMonitoringService) GeneratePerformanceReport(ctx context.Context, workflowID uuid.UUID, timeRange TimeRange) (*PerformanceReport, error) {
	return s.performanceAnalyzer.AnalyzeWorkflowPerformance(ctx, workflowID, timeRange)
}

// GetWorkflowDashboard returns the dashboard for a workflow
func (s *WorkflowMonitoringService) GetWorkflowDashboard(ctx context.Context, workflowID uuid.UUID) (*Dashboard, error) {
	filter := DashboardFilter{
		WorkflowIDs: []uuid.UUID{workflowID},
		Type:        "workflow",
		Limit:       1,
	}

	dashboards, err := s.dashboardService.ListDashboards(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get workflow dashboard")
	}

	if len(dashboards) == 0 {
		return nil, errors.New("no dashboard found for workflow")
	}

	return dashboards[0], nil
}

// Helper methods
func (s *WorkflowMonitoringService) createDefaultWidgets(workflowID uuid.UUID) []*DashboardWidget {
	widgets := []*DashboardWidget{
		{
			ID:    uuid.New(),
			Type:  "metric",
			Title: "Execution Count",
			Configuration: &WidgetConfiguration{
				Format: "number",
			},
			Position: &WidgetPosition{X: 0, Y: 0},
			Size:     &WidgetSize{Width: 1, Height: 1},
			DataSource: &WidgetDataSource{
				Type:        "metrics",
				Query:       "execution_count",
				Aggregation: "sum",
				TimeRange:   &TimeRange{StartTime: time.Now().Add(-24 * time.Hour), EndTime: time.Now()},
			},
			RefreshRate: 30 * time.Second,
		},
		{
			ID:    uuid.New(),
			Type:  "chart",
			Title: "Success Rate",
			Configuration: &WidgetConfiguration{
				ChartType:  "line",
				ShowLegend: true,
				ShowGrid:   true,
				Thresholds: []*Threshold{
					{Value: 0.9, Operator: "gte", Color: "green", Label: "Good"},
					{Value: 0.7, Operator: "gte", Color: "yellow", Label: "Warning"},
					{Value: 0.7, Operator: "lt", Color: "red", Label: "Critical"},
				},
			},
			Position: &WidgetPosition{X: 1, Y: 0},
			Size:     &WidgetSize{Width: 2, Height: 1},
			DataSource: &WidgetDataSource{
				Type:        "metrics",
				Query:       "success_rate",
				Aggregation: "avg",
				TimeRange:   &TimeRange{StartTime: time.Now().Add(-24 * time.Hour), EndTime: time.Now()},
			},
			RefreshRate: 30 * time.Second,
		},
		{
			ID:    uuid.New(),
			Type:  "chart",
			Title: "Execution Duration",
			Configuration: &WidgetConfiguration{
				ChartType:  "histogram",
				ShowLegend: false,
				ShowGrid:   true,
			},
			Position: &WidgetPosition{X: 0, Y: 1},
			Size:     &WidgetSize{Width: 3, Height: 1},
			DataSource: &WidgetDataSource{
				Type:        "metrics",
				Query:       "execution_duration",
				Aggregation: "histogram",
				TimeRange:   &TimeRange{StartTime: time.Now().Add(-24 * time.Hour), EndTime: time.Now()},
			},
			RefreshRate: 30 * time.Second,
		},
		{
			ID:    uuid.New(),
			Type:  "log",
			Title: "Recent Errors",
			Configuration: &WidgetConfiguration{
				Format: "table",
			},
			Position: &WidgetPosition{X: 0, Y: 2},
			Size:     &WidgetSize{Width: 3, Height: 1},
			DataSource: &WidgetDataSource{
				Type:      "logs",
				Query:     "level:error",
				TimeRange: &TimeRange{StartTime: time.Now().Add(-1 * time.Hour), EndTime: time.Now()},
			},
			RefreshRate: 15 * time.Second,
		},
	}

	return widgets
}

func (s *WorkflowMonitoringService) createDefaultHealthChecks(workflowID uuid.UUID) []*HealthCheck {
	return []*HealthCheck{
		{
			ID:       uuid.New(),
			Name:     "Workflow Execution Rate",
			Type:     "metric",
			Target:   "execution_rate",
			Interval: 1 * time.Minute,
			Timeout:  10 * time.Second,
			Retries:  3,
			Thresholds: map[string]float64{
				"warning":  0.1, // executions per minute
				"critical": 0.01,
			},
			Enabled:   true,
			CreatedAt: time.Now(),
		},
		{
			ID:       uuid.New(),
			Name:     "Error Rate",
			Type:     "metric",
			Target:   "error_rate",
			Interval: 1 * time.Minute,
			Timeout:  10 * time.Second,
			Retries:  3,
			Thresholds: map[string]float64{
				"warning":  0.1, // 10% error rate
				"critical": 0.3, // 30% error rate
			},
			Enabled:   true,
			CreatedAt: time.Now(),
		},
	}
}

func (s *WorkflowMonitoringService) createDefaultAlerts(workflowID uuid.UUID, config *MonitoringConfiguration) []*AlertConfig {
	return []*AlertConfig{
		{
			ID:        uuid.New(),
			Name:      "High Error Rate",
			Condition: "error_rate > 0.1",
			Threshold: 0.1,
			Recipients: config.AlertRecipients,
			Actions: []AlertAction{
				{Type: "email", Config: json.RawMessage(`{"template": "high_error_rate"}`)},
				{Type: "webhook", Config: json.RawMessage(`{"url": "` + config.WebhookURL + `"}`)},
			},
			Enabled: true,
		},
		{
			ID:        uuid.New(),
			Name:      "Execution Timeout",
			Condition: "avg_duration > 1800", // 30 minutes
			Threshold: 1800,
			Recipients: config.AlertRecipients,
			Actions: []AlertAction{
				{Type: "email", Config: json.RawMessage(`{"template": "execution_timeout"}`)},
			},
			Enabled: true,
		},
	}
}

func (s *WorkflowMonitoringService) determineMetricType(metricName string) string {
	switch {
	case contains(metricName, "count") || contains(metricName, "total"):
		return "counter"
	case contains(metricName, "rate") || contains(metricName, "ratio"):
		return "gauge"
	case contains(metricName, "duration") || contains(metricName, "time"):
		return "histogram"
	default:
		return "gauge"
	}
}

func (s *WorkflowMonitoringService) determineMetricUnit(metricName string) string {
	switch {
	case contains(metricName, "duration") || contains(metricName, "time"):
		return "seconds"
	case contains(metricName, "count") || contains(metricName, "total"):
		return "count"
	case contains(metricName, "rate"):
		return "per_second"
	case contains(metricName, "ratio") || contains(metricName, "percent"):
		return "percent"
	case contains(metricName, "size") || contains(metricName, "bytes"):
		return "bytes"
	default:
		return "unit"
	}
}

func (s *WorkflowMonitoringService) emitMonitoringEvent(ctx context.Context, eventType string, data map[string]interface{}) error {
	eventData, _ := json.Marshal(data)
	
	event := &WorkflowEvent{
		ID:         uuid.New(),
		WorkflowID: uuid.Nil, // System event
		Type:       eventType,
		Version:    1,
		Data:       eventData,
		Metadata:   map[string]interface{}{"source": "monitoring_service"},
		Timestamp:  time.Now(),
	}

	return s.eventStore.SaveEvent(ctx, event)
}

// MonitoringConfiguration holds monitoring setup configuration
type MonitoringConfiguration struct {
	CreatedBy        uuid.UUID `json:"created_by"`
	AlertRecipients  []string  `json:"alert_recipients"`
	WebhookURL       string    `json:"webhook_url"`
	EnableTracing    bool      `json:"enable_tracing"`
	EnableHealthChecks bool    `json:"enable_health_checks"`
	MetricsRetention time.Duration `json:"metrics_retention"`
	LogsRetention    time.Duration `json:"logs_retention"`
}