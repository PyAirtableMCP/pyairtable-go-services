package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Services ServicesConfig `mapstructure:"services"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Security SecurityConfig `mapstructure:"security"`
	Observability ObservabilityConfig `mapstructure:"observability"`
	Performance PerformanceConfig `mapstructure:"performance"`
	Features FeaturesConfig `mapstructure:"features"`
	Routes   []RouteConfig  `mapstructure:"routes"`
}

type ServerConfig struct {
	Host             string        `mapstructure:"host"`
	Port             int           `mapstructure:"port"`
	Environment      string        `mapstructure:"environment"`
	ReadTimeout      time.Duration `mapstructure:"read_timeout"`
	WriteTimeout     time.Duration `mapstructure:"write_timeout"`
	IdleTimeout      time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout  time.Duration `mapstructure:"shutdown_timeout"`
	BodyLimit        int           `mapstructure:"body_limit"`
	Concurrency      int           `mapstructure:"concurrency"`
	Prefork          bool          `mapstructure:"prefork"`
	DisableKeepalive bool          `mapstructure:"disable_keepalive"`
}

type ServicesConfig struct {
	AuthService        ServiceEndpoint `mapstructure:"auth_service"`
	UserService        ServiceEndpoint `mapstructure:"user_service"`
	WorkspaceService   ServiceEndpoint `mapstructure:"workspace_service"`
	BaseService        ServiceEndpoint `mapstructure:"base_service"`
	TableService       ServiceEndpoint `mapstructure:"table_service"`
	ViewService        ServiceEndpoint `mapstructure:"view_service"`
	RecordService      ServiceEndpoint `mapstructure:"record_service"`
	FieldService       ServiceEndpoint `mapstructure:"field_service"`
	FormulaService     ServiceEndpoint `mapstructure:"formula_service"`
	LLMOrchestrator    ServiceEndpoint `mapstructure:"llm_orchestrator"`
	FileProcessor      ServiceEndpoint `mapstructure:"file_processor"`
	AirtableGateway    ServiceEndpoint `mapstructure:"airtable_gateway"`
	AnalyticsService   ServiceEndpoint `mapstructure:"analytics_service"`
	WorkflowEngine     ServiceEndpoint `mapstructure:"workflow_engine"`
}

type ServiceEndpoint struct {
	URL             string        `mapstructure:"url"`
	Timeout         time.Duration `mapstructure:"timeout"`
	RetryCount      int           `mapstructure:"retry_count"`
	RetryDelay      time.Duration `mapstructure:"retry_delay"`
	HealthCheckPath string        `mapstructure:"health_check_path"`
	Weight          int           `mapstructure:"weight"`
}

type AuthConfig struct {
	JWTSecret           string        `mapstructure:"jwt_secret"`
	JWTExpiry           time.Duration `mapstructure:"jwt_expiry"`
	RefreshTokenExpiry  time.Duration `mapstructure:"refresh_token_expiry"`
	APIKeyHeader        string        `mapstructure:"api_key_header"`
	RequireAuth         bool          `mapstructure:"require_auth"`
}

type RedisConfig struct {
	URL         string `mapstructure:"url"`
	Password    string `mapstructure:"password"`
	DB          int    `mapstructure:"db"`
	PoolSize    int    `mapstructure:"pool_size"`
	MinIdleConn int    `mapstructure:"min_idle_conn"`
}

type SecurityConfig struct {
	CORS          CORSConfig `mapstructure:"cors"`
	RateLimit     RateLimitConfig `mapstructure:"rate_limit"`
	MTLS          MTLSConfig `mapstructure:"mtls"`
	TrustedProxies []string `mapstructure:"trusted_proxies"`
}

type CORSConfig struct {
	AllowOrigins     []string `mapstructure:"allow_origins"`
	AllowMethods     []string `mapstructure:"allow_methods"`
	AllowHeaders     []string `mapstructure:"allow_headers"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
	MaxAge           int      `mapstructure:"max_age"`
}

type RateLimitConfig struct {
	Enabled       bool          `mapstructure:"enabled"`
	GlobalLimit   int           `mapstructure:"global_limit"`
	PerIPLimit    int           `mapstructure:"per_ip_limit"`
	Window        time.Duration `mapstructure:"window"`
	SkipPaths     []string      `mapstructure:"skip_paths"`
}

type MTLSConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	CertFile   string `mapstructure:"cert_file"`
	KeyFile    string `mapstructure:"key_file"`
	CAFile     string `mapstructure:"ca_file"`
	SkipPaths  []string `mapstructure:"skip_paths"`
}

type ObservabilityConfig struct {
	Metrics MetricsConfig `mapstructure:"metrics"`
	Tracing TracingConfig `mapstructure:"tracing"`
	Logging LoggingConfig `mapstructure:"logging"`
}

type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
	Port    int    `mapstructure:"port"`
}

type TracingConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	ServiceName string `mapstructure:"service_name"`
	Endpoint    string `mapstructure:"endpoint"`
	SampleRate  float64 `mapstructure:"sample_rate"`
}

type LoggingConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	Output     string `mapstructure:"output"`
	RequestLog bool   `mapstructure:"request_log"`
}

type PerformanceConfig struct {
	CircuitBreaker CircuitBreakerConfig `mapstructure:"circuit_breaker"`
	LoadBalancing  LoadBalancingConfig  `mapstructure:"load_balancing"`
	Caching        CachingConfig        `mapstructure:"caching"`
}

type CircuitBreakerConfig struct {
	Enabled           bool          `mapstructure:"enabled"`
	MaxRequests       uint32        `mapstructure:"max_requests"`
	Interval          time.Duration `mapstructure:"interval"`
	Timeout           time.Duration `mapstructure:"timeout"`
}

type LoadBalancingConfig struct {
	Algorithm      string `mapstructure:"algorithm"` // round_robin, weighted, least_connections
	HealthCheck    bool   `mapstructure:"health_check"`
	CheckInterval  time.Duration `mapstructure:"check_interval"`
	StickySession  bool   `mapstructure:"sticky_session"`
}

type CachingConfig struct {
	Enabled       bool          `mapstructure:"enabled"`
	DefaultTTL    time.Duration `mapstructure:"default_ttl"`
	MaxMemory     int64         `mapstructure:"max_memory"`
	CleanupInterval time.Duration `mapstructure:"cleanup_interval"`
}

type FeaturesConfig struct {
	WebSocketEnabled    bool     `mapstructure:"websocket_enabled"`
	BlueGreenDeployment bool     `mapstructure:"blue_green_deployment"`
	APIVersioning       bool     `mapstructure:"api_versioning"`
	RequestTracing      bool     `mapstructure:"request_tracing"`
	GracefulShutdown    bool     `mapstructure:"graceful_shutdown"`
	PProf               bool     `mapstructure:"pprof"`
	HealthCheckPaths    []string `mapstructure:"health_check_paths"`
}

type RouteConfig struct {
	Prefix  string   `mapstructure:"prefix"`
	Target  string   `mapstructure:"target"`
	Methods []string `mapstructure:"methods"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath(".")
	
	// Set environment variable prefix
	viper.SetEnvPrefix("GATEWAY")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Set defaults
	setDefaults()

	// Read config file if exists
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Override with environment variables
	overrideWithEnv(&config)

	// Load routes configuration
	if err := loadRoutes(&config); err != nil {
		return nil, fmt.Errorf("error loading routes: %w", err)
	}

	return &config, nil
}

func setDefaults() {
	// Server defaults
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.environment", "development")
	viper.SetDefault("server.read_timeout", "30s")
	viper.SetDefault("server.write_timeout", "30s")
	viper.SetDefault("server.idle_timeout", "120s")
	viper.SetDefault("server.shutdown_timeout", "30s")
	viper.SetDefault("server.body_limit", 4194304) // 4MB
	viper.SetDefault("server.concurrency", 256*1024) // 256K concurrent connections
	viper.SetDefault("server.prefork", false)
	viper.SetDefault("server.disable_keepalive", false)

	// Service defaults
	services := map[string]int{
		"auth_service": 8081, "user_service": 8082, "workspace_service": 8083,
		"base_service": 8084, "table_service": 8085, "view_service": 8086,
		"record_service": 8087, "field_service": 8088, "formula_service": 8089,
		"llm_orchestrator": 8091, "file_processor": 8092, "airtable_gateway": 8093,
		"analytics_service": 8094, "workflow_engine": 8095,
	}
	
	for service, port := range services {
		viper.SetDefault(fmt.Sprintf("services.%s.url", service), fmt.Sprintf("http://localhost:%d", port))
		viper.SetDefault(fmt.Sprintf("services.%s.timeout", service), "30s")
		viper.SetDefault(fmt.Sprintf("services.%s.retry_count", service), 3)
		viper.SetDefault(fmt.Sprintf("services.%s.retry_delay", service), "1s")
		viper.SetDefault(fmt.Sprintf("services.%s.health_check_path", service), "/health")
		viper.SetDefault(fmt.Sprintf("services.%s.weight", service), 1)
	}

	// Auth defaults
	viper.SetDefault("auth.jwt_expiry", "24h")
	viper.SetDefault("auth.refresh_token_expiry", "168h") // 7 days
	viper.SetDefault("auth.api_key_header", "X-API-Key")
	viper.SetDefault("auth.require_auth", true)

	// Redis defaults
	viper.SetDefault("redis.url", "redis://localhost:6379")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("redis.pool_size", 10)
	viper.SetDefault("redis.min_idle_conn", 1)

	// Security defaults
	viper.SetDefault("security.cors.allow_origins", []string{"http://localhost:3000"})
	viper.SetDefault("security.cors.allow_methods", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"})
	viper.SetDefault("security.cors.allow_headers", []string{"Content-Type", "Authorization", "X-API-Key", "X-Trace-ID"})
	viper.SetDefault("security.cors.allow_credentials", true)
	viper.SetDefault("security.cors.max_age", 3600)
	
	viper.SetDefault("security.rate_limit.enabled", true)
	viper.SetDefault("security.rate_limit.global_limit", 10000)
	viper.SetDefault("security.rate_limit.per_ip_limit", 100)
	viper.SetDefault("security.rate_limit.window", "1m")
	viper.SetDefault("security.rate_limit.skip_paths", []string{"/health", "/metrics"})

	// Observability defaults
	viper.SetDefault("observability.metrics.enabled", true)
	viper.SetDefault("observability.metrics.path", "/metrics")
	viper.SetDefault("observability.metrics.port", 9090)
	
	viper.SetDefault("observability.tracing.enabled", true)
	viper.SetDefault("observability.tracing.service_name", "api-gateway")
	viper.SetDefault("observability.tracing.sample_rate", 0.1)
	
	viper.SetDefault("observability.logging.level", "info")
	viper.SetDefault("observability.logging.format", "json")
	viper.SetDefault("observability.logging.output", "stdout")
	viper.SetDefault("observability.logging.request_log", true)

	// Performance defaults
	viper.SetDefault("performance.circuit_breaker.enabled", true)
	viper.SetDefault("performance.circuit_breaker.max_requests", 3)
	viper.SetDefault("performance.circuit_breaker.interval", "10s")
	viper.SetDefault("performance.circuit_breaker.timeout", "60s")
	
	viper.SetDefault("performance.load_balancing.algorithm", "round_robin")
	viper.SetDefault("performance.load_balancing.health_check", true)
	viper.SetDefault("performance.load_balancing.check_interval", "30s")
	viper.SetDefault("performance.load_balancing.sticky_session", false)
	
	viper.SetDefault("performance.caching.enabled", true)
	viper.SetDefault("performance.caching.default_ttl", "5m")
	viper.SetDefault("performance.caching.max_memory", 104857600) // 100MB
	viper.SetDefault("performance.caching.cleanup_interval", "10m")

	// Features defaults
	viper.SetDefault("features.websocket_enabled", true)
	viper.SetDefault("features.blue_green_deployment", false)
	viper.SetDefault("features.api_versioning", true)
	viper.SetDefault("features.request_tracing", true)
	viper.SetDefault("features.graceful_shutdown", true)
	viper.SetDefault("features.pprof", false)
	viper.SetDefault("features.health_check_paths", []string{"/health", "/ready", "/live"})
}

func overrideWithEnv(config *Config) {
	// Override with environment variables that follow Go naming conventions
	if port := os.Getenv("PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Server.Port = p
		}
	}
	
	if jwtSecret := os.Getenv("JWT_SECRET"); jwtSecret != "" {
		config.Auth.JWTSecret = jwtSecret
	}
	
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		config.Redis.URL = redisURL
	}
	
	if env := os.Getenv("ENVIRONMENT"); env != "" {
		config.Server.Environment = env
	}
	
	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		config.Observability.Logging.Level = logLevel
	}
	
	if corsOrigins := os.Getenv("CORS_ORIGINS"); corsOrigins != "" {
		config.Security.CORS.AllowOrigins = strings.Split(corsOrigins, ",")
	}
}

func loadRoutes(config *Config) error {
	routesViper := viper.New()
	routesViper.SetConfigName("routes")
	routesViper.SetConfigType("yaml")
	routesViper.AddConfigPath("./configs")
	routesViper.AddConfigPath(".")
	
	if err := routesViper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Routes file is optional, return empty routes if not found
			return nil
		}
		return fmt.Errorf("error reading routes config: %w", err)
	}
	
	var routesConfig struct {
		Routes []RouteConfig `mapstructure:"routes"`
	}
	
	if err := routesViper.Unmarshal(&routesConfig); err != nil {
		return fmt.Errorf("error unmarshaling routes config: %w", err)
	}
	
	config.Routes = routesConfig.Routes
	return nil
}