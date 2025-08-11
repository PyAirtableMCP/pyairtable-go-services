package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/pyairtable-compose/go-services/pkg/cache"
)

// EventBusPublisher publishes events to an in-memory event bus
type EventBusPublisher struct {
	eventBus EventBus
	name     string
}

// EventBus interface for publishing events
type EventBus interface {
	Publish(eventType string, payload interface{}) error
}

// NewEventBusPublisher creates a new event bus publisher
func NewEventBusPublisher(eventBus EventBus) *EventBusPublisher {
	return &EventBusPublisher{
		eventBus: eventBus,
		name:     "event_bus_publisher",
	}
}

// Publish publishes an event to the event bus
func (p *EventBusPublisher) Publish(ctx context.Context, entry *OutboxEntry) error {
	// Deserialize payload
	var payload map[string]interface{}
	if err := json.Unmarshal(entry.Payload, &payload); err != nil {
		return fmt.Errorf("failed to deserialize payload: %w", err)
	}
	
	// Publish to event bus
	if err := p.eventBus.Publish(entry.EventType, payload); err != nil {
		return fmt.Errorf("failed to publish to event bus: %w", err)
	}
	
	log.Printf("Published event %s to event bus", entry.ID)
	return nil
}

// GetName returns the publisher name
func (p *EventBusPublisher) GetName() string {
	return p.name
}

// KafkaPublisher publishes events to Apache Kafka
type KafkaPublisher struct {
	brokers []string
	topic   string
	name    string
	// In a real implementation, you'd have a Kafka producer here
}

// NewKafkaPublisher creates a new Kafka publisher
func NewKafkaPublisher(brokers []string, topic string) *KafkaPublisher {
	return &KafkaPublisher{
		brokers: brokers,
		topic:   topic,
		name:    "kafka_publisher",
	}
}

// Publish publishes an event to Kafka
func (p *KafkaPublisher) Publish(ctx context.Context, entry *OutboxEntry) error {
	// Create Kafka message
	message := map[string]interface{}{
		"id":             entry.ID,
		"aggregate_id":   entry.AggregateID,
		"aggregate_type": entry.AggregateType,
		"event_type":     entry.EventType,
		"event_version":  entry.EventVersion,
		"payload":        json.RawMessage(entry.Payload),
		"metadata":       json.RawMessage(entry.Metadata),
		"tenant_id":      entry.TenantID,
		"created_at":     entry.CreatedAt,
	}
	
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to serialize Kafka message: %w", err)
	}
	
	// In a real implementation, publish to Kafka here
	log.Printf("Would publish to Kafka topic %s: %s", p.topic, string(messageBytes))
	
	// Simulate potential failure for testing
	if ctx.Err() != nil {
		return fmt.Errorf("context cancelled during Kafka publish")
	}
	
	return nil
}

// GetName returns the publisher name
func (p *KafkaPublisher) GetName() string {
	return p.name
}

// RedisStreamPublisher publishes events to Redis Streams
type RedisStreamPublisher struct {
	redis  *cache.RedisClient
	stream string
	name   string
}

// NewRedisStreamPublisher creates a new Redis Stream publisher
func NewRedisStreamPublisher(redis *cache.RedisClient, stream string) *RedisStreamPublisher {
	return &RedisStreamPublisher{
		redis:  redis,
		stream: stream,
		name:   "redis_stream_publisher",
	}
}

// Publish publishes an event to Redis Stream
func (p *RedisStreamPublisher) Publish(ctx context.Context, entry *OutboxEntry) error {
	// Create stream entry
	fields := map[string]interface{}{
		"id":             entry.ID,
		"aggregate_id":   entry.AggregateID,
		"aggregate_type": entry.AggregateType,
		"event_type":     entry.EventType,
		"event_version":  entry.EventVersion,
		"payload":        string(entry.Payload),
		"metadata":       string(entry.Metadata),
		"tenant_id":      entry.TenantID,
		"created_at":     entry.CreatedAt.Format(time.RFC3339),
	}
	
	// In a real implementation, use Redis XADD command
	fieldsJson, _ := json.Marshal(fields)
	log.Printf("Would publish to Redis stream %s: %s", p.stream, string(fieldsJson))
	
	return nil
}

// GetName returns the publisher name
func (p *RedisStreamPublisher) GetName() string {
	return p.name
}

// WebhookPublisher publishes events via HTTP webhooks
type WebhookPublisher struct {
	webhookURL string
	headers    map[string]string
	timeout    time.Duration
	name       string
}

// NewWebhookPublisher creates a new webhook publisher
func NewWebhookPublisher(webhookURL string, headers map[string]string, timeout time.Duration) *WebhookPublisher {
	return &WebhookPublisher{
		webhookURL: webhookURL,
		headers:    headers,
		timeout:    timeout,
		name:       "webhook_publisher",
	}
}

// Publish publishes an event via HTTP webhook
func (p *WebhookPublisher) Publish(ctx context.Context, entry *OutboxEntry) error {
	// Create webhook payload
	payload := map[string]interface{}{
		"id":             entry.ID,
		"aggregate_id":   entry.AggregateID,
		"aggregate_type": entry.AggregateType,
		"event_type":     entry.EventType,
		"event_version":  entry.EventVersion,
		"payload":        json.RawMessage(entry.Payload),
		"metadata":       json.RawMessage(entry.Metadata),
		"tenant_id":      entry.TenantID,
		"created_at":     entry.CreatedAt,
	}
	
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize webhook payload: %w", err)
	}
	
	// In a real implementation, make HTTP POST request
	log.Printf("Would send webhook to %s: %s", p.webhookURL, string(payloadBytes))
	
	// Simulate context timeout
	select {
	case <-time.After(100 * time.Millisecond): // Simulate processing time
		return nil
	case <-ctx.Done():
		return fmt.Errorf("webhook request timed out")
	}
}

// GetName returns the publisher name
func (p *WebhookPublisher) GetName() string {
	return p.name
}

// CompositePublisher publishes to multiple publishers with delivery guarantees
type CompositePublisher struct {
	publishers []EventPublisher
	strategy   PublishStrategy
	name       string
}

// PublishStrategy defines how multiple publishers are handled
type PublishStrategy int

const (
	PublishAll     PublishStrategy = iota // Publish to all publishers
	PublishAny                            // Succeed if any publisher succeeds
	PublishPrimary                        // Try primary first, fallback to others
)

// NewCompositePublisher creates a new composite publisher
func NewCompositePublisher(publishers []EventPublisher, strategy PublishStrategy) *CompositePublisher {
	return &CompositePublisher{
		publishers: publishers,
		strategy:   strategy,
		name:       "composite_publisher",
	}
}

// Publish publishes using the configured strategy
func (p *CompositePublisher) Publish(ctx context.Context, entry *OutboxEntry) error {
	switch p.strategy {
	case PublishAll:
		return p.publishToAll(ctx, entry)
	case PublishAny:
		return p.publishToAny(ctx, entry)
	case PublishPrimary:
		return p.publishWithFallback(ctx, entry)
	default:
		return fmt.Errorf("unknown publish strategy: %v", p.strategy)
	}
}

// publishToAll publishes to all publishers (all must succeed)
func (p *CompositePublisher) publishToAll(ctx context.Context, entry *OutboxEntry) error {
	var errors []error
	
	for _, publisher := range p.publishers {
		if err := publisher.Publish(ctx, entry); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", publisher.GetName(), err))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("failed to publish to all publishers: %v", errors)
	}
	
	return nil
}

// publishToAny publishes to any publisher (at least one must succeed)
func (p *CompositePublisher) publishToAny(ctx context.Context, entry *OutboxEntry) error {
	var errors []error
	
	for _, publisher := range p.publishers {
		if err := publisher.Publish(ctx, entry); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", publisher.GetName(), err))
		} else {
			// At least one succeeded
			return nil
		}
	}
	
	return fmt.Errorf("failed to publish to any publisher: %v", errors)
}

// publishWithFallback tries primary publisher first, then fallbacks
func (p *CompositePublisher) publishWithFallback(ctx context.Context, entry *OutboxEntry) error {
	if len(p.publishers) == 0 {
		return fmt.Errorf("no publishers configured")
	}
	
	// Try primary publisher first
	primary := p.publishers[0]
	if err := primary.Publish(ctx, entry); err == nil {
		return nil
	} else {
		log.Printf("Primary publisher %s failed: %v, trying fallbacks", primary.GetName(), err)
	}
	
	// Try fallback publishers
	var errors []error
	for i := 1; i < len(p.publishers); i++ {
		publisher := p.publishers[i]
		if err := publisher.Publish(ctx, entry); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", publisher.GetName(), err))
		} else {
			log.Printf("Fallback publisher %s succeeded", publisher.GetName())
			return nil
		}
	}
	
	return fmt.Errorf("all publishers failed: %v", errors)
}

// GetName returns the publisher name
func (p *CompositePublisher) GetName() string {
	return p.name
}

// RetryablePublisher wraps a publisher with retry logic
type RetryablePublisher struct {
	publisher    EventPublisher
	maxRetries   int
	retryBackoff time.Duration
	name         string
}

// NewRetryablePublisher creates a new retryable publisher
func NewRetryablePublisher(publisher EventPublisher, maxRetries int, retryBackoff time.Duration) *RetryablePublisher {
	return &RetryablePublisher{
		publisher:    publisher,
		maxRetries:   maxRetries,
		retryBackoff: retryBackoff,
		name:         fmt.Sprintf("retryable_%s", publisher.GetName()),
	}
}

// Publish publishes with retry logic
func (p *RetryablePublisher) Publish(ctx context.Context, entry *OutboxEntry) error {
	var lastErr error
	
	for attempt := 0; attempt < p.maxRetries; attempt++ {
		if err := p.publisher.Publish(ctx, entry); err != nil {
			lastErr = err
			if attempt < p.maxRetries-1 {
				log.Printf("Publish attempt %d failed for %s: %v, retrying in %v", 
					attempt+1, entry.ID, err, p.retryBackoff)
				
				select {
				case <-time.After(p.retryBackoff):
					// Continue to next attempt
				case <-ctx.Done():
					return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
				}
			}
		} else {
			// Success
			if attempt > 0 {
				log.Printf("Publish succeeded for %s on attempt %d", entry.ID, attempt+1)
			}
			return nil
		}
	}
	
	return fmt.Errorf("failed after %d attempts: %w", p.maxRetries, lastErr)
}

// GetName returns the publisher name
func (p *RetryablePublisher) GetName() string {
	return p.name
}

// CircuitBreakerPublisher wraps a publisher with circuit breaker pattern
type CircuitBreakerPublisher struct {
	publisher     EventPublisher
	failureCount  int
	maxFailures   int
	resetTimeout  time.Duration
	state         CircuitState
	lastFailTime  time.Time
	name          string
}

// CircuitState represents the state of the circuit breaker
type CircuitState int

const (
	CircuitClosed CircuitState = iota // Normal operation
	CircuitOpen                       // Failing, reject requests
	CircuitHalfOpen                   // Testing if service recovered
)

// NewCircuitBreakerPublisher creates a new circuit breaker publisher
func NewCircuitBreakerPublisher(publisher EventPublisher, maxFailures int, resetTimeout time.Duration) *CircuitBreakerPublisher {
	return &CircuitBreakerPublisher{
		publisher:    publisher,
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        CircuitClosed,
		name:         fmt.Sprintf("circuit_breaker_%s", publisher.GetName()),
	}
}

// Publish publishes with circuit breaker protection
func (p *CircuitBreakerPublisher) Publish(ctx context.Context, entry *OutboxEntry) error {
	switch p.state {
	case CircuitOpen:
		// Check if it's time to try again
		if time.Since(p.lastFailTime) > p.resetTimeout {
			p.state = CircuitHalfOpen
		} else {
			return fmt.Errorf("circuit breaker is open for publisher %s", p.publisher.GetName())
		}
	case CircuitHalfOpen:
		// Try once to see if service is back
		if err := p.publisher.Publish(ctx, entry); err != nil {
			p.recordFailure()
			return fmt.Errorf("circuit breaker test failed: %w", err)
		} else {
			p.recordSuccess()
			return nil
		}
	case CircuitClosed:
		// Normal operation
		if err := p.publisher.Publish(ctx, entry); err != nil {
			p.recordFailure()
			return err
		} else {
			p.recordSuccess()
			return nil
		}
	}
	
	return nil
}

// recordFailure records a failure and potentially opens the circuit
func (p *CircuitBreakerPublisher) recordFailure() {
	p.failureCount++
	p.lastFailTime = time.Now()
	
	if p.failureCount >= p.maxFailures {
		p.state = CircuitOpen
		log.Printf("Circuit breaker opened for publisher %s after %d failures", 
			p.publisher.GetName(), p.failureCount)
	}
}

// recordSuccess records a success and potentially closes the circuit
func (p *CircuitBreakerPublisher) recordSuccess() {
	p.failureCount = 0
	if p.state == CircuitHalfOpen {
		p.state = CircuitClosed
		log.Printf("Circuit breaker closed for publisher %s", p.publisher.GetName())
	}
}

// GetName returns the publisher name
func (p *CircuitBreakerPublisher) GetName() string {
	return p.name
}

// GetState returns the current circuit breaker state
func (p *CircuitBreakerPublisher) GetState() CircuitState {
	return p.state
}