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

// SunsetMonitoringService monitors version sunset and client migration
type SunsetMonitoringService struct {
	analytics      *VersionAnalyticsService
	lifecycle      *VersionLifecycleManager
	notifications  *NotificationService
	clientTracker  *ClientTracker
	migrationPlan  *MigrationPlanManager
	config         *SunsetMonitoringConfig
	mu             sync.RWMutex
}

// SunsetMonitoringConfig configures sunset monitoring
type SunsetMonitoringConfig struct {
	Enabled                    bool          `json:"enabled"`
	MonitoringInterval         time.Duration `json:"monitoring_interval"`
	UsageThresholdForWarning   int64         `json:"usage_threshold_for_warning"`   // Daily requests to trigger warning
	ClientMigrationTracking    bool          `json:"client_migration_tracking"`
	AutomatedNotifications     bool          `json:"automated_notifications"`
	GracePeriodEnforcement     bool          `json:"grace_period_enforcement"`
	PreSunsetAnalyisis         bool          `json:"pre_sunset_analysis"`
	ImpactAnalysisThreshold    float64       `json:"impact_analysis_threshold"`     // Percentage of traffic to analyze
	ClientSegmentation         bool          `json:"client_segmentation"`
	RollbackCapability         bool          `json:"rollback_capability"`
}

// ClientTracker tracks individual clients and their version usage
type ClientTracker struct {
	clients        map[string]*ClientProfile
	versionUsage   map[string]map[string]*ClientVersionUsage // client -> version -> usage
	migrationStatus map[string]*ClientMigrationStatus        // client -> migration status
	mu             sync.RWMutex
}

// ClientProfile contains information about a client
type ClientProfile struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Type            ClientType             `json:"type"`
	Tier            ClientTier             `json:"tier"`
	Contact         *ContactInfo           `json:"contact"`
	CurrentVersions []string               `json:"current_versions"`
	PreferredVersion string                `json:"preferred_version"`
	LastSeen        time.Time              `json:"last_seen"`
	FirstSeen       time.Time              `json:"first_seen"`
	TotalRequests   int64                  `json:"total_requests"`
	Configuration   *ClientConfiguration   `json:"configuration"`
	Metadata        map[string]interface{} `json:"metadata"`
	Tags            []string               `json:"tags"`
	Status          ClientStatus           `json:"status"`
}

// ClientType represents the type of client
type ClientType string

const (
	ClientTypeWebApp      ClientType = "web_app"
	ClientTypeMobileApp   ClientType = "mobile_app"
	ClientTypeBackend     ClientType = "backend_service"
	ClientTypeIntegration ClientType = "integration"
	ClientTypeSDK         ClientType = "sdk"
	ClientTypeScript      ClientType = "script"
	ClientTypeThirdParty  ClientType = "third_party"
)

// ClientTier represents the client tier (for prioritization)
type ClientTier string

const (
	ClientTierCritical    ClientTier = "critical"     // Mission-critical clients
	ClientTierEnterprise  ClientTier = "enterprise"   // Enterprise clients
	ClientTierProfessional ClientTier = "professional" // Professional clients
	ClientTierStandard    ClientTier = "standard"     // Standard clients
	ClientTierCommunity   ClientTier = "community"    // Community/open source
)

// ClientStatus represents client status
type ClientStatus string

const (
	ClientStatusActive    ClientStatus = "active"
	ClientStatusInactive  ClientStatus = "inactive"
	ClientStatusMigrating ClientStatus = "migrating"
	ClientStatusBlocked   ClientStatus = "blocked"
	ClientStatusSunset    ClientStatus = "sunset"
)

// ContactInfo contains client contact information
type ContactInfo struct {
	Email       string   `json:"email"`
	Name        string   `json:"name"`
	Company     string   `json:"company"`
	Phone       string   `json:"phone"`
	SlackChannel string  `json:"slack_channel"`
	Teams       []string `json:"teams"`
	Timezone    string   `json:"timezone"`
}

// ClientConfiguration contains client-specific configuration
type ClientConfiguration struct {
	AllowedVersions    []string          `json:"allowed_versions"`
	MigrationSchedule  *MigrationSchedule `json:"migration_schedule"`
	NotificationPrefs  *NotificationPrefs `json:"notification_prefs"`
	GracePeriodOverride *time.Duration   `json:"grace_period_override"`
	AutoUpgrade        bool              `json:"auto_upgrade"`
	TestingEnvironment string            `json:"testing_environment"`
}

// MigrationSchedule defines when a client should migrate
type MigrationSchedule struct {
	StartDate      time.Time           `json:"start_date"`
	TargetDate     time.Time           `json:"target_date"`
	Milestones     []MigrationMilestone `json:"milestones"`
	Flexibility    ScheduleFlexibility  `json:"flexibility"`
	Dependencies   []string            `json:"dependencies"`
	Resources      []string            `json:"resources"`
}

// MigrationMilestone represents a migration milestone
type MigrationMilestone struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	DueDate     time.Time `json:"due_date"`
	Completed   bool      `json:"completed"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Deliverables []string `json:"deliverables"`
}

// ScheduleFlexibility indicates how flexible the migration schedule is
type ScheduleFlexibility string

const (
	FlexibilityRigid     ScheduleFlexibility = "rigid"      // No flexibility
	FlexibilityLimited   ScheduleFlexibility = "limited"    // Some flexibility
	FlexibilityModerate  ScheduleFlexibility = "moderate"   // Moderate flexibility
	FlexibilityFlexible  ScheduleFlexibility = "flexible"   // High flexibility
)

// NotificationPrefs defines client notification preferences
type NotificationPrefs struct {
	Email    bool     `json:"email"`
	Slack    bool     `json:"slack"`
	Webhook  bool     `json:"webhook"`
	InApp    bool     `json:"in_app"`
	Frequency string  `json:"frequency"`  // immediate, daily, weekly
	Channels []string `json:"channels"`
}

// ClientVersionUsage tracks a client's usage of a specific version
type ClientVersionUsage struct {
	ClientID      string    `json:"client_id"`
	Version       string    `json:"version"`
	FirstUsed     time.Time `json:"first_used"`
	LastUsed      time.Time `json:"last_used"`
	RequestCount  int64     `json:"request_count"`
	ErrorCount    int64     `json:"error_count"`
	DataTransfer  int64     `json:"data_transfer"`
	Endpoints     map[string]int64 `json:"endpoints"`  // endpoint -> request count
	Patterns      []UsagePattern   `json:"patterns"`
	Trend         UsageTrend       `json:"trend"`
}

// UsagePattern represents a usage pattern
type UsagePattern struct {
	Type        PatternType `json:"type"`
	Description string      `json:"description"`
	Frequency   string      `json:"frequency"`   // hourly, daily, weekly
	Intensity   string      `json:"intensity"`   // low, medium, high
	Endpoints   []string    `json:"endpoints"`
}

// PatternType defines types of usage patterns
type PatternType string

const (
	PatternBatch       PatternType = "batch"        // Batch processing
	PatternRealTime    PatternType = "realtime"     // Real-time usage
	PatternPeriodic    PatternType = "periodic"     // Periodic usage
	PatternBurst       PatternType = "burst"        // Burst usage
	PatternBackground  PatternType = "background"   // Background usage
)

// UsageTrend indicates usage trend
type UsageTrend string

const (
	TrendIncreasing UsageTrend = "increasing"
	TrendStable     UsageTrend = "stable"
	TrendDecreasing UsageTrend = "decreasing"
	TrendErratic    UsageTrend = "erratic"
)

// ClientMigrationStatus tracks client migration progress
type ClientMigrationStatus struct {
	ClientID         string               `json:"client_id"`
	FromVersion      string               `json:"from_version"`
	ToVersion        string               `json:"to_version"`
	Status           MigrationStatus      `json:"status"`
	StartedAt        time.Time            `json:"started_at"`
	CompletedAt      *time.Time           `json:"completed_at,omitempty"`
	Progress         float64              `json:"progress"`        // 0.0 to 1.0
	CurrentPhase     MigrationPhase       `json:"current_phase"`
	Phases           []MigrationPhaseStatus `json:"phases"`
	Issues           []MigrationIssue     `json:"issues"`
	Rollbacks        []MigrationRollback  `json:"rollbacks"`
	AssignedTo       string               `json:"assigned_to"`
	NextCheckIn      time.Time            `json:"next_check_in"`
	Notes            []MigrationNote      `json:"notes"`
}

// MigrationStatus represents migration status
type MigrationStatus string

const (
	MigrationStatusNotStarted MigrationStatus = "not_started"
	MigrationStatusInProgress MigrationStatus = "in_progress"
	MigrationStatusCompleted  MigrationStatus = "completed"
	MigrationStatusStalled    MigrationStatus = "stalled"
	MigrationStatusFailed     MigrationStatus = "failed"
	MigrationStatusRolledBack MigrationStatus = "rolled_back"
)

// MigrationPhase represents phases of migration
type MigrationPhase string

const (
	PhaseAssessment    MigrationPhase = "assessment"
	PhasePlanning      MigrationPhase = "planning"
	PhaseDevelopment   MigrationPhase = "development"
	PhaseTesting       MigrationPhase = "testing"
	PhaseDeployment    MigrationPhase = "deployment"
	PhaseValidation    MigrationPhase = "validation"
	PhaseCleanup       MigrationPhase = "cleanup"
)

// MigrationPhaseStatus tracks the status of a migration phase
type MigrationPhaseStatus struct {
	Phase       MigrationPhase `json:"phase"`
	Status      string         `json:"status"`    // pending, active, completed, failed
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Duration    time.Duration  `json:"duration"`
	Tasks       []MigrationTask `json:"tasks"`
	Blockers    []string       `json:"blockers"`
}

// MigrationTask represents a task within a migration phase
type MigrationTask struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Status      string     `json:"status"`     // pending, active, completed, failed
	AssignedTo  string     `json:"assigned_to"`
	DueDate     time.Time  `json:"due_date"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Dependencies []string  `json:"dependencies"`
	Effort      string     `json:"effort"`     // hours, days, weeks
}

// MigrationIssue represents an issue during migration
type MigrationIssue struct {
	ID          string                 `json:"id"`
	Type        IssueType              `json:"type"`
	Severity    IssueSeverity          `json:"severity"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	ReportedAt  time.Time              `json:"reported_at"`
	ReportedBy  string                 `json:"reported_by"`
	ResolvedAt  *time.Time             `json:"resolved_at,omitempty"`
	ResolvedBy  string                 `json:"resolved_by,omitempty"`
	Resolution  string                 `json:"resolution,omitempty"`
	Impact      string                 `json:"impact"`
	Workaround  string                 `json:"workaround,omitempty"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// IssueType represents types of migration issues
type IssueType string

const (
	IssueTypeBreaking     IssueType = "breaking_change"
	IssueTypeData         IssueType = "data_migration"
	IssueTypePerformance  IssueType = "performance"
	IssueTypeCompatibility IssueType = "compatibility"
	IssueTypeConfiguration IssueType = "configuration"
	IssueTypeDocumentation IssueType = "documentation"
	IssueTypeSupport      IssueType = "support"
)

// IssueSeverity represents issue severity
type IssueSeverity string

const (
	SeverityLow      IssueSeverity = "low"
	SeverityMedium   IssueSeverity = "medium"
	SeverityHigh     IssueSeverity = "high"
	SeverityCritical IssueSeverity = "critical"
	SeverityBlocking IssueSeverity = "blocking"
)

// MigrationRollback represents a migration rollback
type MigrationRollback struct {
	ID         string    `json:"id"`
	Reason     string    `json:"reason"`
	RolledBackAt time.Time `json:"rolled_back_at"`
	RolledBackBy string   `json:"rolled_back_by"`
	FromVersion  string   `json:"from_version"`
	ToVersion    string   `json:"to_version"`
	Issues       []string `json:"issues"`      // Issue IDs that triggered rollback
	Resolution   string   `json:"resolution"`
}

// MigrationNote represents a note about migration progress
type MigrationNote struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	Type      string    `json:"type"`       // update, issue, resolution, etc.
	Tags      []string  `json:"tags"`
}

// MigrationPlanManager manages migration plans and execution
type MigrationPlanManager struct {
	plans        map[string]*MigrationPlan
	templates    map[string]*MigrationPlanTemplate
	executors    map[string]MigrationExecutor
	mu           sync.RWMutex
}

// MigrationPlan represents a comprehensive migration plan
type MigrationPlan struct {
	ID              string                    `json:"id"`
	Name            string                    `json:"name"`
	Description     string                    `json:"description"`
	FromVersion     string                    `json:"from_version"`
	ToVersion       string                    `json:"to_version"`
	TargetClients   []string                  `json:"target_clients"`
	Strategy        MigrationStrategy         `json:"strategy"`
	Phases          []PlanPhase               `json:"phases"`
	Timeline        *MigrationTimeline        `json:"timeline"`
	RiskAssessment  *RiskAssessment           `json:"risk_assessment"`
	Success         *SuccessCriteria          `json:"success_criteria"`
	Communication   *CommunicationPlan        `json:"communication_plan"`
	Rollback        *RollbackPlan             `json:"rollback_plan"`
	Resources       []Resource                `json:"resources"`
	Dependencies    []Dependency              `json:"dependencies"`
	Approval        *ApprovalProcess          `json:"approval_process"`
	CreatedAt       time.Time                 `json:"created_at"`
	CreatedBy       string                    `json:"created_by"`
	Status          PlanStatus                `json:"status"`
	Metadata        map[string]interface{}    `json:"metadata"`
}

// MigrationStrategy defines the approach for migration
type MigrationStrategy string

const (
	StrategyBigBang      MigrationStrategy = "big_bang"       // All at once
	StrategyPhased       MigrationStrategy = "phased"         // Gradual phases
	StrategyCanary       MigrationStrategy = "canary"         // Canary deployment
	StrategyBlueGreen    MigrationStrategy = "blue_green"     // Blue-green deployment
	StrategyRolling      MigrationStrategy = "rolling"        // Rolling deployment
	StrategyClientDriven MigrationStrategy = "client_driven"  // Client-driven migration
)

// PlanPhase represents a phase in the migration plan
type PlanPhase struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Duration    time.Duration `json:"duration"`
	Objectives  []string      `json:"objectives"`
	Activities  []Activity    `json:"activities"`
	Dependencies []string     `json:"dependencies"`
	RiskLevel   string        `json:"risk_level"`
	GoNoGoGates []string      `json:"go_no_go_gates"`
}

// Activity represents an activity within a migration phase
type Activity struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Owner       string        `json:"owner"`
	Duration    time.Duration `json:"duration"`
	Type        ActivityType  `json:"type"`
	Prerequisites []string    `json:"prerequisites"`
	Deliverables []string     `json:"deliverables"`
}

// ActivityType represents types of migration activities
type ActivityType string

const (
	ActivityTypeAnalysis      ActivityType = "analysis"
	ActivityTypeDevelopment   ActivityType = "development"
	ActivityTypeTesting       ActivityType = "testing"
	ActivityTypeDeployment    ActivityType = "deployment"
	ActivityTypeCommunication ActivityType = "communication"
	ActivityTypeValidation    ActivityType = "validation"
	ActivityTypeRollback      ActivityType = "rollback"
)

// MigrationTimeline defines the timeline for migration
type MigrationTimeline struct {
	StartDate    time.Time              `json:"start_date"`
	EndDate      time.Time              `json:"end_date"`
	Milestones   []TimelineMilestone    `json:"milestones"`
	Buffer       time.Duration          `json:"buffer"`         // Buffer time
	Constraints  []TimelineConstraint   `json:"constraints"`
}

// TimelineMilestone represents a timeline milestone
type TimelineMilestone struct {
	Name        string    `json:"name"`
	Date        time.Time `json:"date"`
	Description string    `json:"description"`
	Critical    bool      `json:"critical"`
	Dependencies []string `json:"dependencies"`
}

// TimelineConstraint represents a timeline constraint
type TimelineConstraint struct {
	Type        string    `json:"type"`        // blackout, dependency, resource
	Description string    `json:"description"`
	StartDate   time.Time `json:"start_date"`
	EndDate     time.Time `json:"end_date"`
	Impact      string    `json:"impact"`
}

// RiskAssessment assesses migration risks
type RiskAssessment struct {
	OverallRisk    RiskLevel    `json:"overall_risk"`
	Risks          []Risk       `json:"risks"`
	Mitigations    []Mitigation `json:"mitigations"`
	Assumptions    []string     `json:"assumptions"`
	Dependencies   []string     `json:"dependencies"`
	AssessedBy     string       `json:"assessed_by"`
	AssessedAt     time.Time    `json:"assessed_at"`
}

// RiskLevel represents risk levels
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

// Risk represents a migration risk
type Risk struct {
	ID          string    `json:"id"`
	Category    string    `json:"category"`     // technical, business, operational
	Description string    `json:"description"`
	Impact      RiskLevel `json:"impact"`
	Probability string    `json:"probability"`  // low, medium, high
	Owner       string    `json:"owner"`
	Mitigation  string    `json:"mitigation"`
}

// Mitigation represents a risk mitigation
type Mitigation struct {
	RiskID      string `json:"risk_id"`
	Description string `json:"description"`
	Action      string `json:"action"`
	Owner       string `json:"owner"`
	DueDate     time.Time `json:"due_date"`
	Status      string `json:"status"`
}

// SuccessCriteria defines success criteria for migration
type SuccessCriteria struct {
	Metrics     []SuccessMetric `json:"metrics"`
	Thresholds  []Threshold     `json:"thresholds"`
	Validation  []string        `json:"validation"`
	Acceptance  []string        `json:"acceptance"`
}

// SuccessMetric defines a success metric
type SuccessMetric struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Target      string `json:"target"`
	Measurement string `json:"measurement"`
	Baseline    string `json:"baseline"`
}

// Threshold defines a success threshold
type Threshold struct {
	Metric    string  `json:"metric"`
	Operator  string  `json:"operator"`  // <, >, <=, >=, =
	Value     float64 `json:"value"`
	Critical  bool    `json:"critical"`
}

// CommunicationPlan defines communication during migration
type CommunicationPlan struct {
	Stakeholders []Stakeholder      `json:"stakeholders"`
	Channels     []CommChannel      `json:"channels"`
	Schedule     []CommScheduleItem `json:"schedule"`
	Templates    []CommTemplate     `json:"templates"`
	Escalation   []EscalationPath   `json:"escalation"`
}

// Stakeholder represents a migration stakeholder
type Stakeholder struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Role     string   `json:"role"`
	Interest string   `json:"interest"`  // high, medium, low
	Influence string  `json:"influence"` // high, medium, low
	Contact  []string `json:"contact"`
}

// CommChannel represents a communication channel
type CommChannel struct {
	Type        string   `json:"type"`         // email, slack, dashboard, etc.
	Audience    []string `json:"audience"`     // stakeholder IDs
	Frequency   string   `json:"frequency"`
	Content     string   `json:"content"`
	Responsible string   `json:"responsible"`
}

// CommScheduleItem represents a scheduled communication
type CommScheduleItem struct {
	Date        time.Time `json:"date"`
	Type        string    `json:"type"`
	Audience    []string  `json:"audience"`
	Message     string    `json:"message"`
	Channel     string    `json:"channel"`
	Responsible string    `json:"responsible"`
}

// CommTemplate represents a communication template
type CommTemplate struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Subject  string `json:"subject"`
	Content  string `json:"content"`
	Variables []string `json:"variables"`
}

// EscalationPath defines escalation procedures
type EscalationPath struct {
	Level       int      `json:"level"`
	Condition   string   `json:"condition"`
	Contacts    []string `json:"contacts"`
	Actions     []string `json:"actions"`
	Timeframe   time.Duration `json:"timeframe"`
}

// RollbackPlan defines rollback procedures
type RollbackPlan struct {
	Triggers    []RollbackTrigger `json:"triggers"`
	Procedures  []RollbackProcedure `json:"procedures"`
	Approval    []string          `json:"approval"`    // Who can approve rollback
	Recovery    *RecoveryPlan     `json:"recovery"`
	Testing     []string          `json:"testing"`
}

// RollbackTrigger defines when to trigger rollback
type RollbackTrigger struct {
	ID        string `json:"id"`
	Condition string `json:"condition"`
	Threshold string `json:"threshold"`
	Automatic bool   `json:"automatic"`
}

// RollbackProcedure defines rollback procedures
type RollbackProcedure struct {
	Step        int      `json:"step"`
	Description string   `json:"description"`
	Action      string   `json:"action"`
	Validation  []string `json:"validation"`
	Rollback    string   `json:"rollback"`    // What to rollback
	Owner       string   `json:"owner"`
}

// RecoveryPlan defines recovery after rollback
type RecoveryPlan struct {
	Analysis    []string `json:"analysis"`    // What to analyze
	Fixes       []string `json:"fixes"`       // What to fix
	Retesting   []string `json:"retesting"`   // What to retest
	Timeline    string   `json:"timeline"`    // Recovery timeline
}

// Resource represents a migration resource
type Resource struct {
	Type        string `json:"type"`         // human, system, tool
	Name        string `json:"name"`
	Description string `json:"description"`
	Allocation  string `json:"allocation"`   // % or hours
	Period      string `json:"period"`       // Duration of allocation
	Cost        string `json:"cost"`
}

// Dependency represents a migration dependency
type Dependency struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`         // external, internal, technical
	Description string `json:"description"`
	Owner       string `json:"owner"`
	Status      string `json:"status"`
	DueDate     time.Time `json:"due_date"`
	Critical    bool   `json:"critical"`
}

// ApprovalProcess defines the approval process for migration
type ApprovalProcess struct {
	Required   bool              `json:"required"`
	Approvers  []Approver        `json:"approvers"`
	Criteria   []ApprovalCriteria `json:"criteria"`
	Process    []ApprovalStep    `json:"process"`
}

// Approver represents an approver
type Approver struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Level    int    `json:"level"`
	Required bool   `json:"required"`
}

// ApprovalCriteria defines approval criteria
type ApprovalCriteria struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`    // technical, business, operational
	Weight      int    `json:"weight"`
}

// ApprovalStep represents an approval step
type ApprovalStep struct {
	Step        int      `json:"step"`
	Name        string   `json:"name"`
	Approvers   []string `json:"approvers"`
	Criteria    []string `json:"criteria"`
	Parallel    bool     `json:"parallel"`    // Can be done in parallel
	Required    bool     `json:"required"`
}

// PlanStatus represents migration plan status
type PlanStatus string

const (
	PlanStatusDraft     PlanStatus = "draft"
	PlanStatusReview    PlanStatus = "review"
	PlanStatusApproved  PlanStatus = "approved"
	PlanStatusActive    PlanStatus = "active"
	PlanStatusCompleted PlanStatus = "completed"
	PlanStatusCancelled PlanStatus = "cancelled"
	PlanStatusFailed    PlanStatus = "failed"
)

// MigrationPlanTemplate provides templates for migration plans
type MigrationPlanTemplate struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	Phases      []PlanPhase            `json:"phases"`
	Variables   map[string]interface{} `json:"variables"`
	Defaults    map[string]interface{} `json:"defaults"`
}

// MigrationExecutor interface for executing migrations
type MigrationExecutor interface {
	Execute(ctx context.Context, plan *MigrationPlan) error
	GetStatus(planID string) (*MigrationStatus, error)
	Pause(planID string) error
	Resume(planID string) error
	Rollback(planID string, reason string) error
}

// ABTestingService provides A/B testing across API versions
type ABTestingService struct {
	experiments    map[string]*Experiment
	variants       map[string]*VariantConfig
	assignments    map[string]string        // user/client -> variant
	analytics      *ExperimentAnalytics
	config         *ABTestingConfig
	mu             sync.RWMutex
}

// ABTestingConfig configures A/B testing
type ABTestingConfig struct {
	Enabled            bool          `json:"enabled"`
	DefaultTrafficSplit float64      `json:"default_traffic_split"`
	MinimumSampleSize   int64        `json:"minimum_sample_size"`
	ConfidenceLevel     float64      `json:"confidence_level"`
	ExperimentDuration  time.Duration `json:"experiment_duration"`
	PowerAnalysis       bool         `json:"power_analysis"`
	SequentialTesting   bool         `json:"sequential_testing"`
}

// Experiment represents an A/B test experiment
type Experiment struct {
	ID              string                    `json:"id"`
	Name            string                    `json:"name"`
	Description     string                    `json:"description"`
	Hypothesis      string                    `json:"hypothesis"`
	Versions        []string                  `json:"versions"`
	Variants        map[string]*VariantConfig `json:"variants"`
	TrafficSplit    map[string]float64        `json:"traffic_split"`  // variant -> percentage
	TargetMetrics   []ExperimentMetric        `json:"target_metrics"`
	Segments        []ExperimentSegment       `json:"segments"`
	StartDate       time.Time                 `json:"start_date"`
	EndDate         time.Time                 `json:"end_date"`
	Status          ExperimentStatus          `json:"status"`
	Results         *ExperimentResults        `json:"results"`
	Configuration   *ExperimentConfiguration  `json:"configuration"`
	CreatedBy       string                    `json:"created_by"`
	CreatedAt       time.Time                 `json:"created_at"`
}

// VariantConfig configures an experiment variant
type VariantConfig struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Version     string                 `json:"version"`
	Features    map[string]interface{} `json:"features"`
	Config      map[string]interface{} `json:"config"`
	Weight      float64                `json:"weight"`
}

// ExperimentMetric defines metrics to track
type ExperimentMetric struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"`        // conversion, value, count
	Goal        string  `json:"goal"`        // increase, decrease, maintain
	Baseline    float64 `json:"baseline"`
	Target      float64 `json:"target"`
	Significance float64 `json:"significance"`
	Primary     bool    `json:"primary"`
}

// ExperimentSegment defines user segments for experiments
type ExperimentSegment struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Criteria    map[string]interface{} `json:"criteria"`
	Percentage  float64                `json:"percentage"`
}

// ExperimentStatus represents experiment status
type ExperimentStatus string

const (
	ExperimentStatusDraft     ExperimentStatus = "draft"
	ExperimentStatusRunning   ExperimentStatus = "running"
	ExperimentStatusPaused    ExperimentStatus = "paused"
	ExperimentStatusCompleted ExperimentStatus = "completed"
	ExperimentStatusStopped   ExperimentStatus = "stopped"
)

// ExperimentResults contains experiment results
type ExperimentResults struct {
	Summary     *ResultsSummary            `json:"summary"`
	Variants    map[string]*VariantResults `json:"variants"`
	Statistical *StatisticalResults        `json:"statistical"`
	Insights    []Insight                  `json:"insights"`
	GeneratedAt time.Time                  `json:"generated_at"`
}

// ResultsSummary provides overall results summary
type ResultsSummary struct {
	Winner        string    `json:"winner"`
	Confidence    float64   `json:"confidence"`
	Significance  bool      `json:"significance"`
	Effect        float64   `json:"effect"`
	Recommendation string   `json:"recommendation"`
	CompletedAt   time.Time `json:"completed_at"`
}

// VariantResults contains results for a specific variant
type VariantResults struct {
	VariantID     string                    `json:"variant_id"`
	Participants  int64                     `json:"participants"`
	Conversions   int64                     `json:"conversions"`
	ConversionRate float64                  `json:"conversion_rate"`
	Metrics       map[string]*MetricResult  `json:"metrics"`
	Confidence    *ConfidenceInterval       `json:"confidence"`
}

// MetricResult contains results for a specific metric
type MetricResult struct {
	Metric      string  `json:"metric"`
	Value       float64 `json:"value"`
	Baseline    float64 `json:"baseline"`
	Change      float64 `json:"change"`
	Improvement float64 `json:"improvement"`
	Significance bool   `json:"significance"`
}

// ConfidenceInterval represents a confidence interval
type ConfidenceInterval struct {
	Lower  float64 `json:"lower"`
	Upper  float64 `json:"upper"`
	Level  float64 `json:"level"`
}

// StatisticalResults contains statistical analysis
type StatisticalResults struct {
	PValue      float64 `json:"p_value"`
	ChiSquare   float64 `json:"chi_square"`
	DegreesOfFreedom int `json:"degrees_of_freedom"`
	Effect      float64 `json:"effect"`
	Power       float64 `json:"power"`
	SampleSize  int64   `json:"sample_size"`
}

// Insight represents an experiment insight
type Insight struct {
	Type        string    `json:"type"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Severity    string    `json:"severity"`
	Action      string    `json:"action"`
	GeneratedAt time.Time `json:"generated_at"`
}

// ExperimentConfiguration configures experiment behavior
type ExperimentConfiguration struct {
	SampleSize        int64   `json:"sample_size"`
	PowerLevel        float64 `json:"power_level"`
	AlphaLevel        float64 `json:"alpha_level"`
	MinimumEffect     float64 `json:"minimum_effect"`
	EarlyStoppingRule string  `json:"early_stopping_rule"`
	Randomization     string  `json:"randomization"`
}

// ExperimentAnalytics tracks experiment analytics
type ExperimentAnalytics struct {
	experiments map[string]*ExperimentData
	mu          sync.RWMutex
}

// ExperimentData contains analytics data for an experiment
type ExperimentData struct {
	ExperimentID string                    `json:"experiment_id"`
	Assignments  map[string]string         `json:"assignments"`  // user -> variant
	Events       []ExperimentEvent         `json:"events"`
	Metrics      map[string]*MetricData    `json:"metrics"`
	Updated      time.Time                 `json:"updated"`
}

// ExperimentEvent represents an experiment event
type ExperimentEvent struct {
	UserID      string                 `json:"user_id"`
	Variant     string                 `json:"variant"`
	Event       string                 `json:"event"`
	Value       float64                `json:"value"`
	Timestamp   time.Time              `json:"timestamp"`
	Properties  map[string]interface{} `json:"properties"`
}

// MetricData contains data for a specific metric
type MetricData struct {
	Name   string              `json:"name"`
	Values map[string][]float64 `json:"values"`  // variant -> values
	Count  map[string]int64     `json:"count"`   // variant -> count
}

// NewSunsetMonitoringService creates a new sunset monitoring service
func NewSunsetMonitoringService(analytics *VersionAnalyticsService, lifecycle *VersionLifecycleManager, config *SunsetMonitoringConfig) *SunsetMonitoringService {
	if config == nil {
		config = &SunsetMonitoringConfig{
			Enabled:                    true,
			MonitoringInterval:         1 * time.Hour,
			UsageThresholdForWarning:   1000,
			ClientMigrationTracking:    true,
			AutomatedNotifications:     true,
			GracePeriodEnforcement:     true,
			PreSunsetAnalyisis:         true,
			ImpactAnalysisThreshold:    0.05,
			ClientSegmentation:         true,
			RollbackCapability:         true,
		}
	}
	
	return &SunsetMonitoringService{
		analytics:     analytics,
		lifecycle:     lifecycle,
		notifications: NewNotificationService(),
		clientTracker: &ClientTracker{
			clients:         make(map[string]*ClientProfile),
			versionUsage:    make(map[string]map[string]*ClientVersionUsage),
			migrationStatus: make(map[string]*ClientMigrationStatus),
		},
		migrationPlan:  &MigrationPlanManager{
			plans:     make(map[string]*MigrationPlan),
			templates: make(map[string]*MigrationPlanTemplate),
			executors: make(map[string]MigrationExecutor),
		},
		config: config,
	}
}

// MonitorSunsetProgress monitors progress of version sunset
func (sms *SunsetMonitoringService) MonitorSunsetProgress(ctx context.Context) {
	if !sms.config.Enabled {
		return
	}
	
	ticker := time.NewTicker(sms.config.MonitoringInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sms.performMonitoringCycle(ctx)
		}
	}
}

// performMonitoringCycle performs a monitoring cycle
func (sms *SunsetMonitoringService) performMonitoringCycle(ctx context.Context) {
	sms.mu.Lock()
	defer sms.mu.Unlock()
	
	// Get all deprecated and sunset versions
	versions := sms.lifecycle.registry.GetAllVersions()
	
	for versionStr, version := range versions {
		if version.Status == StatusDeprecated || version.Status == StatusSunset {
			sms.analyzeVersionUsage(ctx, versionStr, version)
			sms.trackClientMigration(ctx, versionStr)
			sms.enforceGracePeriod(ctx, versionStr, version)
		}
	}
}

// analyzeVersionUsage analyzes usage of deprecated/sunset versions
func (sms *SunsetMonitoringService) analyzeVersionUsage(ctx context.Context, versionStr string, version *Version) {
	// Get usage metrics for the last 24 hours
	query := &MetricsQuery{
		Versions:  []string{versionStr},
		StartTime: time.Now().Add(-24 * time.Hour),
		EndTime:   time.Now(),
		Metrics:   []MetricType{MetricRequestCount, MetricUniqueClients},
	}
	
	result, err := sms.analytics.GetMetrics(ctx, query)
	if err != nil {
		return
	}
	
	if result.Summary.TotalRequests > sms.config.UsageThresholdForWarning {
		sms.sendHighUsageAlert(versionStr, result.Summary.TotalRequests)
	}
	
	// Analyze client distribution
	sms.analyzeClientDistribution(ctx, versionStr, result)
}

// trackClientMigration tracks client migration progress
func (sms *SunsetMonitoringService) trackClientMigration(ctx context.Context, versionStr string) {
	// Get clients using this version
	clients := sms.getClientsUsingVersion(versionStr)
	
	for _, clientID := range clients {
		status := sms.clientTracker.migrationStatus[clientID]
		if status != nil && status.FromVersion == versionStr {
			sms.updateMigrationProgress(ctx, clientID, status)
		}
	}
}

// enforceGracePeriod enforces grace period policies
func (sms *SunsetMonitoringService) enforceGracePeriod(ctx context.Context, versionStr string, version *Version) {
	if !sms.config.GracePeriodEnforcement || version.Status != StatusSunset {
		return
	}
	
	if version.SunsetDate != nil && time.Now().After(version.SunsetDate.Add(30*24*time.Hour)) {
		// Grace period has ended, start blocking requests
		sms.blockSunsetVersion(versionStr)
	}
}

// sendHighUsageAlert sends alert for high usage of deprecated version
func (sms *SunsetMonitoringService) sendHighUsageAlert(versionStr string, requestCount int64) {
	// Implementation would send alert
	fmt.Printf("HIGH USAGE ALERT: Version %s received %d requests in last 24h\n", versionStr, requestCount)
}

// analyzeClientDistribution analyzes client distribution for a version
func (sms *SunsetMonitoringService) analyzeClientDistribution(ctx context.Context, versionStr string, result *MetricsResult) {
	// Implementation would analyze client distribution
}

// getClientsUsingVersion returns clients using a specific version
func (sms *SunsetMonitoringService) getClientsUsingVersion(versionStr string) []string {
	clients := make([]string, 0)
	
	for clientID, versions := range sms.clientTracker.versionUsage {
		if _, exists := versions[versionStr]; exists {
			clients = append(clients, clientID)
		}
	}
	
	return clients
}

// updateMigrationProgress updates migration progress for a client
func (sms *SunsetMonitoringService) updateMigrationProgress(ctx context.Context, clientID string, status *ClientMigrationStatus) {
	// Implementation would update migration progress
}

// blockSunsetVersion blocks access to sunset version
func (sms *SunsetMonitoringService) blockSunsetVersion(versionStr string) {
	// Implementation would block version access
	fmt.Printf("BLOCKING VERSION: %s has exceeded grace period\n", versionStr)
}

// SunsetMonitoringHandler provides HTTP endpoints for sunset monitoring
func (sms *SunsetMonitoringService) SunsetMonitoringHandler() http.Handler {
	mux := http.NewServeMux()
	
	mux.HandleFunc("/sunset-status", sms.handleSunsetStatus)
	mux.HandleFunc("/client-migration", sms.handleClientMigration)
	mux.HandleFunc("/migration-plans", sms.handleMigrationPlans)
	
	return mux
}

// handleSunsetStatus handles sunset status requests
func (sms *SunsetMonitoringService) handleSunsetStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	// Return sunset status for all versions
	status := make(map[string]interface{})
	
	for versionStr, version := range sms.lifecycle.registry.GetAllVersions() {
		if version.Status == StatusDeprecated || version.Status == StatusSunset {
			status[versionStr] = map[string]interface{}{
				"status":          version.Status,
				"deprecated_date": version.DeprecatedDate,
				"sunset_date":     version.SunsetDate,
				"usage_stats":     "TODO: Get usage stats",
				"client_count":    len(sms.getClientsUsingVersion(versionStr)),
			}
		}
	}
	
	json.NewEncoder(w).Encode(status)
}

// handleClientMigration handles client migration requests
func (sms *SunsetMonitoringService) handleClientMigration(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	clientID := r.URL.Query().Get("client")
	if clientID == "" {
		// Return all client migration statuses
		json.NewEncoder(w).Encode(sms.clientTracker.migrationStatus)
		return
	}
	
	status, exists := sms.clientTracker.migrationStatus[clientID]
	if !exists {
		http.Error(w, "Client not found", http.StatusNotFound)
		return
	}
	
	json.NewEncoder(w).Encode(status)
}

// handleMigrationPlans handles migration plan requests
func (sms *SunsetMonitoringService) handleMigrationPlans(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	planID := r.URL.Query().Get("plan")
	if planID == "" {
		// Return all plans
		plans := make([]string, 0, len(sms.migrationPlan.plans))
		for id := range sms.migrationPlan.plans {
			plans = append(plans, id)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"plans": plans,
		})
		return
	}
	
	plan, exists := sms.migrationPlan.plans[planID]
	if !exists {
		http.Error(w, "Plan not found", http.StatusNotFound)
		return
	}
	
	json.NewEncoder(w).Encode(plan)
}