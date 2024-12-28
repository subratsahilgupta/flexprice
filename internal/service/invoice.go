package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/publisher"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type InvoiceService interface {
	CreateInvoice(ctx context.Context, req dto.CreateInvoiceRequest) (*dto.InvoiceResponse, error)
	GetInvoice(ctx context.Context, id string) (*dto.InvoiceResponse, error)
	ListInvoices(ctx context.Context, filter *types.InvoiceFilter) (*dto.ListInvoicesResponse, error)
	FinalizeInvoice(ctx context.Context, id string) error
	VoidInvoice(ctx context.Context, id string) error
	MarkInvoiceAsPaid(ctx context.Context, id string, paymentIntentID string) error
	CreateSubscriptionInvoice(ctx context.Context, subscriptionID string, amount decimal.Decimal, billingReason types.InvoiceBillingReason) (*dto.InvoiceResponse, error)
}

type invoiceService struct {
	invoiceRepo      invoice.Repository
	subscriptionRepo subscription.Repository
	publisher        publisher.EventPublisher
	logger           *logger.Logger
}

func NewInvoiceService(
	invoiceRepo invoice.Repository,
	subscriptionRepo subscription.Repository,
	publisher publisher.EventPublisher,
	logger *logger.Logger,
) InvoiceService {
	return &invoiceService{
		invoiceRepo:      invoiceRepo,
		subscriptionRepo: subscriptionRepo,
		publisher:        publisher,
		logger:           logger,
	}
}

func (s *invoiceService) CreateInvoice(ctx context.Context, req dto.CreateInvoiceRequest) (*dto.InvoiceResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	inv := &invoice.Invoice{
		ID:              uuid.New().String(),
		TenantID:        req.TenantID,
		CustomerID:      req.CustomerID,
		SubscriptionID:  req.SubscriptionID,
		WalletID:        req.WalletID,
		InvoiceStatus:   types.InvoiceStatusDraft,
		Currency:        req.Currency,
		AmountDue:       req.AmountDue,
		AmountPaid:      decimal.Zero,
		AmountRemaining: req.AmountDue,
		Description:     req.Description,
		DueDate:         req.DueDate,
		BillingReason:   string(req.BillingReason),
		Metadata:        req.Metadata,
		Version:         1,
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

	if inv.InvoiceStatus == types.InvoiceStatusPaid || inv.InvoiceStatus == types.InvoiceStatusVoided {
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

func (s *invoiceService) MarkInvoiceAsPaid(ctx context.Context, id string, paymentIntentID string) error {
	inv, err := s.invoiceRepo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get invoice: %w", err)
	}

	if inv.InvoiceStatus != types.InvoiceStatusFinalized {
		return fmt.Errorf("invoice is not in finalized status")
	}

	now := time.Now().UTC()
	inv.InvoiceStatus = types.InvoiceStatusPaid
	inv.PaidAt = &now
	inv.PaymentIntentID = &paymentIntentID
	inv.AmountPaid = inv.AmountDue
	inv.AmountRemaining = decimal.Zero
	inv.Version++

	if err := s.invoiceRepo.Update(ctx, inv); err != nil {
		return fmt.Errorf("failed to update invoice: %w", err)
	}

	// TODO: add publisher event for invoice paid

	return nil
}

func (s *invoiceService) CreateSubscriptionInvoice(ctx context.Context, subscriptionID string, amount decimal.Decimal, billingReason types.InvoiceBillingReason) (*dto.InvoiceResponse, error) {
	sub, err := s.subscriptionRepo.Get(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	description := fmt.Sprintf("Subscription charges for %s", sub.ID)
	if billingReason == types.InvoiceBillingReasonSubscriptionCreate {
		description = fmt.Sprintf("Initial subscription charges for %s", sub.ID)
	}

	req := dto.CreateInvoiceRequest{
		TenantID:       sub.TenantID,
		CustomerID:     sub.CustomerID,
		SubscriptionID: &sub.ID,
		Currency:       sub.Currency,
		AmountDue:      amount,
		Description:    description,
		BillingReason:  billingReason,
		DueDate:        &sub.CurrentPeriodEnd,
	}

	return s.CreateInvoice(ctx, req)
}
