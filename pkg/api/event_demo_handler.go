package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/pyairtable-compose/go-services/domain/shared"
	"github.com/pyairtable-compose/go-services/pkg/eventbus"
	"github.com/pyairtable-compose/go-services/pkg/eventhandlers"
	"github.com/pyairtable-compose/go-services/pkg/eventprocessor"
	"github.com/pyairtable-compose/go-services/pkg/eventstore"
	"github.com/pyairtable-compose/go-services/pkg/services"
	"go.uber.org/zap"
)

// EventDemoHandler provides HTTP endpoints for demonstrating the DDD event flow
type EventDemoHandler struct {
	db                *sql.DB
	logger            *zap.Logger
	eventStore        *eventstore.PostgresEventStore
	eventBus          *eventbus.InMemoryEventBus
	eventProcessor    *eventprocessor.EventProcessor
	userService       *services.UserRegistrationServiceImpl
	projectionHandler *eventhandlers.UserProjectionHandler
}

// NewEventDemoHandler creates a new event demo handler
func NewEventDemoHandler(db *sql.DB, logger *zap.Logger) *EventDemoHandler {
	// Create event infrastructure
	eventStore := eventstore.NewPostgresEventStore(db)
	eventBus := eventbus.NewInMemoryEventBus(logger)
	eventProcessor := eventprocessor.NewEventProcessor(eventStore, eventBus, db, logger)

	// Create projection handler
	projectionHandler := eventhandlers.NewUserProjectionHandler(db, logger)

	// Subscribe projection handler to user events
	eventBus.Subscribe(shared.EventTypeUserRegistered, projectionHandler)
	eventBus.Subscribe(shared.EventTypeUserActivated, projectionHandler)
	eventBus.Subscribe(shared.EventTypeUserDeactivated, projectionHandler)
	eventBus.Subscribe(shared.EventTypeUserUpdated, projectionHandler)
	eventBus.Subscribe(shared.EventTypeUserRoleChanged, projectionHandler)

	// Create user service
	userService := services.NewUserRegistrationServiceImpl(eventStore, eventBus, db)

	// Start event processor
	eventProcessor.Start()

	return &EventDemoHandler{
		db:                db,
		logger:            logger,
		eventStore:        eventStore,
		eventBus:          eventBus,
		eventProcessor:    eventProcessor,
		userService:       userService,
		projectionHandler: projectionHandler,
	}
}

// DemoEventFlow demonstrates the complete event flow
func (h *EventDemoHandler) DemoEventFlow(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("Starting event flow demonstration")

	// Run the demonstration
	result, err := h.userService.DemonstrateEventFlow()
	if err != nil {
		h.logger.Error("Event flow demonstration failed", zap.Error(err))
		http.Error(w, "Event flow demonstration failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Wait a moment for event processing
	time.Sleep(2 * time.Second)

	// Get event processing stats
	stats, err := h.eventProcessor.GetStats()
	if err != nil {
		h.logger.Warn("Failed to get event stats", zap.Error(err))
	}

	// Return the result
	response := map[string]interface{}{
		"success": true,
		"message": "Event flow demonstration completed successfully",
		"result":  result,
		"stats":   stats,
		"event_types_registered": h.eventBus.GetRegisteredEventTypes(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	h.logger.Info("Event flow demonstration completed successfully")
}

// GetEventStats returns event processing statistics
func (h *EventDemoHandler) GetEventStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.eventProcessor.GetStats()
	if err != nil {
		h.logger.Error("Failed to get event stats", zap.Error(err))
		http.Error(w, "Failed to get event stats: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"stats":                  stats,
		"event_types_registered": h.eventBus.GetRegisteredEventTypes(),
		"handler_counts": map[string]int{
			shared.EventTypeUserRegistered:   h.eventBus.GetHandlerCount(shared.EventTypeUserRegistered),
			shared.EventTypeUserActivated:    h.eventBus.GetHandlerCount(shared.EventTypeUserActivated),
			shared.EventTypeUserDeactivated:  h.eventBus.GetHandlerCount(shared.EventTypeUserDeactivated),
			shared.EventTypeUserUpdated:      h.eventBus.GetHandlerCount(shared.EventTypeUserUpdated),
			shared.EventTypeUserRoleChanged:  h.eventBus.GetHandlerCount(shared.EventTypeUserRoleChanged),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetEvents returns recent events
func (h *EventDemoHandler) GetEvents(w http.ResponseWriter, r *http.Request) {
	eventType := r.URL.Query().Get("type")
	limitStr := r.URL.Query().Get("limit")
	
	limit := 50
	if limitStr != "" {
		if parsedLimit, err := json.Number(limitStr).Int64(); err == nil && parsedLimit > 0 {
			limit = int(parsedLimit)
		}
	}

	var events []shared.DomainEvent
	var err error

	if eventType != "" {
		// Get events by type for the last 24 hours
		from := time.Now().Add(-24 * time.Hour)
		to := time.Now()
		events, err = h.eventStore.GetEventsByType(eventType, from, to)
	} else {
		// Get unprocessed events
		events, err = h.eventStore.GetUnprocessedEvents(limit)
	}

	if err != nil {
		h.logger.Error("Failed to get events", zap.Error(err))
		http.Error(w, "Failed to get events: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert events to a JSON-friendly format
	eventData := make([]map[string]interface{}, len(events))
	for i, event := range events {
		eventData[i] = map[string]interface{}{
			"event_id":      event.EventID(),
			"event_type":    event.EventType(),
			"aggregate_id":  event.AggregateID(),
			"aggregate_type": event.AggregateType(),
			"version":       event.Version(),
			"tenant_id":     event.TenantID(),
			"occurred_at":   event.OccurredAt(),
			"payload":       event.Payload(),
		}
	}

	response := map[string]interface{}{
		"events": eventData,
		"count":  len(events),
		"filter": map[string]interface{}{
			"event_type": eventType,
			"limit":      limit,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetUserProjections returns user data from read model projections
func (h *EventDemoHandler) GetUserProjections(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT id, email, first_name, last_name, display_name, 
			   tenant_id, status, email_verified, is_active,
			   created_at, updated_at, event_version
		FROM user_projections 
		ORDER BY created_at DESC 
		LIMIT 10
	`)
	if err != nil {
		h.logger.Error("Failed to query user projections", zap.Error(err))
		http.Error(w, "Failed to query user projections: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []map[string]interface{}
	for rows.Next() {
		var (
			id            string
			email         string
			firstName     string
			lastName      string
			displayName   string
			tenantID      string
			status        string
			emailVerified bool
			isActive      bool
			createdAt     time.Time
			updatedAt     time.Time
			eventVersion  int64
		)

		err := rows.Scan(&id, &email, &firstName, &lastName, &displayName,
			&tenantID, &status, &emailVerified, &isActive,
			&createdAt, &updatedAt, &eventVersion)
		if err != nil {
			h.logger.Error("Failed to scan user projection", zap.Error(err))
			continue
		}

		users = append(users, map[string]interface{}{
			"id":             id,
			"email":          email,
			"first_name":     firstName,
			"last_name":      lastName,
			"display_name":   displayName,
			"tenant_id":      tenantID,
			"status":         status,
			"email_verified": emailVerified,
			"is_active":      isActive,
			"created_at":     createdAt,
			"updated_at":     updatedAt,
			"event_version":  eventVersion,
		})
	}

	if err := rows.Err(); err != nil {
		h.logger.Error("Error iterating over user projections", zap.Error(err))
		http.Error(w, "Error reading user projections: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"users": users,
		"count": len(users),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ReplayEvents replays events of a specific type
func (h *EventDemoHandler) ReplayEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	eventType := r.URL.Query().Get("type")
	if eventType == "" {
		http.Error(w, "event type is required", http.StatusBadRequest)
		return
	}

	hoursBack := 24
	if hoursStr := r.URL.Query().Get("hours"); hoursStr != "" {
		if hours, err := json.Number(hoursStr).Int64(); err == nil && hours > 0 {
			hoursBack = int(hours)
		}
	}

	from := time.Now().Add(-time.Duration(hoursBack) * time.Hour)
	to := time.Now()

	h.logger.Info("Replaying events", 
		zap.String("event_type", eventType),
		zap.Int("hours_back", hoursBack),
	)

	err := h.eventProcessor.ProcessEventsByType(eventType, from, to)
	if err != nil {
		h.logger.Error("Failed to replay events", zap.Error(err))
		http.Error(w, "Failed to replay events: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success":    true,
		"message":    "Events replayed successfully",
		"event_type": eventType,
		"time_range": map[string]interface{}{
			"from": from,
			"to":   to,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Health check endpoint
func (h *EventDemoHandler) Health(w http.ResponseWriter, r *http.Request) {
	stats, err := h.eventProcessor.GetStats()
	if err != nil {
		http.Error(w, "Health check failed", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"status": "healthy",
		"event_infrastructure": map[string]interface{}{
			"event_store_connected": true,
			"event_bus_active":     true,
			"processor_running":    true,
			"handlers_registered":  len(h.eventBus.GetRegisteredEventTypes()),
		},
		"stats": stats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}