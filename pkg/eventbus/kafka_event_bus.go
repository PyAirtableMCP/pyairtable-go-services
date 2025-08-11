package eventbus

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/pyairtable-compose/go-services/domain/shared"
	"go.uber.org/zap"
)

// KafkaEventBus implements the EventBus interface using Apache Kafka
// This provides reliable, scalable event distribution for production deployments
type KafkaEventBus struct {
	config          *KafkaConfig
	producer        sarama.AsyncProducer
	consumers       map[string]sarama.ConsumerGroup
	handlers        map[string][]shared.EventHandler
	logger          *zap.Logger
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	mu              sync.RWMutex
	started         bool
	topicPrefix     string
	consumerGroup   string
	partitioner     sarama.PartitionerConstructor
	deadLetterTopic string
}

// Ensure KafkaEventBus implements EventBus interface
var _ EventBus = (*KafkaEventBus)(nil)

// KafkaConfig holds configuration for Kafka event bus
type KafkaConfig struct {
	Brokers         []string
	TopicPrefix     string
	ConsumerGroup   string
	SecurityConfig  *SecurityConfig
	ProducerConfig  *ProducerConfig
	ConsumerConfig  *ConsumerConfig
	DeadLetterTopic string
	RetryAttempts   int
	RetryDelay      time.Duration
}

// SecurityConfig holds security configuration for Kafka
type SecurityConfig struct {
	Protocol   string // PLAINTEXT, SSL, SASL_PLAINTEXT, SASL_SSL
	SASLConfig *SASLConfig
	TLSConfig  *TLSConfig
}

// SASLConfig holds SASL authentication configuration
type SASLConfig struct {
	Mechanism string // PLAIN, SCRAM-SHA-256, SCRAM-SHA-512, GSSAPI, AWS_MSK_IAM
	Username  string
	Password  string
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled               bool
	InsecureSkipVerify    bool
	CertFile              string
	KeyFile               string
	CAFile                string
}

// ProducerConfig holds producer-specific configuration
type ProducerConfig struct {
	Acks                   sarama.RequiredAcks
	Retry                  *sarama.Config
	CompressionType        sarama.CompressionCodec
	MaxMessageBytes        int
	IdempotentWrites       bool
	BatchSize              int
	LingerMS               time.Duration
	FlushFrequency         time.Duration
	EnableTransactions     bool
}

// ConsumerConfig holds consumer-specific configuration
type ConsumerConfig struct {
	AutoOffsetReset    int64 // sarama.OffsetNewest or sarama.OffsetOldest
	SessionTimeout     time.Duration
	HeartbeatInterval  time.Duration
	RebalanceStrategy  string
	IsolationLevel     sarama.IsolationLevel
	MaxProcessingTime  time.Duration
	FetchMin           int32
	FetchMax           int32
}

// DefaultKafkaConfig returns a production-ready Kafka configuration
func DefaultKafkaConfig(brokers []string, topicPrefix, consumerGroup string) *KafkaConfig {
	return &KafkaConfig{
		Brokers:       brokers,
		TopicPrefix:   topicPrefix,
		ConsumerGroup: consumerGroup,
		SecurityConfig: &SecurityConfig{
			Protocol: "PLAINTEXT", // Change to SASL_SSL for production
		},
		ProducerConfig: &ProducerConfig{
			Acks:                sarama.WaitForAll, // Wait for all replicas
			CompressionType:     sarama.CompressionSnappy,
			MaxMessageBytes:     1000000, // 1MB
			IdempotentWrites:    true,
			BatchSize:           16384,
			LingerMS:            5 * time.Millisecond,
			FlushFrequency:      100 * time.Millisecond,
			EnableTransactions:  false,
		},
		ConsumerConfig: &ConsumerConfig{
			AutoOffsetReset:   sarama.OffsetOldest,
			SessionTimeout:    30 * time.Second,
			HeartbeatInterval: 3 * time.Second,
			RebalanceStrategy: "roundrobin",
			IsolationLevel:    sarama.ReadCommitted,
			MaxProcessingTime: 5 * time.Minute,
			FetchMin:          1,
			FetchMax:          1024 * 1024, // 1MB
		},
		DeadLetterTopic: "pyairtable.dlq.events",
		RetryAttempts:   3,
		RetryDelay:      1 * time.Second,
	}
}

// NewKafkaEventBus creates a new Kafka event bus
func NewKafkaEventBus(config *KafkaConfig, logger *zap.Logger) (*KafkaEventBus, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &KafkaEventBus{
		config:          config,
		consumers:       make(map[string]sarama.ConsumerGroup),
		handlers:        make(map[string][]shared.EventHandler),
		logger:          logger,
		ctx:             ctx,
		cancel:          cancel,
		topicPrefix:     config.TopicPrefix,
		consumerGroup:   config.ConsumerGroup,
		partitioner:     sarama.NewHashPartitioner,
		deadLetterTopic: config.DeadLetterTopic,
	}, nil
}

// Start initializes the Kafka producer and begins consuming
func (kb *KafkaEventBus) Start() error {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	if kb.started {
		return fmt.Errorf("kafka event bus already started")
	}

	// Create Kafka config
	saramaConfig := sarama.NewConfig()
	
	// Apply security configuration
	if err := kb.applySecurity(saramaConfig); err != nil {
		return fmt.Errorf("failed to apply security config: %w", err)
	}
	
	// Apply producer configuration
	kb.applyProducerConfig(saramaConfig)
	
	// Apply consumer configuration
	kb.applyConsumerConfig(saramaConfig)

	// Create producer
	producer, err := sarama.NewAsyncProducer(kb.config.Brokers, saramaConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	kb.producer = producer
	
	// Start producer error handling
	kb.wg.Add(1)
	go kb.handleProducerErrors()

	// Start producer success handling
	kb.wg.Add(1)
	go kb.handleProducerSuccesses()

	kb.started = true

	kb.logger.Info("Kafka event bus started successfully",
		zap.Strings("brokers", kb.config.Brokers),
		zap.String("topic_prefix", kb.topicPrefix),
		zap.String("consumer_group", kb.consumerGroup),
	)

	return nil
}

// Stop gracefully shuts down the Kafka event bus
func (kb *KafkaEventBus) Stop() error {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	if !kb.started {
		return nil
	}

	kb.logger.Info("Stopping Kafka event bus...")

	// Cancel context to stop all goroutines
	kb.cancel()

	// Close producer
	if kb.producer != nil {
		if err := kb.producer.Close(); err != nil {
			kb.logger.Error("Failed to close Kafka producer", zap.Error(err))
		}
	}

	// Close all consumers
	for topic, consumer := range kb.consumers {
		if err := consumer.Close(); err != nil {
			kb.logger.Error("Failed to close consumer", 
				zap.String("topic", topic), 
				zap.Error(err))
		}
	}

	// Wait for all goroutines to finish
	kb.wg.Wait()

	kb.started = false
	
	kb.logger.Info("Kafka event bus stopped successfully")
	return nil
}

// Publish publishes an event to Kafka
func (kb *KafkaEventBus) Publish(event shared.DomainEvent) error {
	if !kb.started {
		return fmt.Errorf("kafka event bus not started")
	}

	// Determine topic name
	topic := kb.getTopicName(event.EventType())

	// Serialize event
	eventData, err := kb.serializeEvent(event)
	if err != nil {
		return fmt.Errorf("failed to serialize event: %w", err)
	}

	// Create Kafka message
	message := &sarama.ProducerMessage{
		Topic:     topic,
		Key:       sarama.StringEncoder(event.AggregateID()), // Use aggregate ID as partition key
		Value:     sarama.ByteEncoder(eventData),
		Timestamp: event.Timestamp(),
		Headers: []sarama.RecordHeader{
			{
				Key:   []byte("event_type"),
				Value: []byte(event.EventType()),
			},
			{
				Key:   []byte("event_id"),
				Value: []byte(event.EventID()),
			},
			{
				Key:   []byte("aggregate_type"),
				Value: []byte(event.AggregateType()),
			},
			{
				Key:   []byte("tenant_id"),
				Value: []byte(event.TenantID()),
			},
		},
	}

	// Send message asynchronously
	select {
	case kb.producer.Input() <- message:
		kb.logger.Debug("Event sent to Kafka",
			zap.String("event_type", event.EventType()),
			zap.String("event_id", event.EventID()),
			zap.String("topic", topic),
			zap.String("aggregate_id", event.AggregateID()),
		)
		return nil
	case <-kb.ctx.Done():
		return fmt.Errorf("kafka event bus is shutting down")
	default:
		return fmt.Errorf("producer input channel is full")
	}
}

// Subscribe registers an event handler for specific event types
func (kb *KafkaEventBus) Subscribe(eventType string, handler shared.EventHandler) error {
	if !handler.CanHandle(eventType) {
		return fmt.Errorf("handler cannot handle event type: %s", eventType)
	}

	kb.mu.Lock()
	defer kb.mu.Unlock()

	// Add handler to map
	if kb.handlers[eventType] == nil {
		kb.handlers[eventType] = make([]shared.EventHandler, 0)
	}
	kb.handlers[eventType] = append(kb.handlers[eventType], handler)

	// Start consumer for this event type if not already started
	topic := kb.getTopicName(eventType)
	if _, exists := kb.consumers[topic]; !exists && kb.started {
		if err := kb.startConsumer(topic); err != nil {
			return fmt.Errorf("failed to start consumer for topic %s: %w", topic, err)
		}
	}

	kb.logger.Info("Event handler subscribed",
		zap.String("event_type", eventType),
		zap.String("topic", topic),
		zap.Int("total_handlers", len(kb.handlers[eventType])),
	)

	return nil
}

// Unsubscribe removes an event handler
func (kb *KafkaEventBus) Unsubscribe(eventType string, handler shared.EventHandler) error {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	handlers, exists := kb.handlers[eventType]
	if !exists {
		return fmt.Errorf("no handlers found for event type: %s", eventType)
	}

	// Find and remove the handler
	for i, h := range handlers {
		if h == handler {
			kb.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
			
			kb.logger.Info("Event handler unsubscribed",
				zap.String("event_type", eventType),
				zap.Int("remaining_handlers", len(kb.handlers[eventType])),
			)
			
			return nil
		}
	}

	return fmt.Errorf("handler not found for event type: %s", eventType)
}

// GetHandlerCount returns the number of handlers for an event type
func (kb *KafkaEventBus) GetHandlerCount(eventType string) int {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	
	if handlers, exists := kb.handlers[eventType]; exists {
		return len(handlers)
	}
	return 0
}

// GetRegisteredEventTypes returns all event types that have handlers
func (kb *KafkaEventBus) GetRegisteredEventTypes() []string {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	
	types := make([]string, 0, len(kb.handlers))
	for eventType := range kb.handlers {
		if len(kb.handlers[eventType]) > 0 {
			types = append(types, eventType)
		}
	}
	return types
}

// startConsumer starts a consumer for a specific topic
func (kb *KafkaEventBus) startConsumer(topic string) error {
	saramaConfig := sarama.NewConfig()
	
	// Apply security configuration
	if err := kb.applySecurity(saramaConfig); err != nil {
		return fmt.Errorf("failed to apply security config: %w", err)
	}
	
	// Apply consumer configuration
	kb.applyConsumerConfig(saramaConfig)

	consumer, err := sarama.NewConsumerGroup(kb.config.Brokers, kb.consumerGroup, saramaConfig)
	if err != nil {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	kb.consumers[topic] = consumer

	// Start consumer goroutine
	kb.wg.Add(1)
	go kb.runConsumer(topic, consumer)

	kb.logger.Info("Started Kafka consumer",
		zap.String("topic", topic),
		zap.String("consumer_group", kb.consumerGroup),
	)

	return nil
}

// runConsumer runs the consumer loop for a specific topic
func (kb *KafkaEventBus) runConsumer(topic string, consumer sarama.ConsumerGroup) {
	defer kb.wg.Done()

	handler := &consumerGroupHandler{
		eventBus: kb,
		topic:    topic,
		logger:   kb.logger,
	}

	for {
		select {
		case <-kb.ctx.Done():
			kb.logger.Info("Consumer stopping", zap.String("topic", topic))
			return
		default:
			if err := consumer.Consume(kb.ctx, []string{topic}, handler); err != nil {
				if err == sarama.ErrClosedConsumerGroup {
					return
				}
				kb.logger.Error("Consumer error",
					zap.String("topic", topic),
					zap.Error(err),
				)
				time.Sleep(kb.config.RetryDelay)
			}
		}
	}
}

// getTopicName generates the full topic name with prefix
func (kb *KafkaEventBus) getTopicName(eventType string) string {
	if kb.topicPrefix == "" {
		return eventType
	}
	
	// Convert event type to topic name format
	// e.g., "user.registered" -> "pyairtable.user.registered"
	parts := strings.Split(eventType, ".")
	if len(parts) >= 2 {
		domain := parts[0]
		event := strings.Join(parts[1:], ".")
		return fmt.Sprintf("%s.%s.%s", kb.topicPrefix, domain, event)
	}
	
	return fmt.Sprintf("%s.%s", kb.topicPrefix, eventType)
}

// serializeEvent serializes an event to JSON
func (kb *KafkaEventBus) serializeEvent(event shared.DomainEvent) ([]byte, error) {
	eventData := map[string]interface{}{
		"event_id":       event.EventID(),
		"event_type":     event.EventType(),
		"aggregate_id":   event.AggregateID(),
		"aggregate_type": event.AggregateType(),
		"version":        event.Version(),
		"timestamp":      event.Timestamp(),
		"tenant_id":      event.TenantID(),
		"payload":        event.Payload(),
		"metadata":       event.Metadata(),
	}

	return json.Marshal(eventData)
}

// deserializeEvent deserializes an event from JSON
func (kb *KafkaEventBus) deserializeEvent(data []byte) (map[string]interface{}, error) {
	var eventData map[string]interface{}
	if err := json.Unmarshal(data, &eventData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event data: %w", err)
	}
	return eventData, nil
}

// handleEvent processes an event with all registered handlers
func (kb *KafkaEventBus) handleEvent(eventData map[string]interface{}) error {
	eventType, ok := eventData["event_type"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid event_type")
	}

	kb.mu.RLock()
	handlers, exists := kb.handlers[eventType]
	kb.mu.RUnlock()

	if !exists || len(handlers) == 0 {
		kb.logger.Debug("No handlers registered for event type",
			zap.String("event_type", eventType),
		)
		return nil
	}

	// Create a domain event from the data
	domainEvent := &eventFromMap{data: eventData}

	// Process handlers
	var errors []error
	for _, handler := range handlers {
		if err := kb.handleEventSafely(handler, domainEvent); err != nil {
			errors = append(errors, err)
			kb.logger.Error("Event handler failed",
				zap.String("event_type", eventType),
				zap.String("event_id", domainEvent.EventID()),
				zap.Error(err),
			)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("some handlers failed: %d errors", len(errors))
	}

	return nil
}

// handleEventSafely executes an event handler with error recovery
func (kb *KafkaEventBus) handleEventSafely(handler shared.EventHandler, event shared.DomainEvent) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("event handler panicked: %v", r)
			kb.logger.Error("Event handler panic recovered",
				zap.String("event_type", event.EventType()),
				zap.String("event_id", event.EventID()),
				zap.Any("panic", r),
			)
		}
	}()

	return handler.Handle(event)
}

// handleProducerErrors handles producer errors
func (kb *KafkaEventBus) handleProducerErrors() {
	defer kb.wg.Done()

	for {
		select {
		case err := <-kb.producer.Errors():
			if err != nil {
				kb.logger.Error("Kafka producer error",
					zap.String("topic", err.Msg.Topic),
					zap.Any("key", err.Msg.Key),
					zap.Error(err.Err),
				)
				
				// Optionally send to dead letter queue
				go kb.sendToDeadLetterQueue(err.Msg, err.Err)
			}
		case <-kb.ctx.Done():
			return
		}
	}
}

// handleProducerSuccesses handles producer successes
func (kb *KafkaEventBus) handleProducerSuccesses() {
	defer kb.wg.Done()

	for {
		select {
		case msg := <-kb.producer.Successes():
			if msg != nil {
				kb.logger.Debug("Event published successfully",
					zap.String("topic", msg.Topic),
					zap.Int32("partition", msg.Partition),
					zap.Int64("offset", msg.Offset),
				)
			}
		case <-kb.ctx.Done():
			return
		}
	}
}

// sendToDeadLetterQueue sends failed messages to the dead letter queue
func (kb *KafkaEventBus) sendToDeadLetterQueue(originalMsg *sarama.ProducerMessage, err error) {
	if kb.deadLetterTopic == "" {
		return
	}

	// Create dead letter message with error information
	dlqMessage := &sarama.ProducerMessage{
		Topic:     kb.deadLetterTopic,
		Key:       originalMsg.Key,
		Value:     originalMsg.Value,
		Headers:   originalMsg.Headers,
		Timestamp: time.Now(),
	}

	// Add error information to headers
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
	case kb.producer.Input() <- dlqMessage:
		kb.logger.Info("Message sent to dead letter queue",
			zap.String("original_topic", originalMsg.Topic),
			zap.String("dlq_topic", kb.deadLetterTopic),
		)
	default:
		kb.logger.Error("Failed to send message to dead letter queue - channel full",
			zap.String("original_topic", originalMsg.Topic),
		)
	}
}

// applySecurity applies security configuration to Sarama config
func (kb *KafkaEventBus) applySecurity(config *sarama.Config) error {
	if kb.config.SecurityConfig == nil {
		return nil
	}

	switch kb.config.SecurityConfig.Protocol {
	case "PLAINTEXT":
		// No additional configuration needed
	case "SSL":
		config.Net.TLS.Enable = true
		if kb.config.SecurityConfig.TLSConfig != nil {
			// Apply TLS configuration
			config.Net.TLS.Config = &tls.Config{
				InsecureSkipVerify: kb.config.SecurityConfig.TLSConfig.InsecureSkipVerify,
			}
		}
	case "SASL_PLAINTEXT", "SASL_SSL":
		config.Net.SASL.Enable = true
		if kb.config.SecurityConfig.SASLConfig != nil {
			switch kb.config.SecurityConfig.SASLConfig.Mechanism {
			case "PLAIN":
				config.Net.SASL.Mechanism = sarama.SASLTypePlaintext
				config.Net.SASL.User = kb.config.SecurityConfig.SASLConfig.Username
				config.Net.SASL.Password = kb.config.SecurityConfig.SASLConfig.Password
			case "SCRAM-SHA-256":
				config.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
				config.Net.SASL.User = kb.config.SecurityConfig.SASLConfig.Username
				config.Net.SASL.Password = kb.config.SecurityConfig.SASLConfig.Password
			case "SCRAM-SHA-512":
				config.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
				config.Net.SASL.User = kb.config.SecurityConfig.SASLConfig.Username
				config.Net.SASL.Password = kb.config.SecurityConfig.SASLConfig.Password
			}
		}
		
		if kb.config.SecurityConfig.Protocol == "SASL_SSL" {
			config.Net.TLS.Enable = true
			if kb.config.SecurityConfig.TLSConfig != nil {
				config.Net.TLS.Config = &tls.Config{
					InsecureSkipVerify: kb.config.SecurityConfig.TLSConfig.InsecureSkipVerify,
				}
			}
		}
	default:
		return fmt.Errorf("unsupported security protocol: %s", kb.config.SecurityConfig.Protocol)
	}

	return nil
}

// applyProducerConfig applies producer configuration to Sarama config
func (kb *KafkaEventBus) applyProducerConfig(config *sarama.Config) {
	if kb.config.ProducerConfig == nil {
		return
	}

	pc := kb.config.ProducerConfig
	
	config.Producer.RequiredAcks = pc.Acks
	config.Producer.Compression = pc.CompressionType
	config.Producer.MaxMessageBytes = pc.MaxMessageBytes
	config.Producer.Idempotent = pc.IdempotentWrites
	config.Producer.Flush.Frequency = pc.FlushFrequency
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true
	
	// Partitioner
	config.Producer.Partitioner = kb.partitioner
	
	// Retry configuration
	config.Producer.Retry.Max = kb.config.RetryAttempts
	config.Producer.Retry.Backoff = kb.config.RetryDelay
	
	// Transaction configuration
	if pc.EnableTransactions {
		config.Producer.Transaction.Retry.Max = kb.config.RetryAttempts
		config.Producer.Transaction.Retry.Backoff = kb.config.RetryDelay
	}
}

// applyConsumerConfig applies consumer configuration to Sarama config
func (kb *KafkaEventBus) applyConsumerConfig(config *sarama.Config) {
	if kb.config.ConsumerConfig == nil {
		return
	}

	cc := kb.config.ConsumerConfig
	
	config.Consumer.Offsets.Initial = cc.AutoOffsetReset
	config.Consumer.Group.Session.Timeout = cc.SessionTimeout
	config.Consumer.Group.Heartbeat.Interval = cc.HeartbeatInterval
	config.Consumer.IsolationLevel = cc.IsolationLevel
	config.Consumer.MaxProcessingTime = cc.MaxProcessingTime
	config.Consumer.Fetch.Min = cc.FetchMin
	config.Consumer.Fetch.Max = cc.FetchMax
	
	// Rebalance strategy
	switch cc.RebalanceStrategy {
	case "range":
		config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRange
	case "roundrobin":
		config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	case "sticky":
		config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategySticky
	default:
		config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	}
	
	config.Consumer.Return.Errors = true
}

// consumerGroupHandler implements sarama.ConsumerGroupHandler
type consumerGroupHandler struct {
	eventBus *KafkaEventBus
	topic    string
	logger   *zap.Logger
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (h *consumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	h.logger.Info("Consumer group session setup", zap.String("topic", h.topic))
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (h *consumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	h.logger.Info("Consumer group session cleanup", zap.String("topic", h.topic))
	return nil
}

// ConsumeClaim starts a consumer loop of ConsumerGroupClaim's Messages()
func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}
			
			h.logger.Debug("Received message",
				zap.String("topic", message.Topic),
				zap.Int32("partition", message.Partition),
				zap.Int64("offset", message.Offset),
			)
			
			// Deserialize event
			eventData, err := h.eventBus.deserializeEvent(message.Value)
			if err != nil {
				h.logger.Error("Failed to deserialize event",
					zap.String("topic", message.Topic),
					zap.Int64("offset", message.Offset),
					zap.Error(err),
				)
				
				// Mark message as processed to avoid infinite retry
				session.MarkMessage(message, "")
				continue
			}
			
			// Handle event
			if err := h.eventBus.handleEvent(eventData); err != nil {
				h.logger.Error("Failed to handle event",
					zap.String("topic", message.Topic),
					zap.Int64("offset", message.Offset),
					zap.Error(err),
				)
				
				// For now, mark as processed to avoid infinite retry
				// In production, you might want to implement retry logic
				// or send to dead letter queue
				session.MarkMessage(message, "")
				continue
			}
			
			// Mark message as processed
			session.MarkMessage(message, "")
			
		case <-session.Context().Done():
			return nil
		}
	}
}

// eventFromMap implements shared.DomainEvent interface for deserialized events
type eventFromMap struct {
	data map[string]interface{}
}

func (e *eventFromMap) EventID() string {
	if id, ok := e.data["event_id"].(string); ok {
		return id
	}
	return ""
}

func (e *eventFromMap) EventType() string {
	if eventType, ok := e.data["event_type"].(string); ok {
		return eventType
	}
	return ""
}

func (e *eventFromMap) AggregateID() string {
	if id, ok := e.data["aggregate_id"].(string); ok {
		return id
	}
	return ""
}

func (e *eventFromMap) AggregateType() string {
	if aggregateType, ok := e.data["aggregate_type"].(string); ok {
		return aggregateType
	}
	return ""
}

func (e *eventFromMap) Version() int64 {
	if version, ok := e.data["version"].(float64); ok {
		return int64(version)
	}
	return 0
}

func (e *eventFromMap) Timestamp() time.Time {
	if timestamp, ok := e.data["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
			return t
		}
	}
	return time.Now()
}

func (e *eventFromMap) TenantID() string {
	if tenantID, ok := e.data["tenant_id"].(string); ok {
		return tenantID
	}
	return ""
}

func (e *eventFromMap) Payload() map[string]interface{} {
	if payload, ok := e.data["payload"].(map[string]interface{}); ok {
		return payload
	}
	return make(map[string]interface{})
}

func (e *eventFromMap) Metadata() map[string]interface{} {
	if metadata, ok := e.data["metadata"].(map[string]interface{}); ok {
		return metadata
	}
	return make(map[string]interface{})
}