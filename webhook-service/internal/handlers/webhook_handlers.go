package handlers

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/Reg-Kris/pyairtable-webhook-service/internal/models"
)

// CreateWebhookRequest represents the request to create a webhook
type CreateWebhookRequest struct {
	Name           string                 `json:"name" validate:"required,min=1,max=255"`
	URL            string                 `json:"url" validate:"required,url,max=2048"`
	Description    string                 `json:"description,omitempty"`
	WorkspaceID    *uint                  `json:"workspace_id,omitempty"`
	MaxRetries     int                    `json:"max_retries,omitempty"`
	TimeoutSeconds int                    `json:"timeout_seconds,omitempty"`
	RateLimitRPS   int                    `json:"rate_limit_rps,omitempty"`
	Headers        map[string]interface{} `json:"headers,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	Subscriptions  []SubscriptionRequest  `json:"subscriptions,omitempty"`
}

// UpdateWebhookRequest represents the request to update a webhook
type UpdateWebhookRequest struct {
	Name           *string                `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	URL            *string                `json:"url,omitempty" validate:"omitempty,url,max=2048"`
	Description    *string                `json:"description,omitempty"`
	Status         *models.WebhookStatus  `json:"status,omitempty"`
	MaxRetries     *int                   `json:"max_retries,omitempty"`
	TimeoutSeconds *int                   `json:"timeout_seconds,omitempty"`
	RateLimitRPS   *int                   `json:"rate_limit_rps,omitempty"`
	Headers        map[string]interface{} `json:"headers,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// SubscriptionRequest represents a subscription within a webhook request
type SubscriptionRequest struct {
	EventType  models.EventType       `json:"event_type" validate:"required"`
	ResourceID *string                `json:"resource_id,omitempty"`
	IsActive   *bool                  `json:"is_active,omitempty"`
	Filters    map[string]interface{} `json:"filters,omitempty"`
}

// CreateWebhook handles POST /webhooks
func (h *Handlers) CreateWebhook(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	var req CreateWebhookRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}
	
	// TODO: Add validation using a validator library
	
	webhook, err := h.services.Webhook.CreateWebhook(c.Context(), userID, &req)
	if err != nil {
		h.logger.Error("Failed to create webhook", "error", err, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create webhook")
	}
	
	return c.Status(fiber.StatusCreated).JSON(webhook)
}

// GetWebhooks handles GET /webhooks
func (h *Handlers) GetWebhooks(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	workspaceID, err := h.getWorkspaceIDFromQuery(c)
	if err != nil {
		return err
	}
	
	limit, offset := h.getPaginationParams(c)
	
	webhooks, err := h.services.Webhook.GetWebhooks(c.Context(), userID, workspaceID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to get webhooks", "error", err, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to get webhooks")
	}
	
	return c.JSON(fiber.Map{
		"webhooks": webhooks,
		"pagination": fiber.Map{
			"limit":  limit,
			"offset": offset,
		},
	})
}

// GetWebhook handles GET /webhooks/:id
func (h *Handlers) GetWebhook(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	webhookID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	
	webhook, err := h.services.Webhook.GetWebhook(c.Context(), uint(webhookID), userID)
	if err != nil {
		h.logger.Error("Failed to get webhook", "error", err, "webhook_id", webhookID, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to get webhook")
	}
	
	if webhook == nil {
		return fiber.NewError(fiber.StatusNotFound, "Webhook not found")
	}
	
	return c.JSON(webhook)
}

// UpdateWebhook handles PUT /webhooks/:id
func (h *Handlers) UpdateWebhook(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	webhookID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	
	var req UpdateWebhookRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}
	
	webhook, err := h.services.Webhook.UpdateWebhook(c.Context(), uint(webhookID), userID, &req)
	if err != nil {
		h.logger.Error("Failed to update webhook", "error", err, "webhook_id", webhookID, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to update webhook")
	}
	
	if webhook == nil {
		return fiber.NewError(fiber.StatusNotFound, "Webhook not found")
	}
	
	return c.JSON(webhook)
}

// DeleteWebhook handles DELETE /webhooks/:id
func (h *Handlers) DeleteWebhook(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	webhookID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	
	err = h.services.Webhook.DeleteWebhook(c.Context(), uint(webhookID), userID)
	if err != nil {
		h.logger.Error("Failed to delete webhook", "error", err, "webhook_id", webhookID, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete webhook")
	}
	
	return c.Status(fiber.StatusNoContent).Send(nil)
}

// GetWebhookDeliveries handles GET /webhooks/:id/deliveries
func (h *Handlers) GetWebhookDeliveries(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	webhookID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	
	limit, offset := h.getPaginationParams(c)
	
	deliveries, err := h.services.Webhook.GetWebhookDeliveries(c.Context(), uint(webhookID), userID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to get webhook deliveries", "error", err, "webhook_id", webhookID, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to get webhook deliveries")
	}
	
	return c.JSON(fiber.Map{
		"deliveries": deliveries,
		"pagination": fiber.Map{
			"limit":  limit,
			"offset": offset,
		},
	})
}

// GetWebhookMetrics handles GET /webhooks/:id/metrics
func (h *Handlers) GetWebhookMetrics(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	webhookID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	
	// Get time range from query params (default to last 24 hours)
	daysStr := c.Query("days", "1")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days < 1 || days > 365 {
		days = 1
	}
	
	metrics, err := h.services.Webhook.GetWebhookMetrics(c.Context(), uint(webhookID), userID, days)
	if err != nil {
		h.logger.Error("Failed to get webhook metrics", "error", err, "webhook_id", webhookID, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to get webhook metrics")
	}
	
	return c.JSON(metrics)
}

// TestWebhook handles POST /webhooks/:id/test
func (h *Handlers) TestWebhook(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	webhookID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	
	var payload map[string]interface{}
	if err := c.BodyParser(&payload); err != nil {
		// Use default test payload if none provided
		payload = map[string]interface{}{
			"test": true,
			"message": "This is a test webhook delivery",
		}
	}
	
	result, err := h.services.Webhook.TestWebhook(c.Context(), uint(webhookID), userID, payload)
	if err != nil {
		h.logger.Error("Failed to test webhook", "error", err, "webhook_id", webhookID, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to test webhook")
	}
	
	return c.JSON(result)
}

// RegenerateSecret handles POST /webhooks/:id/regenerate-secret
func (h *Handlers) RegenerateSecret(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	webhookID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	
	newSecret, err := h.services.Webhook.RegenerateSecret(c.Context(), uint(webhookID), userID)
	if err != nil {
		h.logger.Error("Failed to regenerate webhook secret", "error", err, "webhook_id", webhookID, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to regenerate secret")
	}
	
	return c.JSON(fiber.Map{
		"secret": newSecret,
		"message": fmt.Sprintf("Secret regenerated for webhook %d. Please update your webhook endpoint to use the new secret.", webhookID),
	})
}