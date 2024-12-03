package types

type BillingModel string
type BillingPeriod string
type BillingCadence string
type BillingTier string

const (
	BILLING_MODEL_FLAT_FEE BillingModel = "FLAT_FEE"
	BILLING_MODEL_PER_UNIT BillingModel = "PER_UNIT"

	// For BILLING_CADENCE_RECURRING
	BILLING_PERIOD_MONTHLY BillingPeriod = "MONTHLY"
	BILLING_PERIOD_ANNUAL  BillingPeriod = "ANNUAL"
	BILLING_PERIOD_WEEKLY  BillingPeriod = "WEEKLY"
	BILLING_PERIOD_DAILY   BillingPeriod = "DAILY"

	BILLING_CADENCE_RECURRING BillingCadence = "RECURRING"
	BILLING_CADENCE_ONETIME   BillingCadence = "ONETIME"

	// For BILLING_MODEL_PER_UNIT
	BILLING_TIER_FLAT   BillingTier = "FLAT"
	BILLING_TIER_VOLUME BillingTier = "VOLUME"
	BILLING_TIER_SLAB   BillingTier = "SLAB"
)
