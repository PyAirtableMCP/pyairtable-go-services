package workflows

import (
	"time"
	"encoding/json"
	"github.com/google/uuid"
)

// Workflow Types
type WorkflowType string

const (
	WorkflowTypeAirtableSync     WorkflowType = "airtable_sync"
	WorkflowTypeAIAnalytics      WorkflowType = "ai_analytics"
	WorkflowTypeTeamCollaboration WorkflowType = "team_collaboration"
	WorkflowTypeAutomationEngine WorkflowType = "automation_engine"
	WorkflowTypeDataPipeline     WorkflowType = "data_pipeline"
)

// Workflow Status
type WorkflowStatus string

const (
	WorkflowStatusPending    WorkflowStatus = "pending"
	WorkflowStatusRunning    WorkflowStatus = "running"
	WorkflowStatusCompleted  WorkflowStatus = "completed"
	WorkflowStatusFailed     WorkflowStatus = "failed"
	WorkflowStatusCancelled  WorkflowStatus = "cancelled"
	WorkflowStatusRetrying   WorkflowStatus = "retrying"
)

// Workflow Instance
type WorkflowInstance struct {
	ID              uuid.UUID                `json:"id"`
	Type            WorkflowType             `json:"type"`
	Status          WorkflowStatus           `json:"status"`
	TenantID        uuid.UUID                `json:"tenant_id"`
	UserID          uuid.UUID                `json:"user_id"`
	Configuration   json.RawMessage          `json:"configuration"`
	Context         map[string]interface{}   `json:"context"`
	Steps           []WorkflowStep           `json:"steps"`
	CurrentStepIndex int                     `json:"current_step_index"`
	Progress        WorkflowProgress         `json:"progress"`
	Error           *WorkflowError           `json:"error,omitempty"`
	CreatedAt       time.Time                `json:"created_at"`
	StartedAt       *time.Time               `json:"started_at,omitempty"`
	CompletedAt     *time.Time               `json:"completed_at,omitempty"`
	UpdatedAt       time.Time                `json:"updated_at"`
	Metadata        map[string]interface{}   `json:"metadata"`
}

// Workflow Step
type WorkflowStep struct {
	ID              uuid.UUID                `json:"id"`
	WorkflowID      uuid.UUID                `json:"workflow_id"`
	Name            string                   `json:"name"`
	Type            string                   `json:"type"`
	Status          WorkflowStatus           `json:"status"`
	Configuration   json.RawMessage          `json:"configuration"`
	Input           map[string]interface{}   `json:"input"`
	Output          map[string]interface{}   `json:"output"`
	Error           *WorkflowError           `json:"error,omitempty"`
	RetryCount      int                      `json:"retry_count"`
	MaxRetries      int                      `json:"max_retries"`
	StartedAt       *time.Time               `json:"started_at,omitempty"`
	CompletedAt     *time.Time               `json:"completed_at,omitempty"`
	Duration        *time.Duration           `json:"duration,omitempty"`
	Compensated     bool                     `json:"compensated"`
	CompensationData map[string]interface{} `json:"compensation_data,omitempty"`
}

// Workflow Progress
type WorkflowProgress struct {
	TotalSteps      int     `json:"total_steps"`
	CompletedSteps  int     `json:"completed_steps"`
	FailedSteps     int     `json:"failed_steps"`
	PercentComplete float64 `json:"percent_complete"`
	EstimatedTimeRemaining *time.Duration `json:"estimated_time_remaining,omitempty"`
}

// Workflow Error
type WorkflowError struct {
	Code        string            `json:"code"`
	Message     string            `json:"message"`
	Details     map[string]interface{} `json:"details,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
	Recoverable bool              `json:"recoverable"`
}

// Airtable Sync Workflow Configuration
type AirtableSyncConfig struct {
	SourceBases     []AirtableBaseConfig  `json:"source_bases"`
	TargetBases     []AirtableBaseConfig  `json:"target_bases"`
	SyncRules       []SyncRule            `json:"sync_rules"`
	ConflictResolution ConflictResolutionStrategy `json:"conflict_resolution"`
	WebhookConfig   WebhookConfig         `json:"webhook_config"`
	BatchSize       int                   `json:"batch_size"`
	RealTimeSync    bool                  `json:"real_time_sync"`
	ProgressTracking bool                 `json:"progress_tracking"`
}

type AirtableBaseConfig struct {
	BaseID      string            `json:"base_id"`
	APIKey      string            `json:"api_key"`
	Tables      []string          `json:"tables"`
	Fields      []string          `json:"fields,omitempty"`
	Filters     map[string]interface{} `json:"filters,omitempty"`
	LastSyncAt  *time.Time        `json:"last_sync_at,omitempty"`
}

type SyncRule struct {
	SourceTable string            `json:"source_table"`
	TargetTable string            `json:"target_table"`
	FieldMapping map[string]string `json:"field_mapping"`
	Transform   string            `json:"transform,omitempty"`
	Condition   string            `json:"condition,omitempty"`
}

type ConflictResolutionStrategy string

const (
	ConflictResolutionSourceWins ConflictResolutionStrategy = "source_wins"
	ConflictResolutionTargetWins ConflictResolutionStrategy = "target_wins"
	ConflictResolutionMerge      ConflictResolutionStrategy = "merge"
	ConflictResolutionManual     ConflictResolutionStrategy = "manual"
)

type WebhookConfig struct {
	Enabled     bool              `json:"enabled"`
	URL         string            `json:"url"`
	Secret      string            `json:"secret"`
	Events      []string          `json:"events"`
	Deduplication bool            `json:"deduplication"`
	RetryConfig RetryConfig       `json:"retry_config"`
}

type RetryConfig struct {
	MaxRetries      int           `json:"max_retries"`
	BackoffStrategy string        `json:"backoff_strategy"`
	InitialDelay    time.Duration `json:"initial_delay"`
	MaxDelay        time.Duration `json:"max_delay"`
}

// AI Analytics Workflow Configuration
type AIAnalyticsConfig struct {
	GeminiConfig    GeminiConfig          `json:"gemini_config"`
	DataSources     []DataSource          `json:"data_sources"`
	AnalysisTypes   []AnalysisType        `json:"analysis_types"`
	Reports         []ReportConfig        `json:"reports"`
	Alerts          []AlertConfig         `json:"alerts"`
	MLModels        []MLModelConfig       `json:"ml_models"`
	Schedule        ScheduleConfig        `json:"schedule"`
}

type GeminiConfig struct {
	APIKey          string            `json:"api_key"`
	Model           string            `json:"model"`
	Temperature     float64           `json:"temperature"`
	MaxTokens       int               `json:"max_tokens"`
	SystemPrompt    string            `json:"system_prompt"`
	SafetySettings  map[string]interface{} `json:"safety_settings"`
}

type DataSource struct {
	ID           uuid.UUID         `json:"id"`
	Type         string            `json:"type"` // airtable, postgres, api
	Config       json.RawMessage   `json:"config"`
	RefreshRate  time.Duration     `json:"refresh_rate"`
	LastRefresh  *time.Time        `json:"last_refresh,omitempty"`
}

type AnalysisType string

const (
	AnalysisTypeDescriptive  AnalysisType = "descriptive"
	AnalysisTypePredictive   AnalysisType = "predictive"
	AnalysisTypePrescriptive AnalysisType = "prescriptive"
	AnalysisTypeAnomaly      AnalysisType = "anomaly"
	AnalysisTypeNLP          AnalysisType = "nlp"
)

type ReportConfig struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Template    string            `json:"template"`
	Recipients  []string          `json:"recipients"`
	Schedule    ScheduleConfig    `json:"schedule"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type AlertConfig struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Condition   string            `json:"condition"`
	Threshold   float64           `json:"threshold"`
	Recipients  []string          `json:"recipients"`
	Actions     []AlertAction     `json:"actions"`
	Enabled     bool              `json:"enabled"`
}

type AlertAction struct {
	Type        string            `json:"type"`
	Config      json.RawMessage   `json:"config"`
}

type MLModelConfig struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Algorithm   string            `json:"algorithm"`
	Features    []string          `json:"features"`
	Target      string            `json:"target"`
	TrainingData DataSource       `json:"training_data"`
	Parameters  map[string]interface{} `json:"parameters"`
	LastTrained *time.Time        `json:"last_trained,omitempty"`
}

type ScheduleConfig struct {
	Enabled     bool              `json:"enabled"`
	CronExpr    string            `json:"cron_expr"`
	Timezone    string            `json:"timezone"`
	NextRun     *time.Time        `json:"next_run,omitempty"`
}

// Team Collaboration Workflow Configuration
type TeamCollaborationConfig struct {
	WorkspaceID     uuid.UUID         `json:"workspace_id"`
	RBACConfig      RBACConfig        `json:"rbac_config"`
	CollaborationFeatures CollaborationFeatures `json:"collaboration_features"`
	AuditConfig     AuditConfig       `json:"audit_config"`
	NotificationConfig NotificationConfig `json:"notification_config"`
}

type RBACConfig struct {
	Roles       []Role            `json:"roles"`
	Permissions []Permission      `json:"permissions"`
	Inheritance bool              `json:"inheritance"`
	DefaultRole string            `json:"default_role"`
}

type Role struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Permissions []uuid.UUID       `json:"permissions"`
	Inherits    []uuid.UUID       `json:"inherits,omitempty"`
}

type Permission struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Resource    string            `json:"resource"`
	Action      string            `json:"action"`
	Conditions  map[string]interface{} `json:"conditions,omitempty"`
}

type CollaborationFeatures struct {
	RealTimeEditing bool              `json:"real_time_editing"`
	Comments        bool              `json:"comments"`
	Mentions        bool              `json:"mentions"`
	ChangeTracking  bool              `json:"change_tracking"`
	VersionControl  bool              `json:"version_control"`
	Sharing         SharingConfig     `json:"sharing"`
}

type SharingConfig struct {
	Public          bool              `json:"public"`
	LinkSharing     bool              `json:"link_sharing"`
	ExternalSharing bool              `json:"external_sharing"`
	ExpirationTime  *time.Duration    `json:"expiration_time,omitempty"`
}

type AuditConfig struct {
	Enabled         bool              `json:"enabled"`
	RetentionPeriod time.Duration     `json:"retention_period"`
	Events          []string          `json:"events"`
	Storage         string            `json:"storage"`
}

type NotificationConfig struct {
	Channels        []NotificationChannel `json:"channels"`
	Templates       map[string]string     `json:"templates"`
	Preferences     map[string]interface{} `json:"preferences"`
}

type NotificationChannel struct {
	Type            string            `json:"type"`
	Config          json.RawMessage   `json:"config"`
	Enabled         bool              `json:"enabled"`
}

// Automation Engine Workflow Configuration
type AutomationEngineConfig struct {
	WorkflowBuilder WorkflowBuilderConfig `json:"workflow_builder"`
	Triggers        []TriggerConfig       `json:"triggers"`
	Actions         []ActionConfig        `json:"actions"`
	Conditions      []ConditionConfig     `json:"conditions"`
	Integrations    []IntegrationConfig   `json:"integrations"`
	ExecutionConfig ExecutionConfig       `json:"execution_config"`
}

type WorkflowBuilderConfig struct {
	UI              UIConfig          `json:"ui"`
	Templates       []WorkflowTemplate `json:"templates"`
	CustomActions   bool              `json:"custom_actions"`
	Variables       bool              `json:"variables"`
	ErrorHandling   bool              `json:"error_handling"`
}

type UIConfig struct {
	Theme           string            `json:"theme"`
	Layout          string            `json:"layout"`
	Components      []string          `json:"components"`
}

type WorkflowTemplate struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Category    string            `json:"category"`
	Description string            `json:"description"`
	Config      json.RawMessage   `json:"config"`
	Popular     bool              `json:"popular"`
}

type TriggerConfig struct {
	ID          uuid.UUID         `json:"id"`
	Type        string            `json:"type"` // time, event, condition, webhook
	Config      json.RawMessage   `json:"config"`
	Enabled     bool              `json:"enabled"`
}

type ActionConfig struct {
	ID          uuid.UUID         `json:"id"`
	Type        string            `json:"type"`
	Service     string            `json:"service"`
	Method      string            `json:"method"`
	Config      json.RawMessage   `json:"config"`
	Timeout     time.Duration     `json:"timeout"`
}

type ConditionConfig struct {
	ID          uuid.UUID         `json:"id"`
	Expression  string            `json:"expression"`
	Variables   map[string]interface{} `json:"variables"`
}

type IntegrationConfig struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Auth        AuthConfig        `json:"auth"`
	Endpoints   []EndpointConfig  `json:"endpoints"`
	RateLimit   RateLimitConfig   `json:"rate_limit"`
}

type AuthConfig struct {
	Type        string            `json:"type"`
	Config      json.RawMessage   `json:"config"`
}

type EndpointConfig struct {
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	Method      string            `json:"method"`
	Headers     map[string]string `json:"headers"`
}

type RateLimitConfig struct {
	Requests    int               `json:"requests"`
	Period      time.Duration     `json:"period"`
	Burst       int               `json:"burst"`
}

type ExecutionConfig struct {
	MaxConcurrent   int               `json:"max_concurrent"`
	Timeout         time.Duration     `json:"timeout"`
	RetryPolicy     RetryConfig       `json:"retry_policy"`
	Monitoring      bool              `json:"monitoring"`
}

// Data Pipeline Workflow Configuration
type DataPipelineConfig struct {
	Pipeline        PipelineConfig    `json:"pipeline"`
	Sources         []DataSource      `json:"sources"`
	Destinations    []DataDestination `json:"destinations"`
	Transformations []Transformation  `json:"transformations"`
	Validation      ValidationConfig  `json:"validation"`
	Monitoring      MonitoringConfig  `json:"monitoring"`
	ErrorHandling   ErrorHandlingConfig `json:"error_handling"`
}

type PipelineConfig struct {
	Type            string            `json:"type"` // ETL, ELT, streaming
	Mode            string            `json:"mode"` // batch, streaming, incremental
	Schedule        ScheduleConfig    `json:"schedule"`
	Parallelism     int               `json:"parallelism"`
	CheckpointConfig CheckpointConfig `json:"checkpoint_config"`
}

type DataDestination struct {
	ID          uuid.UUID         `json:"id"`
	Type        string            `json:"type"`
	Config      json.RawMessage   `json:"config"`
	WriteMode   string            `json:"write_mode"` // append, overwrite, upsert
}

type Transformation struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"` // map, filter, aggregate, join
	Config      json.RawMessage   `json:"config"`
	Order       int               `json:"order"`
}

type ValidationConfig struct {
	Enabled     bool              `json:"enabled"`
	Rules       []ValidationRule  `json:"rules"`
	OnFailure   string            `json:"on_failure"` // stop, skip, log
}

type ValidationRule struct {
	ID          uuid.UUID         `json:"id"`
	Field       string            `json:"field"`
	Type        string            `json:"type"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type MonitoringConfig struct {
	Metrics     []MetricConfig    `json:"metrics"`
	Alerts      []AlertConfig     `json:"alerts"`
	Dashboard   DashboardConfig   `json:"dashboard"`
}

type MetricConfig struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Aggregation string            `json:"aggregation"`
}

type DashboardConfig struct {
	Enabled     bool              `json:"enabled"`
	Widgets     []string          `json:"widgets"`
	RefreshRate time.Duration     `json:"refresh_rate"`
}

type ErrorHandlingConfig struct {
	Strategy    string            `json:"strategy"` // retry, skip, stop, dlq
	RetryConfig RetryConfig       `json:"retry_config"`
	DLQConfig   DLQConfig         `json:"dlq_config"`
}

type DLQConfig struct {
	Enabled     bool              `json:"enabled"`
	MaxSize     int               `json:"max_size"`
	TTL         time.Duration     `json:"ttl"`
}

type CheckpointConfig struct {
	Enabled     bool              `json:"enabled"`
	Interval    time.Duration     `json:"interval"`
	Storage     string            `json:"storage"`
}

// Workflow Events for Event Sourcing
type WorkflowEvent struct {
	ID          uuid.UUID         `json:"id"`
	WorkflowID  uuid.UUID         `json:"workflow_id"`
	Type        string            `json:"type"`
	Version     int               `json:"version"`
	Data        json.RawMessage   `json:"data"`
	Metadata    map[string]interface{} `json:"metadata"`
	Timestamp   time.Time         `json:"timestamp"`
	UserID      uuid.UUID         `json:"user_id"`
	TenantID    uuid.UUID         `json:"tenant_id"`
}

// Event Types
const (
	EventWorkflowCreated    = "workflow.created"
	EventWorkflowStarted    = "workflow.started"
	EventWorkflowCompleted  = "workflow.completed"
	EventWorkflowFailed     = "workflow.failed"
	EventWorkflowCancelled  = "workflow.cancelled"
	EventStepStarted        = "workflow.step.started"
	EventStepCompleted      = "workflow.step.completed"
	EventStepFailed         = "workflow.step.failed"
	EventStepRetried        = "workflow.step.retried"
	EventStepCompensated    = "workflow.step.compensated"
)