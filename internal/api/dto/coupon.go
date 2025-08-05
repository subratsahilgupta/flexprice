package dto

import (
	"time"

	coupon "github.com/flexprice/flexprice/internal/domain/coupon"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// CreateCouponRequest represents the request to create a new coupon
type CreateCouponRequest struct {
	Name              string                  `json:"name" validate:"required"`
	RedeemAfter       *time.Time              `json:"redeem_after,omitempty"`
	RedeemBefore      *time.Time              `json:"redeem_before,omitempty"`
	MaxRedemptions    *int                    `json:"max_redemptions,omitempty"`
	Rules             *map[string]interface{} `json:"rules,omitempty"`
	AmountOff         *decimal.Decimal        `json:"amount_off,omitempty"`
	PercentageOff     *decimal.Decimal        `json:"percentage_off,omitempty"`
	Type              types.CouponType        `json:"type" validate:"required,oneof=fixed percentage"`
	Cadence           types.CouponCadence     `json:"cadence" validate:"required,oneof=once repeated forever"`
	DurationInPeriods *int                    `json:"duration_in_periods,omitempty"`
	Metadata          *map[string]string      `json:"metadata,omitempty"`
	Currency          *string                 `json:"currency,omitempty"`
}

// UpdateCouponRequest represents the request to update an existing coupon
type UpdateCouponRequest struct {
	Name     *string            `json:"name,omitempty"`
	Metadata *map[string]string `json:"metadata,omitempty"`
}

// Validate validates the CreateCouponRequest
func (r *CreateCouponRequest) Validate() error {
	if r.Name == "" {
		return ierr.NewError("name is required").
			WithHint("Please provide a coupon name").
			Mark(ierr.ErrValidation)
	}

	if r.Type == "" {
		return ierr.NewError("type is required").
			WithHint("Please provide a discount type (fixed or percentage)").
			Mark(ierr.ErrValidation)
	}

	// Validate discount type specific fields
	switch r.Type {
	case types.CouponTypeFixed:
		if r.AmountOff == nil {
			return ierr.NewError("amount_off is required for fixed discount").
				WithHint("Please provide a valid discount amount").
				Mark(ierr.ErrValidation)
		}
		if r.AmountOff.LessThanOrEqual(decimal.Zero) {
			return ierr.NewError("amount_off must be greater than zero for fixed discount").
				WithHint("Please provide a valid discount amount").
				Mark(ierr.ErrValidation)
		}
	case types.CouponTypePercentage:
		if r.PercentageOff == nil {
			return ierr.NewError("percentage_off is required for percentage discount").
				WithHint("Please provide a valid discount percentage").
				Mark(ierr.ErrValidation)
		}
		if r.PercentageOff.LessThanOrEqual(decimal.Zero) || r.PercentageOff.GreaterThan(decimal.NewFromInt(100)) {
			return ierr.NewError("percentage_off must be between 0 and 100 for percentage discount").
				WithHint("Please provide a valid percentage between 0 and 100").
				Mark(ierr.ErrValidation)
		}
	}

	if r.RedeemAfter != nil && r.RedeemBefore != nil {
		if r.RedeemAfter.After(*r.RedeemBefore) {
			return ierr.NewError("redeem_after must be before redeem_before").
				WithHint("Please provide valid date range").
				Mark(ierr.ErrValidation)
		}
	}

	if r.MaxRedemptions != nil && *r.MaxRedemptions <= 0 {
		return ierr.NewError("max_redemptions must be greater than zero").
			WithHint("Please provide a valid maximum redemption count").
			Mark(ierr.ErrValidation)
	}

	// Validate duration_in_periods based on cadence
	if r.Cadence == types.CouponCadenceRepeated {
		if r.DurationInPeriods == nil {
			return ierr.NewError("duration_in_periods is required for repeated cadence").
				WithHint("Please specify how many billing periods this coupon should apply to").
				Mark(ierr.ErrValidation)
		}
		if *r.DurationInPeriods <= 0 {
			return ierr.NewError("duration_in_periods must be greater than zero for repeated cadence").
				WithHint("Please provide a valid number of billing periods").
				Mark(ierr.ErrValidation)
		}
	} else if r.DurationInPeriods != nil {
		// For non-repeated cadences, duration_in_periods should not be set
		return ierr.NewError("duration_in_periods should not be set for non-repeated cadence").
			WithHint("Duration in periods is only applicable for repeated cadence").
			Mark(ierr.ErrValidation)
	}

	return nil
}

// Validate validates the UpdateCouponRequest
func (r *UpdateCouponRequest) Validate() error {
	if r.Name != nil && *r.Name == "" {
		return ierr.NewError("name is required").
			WithHint("Please provide a coupon name").
			Mark(ierr.ErrValidation)
	}

	return nil
}

// CouponResponse represents the response for coupon data
type CouponResponse struct {
	*coupon.Coupon `json:",inline"`
}

// ListCouponsResponse represents the response for listing coupons
type ListCouponsResponse = types.ListResponse[*CouponResponse]
