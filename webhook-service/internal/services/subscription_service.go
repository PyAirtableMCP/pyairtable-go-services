package services

import (
	"context"
	"fmt"

	"github.com/Reg-Kris/pyairtable-webhook-service/internal/handlers"
	"github.com/Reg-Kris/pyairtable-webhook-service/internal/models"
	"github.com/Reg-Kris/pyairtable-webhook-service/internal/repositories"
	"go.uber.org/zap"
)

// SubscriptionService handles subscription business logic
type SubscriptionService struct {
	repos  *repositories.Repositories
	logger *zap.Logger
}

// NewSubscriptionService creates a new subscription service
func NewSubscriptionService(repos *repositories.Repositories, logger *zap.Logger) *SubscriptionService {
	return &SubscriptionService{
		repos:  repos,
		logger: logger,
	}
}

// CreateSubscription creates a new subscription
func (s *SubscriptionService) CreateSubscription(ctx context.Context, webhookID, userID uint, req *handlers.CreateSubscriptionRequest) (*models.Subscription, error) {
	// Verify webhook ownership
	webhook, err := s.repos.Webhook.GetByID(ctx, webhookID)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook: %w", err)
	}
	
	if webhook == nil || webhook.UserID != userID {
		return nil, fmt.Errorf("webhook not found or access denied")
	}
	
	// Set defaults
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	
	// Create subscription
	subscription := &models.Subscription{
		WebhookID:  webhookID,
		EventType:  req.EventType,
		ResourceID: req.ResourceID,
		IsActive:   isActive,
		Filters:    models.JSONMap(req.Filters),
	}
	
	if err := s.repos.Subscription.Create(ctx, subscription); err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}
	
	s.logger.Info("Created subscription",
		zap.Uint("subscription_id", subscription.ID),
		zap.Uint("webhook_id", webhookID),
		zap.String("event_type", string(req.EventType)))
	
	return subscription, nil
}

// GetSubscriptions retrieves subscriptions for a webhook
func (s *SubscriptionService) GetSubscriptions(ctx context.Context, webhookID, userID uint) ([]*models.Subscription, error) {
	// Verify webhook ownership
	webhook, err := s.repos.Webhook.GetByID(ctx, webhookID)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook: %w", err)
	}
	
	if webhook == nil || webhook.UserID != userID {
		return nil, fmt.Errorf("webhook not found or access denied")
	}
	
	return s.repos.Subscription.GetByWebhookID(ctx, webhookID)
}

// GetSubscription retrieves a subscription by ID
func (s *SubscriptionService) GetSubscription(ctx context.Context, subscriptionID, webhookID, userID uint) (*models.Subscription, error) {
	// Verify webhook ownership
	webhook, err := s.repos.Webhook.GetByID(ctx, webhookID)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook: %w", err)
	}
	
	if webhook == nil || webhook.UserID != userID {
		return nil, fmt.Errorf("webhook not found or access denied")
	}
	
	// Get all subscriptions for the webhook and find the matching one
	subscriptions, err := s.repos.Subscription.GetByWebhookID(ctx, webhookID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscriptions: %w", err)
	}
	
	for _, sub := range subscriptions {
		if sub.ID == subscriptionID {
			return sub, nil
		}
	}
	
	return nil, nil
}

// UpdateSubscription updates a subscription
func (s *SubscriptionService) UpdateSubscription(ctx context.Context, subscriptionID, webhookID, userID uint, req *handlers.UpdateSubscriptionRequest) (*models.Subscription, error) {
	subscription, err := s.GetSubscription(ctx, subscriptionID, webhookID, userID)
	if err != nil {
		return nil, err
	}
	
	if subscription == nil {
		return nil, nil
	}
	
	// Update fields
	if req.EventType != nil {
		subscription.EventType = *req.EventType
	}
	
	if req.ResourceID != nil {
		subscription.ResourceID = req.ResourceID
	}
	
	if req.IsActive != nil {
		subscription.IsActive = *req.IsActive
	}
	
	if req.Filters != nil {
		subscription.Filters = models.JSONMap(req.Filters)
	}
	
	if err := s.repos.Subscription.Update(ctx, subscription); err != nil {
		return nil, fmt.Errorf("failed to update subscription: %w", err)
	}
	
	s.logger.Info("Updated subscription",
		zap.Uint("subscription_id", subscriptionID),
		zap.Uint("webhook_id", webhookID))
	
	return subscription, nil
}

// DeleteSubscription deletes a subscription
func (s *SubscriptionService) DeleteSubscription(ctx context.Context, subscriptionID, webhookID, userID uint) error {
	subscription, err := s.GetSubscription(ctx, subscriptionID, webhookID, userID)
	if err != nil {
		return err
	}
	
	if subscription == nil {
		return fmt.Errorf("subscription not found")
	}
	
	if err := s.repos.Subscription.Delete(ctx, subscriptionID); err != nil {
		return fmt.Errorf("failed to delete subscription: %w", err)
	}
	
	s.logger.Info("Deleted subscription",
		zap.Uint("subscription_id", subscriptionID),
		zap.Uint("webhook_id", webhookID))
	
	return nil
}