package types

import "time"

type InvoiceCadence string

const (
	// InvoiceCadenceArrear raises an invoice at the end of each billing period (in arrears)
	InvoiceCadenceArrear InvoiceCadence = "ARREAR"
	// InvoiceCadenceAdvance raises an invoice at the beginning of each billing period (in advance)
	InvoiceCadenceAdvance InvoiceCadence = "ADVANCE"
)

type InvoiceStatus string

const (
	// InvoiceStatusDraft indicates invoice is in draft state and can be modified
	InvoiceStatusDraft InvoiceStatus = "DRAFT"
	// InvoiceStatusFinalized indicates invoice is finalized and ready for payment
	InvoiceStatusFinalized InvoiceStatus = "FINALIZED"
	// InvoiceStatusPaid indicates invoice has been paid
	InvoiceStatusPaid InvoiceStatus = "PAID"
	// InvoiceStatusVoided indicates invoice has been voided
	InvoiceStatusVoided InvoiceStatus = "VOIDED"
	// InvoiceStatusPartiallyPaid indicates invoice has been partially paid
	InvoiceStatusPartiallyPaid InvoiceStatus = "PARTIALLY_PAID"
	// InvoiceStatusUncollectible indicates invoice is uncollectible
	InvoiceStatusUncollectible InvoiceStatus = "UNCOLLECTIBLE"
)

type InvoiceBillingReason string

const (
	// InvoiceBillingReasonSubscriptionCreate indicates invoice is for subscription creation
	InvoiceBillingReasonSubscriptionCreate InvoiceBillingReason = "SUBSCRIPTION_CREATE"
	// InvoiceBillingReasonSubscriptionCycle indicates invoice is for subscription renewal
	InvoiceBillingReasonSubscriptionCycle InvoiceBillingReason = "SUBSCRIPTION_CYCLE"
	// InvoiceBillingReasonSubscriptionUpdate indicates invoice is for subscription update
	InvoiceBillingReasonSubscriptionUpdate InvoiceBillingReason = "SUBSCRIPTION_UPDATE"
	// InvoiceBillingReasonWalletTopup indicates invoice is for wallet topup
	InvoiceBillingReasonWalletTopup InvoiceBillingReason = "WALLET_TOPUP"
	// InvoiceBillingReasonManual indicates invoice is created manually
	InvoiceBillingReasonManual InvoiceBillingReason = "MANUAL"
)

// InvoiceFilter represents the filter options for listing invoices
type InvoiceFilter struct {
	CustomerID     string          `json:"customer_id,omitempty"`
	SubscriptionID string          `json:"subscription_id,omitempty"`
	WalletID       string          `json:"wallet_id,omitempty"`
	Status         []InvoiceStatus `json:"status,omitempty"`
	StartTime      *time.Time      `json:"start_time,omitempty"`
	EndTime        *time.Time      `json:"end_time,omitempty"`
	Limit          int             `json:"limit,omitempty"`
	Offset         int             `json:"offset,omitempty"`
}
