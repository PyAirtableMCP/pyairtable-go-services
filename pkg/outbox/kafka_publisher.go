package outbox

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// KafkaEventPublisher implements EventPublisher interface for Apache Kafka
type KafkaEventPublisher struct {
	producer    sarama.AsyncProducer
	topicPrefix string
	logger      *zap.Logger
	config      *KafkaPublisherConfig
}

// KafkaPublisherConfig holds configuration for Kafka publisher
type KafkaPublisherConfig struct {
	Brokers         []string
	TopicPrefix     string
	SecurityConfig  *KafkaSecurityConfig
	ProducerConfig  *KafkaProducerConfig
	RetryAttempts   int
	RetryDelay      time.Duration
	DeadLetterTopic string
}

// KafkaSecurityConfig holds security configuration for Kafka
type KafkaSecurityConfig struct {
	SecurityProtocol string // PLAINTEXT, SSL, SASL_PLAINTEXT, SASL_SSL
	SASLMechanism    string // PLAIN, SCRAM-SHA-256, SCRAM-SHA-512, AWS_MSK_IAM
	SASLUsername     string
	SASLPassword     string
	TLSConfig        *KafkaTLSConfig
}

// KafkaTLSConfig holds TLS configuration
type KafkaTLSConfig struct {
	Enabled               bool
	InsecureSkipVerify    bool
	CertFile              string
	KeyFile               string
	CAFile                string
}

// KafkaProducerConfig holds producer-specific configuration
type KafkaProducerConfig struct {
	Acks                   sarama.RequiredAcks
	CompressionType        sarama.CompressionCodec
	MaxMessageBytes        int
	IdempotentWrites       bool
	BatchSize              int
	LingerMS               time.Duration
	EnableTransactions     bool
	TransactionID          string
}

// DefaultKafkaPublisherConfig returns a production-ready configuration
func DefaultKafkaPublisherConfig(brokers []string, topicPrefix string) *KafkaPublisherConfig {
	return &KafkaPublisherConfig{
		Brokers:     brokers,
		TopicPrefix: topicPrefix,
		SecurityConfig: &KafkaSecurityConfig{
			SecurityProtocol: "PLAINTEXT", // Change to SASL_SSL for production
		},
		ProducerConfig: &KafkaProducerConfig{
			Acks:                sarama.WaitForAll, // Wait for all replicas
			CompressionType:     sarama.CompressionSnappy,
			MaxMessageBytes:     1000000, // 1MB
			IdempotentWrites:    true,
			BatchSize:           16384,
			LingerMS:            5 * time.Millisecond,
			EnableTransactions:  false, // Set to true for exactly-once semantics
		},
		RetryAttempts:   3,
		RetryDelay:      1 * time.Second,
		DeadLetterTopic: fmt.Sprintf("%s.dlq.events", topicPrefix),
	}
}

// NewKafkaEventPublisher creates a new Kafka event publisher
func NewKafkaEventPublisher(config *KafkaPublisherConfig, logger *zap.Logger) (*KafkaEventPublisher, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Create Sarama configuration
	saramaConfig := sarama.NewConfig()
	
	// Apply security configuration
	if err := applyKafkaSecurityConfig(saramaConfig, config.SecurityConfig); err != nil {
		return nil, fmt.Errorf("failed to apply security config: %w", err)
	}
	
	// Apply producer configuration
	applyKafkaProducerConfig(saramaConfig, config.ProducerConfig)

	// Create producer
	producer, err := sarama.NewAsyncProducer(config.Brokers, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	publisher := &KafkaEventPublisher{
		producer:    producer,
		topicPrefix: config.TopicPrefix,
		logger:      logger,
		config:      config,
	}

	// Start error and success handling goroutines
	go publisher.handleProducerMessages()

	logger.Info("Kafka event publisher created successfully",
		zap.Strings("brokers", config.Brokers),
		zap.String("topic_prefix", config.TopicPrefix),
	)

	return publisher, nil
}

// Publish publishes an outbox entry to Kafka
func (p *KafkaEventPublisher) Publish(ctx context.Context, entry *OutboxEntry) error {
	// Determine topic name
	topic := p.getTopicName(entry.EventType)

	// Create Kafka message
	message := &sarama.ProducerMessage{
		Topic:     topic,
		Key:       sarama.StringEncoder(entry.AggregateID),
		Value:     sarama.ByteEncoder(entry.Payload),
		Timestamp: entry.CreatedAt,
		Headers: []sarama.RecordHeader{
			{
				Key:   []byte("event_type"),
				Value: []byte(entry.EventType),
			},
			{
				Key:   []byte("event_id"),
				Value: []byte(entry.ID),
			},
			{
				Key:   []byte("aggregate_type"),
				Value: []byte(entry.AggregateType),
			},
			{
				Key:   []byte("aggregate_id"),
				Value: []byte(entry.AggregateID),
			},
			{
				Key:   []byte("event_version"),
				Value: []byte(fmt.Sprintf("%d", entry.EventVersion)),
			},
			{
				Key:   []byte("tenant_id"),
				Value: []byte(entry.TenantID),
			},
			{
				Key:   []byte("correlation_id"),
				Value: []byte(p.getCorrelationID(entry.Metadata)),
			},
		},
	}

	// Add metadata as headers
	if err := p.addMetadataHeaders(message, entry.Metadata); err != nil {
		p.logger.Warn("Failed to add metadata headers", zap.Error(err))
	}

	// Send message with context timeout
	select {
	case p.producer.Input() <- message:
		p.logger.Debug("Event sent to Kafka",
			zap.String("event_type", entry.EventType),
			zap.String("event_id", entry.ID),
			zap.String("topic", topic),
			zap.String("aggregate_id", entry.AggregateID),
		)
		return nil
	case <-ctx.Done():
		return fmt.Errorf("context cancelled while sending event: %w", ctx.Err())
	}
}

// GetName returns the publisher name
func (p *KafkaEventPublisher) GetName() string {
	return "kafka"
}

// Close gracefully shuts down the publisher
func (p *KafkaEventPublisher) Close() error {
	if p.producer != nil {
		return p.producer.Close()
	}
	return nil
}

// getTopicName generates the topic name from event type
func (p *KafkaEventPublisher) getTopicName(eventType string) string {
	// Convert event type to topic name format
	// e.g., "user.registered" -> "pyairtable.auth.events"
	parts := strings.Split(eventType, ".")
	if len(parts) >= 2 {
		domain := parts[0]
		return fmt.Sprintf("%s.%s.events", p.topicPrefix, domain)
	}
	return fmt.Sprintf("%s.system.events", p.topicPrefix)
}

// getCorrelationID extracts correlation ID from metadata
func (p *KafkaEventPublisher) getCorrelationID(metadata []byte) string {
	if len(metadata) == 0 {
		return ""
	}

	var meta map[string]interface{}
	if err := json.Unmarshal(metadata, &meta); err != nil {
		return ""
	}

	if correlationID, ok := meta["correlation_id"].(string); ok {
		return correlationID
	}
	return ""
}

// addMetadataHeaders adds metadata as Kafka headers
func (p *KafkaEventPublisher) addMetadataHeaders(message *sarama.ProducerMessage, metadata []byte) error {
	if len(metadata) == 0 {
		return nil
	}

	var meta map[string]interface{}
	if err := json.Unmarshal(metadata, &meta); err != nil {
		return fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	// Add selected metadata as headers
	for key, value := range meta {
		// Skip large values and only include string/number metadata
		if strValue, ok := value.(string); ok && len(strValue) < 1000 {
			message.Headers = append(message.Headers, sarama.RecordHeader{
				Key:   []byte(fmt.Sprintf("meta_%s", key)),
				Value: []byte(strValue),
			})
		}
	}

	return nil
}

// handleProducerMessages handles producer success and error messages
func (p *KafkaEventPublisher) handleProducerMessages() {
	for {
		select {
		case success := <-p.producer.Successes():
			if success != nil {
				p.logger.Debug("Event published successfully",
					zap.String("topic", success.Topic),
					zap.Int32("partition", success.Partition),
					zap.Int64("offset", success.Offset),
				)
			}
		case err := <-p.producer.Errors():
			if err != nil {
				p.logger.Error("Failed to publish event",
					zap.String("topic", err.Msg.Topic),
					zap.Any("key", err.Msg.Key),
					zap.Error(err.Err),
				)

				// Optionally send to dead letter queue
				go p.sendToDeadLetterQueue(err.Msg, err.Err)
			}
		}
	}
}

// sendToDeadLetterQueue sends failed messages to the dead letter queue
func (p *KafkaEventPublisher) sendToDeadLetterQueue(originalMsg *sarama.ProducerMessage, err error) {
	if p.config.DeadLetterTopic == "" {
		return
	}

	// Create dead letter message
	dlqMessage := &sarama.ProducerMessage{
		Topic:     p.config.DeadLetterTopic,
		Key:       originalMsg.Key,
		Value:     originalMsg.Value,
		Headers:   make([]sarama.RecordHeader, len(originalMsg.Headers)),
		Timestamp: time.Now(),
	}

	// Copy original headers
	copy(dlqMessage.Headers, originalMsg.Headers)

	// Add error information
	dlqMessage.Headers = append(dlqMessage.Headers,
		sarama.RecordHeader{
			Key:   []byte("original_topic"),
			Value: []byte(originalMsg.Topic),
		},
		sarama.RecordHeader{
			Key:   []byte("error_message"),
			Value: []byte(err.Error()),
		},
		sarama.RecordHeader{
			Key:   []byte("failed_at"),
			Value: []byte(time.Now().Format(time.RFC3339)),
		},
	)

	// Send to DLQ (best effort)
	select {
	case p.producer.Input() <- dlqMessage:
		p.logger.Info("Message sent to dead letter queue",
			zap.String("original_topic", originalMsg.Topic),
			zap.String("dlq_topic", p.config.DeadLetterTopic),
		)
	default:
		p.logger.Error("Failed to send message to dead letter queue - channel full",
			zap.String("original_topic", originalMsg.Topic),
		)
	}
}

// applyKafkaSecurityConfig applies security configuration to Sarama config
func applyKafkaSecurityConfig(config *sarama.Config, securityConfig *KafkaSecurityConfig) error {
	if securityConfig == nil {
		return nil
	}

	switch securityConfig.SecurityProtocol {
	case "PLAINTEXT":
		// No additional configuration needed
	case "SSL":
		config.Net.TLS.Enable = true
		if securityConfig.TLSConfig != nil {
			// Apply TLS configuration
			config.Net.TLS.Config = &tls.Config{
				InsecureSkipVerify: securityConfig.TLSConfig.InsecureSkipVerify,
			}
		}
	case "SASL_PLAINTEXT", "SASL_SSL":
		config.Net.SASL.Enable = true
		switch securityConfig.SASLMechanism {
		case "PLAIN":
			config.Net.SASL.Mechanism = sarama.SASLTypePlaintext
			config.Net.SASL.User = securityConfig.SASLUsername
			config.Net.SASL.Password = securityConfig.SASLPassword
		case "SCRAM-SHA-256":
			config.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
			config.Net.SASL.User = securityConfig.SASLUsername
			config.Net.SASL.Password = securityConfig.SASLPassword
		case "SCRAM-SHA-512":
			config.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
			config.Net.SASL.User = securityConfig.SASLUsername
			config.Net.SASL.Password = securityConfig.SASLPassword
		}

		if securityConfig.SecurityProtocol == "SASL_SSL" {
			config.Net.TLS.Enable = true
			if securityConfig.TLSConfig != nil {
				config.Net.TLS.Config = &tls.Config{
					InsecureSkipVerify: securityConfig.TLSConfig.InsecureSkipVerify,
				}
			}
		}
	default:
		return fmt.Errorf("unsupported security protocol: %s", securityConfig.SecurityProtocol)
	}

	return nil
}

// applyKafkaProducerConfig applies producer configuration to Sarama config
func applyKafkaProducerConfig(config *sarama.Config, producerConfig *KafkaProducerConfig) {
	if producerConfig == nil {
		return
	}

	config.Producer.RequiredAcks = producerConfig.Acks
	config.Producer.Compression = producerConfig.CompressionType
	config.Producer.MaxMessageBytes = producerConfig.MaxMessageBytes
	config.Producer.Idempotent = producerConfig.IdempotentWrites
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true

	// Partitioner
	config.Producer.Partitioner = sarama.NewHashPartitioner

	// Transaction configuration
	if producerConfig.EnableTransactions {
		config.Producer.Transaction.ID = producerConfig.TransactionID
		config.Net.MaxOpenRequests = 1 // Required for transactions
	}
}

// KafkaEventPublisherFactory creates Kafka event publishers with common configuration
type KafkaEventPublisherFactory struct {
	config *KafkaPublisherConfig
	logger *zap.Logger
}

// NewKafkaEventPublisherFactory creates a new factory
func NewKafkaEventPublisherFactory(config *KafkaPublisherConfig, logger *zap.Logger) *KafkaEventPublisherFactory {
	return &KafkaEventPublisherFactory{
		config: config,
		logger: logger,
	}
}

// CreatePublisher creates a new Kafka event publisher
func (f *KafkaEventPublisherFactory) CreatePublisher() (EventPublisher, error) {
	return NewKafkaEventPublisher(f.config, f.logger)
}

// CreateTransactionalPublisher creates a Kafka publisher with transactions enabled
func (f *KafkaEventPublisherFactory) CreateTransactionalPublisher(transactionID string) (EventPublisher, error) {
	// Create a copy of the config with transactions enabled
	config := *f.config
	if config.ProducerConfig == nil {
		config.ProducerConfig = &KafkaProducerConfig{}
	}
	
	producerConfig := *config.ProducerConfig
	producerConfig.EnableTransactions = true
	producerConfig.TransactionID = transactionID
	config.ProducerConfig = &producerConfig

	return NewKafkaEventPublisher(&config, f.logger)
}