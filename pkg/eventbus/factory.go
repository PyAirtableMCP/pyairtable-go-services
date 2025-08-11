package eventbus

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"go.uber.org/zap"
)

// EventBusType represents the type of event bus implementation
type EventBusType string

const (
	EventBusTypeInMemory EventBusType = "inmemory"
	EventBusTypeKafka    EventBusType = "kafka"
)

// EventBus interface that both implementations must satisfy
type EventBus interface {
	Publish(event shared.DomainEvent) error
	Subscribe(eventType string, handler shared.EventHandler) error
	Unsubscribe(eventType string, handler shared.EventHandler) error
	GetHandlerCount(eventType string) int
	GetRegisteredEventTypes() []string
}

// EventBusConfig holds configuration for creating event bus instances
type EventBusConfig struct {
	Type               EventBusType
	KafkaBrokers       []string
	KafkaTopicPrefix   string
	KafkaConsumerGroup string
	KafkaSecurityConfig *SecurityConfig
	Logger             *zap.Logger
}

// NewEventBusFromEnvironment creates an event bus based on environment variables
func NewEventBusFromEnvironment(logger *zap.Logger) (EventBus, error) {
	config, err := LoadEventBusConfigFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to load event bus config from environment: %w", err)
	}
	
	config.Logger = logger
	return NewEventBus(config)
}

// NewEventBus creates an event bus based on the provided configuration
func NewEventBus(config *EventBusConfig) (EventBus, error) {
	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}

	switch config.Type {
	case EventBusTypeInMemory:
		return NewInMemoryEventBus(config.Logger), nil
		
	case EventBusTypeKafka:
		if len(config.KafkaBrokers) == 0 {
			return nil, fmt.Errorf("kafka brokers must be specified for kafka event bus")
		}
		
		kafkaConfig := DefaultKafkaConfig(
			config.KafkaBrokers,
			config.KafkaTopicPrefix,
			config.KafkaConsumerGroup,
		)
		
		if config.KafkaSecurityConfig != nil {
			kafkaConfig.SecurityConfig = config.KafkaSecurityConfig
		}
		
		kafkaEventBus, err := NewKafkaEventBus(kafkaConfig, config.Logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create kafka event bus: %w", err)
		}
		
		// Start the Kafka event bus
		if err := kafkaEventBus.Start(); err != nil {
			return nil, fmt.Errorf("failed to start kafka event bus: %w", err)
		}
		
		return kafkaEventBus, nil
		
	default:
		return nil, fmt.Errorf("unsupported event bus type: %s", config.Type)
	}
}

// LoadEventBusConfigFromEnv loads event bus configuration from environment variables
func LoadEventBusConfigFromEnv() (*EventBusConfig, error) {
	config := &EventBusConfig{}
	
	// Event bus type
	eventBusType := getEnvOrDefault("EVENT_BUS_TYPE", "inmemory")
	switch strings.ToLower(eventBusType) {
	case "inmemory", "in-memory":
		config.Type = EventBusTypeInMemory
	case "kafka":
		config.Type = EventBusTypeKafka
	default:
		return nil, fmt.Errorf("unsupported event bus type: %s", eventBusType)
	}
	
	// Kafka configuration
	if config.Type == EventBusTypeKafka {
		// Kafka brokers
		brokersStr := getEnvOrDefault("KAFKA_BROKERS", "localhost:9092")
		config.KafkaBrokers = strings.Split(brokersStr, ",")
		for i, broker := range config.KafkaBrokers {
			config.KafkaBrokers[i] = strings.TrimSpace(broker)
		}
		
		// Topic prefix
		config.KafkaTopicPrefix = getEnvOrDefault("KAFKA_TOPIC_PREFIX", "pyairtable")
		
		// Consumer group
		config.KafkaConsumerGroup = getEnvOrDefault("KAFKA_CONSUMER_GROUP", "pyairtable-services")
		
		// Security configuration
		securityConfig, err := loadKafkaSecurityConfigFromEnv()
		if err != nil {
			return nil, fmt.Errorf("failed to load kafka security config: %w", err)
		}
		config.KafkaSecurityConfig = securityConfig
	}
	
	return config, nil
}

// loadKafkaSecurityConfigFromEnv loads Kafka security configuration from environment variables
func loadKafkaSecurityConfigFromEnv() (*SecurityConfig, error) {
	protocol := getEnvOrDefault("KAFKA_SECURITY_PROTOCOL", "PLAINTEXT")
	
	securityConfig := &SecurityConfig{
		Protocol: protocol,
	}
	
	// SASL configuration
	if protocol == "SASL_PLAINTEXT" || protocol == "SASL_SSL" {
		mechanism := getEnvOrDefault("KAFKA_SASL_MECHANISM", "PLAIN")
		username := os.Getenv("KAFKA_SASL_USERNAME")
		password := os.Getenv("KAFKA_SASL_PASSWORD")
		
		if username == "" || password == "" {
			return nil, fmt.Errorf("KAFKA_SASL_USERNAME and KAFKA_SASL_PASSWORD must be set for SASL authentication")
		}
		
		securityConfig.SASLConfig = &SASLConfig{
			Mechanism: mechanism,
			Username:  username,
			Password:  password,
		}
	}
	
	// TLS configuration
	if protocol == "SSL" || protocol == "SASL_SSL" {
		insecureSkipVerify, _ := strconv.ParseBool(getEnvOrDefault("KAFKA_TLS_INSECURE_SKIP_VERIFY", "false"))
		
		securityConfig.TLSConfig = &TLSConfig{
			Enabled:            true,
			InsecureSkipVerify: insecureSkipVerify,
			CertFile:           os.Getenv("KAFKA_TLS_CERT_FILE"),
			KeyFile:            os.Getenv("KAFKA_TLS_KEY_FILE"),
			CAFile:             os.Getenv("KAFKA_TLS_CA_FILE"),
		}
	}
	
	return securityConfig, nil
}

// getEnvOrDefault returns the value of an environment variable or a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// EventBusManager manages the lifecycle of event bus instances
type EventBusManager struct {
	eventBus EventBus
	logger   *zap.Logger
}

// NewEventBusManager creates a new event bus manager
func NewEventBusManager(logger *zap.Logger) *EventBusManager {
	return &EventBusManager{
		logger: logger,
	}
}

// Initialize initializes the event bus from environment configuration
func (m *EventBusManager) Initialize() error {
	eventBus, err := NewEventBusFromEnvironment(m.logger)
	if err != nil {
		return fmt.Errorf("failed to initialize event bus: %w", err)
	}
	
	m.eventBus = eventBus
	m.logger.Info("Event bus initialized successfully")
	return nil
}

// GetEventBus returns the event bus instance
func (m *EventBusManager) GetEventBus() EventBus {
	return m.eventBus
}

// Shutdown gracefully shuts down the event bus
func (m *EventBusManager) Shutdown() error {
	if m.eventBus == nil {
		return nil
	}
	
	// If it's a Kafka event bus, stop it
	if kafkaEventBus, ok := m.eventBus.(*KafkaEventBus); ok {
		return kafkaEventBus.Stop()
	}
	
	return nil
}

// DefaultEventBusManager provides a global event bus manager instance
var DefaultEventBusManager = NewEventBusManager(zap.NewNop())

// InitializeDefaultEventBus initializes the default global event bus
func InitializeDefaultEventBus(logger *zap.Logger) error {
	DefaultEventBusManager.logger = logger
	return DefaultEventBusManager.Initialize()
}

// GetDefaultEventBus returns the default global event bus
func GetDefaultEventBus() EventBus {
	return DefaultEventBusManager.GetEventBus()
}

// ShutdownDefaultEventBus shuts down the default global event bus
func ShutdownDefaultEventBus() error {
	return DefaultEventBusManager.Shutdown()
}