package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the plugin service
type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Redis      RedisConfig      `mapstructure:"redis"`
	Runtime    RuntimeConfig    `mapstructure:"runtime"`
	Security   SecurityConfig   `mapstructure:"security"`
	Registry   RegistryConfig   `mapstructure:"registry"`
	Monitoring MonitoringConfig `mapstructure:"monitoring"`
	Storage    StorageConfig    `mapstructure:"storage"`
	Logging    LoggingConfig    `mapstructure:"logging"`
}

// ServerConfig contains HTTP server configuration
type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
	TLS          TLSConfig     `mapstructure:"tls"`
}

// TLSConfig contains TLS configuration
type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
}

// DatabaseConfig contains database configuration
type DatabaseConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	DBName          string        `mapstructure:"dbname"`
	SSLMode         string        `mapstructure:"sslmode"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	MigrationsPath  string        `mapstructure:"migrations_path"`
}

// RedisConfig contains Redis configuration
type RedisConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	Password     string        `mapstructure:"password"`
	DB           int           `mapstructure:"db"`
	PoolSize     int           `mapstructure:"pool_size"`
	DialTimeout  time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
}

// RuntimeConfig contains WASM runtime configuration
type RuntimeConfig struct {
	Engine          string                 `mapstructure:"engine"` // wasmtime, wazero
	MaxInstances    int                    `mapstructure:"max_instances"`
	InstanceTimeout time.Duration          `mapstructure:"instance_timeout"`
	DefaultLimits   RuntimeResourceLimits  `mapstructure:"default_limits"`
	MaxLimits       RuntimeResourceLimits  `mapstructure:"max_limits"`
	CacheSize       int                    `mapstructure:"cache_size"`
	PrecompilePlugins bool                 `mapstructure:"precompile_plugins"`
}

// RuntimeResourceLimits defines default and maximum resource limits
type RuntimeResourceLimits struct {
	MaxMemoryMB    int32         `mapstructure:"max_memory_mb"`
	MaxCPUPercent  int32         `mapstructure:"max_cpu_percent"`
	MaxExecutionMs int32         `mapstructure:"max_execution_ms"`
	MaxStorageMB   int32         `mapstructure:"max_storage_mb"`
	MaxNetworkReqs int32         `mapstructure:"max_network_reqs"`
}

// SecurityConfig contains security configuration
type SecurityConfig struct {
	JWTSecret              string        `mapstructure:"jwt_secret"`
	JWTExpirationTime      time.Duration `mapstructure:"jwt_expiration_time"`
	CodeSigningEnabled     bool          `mapstructure:"code_signing_enabled"`
	TrustedSigners         []string      `mapstructure:"trusted_signers"`
	SandboxEnabled         bool          `mapstructure:"sandbox_enabled"`
	PermissionCheckEnabled bool          `mapstructure:"permission_check_enabled"`
	MaxPluginSize          int64         `mapstructure:"max_plugin_size"`
	AllowedFileTypes       []string      `mapstructure:"allowed_file_types"`
	QuarantineEnabled      bool          `mapstructure:"quarantine_enabled"`
	QuarantineTime         time.Duration `mapstructure:"quarantine_time"`
}

// RegistryConfig contains plugin registry configuration
type RegistryConfig struct {
	DefaultRegistry  string              `mapstructure:"default_registry"`
	OfficialRegistry string              `mapstructure:"official_registry"`
	CacheEnabled     bool                `mapstructure:"cache_enabled"`
	CacheTTL         time.Duration       `mapstructure:"cache_ttl"`
	SyncInterval     time.Duration       `mapstructure:"sync_interval"`
	Registries       []RegistryEndpoint  `mapstructure:"registries"`
}

// RegistryEndpoint represents a plugin registry endpoint
type RegistryEndpoint struct {
	Name      string   `mapstructure:"name"`
	URL       string   `mapstructure:"url"`
	Type      string   `mapstructure:"type"` // official, community, private
	Enabled   bool     `mapstructure:"enabled"`
	TrustedKeys []string `mapstructure:"trusted_keys"`
}

// MonitoringConfig contains monitoring configuration
type MonitoringConfig struct {
	Enabled           bool          `mapstructure:"enabled"`
	MetricsEnabled    bool          `mapstructure:"metrics_enabled"`
	MetricsPath       string        `mapstructure:"metrics_path"`
	HealthCheckPath   string        `mapstructure:"health_check_path"`
	TracingEnabled    bool          `mapstructure:"tracing_enabled"`
	TracingEndpoint   string        `mapstructure:"tracing_endpoint"`
	ServiceName       string        `mapstructure:"service_name"`
	LogLevel          string        `mapstructure:"log_level"`
	AuditLogEnabled   bool          `mapstructure:"audit_log_enabled"`
	MetricsInterval   time.Duration `mapstructure:"metrics_interval"`
}

// StorageConfig contains storage configuration
type StorageConfig struct {
	Type         string          `mapstructure:"type"` // local, s3, gcs, azure
	Local        LocalStorage    `mapstructure:"local"`
	S3           S3Storage       `mapstructure:"s3"`
	MaxFileSize  int64           `mapstructure:"max_file_size"`
	TempDir      string          `mapstructure:"temp_dir"`
	Encryption   EncryptionConfig `mapstructure:"encryption"`
}

// LocalStorage contains local storage configuration
type LocalStorage struct {
	BasePath    string `mapstructure:"base_path"`
	Permissions int    `mapstructure:"permissions"`
}

// S3Storage contains S3 storage configuration
type S3Storage struct {
	Bucket          string `mapstructure:"bucket"`
	Region          string `mapstructure:"region"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
	Endpoint        string `mapstructure:"endpoint"`
	UseSSL          bool   `mapstructure:"use_ssl"`
	PathStyle       bool   `mapstructure:"path_style"`
}

// EncryptionConfig contains encryption configuration
type EncryptionConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Key       string `mapstructure:"key"`
	Algorithm string `mapstructure:"algorithm"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"` // json, text
	Output     string `mapstructure:"output"` // stdout, file, both
	FilePath   string `mapstructure:"file_path"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
	Compress   bool   `mapstructure:"compress"`
}

// Load loads configuration from various sources
func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/plugin-service")

	// Set default values
	setDefaults()

	// Enable environment variable support
	viper.AutomaticEnv()
	viper.SetEnvPrefix("PLUGIN_SERVICE")

	// Read configuration file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// setDefaults sets default configuration values
func setDefaults() {
	// Server defaults
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.read_timeout", "30s")
	viper.SetDefault("server.write_timeout", "30s")
	viper.SetDefault("server.idle_timeout", "120s")
	viper.SetDefault("server.tls.enabled", false)

	// Database defaults
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "postgres")
	viper.SetDefault("database.password", "")
	viper.SetDefault("database.dbname", "plugin_service")
	viper.SetDefault("database.sslmode", "disable")
	viper.SetDefault("database.max_open_conns", 25)
	viper.SetDefault("database.max_idle_conns", 5)
	viper.SetDefault("database.conn_max_lifetime", "5m")
	viper.SetDefault("database.migrations_path", "./migrations")

	// Redis defaults
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("redis.pool_size", 10)
	viper.SetDefault("redis.dial_timeout", "5s")
	viper.SetDefault("redis.read_timeout", "3s")
	viper.SetDefault("redis.write_timeout", "3s")
	viper.SetDefault("redis.idle_timeout", "5m")

	// Runtime defaults
	viper.SetDefault("runtime.engine", "wazero")
	viper.SetDefault("runtime.max_instances", 100)
	viper.SetDefault("runtime.instance_timeout", "30s")
	viper.SetDefault("runtime.cache_size", 50)
	viper.SetDefault("runtime.precompile_plugins", true)
	
	// Default resource limits
	viper.SetDefault("runtime.default_limits.max_memory_mb", 64)
	viper.SetDefault("runtime.default_limits.max_cpu_percent", 50)
	viper.SetDefault("runtime.default_limits.max_execution_ms", 5000)
	viper.SetDefault("runtime.default_limits.max_storage_mb", 10)
	viper.SetDefault("runtime.default_limits.max_network_reqs", 10)
	
	// Maximum resource limits
	viper.SetDefault("runtime.max_limits.max_memory_mb", 256)
	viper.SetDefault("runtime.max_limits.max_cpu_percent", 100)
	viper.SetDefault("runtime.max_limits.max_execution_ms", 30000)
	viper.SetDefault("runtime.max_limits.max_storage_mb", 100)
	viper.SetDefault("runtime.max_limits.max_network_reqs", 100)

	// Security defaults
	viper.SetDefault("security.jwt_secret", "change-me-in-production")
	viper.SetDefault("security.jwt_expiration_time", "1h")
	viper.SetDefault("security.code_signing_enabled", true)
	viper.SetDefault("security.sandbox_enabled", true)
	viper.SetDefault("security.permission_check_enabled", true)
	viper.SetDefault("security.max_plugin_size", 10485760) // 10MB
	viper.SetDefault("security.allowed_file_types", []string{"wasm", "wat"})
	viper.SetDefault("security.quarantine_enabled", true)
	viper.SetDefault("security.quarantine_time", "24h")

	// Registry defaults
	viper.SetDefault("registry.default_registry", "official")
	viper.SetDefault("registry.official_registry", "https://plugins.pyairtable.com")
	viper.SetDefault("registry.cache_enabled", true)
	viper.SetDefault("registry.cache_ttl", "1h")
	viper.SetDefault("registry.sync_interval", "24h")

	// Monitoring defaults
	viper.SetDefault("monitoring.enabled", true)
	viper.SetDefault("monitoring.metrics_enabled", true)
	viper.SetDefault("monitoring.metrics_path", "/metrics")
	viper.SetDefault("monitoring.health_check_path", "/health")
	viper.SetDefault("monitoring.tracing_enabled", false)
	viper.SetDefault("monitoring.service_name", "plugin-service")
	viper.SetDefault("monitoring.log_level", "info")
	viper.SetDefault("monitoring.audit_log_enabled", true)
	viper.SetDefault("monitoring.metrics_interval", "15s")

	// Storage defaults
	viper.SetDefault("storage.type", "local")
	viper.SetDefault("storage.local.base_path", "./data/plugins")
	viper.SetDefault("storage.local.permissions", 0755)
	viper.SetDefault("storage.max_file_size", 52428800) // 50MB
	viper.SetDefault("storage.temp_dir", "/tmp/plugin-service")
	viper.SetDefault("storage.encryption.enabled", false)
	viper.SetDefault("storage.encryption.algorithm", "AES-256-GCM")

	// Logging defaults
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")
	viper.SetDefault("logging.output", "stdout")
	viper.SetDefault("logging.max_size", 100)
	viper.SetDefault("logging.max_backups", 3)
	viper.SetDefault("logging.max_age", 28)
	viper.SetDefault("logging.compress", true)
}

// validateConfig validates the configuration
func validateConfig(config *Config) error {
	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", config.Server.Port)
	}

	if config.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}

	if config.Database.DBName == "" {
		return fmt.Errorf("database name is required")
	}

	if config.Runtime.MaxInstances <= 0 {
		return fmt.Errorf("runtime max_instances must be positive")
	}

	if config.Security.JWTSecret == "change-me-in-production" {
		fmt.Println("WARNING: Using default JWT secret. Please change it in production!")
	}

	return nil
}

// GetDSN returns the database connection string
func (c *DatabaseConfig) GetDSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode)
}

// GetRedisAddr returns the Redis address
func (c *RedisConfig) GetRedisAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}