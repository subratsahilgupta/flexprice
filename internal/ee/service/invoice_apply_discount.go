package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/coupon"
	"github.com/flexprice/flexprice/internal/domain/coupon_association"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// ApplyDiscountToInvoice recomputes this draft invoice's discount from its current standing
// CouponAssociation records. Safe to call any number of times: every call wipes whatever
// CouponApplication rows and line-item discount state it previously produced and rebuilds them
// from scratch, so repeated calls converge on the same result rather than compounding.
func (s *invoiceService) ApplyDiscountToInvoice(ctx context.Context, invoiceID string) (*dto.InvoiceResponse, error) {
	var lockedInv *invoice.Invoice

	err := s.DB.WithTx(ctx, func(txCtx context.Context) error {
		inv, err := s.InvoiceRepo.GetForUpdate(txCtx, invoiceID)
		if err != nil {
			return err
		}

		if inv.InvoiceStatus != types.InvoiceStatusDraft {
			return ierr.NewError("invoice is not in draft status").
				WithHint("Only draft invoices can have their discount reapplied").
				WithReportableDetails(map[string]interface{}{
					"invoice_id":     inv.ID,
					"current_status": inv.InvoiceStatus,
				}).
				Mark(ierr.ErrValidation)
		}

		if err := s.wipeCouponApplications(txCtx, inv); err != nil {
			return err
		}

		invoiceCoupons, lineItemCoupons, err := s.resolveCurrentInvoiceCoupons(txCtx, inv)
		if err != nil {
			return err
		}

		couponApplicationService := NewCouponApplicationService(s.ServiceParams)
		couponResult, err := couponApplicationService.ApplyCouponsToInvoice(txCtx, dto.ApplyCouponsToInvoiceRequest{
			Invoice:         inv,
			InvoiceCoupons:  invoiceCoupons,
			LineItemCoupons: lineItemCoupons,
		})
		if err != nil {
			return err
		}
		inv.TotalDiscount = couponResult.TotalDiscountAmount

		// Discount-first-then-tax, same formula as applyTaxesToInvoice: this endpoint may run
		// before or after tax has already been computed on this invoice, so it must not drop an
		// existing TotalTax contribution the way a naive Subtotal-minus-discount formula would.
		newTotal := inv.Subtotal.Sub(inv.TotalPrepaidCreditsApplied).Sub(inv.TotalDiscount).Add(inv.TotalTax)
		if newTotal.IsNegative() {
			newTotal = decimal.Zero
		}
		inv.Total = newTotal
		inv.AmountDue = inv.Total
		inv.AmountRemaining = inv.Total.Sub(inv.AmountPaid)

		if err := s.InvoiceRepo.Update(txCtx, inv); err != nil {
			return err
		}

		s.Logger.Info(txCtx, "reapplied discount to invoice",
			"invoice_id", inv.ID,
			"total_discount", inv.TotalDiscount,
			"new_total", inv.Total,
			"invoice_coupon_count", len(invoiceCoupons),
			"line_item_coupon_count", len(lineItemCoupons),
		)

		lockedInv = inv
		return nil
	})
	if err != nil {
		return nil, err
	}

	return dto.NewInvoiceResponse(lockedInv), nil
}

// wipeCouponApplications deletes every existing CouponApplication row for this invoice and
// resets each line item's LineItemDiscount to zero. Both are required for idempotency:
// ApplyCouponsToInvoice always inserts fresh CouponApplication rows and ADDS to
// LineItemDiscount rather than overwriting it, so without this in-memory reset a second call
// would double the discount even after the stale DB rows are deleted.
func (s *invoiceService) wipeCouponApplications(ctx context.Context, inv *invoice.Invoice) error {
	filter := types.NewNoLimitCouponApplicationFilter()
	filter.InvoiceIDs = []string{inv.ID}
	existing, err := s.CouponApplicationRepo.List(ctx, filter)
	if err != nil {
		return ierr.WithError(err).
			WithHint("failed to list existing coupon applications").
			Mark(ierr.ErrDatabase)
	}

	for _, app := range existing {
		if err := s.CouponApplicationRepo.Delete(ctx, app.ID); err != nil {
			return ierr.WithError(err).
				WithHint("failed to delete existing coupon application").
				Mark(ierr.ErrDatabase)
		}
	}

	for _, li := range inv.LineItems {
		li.LineItemDiscount = decimal.Zero
	}

	return nil
}

// resolveCurrentInvoiceCoupons resolves the coupons currently standing against this invoice's
// subscription (via CouponAssociation), scoped to the invoice's billing period and filtered to
// price IDs actually present on its line items. Returns two empty slices (not an error) for
// one-off/credit invoices with no subscription, or invoices missing period dates.
func (s *invoiceService) resolveCurrentInvoiceCoupons(ctx context.Context, inv *invoice.Invoice) ([]dto.InvoiceCoupon, []dto.InvoiceLineItemCoupon, error) {
	if inv.SubscriptionID == nil || inv.PeriodStart == nil || inv.PeriodEnd == nil {
		return []dto.InvoiceCoupon{}, []dto.InvoiceLineItemCoupon{}, nil
	}

	sub, _, err := s.SubRepo.GetWithLineItems(ctx, *inv.SubscriptionID)
	if err != nil {
		return nil, nil, err
	}

	couponValidationService := NewCouponValidationService(s.ServiceParams)
	filter := func(c *coupon.Coupon, _ *coupon_association.CouponAssociation) bool {
		return couponValidationService.ValidateCoupon(ctx, *c, sub) == nil
	}
	sel, err := selectSubscriptionCoupons(ctx, s.ServiceParams, []*subscription.Subscription{sub}, *inv.PeriodStart, *inv.PeriodEnd, filter)
	if err != nil {
		return nil, nil, err
	}

	invoiceLineItemPriceIDs := make(map[string]bool)
	for _, li := range inv.LineItems {
		if li.PriceID != nil {
			invoiceLineItemPriceIDs[*li.PriceID] = true
		}
	}

	invoiceCoupons := make([]dto.InvoiceCoupon, 0)
	for _, assocs := range sel.SubLevel {
		for _, assoc := range assocs {
			invoiceCoupons = append(invoiceCoupons, dto.InvoiceCoupon{
				CouponID:            assoc.CouponID,
				CouponAssociationID: &assoc.ID,
			})
		}
	}

	lineItemCoupons := make([]dto.InvoiceLineItemCoupon, 0)
	for sliID, assocs := range sel.LineLevel {
		priceID, ok := sel.SubLineItemIDToPriceID[sliID]
		if !ok || priceID == "" || !invoiceLineItemPriceIDs[priceID] {
			continue
		}
		for _, assoc := range assocs {
			lineItemCoupons = append(lineItemCoupons, dto.InvoiceLineItemCoupon{
				LineItemID:          priceID,
				CouponID:            assoc.CouponID,
				CouponAssociationID: &assoc.ID,
			})
		}
	}

	return invoiceCoupons, lineItemCoupons, nil
}
