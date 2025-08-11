package storage

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/pyairtable/go-services/file-processing-service/internal/models"
)

// Interface defines the storage backend interface
type Interface interface {
	// StreamingUpload uploads a file using streaming
	StreamingUpload(ctx context.Context, params *UploadParams) (*models.StorageInfo, error)
	
	// InitMultipartUpload initializes a multipart upload
	InitMultipartUpload(ctx context.Context, params *MultipartUploadParams) (*UploadInfo, error)
	
	// UploadPart uploads a part in a multipart upload
	UploadPart(ctx context.Context, params *UploadPartParams) (*PartInfo, error)
	
	// CompleteMultipartUpload completes a multipart upload
	CompleteMultipartUpload(ctx context.Context, params *CompleteMultipartUploadParams) (*models.StorageInfo, error)
	
	// AbortMultipartUpload aborts a multipart upload
	AbortMultipartUpload(ctx context.Context, fileID, uploadID string) error
	
	// Download downloads a file
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	
	// GetDownloadURL generates a pre-signed download URL
	GetDownloadURL(ctx context.Context, key string, expiry time.Duration) (string, error)
	
	// Delete deletes a file
	Delete(ctx context.Context, key string) error
	
	// Exists checks if a file exists
	Exists(ctx context.Context, key string) (bool, error)
	
	// GetFileInfo gets information about a file
	GetFileInfo(ctx context.Context, key string) (*FileInfo, error)
	
	// ListFiles lists files with optional prefix
	ListFiles(ctx context.Context, prefix string, limit int) ([]*FileInfo, error)
	
	// Copy copies a file from one key to another
	Copy(ctx context.Context, sourceKey, destKey string) error
	
	// Move moves a file from one key to another
	Move(ctx context.Context, sourceKey, destKey string) error
	
	// GenerateThumbnail generates a thumbnail for supported file types
	GenerateThumbnail(ctx context.Context, sourceKey, thumbnailKey string, width, height int) error
	
	// Health checks the health of the storage backend
	Health(ctx context.Context) error
}

// UploadParams contains parameters for file upload
type UploadParams struct {
	FileID      string
	Filename    string
	ContentType string
	Size        int64
	Reader      io.Reader
	Metadata    map[string]string
}

// MultipartUploadParams contains parameters for multipart upload initialization
type MultipartUploadParams struct {
	FileID      string
	Filename    string
	ContentType string
	Size        int64
	Metadata    map[string]string
}

// UploadPartParams contains parameters for uploading a part
type UploadPartParams struct {
	FileID     string
	UploadID   string
	PartNumber int
	Reader     io.Reader
	Size       int64
}

// CompleteMultipartUploadParams contains parameters for completing multipart upload
type CompleteMultipartUploadParams struct {
	FileID   string
	UploadID string
	Parts    []CompletePart
}

// CompletePart represents a completed part in multipart upload
type CompletePart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
}

// UploadInfo contains information about an initiated upload
type UploadInfo struct {
	UploadID    string            `json:"upload_id"`
	Key         string            `json:"key"`
	UploadURL   string            `json:"upload_url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	ExpiresAt   time.Time         `json:"expires_at"`
}

// PartInfo contains information about an uploaded part
type PartInfo struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
	Size       int64  `json:"size"`
}

// FileInfo contains information about a stored file
type FileInfo struct {
	Key          string            `json:"key"`
	Size         int64             `json:"size"`
	ContentType  string            `json:"content_type"`
	LastModified time.Time         `json:"last_modified"`
	ETag         string            `json:"etag"`
	Metadata     map[string]string `json:"metadata"`
}

// Config contains storage configuration
type Config struct {
	Provider string `json:"provider"`
	
	// S3/MinIO configuration
	Endpoint        string `json:"endpoint,omitempty"`
	Region          string `json:"region,omitempty"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token,omitempty"`
	UseSSL          bool   `json:"use_ssl"`
	ForcePathStyle  bool   `json:"force_path_style"`
	
	// Local storage configuration
	BasePath   string `json:"base_path,omitempty"`
	ServeFiles bool   `json:"serve_files,omitempty"`
	URLPrefix  string `json:"url_prefix,omitempty"`
	
	// Common configuration
	MaxRetries      int           `json:"max_retries"`
	RetryDelay      time.Duration `json:"retry_delay"`
	RequestTimeout  time.Duration `json:"request_timeout"`
	PartSize        int64         `json:"part_size"`        // For multipart uploads
	MaxUploadParts  int           `json:"max_upload_parts"` // Maximum number of parts
	Concurrency     int           `json:"concurrency"`      // Upload concurrency
}

// Storage errors
type StorageError struct {
	Op  string
	Err error
}

func (e *StorageError) Error() string {
	return e.Op + ": " + e.Err.Error()
}

func (e *StorageError) Unwrap() error {
	return e.Err
}

// Common storage errors
var (
	ErrFileNotFound    = &StorageError{Op: "storage", Err: fmt.Errorf("file not found")}
	ErrUploadFailed    = &StorageError{Op: "storage", Err: fmt.Errorf("upload failed")}
	ErrDownloadFailed  = &StorageError{Op: "storage", Err: fmt.Errorf("download failed")}
	ErrDeleteFailed    = &StorageError{Op: "storage", Err: fmt.Errorf("delete failed")}
	ErrInvalidKey      = &StorageError{Op: "storage", Err: fmt.Errorf("invalid key")}
	ErrPermissionDenied = &StorageError{Op: "storage", Err: fmt.Errorf("permission denied")}
)

// Helper functions

// GenerateKey generates a storage key for a file
func GenerateKey(fileID, filename string) string {
	timestamp := time.Now().Format("2006/01/02")
	return fmt.Sprintf("files/%s/%s_%s", timestamp, fileID, filename)
}

// GenerateThumbnailKey generates a storage key for a thumbnail
func GenerateThumbnailKey(fileID, filename string, width, height int) string {
	timestamp := time.Now().Format("2006/01/02")
	return fmt.Sprintf("thumbnails/%s/%s_%s_%dx%d.jpg", timestamp, fileID, filename, width, height)
}

// GetContentType determines content type from filename
func GetContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	contentTypes := map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".webp": "image/webp",
		".pdf":  "application/pdf",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".csv":  "text/csv",
	}
	
	if contentType, ok := contentTypes[ext]; ok {
		return contentType
	}
	return "application/octet-stream"
}

// ValidateKey validates a storage key
func ValidateKey(key string) error {
	if key == "" {
		return ErrInvalidKey
	}
	if len(key) > 1024 {
		return &StorageError{Op: "validate", Err: fmt.Errorf("key too long")}
	}
	if strings.Contains(key, "..") {
		return &StorageError{Op: "validate", Err: fmt.Errorf("key contains invalid path")}
	}
	return nil
}

// CalculatePartSize calculates optimal part size for multipart upload
func CalculatePartSize(fileSize int64, maxParts int) int64 {
	minPartSize := int64(5 * 1024 * 1024) // 5MB minimum
	maxPartSize := int64(5 * 1024 * 1024 * 1024) // 5GB maximum
	
	partSize := fileSize / int64(maxParts)
	if partSize < minPartSize {
		partSize = minPartSize
	}
	if partSize > maxPartSize {
		partSize = maxPartSize
	}
	
	return partSize
}