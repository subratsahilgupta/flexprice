package types

// AmortizationMethod weights the days of a revenue schedule segment.
type AmortizationMethod string

const (
	AmortizationDaily              AmortizationMethod = "daily"
	AmortizationPeriodEven         AmortizationMethod = "period_even"
	AmortizationPeriodEvenProrated AmortizationMethod = "period_even_prorated"
)

type ContractStatus string

const (
	ContractStatusDraft     ContractStatus = "draft"
	ContractStatusActive    ContractStatus = "active"
	ContractStatusEnded     ContractStatus = "ended"
	ContractStatusCancelled ContractStatus = "cancelled"
)

type PerformanceObligationStatus string

const (
	PerformanceObligationStatusDraft     PerformanceObligationStatus = "draft"
	PerformanceObligationStatusActive    PerformanceObligationStatus = "active"
	PerformanceObligationStatusSatisfied PerformanceObligationStatus = "satisfied"
	PerformanceObligationStatusCancelled PerformanceObligationStatus = "cancelled"
)

type AccountingPeriodStatus string

const (
	AccountingPeriodStatusOpen   AccountingPeriodStatus = "open"
	AccountingPeriodStatusClosed AccountingPeriodStatus = "closed" // reserved; close is deferred
)

// RevRecEntityType is the kind of entity a revrec record links to or is sourced from.
type RevRecEntityType string

const (
	RevRecEntityTypeSubscription         RevRecEntityType = "subscription"
	RevRecEntityTypeSubscriptionLineItem RevRecEntityType = "subscription_line_item"
)
