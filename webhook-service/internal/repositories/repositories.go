package repositories

import (
	"context"
	"time"

	"github.com/Reg-Kris/pyairtable-webhook-service/internal/models"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// WebhookRepository defines the interface for webhook data operations
type WebhookRepository interface {
	Create(ctx context.Context, webhook *models.Webhook) error
	GetByID(ctx context.Context, id uint) (*models.Webhook, error)
	GetByUserID(ctx context.Context, userID uint, limit, offset int) ([]*models.Webhook, error)
	GetByWorkspaceID(ctx context.Context, workspaceID uint, limit, offset int) ([]*models.Webhook, error)
	Update(ctx context.Context, webhook *models.Webhook) error
	Delete(ctx context.Context, id uint) error
	GetActiveWebhooks(ctx context.Context) ([]*models.Webhook, error)
	UpdateStats(ctx context.Context, webhookID uint, success bool, deliveryTime time.Time) error
}

// SubscriptionRepository defines the interface for subscription operations
type SubscriptionRepository interface {
	Create(ctx context.Context, subscription *models.Subscription) error
	GetByWebhookID(ctx context.Context, webhookID uint) ([]*models.Subscription, error)
	GetByEventType(ctx context.Context, eventType models.EventType) ([]*models.Subscription, error)
	Update(ctx context.Context, subscription *models.Subscription) error
	Delete(ctx context.Context, id uint) error
	GetActiveSubscriptions(ctx context.Context, eventType models.EventType, resourceID *string) ([]*models.Subscription, error)
}

// DeliveryAttemptRepository defines the interface for delivery attempt operations
type DeliveryAttemptRepository interface {
	Create(ctx context.Context, attempt *models.DeliveryAttempt) error
	GetByID(ctx context.Context, id uint) (*models.DeliveryAttempt, error)
	GetByWebhookID(ctx context.Context, webhookID uint, limit, offset int) ([]*models.DeliveryAttempt, error)
	GetPendingDeliveries(ctx context.Context, limit int) ([]*models.DeliveryAttempt, error)
	GetRetriesReady(ctx context.Context, limit int) ([]*models.DeliveryAttempt, error)
	Update(ctx context.Context, attempt *models.DeliveryAttempt) error
	UpdateStatus(ctx context.Context, id uint, status models.DeliveryStatus) error
	GetMetrics(ctx context.Context, webhookID uint, since time.Time) (*models.WebhookMetrics, error)
}

// DeadLetterRepository defines the interface for dead letter operations
type DeadLetterRepository interface {
	Create(ctx context.Context, deadLetter *models.DeadLetter) error
	GetByWebhookID(ctx context.Context, webhookID uint, limit, offset int) ([]*models.DeadLetter, error)
	GetUnprocessed(ctx context.Context, limit int) ([]*models.DeadLetter, error)
	Update(ctx context.Context, deadLetter *models.DeadLetter) error
	MarkProcessed(ctx context.Context, id uint) error
	Replay(ctx context.Context, id uint) error
}

type Repositories struct {
	Webhook         WebhookRepository
	Subscription    SubscriptionRepository
	DeliveryAttempt DeliveryAttemptRepository
	DeadLetter      DeadLetterRepository
	
	db    *gorm.DB
	redis *redis.Client
}

func New(db *gorm.DB, redis *redis.Client) *Repositories {
	return &Repositories{
		db:              db,
		redis:           redis,
		Webhook:         NewWebhookRepository(db, redis),
		Subscription:    NewSubscriptionRepository(db, redis),
		DeliveryAttempt: NewDeliveryAttemptRepository(db, redis),
		DeadLetter:      NewDeadLetterRepository(db, redis),
	}
}