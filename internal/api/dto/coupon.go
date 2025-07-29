package dto

import (
	"time"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// CreateCouponRequest represents the request to create a new coupon
type CreateCouponRequest struct {
	Name           string                 `json:"name" validate:"required"`
	RedeemAfter    *time.Time             `json:"redeem_after,omitempty"`
	RedeemBefore   *time.Time             `json:"redeem_before,omitempty"`
	MaxRedemptions *int                   `json:"max_redemptions,omitempty"`
	Rules          map[string]interface{} `json:"rules,omitempty"`
	AmountOff      decimal.Decimal        `json:"amount_off"`
	PercentageOff  decimal.Decimal        `json:"percentage_off"`
	Type           types.DiscountType     `json:"type" validate:"required,oneof=fixed percentage"`
	Cadence        types.DiscountCadence  `json:"cadence" validate:"required,oneof=once repeated forever"`
	Currency       string                 `json:"currency" validate:"required"`
}

// UpdateCouponRequest represents the request to update an existing coupon
type UpdateCouponRequest struct {
	Name           *string                 `json:"name,omitempty"`
	RedeemAfter    *time.Time              `json:"redeem_after,omitempty"`
	RedeemBefore   *time.Time              `json:"redeem_before,omitempty"`
	MaxRedemptions *int                    `json:"max_redemptions,omitempty"`
	Rules          *map[string]interface{} `json:"rules,omitempty"`
	AmountOff      *decimal.Decimal        `json:"amount_off,omitempty"`
	PercentageOff  *decimal.Decimal        `json:"percentage_off,omitempty"`
	Type           *types.DiscountType     `json:"type,omitempty" validate:"omitempty,oneof=fixed percentage"`
	Cadence        *types.DiscountCadence  `json:"cadence,omitempty" validate:"omitempty,oneof=once repeated forever"`
	Currency       *string                 `json:"currency,omitempty"`
}

// CouponResponse represents the response for coupon data
type CouponResponse struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	RedeemAfter      *time.Time             `json:"redeem_after,omitempty"`
	RedeemBefore     *time.Time             `json:"redeem_before,omitempty"`
	MaxRedemptions   *int                   `json:"max_redemptions,omitempty"`
	TotalRedemptions int                    `json:"total_redemptions"`
	Rules            map[string]interface{} `json:"rules,omitempty"`
	AmountOff        decimal.Decimal        `json:"amount_off"`
	PercentageOff    decimal.Decimal        `json:"percentage_off"`
	Type             types.DiscountType     `json:"type"`
	Cadence          types.DiscountCadence  `json:"cadence"`
	IsActive         bool                   `json:"is_active"`
	Currency         string                 `json:"currency"`
	Status           types.Status           `json:"status"`
	TenantID         string                 `json:"tenant_id"`
	EnvironmentID    string                 `json:"environment_id"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	CreatedBy        string                 `json:"created_by"`
	UpdatedBy        string                 `json:"updated_by"`
}

// ListCouponsRequest represents the request to list coupons
type ListCouponsRequest struct {
	IsActive   *bool                    `json:"is_active,omitempty"`
	Type       *types.DiscountType      `json:"type,omitempty"`
	Cadence    *types.DiscountCadence   `json:"cadence,omitempty"`
	Pagination types.PaginationResponse `json:"pagination"`
}

// ListCouponsResponse represents the response for listing coupons
type ListCouponsResponse struct {
	Coupons    []*CouponResponse        `json:"coupons"`
	Pagination types.PaginationResponse `json:"pagination"`
	TotalCount int                      `json:"total_count"`
}
