package service

import (
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/shopspring/decimal"
)

// recalculateTotalsFromLineItems recomputes inv.Subtotal from lineItems and re-derives
// inv.Total/AmountDue/AmountRemaining from the invoice's already-applied
// TotalDiscount/TotalTax/TotalPrepaidCreditsApplied. It does not filter
// lineItems by status - callers must pass only published, non-archived,
// non-deleted line items. It does not recompute TotalDiscount/TotalTax.
//
// Named distinctly from the existing recalculateInvoiceTotals(*dto.InvoiceResponse)
// in invoice.go (a different, unrelated helper operating on the response DTO) to
// avoid a method name collision on the same *invoiceService receiver.
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
