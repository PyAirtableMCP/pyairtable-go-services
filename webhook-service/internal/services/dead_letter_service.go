package services

import (
	"context"
	"fmt"

	"github.com/Reg-Kris/pyairtable-webhook-service/internal/models"
	"github.com/Reg-Kris/pyairtable-webhook-service/internal/repositories"
	"go.uber.org/zap"
)

// DeadLetterService handles dead letter queue operations
type DeadLetterService struct {
	repos  *repositories.Repositories
	logger *zap.Logger
}

// NewDeadLetterService creates a new dead letter service
func NewDeadLetterService(repos *repositories.Repositories, logger *zap.Logger) *DeadLetterService {
	return &DeadLetterService{
		repos:  repos,
		logger: logger,
	}
}

// GetDeadLetters retrieves dead letters for a webhook
func (s *DeadLetterService) GetDeadLetters(ctx context.Context, webhookID, userID uint, limit, offset int) ([]*models.DeadLetter, error) {
	// Verify webhook ownership
	webhook, err := s.repos.Webhook.GetByID(ctx, webhookID)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook: %w", err)
	}
	
	if webhook == nil || webhook.UserID != userID {
		return nil, fmt.Errorf("webhook not found or access denied")
	}
	
	return s.repos.DeadLetter.GetByWebhookID(ctx, webhookID, limit, offset)
}

// GetAllDeadLetters retrieves all dead letters for a user
func (s *DeadLetterService) GetAllDeadLetters(ctx context.Context, userID uint, limit, offset int) ([]*models.DeadLetter, error) {
	// Get all webhooks for the user first
	webhooks, err := s.repos.Webhook.GetByUserID(ctx, userID, 0, 0) // Get all webhooks
	if err != nil {
		return nil, fmt.Errorf("failed to get user webhooks: %w", err)
	}
	
	var allDeadLetters []*models.DeadLetter
	
	// Get dead letters for each webhook
	for _, webhook := range webhooks {
		deadLetters, err := s.repos.DeadLetter.GetByWebhookID(ctx, webhook.ID, 0, 0) // Get all for each webhook
		if err != nil {
			s.logger.Error("Failed to get dead letters for webhook", zap.Error(err), zap.Uint("webhook_id", webhook.ID))
			continue
		}
		allDeadLetters = append(allDeadLetters, deadLetters...)
	}
	
	// Apply pagination manually (in production, this should be done at DB level)
	total := len(allDeadLetters)
	if offset >= total {
		return []*models.DeadLetter{}, nil
	}
	
	end := offset + limit
	if end > total {
		end = total
	}
	
	return allDeadLetters[offset:end], nil
}

// ReplayDeadLetter replays a single dead letter
func (s *DeadLetterService) ReplayDeadLetter(ctx context.Context, deadLetterID, webhookID, userID uint) error {
	// Verify webhook ownership
	webhook, err := s.repos.Webhook.GetByID(ctx, webhookID)
	if err != nil {
		return fmt.Errorf("failed to get webhook: %w", err)
	}
	
	if webhook == nil || webhook.UserID != userID {
		return fmt.Errorf("webhook not found or access denied")
	}
	
	// Get dead letters for the webhook to verify the dead letter belongs to it
	deadLetters, err := s.repos.DeadLetter.GetByWebhookID(ctx, webhookID, 0, 0)
	if err != nil {
		return fmt.Errorf("failed to get dead letters: %w", err)
	}
	
	var targetDeadLetter *models.DeadLetter
	for _, dl := range deadLetters {
		if dl.ID == deadLetterID {
			targetDeadLetter = dl
			break
		}
	}
	
	if targetDeadLetter == nil {
		return fmt.Errorf("dead letter not found")
	}
	
	if targetDeadLetter.IsProcessed {
		return fmt.Errorf("dead letter has already been processed")
	}
	
	// Replay the dead letter
	if err := s.repos.DeadLetter.Replay(ctx, deadLetterID); err != nil {
		return fmt.Errorf("failed to replay dead letter: %w", err)
	}
	
	s.logger.Info("Replayed dead letter",
		zap.Uint("dead_letter_id", deadLetterID),
		zap.Uint("webhook_id", webhookID),
		zap.String("event_id", targetDeadLetter.EventID))
	
	return nil
}

// BulkReplayResult represents the result of a bulk replay operation
type BulkReplayResult struct {
	DeadLetterID uint   `json:"dead_letter_id"`
	Success      bool   `json:"success"`
	Error        string `json:"error,omitempty"`
}

// BulkReplayDeadLetters replays multiple dead letters
func (s *DeadLetterService) BulkReplayDeadLetters(ctx context.Context, deadLetterIDs []uint, webhookID, userID uint) ([]BulkReplayResult, error) {
	// Verify webhook ownership
	webhook, err := s.repos.Webhook.GetByID(ctx, webhookID)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook: %w", err)
	}
	
	if webhook == nil || webhook.UserID != userID {
		return nil, fmt.Errorf("webhook not found or access denied")
	}
	
	// Get dead letters for the webhook
	deadLetters, err := s.repos.DeadLetter.GetByWebhookID(ctx, webhookID, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get dead letters: %w", err)
	}
	
	// Create a map for quick lookup
	deadLetterMap := make(map[uint]*models.DeadLetter)
	for _, dl := range deadLetters {
		deadLetterMap[dl.ID] = dl
	}
	
	results := make([]BulkReplayResult, 0, len(deadLetterIDs))
	
	// Process each dead letter ID
	for _, deadLetterID := range deadLetterIDs {
		result := BulkReplayResult{
			DeadLetterID: deadLetterID,
		}
		
		// Check if dead letter exists and belongs to the webhook
		deadLetter, exists := deadLetterMap[deadLetterID]
		if !exists {
			result.Error = "dead letter not found"
			results = append(results, result)
			continue
		}
		
		if deadLetter.IsProcessed {
			result.Error = "dead letter already processed"
			results = append(results, result)
			continue
		}
		
		// Attempt replay
		if err := s.repos.DeadLetter.Replay(ctx, deadLetterID); err != nil {
			result.Error = fmt.Sprintf("replay failed: %v", err)
			s.logger.Error("Failed to replay dead letter in bulk operation",
				zap.Error(err),
				zap.Uint("dead_letter_id", deadLetterID))
		} else {
			result.Success = true
			s.logger.Info("Replayed dead letter in bulk operation",
				zap.Uint("dead_letter_id", deadLetterID),
				zap.String("event_id", deadLetter.EventID))
		}
		
		results = append(results, result)
	}
	
	// Log summary
	successCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		}
	}
	
	s.logger.Info("Completed bulk replay operation",
		zap.Int("total", len(deadLetterIDs)),
		zap.Int("successful", successCount),
		zap.Int("failed", len(deadLetterIDs)-successCount),
		zap.Uint("webhook_id", webhookID))
	
	return results, nil
}