package eventbus

import (
	"fmt"
	"sync"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"go.uber.org/zap"
)

// InMemoryEventBus implements the EventBus interface using in-memory channels
// This is suitable for development and single-instance deployments
type InMemoryEventBus struct {
	handlers map[string][]shared.EventHandler
	logger   *zap.Logger
	mutex    sync.RWMutex
}

// Ensure InMemoryEventBus implements EventBus interface
var _ EventBus = (*InMemoryEventBus)(nil)

// NewInMemoryEventBus creates a new in-memory event bus
func NewInMemoryEventBus(logger *zap.Logger) *InMemoryEventBus {
	return &InMemoryEventBus{
		handlers: make(map[string][]shared.EventHandler),
		logger:   logger,
	}
}

// Publish publishes an event to all registered handlers
func (bus *InMemoryEventBus) Publish(event shared.DomainEvent) error {
	bus.mutex.RLock()
	handlers, exists := bus.handlers[event.EventType()]
	bus.mutex.RUnlock()

	if !exists {
		bus.logger.Debug("No handlers registered for event type", 
			zap.String("event_type", event.EventType()),
			zap.String("event_id", event.EventID()),
		)
		return nil
	}

	bus.logger.Info("Publishing event", 
		zap.String("event_type", event.EventType()),
		zap.String("event_id", event.EventID()),
		zap.String("aggregate_id", event.AggregateID()),
		zap.Int("handler_count", len(handlers)),
	)

	// Process handlers synchronously for now
	// In production, you might want to process them asynchronously
	for _, handler := range handlers {
		if err := bus.handleEvent(handler, event); err != nil {
			bus.logger.Error("Event handler failed", 
				zap.String("event_type", event.EventType()),
				zap.String("event_id", event.EventID()),
				zap.Error(err),
			)
			// Continue processing other handlers even if one fails
		}
	}

	return nil
}

// Subscribe registers an event handler for specific event types
func (bus *InMemoryEventBus) Subscribe(eventType string, handler shared.EventHandler) error {
	if !handler.CanHandle(eventType) {
		return fmt.Errorf("handler cannot handle event type: %s", eventType)
	}

	bus.mutex.Lock()
	defer bus.mutex.Unlock()

	if bus.handlers[eventType] == nil {
		bus.handlers[eventType] = make([]shared.EventHandler, 0)
	}

	bus.handlers[eventType] = append(bus.handlers[eventType], handler)

	bus.logger.Info("Event handler subscribed", 
		zap.String("event_type", eventType),
		zap.Int("total_handlers", len(bus.handlers[eventType])),
	)

	return nil
}

// Unsubscribe removes an event handler
func (bus *InMemoryEventBus) Unsubscribe(eventType string, handler shared.EventHandler) error {
	bus.mutex.Lock()
	defer bus.mutex.Unlock()

	handlers, exists := bus.handlers[eventType]
	if !exists {
		return fmt.Errorf("no handlers found for event type: %s", eventType)
	}

	// Find and remove the handler
	for i, h := range handlers {
		if h == handler {
			// Remove handler from slice
			bus.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
			
			bus.logger.Info("Event handler unsubscribed", 
				zap.String("event_type", eventType),
				zap.Int("remaining_handlers", len(bus.handlers[eventType])),
			)
			
			return nil
		}
	}

	return fmt.Errorf("handler not found for event type: %s", eventType)
}

// handleEvent safely executes an event handler with error recovery
func (bus *InMemoryEventBus) handleEvent(handler shared.EventHandler, event shared.DomainEvent) (err error) {
	// Recover from panics in event handlers
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("event handler panicked: %v", r)
			bus.logger.Error("Event handler panic recovered", 
				zap.String("event_type", event.EventType()),
				zap.String("event_id", event.EventID()),
				zap.Any("panic", r),
			)
		}
	}()

	return handler.Handle(event)
}

// GetHandlerCount returns the number of handlers for an event type
func (bus *InMemoryEventBus) GetHandlerCount(eventType string) int {
	bus.mutex.RLock()
	defer bus.mutex.RUnlock()
	
	if handlers, exists := bus.handlers[eventType]; exists {
		return len(handlers)
	}
	return 0
}

// GetRegisteredEventTypes returns all event types that have handlers
func (bus *InMemoryEventBus) GetRegisteredEventTypes() []string {
	bus.mutex.RLock()
	defer bus.mutex.RUnlock()
	
	types := make([]string, 0, len(bus.handlers))
	for eventType := range bus.handlers {
		if len(bus.handlers[eventType]) > 0 {
			types = append(types, eventType)
		}
	}
	return types
}