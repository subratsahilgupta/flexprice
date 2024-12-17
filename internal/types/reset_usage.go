package types

type ResetUsage string

const (
	ResetUsageBillingPeriod ResetUsage = "BILLING_PERIOD"
	ResetUsageNever         ResetUsage = "NEVER"
	// TODO: support more reset values like MONTHLY, WEEKLY, DAILY in future
)
