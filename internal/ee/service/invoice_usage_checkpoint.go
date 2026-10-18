package service

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

// saveCompletedCycleUsageCheckpoints is best-effort: a failed save leaves the preceding checkpoint usable.
func (s *invoiceService) saveCompletedCycleUsageCheckpoints(ctx context.Context, inv *invoice.Invoice) {
	if s.MeterUsageCheckpointRepo == nil || inv == nil || inv.InvoiceType != types.InvoiceTypeSubscription ||
		inv.SubscriptionID == nil || inv.PeriodStart == nil || inv.PeriodEnd == nil ||
		inv.PeriodEnd.After(time.Now().UTC()) || types.InvoiceBillingReason(inv.BillingReason) != types.InvoiceBillingReasonSubscriptionCycle {
		return
	}
	if err := s.saveCycleUsageCheckpoints(ctx, inv); err != nil {
		s.Logger.Error(ctx, "saving usage checkpoints failed; reads use the preceding checkpoint",
			"subscription_id", *inv.SubscriptionID, "invoice_id", inv.ID, "error", err)
	}
}

func (s *invoiceService) saveCycleUsageCheckpoints(ctx context.Context, inv *invoice.Invoice) error {
	sub, err := s.SubRepo.Get(ctx, *inv.SubscriptionID)
	if err != nil {
		return err
	}
	usageService := NewMeterUsageService(s.ServiceParams).(*meterUsageService)
	items, err := usageService.listLineItemsForUsageWindow(ctx, sub.ID, *inv.PeriodStart, false)
	if err != nil {
		return err
	}
	snapshot := *sub
	snapshot.LineItems = items
	return usageService.SaveUsageCheckpointsForSubscription(ctx, &snapshot, *inv.PeriodEnd, lo.ToPtr(inv.ID))
}
