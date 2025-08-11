package processors

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/pyairtable/go-services/file-processing-service/internal/models"
	"github.com/pyairtable/go-services/file-processing-service/internal/websocket"
)

// Processor defines the interface for file processors
type Processor interface {
	// Process processes a file and returns extracted data
	Process(ctx *ProcessingContext) (models.JSONMap, error)
	
	// GetSupportedTypes returns the file types this processor supports
	GetSupportedTypes() []models.FileType
	
	// ValidateFile validates that the file can be processed
	ValidateFile(ctx *ProcessingContext) error
	
	// EstimateProcessingTime estimates how long processing will take
	EstimateProcessingTime(ctx *ProcessingContext) (time.Duration, error)
	
	// GetMemoryRequirement estimates memory needed for processing
	GetMemoryRequirement(ctx *ProcessingContext) int64
}

// ProcessingContext contains context for file processing
type ProcessingContext struct {
	FileRecord      *models.FileRecord
	Reader          io.Reader
	Config          *models.ProcessingConfig
	ProgressTracker *websocket.ProgressTracker
	Context         context.Context
	TempDir         string
}

// BaseProcessor provides common functionality for processors
type BaseProcessor struct {
	supportedTypes []models.FileType
}

// NewBaseProcessor creates a new base processor
func NewBaseProcessor(supportedTypes []models.FileType) *BaseProcessor {
	return &BaseProcessor{
		supportedTypes: supportedTypes,
	}
}

// GetSupportedTypes returns supported file types
func (bp *BaseProcessor) GetSupportedTypes() []models.FileType {
	return bp.supportedTypes
}

// ValidateFile performs basic validation
func (bp *BaseProcessor) ValidateFile(ctx *ProcessingContext) error {
	if ctx.FileRecord == nil {
		return fmt.Errorf("file record is nil")
	}
	
	if ctx.Reader == nil {
		return fmt.Errorf("file reader is nil")
	}
	
	// Check if file type is supported
	supported := false
	for _, fileType := range bp.supportedTypes {
		if ctx.FileRecord.FileType == fileType {
			supported = true
			break
		}
	}
	
	if !supported {
		return fmt.Errorf("file type %s not supported by this processor", ctx.FileRecord.FileType)
	}
	
	return nil
}

// EstimateProcessingTime provides a default estimation
func (bp *BaseProcessor) EstimateProcessingTime(ctx *ProcessingContext) (time.Duration, error) {
	// Base estimation: 1 second per MB
	return time.Duration(ctx.FileRecord.Size/1024/1024) * time.Second, nil
}

// GetMemoryRequirement provides a default memory requirement
func (bp *BaseProcessor) GetMemoryRequirement(ctx *ProcessingContext) int64 {
	// Default: 2x file size plus 50MB base
	return (ctx.FileRecord.Size * 2) + (50 << 20)
}

// UpdateProgress updates processing progress
func (bp *BaseProcessor) UpdateProgress(ctx *ProcessingContext, step models.ProcessingStep, progress float64, message string) {
	if ctx.ProgressTracker != nil {
		progressUpdate := models.ProcessingProgress{
			FileID:    ctx.FileRecord.ID,
			Step:      step,
			Progress:  progress,
			Message:   message,
			StartedAt: time.Now(),
		}
		ctx.ProgressTracker.UpdateProgress(ctx.FileRecord.ID, progressUpdate)
	}
}

// ProcessorMetrics contains metrics for processor performance
type ProcessorMetrics struct {
	ProcessorType    string        `json:"processor_type"`
	TotalFiles       int64         `json:"total_files"`
	SuccessfulFiles  int64         `json:"successful_files"`
	FailedFiles      int64         `json:"failed_files"`
	AverageTime      time.Duration `json:"average_time"`
	TotalTime        time.Duration `json:"total_time"`
	PeakMemoryUsage  int64         `json:"peak_memory_usage"`
	LastProcessed    time.Time     `json:"last_processed"`
}

// ProcessorRegistry manages available processors
type ProcessorRegistry struct {
	processors map[models.FileType]Processor
	metrics    map[string]*ProcessorMetrics
	mu         sync.RWMutex
}

// NewProcessorRegistry creates a new processor registry
func NewProcessorRegistry() *ProcessorRegistry {
	return &ProcessorRegistry{
		processors: make(map[models.FileType]Processor),
		metrics:    make(map[string]*ProcessorMetrics),
	}
}

// Register registers a processor for specific file types
func (pr *ProcessorRegistry) Register(processor Processor) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	
	supportedTypes := processor.GetSupportedTypes()
	if len(supportedTypes) == 0 {
		return fmt.Errorf("processor must support at least one file type")
	}
	
	for _, fileType := range supportedTypes {
		if _, exists := pr.processors[fileType]; exists {
			return fmt.Errorf("processor for file type %s already registered", fileType)
		}
		pr.processors[fileType] = processor
	}
	
	// Initialize metrics
	processorName := fmt.Sprintf("%T", processor)
	pr.metrics[processorName] = &ProcessorMetrics{
		ProcessorType: processorName,
	}
	
	return nil
}

// GetProcessor returns a processor for the given file type
func (pr *ProcessorRegistry) GetProcessor(fileType models.FileType) (Processor, error) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	
	processor, exists := pr.processors[fileType]
	if !exists {
		return nil, fmt.Errorf("no processor registered for file type: %s", fileType)
	}
	
	return processor, nil
}

// GetSupportedTypes returns all supported file types
func (pr *ProcessorRegistry) GetSupportedTypes() []models.FileType {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	
	types := make([]models.FileType, 0, len(pr.processors))
	for fileType := range pr.processors {
		types = append(types, fileType)
	}
	
	return types
}

// UpdateMetrics updates processor metrics
func (pr *ProcessorRegistry) UpdateMetrics(processor Processor, duration time.Duration, success bool) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	
	processorName := fmt.Sprintf("%T", processor)
	metrics, exists := pr.metrics[processorName]
	if !exists {
		return
	}
	
	metrics.TotalFiles++
	metrics.TotalTime += duration
	metrics.AverageTime = metrics.TotalTime / time.Duration(metrics.TotalFiles)
	metrics.LastProcessed = time.Now()
	
	if success {
		metrics.SuccessfulFiles++
	} else {
		metrics.FailedFiles++
	}
}

// GetMetrics returns metrics for all processors
func (pr *ProcessorRegistry) GetMetrics() map[string]*ProcessorMetrics {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	
	result := make(map[string]*ProcessorMetrics)
	for name, metrics := range pr.metrics {
		// Create a copy to avoid race conditions
		result[name] = &ProcessorMetrics{
			ProcessorType:    metrics.ProcessorType,
			TotalFiles:       metrics.TotalFiles,
			SuccessfulFiles:  metrics.SuccessfulFiles,
			FailedFiles:      metrics.FailedFiles,
			AverageTime:      metrics.AverageTime,
			TotalTime:        metrics.TotalTime,
			PeakMemoryUsage:  metrics.PeakMemoryUsage,
			LastProcessed:    metrics.LastProcessed,
		}
	}
	
	return result
}

// ValidationError represents a file validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error in %s: %s", e.Field, e.Message)
}

// ProcessingError represents a file processing error
type ProcessingError struct {
	Stage   string
	Message string
	Cause   error
}

func (e *ProcessingError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("processing error in %s: %s (caused by: %v)", e.Stage, e.Message, e.Cause)
	}
	return fmt.Sprintf("processing error in %s: %s", e.Stage, e.Message)
}

func (e *ProcessingError) Unwrap() error {
	return e.Cause
}

// Common processing errors
var (
	ErrUnsupportedFileType = fmt.Errorf("unsupported file type")
	ErrFileCorrupted       = fmt.Errorf("file is corrupted or invalid")
	ErrFileTooLarge        = fmt.Errorf("file is too large to process")
	ErrMemoryExhausted     = fmt.Errorf("insufficient memory to process file")
	ErrTimeoutExceeded     = fmt.Errorf("processing timeout exceeded")
	ErrInvalidFormat       = fmt.Errorf("invalid file format")
)

// ProgressCallback is a function type for progress updates
type ProgressCallback func(step models.ProcessingStep, progress float64, message string)

// StreamingProcessor defines interface for processors that support streaming
type StreamingProcessor interface {
	Processor
	
	// ProcessStream processes a file using streaming to handle large files
	ProcessStream(ctx *ProcessingContext, callback ProgressCallback) (models.JSONMap, error)
	
	// SupportsStreaming returns true if the processor supports streaming
	SupportsStreaming() bool
	
	// GetStreamingThreshold returns the file size threshold for streaming
	GetStreamingThreshold() int64
}

// BatchProcessor defines interface for processors that can handle multiple files
type BatchProcessor interface {
	Processor
	
	// ProcessBatch processes multiple files in a batch
	ProcessBatch(contexts []*ProcessingContext) ([]models.JSONMap, error)
	
	// SupportsBatchProcessing returns true if batch processing is supported
	SupportsBatchProcessing() bool
	
	// GetOptimalBatchSize returns the optimal batch size
	GetOptimalBatchSize() int
}