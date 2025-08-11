package versioning

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"
)

// VersionLifecycleManager manages the lifecycle of API versions
type VersionLifecycleManager struct {
	registry       *VersionRegistry
	notifications  *NotificationService
	migrations     *MigrationService
	monitoring     *LifecycleMonitoring
	policies       *LifecyclePolicies
	schedules      map[string]*LifecycleSchedule
	mu             sync.RWMutex
}

// LifecyclePolicies defines policies for version lifecycle management
type LifecyclePolicies struct {
	DeprecationWarningPeriod time.Duration `json:"deprecation_warning_period"` // Warning before deprecation
	DeprecationPeriod        time.Duration `json:"deprecation_period"`         // Time before sunset
	GracePeriod              time.Duration `json:"grace_period"`               // Grace period after sunset
	AutomaticTransitions     bool          `json:"automatic_transitions"`      // Auto-transition between stages
	RequireApproval          bool          `json:"require_approval"`           // Require approval for transitions
	NotificationChannels     []string      `json:"notification_channels"`      // Where to send notifications
	ClientSupport            ClientSupportPolicy `json:"client_support"`       // Client-specific policies
}

// ClientSupportPolicy defines how to handle different clients during lifecycle
type ClientSupportPolicy struct {
	MinimumSupportedVersions map[string]string `json:"minimum_supported_versions"` // client -> min version
	ForceUpgradeClients      []string          `json:"force_upgrade_clients"`      // Clients that must upgrade
	GracePeriodByClient      map[string]time.Duration `json:"grace_period_by_client"` // Client-specific grace periods
}

// LifecycleSchedule defines the schedule for version transitions
type LifecycleSchedule struct {
	VersionString     string                `json:"version"`
	CurrentStage      VersionStatus         `json:"current_stage"`
	Transitions       []LifecycleTransition `json:"transitions"`
	AutomaticActions  []AutomaticAction     `json:"automatic_actions"`
	ApprovalRequired  bool                  `json:"approval_required"`
	ApprovalStatus    ApprovalStatus        `json:"approval_status"`
	CreatedAt         time.Time             `json:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at"`
}

// LifecycleTransition defines a transition between version stages
type LifecycleTransition struct {
	FromStage     VersionStatus `json:"from_stage"`
	ToStage       VersionStatus `json:"to_stage"`
	ScheduledTime time.Time     `json:"scheduled_time"`
	ActualTime    *time.Time    `json:"actual_time,omitempty"`
	Status        TransitionStatus `json:"status"`
	Reason        string        `json:"reason"`
	Actions       []string      `json:"actions"`         // Actions to perform during transition
	Prerequisites []string      `json:"prerequisites"`   // Prerequisites for transition
	Notifications []NotificationConfig `json:"notifications"`
}

// AutomaticAction defines actions to perform automatically during lifecycle
type AutomaticAction struct {
	Action        ActionType    `json:"action"`
	Stage         VersionStatus `json:"stage"`
	Trigger       TriggerType   `json:"trigger"`
	Conditions    []Condition   `json:"conditions"`
	Parameters    map[string]interface{} `json:"parameters"`
	Enabled       bool          `json:"enabled"`
}

// TransitionStatus represents the status of a lifecycle transition
type TransitionStatus string

const (
	TransitionPending   TransitionStatus = "pending"
	TransitionApproved  TransitionStatus = "approved"
	TransitionRejected  TransitionStatus = "rejected"
	TransitionExecuting TransitionStatus = "executing"
	TransitionCompleted TransitionStatus = "completed"
	TransitionFailed    TransitionStatus = "failed"
)

// ApprovalStatus represents the approval status for lifecycle actions
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalRejected ApprovalStatus = "rejected"
)

// ActionType defines types of automatic actions
type ActionType string

const (
	ActionNotifyUsers       ActionType = "notify_users"
	ActionUpdateDocumentation ActionType = "update_documentation"
	ActionGenerateMigrationGuide ActionType = "generate_migration_guide"
	ActionCreateClientSDKs ActionType = "create_client_sdks"
	ActionRunTests         ActionType = "run_tests"
	ActionUpdateLoadBalancer ActionType = "update_load_balancer"
	ActionScaleResources   ActionType = "scale_resources"
)

// TriggerType defines when automatic actions are triggered
type TriggerType string

const (
	TriggerTimeSchedule TriggerType = "time_schedule"
	TriggerUsageLevel   TriggerType = "usage_level"
	TriggerErrorRate    TriggerType = "error_rate"
	TriggerManual       TriggerType = "manual"
)

// Condition defines conditions for automatic actions
type Condition struct {
	Type      string      `json:"type"`       // e.g., "usage_below", "error_rate_above"
	Threshold interface{} `json:"threshold"`  // Threshold value
	Duration  time.Duration `json:"duration"` // How long condition must be met
}

// NotificationConfig defines how to notify about lifecycle events
type NotificationConfig struct {
	Channel   string                 `json:"channel"`   // email, webhook, slack, etc.
	Recipients []string              `json:"recipients"`
	Template  string                 `json:"template"`
	Timing    NotificationTiming     `json:"timing"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// NotificationTiming defines when to send notifications
type NotificationTiming struct {
	Immediate bool          `json:"immediate"`
	Schedule  []time.Time   `json:"schedule"`   // Specific times to send
	Intervals []time.Duration `json:"intervals"` // Intervals before transition
}

// NotificationService handles sending lifecycle notifications
type NotificationService struct {
	channels map[string]NotificationChannel
	mu       sync.RWMutex
}

// NotificationChannel interface for different notification channels
type NotificationChannel interface {
	Send(ctx context.Context, notification *Notification) error
	GetType() string
}

// Notification represents a lifecycle notification
type Notification struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Subject   string                 `json:"subject"`
	Body      string                 `json:"body"`
	Recipients []string              `json:"recipients"`
	Metadata  map[string]interface{} `json:"metadata"`
	CreatedAt time.Time              `json:"created_at"`
	SentAt    *time.Time             `json:"sent_at,omitempty"`
}

// MigrationService helps with version migrations
type MigrationService struct {
	guides    map[string]*MigrationGuide
	tools     map[string]MigrationTool
	mu        sync.RWMutex
}

// MigrationGuide provides guidance for migrating between versions
type MigrationGuide struct {
	FromVersion   string              `json:"from_version"`
	ToVersion     string              `json:"to_version"`
	Title         string              `json:"title"`
	Description   string              `json:"description"`
	Complexity    MigrationComplexity `json:"complexity"`
	EstimatedTime time.Duration       `json:"estimated_time"`
	Steps         []MigrationStep     `json:"steps"`
	CodeExamples  []CodeExample       `json:"code_examples"`
	Tools         []string            `json:"tools"`           // Available migration tools
	Prerequisites []string            `json:"prerequisites"`
	Warnings      []string            `json:"warnings"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

// MigrationComplexity indicates how complex a migration is
type MigrationComplexity string

const (
	ComplexityLow    MigrationComplexity = "low"
	ComplexityMedium MigrationComplexity = "medium"
	ComplexityHigh   MigrationComplexity = "high"
)

// MigrationStep represents a step in the migration process
type MigrationStep struct {
	Order       int                    `json:"order"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Type        StepType               `json:"type"`
	Required    bool                   `json:"required"`
	Actions     []string               `json:"actions"`
	Validation  string                 `json:"validation"`
	Examples    []CodeExample          `json:"examples"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// StepType defines the type of migration step
type StepType string

const (
	StepCodeChange     StepType = "code_change"
	StepConfiguration  StepType = "configuration"
	StepDataMigration  StepType = "data_migration"
	StepTesting        StepType = "testing"
	StepDeployment     StepType = "deployment"
	StepValidation     StepType = "validation"
)

// CodeExample provides code examples for migration
type CodeExample struct {
	Language    string `json:"language"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Before      string `json:"before"`
	After       string `json:"after"`
	Explanation string `json:"explanation"`
}

// MigrationTool interface for migration tools
type MigrationTool interface {
	GetName() string
	GetDescription() string
	CanMigrate(fromVersion, toVersion string) bool
	GenerateScript(fromVersion, toVersion string, options map[string]interface{}) (string, error)
	ValidateMigration(fromVersion, toVersion string) error
}

// LifecycleMonitoring monitors version lifecycle health
type LifecycleMonitoring struct {
	alerts    []LifecycleAlert
	metrics   *LifecycleMetrics
	mu        sync.RWMutex
}

// LifecycleAlert represents an alert about version lifecycle
type LifecycleAlert struct {
	ID          string        `json:"id"`
	Type        AlertType     `json:"type"`
	Severity    AlertSeverity `json:"severity"`
	Version     string        `json:"version"`
	Message     string        `json:"message"`
	CreatedAt   time.Time     `json:"created_at"`
	ResolvedAt  *time.Time    `json:"resolved_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// AlertType defines types of lifecycle alerts
type AlertType string

const (
	AlertDeprecationWarning AlertType = "deprecation_warning"
	AlertSunsetReminder     AlertType = "sunset_reminder"
	AlertHighUsage          AlertType = "high_usage"
	AlertLowUsage           AlertType = "low_usage"
	AlertErrorSpike         AlertType = "error_spike"
	AlertClientImpact       AlertType = "client_impact"
)

// AlertSeverity defines alert severity levels
type AlertSeverity string

const (
	SeverityLow      AlertSeverity = "low"
	SeverityMedium   AlertSeverity = "medium"
	SeverityHigh     AlertSeverity = "high"
	SeverityCritical AlertSeverity = "critical"
)

// LifecycleMetrics tracks lifecycle-related metrics
type LifecycleMetrics struct {
	VersionMetrics map[string]*VersionLifecycleMetrics `json:"version_metrics"`
	UpdatedAt      time.Time                           `json:"updated_at"`
}

// VersionLifecycleMetrics tracks metrics for a specific version
type VersionLifecycleMetrics struct {
	Version           string    `json:"version"`
	Stage             VersionStatus `json:"stage"`
	DaysInStage       int       `json:"days_in_stage"`
	RequestCount      int64     `json:"request_count"`
	UniqueClients     int       `json:"unique_clients"`
	ErrorRate         float64   `json:"error_rate"`
	LastUsed          time.Time `json:"last_used"`
	ClientDistribution map[string]int `json:"client_distribution"`
}

// NewVersionLifecycleManager creates a new version lifecycle manager
func NewVersionLifecycleManager(registry *VersionRegistry, policies *LifecyclePolicies) *VersionLifecycleManager {
	if policies == nil {
		policies = &LifecyclePolicies{
			DeprecationWarningPeriod: 30 * 24 * time.Hour, // 30 days
			DeprecationPeriod:        90 * 24 * time.Hour, // 90 days
			GracePeriod:              30 * 24 * time.Hour, // 30 days
			AutomaticTransitions:     false,
			RequireApproval:          true,
			NotificationChannels:     []string{"email", "webhook"},
		}
	}
	
	return &VersionLifecycleManager{
		registry:      registry,
		notifications: NewNotificationService(),
		migrations:    NewMigrationService(),
		monitoring:    NewLifecycleMonitoring(),
		policies:      policies,
		schedules:     make(map[string]*LifecycleSchedule),
	}
}

// NewNotificationService creates a new notification service
func NewNotificationService() *NotificationService {
	return &NotificationService{
		channels: make(map[string]NotificationChannel),
	}
}

// NewMigrationService creates a new migration service
func NewMigrationService() *MigrationService {
	return &MigrationService{
		guides: make(map[string]*MigrationGuide),
		tools:  make(map[string]MigrationTool),
	}
}

// NewLifecycleMonitoring creates a new lifecycle monitoring service
func NewLifecycleMonitoring() *LifecycleMonitoring {
	return &LifecycleMonitoring{
		alerts: make([]LifecycleAlert, 0),
		metrics: &LifecycleMetrics{
			VersionMetrics: make(map[string]*VersionLifecycleMetrics),
			UpdatedAt:      time.Now(),
		},
	}
}

// ScheduleDeprecation schedules a version for deprecation
func (vlm *VersionLifecycleManager) ScheduleDeprecation(versionStr string, deprecationTime time.Time, reason string) error {
	vlm.mu.Lock()
	defer vlm.mu.Unlock()
	
	version, exists := vlm.registry.GetVersion(versionStr)
	if !exists {
		return fmt.Errorf("version %s not found", versionStr)
	}
	
	if version.Status == StatusDeprecated || version.Status == StatusSunset {
		return fmt.Errorf("version %s is already deprecated or sunset", versionStr)
	}
	
	// Calculate sunset time
	sunsetTime := deprecationTime.Add(vlm.policies.DeprecationPeriod)
	
	// Create lifecycle schedule
	schedule := &LifecycleSchedule{
		VersionString: versionStr,
		CurrentStage:  version.Status,
		Transitions: []LifecycleTransition{
			{
				FromStage:     version.Status,
				ToStage:       StatusDeprecated,
				ScheduledTime: deprecationTime,
				Status:        TransitionPending,
				Reason:        reason,
				Actions:       []string{"notify_users", "update_documentation"},
				Notifications: vlm.createDeprecationNotifications(versionStr, deprecationTime),
			},
			{
				FromStage:     StatusDeprecated,
				ToStage:       StatusSunset,
				ScheduledTime: sunsetTime,
				Status:        TransitionPending,
				Reason:        "Scheduled sunset after deprecation period",
				Actions:       []string{"notify_users", "update_load_balancer"},
				Notifications: vlm.createSunsetNotifications(versionStr, sunsetTime),
			},
		},
		ApprovalRequired: vlm.policies.RequireApproval,
		ApprovalStatus:   ApprovalPending,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	
	vlm.schedules[versionStr] = schedule
	
	// Generate migration guide
	vlm.generateMigrationGuide(versionStr)
	
	return nil
}

// createDeprecationNotifications creates notifications for deprecation
func (vlm *VersionLifecycleManager) createDeprecationNotifications(versionStr string, deprecationTime time.Time) []NotificationConfig {
	notifications := []NotificationConfig{
		{
			Channel:    "email",
			Recipients: []string{"developers@example.com", "api-users@example.com"},
			Template:   "deprecation_warning",
			Timing: NotificationTiming{
				Intervals: []time.Duration{
					30 * 24 * time.Hour, // 30 days before
					7 * 24 * time.Hour,  // 7 days before
					1 * 24 * time.Hour,  // 1 day before
				},
			},
		},
		{
			Channel:    "webhook",
			Recipients: []string{"https://api.example.com/webhooks/deprecation"},
			Template:   "deprecation_webhook",
			Timing: NotificationTiming{
				Immediate: true,
			},
		},
	}
	
	return notifications
}

// createSunsetNotifications creates notifications for sunset
func (vlm *VersionLifecycleManager) createSunsetNotifications(versionStr string, sunsetTime time.Time) []NotificationConfig {
	notifications := []NotificationConfig{
		{
			Channel:    "email",
			Recipients: []string{"developers@example.com", "api-users@example.com"},
			Template:   "sunset_warning",
			Timing: NotificationTiming{
				Intervals: []time.Duration{
					30 * 24 * time.Hour, // 30 days before
					7 * 24 * time.Hour,  // 7 days before
					1 * 24 * time.Hour,  // 1 day before
				},
			},
		},
	}
	
	return notifications
}

// generateMigrationGuide generates migration guide for version transition
func (vlm *VersionLifecycleManager) generateMigrationGuide(fromVersion string) {
	// Find the latest stable version as target
	var latestVersion *Version
	for _, version := range vlm.registry.GetAllVersions() {
		if version.Status == StatusStable && (latestVersion == nil || version.IsNewerThan(latestVersion)) {
			latestVersion = version
		}
	}
	
	if latestVersion == nil {
		return
	}
	
	toVersion := latestVersion.String()
	guideKey := fmt.Sprintf("%s->%s", fromVersion, toVersion)
	
	guide := &MigrationGuide{
		FromVersion:   fromVersion,
		ToVersion:     toVersion,
		Title:         fmt.Sprintf("Migration Guide: %s to %s", fromVersion, toVersion),
		Description:   fmt.Sprintf("Guide for migrating from API version %s to %s", fromVersion, toVersion),
		Complexity:    ComplexityMedium,
		EstimatedTime: 2 * time.Hour,
		Steps:         vlm.generateMigrationSteps(fromVersion, toVersion),
		CodeExamples:  vlm.generateCodeExamples(fromVersion, toVersion),
		Prerequisites: []string{"Review breaking changes", "Update client libraries", "Test in development environment"},
		Warnings:      []string{"This version will be sunset", "Breaking changes may affect your application"},
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	
	vlm.migrations.guides[guideKey] = guide
}

// generateMigrationSteps generates migration steps between versions
func (vlm *VersionLifecycleManager) generateMigrationSteps(fromVersion, toVersion string) []MigrationStep {
	return []MigrationStep{
		{
			Order:       1,
			Title:       "Review Breaking Changes",
			Description: "Review all breaking changes between versions",
			Type:        StepValidation,
			Required:    true,
			Actions:     []string{"Read changelog", "Identify affected code"},
		},
		{
			Order:       2,
			Title:       "Update Authentication",
			Description: "Update authentication mechanism if changed",
			Type:        StepCodeChange,
			Required:    true,
			Actions:     []string{"Update auth headers", "Update token handling"},
		},
		{
			Order:       3,
			Title:       "Update Request/Response Handling",
			Description: "Update request and response data structures",
			Type:        StepCodeChange,
			Required:    true,
			Actions:     []string{"Update data models", "Update serialization"},
		},
		{
			Order:       4,
			Title:       "Test Changes",
			Description: "Thoroughly test all changes",
			Type:        StepTesting,
			Required:    true,
			Actions:     []string{"Unit tests", "Integration tests", "Load testing"},
		},
	}
}

// generateCodeExamples generates code examples for migration
func (vlm *VersionLifecycleManager) generateCodeExamples(fromVersion, toVersion string) []CodeExample {
	return []CodeExample{
		{
			Language:    "javascript",
			Title:       "Authentication Update",
			Description: "Update from basic auth to OAuth2",
			Before: `// Old version
const response = await fetch('/api/v1/users', {
  headers: {
    'Authorization': 'Basic ' + btoa(username + ':' + password)
  }
});`,
			After: `// New version
const response = await fetch('/api/v2/users', {
  headers: {
    'Authorization': 'Bearer ' + accessToken
  }
});`,
			Explanation: "Authentication has changed from basic auth to OAuth2 bearer tokens",
		},
	}
}

// ProcessScheduledTransitions processes any pending scheduled transitions
func (vlm *VersionLifecycleManager) ProcessScheduledTransitions() error {
	vlm.mu.Lock()
	defer vlm.mu.Unlock()
	
	now := time.Now()
	
	for versionStr, schedule := range vlm.schedules {
		for i := range schedule.Transitions {
			transition := &schedule.Transitions[i]
			
			if transition.Status == TransitionPending && 
			   transition.ScheduledTime.Before(now) &&
			   (!vlm.policies.RequireApproval || schedule.ApprovalStatus == ApprovalApproved) {
				
				err := vlm.executeTransition(versionStr, transition)
				if err != nil {
					log.Printf("Failed to execute transition for version %s: %v", versionStr, err)
					transition.Status = TransitionFailed
				} else {
					transition.Status = TransitionCompleted
					actualTime := now
					transition.ActualTime = &actualTime
					schedule.CurrentStage = transition.ToStage
				}
				
				schedule.UpdatedAt = now
			}
		}
	}
	
	return nil
}

// executeTransition executes a lifecycle transition
func (vlm *VersionLifecycleManager) executeTransition(versionStr string, transition *LifecycleTransition) error {
	version, exists := vlm.registry.GetVersion(versionStr)
	if !exists {
		return fmt.Errorf("version %s not found", versionStr)
	}
	
	// Update version status
	version.Status = transition.ToStage
	
	if transition.ToStage == StatusDeprecated {
		now := time.Now()
		version.DeprecatedDate = &now
		
		// Calculate sunset date
		sunsetDate := now.Add(vlm.policies.DeprecationPeriod)
		version.SunsetDate = &sunsetDate
	}
	
	// Execute actions
	for _, action := range transition.Actions {
		err := vlm.executeAction(action, versionStr, transition)
		if err != nil {
			log.Printf("Failed to execute action %s for version %s: %v", action, versionStr, err)
		}
	}
	
	// Send notifications
	for _, notificationConfig := range transition.Notifications {
		err := vlm.sendNotification(versionStr, transition, notificationConfig)
		if err != nil {
			log.Printf("Failed to send notification for version %s: %v", versionStr, err)
		}
	}
	
	return nil
}

// executeAction executes a lifecycle action
func (vlm *VersionLifecycleManager) executeAction(action, versionStr string, transition *LifecycleTransition) error {
	switch action {
	case "notify_users":
		return vlm.notifyUsers(versionStr, transition)
	case "update_documentation":
		return vlm.updateDocumentation(versionStr, transition)
	case "update_load_balancer":
		return vlm.updateLoadBalancer(versionStr, transition)
	default:
		log.Printf("Unknown action: %s", action)
		return nil
	}
}

// notifyUsers sends notifications to users
func (vlm *VersionLifecycleManager) notifyUsers(versionStr string, transition *LifecycleTransition) error {
	// Implementation would send notifications to users
	log.Printf("Notifying users about version %s transition from %s to %s", 
		versionStr, transition.FromStage, transition.ToStage)
	return nil
}

// updateDocumentation updates API documentation
func (vlm *VersionLifecycleManager) updateDocumentation(versionStr string, transition *LifecycleTransition) error {
	// Implementation would update documentation
	log.Printf("Updating documentation for version %s transition", versionStr)
	return nil
}

// updateLoadBalancer updates load balancer configuration
func (vlm *VersionLifecycleManager) updateLoadBalancer(versionStr string, transition *LifecycleTransition) error {
	// Implementation would update load balancer
	log.Printf("Updating load balancer for version %s transition", versionStr)
	return nil
}

// sendNotification sends a lifecycle notification
func (vlm *VersionLifecycleManager) sendNotification(versionStr string, transition *LifecycleTransition, config NotificationConfig) error {
	// Implementation would send notifications via configured channels
	log.Printf("Sending %s notification for version %s transition", config.Channel, versionStr)
	return nil
}

// GetLifecycleStatus returns the lifecycle status of all versions
func (vlm *VersionLifecycleManager) GetLifecycleStatus() map[string]*LifecycleSchedule {
	vlm.mu.RLock()
	defer vlm.mu.RUnlock()
	
	// Return a copy to avoid race conditions
	status := make(map[string]*LifecycleSchedule)
	for k, v := range vlm.schedules {
		status[k] = v
	}
	
	return status
}

// ApproveTransition approves a pending transition
func (vlm *VersionLifecycleManager) ApproveTransition(versionStr string) error {
	vlm.mu.Lock()
	defer vlm.mu.Unlock()
	
	schedule, exists := vlm.schedules[versionStr]
	if !exists {
		return fmt.Errorf("no scheduled transitions for version %s", versionStr)
	}
	
	schedule.ApprovalStatus = ApprovalApproved
	schedule.UpdatedAt = time.Now()
	
	return nil
}

// RejectTransition rejects a pending transition
func (vlm *VersionLifecycleManager) RejectTransition(versionStr string, reason string) error {
	vlm.mu.Lock()
	defer vlm.mu.Unlock()
	
	schedule, exists := vlm.schedules[versionStr]
	if !exists {
		return fmt.Errorf("no scheduled transitions for version %s", versionStr)
	}
	
	schedule.ApprovalStatus = ApprovalRejected
	schedule.UpdatedAt = time.Now()
	
	// Update transition status
	for i := range schedule.Transitions {
		if schedule.Transitions[i].Status == TransitionPending {
			schedule.Transitions[i].Status = TransitionRejected
			schedule.Transitions[i].Reason = reason
		}
	}
	
	return nil
}

// GetMigrationGuide returns a migration guide between versions
func (vlm *VersionLifecycleManager) GetMigrationGuide(fromVersion, toVersion string) (*MigrationGuide, error) {
	vlm.migrations.mu.RLock()
	defer vlm.migrations.mu.RUnlock()
	
	key := fmt.Sprintf("%s->%s", fromVersion, toVersion)
	guide, exists := vlm.migrations.guides[key]
	if !exists {
		return nil, fmt.Errorf("no migration guide found for %s to %s", fromVersion, toVersion)
	}
	
	return guide, nil
}

// LifecycleStatusHandler provides HTTP endpoint for lifecycle status
func (vlm *VersionLifecycleManager) LifecycleStatusHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		status := vlm.GetLifecycleStatus()
		json.NewEncoder(w).Encode(status)
	})
}

// MigrationGuideHandler provides HTTP endpoint for migration guides
func (vlm *VersionLifecycleManager) MigrationGuideHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		fromVersion := r.URL.Query().Get("from")
		toVersion := r.URL.Query().Get("to")
		
		if fromVersion == "" || toVersion == "" {
			http.Error(w, "Both 'from' and 'to' parameters are required", http.StatusBadRequest)
			return
		}
		
		guide, err := vlm.GetMigrationGuide(fromVersion, toVersion)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		
		json.NewEncoder(w).Encode(guide)
	})
}