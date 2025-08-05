package coupon_application

import (
	"time"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// CouponApplication represents a coupon application to an invoice
type CouponApplication struct {
	ID                  string                 `json:"id" db:"id"`
	CouponID            string                 `json:"coupon_id" db:"coupon_id"`
	CouponAssociationID string                 `json:"coupon_association_id" db:"coupon_association_id"`
	InvoiceID           string                 `json:"invoice_id" db:"invoice_id"`
	InvoiceLineItemID   *string                `json:"invoice_line_item_id,omitempty" db:"invoice_line_item_id"`
	SubscriptionID      *string                `json:"subscription_id,omitempty" db:"subscription_id"`
	AppliedAt           time.Time              `json:"applied_at" db:"applied_at"`
	OriginalPrice       decimal.Decimal        `json:"original_price" db:"original_price"`
	FinalPrice          decimal.Decimal        `json:"final_price" db:"final_price"`
	DiscountedAmount    decimal.Decimal        `json:"discounted_amount" db:"discounted_amount"`
	DiscountType        types.CouponType       `json:"discount_type" db:"discount_type"`
	DiscountPercentage  *decimal.Decimal       `json:"discount_percentage,omitempty" db:"discount_percentage"`
	Currency            string                 `json:"currency" db:"currency"`
	CouponSnapshot      map[string]interface{} `json:"coupon_snapshot,omitempty" db:"coupon_snapshot"`
	Metadata            map[string]string      `json:"metadata,omitempty" db:"metadata"`
	EnvironmentID       string                 `json:"environment_id" db:"environment_id"`

	types.BaseModel
}

// IsLineItemLevel returns true if the coupon application is applied at invoice line item level
func (ca *CouponApplication) IsLineItemLevel() bool {
	return ca.InvoiceLineItemID != nil
}

// IsInvoiceLevel returns true if the coupon application is applied at invoice level
func (ca *CouponApplication) IsInvoiceLevel() bool {
	return ca.InvoiceLineItemID == nil
}

// GetDiscountPercentage returns the discount percentage as a decimal
func (ca *CouponApplication) GetDiscountPercentage() decimal.Decimal {
	if ca.DiscountPercentage != nil {
		return *ca.DiscountPercentage
	}
	return decimal.Zero
}

// GetDiscountRate returns the discount rate as a decimal (e.g., 0.10 for 10%)
func (ca *CouponApplication) GetDiscountRate() decimal.Decimal {
	if ca.DiscountType == types.CouponTypePercentage {
		return ca.GetDiscountPercentage().Div(decimal.NewFromInt(100))
	}
	return decimal.Zero
}

func FromEnt(e *ent.CouponApplication) *CouponApplication {
	if e == nil {
		return nil
	}

	ca := &CouponApplication{
		ID:                 e.ID,
		CouponID:           e.CouponID,
		InvoiceID:          e.InvoiceID,
		InvoiceLineItemID:  e.InvoiceLineItemID,
		SubscriptionID:     e.SubscriptionID,
		AppliedAt:          e.AppliedAt,
		OriginalPrice:      e.OriginalPrice,
		FinalPrice:         e.FinalPrice,
		DiscountedAmount:   e.DiscountedAmount,
		DiscountType:       types.CouponType(e.DiscountType),
		DiscountPercentage: e.DiscountPercentage,
		CouponSnapshot:     e.CouponSnapshot,
		Metadata:           e.Metadata,
		EnvironmentID:      e.EnvironmentID,
		BaseModel: types.BaseModel{
			TenantID:  e.TenantID,
			Status:    types.Status(e.Status),
			CreatedBy: e.CreatedBy,
			UpdatedBy: e.UpdatedBy,
			CreatedAt: e.CreatedAt,
			UpdatedAt: e.UpdatedAt,
		},
	}

	// Handle nullable fields
	if e.CouponAssociationID != nil {
		ca.CouponAssociationID = *e.CouponAssociationID
	}
	if e.Currency != nil {
		ca.Currency = *e.Currency
	}

	return ca
}

// FromEntList converts a list of ent.Coupon to domain Coupons
func FromEntList(list []*ent.CouponApplication) []*CouponApplication {
	if list == nil {
		return nil
	}
	couponApplications := make([]*CouponApplication, len(list))
	for i, item := range list {
		couponApplications[i] = FromEnt(item)
	}
	return couponApplications
}
