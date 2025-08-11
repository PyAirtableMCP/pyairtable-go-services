package integration

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/pyairtable/go-services/file-processing-service/internal/models"
	"github.com/pyairtable/go-services/file-processing-service/internal/processors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBasicCSVProcessing tests basic CSV processing functionality
func TestBasicCSVProcessing(t *testing.T) {
	// Create test CSV data
	csvData := `Name,Age,City
John Doe,30,New York
Jane Smith,25,Los Angeles
Bob Johnson,35,Chicago`

	// Create CSV processor
	processor := processors.NewCSVProcessor()
	
	// Create processing context
	config := &models.ProcessingConfig{
		CSV: models.CSVConfig{
			MaxRows:            1000,
			MaxColumns:         100,
			MaxCellSize:        32768,
			DetectDelimiter:    true,
			SkipEmptyLines:     true,
			TrimSpaces:         true,
			StreamingThreshold: 10 << 20, // 10MB
			BufferSize:         8192,
			SampleSize:         1024,
		},
	}
	
	fileRecord := models.NewFileRecord(
		"test.csv",
		"text/csv",
		int64(len(csvData)),
		models.FileTypeCSV,
	)
	
	ctx := &processors.ProcessingContext{
		FileRecord: fileRecord,
		Reader:     bytes.NewReader([]byte(csvData)),
		Config:     config,
		Context:    context.Background(),
	}
	
	// Process the CSV
	result, err := processor.Process(ctx)
	require.NoError(t, err, "CSV processing should not fail")
	
	// Verify results
	assert.NotNil(t, result, "Result should not be nil")
	
	// Check records
	records, ok := result["records"].([]models.JSONMap)
	require.True(t, ok, "Records should be present and correct type")
	assert.Len(t, records, 3, "Should have 3 data records")
	
	// Check first record
	firstRecord := records[0]
	assert.Equal(t, "John Doe", firstRecord["Name"], "Name should match")
	assert.Equal(t, int64(30), firstRecord["Age"], "Age should be converted to int")
	assert.Equal(t, "New York", firstRecord["City"], "City should match")
	
	// Check columns
	columns, ok := result["columns"].([]string)
	require.True(t, ok, "Columns should be present")
	assert.Equal(t, []string{"Name", "Age", "City"}, columns, "Columns should match")
	
	// Check count
	count, ok := result["count"].(int)
	require.True(t, ok, "Count should be present")
	assert.Equal(t, 3, count, "Count should be 3")
}

// TestFileRecordCreation tests file record creation and validation
func TestFileRecordCreation(t *testing.T) {
	filename := "test.csv"
	contentType := "text/csv"
	size := int64(1024)
	fileType := models.FileTypeCSV
	
	record := models.NewFileRecord(filename, contentType, size, fileType)
	
	// Verify basic properties
	assert.NotEmpty(t, record.ID, "ID should be generated")
	assert.Equal(t, filename, record.Filename, "Filename should match")
	assert.Equal(t, filename, record.OriginalName, "Original name should match")
	assert.Equal(t, contentType, record.ContentType, "Content type should match")
	assert.Equal(t, size, record.Size, "Size should match")
	assert.Equal(t, fileType, record.FileType, "File type should match")
	assert.Equal(t, models.StatusPending, record.Status, "Status should be pending")
	assert.Equal(t, 0.0, record.Progress, "Progress should be 0")
	assert.False(t, record.VirusScanned, "Virus scanned should be false")
	
	// Test validation
	err := record.Validate()
	assert.NoError(t, err, "Valid record should pass validation")
	
	// Test status updates
	record.SetStatus(models.StatusProcessing)
	assert.Equal(t, models.StatusProcessing, record.Status, "Status should be updated")
	
	record.SetStatus(models.StatusCompleted)
	assert.Equal(t, models.StatusCompleted, record.Status, "Status should be completed")
	assert.NotNil(t, record.ProcessedAt, "ProcessedAt should be set")
	assert.Equal(t, 1.0, record.Progress, "Progress should be 1.0")
	
	// Test progress updates
	record.UpdateProgress(0.5, "Half done")
	assert.Equal(t, 0.5, record.Progress, "Progress should be updated")
	assert.NotNil(t, record.ProcessingData, "Processing data should be set")
}

// TestConfigurationDefaults tests default configuration
func TestConfigurationDefaults(t *testing.T) {
	config := models.DefaultConfig()
	
	// Test server defaults
	assert.Equal(t, "0.0.0.0", config.Server.Host, "Default host should be 0.0.0.0")
	assert.Equal(t, 8080, config.Server.Port, "Default port should be 8080")
	assert.Equal(t, 30*time.Second, config.Server.ReadTimeout, "Default read timeout")
	
	// Test database defaults
	assert.Equal(t, "postgres", config.Database.Driver, "Default database driver")
	assert.Equal(t, "localhost", config.Database.Host, "Default database host")
	assert.Equal(t, 5432, config.Database.Port, "Default database port")
	
	// Test Redis defaults
	assert.Equal(t, "localhost", config.Redis.Host, "Default Redis host")
	assert.Equal(t, 6379, config.Redis.Port, "Default Redis port")
	assert.Equal(t, 0, config.Redis.DB, "Default Redis DB")
	
	// Test storage defaults
	assert.Equal(t, "local", config.Storage.Provider, "Default storage provider")
	assert.Equal(t, int64(100<<20), config.Storage.MaxFileSize, "Default max file size")
	
	// Test processing defaults
	assert.Equal(t, 4, config.Processing.WorkerCount, "Default worker count")
	assert.Equal(t, 1000, config.Processing.QueueSize, "Default queue size")
	assert.Equal(t, 5*time.Minute, config.Processing.ProcessingTimeout, "Default processing timeout")
	
	// Test CSV defaults
	assert.Equal(t, 100000, config.Processing.CSV.MaxRows, "Default CSV max rows")
	assert.Equal(t, 1000, config.Processing.CSV.MaxColumns, "Default CSV max columns")
	assert.True(t, config.Processing.CSV.DetectDelimiter, "Default delimiter detection")
	
	// Test security defaults
	assert.False(t, config.Security.EnableVirusScanning, "Default virus scanning disabled")
	assert.True(t, config.Security.EnableFileTypeCheck, "Default file type check enabled")
	assert.True(t, config.Security.BlockExecutables, "Default block executables")
	
	// Test monitoring defaults
	assert.True(t, config.Monitoring.Enabled, "Default monitoring enabled")
	assert.Equal(t, 9090, config.Monitoring.MetricsPort, "Default metrics port")
	assert.Equal(t, "info", config.Monitoring.LogLevel, "Default log level")
	
	// Test WebSocket defaults
	assert.True(t, config.WebSocket.Enabled, "Default WebSocket enabled")
	assert.Equal(t, "/ws", config.WebSocket.Path, "Default WebSocket path")
	assert.Equal(t, 1000, config.WebSocket.MaxConnections, "Default max connections")
}