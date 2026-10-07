package service

import (
	"context"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// rejectPrepaidCrossCurrencyOneOff rejects a pre-paid one-off for a customer billed in another
// currency: the amount is only known in that currency after finalize.
func (s *invoiceService) rejectPrepaidCrossCurrencyOneOff(ctx context.Context, req dto.CreateInvoiceRequest) error {
	hasPayment := (req.AmountPaid != nil && !req.AmountPaid.IsZero()) ||
		(req.PaymentStatus != nil && *req.PaymentStatus != types.PaymentStatusPending)
	if !hasPayment {
		return nil
	}

	cust, err := s.CustomerRepo.Get(ctx, req.CustomerID)
	if err != nil {
		return err
	}
	if cust.BillingCurrency == nil || *cust.BillingCurrency == "" {
		return nil
	}
	if types.IsMatchingCurrency(*cust.BillingCurrency, req.Currency) {
		return nil
	}

	return ierr.NewError("cannot create a pre-paid invoice for a cross-currency customer").
		WithHintf("This customer is billed in %s; create the invoice without payment, then pay it once it is finalized in %s.",
			strings.ToUpper(*cust.BillingCurrency), strings.ToUpper(*cust.BillingCurrency)).
		WithReportableDetails(map[string]any{
			"request_currency": req.Currency,
			"billing_currency": *cust.BillingCurrency,
		}).
		Mark(ierr.ErrValidation)
}

// conversionTarget returns the billing currency an unconverted invoice must be converted to, or "".
func (s *invoiceService) conversionTarget(ctx context.Context, inv *invoice.Invoice) (string, error) {
	if inv.FxConversion != nil || inv.CustomCurrency != nil {
		return "", nil
	}
	cust, err := s.CustomerRepo.Get(ctx, inv.CustomerID)
	if err != nil {
		return "", err
	}
	billing := lo.FromPtr(cust.BillingCurrency)
	if billing == "" || types.IsMatchingCurrency(billing, inv.Currency) {
		return "", nil
	}
	return billing, nil
}

// convertAndRetaxInvoice converts an invoice to its customer's billing currency once and re-taxes it.
// No-op when nothing needs converting or the invoice is already paid; a missing rate errors.
func (s *invoiceService) convertAndRetaxInvoice(ctx context.Context, inv *invoice.Invoice) error {
	if inv.FxConversion != nil || inv.CustomCurrency != nil {
		return nil
	}
	if !inv.AmountPaid.IsZero() {
		s.Logger.Info(ctx, "skipping fx conversion: invoice already carries a payment",
			"invoice_id", inv.ID, "amount_paid", inv.AmountPaid.String(), "currency", inv.Currency)
		return nil
	}

	billing, err := s.conversionTarget(ctx, inv)
	if err != nil || billing == "" {
		return err
	}

	fxSvc := NewFXRateService(s.ServiceParams)
	resolution, err := fxSvc.ResolveRate(ctx, ResolveFXRateRequest{
		From:           inv.Currency,
		To:             billing,
		CustomerID:     inv.CustomerID,
		SubscriptionID: lo.FromPtr(inv.SubscriptionID),
	})
	if err != nil {
		s.Logger.Error(ctx, "cannot convert invoice: no fx rate for the pair",
			"error", err, "invoice_id", inv.ID, "from", inv.Currency, "to", billing)
		// A fresh error, not a wrap: the resolver's not-found would otherwise also match and make
		// the HTTP status non-deterministic. Invalid-operation keeps Temporal from retrying.
		return ierr.NewErrorf("no exchange rate configured for %s to %s", inv.Currency, billing).
			WithHintf("No exchange rate configured for %s to %s; set a rate for the pair, then retry.", inv.Currency, billing).
			WithReportableDetails(map[string]any{"from": inv.Currency, "to": billing}).
			Mark(ierr.ErrInvalidOperation)
	}

	if inv.LineItems == nil {
		lineItems, err := s.InvoiceLineItemRepo.ListByInvoiceID(ctx, inv.ID)
		if err != nil {
			return err
		}
		inv.LineItems = lineItems
	}

	if err := ConvertInvoice(inv, resolution, time.Now().UTC()); err != nil {
		return err
	}

	for _, item := range inv.LineItems {
		if err := s.InvoiceLineItemRepo.Update(ctx, item); err != nil {
			return err
		}
	}
	if err := s.InvoiceRepo.Update(ctx, inv); err != nil {
		return err
	}

	return s.recomputeTaxOnConvertedInvoice(ctx, inv)
}

// recomputeTaxOnConvertedInvoice re-taxes a converted invoice: subscription invoices from their
// tax associations, others from the rates recorded on their tax_applied rows.
func (s *invoiceService) recomputeTaxOnConvertedInvoice(ctx context.Context, inv *invoice.Invoice) error {
	if inv.InvoiceType == types.InvoiceTypeSubscription && inv.SubscriptionID != nil {
		_, err := s.RecalculateTaxesOnInvoice(ctx, inv)
		return err
	}

	taxService := NewTaxService(s.ServiceParams)
	taxRates, err := taxService.PrepareTaxRatesFromApplied(ctx, inv)
	if err != nil {
		return err
	}

	// No tax rows: zero the tax and recompute totals in the billing currency.
	if len(taxRates.GetRates()) == 0 {
		applyTaxResultToInvoice(inv, &TaxCalculationResult{
			TotalTaxAmount:    decimal.Zero,
			TaxAppliedRecords: []*dto.TaxAppliedResponse{},
		})
		return s.InvoiceRepo.Update(ctx, inv)
	}

	result, err := taxService.ApplyTaxesOnInvoice(ctx, inv, taxRates)
	if err != nil {
		return err
	}
	applyTaxResultToInvoice(inv, result)
	return s.InvoiceRepo.Update(ctx, inv)
}
