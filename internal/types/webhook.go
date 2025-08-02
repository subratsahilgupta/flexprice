package types

import (
	"encoding/json"
	"time"
)

// WebhookEvent represents a webhook event to be delivered
type WebhookEvent struct {
	ID            string          `json:"id"`
	EventName     string          `json:"event_name"`
	TenantID      string          `json:"tenant_id"`
	EnvironmentID string          `json:"environment_id"`
	UserID        string          `json:"user_id"`
	Timestamp     time.Time       `json:"timestamp"`
	Payload       json.RawMessage `json:"payload"`
}

// invoice event names
const (
	WebhookEventInvoiceCreateDraft = "invoice.create.drafted"
)

// subscription event names
const (
	WebhookEventSubscriptionCreated   = "subscription.created"
	WebhookEventSubscriptionPaused    = "subscription.paused"
	WebhookEventSubscriptionCancelled = "subscription.cancelled"
	WebhookEventSubscriptionResumed   = "subscription.resumed"
	WebhookEventSubscriptionUpdated   = "subscription.updated"
)

// feature event names
const (
	WebhookEventFeatureCreated = "feature.created"
	WebhookEventFeatureUpdated = "feature.updated"
	WebhookEventFeatureDeleted = "feature.deleted"
)

// entitlement event names
const (
	WebhookEventEntitlementCreated = "entitlement.created"
	WebhookEventEntitlementUpdated = "entitlement.updated"
	WebhookEventEntitlementDeleted = "entitlement.deleted"
)

// wallet event names
const (
	WebhookEventWalletCreated            = "wallet.created"
	WebhookEventWalletUpdated            = "wallet.updated"
	WebhookEventWalletTerminated         = "wallet.terminated"
	WebhookEventWalletTransactionCreated = "wallet.transaction.created"
)

// payment event names
const (
	WebhookEventPaymentCreated = "payment.created"
	WebhookEventPaymentUpdated = "payment.updated"
	WebhookEventPaymentFailed  = "payment.failed"
	WebhookEventPaymentSuccess = "payment.success"
	WebhookEventPaymentPending = "payment.pending"
)

// customer event names
const (
	WebhookEventCustomerCreated = "customer.created"
	WebhookEventCustomerUpdated = "customer.updated"
	WebhookEventCustomerDeleted = "customer.deleted"
)

// TODO: Below events should be cron triggered webhook event names
const (
	WebhookEventInvoiceUpdateFinalized = "invoice.update.finalized"
	WebhookEventInvoiceUpdatePayment   = "invoice.update.payment"
	WebhookEventInvoiceUpdateVoided    = "invoice.update.voided"

	WebhookEventInvoicePaymentOverdue = "invoice.payment.overdue"
)

// alert event names
const (
	WebhookEventWalletCreditBalanceDropped  = "wallet.credit_balance.dropped"
	WebhookEventWalletOngoingBalanceDropped = "wallet.ongoing_balance.dropped"
)
