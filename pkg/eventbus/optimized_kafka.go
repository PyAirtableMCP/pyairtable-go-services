package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"pyairtable-go/pkg/config"
)

// OptimizedKafkaEventBus provides high-performance Kafka event processing
type OptimizedKafkaEventBus struct {
	producer         sarama.AsyncProducer
	consumerGroup    sarama.ConsumerGroup
	config           *KafkaConfig
	handlers         map[string][]EventHandler
	batchProcessor   *BatchProcessor
	compressionCodec *CompressionCodec
	metrics          *KafkaMetrics
	mu               sync.RWMutex
	ctx              context.Context
	cancel           context.CancelFunc
}

// KafkaConfig contains optimized Kafka configuration
type KafkaConfig struct {
	Brokers               []string
	ClientID              string
	ConsumerGroup         string
	BatchSize             int
	BatchTimeout          time.Duration
	CompressionType       sarama.CompressionCodec
	RequiredAcks          sarama.RequiredAcks
	RetryMax              int
	RetryBackoff          time.Duration
	FlushFrequency        time.Duration
	FlushBytes            int
	FlushMessages         int
	ChannelBufferSize     int
	NetMaxOpenRequests    int
	NetDialTimeout        time.Duration
	NetReadTimeout        time.Duration
	NetWriteTimeout       time.Duration
	EnableIdempotence     bool
	MaxMessageBytes       int
	FetchMin              int32
	FetchDefault          int32
	FetchMax              int32
	SessionTimeout        time.Duration
	HeartbeatInterval     time.Duration
	RebalanceTimeout      time.Duration
	ProcessingTimeout     time.Duration
	ParallelConsumers     int
	DeadLetterTopic       string
	RetryTopic            string
	EnableDeadLetterQueue bool
	EnableMetrics         bool
}

// BatchProcessor handles event batching for improved throughput
type BatchProcessor struct {
	events    []Event
	mu        sync.Mutex
	batchSize int
	timeout   time.Duration
	processor func([]Event) error
	ticker    *time.Ticker
	done      chan bool
}

// CompressionCodec provides advanced compression for events
type CompressionCodec struct {
	codec sarama.CompressionCodec
}

// KafkaMetrics contains Prometheus metrics for Kafka operations
type KafkaMetrics struct {
	messagesProduced     *prometheus.CounterVec
	messagesConsumed     *prometheus.CounterVec
	messageSize          *prometheus.HistogramVec
	processingDuration   *prometheus.HistogramVec
	batchSize           *prometheus.HistogramVec
	producerErrors      prometheus.Counter
	consumerErrors      prometheus.Counter
	deadLetterMessages  prometheus.Counter
	retryMessages       prometheus.Counter
	compressionRatio    *prometheus.GaugeVec
}

// NewOptimizedKafkaEventBus creates a new optimized Kafka event bus
func NewOptimizedKafkaEventBus(cfg *config.Config, kafkaConfig *KafkaConfig) (*OptimizedKafkaEventBus, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Set default configuration if not provided
	if kafkaConfig == nil {
		kafkaConfig = &KafkaConfig{
			Brokers:               []string{"localhost:9092"},
			ClientID:              "pyairtable-producer",
			ConsumerGroup:         "pyairtable-consumers",
			BatchSize:             1000,
			BatchTimeout:          100 * time.Millisecond,
			CompressionType:       sarama.CompressionLZ4,
			RequiredAcks:          sarama.WaitForLocal,
			RetryMax:              3,
			RetryBackoff:          100 * time.Millisecond,
			FlushFrequency:        10 * time.Millisecond,
			FlushBytes:            1024 * 1024, // 1MB
			FlushMessages:         1000,
			ChannelBufferSize:     1000,
			NetMaxOpenRequests:    5,
			NetDialTimeout:        10 * time.Second,
			NetReadTimeout:        10 * time.Second,
			NetWriteTimeout:       10 * time.Second,
			EnableIdempotence:     true,
			MaxMessageBytes:       1024 * 1024, // 1MB
			FetchMin:              1024,         // 1KB
			FetchDefault:          1024 * 1024,  // 1MB
			FetchMax:              10 * 1024 * 1024, // 10MB
			SessionTimeout:        30 * time.Second,
			HeartbeatInterval:     3 * time.Second,
			RebalanceTimeout:      60 * time.Second,
			ProcessingTimeout:     5 * time.Second,
			ParallelConsumers:     5,
			DeadLetterTopic:       "dead-letter-queue",
			RetryTopic:           "retry-queue",
			EnableDeadLetterQueue: true,
			EnableMetrics:        true,
		}
	}

	// Create producer
	producer, err := createOptimizedProducer(kafkaConfig)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	// Create consumer group
	consumerGroup, err := createOptimizedConsumerGroup(kafkaConfig)
	if err != nil {
		producer.Close()
		cancel()
		return nil, fmt.Errorf("failed to create consumer group: %w", err)
	}

	// Initialize metrics
	var metrics *KafkaMetrics
	if kafkaConfig.EnableMetrics {
		metrics = NewKafkaMetrics()
	}

	// Create batch processor
	batchProcessor := NewBatchProcessor(kafkaConfig.BatchSize, kafkaConfig.BatchTimeout)

	eventBus := &OptimizedKafkaEventBus{
		producer:      producer,
		consumerGroup: consumerGroup,
		config:        kafkaConfig,
		handlers:      make(map[string][]EventHandler),
		batchProcessor: batchProcessor,
		compressionCodec: &CompressionCodec{codec: kafkaConfig.CompressionType},
		metrics:       metrics,
		ctx:           ctx,
		cancel:        cancel,
	}

	// Start error handling goroutines
	go eventBus.handleProducerErrors()
	go eventBus.handleProducerSuccesses()

	return eventBus, nil
}

// createOptimizedProducer creates a high-performance Kafka producer
func createOptimizedProducer(cfg *KafkaConfig) (sarama.AsyncProducer, error) {
	config := sarama.NewConfig()
	
	// Producer Performance Settings
	config.Producer.RequiredAcks = cfg.RequiredAcks
	config.Producer.Compression = cfg.CompressionType
	config.Producer.Flush.Frequency = cfg.FlushFrequency
	config.Producer.Flush.Bytes = cfg.FlushBytes
	config.Producer.Flush.Messages = cfg.FlushMessages
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true
	config.Producer.Retry.Max = cfg.RetryMax
	config.Producer.Retry.Backoff = cfg.RetryBackoff
	config.Producer.MaxMessageBytes = cfg.MaxMessageBytes
	config.Producer.Idempotent = cfg.EnableIdempotence
	
	// Network Settings
	config.Net.MaxOpenRequests = cfg.NetMaxOpenRequests
	config.Net.DialTimeout = cfg.NetDialTimeout
	config.Net.ReadTimeout = cfg.NetReadTimeout
	config.Net.WriteTimeout = cfg.NetWriteTimeout
	
	// Channel Settings
	config.ChannelBufferSize = cfg.ChannelBufferSize
	
	// Client ID
	config.ClientID = cfg.ClientID
	
	// Partitioner - use hash partitioner for even distribution
	config.Producer.Partitioner = sarama.NewHashPartitioner
	
	return sarama.NewAsyncProducer(cfg.Brokers, config)
}

// createOptimizedConsumerGroup creates a high-performance Kafka consumer group
func createOptimizedConsumerGroup(cfg *KafkaConfig) (sarama.ConsumerGroup, error) {
	config := sarama.NewConfig()
	
	// Consumer Performance Settings
	config.Consumer.Fetch.Min = cfg.FetchMin
	config.Consumer.Fetch.Default = cfg.FetchDefault
	config.Consumer.Fetch.Max = cfg.FetchMax
	config.Consumer.Group.Session.Timeout = cfg.SessionTimeout
	config.Consumer.Group.Heartbeat.Interval = cfg.HeartbeatInterval
	config.Consumer.Group.Rebalance.Timeout = cfg.RebalanceTimeout
	config.Consumer.Return.Errors = true
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	
	// Enable auto-commit with optimized interval
	config.Consumer.Offsets.AutoCommit.Enable = true
	config.Consumer.Offsets.AutoCommit.Interval = 1 * time.Second
	
	// Network Settings
	config.Net.DialTimeout = cfg.NetDialTimeout
	config.Net.ReadTimeout = cfg.NetReadTimeout
	config.Net.WriteTimeout = cfg.NetWriteTimeout
	
	// Client ID
	config.ClientID = cfg.ClientID
	
	return sarama.NewConsumerGroup(cfg.Brokers, cfg.ConsumerGroup, config)
}

// PublishBatch publishes multiple events as a batch for better throughput
func (k *OptimizedKafkaEventBus) PublishBatch(events []Event) error {
	if len(events) == 0 {
		return nil
	}

	start := time.Now()
	var publishedCount int
	
	for _, event := range events {
		// Serialize event
		data, err := json.Marshal(event)
		if err != nil {
			log.Printf("Failed to serialize event: %v", err)
			continue
		}

		// Create producer message
		msg := &sarama.ProducerMessage{
			Topic: event.Topic,
			Key:   sarama.StringEncoder(event.AggregateID),
			Value: sarama.ByteEncoder(data),
			Headers: []sarama.RecordHeader{
				{Key: []byte("event-type"), Value: []byte(event.Type)},
				{Key: []byte("event-version"), Value: []byte(event.Version)},
				{Key: []byte("correlation-id"), Value: []byte(event.CorrelationID)},
				{Key: []byte("timestamp"), Value: []byte(event.Timestamp.Format(time.RFC3339))},
			},
		}

		// Send message asynchronously
		select {
		case k.producer.Input() <- msg:
			publishedCount++
		case <-k.ctx.Done():
			return fmt.Errorf("context cancelled")
		}

		// Record metrics
		if k.metrics != nil {
			k.metrics.messagesProduced.WithLabelValues(event.Topic, event.Type).Inc()
			k.metrics.messageSize.WithLabelValues(event.Topic).Observe(float64(len(data)))
		}
	}

	duration := time.Since(start)
	log.Printf("Published batch of %d events in %v", publishedCount, duration)

	// Record batch metrics
	if k.metrics != nil {
		k.metrics.batchSize.WithLabelValues("publish").Observe(float64(publishedCount))
		k.metrics.processingDuration.WithLabelValues("batch_publish", "").Observe(duration.Seconds())
	}

	return nil
}

// ConsumeWithParallelProcessing starts consuming events with parallel processing
func (k *OptimizedKafkaEventBus) ConsumeWithParallelProcessing(topics []string) error {
	consumer := &optimizedConsumer{
		eventBus: k,
		ready:    make(chan bool),
	}

	// Start multiple consumer goroutines for parallel processing
	for i := 0; i < k.config.ParallelConsumers; i++ {
		go func(consumerID int) {
			for {
				select {
				case <-k.ctx.Done():
					return
				default:
					if err := k.consumerGroup.Consume(k.ctx, topics, consumer); err != nil {
						log.Printf("Consumer %d error: %v", consumerID, err)
						time.Sleep(1 * time.Second)
					}
				}
			}
		}(i)
	}

	<-consumer.ready
	log.Printf("Kafka consumer group started with %d parallel consumers", k.config.ParallelConsumers)
	return nil
}

// optimizedConsumer implements sarama.ConsumerGroupHandler with optimizations
type optimizedConsumer struct {
	eventBus *OptimizedKafkaEventBus
	ready    chan bool
}

func (c *optimizedConsumer) Setup(sarama.ConsumerGroupSession) error {
	close(c.ready)
	return nil
}

func (c *optimizedConsumer) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

func (c *optimizedConsumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	batch := make([]*sarama.ConsumerMessage, 0, c.eventBus.config.BatchSize)
	batchTimer := time.NewTimer(c.eventBus.config.BatchTimeout)
	
	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}

			batch = append(batch, message)
			
			// Process batch when full or timeout reached
			if len(batch) >= c.eventBus.config.BatchSize {
				c.processBatch(session, batch)
				batch = batch[:0] // Reset slice
				batchTimer.Reset(c.eventBus.config.BatchTimeout)
			}

		case <-batchTimer.C:
			if len(batch) > 0 {
				c.processBatch(session, batch)
				batch = batch[:0] // Reset slice
			}
			batchTimer.Reset(c.eventBus.config.BatchTimeout)

		case <-session.Context().Done():
			if len(batch) > 0 {
				c.processBatch(session, batch)
			}
			return nil
		}
	}
}

// processBatch processes a batch of messages
func (c *optimizedConsumer) processBatch(session sarama.ConsumerGroupSession, messages []*sarama.ConsumerMessage) {
	start := time.Now()
	processedCount := 0
	errorCount := 0

	for _, message := range messages {
		if err := c.processMessage(message); err != nil {
			log.Printf("Failed to process message: %v", err)
			errorCount++
			
			// Send to dead letter queue if configured
			if c.eventBus.config.EnableDeadLetterQueue {
				c.sendToDeadLetterQueue(message, err)
			}
		} else {
			processedCount++
		}

		// Mark message as processed
		session.MarkMessage(message, "")
	}

	duration := time.Since(start)
	log.Printf("Processed batch: %d successful, %d errors, duration: %v", 
		processedCount, errorCount, duration)

	// Record metrics
	if c.eventBus.metrics != nil {
		c.eventBus.metrics.batchSize.WithLabelValues("consume").Observe(float64(len(messages)))
		c.eventBus.metrics.processingDuration.WithLabelValues("batch_consume", "").Observe(duration.Seconds())
	}
}

// processMessage processes a single message
func (c *optimizedConsumer) processMessage(message *sarama.ConsumerMessage) error {
	start := time.Now()
	
	// Deserialize event
	var event Event
	if err := json.Unmarshal(message.Value, &event); err != nil {
		return fmt.Errorf("failed to deserialize event: %w", err)
	}

	// Get handlers for this event type
	c.eventBus.mu.RLock()
	handlers, exists := c.eventBus.handlers[event.Type]
	c.eventBus.mu.RUnlock()

	if !exists {
		log.Printf("No handlers found for event type: %s", event.Type)
		return nil
	}

	// Process with timeout
	ctx, cancel := context.WithTimeout(context.Background(), c.eventBus.config.ProcessingTimeout)
	defer cancel()

	// Execute handlers concurrently
	var wg sync.WaitGroup
	errChan := make(chan error, len(handlers))

	for _, handler := range handlers {
		wg.Add(1)
		go func(h EventHandler) {
			defer wg.Done()
			if err := h.Handle(ctx, event); err != nil {
				errChan <- err
			}
		}(handler)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	duration := time.Since(start)

	// Record metrics
	if c.eventBus.metrics != nil {
		status := "success"
		if len(errors) > 0 {
			status = "error"
		}
		c.eventBus.metrics.messagesConsumed.WithLabelValues(message.Topic, event.Type).Inc()
		c.eventBus.metrics.processingDuration.WithLabelValues("message_process", status).Observe(duration.Seconds())
	}

	if len(errors) > 0 {
		return fmt.Errorf("handler errors: %v", errors)
	}

	return nil
}

// sendToDeadLetterQueue sends failed messages to dead letter queue
func (c *optimizedConsumer) sendToDeadLetterQueue(message *sarama.ConsumerMessage, processingError error) {
	dlqMessage := &sarama.ProducerMessage{
		Topic: c.eventBus.config.DeadLetterTopic,
		Key:   message.Key,
		Value: message.Value,
		Headers: append(message.Headers, sarama.RecordHeader{
			Key:   []byte("processing-error"),
			Value: []byte(processingError.Error()),
		}, sarama.RecordHeader{
			Key:   []byte("original-topic"),
			Value: []byte(message.Topic),
		}, sarama.RecordHeader{
			Key:   []byte("failed-at"),
			Value: []byte(time.Now().Format(time.RFC3339)),
		}),
	}

	select {
	case c.eventBus.producer.Input() <- dlqMessage:
		if c.eventBus.metrics != nil {
			c.eventBus.metrics.deadLetterMessages.Inc()
		}
		log.Printf("Sent message to dead letter queue: %s", processingError.Error())
	default:
		log.Printf("Failed to send message to dead letter queue: producer channel full")
	}
}

// handleProducerErrors handles producer errors
func (k *OptimizedKafkaEventBus) handleProducerErrors() {
	for err := range k.producer.Errors() {
		log.Printf("Producer error: %v", err)
		if k.metrics != nil {
			k.metrics.producerErrors.Inc()
		}
	}
}

// handleProducerSuccesses handles producer successes for metrics
func (k *OptimizedKafkaEventBus) handleProducerSuccesses() {
	for range k.producer.Successes() {
		// Just drain the channel - metrics are recorded during publish
	}
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(batchSize int, timeout time.Duration) *BatchProcessor {
	bp := &BatchProcessor{
		events:    make([]Event, 0, batchSize),
		batchSize: batchSize,
		timeout:   timeout,
		ticker:    time.NewTicker(timeout),
		done:      make(chan bool),
	}

	go bp.processBatches()
	return bp
}

// processBatches processes batches periodically
func (bp *BatchProcessor) processBatches() {
	for {
		select {
		case <-bp.ticker.C:
			bp.mu.Lock()
			if len(bp.events) > 0 && bp.processor != nil {
				if err := bp.processor(bp.events); err != nil {
					log.Printf("Batch processing error: %v", err)
				}
				bp.events = bp.events[:0] // Reset slice
			}
			bp.mu.Unlock()
		case <-bp.done:
			bp.ticker.Stop()
			return
		}
	}
}

// NewKafkaMetrics creates a new KafkaMetrics instance
func NewKafkaMetrics() *KafkaMetrics {
	return &KafkaMetrics{
		messagesProduced: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "kafka_messages_produced_total",
				Help: "Total number of messages produced to Kafka",
			},
			[]string{"topic", "event_type"},
		),
		messagesConsumed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "kafka_messages_consumed_total",
				Help: "Total number of messages consumed from Kafka",
			},
			[]string{"topic", "event_type"},
		),
		messageSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "kafka_message_size_bytes",
				Help:    "Size of Kafka messages in bytes",
				Buckets: prometheus.ExponentialBuckets(64, 2, 16),
			},
			[]string{"topic"},
		),
		processingDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "kafka_processing_duration_seconds",
				Help:    "Duration of Kafka message processing",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 15),
			},
			[]string{"operation", "status"},
		),
		batchSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "kafka_batch_size",
				Help:    "Size of message batches",
				Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000},
			},
			[]string{"operation"},
		),
		producerErrors: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "kafka_producer_errors_total",
				Help: "Total number of Kafka producer errors",
			},
		),
		consumerErrors: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "kafka_consumer_errors_total",
				Help: "Total number of Kafka consumer errors",
			},
		),
		deadLetterMessages: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "kafka_dead_letter_messages_total",
				Help: "Total number of messages sent to dead letter queue",
			},
		),
		retryMessages: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "kafka_retry_messages_total",
				Help: "Total number of messages sent to retry queue",
			},
		),
		compressionRatio: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "kafka_compression_ratio",
				Help: "Compression ratio for Kafka messages",
			},
			[]string{"topic", "codec"},
		),
	}
}

// Close gracefully shuts down the Kafka event bus
func (k *OptimizedKafkaEventBus) Close() error {
	log.Println("Shutting down optimized Kafka event bus...")
	
	k.cancel()
	
	if err := k.producer.Close(); err != nil {
		log.Printf("Error closing producer: %v", err)
	}
	
	if err := k.consumerGroup.Close(); err != nil {
		log.Printf("Error closing consumer group: %v", err)
	}
	
	k.batchProcessor.done <- true
	
	log.Println("Optimized Kafka event bus shut down complete")
	return nil
}