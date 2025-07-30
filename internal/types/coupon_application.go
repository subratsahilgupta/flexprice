package types

// CouponApplicationType represents the type of coupon application
type CouponApplicationType string

const (
	// CouponApplicationTypeInvoice represents an invoice-level coupon application
	CouponApplicationTypeInvoice CouponApplicationType = "invoice"
	// CouponApplicationTypeLineItem represents a line-item-level coupon application
	CouponApplicationTypeLineItem CouponApplicationType = "line_item"
)

// CouponApplicationStatus represents the status of a coupon application
type CouponApplicationStatus string

const (
	// CouponApplicationStatusApplied represents a successfully applied coupon
	CouponApplicationStatusApplied CouponApplicationStatus = "applied"
	// CouponApplicationStatusReversed represents a reversed coupon application
	CouponApplicationStatusReversed CouponApplicationStatus = "reversed"
	// CouponApplicationStatusExpired represents an expired coupon application
	CouponApplicationStatusExpired CouponApplicationStatus = "expired"
)
