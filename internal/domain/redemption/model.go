package redemption

import (
	"time"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// Redemption represents a coupon application to an invoice
type Redemption struct {
	ID                 string                 `json:"id" db:"id"`
	CouponID           string                 `json:"coupon_id" db:"coupon_id"`
	DiscountID         string                 `json:"discount_id" db:"discount_id"`
	InvoiceID          string                 `json:"invoice_id" db:"invoice_id"`
	InvoiceLineItemID  *string                `json:"invoice_line_item_id,omitempty" db:"invoice_line_item_id"`
	RedeemedAt         time.Time              `json:"redeemed_at" db:"redeemed_at"`
	OriginalPrice      decimal.Decimal        `json:"original_price" db:"original_price"`
	FinalPrice         decimal.Decimal        `json:"final_price" db:"final_price"`
	DiscountedAmount   decimal.Decimal        `json:"discounted_amount" db:"discounted_amount"`
	DiscountType       types.DiscountType     `json:"discount_type" db:"discount_type"`
	DiscountPercentage *decimal.Decimal       `json:"discount_percentage,omitempty" db:"discount_percentage"`
	Currency           string                 `json:"currency" db:"currency"`
	CouponSnapshot     map[string]interface{} `json:"coupon_snapshot,omitempty" db:"coupon_snapshot"`
	TenantID           string                 `json:"tenant_id" db:"tenant_id"`
	Status             types.Status           `json:"status" db:"status"`
	CreatedAt          time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at" db:"updated_at"`
	CreatedBy          string                 `json:"created_by" db:"created_by"`
	UpdatedBy          string                 `json:"updated_by" db:"updated_by"`
	EnvironmentID      string                 `json:"environment_id" db:"environment_id"`
}

// IsLineItemLevel returns true if the redemption is applied at invoice line item level
func (r *Redemption) IsLineItemLevel() bool {
	return r.InvoiceLineItemID != nil
}

// IsInvoiceLevel returns true if the redemption is applied at invoice level
func (r *Redemption) IsInvoiceLevel() bool {
	return r.InvoiceLineItemID == nil
}

// GetDiscountPercentage returns the discount percentage as a decimal
func (r *Redemption) GetDiscountPercentage() decimal.Decimal {
	if r.DiscountPercentage != nil {
		return *r.DiscountPercentage
	}
	return decimal.Zero
}

// GetDiscountRate returns the discount rate as a decimal (e.g., 0.10 for 10%)
func (r *Redemption) GetDiscountRate() decimal.Decimal {
	if r.DiscountType == types.DiscountTypePercentage {
		return r.GetDiscountPercentage().Div(decimal.NewFromInt(100))
	}
	return decimal.Zero
}
