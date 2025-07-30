package types

// CouponAssociationType represents the type of coupon association
type CouponAssociationType string

const (
	// CouponAssociationTypeSubscription represents a subscription-level coupon association
	CouponAssociationTypeSubscription CouponAssociationType = "subscription"
	// CouponAssociationTypeLineItem represents a line-item-level coupon association
	CouponAssociationTypeLineItem CouponAssociationType = "line_item"
)
