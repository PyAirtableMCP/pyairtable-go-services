package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Reg-Kris/pyairtable-webhook-service/internal/models"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// EventListener listens for events from other PyAirtable services
type EventListener struct {
	redis      *redis.Client
	dispatcher *EventDispatcher
	logger     *zap.Logger
	channels   []string
	stopChan   chan struct{}
}

// NewEventListener creates a new event listener
func NewEventListener(redis *redis.Client, dispatcher *EventDispatcher, logger *zap.Logger) *EventListener {
	return &EventListener{
		redis:      redis,
		dispatcher: dispatcher,
		logger:     logger,
		channels: []string{
			"pyairtable:events:table",
			"pyairtable:events:record",
			"pyairtable:events:workspace",
			"pyairtable:events:user",
		},
		stopChan: make(chan struct{}),
	}
}

// Start begins listening for events
func (l *EventListener) Start(ctx context.Context) error {
	l.logger.Info("Starting event listener", zap.Strings("channels", l.channels))
	
	// Subscribe to Redis channels
	pubsub := l.redis.Subscribe(ctx, l.channels...)
	defer pubsub.Close()
	
	// Wait for subscription confirmation
	_, err := pubsub.Receive(ctx)
	if err != nil {
		return fmt.Errorf("failed to subscribe to channels: %w", err)
	}
	
	l.logger.Info("Successfully subscribed to event channels")
	
	// Start processing messages
	ch := pubsub.Channel()
	
	for {
		select {
		case <-ctx.Done():
			l.logger.Info("Event listener stopped due to context cancellation")
			return ctx.Err()
		case <-l.stopChan:
			l.logger.Info("Event listener stopped")
			return nil
		case msg := <-ch:
			if err := l.processMessage(ctx, msg); err != nil {
				l.logger.Error("Failed to process message", zap.Error(err), zap.String("channel", msg.Channel))
			}
		}
	}
}

// Stop stops the event listener
func (l *EventListener) Stop() {
	close(l.stopChan)
}

// processMessage processes an incoming event message
func (l *EventListener) processMessage(ctx context.Context, msg *redis.Message) error {
	l.logger.Debug("Received event message", zap.String("channel", msg.Channel), zap.String("payload", msg.Payload))
	
	// Parse the event
	var rawEvent map[string]interface{}
	if err := json.Unmarshal([]byte(msg.Payload), &rawEvent); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}
	
	// Convert to WebhookEvent
	event, err := l.convertToWebhookEvent(msg.Channel, rawEvent)
	if err != nil {
		return fmt.Errorf("failed to convert event: %w", err)
	}
	
	// Dispatch the event
	if err := l.dispatcher.DispatchEvent(ctx, event); err != nil {
		l.logger.Error("Failed to dispatch event", zap.Error(err), zap.String("event_id", event.ID))
		return err
	}
	
	return nil
}

// convertToWebhookEvent converts a raw event to a WebhookEvent
func (l *EventListener) convertToWebhookEvent(channel string, rawEvent map[string]interface{}) (*models.WebhookEvent, error) {
	// Extract common fields
	eventID, ok := rawEvent["id"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid event ID")
	}
	
	action, ok := rawEvent["action"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid action")
	}
	
	resourceID, ok := rawEvent["resource_id"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid resource_id")
	}
	
	// Parse timestamp
	timestampStr, ok := rawEvent["timestamp"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid timestamp")
	}
	
	timestamp, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp: %w", err)
	}
	
	// Extract user ID
	var userID uint\n	if userIDFloat, ok := rawEvent["user_id"].(float64); ok {\n		userID = uint(userIDFloat)\n	} else {\n		return nil, fmt.Errorf(\"missing or invalid user_id\")\n	}\n	\n	// Extract workspace ID (optional)\n	var workspaceID *uint\n	if workspaceIDFloat, ok := rawEvent[\"workspace_id\"].(float64); ok {\n		wsID := uint(workspaceIDFloat)\n		workspaceID = &wsID\n	}\n	\n	// Determine event type based on channel and action\n	eventType, err := l.determineEventType(channel, action)\n	if err != nil {\n		return nil, err\n	}\n	\n	// Extract event data\n	data, ok := rawEvent[\"data\"].(map[string]interface{})\n	if !ok {\n		data = make(map[string]interface{})\n	}\n	\n	// Extract metadata\n	metadata, ok := rawEvent[\"metadata\"].(map[string]interface{})\n	if !ok {\n		metadata = make(map[string]interface{})\n	}\n	\n	return &models.WebhookEvent{\n		ID:          eventID,\n		Type:        eventType,\n		ResourceID:  resourceID,\n		WorkspaceID: workspaceID,\n		UserID:      userID,\n		Timestamp:   timestamp,\n		Data:        models.JSONMap(data),\n		Metadata:    models.JSONMap(metadata),\n	}, nil\n}\n\n// determineEventType maps channel and action to EventType\nfunc (l *EventListener) determineEventType(channel, action string) (models.EventType, error) {\n	switch channel {\n	case \"pyairtable:events:table\":\n		switch action {\n		case \"created\":\n			return models.EventTypeTableCreated, nil\n		case \"updated\":\n			return models.EventTypeTableUpdated, nil\n		case \"deleted\":\n			return models.EventTypeTableDeleted, nil\n		}\n	case \"pyairtable:events:record\":\n		switch action {\n		case \"created\":\n			return models.EventTypeRecordCreated, nil\n		case \"updated\":\n			return models.EventTypeRecordUpdated, nil\n		case \"deleted\":\n			return models.EventTypeRecordDeleted, nil\n		}\n	case \"pyairtable:events:workspace\":\n		switch action {\n		case \"created\":\n			return models.EventTypeWorkspaceCreated, nil\n		case \"updated\":\n			return models.EventTypeWorkspaceUpdated, nil\n		}\n	case \"pyairtable:events:user\":\n		switch action {\n		case \"invited\":\n			return models.EventTypeUserInvited, nil\n		case \"joined\":\n			return models.EventTypeUserJoined, nil\n		}\n	}\n	\n	return \"\", fmt.Errorf(\"unknown event type for channel %s and action %s\", channel, action)\n}\n\n// PublishEvent publishes an event for testing purposes\nfunc (l *EventListener) PublishEvent(ctx context.Context, channel string, event map[string]interface{}) error {\n	eventJSON, err := json.Marshal(event)\n	if err != nil {\n		return fmt.Errorf(\"failed to marshal event: %w\", err)\n	}\n	\n	if err := l.redis.Publish(ctx, channel, eventJSON).Err(); err != nil {\n		return fmt.Errorf(\"failed to publish event: %w\", err)\n	}\n	\n	l.logger.Info(\"Published test event\", zap.String(\"channel\", channel))\n	return nil\n}\n\n// GetEventChannels returns the list of channels being listened to\nfunc (l *EventListener) GetEventChannels() []string {\n	return l.channels\n}\n\n// AddEventChannel adds a new channel to listen to\nfunc (l *EventListener) AddEventChannel(channel string) {\n	l.channels = append(l.channels, channel)\n}\n\n// RemoveEventChannel removes a channel from the listener\nfunc (l *EventListener) RemoveEventChannel(channel string) {\n	for i, ch := range l.channels {\n		if ch == channel {\n			l.channels = append(l.channels[:i], l.channels[i+1:]...)\n			break\n		}\n	}\n}