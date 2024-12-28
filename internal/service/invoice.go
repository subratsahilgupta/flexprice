package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/publisher"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

type InvoiceService interface {
	CreateInvoice(ctx context.Context, req dto.CreateInvoiceRequest) (*dto.InvoiceResponse, error)
	GetInvoice(ctx context.Context, id string) (*dto.InvoiceResponse, error)
	ListInvoices(ctx context.Context, filter *types.InvoiceFilter) (*dto.ListInvoicesResponse, error)
	FinalizeInvoice(ctx context.Context, id string) error
	VoidInvoice(ctx context.Context, id string) error
	UpdatePaymentStatus(ctx context.Context, id string, status types.InvoicePaymentStatus, amount *decimal.Decimal) error
}

type invoiceService struct {
	invoiceRepo invoice.Repository
	publisher   publisher.EventPublisher
	logger      *logger.Logger
}

func NewInvoiceService(
	invoiceRepo invoice.Repository,
	publisher publisher.EventPublisher,
	logger *logger.Logger,
) InvoiceService {
	return &invoiceService{
		invoiceRepo: invoiceRepo,
		publisher:   publisher,
		logger:      logger,
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
	inv.Version++

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

	if inv.InvoiceStatus == types.InvoiceStatusVoided {
		return fmt.Errorf("invoice cannot be voided")
	}

	now := time.Now().UTC()
	inv.InvoiceStatus = types.InvoiceStatusVoided
	inv.VoidedAt = &now
	inv.Version++

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

	// Validate the payment status transition
	if err := s.validatePaymentStatusTransition(inv.PaymentStatus, status); err != nil {
		return fmt.Errorf("invalid payment status transition: %w", err)
	}

	now := time.Now().UTC()
	inv.PaymentStatus = status
	inv.Version++

	switch status {
	case types.InvoicePaymentStatusSucceeded:
		if amount != nil {
			inv.AmountPaid = *amount
			inv.AmountRemaining = inv.AmountDue.Sub(inv.AmountPaid)
		} else {
			inv.AmountPaid = inv.AmountDue
			inv.AmountRemaining = decimal.Zero
		}
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

func (s *invoiceService) validatePaymentStatusTransition(from, to types.InvoicePaymentStatus) error {
	// Define valid transitions
	validTransitions := map[types.InvoicePaymentStatus][]types.InvoicePaymentStatus{
		types.InvoicePaymentStatusPending: {
			types.InvoicePaymentStatusPending,
			types.InvoicePaymentStatusSucceeded,
			types.InvoicePaymentStatusFailed,
		},
		types.InvoicePaymentStatusFailed: {
			types.InvoicePaymentStatusPending,
		},
	}

	// Check if transition is valid
	validNextStates, exists := validTransitions[from]
	if !exists {
		return fmt.Errorf("invalid current payment status: %s", from)
	}

	for _, validState := range validNextStates {
		if validState == to {
			return nil
		}
	}

	return fmt.Errorf("invalid payment status transition from %s to %s", from, to)
}
