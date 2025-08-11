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

// AirtableSyncWorkflow implements advanced multi-base synchronization
type AirtableSyncWorkflow struct {
	instance        *WorkflowInstance
	config          *AirtableSyncConfig
	eventStore      EventStore
	airtableClient  AirtableClient
	conflictResolver ConflictResolver
	webhookManager  WebhookManager
	progressTracker ProgressTracker
	mu              sync.RWMutex
}

// EventStore interface for event sourcing
type EventStore interface {
	SaveEvent(ctx context.Context, event *WorkflowEvent) error
	LoadEvents(ctx context.Context, workflowID uuid.UUID) ([]*WorkflowEvent, error)
}

// AirtableClient interface for Airtable API operations
type AirtableClient interface {
	GetBases(ctx context.Context, config AirtableBaseConfig) ([]*AirtableBase, error)
	GetRecords(ctx context.Context, baseID, tableID string, config AirtableBaseConfig) ([]*AirtableRecord, error)
	CreateRecords(ctx context.Context, baseID, tableID string, records []*AirtableRecord, config AirtableBaseConfig) error
	UpdateRecords(ctx context.Context, baseID, tableID string, records []*AirtableRecord, config AirtableBaseConfig) error
	DeleteRecords(ctx context.Context, baseID, tableID string, recordIDs []string, config AirtableBaseConfig) error
	GetSchema(ctx context.Context, baseID string, config AirtableBaseConfig) (*AirtableSchema, error)
	DetectSchemaChanges(ctx context.Context, baseID string, lastSchema *AirtableSchema, config AirtableBaseConfig) (*SchemaChanges, error)
}

// ConflictResolver interface for handling sync conflicts
type ConflictResolver interface {
	ResolveConflicts(ctx context.Context, conflicts []*SyncConflict, strategy ConflictResolutionStrategy) ([]*ConflictResolution, error)
}

// WebhookManager interface for webhook processing
type WebhookManager interface {
	RegisterWebhook(ctx context.Context, config WebhookConfig) (*Webhook, error)
	ProcessWebhookEvent(ctx context.Context, event *WebhookEvent) error
	DeduplicateEvents(ctx context.Context, events []*WebhookEvent) ([]*WebhookEvent, error)
}

// ProgressTracker interface for tracking sync progress
type ProgressTracker interface {
	StartTracking(ctx context.Context, workflowID uuid.UUID, totalItems int) error
	UpdateProgress(ctx context.Context, workflowID uuid.UUID, completedItems int) error
	GetProgress(ctx context.Context, workflowID uuid.UUID) (*SyncProgress, error)
}

// Domain entities
type AirtableBase struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Tables []*AirtableTable  `json:"tables"`
}

type AirtableTable struct {
	ID     string             `json:"id"`
	Name   string             `json:"name"`
	Fields []*AirtableField   `json:"fields"`
}

type AirtableField struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type AirtableRecord struct {
	ID        string                 `json:"id"`
	Fields    map[string]interface{} `json:"fields"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

type AirtableSchema struct {
	Version   int                    `json:"version"`
	Tables    map[string]*AirtableTable `json:"tables"`
	Timestamp time.Time              `json:"timestamp"`
}

type SchemaChanges struct {
	AddedTables    []*AirtableTable  `json:"added_tables"`
	RemovedTables  []*AirtableTable  `json:"removed_tables"`
	ModifiedTables []*TableChange    `json:"modified_tables"`
}

type TableChange struct {
	TableID      string             `json:"table_id"`
	AddedFields  []*AirtableField   `json:"added_fields"`
	RemovedFields []*AirtableField  `json:"removed_fields"`
	ModifiedFields []*FieldChange   `json:"modified_fields"`
}

type FieldChange struct {
	FieldID   string `json:"field_id"`
	OldType   string `json:"old_type"`
	NewType   string `json:"new_type"`
	Breaking  bool   `json:"breaking"`
}

type SyncConflict struct {
	ID            uuid.UUID              `json:"id"`
	Type          string                 `json:"type"`
	SourceRecord  *AirtableRecord        `json:"source_record"`
	TargetRecord  *AirtableRecord        `json:"target_record"`
	ConflictData  map[string]interface{} `json:"conflict_data"`
	Priority      int                    `json:"priority"`
}

type ConflictResolution struct {
	ConflictID    uuid.UUID              `json:"conflict_id"`
	Resolution    string                 `json:"resolution"`
	ResolvedData  map[string]interface{} `json:"resolved_data"`
	Reasoning     string                 `json:"reasoning"`
}

type Webhook struct {
	ID      uuid.UUID     `json:"id"`
	URL     string        `json:"url"`
	Secret  string        `json:"secret"`
	Events  []string      `json:"events"`
	Active  bool          `json:"active"`
}

type WebhookEvent struct {
	ID        uuid.UUID              `json:"id"`
	Type      string                 `json:"type"`
	BaseID    string                 `json:"base_id"`
	TableID   string                 `json:"table_id"`
	RecordID  string                 `json:"record_id"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
	Signature string                 `json:"signature"`
}

type SyncProgress struct {
	WorkflowID     uuid.UUID `json:"workflow_id"`
	TotalItems     int       `json:"total_items"`
	CompletedItems int       `json:"completed_items"`
	FailedItems    int       `json:"failed_items"`
	PercentComplete float64  `json:"percent_complete"`
	EstimatedCompletion *time.Time `json:"estimated_completion,omitempty"`
	CurrentOperation string   `json:"current_operation"`
}

// NewAirtableSyncWorkflow creates a new Airtable sync workflow instance
func NewAirtableSyncWorkflow(
	instance *WorkflowInstance,
	eventStore EventStore,
	airtableClient AirtableClient,
	conflictResolver ConflictResolver,
	webhookManager WebhookManager,
	progressTracker ProgressTracker,
) (*AirtableSyncWorkflow, error) {
	var config AirtableSyncConfig
	if err := json.Unmarshal(instance.Configuration, &config); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal airtable sync configuration")
	}

	return &AirtableSyncWorkflow{
		instance:        instance,
		config:          &config,
		eventStore:      eventStore,
		airtableClient:  airtableClient,
		conflictResolver: conflictResolver,
		webhookManager:  webhookManager,
		progressTracker: progressTracker,
	}, nil
}

// Execute runs the complete Airtable synchronization workflow
func (w *AirtableSyncWorkflow) Execute(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Emit workflow started event
	if err := w.emitEvent(ctx, EventWorkflowStarted, map[string]interface{}{
		"workflow_type": WorkflowTypeAirtableSync,
		"config":        w.config,
	}); err != nil {
		return errors.Wrap(err, "failed to emit workflow started event")
	}

	// Update workflow status
	w.instance.Status = WorkflowStatusRunning
	now := time.Now()
	w.instance.StartedAt = &now

	// Execute workflow steps
	steps := []WorkflowStepFunc{
		w.validateConfiguration,
		w.detectSchemaChanges,
		w.setupWebhooks,
		w.performInitialSync,
		w.enableRealTimeSync,
		w.startProgressTracking,
	}

	for i, step := range steps {
		w.instance.CurrentStepIndex = i
		stepName := w.getStepName(i)
		
		if err := w.executeStep(ctx, stepName, step); err != nil {
			w.instance.Status = WorkflowStatusFailed
			w.instance.Error = &WorkflowError{
				Code:        "STEP_EXECUTION_FAILED",
				Message:     fmt.Sprintf("Step %s failed: %v", stepName, err),
				Timestamp:   time.Now(),
				Recoverable: w.isRecoverableError(err),
			}
			
			// Emit workflow failed event
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
		"stats":    w.getSyncStats(),
	})
}

type WorkflowStepFunc func(ctx context.Context) error

// Workflow step implementations
func (w *AirtableSyncWorkflow) validateConfiguration(ctx context.Context) error {
	if len(w.config.SourceBases) == 0 {
		return errors.New("no source bases configured")
	}
	if len(w.config.TargetBases) == 0 {
		return errors.New("no target bases configured")
	}
	if len(w.config.SyncRules) == 0 {
		return errors.New("no sync rules configured")
	}

	// Validate each base configuration
	for _, base := range append(w.config.SourceBases, w.config.TargetBases...) {
		if base.BaseID == "" || base.APIKey == "" {
			return errors.New("invalid base configuration: missing BaseID or APIKey")
		}
	}

	return nil
}

func (w *AirtableSyncWorkflow) detectSchemaChanges(ctx context.Context) error {
	var allChanges []*SchemaChanges

	for _, base := range w.config.SourceBases {
		// Get current schema
		currentSchema, err := w.airtableClient.GetSchema(ctx, base.BaseID, base)
		if err != nil {
			return errors.Wrapf(err, "failed to get schema for base %s", base.BaseID)
		}

		// Check for changes if we have a previous schema
		if base.LastSyncAt != nil {
			// In a real implementation, you'd load the previous schema from storage
			var lastSchema *AirtableSchema // Would be loaded from storage
			
			changes, err := w.airtableClient.DetectSchemaChanges(ctx, base.BaseID, lastSchema, base)
			if err != nil {
				return errors.Wrapf(err, "failed to detect schema changes for base %s", base.BaseID)
			}

			if changes != nil {
				allChanges = append(allChanges, changes)
				
				// Emit schema change event
				w.emitEvent(ctx, "workflow.schema.changed", map[string]interface{}{
					"base_id": base.BaseID,
					"changes": changes,
				})
			}
		}
	}

	// Handle breaking changes
	if len(allChanges) > 0 {
		return w.handleSchemaChanges(ctx, allChanges)
	}

	return nil
}

func (w *AirtableSyncWorkflow) setupWebhooks(ctx context.Context) error {
	if !w.config.WebhookConfig.Enabled {
		return nil // Skip if webhooks are disabled
	}

	for _, base := range w.config.SourceBases {
		webhook, err := w.webhookManager.RegisterWebhook(ctx, w.config.WebhookConfig)
		if err != nil {
			return errors.Wrapf(err, "failed to setup webhook for base %s", base.BaseID)
		}

		// Store webhook info in context
		w.instance.Context[fmt.Sprintf("webhook_%s", base.BaseID)] = webhook.ID.String()
	}

	return nil
}

func (w *AirtableSyncWorkflow) performInitialSync(ctx context.Context) error {
	totalRecords := 0
	
	// Calculate total records for progress tracking
	for _, base := range w.config.SourceBases {
		for _, table := range base.Tables {
			records, err := w.airtableClient.GetRecords(ctx, base.BaseID, table, base)
			if err != nil {
				return errors.Wrapf(err, "failed to count records in base %s, table %s", base.BaseID, table)
			}
			totalRecords += len(records)
		}
	}

	// Start progress tracking
	if err := w.progressTracker.StartTracking(ctx, w.instance.ID, totalRecords); err != nil {
		return errors.Wrap(err, "failed to start progress tracking")
	}

	completedRecords := 0
	
	// Process each sync rule
	for _, rule := range w.config.SyncRules {
		if err := w.executeSyncRule(ctx, rule, &completedRecords); err != nil {
			return errors.Wrapf(err, "failed to execute sync rule: %s -> %s", rule.SourceTable, rule.TargetTable)
		}
	}

	return nil
}

func (w *AirtableSyncWorkflow) executeSyncRule(ctx context.Context, rule SyncRule, completedRecords *int) error {
	// Find source and target bases
	var sourceBase, targetBase *AirtableBaseConfig
	
	for _, base := range w.config.SourceBases {
		if w.containsTable(base.Tables, rule.SourceTable) {
			sourceBase = &base
			break
		}
	}
	
	for _, base := range w.config.TargetBases {
		if w.containsTable(base.Tables, rule.TargetTable) {
			targetBase = &base
			break
		}
	}

	if sourceBase == nil || targetBase == nil {
		return errors.Errorf("could not find source or target base for rule %s -> %s", rule.SourceTable, rule.TargetTable)
	}

	// Get source records
	sourceRecords, err := w.airtableClient.GetRecords(ctx, sourceBase.BaseID, rule.SourceTable, *sourceBase)
	if err != nil {
		return errors.Wrap(err, "failed to get source records")
	}

	// Get target records for conflict detection
	targetRecords, err := w.airtableClient.GetRecords(ctx, targetBase.BaseID, rule.TargetTable, *targetBase)
	if err != nil {
		return errors.Wrap(err, "failed to get target records")
	}

	// Process records in batches
	batchSize := w.config.BatchSize
	if batchSize <= 0 {
		batchSize = 100 // Default batch size
	}

	for i := 0; i < len(sourceRecords); i += batchSize {
		end := i + batchSize
		if end > len(sourceRecords) {
			end = len(sourceRecords)
		}

		batch := sourceRecords[i:end]
		
		// Transform records according to field mapping
		transformedRecords, err := w.transformRecords(batch, rule)
		if err != nil {
			return errors.Wrap(err, "failed to transform records")
		}

		// Detect conflicts
		conflicts := w.detectConflicts(transformedRecords, targetRecords)
		
		// Resolve conflicts if any
		if len(conflicts) > 0 {
			resolutions, err := w.conflictResolver.ResolveConflicts(ctx, conflicts, w.config.ConflictResolution)
			if err != nil {
				return errors.Wrap(err, "failed to resolve conflicts")
			}
			
			// Apply conflict resolutions
			transformedRecords = w.applyConflictResolutions(transformedRecords, resolutions)
		}

		// Sync records to target
		if err := w.syncRecordsToTarget(ctx, transformedRecords, targetBase, rule.TargetTable); err != nil {
			return errors.Wrap(err, "failed to sync records to target")
		}

		// Update progress
		*completedRecords += len(batch)
		w.progressTracker.UpdateProgress(ctx, w.instance.ID, *completedRecords)
	}

	return nil
}

func (w *AirtableSyncWorkflow) enableRealTimeSync(ctx context.Context) error {
	if !w.config.RealTimeSync {
		return nil // Skip if real-time sync is disabled
	}

	// Start webhook event processing goroutine
	go w.processWebhookEvents(ctx)

	return nil
}

func (w *AirtableSyncWorkflow) startProgressTracking(ctx context.Context) error {
	if !w.config.ProgressTracking {
		return nil
	}

	// Start periodic progress updates
	go w.trackProgressPeriodically(ctx)

	return nil
}

// Helper methods
func (w *AirtableSyncWorkflow) executeStep(ctx context.Context, stepName string, stepFunc WorkflowStepFunc) error {
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
			Code:        "STEP_EXECUTION_ERROR",
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

func (w *AirtableSyncWorkflow) emitEvent(ctx context.Context, eventType string, data map[string]interface{}) error {
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

func (w *AirtableSyncWorkflow) getStepName(index int) string {
	stepNames := []string{
		"validate_configuration",
		"detect_schema_changes",
		"setup_webhooks",
		"perform_initial_sync",
		"enable_real_time_sync",
		"start_progress_tracking",
	}
	
	if index < len(stepNames) {
		return stepNames[index]
	}
	return fmt.Sprintf("step_%d", index)
}

func (w *AirtableSyncWorkflow) isRecoverableError(err error) bool {
	// Determine if error is recoverable based on error type/message
	// This is a simplified implementation
	errorStr := err.Error()
	recoverableErrors := []string{
		"timeout",
		"rate limit",
		"temporary",
		"network",
		"connection",
	}

	for _, recoverable := range recoverableErrors {
		if contains(errorStr, recoverable) {
			return true
		}
	}
	return false
}

func (w *AirtableSyncWorkflow) handleSchemaChanges(ctx context.Context, changes []*SchemaChanges) error {
	// Handle schema migration logic
	// This is a complex operation that would involve:
	// 1. Analyzing breaking changes
	// 2. Creating migration plan
	// 3. Updating sync rules
	// 4. Migrating existing data
	
	log.Printf("Handling schema changes: %+v", changes)
	return nil
}

func (w *AirtableSyncWorkflow) containsTable(tables []string, table string) bool {
	for _, t := range tables {
		if t == table {
			return true
		}
	}
	return false
}

func (w *AirtableSyncWorkflow) transformRecords(records []*AirtableRecord, rule SyncRule) ([]*AirtableRecord, error) {
	transformed := make([]*AirtableRecord, len(records))
	
	for i, record := range records {
		newRecord := &AirtableRecord{
			ID:        record.ID,
			Fields:    make(map[string]interface{}),
			CreatedAt: record.CreatedAt,
			UpdatedAt: record.UpdatedAt,
		}

		// Apply field mapping
		for sourceField, targetField := range rule.FieldMapping {
			if value, exists := record.Fields[sourceField]; exists {
				newRecord.Fields[targetField] = value
			}
		}

		transformed[i] = newRecord
	}

	return transformed, nil
}

func (w *AirtableSyncWorkflow) detectConflicts(sourceRecords, targetRecords []*AirtableRecord) []*SyncConflict {
	var conflicts []*SyncConflict
	
	// Create a map of target records for quick lookup
	targetMap := make(map[string]*AirtableRecord)
	for _, record := range targetRecords {
		targetMap[record.ID] = record
	}

	// Check for conflicts
	for _, sourceRecord := range sourceRecords {
		if targetRecord, exists := targetMap[sourceRecord.ID]; exists {
			// Check if records are different
			if !w.recordsEqual(sourceRecord, targetRecord) {
				conflict := &SyncConflict{
					ID:           uuid.New(),
					Type:         "update_conflict",
					SourceRecord: sourceRecord,
					TargetRecord: targetRecord,
					Priority:     w.calculateConflictPriority(sourceRecord, targetRecord),
				}
				conflicts = append(conflicts, conflict)
			}
		}
	}

	return conflicts
}

func (w *AirtableSyncWorkflow) recordsEqual(r1, r2 *AirtableRecord) bool {
	// Simple field comparison - in practice, this would be more sophisticated
	if len(r1.Fields) != len(r2.Fields) {
		return false
	}

	for key, value := range r1.Fields {
		if r2.Fields[key] != value {
			return false
		}
	}

	return true
}

func (w *AirtableSyncWorkflow) calculateConflictPriority(source, target *AirtableRecord) int {
	// Calculate priority based on record timestamps and other factors
	if source.UpdatedAt.After(target.UpdatedAt) {
		return 1 // Source is newer
	}
	return 0 // Target is newer or same
}

func (w *AirtableSyncWorkflow) applyConflictResolutions(records []*AirtableRecord, resolutions []*ConflictResolution) []*AirtableRecord {
	// Apply conflict resolutions to records
	// This is a simplified implementation
	return records
}

func (w *AirtableSyncWorkflow) syncRecordsToTarget(ctx context.Context, records []*AirtableRecord, targetBase *AirtableBaseConfig, targetTable string) error {
	// Sync records to target base
	return w.airtableClient.CreateRecords(ctx, targetBase.BaseID, targetTable, records, *targetBase)
}

func (w *AirtableSyncWorkflow) processWebhookEvents(ctx context.Context) {
	// Process webhook events in real-time
	// This would typically involve subscribing to webhook events and processing them
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Process webhook events
			time.Sleep(1 * time.Second)
		}
	}
}

func (w *AirtableSyncWorkflow) trackProgressPeriodically(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Update progress tracking
			w.progressTracker.UpdateProgress(ctx, w.instance.ID, w.instance.Progress.CompletedSteps)
		}
	}
}

func (w *AirtableSyncWorkflow) getSyncStats() map[string]interface{} {
	return map[string]interface{}{
		"total_steps":     len(w.instance.Steps),
		"completed_steps": w.instance.Progress.CompletedSteps,
		"failed_steps":    w.instance.Progress.FailedSteps,
		"success_rate":    float64(w.instance.Progress.CompletedSteps) / float64(len(w.instance.Steps)),
	}
}

// Utility functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr
}