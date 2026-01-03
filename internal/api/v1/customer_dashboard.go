package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/gin-gonic/gin"
)

// CustomerDashboardHandler handles customer dashboard API requests
type CustomerDashboardHandler struct {
	dashboardService service.CustomerDashboardService
	log              *logger.Logger
}

// NewCustomerDashboardHandler creates a new customer dashboard handler
func NewCustomerDashboardHandler(
	dashboardService service.CustomerDashboardService,
	log *logger.Logger,
) *CustomerDashboardHandler {
	return &CustomerDashboardHandler{
		dashboardService: dashboardService,
		log:              log,
	}
}

// CreateSession creates a dashboard session for a customer
// @Summary Create a customer dashboard session
// @Description Generate a dashboard URL/token for a customer to access their billing information
// @Tags CustomerDashboard
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body dto.CreateDashboardSessionRequest true "Dashboard session request"
// @Success 200 {object} dto.DashboardSessionResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customer-dashboard/session [post]
func (h *CustomerDashboardHandler) CreateSession(c *gin.Context) {
	var req dto.CreateDashboardSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).Mark(ierr.ErrValidation))
		return
	}

	response, err := h.dashboardService.CreateDashboardSession(c.Request.Context(), req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetCustomer returns the current customer's information
// @Summary Get customer information
// @Description Get the authenticated customer's information
// @Tags CustomerDashboard
// @Produce json
// @Security BearerAuth
// @Success 200 {object} dto.CustomerResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customer-dashboard/info [get]
func (h *CustomerDashboardHandler) GetCustomer(c *gin.Context) {
	response, err := h.dashboardService.GetCustomer(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// UpdateCustomer updates the current customer's information
// @Summary Update customer information
// @Description Update the authenticated customer's profile information
// @Tags CustomerDashboard
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.UpdateCustomerRequest true "Update customer request"
// @Success 200 {object} dto.CustomerResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customer-dashboard/info [put]
func (h *CustomerDashboardHandler) UpdateCustomer(c *gin.Context) {
	var req dto.UpdateCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).Mark(ierr.ErrValidation))
		return
	}

	response, err := h.dashboardService.UpdateCustomer(c.Request.Context(), req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetSubscriptions returns the customer's subscriptions
// @Summary Get customer subscriptions
// @Description Get all subscriptions for the authenticated customer
// @Tags CustomerDashboard
// @Produce json
// @Security BearerAuth
// @Success 200 {object} dto.ListSubscriptionsResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customer-dashboard/subscriptions [get]
func (h *CustomerDashboardHandler) GetSubscriptions(c *gin.Context) {
	var req dto.DashboardPaginatedRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.Error(ierr.WithError(err).Mark(ierr.ErrValidation))
		return
	}

	response, err := h.dashboardService.GetSubscriptions(c.Request.Context(), req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetInvoices returns the customer's invoices
// @Summary Get customer invoices
// @Description Get all invoices for the authenticated customer
// @Tags CustomerDashboard
// @Produce json
// @Security BearerAuth
// @Success 200 {object} dto.ListInvoicesResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customer-dashboard/invoices [get]
func (h *CustomerDashboardHandler) GetInvoices(c *gin.Context) {
	var req dto.DashboardPaginatedRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.Error(ierr.WithError(err).Mark(ierr.ErrValidation))
		return
	}

	response, err := h.dashboardService.GetInvoices(c.Request.Context(), req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetInvoice returns a specific invoice
// @Summary Get invoice by ID
// @Description Get a specific invoice for the authenticated customer
// @Tags CustomerDashboard
// @Produce json
// @Security BearerAuth
// @Param id path string true "Invoice ID"
// @Success 200 {object} dto.InvoiceResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customer-dashboard/invoices/{id} [get]
func (h *CustomerDashboardHandler) GetInvoice(c *gin.Context) {
	invoiceID := c.Param("id")
	if invoiceID == "" {
		c.Error(ierr.NewError("invoice_id is required").Mark(ierr.ErrValidation))
		return
	}

	response, err := h.dashboardService.GetInvoice(c.Request.Context(), invoiceID)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetSubscription returns a specific subscription
// @Summary Get subscription by ID
// @Description Get a specific subscription for the authenticated customer
// @Tags CustomerDashboard
// @Produce json
// @Security BearerAuth
// @Param id path string true "Subscription ID"
// @Success 200 {object} dto.SubscriptionResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customer-dashboard/subscriptions/{id} [get]
func (h *CustomerDashboardHandler) GetSubscription(c *gin.Context) {
	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		c.Error(ierr.NewError("subscription_id is required").Mark(ierr.ErrValidation))
		return
	}

	response, err := h.dashboardService.GetSubscription(c.Request.Context(), subscriptionID)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetWallets returns the customer's wallet balances
// @Summary Get customer wallets
// @Description Get all wallet balances for the authenticated customer
// @Tags CustomerDashboard
// @Produce json
// @Security BearerAuth
// @Success 200 {array} dto.WalletBalanceResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customer-dashboard/wallets [get]
func (h *CustomerDashboardHandler) GetWallets(c *gin.Context) {
	response, err := h.dashboardService.GetWallets(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetWallet returns a specific wallet
// @Summary Get wallet by ID
// @Description Get a specific wallet for the authenticated customer
// @Tags CustomerDashboard
// @Produce json
// @Security BearerAuth
// @Param id path string true "Wallet ID"
// @Success 200 {object} dto.WalletBalanceResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customer-dashboard/wallets/{id} [get]
func (h *CustomerDashboardHandler) GetWallet(c *gin.Context) {
	walletID := c.Param("id")
	if walletID == "" {
		c.Error(ierr.NewError("wallet_id is required").Mark(ierr.ErrValidation))
		return
	}

	response, err := h.dashboardService.GetWallet(c.Request.Context(), walletID)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetAnalytics returns usage analytics for the customer
// @Summary Get customer analytics
// @Description Get usage analytics for the authenticated customer
// @Tags CustomerDashboard
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.DashboardAnalyticsRequest true "Analytics request"
// @Success 200 {object} dto.GetUsageAnalyticsResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customer-dashboard/analytics [post]
func (h *CustomerDashboardHandler) GetAnalytics(c *gin.Context) {
	var req dto.DashboardAnalyticsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).Mark(ierr.ErrValidation))
		return
	}

	response, err := h.dashboardService.GetAnalytics(c.Request.Context(), req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetCostAnalytics returns cost analytics for the customer
// @Summary Get customer cost analytics
// @Description Get cost analytics for the authenticated customer
// @Tags CustomerDashboard
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.DashboardCostAnalyticsRequest true "Cost analytics request"
// @Success 200 {object} dto.GetDetailedCostAnalyticsResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customer-dashboard/cost-analytics [post]
func (h *CustomerDashboardHandler) GetCostAnalytics(c *gin.Context) {
	var req dto.DashboardCostAnalyticsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).Mark(ierr.ErrValidation))
		return
	}

	response, err := h.dashboardService.GetCostAnalytics(c.Request.Context(), req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}
