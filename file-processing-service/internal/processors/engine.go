package processors

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/pyairtable/go-services/file-processing-service/internal/models"
	"github.com/pyairtable/go-services/file-processing-service/internal/storage"
	"github.com/pyairtable/go-services/file-processing-service/internal/websocket"
	"golang.org/x/sync/semaphore"
)

// ProcessingEngine manages concurrent file processing with worker pools
type ProcessingEngine struct {
	workers         int
	queueSize       int
	taskQueue       chan *ProcessingTask
	storage         storage.Interface
	progressTracker *websocket.ProgressTracker
	processors      map[models.FileType]Processor
	config          *models.ProcessingConfig
	
	// Concurrency controls
	uploadSemaphore *semaphore.Weighted
	memorySemaphore *semaphore.Weighted
	
	// State management
	running         bool
	wg              sync.WaitGroup
	mu              sync.RWMutex
	
	// Metrics
	metrics *ProcessingMetrics
	
	// Context for shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// ProcessingTask represents a file processing task
type ProcessingTask struct {
	FileRecord    *models.FileRecord
	Priority      int
	SubmittedAt   time.Time
	StartedAt     *time.Time
	CompletedAt   *time.Time
	RetryCount    int
	MaxRetries    int
	Context       context.Context
	ResultChannel chan *ProcessingResult
}

// ProcessingResult represents the result of processing
type ProcessingResult struct {
	FileRecord *models.FileRecord
	Data       interface{}
	Error      error
	Metrics    ProcessingTaskMetrics
}

// ProcessingTaskMetrics contains metrics for a single processing task
type ProcessingTaskMetrics struct {
	Duration        time.Duration
	MemoryUsed      int64
	CPUTime         time.Duration
	BytesProcessed  int64
	RecordsExtracted int
}

// ProcessingMetrics contains overall processing metrics
type ProcessingMetrics struct {
	mu                    sync.RWMutex
	TasksQueued          int64
	TasksProcessed       int64
	TasksFailed          int64
	TotalProcessingTime  time.Duration
	AverageProcessingTime time.Duration
	PeakMemoryUsage      int64
	ActiveWorkers        int
	QueueDepth           int
}

// NewProcessingEngine creates a new processing engine
func NewProcessingEngine(
	config *models.ProcessingConfig,
	storage storage.Interface,
	progressTracker *websocket.ProgressTracker,
) *ProcessingEngine {
	ctx, cancel := context.WithCancel(context.Background())
	
	workers := config.WorkerCount
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	
	queueSize := config.QueueSize
	if queueSize <= 0 {
		queueSize = workers * 10
	}
	
	engine := &ProcessingEngine{
		workers:         workers,
		queueSize:       queueSize,
		taskQueue:       make(chan *ProcessingTask, queueSize),
		storage:         storage,
		progressTracker: progressTracker,
		processors:      make(map[models.FileType]Processor),
		config:          config,
		uploadSemaphore: semaphore.NewWeighted(int64(config.MaxConcurrentUploads)),
		memorySemaphore: semaphore.NewWeighted(config.MemoryLimit),
		metrics:         &ProcessingMetrics{},
		ctx:             ctx,
		cancel:          cancel,
	}
	
	// Register processors
	engine.registerProcessors()
	
	return engine
}

// Start starts the processing engine
func (pe *ProcessingEngine) Start() error {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	
	if pe.running {
		return fmt.Errorf("processing engine is already running")
	}
	
	pe.running = true
	
	// Start worker goroutines
	for i := 0; i < pe.workers; i++ {
		pe.wg.Add(1)
		go pe.worker(i)
	}
	
	// Start metrics collection
	pe.wg.Add(1)
	go pe.metricsCollector()
	
	// Start cleanup routine
	pe.wg.Add(1)
	go pe.cleanup()
	
	log.Printf("Processing engine started with %d workers", pe.workers)
	return nil
}

// Stop stops the processing engine gracefully
func (pe *ProcessingEngine) Stop() error {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	
	if !pe.running {
		return fmt.Errorf("processing engine is not running")
	}
	
	pe.running = false
	pe.cancel()
	
	// Close task queue
	close(pe.taskQueue)
	
	// Wait for all workers to finish
	done := make(chan struct{})
	go func() {
		pe.wg.Wait()
		close(done)
	}()
	
	// Wait with timeout
	select {
	case <-done:
		log.Println("Processing engine stopped gracefully")
		return nil
	case <-time.After(30 * time.Second):
		log.Println("Processing engine stopped with timeout")
		return fmt.Errorf("shutdown timeout exceeded")
	}
}

// Enqueue adds a file processing task to the queue
func (pe *ProcessingEngine) Enqueue(ctx context.Context, fileRecord *models.FileRecord) error {
	pe.mu.RLock()
	if !pe.running {
		pe.mu.RUnlock()
		return fmt.Errorf("processing engine is not running")
	}
	pe.mu.RUnlock()
	
	task := &ProcessingTask{
		FileRecord:    fileRecord,
		Priority:      1, // Default priority
		SubmittedAt:   time.Now(),
		MaxRetries:    3,
		Context:       ctx,
		ResultChannel: make(chan *ProcessingResult, 1),
	}
	
	select {
	case pe.taskQueue <- task:
		pe.metrics.mu.Lock()
		pe.metrics.TasksQueued++
		pe.metrics.QueueDepth = len(pe.taskQueue)
		pe.metrics.mu.Unlock()
		
		// Update file status
		fileRecord.SetStatus(models.StatusProcessing)
		pe.notifyProgress(fileRecord, models.StepExtraction, 0.0, "Queued for processing")
		
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("task queue is full")
	}
}

// EnqueueWithPriority adds a high-priority task to the queue
func (pe *ProcessingEngine) EnqueueWithPriority(ctx context.Context, fileRecord *models.FileRecord, priority int) error {
	pe.mu.RLock()
	if !pe.running {
		pe.mu.RUnlock()
		return fmt.Errorf("processing engine is not running")
	}
	pe.mu.RUnlock()
	
	task := &ProcessingTask{
		FileRecord:    fileRecord,
		Priority:      priority,
		SubmittedAt:   time.Now(),
		MaxRetries:    3,
		Context:       ctx,
		ResultChannel: make(chan *ProcessingResult, 1),
	}
	
	// For high priority tasks, try to insert at front of queue
	if priority > 5 {
		select {
		case pe.taskQueue <- task:
			pe.metrics.mu.Lock()
			pe.metrics.TasksQueued++
			pe.metrics.QueueDepth = len(pe.taskQueue)
			pe.metrics.mu.Unlock()
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
			return fmt.Errorf("task queue is full")
		}
	}
	
	return pe.Enqueue(ctx, fileRecord)
}

// worker processes tasks from the queue
func (pe *ProcessingEngine) worker(workerID int) {
	defer pe.wg.Done()
	
	log.Printf("Worker %d started", workerID)
	
	for {
		select {
		case task, ok := <-pe.taskQueue:
			if !ok {
				log.Printf("Worker %d stopped: queue closed", workerID)
				return
			}
			
			pe.processTask(workerID, task)
			
		case <-pe.ctx.Done():
			log.Printf("Worker %d stopped: context cancelled", workerID)
			return
		}
	}
}

// processTask processes a single task
func (pe *ProcessingEngine) processTask(workerID int, task *ProcessingTask) {
	startTime := time.Now()
	task.StartedAt = &startTime
	
	pe.metrics.mu.Lock()
	pe.metrics.ActiveWorkers++
	pe.metrics.QueueDepth = len(pe.taskQueue)
	pe.metrics.mu.Lock()
	
	defer func() {
		pe.metrics.mu.Lock()
		pe.metrics.ActiveWorkers--
		pe.metrics.mu.Unlock()
	}()
	
	log.Printf("Worker %d processing file %s (%s)", workerID, task.FileRecord.ID, task.FileRecord.Filename)
	
	// Update progress
	pe.notifyProgress(task.FileRecord, models.StepExtraction, 0.1, fmt.Sprintf("Started processing on worker %d", workerID))
	
	// Acquire memory semaphore based on file size
	memoryNeeded := estimateMemoryNeeded(task.FileRecord)
	if err := pe.memorySemaphore.Acquire(task.Context, memoryNeeded); err != nil {
		pe.handleTaskError(task, fmt.Errorf("failed to acquire memory semaphore: %w", err))
		return
	}
	defer pe.memorySemaphore.Release(memoryNeeded)
	
	// Process the file
	result := pe.processFile(task)
	
	// Update metrics
	duration := time.Since(startTime)
	pe.metrics.mu.Lock()
	pe.metrics.TasksProcessed++
	pe.metrics.TotalProcessingTime += duration
	pe.metrics.AverageProcessingTime = pe.metrics.TotalProcessingTime / time.Duration(pe.metrics.TasksProcessed)
	pe.metrics.mu.Unlock()
	
	// Send result
	select {
	case task.ResultChannel <- result:
	default:
		// Channel might be closed or full
	}
	
	completedAt := time.Now()
	task.CompletedAt = &completedAt
	
	if result.Error != nil {
		log.Printf("Worker %d failed to process file %s: %v", workerID, task.FileRecord.ID, result.Error)
		pe.handleTaskError(task, result.Error)
	} else {
		log.Printf("Worker %d completed processing file %s in %v", workerID, task.FileRecord.ID, duration)
		pe.handleTaskSuccess(task, result)
	}
}

// processFile processes a file using the appropriate processor
func (pe *ProcessingEngine) processFile(task *ProcessingTask) *ProcessingResult {
	fileRecord := task.FileRecord
	
	// Get the appropriate processor
	processor, exists := pe.processors[fileRecord.FileType]
	if !exists {
		return &ProcessingResult{
			FileRecord: fileRecord,
			Error:      fmt.Errorf("no processor available for file type: %s", fileRecord.FileType),
		}
	}
	
	// Download file from storage
	pe.notifyProgress(fileRecord, models.StepExtraction, 0.2, "Downloading file from storage")
	
	reader, err := pe.storage.Download(task.Context, fileRecord.StoragePath)
	if err != nil {
		return &ProcessingResult{
			FileRecord: fileRecord,
			Error:      fmt.Errorf("failed to download file: %w", err),
		}
	}
	defer reader.Close()
	
	// Create processing context
	procCtx := &ProcessingContext{
		FileRecord:      fileRecord,
		Reader:          reader,
		Config:          pe.config,
		ProgressTracker: pe.progressTracker,
		Context:         task.Context,
	}
	
	// Process the file
	pe.notifyProgress(fileRecord, models.StepExtraction, 0.3, "Processing file content")
	
	startTime := time.Now()
	data, err := processor.Process(procCtx)
	processingTime := time.Since(startTime)
	
	result := &ProcessingResult{
		FileRecord: fileRecord,
		Data:       data,
		Error:      err,
		Metrics: ProcessingTaskMetrics{
			Duration: processingTime,
		},
	}
	
	if err != nil {
		return result
	}
	
	// Update file record with processed data
	fileRecord.ProcessingData = data.(models.JSONMap)
	fileRecord.SetStatus(models.StatusCompleted)
	
	pe.notifyProgress(fileRecord, models.StepComplete, 1.0, "Processing completed successfully")
	
	return result
}

// handleTaskSuccess handles successful task completion
func (pe *ProcessingEngine) handleTaskSuccess(task *ProcessingTask, result *ProcessingResult) {
	// Notify completion via WebSocket
	if pe.progressTracker != nil {
		processingResult := &models.ProcessingResult{
			FileID:         task.FileRecord.ID,
			Status:         models.StatusCompleted,
			Data:           result.Data,
			ProcessingTime: result.Metrics.Duration,
			Metadata:       task.FileRecord.Metadata,
		}
		pe.progressTracker.NotifyCompletion(task.FileRecord.ID, processingResult)
	}
}

// handleTaskError handles task processing errors
func (pe *ProcessingEngine) handleTaskError(task *ProcessingTask, err error) {
	task.RetryCount++
	
	if task.RetryCount <= task.MaxRetries {
		// Retry the task
		log.Printf("Retrying task for file %s (attempt %d/%d)", task.FileRecord.ID, task.RetryCount, task.MaxRetries)
		
		// Add exponential backoff
		backoff := time.Duration(task.RetryCount) * time.Second
		time.Sleep(backoff)
		
		// Re-queue the task
		select {
		case pe.taskQueue <- task:
			return
		default:
			// Queue is full, treat as final failure
		}
	}
	
	// Final failure
	task.FileRecord.SetStatus(models.StatusFailed, err.Error())
	
	pe.metrics.mu.Lock()
	pe.metrics.TasksFailed++
	pe.metrics.mu.Unlock()
	
	// Notify error via WebSocket
	if pe.progressTracker != nil {
		pe.progressTracker.NotifyError(task.FileRecord.ID, err)
	}
	
	log.Printf("Task failed permanently for file %s: %v", task.FileRecord.ID, err)
}

// notifyProgress sends progress updates via WebSocket
func (pe *ProcessingEngine) notifyProgress(fileRecord *models.FileRecord, step models.ProcessingStep, progress float64, message string) {
	if pe.progressTracker != nil {
		progressUpdate := models.ProcessingProgress{
			FileID:    fileRecord.ID,
			Step:      step,
			Progress:  progress,
			Message:   message,
			StartedAt: time.Now(),
		}
		pe.progressTracker.UpdateProgress(fileRecord.ID, progressUpdate)
	}
}

// metricsCollector collects and updates metrics periodically
func (pe *ProcessingEngine) metricsCollector() {
	defer pe.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			pe.collectMetrics()
		case <-pe.ctx.Done():
			return
		}
	}
}

// collectMetrics collects system metrics
func (pe *ProcessingEngine) collectMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	pe.metrics.mu.Lock()
	if int64(m.Alloc) > pe.metrics.PeakMemoryUsage {
		pe.metrics.PeakMemoryUsage = int64(m.Alloc)
	}
	pe.metrics.QueueDepth = len(pe.taskQueue)
	pe.metrics.mu.Unlock()
}

// cleanup performs periodic cleanup tasks
func (pe *ProcessingEngine) cleanup() {
	defer pe.wg.Done()
	
	ticker := time.NewTicker(pe.config.CleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			pe.performCleanup()
		case <-pe.ctx.Done():
			return
		}
	}
}

// performCleanup performs cleanup tasks
func (pe *ProcessingEngine) performCleanup() {
	// Force garbage collection
	runtime.GC()
	
	// Clean up temporary files
	if pe.config.TempDir != "" {
		// Implementation would clean up old temp files
		log.Println("Performing cleanup of temporary files")
	}
}

// GetMetrics returns current processing metrics
func (pe *ProcessingEngine) GetMetrics() ProcessingMetrics {
	pe.metrics.mu.RLock()
	defer pe.metrics.mu.RUnlock()
	return *pe.metrics
}

// GetQueueDepth returns current queue depth
func (pe *ProcessingEngine) GetQueueDepth() int {
	return len(pe.taskQueue)
}

// IsRunning returns whether the engine is running
func (pe *ProcessingEngine) IsRunning() bool {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return pe.running
}

// Helper functions

// estimateMemoryNeeded estimates memory needed for processing a file
func estimateMemoryNeeded(fileRecord *models.FileRecord) int64 {
	// Base memory requirement
	baseMemory := int64(50 << 20) // 50MB
	
	// Add memory based on file size and type
	switch fileRecord.FileType {
	case models.FileTypeCSV, models.FileTypeXLSX:
		// Spreadsheets need more memory for parsing
		return baseMemory + (fileRecord.Size * 3)
	case models.FileTypePDF:
		// PDFs need memory for text extraction
		return baseMemory + (fileRecord.Size * 2)
	case models.FileTypeJPEG, models.FileTypePNG, models.FileTypeWEBP:
		// Images need memory for decompression
		return baseMemory + (fileRecord.Size * 4)
	default:
		return baseMemory + fileRecord.Size
	}
}

// registerProcessors registers all available processors
func (pe *ProcessingEngine) registerProcessors() {
	// Register CSV processor
	pe.processors[models.FileTypeCSV] = NewCSVProcessor()
	
	// Register PDF processor
	pe.processors[models.FileTypePDF] = NewPDFProcessor()
	
	// Register DOCX processor
	pe.processors[models.FileTypeDOCX] = NewDOCXProcessor()
	
	// Register Excel processor
	pe.processors[models.FileTypeXLSX] = NewExcelProcessor()
	
	// Register image processors
	pe.processors[models.FileTypeJPEG] = NewImageProcessor()
	pe.processors[models.FileTypePNG] = NewImageProcessor()
	pe.processors[models.FileTypeWEBP] = NewImageProcessor()
}