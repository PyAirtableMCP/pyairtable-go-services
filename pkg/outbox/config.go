package outbox

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/pyairtable-compose/go-services/pkg/cache"
)

// OutboxServiceConfig aggregates all outbox-related configuration
type OutboxServiceConfig struct {
	// Database configuration
	DatabaseURL      string
	OutboxTableName  string
	
	// Basic outbox settings
	BatchSize           int
	PollInterval        time.Duration
	MaxRetries          int
	RetryBackoffBase    time.Duration
	RetryBackoffMax     time.Duration
	PublishTimeout      time.Duration
	
	// Worker configuration
	WorkerCount         int
	ProcessingTimeout   time.Duration
	MaxConcurrency      int
	HeartbeatInterval   time.Duration
	
	// Publisher configuration
	PublisherType       string // event_bus, kafka, redis_stream, webhook, composite
	PublisherConfig     map[string]interface{}
	
	// Monitoring and observability
	EnableMetrics       bool
	MetricsNamespace    string
	EnableDeliveryLog   bool
	LogSuccessfulDeliveries bool
	
	// Maintenance
	CleanupInterval     time.Duration
	RetentionDays       int
	
	// Circuit breaker
	CircuitBreakerEnabled    bool
	CircuitBreakerMaxFailures int
	CircuitBreakerResetTimeout time.Duration
	
	// Service identification
	ServiceName         string
	ProcessorID         string
	ProcessorName       string
}

// DefaultOutboxConfig returns a default configuration
func DefaultOutboxConfig() OutboxServiceConfig {
	return OutboxServiceConfig{
		OutboxTableName:     "event_outbox",
		BatchSize:          50,
		PollInterval:       5 * time.Second,
		MaxRetries:         3,
		RetryBackoffBase:   60 * time.Second,
		RetryBackoffMax:    300 * time.Second,
		PublishTimeout:     30 * time.Second,
		WorkerCount:        2,
		ProcessingTimeout:  60 * time.Second,
		MaxConcurrency:     2,
		HeartbeatInterval:  30 * time.Second,
		PublisherType:      "event_bus",
		EnableMetrics:      true,
		MetricsNamespace:   "pyairtable",
		EnableDeliveryLog:  true,
		LogSuccessfulDeliveries: false,
		CleanupInterval:    1 * time.Hour,
		RetentionDays:      7,
		CircuitBreakerEnabled: true,
		CircuitBreakerMaxFailures: 5,
		CircuitBreakerResetTimeout: 60 * time.Second,
		ServiceName:        "unknown",
	}
}

// LoadFromEnvironment loads configuration from environment variables
func (c *OutboxServiceConfig) LoadFromEnvironment() {
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		c.DatabaseURL = dbURL
	}
	
	if tableName := os.Getenv("OUTBOX_TABLE_NAME"); tableName != "" {
		c.OutboxTableName = tableName
	}
	
	if batchSize := os.Getenv("OUTBOX_BATCH_SIZE"); batchSize != "" {
		if size, err := strconv.Atoi(batchSize); err == nil {
			c.BatchSize = size
		}
	}
	
	if pollInterval := os.Getenv("OUTBOX_POLL_INTERVAL"); pollInterval != "" {
		if duration, err := time.ParseDuration(pollInterval); err == nil {
			c.PollInterval = duration
		}
	}
	
	if maxRetries := os.Getenv("OUTBOX_MAX_RETRIES"); maxRetries != "" {
		if retries, err := strconv.Atoi(maxRetries); err == nil {
			c.MaxRetries = retries
		}
	}
	
	if workerCount := os.Getenv("OUTBOX_WORKER_COUNT"); workerCount != "" {
		if count, err := strconv.Atoi(workerCount); err == nil {
			c.WorkerCount = count
		}
	}
	
	if publisherType := os.Getenv("OUTBOX_PUBLISHER_TYPE"); publisherType != "" {
		c.PublisherType = publisherType
	}
	
	if serviceName := os.Getenv("SERVICE_NAME"); serviceName != "" {
		c.ServiceName = serviceName
		c.ProcessorName = fmt.Sprintf("%s-outbox-processor", serviceName)
	}
	
	if enableMetrics := os.Getenv("OUTBOX_ENABLE_METRICS"); enableMetrics != "" {
		c.EnableMetrics = enableMetrics == "true"
	}
	
	if retentionDays := os.Getenv("OUTBOX_RETENTION_DAYS"); retentionDays != "" {
		if days, err := strconv.Atoi(retentionDays); err == nil {
			c.RetentionDays = days
		}
	}
}

// Validate validates the configuration
func (c *OutboxServiceConfig) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("database URL is required")
	}
	
	if c.OutboxTableName == "" {
		return fmt.Errorf("outbox table name is required")
	}
	
	if c.BatchSize <= 0 {
		return fmt.Errorf("batch size must be greater than 0")
	}
	
	if c.WorkerCount <= 0 {
		return fmt.Errorf("worker count must be greater than 0")
	}
	
	if c.MaxRetries < 0 {
		return fmt.Errorf("max retries cannot be negative")
	}
	
	if c.ServiceName == "" {
		return fmt.Errorf("service name is required")
	}
	
	return nil
}

// OutboxInitializer helps initialize outbox components for a service
type OutboxInitializer struct {
	config OutboxServiceConfig
	db     *sql.DB
}

// NewOutboxInitializer creates a new outbox initializer
func NewOutboxInitializer(config OutboxServiceConfig, db *sql.DB) *OutboxInitializer {
	return &OutboxInitializer{
		config: config,
		db:     db,
	}
}

// Initialize sets up all outbox components
func (oi *OutboxInitializer) Initialize() (*OutboxService, error) {
	// Validate configuration
	if err := oi.config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	// Create publisher
	publisher, err := oi.createPublisher()
	if err != nil {
		return nil, fmt.Errorf("failed to create publisher: %w", err)
	}
	
	// Create enhanced outbox
	enhancedConfig := oi.createEnhancedConfig()
	outbox := NewEnhancedTransactionalOutbox(oi.db, enhancedConfig, publisher)
	
	// Create repository wrapper
	wrapper := NewOutboxRepositoryWrapper(oi.db, outbox)
	
	// Create worker manager
	workerConfig := oi.createWorkerConfig()
	workerManager := NewOutboxWorkerManager(oi.db, workerConfig, publisher)
	
	// Create outbox service
	service := &OutboxService{
		Outbox:        outbox,
		Wrapper:       wrapper,
		WorkerManager: workerManager,
		Config:        oi.config,
	}
	
	return service, nil
}

// createPublisher creates the appropriate publisher based on configuration
func (oi *OutboxInitializer) createPublisher() (EventPublisher, error) {
	switch oi.config.PublisherType {
	case "event_bus":
		return oi.createEventBusPublisher()
	case "kafka":
		return oi.createKafkaPublisher()
	case "redis_stream":
		return oi.createRedisStreamPublisher()
	case "webhook":
		return oi.createWebhookPublisher()
	case "composite":
		return oi.createCompositePublisher()
	default:
		return nil, fmt.Errorf("unknown publisher type: %s", oi.config.PublisherType)
	}
}

// createEventBusPublisher creates an in-memory event bus publisher
func (oi *OutboxInitializer) createEventBusPublisher() (EventPublisher, error) {
	// Create a simple in-memory event bus
	eventBus := &SimpleEventBus{}
	return NewEventBusPublisher(eventBus), nil
}

// createKafkaPublisher creates a Kafka publisher
func (oi *OutboxInitializer) createKafkaPublisher() (EventPublisher, error) {
	brokers, ok := oi.config.PublisherConfig["brokers"].([]string)
	if !ok {
		brokers = []string{"localhost:9092"}
	}
	
	topic, ok := oi.config.PublisherConfig["topic"].(string)
	if !ok {
		topic = "pyairtable-events"
	}
	
	publisher := NewKafkaPublisher(brokers, topic)
	
	// Add circuit breaker if enabled
	if oi.config.CircuitBreakerEnabled {
		publisher = NewCircuitBreakerPublisher(
			publisher,
			oi.config.CircuitBreakerMaxFailures,
			oi.config.CircuitBreakerResetTimeout,
		)
	}
	
	return publisher, nil
}

// createRedisStreamPublisher creates a Redis Stream publisher
func (oi *OutboxInitializer) createRedisStreamPublisher() (EventPublisher, error) {
	redisURL, ok := oi.config.PublisherConfig["redis_url"].(string)
	if !ok {
		redisURL = "redis://localhost:6379"
	}
	
	stream, ok := oi.config.PublisherConfig["stream"].(string)
	if !ok {
		stream = "pyairtable:events"
	}
	
	// Create Redis client (simplified - you'd want proper connection management)
	redisClient := &cache.RedisClient{} // Assuming this exists
	
	return NewRedisStreamPublisher(redisClient, stream), nil
}

// createWebhookPublisher creates a webhook publisher
func (oi *OutboxInitializer) createWebhookPublisher() (EventPublisher, error) {
	webhookURL, ok := oi.config.PublisherConfig["url"].(string)
	if !ok {
		return nil, fmt.Errorf("webhook URL is required")
	}
	
	headers, ok := oi.config.PublisherConfig["headers"].(map[string]string)
	if !ok {
		headers = make(map[string]string)
	}
	
	timeout := oi.config.PublishTimeout
	
	return NewWebhookPublisher(webhookURL, headers, timeout), nil
}

// createCompositePublisher creates a composite publisher with multiple backends
func (oi *OutboxInitializer) createCompositePublisher() (EventPublisher, error) {
	publishers := []EventPublisher{}
	
	// Add event bus as primary
	eventBusPublisher, _ := oi.createEventBusPublisher()
	publishers = append(publishers, eventBusPublisher)
	
	// Add additional publishers if configured
	if kafkaConfig, ok := oi.config.PublisherConfig["kafka"]; ok && kafkaConfig != nil {
		kafkaPublisher, err := oi.createKafkaPublisher()
		if err == nil {
			publishers = append(publishers, kafkaPublisher)
		}
	}
	
	strategy := PublishPrimary // Default strategy
	if strategyStr, ok := oi.config.PublisherConfig["strategy"].(string); ok {
		switch strategyStr {
		case "all":
			strategy = PublishAll
		case "any":
			strategy = PublishAny
		case "primary":
			strategy = PublishPrimary
		}
	}
	
	return NewCompositePublisher(publishers, strategy), nil
}

// createEnhancedConfig creates enhanced outbox configuration
func (oi *OutboxInitializer) createEnhancedConfig() EnhancedOutboxConfig {
	return EnhancedOutboxConfig{
		OutboxConfig: OutboxConfig{
			TableName:           oi.config.OutboxTableName,
			BatchSize:           oi.config.BatchSize,
			PollInterval:        oi.config.PollInterval,
			MaxRetries:          oi.config.MaxRetries,
			RetryBackoffBase:    oi.config.RetryBackoffBase,
			RetryBackoffMax:     oi.config.RetryBackoffMax,
			WorkerCount:         oi.config.WorkerCount,
			EnableDeduplication: true,
			PublishTimeout:      oi.config.PublishTimeout,
		},
		ProcessorID:       oi.config.ProcessorID,
		ProcessorName:     oi.config.ProcessorName,
		ProcessorType:     oi.config.PublisherType,
		HeartbeatInterval: oi.config.HeartbeatInterval,
		CleanupInterval:   oi.config.CleanupInterval,
		RetentionDays:     oi.config.RetentionDays,
		DeadLetterThreshold: oi.config.MaxRetries * 2,
		CircuitBreakerConfig: CircuitBreakerConfig{
			MaxFailures:      oi.config.CircuitBreakerMaxFailures,
			ResetTimeout:     oi.config.CircuitBreakerResetTimeout,
			FailureThreshold: 0.5,
		},
		EnableMetrics:           oi.config.EnableMetrics,
		MetricsNamespace:        oi.config.MetricsNamespace,
		EnableDeliveryLog:       oi.config.EnableDeliveryLog,
		LogSuccessfulDeliveries: oi.config.LogSuccessfulDeliveries,
	}
}

// createWorkerConfig creates worker configuration
func (oi *OutboxInitializer) createWorkerConfig() WorkerConfig {
	return WorkerConfig{
		WorkerID:          oi.config.ProcessorID,
		BatchSize:         oi.config.BatchSize,
		PollInterval:      oi.config.PollInterval,
		ProcessingTimeout: oi.config.ProcessingTimeout,
		MaxConcurrency:    oi.config.MaxConcurrency,
		RetryDelay:        oi.config.RetryBackoffBase,
		DeadLetterDelay:   oi.config.RetryBackoffMax,
		EnableMetrics:     oi.config.EnableMetrics,
		MetricsNamespace:  oi.config.MetricsNamespace,
	}
}

// OutboxService aggregates all outbox components
type OutboxService struct {
	Outbox        *EnhancedTransactionalOutbox
	Wrapper       *OutboxRepositoryWrapper
	WorkerManager *OutboxWorkerManager
	Config        OutboxServiceConfig
}

// Start starts all outbox components
func (os *OutboxService) Start() error {
	log.Printf("Starting outbox service for %s", os.Config.ServiceName)
	
	// Start outbox
	if err := os.Outbox.Start(); err != nil {
		return fmt.Errorf("failed to start outbox: %w", err)
	}
	
	// Start worker manager
	if err := os.WorkerManager.Start(); err != nil {
		return fmt.Errorf("failed to start worker manager: %w", err)
	}
	
	log.Printf("Outbox service started successfully")
	return nil
}

// Stop stops all outbox components
func (os *OutboxService) Stop() error {
	log.Printf("Stopping outbox service for %s", os.Config.ServiceName)
	
	// Stop worker manager
	if err := os.WorkerManager.Stop(); err != nil {
		log.Printf("Error stopping worker manager: %v", err)
	}
	
	// Stop outbox
	if err := os.Outbox.Stop(); err != nil {
		log.Printf("Error stopping outbox: %v", err)
	}
	
	log.Printf("Outbox service stopped")
	return nil
}

// GetStatus returns the status of all components
func (os *OutboxService) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"service":        os.Config.ServiceName,
		"processor":      os.Outbox.GetProcessorStatus(),
		"outbox_stats":   os.Outbox.GetStats(),
		"config":         os.Config,
	}
}

// SimpleEventBus is a basic in-memory event bus implementation
type SimpleEventBus struct {
	handlers map[string][]func(interface{}) error
	mu       sync.RWMutex
}

// Publish publishes an event to all registered handlers
func (eb *SimpleEventBus) Publish(eventType string, payload interface{}) error {
	eb.mu.RLock()
	handlers, exists := eb.handlers[eventType]
	eb.mu.RUnlock()
	
	if !exists {
		return nil // No handlers registered
	}
	
	for _, handler := range handlers {
		if err := handler(payload); err != nil {
			log.Printf("Event handler error for type %s: %v", eventType, err)
		}
	}
	
	return nil
}

// Subscribe adds a handler for an event type
func (eb *SimpleEventBus) Subscribe(eventType string, handler func(interface{}) error) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	
	if eb.handlers == nil {
		eb.handlers = make(map[string][]func(interface{}) error)
	}
	
	eb.handlers[eventType] = append(eb.handlers[eventType], handler)
}

// Quick setup functions for common scenarios

// SetupForUserService sets up outbox for the user service
func SetupForUserService(db *sql.DB) (*OutboxService, error) {
	config := DefaultOutboxConfig()
	config.ServiceName = "user-service"
	config.ProcessorName = "user-service-outbox"
	config.LoadFromEnvironment()
	
	initializer := NewOutboxInitializer(config, db)
	return initializer.Initialize()
}

// SetupForWorkspaceService sets up outbox for the workspace service
func SetupForWorkspaceService(db *sql.DB) (*OutboxService, error) {
	config := DefaultOutboxConfig()
	config.ServiceName = "workspace-service"
	config.ProcessorName = "workspace-service-outbox"
	config.LoadFromEnvironment()
	
	initializer := NewOutboxInitializer(config, db)
	return initializer.Initialize()
}

// SetupForTenantService sets up outbox for the tenant service
func SetupForTenantService(db *sql.DB) (*OutboxService, error) {
	config := DefaultOutboxConfig()
	config.ServiceName = "tenant-service"
	config.ProcessorName = "tenant-service-outbox"
	config.LoadFromEnvironment()
	
	initializer := NewOutboxInitializer(config, db)
	return initializer.Initialize()
}