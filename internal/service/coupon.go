package service

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/coupon"
	"github.com/flexprice/flexprice/internal/domain/coupon_application"
	"github.com/flexprice/flexprice/internal/domain/coupon_association"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// CouponService defines the interface for coupon operations
type CouponService interface {
	// Coupon operations
	CreateCoupon(ctx context.Context, req dto.CreateCouponRequest) (*dto.CouponResponse, error)
	GetCoupon(ctx context.Context, id string) (*dto.CouponResponse, error)
	UpdateCoupon(ctx context.Context, id string, req dto.UpdateCouponRequest) (*dto.CouponResponse, error)
	DeleteCoupon(ctx context.Context, id string) error
	ListCoupons(ctx context.Context, filter *types.CouponFilter) (*dto.ListCouponsResponse, error)

	// Coupon association operations
	CreateCouponAssociation(ctx context.Context, req dto.CreateCouponAssociationRequest) (*dto.CouponAssociationResponse, error)
	GetCouponAssociation(ctx context.Context, id string) (*dto.CouponAssociationResponse, error)
	DeleteCouponAssociation(ctx context.Context, id string) error
	GetCouponAssociationsBySubscription(ctx context.Context, subscriptionID string) ([]*dto.CouponAssociationResponse, error)
	GetCouponAssociationsBySubscriptionLineItem(ctx context.Context, subscriptionLineItemID string) ([]*dto.CouponAssociationResponse, error)

	// Coupon application operations
	CreateCouponApplication(ctx context.Context, req dto.CreateCouponApplicationRequest) (*dto.CouponApplicationResponse, error)
	GetCouponApplication(ctx context.Context, id string) (*dto.CouponApplicationResponse, error)
	GetCouponApplicationsByInvoice(ctx context.Context, invoiceID string) ([]*dto.CouponApplicationResponse, error)
	GetCouponApplicationsByInvoiceLineItem(ctx context.Context, invoiceLineItemID string) ([]*dto.CouponApplicationResponse, error)

	// Business logic operations
	ApplyCouponToSubscription(ctx context.Context, couponIDs []string, subscriptionID string) (*dto.CouponAssociationResponse, error)
	ApplyCouponToSubscriptionLineItem(ctx context.Context, couponIDs []string, subscriptionID string, subscriptionLineItemID string) (*dto.CouponAssociationResponse, error)

	ApplyCouponToInvoice(ctx context.Context, couponID string, invoiceID string, originalPrice decimal.Decimal) (*dto.CouponApplicationResponse, error)
	ValidateCouponForSubscription(ctx context.Context, couponID string, subscriptionID string) error
	CalculateDiscount(ctx context.Context, couponID string, originalPrice decimal.Decimal) (decimal.Decimal, error)
}

type couponService struct {
	ServiceParams
}

// NewCouponService creates a new coupon service
func NewCouponService(
	params ServiceParams,
) CouponService {
	return &couponService{
		ServiceParams: params,
	}
}

// CreateCoupon creates a new coupon
func (s *couponService) CreateCoupon(ctx context.Context, req dto.CreateCouponRequest) (*dto.CouponResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	baseModel := types.GetDefaultBaseModel(ctx)
	c := &coupon.Coupon{
		ID:                types.GenerateUUIDWithPrefix(types.UUID_PREFIX_COUPON),
		Name:              req.Name,
		RedeemAfter:       req.RedeemAfter,
		RedeemBefore:      req.RedeemBefore,
		MaxRedemptions:    req.MaxRedemptions,
		TotalRedemptions:  0,
		Rules:             req.Rules,
		AmountOff:         req.AmountOff,
		PercentageOff:     req.PercentageOff,
		Type:              req.Type,
		Cadence:           req.Cadence,
		DurationInPeriods: req.DurationInPeriods,
		Metadata:          req.Metadata,
		Currency:          req.Currency,
		TenantID:          baseModel.TenantID,
		Status:            baseModel.Status,
		CreatedAt:         baseModel.CreatedAt,
		UpdatedAt:         baseModel.UpdatedAt,
		CreatedBy:         baseModel.CreatedBy,
		UpdatedBy:         baseModel.UpdatedBy,
		EnvironmentID:     types.GetEnvironmentID(ctx),
	}

	if err := s.CouponRepo.Create(ctx, c); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to create coupon").
			Mark(ierr.ErrInternal)
	}

	return s.toCouponResponse(c), nil
}

// GetCoupon retrieves a coupon by ID
func (s *couponService) GetCoupon(ctx context.Context, id string) (*dto.CouponResponse, error) {
	c, err := s.CouponRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.toCouponResponse(c), nil
}

// UpdateCoupon updates an existing coupon
func (s *couponService) UpdateCoupon(ctx context.Context, id string, req dto.UpdateCouponRequest) (*dto.CouponResponse, error) {
	c, err := s.CouponRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Update fields if provided
	if req.Name != nil {
		c.Name = *req.Name
	}
	if req.RedeemAfter != nil {
		c.RedeemAfter = req.RedeemAfter
	}
	if req.RedeemBefore != nil {
		c.RedeemBefore = req.RedeemBefore
	}
	if req.MaxRedemptions != nil {
		c.MaxRedemptions = req.MaxRedemptions
	}
	if req.Rules != nil {
		c.Rules = req.Rules
	}
	if req.AmountOff != nil {
		c.AmountOff = req.AmountOff
	}
	if req.PercentageOff != nil {
		c.PercentageOff = req.PercentageOff
	}
	if req.Type != nil {
		c.Type = *req.Type
	}
	if req.Cadence != nil {
		c.Cadence = *req.Cadence
	}
	if req.Currency != nil {
		c.Currency = req.Currency
	}

	c.UpdatedAt = time.Now()
	c.UpdatedBy = types.GetUserID(ctx)

	if err := s.CouponRepo.Update(ctx, c); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to update coupon").
			Mark(ierr.ErrInternal)
	}

	return s.toCouponResponse(c), nil
}

// DeleteCoupon deletes a coupon
func (s *couponService) DeleteCoupon(ctx context.Context, id string) error {
	return s.CouponRepo.Delete(ctx, id)
}

// ListCoupons lists coupons with filtering
func (s *couponService) ListCoupons(ctx context.Context, filter *types.CouponFilter) (*dto.ListCouponsResponse, error) {
	if filter == nil {
		filter = types.NewCouponFilter()
	}

	if err := filter.Validate(); err != nil {
		return nil, err
	}

	coupons, err := s.CouponRepo.List(ctx, filter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to list coupons").
			Mark(ierr.ErrInternal)
	}

	total, err := s.CouponRepo.Count(ctx, filter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to count coupons").
			Mark(ierr.ErrInternal)
	}

	responses := make([]*dto.CouponResponse, len(coupons))
	for i, c := range coupons {
		responses[i] = s.toCouponResponse(c)
	}

	listResponse := types.NewListResponse(responses, total, filter.GetLimit(), filter.GetOffset())
	return &listResponse, nil
}

// CreateCouponAssociation creates a new coupon association
func (s *couponService) CreateCouponAssociation(ctx context.Context, req dto.CreateCouponAssociationRequest) (*dto.CouponAssociationResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Validate that subscription_id is provided (mandatory)
	if req.SubscriptionID == nil {
		return nil, ierr.NewError("subscription_id is required").
			WithHint("Please provide a subscription ID").
			Mark(ierr.ErrValidation)
	}

	// subscription_line_item_id is optional - can be nil for subscription-level associations

	var response *dto.CouponAssociationResponse

	// Use transaction for atomic operations
	err := s.DB.WithTx(ctx, func(txCtx context.Context) error {
		// Validate that the coupon exists and is valid
		coupon, err := s.CouponRepo.Get(txCtx, req.CouponID)
		if err != nil {
			return ierr.WithError(err).
				WithHint("Failed to get coupon for association").
				WithReportableDetails(map[string]interface{}{
					"coupon_id": req.CouponID,
				}).
				Mark(ierr.ErrNotFound)
		}

		// Validate coupon is active
		if coupon.Status != types.StatusPublished {
			return ierr.NewError("only active coupons can be associated").
				WithHint("Please select an active coupon").
				WithReportableDetails(map[string]interface{}{
					"coupon_id": req.CouponID,
					"status":    coupon.Status,
				}).
				Mark(ierr.ErrValidation)
		}

		// Validate subscription exists if provided
		if req.SubscriptionID != nil {
			subscription, err := s.SubRepo.Get(txCtx, *req.SubscriptionID)
			if err != nil {
				return ierr.WithError(err).
					WithHint("Failed to get subscription for association").
					WithReportableDetails(map[string]interface{}{
						"subscription_id": *req.SubscriptionID,
					}).
					Mark(ierr.ErrNotFound)
			}

			// Validate subscription is active
			if subscription.SubscriptionStatus != types.SubscriptionStatusActive &&
				subscription.SubscriptionStatus != types.SubscriptionStatusTrialing {
				return ierr.NewError("coupons can only be associated with active or trialing subscriptions").
					WithHint("Please ensure the subscription is active or trialing").
					WithReportableDetails(map[string]interface{}{
						"subscription_id": *req.SubscriptionID,
						"status":          subscription.SubscriptionStatus,
					}).
					Mark(ierr.ErrValidation)
			}
		}

		// Validate subscription line item exists if provided
		if req.SubscriptionLineItemID != nil {
			// Get subscription with line items to validate the line item exists
			_, lineItems, err := s.SubRepo.GetWithLineItems(txCtx, *req.SubscriptionID)
			if err != nil {
				return ierr.WithError(err).
					WithHint("Failed to get subscription line items for validation").
					WithReportableDetails(map[string]interface{}{
						"subscription_id": *req.SubscriptionID,
					}).
					Mark(ierr.ErrNotFound)
			}

			// Validate that the line item exists and belongs to this subscription
			lineItemExists := false
			for _, item := range lineItems {
				if item.ID == *req.SubscriptionLineItemID {
					lineItemExists = true
					break
				}
			}

			if !lineItemExists {
				return ierr.NewError("subscription line item not found").
					WithHint("The specified line item does not exist for this subscription").
					WithReportableDetails(map[string]interface{}{
						"subscription_id":           *req.SubscriptionID,
						"subscription_line_item_id": *req.SubscriptionLineItemID,
					}).
					Mark(ierr.ErrNotFound)
			}
		}

		// Check for existing association to prevent duplicates
		// Note: This is a simplified check. In a production system, you might want to implement
		// a more sophisticated duplicate detection mechanism
		if req.SubscriptionID != nil {
			existingAssociations, err := s.CouponAssociationRepo.GetBySubscription(txCtx, *req.SubscriptionID)
			if err != nil {
				s.Logger.Errorw("failed to check existing coupon associations",
					"subscription_id", *req.SubscriptionID,
					"error", err)
				// Don't fail the operation for this check
			} else {
				for _, existing := range existingAssociations {
					if existing.CouponID == req.CouponID {
						return ierr.NewError("coupon is already associated with this subscription").
							WithHint("This coupon is already applied to the subscription").
							WithReportableDetails(map[string]interface{}{
								"coupon_id":       req.CouponID,
								"subscription_id": *req.SubscriptionID,
							}).
							Mark(ierr.ErrAlreadyExists)
					}
				}
			}
		}

		if req.SubscriptionLineItemID != nil {
			existingAssociations, err := s.CouponAssociationRepo.GetBySubscriptionLineItem(txCtx, *req.SubscriptionLineItemID)
			if err != nil {
				s.Logger.Errorw("failed to check existing coupon associations",
					"subscription_line_item_id", *req.SubscriptionLineItemID,
					"error", err)
				// Don't fail the operation for this check
			} else {
				for _, existing := range existingAssociations {
					if existing.CouponID == req.CouponID {
						return ierr.NewError("coupon is already associated with this subscription line item").
							WithHint("This coupon is already applied to the line item").
							WithReportableDetails(map[string]interface{}{
								"coupon_id":                 req.CouponID,
								"subscription_line_item_id": *req.SubscriptionLineItemID,
							}).
							Mark(ierr.ErrAlreadyExists)
					}
				}
			}
		}

		// Create the coupon association object properly
		baseModel := types.GetDefaultBaseModel(txCtx)
		ca := &coupon_association.CouponAssociation{
			ID:                     types.GenerateUUIDWithPrefix(types.UUID_PREFIX_COUPON_ASSOCIATION),
			CouponID:               req.CouponID,
			SubscriptionID:         req.SubscriptionID,
			SubscriptionLineItemID: req.SubscriptionLineItemID,
			Metadata:               req.Metadata,
			BaseModel:              baseModel,
			EnvironmentID:          types.GetEnvironmentID(txCtx),
		}

		if err := s.CouponAssociationRepo.Create(txCtx, ca); err != nil {
			return ierr.WithError(err).
				WithHint("Failed to create coupon association").
				WithReportableDetails(map[string]interface{}{
					"coupon_id": req.CouponID,
				}).
				Mark(ierr.ErrInternal)
		}

		s.Logger.Infow("created coupon association",
			"association_id", ca.ID,
			"coupon_id", req.CouponID,
			"subscription_id", req.SubscriptionID,
			"subscription_line_item_id", req.SubscriptionLineItemID,
			"created_by", types.GetUserID(txCtx))

		response = s.toCouponAssociationResponse(ca)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// GetCouponAssociation retrieves a coupon association by ID
func (s *couponService) GetCouponAssociation(ctx context.Context, id string) (*dto.CouponAssociationResponse, error) {
	ca, err := s.CouponAssociationRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.toCouponAssociationResponse(ca), nil
}

// DeleteCouponAssociation deletes a coupon association
func (s *couponService) DeleteCouponAssociation(ctx context.Context, id string) error {
	return s.CouponAssociationRepo.Delete(ctx, id)
}

// GetCouponAssociationsBySubscription retrieves coupon associations for a subscription
func (s *couponService) GetCouponAssociationsBySubscription(ctx context.Context, subscriptionID string) ([]*dto.CouponAssociationResponse, error) {
	associations, err := s.CouponAssociationRepo.GetBySubscription(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}

	responses := make([]*dto.CouponAssociationResponse, len(associations))
	for i, ca := range associations {
		responses[i] = s.toCouponAssociationResponse(ca)
	}

	return responses, nil
}

// GetCouponAssociationsBySubscriptionLineItem retrieves coupon associations for a subscription line item
func (s *couponService) GetCouponAssociationsBySubscriptionLineItem(ctx context.Context, subscriptionLineItemID string) ([]*dto.CouponAssociationResponse, error) {
	associations, err := s.CouponAssociationRepo.GetBySubscriptionLineItem(ctx, subscriptionLineItemID)
	if err != nil {
		return nil, err
	}

	responses := make([]*dto.CouponAssociationResponse, len(associations))
	for i, ca := range associations {
		responses[i] = s.toCouponAssociationResponse(ca)
	}

	return responses, nil
}

// CreateCouponApplication creates a new coupon application
func (s *couponService) CreateCouponApplication(ctx context.Context, req dto.CreateCouponApplicationRequest) (*dto.CouponApplicationResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	var response *dto.CouponApplicationResponse

	// Use transaction for atomic operations
	err := s.DB.WithTx(ctx, func(txCtx context.Context) error {
		baseModel := types.GetDefaultBaseModel(txCtx)
		ca := &coupon_application.CouponApplication{
			ID:                  types.GenerateUUIDWithPrefix(types.UUID_PREFIX_COUPON_APPLICATION),
			CouponID:            req.CouponID,
			CouponAssociationID: req.CouponAssociationID,
			InvoiceID:           req.InvoiceID,
			InvoiceLineItemID:   req.InvoiceLineItemID,
			AppliedAt:           time.Now(),
			OriginalPrice:       req.OriginalPrice,
			FinalPrice:          req.FinalPrice,
			DiscountedAmount:    req.DiscountedAmount,
			DiscountType:        req.DiscountType,
			DiscountPercentage:  req.DiscountPercentage,
			Currency:            req.Currency,
			CouponSnapshot:      req.CouponSnapshot,
			Metadata:            req.Metadata,
			TenantID:            baseModel.TenantID,
			Status:              baseModel.Status,
			CreatedAt:           baseModel.CreatedAt,
			UpdatedAt:           baseModel.UpdatedAt,
			CreatedBy:           baseModel.CreatedBy,
			UpdatedBy:           baseModel.UpdatedBy,
			EnvironmentID:       types.GetEnvironmentID(txCtx),
		}

		if err := s.CouponApplicationRepo.Create(txCtx, ca); err != nil {
			return ierr.WithError(err).
				WithHint("Failed to create coupon application").
				Mark(ierr.ErrInternal)
		}

		// Increment coupon redemptions within the same transaction
		if err := s.CouponRepo.IncrementRedemptions(txCtx, req.CouponID); err != nil {
			s.Logger.Errorw("failed to increment coupon redemptions",
				"coupon_id", req.CouponID,
				"error", err)
			// Don't fail the entire operation if this fails, but log it
		}

		response = s.toCouponApplicationResponse(ca)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// GetCouponApplication retrieves a coupon application by ID
func (s *couponService) GetCouponApplication(ctx context.Context, id string) (*dto.CouponApplicationResponse, error) {
	ca, err := s.CouponApplicationRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.toCouponApplicationResponse(ca), nil
}

// GetCouponApplicationsByInvoice retrieves coupon applications for an invoice
func (s *couponService) GetCouponApplicationsByInvoice(ctx context.Context, invoiceID string) ([]*dto.CouponApplicationResponse, error) {
	applications, err := s.CouponApplicationRepo.GetByInvoice(ctx, invoiceID)
	if err != nil {
		return nil, err
	}

	responses := make([]*dto.CouponApplicationResponse, len(applications))
	for i, ca := range applications {
		responses[i] = s.toCouponApplicationResponse(ca)
	}

	return responses, nil
}

// GetCouponApplicationsByInvoiceLineItem retrieves coupon applications for an invoice line item
func (s *couponService) GetCouponApplicationsByInvoiceLineItem(ctx context.Context, invoiceLineItemID string) ([]*dto.CouponApplicationResponse, error) {
	applications, err := s.CouponApplicationRepo.GetByInvoiceLineItem(ctx, invoiceLineItemID)
	if err != nil {
		return nil, err
	}

	responses := make([]*dto.CouponApplicationResponse, len(applications))
	for i, ca := range applications {
		responses[i] = s.toCouponApplicationResponse(ca)
	}

	return responses, nil
}

// ApplyCouponToInvoice applies a coupon to an invoice and creates a coupon application
func (s *couponService) ApplyCouponToInvoice(ctx context.Context, couponID string, invoiceID string, originalPrice decimal.Decimal) (*dto.CouponApplicationResponse, error) {
	// Validate input parameters
	if couponID == "" {
		return nil, ierr.NewError("coupon_id is required").
			WithHint("Please provide a valid coupon ID").
			Mark(ierr.ErrValidation)
	}

	if invoiceID == "" {
		return nil, ierr.NewError("invoice_id is required").
			WithHint("Please provide a valid invoice ID").
			Mark(ierr.ErrValidation)
	}

	if originalPrice.LessThanOrEqual(decimal.Zero) {
		return nil, ierr.NewError("original_price must be greater than zero").
			WithHint("Please provide a valid original price").
			WithReportableDetails(map[string]interface{}{
				"original_price": originalPrice,
			}).
			Mark(ierr.ErrValidation)
	}

	s.Logger.Infow("applying coupon to invoice",
		"coupon_id", couponID,
		"invoice_id", invoiceID,
		"original_price", originalPrice)

	var response *dto.CouponApplicationResponse

	// Use transaction for atomic operations
	err := s.DB.WithTx(ctx, func(txCtx context.Context) error {
		// Get the coupon
		c, err := s.CouponRepo.Get(txCtx, couponID)
		if err != nil {
			return ierr.WithError(err).
				WithHint("Failed to get coupon for invoice application").
				WithReportableDetails(map[string]interface{}{
					"coupon_id":  couponID,
					"invoice_id": invoiceID,
				}).
				Mark(ierr.ErrNotFound)
		}

		// Validate coupon is active
		if c.Status != types.StatusPublished {
			return ierr.NewError("only active coupons can be applied").
				WithHint("Please select an active coupon").
				WithReportableDetails(map[string]interface{}{
					"coupon_id":  couponID,
					"invoice_id": invoiceID,
					"status":     c.Status,
				}).
				Mark(ierr.ErrValidation)
		}

		// Validate coupon is valid for redemption
		if !c.IsValid() {
			return ierr.NewError("coupon is not valid for redemption").
				WithHint("Coupon may be expired, have reached maximum redemptions, or not yet available for redemption").
				WithReportableDetails(map[string]interface{}{
					"coupon_id":         couponID,
					"invoice_id":        invoiceID,
					"redeem_after":      c.RedeemAfter,
					"redeem_before":     c.RedeemBefore,
					"total_redemptions": c.TotalRedemptions,
					"max_redemptions":   c.MaxRedemptions,
				}).
				Mark(ierr.ErrValidation)
		}

		// Calculate discount
		discount := c.CalculateDiscount(originalPrice)
		finalPrice := c.ApplyDiscount(originalPrice)

		// Validate that the final price is not negative
		if finalPrice.LessThan(decimal.Zero) {
			return ierr.NewError("discount amount exceeds original price").
				WithHint("The discount amount cannot be greater than the original price").
				WithReportableDetails(map[string]interface{}{
					"coupon_id":      couponID,
					"invoice_id":     invoiceID,
					"original_price": originalPrice,
					"discount":       discount,
					"final_price":    finalPrice,
				}).
				Mark(ierr.ErrValidation)
		}

		// Create coupon application
		req := dto.CreateCouponApplicationRequest{
			CouponID:         couponID,
			InvoiceID:        invoiceID,
			OriginalPrice:    originalPrice,
			FinalPrice:       finalPrice,
			DiscountedAmount: discount,
			DiscountType:     c.Type,
			Currency:         *c.Currency,
			CouponSnapshot: map[string]interface{}{
				"name":           c.Name,
				"type":           c.Type,
				"cadence":        c.Cadence,
				"amount_off":     c.AmountOff,
				"percentage_off": c.PercentageOff,
			},
		}

		if c.Type == types.CouponTypePercentage {
			req.DiscountPercentage = c.PercentageOff
		}

		application, err := s.CreateCouponApplication(txCtx, req)
		if err != nil {
			s.Logger.Errorw("failed to create coupon application for invoice",
				"coupon_id", couponID,
				"invoice_id", invoiceID,
				"error", err)
			return err
		}

		s.Logger.Infow("successfully applied coupon to invoice",
			"coupon_id", couponID,
			"invoice_id", invoiceID,
			"application_id", application.ID,
			"original_price", originalPrice,
			"final_price", finalPrice,
			"discount", discount)

		response = application
		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// ValidateCouponForSubscription validates if a coupon can be applied to a subscription
func (s *couponService) ValidateCouponForSubscription(ctx context.Context, couponID string, subscriptionID string) error {
	// Get the coupon
	c, err := s.CouponRepo.Get(ctx, couponID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get coupon for validation").
			WithReportableDetails(map[string]interface{}{
				"coupon_id":       couponID,
				"subscription_id": subscriptionID,
			}).
			Mark(ierr.ErrNotFound)
	}

	// Check if coupon is active
	if c.Status != types.StatusPublished {
		return ierr.NewError("only active coupons can be applied").
			WithHint("Please select an active coupon").
			WithReportableDetails(map[string]interface{}{
				"coupon_id":       couponID,
				"subscription_id": subscriptionID,
				"status":          c.Status,
			}).
			Mark(ierr.ErrValidation)
	}

	// Check if coupon is valid for redemption
	if !c.IsValid() {
		return ierr.NewError("coupon is not valid for redemption").
			WithHint("Coupon may be expired, have reached maximum redemptions, or not yet available for redemption").
			WithReportableDetails(map[string]interface{}{
				"coupon_id":         couponID,
				"subscription_id":   subscriptionID,
				"redeem_after":      c.RedeemAfter,
				"redeem_before":     c.RedeemBefore,
				"total_redemptions": c.TotalRedemptions,
				"max_redemptions":   c.MaxRedemptions,
			}).
			Mark(ierr.ErrValidation)
	}

	// TODO: Add additional validation logic based on subscription and coupon rules
	// This could include checking subscription status, customer eligibility, etc.
	// For example:
	// - Check if coupon is valid for the subscription's customer
	// - Check if coupon is valid for the subscription's plan
	// - Check if coupon has any usage restrictions
	// - Check if coupon is valid for the subscription's billing cycle

	return nil
}

// CalculateDiscount calculates the discount amount for a given coupon and price
func (s *couponService) CalculateDiscount(ctx context.Context, couponID string, originalPrice decimal.Decimal) (decimal.Decimal, error) {
	// Validate input parameters
	if couponID == "" {
		return decimal.Zero, ierr.NewError("coupon_id is required").
			WithHint("Please provide a valid coupon ID").
			Mark(ierr.ErrValidation)
	}

	if originalPrice.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, ierr.NewError("original_price must be greater than zero").
			WithHint("Please provide a valid original price").
			WithReportableDetails(map[string]interface{}{
				"original_price": originalPrice,
			}).
			Mark(ierr.ErrValidation)
	}

	s.Logger.Debugw("calculating discount for coupon",
		"coupon_id", couponID,
		"original_price", originalPrice)

	// Get the coupon
	c, err := s.CouponRepo.Get(ctx, couponID)
	if err != nil {
		return decimal.Zero, ierr.WithError(err).
			WithHint("Failed to get coupon for discount calculation").
			WithReportableDetails(map[string]interface{}{
				"coupon_id": couponID,
			}).
			Mark(ierr.ErrNotFound)
	}

	// Validate coupon is active
	if c.Status != types.StatusPublished {
		return decimal.Zero, ierr.NewError("only active coupons can be used for discount calculation").
			WithHint("Please select an active coupon").
			WithReportableDetails(map[string]interface{}{
				"coupon_id": couponID,
				"status":    c.Status,
			}).
			Mark(ierr.ErrValidation)
	}

	// Validate coupon is valid for redemption
	if !c.IsValid() {
		return decimal.Zero, ierr.NewError("coupon is not valid for redemption").
			WithHint("Coupon may be expired, have reached maximum redemptions, or not yet available for redemption").
			WithReportableDetails(map[string]interface{}{
				"coupon_id":         couponID,
				"redeem_after":      c.RedeemAfter,
				"redeem_before":     c.RedeemBefore,
				"total_redemptions": c.TotalRedemptions,
				"max_redemptions":   c.MaxRedemptions,
			}).
			Mark(ierr.ErrValidation)
	}

	discount := c.CalculateDiscount(originalPrice)

	s.Logger.Debugw("calculated discount for coupon",
		"coupon_id", couponID,
		"original_price", originalPrice,
		"discount", discount,
		"coupon_type", c.Type)

	return discount, nil
}

func (s *couponService) ApplyCouponToSubscription(ctx context.Context, couponIDs []string, subscriptionID string) (*dto.CouponAssociationResponse, error) {
	// Validate input parameters
	if len(couponIDs) == 0 {
		return nil, ierr.NewError("at least one coupon_id is required").
			WithHint("Please provide at least one coupon ID to apply").
			Mark(ierr.ErrValidation)
	}

	if subscriptionID == "" {
		return nil, ierr.NewError("subscription_id is required").
			WithHint("Please provide a valid subscription ID").
			Mark(ierr.ErrValidation)
	}

	s.Logger.Infow("applying coupons to subscription",
		"subscription_id", subscriptionID,
		"coupon_count", len(couponIDs),
		"coupon_ids", couponIDs)

	// Get the subscription to validate it exists and is active
	subscription, err := s.SubRepo.Get(ctx, subscriptionID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get subscription for coupon application").
			WithReportableDetails(map[string]interface{}{
				"subscription_id": subscriptionID,
			}).
			Mark(ierr.ErrNotFound)
	}

	// Validate subscription status
	if subscription.SubscriptionStatus != types.SubscriptionStatusActive &&
		subscription.SubscriptionStatus != types.SubscriptionStatusTrialing {
		return nil, ierr.NewError("coupons can only be applied to active or trialing subscriptions").
			WithHint("Please ensure the subscription is active or trialing before applying coupons").
			WithReportableDetails(map[string]interface{}{
				"subscription_id": subscriptionID,
				"status":          subscription.SubscriptionStatus,
			}).
			Mark(ierr.ErrValidation)
	}

	// Validate each coupon
	for _, couponID := range couponIDs {
		if err := s.ValidateCouponForSubscription(ctx, couponID, subscriptionID); err != nil {
			return nil, ierr.WithError(err).
				WithHint("Coupon validation failed").
				WithReportableDetails(map[string]interface{}{
					"coupon_id":       couponID,
					"subscription_id": subscriptionID,
				}).
				Mark(ierr.ErrValidation)
		}
	}

	// Apply all coupons to the subscription
	var lastAssociation *dto.CouponAssociationResponse
	appliedCoupons := make([]string, 0, len(couponIDs))
	failedCoupons := make([]string, 0)

	for _, couponID := range couponIDs {
		// Create coupon association request
		req := dto.CreateCouponAssociationRequest{
			CouponID:       couponID,
			SubscriptionID: &subscriptionID,
			Metadata:       map[string]string{},
		}

		// Create the coupon association
		association, err := s.CreateCouponAssociation(ctx, req)
		if err != nil {
			s.Logger.Errorw("failed to create coupon association",
				"subscription_id", subscriptionID,
				"coupon_id", couponID,
				"error", err)
			failedCoupons = append(failedCoupons, couponID)
			continue // Continue with other coupons even if one fails
		}

		appliedCoupons = append(appliedCoupons, couponID)
		lastAssociation = association

		s.Logger.Infow("successfully applied coupon to subscription",
			"subscription_id", subscriptionID,
			"coupon_id", couponID,
			"association_id", association.ID)
	}

	// If no coupons were applied successfully, return an error
	if len(appliedCoupons) == 0 {
		return nil, ierr.NewError("failed to apply any coupons to subscription").
			WithHint("All requested coupons failed to be applied").
			WithReportableDetails(map[string]interface{}{
				"subscription_id":   subscriptionID,
				"requested_coupons": couponIDs,
				"failed_coupons":    failedCoupons,
			}).
			Mark(ierr.ErrValidation)
	}

	// If some coupons failed, log a warning but return the last successful association
	if len(failedCoupons) > 0 {
		s.Logger.Warnw("some coupons failed to be applied",
			"subscription_id", subscriptionID,
			"applied_coupons", appliedCoupons,
			"failed_coupons", failedCoupons,
			"total_requested", len(couponIDs),
			"total_applied", len(appliedCoupons))
	}

	s.Logger.Infow("completed coupon application to subscription",
		"subscription_id", subscriptionID,
		"total_requested", len(couponIDs),
		"total_applied", len(appliedCoupons),
		"total_failed", len(failedCoupons))

	return lastAssociation, nil
}

func (s *couponService) ApplyCouponToSubscriptionLineItem(ctx context.Context, couponIDs []string, subscriptionID string, subscriptionLineItemID string) (*dto.CouponAssociationResponse, error) {
	// Validate input parameters
	if len(couponIDs) == 0 {
		return nil, ierr.NewError("at least one coupon_id is required").
			WithHint("Please provide at least one coupon ID to apply").
			Mark(ierr.ErrValidation)
	}

	if subscriptionID == "" {
		return nil, ierr.NewError("subscription_id is required").
			WithHint("Please provide a valid subscription ID").
			Mark(ierr.ErrValidation)
	}

	if subscriptionLineItemID == "" {
		return nil, ierr.NewError("subscription_line_item_id is required").
			WithHint("Please provide a valid subscription line item ID").
			Mark(ierr.ErrValidation)
	}

	s.Logger.Infow("applying coupons to subscription line item",
		"subscription_id", subscriptionID,
		"subscription_line_item_id", subscriptionLineItemID,
		"coupon_count", len(couponIDs),
		"coupon_ids", couponIDs)

	// Get the subscription to validate it exists and is active
	subscription, err := s.SubRepo.Get(ctx, subscriptionID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get subscription for coupon application").
			WithReportableDetails(map[string]interface{}{
				"subscription_id": subscriptionID,
			}).
			Mark(ierr.ErrNotFound)
	}

	// Validate subscription status
	if subscription.SubscriptionStatus != types.SubscriptionStatusActive &&
		subscription.SubscriptionStatus != types.SubscriptionStatusTrialing {
		return nil, ierr.NewError("coupons can only be applied to active or trialing subscriptions").
			WithHint("Please ensure the subscription is active or trialing before applying coupons").
			WithReportableDetails(map[string]interface{}{
				"subscription_id": subscriptionID,
				"status":          subscription.SubscriptionStatus,
			}).
			Mark(ierr.ErrValidation)
	}

	// Get subscription with line items to validate the line item exists
	_, lineItems, err := s.SubRepo.GetWithLineItems(ctx, subscriptionID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get subscription line items for validation").
			WithReportableDetails(map[string]interface{}{
				"subscription_id": subscriptionID,
			}).
			Mark(ierr.ErrNotFound)
	}

	// Validate that the line item exists and belongs to this subscription
	lineItemExists := false
	for _, item := range lineItems {
		if item.ID == subscriptionLineItemID {
			lineItemExists = true
			break
		}
	}

	if !lineItemExists {
		return nil, ierr.NewError("subscription line item not found").
			WithHint("The specified line item does not exist for this subscription").
			WithReportableDetails(map[string]interface{}{
				"subscription_id":           subscriptionID,
				"subscription_line_item_id": subscriptionLineItemID,
			}).
			Mark(ierr.ErrNotFound)
	}

	// Validate each coupon
	for _, couponID := range couponIDs {
		if err := s.ValidateCouponForSubscription(ctx, couponID, subscriptionID); err != nil {
			return nil, ierr.WithError(err).
				WithHint("Coupon validation failed").
				WithReportableDetails(map[string]interface{}{
					"coupon_id":       couponID,
					"subscription_id": subscriptionID,
				}).
				Mark(ierr.ErrValidation)
		}
	}

	// Apply all coupons to the subscription line item
	var lastAssociation *dto.CouponAssociationResponse
	appliedCoupons := make([]string, 0, len(couponIDs))
	failedCoupons := make([]string, 0)

	for _, couponID := range couponIDs {
		// Create coupon association request
		req := dto.CreateCouponAssociationRequest{
			CouponID:               couponID,
			SubscriptionID:         &subscriptionID,
			SubscriptionLineItemID: &subscriptionLineItemID,
			Metadata:               map[string]string{},
		}

		// Create the coupon association
		association, err := s.CreateCouponAssociation(ctx, req)
		if err != nil {
			s.Logger.Errorw("failed to create coupon association for line item",
				"subscription_id", subscriptionID,
				"subscription_line_item_id", subscriptionLineItemID,
				"coupon_id", couponID,
				"error", err)
			failedCoupons = append(failedCoupons, couponID)
			continue // Continue with other coupons even if one fails
		}

		appliedCoupons = append(appliedCoupons, couponID)
		lastAssociation = association

		s.Logger.Infow("successfully applied coupon to subscription line item",
			"subscription_id", subscriptionID,
			"subscription_line_item_id", subscriptionLineItemID,
			"coupon_id", couponID,
			"association_id", association.ID)
	}

	// If no coupons were applied successfully, return an error
	if len(appliedCoupons) == 0 {
		return nil, ierr.NewError("failed to apply any coupons to subscription line item").
			WithHint("All requested coupons failed to be applied").
			WithReportableDetails(map[string]interface{}{
				"subscription_id":           subscriptionID,
				"subscription_line_item_id": subscriptionLineItemID,
				"requested_coupons":         couponIDs,
				"failed_coupons":            failedCoupons,
			}).
			Mark(ierr.ErrValidation)
	}

	// If some coupons failed, log a warning but return the last successful association
	if len(failedCoupons) > 0 {
		s.Logger.Warnw("some coupons failed to be applied to line item",
			"subscription_id", subscriptionID,
			"subscription_line_item_id", subscriptionLineItemID,
			"applied_coupons", appliedCoupons,
			"failed_coupons", failedCoupons,
			"total_requested", len(couponIDs),
			"total_applied", len(appliedCoupons))
	}

	s.Logger.Infow("completed coupon application to subscription line item",
		"subscription_id", subscriptionID,
		"subscription_line_item_id", subscriptionLineItemID,
		"total_requested", len(couponIDs),
		"total_applied", len(appliedCoupons),
		"total_failed", len(failedCoupons))

	return lastAssociation, nil
}

// Helper methods to convert domain models to DTOs
func (s *couponService) toCouponResponse(c *coupon.Coupon) *dto.CouponResponse {
	return &dto.CouponResponse{
		Coupon: c,
	}
}

func (s *couponService) toCouponAssociationResponse(ca *coupon_association.CouponAssociation) *dto.CouponAssociationResponse {
	return &dto.CouponAssociationResponse{
		CouponAssociation: ca,
	}
}

func (s *couponService) toCouponApplicationResponse(ca *coupon_application.CouponApplication) *dto.CouponApplicationResponse {
	return &dto.CouponApplicationResponse{
		CouponApplication: ca,
	}
}
