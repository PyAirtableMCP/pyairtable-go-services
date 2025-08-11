package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/pyairtable/go-services/plugin-service/internal/config"
	"github.com/pyairtable/go-services/plugin-service/internal/models"
	"github.com/pyairtable/go-services/plugin-service/internal/runtime"
	"github.com/pyairtable/go-services/plugin-service/internal/security"
)

// Service errors
var (
	ErrPluginNotFound       = errors.New("plugin not found")
	ErrInstallationNotFound = errors.New("installation not found")
	ErrUnauthorized         = errors.New("unauthorized")
	ErrForbidden            = errors.New("forbidden")
	ErrValidationFailed     = errors.New("validation failed")
	ErrPluginAlreadyExists  = errors.New("plugin already exists")
	ErrInvalidVersion       = errors.New("invalid version")
	ErrResourceExhausted    = errors.New("resource exhausted")
)

// Repository interfaces
type PluginRepository interface {
	Create(ctx context.Context, plugin *models.Plugin) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Plugin, error)
	GetByName(ctx context.Context, name string) (*models.Plugin, error)
	Update(ctx context.Context, plugin *models.Plugin) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, req *models.ListPluginsRequest) (*models.ListPluginsResponse, error)
	GetByDeveloper(ctx context.Context, developerID uuid.UUID) ([]*models.Plugin, error)
}

type InstallationRepository interface {
	Create(ctx context.Context, installation *models.PluginInstallation) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.PluginInstallation, error)
	GetByPluginAndWorkspace(ctx context.Context, pluginID, workspaceID uuid.UUID) (*models.PluginInstallation, error)
	Update(ctx context.Context, installation *models.PluginInstallation) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]*models.PluginInstallation, error)
	ListByPlugin(ctx context.Context, pluginID uuid.UUID) ([]*models.PluginInstallation, error)
}

type ExecutionRepository interface {
	Create(ctx context.Context, execution *models.PluginExecution) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.PluginExecution, error)
	ListByInstallation(ctx context.Context, installationID uuid.UUID, limit int) ([]*models.PluginExecution, error)
	GetStats(ctx context.Context, installationID uuid.UUID, since time.Time) (*ExecutionStats, error)
}

// PluginService provides plugin management operations
type PluginService struct {
	config               *config.Config
	logger               *zap.Logger
	pluginRepo           PluginRepository
	installationRepo     InstallationRepository
	executionRepo        ExecutionRepository
	wasmRuntime          *runtime.WASMRuntime
	securityManager      *security.SecurityManager
	registryService      *RegistryService
	notificationService  NotificationService
	auditLogger          AuditLogger
}

// NotificationService interface for sending notifications
type NotificationService interface {
	SendPluginNotification(ctx context.Context, notification *PluginNotification) error
}

// AuditLogger interface for audit logging
type AuditLogger interface {
	LogPluginEvent(ctx context.Context, event *AuditEvent) error
}

// PluginNotification represents a plugin-related notification
type PluginNotification struct {
	Type        string    `json:"type"`
	PluginID    uuid.UUID `json:"plugin_id"`
	UserID      uuid.UUID `json:"user_id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	Message     string    `json:"message"`
	Data        map[string]interface{} `json:"data"`
}

// AuditEvent represents an audit event
type AuditEvent struct {
	Type        string                 `json:"type"`
	PluginID    uuid.UUID             `json:"plugin_id"`
	UserID      uuid.UUID             `json:"user_id"`
	WorkspaceID uuid.UUID             `json:"workspace_id"`
	Action      string                `json:"action"`
	Resource    string                `json:"resource"`
	Timestamp   time.Time             `json:"timestamp"`
	IPAddress   string                `json:"ip_address,omitempty"`
	UserAgent   string                `json:"user_agent,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// ExecutionStats contains plugin execution statistics
type ExecutionStats struct {
	TotalExecutions   int64         `json:"total_executions"`
	SuccessfulRuns    int64         `json:"successful_runs"`
	FailedRuns        int64         `json:"failed_runs"`
	AverageTimeMs     float64       `json:"average_time_ms"`
	TotalResourceUsage ResourceUsage `json:"total_resource_usage"`
}

// ResourceUsage represents resource usage statistics
type ResourceUsage struct {
	TotalMemoryMB     int64 `json:"total_memory_mb"`
	TotalCPUTimeMs    int64 `json:"total_cpu_time_ms"`
	TotalNetworkReqs  int64 `json:"total_network_reqs"`
	TotalStorageMB    int64 `json:"total_storage_mb"`
}

// ServiceStats contains service-wide statistics
type ServiceStats struct {
	TotalPlugins         int64                          `json:"total_plugins"`
	ActiveInstallations  int64                          `json:"active_installations"`
	TotalExecutions      int64                          `json:"total_executions"`
	RuntimeStats         map[uuid.UUID]*runtime.InstanceStats `json:"runtime_stats"`
	ResourceUsage        ResourceUsage                  `json:"resource_usage"`
	QuarantinedPlugins   int64                          `json:"quarantined_plugins"`
}

// HealthStatus represents service health status
type HealthStatus struct {
	Status      string                 `json:"status"`
	Timestamp   time.Time             `json:"timestamp"`
	Version     string                `json:"version"`
	Components  map[string]string     `json:"components"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// NewPluginService creates a new plugin service
func NewPluginService(
	config *config.Config,
	logger *zap.Logger,
	pluginRepo PluginRepository,
	installationRepo InstallationRepository,
	executionRepo ExecutionRepository,
	wasmRuntime *runtime.WASMRuntime,
	securityManager *security.SecurityManager,
	registryService *RegistryService,
	notificationService NotificationService,
	auditLogger AuditLogger,
) *PluginService {
	return &PluginService{
		config:              config,
		logger:              logger,
		pluginRepo:          pluginRepo,
		installationRepo:    installationRepo,
		executionRepo:       executionRepo,
		wasmRuntime:         wasmRuntime,
		securityManager:     securityManager,
		registryService:     registryService,
		notificationService: notificationService,
		auditLogger:         auditLogger,
	}
}

// CreatePlugin creates a new plugin
func (s *PluginService) CreatePlugin(ctx context.Context, plugin *models.Plugin, userID uuid.UUID) (*models.Plugin, error) {
	s.logger.Info("Creating plugin", zap.String("name", plugin.Name), zap.String("user_id", userID.String()))

	// Set metadata
	plugin.ID = uuid.New()
	plugin.CreatedAt = time.Now()
	plugin.UpdatedAt = time.Now()
	plugin.Status = models.PluginStatusPending
	plugin.DeveloperID = userID

	// Calculate hash
	if len(plugin.WasmBinary) > 0 {
		plugin.WasmHash = s.calculateHash(plugin.WasmBinary)
	}

	// Validate plugin security
	workspaceID := uuid.Nil
	if plugin.WorkspaceID != nil {
		workspaceID = *plugin.WorkspaceID
	}

	securityResult, err := s.securityManager.ValidatePlugin(ctx, plugin, userID, workspaceID)
	if err != nil {
		s.logger.Error("Plugin security validation failed", zap.Error(err))
		return nil, fmt.Errorf("security validation failed: %w", err)
	}

	if !securityResult.Passed {
		s.logger.Warn("Plugin failed security validation", 
			zap.Strings("violations", securityResult.Violations))
		return nil, fmt.Errorf("plugin validation failed: %v", securityResult.Violations)
	}

	// Create plugin in repository
	if err := s.pluginRepo.Create(ctx, plugin); err != nil {
		s.logger.Error("Failed to create plugin", zap.Error(err))
		return nil, fmt.Errorf("failed to create plugin: %w", err)
	}

	// Log audit event
	s.auditLogger.LogPluginEvent(ctx, &AuditEvent{
		Type:        "plugin.created",
		PluginID:    plugin.ID,
		UserID:      userID,
		WorkspaceID: workspaceID,
		Action:      "create",
		Resource:    "plugin",
		Timestamp:   time.Now(),
		Details: map[string]interface{}{
			"plugin_name": plugin.Name,
			"plugin_type": plugin.Type,
			"version":     plugin.Version,
		},
	})

	// Send notification
	s.notificationService.SendPluginNotification(ctx, &PluginNotification{
		Type:        "plugin.created",
		PluginID:    plugin.ID,
		UserID:      userID,
		WorkspaceID: workspaceID,
		Message:     fmt.Sprintf("Plugin '%s' has been created", plugin.Name),
		Data: map[string]interface{}{
			"plugin_name": plugin.Name,
			"version":     plugin.Version,
		},
	})

	return plugin, nil
}

// GetPlugin retrieves a plugin by ID
func (s *PluginService) GetPlugin(ctx context.Context, pluginID, userID, workspaceID uuid.UUID) (*models.Plugin, error) {
	plugin, err := s.pluginRepo.GetByID(ctx, pluginID)
	if err != nil {
		return nil, ErrPluginNotFound
	}

	// Check permissions
	if !s.canAccessPlugin(plugin, userID, workspaceID) {
		return nil, ErrForbidden
	}

	return plugin, nil
}

// UpdatePlugin updates an existing plugin
func (s *PluginService) UpdatePlugin(ctx context.Context, pluginID uuid.UUID, updates *models.Plugin, userID, workspaceID uuid.UUID) (*models.Plugin, error) {
	// Get existing plugin
	plugin, err := s.pluginRepo.GetByID(ctx, pluginID)
	if err != nil {
		return nil, ErrPluginNotFound
	}

	// Check permissions
	if !s.canModifyPlugin(plugin, userID, workspaceID) {
		return nil, ErrForbidden
	}

	// Apply updates
	if updates.Description != "" {
		plugin.Description = updates.Description
	}
	if updates.LongDescription != "" {
		plugin.LongDescription = updates.LongDescription
	}
	if len(updates.Tags) > 0 {
		plugin.Tags = updates.Tags
	}
	if len(updates.Categories) > 0 {
		plugin.Categories = updates.Categories
	}
	if len(updates.WasmBinary) > 0 {
		plugin.WasmBinary = updates.WasmBinary
		plugin.WasmHash = s.calculateHash(updates.WasmBinary)
		
		// Re-validate security if binary changed
		securityResult, err := s.securityManager.ValidatePlugin(ctx, plugin, userID, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("security validation failed: %w", err)
		}
		if !securityResult.Passed {
			return nil, fmt.Errorf("plugin validation failed: %v", securityResult.Violations)
		}
	}

	plugin.UpdatedAt = time.Now()

	// Update in repository
	if err := s.pluginRepo.Update(ctx, plugin); err != nil {
		return nil, fmt.Errorf("failed to update plugin: %w", err)
	}

	// Log audit event
	s.auditLogger.LogPluginEvent(ctx, &AuditEvent{
		Type:        "plugin.updated",
		PluginID:    plugin.ID,
		UserID:      userID,
		WorkspaceID: workspaceID,
		Action:      "update",
		Resource:    "plugin",
		Timestamp:   time.Now(),
	})

	return plugin, nil
}

// DeletePlugin deletes a plugin
func (s *PluginService) DeletePlugin(ctx context.Context, pluginID, userID, workspaceID uuid.UUID) error {
	// Get plugin
	plugin, err := s.pluginRepo.GetByID(ctx, pluginID)
	if err != nil {
		return ErrPluginNotFound
	}

	// Check permissions
	if !s.canModifyPlugin(plugin, userID, workspaceID) {
		return ErrForbidden
	}

	// Check if plugin has active installations
	installations, err := s.installationRepo.ListByPlugin(ctx, pluginID)
	if err != nil {
		return fmt.Errorf("failed to check installations: %w", err)
	}

	activeInstallations := 0
	for _, installation := range installations {
		if installation.Status == models.PluginStatusActive {
			activeInstallations++
		}
	}

	if activeInstallations > 0 {
		return fmt.Errorf("cannot delete plugin with %d active installations", activeInstallations)
	}

	// Delete plugin
	if err := s.pluginRepo.Delete(ctx, pluginID); err != nil {
		return fmt.Errorf("failed to delete plugin: %w", err)
	}

	// Log audit event
	s.auditLogger.LogPluginEvent(ctx, &AuditEvent{
		Type:        "plugin.deleted",
		PluginID:    plugin.ID,
		UserID:      userID,
		WorkspaceID: workspaceID,
		Action:      "delete",
		Resource:    "plugin",
		Timestamp:   time.Now(),
	})

	return nil
}

// ListPlugins lists plugins with filtering and pagination
func (s *PluginService) ListPlugins(ctx context.Context, req *models.ListPluginsRequest, userID, workspaceID uuid.UUID) (*models.ListPluginsResponse, error) {
	// Apply workspace filter if not admin
	if req.WorkspaceID == nil {
		req.WorkspaceID = &workspaceID
	}

	return s.pluginRepo.List(ctx, req)
}

// InstallPlugin installs a plugin in a workspace
func (s *PluginService) InstallPlugin(ctx context.Context, pluginID uuid.UUID, version string, config json.RawMessage, userID, workspaceID uuid.UUID) (*models.PluginInstallation, error) {
	s.logger.Info("Installing plugin", 
		zap.String("plugin_id", pluginID.String()),
		zap.String("workspace_id", workspaceID.String()),
		zap.String("version", version))

	// Get plugin
	plugin, err := s.pluginRepo.GetByID(ctx, pluginID)
	if err != nil {
		return nil, ErrPluginNotFound
	}

	// Check if already installed
	existing, err := s.installationRepo.GetByPluginAndWorkspace(ctx, pluginID, workspaceID)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("plugin already installed in workspace")
	}

	// Validate plugin can be installed
	securityResult, err := s.securityManager.ValidatePlugin(ctx, plugin, userID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("security validation failed: %w", err)
	}
	if !securityResult.Passed || securityResult.Quarantined {
		return nil, fmt.Errorf("plugin cannot be installed: security validation failed")
	}

	// Create installation
	installation := &models.PluginInstallation{
		ID:          uuid.New(),
		PluginID:    pluginID,
		WorkspaceID: workspaceID,
		UserID:      userID,
		Version:     version,
		Status:      models.PluginStatusInstalled,
		Config:      config,
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.installationRepo.Create(ctx, installation); err != nil {
		return nil, fmt.Errorf("failed to create installation: %w", err)
	}

	// Load plugin into runtime
	if _, err := s.wasmRuntime.LoadPlugin(ctx, plugin, installation); err != nil {
		s.logger.Error("Failed to load plugin into runtime", zap.Error(err))
		// Update installation status to error
		installation.Status = models.PluginStatusError
		installation.LastError = &err.Error()
		s.installationRepo.Update(ctx, installation)
		return nil, fmt.Errorf("failed to load plugin: %w", err)
	}

	// Update status to active
	installation.Status = models.PluginStatusActive
	installation.LastActiveAt = &installation.InstalledAt
	if err := s.installationRepo.Update(ctx, installation); err != nil {
		s.logger.Error("Failed to update installation status", zap.Error(err))
	}

	// Log audit event
	s.auditLogger.LogPluginEvent(ctx, &AuditEvent{
		Type:        "plugin.installed",
		PluginID:    pluginID,
		UserID:      userID,
		WorkspaceID: workspaceID,
		Action:      "install",
		Resource:    "plugin",
		Timestamp:   time.Now(),
		Details: map[string]interface{}{
			"installation_id": installation.ID,
			"version":         version,
		},
	})

	return installation, nil
}

// ExecutePlugin executes a plugin function
func (s *PluginService) ExecutePlugin(ctx context.Context, installationID uuid.UUID, functionName string, input json.RawMessage, metadata map[string]interface{}, userID, workspaceID uuid.UUID) (*runtime.ExecutionResult, error) {
	// Get installation
	installation, err := s.installationRepo.GetByID(ctx, installationID)
	if err != nil {
		return nil, ErrInstallationNotFound
	}

	// Check permissions
	if installation.WorkspaceID != workspaceID {
		return nil, ErrForbidden
	}

	// Create execution context
	execCtx := &runtime.ExecutionContext{
		UserID:      userID,
		WorkspaceID: workspaceID,
		RequestID:   uuid.New().String(),
		Timestamp:   time.Now(),
		Input:       input,
		Metadata:    metadata,
	}

	// Execute plugin
	result, err := s.wasmRuntime.ExecutePlugin(ctx, installationID, functionName, execCtx)
	if err != nil {
		s.logger.Error("Plugin execution failed", zap.Error(err))
		return nil, fmt.Errorf("plugin execution failed: %w", err)
	}

	// Record execution
	execution := &models.PluginExecution{
		ID:              uuid.New(),
		PluginID:        installation.PluginID,
		InstallationID:  installationID,
		WorkspaceID:     workspaceID,
		UserID:          userID,
		TriggerType:     "api",
		TriggerData:     input,
		MemoryUsedMB:    result.ResourceUsage.MemoryMB,
		CPUTimeMs:       result.ResourceUsage.CPUTimeMs,
		ExecutionTimeMs: int32(result.ExecutionTimeMs),
		StartedAt:       execCtx.Timestamp,
	}

	if result.Success {
		execution.Status = "success"
		execution.Result = result.Result
		now := time.Now()
		execution.CompletedAt = &now
	} else {
		execution.Status = "error"
		execution.Error = &result.Error
		now := time.Now()
		execution.CompletedAt = &now
	}

	if err := s.executionRepo.Create(ctx, execution); err != nil {
		s.logger.Error("Failed to record execution", zap.Error(err))
	}

	return result, nil
}

// Helper methods

func (s *PluginService) canAccessPlugin(plugin *models.Plugin, userID, workspaceID uuid.UUID) bool {
	// Plugin owner can always access
	if plugin.DeveloperID == userID {
		return true
	}

	// Workspace members can access workspace plugins
	if plugin.WorkspaceID != nil && *plugin.WorkspaceID == workspaceID {
		return true
	}

	// Public plugins can be accessed by anyone
	if plugin.WorkspaceID == nil {
		return true
	}

	return false
}

func (s *PluginService) canModifyPlugin(plugin *models.Plugin, userID, workspaceID uuid.UUID) bool {
	// Only plugin owner can modify
	return plugin.DeveloperID == userID
}

func (s *PluginService) calculateHash(binary []byte) string {
	// Implementation would use crypto/sha256
	return fmt.Sprintf("sha256:%x", binary[:min(len(binary), 32)])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Additional service methods would be implemented here...

// GetStats returns service statistics
func (s *PluginService) GetStats(ctx context.Context, userID, workspaceID uuid.UUID) (*ServiceStats, error) {
	// Implementation would collect various statistics
	return &ServiceStats{
		TotalPlugins:        100, // placeholder
		ActiveInstallations: 50,  // placeholder
		TotalExecutions:     1000, // placeholder
		RuntimeStats:        s.wasmRuntime.GetInstanceStats(),
	}, nil
}

// HealthCheck returns service health status
func (s *PluginService) HealthCheck(ctx context.Context) *HealthStatus {
	// Implementation would check various components
	return &HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   "1.0.0",
		Components: map[string]string{
			"database": "healthy",
			"runtime":  "healthy",
			"security": "healthy",
		},
	}
}

// GetMetrics returns service metrics
func (s *PluginService) GetMetrics(ctx context.Context) (map[string]interface{}, error) {
	// Implementation would return Prometheus-style metrics
	return map[string]interface{}{
		"plugin_service_plugins_total":       100,
		"plugin_service_installations_total": 50,
		"plugin_service_executions_total":    1000,
	}, nil
}

// ValidatePlugin validates a plugin
func (s *PluginService) ValidatePlugin(ctx context.Context, pluginID, userID, workspaceID uuid.UUID) (*security.SecurityCheckResult, error) {
	plugin, err := s.pluginRepo.GetByID(ctx, pluginID)
	if err != nil {
		return nil, ErrPluginNotFound
	}

	if !s.canAccessPlugin(plugin, userID, workspaceID) {
		return nil, ErrForbidden
	}

	return s.securityManager.ValidatePlugin(ctx, plugin, userID, workspaceID)
}

// QuarantinePlugin quarantines a plugin
func (s *PluginService) QuarantinePlugin(ctx context.Context, pluginID uuid.UUID, reason string, metadata map[string]interface{}, userID uuid.UUID) error {
	return s.securityManager.QuarantinePlugin(pluginID, reason, metadata)
}