package processors

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pyairtable/go-services/file-processing-service/internal/models"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// CSVProcessor handles CSV file processing with streaming support
type CSVProcessor struct {
	*BaseProcessor
}

// NewCSVProcessor creates a new CSV processor
func NewCSVProcessor() *CSVProcessor {
	return &CSVProcessor{
		BaseProcessor: NewBaseProcessor([]models.FileType{models.FileTypeCSV}),
	}
}

// Process processes a CSV file
func (cp *CSVProcessor) Process(ctx *ProcessingContext) (models.JSONMap, error) {
	// Use streaming for large files
	if ctx.FileRecord.Size > ctx.Config.CSV.StreamingThreshold {
		return cp.ProcessStream(ctx, func(step models.ProcessingStep, progress float64, message string) {
			cp.UpdateProgress(ctx, step, progress, message)
		})
	}
	
	// Regular processing for smaller files
	return cp.processRegular(ctx)
}

// ProcessStream processes a CSV file using streaming
func (cp *CSVProcessor) ProcessStream(ctx *ProcessingContext, callback ProgressCallback) (models.JSONMap, error) {
	cp.UpdateProgress(ctx, models.StepExtraction, 0.1, "Starting CSV stream processing")
	
	// Detect encoding and delimiter
	reader, encoding, delimiter, err := cp.prepareReader(ctx)
	if err != nil {
		return nil, &ProcessingError{Stage: "preparation", Message: "failed to prepare CSV reader", Cause: err}
	}
	
	cp.UpdateProgress(ctx, models.StepExtraction, 0.2, fmt.Sprintf("Detected encoding: %s, delimiter: %c", encoding, delimiter))
	
	// Create CSV reader with detected delimiter
	csvReader := csv.NewReader(reader)
	csvReader.Comma = delimiter
	csvReader.LazyQuotes = true
	csvReader.TrimLeadingSpace = ctx.Config.CSV.TrimSpaces
	
	// Read headers
	headers, err := csvReader.Read()
	if err != nil {
		return nil, &ProcessingError{Stage: "headers", Message: "failed to read CSV headers", Cause: err}
	}
	
	// Validate headers
	if len(headers) == 0 {
		return nil, &ProcessingError{Stage: "validation", Message: "CSV file has no columns"}
	}
	
	if len(headers) > ctx.Config.CSV.MaxColumns {
		return nil, &ProcessingError{Stage: "validation", Message: fmt.Sprintf("CSV file has too many columns (%d > %d)", len(headers), ctx.Config.CSV.MaxColumns)}
	}
	
	// Clean and validate headers
	cleanHeaders := make([]string, len(headers))
	for i, header := range headers {
		cleanHeaders[i] = cp.cleanString(header)
		if cleanHeaders[i] == "" {
			cleanHeaders[i] = fmt.Sprintf("Column_%d", i+1)
		}
	}
	
	cp.UpdateProgress(ctx, models.StepExtraction, 0.3, fmt.Sprintf("Processing CSV with %d columns", len(cleanHeaders)))
	
	// Process records with streaming
	records := make([]models.JSONMap, 0, 1000) // Pre-allocate with reasonable capacity
	recordCount := 0
	totalBytesProcessed := int64(0)
	
	// Estimate total bytes for progress calculation
	estimatedTotalBytes := ctx.FileRecord.Size
	
	for {
		select {
		case <-ctx.Context.Done():
			return nil, ctx.Context.Err()
		default:
		}
		
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip invalid records but continue processing
			continue
		}
		
		recordCount++
		if recordCount > ctx.Config.CSV.MaxRows {
			return nil, &ProcessingError{Stage: "validation", Message: fmt.Sprintf("CSV file has too many rows (%d > %d)", recordCount, ctx.Config.CSV.MaxRows)}
		}
		
		// Skip empty lines if configured
		if ctx.Config.CSV.SkipEmptyLines && cp.isEmptyRecord(record) {
			continue
		}
		
		// Process record
		processedRecord := cp.processRecord(record, cleanHeaders, ctx.Config.CSV)
		if processedRecord != nil {
			records = append(records, processedRecord)
		}
		
		// Update progress periodically
		if recordCount%1000 == 0 {
			// Estimate bytes processed (rough approximation)
			avgRecordSize := estimatedTotalBytes / int64(ctx.Config.CSV.MaxRows)
			totalBytesProcessed = int64(recordCount) * avgRecordSize
			progress := 0.3 + (float64(totalBytesProcessed)/float64(estimatedTotalBytes))*0.6
			if progress > 0.9 {
				progress = 0.9
			}
			
			callback(models.StepExtraction, progress, fmt.Sprintf("Processed %d records", recordCount))
		}
		
		// Memory management for large files
		if len(records) >= 10000 {
			// For very large files, we might want to implement batch processing
			// or write to temporary storage
			break
		}
	}
	
	cp.UpdateProgress(ctx, models.StepValidation, 0.95, "Finalizing CSV processing")
	
	// Create result
	result := models.JSONMap{
		"records":      records,
		"count":        len(records),
		"total_rows":   recordCount,
		"columns":      cleanHeaders,
		"column_count": len(cleanHeaders),
		"encoding":     encoding,
		"delimiter":    string(delimiter),
		"metadata": models.JSONMap{
			"has_headers":     true,
			"rows_processed":  recordCount,
			"rows_included":   len(records),
			"bytes_processed": totalBytesProcessed,
		},
	}
	
	// Update file metadata
	ctx.FileRecord.Metadata.RowCount = &recordCount
	ctx.FileRecord.Metadata.ColumnCount = &len(cleanHeaders)
	ctx.FileRecord.Metadata.Encoding = encoding
	
	return result, nil
}

// processRegular processes smaller CSV files in memory
func (cp *CSVProcessor) processRegular(ctx *ProcessingContext) (models.JSONMap, error) {
	cp.UpdateProgress(ctx, models.StepExtraction, 0.1, "Reading CSV file into memory")
	
	// Read entire file into memory
	data, err := io.ReadAll(ctx.Reader)
	if err != nil {
		return nil, &ProcessingError{Stage: "reading", Message: "failed to read CSV file", Cause: err}
	}
	
	// Detect encoding and convert if necessary
	reader, encoding, delimiter, err := cp.prepareReaderFromBytes(data, ctx)
	if err != nil {
		return nil, &ProcessingError{Stage: "preparation", Message: "failed to prepare CSV reader", Cause: err}
	}
	
	cp.UpdateProgress(ctx, models.StepExtraction, 0.3, fmt.Sprintf("Processing CSV (encoding: %s, delimiter: %c)", encoding, delimiter))
	
	// Create CSV reader
	csvReader := csv.NewReader(reader)
	csvReader.Comma = delimiter
	csvReader.LazyQuotes = true
	csvReader.TrimLeadingSpace = ctx.Config.CSV.TrimSpaces
	
	// Read all records
	allRecords, err := csvReader.ReadAll()
	if err != nil {
		return nil, &ProcessingError{Stage: "parsing", Message: "failed to parse CSV", Cause: err}
	}
	
	if len(allRecords) == 0 {
		return nil, &ProcessingError{Stage: "validation", Message: "CSV file is empty"}
	}
	
	cp.UpdateProgress(ctx, models.StepExtraction, 0.6, fmt.Sprintf("Read %d rows", len(allRecords)))
	
	// Process headers
	headers := allRecords[0]
	cleanHeaders := make([]string, len(headers))
	for i, header := range headers {
		cleanHeaders[i] = cp.cleanString(header)
		if cleanHeaders[i] == "" {
			cleanHeaders[i] = fmt.Sprintf("Column_%d", i+1)
		}
	}
	
	// Process data records
	records := make([]models.JSONMap, 0, len(allRecords)-1)
	for i, record := range allRecords[1:] {
		if ctx.Config.CSV.SkipEmptyLines && cp.isEmptyRecord(record) {
			continue
		}
		
		processedRecord := cp.processRecord(record, cleanHeaders, ctx.Config.CSV)
		if processedRecord != nil {
			records = append(records, processedRecord)
		}
		
		// Update progress
		if i%1000 == 0 {
			progress := 0.6 + (float64(i)/float64(len(allRecords)-1))*0.3
			cp.UpdateProgress(ctx, models.StepExtraction, progress, fmt.Sprintf("Processed %d/%d records", i+1, len(allRecords)-1))
		}
	}
	
	cp.UpdateProgress(ctx, models.StepValidation, 0.95, "Finalizing CSV processing")
	
	// Create result
	result := models.JSONMap{
		"records":      records,
		"count":        len(records),
		"total_rows":   len(allRecords) - 1, // Exclude header
		"columns":      cleanHeaders,
		"column_count": len(cleanHeaders),
		"encoding":     encoding,
		"delimiter":    string(delimiter),
		"metadata": models.JSONMap{
			"has_headers":     true,
			"rows_processed":  len(allRecords) - 1,
			"rows_included":   len(records),
			"bytes_processed": len(data),
		},
	}
	
	// Update file metadata
	rowCount := len(allRecords) - 1
	ctx.FileRecord.Metadata.RowCount = &rowCount
	ctx.FileRecord.Metadata.ColumnCount = &len(cleanHeaders)
	ctx.FileRecord.Metadata.Encoding = encoding
	
	return result, nil
}

// prepareReader prepares a reader with encoding detection and delimiter detection
func (cp *CSVProcessor) prepareReader(ctx *ProcessingContext) (io.Reader, string, rune, error) {
	// Read a sample for detection
	sample := make([]byte, ctx.Config.CSV.SampleSize)
	n, err := ctx.Reader.Read(sample)
	if err != nil && err != io.EOF {
		return nil, "", 0, err
	}
	sample = sample[:n]
	
	// Detect encoding
	encoding := cp.detectEncoding(sample)
	
	// Detect delimiter
	delimiter := cp.detectDelimiter(sample, ctx.Config.CSV)
	
	// Create a new reader with the sample and remaining data
	remainingData, _ := io.ReadAll(ctx.Reader)
	fullData := append(sample, remainingData...)
	
	return cp.prepareReaderFromBytes(fullData, ctx)
}

// prepareReaderFromBytes prepares a reader from byte data
func (cp *CSVProcessor) prepareReaderFromBytes(data []byte, ctx *ProcessingContext) (io.Reader, string, rune, error) {
	// Detect encoding
	encoding := cp.detectEncoding(data)
	
	// Convert encoding if necessary
	var reader io.Reader
	switch encoding {
	case "UTF-8":
		reader = bytes.NewReader(data)
	case "UTF-16":
		decoder := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()
		convertedData, _, err := transform.Bytes(decoder, data)
		if err != nil {
			return nil, "", 0, fmt.Errorf("failed to convert UTF-16 to UTF-8: %w", err)
		}
		reader = bytes.NewReader(convertedData)
		data = convertedData // Update data for delimiter detection
	case "Windows-1252":
		decoder := charmap.Windows1252.NewDecoder()
		convertedData, _, err := transform.Bytes(decoder, data)
		if err != nil {
			return nil, "", 0, fmt.Errorf("failed to convert Windows-1252 to UTF-8: %w", err)
		}
		reader = bytes.NewReader(convertedData)
		data = convertedData
	default:
		reader = bytes.NewReader(data)
	}
	
	// Detect delimiter
	delimiter := cp.detectDelimiter(data, ctx.Config.CSV)
	
	return reader, encoding, delimiter, nil
}

// detectEncoding detects the encoding of the data
func (cp *CSVProcessor) detectEncoding(data []byte) string {
	// Check for BOM
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return "UTF-8"
	}
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		return "UTF-16"
	}
	if len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF {
		return "UTF-16"
	}
	
	// Check if data is valid UTF-8
	if utf8.Valid(data) {
		return "UTF-8"
	}
	
	// Default to Windows-1252 for other cases
	return "Windows-1252"
}

// detectDelimiter detects the CSV delimiter
func (cp *CSVProcessor) detectDelimiter(data []byte, config models.CSVConfig) rune {
	if !config.DetectDelimiter {
		return ',' // Default delimiter
	}
	
	// Look at first few lines
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lines := make([]string, 0, 5)
	for i := 0; i < 5 && scanner.Scan(); i++ {
		lines = append(lines, scanner.Text())
	}
	
	if len(lines) == 0 {
		return ','
	}
	
	// Count potential delimiters
	delimiters := []rune{',', ';', '\t', '|'}
	scores := make(map[rune]int)
	
	for _, delimiter := range delimiters {
		consistent := true
		var fieldCount int
		
		for i, line := range lines {
			count := strings.Count(line, string(delimiter))
			if i == 0 {
				fieldCount = count
			} else if count != fieldCount {
				consistent = false
				break
			}
		}
		
		if consistent && fieldCount > 0 {
			scores[delimiter] = fieldCount
		}
	}
	
	// Return delimiter with highest score
	bestDelimiter := ','
	bestScore := 0
	for delimiter, score := range scores {
		if score > bestScore {
			bestScore = score
			bestDelimiter = delimiter
		}
	}
	
	return bestDelimiter
}

// processRecord processes a single CSV record
func (cp *CSVProcessor) processRecord(record []string, headers []string, config models.CSVConfig) models.JSONMap {
	if len(record) == 0 {
		return nil
	}
	
	result := make(models.JSONMap)
	
	for i, value := range record {
		var headerName string
		if i < len(headers) {
			headerName = headers[i]
		} else {
			headerName = fmt.Sprintf("Column_%d", i+1)
		}
		
		// Clean and validate value
		cleanValue := cp.cleanString(value)
		
		// Check cell size limit
		if len(cleanValue) > config.MaxCellSize {
			cleanValue = cleanValue[:config.MaxCellSize] + "..."
		}
		
		// Try to convert to appropriate type
		convertedValue := cp.convertValue(cleanValue)
		result[headerName] = convertedValue
	}
	
	return result
}

// convertValue attempts to convert a string value to appropriate type
func (cp *CSVProcessor) convertValue(value string) interface{} {
	if value == "" {
		return nil
	}
	
	// Try to convert to number
	if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
		return intVal
	}
	
	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal
	}
	
	// Try to convert to boolean
	if boolVal, err := strconv.ParseBool(value); err == nil {
		return boolVal
	}
	
	// Try to parse as date
	if dateVal, err := time.Parse("2006-01-02", value); err == nil {
		return dateVal.Format("2006-01-02")
	}
	
	if dateVal, err := time.Parse("01/02/2006", value); err == nil {
		return dateVal.Format("2006-01-02")
	}
	
	// Return as string
	return value
}

// cleanString cleans and trims a string value
func (cp *CSVProcessor) cleanString(value string) string {
	// Trim whitespace
	value = strings.TrimSpace(value)
	
	// Remove null bytes and other control characters
	value = strings.ReplaceAll(value, "\x00", "")
	value = strings.ReplaceAll(value, "\r", "")
	
	return value
}

// isEmptyRecord checks if a record is empty
func (cp *CSVProcessor) isEmptyRecord(record []string) bool {
	for _, field := range record {
		if strings.TrimSpace(field) != "" {
			return false
		}
	}
	return true
}

// StreamingProcessor interface implementation
func (cp *CSVProcessor) SupportsStreaming() bool {
	return true
}

func (cp *CSVProcessor) GetStreamingThreshold() int64 {
	return 10 << 20 // 10MB
}

// EstimateProcessingTime estimates processing time based on file size
func (cp *CSVProcessor) EstimateProcessingTime(ctx *ProcessingContext) (time.Duration, error) {
	// Estimate: 1MB per second for CSV processing
	seconds := ctx.FileRecord.Size / (1024 * 1024)
	if seconds < 1 {
		seconds = 1
	}
	return time.Duration(seconds) * time.Second, nil
}

// GetMemoryRequirement estimates memory needed for CSV processing
func (cp *CSVProcessor) GetMemoryRequirement(ctx *ProcessingContext) int64 {
	if ctx.FileRecord.Size > cp.GetStreamingThreshold() {
		// Streaming mode uses less memory
		return 100 << 20 // 100MB base memory
	}
	// Regular mode loads entire file into memory
	return ctx.FileRecord.Size + (50 << 20) // File size + 50MB overhead
}