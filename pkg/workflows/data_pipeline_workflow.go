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

// DataPipelineWorkflow implements ETL/ELT pipeline orchestration
type DataPipelineWorkflow struct {
	instance            *WorkflowInstance
	config              *DataPipelineConfig
	eventStore          EventStore
	pipelineOrchestrator PipelineOrchestrator
	dataExtractor       DataExtractor
	dataTransformer     DataTransformer
	dataLoader          DataLoader
	dataValidator       DataValidator
	checkpointManager   CheckpointManager
	errorHandler        PipelineErrorHandler
	monitoringService   PipelineMonitoringService
	mu                  sync.RWMutex
}

// PipelineOrchestrator interface for pipeline orchestration
type PipelineOrchestrator interface {
	CreatePipeline(ctx context.Context, config *PipelineDefinition) (*Pipeline, error)
	UpdatePipeline(ctx context.Context, pipelineID uuid.UUID, config *PipelineDefinition) error
	DeletePipeline(ctx context.Context, pipelineID uuid.UUID) error
	GetPipeline(ctx context.Context, pipelineID uuid.UUID) (*Pipeline, error)
	ListPipelines(ctx context.Context, filter PipelineFilter) ([]*Pipeline, error)
	ExecutePipeline(ctx context.Context, pipelineID uuid.UUID, params map[string]interface{}) (*PipelineExecution, error)
	SchedulePipeline(ctx context.Context, pipelineID uuid.UUID, schedule ScheduleConfig) error
	PausePipeline(ctx context.Context, pipelineID uuid.UUID) error
	ResumePipeline(ctx context.Context, pipelineID uuid.UUID) error
}

// DataExtractor interface for data extraction operations
type DataExtractor interface {
	ExtractFromSource(ctx context.Context, source DataSource, config ExtractionConfig) (*ExtractedData, error)
	ValidateSource(ctx context.Context, source DataSource) error
	GetSourceSchema(ctx context.Context, source DataSource) (*DataSchema, error)
	EstimateDataSize(ctx context.Context, source DataSource) (*DataSizeEstimate, error)
	SupportIncrementalExtraction(ctx context.Context, source DataSource) bool
	ExtractIncremental(ctx context.Context, source DataSource, checkpoint *Checkpoint) (*ExtractedData, error)
}

// DataTransformer interface for data transformation operations
type DataTransformer interface {
	TransformData(ctx context.Context, data *DataBatch, transformations []Transformation) (*DataBatch, error)
	ValidateTransformation(ctx context.Context, transformation Transformation) error
	CompileTransformation(ctx context.Context, transformation Transformation) (*CompiledTransformation, error)
	ExecuteTransformationPipeline(ctx context.Context, data *DataBatch, pipeline *TransformationPipeline) (*DataBatch, error)
	GetSupportedTransformations(ctx context.Context) ([]*TransformationType, error)
}

// DataLoader interface for data loading operations
type DataLoader interface {
	LoadToDestination(ctx context.Context, data *DataBatch, destination DataDestination, config LoadingConfig) (*LoadResult, error)
	ValidateDestination(ctx context.Context, destination DataDestination) error
	GetDestinationSchema(ctx context.Context, destination DataDestination) (*DataSchema, error)
	SupportsBulkLoading(ctx context.Context, destination DataDestination) bool
	LoadBulk(ctx context.Context, data []*DataBatch, destination DataDestination, config BulkLoadingConfig) (*BulkLoadResult, error)
	CreateTable(ctx context.Context, destination DataDestination, schema *DataSchema) error
}

// DataValidator interface for data validation operations
type DataValidator interface {
	ValidateData(ctx context.Context, data *DataBatch, rules []ValidationRule) (*ValidationResult, error)
	ValidateSchema(ctx context.Context, data *DataBatch, expectedSchema *DataSchema) (*SchemaValidationResult, error)
	ValidateDataQuality(ctx context.Context, data *DataBatch, qualityRules []DataQualityRule) (*DataQualityResult, error)
	CreateValidationProfile(ctx context.Context, data *DataBatch) (*ValidationProfile, error)
	CompareDataProfiles(ctx context.Context, profile1, profile2 *ValidationProfile) (*ProfileComparison, error)
}

// CheckpointManager interface for checkpoint management
type CheckpointManager interface {
	CreateCheckpoint(ctx context.Context, pipelineID uuid.UUID, stage string, data map[string]interface{}) (*Checkpoint, error)
	GetLatestCheckpoint(ctx context.Context, pipelineID uuid.UUID, stage string) (*Checkpoint, error)
	GetCheckpointHistory(ctx context.Context, pipelineID uuid.UUID, stage string, limit int) ([]*Checkpoint, error)
	UpdateCheckpoint(ctx context.Context, checkpoint *Checkpoint) error
	DeleteCheckpoint(ctx context.Context, checkpointID uuid.UUID) error
	RestoreFromCheckpoint(ctx context.Context, checkpoint *Checkpoint) (*RestorationResult, error)
}

// PipelineErrorHandler interface for error handling
type PipelineErrorHandler interface {
	HandleError(ctx context.Context, err *PipelineError, config ErrorHandlingConfig) (*ErrorHandlingResult, error)
	CreateDeadLetterRecord(ctx context.Context, data *DataBatch, error *PipelineError) (*DeadLetterRecord, error)
	ProcessDeadLetterQueue(ctx context.Context, pipelineID uuid.UUID) ([]*DeadLetterRecord, error)
	RetryFailedBatch(ctx context.Context, batchID uuid.UUID) (*RetryResult, error)
	GetErrorStatistics(ctx context.Context, pipelineID uuid.UUID, timeRange TimeRange) (*ErrorStatistics, error)
}

// PipelineMonitoringService interface for monitoring
type PipelineMonitoringService interface {
	StartMonitoring(ctx context.Context, pipelineID uuid.UUID) error
	StopMonitoring(ctx context.Context, pipelineID uuid.UUID) error
	GetMetrics(ctx context.Context, pipelineID uuid.UUID, timeRange TimeRange) (*PipelineMetrics, error)
	GetPerformanceStats(ctx context.Context, pipelineID uuid.UUID) (*PerformanceStats, error)
	CreateAlert(ctx context.Context, alert *PipelineAlert) error
	TriggerAlert(ctx context.Context, alertID uuid.UUID, data map[string]interface{}) error
	GetDashboard(ctx context.Context, pipelineID uuid.UUID) (*MonitoringDashboard, error)
}

// Domain entities
type Pipeline struct {
	ID            uuid.UUID                `json:"id"`
	Name          string                   `json:"name"`
	Description   string                   `json:"description"`
	Type          string                   `json:"type"` // ETL, ELT, streaming, batch
	Status        string                   `json:"status"`
	Configuration *PipelineDefinition      `json:"configuration"`
	Schedule      *ScheduleConfig          `json:"schedule,omitempty"`
	Metadata      map[string]interface{}   `json:"metadata"`
	CreatedBy     uuid.UUID                `json:"created_by"`
	CreatedAt     time.Time                `json:"created_at"`
	UpdatedAt     time.Time                `json:"updated_at"`
	LastExecution *time.Time               `json:"last_execution,omitempty"`
	NextExecution *time.Time               `json:"next_execution,omitempty"`
}

type PipelineDefinition struct {
	Sources         []DataSource             `json:"sources"`
	Destinations    []DataDestination        `json:"destinations"`
	Transformations []Transformation         `json:"transformations"`
	Validation      ValidationConfig         `json:"validation"`
	ErrorHandling   ErrorHandlingConfig      `json:"error_handling"`
	Monitoring      MonitoringConfig         `json:"monitoring"`
	Performance     PerformanceConfig        `json:"performance"`
	Security        SecurityConfig           `json:"security"`
}

type PipelineExecution struct {
	ID              uuid.UUID                `json:"id"`
	PipelineID      uuid.UUID                `json:"pipeline_id"`
	Status          string                   `json:"status"`
	Type            string                   `json:"type"` // full, incremental, backfill
	StartedAt       time.Time                `json:"started_at"`
	CompletedAt     *time.Time               `json:"completed_at,omitempty"`
	Duration        *time.Duration           `json:"duration,omitempty"`
	Stages          []*PipelineStage         `json:"stages"`
	Metrics         *PipelineMetrics         `json:"metrics"`
	Error           *PipelineError           `json:"error,omitempty"`
	Checkpoints     []*Checkpoint            `json:"checkpoints"`
	TriggeredBy     string                   `json:"triggered_by"`
	Parameters      map[string]interface{}   `json:"parameters"`
}

type PipelineStage struct {
	ID          uuid.UUID                `json:"id"`
	Name        string                   `json:"name"`
	Type        string                   `json:"type"` // extract, transform, validate, load
	Status      string                   `json:"status"`
	StartedAt   time.Time                `json:"started_at"`
	CompletedAt *time.Time               `json:"completed_at,omitempty"`
	Duration    *time.Duration           `json:"duration,omitempty"`
	Input       *StageInput              `json:"input"`
	Output      *StageOutput             `json:"output"`
	Error       *PipelineError           `json:"error,omitempty"`
	RetryCount  int                      `json:"retry_count"`
	Metrics     map[string]interface{}   `json:"metrics"`
}

type StageInput struct {
	DataBatches []*DataBatch             `json:"data_batches"`
	Metadata    map[string]interface{}   `json:"metadata"`
	Size        int64                    `json:"size"`
	RecordCount int64                    `json:"record_count"`
}

type StageOutput struct {
	DataBatches []*DataBatch             `json:"data_batches"`
	Metadata    map[string]interface{}   `json:"metadata"`
	Size        int64                    `json:"size"`
	RecordCount int64                    `json:"record_count"`
	Quality     *DataQualityMetrics      `json:"quality"`
}

type ExtractedData struct {
	Batches     []*DataBatch             `json:"batches"`
	Schema      *DataSchema              `json:"schema"`
	Metadata    *ExtractionMetadata      `json:"metadata"`
	Size        int64                    `json:"size"`
	RecordCount int64                    `json:"record_count"`
	Checksum    string                   `json:"checksum"`
}

type DataBatch struct {
	ID          uuid.UUID                `json:"id"`
	Data        []map[string]interface{} `json:"data"`
	Schema      *DataSchema              `json:"schema"`
	Metadata    map[string]interface{}   `json:"metadata"`
	Size        int64                    `json:"size"`
	RecordCount int                      `json:"record_count"`
	BatchIndex  int                      `json:"batch_index"`
	CreatedAt   time.Time                `json:"created_at"`
}

type ExtractionConfig struct {
	BatchSize       int                      `json:"batch_size"`
	MaxConcurrency  int                      `json:"max_concurrency"`
	Timeout         time.Duration            `json:"timeout"`
	Incremental     bool                     `json:"incremental"`
	WatermarkColumn string                   `json:"watermark_column,omitempty"`
	FilterCondition string                   `json:"filter_condition,omitempty"`
	Compression     bool                     `json:"compression"`
}

type ExtractionMetadata struct {
	SourceType      string                   `json:"source_type"`
	ExtractionTime  time.Time                `json:"extraction_time"`
	BatchCount      int                      `json:"batch_count"`
	TotalRecords    int64                    `json:"total_records"`
	DataRange       *DataRange               `json:"data_range,omitempty"`
	Watermark       interface{}              `json:"watermark,omitempty"`
	Compression     string                   `json:"compression,omitempty"`
}

type DataRange struct {
	StartTime *time.Time   `json:"start_time,omitempty"`
	EndTime   *time.Time   `json:"end_time,omitempty"`
	StartID   *int64       `json:"start_id,omitempty"`
	EndID     *int64       `json:"end_id,omitempty"`
}

type DataSizeEstimate struct {
	EstimatedSize    int64     `json:"estimated_size"`
	EstimatedRecords int64     `json:"estimated_records"`
	Confidence       float64   `json:"confidence"`
	BasedOn          string    `json:"based_on"`
	Timestamp        time.Time `json:"timestamp"`
}

type CompiledTransformation struct {
	Transformation *Transformation          `json:"transformation"`
	CompiledCode   interface{}              `json:"compiled_code"`
	Dependencies   []string                 `json:"dependencies"`
	OptimizedPlan  *ExecutionPlan           `json:"optimized_plan"`
}

type TransformationPipeline struct {
	ID              uuid.UUID                `json:"id"`
	Transformations []*CompiledTransformation `json:"transformations"`
	ExecutionPlan   *ExecutionPlan           `json:"execution_plan"`
	OptimizedFor    string                   `json:"optimized_for"` // speed, memory, accuracy
}

type TransformationType struct {
	Name        string                   `json:"name"`
	Category    string                   `json:"category"`
	Description string                   `json:"description"`
	Parameters  []*ParameterDefinition   `json:"parameters"`
	Examples    []*TransformationExample `json:"examples"`
}

type TransformationExample struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Input       interface{}              `json:"input"`
	Output      interface{}              `json:"output"`
	Config      map[string]interface{}   `json:"config"`
}

type ExecutionPlan struct {
	Steps       []*ExecutionStep         `json:"steps"`
	Parallelism int                      `json:"parallelism"`
	EstimatedCost *ExecutionCost         `json:"estimated_cost"`
	OptimizationHints []string           `json:"optimization_hints"`
}

type ExecutionStep struct {
	ID           uuid.UUID                `json:"id"`
	Type         string                   `json:"type"`
	Dependencies []uuid.UUID              `json:"dependencies"`
	CanParallel  bool                     `json:"can_parallel"`
	EstimatedDuration time.Duration       `json:"estimated_duration"`
	ResourceRequirements *ResourceRequirements `json:"resource_requirements"`
}

type ExecutionCost struct {
	CPUCost    float64 `json:"cpu_cost"`
	MemoryCost float64 `json:"memory_cost"`
	IOCost     float64 `json:"io_cost"`
	NetworkCost float64 `json:"network_cost"`
	TotalCost  float64 `json:"total_cost"`
}

type ResourceRequirements struct {
	CPU    int   `json:"cpu"`    // CPU cores
	Memory int64 `json:"memory"` // Memory in MB
	Disk   int64 `json:"disk"`   // Disk space in MB
	Network int  `json:"network"` // Network bandwidth in Mbps
}

type LoadingConfig struct {
	BatchSize      int           `json:"batch_size"`
	MaxConcurrency int           `json:"max_concurrency"`
	Timeout        time.Duration `json:"timeout"`
	WriteMode      string        `json:"write_mode"` // append, overwrite, upsert
	CreateTable    bool          `json:"create_table"`
	Compression    bool          `json:"compression"`
	Partitioning   *PartitioningConfig `json:"partitioning,omitempty"`
}

type BulkLoadingConfig struct {
	LoadingConfig
	ChunkSize       int           `json:"chunk_size"`
	ParallelLoaders int           `json:"parallel_loaders"`
	PreLoadActions  []string      `json:"pre_load_actions"`
	PostLoadActions []string      `json:"post_load_actions"`
}

type PartitioningConfig struct {
	Strategy string   `json:"strategy"` // time, hash, range
	Columns  []string `json:"columns"`
	Size     string   `json:"size"`     // daily, weekly, monthly
}

type LoadResult struct {
	Success       bool                     `json:"success"`
	RecordsLoaded int64                    `json:"records_loaded"`
	RecordsFailed int64                    `json:"records_failed"`
	Duration      time.Duration            `json:"duration"`
	Error         *PipelineError           `json:"error,omitempty"`
	Metadata      map[string]interface{}   `json:"metadata"`
}

type BulkLoadResult struct {
	LoadResults   []*LoadResult            `json:"load_results"`
	TotalRecords  int64                    `json:"total_records"`
	SuccessRate   float64                  `json:"success_rate"`
	Duration      time.Duration            `json:"duration"`
	Errors        []*PipelineError         `json:"errors"`
}

type ValidationProfile struct {
	ID              uuid.UUID                `json:"id"`
	DatasetID       uuid.UUID                `json:"dataset_id"`
	Schema          *DataSchema              `json:"schema"`
	Statistics      *DataStatistics          `json:"statistics"`
	QualityMetrics  *DataQualityMetrics      `json:"quality_metrics"`
	Anomalies       []*DataAnomaly           `json:"anomalies"`
	CreatedAt       time.Time                `json:"created_at"`
}

type DataStatistics struct {
	RecordCount     int64                    `json:"record_count"`
	ColumnStats     map[string]*ColumnStats  `json:"column_stats"`
	NullCounts      map[string]int64         `json:"null_counts"`
	UniqueCounts    map[string]int64         `json:"unique_counts"`
	DataTypes       map[string]string        `json:"data_types"`
	DataSize        int64                    `json:"data_size"`
	CompressionRatio float64                 `json:"compression_ratio"`
}

type ColumnStats struct {
	Min         interface{} `json:"min"`
	Max         interface{} `json:"max"`
	Mean        float64     `json:"mean,omitempty"`
	Median      float64     `json:"median,omitempty"`
	StdDev      float64     `json:"std_dev,omitempty"`
	Percentiles map[string]float64 `json:"percentiles,omitempty"`
	TopValues   []interface{} `json:"top_values,omitempty"`
}

type DataQualityMetrics struct {
	Completeness    float64                  `json:"completeness"`
	Accuracy        float64                  `json:"accuracy"`
	Consistency     float64                  `json:"consistency"`
	Validity        float64                  `json:"validity"`
	Uniqueness      float64                  `json:"uniqueness"`
	Timeliness      float64                  `json:"timeliness"`
	OverallScore    float64                  `json:"overall_score"`
	Issues          []*QualityIssue          `json:"issues"`
}

type QualityIssue struct {
	Type        string      `json:"type"`
	Severity    string      `json:"severity"`
	Column      string      `json:"column,omitempty"`
	Description string      `json:"description"`
	Count       int         `json:"count"`
	Examples    []interface{} `json:"examples,omitempty"`
}

type SchemaValidationResult struct {
	IsValid         bool                     `json:"is_valid"`
	Errors          []*SchemaValidationError `json:"errors"`
	Warnings        []*SchemaValidationWarning `json:"warnings"`
	Score           float64                  `json:"score"`
	CompatibilityLevel string                `json:"compatibility_level"`
}

type SchemaValidationError struct {
	Field       string `json:"field"`
	ExpectedType string `json:"expected_type"`
	ActualType  string `json:"actual_type"`
	Message     string `json:"message"`
}

type SchemaValidationWarning struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Impact  string `json:"impact"`
}

type DataQualityRule struct {
	ID          uuid.UUID                `json:"id"`
	Name        string                   `json:"name"`
	Type        string                   `json:"type"`
	Column      string                   `json:"column,omitempty"`
	Expression  string                   `json:"expression"`
	Severity    string                   `json:"severity"`
	Threshold   float64                  `json:"threshold"`
	Enabled     bool                     `json:"enabled"`
}

type DataQualityResult struct {
	OverallScore float64                  `json:"overall_score"`
	RuleResults  []*RuleResult            `json:"rule_results"`
	Issues       []*QualityIssue          `json:"issues"`
	Recommendations []*QualityRecommendation `json:"recommendations"`
}

type RuleResult struct {
	RuleID      uuid.UUID `json:"rule_id"`
	Passed      bool      `json:"passed"`
	Score       float64   `json:"score"`
	ViolationCount int    `json:"violation_count"`
	Message     string    `json:"message"`
}

type QualityRecommendation struct {
	Type        string `json:"type"`
	Priority    string `json:"priority"`
	Description string `json:"description"`
	Action      string `json:"action"`
}

type ProfileComparison struct {
	Profile1ID      uuid.UUID                `json:"profile1_id"`
	Profile2ID      uuid.UUID                `json:"profile2_id"`
	SimilarityScore float64                  `json:"similarity_score"`
	Differences     []*ProfileDifference     `json:"differences"`
	SchemaChanges   []*SchemaChange          `json:"schema_changes"`
	QualityChanges  []*QualityChange         `json:"quality_changes"`
}

type ProfileDifference struct {
	Field       string      `json:"field"`
	Type        string      `json:"type"`
	OldValue    interface{} `json:"old_value"`
	NewValue    interface{} `json:"new_value"`
	Significance string     `json:"significance"`
}

type SchemaChange struct {
	Type        string `json:"type"` // added, removed, modified
	Field       string `json:"field"`
	OldType     string `json:"old_type,omitempty"`
	NewType     string `json:"new_type,omitempty"`
	Breaking    bool   `json:"breaking"`
}

type QualityChange struct {
	Metric      string  `json:"metric"`
	OldScore    float64 `json:"old_score"`
	NewScore    float64 `json:"new_score"`
	Change      float64 `json:"change"`
	Significant bool    `json:"significant"`
}

type Checkpoint struct {
	ID          uuid.UUID                `json:"id"`
	PipelineID  uuid.UUID                `json:"pipeline_id"`
	Stage       string                   `json:"stage"`
	Data        map[string]interface{}   `json:"data"`
	Metadata    map[string]interface{}   `json:"metadata"`
	CreatedAt   time.Time                `json:"created_at"`
	ExpiresAt   *time.Time               `json:"expires_at,omitempty"`
}

type RestorationResult struct {
	Success     bool                     `json:"success"`
	CheckpointID uuid.UUID               `json:"checkpoint_id"`
	RestoredData map[string]interface{}  `json:"restored_data"`
	Duration    time.Duration            `json:"duration"`
	Error       *PipelineError           `json:"error,omitempty"`
}

type PipelineError struct {
	ID          uuid.UUID                `json:"id"`
	Code        string                   `json:"code"`
	Message     string                   `json:"message"`
	Stage       string                   `json:"stage"`
	Details     map[string]interface{}   `json:"details"`
	Data        *DataBatch               `json:"data,omitempty"`
	Recoverable bool                     `json:"recoverable"`
	RetryCount  int                      `json:"retry_count"`
	Timestamp   time.Time                `json:"timestamp"`
	StackTrace  string                   `json:"stack_trace,omitempty"`
}

type ErrorHandlingResult struct {
	Action      string                   `json:"action"` // retry, skip, dlq, stop
	Success     bool                     `json:"success"`
	RetryDelay  *time.Duration           `json:"retry_delay,omitempty"`
	DLQRecord   *DeadLetterRecord        `json:"dlq_record,omitempty"`
	Metadata    map[string]interface{}   `json:"metadata"`
}

type DeadLetterRecord struct {
	ID          uuid.UUID                `json:"id"`
	PipelineID  uuid.UUID                `json:"pipeline_id"`
	BatchID     uuid.UUID                `json:"batch_id"`
	Data        *DataBatch               `json:"data"`
	Error       *PipelineError           `json:"error"`
	RetryCount  int                      `json:"retry_count"`
	CreatedAt   time.Time                `json:"created_at"`
	LastRetry   *time.Time               `json:"last_retry,omitempty"`
	Status      string                   `json:"status"`
}

type RetryResult struct {
	Success     bool                     `json:"success"`
	BatchID     uuid.UUID                `json:"batch_id"`
	AttemptCount int                     `json:"attempt_count"`
	Duration    time.Duration            `json:"duration"`
	Error       *PipelineError           `json:"error,omitempty"`
}

type ErrorStatistics struct {
	PipelineID      uuid.UUID                `json:"pipeline_id"`
	TimeRange       TimeRange                `json:"time_range"`
	TotalErrors     int64                    `json:"total_errors"`
	ErrorsByStage   map[string]int64         `json:"errors_by_stage"`
	ErrorsByType    map[string]int64         `json:"errors_by_type"`
	RecoverableErrors int64                  `json:"recoverable_errors"`
	FatalErrors     int64                    `json:"fatal_errors"`
	ErrorRate       float64                  `json:"error_rate"`
	MTTR            time.Duration            `json:"mttr"` // Mean Time To Recovery
}

type PipelineMetrics struct {
	PipelineID      uuid.UUID                `json:"pipeline_id"`
	ExecutionID     uuid.UUID                `json:"execution_id"`
	StartTime       time.Time                `json:"start_time"`
	EndTime         *time.Time               `json:"end_time,omitempty"`
	Duration        *time.Duration           `json:"duration,omitempty"`
	RecordsProcessed int64                   `json:"records_processed"`
	DataProcessed   int64                    `json:"data_processed"`
	Throughput      float64                  `json:"throughput"` // records per second
	SuccessRate     float64                  `json:"success_rate"`
	StageMetrics    map[string]*StageMetrics `json:"stage_metrics"`
	ResourceUsage   *ResourceUsage           `json:"resource_usage"`
	QualityMetrics  *DataQualityMetrics      `json:"quality_metrics"`
}

type StageMetrics struct {
	StageName       string                   `json:"stage_name"`
	Duration        time.Duration            `json:"duration"`
	RecordsIn       int64                    `json:"records_in"`
	RecordsOut      int64                    `json:"records_out"`
	Throughput      float64                  `json:"throughput"`
	ErrorCount      int64                    `json:"error_count"`
	RetryCount      int64                    `json:"retry_count"`
	ResourceUsage   *ResourceUsage           `json:"resource_usage"`
}

type ResourceUsage struct {
	CPUUsage        float64                  `json:"cpu_usage"`        // percentage
	MemoryUsage     int64                    `json:"memory_usage"`     // bytes
	DiskUsage       int64                    `json:"disk_usage"`       // bytes
	NetworkIO       int64                    `json:"network_io"`       // bytes
	PeakMemory      int64                    `json:"peak_memory"`      // bytes
	AvgCPU          float64                  `json:"avg_cpu"`          // percentage
}

type PerformanceStats struct {
	PipelineID      uuid.UUID                `json:"pipeline_id"`
	TimeRange       TimeRange                `json:"time_range"`
	ExecutionCount  int64                    `json:"execution_count"`
	SuccessCount    int64                    `json:"success_count"`
	FailureCount    int64                    `json:"failure_count"`
	AvgDuration     time.Duration            `json:"avg_duration"`
	MinDuration     time.Duration            `json:"min_duration"`
	MaxDuration     time.Duration            `json:"max_duration"`
	AvgThroughput   float64                  `json:"avg_throughput"`
	MaxThroughput   float64                  `json:"max_throughput"`
	TotalDataProcessed int64                 `json:"total_data_processed"`
	SuccessRate     float64                  `json:"success_rate"`
	SLACompliance   float64                  `json:"sla_compliance"`
}

type PipelineAlert struct {
	ID          uuid.UUID                `json:"id"`
	PipelineID  uuid.UUID                `json:"pipeline_id"`
	Name        string                   `json:"name"`
	Type        string                   `json:"type"`
	Condition   string                   `json:"condition"`
	Threshold   float64                  `json:"threshold"`
	Severity    string                   `json:"severity"`
	Recipients  []string                 `json:"recipients"`
	Actions     []AlertAction            `json:"actions"`
	Enabled     bool                     `json:"enabled"`
	CreatedAt   time.Time                `json:"created_at"`
}

type MonitoringDashboard struct {
	PipelineID  uuid.UUID                `json:"pipeline_id"`
	Widgets     []*DashboardWidget       `json:"widgets"`
	LastUpdated time.Time                `json:"last_updated"`
	RefreshRate time.Duration            `json:"refresh_rate"`
}

type DashboardWidget struct {
	ID      uuid.UUID                `json:"id"`
	Type    string                   `json:"type"`
	Title   string                   `json:"title"`
	Config  map[string]interface{}   `json:"config"`
	Data    interface{}              `json:"data"`
	Position *WidgetPosition         `json:"position"`
}

type WidgetPosition struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type PipelineFilter struct {
	Status    string     `json:"status,omitempty"`
	Type      string     `json:"type,omitempty"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty"`
	Tags      []string   `json:"tags,omitempty"`
	Search    string     `json:"search,omitempty"`
	Limit     int        `json:"limit,omitempty"`
	Offset    int        `json:"offset,omitempty"`
}

type PerformanceConfig struct {
	MaxConcurrency    int           `json:"max_concurrency"`
	BatchSize         int           `json:"batch_size"`
	MemoryLimit       int64         `json:"memory_limit"`
	Timeout           time.Duration `json:"timeout"`
	OptimizationLevel string        `json:"optimization_level"`
	Caching           bool          `json:"caching"`
}

type SecurityConfig struct {
	Encryption        bool              `json:"encryption"`
	DataMasking       bool              `json:"data_masking"`
	AccessControl     bool              `json:"access_control"`
	AuditLogging      bool              `json:"audit_logging"`
	Compliance        []string          `json:"compliance"`
	RetentionPolicy   *RetentionPolicy  `json:"retention_policy"`
}

type RetentionPolicy struct {
	DataRetention     time.Duration `json:"data_retention"`
	LogRetention      time.Duration `json:"log_retention"`
	MetricsRetention  time.Duration `json:"metrics_retention"`
	ArchivePolicy     string        `json:"archive_policy"`
}

// NewDataPipelineWorkflow creates a new data pipeline workflow instance
func NewDataPipelineWorkflow(
	instance *WorkflowInstance,
	eventStore EventStore,
	pipelineOrchestrator PipelineOrchestrator,
	dataExtractor DataExtractor,
	dataTransformer DataTransformer,
	dataLoader DataLoader,
	dataValidator DataValidator,
	checkpointManager CheckpointManager,
	errorHandler PipelineErrorHandler,
	monitoringService PipelineMonitoringService,
) (*DataPipelineWorkflow, error) {
	var config DataPipelineConfig
	if err := json.Unmarshal(instance.Configuration, &config); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal data pipeline configuration")
	}

	return &DataPipelineWorkflow{
		instance:            instance,
		config:              &config,
		eventStore:          eventStore,
		pipelineOrchestrator: pipelineOrchestrator,
		dataExtractor:       dataExtractor,
		dataTransformer:     dataTransformer,
		dataLoader:          dataLoader,
		dataValidator:       dataValidator,
		checkpointManager:   checkpointManager,
		errorHandler:        errorHandler,
		monitoringService:   monitoringService,
	}, nil
}

// Execute runs the complete Data Pipeline workflow
func (w *DataPipelineWorkflow) Execute(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Emit workflow started event
	if err := w.emitEvent(ctx, EventWorkflowStarted, map[string]interface{}{
		"workflow_type": WorkflowTypeDataPipeline,
		"config":        w.config,
	}); err != nil {
		return errors.Wrap(err, "failed to emit workflow started event")
	}

	// Update workflow status
	w.instance.Status = WorkflowStatusRunning
	now := time.Now()
	w.instance.StartedAt = &now

	// Execute workflow steps
	steps := []PipelineStepFunc{
		w.validateConfiguration,
		w.initializePipelineOrchestrator,
		w.setupDataSources,
		w.configureTransformations,
		w.setupDataDestinations,
		w.configureValidationRules,
		w.setupErrorHandling,
		w.enableCheckpointing,
		w.startMonitoring,
		w.executeTestPipeline,
	}

	for i, step := range steps {
		w.instance.CurrentStepIndex = i
		stepName := w.getStepName(i)
		
		if err := w.executeStep(ctx, stepName, step); err != nil {
			w.instance.Status = WorkflowStatusFailed
			w.instance.Error = &WorkflowError{
				Code:        "DATA_PIPELINE_STEP_FAILED",
				Message:     fmt.Sprintf("Step %s failed: %v", stepName, err),
				Timestamp:   time.Now(),
				Recoverable: w.isRecoverableError(err),
			}
			
			w.emitEvent(ctx, EventWorkflowFailed, map[string]interface{}{
				"error": w.instance.Error,
				"step":  stepName,
			})
			
			return err
		}

		// Update progress
		w.instance.Progress.CompletedSteps = i + 1
		w.instance.Progress.PercentComplete = float64(i+1) / float64(len(steps)) * 100
	}

	// Mark as completed
	w.instance.Status = WorkflowStatusCompleted
	completedAt := time.Now()
	w.instance.CompletedAt = &completedAt

	// Emit workflow completed event
	return w.emitEvent(ctx, EventWorkflowCompleted, map[string]interface{}{
		"duration": completedAt.Sub(*w.instance.StartedAt).String(),
		"stats":    w.getPipelineStats(),
	})
}

type PipelineStepFunc func(ctx context.Context) error

// Data pipeline workflow step implementations
func (w *DataPipelineWorkflow) validateConfiguration(ctx context.Context) error {
	if len(w.config.Sources) == 0 {
		return errors.New("no data sources configured")
	}
	if len(w.config.Destinations) == 0 {
		return errors.New("no data destinations configured")
	}
	if w.config.Pipeline.Type == "" {
		return errors.New("pipeline type must be specified")
	}
	if w.config.Pipeline.Mode == "" {
		return errors.New("pipeline mode must be specified")
	}

	// Validate sources
	for i, source := range w.config.Sources {
		if err := w.dataExtractor.ValidateSource(ctx, source); err != nil {
			return errors.Wrapf(err, "source validation failed for source %d", i)
		}
	}

	// Validate destinations
	for i, destination := range w.config.Destinations {
		if err := w.dataLoader.ValidateDestination(ctx, destination); err != nil {
			return errors.Wrapf(err, "destination validation failed for destination %d", i)
		}
	}

	return nil
}

func (w *DataPipelineWorkflow) initializePipelineOrchestrator(ctx context.Context) error {
	// Create pipeline definition
	pipelineDefinition := &PipelineDefinition{
		Sources:         w.config.Sources,
		Destinations:    w.config.Destinations,
		Transformations: w.config.Transformations,
		Validation:      w.config.Validation,
		ErrorHandling:   w.config.ErrorHandling,
		Monitoring:      w.config.Monitoring,
		Performance:     PerformanceConfig{
			MaxConcurrency:    w.config.Pipeline.Parallelism,
			BatchSize:         1000, // Default batch size
			MemoryLimit:       1024 * 1024 * 1024, // 1GB default
			Timeout:           30 * time.Minute,
			OptimizationLevel: "balanced",
			Caching:           true,
		},
		Security: SecurityConfig{
			Encryption:    true,
			DataMasking:   false,
			AccessControl: true,
			AuditLogging:  true,
			Compliance:    []string{"GDPR", "SOX"},
			RetentionPolicy: &RetentionPolicy{
				DataRetention:    90 * 24 * time.Hour, // 90 days
				LogRetention:     30 * 24 * time.Hour, // 30 days
				MetricsRetention: 365 * 24 * time.Hour, // 1 year
				ArchivePolicy:    "compress_and_store",
			},
		},
	}

	// Create pipeline
	pipeline, err := w.pipelineOrchestrator.CreatePipeline(ctx, pipelineDefinition)
	if err != nil {
		return errors.Wrap(err, "failed to create pipeline")
	}

	// Schedule pipeline if needed
	if w.config.Pipeline.Schedule.Enabled {
		if err := w.pipelineOrchestrator.SchedulePipeline(ctx, pipeline.ID, w.config.Pipeline.Schedule); err != nil {
			log.Printf("Failed to schedule pipeline: %v", err)
		}
	}

	// Store pipeline information
	w.instance.Metadata["pipeline"] = pipeline
	w.instance.Metadata["pipeline_id"] = pipeline.ID

	// Emit pipeline created event
	w.emitEvent(ctx, "pipeline.created", map[string]interface{}{
		"pipeline_id":   pipeline.ID,
		"pipeline_type": pipeline.Type,
		"sources_count": len(w.config.Sources),
		"destinations_count": len(w.config.Destinations),
		"transformations_count": len(w.config.Transformations),
	})

	return nil
}

func (w *DataPipelineWorkflow) setupDataSources(ctx context.Context) error {
	sourceSchemas := make(map[uuid.UUID]*DataSchema)
	sourceEstimates := make(map[uuid.UUID]*DataSizeEstimate)

	for _, source := range w.config.Sources {
		// Get source schema
		schema, err := w.dataExtractor.GetSourceSchema(ctx, source)
		if err != nil {
			log.Printf("Failed to get schema for source %s: %v", source.ID, err)
		} else {
			sourceSchemas[source.ID] = schema
		}

		// Estimate data size
		estimate, err := w.dataExtractor.EstimateDataSize(ctx, source)
		if err != nil {
			log.Printf("Failed to estimate data size for source %s: %v", source.ID, err)
		} else {
			sourceEstimates[source.ID] = estimate
		}

		// Test extraction capability
		supportsIncremental := w.dataExtractor.SupportIncrementalExtraction(ctx, source)
		
		// Emit source setup event
		w.emitEvent(ctx, "pipeline.source.setup", map[string]interface{}{
			"source_id":            source.ID,
			"source_type":          source.Type,
			"schema_available":     schema != nil,
			"supports_incremental": supportsIncremental,
			"estimated_records":    estimate.EstimatedRecords,
			"estimated_size":       estimate.EstimatedSize,
		})
	}

	// Store source information
	w.instance.Metadata["source_schemas"] = sourceSchemas
	w.instance.Metadata["source_estimates"] = sourceEstimates

	return nil
}

func (w *DataPipelineWorkflow) configureTransformations(ctx context.Context) error {
	if len(w.config.Transformations) == 0 {
		return nil // Skip if no transformations configured
	}

	// Get supported transformation types
	supportedTypes, err := w.dataTransformer.GetSupportedTransformations(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get supported transformations")
	}

	// Validate and compile transformations
	compiledTransformations := make([]*CompiledTransformation, 0)
	for _, transformation := range w.config.Transformations {
		// Validate transformation
		if err := w.dataTransformer.ValidateTransformation(ctx, transformation); err != nil {
			log.Printf("Transformation validation failed: %v", err)
			continue
		}

		// Compile transformation
		compiled, err := w.dataTransformer.CompileTransformation(ctx, transformation)
		if err != nil {
			log.Printf("Transformation compilation failed: %v", err)
			continue
		}

		compiledTransformations = append(compiledTransformations, compiled)
	}

	// Create transformation pipeline
	transformationPipeline := &TransformationPipeline{
		ID:              uuid.New(),
		Transformations: compiledTransformations,
		ExecutionPlan: &ExecutionPlan{
			Parallelism: w.config.Pipeline.Parallelism,
			EstimatedCost: &ExecutionCost{
				CPUCost:    100.0,
				MemoryCost: 50.0,
				IOCost:     25.0,
				NetworkCost: 10.0,
				TotalCost:  185.0,
			},
			OptimizationHints: []string{
				"batch_operations",
				"memory_efficient",
				"parallel_processing",
			},
		},
		OptimizedFor: "balanced",
	}

	// Store transformation information
	w.instance.Metadata["supported_transformation_types"] = supportedTypes
	w.instance.Metadata["compiled_transformations"] = compiledTransformations
	w.instance.Metadata["transformation_pipeline"] = transformationPipeline

	// Emit transformations configured event
	w.emitEvent(ctx, "pipeline.transformations.configured", map[string]interface{}{
		"supported_types":        len(supportedTypes),
		"configured_transformations": len(w.config.Transformations),
		"compiled_transformations":   len(compiledTransformations),
		"pipeline_id":               transformationPipeline.ID,
	})

	return nil
}

func (w *DataPipelineWorkflow) setupDataDestinations(ctx context.Context) error {
	destinationSchemas := make(map[uuid.UUID]*DataSchema)
	bulkLoadingSupport := make(map[uuid.UUID]bool)

	for _, destination := range w.config.Destinations {
		// Get destination schema
		schema, err := w.dataLoader.GetDestinationSchema(ctx, destination)
		if err != nil {
			log.Printf("Failed to get schema for destination %s: %v", destination.ID, err)
		} else {
			destinationSchemas[destination.ID] = schema
		}

		// Check bulk loading support
		supportsBulk := w.dataLoader.SupportsBulkLoading(ctx, destination)
		bulkLoadingSupport[destination.ID] = supportsBulk

		// Emit destination setup event
		w.emitEvent(ctx, "pipeline.destination.setup", map[string]interface{}{
			"destination_id":       destination.ID,
			"destination_type":     destination.Type,
			"schema_available":     schema != nil,
			"supports_bulk_loading": supportsBulk,
			"write_mode":          destination.WriteMode,
		})
	}

	// Store destination information
	w.instance.Metadata["destination_schemas"] = destinationSchemas
	w.instance.Metadata["bulk_loading_support"] = bulkLoadingSupport

	return nil
}

func (w *DataPipelineWorkflow) configureValidationRules(ctx context.Context) error {
	if !w.config.Validation.Enabled {
		return nil // Skip if validation is disabled
	}

	validationRules := w.config.Validation.Rules
	qualityRules := make([]DataQualityRule, 0)

	// Convert validation rules to quality rules
	for _, rule := range validationRules {
		qualityRule := DataQualityRule{
			ID:         rule.ID,
			Name:       fmt.Sprintf("validation_rule_%s", rule.Field),
			Type:       rule.Type,
			Column:     rule.Field,
			Expression: "",
			Severity:   "error",
			Threshold:  0.95,
			Enabled:    true,
		}

		// Set expression based on rule type
		switch rule.Type {
		case "not_null":
			qualityRule.Expression = fmt.Sprintf("%s IS NOT NULL", rule.Field)
		case "range":
			if min, exists := rule.Parameters["min"]; exists {
				if max, exists := rule.Parameters["max"]; exists {
					qualityRule.Expression = fmt.Sprintf("%s BETWEEN %v AND %v", rule.Field, min, max)
				}
			}
		case "pattern":
			if pattern, exists := rule.Parameters["pattern"]; exists {
				qualityRule.Expression = fmt.Sprintf("%s REGEXP '%s'", rule.Field, pattern)
			}
		case "uniqueness":
			qualityRule.Expression = fmt.Sprintf("COUNT(DISTINCT %s) = COUNT(%s)", rule.Field, rule.Field)
		}

		qualityRules = append(qualityRules, qualityRule)
	}

	// Store validation configuration
	w.instance.Metadata["validation_rules"] = validationRules
	w.instance.Metadata["quality_rules"] = qualityRules
	w.instance.Metadata["validation_enabled"] = w.config.Validation.Enabled
	w.instance.Metadata["validation_on_failure"] = w.config.Validation.OnFailure

	// Emit validation configured event
	w.emitEvent(ctx, "pipeline.validation.configured", map[string]interface{}{
		"validation_enabled":  w.config.Validation.Enabled,
		"validation_rules":    len(validationRules),
		"quality_rules":       len(qualityRules),
		"on_failure_action":   w.config.Validation.OnFailure,
	})

	return nil
}

func (w *DataPipelineWorkflow) setupErrorHandling(ctx context.Context) error {
	errorConfig := w.config.ErrorHandling

	// Configure error handling strategies
	strategies := map[string]string{
		"network_errors":    errorConfig.Strategy,
		"data_quality_errors": errorConfig.Strategy,
		"transformation_errors": errorConfig.Strategy,
		"loading_errors":    errorConfig.Strategy,
	}

	// Set up dead letter queue if enabled
	var dlqConfig *DLQConfig
	if errorConfig.DLQConfig.Enabled {
		dlqConfig = &errorConfig.DLQConfig
	}

	// Store error handling configuration
	w.instance.Metadata["error_handling_strategies"] = strategies
	w.instance.Metadata["retry_config"] = errorConfig.RetryConfig
	w.instance.Metadata["dlq_config"] = dlqConfig
	w.instance.Metadata["error_handling_strategy"] = errorConfig.Strategy

	// Emit error handling configured event
	w.emitEvent(ctx, "pipeline.error_handling.configured", map[string]interface{}{
		"strategy":           errorConfig.Strategy,
		"max_retry_attempts": errorConfig.RetryConfig.MaxAttempts,
		"dlq_enabled":        errorConfig.DLQConfig.Enabled,
		"dlq_max_size":       errorConfig.DLQConfig.MaxSize,
	})

	return nil
}

func (w *DataPipelineWorkflow) enableCheckpointing(ctx context.Context) error {
	checkpointConfig := w.config.Pipeline.CheckpointConfig
	
	if !checkpointConfig.Enabled {
		return nil // Skip if checkpointing is disabled
	}

	pipelineID := w.instance.Metadata["pipeline_id"].(uuid.UUID)

	// Create initial checkpoint
	initialCheckpoint, err := w.checkpointManager.CreateCheckpoint(ctx, pipelineID, "initialization", map[string]interface{}{
		"workflow_id": w.instance.ID,
		"started_at":  time.Now(),
		"config":      w.config,
	})
	if err != nil {
		return errors.Wrap(err, "failed to create initial checkpoint")
	}

	// Store checkpoint configuration
	w.instance.Metadata["checkpoint_config"] = checkpointConfig
	w.instance.Metadata["initial_checkpoint"] = initialCheckpoint
	w.instance.Metadata["checkpointing_enabled"] = true

	// Emit checkpointing enabled event
	w.emitEvent(ctx, "pipeline.checkpointing.enabled", map[string]interface{}{
		"checkpoint_interval": checkpointConfig.Interval.String(),
		"checkpoint_storage":  checkpointConfig.Storage,
		"initial_checkpoint":  initialCheckpoint.ID,
	})

	return nil
}

func (w *DataPipelineWorkflow) startMonitoring(ctx context.Context) error {
	if len(w.config.Monitoring.Metrics) == 0 {
		return nil // Skip if no monitoring configured
	}

	pipelineID := w.instance.Metadata["pipeline_id"].(uuid.UUID)

	// Start monitoring
	if err := w.monitoringService.StartMonitoring(ctx, pipelineID); err != nil {
		return errors.Wrap(err, "failed to start pipeline monitoring")
	}

	// Create monitoring alerts
	createdAlerts := make([]*PipelineAlert, 0)
	for _, alertConfig := range w.config.Monitoring.Alerts {
		alert := &PipelineAlert{
			ID:         alertConfig.ID,
			PipelineID: pipelineID,
			Name:       alertConfig.Name,
			Type:       "threshold",
			Condition:  alertConfig.Condition,
			Threshold:  alertConfig.Threshold,
			Severity:   "medium",
			Recipients: alertConfig.Recipients,
			Actions:    alertConfig.Actions,
			Enabled:    alertConfig.Enabled,
			CreatedAt:  time.Now(),
		}

		if err := w.monitoringService.CreateAlert(ctx, alert); err != nil {
			log.Printf("Failed to create alert %s: %v", alert.Name, err)
		} else {
			createdAlerts = append(createdAlerts, alert)
		}
	}

	// Create dashboard
	dashboard, err := w.monitoringService.GetDashboard(ctx, pipelineID)
	if err != nil {
		log.Printf("Failed to create dashboard: %v", err)
	}

	// Store monitoring configuration
	w.instance.Metadata["monitoring_enabled"] = true
	w.instance.Metadata["monitoring_metrics"] = w.config.Monitoring.Metrics
	w.instance.Metadata["created_alerts"] = createdAlerts
	if dashboard != nil {
		w.instance.Metadata["dashboard"] = dashboard
	}

	// Emit monitoring started event
	w.emitEvent(ctx, "pipeline.monitoring.started", map[string]interface{}{
		"pipeline_id":    pipelineID,
		"metrics_count":  len(w.config.Monitoring.Metrics),
		"alerts_count":   len(createdAlerts),
		"dashboard_created": dashboard != nil,
	})

	return nil
}

func (w *DataPipelineWorkflow) executeTestPipeline(ctx context.Context) error {
	pipelineID := w.instance.Metadata["pipeline_id"].(uuid.UUID)

	// Execute test pipeline with sample data
	testParams := map[string]interface{}{
		"test_run": true,
		"sample_size": 100,
		"dry_run": true,
	}

	execution, err := w.pipelineOrchestrator.ExecutePipeline(ctx, pipelineID, testParams)
	if err != nil {
		return errors.Wrap(err, "test pipeline execution failed")
	}

	// Wait for test execution to complete (with timeout)
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			log.Printf("Test pipeline execution timed out")
			return errors.New("test pipeline execution timed out")
		case <-ticker.C:
			// Check execution status
			currentExecution, err := w.monitoringService.GetMetrics(ctx, pipelineID, TimeRange{
				StartTime: execution.StartedAt,
				EndTime:   time.Now(),
			})
			if err != nil {
				log.Printf("Failed to get execution metrics: %v", err)
				continue
			}

			if execution.Status == "completed" || execution.Status == "failed" {
				// Test execution completed
				w.instance.Metadata["test_execution"] = execution
				w.instance.Metadata["test_metrics"] = currentExecution

				// Emit test execution completed event
				w.emitEvent(ctx, "pipeline.test_execution.completed", map[string]interface{}{
					"execution_id":      execution.ID,
					"status":            execution.Status,
					"duration":          execution.Duration,
					"records_processed": currentExecution.RecordsProcessed,
					"success_rate":      currentExecution.SuccessRate,
				})

				if execution.Status == "failed" {
					return errors.Errorf("test pipeline execution failed: %v", execution.Error)
				}

				return nil
			}
		}
	}
}

// Helper methods
func (w *DataPipelineWorkflow) executeStep(ctx context.Context, stepName string, stepFunc PipelineStepFunc) error {
	// Create step record
	step := &WorkflowStep{
		ID:           uuid.New(),
		WorkflowID:   w.instance.ID,
		Name:         stepName,
		Status:       WorkflowStatusRunning,
		StartedAt:    &time.Time{},
		MaxRetries:   3,
	}
	*step.StartedAt = time.Now()

	// Emit step started event
	w.emitEvent(ctx, EventStepStarted, map[string]interface{}{
		"step": step,
	})

	// Execute step with retry logic
	var err error
	for attempt := 0; attempt <= step.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := time.Duration(attempt*attempt) * time.Second
			time.Sleep(delay)
		}

		err = stepFunc(ctx)
		if err == nil {
			break
		}

		step.RetryCount = attempt
		if attempt < step.MaxRetries {
			w.emitEvent(ctx, EventStepRetried, map[string]interface{}{
				"step":    step,
				"attempt": attempt + 1,
				"error":   err.Error(),
			})
		}
	}

	// Update step status
	completedAt := time.Now()
	step.CompletedAt = &completedAt
	duration := completedAt.Sub(*step.StartedAt)
	step.Duration = &duration

	if err != nil {
		step.Status = WorkflowStatusFailed
		step.Error = &WorkflowError{
			Code:        "DATA_PIPELINE_STEP_ERROR",
			Message:     err.Error(),
			Timestamp:   time.Now(),
			Recoverable: w.isRecoverableError(err),
		}

		w.emitEvent(ctx, EventStepFailed, map[string]interface{}{
			"step":  step,
			"error": step.Error,
		})
	} else {
		step.Status = WorkflowStatusCompleted
		w.emitEvent(ctx, EventStepCompleted, map[string]interface{}{
			"step":     step,
			"duration": duration.String(),
		})
	}

	// Add step to workflow instance
	w.instance.Steps = append(w.instance.Steps, *step)

	return err
}

func (w *DataPipelineWorkflow) emitEvent(ctx context.Context, eventType string, data map[string]interface{}) error {
	eventData, _ := json.Marshal(data)
	
	event := &WorkflowEvent{
		ID:         uuid.New(),
		WorkflowID: w.instance.ID,
		Type:       eventType,
		Version:    1,
		Data:       eventData,
		Metadata: map[string]interface{}{
			"workflow_type": string(w.instance.Type),
			"tenant_id":     w.instance.TenantID.String(),
			"user_id":       w.instance.UserID.String(),
		},
		Timestamp: time.Now(),
		UserID:    w.instance.UserID,
		TenantID:  w.instance.TenantID,
	}

	return w.eventStore.SaveEvent(ctx, event)
}

func (w *DataPipelineWorkflow) getStepName(index int) string {
	stepNames := []string{
		"validate_configuration",
		"initialize_pipeline_orchestrator",
		"setup_data_sources",
		"configure_transformations",
		"setup_data_destinations",
		"configure_validation_rules",
		"setup_error_handling",
		"enable_checkpointing",
		"start_monitoring",
		"execute_test_pipeline",
	}
	
	if index < len(stepNames) {
		return stepNames[index]
	}
	return fmt.Sprintf("step_%d", index)
}

func (w *DataPipelineWorkflow) isRecoverableError(err error) bool {
	errorStr := err.Error()
	recoverableErrors := []string{
		"timeout",
		"rate limit",
		"temporary",
		"network",
		"connection",
		"quota",
		"unavailable",
		"throttled",
	}

	for _, recoverable := range recoverableErrors {
		if contains(errorStr, recoverable) {
			return true
		}
	}
	return false
}

func (w *DataPipelineWorkflow) getPipelineStats() map[string]interface{} {
	stats := map[string]interface{}{
		"total_steps":       len(w.instance.Steps),
		"completed_steps":   w.instance.Progress.CompletedSteps,
		"failed_steps":      w.instance.Progress.FailedSteps,
		"success_rate":      float64(w.instance.Progress.CompletedSteps) / float64(len(w.instance.Steps)),
	}

	// Add pipeline-specific stats
	if pipelineID, exists := w.instance.Metadata["pipeline_id"]; exists {
		stats["pipeline_id"] = pipelineID
	}

	if pipeline, exists := w.instance.Metadata["pipeline"]; exists {
		p := pipeline.(*Pipeline)
		stats["pipeline_type"] = p.Type
		stats["pipeline_status"] = p.Status
	}

	// Add source stats
	if sourceSchemas, exists := w.instance.Metadata["source_schemas"]; exists {
		schemaMap := sourceSchemas.(map[uuid.UUID]*DataSchema)
		stats["sources_count"] = len(w.config.Sources)
		stats["sources_with_schema"] = len(schemaMap)
	}

	if sourceEstimates, exists := w.instance.Metadata["source_estimates"]; exists {
		estimateMap := sourceEstimates.(map[uuid.UUID]*DataSizeEstimate)
		totalRecords := int64(0)
		totalSize := int64(0)
		for _, estimate := range estimateMap {
			totalRecords += estimate.EstimatedRecords
			totalSize += estimate.EstimatedSize
		}
		stats["estimated_total_records"] = totalRecords
		stats["estimated_total_size"] = totalSize
	}

	// Add transformation stats
	if compiledTransformations, exists := w.instance.Metadata["compiled_transformations"]; exists {
		transformationList := compiledTransformations.([]*CompiledTransformation)
		stats["transformations_configured"] = len(w.config.Transformations)
		stats["transformations_compiled"] = len(transformationList)
	}

	// Add destination stats
	if destinationSchemas, exists := w.instance.Metadata["destination_schemas"]; exists {
		schemaMap := destinationSchemas.(map[uuid.UUID]*DataSchema)
		stats["destinations_count"] = len(w.config.Destinations)
		stats["destinations_with_schema"] = len(schemaMap)
	}

	if bulkLoadingSupport, exists := w.instance.Metadata["bulk_loading_support"]; exists {
		supportMap := bulkLoadingSupport.(map[uuid.UUID]bool)
		bulkSupportCount := 0
		for _, supported := range supportMap {
			if supported {
				bulkSupportCount++
			}
		}
		stats["destinations_with_bulk_support"] = bulkSupportCount
	}

	// Add validation stats
	if validationEnabled, exists := w.instance.Metadata["validation_enabled"]; exists {
		stats["validation_enabled"] = validationEnabled
	}

	if validationRules, exists := w.instance.Metadata["validation_rules"]; exists {
		ruleList := validationRules.([]ValidationRule)
		stats["validation_rules_count"] = len(ruleList)
	}

	// Add monitoring stats
	if monitoringEnabled, exists := w.instance.Metadata["monitoring_enabled"]; exists {
		stats["monitoring_enabled"] = monitoringEnabled
	}

	if createdAlerts, exists := w.instance.Metadata["created_alerts"]; exists {
		alertList := createdAlerts.([]*PipelineAlert)
		stats["monitoring_alerts_count"] = len(alertList)
	}

	// Add checkpointing stats
	if checkpointingEnabled, exists := w.instance.Metadata["checkpointing_enabled"]; exists {
		stats["checkpointing_enabled"] = checkpointingEnabled
	}

	// Add test execution stats
	if testExecution, exists := w.instance.Metadata["test_execution"]; exists {
		execution := testExecution.(*PipelineExecution)
		stats["test_execution_status"] = execution.Status
		if execution.Duration != nil {
			stats["test_execution_duration"] = execution.Duration.String()
		}
	}

	if testMetrics, exists := w.instance.Metadata["test_metrics"]; exists {
		metrics := testMetrics.(*PipelineMetrics)
		stats["test_records_processed"] = metrics.RecordsProcessed
		stats["test_success_rate"] = metrics.SuccessRate
		stats["test_throughput"] = metrics.Throughput
	}

	return stats
}