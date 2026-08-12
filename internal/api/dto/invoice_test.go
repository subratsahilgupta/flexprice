package dto

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestCreateInvoiceRequest_ZeroOutAmounts(t *testing.T) {
	req := CreateInvoiceRequest{
		Subtotal:  decimal.NewFromInt(99),
		Total:     decimal.NewFromInt(99),
		AmountDue: decimal.NewFromInt(99),
		LineItems: []CreateInvoiceLineItemRequest{
			{Amount: decimal.NewFromInt(99), Quantity: decimal.NewFromInt(2)},
			{Amount: decimal.NewFromInt(49), Quantity: decimal.NewFromInt(1)},
		},
	}

	req.ZeroOutAmounts()

	assert.True(t, req.Subtotal.IsZero(), "Subtotal must be zero")
	assert.True(t, req.Total.IsZero(), "Total must be zero")
	assert.True(t, req.AmountDue.IsZero(), "AmountDue must be zero")

	for i, li := range req.LineItems {
		assert.True(t, li.Amount.IsZero(), "line item %d Amount must be zero", i)
		// Quantity is deliberately preserved — it shows the pricing skeleton.
		assert.False(t, li.Quantity.IsZero(), "line item %d Quantity must be preserved", i)
	}
}

func TestCreateInvoiceRequest_ZeroOutAmounts_EmptyLineItems(t *testing.T) {
	req := CreateInvoiceRequest{
		Subtotal:  decimal.NewFromInt(50),
		Total:     decimal.NewFromInt(50),
		AmountDue: decimal.NewFromInt(50),
	}
	req.ZeroOutAmounts() // must not panic on nil/empty LineItems
	assert.True(t, req.Subtotal.IsZero())
	assert.True(t, req.Total.IsZero())
	assert.True(t, req.AmountDue.IsZero())
}

// A line tied to a subscription line item is the billed history proration reads
// back by service period. Stored without bounds it is invisible to that read, and
// the customer silently loses the credit for it when the line is removed.
func TestCreateInvoiceLineItemRequest_Validate_SubscriptionLineNeedsItsPeriod(t *testing.T) {
	lineItemID := "subs_line_1"
	periodStart := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)

	withPeriod := CreateInvoiceLineItemRequest{
		Amount:                 decimal.NewFromInt(20),
		Quantity:               decimal.NewFromInt(1),
		SubscriptionLineItemID: &lineItemID,
		PeriodStart:            &periodStart,
		PeriodEnd:              &periodEnd,
	}
	assert.NoError(t, withPeriod.Validate(types.InvoiceTypeOneOff))

	missingEnd := withPeriod
	missingEnd.PeriodEnd = nil
	assert.Error(t, missingEnd.Validate(types.InvoiceTypeOneOff))

	missingBoth := withPeriod
	missingBoth.PeriodStart, missingBoth.PeriodEnd = nil, nil
	assert.Error(t, missingBoth.Validate(types.InvoiceTypeOneOff))

	unlinked := missingBoth
	unlinked.SubscriptionLineItemID = nil
	assert.NoError(t, unlinked.Validate(types.InvoiceTypeOneOff), "a line with no subscription line item is unaffected")
}
