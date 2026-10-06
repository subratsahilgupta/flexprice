package service

import (
	"time"

	"github.com/flexprice/flexprice/internal/domain/invoice"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// ConvertInvoice converts a draft's amounts to the billing currency at a frozen rate, putting the
// rounding residual on one line so lines sum to the net. Tax is left to the caller.
func ConvertInvoice(inv *invoice.Invoice, resolution *FXRateResolution, convertedAt time.Time) error {
	if inv == nil {
		return ierr.NewError("invoice cannot be nil").WithHint("invoice cannot be nil").Mark(ierr.ErrValidation)
	}
	if resolution == nil {
		return ierr.NewError("fx resolution cannot be nil").WithHint("a resolved rate is required").Mark(ierr.ErrValidation)
	}

	chargeCurrency := inv.Currency
	billingCurrency := resolution.To

	// Defensive no-op: nothing to convert when the currencies match.
	if types.IsMatchingCurrency(chargeCurrency, billingCurrency) {
		return nil
	}

	rate := resolution.Rate
	if rate.LessThanOrEqual(decimal.Zero) {
		return ierr.NewError("fx rate must be positive").
			WithHint("Cannot convert an invoice with a non-positive rate").
			WithReportableDetails(map[string]any{"rate": rate.String()}).
			Mark(ierr.ErrValidation)
	}

	srcSubtotal := inv.Subtotal
	srcDiscount := inv.TotalDiscount
	srcPrepaid := inv.TotalPrepaidCreditsApplied
	netCharge := srcSubtotal.Sub(srcDiscount).Sub(srcPrepaid)
	netBilling := types.RoundToCurrencyPrecision(netCharge.Mul(rate), billingCurrency)

	// A rate too small for the billing currency's precision would issue a free invoice for a real charge.
	if !netCharge.IsZero() && netBilling.IsZero() {
		return ierr.NewError("fx rate too small for the billing currency precision").
			WithHintf("%s %s at %s rounds to zero %s; fix the rate before finalizing.", netCharge.String(), chargeCurrency, rate.String(), billingCurrency).
			WithReportableDetails(map[string]any{"net": netCharge.String(), "rate": rate.String(), "billing_currency": billingCurrency}).
			Mark(ierr.ErrInvalidOperation)
	}

	convert := func(v decimal.Decimal) decimal.Decimal {
		return types.RoundToCurrencyPrecision(v.Mul(rate), billingCurrency)
	}

	var roundingAdjustment decimal.Decimal
	roundingLineItemID := ""

	if len(inv.LineItems) > 0 {
		for _, li := range inv.LineItems {
			li.OriginalCurrency = lo.ToPtr(li.Currency)
			li.OriginalAmount = lo.ToPtr(li.Amount)
			li.Amount = convert(li.Amount)
			li.LineItemDiscount = convert(li.LineItemDiscount)
			li.InvoiceLevelDiscount = convert(li.InvoiceLevelDiscount)
			li.PrepaidCreditsApplied = convert(li.PrepaidCreditsApplied)
			li.Currency = billingCurrency
		}

		netFromLines := decimal.Zero
		for _, li := range inv.LineItems {
			netFromLines = netFromLines.Add(li.Amount).
				Sub(li.LineItemDiscount).Sub(li.InvoiceLevelDiscount).Sub(li.PrepaidCreditsApplied)
		}

		target := residualLine(inv.LineItems)
		roundingAdjustment = netBilling.Sub(netFromLines)
		target.Amount = target.Amount.Add(roundingAdjustment)
		roundingLineItemID = target.ID

		inv.Subtotal = decimal.Zero
		inv.TotalDiscount = decimal.Zero
		inv.TotalPrepaidCreditsApplied = decimal.Zero
		for _, li := range inv.LineItems {
			inv.Subtotal = inv.Subtotal.Add(li.Amount)
			inv.TotalDiscount = inv.TotalDiscount.Add(li.LineItemDiscount).Add(li.InvoiceLevelDiscount)
			inv.TotalPrepaidCreditsApplied = inv.TotalPrepaidCreditsApplied.Add(li.PrepaidCreditsApplied)
		}
	} else {
		inv.Subtotal = convert(srcSubtotal)
		inv.TotalDiscount = convert(srcDiscount)
		inv.TotalPrepaidCreditsApplied = convert(srcPrepaid)
	}

	// Pre-tax total; tax is added on the converted amounts in the finalize step that follows.
	inv.Total = inv.Subtotal.Sub(inv.TotalDiscount).Sub(inv.TotalPrepaidCreditsApplied)
	inv.AmountDue = inv.Total
	inv.AmountRemaining = inv.Total // nothing is paid before conversion
	inv.Currency = billingCurrency

	inv.FxConversion = &types.FxConversion{
		ChargeCurrency:  chargeCurrency,
		BillingCurrency: billingCurrency,
		Rate:            rate,
		RateID:          resolution.RateID,
		Scope:           resolution.Scope,
		ConvertedAt:     convertedAt,
		Source: types.FxConversionSource{
			Subtotal:                   srcSubtotal,
			TotalDiscount:              srcDiscount,
			TotalPrepaidCreditsApplied: srcPrepaid,
			Net:                        netCharge,
		},
		RoundingAdjustment: roundingAdjustment,
		RoundingLineItemID: roundingLineItemID,
	}

	return nil
}

// residualLine picks the line that absorbs the rounding residual: the largest positive amount,
// else the largest absolute amount; ties go to the lowest line id.
func residualLine(lines []*invoice.InvoiceLineItem) *invoice.InvoiceLineItem {
	var best *invoice.InvoiceLineItem
	for _, li := range lines {
		if best == nil {
			best = li
			continue
		}
		if betterResidualCandidate(li, best) {
			best = li
		}
	}
	return best
}

func betterResidualCandidate(candidate, best *invoice.InvoiceLineItem) bool {
	cPos := candidate.Amount.IsPositive()
	bPos := best.Amount.IsPositive()

	// A positive line always beats a non-positive one.
	if cPos != bPos {
		return cPos
	}

	if cPos {
		// Both positive: larger amount wins, lower id breaks ties.
		if candidate.Amount.Equal(best.Amount) {
			return candidate.ID < best.ID
		}
		return candidate.Amount.GreaterThan(best.Amount)
	}

	// Neither positive: larger absolute value wins, lower id breaks ties.
	cAbs := candidate.Amount.Abs()
	bAbs := best.Amount.Abs()
	if cAbs.Equal(bAbs) {
		return candidate.ID < best.ID
	}
	return cAbs.GreaterThan(bAbs)
}
