package models

import (
	"time"
)

// Config represents the application configuration
type Config struct {
	Server     ServerConfig     `yaml:"server" json:"server"`
	Database   DatabaseConfig   `yaml:"database" json:"database"`
	Redis      RedisConfig      `yaml:"redis" json:"redis"`
	Storage    StorageConfig    `yaml:"storage" json:"storage"`
	Processing ProcessingConfig `yaml:"processing" json:"processing"`
	Security   SecurityConfig   `yaml:"security" json:"security"`
	Monitoring MonitoringConfig `yaml:"monitoring" json:"monitoring"`
	WebSocket  WebSocketConfig  `yaml:"websocket" json:"websocket"`
}

// ServerConfig contains HTTP server configuration
type ServerConfig struct {
	Host              string        `yaml:"host" json:"host"`
	Port              int           `yaml:"port" json:"port"`
	ReadTimeout       time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout" json:"write_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
	MaxHeaderBytes    int           `yaml:"max_header_bytes" json:"max_header_bytes"`
	ShutdownTimeout   time.Duration `yaml:"shutdown_timeout" json:"shutdown_timeout"`
	EnableProfiling   bool          `yaml:"enable_profiling" json:"enable_profiling"`
	TrustedProxies    []string      `yaml:"trusted_proxies" json:"trusted_proxies"`
	MaxUploadSize     int64         `yaml:"max_upload_size" json:"max_upload_size"`
	EnableCompression bool          `yaml:"enable_compression" json:"enable_compression"`
}

// DatabaseConfig contains database configuration
type DatabaseConfig struct {
	Driver          string        `yaml:"driver" json:"driver"`
	Host            string        `yaml:"host" json:"host"`
	Port            int           `yaml:"port" json:"port"`
	Name            string        `yaml:"name" json:"name"`
	User            string        `yaml:"user" json:"user"`
	Password        string        `yaml:"password" json:"password"`
	SSLMode         string        `yaml:"ssl_mode" json:"ssl_mode"`
	MaxOpenConns    int           `yaml:"max_open_conns" json:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns" json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time" json:"conn_max_idle_time"`
	MigrationsPath  string        `yaml:"migrations_path" json:"migrations_path"`
}

// RedisConfig contains Redis configuration
type RedisConfig struct {
	Host         string        `yaml:"host" json:"host"`
	Port         int           `yaml:"port" json:"port"`
	Password     string        `yaml:"password" json:"password"`
	DB           int           `yaml:"db" json:"db"`
	PoolSize     int           `yaml:"pool_size" json:"pool_size"`
	DialTimeout  time.Duration `yaml:"dial_timeout" json:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout" json:"write_timeout"`
	MaxRetries   int           `yaml:"max_retries" json:"max_retries"`
	CacheTTL     time.Duration `yaml:"cache_ttl" json:"cache_ttl"`
	KeyPrefix    string        `yaml:"key_prefix" json:"key_prefix"`
}

// StorageConfig contains storage backend configuration
type StorageConfig struct {
	Provider        string         `yaml:"provider" json:"provider"` // s3, minio, local
	S3Config        *S3Config      `yaml:"s3,omitempty" json:"s3,omitempty"`
	MinIOConfig     *MinIOConfig   `yaml:"minio,omitempty" json:"minio,omitempty"`
	LocalConfig     *LocalConfig   `yaml:"local,omitempty" json:"local,omitempty"`
	UploadTimeout   time.Duration  `yaml:"upload_timeout" json:"upload_timeout"`
	DownloadTimeout time.Duration  `yaml:"download_timeout" json:"download_timeout"`
	MaxFileSize     int64          `yaml:"max_file_size" json:"max_file_size"`
	AllowedTypes    []string       `yaml:"allowed_types" json:"allowed_types"`
	CDN             *CDNConfig     `yaml:"cdn,omitempty" json:"cdn,omitempty"`
}

// S3Config contains AWS S3 configuration
type S3Config struct {
	Region          string `yaml:"region" json:"region"`
	Bucket          string `yaml:"bucket" json:"bucket"`
	AccessKeyID     string `yaml:"access_key_id" json:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key" json:"secret_access_key"`
	SessionToken    string `yaml:"session_token,omitempty" json:"session_token,omitempty"`
	Endpoint        string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	DisableSSL      bool   `yaml:"disable_ssl" json:"disable_ssl"`
	ForcePathStyle  bool   `yaml:"force_path_style" json:"force_path_style"`
}

// MinIOConfig contains MinIO configuration
type MinIOConfig struct {
	Endpoint        string `yaml:"endpoint" json:"endpoint"`
	AccessKeyID     string `yaml:"access_key_id" json:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key" json:"secret_access_key"`
	UseSSL          bool   `yaml:"use_ssl" json:"use_ssl"`
	Bucket          string `yaml:"bucket" json:"bucket"`
}

// LocalConfig contains local storage configuration
type LocalConfig struct {
	BasePath      string `yaml:"base_path" json:"base_path"`
	ServeFiles    bool   `yaml:"serve_files" json:"serve_files"`
	URLPrefix     string `yaml:"url_prefix" json:"url_prefix"`
	MaxDiskUsage  int64  `yaml:"max_disk_usage" json:"max_disk_usage"`
}

// CDNConfig contains CDN configuration
type CDNConfig struct {
	Enabled     bool   `yaml:"enabled" json:"enabled"`
	BaseURL     string `yaml:"base_url" json:"base_url"`
	CacheTTL    int    `yaml:"cache_ttl" json:"cache_ttl"`
	PurgeSecret string `yaml:"purge_secret" json:"purge_secret"`
}

// ProcessingConfig contains file processing configuration
type ProcessingConfig struct {
	WorkerCount          int           `yaml:"worker_count" json:"worker_count"`
	QueueSize            int           `yaml:"queue_size" json:"queue_size"`
	MaxConcurrentUploads int           `yaml:"max_concurrent_uploads" json:"max_concurrent_uploads"`
	ProcessingTimeout    time.Duration `yaml:"processing_timeout" json:"processing_timeout"`
	MemoryLimit          int64         `yaml:"memory_limit" json:"memory_limit"`
	TempDir              string        `yaml:"temp_dir" json:"temp_dir"`
	CleanupInterval      time.Duration `yaml:"cleanup_interval" json:"cleanup_interval"`
	TempFileRetention    time.Duration `yaml:"temp_file_retention" json:"temp_file_retention"`
	
	// File type specific configurations
	CSV   CSVConfig   `yaml:"csv" json:"csv"`
	PDF   PDFConfig   `yaml:"pdf" json:"pdf"`
	Excel ExcelConfig `yaml:"excel" json:"excel"`
	Image ImageConfig `yaml:"image" json:"image"`
}

// CSVConfig contains CSV processing configuration
type CSVConfig struct {
	MaxRows              int     `yaml:"max_rows" json:"max_rows"`
	MaxColumns           int     `yaml:"max_columns" json:"max_columns"`
	MaxCellSize          int     `yaml:"max_cell_size" json:"max_cell_size"`
	DetectDelimiter      bool    `yaml:"detect_delimiter" json:"detect_delimiter"`
	SkipEmptyLines       bool    `yaml:"skip_empty_lines" json:"skip_empty_lines"`
	TrimSpaces           bool    `yaml:"trim_spaces" json:"trim_spaces"`
	StreamingThreshold   int64   `yaml:"streaming_threshold" json:"streaming_threshold"`
	BufferSize           int     `yaml:"buffer_size" json:"buffer_size"`
	SampleSize           int     `yaml:"sample_size" json:"sample_size"`
}

// PDFConfig contains PDF processing configuration
type PDFConfig struct {
	MaxPages             int     `yaml:"max_pages" json:"max_pages"`
	ExtractImages        bool    `yaml:"extract_images" json:"extract_images"`
	ExtractText          bool    `yaml:"extract_text" json:"extract_text"`
	ExtractMetadata      bool    `yaml:"extract_metadata" json:"extract_metadata"`
	GenerateThumbnails   bool    `yaml:"generate_thumbnails" json:"generate_thumbnails"`
	ThumbnailQuality     int     `yaml:"thumbnail_quality" json:"thumbnail_quality"`
	ThumbnailWidth       int     `yaml:"thumbnail_width" json:"thumbnail_width"`
	ThumbnailHeight      int     `yaml:"thumbnail_height" json:"thumbnail_height"`
	OCREnabled           bool    `yaml:"ocr_enabled" json:"ocr_enabled"`
	OCRLanguage          string  `yaml:"ocr_language" json:"ocr_language"`
}

// ExcelConfig contains Excel processing configuration
type ExcelConfig struct {
	MaxSheets            int     `yaml:"max_sheets" json:"max_sheets"`
	MaxRows              int     `yaml:"max_rows" json:"max_rows"`
	MaxColumns           int     `yaml:"max_columns" json:"max_columns"`
	ReadFormulas         bool    `yaml:"read_formulas" json:"read_formulas"`
	ReadComments         bool    `yaml:"read_comments" json:"read_comments"`
	StreamingThreshold   int64   `yaml:"streaming_threshold" json:"streaming_threshold"`
	BufferSize           int     `yaml:"buffer_size" json:"buffer_size"`
}

// ImageConfig contains image processing configuration
type ImageConfig struct {
	MaxWidth             int     `yaml:"max_width" json:"max_width"`
	MaxHeight            int     `yaml:"max_height" json:"max_height"`
	ThumbnailWidth       int     `yaml:"thumbnail_width" json:"thumbnail_width"`
	ThumbnailHeight      int     `yaml:"thumbnail_height" json:"thumbnail_height"`
	JpegQuality          int     `yaml:"jpeg_quality" json:"jpeg_quality"`
	WebpQuality          int     `yaml:"webp_quality" json:"webp_quality"`
	EnableWebp           bool    `yaml:"enable_webp" json:"enable_webp"`
	ExtractEXIF          bool    `yaml:"extract_exif" json:"extract_exif"`
	AutoOrient           bool    `yaml:"auto_orient" json:"auto_orient"`
	StripMetadata        bool    `yaml:"strip_metadata" json:"strip_metadata"`
}

// SecurityConfig contains security configuration
type SecurityConfig struct {
	EnableVirusScanning  bool          `yaml:"enable_virus_scanning" json:"enable_virus_scanning"`
	ClamAVHost          string        `yaml:"clamav_host" json:"clamav_host"`
	ClamAVPort          int           `yaml:"clamav_port" json:"clamav_port"`
	ClamAVTimeout       time.Duration `yaml:"clamav_timeout" json:"clamav_timeout"`
	EnableFileTypeCheck bool          `yaml:"enable_file_type_check" json:"enable_file_type_check"`
	BlockExecutables    bool          `yaml:"block_executables" json:"block_executables"`
	MaxFilenameLenth    int           `yaml:"max_filename_length" json:"max_filename_length"`
	AllowedExtensions   []string      `yaml:"allowed_extensions" json:"allowed_extensions"`
	BlockedExtensions   []string      `yaml:"blocked_extensions" json:"blocked_extensions"`
	ScanResultCacheTTL  time.Duration `yaml:"scan_result_cache_ttl" json:"scan_result_cache_ttl"`
}

// MonitoringConfig contains monitoring and observability configuration
type MonitoringConfig struct {
	Enabled         bool          `yaml:"enabled" json:"enabled"`
	MetricsPort     int           `yaml:"metrics_port" json:"metrics_port"`
	MetricsPath     string        `yaml:"metrics_path" json:"metrics_path"`
	HealthPath      string        `yaml:"health_path" json:"health_path"`
	PprofEnabled    bool          `yaml:"pprof_enabled" json:"pprof_enabled"`
	LogLevel        string        `yaml:"log_level" json:"log_level"`
	LogFormat       string        `yaml:"log_format" json:"log_format"`
	TracingEnabled  bool          `yaml:"tracing_enabled" json:"tracing_enabled"`
	TracingEndpoint string        `yaml:"tracing_endpoint" json:"tracing_endpoint"`
	SampleRate      float64       `yaml:"sample_rate" json:"sample_rate"`
	RequestTimeout  time.Duration `yaml:"request_timeout" json:"request_timeout"`
}

// WebSocketConfig contains WebSocket configuration
type WebSocketConfig struct {
	Enabled            bool          `yaml:"enabled" json:"enabled"`
	Path               string        `yaml:"path" json:"path"`
	ReadBufferSize     int           `yaml:"read_buffer_size" json:"read_buffer_size"`
	WriteBufferSize    int           `yaml:"write_buffer_size" json:"write_buffer_size"`
	HandshakeTimeout   time.Duration `yaml:"handshake_timeout" json:"handshake_timeout"`
	ReadDeadline       time.Duration `yaml:"read_deadline" json:"read_deadline"`
	WriteDeadline      time.Duration `yaml:"write_deadline" json:"write_deadline"`
	PingPeriod         time.Duration `yaml:"ping_period" json:"ping_period"`
	MaxMessageSize     int64         `yaml:"max_message_size" json:"max_message_size"`
	CheckOrigin        bool          `yaml:"check_origin" json:"check_origin"`
	EnableCompression  bool          `yaml:"enable_compression" json:"enable_compression"`
	MaxConnections     int           `yaml:"max_connections" json:"max_connections"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:              "0.0.0.0",
			Port:              8080,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    1 << 20, // 1MB
			ShutdownTimeout:   30 * time.Second,
			EnableProfiling:   false,
			MaxUploadSize:     100 << 20, // 100MB
			EnableCompression: true,
		},
		Database: DatabaseConfig{
			Driver:          "postgres",
			Host:            "localhost",
			Port:            5432,
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
		},
		Redis: RedisConfig{
			Host:         "localhost",
			Port:         6379,
			DB:           0,
			PoolSize:     10,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			MaxRetries:   3,
			CacheTTL:     1 * time.Hour,
			KeyPrefix:    "file-processing:",
		},
		Storage: StorageConfig{
			Provider:        "local",
			UploadTimeout:   10 * time.Minute,
			DownloadTimeout: 5 * time.Minute,
			MaxFileSize:     100 << 20, // 100MB
			AllowedTypes:    []string{"csv", "pdf", "docx", "xlsx", "jpeg", "png", "webp"},
		},
		Processing: ProcessingConfig{
			WorkerCount:          4,
			QueueSize:            1000,
			MaxConcurrentUploads: 10,
			ProcessingTimeout:    5 * time.Minute,
			MemoryLimit:          512 << 20, // 512MB
			TempDir:              "/tmp/file-processing",
			CleanupInterval:      1 * time.Hour,
			TempFileRetention:    24 * time.Hour,
			CSV: CSVConfig{
				MaxRows:            100000,
				MaxColumns:         1000,
				MaxCellSize:        32768,
				DetectDelimiter:    true,
				SkipEmptyLines:     true,
				TrimSpaces:         true,
				StreamingThreshold: 10 << 20, // 10MB
				BufferSize:         8192,
				SampleSize:         1024,
			},
			PDF: PDFConfig{
				MaxPages:           1000,
				ExtractImages:      true,
				ExtractText:        true,
				ExtractMetadata:    true,
				GenerateThumbnails: true,
				ThumbnailQuality:   80,
				ThumbnailWidth:     200,
				ThumbnailHeight:    200,
				OCREnabled:         false,
				OCRLanguage:        "eng",
			},
			Excel: ExcelConfig{
				MaxSheets:          50,
				MaxRows:            100000,
				MaxColumns:         1000,
				ReadFormulas:       false,
				ReadComments:       false,
				StreamingThreshold: 10 << 20, // 10MB
				BufferSize:         8192,
			},
			Image: ImageConfig{
				MaxWidth:        4096,
				MaxHeight:       4096,
				ThumbnailWidth:  200,
				ThumbnailHeight: 200,
				JpegQuality:     90,
				WebpQuality:     80,
				EnableWebp:      true,
				ExtractEXIF:     true,
				AutoOrient:      true,
				StripMetadata:   false,
			},
		},
		Security: SecurityConfig{
			EnableVirusScanning:  false,
			ClamAVHost:          "localhost",
			ClamAVPort:          3310,
			ClamAVTimeout:       30 * time.Second,
			EnableFileTypeCheck: true,
			BlockExecutables:    true,
			MaxFilenameLenth:    255,
			ScanResultCacheTTL:  1 * time.Hour,
		},
		Monitoring: MonitoringConfig{
			Enabled:        true,
			MetricsPort:    9090,
			MetricsPath:    "/metrics",
			HealthPath:     "/health",
			PprofEnabled:   false,
			LogLevel:       "info",
			LogFormat:      "json",
			TracingEnabled: false,
			SampleRate:     0.1,
			RequestTimeout: 30 * time.Second,
		},
		WebSocket: WebSocketConfig{
			Enabled:           true,
			Path:              "/ws",
			ReadBufferSize:    1024,
			WriteBufferSize:   1024,
			HandshakeTimeout:  10 * time.Second,
			ReadDeadline:      60 * time.Second,
			WriteDeadline:     10 * time.Second,
			PingPeriod:        54 * time.Second,
			MaxMessageSize:    512,
			CheckOrigin:       false,
			EnableCompression: true,
			MaxConnections:    1000,
		},
	}
}