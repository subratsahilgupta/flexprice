package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/auth"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/gin-gonic/gin"
)

// CustomerDashboardHandler handles customer dashboard API requests
type CustomerDashboardHandler struct {
	dashboardService service.CustomerDashboardService
	authProvider     auth.Provider
	log              *logger.Logger
}

// NewCustomerDashboardHandler creates a new customer dashboard handler
func NewCustomerDashboardHandler(
	dashboardService service.CustomerDashboardService,
	authProvider auth.Provider,
	log *logger.Logger,
) *CustomerDashboardHandler {
	return &CustomerDashboardHandler{
		dashboardService: dashboardService,
		authProvider:     authProvider,
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
// @Router /customer-dashboard/me [get]
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
// @Router /customer-dashboard/me [put]
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
	response, err := h.dashboardService.GetSubscriptions(c.Request.Context())
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

	response, err := h.dashboardService.GetInvoices(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}
