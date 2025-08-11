package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/Reg-Kris/pyairtable-webhook-service/internal/models"
)

// CreateSubscriptionRequest represents the request to create a subscription
type CreateSubscriptionRequest struct {
	EventType  models.EventType       `json:"event_type" validate:"required"`
	ResourceID *string                `json:"resource_id,omitempty"`
	IsActive   *bool                  `json:"is_active,omitempty"`
	Filters    map[string]interface{} `json:"filters,omitempty"`
}

// UpdateSubscriptionRequest represents the request to update a subscription
type UpdateSubscriptionRequest struct {
	EventType  *models.EventType      `json:"event_type,omitempty"`
	ResourceID *string                `json:"resource_id,omitempty"`
	IsActive   *bool                  `json:"is_active,omitempty"`
	Filters    map[string]interface{} `json:"filters,omitempty"`
}

// CreateSubscription handles POST /webhooks/:webhook_id/subscriptions
func (h *Handlers) CreateSubscription(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	webhookID, err := strconv.ParseUint(c.Params("webhook_id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	
	var req CreateSubscriptionRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}
	
	// TODO: Add validation using a validator library
	
	subscription, err := h.services.Subscription.CreateSubscription(c.Context(), uint(webhookID), userID, &req)
	if err != nil {
		h.logger.Error("Failed to create subscription", "error", err, "webhook_id", webhookID, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create subscription")
	}
	
	return c.Status(fiber.StatusCreated).JSON(subscription)
}

// GetSubscriptions handles GET /webhooks/:webhook_id/subscriptions
func (h *Handlers) GetSubscriptions(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	webhookID, err := strconv.ParseUint(c.Params("webhook_id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	
	subscriptions, err := h.services.Subscription.GetSubscriptions(c.Context(), uint(webhookID), userID)
	if err != nil {
		h.logger.Error("Failed to get subscriptions", "error", err, "webhook_id", webhookID, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to get subscriptions")
	}
	
	return c.JSON(fiber.Map{
		"subscriptions": subscriptions,
	})
}

// GetSubscription handles GET /webhooks/:webhook_id/subscriptions/:id
func (h *Handlers) GetSubscription(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	webhookID, err := strconv.ParseUint(c.Params("webhook_id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	
	subscriptionID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid subscription ID")
	}
	
	subscription, err := h.services.Subscription.GetSubscription(c.Context(), uint(subscriptionID), uint(webhookID), userID)
	if err != nil {
		h.logger.Error("Failed to get subscription", "error", err, "subscription_id", subscriptionID, "webhook_id", webhookID, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to get subscription")
	}
	
	if subscription == nil {
		return fiber.NewError(fiber.StatusNotFound, "Subscription not found")
	}
	
	return c.JSON(subscription)
}

// UpdateSubscription handles PUT /webhooks/:webhook_id/subscriptions/:id
func (h *Handlers) UpdateSubscription(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	webhookID, err := strconv.ParseUint(c.Params("webhook_id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	
	subscriptionID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid subscription ID")
	}
	
	var req UpdateSubscriptionRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}
	
	subscription, err := h.services.Subscription.UpdateSubscription(c.Context(), uint(subscriptionID), uint(webhookID), userID, &req)
	if err != nil {
		h.logger.Error("Failed to update subscription", "error", err, "subscription_id", subscriptionID, "webhook_id", webhookID, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to update subscription")
	}
	
	if subscription == nil {
		return fiber.NewError(fiber.StatusNotFound, "Subscription not found")
	}
	
	return c.JSON(subscription)
}

// DeleteSubscription handles DELETE /webhooks/:webhook_id/subscriptions/:id
func (h *Handlers) DeleteSubscription(c fiber.Ctx) error {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		return err
	}
	
	webhookID, err := strconv.ParseUint(c.Params("webhook_id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid webhook ID")
	}
	
	subscriptionID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid subscription ID")
	}
	
	err = h.services.Subscription.DeleteSubscription(c.Context(), uint(subscriptionID), uint(webhookID), userID)
	if err != nil {
		h.logger.Error("Failed to delete subscription", "error", err, "subscription_id", subscriptionID, "webhook_id", webhookID, "user_id", userID)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete subscription")
	}
	
	return c.Status(fiber.StatusNoContent).Send(nil)
}

// GetAvailableEventTypes handles GET /event-types
func (h *Handlers) GetAvailableEventTypes(c fiber.Ctx) error {
	eventTypes := []models.EventType{
		models.EventTypeTableCreated,
		models.EventTypeTableUpdated,
		models.EventTypeTableDeleted,
		models.EventTypeRecordCreated,
		models.EventTypeRecordUpdated,
		models.EventTypeRecordDeleted,
		models.EventTypeWorkspaceCreated,
		models.EventTypeWorkspaceUpdated,
		models.EventTypeUserInvited,
		models.EventTypeUserJoined,
	}
	
	return c.JSON(fiber.Map{
		"event_types": eventTypes,
	})
}