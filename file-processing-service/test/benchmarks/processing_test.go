package benchmarks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pyairtable/go-services/file-processing-service/internal/models"
	"github.com/pyairtable/go-services/file-processing-service/internal/processors"
)

// Benchmark data generators
func generateCSVData(rows, cols int) []byte {
	var buf bytes.Buffer
	
	// Write headers
	for i := 0; i < cols; i++ {
		if i > 0 {
			buf.WriteString(",")
		}
		buf.WriteString(fmt.Sprintf("Column_%d", i+1))
	}
	buf.WriteString("\n")
	
	// Write data rows
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			if col > 0 {
				buf.WriteString(",")
			}
			buf.WriteString(fmt.Sprintf("Data_%d_%d", row+1, col+1))
		}
		buf.WriteString("\n")
	}
	
	return buf.Bytes()
}

func generateLargeCSVData(sizeInMB int) []byte {
	// Estimate rows needed for target size
	estimatedRowSize := 100 // Average bytes per row
	targetRows := (sizeInMB * 1024 * 1024) / estimatedRowSize
	
	return generateCSVData(targetRows, 10)
}

// CSV Processing Benchmarks
func BenchmarkCSVProcessingSmall(b *testing.B) {
	benchmarkCSVProcessing(b, 1000, 10) // 1K rows, 10 columns
}

func BenchmarkCSVProcessingMedium(b *testing.B) {
	benchmarkCSVProcessing(b, 10000, 20) // 10K rows, 20 columns
}

func BenchmarkCSVProcessingLarge(b *testing.B) {
	benchmarkCSVProcessing(b, 100000, 50) // 100K rows, 50 columns
}

func BenchmarkCSVProcessingHuge(b *testing.B) {
	benchmarkCSVProcessing(b, 1000000, 100) // 1M rows, 100 columns
}

func benchmarkCSVProcessing(b *testing.B, rows, cols int) {
	data := generateCSVData(rows, cols)
	processor := processors.NewCSVProcessor()
	
	config := &models.ProcessingConfig{
		CSV: models.CSVConfig{
			MaxRows:            rows + 1000,
			MaxColumns:         cols + 10,
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
		int64(len(data)),
		models.FileTypeCSV,
	)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(data)
		ctx := &processors.ProcessingContext{
			FileRecord: fileRecord,
			Reader:     reader,
			Config:     config,
			Context:    context.Background(),
		}
		
		result, err := processor.Process(ctx)
		if err != nil {
			b.Fatalf("Processing failed: %v", err)
		}
		
		// Verify some basic results
		if records, ok := result["records"].([]models.JSONMap); ok {
			if len(records) == 0 {
				b.Error("No records processed")
			}
		}
	}
	
	// Report memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	b.ReportMetric(float64(m.Alloc), "bytes/alloc")
	b.ReportMetric(float64(len(data)), "bytes/input")
}

// Memory-focused benchmarks
func BenchmarkCSVMemoryUsage1MB(b *testing.B) {
	benchmarkCSVMemoryUsage(b, 1)
}

func BenchmarkCSVMemoryUsage10MB(b *testing.B) {
	benchmarkCSVMemoryUsage(b, 10)
}

func BenchmarkCSVMemoryUsage100MB(b *testing.B) {
	benchmarkCSVMemoryUsage(b, 100)
}

func benchmarkCSVMemoryUsage(b *testing.B, sizeInMB int) {
	data := generateLargeCSVData(sizeInMB)
	processor := processors.NewCSVProcessor()
	
	config := &models.ProcessingConfig{
		CSV: models.CSVConfig{
			MaxRows:            10000000, // Large limit
			MaxColumns:         1000,
			MaxCellSize:        32768,
			DetectDelimiter:    true,
			SkipEmptyLines:     true,
			TrimSpaces:         true,
			StreamingThreshold: 5 << 20, // 5MB threshold for streaming
			BufferSize:         8192,
			SampleSize:         1024,
		},
	}
	
	fileRecord := models.NewFileRecord(
		"large_test.csv",
		"text/csv",
		int64(len(data)),
		models.FileTypeCSV,
	)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	// Measure peak memory usage
	var peakAlloc uint64
	
	for i := 0; i < b.N; i++ {
		runtime.GC()
		var startMem runtime.MemStats
		runtime.ReadMemStats(&startMem)
		
		reader := bytes.NewReader(data)
		ctx := &processors.ProcessingContext{
			FileRecord: fileRecord,
			Reader:     reader,
			Config:     config,
			Context:    context.Background(),
		}
		
		_, err := processor.Process(ctx)
		if err != nil {
			b.Fatalf("Processing failed: %v", err)
		}
		
		var endMem runtime.MemStats
		runtime.ReadMemStats(&endMem)
		
		allocDiff := endMem.Alloc - startMem.Alloc
		if allocDiff > peakAlloc {
			peakAlloc = allocDiff
		}
	}
	
	b.ReportMetric(float64(peakAlloc), "bytes/peak")
	b.ReportMetric(float64(len(data)), "bytes/input")
	b.ReportMetric(float64(peakAlloc)/float64(len(data)), "memory/ratio")
}

// Concurrent processing benchmarks
func BenchmarkCSVConcurrentProcessing(b *testing.B) {
	concurrencies := []int{1, 2, 4, 8, 16}
	
	for _, concurrency := range concurrencies {
		b.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(b *testing.B) {
			benchmarkCSVConcurrentProcessing(b, concurrency)
		})
	}
}

func benchmarkCSVConcurrentProcessing(b *testing.B, concurrency int) {
	data := generateCSVData(10000, 20) // 10K rows, 20 columns
	processor := processors.NewCSVProcessor()
	
	config := &models.ProcessingConfig{
		CSV: models.CSVConfig{
			MaxRows:            15000,
			MaxColumns:         30,
			MaxCellSize:        32768,
			DetectDelimiter:    true,
			SkipEmptyLines:     true,
			TrimSpaces:         true,
			StreamingThreshold: 10 << 20,
			BufferSize:         8192,
			SampleSize:         1024,
		},
	}
	
	b.SetParallelism(concurrency)
	b.ResetTimer()
	b.ReportAllocs()
	
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			fileRecord := models.NewFileRecord(
				"concurrent_test.csv",
				"text/csv",
				int64(len(data)),
				models.FileTypeCSV,
			)
			
			reader := bytes.NewReader(data)
			ctx := &processors.ProcessingContext{
				FileRecord: fileRecord,
				Reader:     reader,
				Config:     config,
				Context:    context.Background(),
			}
			
			_, err := processor.Process(ctx)
			if err != nil {
				b.Errorf("Processing failed: %v", err)
			}
		}
	})
}

// Streaming vs Regular processing comparison
func BenchmarkCSVStreamingVsRegular(b *testing.B) {
	sizes := []int{1, 5, 10, 50, 100} // MB
	
	for _, size := range sizes {
		data := generateLargeCSVData(size)
		
		b.Run(fmt.Sprintf("Regular_%dMB", size), func(b *testing.B) {
			benchmarkCSVRegular(b, data)
		})
		
		b.Run(fmt.Sprintf("Streaming_%dMB", size), func(b *testing.B) {
			benchmarkCSVStreaming(b, data)
		})
	}
}

func benchmarkCSVRegular(b *testing.B, data []byte) {
	processor := processors.NewCSVProcessor()
	
	config := &models.ProcessingConfig{
		CSV: models.CSVConfig{
			MaxRows:            10000000,
			MaxColumns:         1000,
			MaxCellSize:        32768,
			DetectDelimiter:    true,
			SkipEmptyLines:     true,
			TrimSpaces:         true,
			StreamingThreshold: int64(len(data) + 1), // Force regular processing
			BufferSize:         8192,
			SampleSize:         1024,
		},
	}
	
	fileRecord := models.NewFileRecord(
		"regular_test.csv",
		"text/csv",
		int64(len(data)),
		models.FileTypeCSV,
	)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(data)
		ctx := &processors.ProcessingContext{
			FileRecord: fileRecord,
			Reader:     reader,
			Config:     config,
			Context:    context.Background(),
		}
		
		_, err := processor.Process(ctx)
		if err != nil {
			b.Fatalf("Regular processing failed: %v", err)
		}
	}
}

func benchmarkCSVStreaming(b *testing.B, data []byte) {
	processor := processors.NewCSVProcessor()
	
	config := &models.ProcessingConfig{
		CSV: models.CSVConfig{
			MaxRows:            10000000,
			MaxColumns:         1000,
			MaxCellSize:        32768,
			DetectDelimiter:    true,
			SkipEmptyLines:     true,
			TrimSpaces:         true,
			StreamingThreshold: 1024, // Force streaming processing
			BufferSize:         8192,
			SampleSize:         1024,
		},
	}
	
	fileRecord := models.NewFileRecord(
		"streaming_test.csv",
		"text/csv",
		int64(len(data)),
		models.FileTypeCSV,
	)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(data)
		ctx := &processors.ProcessingContext{
			FileRecord: fileRecord,
			Reader:     reader,
			Config:     config,
			Context:    context.Background(),
		}
		
		_, err := processor.ProcessStream(ctx, nil)
		if err != nil {
			b.Fatalf("Streaming processing failed: %v", err)
		}
	}
}

// Delimiter detection benchmarks
func BenchmarkCSVDelimiterDetection(b *testing.B) {
	delimiters := []rune{',', ';', '\t', '|'}
	
	for _, delimiter := range delimiters {
		data := generateCSVDataWithDelimiter(1000, 10, delimiter)
		
		b.Run(fmt.Sprintf("Delimiter_%c", delimiter), func(b *testing.B) {
			processor := processors.NewCSVProcessor()
			
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				reader := bytes.NewReader(data)
				config := &models.ProcessingConfig{
					CSV: models.CSVConfig{
						MaxRows:         2000,
						MaxColumns:      20,
						DetectDelimiter: true,
						SampleSize:      1024,
					},
				}
				
				fileRecord := models.NewFileRecord(
					"delimiter_test.csv",
					"text/csv",
					int64(len(data)),
					models.FileTypeCSV,
				)
				
				ctx := &processors.ProcessingContext{
					FileRecord: fileRecord,
					Reader:     reader,
					Config:     config,
					Context:    context.Background(),
				}
				
				_, err := processor.Process(ctx)
				if err != nil {
					b.Fatalf("Processing failed: %v", err)
				}
			}
		})
	}
}

func generateCSVDataWithDelimiter(rows, cols int, delimiter rune) []byte {
	var buf bytes.Buffer
	
	// Write headers
	for i := 0; i < cols; i++ {
		if i > 0 {
			buf.WriteRune(delimiter)
		}
		buf.WriteString(fmt.Sprintf("Column_%d", i+1))
	}
	buf.WriteString("\n")
	
	// Write data rows
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			if col > 0 {
				buf.WriteRune(delimiter)
			}
			buf.WriteString(fmt.Sprintf("Data_%d_%d", row+1, col+1))
		}
		buf.WriteString("\n")
	}
	
	return buf.Bytes()
}

// Edge case benchmarks
func BenchmarkCSVEdgeCases(b *testing.B) {
	testCases := map[string][]byte{
		"EmptyFile":      []byte(""),
		"HeaderOnly":     []byte("Col1,Col2,Col3\n"),
		"SingleColumn":   generateCSVData(1000, 1),
		"ManyColumns":    generateCSVData(100, 1000),
		"QuotedFields":   generateQuotedCSV(1000, 10),
		"MixedDelimiter": generateMixedDelimiterCSV(1000, 10),
	}
	
	for name, data := range testCases {
		b.Run(name, func(b *testing.B) {
			if len(data) == 0 {
				b.Skip("Skipping empty file test")
			}
			
			processor := processors.NewCSVProcessor()
			config := &models.ProcessingConfig{
				CSV: models.CSVConfig{
					MaxRows:         10000,
					MaxColumns:      1500,
					MaxCellSize:     32768,
					DetectDelimiter: true,
					SkipEmptyLines:  true,
					TrimSpaces:      true,
					BufferSize:      8192,
					SampleSize:      1024,
				},
			}
			
			fileRecord := models.NewFileRecord(
				fmt.Sprintf("%s_test.csv", name),
				"text/csv",
				int64(len(data)),
				models.FileTypeCSV,
			)
			
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				reader := bytes.NewReader(data)
				ctx := &processors.ProcessingContext{
					FileRecord: fileRecord,
					Reader:     reader,
					Config:     config,
					Context:    context.Background(),
				}
				
				_, err := processor.Process(ctx)
				if err != nil && !strings.Contains(err.Error(), "empty") {
					b.Fatalf("Processing failed: %v", err)
				}
			}
		})
	}
}

func generateQuotedCSV(rows, cols int) []byte {
	var buf bytes.Buffer
	
	// Write headers
	for i := 0; i < cols; i++ {
		if i > 0 {
			buf.WriteString(",")
		}
		buf.WriteString(fmt.Sprintf("\"Column %d\"", i+1))
	}
	buf.WriteString("\n")
	
	// Write data rows with quoted fields
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			if col > 0 {
				buf.WriteString(",")
			}
			buf.WriteString(fmt.Sprintf("\"Data %d, %d\"", row+1, col+1))
		}
		buf.WriteString("\n")
	}
	
	return buf.Bytes()
}

func generateMixedDelimiterCSV(rows, cols int) []byte {
	var buf bytes.Buffer
	
	// Write headers with commas
	for i := 0; i < cols; i++ {
		if i > 0 {
			buf.WriteString(",")
		}
		buf.WriteString(fmt.Sprintf("Column_%d", i+1))
	}
	buf.WriteString("\n")
	
	// Write some rows with semicolons (to test delimiter consistency)
	for row := 0; row < rows/2; row++ {
		for col := 0; col < cols; col++ {
			if col > 0 {
				buf.WriteString(",")
			}
			buf.WriteString(fmt.Sprintf("Data_%d_%d", row+1, col+1))
		}
		buf.WriteString("\n")
	}
	
	// Write remaining rows with semicolons (mixed content)
	for row := rows / 2; row < rows; row++ {
		for col := 0; col < cols; col++ {
			if col > 0 {
				buf.WriteString(",")
			}
			buf.WriteString(fmt.Sprintf("Data;%d;%d", row+1, col+1))
		}
		buf.WriteString("\n")
	}
	
	return buf.Bytes()
}

// Performance comparison with the Python version
func BenchmarkCSVProcessingComparison(b *testing.B) {
	// Generate test data similar to what the Python version would handle
	data := generateCSVData(50000, 25) // 50K rows, 25 columns
	
	processor := processors.NewCSVProcessor()
	config := &models.ProcessingConfig{
		CSV: models.CSVConfig{
			MaxRows:            100000,
			MaxColumns:         50,
			MaxCellSize:        32768,
			DetectDelimiter:    true,
			SkipEmptyLines:     true,
			TrimSpaces:         true,
			StreamingThreshold: 10 << 20,
			BufferSize:         8192,
			SampleSize:         1024,
		},
	}
	
	fileRecord := models.NewFileRecord(
		"comparison_test.csv",
		"text/csv",
		int64(len(data)),
		models.FileTypeCSV,
	)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	start := time.Now()
	
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(data)
		ctx := &processors.ProcessingContext{
			FileRecord: fileRecord,
			Reader:     reader,
			Config:     config,
			Context:    context.Background(),
		}
		
		result, err := processor.Process(ctx)
		if err != nil {
			b.Fatalf("Processing failed: %v", err)
		}
		
		// Verify results
		if records, ok := result["records"].([]models.JSONMap); ok {
			if len(records) == 0 {
				b.Error("No records processed")
			}
		}
	}
	
	elapsed := time.Since(start)
	
	// Calculate performance metrics
	totalBytes := int64(len(data)) * int64(b.N)
	bytesPerSecond := float64(totalBytes) / elapsed.Seconds()
	
	b.ReportMetric(bytesPerSecond/1024/1024, "MB/s")
	b.ReportMetric(float64(len(data))/1024/1024, "MB/file")
}