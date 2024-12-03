package types

// BillingModel is the billing model for the price ex FLAT_FEE, PACKAGE, TIERED, USAGE
type BillingModel string

// BillingPeriod is the billing period for the price ex MONTHLY, ANNUAL, WEEKLY, DAILY
type BillingPeriod string

// BillingCadence is the billing cadence for the price ex RECURRING, ONETIME
type BillingCadence string

// BillingTier when Billing model is TIERED defines how to
// calculate the price for a given quantity
type BillingTier string

const (
	// Billing model for a flat fee per unit
	BILLING_MODEL_FLAT_FEE BillingModel = "FLAT_FEE"

	// Billing model for a package of units ex 1000 emails for $100
	BILLING_MODEL_PACKAGE BillingModel = "PACKAGE"

	// Billing model for a tiered pricing model
	// ex 1-100 emails for $100, 101-1000 emails for $90
	BILLING_MODEL_TIERED BillingModel = "TIERED"

	// Billing model for a usage based pricing model
	BILLING_MODEL_USAGE BillingModel = "USAGE"

	// For BILLING_CADENCE_RECURRING
	BILLING_PERIOD_MONTHLY BillingPeriod = "MONTHLY"
	BILLING_PERIOD_ANNUAL  BillingPeriod = "ANNUAL"
	BILLING_PERIOD_WEEKLY  BillingPeriod = "WEEKLY"
	BILLING_PERIOD_DAILY   BillingPeriod = "DAILY"

	BILLING_CADENCE_RECURRING BillingCadence = "RECURRING"
	BILLING_CADENCE_ONETIME   BillingCadence = "ONETIME"

	// BILLING_TIER_VOLUME means all units price based on final tier reached.
	BILLING_TIER_VOLUME BillingTier = "VOLUME"

	// BILLING_TIER_SLAB means Tiers apply progressively as quantity increases
	BILLING_TIER_SLAB BillingTier = "SLAB"
)

type UsageType string

const (
	USAGE_TYPE_METERED  UsageType = "METERED"
	USAGE_TYPE_LICENSED UsageType = "LICENSED"
)
