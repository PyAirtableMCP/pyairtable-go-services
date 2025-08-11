package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// GetDeadLetters handles GET /webhooks/:webhook_id/dead-letters
func (h *Handlers) GetDeadLetters(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	webhookID, err := strconv.ParseUint(c.Params("webhook_id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	
	limit, offset := h.getPaginationParams(c)
	
	deadLetters, err := h.services.DeadLetter.GetDeadLetters(c.Context(), uint(webhookID), userID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to get dead letters", "error", err, "webhook_id", webhookID, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to get dead letters")
	}
	
	return c.JSON(fiber.Map{
		"dead_letters": deadLetters,
		"pagination": fiber.Map{
			"limit":  limit,
			"offset": offset,
		},
	})
}

// ReplayDeadLetter handles POST /webhooks/:webhook_id/dead-letters/:id/replay
func (h *Handlers) ReplayDeadLetter(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	webhookID, err := strconv.ParseUint(c.Params("webhook_id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	
	deadLetterID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid dead letter ID")
	}
	
	err = h.services.DeadLetter.ReplayDeadLetter(c.Context(), uint(deadLetterID), uint(webhookID), userID)
	if err != nil {
		h.logger.Error("Failed to replay dead letter", "error", err, "dead_letter_id", deadLetterID, "webhook_id", webhookID, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to replay dead letter")
	}
	
	return c.JSON(fiber.Map{
		"message": "Dead letter replayed successfully",
	})
}

// BulkReplayDeadLetters handles POST /webhooks/:webhook_id/dead-letters/bulk-replay
func (h *Handlers) BulkReplayDeadLetters(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	webhookID, err := strconv.ParseUint(c.Params("webhook_id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	
	var req struct {
		DeadLetterIDs []uint `json:"dead_letter_ids" validate:"required,min=1,max=100"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}
	
	if len(req.DeadLetterIDs) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "At least one dead letter ID is required")
	}
	
	if len(req.DeadLetterIDs) > 100 {
		return fiber.NewError(fiber.StatusBadRequest, "Cannot replay more than 100 dead letters at once")
	}
	
	results, err := h.services.DeadLetter.BulkReplayDeadLetters(c.Context(), req.DeadLetterIDs, uint(webhookID), userID)
	if err != nil {
		h.logger.Error("Failed to bulk replay dead letters", "error", err, "webhook_id", webhookID, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to bulk replay dead letters")
	}
	
	return c.JSON(fiber.Map{
		"results": results,
	})
}

// GetAllDeadLetters handles GET /dead-letters (admin endpoint for all dead letters)
func (h *Handlers) GetAllDeadLetters(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	limit, offset := h.getPaginationParams(c)
	
	deadLetters, err := h.services.DeadLetter.GetAllDeadLetters(c.Context(), userID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to get all dead letters", "error", err, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to get dead letters")
	}
	
	return c.JSON(fiber.Map{
		"dead_letters": deadLetters,
		"pagination": fiber.Map{
			"limit":  limit,
			"offset": offset,
		},
	})
}