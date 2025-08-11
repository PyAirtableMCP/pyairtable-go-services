package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
)

// AutomationEngineWorkflow implements visual workflow builder and automation engine
type AutomationEngineWorkflow struct {
	instance           *WorkflowInstance
	config             *AutomationEngineConfig
	eventStore         EventStore
	workflowEngine     WorkflowEngine
	triggerManager     TriggerManager
	actionExecutor     ActionExecutor
	conditionEvaluator ConditionEvaluator
	integrationManager IntegrationManager
	templateManager    TemplateManager
	executionManager   ExecutionManager
	cronScheduler      *cron.Cron
	mu                 sync.RWMutex
}

// WorkflowEngine interface for workflow execution
type WorkflowEngine interface {
	CreateWorkflow(ctx context.Context, definition *WorkflowDefinition) (*ExecutableWorkflow, error)
	UpdateWorkflow(ctx context.Context, workflowID uuid.UUID, definition *WorkflowDefinition) error
	DeleteWorkflow(ctx context.Context, workflowID uuid.UUID) error
	GetWorkflow(ctx context.Context, workflowID uuid.UUID) (*ExecutableWorkflow, error)
	ListWorkflows(ctx context.Context, filter WorkflowFilter) ([]*ExecutableWorkflow, error)
	ExecuteWorkflow(ctx context.Context, workflowID uuid.UUID, input map[string]interface{}) (*WorkflowExecution, error)
	ValidateWorkflow(ctx context.Context, definition *WorkflowDefinition) (*ValidationResult, error)
}

// TriggerManager interface for managing workflow triggers
type TriggerManager interface {
	RegisterTrigger(ctx context.Context, trigger *WorkflowTrigger) error
	UnregisterTrigger(ctx context.Context, triggerID uuid.UUID) error
	GetTrigger(ctx context.Context, triggerID uuid.UUID) (*WorkflowTrigger, error)
	ListTriggers(ctx context.Context, workflowID uuid.UUID) ([]*WorkflowTrigger, error)
	EvaluateTriggers(ctx context.Context, event *TriggerEvent) ([]*WorkflowTrigger, error)
	HandleWebhookTrigger(ctx context.Context, webhookData *WebhookTriggerData) error
}

// ActionExecutor interface for executing workflow actions
type ActionExecutor interface {
	ExecuteAction(ctx context.Context, action *WorkflowAction, context *ActionContext) (*ActionResult, error)
	ValidateAction(ctx context.Context, action *WorkflowAction) error
	GetSupportedActions(ctx context.Context) ([]*ActionDefinition, error)
	RegisterCustomAction(ctx context.Context, action *CustomActionDefinition) error
	ExecuteBatch(ctx context.Context, actions []*WorkflowAction, context *ActionContext) ([]*ActionResult, error)
}

// ConditionEvaluator interface for evaluating workflow conditions
type ConditionEvaluator interface {
	EvaluateCondition(ctx context.Context, condition *WorkflowCondition, context *ConditionContext) (bool, error)
	ValidateCondition(ctx context.Context, condition *WorkflowCondition) error
	GetSupportedOperators(ctx context.Context) ([]string, error)
	CompileExpression(ctx context.Context, expression string) (*CompiledExpression, error)
}

// IntegrationManager interface for managing external integrations
type IntegrationManager interface {
	CreateIntegration(ctx context.Context, integration *Integration) error
	UpdateIntegration(ctx context.Context, integration *Integration) error
	DeleteIntegration(ctx context.Context, integrationID uuid.UUID) error
	GetIntegration(ctx context.Context, integrationID uuid.UUID) (*Integration, error)
	ListIntegrations(ctx context.Context) ([]*Integration, error)
	TestIntegration(ctx context.Context, integrationID uuid.UUID) (*IntegrationTestResult, error)
	ExecuteIntegrationAction(ctx context.Context, integrationID uuid.UUID, action string, params map[string]interface{}) (interface{}, error)
}

// TemplateManager interface for managing workflow templates
type TemplateManager interface {
	CreateTemplate(ctx context.Context, template *WorkflowTemplate) error
	UpdateTemplate(ctx context.Context, template *WorkflowTemplate) error
	DeleteTemplate(ctx context.Context, templateID uuid.UUID) error
	GetTemplate(ctx context.Context, templateID uuid.UUID) (*WorkflowTemplate, error)
	ListTemplates(ctx context.Context, filter TemplateFilter) ([]*WorkflowTemplate, error)
	InstantiateTemplate(ctx context.Context, templateID uuid.UUID, params map[string]interface{}) (*WorkflowDefinition, error)
}

// ExecutionManager interface for managing workflow executions
type ExecutionManager interface {
	StartExecution(ctx context.Context, workflowID uuid.UUID, input map[string]interface{}) (*WorkflowExecution, error)
	StopExecution(ctx context.Context, executionID uuid.UUID) error
	PauseExecution(ctx context.Context, executionID uuid.UUID) error
	ResumeExecution(ctx context.Context, executionID uuid.UUID) error
	GetExecution(ctx context.Context, executionID uuid.UUID) (*WorkflowExecution, error)
	ListExecutions(ctx context.Context, filter ExecutionFilter) ([]*WorkflowExecution, error)
	GetExecutionLogs(ctx context.Context, executionID uuid.UUID) ([]*ExecutionLog, error)
	GetExecutionMetrics(ctx context.Context, workflowID uuid.UUID, period string) (*ExecutionMetrics, error)
}

// Domain entities
type WorkflowDefinition struct {
	ID            uuid.UUID                `json:"id"`
	Name          string                   `json:"name"`
	Description   string                   `json:"description"`
	Version       string                   `json:"version"`
	Triggers      []*WorkflowTrigger       `json:"triggers"`
	Steps         []*WorkflowStep          `json:"steps"`
	Variables     map[string]interface{}   `json:"variables"`
	Settings      *WorkflowSettings        `json:"settings"`
	Metadata      map[string]interface{}   `json:"metadata"`
	CreatedBy     uuid.UUID                `json:"created_by"`
	CreatedAt     time.Time                `json:"created_at"`
	UpdatedAt     time.Time                `json:"updated_at"`
}

type ExecutableWorkflow struct {
	Definition    *WorkflowDefinition      `json:"definition"`
	CompiledSteps []*CompiledStep          `json:"compiled_steps"`
	Status        string                   `json:"status"`
	IsEnabled     bool                     `json:"is_enabled"`
	LastExecution *time.Time               `json:"last_execution,omitempty"`
	ExecutionCount int                     `json:"execution_count"`
	SuccessRate   float64                  `json:"success_rate"`
}

type WorkflowTrigger struct {
	ID            uuid.UUID                `json:"id"`
	WorkflowID    uuid.UUID                `json:"workflow_id"`
	Type          string                   `json:"type"` // time, event, webhook, manual
	Configuration json.RawMessage          `json:"configuration"`
	IsEnabled     bool                     `json:"is_enabled"`
	Conditions    []*WorkflowCondition     `json:"conditions"`
	CreatedAt     time.Time                `json:"created_at"`
	UpdatedAt     time.Time                `json:"updated_at"`
}

type WorkflowStep struct {
	ID            uuid.UUID                `json:"id"`
	Name          string                   `json:"name"`
	Type          string                   `json:"type"` // action, condition, loop, parallel, error_handler
	Configuration json.RawMessage          `json:"configuration"`
	Position      *StepPosition            `json:"position"`
	Connections   []*StepConnection        `json:"connections"`
	ErrorHandlers []*ErrorHandler          `json:"error_handlers"`
	RetryPolicy   *RetryPolicy             `json:"retry_policy"`
	Timeout       *time.Duration           `json:"timeout,omitempty"`
	Metadata      map[string]interface{}   `json:"metadata"`
}

type WorkflowAction struct {
	ID            uuid.UUID                `json:"id"`
	Type          string                   `json:"type"`
	Service       string                   `json:"service"`
	Method        string                   `json:"method"`
	Parameters    map[string]interface{}   `json:"parameters"`
	Timeout       time.Duration            `json:"timeout"`
	RetryPolicy   *RetryPolicy             `json:"retry_policy"`
	ErrorHandling *ErrorHandlingPolicy     `json:"error_handling"`
}

type WorkflowCondition struct {
	ID         uuid.UUID                `json:"id"`
	Expression string                   `json:"expression"`
	Variables  map[string]interface{}   `json:"variables"`
	Operator   string                   `json:"operator"`
	Left       interface{}              `json:"left"`
	Right      interface{}              `json:"right"`
	Type       string                   `json:"type"` // simple, complex, script
}

type WorkflowSettings struct {
	MaxConcurrentExecutions int           `json:"max_concurrent_executions"`
	DefaultTimeout          time.Duration `json:"default_timeout"`
	RetryPolicy             *RetryPolicy  `json:"retry_policy"`
	ErrorHandling           *ErrorHandlingPolicy `json:"error_handling"`
	Notifications           *NotificationSettings `json:"notifications"`
	Logging                 *LoggingSettings      `json:"logging"`
}

type CompiledStep struct {
	Step         *WorkflowStep            `json:"step"`
	CompiledCode interface{}              `json:"compiled_code"`
	Dependencies []uuid.UUID              `json:"dependencies"`
	IsParallel   bool                     `json:"is_parallel"`
}

type StepPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type StepConnection struct {
	FromStepID   uuid.UUID `json:"from_step_id"`
	ToStepID     uuid.UUID `json:"to_step_id"`
	Condition    string    `json:"condition,omitempty"`
	Label        string    `json:"label,omitempty"`
}

type ErrorHandler struct {
	ID         uuid.UUID                `json:"id"`
	ErrorType  string                   `json:"error_type"`
	Action     string                   `json:"action"` // retry, skip, stop, custom
	Parameters map[string]interface{}   `json:"parameters"`
}

type RetryPolicy struct {
	MaxAttempts     int           `json:"max_attempts"`
	InitialDelay    time.Duration `json:"initial_delay"`
	MaxDelay        time.Duration `json:"max_delay"`
	BackoffStrategy string        `json:"backoff_strategy"` // linear, exponential, fixed
	RetryConditions []string      `json:"retry_conditions"`
}

type ErrorHandlingPolicy struct {
	OnError       string                   `json:"on_error"` // stop, continue, retry, custom
	CustomHandler *ErrorHandler            `json:"custom_handler,omitempty"`
	Notifications bool                     `json:"notifications"`
}

type NotificationSettings struct {
	OnSuccess    bool     `json:"on_success"`
	OnFailure    bool     `json:"on_failure"`
	OnTimeout    bool     `json:"on_timeout"`
	Recipients   []string `json:"recipients"`
	Channels     []string `json:"channels"`
}

type LoggingSettings struct {
	Level          string   `json:"level"`
	IncludePayload bool     `json:"include_payload"`
	Fields         []string `json:"fields"`
	Retention      string   `json:"retention"`
}

type WorkflowExecution struct {
	ID              uuid.UUID                `json:"id"`
	WorkflowID      uuid.UUID                `json:"workflow_id"`
	Status          string                   `json:"status"` // pending, running, completed, failed, cancelled, timeout
	Input           map[string]interface{}   `json:"input"`
	Output          map[string]interface{}   `json:"output"`
	Context         *ExecutionContext        `json:"context"`
	Steps           []*StepExecution         `json:"steps"`
	Metrics         *ExecutionMetrics        `json:"metrics"`
	Error           *ExecutionError          `json:"error,omitempty"`
	TriggeredBy     string                   `json:"triggered_by"`
	TriggerData     map[string]interface{}   `json:"trigger_data"`
	StartedAt       time.Time                `json:"started_at"`
	CompletedAt     *time.Time               `json:"completed_at,omitempty"`
	Duration        *time.Duration           `json:"duration,omitempty"`
}

type ExecutionContext struct {
	Variables       map[string]interface{}   `json:"variables"`
	UserID          uuid.UUID                `json:"user_id"`
	TenantID        uuid.UUID                `json:"tenant_id"`
	WorkspaceID     *uuid.UUID               `json:"workspace_id,omitempty"`
	CorrelationID   string                   `json:"correlation_id"`
	ParentExecution *uuid.UUID               `json:"parent_execution,omitempty"`
	Metadata        map[string]interface{}   `json:"metadata"`
}

type StepExecution struct {
	ID              uuid.UUID                `json:"id"`
	StepID          uuid.UUID                `json:"step_id"`
	Status          string                   `json:"status"`
	Input           map[string]interface{}   `json:"input"`
	Output          map[string]interface{}   `json:"output"`
	Error           *ExecutionError          `json:"error,omitempty"`
	AttemptCount    int                      `json:"attempt_count"`
	StartedAt       time.Time                `json:"started_at"`
	CompletedAt     *time.Time               `json:"completed_at,omitempty"`
	Duration        *time.Duration           `json:"duration,omitempty"`
}

type ExecutionError struct {
	Code        string                   `json:"code"`
	Message     string                   `json:"message"`
	Details     map[string]interface{}   `json:"details"`
	Recoverable bool                     `json:"recoverable"`
	Timestamp   time.Time                `json:"timestamp"`
	StackTrace  string                   `json:"stack_trace,omitempty"`
}

type ExecutionMetrics struct {
	ExecutionTime      time.Duration            `json:"execution_time"`
	StepsExecuted      int                      `json:"steps_executed"`
	StepsFailed        int                      `json:"steps_failed"`
	RetryCount         int                      `json:"retry_count"`
	ActionsExecuted    int                      `json:"actions_executed"`
	DataProcessed      int64                    `json:"data_processed"`
	APICallsMade       int                      `json:"api_calls_made"`
	ResourcesUsed      map[string]interface{}   `json:"resources_used"`
}

type ActionContext struct {
	ExecutionID   uuid.UUID                `json:"execution_id"`
	StepID        uuid.UUID                `json:"step_id"`
	Variables     map[string]interface{}   `json:"variables"`
	PreviousOutput map[string]interface{}  `json:"previous_output"`
	UserID        uuid.UUID                `json:"user_id"`
	TenantID      uuid.UUID                `json:"tenant_id"`
	Timeout       time.Duration            `json:"timeout"`
}

type ActionResult struct {
	Success       bool                     `json:"success"`
	Output        map[string]interface{}   `json:"output"`
	Error         *ExecutionError          `json:"error,omitempty"`
	Duration      time.Duration            `json:"duration"`
	RetryCount    int                      `json:"retry_count"`
	Metadata      map[string]interface{}   `json:"metadata"`
}

type ActionDefinition struct {
	Type        string                   `json:"type"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Service     string                   `json:"service"`
	Category    string                   `json:"category"`
	Icon        string                   `json:"icon"`
	Parameters  []*ParameterDefinition   `json:"parameters"`
	Returns     []*ReturnDefinition      `json:"returns"`
	Examples    []*ActionExample         `json:"examples"`
}

type CustomActionDefinition struct {
	ActionDefinition
	Code        string                   `json:"code"`
	Runtime     string                   `json:"runtime"`
	Dependencies []string                `json:"dependencies"`
}

type ParameterDefinition struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
	Description string      `json:"description"`
	Validation  string      `json:"validation,omitempty"`
}

type ReturnDefinition struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type ActionExample struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Parameters  map[string]interface{}   `json:"parameters"`
	Expected    map[string]interface{}   `json:"expected"`
}

type ConditionContext struct {
	Variables     map[string]interface{}   `json:"variables"`
	StepOutput    map[string]interface{}   `json:"step_output"`
	ExecutionData map[string]interface{}   `json:"execution_data"`
	UserID        uuid.UUID                `json:"user_id"`
	TenantID      uuid.UUID                `json:"tenant_id"`
}

type CompiledExpression struct {
	Expression  string      `json:"expression"`
	CompiledCode interface{} `json:"compiled_code"`
	Variables  []string    `json:"variables"`
	Dependencies []string   `json:"dependencies"`
}

type Integration struct {
	ID            uuid.UUID                `json:"id"`
	Name          string                   `json:"name"`
	Type          string                   `json:"type"`
	Configuration *IntegrationConfig       `json:"configuration"`
	Status        string                   `json:"status"`
	CreatedBy     uuid.UUID                `json:"created_by"`
	CreatedAt     time.Time                `json:"created_at"`
	UpdatedAt     time.Time                `json:"updated_at"`
	LastTested    *time.Time               `json:"last_tested,omitempty"`
	TestResults   *IntegrationTestResult   `json:"test_results,omitempty"`
}

type IntegrationConfig struct {
	BaseURL       string                   `json:"base_url"`
	Authentication *AuthenticationConfig   `json:"authentication"`
	Headers       map[string]string        `json:"headers"`
	Endpoints     []*EndpointDefinition    `json:"endpoints"`
	RateLimit     *RateLimitConfig         `json:"rate_limit"`
	Timeout       time.Duration            `json:"timeout"`
}

type AuthenticationConfig struct {
	Type   string                   `json:"type"` // api_key, oauth, basic, bearer
	Config map[string]interface{}   `json:"config"`
}

type EndpointDefinition struct {
	Name        string                   `json:"name"`
	Path        string                   `json:"path"`
	Method      string                   `json:"method"`
	Parameters  []*ParameterDefinition   `json:"parameters"`
	Returns     []*ReturnDefinition      `json:"returns"`
	Description string                   `json:"description"`
}

type RateLimitConfig struct {
	RequestsPerSecond int           `json:"requests_per_second"`
	BurstSize        int           `json:"burst_size"`
	BackoffStrategy  string        `json:"backoff_strategy"`
}

type IntegrationTestResult struct {
	Success      bool                     `json:"success"`
	ResponseTime time.Duration            `json:"response_time"`
	Errors       []string                 `json:"errors"`
	TestedAt     time.Time                `json:"tested_at"`
	Details      map[string]interface{}   `json:"details"`
}

type TriggerEvent struct {
	ID        uuid.UUID                `json:"id"`
	Type      string                   `json:"type"`
	Source    string                   `json:"source"`
	Data      map[string]interface{}   `json:"data"`
	Timestamp time.Time                `json:"timestamp"`
	UserID    *uuid.UUID               `json:"user_id,omitempty"`
	TenantID  *uuid.UUID               `json:"tenant_id,omitempty"`
}

type WebhookTriggerData struct {
	URL       string                   `json:"url"`
	Method    string                   `json:"method"`
	Headers   map[string]string        `json:"headers"`
	Body      []byte                   `json:"body"`
	Signature string                   `json:"signature"`
	Timestamp time.Time                `json:"timestamp"`
}

type WorkflowFilter struct {
	Status    string     `json:"status,omitempty"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty"`
	Tags      []string   `json:"tags,omitempty"`
	Search    string     `json:"search,omitempty"`
	Limit     int        `json:"limit,omitempty"`
	Offset    int        `json:"offset,omitempty"`
}

type TemplateFilter struct {
	Category  string   `json:"category,omitempty"`
	Popular   *bool    `json:"popular,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Search    string   `json:"search,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Offset    int      `json:"offset,omitempty"`
}

type ExecutionFilter struct {
	WorkflowID *uuid.UUID `json:"workflow_id,omitempty"`
	Status     string     `json:"status,omitempty"`
	StartDate  *time.Time `json:"start_date,omitempty"`
	EndDate    *time.Time `json:"end_date,omitempty"`
	Limit      int        `json:"limit,omitempty"`
	Offset     int        `json:"offset,omitempty"`
}

type ExecutionLog struct {
	ID          uuid.UUID                `json:"id"`
	ExecutionID uuid.UUID                `json:"execution_id"`
	StepID      *uuid.UUID               `json:"step_id,omitempty"`
	Level       string                   `json:"level"`
	Message     string                   `json:"message"`
	Data        map[string]interface{}   `json:"data"`
	Timestamp   time.Time                `json:"timestamp"`
}

// NewAutomationEngineWorkflow creates a new automation engine workflow instance
func NewAutomationEngineWorkflow(
	instance *WorkflowInstance,
	eventStore EventStore,
	workflowEngine WorkflowEngine,
	triggerManager TriggerManager,
	actionExecutor ActionExecutor,
	conditionEvaluator ConditionEvaluator,
	integrationManager IntegrationManager,
	templateManager TemplateManager,
	executionManager ExecutionManager,
) (*AutomationEngineWorkflow, error) {
	var config AutomationEngineConfig
	if err := json.Unmarshal(instance.Configuration, &config); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal automation engine configuration")
	}

	cronScheduler := cron.New(cron.WithSeconds())

	return &AutomationEngineWorkflow{
		instance:           instance,
		config:             &config,
		eventStore:         eventStore,
		workflowEngine:     workflowEngine,
		triggerManager:     triggerManager,
		actionExecutor:     actionExecutor,
		conditionEvaluator: conditionEvaluator,
		integrationManager: integrationManager,
		templateManager:    templateManager,
		executionManager:   executionManager,
		cronScheduler:      cronScheduler,
	}, nil
}

// Execute runs the complete Automation Engine workflow
func (w *AutomationEngineWorkflow) Execute(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Emit workflow started event
	if err := w.emitEvent(ctx, EventWorkflowStarted, map[string]interface{}{
		"workflow_type": WorkflowTypeAutomationEngine,
		"config":        w.config,
	}); err != nil {
		return errors.Wrap(err, "failed to emit workflow started event")
	}

	// Update workflow status
	w.instance.Status = WorkflowStatusRunning
	now := time.Now()
	w.instance.StartedAt = &now

	// Execute workflow steps
	steps := []AutomationStepFunc{
		w.validateConfiguration,
		w.setupWorkflowBuilder,
		w.initializeTriggerSystem,
		w.setupActionExecutors,
		w.configureConditionEvaluators,
		w.setupIntegrations,
		w.loadWorkflowTemplates,
		w.startExecutionEngine,
		w.enableMonitoring,
	}

	for i, step := range steps {
		w.instance.CurrentStepIndex = i
		stepName := w.getStepName(i)
		
		if err := w.executeStep(ctx, stepName, step); err != nil {
			w.instance.Status = WorkflowStatusFailed
			w.instance.Error = &WorkflowError{
				Code:        "AUTOMATION_ENGINE_STEP_FAILED",
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

	// Start the cron scheduler
	w.cronScheduler.Start()

	// Emit workflow completed event
	return w.emitEvent(ctx, EventWorkflowCompleted, map[string]interface{}{
		"duration": completedAt.Sub(*w.instance.StartedAt).String(),
		"stats":    w.getAutomationStats(),
	})
}

type AutomationStepFunc func(ctx context.Context) error

// Automation engine workflow step implementations
func (w *AutomationEngineWorkflow) validateConfiguration(ctx context.Context) error {
	if len(w.config.Triggers) == 0 {
		return errors.New("no triggers configured")
	}
	if len(w.config.Actions) == 0 {
		return errors.New("no actions configured")
	}
	if w.config.ExecutionConfig.MaxConcurrent <= 0 {
		return errors.New("max concurrent executions must be greater than 0")
	}
	if w.config.ExecutionConfig.Timeout <= 0 {
		return errors.New("execution timeout must be greater than 0")
	}

	return nil
}

func (w *AutomationEngineWorkflow) setupWorkflowBuilder(ctx context.Context) error {
	// Initialize visual workflow builder components
	builderConfig := w.config.WorkflowBuilder

	// Set up UI components
	uiComponents := []string{
		"trigger_selector",
		"action_palette",
		"condition_builder",
		"variable_manager",
		"debug_console",
		"execution_monitor",
	}

	if builderConfig.CustomActions {
		uiComponents = append(uiComponents, "custom_action_editor")
	}

	if builderConfig.Variables {
		uiComponents = append(uiComponents, "variable_editor")
	}

	if builderConfig.ErrorHandling {
		uiComponents = append(uiComponents, "error_handler_editor")
	}

	// Store UI configuration
	w.instance.Metadata["ui_components"] = uiComponents
	w.instance.Metadata["ui_theme"] = builderConfig.UI.Theme
	w.instance.Metadata["ui_layout"] = builderConfig.UI.Layout

	// Load workflow templates
	templates := make([]*WorkflowTemplate, 0)
	for _, template := range builderConfig.Templates {
		templates = append(templates, &template)
	}

	w.instance.Metadata["loaded_templates"] = templates

	// Emit builder setup event
	w.emitEvent(ctx, "automation.builder.setup", map[string]interface{}{
		"ui_components":   len(uiComponents),
		"templates_count": len(templates),
		"custom_actions":  builderConfig.CustomActions,
	})

	return nil
}

func (w *AutomationEngineWorkflow) initializeTriggerSystem(ctx context.Context) error {
	registeredTriggers := make([]*WorkflowTrigger, 0)

	for _, triggerConfig := range w.config.Triggers {
		trigger := &WorkflowTrigger{
			ID:            triggerConfig.ID,
			WorkflowID:    w.instance.ID,
			Type:          triggerConfig.Type,
			Configuration: triggerConfig.Config,
			IsEnabled:     triggerConfig.Enabled,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		if err := w.triggerManager.RegisterTrigger(ctx, trigger); err != nil {
			return errors.Wrapf(err, "failed to register trigger %s", trigger.ID)
		}

		registeredTriggers = append(registeredTriggers, trigger)

		// Set up scheduled triggers
		if trigger.Type == "time" {
			if err := w.setupScheduledTrigger(ctx, trigger); err != nil {
				log.Printf("Failed to setup scheduled trigger %s: %v", trigger.ID, err)
			}
		}

		// Emit trigger registered event
		w.emitEvent(ctx, "automation.trigger.registered", map[string]interface{}{
			"trigger_id":   trigger.ID,
			"trigger_type": trigger.Type,
			"enabled":      trigger.IsEnabled,
		})
	}

	w.instance.Metadata["registered_triggers"] = registeredTriggers

	return nil
}

func (w *AutomationEngineWorkflow) setupActionExecutors(ctx context.Context) error {
	availableActions := make([]*ActionDefinition, 0)

	// Get supported actions
	supportedActions, err := w.actionExecutor.GetSupportedActions(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get supported actions")
	}

	availableActions = append(availableActions, supportedActions...)

	// Register custom actions
	customActionsCount := 0
	for _, actionConfig := range w.config.Actions {
		if err := w.actionExecutor.ValidateAction(ctx, &WorkflowAction{
			ID:         actionConfig.ID,
			Type:       actionConfig.Type,
			Service:    actionConfig.Service,
			Method:     actionConfig.Method,
			Parameters: make(map[string]interface{}),
			Timeout:    actionConfig.Timeout,
		}); err != nil {
			log.Printf("Action validation failed for %s: %v", actionConfig.Type, err)
		} else {
			customActionsCount++
		}
	}

	w.instance.Metadata["available_actions"] = availableActions
	w.instance.Metadata["custom_actions_count"] = customActionsCount

	// Emit actions setup event
	w.emitEvent(ctx, "automation.actions.setup", map[string]interface{}{
		"supported_actions": len(supportedActions),
		"custom_actions":   customActionsCount,
		"total_actions":    len(availableActions) + customActionsCount,
	})

	return nil
}

func (w *AutomationEngineWorkflow) configureConditionEvaluators(ctx context.Context) error {
	// Get supported operators
	operators, err := w.conditionEvaluator.GetSupportedOperators(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get supported operators")
	}

	// Pre-compile common expressions
	compiledExpressions := make(map[string]*CompiledExpression)
	commonExpressions := []string{
		"{{input.value}} > 0",
		"{{status}} == 'success'",
		"{{count}} >= {{threshold}}",
		"{{user.role}} == 'admin'",
	}

	for _, expr := range commonExpressions {
		compiled, err := w.conditionEvaluator.CompileExpression(ctx, expr)
		if err != nil {
			log.Printf("Failed to compile expression %s: %v", expr, err)
		} else {
			compiledExpressions[expr] = compiled
		}
	}

	// Validate condition configurations
	validConditionsCount := 0
	for _, conditionConfig := range w.config.Conditions {
		condition := &WorkflowCondition{
			ID:         conditionConfig.ID,
			Expression: conditionConfig.Expression,
			Variables:  conditionConfig.Variables,
		}

		if err := w.conditionEvaluator.ValidateCondition(ctx, condition); err != nil {
			log.Printf("Condition validation failed for %s: %v", condition.ID, err)
		} else {
			validConditionsCount++
		}
	}

	w.instance.Metadata["supported_operators"] = operators
	w.instance.Metadata["compiled_expressions"] = compiledExpressions
	w.instance.Metadata["valid_conditions_count"] = validConditionsCount

	// Emit conditions setup event
	w.emitEvent(ctx, "automation.conditions.setup", map[string]interface{}{
		"supported_operators":    len(operators),
		"compiled_expressions":   len(compiledExpressions),
		"valid_conditions":       validConditionsCount,
	})

	return nil
}

func (w *AutomationEngineWorkflow) setupIntegrations(ctx context.Context) error {
	if len(w.config.Integrations) == 0 {
		return nil // Skip if no integrations configured
	}

	setupIntegrations := make([]*Integration, 0)
	successfulTests := 0

	for _, integrationConfig := range w.config.Integrations {
		integration := &Integration{
			ID:     integrationConfig.ID,
			Name:   integrationConfig.Name,
			Type:   integrationConfig.Type,
			Status: "inactive",
			Configuration: &IntegrationConfig{
				BaseURL: integrationConfig.Auth.Config["base_url"].(string),
				Authentication: &AuthenticationConfig{
					Type:   integrationConfig.Auth.Type,
					Config: integrationConfig.Auth.Config,
				},
				Endpoints:  integrationConfig.Endpoints,
				RateLimit:  &integrationConfig.RateLimit,
				Timeout:    30 * time.Second, // Default timeout
			},
			CreatedBy: w.instance.UserID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := w.integrationManager.CreateIntegration(ctx, integration); err != nil {
			log.Printf("Failed to create integration %s: %v", integration.Name, err)
			continue
		}

		// Test the integration
		testResult, err := w.integrationManager.TestIntegration(ctx, integration.ID)
		if err != nil {
			log.Printf("Integration test failed for %s: %v", integration.Name, err)
			integration.Status = "error"
		} else {
			integration.TestResults = testResult
			if testResult.Success {
				integration.Status = "active"
				successfulTests++
			} else {
				integration.Status = "error"
			}
			integration.LastTested = &testResult.TestedAt
		}

		setupIntegrations = append(setupIntegrations, integration)

		// Emit integration setup event
		w.emitEvent(ctx, "automation.integration.setup", map[string]interface{}{
			"integration_id":   integration.ID,
			"integration_name": integration.Name,
			"integration_type": integration.Type,
			"status":          integration.Status,
			"test_success":    testResult != nil && testResult.Success,
		})
	}

	w.instance.Metadata["setup_integrations"] = setupIntegrations
	w.instance.Metadata["successful_integration_tests"] = successfulTests

	return nil
}

func (w *AutomationEngineWorkflow) loadWorkflowTemplates(ctx context.Context) error {
	// Load built-in templates
	builtinTemplates := w.createBuiltinTemplates()
	loadedTemplates := make([]*WorkflowTemplate, 0)

	for _, template := range builtinTemplates {
		if err := w.templateManager.CreateTemplate(ctx, template); err != nil {
			log.Printf("Failed to create template %s: %v", template.Name, err)
		} else {
			loadedTemplates = append(loadedTemplates, template)
		}
	}

	// Load user-defined templates from configuration
	for _, templateConfig := range w.config.WorkflowBuilder.Templates {
		template := &WorkflowTemplate{
			ID:          templateConfig.ID,
			Name:        templateConfig.Name,
			Category:    templateConfig.Category,
			Description: templateConfig.Description,
			Popular:     templateConfig.Popular,
		}

		if err := w.templateManager.CreateTemplate(ctx, template); err != nil {
			log.Printf("Failed to create user template %s: %v", template.Name, err)
		} else {
			loadedTemplates = append(loadedTemplates, template)
		}
	}

	w.instance.Metadata["loaded_templates"] = loadedTemplates

	// Emit templates loaded event
	w.emitEvent(ctx, "automation.templates.loaded", map[string]interface{}{
		"builtin_templates": len(builtinTemplates),
		"user_templates":   len(w.config.WorkflowBuilder.Templates),
		"total_templates":  len(loadedTemplates),
	})

	return nil
}

func (w *AutomationEngineWorkflow) startExecutionEngine(ctx context.Context) error {
	// Initialize execution manager settings
	executionConfig := w.config.ExecutionConfig

	// Start background execution monitoring
	go w.monitorExecutions(ctx)

	// Set up execution metrics collection
	go w.collectExecutionMetrics(ctx)

	// Test execution engine with a simple workflow
	testWorkflow := w.createTestWorkflow()
	executableWorkflow, err := w.workflowEngine.CreateWorkflow(ctx, testWorkflow)
	if err != nil {
		return errors.Wrap(err, "failed to create test workflow")
	}

	// Execute test workflow
	testInput := map[string]interface{}{
		"test": true,
		"timestamp": time.Now(),
	}

	execution, err := w.executionManager.StartExecution(ctx, executableWorkflow.Definition.ID, testInput)
	if err != nil {
		log.Printf("Test execution failed: %v", err)
	} else {
		w.instance.Metadata["test_execution_id"] = execution.ID
	}

	w.instance.Metadata["execution_config"] = executionConfig
	w.instance.Metadata["test_workflow_id"] = executableWorkflow.Definition.ID

	// Emit execution engine started event
	w.emitEvent(ctx, "automation.execution_engine.started", map[string]interface{}{
		"max_concurrent": executionConfig.MaxConcurrent,
		"timeout":       executionConfig.Timeout.String(),
		"monitoring":    executionConfig.Monitoring,
		"test_workflow_created": executableWorkflow.Definition.ID,
	})

	return nil
}

func (w *AutomationEngineWorkflow) enableMonitoring(ctx context.Context) error {
	if !w.config.ExecutionConfig.Monitoring {
		return nil // Skip if monitoring is disabled
	}

	// Set up monitoring dashboards
	dashboards := []string{
		"workflow_executions",
		"trigger_activity",
		"action_performance",
		"integration_health",
		"error_tracking",
		"resource_usage",
	}

	// Set up alerting rules
	alertRules := []string{
		"high_failure_rate",
		"execution_timeout",
		"integration_down",
		"resource_exhaustion",
		"unusual_activity",
	}

	// Start monitoring services
	go w.monitorWorkflowHealth(ctx)
	go w.monitorResourceUsage(ctx)
	go w.monitorIntegrationHealth(ctx)

	w.instance.Metadata["monitoring_dashboards"] = dashboards
	w.instance.Metadata["alert_rules"] = alertRules
	w.instance.Metadata["monitoring_enabled"] = true

	// Emit monitoring enabled event
	w.emitEvent(ctx, "automation.monitoring.enabled", map[string]interface{}{
		"dashboards":  len(dashboards),
		"alert_rules": len(alertRules),
		"enabled":     true,
	})

	return nil
}

// Helper methods
func (w *AutomationEngineWorkflow) setupScheduledTrigger(ctx context.Context, trigger *WorkflowTrigger) error {
	// Parse trigger configuration for cron expression
	var config map[string]interface{}
	if err := json.Unmarshal(trigger.Configuration, &config); err != nil {
		return errors.Wrap(err, "failed to parse trigger configuration")
	}

	cronExpr, ok := config["cron"].(string)
	if !ok {
		return errors.New("no cron expression found in trigger configuration")
	}

	// Add cron job
	_, err := w.cronScheduler.AddFunc(cronExpr, func() {
		// Execute workflow when trigger fires
		if trigger.IsEnabled {
			execution, err := w.executionManager.StartExecution(ctx, trigger.WorkflowID, map[string]interface{}{
				"triggered_by": "schedule",
				"trigger_id":   trigger.ID,
				"timestamp":    time.Now(),
			})
			if err != nil {
				log.Printf("Failed to start scheduled execution: %v", err)
			} else {
				log.Printf("Started scheduled execution %s for workflow %s", execution.ID, trigger.WorkflowID)
			}
		}
	})

	return err
}

func (w *AutomationEngineWorkflow) createBuiltinTemplates() []*WorkflowTemplate {
	templates := []*WorkflowTemplate{
		{
			ID:          uuid.New(),
			Name:        "Data Processing Pipeline",
			Category:    "data",
			Description: "Extract, transform, and load data from various sources",
			Popular:     true,
		},
		{
			ID:          uuid.New(),
			Name:        "Notification Workflow",
			Category:    "communication",
			Description: "Send notifications based on events and conditions",
			Popular:     true,
		},
		{
			ID:          uuid.New(),
			Name:        "Approval Process",
			Category:    "business",
			Description: "Multi-step approval workflow with conditions",
			Popular:     false,
		},
		{
			ID:          uuid.New(),
			Name:        "Integration Sync",
			Category:    "integration",
			Description: "Synchronize data between different services",
			Popular:     true,
		},
		{
			ID:          uuid.New(),
			Name:        "Error Handling Pattern",
			Category:    "utility",
			Description: "Handle errors gracefully with retry and fallback",
			Popular:     false,
		},
	}

	return templates
}

func (w *AutomationEngineWorkflow) createTestWorkflow() *WorkflowDefinition {
	return &WorkflowDefinition{
		ID:          uuid.New(),
		Name:        "Test Workflow",
		Description: "Simple test workflow to validate execution engine",
		Version:     "1.0.0",
		Steps: []*WorkflowStep{
			{
				ID:   uuid.New(),
				Name: "test_step",
				Type: "action",
				Configuration: json.RawMessage(`{
					"type": "log",
					"message": "Test workflow executed successfully"
				}`),
				Position: &StepPosition{X: 100, Y: 100},
			},
		},
		Variables: map[string]interface{}{
			"test_var": "test_value",
		},
		Settings: &WorkflowSettings{
			MaxConcurrentExecutions: 1,
			DefaultTimeout:          30 * time.Second,
		},
		CreatedBy: w.instance.UserID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func (w *AutomationEngineWorkflow) monitorExecutions(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Monitor running executions
			filter := ExecutionFilter{
				Status: "running",
				Limit:  100,
			}
			
			executions, err := w.executionManager.ListExecutions(ctx, filter)
			if err != nil {
				log.Printf("Failed to list running executions: %v", err)
				continue
			}

			// Check for timeouts and stuck executions
			now := time.Now()
			for _, execution := range executions {
				if execution.StartedAt.Add(w.config.ExecutionConfig.Timeout).Before(now) {
					log.Printf("Execution %s timed out, stopping", execution.ID)
					w.executionManager.StopExecution(ctx, execution.ID)
				}
			}
		}
	}
}

func (w *AutomationEngineWorkflow) collectExecutionMetrics(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Collect metrics for all workflows
			// This would typically involve querying the database and aggregating metrics
			log.Printf("Collecting execution metrics...")
		}
	}
}

func (w *AutomationEngineWorkflow) monitorWorkflowHealth(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Monitor workflow health metrics
			log.Printf("Monitoring workflow health...")
		}
	}
}

func (w *AutomationEngineWorkflow) monitorResourceUsage(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Monitor resource usage
			log.Printf("Monitoring resource usage...")
		}
	}
}

func (w *AutomationEngineWorkflow) monitorIntegrationHealth(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Test integration health
			if integrations, exists := w.instance.Metadata["setup_integrations"]; exists {
				integrationList := integrations.([]*Integration)
				for _, integration := range integrationList {
					if integration.Status == "active" {
						_, err := w.integrationManager.TestIntegration(ctx, integration.ID)
						if err != nil {
							log.Printf("Integration %s health check failed: %v", integration.Name, err)
						}
					}
				}
			}
		}
	}
}

func (w *AutomationEngineWorkflow) executeStep(ctx context.Context, stepName string, stepFunc AutomationStepFunc) error {
	// Create step record
	step := &WorkflowStep{
		ID:           uuid.New(),
		WorkflowID:   w.instance.ID,
		Name:         stepName,
		Type:         "setup",
		Position:     &StepPosition{X: 0, Y: 0},
	}

	// Execute step with retry logic
	var err error
	maxRetries := 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := time.Duration(attempt*attempt) * time.Second
			time.Sleep(delay)
		}

		err = stepFunc(ctx)
		if err == nil {
			break
		}

		if attempt < maxRetries {
			w.emitEvent(ctx, EventStepRetried, map[string]interface{}{
				"step":    step,
				"attempt": attempt + 1,
				"error":   err.Error(),
			})
		}
	}

	if err != nil {
		w.emitEvent(ctx, EventStepFailed, map[string]interface{}{
			"step":  step,
			"error": err.Error(),
		})
	} else {
		w.emitEvent(ctx, EventStepCompleted, map[string]interface{}{
			"step": step,
		})
	}

	// Add step to workflow instance
	w.instance.Steps = append(w.instance.Steps, *step)

	return err
}

func (w *AutomationEngineWorkflow) emitEvent(ctx context.Context, eventType string, data map[string]interface{}) error {
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

func (w *AutomationEngineWorkflow) getStepName(index int) string {
	stepNames := []string{
		"validate_configuration",
		"setup_workflow_builder",
		"initialize_trigger_system",
		"setup_action_executors",
		"configure_condition_evaluators",
		"setup_integrations",
		"load_workflow_templates",
		"start_execution_engine",
		"enable_monitoring",
	}
	
	if index < len(stepNames) {
		return stepNames[index]
	}
	return fmt.Sprintf("step_%d", index)
}

func (w *AutomationEngineWorkflow) isRecoverableError(err error) bool {
	errorStr := strings.ToLower(err.Error())
	recoverableErrors := []string{
		"timeout",
		"rate limit",
		"temporary",
		"network",
		"connection",
		"quota",
		"unavailable",
	}

	for _, recoverable := range recoverableErrors {
		if strings.Contains(errorStr, recoverable) {
			return true
		}
	}
	return false
}

func (w *AutomationEngineWorkflow) getAutomationStats() map[string]interface{} {
	stats := map[string]interface{}{
		"total_steps":       len(w.instance.Steps),
		"completed_steps":   w.instance.Progress.CompletedSteps,
		"failed_steps":      w.instance.Progress.FailedSteps,
		"success_rate":      float64(w.instance.Progress.CompletedSteps) / float64(len(w.instance.Steps)),
	}

	// Add trigger stats
	if triggers, exists := w.instance.Metadata["registered_triggers"]; exists {
		triggerList := triggers.([]*WorkflowTrigger)
		stats["triggers_registered"] = len(triggerList)
		
		enabledCount := 0
		for _, trigger := range triggerList {
			if trigger.IsEnabled {
				enabledCount++
			}
		}
		stats["triggers_enabled"] = enabledCount
	}

	// Add action stats
	if actions, exists := w.instance.Metadata["available_actions"]; exists {
		actionList := actions.([]*ActionDefinition)
		stats["available_actions"] = len(actionList)
	}
	
	if customActionsCount, exists := w.instance.Metadata["custom_actions_count"]; exists {
		stats["custom_actions"] = customActionsCount
	}

	// Add condition stats
	if operators, exists := w.instance.Metadata["supported_operators"]; exists {
		operatorList := operators.([]string)
		stats["supported_operators"] = len(operatorList)
	}
	
	if validConditionsCount, exists := w.instance.Metadata["valid_conditions_count"]; exists {
		stats["valid_conditions"] = validConditionsCount
	}

	// Add integration stats
	if integrations, exists := w.instance.Metadata["setup_integrations"]; exists {
		integrationList := integrations.([]*Integration)
		stats["integrations_setup"] = len(integrationList)
		
		activeCount := 0
		for _, integration := range integrationList {
			if integration.Status == "active" {
				activeCount++
			}
		}
		stats["integrations_active"] = activeCount
	}

	// Add template stats
	if templates, exists := w.instance.Metadata["loaded_templates"]; exists {
		templateList := templates.([]*WorkflowTemplate)
		stats["templates_loaded"] = len(templateList)
	}

	// Add execution stats
	if testWorkflowID, exists := w.instance.Metadata["test_workflow_id"]; exists {
		stats["test_workflow_created"] = testWorkflowID
	}
	
	if testExecutionID, exists := w.instance.Metadata["test_execution_id"]; exists {
		stats["test_execution_started"] = testExecutionID
	}

	// Add monitoring stats
	if monitoringEnabled, exists := w.instance.Metadata["monitoring_enabled"]; exists {
		stats["monitoring_enabled"] = monitoringEnabled
	}

	stats["cron_scheduler_running"] = w.cronScheduler != nil

	return stats
}