package service

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/coupon"
	"github.com/flexprice/flexprice/internal/domain/coupon_application"
	"github.com/flexprice/flexprice/internal/domain/coupon_association"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
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
	ApplyCouponToInvoice(ctx context.Context, couponID string, invoiceID string, originalPrice decimal.Decimal) (*dto.CouponApplicationResponse, error)
	ValidateCouponForSubscription(ctx context.Context, couponID string, subscriptionID string) error
	CalculateDiscount(ctx context.Context, couponID string, originalPrice decimal.Decimal) (decimal.Decimal, error)
}

type couponService struct {
	ServiceParams
	couponRepo            coupon.Repository
	couponAssociationRepo coupon_association.Repository
	couponApplicationRepo coupon_application.Repository
	logger                *logger.Logger
}

// NewCouponService creates a new coupon service
func NewCouponService(
	params ServiceParams,
	couponRepo coupon.Repository,
	couponAssociationRepo coupon_association.Repository,
	couponApplicationRepo coupon_application.Repository,
	logger *logger.Logger,
) CouponService {
	return &couponService{
		ServiceParams:         params,
		couponRepo:            couponRepo,
		couponAssociationRepo: couponAssociationRepo,
		couponApplicationRepo: couponApplicationRepo,
		logger:                logger,
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

	if err := s.couponRepo.Create(ctx, c); err != nil {
		s.logger.Error("failed to create coupon", "error", err)
		return nil, err
	}

	return s.toCouponResponse(c), nil
}

// GetCoupon retrieves a coupon by ID
func (s *couponService) GetCoupon(ctx context.Context, id string) (*dto.CouponResponse, error) {
	c, err := s.couponRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.toCouponResponse(c), nil
}

// UpdateCoupon updates an existing coupon
func (s *couponService) UpdateCoupon(ctx context.Context, id string, req dto.UpdateCouponRequest) (*dto.CouponResponse, error) {
	c, err := s.couponRepo.Get(ctx, id)
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

	if err := s.couponRepo.Update(ctx, c); err != nil {
		s.logger.Error("failed to update coupon", "error", err)
		return nil, err
	}

	return s.toCouponResponse(c), nil
}

// DeleteCoupon deletes a coupon
func (s *couponService) DeleteCoupon(ctx context.Context, id string) error {
	return s.couponRepo.Delete(ctx, id)
}

// ListCoupons lists coupons with filtering
func (s *couponService) ListCoupons(ctx context.Context, filter *types.CouponFilter) (*dto.ListCouponsResponse, error) {
	if filter == nil {
		filter = types.NewCouponFilter()
	}

	if err := filter.Validate(); err != nil {
		return nil, err
	}

	coupons, err := s.couponRepo.List(ctx, filter)
	if err != nil {
		s.logger.Error("failed to list coupons", "error", err)
		return nil, err
	}

	total, err := s.couponRepo.Count(ctx, filter)
	if err != nil {
		s.logger.Error("failed to count coupons", "error", err)
		return nil, err
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

	// Validate that either subscription_id or subscription_line_item_id is provided, but not both
	if req.SubscriptionID == nil && req.SubscriptionLineItemID == nil {
		return nil, ierr.NewError("either subscription_id or subscription_line_item_id must be provided").
			WithHint("Please provide either a subscription ID or subscription line item ID").
			Mark(ierr.ErrValidation)
	}

	if req.SubscriptionID != nil && req.SubscriptionLineItemID != nil {
		return nil, ierr.NewError("only one of subscription_id or subscription_line_item_id can be provided").
			WithHint("Please provide either a subscription ID or subscription line item ID, not both").
			Mark(ierr.ErrValidation)
	}

	baseModel := types.GetDefaultBaseModel(ctx)
	ca := &coupon_association.CouponAssociation{
		ID:                     types.GenerateUUIDWithPrefix(types.UUID_PREFIX_COUPON_ASSOCIATION),
		CouponID:               req.CouponID,
		SubscriptionID:         req.SubscriptionID,
		SubscriptionLineItemID: req.SubscriptionLineItemID,
		Metadata:               req.Metadata,
		TenantID:               baseModel.TenantID,
		Status:                 baseModel.Status,
		CreatedAt:              baseModel.CreatedAt,
		UpdatedAt:              baseModel.UpdatedAt,
		CreatedBy:              baseModel.CreatedBy,
		UpdatedBy:              baseModel.UpdatedBy,
		EnvironmentID:          types.GetEnvironmentID(ctx),
	}

	if err := s.couponAssociationRepo.Create(ctx, ca); err != nil {
		s.logger.Error("failed to create coupon association", "error", err)
		return nil, err
	}

	return s.toCouponAssociationResponse(ca), nil
}

// GetCouponAssociation retrieves a coupon association by ID
func (s *couponService) GetCouponAssociation(ctx context.Context, id string) (*dto.CouponAssociationResponse, error) {
	ca, err := s.couponAssociationRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.toCouponAssociationResponse(ca), nil
}

// DeleteCouponAssociation deletes a coupon association
func (s *couponService) DeleteCouponAssociation(ctx context.Context, id string) error {
	return s.couponAssociationRepo.Delete(ctx, id)
}

// GetCouponAssociationsBySubscription retrieves coupon associations for a subscription
func (s *couponService) GetCouponAssociationsBySubscription(ctx context.Context, subscriptionID string) ([]*dto.CouponAssociationResponse, error) {
	associations, err := s.couponAssociationRepo.GetBySubscription(ctx, subscriptionID)
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
	associations, err := s.couponAssociationRepo.GetBySubscriptionLineItem(ctx, subscriptionLineItemID)
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

	baseModel := types.GetDefaultBaseModel(ctx)
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
		EnvironmentID:       types.GetEnvironmentID(ctx),
	}

	if err := s.couponApplicationRepo.Create(ctx, ca); err != nil {
		s.logger.Error("failed to create coupon application", "error", err)
		return nil, err
	}

	// Increment coupon redemptions
	if err := s.couponRepo.IncrementRedemptions(ctx, req.CouponID); err != nil {
		s.logger.Error("failed to increment coupon redemptions", "error", err)
		// Don't fail the entire operation if this fails
	}

	return s.toCouponApplicationResponse(ca), nil
}

// GetCouponApplication retrieves a coupon application by ID
func (s *couponService) GetCouponApplication(ctx context.Context, id string) (*dto.CouponApplicationResponse, error) {
	ca, err := s.couponApplicationRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.toCouponApplicationResponse(ca), nil
}

// GetCouponApplicationsByInvoice retrieves coupon applications for an invoice
func (s *couponService) GetCouponApplicationsByInvoice(ctx context.Context, invoiceID string) ([]*dto.CouponApplicationResponse, error) {
	applications, err := s.couponApplicationRepo.GetByInvoice(ctx, invoiceID)
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
	applications, err := s.couponApplicationRepo.GetByInvoiceLineItem(ctx, invoiceLineItemID)
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
	// Get the coupon
	c, err := s.couponRepo.Get(ctx, couponID)
	if err != nil {
		return nil, err
	}

	// Validate coupon
	if !c.IsValid() {
		return nil, ierr.NewError("coupon is not valid for redemption").
			WithHint("Coupon may be expired, have reached maximum redemptions, or not yet available for redemption").
			Mark(ierr.ErrValidation)
	}

	// Calculate discount
	discount := c.CalculateDiscount(originalPrice)
	finalPrice := c.ApplyDiscount(originalPrice)

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

	return s.CreateCouponApplication(ctx, req)
}

// ValidateCouponForSubscription validates if a coupon can be applied to a subscription
func (s *couponService) ValidateCouponForSubscription(ctx context.Context, couponID string, subscriptionID string) error {
	// Get the coupon
	c, err := s.couponRepo.Get(ctx, couponID)
	if err != nil {
		return err
	}

	// Check if coupon is valid
	if !c.IsValid() {
		return ierr.NewError("coupon is not valid for redemption").
			WithHint("Coupon may be expired, have reached maximum redemptions, or not yet available for redemption").
			Mark(ierr.ErrValidation)
	}

	// TODO: Add additional validation logic based on subscription and coupon rules
	// This could include checking subscription status, customer eligibility, etc.

	return nil
}

// CalculateDiscount calculates the discount amount for a given coupon and price
func (s *couponService) CalculateDiscount(ctx context.Context, couponID string, originalPrice decimal.Decimal) (decimal.Decimal, error) {
	// Get the coupon
	c, err := s.couponRepo.Get(ctx, couponID)
	if err != nil {
		return decimal.Zero, err
	}

	// Validate coupon
	if !c.IsValid() {
		return decimal.Zero, ierr.NewError("coupon is not valid for redemption").
			WithHint("Coupon may be expired, have reached maximum redemptions, or not yet available for redemption").
			Mark(ierr.ErrValidation)
	}

	return c.CalculateDiscount(originalPrice), nil
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
