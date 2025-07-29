package coupon

import (
	"time"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// Coupon represents a discount coupon entity
type Coupon struct {
	ID               string                 `json:"id" db:"id"`
	Name             string                 `json:"name" db:"name"`
	RedeemAfter      *time.Time             `json:"redeem_after,omitempty" db:"redeem_after"`
	RedeemBefore     *time.Time             `json:"redeem_before,omitempty" db:"redeem_before"`
	MaxRedemptions   *int                   `json:"max_redemptions,omitempty" db:"max_redemptions"`
	TotalRedemptions int                    `json:"total_redemptions" db:"total_redemptions"`
	Rules            map[string]interface{} `json:"rules,omitempty" db:"rules"`
	AmountOff        decimal.Decimal        `json:"amount_off" db:"amount_off"`
	PercentageOff    decimal.Decimal        `json:"percentage_off" db:"percentage_off"`
	Type             types.DiscountType     `json:"type" db:"type"`
	Cadence          types.DiscountCadence  `json:"cadence" db:"cadence"`
	IsActive         bool                   `json:"is_active" db:"is_active"`
	Currency         string                 `json:"currency" db:"currency"`
	TenantID         string                 `json:"tenant_id" db:"tenant_id"`
	Status           types.Status           `json:"status" db:"status"`
	CreatedAt        time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at" db:"updated_at"`
	CreatedBy        string                 `json:"created_by" db:"created_by"`
	UpdatedBy        string                 `json:"updated_by" db:"updated_by"`
	EnvironmentID    string                 `json:"environment_id" db:"environment_id"`
}

// IsValid checks if the coupon is valid for redemption
func (c *Coupon) IsValid() bool {
	if !c.IsActive {
		return false
	}

	now := time.Now()

	// Check if coupon is within valid date range
	if c.RedeemAfter != nil && now.Before(*c.RedeemAfter) {
		return false
	}

	if c.RedeemBefore != nil && now.After(*c.RedeemBefore) {
		return false
	}

	// Check if coupon has reached maximum redemptions
	if c.MaxRedemptions != nil && c.TotalRedemptions >= *c.MaxRedemptions {
		return false
	}

	return true
}

// CalculateDiscount calculates the discount amount for a given price
func (c *Coupon) CalculateDiscount(originalPrice decimal.Decimal) decimal.Decimal {
	switch c.Type {
	case types.DiscountTypeFixed:
		return c.AmountOff
	case types.DiscountTypePercentage:
		return originalPrice.Mul(c.PercentageOff).Div(decimal.NewFromInt(100))
	default:
		return decimal.Zero
	}
}

// ApplyDiscount applies the discount to a given price and returns the final price
func (c *Coupon) ApplyDiscount(originalPrice decimal.Decimal) decimal.Decimal {
	discount := c.CalculateDiscount(originalPrice)
	finalPrice := originalPrice.Sub(discount)

	// Ensure final price doesn't go below zero
	if finalPrice.LessThan(decimal.Zero) {
		return decimal.Zero
	}

	return finalPrice
}
