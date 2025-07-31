package service

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/coupon"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

// CouponValidationError represents validation errors with structured details
type CouponValidationError struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func (e *CouponValidationError) Error() string {
	return e.Message
}

// CouponValidationService defines the interface for coupon validation operations
type CouponValidationService interface {
	// Core validation method used for both subscription and invoice scenarios
	ValidateCouponForSubscription(ctx context.Context, couponID string, subscriptionID string) error
	ValidateCouponForInvoice(ctx context.Context, couponID string, invoiceID string, subscriptionID string) error

	// Validate coupon redemption increment when coupon is associated with subscription
	ValidateCouponRedemptionIncrement(ctx context.Context, couponID string) error
}

// couponValidationService implements CouponValidationService
type couponValidationService struct {
	ServiceParams
}

// NewCouponValidationService creates a new coupon validation service
func NewCouponValidationService(params ServiceParams) CouponValidationService {
	return &couponValidationService{
		ServiceParams: params,
	}
}

// ValidateCouponForSubscription validates a coupon before associating it with a subscription
func (s *couponValidationService) ValidateCouponForSubscription(ctx context.Context, couponID string, subscriptionID string) error {
	s.Logger.Infow("validating coupon for subscription association",
		"coupon_id", couponID,
		"subscription_id", subscriptionID)

	// Get coupon details
	coupon, err := s.CouponRepo.Get(ctx, couponID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get coupon details").
			Mark(ierr.ErrNotFound)
	}

	// Get subscription details
	subscription, err := s.SubRepo.Get(ctx, subscriptionID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get subscription details").
			Mark(ierr.ErrNotFound)
	}

	// Priority 1: Basic coupon validation
	if err := s.ValidateCouponBasic(coupon); err != nil {
		return err
	}

	// Priority 2: Business rule validation
	if err := s.validateCouponBusinessRules(coupon, subscription); err != nil {
		return err
	}

	// Priority 3: Redemption validation
	if err := s.validateCouponRedemption(coupon); err != nil {
		return err
	}

	// Priority 4: Subscription-specific validation
	if err := s.validateCouponForSubscriptionSpecific(coupon, subscription); err != nil {
		return err
	}

	s.Logger.Infow("coupon validation for subscription successful",
		"coupon_id", couponID,
		"subscription_id", subscriptionID)

	return nil
}

func (s *couponValidationService) ValidateCouponForInvoice(ctx context.Context, couponID string, invoiceID string, subscriptionID string) error {
	s.Logger.Infow("validating coupon for invoice application",
		"coupon_id", couponID,
		"invoice_id", invoiceID,
		"subscription_id", subscriptionID)

	// Get coupon details
	coupon, err := s.CouponRepo.Get(ctx, couponID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get coupon details").
			Mark(ierr.ErrNotFound)
	}

	// Get invoice details
	invoice, err := s.InvoiceRepo.Get(ctx, invoiceID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get invoice details").
			Mark(ierr.ErrNotFound)
	}

	// Get subscription details
	subscription, err := s.SubRepo.Get(ctx, subscriptionID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get subscription details").
			Mark(ierr.ErrNotFound)
	}

	// Priority 1: Basic coupon validation
	if err := s.ValidateCouponBasic(coupon); err != nil {
		return err
	}

	// Priority 2: Business rule validation
	if err := s.validateCouponBusinessRules(coupon, subscription); err != nil {
		return err
	}

	// Priority 3: Redemption validation
	if err := s.validateCouponRedemption(coupon); err != nil {
		return err
	}

	// Priority 4: Invoice-specific validation (cadence rules)
	if err := s.validateCouponForInvoiceSpecific(ctx, coupon, invoice, subscription); err != nil {
		return err
	}

	// Priority 5: Subscription-specific validation
	if err := s.validateCouponForSubscriptionSpecific(coupon, subscription); err != nil {
		return err
	}

	s.Logger.Infow("coupon validation for invoice successful",
		"coupon_id", couponID,
		"invoice_id", invoiceID,
		"subscription_id", subscriptionID)

	return nil
}

// ValidateCouponRedemptionIncrement validates if coupon redemption can be incremented
func (s *couponValidationService) ValidateCouponRedemptionIncrement(ctx context.Context, couponID string) error {
	s.Logger.Infow("validating coupon redemption increment",
		"coupon_id", couponID)

	// Get coupon details
	coupon, err := s.CouponRepo.Get(ctx, couponID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get coupon details for redemption increment").
			Mark(ierr.ErrNotFound)
	}

	// Priority 1: Basic coupon validation
	if err := s.ValidateCouponBasic(coupon); err != nil {
		return err
	}

	// Priority 2: Redemption validation - check if we can increment
	if err := s.validateCouponRedemption(coupon); err != nil {
		return err
	}

	s.Logger.Infow("coupon redemption increment validation successful",
		"coupon_id", couponID)

	return nil
}

// ValidateCouponBasic performs basic coupon validation (public method)
func (s *couponValidationService) ValidateCouponBasic(coupon *coupon.Coupon) error {
	// Check if coupon exists
	if coupon == nil {
		return &CouponValidationError{
			Code:    "COUPON_NOT_FOUND",
			Message: "Coupon not found",
		}
	}

	// Check if coupon is published
	if coupon.Status != types.StatusPublished {
		return &CouponValidationError{
			Code:    "COUPON_NOT_PUBLISHED",
			Message: "Coupon is not published",
			Details: map[string]interface{}{
				"coupon_id": coupon.ID,
				"status":    coupon.Status,
			},
		}
	}

	return nil
}

// Priority 2: Business rule validation for subscription
func (s *couponValidationService) validateCouponBusinessRules(coupon *coupon.Coupon, subscription *subscription.Subscription) error {
	// Date range validation
	if err := s.validateCouponDateRange(coupon); err != nil {
		return err
	}

	// Currency validation (if subscription has currency)
	if subscription.Currency != "" {
		if err := s.validateCouponCurrency(coupon, subscription.Currency); err != nil {
			return err
		}
	}

	return nil
}

// Date range validation
func (s *couponValidationService) validateCouponDateRange(coupon *coupon.Coupon) error {
	now := time.Now()

	// Check redeem_after date
	if coupon.RedeemAfter != nil {
		if now.Before(*coupon.RedeemAfter) {
			return &CouponValidationError{
				Code:    "COUPON_NOT_ACTIVE",
				Message: "Coupon is not yet active",
				Details: map[string]interface{}{
					"coupon_id":    coupon.ID,
					"redeem_after": coupon.RedeemAfter,
					"current_time": now,
				},
			}
		}
	}

	// Check redeem_before date
	if coupon.RedeemBefore != nil {
		if now.After(*coupon.RedeemBefore) {
			return &CouponValidationError{
				Code:    "COUPON_EXPIRED",
				Message: "Coupon has expired",
				Details: map[string]interface{}{
					"coupon_id":     coupon.ID,
					"redeem_before": coupon.RedeemBefore,
					"current_time":  now,
				},
			}
		}
	}

	return nil
}

// Environment validation
func (s *couponValidationService) validateCouponEnvironment(coupon *coupon.Coupon) error {
	// Get environment from context
	envID := types.GetEnvironmentID(context.Background())

	if coupon.EnvironmentID != envID {
		return &CouponValidationError{
			Code:    "ENVIRONMENT_MISMATCH",
			Message: "Coupon environment does not match current environment",
			Details: map[string]interface{}{
				"coupon_id":           coupon.ID,
				"coupon_environment":  coupon.EnvironmentID,
				"current_environment": envID,
			},
		}
	}

	return nil
}

// Currency validation
func (s *couponValidationService) validateCouponCurrency(coupon *coupon.Coupon, targetCurrency string) error {
	// If coupon has specific currency, it must match target currency
	if coupon.Currency != nil && *coupon.Currency != "" {
		if *coupon.Currency != targetCurrency {
			return &CouponValidationError{
				Code:    "CURRENCY_MISMATCH",
				Message: "Coupon currency does not match target currency",
				Details: map[string]interface{}{
					"coupon_id":       coupon.ID,
					"coupon_currency": *coupon.Currency,
					"target_currency": targetCurrency,
				},
			}
		}
	}

	return nil
}

// Priority 5: Redemption validation
func (s *couponValidationService) validateCouponRedemption(coupon *coupon.Coupon) error {
	// Check if coupon has reached max redemptions
	if coupon.MaxRedemptions != nil {
		if coupon.TotalRedemptions >= *coupon.MaxRedemptions {
			return &CouponValidationError{
				Code:    "REDEMPTION_LIMIT_REACHED",
				Message: "Coupon has reached maximum redemptions",
				Details: map[string]interface{}{
					"coupon_id":         coupon.ID,
					"max_redemptions":   *coupon.MaxRedemptions,
					"total_redemptions": coupon.TotalRedemptions,
				},
			}
		}
	}

	return nil
}

// Priority 4: Subscription-specific validation
func (s *couponValidationService) validateCouponForSubscriptionSpecific(coupon *coupon.Coupon, subscription *subscription.Subscription) error {
	// Check if subscription is in a valid state for coupon association
	if subscription.SubscriptionStatus == types.SubscriptionStatusCancelled {
		return &CouponValidationError{
			Code:    "INVALID_SUBSCRIPTION_STATUS",
			Message: "Cannot associate coupon with cancelled subscription",
			Details: map[string]interface{}{
				"coupon_id":       coupon.ID,
				"subscription_id": subscription.ID,
				"status":          subscription.SubscriptionStatus,
			},
		}
	}

	return nil
}

// validateCouponForInvoiceSpecific implements cadence-specific validation for invoice application
func (s *couponValidationService) validateCouponForInvoiceSpecific(ctx context.Context, coupon *coupon.Coupon, invoice *invoice.Invoice, subscription *subscription.Subscription) error {
	s.Logger.Debugw("validating coupon cadence for invoice",
		"coupon_id", coupon.ID,
		"invoice_id", invoice.ID,
		"cadence", coupon.Cadence)

	// Validate cadence-specific rules
	switch coupon.Cadence {
	case types.CouponCadenceOnce:
		return s.validateOnceCadenceForInvoice(ctx, coupon, invoice, subscription)
	case types.CouponCadenceForever:
		return s.validateForeverCadenceForInvoice(coupon, invoice, subscription)
	case types.CouponCadenceRepeated:
		return s.validateRepeatedCadenceForInvoice(ctx, coupon, invoice, subscription)
	default:
		return &CouponValidationError{
			Code:    "INVALID_CADENCE",
			Message: "Invalid coupon cadence",
			Details: map[string]interface{}{
				"coupon_id": coupon.ID,
				"cadence":   coupon.Cadence,
			},
		}
	}
}

// validateOnceCadenceForInvoice validates "once" cadence - coupon should only be applied to first invoice
func (s *couponValidationService) validateOnceCadenceForInvoice(ctx context.Context, coupon *coupon.Coupon, invoice *invoice.Invoice, subscription *subscription.Subscription) error {
	s.Logger.Debugw("validating once cadence for invoice",
		"coupon_id", coupon.ID,
		"invoice_id", invoice.ID,
		"subscription_id", subscription.ID)

	// Get all invoices for this subscription to check if this is the first one
	limit := 100
	invoiceFilter := &types.InvoiceFilter{
		QueryFilter: &types.QueryFilter{
			Limit: &limit, // Reasonable limit to check previous invoices
		},
		SubscriptionID: subscription.ID,
	}

	previousInvoices, err := s.InvoiceRepo.List(ctx, invoiceFilter)
	if err != nil {
		return &CouponValidationError{
			Code:    "DATABASE_ERROR",
			Message: "Failed to get previous invoices for once cadence validation",
			Details: map[string]interface{}{
				"subscription_id": subscription.ID,
				"error":           err.Error(),
			},
		}
	}

	// Count invoices that are not the current invoice (excluding drafts)
	var previousInvoiceCount int
	for _, inv := range previousInvoices {
		// Only count finalized invoices, exclude current invoice and drafts
		if inv.ID != invoice.ID && inv.InvoiceStatus != types.InvoiceStatusDraft {
			previousInvoiceCount++
		}
	}

	// For "once" cadence, this should be applied only to the first invoice
	if previousInvoiceCount > 0 {
		return &CouponValidationError{
			Code:    "ONCE_CADENCE_VIOLATION",
			Message: "Once cadence coupon can only be applied to the first invoice",
			Details: map[string]interface{}{
				"coupon_id":              coupon.ID,
				"subscription_id":        subscription.ID,
				"current_invoice_id":     invoice.ID,
				"previous_invoice_count": previousInvoiceCount,
			},
		}
	}

	s.Logger.Debugw("once cadence validation passed - this is the first invoice",
		"coupon_id", coupon.ID,
		"subscription_id", subscription.ID)

	return nil
}

// validateForeverCadenceForInvoice validates "forever" cadence - coupon is always applied
func (s *couponValidationService) validateForeverCadenceForInvoice(coupon *coupon.Coupon, invoice *invoice.Invoice, subscription *subscription.Subscription) error {
	s.Logger.Debugw("validating forever cadence for invoice",
		"coupon_id", coupon.ID,
		"invoice_id", invoice.ID)

	// Forever cadence coupons are always valid for application
	// Even if the coupon has expired, it should still be applied to invoices
	// where it was already associated with the subscription
	// This follows the requirement: "coupon expires, it will still be applied on all future invoices"

	return nil
}

// validateRepeatedCadenceForInvoice validates "repeated" cadence - coupon applied for duration_in_periods times
func (s *couponValidationService) validateRepeatedCadenceForInvoice(ctx context.Context, coupon *coupon.Coupon, invoice *invoice.Invoice, subscription *subscription.Subscription) error {
	s.Logger.Debugw("validating repeated cadence for invoice",
		"coupon_id", coupon.ID,
		"invoice_id", invoice.ID,
		"duration_in_periods", coupon.DurationInPeriods)

	// Check if duration_in_periods is set for repeated cadence
	if coupon.DurationInPeriods == nil || *coupon.DurationInPeriods <= 0 {
		return &CouponValidationError{
			Code:    "INVALID_REPEATED_CADENCE",
			Message: "Repeated cadence requires valid duration_in_periods",
			Details: map[string]interface{}{
				"coupon_id":           coupon.ID,
				"duration_in_periods": coupon.DurationInPeriods,
			},
		}
	}

	// Get all invoices for this subscription to count coupon applications across all periods
	subscriptionLimit := 1000
	invoiceFilter := &types.InvoiceFilter{
		QueryFilter: &types.QueryFilter{
			Limit: &subscriptionLimit, // Reasonable limit to check all invoices
		},
		SubscriptionID: subscription.ID,
	}

	subscriptionInvoices, err := s.InvoiceRepo.List(ctx, invoiceFilter)
	if err != nil {
		s.Logger.Warnw("failed to get subscription invoices for repeated cadence validation",
			"coupon_id", coupon.ID,
			"subscription_id", subscription.ID,
			"error", err)
		// Don't fail validation if we can't get invoices - allow the application to proceed
		return nil
	}

	// Count how many times this specific coupon has been applied across all invoices for this subscription
	var existingApplicationCount int
	for _, inv := range subscriptionInvoices {
		// Skip the current invoice since we're validating if we can apply to it
		if inv.ID == invoice.ID {
			continue
		}

		// Get applications for each invoice
		applications, err := s.CouponApplicationRepo.GetByInvoice(ctx, inv.ID)
		if err != nil {
			s.Logger.Warnw("failed to get applications for invoice",
				"invoice_id", inv.ID,
				"error", err)
			continue
		}

		// Count applications for this specific coupon
		for _, app := range applications {
			if app.CouponID == coupon.ID {
				existingApplicationCount++
			}
		}
	}

	// For repeated cadence, check if we've exceeded the duration_in_periods limit
	if existingApplicationCount >= *coupon.DurationInPeriods {
		return &CouponValidationError{
			Code:    "REPEATED_CADENCE_LIMIT_REACHED",
			Message: "Repeated cadence coupon has reached its application limit",
			Details: map[string]interface{}{
				"coupon_id":             coupon.ID,
				"subscription_id":       subscription.ID,
				"duration_in_periods":   *coupon.DurationInPeriods,
				"existing_applications": existingApplicationCount,
			},
		}
	}

	s.Logger.Debugw("repeated cadence validation passed",
		"coupon_id", coupon.ID,
		"existing_applications", existingApplicationCount,
		"duration_in_periods", *coupon.DurationInPeriods)

	return nil
}
