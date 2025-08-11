package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pyairtable/go-services/file-processing-service/internal/models"
	"gopkg.in/yaml.v3"
)

// Load loads configuration from file and environment variables
func Load(configPath string) (*models.Config, error) {
	config := models.DefaultConfig()

	// Load from file if provided
	if configPath != "" && fileExists(configPath) {
		if err := loadFromFile(config, configPath); err != nil {
			return nil, fmt.Errorf("failed to load config from file: %w", err)
		}
	}

	// Override with environment variables
	if err := loadFromEnv(config); err != nil {
		return nil, fmt.Errorf("failed to load config from environment: %w", err)
	}

	// Validate configuration
	if err := validate(config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// loadFromFile loads configuration from YAML file
func loadFromFile(config *models.Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	return nil
}

// loadFromEnv loads configuration from environment variables
func loadFromEnv(config *models.Config) error {
	// Server configuration
	if host := os.Getenv("SERVER_HOST"); host != "" {
		config.Server.Host = host
	}
	if port := os.Getenv("SERVER_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Server.Port = p
		}
	}
	if timeout := os.Getenv("SERVER_READ_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			config.Server.ReadTimeout = d
		}
	}
	if timeout := os.Getenv("SERVER_WRITE_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			config.Server.WriteTimeout = d
		}
	}
	if size := os.Getenv("SERVER_MAX_UPLOAD_SIZE"); size != "" {
		if s, err := strconv.ParseInt(size, 10, 64); err == nil {
			config.Server.MaxUploadSize = s
		}
	}

	// Database configuration
	if host := os.Getenv("DB_HOST"); host != "" {
		config.Database.Host = host
	}
	if port := os.Getenv("DB_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Database.Port = p
		}
	}
	if name := os.Getenv("DB_NAME"); name != "" {
		config.Database.Name = name
	}
	if user := os.Getenv("DB_USER"); user != "" {
		config.Database.User = user
	}
	if password := os.Getenv("DB_PASSWORD"); password != "" {
		config.Database.Password = password
	}
	if sslMode := os.Getenv("DB_SSL_MODE"); sslMode != "" {
		config.Database.SSLMode = sslMode
	}
	if maxConns := os.Getenv("DB_MAX_OPEN_CONNS"); maxConns != "" {
		if c, err := strconv.Atoi(maxConns); err == nil {
			config.Database.MaxOpenConns = c
		}
	}

	// Redis configuration
	if host := os.Getenv("REDIS_HOST"); host != "" {
		config.Redis.Host = host
	}
	if port := os.Getenv("REDIS_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Redis.Port = p
		}
	}
	if password := os.Getenv("REDIS_PASSWORD"); password != "" {
		config.Redis.Password = password
	}
	if db := os.Getenv("REDIS_DB"); db != "" {
		if d, err := strconv.Atoi(db); err == nil {
			config.Redis.DB = d
		}
	}
	if ttl := os.Getenv("REDIS_CACHE_TTL"); ttl != "" {
		if d, err := time.ParseDuration(ttl); err == nil {
			config.Redis.CacheTTL = d
		}
	}

	// Storage configuration
	if provider := os.Getenv("STORAGE_PROVIDER"); provider != "" {
		config.Storage.Provider = provider
	}
	if maxSize := os.Getenv("STORAGE_MAX_FILE_SIZE"); maxSize != "" {
		if s, err := strconv.ParseInt(maxSize, 10, 64); err == nil {
			config.Storage.MaxFileSize = s
		}
	}

	// S3 configuration
	if config.Storage.Provider == "s3" {
		if config.Storage.S3Config == nil {
			config.Storage.S3Config = &models.S3Config{}
		}
		if region := os.Getenv("AWS_REGION"); region != "" {
			config.Storage.S3Config.Region = region
		}
		if bucket := os.Getenv("S3_BUCKET"); bucket != "" {
			config.Storage.S3Config.Bucket = bucket
		}
		if accessKey := os.Getenv("AWS_ACCESS_KEY_ID"); accessKey != "" {
			config.Storage.S3Config.AccessKeyID = accessKey
		}
		if secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY"); secretKey != "" {
			config.Storage.S3Config.SecretAccessKey = secretKey
		}
		if sessionToken := os.Getenv("AWS_SESSION_TOKEN"); sessionToken != "" {
			config.Storage.S3Config.SessionToken = sessionToken
		}
		if endpoint := os.Getenv("S3_ENDPOINT"); endpoint != "" {
			config.Storage.S3Config.Endpoint = endpoint
		}
		if disableSSL := os.Getenv("S3_DISABLE_SSL"); disableSSL == "true" {
			config.Storage.S3Config.DisableSSL = true
		}
		if forcePathStyle := os.Getenv("S3_FORCE_PATH_STYLE"); forcePathStyle == "true" {
			config.Storage.S3Config.ForcePathStyle = true
		}
	}

	// MinIO configuration
	if config.Storage.Provider == "minio" {
		if config.Storage.MinIOConfig == nil {
			config.Storage.MinIOConfig = &models.MinIOConfig{}
		}
		if endpoint := os.Getenv("MINIO_ENDPOINT"); endpoint != "" {
			config.Storage.MinIOConfig.Endpoint = endpoint
		}
		if accessKey := os.Getenv("MINIO_ACCESS_KEY"); accessKey != "" {
			config.Storage.MinIOConfig.AccessKeyID = accessKey
		}
		if secretKey := os.Getenv("MINIO_SECRET_KEY"); secretKey != "" {
			config.Storage.MinIOConfig.SecretAccessKey = secretKey
		}
		if bucket := os.Getenv("MINIO_BUCKET"); bucket != "" {
			config.Storage.MinIOConfig.Bucket = bucket
		}
		if useSSL := os.Getenv("MINIO_USE_SSL"); useSSL == "true" {
			config.Storage.MinIOConfig.UseSSL = true
		}
	}

	// Local storage configuration
	if config.Storage.Provider == "local" {
		if config.Storage.LocalConfig == nil {
			config.Storage.LocalConfig = &models.LocalConfig{}
		}
		if basePath := os.Getenv("LOCAL_STORAGE_PATH"); basePath != "" {
			config.Storage.LocalConfig.BasePath = basePath
		}
		if serveFiles := os.Getenv("LOCAL_SERVE_FILES"); serveFiles == "true" {
			config.Storage.LocalConfig.ServeFiles = true
		}
		if urlPrefix := os.Getenv("LOCAL_URL_PREFIX"); urlPrefix != "" {
			config.Storage.LocalConfig.URLPrefix = urlPrefix
		}
	}

	// Processing configuration
	if workers := os.Getenv("PROCESSING_WORKER_COUNT"); workers != "" {
		if w, err := strconv.Atoi(workers); err == nil {
			config.Processing.WorkerCount = w
		}
	}
	if queueSize := os.Getenv("PROCESSING_QUEUE_SIZE"); queueSize != "" {
		if q, err := strconv.Atoi(queueSize); err == nil {
			config.Processing.QueueSize = q
		}
	}
	if timeout := os.Getenv("PROCESSING_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			config.Processing.ProcessingTimeout = d
		}
	}
	if memLimit := os.Getenv("PROCESSING_MEMORY_LIMIT"); memLimit != "" {
		if m, err := strconv.ParseInt(memLimit, 10, 64); err == nil {
			config.Processing.MemoryLimit = m
		}
	}
	if tempDir := os.Getenv("PROCESSING_TEMP_DIR"); tempDir != "" {
		config.Processing.TempDir = tempDir
	}

	// Security configuration
	if virusScan := os.Getenv("SECURITY_VIRUS_SCANNING"); virusScan == "true" {
		config.Security.EnableVirusScanning = true
	}
	if clamHost := os.Getenv("CLAMAV_HOST"); clamHost != "" {
		config.Security.ClamAVHost = clamHost
	}
	if clamPort := os.Getenv("CLAMAV_PORT"); clamPort != "" {
		if p, err := strconv.Atoi(clamPort); err == nil {
			config.Security.ClamAVPort = p
		}
	}

	// Monitoring configuration
	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		config.Monitoring.LogLevel = logLevel
	}
	if logFormat := os.Getenv("LOG_FORMAT"); logFormat != "" {
		config.Monitoring.LogFormat = logFormat
	}
	if metricsPort := os.Getenv("METRICS_PORT"); metricsPort != "" {
		if p, err := strconv.Atoi(metricsPort); err == nil {
			config.Monitoring.MetricsPort = p
		}
	}
	if tracing := os.Getenv("TRACING_ENABLED"); tracing == "true" {
		config.Monitoring.TracingEnabled = true
	}
	if endpoint := os.Getenv("TRACING_ENDPOINT"); endpoint != "" {
		config.Monitoring.TracingEndpoint = endpoint
	}

	// WebSocket configuration
	if wsEnabled := os.Getenv("WEBSOCKET_ENABLED"); wsEnabled == "false" {
		config.WebSocket.Enabled = false
	}
	if wsPath := os.Getenv("WEBSOCKET_PATH"); wsPath != "" {
		config.WebSocket.Path = wsPath
	}
	if maxConns := os.Getenv("WEBSOCKET_MAX_CONNECTIONS"); maxConns != "" {
		if c, err := strconv.Atoi(maxConns); err == nil {
			config.WebSocket.MaxConnections = c
		}
	}

	return nil
}

// validate validates the configuration
func validate(config *models.Config) error {
	// Validate server configuration
	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", config.Server.Port)
	}
	if config.Server.MaxUploadSize <= 0 {
		return fmt.Errorf("max upload size must be positive")
	}

	// Validate database configuration
	if config.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if config.Database.Name == "" {
		return fmt.Errorf("database name is required")
	}
	if config.Database.User == "" {
		return fmt.Errorf("database user is required")
	}

	// Validate Redis configuration
	if config.Redis.Host == "" {
		return fmt.Errorf("redis host is required")
	}
	if config.Redis.Port <= 0 || config.Redis.Port > 65535 {
		return fmt.Errorf("invalid redis port: %d", config.Redis.Port)
	}

	// Validate storage configuration
	if config.Storage.Provider == "" {
		return fmt.Errorf("storage provider is required")
	}
	
	validProviders := map[string]bool{
		"s3": true, "minio": true, "local": true,
	}
	if !validProviders[config.Storage.Provider] {
		return fmt.Errorf("invalid storage provider: %s", config.Storage.Provider)
	}

	// Validate provider-specific configuration
	switch config.Storage.Provider {
	case "s3":
		if config.Storage.S3Config == nil {
			return fmt.Errorf("S3 configuration is required when using S3 provider")
		}
		if config.Storage.S3Config.Region == "" {
			return fmt.Errorf("S3 region is required")
		}
		if config.Storage.S3Config.Bucket == "" {
			return fmt.Errorf("S3 bucket is required")
		}
	case "minio":
		if config.Storage.MinIOConfig == nil {
			return fmt.Errorf("MinIO configuration is required when using MinIO provider")
		}
		if config.Storage.MinIOConfig.Endpoint == "" {
			return fmt.Errorf("MinIO endpoint is required")
		}
		if config.Storage.MinIOConfig.Bucket == "" {
			return fmt.Errorf("MinIO bucket is required")
		}
	case "local":
		if config.Storage.LocalConfig == nil {
			return fmt.Errorf("local storage configuration is required when using local provider")
		}
		if config.Storage.LocalConfig.BasePath == "" {
			return fmt.Errorf("local storage base path is required")
		}
		// Ensure the directory exists
		if err := os.MkdirAll(config.Storage.LocalConfig.BasePath, 0755); err != nil {
			return fmt.Errorf("failed to create local storage directory: %w", err)
		}
	}

	// Validate processing configuration
	if config.Processing.WorkerCount <= 0 {
		return fmt.Errorf("worker count must be positive")
	}
	if config.Processing.QueueSize <= 0 {
		return fmt.Errorf("queue size must be positive")
	}
	if config.Processing.MemoryLimit <= 0 {
		return fmt.Errorf("memory limit must be positive")
	}
	if config.Processing.TempDir != "" {
		// Ensure the directory exists
		if err := os.MkdirAll(config.Processing.TempDir, 0755); err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
	}

	// Validate monitoring configuration
	validLogLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true, "fatal": true,
	}
	if !validLogLevels[strings.ToLower(config.Monitoring.LogLevel)] {
		return fmt.Errorf("invalid log level: %s", config.Monitoring.LogLevel)
	}
	
	validLogFormats := map[string]bool{
		"json": true, "text": true,
	}
	if !validLogFormats[strings.ToLower(config.Monitoring.LogFormat)] {
		return fmt.Errorf("invalid log format: %s", config.Monitoring.LogFormat)
	}

	if config.Monitoring.MetricsPort <= 0 || config.Monitoring.MetricsPort > 65535 {
		return fmt.Errorf("invalid metrics port: %d", config.Monitoring.MetricsPort)
	}

	// Validate WebSocket configuration
	if config.WebSocket.Enabled {
		if config.WebSocket.Path == "" {
			return fmt.Errorf("WebSocket path is required when WebSocket is enabled")
		}
		if config.WebSocket.MaxConnections <= 0 {
			return fmt.Errorf("WebSocket max connections must be positive")
		}
	}

	return nil
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}

// GetConfigPath returns the configuration file path
func GetConfigPath() string {
	if path := os.Getenv("CONFIG_PATH"); path != "" {
		return path
	}
	
	// Check common locations
	locations := []string{
		"./config.yaml",
		"./configs/config.yaml",
		"/etc/file-processing-service/config.yaml",
	}
	
	for _, location := range locations {
		if fileExists(location) {
			return location
		}
	}
	
	return ""
}

// LoadDefault loads configuration with default values
func LoadDefault() (*models.Config, error) {
	return Load(GetConfigPath())
}