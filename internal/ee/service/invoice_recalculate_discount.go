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
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// RecalculateDiscountOnInvoice recomputes this draft invoice's discount from its current
// standing CouponAssociation records. Idempotent: wipes and rebuilds from scratch each call.
func (s *invoiceService) RecalculateDiscountOnInvoice(ctx context.Context, invoiceID string) (*dto.InvoiceResponse, error) {
	var lockedInv *invoice.Invoice

	err := s.DB.WithTx(ctx, func(txCtx context.Context) error {
		inv, err := s.InvoiceRepo.GetForUpdate(txCtx, invoiceID)
		if err != nil {
			return err
		}

		if inv.InvoiceStatus != types.InvoiceStatusDraft {
			return ierr.NewError("invoice is not in draft status").
				WithHint("Only draft invoices can have their discount recalculated").
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

		// Tax depends on discount (taxableAmount = Subtotal - TotalDiscount); refresh it via real
		// TaxApplied records rather than TotalTax==0, which a full discount can zero legitimately.
		taxFilter := types.NewNoLimitTaxAppliedFilter()
		taxFilter.EntityType = types.TaxRateEntityTypeInvoice
		taxFilter.EntityID = inv.ID
		taxAppliedCount, err := s.TaxAppliedRepo.Count(txCtx, taxFilter)
		if err != nil {
			return ierr.WithError(err).WithHint("failed to check existing tax applications").Mark(ierr.ErrDatabase)
		}
		if taxAppliedCount > 0 {
			if err := s.applyTaxesToInvoice(txCtx, inv, dto.InvoiceComputeRequest{}); err != nil {
				return err
			}
		}

		inv.Total = decimal.Max(inv.Subtotal.Sub(inv.TotalPrepaidCreditsApplied).Sub(inv.TotalDiscount).Add(inv.TotalTax), decimal.Zero)
		inv.AmountDue = inv.Total
		inv.AmountRemaining = inv.Total.Sub(inv.AmountPaid)

		if err := s.InvoiceRepo.Update(txCtx, inv); err != nil {
			return err
		}

		s.Logger.Info(txCtx, "recalculated discount on invoice",
			"invoice_id", inv.ID,
			"total_discount", inv.TotalDiscount,
			"new_total", inv.Total,
		)

		lockedInv = inv
		return nil
	})
	if err != nil {
		return nil, err
	}

	return dto.NewInvoiceResponse(lockedInv), nil
}

// wipeCouponApplications deletes existing CouponApplication rows and persists a zeroed
// discount reset — ApplyCouponsToInvoice only adds to these fields, never resets them.
func (s *invoiceService) wipeCouponApplications(ctx context.Context, inv *invoice.Invoice) error {
	filter := types.NewNoLimitCouponApplicationFilter()
	filter.InvoiceIDs = []string{inv.ID}
	existing, err := s.CouponApplicationRepo.List(ctx, filter)
	if err != nil {
		return ierr.WithError(err).WithHint("failed to list existing coupon applications").Mark(ierr.ErrDatabase)
	}

	for _, app := range existing {
		if err := s.CouponApplicationRepo.Delete(ctx, app.ID); err != nil {
			return ierr.WithError(err).WithHint("failed to delete existing coupon application").Mark(ierr.ErrDatabase)
		}
	}

	for _, li := range inv.LineItems {
		li.LineItemDiscount = decimal.Zero
		li.InvoiceLevelDiscount = decimal.Zero
		if err := s.InvoiceLineItemRepo.Update(ctx, li); err != nil {
			return ierr.WithError(err).WithHint("failed to reset line item discount").Mark(ierr.ErrDatabase)
		}
	}

	return nil
}

// resolveCurrentInvoiceCoupons resolves coupons currently standing against this invoice's
// subscription, scoped to its billing period and filtered to price IDs on its line items.
func (s *invoiceService) resolveCurrentInvoiceCoupons(ctx context.Context, inv *invoice.Invoice) ([]dto.InvoiceCoupon, []dto.InvoiceLineItemCoupon, error) {
	if inv.SubscriptionID == nil || inv.PeriodStart == nil || inv.PeriodEnd == nil {
		return nil, nil, nil
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

	priceIDs := lo.SliceToMap(
		lo.Filter(inv.LineItems, func(li *invoice.InvoiceLineItem, _ int) bool { return li.PriceID != nil }),
		func(li *invoice.InvoiceLineItem) (string, bool) { return *li.PriceID, true },
	)

	invoiceCoupons := lo.FlatMap(lo.Values(sel.SubLevel), func(assocs []*coupon_association.CouponAssociation, _ int) []dto.InvoiceCoupon {
		return lo.Map(assocs, func(a *coupon_association.CouponAssociation, _ int) dto.InvoiceCoupon {
			return dto.InvoiceCoupon{CouponID: a.CouponID, CouponAssociationID: &a.ID}
		})
	})

	lineItemCoupons := make([]dto.InvoiceLineItemCoupon, 0)
	for sliID, assocs := range sel.LineLevel {
		priceID := sel.SubLineItemIDToPriceID[sliID]
		if priceID == "" || !priceIDs[priceID] {
			continue
		}
		lineItemCoupons = append(lineItemCoupons, lo.Map(assocs, func(a *coupon_association.CouponAssociation, _ int) dto.InvoiceLineItemCoupon {
			return dto.InvoiceLineItemCoupon{LineItemID: priceID, CouponID: a.CouponID, CouponAssociationID: &a.ID}
		})...)
	}

	return invoiceCoupons, lineItemCoupons, nil
}
