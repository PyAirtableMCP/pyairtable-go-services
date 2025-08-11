package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// FileType represents supported file types
type FileType string

const (
	FileTypeCSV   FileType = "csv"
	FileTypePDF   FileType = "pdf"
	FileTypeDOCX  FileType = "docx"
	FileTypeXLSX  FileType = "xlsx"
	FileTypeJPEG  FileType = "jpeg"
	FileTypePNG   FileType = "png"
	FileTypeWEBP  FileType = "webp"
)

// ProcessingStatus represents the status of file processing
type ProcessingStatus string

const (
	StatusPending    ProcessingStatus = "pending"
	StatusProcessing ProcessingStatus = "processing"
	StatusCompleted  ProcessingStatus = "completed"
	StatusFailed     ProcessingStatus = "failed"
	StatusUploading  ProcessingStatus = "uploading"
)

// ProcessingStep represents individual processing steps
type ProcessingStep string

const (
	StepUpload      ProcessingStep = "upload"
	StepVirusScan   ProcessingStep = "virus_scan"
	StepExtraction  ProcessingStep = "extraction"
	StepValidation  ProcessingStep = "validation"
	StepCompression ProcessingStep = "compression"
	StepStorage     ProcessingStep = "storage"
	StepComplete    ProcessingStep = "complete"
)

// ProcessingProgress tracks progress of file processing
type ProcessingProgress struct {
	FileID       string           `json:"file_id"`
	Step         ProcessingStep   `json:"step"`
	Progress     float64          `json:"progress"` // 0.0 to 1.0
	Message      string           `json:"message"`
	StartedAt    time.Time        `json:"started_at"`
	CompletedAt  *time.Time       `json:"completed_at,omitempty"`
	Error        string           `json:"error,omitempty"`
	Details      JSONMap          `json:"details,omitempty"`
}

// FileRecord represents a file in the system
type FileRecord struct {
	ID              string             `json:"id" db:"id"`
	Filename        string             `json:"filename" db:"filename"`
	OriginalName    string             `json:"original_name" db:"original_name"`
	FileType        FileType           `json:"file_type" db:"file_type"`
	ContentType     string             `json:"content_type" db:"content_type"`
	Size            int64              `json:"size" db:"size"`
	CompressedSize  *int64             `json:"compressed_size,omitempty" db:"compressed_size"`
	Status          ProcessingStatus   `json:"status" db:"status"`
	Progress        float64            `json:"progress" db:"progress"`
	CreatedAt       time.Time          `json:"created_at" db:"created_at"`
	ProcessedAt     *time.Time         `json:"processed_at,omitempty" db:"processed_at"`
	LastActivity    time.Time          `json:"last_activity" db:"last_activity"`
	ErrorMessage    string             `json:"error_message,omitempty" db:"error_message"`
	ProcessingData  JSONMap            `json:"processing_data,omitempty" db:"processing_data"`
	StoragePath     string             `json:"storage_path" db:"storage_path"`
	ThumbnailPath   string             `json:"thumbnail_path,omitempty" db:"thumbnail_path"`
	Metadata        FileMetadata       `json:"metadata" db:"metadata"`
	VirusScanned    bool               `json:"virus_scanned" db:"virus_scanned"`
	VirusScanResult *VirusScanResult   `json:"virus_scan_result,omitempty" db:"virus_scan_result"`
}

// FileMetadata contains extracted metadata from files
type FileMetadata struct {
	Width         *int             `json:"width,omitempty"`
	Height        *int             `json:"height,omitempty"`
	PageCount     *int             `json:"page_count,omitempty"`
	SheetCount    *int             `json:"sheet_count,omitempty"`
	RowCount      *int             `json:"row_count,omitempty"`
	ColumnCount   *int             `json:"column_count,omitempty"`
	WordCount     *int             `json:"word_count,omitempty"`
	CharCount     *int             `json:"char_count,omitempty"`
	Language      string           `json:"language,omitempty"`
	Encoding      string           `json:"encoding,omitempty"`
	HasImages     bool             `json:"has_images"`
	HasTables     bool             `json:"has_tables"`
	CreatedBy     string           `json:"created_by,omitempty"`
	LastModified  *time.Time       `json:"last_modified,omitempty"`
	Tags          []string         `json:"tags,omitempty"`
	CustomFields  map[string]interface{} `json:"custom_fields,omitempty"`
}

// VirusScanResult contains virus scanning results
type VirusScanResult struct {
	Clean       bool       `json:"clean"`
	Threats     []string   `json:"threats,omitempty"`
	ScannedAt   time.Time  `json:"scanned_at"`
	ScannerInfo string     `json:"scanner_info"`
}

// ProcessingResult represents the result of file processing
type ProcessingResult struct {
	FileID        string               `json:"file_id"`
	Status        ProcessingStatus     `json:"status"`
	Data          interface{}          `json:"data,omitempty"`
	Records       []AirtableRecord     `json:"records,omitempty"`
	RecordsCount  int                  `json:"records_count"`
	Error         string               `json:"error,omitempty"`
	ProcessingTime time.Duration       `json:"processing_time"`
	Metadata      FileMetadata         `json:"metadata"`
	StorageInfo   StorageInfo          `json:"storage_info"`
}

// AirtableRecord represents a record in Airtable format
type AirtableRecord struct {
	Fields map[string]interface{} `json:"fields"`
}

// StorageInfo contains information about file storage
type StorageInfo struct {
	Bucket        string    `json:"bucket"`
	Key           string    `json:"key"`
	URL           string    `json:"url,omitempty"`
	ThumbnailURL  string    `json:"thumbnail_url,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	Size          int64     `json:"size"`
	CompressedSize *int64   `json:"compressed_size,omitempty"`
	ContentType   string    `json:"content_type"`
}

// UploadInfo contains information about file upload
type UploadInfo struct {
	FileID      string    `json:"file_id"`
	UploadURL   string    `json:"upload_url"`
	Method      string    `json:"method"`
	Headers     map[string]string `json:"headers,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
	MaxSize     int64     `json:"max_size"`
}

// JSONMap is a custom type for JSON data
type JSONMap map[string]interface{}

// Value implements the driver.Valuer interface for database storage
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements the sql.Scanner interface for database retrieval
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into JSONMap", value)
	}
	
	return json.Unmarshal(bytes, j)
}

// Value implements the driver.Valuer interface for FileMetadata
func (fm FileMetadata) Value() (driver.Value, error) {
	return json.Marshal(fm)
}

// Scan implements the sql.Scanner interface for FileMetadata
func (fm *FileMetadata) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into FileMetadata", value)
	}
	
	return json.Unmarshal(bytes, fm)
}

// Value implements the driver.Valuer interface for VirusScanResult
func (vsr *VirusScanResult) Value() (driver.Value, error) {
	if vsr == nil {
		return nil, nil
	}
	return json.Marshal(vsr)
}

// Scan implements the sql.Scanner interface for VirusScanResult
func (vsr *VirusScanResult) Scan(value interface{}) error {
	if value == nil {
		*vsr = VirusScanResult{}
		return nil
	}
	
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into VirusScanResult", value)
	}
	
	return json.Unmarshal(bytes, vsr)
}

// NewFileRecord creates a new file record with default values
func NewFileRecord(filename, contentType string, size int64, fileType FileType) *FileRecord {
	now := time.Now()
	return &FileRecord{
		ID:           uuid.New().String(),
		Filename:     filename,
		OriginalName: filename,
		FileType:     fileType,
		ContentType:  contentType,
		Size:         size,
		Status:       StatusPending,
		Progress:     0.0,
		CreatedAt:    now,
		LastActivity: now,
		Metadata:     FileMetadata{},
		VirusScanned: false,
	}
}

// IsImage returns true if the file type is an image
func (fr *FileRecord) IsImage() bool {
	return fr.FileType == FileTypeJPEG || fr.FileType == FileTypePNG || fr.FileType == FileTypeWEBP
}

// IsDocument returns true if the file type is a document
func (fr *FileRecord) IsDocument() bool {
	return fr.FileType == FileTypePDF || fr.FileType == FileTypeDOCX
}

// IsSpreadsheet returns true if the file type is a spreadsheet
func (fr *FileRecord) IsSpreadsheet() bool {
	return fr.FileType == FileTypeCSV || fr.FileType == FileTypeXLSX
}

// CanHaveThumbnail returns true if the file type can have a thumbnail
func (fr *FileRecord) CanHaveThumbnail() bool {
	return fr.IsImage() || fr.FileType == FileTypePDF
}

// UpdateProgress updates the processing progress
func (fr *FileRecord) UpdateProgress(progress float64, message string) {
	fr.Progress = progress
	fr.LastActivity = time.Now()
	if fr.ProcessingData == nil {
		fr.ProcessingData = make(JSONMap)
	}
	fr.ProcessingData["last_message"] = message
}

// SetStatus updates the file status and related fields
func (fr *FileRecord) SetStatus(status ProcessingStatus, errorMsg ...string) {
	fr.Status = status
	fr.LastActivity = time.Now()
	
	if status == StatusCompleted {
		now := time.Now()
		fr.ProcessedAt = &now
		fr.Progress = 1.0
	}
	
	if status == StatusFailed && len(errorMsg) > 0 {
		fr.ErrorMessage = errorMsg[0]
	}
}

// Validate validates the file record
func (fr *FileRecord) Validate() error {
	if fr.ID == "" {
		return fmt.Errorf("file ID is required")
	}
	if fr.Filename == "" {
		return fmt.Errorf("filename is required")
	}
	if fr.Size <= 0 {
		return fmt.Errorf("file size must be positive")
	}
	if fr.FileType == "" {
		return fmt.Errorf("file type is required")
	}
	return nil
}