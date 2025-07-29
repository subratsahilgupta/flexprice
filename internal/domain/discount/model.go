package discount

import (
	"time"

	"github.com/flexprice/flexprice/internal/types"
)

// Discount represents a coupon association with a subscription or subscription line item
type Discount struct {
	ID                     string       `json:"id" db:"id"`
	CouponID               string       `json:"coupon_id" db:"coupon_id"`
	SubscriptionID         *string      `json:"subscription_id,omitempty" db:"subscription_id"`
	SubscriptionLineItemID *string      `json:"subscription_line_item_id,omitempty" db:"subscription_line_item_id"`
	TenantID               string       `json:"tenant_id" db:"tenant_id"`
	Status                 types.Status `json:"status" db:"status"`
	CreatedAt              time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time    `json:"updated_at" db:"updated_at"`
	CreatedBy              string       `json:"created_by" db:"created_by"`
	UpdatedBy              string       `json:"updated_by" db:"updated_by"`
	EnvironmentID          string       `json:"environment_id" db:"environment_id"`
}

// IsSubscriptionLevel returns true if the discount is applied at subscription level
func (d *Discount) IsSubscriptionLevel() bool {
	return d.SubscriptionID != nil && d.SubscriptionLineItemID == nil
}

// IsLineItemLevel returns true if the discount is applied at subscription line item level
func (d *Discount) IsLineItemLevel() bool {
	return d.SubscriptionLineItemID != nil
}

// GetTargetID returns the ID of the target (subscription or line item) for this discount
func (d *Discount) GetTargetID() string {
	if d.IsLineItemLevel() {
		return *d.SubscriptionLineItemID
	}
	return *d.SubscriptionID
}
