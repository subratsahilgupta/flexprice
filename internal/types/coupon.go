package types

// CouponType represents the type of coupon discount (fixed or percentage)
type CouponType string

const (
	// CouponTypeFixed represents a fixed amount coupon discount
	CouponTypeFixed CouponType = "fixed"
	// CouponTypePercentage represents a percentage-based coupon discount
	CouponTypePercentage CouponType = "percentage"
)

// CouponCadence represents the duration type of coupon discount
type CouponCadence string

const (
	// CouponCadenceOnce represents a one-time coupon discount
	CouponCadenceOnce CouponCadence = "once"
	// CouponCadenceRepeated represents a coupon discount that repeats for a specific period
	CouponCadenceRepeated CouponCadence = "repeated"
	// CouponCadenceForever represents a coupon discount that applies forever
	CouponCadenceForever CouponCadence = "forever"
)

// CouponRule represents a rule for applying coupon discounts
type CouponRule struct {
	Field    string      `json:"field"`    // Field to check (e.g., "customer_id", "plan_id", "amount")
	Operator string      `json:"operator"` // Operator (e.g., "equals", "greater_than", "in")
	Value    interface{} `json:"value"`    // Value to compare against
}

// CouponRules represents a collection of coupon discount rules
type CouponRules struct {
	Inclusions []CouponRule `json:"inclusions"` // All conditions must be met (AND logic)
	Exclusions []CouponRule `json:"exclusions"` // Any exclusion prevents application (OR logic)
}
