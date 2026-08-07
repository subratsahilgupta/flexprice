package service

import (
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/shopspring/decimal"
)

// recalculateTotalsFromLineItems re-derives Total/AmountDue/AmountRemaining from the
// invoice's already-applied TotalDiscount/TotalTax/TotalPrepaidCreditsApplied. Callers
// must pass only published, non-archived, non-deleted line items - it does not filter,
// and it does not recompute TotalDiscount/TotalTax itself.
func (s *invoiceService) recalculateTotalsFromLineItems(inv *invoice.Invoice, lineItems []*invoice.InvoiceLineItem) {
	subtotal := decimal.Zero
	for _, li := range lineItems {
		subtotal = subtotal.Add(li.Amount)
	}
	inv.Subtotal = subtotal

	// Discount-first-then-tax: total = subtotal - prepaid credits - discount + tax
	inv.Total = inv.Subtotal.Sub(inv.TotalPrepaidCreditsApplied).Sub(inv.TotalDiscount).Add(inv.TotalTax)
	if inv.Total.IsNegative() {
		inv.Total = decimal.Zero
	}
	inv.AmountDue = inv.Total
	inv.AmountRemaining = inv.Total.Sub(inv.AmountPaid)
}
