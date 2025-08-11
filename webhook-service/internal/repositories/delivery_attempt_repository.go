package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/Reg-Kris/pyairtable-webhook-service/internal/models"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type deliveryAttemptRepository struct {
	db    *gorm.DB
	redis *redis.Client
}

// NewDeliveryAttemptRepository creates a new delivery attempt repository
func NewDeliveryAttemptRepository(db *gorm.DB, redis *redis.Client) DeliveryAttemptRepository {
	return &deliveryAttemptRepository{
		db:    db,
		redis: redis,
	}
}

// Create creates a new delivery attempt
func (r *deliveryAttemptRepository) Create(ctx context.Context, attempt *models.DeliveryAttempt) error {
	if err := r.db.WithContext(ctx).Create(attempt).Error; err != nil {
		return fmt.Errorf("failed to create delivery attempt: %w", err)
	}
	
	return nil
}

// GetByID retrieves a delivery attempt by ID
func (r *deliveryAttemptRepository) GetByID(ctx context.Context, id uint) (*models.DeliveryAttempt, error) {
	var attempt models.DeliveryAttempt
	
	if err := r.db.WithContext(ctx).
		Preload("Webhook").
		First(&attempt, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get delivery attempt by ID: %w", err)
	}
	
	return &attempt, nil
}

// GetByWebhookID retrieves delivery attempts by webhook ID with pagination
func (r *deliveryAttemptRepository) GetByWebhookID(ctx context.Context, webhookID uint, limit, offset int) ([]*models.DeliveryAttempt, error) {
	var attempts []*models.DeliveryAttempt
	
	query := r.db.WithContext(ctx).
		Where("webhook_id = ?", webhookID).
		Order("created_at DESC")
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	
	if err := query.Find(&attempts).Error; err != nil {
		return nil, fmt.Errorf("failed to get delivery attempts by webhook ID: %w", err)
	}
	
	return attempts, nil
}

// GetPendingDeliveries retrieves pending delivery attempts
func (r *deliveryAttemptRepository) GetPendingDeliveries(ctx context.Context, limit int) ([]*models.DeliveryAttempt, error) {
	var attempts []*models.DeliveryAttempt
	
	query := r.db.WithContext(ctx).
		Where("status = ? AND scheduled_at <= ?", models.DeliveryStatusPending, time.Now()).
		Preload("Webhook").
		Order("scheduled_at ASC")
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	
	if err := query.Find(&attempts).Error; err != nil {
		return nil, fmt.Errorf("failed to get pending deliveries: %w", err)
	}
	
	return attempts, nil
}

// GetRetriesReady retrieves delivery attempts ready for retry
func (r *deliveryAttemptRepository) GetRetriesReady(ctx context.Context, limit int) ([]*models.DeliveryAttempt, error) {
	var attempts []*models.DeliveryAttempt
	
	query := r.db.WithContext(ctx).
		Where("status = ? AND next_retry_at <= ?", models.DeliveryStatusRetrying, time.Now()).
		Preload("Webhook").
		Order("next_retry_at ASC")
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	
	if err := query.Find(&attempts).Error; err != nil {
		return nil, fmt.Errorf("failed to get retries ready: %w", err)
	}
	
	return attempts, nil
}

// Update updates a delivery attempt
func (r *deliveryAttemptRepository) Update(ctx context.Context, attempt *models.DeliveryAttempt) error {
	if err := r.db.WithContext(ctx).Save(attempt).Error; err != nil {
		return fmt.Errorf("failed to update delivery attempt: %w", err)
	}
	
	return nil
}

// UpdateStatus updates the status of a delivery attempt
func (r *deliveryAttemptRepository) UpdateStatus(ctx context.Context, id uint, status models.DeliveryStatus) error {
	now := time.Now()
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": now,
	}
	
	// Set delivered_at if status is delivered
	if status == models.DeliveryStatusDelivered {
		updates["delivered_at"] = now
	}
	
	if err := r.db.WithContext(ctx).
		Model(&models.DeliveryAttempt{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update delivery attempt status: %w", err)
	}
	
	return nil
}

// GetMetrics retrieves webhook metrics
func (r *deliveryAttemptRepository) GetMetrics(ctx context.Context, webhookID uint, since time.Time) (*models.WebhookMetrics, error) {
	var metrics struct {
		TotalDeliveries      int64
		SuccessfulDeliveries int64
		FailedDeliveries     int64
		AverageLatencyMs     float64
	}
	
	// Get total deliveries
	if err := r.db.WithContext(ctx).
		Model(&models.DeliveryAttempt{}).
		Where("webhook_id = ? AND created_at >= ?", webhookID, since).
		Count(&metrics.TotalDeliveries).Error; err != nil {
		return nil, fmt.Errorf("failed to get total deliveries: %w", err)
	}
	
	// Get successful deliveries
	if err := r.db.WithContext(ctx).
		Model(&models.DeliveryAttempt{}).
		Where("webhook_id = ? AND status = ? AND created_at >= ?", 
			webhookID, models.DeliveryStatusDelivered, since).
		Count(&metrics.SuccessfulDeliveries).Error; err != nil {
		return nil, fmt.Errorf("failed to get successful deliveries: %w", err)
	}
	
	// Get failed deliveries
	metrics.FailedDeliveries = metrics.TotalDeliveries - metrics.SuccessfulDeliveries
	
	// Get average latency
	var avgLatency *float64
	if err := r.db.WithContext(ctx).
		Model(&models.DeliveryAttempt{}).
		Where("webhook_id = ? AND status = ? AND duration_ms IS NOT NULL AND created_at >= ?", 
			webhookID, models.DeliveryStatusDelivered, since).
		Select("AVG(duration_ms)").
		Row().Scan(&avgLatency); err != nil {
		return nil, fmt.Errorf("failed to get average latency: %w", err)
	}
	
	if avgLatency != nil {
		metrics.AverageLatencyMs = *avgLatency
	}
	
	// Get last delivery time
	var lastDelivery models.DeliveryAttempt
	var lastDeliveryAt *time.Time
	if err := r.db.WithContext(ctx).
		Where("webhook_id = ? AND status = ?", webhookID, models.DeliveryStatusDelivered).
		Order("delivered_at DESC").
		First(&lastDelivery).Error; err == nil {
		lastDeliveryAt = lastDelivery.DeliveredAt
	}
	
	// Calculate success rate
	var successRate float64
	if metrics.TotalDeliveries > 0 {
		successRate = float64(metrics.SuccessfulDeliveries) / float64(metrics.TotalDeliveries) * 100
	}
	
	return &models.WebhookMetrics{
		WebhookID:            webhookID,
		TotalDeliveries:      metrics.TotalDeliveries,
		SuccessfulDeliveries: metrics.SuccessfulDeliveries,
		FailedDeliveries:     metrics.FailedDeliveries,
		AverageLatencyMs:     metrics.AverageLatencyMs,
		LastDeliveryAt:       lastDeliveryAt,
		SuccessRate:          successRate,
	}, nil
}