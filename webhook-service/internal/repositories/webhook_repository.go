package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/Reg-Kris/pyairtable-webhook-service/internal/models"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type webhookRepository struct {
	db    *gorm.DB
	redis *redis.Client
}

// NewWebhookRepository creates a new webhook repository
func NewWebhookRepository(db *gorm.DB, redis *redis.Client) WebhookRepository {
	return &webhookRepository{
		db:    db,
		redis: redis,
	}
}

// Create creates a new webhook
func (r *webhookRepository) Create(ctx context.Context, webhook *models.Webhook) error {
	if err := r.db.WithContext(ctx).Create(webhook).Error; err != nil {
		return fmt.Errorf("failed to create webhook: %w", err)
	}
	
	// Invalidate cache
	r.invalidateWebhookCache(webhook.UserID, webhook.WorkspaceID)
	
	return nil
}

// GetByID retrieves a webhook by ID
func (r *webhookRepository) GetByID(ctx context.Context, id uint) (*models.Webhook, error) {
	var webhook models.Webhook
	
	if err := r.db.WithContext(ctx).
		Preload("Subscriptions").
		First(&webhook, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get webhook by ID: %w", err)
	}
	
	return &webhook, nil
}

// GetByUserID retrieves webhooks by user ID with pagination
func (r *webhookRepository) GetByUserID(ctx context.Context, userID uint, limit, offset int) ([]*models.Webhook, error) {
	var webhooks []*models.Webhook
	
	query := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Preload("Subscriptions").
		Order("created_at DESC")
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	
	if err := query.Find(&webhooks).Error; err != nil {
		return nil, fmt.Errorf("failed to get webhooks by user ID: %w", err)
	}
	
	return webhooks, nil
}

// GetByWorkspaceID retrieves webhooks by workspace ID with pagination
func (r *webhookRepository) GetByWorkspaceID(ctx context.Context, workspaceID uint, limit, offset int) ([]*models.Webhook, error) {
	var webhooks []*models.Webhook
	
	query := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Preload("Subscriptions").
		Order("created_at DESC")
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	
	if err := query.Find(&webhooks).Error; err != nil {
		return nil, fmt.Errorf("failed to get webhooks by workspace ID: %w", err)
	}
	
	return webhooks, nil
}

// Update updates a webhook
func (r *webhookRepository) Update(ctx context.Context, webhook *models.Webhook) error {
	if err := r.db.WithContext(ctx).Save(webhook).Error; err != nil {
		return fmt.Errorf("failed to update webhook: %w", err)
	}
	
	// Invalidate cache
	r.invalidateWebhookCache(webhook.UserID, webhook.WorkspaceID)
	
	return nil
}

// Delete soft deletes a webhook
func (r *webhookRepository) Delete(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Delete(&models.Webhook{}, id).Error; err != nil {
		return fmt.Errorf("failed to delete webhook: %w", err)
	}
	
	return nil
}

// GetActiveWebhooks retrieves all active webhooks
func (r *webhookRepository) GetActiveWebhooks(ctx context.Context) ([]*models.Webhook, error) {
	var webhooks []*models.Webhook
	
	if err := r.db.WithContext(ctx).
		Where("status = ?", models.WebhookStatusActive).
		Preload("Subscriptions", "is_active = ?", true).
		Find(&webhooks).Error; err != nil {
		return nil, fmt.Errorf("failed to get active webhooks: %w", err)
	}
	
	return webhooks, nil
}

// UpdateStats updates webhook statistics
func (r *webhookRepository) UpdateStats(ctx context.Context, webhookID uint, success bool, deliveryTime time.Time) error {
	updates := map[string]interface{}{
		"last_delivery_at": deliveryTime,
	}
	
	if success {
		updates["success_count"] = gorm.Expr("success_count + 1")
		updates["failure_count"] = 0 // Reset failure count on success
	} else {
		updates["failure_count"] = gorm.Expr("failure_count + 1")
		updates["last_failure_at"] = deliveryTime
	}
	
	if err := r.db.WithContext(ctx).
		Model(&models.Webhook{}).
		Where("id = ?", webhookID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update webhook stats: %w", err)
	}
	
	return nil
}

// invalidateWebhookCache invalidates related cache entries
func (r *webhookRepository) invalidateWebhookCache(userID uint, workspaceID *uint) {
	if r.redis == nil {
		return
	}
	
	// Invalidate user webhooks cache
	r.redis.Del(context.Background(), fmt.Sprintf("webhooks:user:%d", userID))
	
	// Invalidate workspace webhooks cache if applicable
	if workspaceID != nil {
		r.redis.Del(context.Background(), fmt.Sprintf("webhooks:workspace:%d", *workspaceID))
	}
}