package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"github.com/pyairtable/go-services/plugin-service/internal/config"
	"github.com/pyairtable/go-services/plugin-service/internal/models"
)

// WASMRuntime manages WebAssembly plugin execution
type WASMRuntime struct {
	ctx             context.Context
	runtime         wazero.Runtime
	config          *config.RuntimeConfig
	instances       map[uuid.UUID]*PluginInstance
	instancesMutex  sync.RWMutex
	resourceMonitor *ResourceMonitor
	eventBus        EventBus
}

// PluginInstance represents a running plugin instance
type PluginInstance struct {
	ID              uuid.UUID
	PluginID        uuid.UUID
	InstallationID  uuid.UUID
	Module          api.Module
	Runtime         *WASMRuntime
	ResourceLimits  models.ResourceLimits
	CreatedAt       time.Time
	LastAccessAt    time.Time
	ExecutionCount  int64
	mutex           sync.RWMutex
	
	// Resource tracking
	memoryUsage     int64
	cpuUsage        time.Duration
	networkRequests int32
	storageUsage    int64
}

// ExecutionContext contains execution context for plugin calls
type ExecutionContext struct {
	UserID      uuid.UUID
	WorkspaceID uuid.UUID
	RequestID   string
	Timestamp   time.Time
	Input       json.RawMessage
	Metadata    map[string]interface{}
}

// ExecutionResult contains the result of plugin execution
type ExecutionResult struct {
	Success         bool            `json:"success"`
	Result          json.RawMessage `json:"result,omitempty"`
	Error           string          `json:"error,omitempty"`
	ResourceUsage   ResourceUsage   `json:"resource_usage"`
	ExecutionTimeMs int64           `json:"execution_time_ms"`
}

// ResourceUsage tracks resource consumption during execution
type ResourceUsage struct {
	MemoryMB        int32 `json:"memory_mb"`
	CPUTimeMs       int32 `json:"cpu_time_ms"`
	NetworkRequests int32 `json:"network_requests"`
	StorageMB       int32 `json:"storage_mb"`
}

// EventBus interface for plugin events
type EventBus interface {
	PublishPluginEvent(event PluginEvent) error
}

// PluginEvent represents plugin lifecycle events
type PluginEvent struct {
	Type         string                 `json:"type"`
	PluginID     uuid.UUID             `json:"plugin_id"`
	InstanceID   uuid.UUID             `json:"instance_id"`
	Timestamp    time.Time             `json:"timestamp"`
	Data         map[string]interface{} `json:"data"`
}

// NewWASMRuntime creates a new WASM runtime
func NewWASMRuntime(ctx context.Context, cfg *config.RuntimeConfig, eventBus EventBus) (*WASMRuntime, error) {
	// Create wazero runtime with configuration
	runtimeConfig := wazero.NewRuntimeConfig()
	
	// Enable debug information in development
	if cfg.Engine == "debug" {
		runtimeConfig = runtimeConfig.WithDebugInfoEnabled(true)
	}

	runtime := wazero.NewRuntimeWithConfig(ctx, runtimeConfig)

	// Instantiate WASI
	wasi_snapshot_preview1.MustInstantiate(ctx, runtime)

	wasmRuntime := &WASMRuntime{
		ctx:             ctx,
		runtime:         runtime,
		config:          cfg,
		instances:       make(map[uuid.UUID]*PluginInstance),
		resourceMonitor: NewResourceMonitor(),
		eventBus:        eventBus,
	}

	// Start resource monitoring
	go wasmRuntime.monitorResources()

	return wasmRuntime, nil
}

// LoadPlugin loads a plugin into the runtime
func (r *WASMRuntime) LoadPlugin(ctx context.Context, plugin *models.Plugin, installation *models.PluginInstallation) (*PluginInstance, error) {
	r.instancesMutex.Lock()
	defer r.instancesMutex.Unlock()

	// Check if instance already exists
	if instance, exists := r.instances[installation.ID]; exists {
		instance.LastAccessAt = time.Now()
		return instance, nil
	}

	// Check instance limit
	if len(r.instances) >= r.config.MaxInstances {
		// Cleanup least recently used instance
		if err := r.cleanupLRUInstance(); err != nil {
			return nil, fmt.Errorf("failed to cleanup LRU instance: %w", err)
		}
	}

	// Compile WASM module
	module, err := r.runtime.Instantiate(ctx, plugin.WasmBinary)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate WASM module: %w", err)
	}

	// Create plugin instance
	instance := &PluginInstance{
		ID:             uuid.New(),
		PluginID:       plugin.ID,
		InstallationID: installation.ID,
		Module:         module,
		Runtime:        r,
		ResourceLimits: plugin.ResourceLimits,
		CreatedAt:      time.Now(),
		LastAccessAt:   time.Now(),
	}

	// Store instance
	r.instances[installation.ID] = instance

	// Publish event
	r.eventBus.PublishPluginEvent(PluginEvent{
		Type:       "plugin.loaded",
		PluginID:   plugin.ID,
		InstanceID: instance.ID,
		Timestamp:  time.Now(),
		Data: map[string]interface{}{
			"installation_id": installation.ID,
		},
	})

	return instance, nil
}

// ExecutePlugin executes a plugin function
func (r *WASMRuntime) ExecutePlugin(ctx context.Context, installationID uuid.UUID, functionName string, execCtx *ExecutionContext) (*ExecutionResult, error) {
	r.instancesMutex.RLock()
	instance, exists := r.instances[installationID]
	r.instancesMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("plugin instance not found: %s", installationID)
	}

	return instance.Execute(ctx, functionName, execCtx)
}

// Execute executes a function in the plugin instance
func (i *PluginInstance) Execute(ctx context.Context, functionName string, execCtx *ExecutionContext) (*ExecutionResult, error) {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	startTime := time.Now()
	i.LastAccessAt = startTime
	i.ExecutionCount++

	// Create execution context with timeout
	execTimeout := time.Duration(i.ResourceLimits.MaxExecutionMs) * time.Millisecond
	ctxWithTimeout, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	// Track resource usage
	resourceTracker := NewResourceTracker(i.ResourceLimits)
	defer resourceTracker.Stop()

	// Get the function to execute
	fn := i.Module.ExportedFunction(functionName)
	if fn == nil {
		return &ExecutionResult{
			Success:         false,
			Error:           fmt.Sprintf("function %s not found", functionName),
			ExecutionTimeMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	// Prepare input data
	inputJSON, err := json.Marshal(execCtx)
	if err != nil {
		return &ExecutionResult{
			Success:         false,
			Error:           fmt.Sprintf("failed to marshal input: %v", err),
			ExecutionTimeMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	// Execute the function with resource monitoring
	result, err := i.executeWithResourceLimits(ctxWithTimeout, fn, inputJSON, resourceTracker)
	
	executionTime := time.Since(startTime)
	
	// Get resource usage
	resourceUsage := resourceTracker.GetUsage()

	// Update instance metrics
	i.memoryUsage = int64(resourceUsage.MemoryMB) * 1024 * 1024
	i.cpuUsage += executionTime
	i.networkRequests += resourceUsage.NetworkRequests
	i.storageUsage = int64(resourceUsage.StorageMB) * 1024 * 1024

	// Publish execution event
	i.Runtime.eventBus.PublishPluginEvent(PluginEvent{
		Type:       "plugin.executed",
		PluginID:   i.PluginID,
		InstanceID: i.ID,
		Timestamp:  time.Now(),
		Data: map[string]interface{}{
			"function_name":    functionName,
			"execution_time_ms": executionTime.Milliseconds(),
			"success":          err == nil,
			"resource_usage":   resourceUsage,
		},
	})

	if err != nil {
		return &ExecutionResult{
			Success:         false,
			Error:           err.Error(),
			ResourceUsage:   resourceUsage,
			ExecutionTimeMs: executionTime.Milliseconds(),
		}, nil
	}

	return &ExecutionResult{
		Success:         true,
		Result:          result,
		ResourceUsage:   resourceUsage,
		ExecutionTimeMs: executionTime.Milliseconds(),
	}, nil
}

// executeWithResourceLimits executes a function with resource monitoring
func (i *PluginInstance) executeWithResourceLimits(ctx context.Context, fn api.Function, input []byte, tracker *ResourceTracker) (json.RawMessage, error) {
	// Start resource monitoring
	tracker.Start()

	// Create a channel to receive the result
	resultChan := make(chan executeResult, 1)
	
	go func() {
		defer func() {
			if r := recover(); r != nil {
				resultChan <- executeResult{
					Error: fmt.Errorf("plugin panicked: %v", r),
				}
			}
		}()

		// Execute the function
		// Note: This is a simplified version. In a real implementation,
		// you would need to handle memory allocation, function parameters, etc.
		results, err := fn.Call(ctx)
		if err != nil {
			resultChan <- executeResult{Error: err}
			return
		}

		// Convert results to JSON
		// This is simplified - you'd need proper result handling
		resultJSON, err := json.Marshal(results)
		if err != nil {
			resultChan <- executeResult{Error: err}
			return
		}

		resultChan <- executeResult{
			Result: resultJSON,
		}
	}()

	// Wait for result or timeout/resource limit violation
	select {
	case result := <-resultChan:
		return result.Result, result.Error
	case <-ctx.Done():
		return nil, fmt.Errorf("execution timeout exceeded")
	case <-tracker.ViolationChan():
		return nil, fmt.Errorf("resource limit exceeded")
	}
}

type executeResult struct {
	Result json.RawMessage
	Error  error
}

// UnloadPlugin unloads a plugin instance
func (r *WASMRuntime) UnloadPlugin(installationID uuid.UUID) error {
	r.instancesMutex.Lock()
	defer r.instancesMutex.Unlock()

	instance, exists := r.instances[installationID]
	if !exists {
		return fmt.Errorf("plugin instance not found: %s", installationID)
	}

	// Close the module
	if err := instance.Module.Close(r.ctx); err != nil {
		return fmt.Errorf("failed to close module: %w", err)
	}

	// Remove from instances
	delete(r.instances, installationID)

	// Publish event
	r.eventBus.PublishPluginEvent(PluginEvent{
		Type:       "plugin.unloaded",
		PluginID:   instance.PluginID,
		InstanceID: instance.ID,
		Timestamp:  time.Now(),
		Data: map[string]interface{}{
			"installation_id": installationID,
		},
	})

	return nil
}

// cleanupLRUInstance removes the least recently used instance
func (r *WASMRuntime) cleanupLRUInstance() error {
	var oldestInstance *PluginInstance
	var oldestInstallationID uuid.UUID

	for id, instance := range r.instances {
		if oldestInstance == nil || instance.LastAccessAt.Before(oldestInstance.LastAccessAt) {
			oldestInstance = instance
			oldestInstallationID = id
		}
	}

	if oldestInstance != nil {
		return r.UnloadPlugin(oldestInstallationID)
	}

	return nil
}

// monitorResources monitors resource usage across all instances
func (r *WASMRuntime) monitorResources() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.collectResourceMetrics()
		}
	}
}

// collectResourceMetrics collects resource metrics from all instances
func (r *WASMRuntime) collectResourceMetrics() {
	r.instancesMutex.RLock()
	defer r.instancesMutex.RUnlock()

	for _, instance := range r.instances {
		instance.mutex.RLock()
		metrics := map[string]interface{}{
			"plugin_id":        instance.PluginID,
			"instance_id":      instance.ID,
			"memory_usage":     instance.memoryUsage,
			"cpu_usage_ms":     instance.cpuUsage.Milliseconds(),
			"network_requests": instance.networkRequests,
			"storage_usage":    instance.storageUsage,
			"execution_count":  instance.ExecutionCount,
			"last_access":      instance.LastAccessAt,
		}
		instance.mutex.RUnlock()

		r.eventBus.PublishPluginEvent(PluginEvent{
			Type:       "plugin.metrics",
			PluginID:   instance.PluginID,
			InstanceID: instance.ID,
			Timestamp:  time.Now(),
			Data:       metrics,
		})
	}
}

// GetInstanceStats returns statistics for all plugin instances
func (r *WASMRuntime) GetInstanceStats() map[uuid.UUID]*InstanceStats {
	r.instancesMutex.RLock()
	defer r.instancesMutex.RUnlock()

	stats := make(map[uuid.UUID]*InstanceStats)
	for id, instance := range r.instances {
		instance.mutex.RLock()
		stats[id] = &InstanceStats{
			PluginID:        instance.PluginID,
			InstallationID:  instance.InstallationID,
			CreatedAt:       instance.CreatedAt,
			LastAccessAt:    instance.LastAccessAt,
			ExecutionCount:  instance.ExecutionCount,
			MemoryUsage:     instance.memoryUsage,
			CPUUsage:        instance.cpuUsage,
			NetworkRequests: instance.networkRequests,
			StorageUsage:    instance.storageUsage,
		}
		instance.mutex.RUnlock()
	}

	return stats
}

// InstanceStats contains statistics for a plugin instance
type InstanceStats struct {
	PluginID        uuid.UUID     `json:"plugin_id"`
	InstallationID  uuid.UUID     `json:"installation_id"`
	CreatedAt       time.Time     `json:"created_at"`
	LastAccessAt    time.Time     `json:"last_access_at"`
	ExecutionCount  int64         `json:"execution_count"`
	MemoryUsage     int64         `json:"memory_usage"`
	CPUUsage        time.Duration `json:"cpu_usage"`
	NetworkRequests int32         `json:"network_requests"`
	StorageUsage    int64         `json:"storage_usage"`
}

// Shutdown gracefully shuts down the runtime
func (r *WASMRuntime) Shutdown() error {
	r.instancesMutex.Lock()
	defer r.instancesMutex.Unlock()

	// Close all instances
	for id := range r.instances {
		if err := r.UnloadPlugin(id); err != nil {
			// Log error but continue cleanup
			fmt.Printf("Error unloading plugin %s: %v\n", id, err)
		}
	}

	// Close the runtime
	return r.runtime.Close(r.ctx)
}