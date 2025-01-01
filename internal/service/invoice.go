package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/publisher"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

type InvoiceService interface {
	CreateInvoice(ctx context.Context, req dto.CreateInvoiceRequest) (*dto.InvoiceResponse, error)
	GetInvoice(ctx context.Context, id string) (*dto.InvoiceResponse, error)
	ListInvoices(ctx context.Context, filter *types.InvoiceFilter) (*dto.ListInvoicesResponse, error)
	FinalizeInvoice(ctx context.Context, id string) error
	VoidInvoice(ctx context.Context, id string) error
	UpdatePaymentStatus(ctx context.Context, id string, status types.InvoicePaymentStatus, amount *decimal.Decimal) error
	CreateSubscriptionInvoice(ctx context.Context, sub *subscription.Subscription, periodStart, periodEnd time.Time, usage *dto.GetUsageBySubscriptionResponse) (*dto.InvoiceResponse, error)
}

type invoiceService struct {
	invoiceRepo invoice.Repository
	publisher   publisher.EventPublisher
	logger      *logger.Logger
	db          postgres.IClient
}

func NewInvoiceService(
	invoiceRepo invoice.Repository,
	publisher publisher.EventPublisher,
	logger *logger.Logger,
	db postgres.IClient,
) InvoiceService {
	return &invoiceService{
		invoiceRepo: invoiceRepo,
		publisher:   publisher,
		logger:      logger,
		db:          db,
	}
}

func (s *invoiceService) CreateInvoice(ctx context.Context, req dto.CreateInvoiceRequest) (*dto.InvoiceResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	inv, err := req.ToInvoice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create invoice: %w", err)
	}

	// Setting default values
	if req.InvoiceType == types.InvoiceTypeOneOff {
		if req.InvoiceStatus == nil {
			inv.InvoiceStatus = types.InvoiceStatusFinalized
		}
		if req.PaymentStatus == nil {
			inv.PaymentStatus = types.InvoicePaymentStatusSucceeded
		}
	} else if req.InvoiceType == types.InvoiceTypeSubscription {
		if req.InvoiceStatus == nil {
			inv.InvoiceStatus = types.InvoiceStatusDraft
		}
		if req.PaymentStatus == nil {
			inv.PaymentStatus = types.InvoicePaymentStatusPending
		}
	}

	if req.AmountPaid == nil {
		if req.PaymentStatus == nil {
			inv.AmountPaid = inv.AmountDue
		}
	}

	// Calculated Amount Remaining
	inv.AmountRemaining = inv.AmountDue.Sub(inv.AmountPaid)

	// Validate invoice
	if err := inv.Validate(); err != nil {
		return nil, err
	}

	if err := s.invoiceRepo.Create(ctx, inv); err != nil {
		return nil, fmt.Errorf("failed to create invoice: %w", err)
	}

	// TODO: add publisher event for invoice created

	return dto.NewInvoiceResponse(inv), nil
}

func (s *invoiceService) GetInvoice(ctx context.Context, id string) (*dto.InvoiceResponse, error) {
	inv, err := s.invoiceRepo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get invoice: %w", err)
	}
	return dto.NewInvoiceResponse(inv), nil
}

func (s *invoiceService) ListInvoices(ctx context.Context, filter *types.InvoiceFilter) (*dto.ListInvoicesResponse, error) {
	invoices, err := s.invoiceRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list invoices: %w", err)
	}

	count, err := s.invoiceRepo.Count(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to count invoices: %w", err)
	}

	items := make([]*dto.InvoiceResponse, len(invoices))
	for i, inv := range invoices {
		items[i] = dto.NewInvoiceResponse(inv)
	}

	return &dto.ListInvoicesResponse{
		Items: items,
		Total: count,
	}, nil
}

func (s *invoiceService) FinalizeInvoice(ctx context.Context, id string) error {
	inv, err := s.invoiceRepo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get invoice: %w", err)
	}

	if inv.InvoiceStatus != types.InvoiceStatusDraft {
		return fmt.Errorf("invoice is not in draft status")
	}

	now := time.Now().UTC()
	inv.InvoiceStatus = types.InvoiceStatusFinalized
	inv.FinalizedAt = &now

	if err := s.invoiceRepo.Update(ctx, inv); err != nil {
		return fmt.Errorf("failed to update invoice: %w", err)
	}

	// TODO: add publisher event for invoice finalized

	return nil
}

func (s *invoiceService) VoidInvoice(ctx context.Context, id string) error {
	inv, err := s.invoiceRepo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get invoice: %w", err)
	}

	allowedInvoiceStatuses := []types.InvoiceStatus{
		types.InvoiceStatusDraft,
		types.InvoiceStatusFinalized,
	}
	if !lo.Contains(allowedInvoiceStatuses, inv.InvoiceStatus) {
		return fmt.Errorf("invoice status - %s is not allowed", inv.InvoiceStatus)
	}

	now := time.Now().UTC()
	inv.InvoiceStatus = types.InvoiceStatusVoided
	inv.VoidedAt = &now

	if err := s.invoiceRepo.Update(ctx, inv); err != nil {
		return fmt.Errorf("failed to update invoice: %w", err)
	}

	// TODO: add publisher event for invoice voided

	return nil
}

func (s *invoiceService) UpdatePaymentStatus(ctx context.Context, id string, status types.InvoicePaymentStatus, amount *decimal.Decimal) error {
	inv, err := s.invoiceRepo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get invoice: %w", err)
	}

	// Validate the invoice status
	allowedInvoiceStatuses := []types.InvoiceStatus{
		types.InvoiceStatusDraft,
		types.InvoiceStatusFinalized,
	}
	if !lo.Contains(allowedInvoiceStatuses, inv.InvoiceStatus) {
		return fmt.Errorf("invoice status - %s is not allowed", inv.InvoiceStatus)
	}

	// Validate the payment status transition
	if err := s.validatePaymentStatusTransition(inv.PaymentStatus, status); err != nil {
		return fmt.Errorf("invalid payment status transition: %w", err)
	}

	// Validate the request amount
	if amount != nil && amount.IsNegative() {
		return fmt.Errorf("amount must be non-negative")
	}

	now := time.Now().UTC()
	inv.PaymentStatus = status

	switch status {
	case types.InvoicePaymentStatusPending:
		if amount != nil {
			inv.AmountPaid = *amount
			inv.AmountRemaining = inv.AmountDue.Sub(*amount)
		}
	case types.InvoicePaymentStatusSucceeded:
		inv.AmountPaid = inv.AmountDue
		inv.AmountRemaining = decimal.Zero
		inv.PaidAt = &now
	case types.InvoicePaymentStatusFailed:
		inv.AmountPaid = decimal.Zero
		inv.AmountRemaining = inv.AmountDue
		inv.PaidAt = nil
	}

	// Validate the final state
	if err := inv.Validate(); err != nil {
		return fmt.Errorf("invalid invoice state: %w", err)
	}

	if err := s.invoiceRepo.Update(ctx, inv); err != nil {
		return fmt.Errorf("failed to update invoice: %w", err)
	}

	return nil
}

func (s *invoiceService) CreateSubscriptionInvoice(ctx context.Context, sub *subscription.Subscription, periodStart, periodEnd time.Time, usage *dto.GetUsageBySubscriptionResponse) (*dto.InvoiceResponse, error) {
	s.logger.Infow("creating subscription invoice",
		"subscription_id", sub.ID,
		"period_start", periodStart,
		"period_end", periodEnd)

	if usage == nil {
		return nil, fmt.Errorf("usage is required")
	}

	// Calculate total amount from usage
	amountDue := decimal.NewFromFloat(usage.Amount)
	invoiceDueDate := periodEnd.Add(24 * time.Hour * types.InvoiceDefaultDueDays)

	// Create invoice using CreateInvoice
	req := dto.CreateInvoiceRequest{
		CustomerID:     sub.CustomerID,
		SubscriptionID: lo.ToPtr(sub.ID),
		InvoiceType:    types.InvoiceTypeSubscription,
		InvoiceStatus:  lo.ToPtr(types.InvoiceStatusDraft),
		PaymentStatus:  lo.ToPtr(types.InvoicePaymentStatusPending),
		Currency:       sub.Currency,
		AmountDue:      amountDue,
		Description:    fmt.Sprintf("Subscription charges for period %s to %s", periodStart.Format("2006-01-02"), periodEnd.Format("2006-01-02")),
		DueDate:        lo.ToPtr(invoiceDueDate),
		BillingReason:  types.InvoiceBillingReasonSubscriptionCycle,
		Metadata: map[string]interface{}{
			"period_start": types.FormatTime(periodStart),
			"period_end":   types.FormatTime(periodEnd),
			"usage":        usage,
		},
	}

	return s.CreateInvoice(ctx, req)
}

func (s *invoiceService) validatePaymentStatusTransition(from, to types.InvoicePaymentStatus) error {
	// Define allowed transitions
	allowedTransitions := map[types.InvoicePaymentStatus][]types.InvoicePaymentStatus{
		types.InvoicePaymentStatusPending: {
			types.InvoicePaymentStatusPending,
			types.InvoicePaymentStatusSucceeded,
			types.InvoicePaymentStatusFailed,
		},
		types.InvoicePaymentStatusFailed: {
			types.InvoicePaymentStatusPending,
		},
	}

	allowed, ok := allowedTransitions[from]
	if !ok {
		return fmt.Errorf("invalid current payment status: %s", from)
	}

	for _, status := range allowed {
		if status == to {
			return nil
		}
	}

	return fmt.Errorf("invalid payment status transition from %s to %s", from, to)
}
