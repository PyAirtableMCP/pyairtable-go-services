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

// WorkflowOrchestrator is the main orchestrator for all workflow types
type WorkflowOrchestrator struct {
	eventStore           EventStore
	sagaOrchestrator     SagaOrchestrator
	cqrsService          CQRSService
	monitoringService    *WorkflowMonitoringService
	workflowRepository   WorkflowRepository
	executionManager     WorkflowExecutionManager
	
	// Workflow implementations
	airtableSyncFactory     AirtableSyncWorkflowFactory
	aiAnalyticsFactory      AIAnalyticsWorkflowFactory
	teamCollaborationFactory TeamCollaborationWorkflowFactory
	automationEngineFactory AutomationEngineWorkflowFactory
	dataPipelineFactory     DataPipelineWorkflowFactory
	
	// Execution tracking
	activeExecutions     map[uuid.UUID]*WorkflowExecution
	executionMutex       sync.RWMutex
	
	// Configuration
	config               *OrchestratorConfig
	mu                   sync.RWMutex
}

// SagaOrchestrator interface for SAGA pattern implementation
type SagaOrchestrator interface {
	StartSaga(ctx context.Context, sagaDefinition *SagaDefinition) (*SagaExecution, error)
	CompensateSaga(ctx context.Context, sagaID uuid.UUID, reason string) error
	GetSagaStatus(ctx context.Context, sagaID uuid.UUID) (*SagaStatus, error)
	RegisterSagaStep(ctx context.Context, sagaID uuid.UUID, step *SagaStep) error
}

// CQRSService interface for CQRS pattern implementation
type CQRSService interface {
	ExecuteCommand(ctx context.Context, command Command) (*CommandResult, error)
	QueryData(ctx context.Context, query Query) (*QueryResult, error)
	UpdateProjection(ctx context.Context, event *WorkflowEvent) error
	InvalidateCache(ctx context.Context, cacheKeys []string) error
}

// WorkflowRepository interface for workflow persistence
type WorkflowRepository interface {
	SaveWorkflow(ctx context.Context, instance *WorkflowInstance) error
	GetWorkflow(ctx context.Context, workflowID uuid.UUID) (*WorkflowInstance, error)
	ListWorkflows(ctx context.Context, filter WorkflowListFilter) ([]*WorkflowInstance, error)
	UpdateWorkflowStatus(ctx context.Context, workflowID uuid.UUID, status WorkflowStatus) error
	DeleteWorkflow(ctx context.Context, workflowID uuid.UUID) error
}

// WorkflowExecutionManager interface for execution management
type WorkflowExecutionManager interface {
	StartExecution(ctx context.Context, workflowID uuid.UUID, input map[string]interface{}) (*WorkflowExecution, error)
	StopExecution(ctx context.Context, executionID uuid.UUID) error
	PauseExecution(ctx context.Context, executionID uuid.UUID) error
	ResumeExecution(ctx context.Context, executionID uuid.UUID) error
	GetExecution(ctx context.Context, executionID uuid.UUID) (*WorkflowExecution, error)
	ListExecutions(ctx context.Context, filter ExecutionListFilter) ([]*WorkflowExecution, error)
	RetryExecution(ctx context.Context, executionID uuid.UUID) error
}

// Workflow factory interfaces
type AirtableSyncWorkflowFactory interface {
	CreateWorkflow(instance *WorkflowInstance, dependencies WorkflowDependencies) (WorkflowExecutor, error)
}

type AIAnalyticsWorkflowFactory interface {
	CreateWorkflow(instance *WorkflowInstance, dependencies WorkflowDependencies) (WorkflowExecutor, error)
}

type TeamCollaborationWorkflowFactory interface {
	CreateWorkflow(instance *WorkflowInstance, dependencies WorkflowDependencies) (WorkflowExecutor, error)
}

type AutomationEngineWorkflowFactory interface {
	CreateWorkflow(instance *WorkflowInstance, dependencies WorkflowDependencies) (WorkflowExecutor, error)
}

type DataPipelineWorkflowFactory interface {
	CreateWorkflow(instance *WorkflowInstance, dependencies WorkflowDependencies) (WorkflowExecutor, error)
}

// WorkflowExecutor interface for executing workflows
type WorkflowExecutor interface {
	Execute(ctx context.Context) error
	Cancel(ctx context.Context) error
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	GetStatus() WorkflowStatus
	GetProgress() WorkflowProgress
}

// Domain entities
type OrchestratorConfig struct {
	MaxConcurrentWorkflows   int           `json:"max_concurrent_workflows"`
	DefaultTimeout           time.Duration `json:"default_timeout"`
	RetryAttempts           int           `json:"retry_attempts"`
	RetryDelay              time.Duration `json:"retry_delay"`
	EnableMonitoring        bool          `json:"enable_monitoring"`
	EnableSagaSupport       bool          `json:"enable_saga_support"`
	EnableCQRS              bool          `json:"enable_cqrs"`
	MetricsRetention        time.Duration `json:"metrics_retention"`
	EventRetention          time.Duration `json:"event_retention"`
}

type WorkflowDependencies struct {
	EventStore              EventStore
	MonitoringService       *WorkflowMonitoringService
	SagaOrchestrator        SagaOrchestrator
	CQRSService            CQRSService
	
	// External service clients
	AirtableClient         AirtableClient
	GeminiClient           GeminiClient
	DataConnector          DataConnector
	NotificationService    NotificationService
	
	// Additional dependencies based on workflow type
	ExternalDependencies   map[string]interface{}
}

type SagaDefinition struct {
	ID          uuid.UUID      `json:"id"`
	WorkflowID  uuid.UUID      `json:"workflow_id"`
	Name        string         `json:"name"`
	Steps       []*SagaStep    `json:"steps"`
	Timeout     time.Duration  `json:"timeout"`
	RetryPolicy *RetryPolicy   `json:"retry_policy"`
}

type SagaStep struct {
	ID              uuid.UUID                `json:"id"`
	Name            string                   `json:"name"`
	Action          string                   `json:"action"`
	CompensationAction string                `json:"compensation_action"`
	Parameters      map[string]interface{}   `json:"parameters"`
	Timeout         time.Duration            `json:"timeout"`
	RetryPolicy     *RetryPolicy             `json:"retry_policy"`
	DependsOn       []uuid.UUID              `json:"depends_on"`
}

type SagaExecution struct {
	ID          uuid.UUID      `json:"id"`
	SagaID      uuid.UUID      `json:"saga_id"`
	WorkflowID  uuid.UUID      `json:"workflow_id"`
	Status      string         `json:"status"`
	CurrentStep int            `json:"current_step"`
	CompletedSteps []uuid.UUID `json:"completed_steps"`
	FailedSteps []uuid.UUID    `json:"failed_steps"`
	StartedAt   time.Time      `json:"started_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Error       *WorkflowError `json:"error,omitempty"`
}

type SagaStatus struct {
	SagaID      uuid.UUID      `json:"saga_id"`
	Status      string         `json:"status"`
	Progress    float64        `json:"progress"`
	StepStatuses map[uuid.UUID]string `json:"step_statuses"`
	LastUpdated time.Time      `json:"last_updated"`
}

type Command struct {
	ID          uuid.UUID                `json:"id"`
	Type        string                   `json:"type"`
	AggregateID uuid.UUID                `json:"aggregate_id"`
	Data        map[string]interface{}   `json:"data"`
	Metadata    map[string]interface{}   `json:"metadata"`
	Timestamp   time.Time                `json:"timestamp"`
	UserID      uuid.UUID                `json:"user_id"`
	TenantID    uuid.UUID                `json:"tenant_id"`
}

type CommandResult struct {
	Success     bool                     `json:"success"`
	Events      []*WorkflowEvent         `json:"events"`
	Data        map[string]interface{}   `json:"data"`
	Error       *WorkflowError           `json:"error,omitempty"`
	Duration    time.Duration            `json:"duration"`
}

type Query struct {
	ID          uuid.UUID                `json:"id"`
	Type        string                   `json:"type"`
	Parameters  map[string]interface{}   `json:"parameters"`
	Filters     map[string]interface{}   `json:"filters"`
	Pagination  *Pagination              `json:"pagination,omitempty"`
	Timestamp   time.Time                `json:"timestamp"`
	TenantID    uuid.UUID                `json:"tenant_id"`
	UserID      uuid.UUID                `json:"user_id"`
}

type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Page   int `json:"page"`
}

type WorkflowListFilter struct {
	Types       []WorkflowType `json:"types,omitempty"`
	Statuses    []WorkflowStatus `json:"statuses,omitempty"`
	TenantID    *uuid.UUID     `json:"tenant_id,omitempty"`
	UserID      *uuid.UUID     `json:"user_id,omitempty"`
	CreatedAfter *time.Time    `json:"created_after,omitempty"`
	CreatedBefore *time.Time   `json:"created_before,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Limit       int            `json:"limit,omitempty"`
	Offset      int            `json:"offset,omitempty"`
}

type ExecutionListFilter struct {
	WorkflowIDs []uuid.UUID    `json:"workflow_ids,omitempty"`
	Statuses    []string       `json:"statuses,omitempty"`
	TenantID    *uuid.UUID     `json:"tenant_id,omitempty"`
	UserID      *uuid.UUID     `json:"user_id,omitempty"`
	StartedAfter *time.Time    `json:"started_after,omitempty"`
	StartedBefore *time.Time   `json:"started_before,omitempty"`
	Limit       int            `json:"limit,omitempty"`
	Offset      int            `json:"offset,omitempty"`
}

// NewWorkflowOrchestrator creates a new workflow orchestrator
func NewWorkflowOrchestrator(
	eventStore EventStore,
	sagaOrchestrator SagaOrchestrator,
	cqrsService CQRSService,
	monitoringService *WorkflowMonitoringService,
	workflowRepository WorkflowRepository,
	executionManager WorkflowExecutionManager,
	config *OrchestratorConfig,
) *WorkflowOrchestrator {
	return &WorkflowOrchestrator{
		eventStore:         eventStore,
		sagaOrchestrator:   sagaOrchestrator,
		cqrsService:        cqrsService,
		monitoringService:  monitoringService,
		workflowRepository: workflowRepository,
		executionManager:   executionManager,
		activeExecutions:   make(map[uuid.UUID]*WorkflowExecution),
		config:            config,
	}
}

// RegisterFactories registers workflow factories
func (o *WorkflowOrchestrator) RegisterFactories(
	airtableSyncFactory AirtableSyncWorkflowFactory,
	aiAnalyticsFactory AIAnalyticsWorkflowFactory,
	teamCollaborationFactory TeamCollaborationWorkflowFactory,
	automationEngineFactory AutomationEngineWorkflowFactory,
	dataPipelineFactory DataPipelineWorkflowFactory,
) {
	o.mu.Lock()
	defer o.mu.Unlock()
	
	o.airtableSyncFactory = airtableSyncFactory
	o.aiAnalyticsFactory = aiAnalyticsFactory
	o.teamCollaborationFactory = teamCollaborationFactory
	o.automationEngineFactory = automationEngineFactory
	o.dataPipelineFactory = dataPipelineFactory
}

// CreateWorkflow creates a new workflow instance
func (o *WorkflowOrchestrator) CreateWorkflow(ctx context.Context, request *CreateWorkflowRequest) (*WorkflowInstance, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Validate request
	if err := o.validateCreateRequest(request); err != nil {
		return nil, errors.Wrap(err, "invalid create workflow request")
	}

	// Create workflow instance
	instance := &WorkflowInstance{
		ID:            uuid.New(),
		Type:          request.Type,
		Status:        WorkflowStatusPending,
		TenantID:      request.TenantID,
		UserID:        request.UserID,
		Configuration: request.Configuration,
		Context:       request.Context,
		Steps:         make([]WorkflowStep, 0),
		Progress: WorkflowProgress{
			TotalSteps:      0,
			CompletedSteps:  0,
			FailedSteps:     0,
			PercentComplete: 0,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  request.Metadata,
	}

	// Save workflow instance
	if err := o.workflowRepository.SaveWorkflow(ctx, instance); err != nil {
		return nil, errors.Wrap(err, "failed to save workflow instance")
	}

	// Start monitoring if enabled
	if o.config.EnableMonitoring {
		monitoringConfig := &MonitoringConfiguration{
			CreatedBy:          request.UserID,
			AlertRecipients:    []string{}, // Would be configured from request
			EnableTracing:      true,
			EnableHealthChecks: true,
			MetricsRetention:   o.config.MetricsRetention,
			LogsRetention:      o.config.EventRetention,
		}
		
		if err := o.monitoringService.StartMonitoring(ctx, instance.ID, monitoringConfig); err != nil {
			log.Printf("Failed to start monitoring for workflow %s: %v", instance.ID, err)
		}
	}

	// Emit workflow created event
	o.emitEvent(ctx, EventWorkflowCreated, map[string]interface{}{
		"workflow_id":   instance.ID,
		"workflow_type": instance.Type,
		"tenant_id":     instance.TenantID,
		"user_id":       instance.UserID,
	})

	return instance, nil
}

// ExecuteWorkflow executes a workflow
func (o *WorkflowOrchestrator) ExecuteWorkflow(ctx context.Context, workflowID uuid.UUID, input map[string]interface{}) (*WorkflowExecution, error) {
	// Check concurrent execution limit
	o.executionMutex.RLock()
	activeCount := len(o.activeExecutions)
	o.executionMutex.RUnlock()

	if activeCount >= o.config.MaxConcurrentWorkflows {
		return nil, errors.New("maximum concurrent workflows exceeded")
	}

	// Get workflow instance
	instance, err := o.workflowRepository.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get workflow instance")
	}

	// Create execution
	execution, err := o.executionManager.StartExecution(ctx, workflowID, input)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start execution")
	}

	// Track active execution
	o.executionMutex.Lock()
	o.activeExecutions[execution.ID] = execution
	o.executionMutex.Unlock()

	// Create workflow executor
	executor, err := o.createWorkflowExecutor(instance, execution)
	if err != nil {
		o.cleanupExecution(execution.ID)
		return nil, errors.Wrap(err, "failed to create workflow executor")
	}

	// Start execution in background
	go o.runWorkflowExecution(ctx, execution, executor)

	return execution, nil
}

// GetWorkflowStatus returns the current status of a workflow
func (o *WorkflowOrchestrator) GetWorkflowStatus(ctx context.Context, workflowID uuid.UUID) (*WorkflowInstance, error) {
	return o.workflowRepository.GetWorkflow(ctx, workflowID)
}

// ListWorkflows returns a list of workflows
func (o *WorkflowOrchestrator) ListWorkflows(ctx context.Context, filter WorkflowListFilter) ([]*WorkflowInstance, error) {
	return o.workflowRepository.ListWorkflows(ctx, filter)
}

// CancelWorkflow cancels a running workflow
func (o *WorkflowOrchestrator) CancelWorkflow(ctx context.Context, workflowID uuid.UUID) error {
	// Find active execution
	o.executionMutex.RLock()
	var execution *WorkflowExecution
	for _, exec := range o.activeExecutions {
		if exec.WorkflowID == workflowID {
			execution = exec
			break
		}
	}
	o.executionMutex.RUnlock()

	if execution == nil {
		return errors.New("no active execution found for workflow")
	}

	// Cancel execution
	if err := o.executionManager.StopExecution(ctx, execution.ID); err != nil {
		return errors.Wrap(err, "failed to cancel execution")
	}

	// Update workflow status
	if err := o.workflowRepository.UpdateWorkflowStatus(ctx, workflowID, WorkflowStatusCancelled); err != nil {
		return errors.Wrap(err, "failed to update workflow status")
	}

	// Emit workflow cancelled event
	o.emitEvent(ctx, EventWorkflowCancelled, map[string]interface{}{
		"workflow_id":  workflowID,
		"execution_id": execution.ID,
	})

	return nil
}

// PauseWorkflow pauses a running workflow
func (o *WorkflowOrchestrator) PauseWorkflow(ctx context.Context, workflowID uuid.UUID) error {
	// Find active execution
	o.executionMutex.RLock()
	var execution *WorkflowExecution
	for _, exec := range o.activeExecutions {
		if exec.WorkflowID == workflowID {
			execution = exec
			break
		}
	}
	o.executionMutex.RUnlock()

	if execution == nil {
		return errors.New("no active execution found for workflow")
	}

	// Pause execution
	if err := o.executionManager.PauseExecution(ctx, execution.ID); err != nil {
		return errors.Wrap(err, "failed to pause execution")
	}

	return nil
}

// ResumeWorkflow resumes a paused workflow
func (o *WorkflowOrchestrator) ResumeWorkflow(ctx context.Context, workflowID uuid.UUID) error {
	// Find active execution
	o.executionMutex.RLock()
	var execution *WorkflowExecution
	for _, exec := range o.activeExecutions {
		if exec.WorkflowID == workflowID {
			execution = exec
			break
		}
	}
	o.executionMutex.RUnlock()

	if execution == nil {
		return errors.New("no active execution found for workflow")
	}

	// Resume execution
	if err := o.executionManager.ResumeExecution(ctx, execution.ID); err != nil {
		return errors.Wrap(err, "failed to resume execution")
	}

	return nil
}

// GetExecutionStatus returns the status of a workflow execution
func (o *WorkflowOrchestrator) GetExecutionStatus(ctx context.Context, executionID uuid.UUID) (*WorkflowExecution, error) {
	return o.executionManager.GetExecution(ctx, executionID)
}

// ListExecutions returns a list of workflow executions
func (o *WorkflowOrchestrator) ListExecutions(ctx context.Context, filter ExecutionListFilter) ([]*WorkflowExecution, error) {
	return o.executionManager.ListExecutions(ctx, filter)
}

// GetWorkflowMetrics returns metrics for a workflow
func (o *WorkflowOrchestrator) GetWorkflowMetrics(ctx context.Context, workflowID uuid.UUID, timeRange TimeRange) (*WorkflowMetrics, error) {
	if !o.config.EnableMonitoring {
		return nil, errors.New("monitoring is not enabled")
	}

	filter := MetricsFilter{
		WorkflowIDs: []uuid.UUID{workflowID},
		TimeRange:   &timeRange,
	}

	metrics, err := o.monitoringService.metricsCollector.GetMetrics(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get workflow metrics")
	}

	return o.aggregateWorkflowMetrics(metrics), nil
}

// Private methods

func (o *WorkflowOrchestrator) validateCreateRequest(request *CreateWorkflowRequest) error {
	if request.Type == "" {
		return errors.New("workflow type is required")
	}
	if request.TenantID == uuid.Nil {
		return errors.New("tenant ID is required")
	}
	if request.UserID == uuid.Nil {
		return errors.New("user ID is required")
	}
	if len(request.Configuration) == 0 {
		return errors.New("workflow configuration is required")
	}

	// Validate workflow type
	validTypes := []WorkflowType{
		WorkflowTypeAirtableSync,
		WorkflowTypeAIAnalytics,
		WorkflowTypeTeamCollaboration,
		WorkflowTypeAutomationEngine,
		WorkflowTypeDataPipeline,
	}

	valid := false
	for _, validType := range validTypes {
		if request.Type == validType {
			valid = true
			break
		}
	}

	if !valid {
		return errors.Errorf("invalid workflow type: %s", request.Type)
	}

	return nil
}

func (o *WorkflowOrchestrator) createWorkflowExecutor(instance *WorkflowInstance, execution *WorkflowExecution) (WorkflowExecutor, error) {
	dependencies := WorkflowDependencies{
		EventStore:         o.eventStore,
		MonitoringService:  o.monitoringService,
		SagaOrchestrator:   o.sagaOrchestrator,
		CQRSService:        o.cqrsService,
		ExternalDependencies: make(map[string]interface{}),
	}

	switch instance.Type {
	case WorkflowTypeAirtableSync:
		if o.airtableSyncFactory == nil {
			return nil, errors.New("airtable sync factory not registered")
		}
		return o.airtableSyncFactory.CreateWorkflow(instance, dependencies)
		
	case WorkflowTypeAIAnalytics:
		if o.aiAnalyticsFactory == nil {
			return nil, errors.New("AI analytics factory not registered")
		}
		return o.aiAnalyticsFactory.CreateWorkflow(instance, dependencies)
		
	case WorkflowTypeTeamCollaboration:
		if o.teamCollaborationFactory == nil {
			return nil, errors.New("team collaboration factory not registered")
		}
		return o.teamCollaborationFactory.CreateWorkflow(instance, dependencies)
		
	case WorkflowTypeAutomationEngine:
		if o.automationEngineFactory == nil {
			return nil, errors.New("automation engine factory not registered")
		}
		return o.automationEngineFactory.CreateWorkflow(instance, dependencies)
		
	case WorkflowTypeDataPipeline:
		if o.dataPipelineFactory == nil {
			return nil, errors.New("data pipeline factory not registered")
		}
		return o.dataPipelineFactory.CreateWorkflow(instance, dependencies)
		
	default:
		return nil, errors.Errorf("unsupported workflow type: %s", instance.Type)
	}
}

func (o *WorkflowOrchestrator) runWorkflowExecution(ctx context.Context, execution *WorkflowExecution, executor WorkflowExecutor) {
	defer o.cleanupExecution(execution.ID)

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, o.config.DefaultTimeout)
	defer cancel()

	// Start trace if monitoring is enabled
	var traceCtx *TraceContext
	if o.config.EnableMonitoring {
		traceCtx, _ = o.monitoringService.StartTrace(timeoutCtx, execution.WorkflowID, "workflow_execution")
	}

	// Update execution status
	execution.Status = "running"
	execution.StartedAt = time.Now()

	// Log execution start
	if o.config.EnableMonitoring {
		o.monitoringService.LogWorkflowEvent(timeoutCtx, execution.WorkflowID, "info", "Workflow execution started", map[string]interface{}{
			"execution_id": execution.ID,
		})
	}

	// Execute workflow with retry logic
	var err error
	for attempt := 0; attempt <= o.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(o.config.RetryDelay)
			log.Printf("Retrying workflow execution %s, attempt %d", execution.ID, attempt+1)
		}

		err = executor.Execute(timeoutCtx)
		if err == nil {
			break
		}

		if !o.isRetryableError(err) {
			break
		}
	}

	// Update execution status
	completedAt := time.Now()
	execution.CompletedAt = &completedAt
	duration := completedAt.Sub(execution.StartedAt)
	execution.Duration = &duration

	if err != nil {
		execution.Status = "failed"
		execution.Error = &WorkflowError{
			Code:        "EXECUTION_FAILED",
			Message:     err.Error(),
			Timestamp:   time.Now(),
			Recoverable: o.isRetryableError(err),
		}

		// Log execution failure
		if o.config.EnableMonitoring {
			o.monitoringService.LogWorkflowEvent(timeoutCtx, execution.WorkflowID, "error", "Workflow execution failed", map[string]interface{}{
				"execution_id": execution.ID,
				"error":        err.Error(),
				"duration":     duration.String(),
			})
		}

		// Emit workflow failed event
		o.emitEvent(timeoutCtx, EventWorkflowFailed, map[string]interface{}{
			"workflow_id":  execution.WorkflowID,
			"execution_id": execution.ID,
			"error":        execution.Error,
			"duration":     duration.String(),
		})
	} else {
		execution.Status = "completed"

		// Log execution success
		if o.config.EnableMonitoring {
			o.monitoringService.LogWorkflowEvent(timeoutCtx, execution.WorkflowID, "info", "Workflow execution completed", map[string]interface{}{
				"execution_id": execution.ID,
				"duration":     duration.String(),
			})
		}

		// Emit workflow completed event
		o.emitEvent(timeoutCtx, EventWorkflowCompleted, map[string]interface{}{
			"workflow_id":  execution.WorkflowID,
			"execution_id": execution.ID,
			"duration":     duration.String(),
		})
	}

	// End trace
	if traceCtx != nil {
		result := TraceResult{
			Status: execution.Status,
		}
		if execution.Error != nil {
			result.Error = &TraceError{
				Type:    execution.Error.Code,
				Message: execution.Error.Message,
			}
		}
		o.monitoringService.EndTrace(timeoutCtx, traceCtx.TraceID, result)
	}

	// Collect metrics
	if o.config.EnableMonitoring {
		metrics := map[string]float64{
			"execution_duration": duration.Seconds(),
			"execution_success":  func() float64 { if err == nil { return 1 } else { return 0 } }(),
		}
		o.monitoringService.CollectWorkflowMetrics(timeoutCtx, execution.WorkflowID, execution.ID, metrics)
	}

	// Update workflow status
	if err != nil {
		o.workflowRepository.UpdateWorkflowStatus(timeoutCtx, execution.WorkflowID, WorkflowStatusFailed)
	} else {
		o.workflowRepository.UpdateWorkflowStatus(timeoutCtx, execution.WorkflowID, WorkflowStatusCompleted)
	}
}

func (o *WorkflowOrchestrator) cleanupExecution(executionID uuid.UUID) {
	o.executionMutex.Lock()
	defer o.executionMutex.Unlock()
	delete(o.activeExecutions, executionID)
}

func (o *WorkflowOrchestrator) isRetryableError(err error) bool {
	errorStr := err.Error()
	retryableErrors := []string{
		"timeout",
		"rate limit",
		"temporary",
		"network",
		"connection",
		"unavailable",
	}

	for _, retryable := range retryableErrors {
		if contains(errorStr, retryable) {
			return true
		}
	}
	return false
}

func (o *WorkflowOrchestrator) emitEvent(ctx context.Context, eventType string, data map[string]interface{}) error {
	eventData, _ := json.Marshal(data)
	
	event := &WorkflowEvent{
		ID:         uuid.New(),
		WorkflowID: uuid.Nil, // System event
		Type:       eventType,
		Version:    1,
		Data:       eventData,
		Metadata:   map[string]interface{}{"source": "workflow_orchestrator"},
		Timestamp:  time.Now(),
	}

	return o.eventStore.SaveEvent(ctx, event)
}

func (o *WorkflowOrchestrator) aggregateWorkflowMetrics(metrics []*WorkflowMetric) *WorkflowMetrics {
	// Aggregate metrics into a summary structure
	aggregated := &WorkflowMetrics{
		ExecutionCount: 0,
		SuccessRate:    0,
		AvgDuration:    0,
		ErrorRate:     0,
		Throughput:    0,
	}

	if len(metrics) == 0 {
		return aggregated
	}

	// Simple aggregation logic
	totalDuration := 0.0
	successCount := 0.0
	totalCount := 0.0

	for _, metric := range metrics {
		switch metric.MetricName {
		case "execution_count":
			aggregated.ExecutionCount += int64(metric.Value)
			totalCount += metric.Value
		case "execution_success":
			if metric.Value == 1 {
				successCount++
			}
		case "execution_duration":
			totalDuration += metric.Value
		}
	}

	if totalCount > 0 {
		aggregated.SuccessRate = successCount / totalCount
		aggregated.ErrorRate = 1 - aggregated.SuccessRate
		aggregated.AvgDuration = totalDuration / totalCount
	}

	return aggregated
}

// Request/Response types
type CreateWorkflowRequest struct {
	Type          WorkflowType           `json:"type"`
	TenantID      uuid.UUID              `json:"tenant_id"`
	UserID        uuid.UUID              `json:"user_id"`
	Configuration json.RawMessage        `json:"configuration"`
	Context       map[string]interface{} `json:"context"`
	Metadata      map[string]interface{} `json:"metadata"`
}

type WorkflowMetrics struct {
	ExecutionCount int64   `json:"execution_count"`
	SuccessRate    float64 `json:"success_rate"`
	AvgDuration    float64 `json:"avg_duration"`
	ErrorRate      float64 `json:"error_rate"`
	Throughput     float64 `json:"throughput"`
}