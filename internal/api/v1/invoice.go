package v1

import (
	"fmt"
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
)

type InvoiceHandler struct {
	invoiceService service.InvoiceService
	logger         *logger.Logger
}

func NewInvoiceHandler(invoiceService service.InvoiceService, logger *logger.Logger) *InvoiceHandler {
	return &InvoiceHandler{
		invoiceService: invoiceService,
		logger:         logger,
	}
}

// CreateInvoice godoc
// @Summary Create a new invoice
// @Description Create a new invoice with the provided details
// @Tags invoices
// @Accept json
// @Produce json
// @Param invoice body dto.CreateInvoiceRequest true "Invoice details"
// @Success 201 {object} dto.InvoiceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/invoices [post]
func (h *InvoiceHandler) CreateInvoice(c *gin.Context) {
	var req dto.CreateInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Errorw("failed to bind request", "error", err)
		NewErrorResponse(c, http.StatusBadRequest, "invalid request", err)
		return
	}

	invoice, err := h.invoiceService.CreateInvoice(c.Request.Context(), req)
	if err != nil {
		h.logger.Errorw("failed to create invoice", "error", err)
		NewErrorResponse(c, http.StatusInternalServerError, "failed to create invoice", err)
		return
	}

	c.JSON(http.StatusCreated, invoice)
}

// GetInvoice godoc
// @Summary Get an invoice by ID
// @Description Get detailed information about an invoice
// @Tags invoices
// @Accept json
// @Produce json
// @Param id path string true "Invoice ID"
// @Success 200 {object} dto.InvoiceResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/invoices/{id} [get]
func (h *InvoiceHandler) GetInvoice(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		NewErrorResponse(c, http.StatusBadRequest, "invalid invoice id", nil)
		return
	}

	invoice, err := h.invoiceService.GetInvoice(c.Request.Context(), id)
	if err != nil {
		h.logger.Errorw("failed to get invoice", "error", err, "invoice_id", id)
		NewErrorResponse(c, http.StatusInternalServerError, "failed to get invoice", err)
		return
	}

	c.JSON(http.StatusOK, invoice)
}

// ListInvoices godoc
// @Summary List invoices
// @Description List invoices with optional filtering
// @Tags invoices
// @Accept json
// @Produce json
// @Param customer_id query string false "Customer ID"
// @Param subscription_id query string false "Subscription ID"
// @Param wallet_id query string false "Wallet ID"
// @Param status query []string false "Invoice statuses"
// @Param start_time query string false "Start time (RFC3339)"
// @Param end_time query string false "End time (RFC3339)"
// @Param limit query int false "Limit"
// @Param offset query int false "Offset"
// @Success 200 {object} dto.ListInvoicesResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/invoices [get]
func (h *InvoiceHandler) ListInvoices(c *gin.Context) {
	filter := &types.InvoiceFilter{
		CustomerID:     c.Query("customer_id"),
		SubscriptionID: c.Query("subscription_id"),
		WalletID:       c.Query("wallet_id"),
	}

	if statuses := c.QueryArray("status"); len(statuses) > 0 {
		filter.Status = make([]types.InvoiceStatus, len(statuses))
		for i, s := range statuses {
			filter.Status[i] = types.InvoiceStatus(s)
		}
	}

	if startTime := c.Query("start_time"); startTime != "" {
		t, err := types.ParseTime(startTime)
		if err != nil {
			NewErrorResponse(c, http.StatusBadRequest, "invalid start time", err)
			return
		}
		filter.StartTime = &t
	}

	if endTime := c.Query("end_time"); endTime != "" {
		t, err := types.ParseTime(endTime)
		if err != nil {
			NewErrorResponse(c, http.StatusBadRequest, "invalid end time", err)
			return
		}
		filter.EndTime = &t
	}

	if limit := c.Query("limit"); limit != "" {
		var l int
		if _, err := fmt.Sscanf(limit, "%d", &l); err != nil {
			NewErrorResponse(c, http.StatusBadRequest, "invalid limit", err)
			return
		}
		filter.Limit = l
	}

	if offset := c.Query("offset"); offset != "" {
		var o int
		if _, err := fmt.Sscanf(offset, "%d", &o); err != nil {
			NewErrorResponse(c, http.StatusBadRequest, "invalid offset", err)
			return
		}
		filter.Offset = o
	}

	response, err := h.invoiceService.ListInvoices(c.Request.Context(), filter)
	if err != nil {
		h.logger.Errorw("failed to list invoices", "error", err)
		NewErrorResponse(c, http.StatusInternalServerError, "failed to list invoices", err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// FinalizeInvoice godoc
// @Summary Finalize an invoice
// @Description Finalize a draft invoice
// @Tags invoices
// @Accept json
// @Produce json
// @Param id path string true "Invoice ID"
// @Success 200
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/invoices/{id}/finalize [post]
func (h *InvoiceHandler) FinalizeInvoice(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		NewErrorResponse(c, http.StatusBadRequest, "invalid invoice id", nil)
		return
	}

	if err := h.invoiceService.FinalizeInvoice(c.Request.Context(), id); err != nil {
		h.logger.Errorw("failed to finalize invoice", "error", err, "invoice_id", id)
		NewErrorResponse(c, http.StatusInternalServerError, "failed to finalize invoice", err)
		return
	}

	c.Status(http.StatusOK)
}

// VoidInvoice godoc
// @Summary Void an invoice
// @Description Void an invoice that hasn't been paid
// @Tags invoices
// @Accept json
// @Produce json
// @Param id path string true "Invoice ID"
// @Success 200
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/invoices/{id}/void [post]
func (h *InvoiceHandler) VoidInvoice(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		NewErrorResponse(c, http.StatusBadRequest, "invalid invoice id", nil)
		return
	}

	if err := h.invoiceService.VoidInvoice(c.Request.Context(), id); err != nil {
		h.logger.Errorw("failed to void invoice", "error", err, "invoice_id", id)
		NewErrorResponse(c, http.StatusInternalServerError, "failed to void invoice", err)
		return
	}

	c.Status(http.StatusOK)
}

// MarkInvoiceAsPaid godoc
// @Summary Mark an invoice as paid
// @Description Mark a finalized invoice as paid with payment intent ID
// @Tags invoices
// @Accept json
// @Produce json
// @Param id path string true "Invoice ID"
// @Param payment_intent_id body string true "Payment Intent ID"
// @Success 200
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/invoices/{id}/mark_paid [post]
func (h *InvoiceHandler) MarkInvoiceAsPaid(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		NewErrorResponse(c, http.StatusBadRequest, "invalid invoice id", nil)
		return
	}

	var req struct {
		PaymentIntentID string `json:"payment_intent_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		NewErrorResponse(c, http.StatusBadRequest, "invalid request", err)
		return
	}

	if err := h.invoiceService.MarkInvoiceAsPaid(c.Request.Context(), id, req.PaymentIntentID); err != nil {
		NewErrorResponse(c, http.StatusInternalServerError, "failed to mark invoice as paid", err)
		return
	}

	c.Status(http.StatusOK)
}
