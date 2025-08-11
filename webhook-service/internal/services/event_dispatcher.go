package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Reg-Kris/pyairtable-webhook-service/internal/models"
	"github.com/Reg-Kris/pyairtable-webhook-service/internal/repositories"
	"go.uber.org/zap"
)

// EventDispatcher handles incoming events and dispatches them to matching webhooks
type EventDispatcher struct {
	repos    *repositories.Repositories
	delivery *DeliveryService
	logger   *zap.Logger
}

// NewEventDispatcher creates a new event dispatcher
func NewEventDispatcher(repos *repositories.Repositories, delivery *DeliveryService, logger *zap.Logger) *EventDispatcher {
	return &EventDispatcher{
		repos:    repos,
		delivery: delivery,
		logger:   logger,
	}
}

// DispatchEvent processes an incoming event and delivers it to matching webhooks
func (e *EventDispatcher) DispatchEvent(ctx context.Context, event *models.WebhookEvent) error {
	e.logger.Info("Dispatching webhook event",
		zap.String("event_id", event.ID),
		zap.String("event_type", string(event.Type)),
		zap.String("resource_id", event.ResourceID))
	
	// Get active subscriptions for this event type
	subscriptions, err := e.repos.Subscription.GetActiveSubscriptions(ctx, event.Type, &event.ResourceID)
	if err != nil {
		return fmt.Errorf("failed to get active subscriptions: %w", err)
	}
	
	if len(subscriptions) == 0 {
		e.logger.Debug("No active subscriptions found for event",
			zap.String("event_type", string(event.Type)),
			zap.String("resource_id", event.ResourceID))
		return nil
	}
	
	// Group subscriptions by webhook to avoid duplicate deliveries
	webhookSubscriptions := make(map[uint][]*models.Subscription)
	for _, sub := range subscriptions {
		webhookSubscriptions[sub.WebhookID] = append(webhookSubscriptions[sub.WebhookID], sub)
	}
	
	// Dispatch to each matching webhook
	var deliveryErrors []error
	
	for webhookID, subs := range webhookSubscriptions {
		// Apply filters to check if event should be delivered
		shouldDeliver := false
		matchingSubscription := (*models.Subscription)(nil)
		
		for _, sub := range subs {
			if e.eventMatchesSubscription(event, sub) {
				shouldDeliver = true
				matchingSubscription = sub
				break
			}
		}
		
		if !shouldDeliver {
			e.logger.Debug("Event does not match subscription filters",
				zap.Uint("webhook_id", webhookID),
				zap.String("event_id", event.ID))
			continue
		}
		
		// Get webhook details
		webhook := &matchingSubscription.Webhook
		if webhook.Status != models.WebhookStatusActive {
			e.logger.Debug("Skipping inactive webhook",
				zap.Uint("webhook_id", webhookID),
				zap.String("status", string(webhook.Status)))
			continue
		}
		
		// Deliver the event
		if err := e.delivery.DeliverWebhook(ctx, webhook, event); err != nil {
			e.logger.Error("Failed to deliver webhook",
				zap.Error(err),
				zap.Uint("webhook_id", webhookID),
				zap.String("event_id", event.ID))
			deliveryErrors = append(deliveryErrors, fmt.Errorf("webhook %d: %w", webhookID, err))
		} else {
			e.logger.Info("Successfully dispatched webhook",
				zap.Uint("webhook_id", webhookID),
				zap.String("event_id", event.ID))
		}
	}
	
	// Return combined errors if any deliveries failed
	if len(deliveryErrors) > 0 {
		var errorMessages []string
		for _, err := range deliveryErrors {
			errorMessages = append(errorMessages, err.Error())
		}
		return fmt.Errorf("delivery failures: %s", strings.Join(errorMessages, "; "))
	}
	
	return nil
}

// eventMatchesSubscription checks if an event matches a subscription's filters
func (e *EventDispatcher) eventMatchesSubscription(event *models.WebhookEvent, subscription *models.Subscription) bool {
	// Basic event type match (already filtered by repository)
	if subscription.EventType != event.Type {
		return false
	}
	
	// Resource ID filter
	if subscription.ResourceID != nil && *subscription.ResourceID != event.ResourceID {
		return false
	}
	
	// Workspace ID filter
	if event.WorkspaceID != nil && subscription.Webhook.WorkspaceID != nil {
		if *event.WorkspaceID != *subscription.Webhook.WorkspaceID {
			return false
		}
	}
	
	// Apply custom filters
	if subscription.Filters != nil && len(subscription.Filters) > 0 {
		return e.applyCustomFilters(event, subscription.Filters)
	}
	
	return true
}

// applyCustomFilters applies custom filtering logic
func (e *EventDispatcher) applyCustomFilters(event *models.WebhookEvent, filters models.JSONMap) bool {
	for filterKey, filterValue := range filters {
		switch filterKey {
		case "user_id":
			if !e.matchesUserIDFilter(event, filterValue) {
				return false
			}
		case "fields":
			if !e.matchesFieldsFilter(event, filterValue) {
				return false
			}
		case "conditions":
			if !e.matchesConditionsFilter(event, filterValue) {
				return false
			}
		case "time_range":
			if !e.matchesTimeRangeFilter(event, filterValue) {
				return false
			}
		default:
			e.logger.Warn("Unknown filter type", zap.String("filter_key", filterKey))
		}
	}
	
	return true
}

// matchesUserIDFilter checks if event matches user ID filter
func (e *EventDispatcher) matchesUserIDFilter(event *models.WebhookEvent, filter interface{}) bool {
	userIDs, ok := filter.([]interface{})
	if !ok {
		// Single user ID
		if userIDFloat, ok := filter.(float64); ok {
			return uint(userIDFloat) == event.UserID
		}
		return false
	}
	
	// Multiple user IDs
	for _, userIDInterface := range userIDs {
		if userIDFloat, ok := userIDInterface.(float64); ok {
			if uint(userIDFloat) == event.UserID {
				return true
			}
		}
	}
	
	return false
}

// matchesFieldsFilter checks if event matches field-based filters
func (e *EventDispatcher) matchesFieldsFilter(event *models.WebhookEvent, filter interface{}) bool {
	fieldsFilter, ok := filter.(map[string]interface{})
	if !ok {
		return false
	}
	
	for fieldName, expectedValue := range fieldsFilter {
		eventValue, exists := event.Data[fieldName]
		if !exists {
			return false
		}
		
		// Compare values
		if !e.compareValues(eventValue, expectedValue) {
			return false
		}
	}
	
	return true
}

// matchesConditionsFilter applies complex conditional logic
func (e *EventDispatcher) matchesConditionsFilter(event *models.WebhookEvent, filter interface{}) bool {
	conditions, ok := filter.(map[string]interface{})
	if !ok {
		return false
	}
	
	operator, hasOperator := conditions["operator"]
	if !hasOperator {
		return false
	}
	
	rules, hasRules := conditions["rules"].([]interface{})
	if !hasRules {
		return false
	}
	
	switch operator {
	case "and":
		return e.evaluateAndConditions(event, rules)
	case "or":
		return e.evaluateOrConditions(event, rules)
	case "not":
		return !e.evaluateOrConditions(event, rules)
	default:
		return false
	}
}

// evaluateAndConditions evaluates AND logic for conditions
func (e *EventDispatcher) evaluateAndConditions(event *models.WebhookEvent, rules []interface{}) bool {
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			return false
		}
		
		if !e.evaluateRule(event, ruleMap) {
			return false
		}
	}
	return true
}

// evaluateOrConditions evaluates OR logic for conditions
func (e *EventDispatcher) evaluateOrConditions(event *models.WebhookEvent, rules []interface{}) bool {
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}
		
		if e.evaluateRule(event, ruleMap) {
			return true
		}
	}
	return false
}

// evaluateRule evaluates a single rule
func (e *EventDispatcher) evaluateRule(event *models.WebhookEvent, rule map[string]interface{}) bool {
	field, hasField := rule["field"].(string)
	operator, hasOperator := rule["operator"].(string)
	value, hasValue := rule["value"]
	
	if !hasField || !hasOperator || !hasValue {
		return false
	}
	
	eventValue, exists := event.Data[field]
	if !exists {
		return operator == "not_exists"
	}
	
	switch operator {
	case "equals":
		return e.compareValues(eventValue, value)
	case "not_equals":
		return !e.compareValues(eventValue, value)
	case "contains":
		return e.containsValue(eventValue, value)
	case "not_contains":
		return !e.containsValue(eventValue, value)
	case "greater_than":
		return e.compareNumeric(eventValue, value, ">")
	case "less_than":
		return e.compareNumeric(eventValue, value, "<")
	case "exists":
		return true // Already checked above
	case "not_exists":
		return false // Already checked above
	default:
		return false
	}
}

// matchesTimeRangeFilter checks if event timestamp matches time range
func (e *EventDispatcher) matchesTimeRangeFilter(event *models.WebhookEvent, filter interface{}) bool {
	timeRange, ok := filter.(map[string]interface{})
	if !ok {
		return false
	}
	
	startTime, hasStart := timeRange["start"].(string)
	endTime, hasEnd := timeRange["end"].(string)
	
	if hasStart {
		start, err := time.Parse(time.RFC3339, startTime)
		if err != nil {
			return false
		}
		if event.Timestamp.Before(start) {
			return false
		}
	}
	
	if hasEnd {
		end, err := time.Parse(time.RFC3339, endTime)
		if err != nil {
			return false
		}
		if event.Timestamp.After(end) {
			return false
		}
	}
	
	return true
}

// compareValues compares two values for equality
func (e *EventDispatcher) compareValues(a, b interface{}) bool {
	// Handle different numeric types
	if aFloat, ok := a.(float64); ok {
		if bFloat, ok := b.(float64); ok {
			return aFloat == bFloat
		}
		if bInt, ok := b.(int); ok {
			return aFloat == float64(bInt)
		}
	}
	
	if aInt, ok := a.(int); ok {
		if bFloat, ok := b.(float64); ok {
			return float64(aInt) == bFloat
		}
		if bInt, ok := b.(int); ok {
			return aInt == bInt
		}
	}
	
	// String comparison
	if aStr, ok := a.(string); ok {
		if bStr, ok := b.(string); ok {
			return aStr == bStr
		}
	}
	
	// Boolean comparison
	if aBool, ok := a.(bool); ok {
		if bBool, ok := b.(bool); ok {
			return aBool == bBool
		}
	}
	
	// Default comparison
	return a == b
}

// containsValue checks if a value contains another value
func (e *EventDispatcher) containsValue(haystack, needle interface{}) bool {
	haystackStr, ok := haystack.(string)
	if !ok {
		return false
	}
	
	needleStr, ok := needle.(string)
	if !ok {
		return false
	}
	
	return strings.Contains(strings.ToLower(haystackStr), strings.ToLower(needleStr))
}

// compareNumeric compares numeric values
func (e *EventDispatcher) compareNumeric(a, b interface{}, operator string) bool {
	aFloat, aOk := e.toFloat64(a)
	bFloat, bOk := e.toFloat64(b)
	
	if !aOk || !bOk {
		return false
	}
	
	switch operator {
	case ">":
		return aFloat > bFloat
	case "<":
		return aFloat < bFloat
	case ">=":
		return aFloat >= bFloat
	case "<=":
		return aFloat <= bFloat
	default:
		return false
	}
}

// toFloat64 converts various numeric types to float64
func (e *EventDispatcher) toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case string:
		if f, err := json.Number(val).Float64(); err == nil {
			return f, true
		}
	}
	return 0, false
}