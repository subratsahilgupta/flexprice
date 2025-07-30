package coupon_association

import (
	"github.com/flexprice/flexprice/internal/types"
)

// CouponAssociation represents a coupon association with a subscription or subscription line item
// subscription_id is mandatory, subscription_line_item_id is optional
type CouponAssociation struct {
	ID                     string            `json:"id" db:"id"`
	CouponID               string            `json:"coupon_id" db:"coupon_id"`
	SubscriptionID         *string           `json:"subscription_id,omitempty" db:"subscription_id"`                     // Mandatory
	SubscriptionLineItemID *string           `json:"subscription_line_item_id,omitempty" db:"subscription_line_item_id"` // Optional
	Metadata               map[string]string `json:"metadata,omitempty" db:"metadata"`
	EnvironmentID          string            `json:"environment_id" db:"environment_id"`

	types.BaseModel
}

// IsSubscriptionLevel returns true if the coupon association is applied at subscription level
func (ca *CouponAssociation) IsSubscriptionLevel() bool {
	return ca.SubscriptionID != nil && ca.SubscriptionLineItemID == nil
}

// IsLineItemLevel returns true if the coupon association is applied at subscription line item level
func (ca *CouponAssociation) IsLineItemLevel() bool {
	return ca.SubscriptionLineItemID != nil
}

// GetTargetID returns the ID of the target (subscription or line item) for this coupon association
func (ca *CouponAssociation) GetTargetID() string {
	if ca.IsLineItemLevel() {
		return *ca.SubscriptionLineItemID
	}
	return *ca.SubscriptionID
}
