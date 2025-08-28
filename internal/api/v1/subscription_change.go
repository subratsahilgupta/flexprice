package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/gin-gonic/gin"
)

type SubscriptionChangeHandler struct {
	service subscription.SubscriptionChangeService
	log     *logger.Logger
}

func NewSubscriptionChangeHandler(service subscription.SubscriptionChangeService, log *logger.Logger) *SubscriptionChangeHandler {
	return &SubscriptionChangeHandler{service: service, log: log}
}

// @Summary Upgrade subscription
// @Description Upgrade a subscription to a higher plan immediately
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Subscription ID"
// @Param upgrade body dto.UpgradeSubscriptionRequest true "Upgrade Request"
// @Success 200 {object} dto.SubscriptionPlanChangeResult
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /subscriptions/{id}/upgrade [post]
func (h *SubscriptionChangeHandler) UpgradeSubscription(c *gin.Context) {
	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		c.Error(ierr.NewError("subscription ID is required").
			WithHint("Please provide a valid subscription ID").
			Mark(ierr.ErrValidation))
		return
	}

	var dtoReq dto.UpgradeSubscriptionRequest
	if err := c.ShouldBindJSON(&dtoReq); err != nil {
		h.log.Error("Failed to bind JSON", "error", err)
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	// Convert DTO to domain type
	req := &subscription.UpgradeSubscriptionRequest{
		TargetPlanID:         dtoReq.TargetPlanID,
		ProrationBehavior:    dtoReq.ProrationBehavior,
		EffectiveImmediately: dtoReq.EffectiveImmediately,
		Metadata:             dtoReq.Metadata,
	}

	result, err := h.service.UpgradeSubscription(c.Request.Context(), subscriptionID, req)
	if err != nil {
		h.log.Error("Failed to upgrade subscription", "error", err, "subscription_id", subscriptionID)
		c.Error(err)
		return
	}

	// Convert domain result to DTO
	dtoResult := &dto.SubscriptionPlanChangeResult{
		Subscription:    result.Subscription,
		Invoice:         convertInvoiceToDTO(result.Invoice),
		Schedule:        convertScheduleToDTO(result.Schedule),
		ProrationAmount: result.ProrationAmount,
		ChangeType:      result.ChangeType,
		EffectiveDate:   result.EffectiveDate,
		Metadata:        result.Metadata,
	}

	c.JSON(http.StatusOK, dtoResult)
}

// @Summary Downgrade subscription
// @Description Downgrade a subscription to a lower plan (typically at period end)
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Subscription ID"
// @Param downgrade body dto.DowngradeSubscriptionRequest true "Downgrade Request"
// @Success 200 {object} dto.SubscriptionPlanChangeResult
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /subscriptions/{id}/downgrade [post]
func (h *SubscriptionChangeHandler) DowngradeSubscription(c *gin.Context) {
	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		c.Error(ierr.NewError("subscription ID is required").
			WithHint("Please provide a valid subscription ID").
			Mark(ierr.ErrValidation))
		return
	}

	var dtoReq dto.DowngradeSubscriptionRequest
	if err := c.ShouldBindJSON(&dtoReq); err != nil {
		h.log.Error("Failed to bind JSON", "error", err)
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	// Convert DTO to domain type
	req := &subscription.DowngradeSubscriptionRequest{
		TargetPlanID:         dtoReq.TargetPlanID,
		ProrationBehavior:    dtoReq.ProrationBehavior,
		EffectiveAtPeriodEnd: dtoReq.EffectiveAtPeriodEnd,
		EffectiveDate:        dtoReq.EffectiveDate,
		Metadata:             dtoReq.Metadata,
	}

	result, err := h.service.DowngradeSubscription(c.Request.Context(), subscriptionID, req)
	if err != nil {
		h.log.Error("Failed to downgrade subscription", "error", err, "subscription_id", subscriptionID)
		c.Error(err)
		return
	}

	// Convert domain result to DTO
	dtoResult := &dto.SubscriptionPlanChangeResult{
		Subscription:    result.Subscription,
		Invoice:         convertInvoiceToDTO(result.Invoice),
		Schedule:        convertScheduleToDTO(result.Schedule),
		ProrationAmount: result.ProrationAmount,
		ChangeType:      result.ChangeType,
		EffectiveDate:   result.EffectiveDate,
		Metadata:        result.Metadata,
	}

	c.JSON(http.StatusOK, dtoResult)
}

// @Summary Preview plan change
// @Description Preview the impact of a plan change without executing it
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Subscription ID"
// @Param preview body dto.PreviewPlanChangeRequest true "Preview Request"
// @Success 200 {object} dto.PlanChangePreviewResult
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /subscriptions/{id}/preview-change [post]
func (h *SubscriptionChangeHandler) PreviewPlanChange(c *gin.Context) {
	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		c.Error(ierr.NewError("subscription ID is required").
			WithHint("Please provide a valid subscription ID").
			Mark(ierr.ErrValidation))
		return
	}

	var dtoReq dto.PreviewPlanChangeRequest
	if err := c.ShouldBindJSON(&dtoReq); err != nil {
		h.log.Error("Failed to bind JSON", "error", err)
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	// Convert DTO to domain type
	req := &subscription.PreviewPlanChangeRequest{
		TargetPlanID:      dtoReq.TargetPlanID,
		ProrationBehavior: dtoReq.ProrationBehavior,
		EffectiveDate:     dtoReq.EffectiveDate,
	}

	preview, err := h.service.PreviewPlanChange(c.Request.Context(), subscriptionID, req)
	if err != nil {
		h.log.Error("Failed to preview plan change", "error", err, "subscription_id", subscriptionID)
		c.Error(err)
		return
	}

	// Convert domain result to DTO
	dtoPreview := &dto.PlanChangePreviewResult{
		CurrentAmount:   preview.CurrentAmount,
		NewAmount:       preview.NewAmount,
		ProrationAmount: preview.ProrationAmount,
		EffectiveDate:   preview.EffectiveDate,
		LineItems:       convertLineItemsToDTO(preview.LineItems),
		Taxes:           convertTaxesToDTO(preview.Taxes),
		Coupons:         convertCouponsToDTO(preview.Coupons),
	}

	c.JSON(http.StatusOK, dtoPreview)
}

// @Summary Cancel pending plan change
// @Description Cancel any pending plan changes for a subscription
// @Tags Subscriptions
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Subscription ID"
// @Success 200 {object} gin.H
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /subscriptions/{id}/pending-changes [delete]
func (h *SubscriptionChangeHandler) CancelPendingPlanChange(c *gin.Context) {
	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		c.Error(ierr.NewError("subscription ID is required").
			WithHint("Please provide a valid subscription ID").
			Mark(ierr.ErrValidation))
		return
	}

	err := h.service.CancelPendingPlanChange(c.Request.Context(), subscriptionID)
	if err != nil {
		h.log.Error("Failed to cancel pending plan change", "error", err, "subscription_id", subscriptionID)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Pending plan change cancelled successfully",
		"subscription_id": subscriptionID,
	})
}

// @Summary Get plan change history
// @Description Get the history of plan changes for a subscription
// @Tags Subscriptions
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Subscription ID"
// @Success 200 {array} subscription.PlanChangeAuditLog
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /subscriptions/{id}/change-history [get]
func (h *SubscriptionChangeHandler) GetPlanChangeHistory(c *gin.Context) {
	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		c.Error(ierr.NewError("subscription ID is required").
			WithHint("Please provide a valid subscription ID").
			Mark(ierr.ErrValidation))
		return
	}

	history, err := h.service.GetPlanChangeHistory(c.Request.Context(), subscriptionID)
	if err != nil {
		h.log.Error("Failed to get plan change history", "error", err, "subscription_id", subscriptionID)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"subscription_id": subscriptionID,
		"history":         history,
	})
}

// Helper functions for converting domain types to DTOs

func convertInvoiceToDTO(invoice interface{}) *dto.InvoiceResponse {
	if invoice == nil {
		return nil
	}
	// TODO: Implement proper invoice conversion
	return nil
}

func convertScheduleToDTO(schedule interface{}) *dto.SubscriptionScheduleResponse {
	if schedule == nil {
		return nil
	}
	// TODO: Implement proper schedule conversion
	return nil
}

func convertLineItemsToDTO(lineItems []interface{}) []dto.ProrationLineItemPreview {
	if lineItems == nil {
		return nil
	}
	// TODO: Implement proper line items conversion
	return []dto.ProrationLineItemPreview{}
}

func convertTaxesToDTO(taxes interface{}) *dto.TaxCalculationPreview {
	if taxes == nil {
		return nil
	}
	// TODO: Implement proper taxes conversion
	return nil
}

func convertCouponsToDTO(coupons []interface{}) []dto.CouponImpactPreview {
	if coupons == nil {
		return nil
	}
	// TODO: Implement proper coupons conversion
	return []dto.CouponImpactPreview{}
}
