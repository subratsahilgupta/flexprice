package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

type CouponHandler struct {
	couponService service.CouponService
	logger        *logger.Logger
}

func NewCouponHandler(couponService service.CouponService, logger *logger.Logger) *CouponHandler {
	return &CouponHandler{
		couponService: couponService,
		logger:        logger,
	}
}

// @Summary Create a new coupon
// @Description Creates a new coupon
// @Tags Coupons
// @Accept json
// @Produce json
// @Param coupon body dto.CreateCouponRequest true "Coupon request"
// @Success 201 {object} dto.CouponResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 403 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /coupons [post]
// @Security ApiKeyAuth
func (h *CouponHandler) CreateCoupon(c *gin.Context) {
	var req dto.CreateCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	response, err := h.couponService.CreateCoupon(c.Request.Context(), req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, response)
}

// @Summary Get a coupon by ID
// @Description Retrieves a coupon by ID
// @Tags Coupons
// @Accept json
// @Produce json
// @Param id path string true "Coupon ID"
// @Success 200 {object} dto.CouponResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 403 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /coupons/{id} [get]
// @Security ApiKeyAuth
func (h *CouponHandler) GetCoupon(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.Error(ierr.NewError("coupon ID is required").
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	response, err := h.couponService.GetCoupon(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Update a coupon
// @Description Updates an existing coupon
// @Tags Coupons
// @Accept json
// @Produce json
// @Param id path string true "Coupon ID"
// @Param coupon body dto.UpdateCouponRequest true "Coupon update request"
// @Success 200 {object} dto.CouponResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 403 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /coupons/{id} [put]
// @Security ApiKeyAuth
func (h *CouponHandler) UpdateCoupon(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.Error(ierr.NewError("coupon ID is required").
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	var req dto.UpdateCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	response, err := h.couponService.UpdateCoupon(c.Request.Context(), id, req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Delete a coupon
// @Description Deletes a coupon
// @Tags Coupons
// @Accept json
// @Produce json
// @Param id path string true "Coupon ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 403 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /coupons/{id} [delete]
// @Security ApiKeyAuth
func (h *CouponHandler) DeleteCoupon(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.Error(ierr.NewError("coupon ID is required").
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	err := h.couponService.DeleteCoupon(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Coupon deleted successfully"})
}

// @Summary List coupons with filtering
// @Description Lists coupons with filtering
// @Tags Coupons
// @Accept json
// @Produce json
// @Param filter query types.CouponFilter true "Filter options"
// @Success 200 {object} dto.ListCouponsResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 403 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /coupons [get]
// @Security ApiKeyAuth
func (h *CouponHandler) ListCouponsByFilter(c *gin.Context) {
	var filter types.CouponFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	if filter.GetLimit() == 0 {
		filter.Limit = lo.ToPtr(types.GetDefaultFilter().Limit)
	}

	response, err := h.couponService.ListCoupons(c.Request.Context(), &filter)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Create a coupon association
// @Description Creates a new coupon association with a subscription or subscription line item
// @Tags Coupon Associations
// @Accept json
// @Produce json
// @Param association body dto.CreateCouponAssociationRequest true "Coupon association request"
// @Success 201 {object} dto.CouponAssociationResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 403 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /coupon-associations [post]
// @Security ApiKeyAuth
func (h *CouponHandler) CreateCouponAssociation(c *gin.Context) {
	var req dto.CreateCouponAssociationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	response, err := h.couponService.CreateCouponAssociation(c.Request.Context(), req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, response)
}

// @Summary Get a coupon association by ID
// @Description Retrieves a coupon association by ID
// @Tags Coupon Associations
// @Accept json
// @Produce json
// @Param id path string true "Coupon association ID"
// @Success 200 {object} dto.CouponAssociationResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 403 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /coupon-associations/{id} [get]
// @Security ApiKeyAuth
func (h *CouponHandler) GetCouponAssociation(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.Error(ierr.NewError("coupon association ID is required").
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	response, err := h.couponService.GetCouponAssociation(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Delete a coupon association
// @Description Deletes a coupon association
// @Tags Coupon Associations
// @Accept json
// @Produce json
// @Param id path string true "Coupon association ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 403 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /coupon-associations/{id} [delete]
// @Security ApiKeyAuth
func (h *CouponHandler) DeleteCouponAssociation(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.Error(ierr.NewError("coupon association ID is required").
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	err := h.couponService.DeleteCouponAssociation(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Coupon association deleted successfully"})
}

// @Summary Get coupon associations by subscription
// @Description Retrieves coupon associations for a subscription
// @Tags Coupon Associations
// @Accept json
// @Produce json
// @Param subscription_id path string true "Subscription ID"
// @Success 200 {array} dto.CouponAssociationResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 403 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /subscriptions/{subscription_id}/coupon-associations [get]
// @Security ApiKeyAuth
func (h *CouponHandler) GetCouponAssociationsBySubscription(c *gin.Context) {
	subscriptionID := c.Param("subscription_id")
	if subscriptionID == "" {
		c.Error(ierr.NewError("subscription ID is required").
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	response, err := h.couponService.GetCouponAssociationsBySubscription(c.Request.Context(), subscriptionID)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Get coupon associations by subscription line item
// @Description Retrieves coupon associations for a subscription line item
// @Tags Coupon Associations
// @Accept json
// @Produce json
// @Param subscription_line_item_id path string true "Subscription line item ID"
// @Success 200 {array} dto.CouponAssociationResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 403 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /subscription-line-items/{subscription_line_item_id}/coupon-associations [get]
// @Security ApiKeyAuth
func (h *CouponHandler) GetCouponAssociationsBySubscriptionLineItem(c *gin.Context) {
	subscriptionLineItemID := c.Param("subscription_line_item_id")
	if subscriptionLineItemID == "" {
		c.Error(ierr.NewError("subscription line item ID is required").
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	response, err := h.couponService.GetCouponAssociationsBySubscriptionLineItem(c.Request.Context(), subscriptionLineItemID)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Create a coupon application
// @Description Creates a new coupon application to an invoice
// @Tags Coupon Applications
// @Accept json
// @Produce json
// @Param application body dto.CreateCouponApplicationRequest true "Coupon application request"
// @Success 201 {object} dto.CouponApplicationResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 403 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /coupon-applications [post]
// @Security ApiKeyAuth
func (h *CouponHandler) CreateCouponApplication(c *gin.Context) {
	var req dto.CreateCouponApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	response, err := h.couponService.CreateCouponApplication(c.Request.Context(), req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, response)
}

// @Summary Get a coupon application by ID
// @Description Retrieves a coupon application by ID
// @Tags Coupon Applications
// @Accept json
// @Produce json
// @Param id path string true "Coupon application ID"
// @Success 200 {object} dto.CouponApplicationResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 403 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /coupon-applications/{id} [get]
// @Security ApiKeyAuth
func (h *CouponHandler) GetCouponApplication(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.Error(ierr.NewError("coupon application ID is required").
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	response, err := h.couponService.GetCouponApplication(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Get coupon applications by invoice
// @Description Retrieves coupon applications for an invoice
// @Tags Coupon Applications
// @Accept json
// @Produce json
// @Param invoice_id path string true "Invoice ID"
// @Success 200 {array} dto.CouponApplicationResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 403 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /invoices/{invoice_id}/coupon-applications [get]
// @Security ApiKeyAuth
func (h *CouponHandler) GetCouponApplicationsByInvoice(c *gin.Context) {
	invoiceID := c.Param("invoice_id")
	if invoiceID == "" {
		c.Error(ierr.NewError("invoice ID is required").
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	response, err := h.couponService.GetCouponApplicationsByInvoice(c.Request.Context(), invoiceID)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Get coupon applications by invoice line item
// @Description Retrieves coupon applications for an invoice line item
// @Tags Coupon Applications
// @Accept json
// @Produce json
// @Param invoice_line_item_id path string true "Invoice line item ID"
// @Success 200 {array} dto.CouponApplicationResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 403 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /invoice-line-items/{invoice_line_item_id}/coupon-applications [get]
// @Security ApiKeyAuth
func (h *CouponHandler) GetCouponApplicationsByInvoiceLineItem(c *gin.Context) {
	invoiceLineItemID := c.Param("invoice_line_item_id")
	if invoiceLineItemID == "" {
		c.Error(ierr.NewError("invoice line item ID is required").
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	response, err := h.couponService.GetCouponApplicationsByInvoiceLineItem(c.Request.Context(), invoiceLineItemID)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Apply coupon to invoice
// @Description Applies a coupon to an invoice and creates a coupon application
// @Tags Coupon Applications
// @Accept json
// @Produce json
// @Param coupon_id path string true "Coupon ID"
// @Param invoice_id path string true "Invoice ID"
// @Param original_price query number true "Original price"
// @Success 200 {object} dto.CouponApplicationResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 403 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /coupons/{coupon_id}/apply-to-invoice/{invoice_id} [post]
// @Security ApiKeyAuth
func (h *CouponHandler) ApplyCouponToInvoice(c *gin.Context) {
	couponID := c.Param("coupon_id")
	if couponID == "" {
		c.Error(ierr.NewError("coupon ID is required").
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	invoiceID := c.Param("invoice_id")
	if invoiceID == "" {
		c.Error(ierr.NewError("invoice ID is required").
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	originalPriceStr := c.Query("original_price")
	if originalPriceStr == "" {
		c.Error(ierr.NewError("original_price is required").
			WithHint("Please provide the original price").
			Mark(ierr.ErrValidation))
		return
	}

	originalPrice, err := decimal.NewFromString(originalPriceStr)
	if err != nil {
		c.Error(ierr.NewError("invalid original_price format").
			WithHint("Please provide a valid decimal number").
			Mark(ierr.ErrValidation))
		return
	}

	response, err := h.couponService.ApplyCouponToInvoice(c.Request.Context(), couponID, invoiceID, originalPrice)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Calculate discount for a coupon
// @Description Calculates the discount amount for a given coupon and price
// @Tags Coupons
// @Accept json
// @Produce json
// @Param coupon_id path string true "Coupon ID"
// @Param original_price query number true "Original price"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 401 {object} ierr.ErrorResponse
// @Failure 403 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /coupons/{coupon_id}/calculate-discount [get]
// @Security ApiKeyAuth
func (h *CouponHandler) CalculateDiscount(c *gin.Context) {
	couponID := c.Param("coupon_id")
	if couponID == "" {
		c.Error(ierr.NewError("coupon ID is required").
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	originalPriceStr := c.Query("original_price")
	if originalPriceStr == "" {
		c.Error(ierr.NewError("original_price is required").
			WithHint("Please provide the original price").
			Mark(ierr.ErrValidation))
		return
	}

	originalPrice, err := decimal.NewFromString(originalPriceStr)
	if err != nil {
		c.Error(ierr.NewError("invalid original_price format").
			WithHint("Please provide a valid decimal number").
			Mark(ierr.ErrValidation))
		return
	}

	discount, err := h.couponService.CalculateDiscount(c.Request.Context(), couponID, originalPrice)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"coupon_id":      couponID,
		"original_price": originalPrice.String(),
		"discount":       discount.String(),
		"final_price":    originalPrice.Sub(discount).String(),
	})
}
