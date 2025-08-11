package handlers

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pyairtable/go-services/file-processing-service/internal/models"
	"github.com/pyairtable/go-services/file-processing-service/internal/storage"
	"github.com/pyairtable/go-services/file-processing-service/internal/websocket"
)

// UploadHandler handles file upload operations
type UploadHandler struct {
	storage         storage.Interface
	progressTracker *websocket.ProgressTracker
	config          *models.Config
	processor       ProcessingQueue
}

// ProcessingQueue interface for file processing
type ProcessingQueue interface {
	Enqueue(ctx context.Context, fileRecord *models.FileRecord) error
}

// NewUploadHandler creates a new upload handler
func NewUploadHandler(storage storage.Interface, tracker *websocket.ProgressTracker, config *models.Config, processor ProcessingQueue) *UploadHandler {
	return &UploadHandler{
		storage:         storage,
		progressTracker: tracker,
		config:          config,
		processor:       processor,
	}
}

// UploadRequest represents an upload request
type UploadRequest struct {
	Purpose     string            `form:"purpose"`     // What the file will be used for
	Metadata    map[string]string `form:"metadata"`    // Additional metadata
	AutoProcess bool              `form:"auto_process"` // Whether to automatically process the file
}

// UploadResponse represents an upload response
type UploadResponse struct {
	FileID       string                 `json:"file_id"`
	Filename     string                 `json:"filename"`
	FileType     models.FileType        `json:"file_type"`
	Size         int64                  `json:"size"`
	Status       models.ProcessingStatus `json:"status"`
	UploadedAt   time.Time              `json:"uploaded_at"`
	StorageInfo  models.StorageInfo     `json:"storage_info"`
	WebSocketURL string                 `json:"websocket_url,omitempty"`
}

// StreamingUploadHandler handles streaming file uploads with progress tracking
func (h *UploadHandler) StreamingUploadHandler(c *gin.Context) {
	ctx := c.Request.Context()
	
	// Parse the upload request
	var req UploadRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request parameters"})
		return
	}

	// Get the file from the form
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}

	// Validate file size
	if fileHeader.Size > h.config.Server.MaxUploadSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"error": fmt.Sprintf("File size exceeds maximum allowed size of %d bytes", h.config.Server.MaxUploadSize),
		})
		return
	}

	// Validate file type
	fileType := getFileTypeFromName(fileHeader.Filename)
	if fileType == "" {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{
			"error": "Unsupported file type",
		})
		return
	}

	if !isAllowedFileType(fileType, h.config.Storage.AllowedTypes) {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{
			"error": fmt.Sprintf("File type %s not allowed", fileType),
		})
		return
	}

	// Create file record
	fileRecord := models.NewFileRecord(
		fileHeader.Filename,
		fileHeader.Header.Get("Content-Type"),
		fileHeader.Size,
		fileType,
	)
	fileRecord.SetStatus(models.StatusUploading)

	// Open the uploaded file
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open uploaded file"})
		return
	}
	defer file.Close()

	// Create a progress tracking reader
	progressReader := &ProgressReader{
		Reader:   file,
		Total:    fileHeader.Size,
		FileID:   fileRecord.ID,
		Tracker:  h.progressTracker,
		Callback: func(progress float64) {
			fileRecord.UpdateProgress(progress*0.8, "Uploading...") // Reserve 20% for processing
		},
	}

	// Start upload with streaming
	ctx, cancel := context.WithTimeout(ctx, h.config.Storage.UploadTimeout)
	defer cancel()

	storageInfo, err := h.storage.StreamingUpload(ctx, &storage.UploadParams{
		FileID:      fileRecord.ID,
		Filename:    fileRecord.Filename,
		ContentType: fileRecord.ContentType,
		Size:        fileRecord.Size,
		Reader:      progressReader,
	})
	if err != nil {
		fileRecord.SetStatus(models.StatusFailed, fmt.Sprintf("Upload failed: %v", err))
		h.notifyProgress(fileRecord, "Upload failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload failed"})
		return
	}

	// Update file record with storage information
	fileRecord.StoragePath = storageInfo.Key
	fileRecord.SetStatus(models.StatusPending)
	fileRecord.UpdateProgress(0.8, "Upload completed, queued for processing")

	// Store file record in database (implement this based on your database layer)
	if err := h.storeFileRecord(ctx, fileRecord); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store file record"})
		return
	}

	// Auto-process if requested
	if req.AutoProcess {
		if err := h.processor.Enqueue(ctx, fileRecord); err != nil {
			// Log error but don't fail the upload
			fmt.Printf("Failed to enqueue file for processing: %v\n", err)
		}
	}

	// Prepare response
	response := &UploadResponse{
		FileID:      fileRecord.ID,
		Filename:    fileRecord.Filename,
		FileType:    fileRecord.FileType,
		Size:        fileRecord.Size,
		Status:      fileRecord.Status,
		UploadedAt:  fileRecord.CreatedAt,
		StorageInfo: *storageInfo,
	}

	if h.config.WebSocket.Enabled {
		response.WebSocketURL = fmt.Sprintf("ws://%s:%d%s?file_id=%s", 
			h.config.Server.Host, h.config.Server.Port, h.config.WebSocket.Path, fileRecord.ID)
	}

	// Final progress notification
	h.notifyProgress(fileRecord, "Upload completed successfully")

	c.JSON(http.StatusCreated, response)
}

// ChunkedUploadInitHandler initiates a chunked upload
func (h *UploadHandler) ChunkedUploadInitHandler(c *gin.Context) {
	ctx := c.Request.Context()

	var req struct {
		Filename    string `json:"filename" binding:"required"`
		Size        int64  `json:"size" binding:"required"`
		ContentType string `json:"content_type"`
		ChunkSize   int64  `json:"chunk_size"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate file size
	if req.Size > h.config.Server.MaxUploadSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"error": fmt.Sprintf("File size exceeds maximum allowed size of %d bytes", h.config.Server.MaxUploadSize),
		})
		return
	}

	// Validate file type
	fileType := getFileTypeFromName(req.Filename)
	if fileType == "" || !isAllowedFileType(fileType, h.config.Storage.AllowedTypes) {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{
			"error": "Unsupported file type",
		})
		return
	}

	// Create file record
	fileRecord := models.NewFileRecord(req.Filename, req.ContentType, req.Size, fileType)
	fileRecord.SetStatus(models.StatusUploading)

	// Calculate chunk parameters
	chunkSize := req.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 5 * 1024 * 1024 // 5MB default
	}
	totalChunks := (req.Size + chunkSize - 1) / chunkSize

	// Initialize multipart upload
	uploadInfo, err := h.storage.InitMultipartUpload(ctx, &storage.MultipartUploadParams{
		FileID:      fileRecord.ID,
		Filename:    fileRecord.Filename,
		ContentType: req.ContentType,
		Size:        req.Size,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize upload"})
		return
	}

	// Store file record
	if err := h.storeFileRecord(ctx, fileRecord); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store file record"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"file_id":      fileRecord.ID,
		"upload_id":    uploadInfo.UploadID,
		"chunk_size":   chunkSize,
		"total_chunks": totalChunks,
		"websocket_url": fmt.Sprintf("ws://%s:%d%s?file_id=%s", 
			h.config.Server.Host, h.config.Server.Port, h.config.WebSocket.Path, fileRecord.ID),
	})
}

// ChunkedUploadChunkHandler handles individual chunk uploads
func (h *UploadHandler) ChunkedUploadChunkHandler(c *gin.Context) {
	ctx := c.Request.Context()
	fileID := c.Param("file_id")
	uploadID := c.Query("upload_id")
	chunkNumberStr := c.Query("chunk_number")

	chunkNumber, err := strconv.Atoi(chunkNumberStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid chunk number"})
		return
	}

	// Get file record
	fileRecord, err := h.getFileRecord(ctx, fileID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	// Read chunk data
	body := c.Request.Body
	defer body.Close()

	// Upload chunk
	partInfo, err := h.storage.UploadPart(ctx, &storage.UploadPartParams{
		FileID:      fileID,
		UploadID:    uploadID,
		PartNumber:  chunkNumber,
		Reader:      body,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload chunk"})
		return
	}

	// Update progress
	progress := float64(chunkNumber) / float64((fileRecord.Size+5*1024*1024-1)/(5*1024*1024)) * 0.8
	fileRecord.UpdateProgress(progress, fmt.Sprintf("Uploading chunk %d", chunkNumber))
	h.notifyProgress(fileRecord, fmt.Sprintf("Uploaded chunk %d", chunkNumber))

	c.JSON(http.StatusOK, gin.H{
		"part_number": chunkNumber,
		"etag":        partInfo.ETag,
	})
}

// ChunkedUploadCompleteHandler completes a chunked upload
func (h *UploadHandler) ChunkedUploadCompleteHandler(c *gin.Context) {
	ctx := c.Request.Context()
	fileID := c.Param("file_id")

	var req struct {
		UploadID string                 `json:"upload_id" binding:"required"`
		Parts    []storage.CompletePart `json:"parts" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get file record
	fileRecord, err := h.getFileRecord(ctx, fileID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	// Complete multipart upload
	storageInfo, err := h.storage.CompleteMultipartUpload(ctx, &storage.CompleteMultipartUploadParams{
		FileID:   fileID,
		UploadID: req.UploadID,
		Parts:    req.Parts,
	})
	if err != nil {
		fileRecord.SetStatus(models.StatusFailed, fmt.Sprintf("Failed to complete upload: %v", err))
		h.notifyProgress(fileRecord, "Upload completion failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete upload"})
		return
	}

	// Update file record
	fileRecord.StoragePath = storageInfo.Key
	fileRecord.SetStatus(models.StatusPending)
	fileRecord.UpdateProgress(0.8, "Upload completed, ready for processing")

	if err := h.updateFileRecord(ctx, fileRecord); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update file record"})
		return
	}

	// Notify completion
	h.notifyProgress(fileRecord, "Upload completed successfully")

	c.JSON(http.StatusOK, gin.H{
		"file_id":      fileRecord.ID,
		"status":       fileRecord.Status,
		"storage_info": storageInfo,
	})
}

// ProgressReader wraps an io.Reader to track read progress
type ProgressReader struct {
	Reader   io.Reader
	Total    int64
	Current  int64
	FileID   string
	Tracker  *websocket.ProgressTracker
	Callback func(progress float64)
}

// Read implements io.Reader interface with progress tracking
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.Current += int64(n)
	
	progress := float64(pr.Current) / float64(pr.Total)
	if pr.Callback != nil {
		pr.Callback(progress)
	}
	
	if pr.Tracker != nil {
		pr.Tracker.UpdateProgress(pr.FileID, models.ProcessingProgress{
			FileID:   pr.FileID,
			Step:     models.StepUpload,
			Progress: progress,
			Message:  fmt.Sprintf("Uploaded %d/%d bytes", pr.Current, pr.Total),
		})
	}
	
	return n, err
}

// Helper functions

func getFileTypeFromName(filename string) models.FileType {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".csv":
		return models.FileTypeCSV
	case ".pdf":
		return models.FileTypePDF
	case ".docx":
		return models.FileTypeDOCX
	case ".xlsx":
		return models.FileTypeXLSX
	case ".jpg", ".jpeg":
		return models.FileTypeJPEG
	case ".png":
		return models.FileTypePNG
	case ".webp":
		return models.FileTypeWEBP
	default:
		return ""
	}
}

func isAllowedFileType(fileType models.FileType, allowedTypes []string) bool {
	if len(allowedTypes) == 0 {
		return true // No restrictions
	}
	
	for _, allowed := range allowedTypes {
		if string(fileType) == allowed {
			return true
		}
	}
	return false
}

func (h *UploadHandler) notifyProgress(fileRecord *models.FileRecord, message string) {
	if h.progressTracker != nil {
		progress := models.ProcessingProgress{
			FileID:    fileRecord.ID,
			Step:      models.StepUpload,
			Progress:  fileRecord.Progress,
			Message:   message,
			StartedAt: time.Now(),
		}
		h.progressTracker.UpdateProgress(fileRecord.ID, progress)
	}
}

// Database operations (implement based on your database layer)
func (h *UploadHandler) storeFileRecord(ctx context.Context, record *models.FileRecord) error {
	// TODO: Implement database storage
	return nil
}

func (h *UploadHandler) getFileRecord(ctx context.Context, fileID string) (*models.FileRecord, error) {
	// TODO: Implement database retrieval
	return nil, fmt.Errorf("not implemented")
}

func (h *UploadHandler) updateFileRecord(ctx context.Context, record *models.FileRecord) error {
	// TODO: Implement database update
	return nil
}