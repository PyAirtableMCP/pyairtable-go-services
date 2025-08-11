package repositories

import (
	"context"
	"fmt"

	"github.com/Reg-Kris/pyairtable-webhook-service/internal/models"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type subscriptionRepository struct {
	db    *gorm.DB
	redis *redis.Client
}

// NewSubscriptionRepository creates a new subscription repository
func NewSubscriptionRepository(db *gorm.DB, redis *redis.Client) SubscriptionRepository {
	return &subscriptionRepository{
		db:    db,
		redis: redis,
	}
}

// Create creates a new subscription
func (r *subscriptionRepository) Create(ctx context.Context, subscription *models.Subscription) error {
	if err := r.db.WithContext(ctx).Create(subscription).Error; err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}
	
	// Invalidate event type cache
	r.invalidateEventTypeCache(subscription.EventType)
	
	return nil
}

// GetByWebhookID retrieves subscriptions by webhook ID
func (r *subscriptionRepository) GetByWebhookID(ctx context.Context, webhookID uint) ([]*models.Subscription, error) {
	var subscriptions []*models.Subscription
	
	if err := r.db.WithContext(ctx).
		Where("webhook_id = ?", webhookID).
		Find(&subscriptions).Error; err != nil {
		return nil, fmt.Errorf("failed to get subscriptions by webhook ID: %w", err)
	}
	
	return subscriptions, nil
}

// GetByEventType retrieves subscriptions by event type
func (r *subscriptionRepository) GetByEventType(ctx context.Context, eventType models.EventType) ([]*models.Subscription, error) {
	var subscriptions []*models.Subscription
	
	if err := r.db.WithContext(ctx).
		Where("event_type = ? AND is_active = ?", eventType, true).
		Preload("Webhook").
		Find(&subscriptions).Error; err != nil {
		return nil, fmt.Errorf("failed to get subscriptions by event type: %w", err)
	}
	
	return subscriptions, nil
}

// Update updates a subscription
func (r *subscriptionRepository) Update(ctx context.Context, subscription *models.Subscription) error {
	if err := r.db.WithContext(ctx).Save(subscription).Error; err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}
	
	// Invalidate cache
	r.invalidateEventTypeCache(subscription.EventType)
	
	return nil
}

// Delete soft deletes a subscription
func (r *subscriptionRepository) Delete(ctx context.Context, id uint) error {
	var subscription models.Subscription
	if err := r.db.WithContext(ctx).First(&subscription, id).Error; err != nil {
		return fmt.Errorf("failed to find subscription: %w", err)
	}
	
	if err := r.db.WithContext(ctx).Delete(&subscription).Error; err != nil {
		return fmt.Errorf("failed to delete subscription: %w", err)
	}
	
	// Invalidate cache
	r.invalidateEventTypeCache(subscription.EventType)
	
	return nil
}

// GetActiveSubscriptions retrieves active subscriptions for an event type with optional resource filtering
func (r *subscriptionRepository) GetActiveSubscriptions(ctx context.Context, eventType models.EventType, resourceID *string) ([]*models.Subscription, error) {
	var subscriptions []*models.Subscription
	
	query := r.db.WithContext(ctx).
		Joins("JOIN webhooks ON webhooks.id = webhook_subscriptions.webhook_id").
		Where("webhook_subscriptions.event_type = ? AND webhook_subscriptions.is_active = ? AND webhooks.status = ?", 
			eventType, true, models.WebhookStatusActive).
		Preload("Webhook")
	
	// Add resource ID filter if provided
	if resourceID != nil {
		query = query.Where("(webhook_subscriptions.resource_id IS NULL OR webhook_subscriptions.resource_id = ?)", *resourceID)
	}
	
	if err := query.Find(&subscriptions).Error; err != nil {
		return nil, fmt.Errorf("failed to get active subscriptions: %w", err)
	}
	
	return subscriptions, nil
}

// invalidateEventTypeCache invalidates event type related cache entries
func (r *subscriptionRepository) invalidateEventTypeCache(eventType models.EventType) {
	if r.redis == nil {
		return
	}
	
	r.redis.Del(context.Background(), fmt.Sprintf("subscriptions:event_type:%s", eventType))
}