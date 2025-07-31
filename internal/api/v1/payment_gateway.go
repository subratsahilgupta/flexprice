package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/gin-gonic/gin"
)

type PaymentGatewayHandler struct {
	paymentService service.PaymentService
	log            *logger.Logger
}

func NewPaymentGatewayHandler(paymentService service.PaymentService, log *logger.Logger) *PaymentGatewayHandler {
	return &PaymentGatewayHandler{
		paymentService: paymentService,
		log:            log,
	}
}

// CreatePaymentLink godoc
// @Summary Create a payment link using any gateway
// @Description Create a payment link for an invoice using the specified or preferred payment gateway
// @Tags Payment Gateway
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param payment_link body dto.CreatePaymentLinkRequest true "Payment link configuration"
// @Success 201 {object} dto.PaymentLinkResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /payments/link [post]
func (h *PaymentGatewayHandler) CreatePaymentLink(c *gin.Context) {
	var req dto.CreatePaymentLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Error("Failed to bind JSON", "error", err)
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	if err := req.Validate(); err != nil {
		h.log.Error("Failed to validate request", "error", err)
		c.Error(err)
		return
	}

	resp, err := h.paymentService.CreatePaymentLink(c.Request.Context(), &req)
	if err != nil {
		h.log.Error("Failed to create payment link", "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, resp)
}

// GetPaymentStatus godoc
// @Summary Get payment status from any gateway
// @Description Get the payment status from any payment gateway by session ID
// @Tags Payment Gateway
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param session_id path string true "Session ID"
// @Success 200 {object} dto.GenericPaymentStatusResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /payments/status/{session_id} [get]
func (h *PaymentGatewayHandler) GetPaymentStatus(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.Error(ierr.NewError("session_id is required").
			WithHint("Session ID is required").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.paymentService.GetPaymentStatus(c.Request.Context(), sessionID)
	if err != nil {
		h.log.Error("Failed to get payment status", "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetSupportedGateways godoc
// @Summary Get supported payment gateways
// @Description Get all supported payment gateways for the environment
// @Tags Payment Gateway
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} dto.GetSupportedGatewaysResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /payments/gateways [get]
func (h *PaymentGatewayHandler) GetSupportedGateways(c *gin.Context) {
	resp, err := h.paymentService.GetSupportedGateways(c.Request.Context())
	if err != nil {
		h.log.Error("Failed to get supported gateways", "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}
