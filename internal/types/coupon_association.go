package types

// DiscountType represents the type of coupon discount (fixed or percentage)
type DiscountType string

const (
	// DiscountTypeFixed represents a fixed amount coupon discount
	DiscountTypeFixed DiscountType = "fixed"
	// DiscountTypePercentage represents a percentage-based coupon discount
	DiscountTypePercentage DiscountType = "percentage"
)

// DiscountCadence represents the duration type of coupon discount
type DiscountCadence string

const (
	// DiscountCadenceOnce represents a one-time coupon discount
	DiscountCadenceOnce DiscountCadence = "once"
	// DiscountCadenceRepeated represents a coupon discount that repeats for a specific period
	DiscountCadenceRepeated DiscountCadence = "repeated"
	// DiscountCadenceForever represents a coupon discount that applies forever
	DiscountCadenceForever DiscountCadence = "forever"
)

// DiscountRule represents a rule for applying coupon discounts
type DiscountRule struct {
	Field    string      `json:"field"`    // Field to check (e.g., "customer_id", "plan_id", "amount")
	Operator string      `json:"operator"` // Operator (e.g., "equals", "greater_than", "in")
	Value    interface{} `json:"value"`    // Value to compare against
}

// DiscountRules represents a collection of coupon discount rules
type DiscountRules struct {
	Inclusions []DiscountRule `json:"inclusions"` // All conditions must be met (AND logic)
	Exclusions []DiscountRule `json:"exclusions"` // Any exclusion prevents application (OR logic)
}
