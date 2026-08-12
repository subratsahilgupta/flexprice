package service

import (
	"testing"

	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestRecalculateInvoiceTotals(t *testing.T) {
	s := &invoiceService{}

	t.Run("normal case sums line items and derives totals", func(t *testing.T) {
		inv := &invoice.Invoice{
			TotalDiscount:              decimal.NewFromInt(10),
			TotalTax:                   decimal.NewFromInt(5),
			TotalPrepaidCreditsApplied: decimal.NewFromInt(20),
			AmountPaid:                 decimal.NewFromInt(30),
		}
		lineItems := []*invoice.InvoiceLineItem{
			{Amount: decimal.NewFromInt(100)},
			{Amount: decimal.NewFromInt(50)},
		}

		s.recalculateTotalsFromLineItems(inv, lineItems)

		// subtotal = 150
		assert.True(t, decimal.NewFromInt(150).Equal(inv.Subtotal))
		// total = 150 - 20 - 10 + 5 = 125
		assert.True(t, decimal.NewFromInt(125).Equal(inv.Total))
		assert.True(t, decimal.NewFromInt(125).Equal(inv.AmountDue))
		// amount_remaining = 125 - 30 = 95
		assert.True(t, decimal.NewFromInt(95).Equal(inv.AmountRemaining))

		// TotalDiscount / TotalTax must be untouched
		assert.True(t, decimal.NewFromInt(10).Equal(inv.TotalDiscount))
		assert.True(t, decimal.NewFromInt(5).Equal(inv.TotalTax))
	})

	t.Run("floors total at zero when discount and tax exceed subtotal", func(t *testing.T) {
		inv := &invoice.Invoice{
			TotalDiscount:              decimal.NewFromInt(80),
			TotalTax:                   decimal.NewFromInt(0),
			TotalPrepaidCreditsApplied: decimal.NewFromInt(30),
			AmountPaid:                 decimal.NewFromInt(0),
		}
		lineItems := []*invoice.InvoiceLineItem{
			{Amount: decimal.NewFromInt(50)},
		}

		s.recalculateTotalsFromLineItems(inv, lineItems)

		// subtotal = 50; raw total = 50 - 30 - 80 + 0 = -60 -> floored to 0
		assert.True(t, decimal.NewFromInt(50).Equal(inv.Subtotal))
		assert.True(t, decimal.Zero.Equal(inv.Total))
		assert.True(t, decimal.Zero.Equal(inv.AmountDue))
		assert.True(t, decimal.Zero.Equal(inv.AmountRemaining))
	})

	t.Run("empty line items yields zero subtotal", func(t *testing.T) {
		inv := &invoice.Invoice{
			TotalDiscount:              decimal.Zero,
			TotalTax:                   decimal.NewFromInt(3),
			TotalPrepaidCreditsApplied: decimal.Zero,
			AmountPaid:                 decimal.Zero,
		}

		s.recalculateTotalsFromLineItems(inv, []*invoice.InvoiceLineItem{})

		assert.True(t, decimal.Zero.Equal(inv.Subtotal))
		// total = 0 - 0 - 0 + 3 = 3
		assert.True(t, decimal.NewFromInt(3).Equal(inv.Total))
	})
}
