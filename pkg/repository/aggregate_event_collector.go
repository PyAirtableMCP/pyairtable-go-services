package repository

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pyairtable-compose/go-services/domain/shared"
)

// AggregateEventCollector automatically collects and manages domain events from aggregates
type AggregateEventCollector struct {
	mu                sync.RWMutex
	eventHandlers     map[string][]EventHandler
	interceptors      []EventInterceptor
	eventFilters      []EventFilter
	eventTransformers []EventTransformer
	metrics           *CollectorMetrics
}

// EventHandler processes collected events
type EventHandler interface {
	HandleEvent(event shared.DomainEvent, aggregate shared.AggregateRoot) error
	GetHandlerName() string
	CanHandle(eventType string) bool
}

// EventInterceptor can modify or block events before they are processed
type EventInterceptor interface {
	InterceptEvent(event shared.DomainEvent, aggregate shared.AggregateRoot) (shared.DomainEvent, bool, error)
	GetInterceptorName() string
}

// EventFilter determines if an event should be processed
type EventFilter interface {
	ShouldProcess(event shared.DomainEvent, aggregate shared.AggregateRoot) bool
	GetFilterName() string
}

// EventTransformer can transform events before they are stored
type EventTransformer interface {
	TransformEvent(event shared.DomainEvent, aggregate shared.AggregateRoot) (shared.DomainEvent, error)
	GetTransformerName() string
}

// CollectorMetrics tracks event collection metrics
type CollectorMetrics struct {
	mu                    sync.RWMutex
	TotalEventsCollected  int64
	EventsProcessed       int64
	EventsFiltered        int64
	EventsTransformed     int64
	EventsIntercepted     int64
	ProcessingTime        time.Duration
	LastCollectionTime    time.Time
	EventTypeBreakdown    map[string]int64
}

// NewAggregateEventCollector creates a new aggregate event collector
func NewAggregateEventCollector() *AggregateEventCollector {
	return &AggregateEventCollector{
		eventHandlers:     make(map[string][]EventHandler),
		interceptors:      make([]EventInterceptor, 0),
		eventFilters:      make([]EventFilter, 0),
		eventTransformers: make([]EventTransformer, 0),
		metrics: &CollectorMetrics{
			EventTypeBreakdown: make(map[string]int64),
			LastCollectionTime: time.Now(),
		},
	}
}

// RegisterEventHandler registers an event handler for specific event types
func (collector *AggregateEventCollector) RegisterEventHandler(eventType string, handler EventHandler) {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	
	if collector.eventHandlers[eventType] == nil {
		collector.eventHandlers[eventType] = make([]EventHandler, 0)
	}
	
	collector.eventHandlers[eventType] = append(collector.eventHandlers[eventType], handler)
	log.Printf("Registered event handler '%s' for event type '%s'", 
		handler.GetHandlerName(), eventType)
}

// RegisterEventInterceptor registers an event interceptor
func (collector *AggregateEventCollector) RegisterEventInterceptor(interceptor EventInterceptor) {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	
	collector.interceptors = append(collector.interceptors, interceptor)
	log.Printf("Registered event interceptor: %s", interceptor.GetInterceptorName())
}

// RegisterEventFilter registers an event filter
func (collector *AggregateEventCollector) RegisterEventFilter(filter EventFilter) {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	
	collector.eventFilters = append(collector.eventFilters, filter)
	log.Printf("Registered event filter: %s", filter.GetFilterName())
}

// RegisterEventTransformer registers an event transformer
func (collector *AggregateEventCollector) RegisterEventTransformer(transformer EventTransformer) {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	
	collector.eventTransformers = append(collector.eventTransformers, transformer)
	log.Printf("Registered event transformer: %s", transformer.GetTransformerName())
}

// CollectEventsFromAggregates collects and processes events from multiple aggregates
func (collector *AggregateEventCollector) CollectEventsFromAggregates(aggregates []shared.AggregateRoot) ([]shared.DomainEvent, error) {
	startTime := time.Now()
	defer func() {
		collector.metrics.mu.Lock()
		collector.metrics.ProcessingTime += time.Since(startTime)
		collector.metrics.LastCollectionTime = time.Now()
		collector.metrics.mu.Unlock()
	}()
	
	var allProcessedEvents []shared.DomainEvent
	
	for _, aggregate := range aggregates {
		events, err := collector.CollectEventsFromAggregate(aggregate)
		if err != nil {
			return nil, fmt.Errorf("failed to collect events from aggregate %s: %w", 
				aggregate.GetID().String(), err)
		}
		
		allProcessedEvents = append(allProcessedEvents, events...)
	}
	
	log.Printf("Collected %d events from %d aggregates", len(allProcessedEvents), len(aggregates))
	return allProcessedEvents, nil
}

// CollectEventsFromAggregate collects and processes events from a single aggregate
func (collector *AggregateEventCollector) CollectEventsFromAggregate(aggregate shared.AggregateRoot) ([]shared.DomainEvent, error) {
	events := aggregate.GetUncommittedEvents()
	if len(events) == 0 {
		return []shared.DomainEvent{}, nil
	}
	
	var processedEvents []shared.DomainEvent
	
	for _, event := range events {
		// Update metrics
		collector.updateMetrics(event)
		
		// Apply filters
		if !collector.shouldProcessEvent(event, aggregate) {
			collector.metrics.mu.Lock()
			collector.metrics.EventsFiltered++
			collector.metrics.mu.Unlock()
			continue
		}
		
		// Apply interceptors
		processedEvent, shouldContinue, err := collector.applyInterceptors(event, aggregate)
		if err != nil {
			return nil, fmt.Errorf("event interceptor failed: %w", err)
		}
		
		if !shouldContinue {
			collector.metrics.mu.Lock()
			collector.metrics.EventsIntercepted++
			collector.metrics.mu.Unlock()
			continue
		}
		
		// Apply transformers
		transformedEvent, err := collector.applyTransformers(processedEvent, aggregate)
		if err != nil {
			return nil, fmt.Errorf("event transformer failed: %w", err)
		}
		
		if transformedEvent != processedEvent {
			collector.metrics.mu.Lock()
			collector.metrics.EventsTransformed++
			collector.metrics.mu.Unlock()
		}
		
		// Handle the event
		if err := collector.handleEvent(transformedEvent, aggregate); err != nil {
			return nil, fmt.Errorf("event handler failed: %w", err)
		}
		
		processedEvents = append(processedEvents, transformedEvent)
		
		collector.metrics.mu.Lock()
		collector.metrics.EventsProcessed++
		collector.metrics.mu.Unlock()
	}
	
	return processedEvents, nil
}

// shouldProcessEvent determines if an event should be processed based on filters
func (collector *AggregateEventCollector) shouldProcessEvent(event shared.DomainEvent, aggregate shared.AggregateRoot) bool {
	collector.mu.RLock()
	defer collector.mu.RUnlock()
	
	for _, filter := range collector.eventFilters {
		if !filter.ShouldProcess(event, aggregate) {
			log.Printf("Event filtered out by %s: %s", filter.GetFilterName(), event.EventType())
			return false
		}
	}
	
	return true
}

// applyInterceptors applies all registered interceptors to an event
func (collector *AggregateEventCollector) applyInterceptors(event shared.DomainEvent, aggregate shared.AggregateRoot) (shared.DomainEvent, bool, error) {
	collector.mu.RLock()
	defer collector.mu.RUnlock()
	
	currentEvent := event
	
	for _, interceptor := range collector.interceptors {
		processedEvent, shouldContinue, err := interceptor.InterceptEvent(currentEvent, aggregate)
		if err != nil {
			return nil, false, fmt.Errorf("interceptor %s failed: %w", 
				interceptor.GetInterceptorName(), err)
		}
		
		if !shouldContinue {
			log.Printf("Event intercepted and blocked by %s: %s", 
				interceptor.GetInterceptorName(), event.EventType())
			return currentEvent, false, nil
		}
		
		currentEvent = processedEvent
	}
	
	return currentEvent, true, nil
}

// applyTransformers applies all registered transformers to an event
func (collector *AggregateEventCollector) applyTransformers(event shared.DomainEvent, aggregate shared.AggregateRoot) (shared.DomainEvent, error) {
	collector.mu.RLock()
	defer collector.mu.RUnlock()
	
	currentEvent := event
	
	for _, transformer := range collector.eventTransformers {
		transformedEvent, err := transformer.TransformEvent(currentEvent, aggregate)
		if err != nil {
			return nil, fmt.Errorf("transformer %s failed: %w", 
				transformer.GetTransformerName(), err)
		}
		
		currentEvent = transformedEvent
	}
	
	return currentEvent, nil
}

// handleEvent processes an event with all registered handlers
func (collector *AggregateEventCollector) handleEvent(event shared.DomainEvent, aggregate shared.AggregateRoot) error {
	collector.mu.RLock()
	handlers := collector.eventHandlers[event.EventType()]
	collector.mu.RUnlock()
	
	for _, handler := range handlers {
		if handler.CanHandle(event.EventType()) {
			if err := handler.HandleEvent(event, aggregate); err != nil {
				return fmt.Errorf("handler %s failed: %w", handler.GetHandlerName(), err)
			}
		}
	}
	
	return nil
}

// updateMetrics updates collection metrics
func (collector *AggregateEventCollector) updateMetrics(event shared.DomainEvent) {
	collector.metrics.mu.Lock()
	defer collector.metrics.mu.Unlock()
	
	collector.metrics.TotalEventsCollected++
	collector.metrics.EventTypeBreakdown[event.EventType()]++
}

// GetMetrics returns current collection metrics
func (collector *AggregateEventCollector) GetMetrics() CollectorMetrics {
	collector.metrics.mu.RLock()
	defer collector.metrics.mu.RUnlock()
	
	// Return a copy to prevent external modification
	metrics := CollectorMetrics{
		TotalEventsCollected: collector.metrics.TotalEventsCollected,
		EventsProcessed:      collector.metrics.EventsProcessed,
		EventsFiltered:       collector.metrics.EventsFiltered,
		EventsTransformed:    collector.metrics.EventsTransformed,
		EventsIntercepted:    collector.metrics.EventsIntercepted,
		ProcessingTime:       collector.metrics.ProcessingTime,
		LastCollectionTime:   collector.metrics.LastCollectionTime,
		EventTypeBreakdown:   make(map[string]int64),
	}
	
	// Copy the breakdown map
	for eventType, count := range collector.metrics.EventTypeBreakdown {
		metrics.EventTypeBreakdown[eventType] = count
	}
	
	return metrics
}

// LogMetrics logs current collection metrics
func (collector *AggregateEventCollector) LogMetrics() {
	metrics := collector.GetMetrics()
	
	log.Printf("Event Collector Metrics:")
	log.Printf("  Total Events Collected: %d", metrics.TotalEventsCollected)
	log.Printf("  Events Processed: %d", metrics.EventsProcessed)
	log.Printf("  Events Filtered: %d", metrics.EventsFiltered)
	log.Printf("  Events Transformed: %d", metrics.EventsTransformed)
	log.Printf("  Events Intercepted: %d", metrics.EventsIntercepted)
	log.Printf("  Total Processing Time: %v", metrics.ProcessingTime)
	log.Printf("  Last Collection: %v", metrics.LastCollectionTime)
	
	if len(metrics.EventTypeBreakdown) > 0 {
		log.Printf("  Event Type Breakdown:")
		for eventType, count := range metrics.EventTypeBreakdown {
			log.Printf("    %s: %d", eventType, count)
		}
	}
}

// DefaultEventHandlers and implementations

// LoggingEventHandler logs all events
type LoggingEventHandler struct {
	logLevel string
}

// NewLoggingEventHandler creates a new logging event handler
func NewLoggingEventHandler(logLevel string) *LoggingEventHandler {
	return &LoggingEventHandler{logLevel: logLevel}
}

// HandleEvent logs the event
func (h *LoggingEventHandler) HandleEvent(event shared.DomainEvent, aggregate shared.AggregateRoot) error {
	log.Printf("[%s] Event: %s, Aggregate: %s, Version: %d", 
		h.logLevel, event.EventType(), aggregate.GetID().String(), event.Version())
	return nil
}

// GetHandlerName returns the handler name
func (h *LoggingEventHandler) GetHandlerName() string {
	return "LoggingEventHandler"
}

// CanHandle returns true for all event types
func (h *LoggingEventHandler) CanHandle(eventType string) bool {
	return true
}

// AuditEventHandler creates audit entries for events
type AuditEventHandler struct {
	auditLogger func(event shared.DomainEvent, aggregate shared.AggregateRoot)
}

// NewAuditEventHandler creates a new audit event handler
func NewAuditEventHandler(auditLogger func(event shared.DomainEvent, aggregate shared.AggregateRoot)) *AuditEventHandler {
	return &AuditEventHandler{auditLogger: auditLogger}
}

// HandleEvent creates an audit entry
func (h *AuditEventHandler) HandleEvent(event shared.DomainEvent, aggregate shared.AggregateRoot) error {
	if h.auditLogger != nil {
		h.auditLogger(event, aggregate)
	}
	return nil
}

// GetHandlerName returns the handler name
func (h *AuditEventHandler) GetHandlerName() string {
	return "AuditEventHandler"
}

// CanHandle returns true for all event types
func (h *AuditEventHandler) CanHandle(eventType string) bool {
	return true
}

// TenantIsolationFilter ensures events belong to the correct tenant
type TenantIsolationFilter struct {
	allowedTenantID string
}

// NewTenantIsolationFilter creates a new tenant isolation filter
func NewTenantIsolationFilter(tenantID string) *TenantIsolationFilter {
	return &TenantIsolationFilter{allowedTenantID: tenantID}
}

// ShouldProcess checks if the event belongs to the allowed tenant
func (f *TenantIsolationFilter) ShouldProcess(event shared.DomainEvent, aggregate shared.AggregateRoot) bool {
	return event.TenantID() == f.allowedTenantID
}

// GetFilterName returns the filter name
func (f *TenantIsolationFilter) GetFilterName() string {
	return "TenantIsolationFilter"
}

// EventTypeFilter filters events by type
type EventTypeFilter struct {
	allowedTypes map[string]bool
}

// NewEventTypeFilter creates a new event type filter
func NewEventTypeFilter(allowedTypes []string) *EventTypeFilter {
	typeMap := make(map[string]bool)
	for _, eventType := range allowedTypes {
		typeMap[eventType] = true
	}
	
	return &EventTypeFilter{allowedTypes: typeMap}
}

// ShouldProcess checks if the event type is allowed
func (f *EventTypeFilter) ShouldProcess(event shared.DomainEvent, aggregate shared.AggregateRoot) bool {
	return f.allowedTypes[event.EventType()]
}

// GetFilterName returns the filter name
func (f *EventTypeFilter) GetFilterName() string {
	return "EventTypeFilter"
}

// MetadataEnrichmentTransformer adds metadata to events
type MetadataEnrichmentTransformer struct {
	enrichmentFunc func(event shared.DomainEvent, aggregate shared.AggregateRoot) map[string]interface{}
}

// NewMetadataEnrichmentTransformer creates a new metadata enrichment transformer
func NewMetadataEnrichmentTransformer(enrichmentFunc func(event shared.DomainEvent, aggregate shared.AggregateRoot) map[string]interface{}) *MetadataEnrichmentTransformer {
	return &MetadataEnrichmentTransformer{enrichmentFunc: enrichmentFunc}
}

// TransformEvent adds enriched metadata to the event
func (t *MetadataEnrichmentTransformer) TransformEvent(event shared.DomainEvent, aggregate shared.AggregateRoot) (shared.DomainEvent, error) {
	if t.enrichmentFunc == nil {
		return event, nil
	}
	
	enrichedMetadata := t.enrichmentFunc(event, aggregate)
	
	// Create a new event with enriched metadata
	// This is a simplified implementation - in practice, you'd want to properly clone the event
	enrichedEvent := &shared.BaseDomainEvent{
		EventIDValue:       event.EventID(),
		EventTypeValue:     event.EventType(),
		AggregateIDValue:   event.AggregateID(),
		AggregateTypeValue: event.AggregateType(),
		VersionValue:       event.Version(),
		OccurredAtValue:    event.OccurredAt(),
		TenantIDValue:      event.TenantID(),
		PayloadValue:       event.Payload().(map[string]interface{}),
	}
	
	// Add enriched metadata to payload
	if enrichedEvent.PayloadValue == nil {
		enrichedEvent.PayloadValue = make(map[string]interface{})
	}
	
	if enrichedEvent.PayloadValue["metadata"] == nil {
		enrichedEvent.PayloadValue["metadata"] = make(map[string]interface{})
	}
	
	metadata := enrichedEvent.PayloadValue["metadata"].(map[string]interface{})
	for key, value := range enrichedMetadata {
		metadata[key] = value
	}
	
	return enrichedEvent, nil
}

// GetTransformerName returns the transformer name
func (t *MetadataEnrichmentTransformer) GetTransformerName() string {
	return "MetadataEnrichmentTransformer"
}

// SecurityInterceptor validates events for security concerns
type SecurityInterceptor struct {
	securityValidator func(event shared.DomainEvent, aggregate shared.AggregateRoot) error
}

// NewSecurityInterceptor creates a new security interceptor
func NewSecurityInterceptor(validator func(event shared.DomainEvent, aggregate shared.AggregateRoot) error) *SecurityInterceptor {
	return &SecurityInterceptor{securityValidator: validator}
}

// InterceptEvent validates the event for security concerns
func (i *SecurityInterceptor) InterceptEvent(event shared.DomainEvent, aggregate shared.AggregateRoot) (shared.DomainEvent, bool, error) {
	if i.securityValidator != nil {
		if err := i.securityValidator(event, aggregate); err != nil {
			return event, false, fmt.Errorf("security validation failed: %w", err)
		}
	}
	
	return event, true, nil
}

// GetInterceptorName returns the interceptor name
func (i *SecurityInterceptor) GetInterceptorName() string {
	return "SecurityInterceptor"
}