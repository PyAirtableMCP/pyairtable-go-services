package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/Reg-Kris/pyairtable-webhook-service/internal/models"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type deadLetterRepository struct {
	db    *gorm.DB
	redis *redis.Client
}

// NewDeadLetterRepository creates a new dead letter repository
func NewDeadLetterRepository(db *gorm.DB, redis *redis.Client) DeadLetterRepository {
	return &deadLetterRepository{
		db:    db,
		redis: redis,
	}
}

// Create creates a new dead letter entry
func (r *deadLetterRepository) Create(ctx context.Context, deadLetter *models.DeadLetter) error {
	if err := r.db.WithContext(ctx).Create(deadLetter).Error; err != nil {
		return fmt.Errorf("failed to create dead letter: %w", err)
	}
	
	return nil
}

// GetByWebhookID retrieves dead letters by webhook ID with pagination
func (r *deadLetterRepository) GetByWebhookID(ctx context.Context, webhookID uint, limit, offset int) ([]*models.DeadLetter, error) {
	var deadLetters []*models.DeadLetter
	
	query := r.db.WithContext(ctx).
		Where("webhook_id = ?", webhookID).
		Order("created_at DESC")
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	
	if err := query.Find(&deadLetters).Error; err != nil {
		return nil, fmt.Errorf("failed to get dead letters by webhook ID: %w", err)
	}
	
	return deadLetters, nil
}

// GetUnprocessed retrieves unprocessed dead letters
func (r *deadLetterRepository) GetUnprocessed(ctx context.Context, limit int) ([]*models.DeadLetter, error) {
	var deadLetters []*models.DeadLetter
	
	query := r.db.WithContext(ctx).
		Where("is_processed = ?", false).
		Preload("Webhook").
		Order("last_attempt_at ASC")
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	
	if err := query.Find(&deadLetters).Error; err != nil {
		return nil, fmt.Errorf("failed to get unprocessed dead letters: %w", err)
	}
	
	return deadLetters, nil
}

// Update updates a dead letter entry
func (r *deadLetterRepository) Update(ctx context.Context, deadLetter *models.DeadLetter) error {
	if err := r.db.WithContext(ctx).Save(deadLetter).Error; err != nil {
		return fmt.Errorf("failed to update dead letter: %w", err)
	}
	
	return nil
}

// MarkProcessed marks a dead letter as processed
func (r *deadLetterRepository) MarkProcessed(ctx context.Context, id uint) error {
	now := time.Now()
	
	if err := r.db.WithContext(ctx).
		Model(&models.DeadLetter{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"is_processed": true,
			"processed_at": now,
			"updated_at":   now,
		}).Error; err != nil {
		return fmt.Errorf("failed to mark dead letter as processed: %w", err)
	}
	
	return nil
}

// Replay creates a new delivery attempt from a dead letter
func (r *deadLetterRepository) Replay(ctx context.Context, id uint) error {
	// Start a transaction
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Get the dead letter
		var deadLetter models.DeadLetter
		if err := tx.Preload("Webhook").First(&deadLetter, id).Error; err != nil {
			return fmt.Errorf("failed to find dead letter: %w", err)
		}
		
		// Create a new delivery attempt
		attempt := &models.DeliveryAttempt{
			WebhookID:      deadLetter.WebhookID,
			EventID:        deadLetter.EventID,
			EventType:      deadLetter.EventType,
			Payload:        deadLetter.Payload,
			Status:         models.DeliveryStatusPending,
			AttemptNumber:  1,
			ScheduledAt:    time.Now(),
			IdempotencyKey: fmt.Sprintf("replay-%d-%d", deadLetter.ID, time.Now().Unix()),
		}
		
		if err := tx.Create(attempt).Error; err != nil {
			return fmt.Errorf("failed to create replay delivery attempt: %w", err)
		}
		
		// Mark dead letter as processed
		now := time.Now()
		if err := tx.Model(&deadLetter).Updates(map[string]interface{}{
			"is_processed": true,
			"processed_at": now,
			"updated_at":   now,
		}).Error; err != nil {
			return fmt.Errorf("failed to mark dead letter as processed: %w", err)
		}
		
		return nil
	})
}