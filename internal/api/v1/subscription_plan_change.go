package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/gin-gonic/gin"
)

// Plan change v2 — swap in place. Preview and execute take the identical request
// type and differ only in whether anything is written, so a quote and its
// execution cannot drift apart.
//
// The handlers below do only what a handler should: read the path param, bind
// the body, delegate, respond. Every decision lives in the service.

// @Summary Preview a plan change (v2, swap in place)
// @ID previewSubscriptionPlanChangeV2
// @Description Shows what changing this subscription's plan would do — line items sliced, addon dispositions, and the money that would move — without writing anything. Unlike the v1 endpoint the subscription is swapped in place, so its id, billing anchor and period bounds are preserved.
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @x-scope "read"
// @Param id path string true "Subscription ID"
// @Param request body dto.SubscriptionChangeV2Request true "Plan change request"
// @Success 200 {object} dto.SubscriptionChangeV2Response
// @Failure 400 {object} ierr.ErrorResponse "Invalid request, or a change this endpoint cannot make (interval, currency, hierarchy, phases, paused)"
// @Failure 404 {object} ierr.ErrorResponse "Subscription or plan not found"
// @Failure 500 {object} ierr.ErrorResponse "Server error"
// @Router /subscriptions/{id}/change/v2/preview [post]
func (h *SubscriptionHandler) PreviewSubscriptionPlanChangeV2(c *gin.Context) {
	subscriptionID, req, ok := bindPlanChangeV2Request(c)
	if !ok {
		return
	}

	resp, err := h.service.PreviewPlanChange(c.Request.Context(), subscriptionID, req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Execute a plan change (v2, swap in place)
// @ID executeSubscriptionPlanChangeV2
// @Description Moves the subscription to a different plan in place. The subscription row survives with its id, billing anchor and period bounds intact; plan line items are sliced at the effective date and settled on a single invoice. Every write happens in one transaction.
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @x-scope "write"
// @Param id path string true "Subscription ID"
// @Param request body dto.SubscriptionChangeV2Request true "Plan change request"
// @Success 200 {object} dto.SubscriptionChangeV2Response
// @Failure 400 {object} ierr.ErrorResponse "Invalid request, or a change this endpoint cannot make (interval, currency, hierarchy, phases, paused)"
// @Failure 404 {object} ierr.ErrorResponse "Subscription or plan not found"
// @Failure 500 {object} ierr.ErrorResponse "Server error"
// @Router /subscriptions/{id}/change/v2/execute [post]
func (h *SubscriptionHandler) ExecuteSubscriptionPlanChangeV2(c *gin.Context) {
	subscriptionID, req, ok := bindPlanChangeV2Request(c)
	if !ok {
		return
	}

	resp, err := h.service.ExecutePlanChange(c.Request.Context(), subscriptionID, req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// bindPlanChangeV2Request is shared so preview and execute cannot diverge in how
// they read the same request.
func bindPlanChangeV2Request(c *gin.Context) (string, dto.SubscriptionChangeV2Request, bool) {
	var req dto.SubscriptionChangeV2Request

	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		c.Error(ierr.NewError("subscription ID is required").
			WithHint("Please provide a valid subscription ID").
			Mark(ierr.ErrValidation))
		return "", req, false
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return "", req, false
	}

	return subscriptionID, req, true
}
