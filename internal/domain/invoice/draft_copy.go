package invoice

import (
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// CopyForDraftEdit returns a DRAFT copy of the invoice for the void-and-recreate edit
// flow. It carries the invoice's business data — description, billing period and the
// period start/end interval, due/issue dates, metadata, PDF URL, subtotal — and resets
// everything tied to the original's lifecycle: payment/refund/credit state (voiding
// refunds those to the customer), discount and tax (their backing records reference
// the original and are re-derived on the copy), invoice number (assigned at finalize),
// timestamps, and replacement lineage. Line items are the caller's responsibility.
func (i *Invoice) CopyForDraftEdit(id string, baseModel types.BaseModel) *Invoice {
	var metadata types.Metadata
	if i.Metadata != nil {
		metadata = lo.Assign(types.Metadata{}, i.Metadata)
	}

	return &Invoice{
		ID:                         id,
		CustomerID:                 i.CustomerID,
		SubscriptionID:             i.SubscriptionID,
		SubscriptionCustomerID:     i.SubscriptionCustomerID,
		InvoiceType:                i.InvoiceType,
		InvoiceStatus:              types.InvoiceStatusDraft,
		PaymentStatus:              types.PaymentStatusPending,
		Currency:                   i.Currency,
		AmountPaid:                 decimal.Zero,
		TotalPrepaidCreditsApplied: decimal.Zero,
		Subtotal:                   i.Subtotal,
		TotalDiscount:              decimal.Zero,
		TotalTax:                   decimal.Zero,
		Description:                i.Description,
		DueDate:                    i.DueDate,
		BillingPeriod:              i.BillingPeriod,
		IssueDate:                  i.IssueDate,
		PeriodStart:                i.PeriodStart,
		PeriodEnd:                  i.PeriodEnd,
		InvoicePDFURL:              i.InvoicePDFURL,
		BillingReason:              i.BillingReason,
		BillingSequence:            i.BillingSequence,
		Metadata:                   metadata,
		EnvironmentID:              i.EnvironmentID,
		// Deterministic key: a retried recreate resolves to the same replacement instead
		// of racing a duplicate (idempotency lookups exclude VOIDED invoices).
		IdempotencyKey:   lo.ToPtr("void_recreate-" + i.ID),
		IsManuallyEdited: true,
		Version:          1,
		BaseModel:        baseModel,
	}
}
