package services

import (
	"log/slog"

	"github.com/Reg-Kris/pyairtable-webhook-service/internal/config"
	"github.com/Reg-Kris/pyairtable-webhook-service/internal/repositories"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type Services struct {
	Webhook      *WebhookService
	Subscription *SubscriptionService
	DeadLetter   *DeadLetterService
	Delivery     *DeliveryService
	Security     *SecurityService
	Dispatcher   *EventDispatcher
	Listener     *EventListener
	
	config *config.Config
	logger *slog.Logger
	repos  *repositories.Repositories
}

func New(repos *repositories.Repositories, redis *redis.Client, config *config.Config, logger *slog.Logger) *Services {
	// Convert slog.Logger to zap.Logger for services that need it
	zapLogger, err := zap.NewProduction()
	if err != nil {
		// Fallback to development logger
		zapLogger, _ = zap.NewDevelopment()
	}
	
	// Initialize core services
	security := NewSecurityService()
	delivery := NewDeliveryService(repos, security, redis, zapLogger)
	dispatcher := NewEventDispatcher(repos, delivery, zapLogger)
	listener := NewEventListener(redis, dispatcher, zapLogger)
	
	return &Services{
		config:       config,
		logger:       logger,
		repos:        repos,
		Security:     security,
		Delivery:     delivery,
		Dispatcher:   dispatcher,
		Listener:     listener,
		Webhook:      NewWebhookService(repos, security, zapLogger),
		Subscription: NewSubscriptionService(repos, zapLogger),
		DeadLetter:   NewDeadLetterService(repos, zapLogger),
	}
}