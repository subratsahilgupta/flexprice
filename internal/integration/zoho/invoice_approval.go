package zoho

import (
	"context"
	"time"
)

const (
	approvalWait     = 5 * time.Second
	approvalMaxPolls = 3
)

// awaitingApproval reports whether the invoice is still moving through the approval flow —
// either not yet submitted, or submitted and queued for an approver.
func awaitingApproval(status string) bool {
	switch NormalizeInvoiceStatus(status) {
	case InvoiceStatusDraft, InvoiceStatusPendingApproval:
		return true
	default:
		return false
	}
}

// ensureApprovedForPayment takes a synced Zoho invoice out of draft so a payment can be
// recorded against it, and reports whether the invoice ended up payable.
func (s *InvoiceService) ensureApprovedForPayment(ctx context.Context, flexpriceInvoiceID string, zohoInv *InvoiceResponse) (bool, error) {
	return s.ensureApprovedForPaymentWithin(ctx, flexpriceInvoiceID, zohoInv, approvalWait, approvalMaxPolls)
}

func (s *InvoiceService) ensureApprovedForPaymentWithin(
	ctx context.Context,
	flexpriceInvoiceID string,
	zohoInv *InvoiceResponse,
	wait time.Duration,
	maxPolls int,
) (bool, error) {
	if zohoInv == nil {
		return false, nil
	}

	settings, err := s.getInvoiceSyncSettings(ctx)
	if err != nil {
		return false, err
	}
	if !settings.IsSubmitForApprovalEnabled() {
		return true, nil
	}

	switch NormalizeInvoiceStatus(zohoInv.Status) {
	case InvoiceStatusDraft:
		if err := s.client.SubmitInvoiceForApproval(ctx, zohoInv.InvoiceID); err != nil {
			return false, err
		}
		s.logger.Info(ctx, "submitted Zoho invoice for approval",
			"invoice_id", flexpriceInvoiceID,
			"zoho_invoice_id", zohoInv.InvoiceID)
	case InvoiceStatusPendingApproval:
		// Already queued; re-submitting would make Zoho error. Fall through and wait.
	case InvoiceStatusRejected:
		return false, s.logRejected(ctx, flexpriceInvoiceID, zohoInv.InvoiceID)
	default:
		// Approved, sent, partially paid, paid: nothing blocks a payment.
		return true, nil
	}

	status := zohoInv.Status
	for i := 0; i < maxPolls; i++ {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(wait):
		}

		current, err := s.client.GetInvoice(ctx, zohoInv.InvoiceID)
		if err != nil {
			return false, err
		}
		if current == nil {
			continue
		}

		status = current.Status
		if NormalizeInvoiceStatus(status) == InvoiceStatusRejected {
			return false, s.logRejected(ctx, flexpriceInvoiceID, zohoInv.InvoiceID)
		}
		if !awaitingApproval(status) {
			return true, nil
		}
	}

	s.logger.Info(ctx, "Zoho invoice still awaiting approval after wait, skipping mark-paid",
		"invoice_id", flexpriceInvoiceID,
		"zoho_invoice_id", zohoInv.InvoiceID,
		"zoho_status", status,
		"polls", maxPolls,
		"wait_per_poll", wait.String())
	return false, nil
}

// logRejected records the terminal case where an approver turned the invoice down. It
// returns a nil error so the caller skips mark-paid without failing the sync.
func (s *InvoiceService) logRejected(ctx context.Context, flexpriceInvoiceID, zohoInvoiceID string) error {
	s.logger.Info(ctx, "Zoho invoice was rejected in approval, skipping mark-paid",
		"invoice_id", flexpriceInvoiceID,
		"zoho_invoice_id", zohoInvoiceID)
	return nil
}
