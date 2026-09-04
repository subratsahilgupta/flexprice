package invoice

import (
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// CopyForDraftEdit returns a DRAFT copy of the invoice for the void-and-recreate edit
// flow, built through NewInvoiceBuilder. The seeded copy carries the invoice's business
// data — description, billing period and the period start/end interval, due/issue dates,
// metadata, PDF URL, subtotal — and the chain resets everything tied to the original's
// lifecycle:
//   - payment/refund/credit state: voiding refunds those to the customer, so the draft
//     starts clean (PENDING, nothing paid or applied)
//   - discount and tax: their backing records (coupon applications, tax-applied rows)
//     reference the original; callers re-derive them on the copy
//   - invoice number (assigned at finalize), lifecycle timestamps, and replacement lineage
//
// Line items are separate entities and are the caller's responsibility.
func (i *Invoice) CopyForDraftEdit(id string, baseModel types.BaseModel) *Invoice {
	return NewInvoiceBuilder(i).
		WithID(id).
		WithInvoiceStatus(types.InvoiceStatusDraft).
		WithPaymentStatus(types.PaymentStatusPending).
		WithAmountPaid(decimal.Zero).
		WithTotalPrepaidCreditsApplied(decimal.Zero).
		WithRefundedAmount(decimal.Zero).
		WithAdjustmentAmount(decimal.Zero).
		WithTotalDiscount(decimal.Zero).
		WithTotalTax(decimal.Zero).
		WithTaxExemptionReasonCode(nil).
		WithInvoiceNumber(nil).
		// Deterministic key: a retried recreate resolves to the same replacement instead
		// of racing a duplicate (idempotency lookups exclude VOIDED invoices).
		WithIdempotencyKey(lo.ToPtr("void_recreate-" + i.ID)).
		WithRecalculatedInvoiceID(nil).
		WithVoidedAt(nil).
		WithFinalizedAt(nil).
		WithPaidAt(nil).
		WithLastComputedAt(nil).
		WithLineItems(nil).
		WithCouponApplications(nil).
		WithIsManuallyEdited(true).
		WithVersion(1).
		WithBaseModel(baseModel).
		Build()
}
