package ent

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/ent/invoice"
	domainInvoice "github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
)

type invoiceRepository struct {
	client *postgres.Client
	logger *logger.Logger
}

// NewInvoiceRepository creates a new invoice repository
func NewInvoiceRepository(client *postgres.Client, logger *logger.Logger) domainInvoice.Repository {
	return &invoiceRepository{
		client: client,
		logger: logger,
	}
}

func (r *invoiceRepository) Create(ctx context.Context, inv *domainInvoice.Invoice) error {
	client := r.client.Querier(ctx)
	invoice, err := client.Invoice.Create().
		SetID(inv.ID).
		SetCustomerID(inv.CustomerID).
		SetSubscriptionID(*inv.SubscriptionID).
		SetWalletID(*inv.WalletID).
		SetStatus(string(inv.InvoiceStatus)).
		SetAmountDue(inv.AmountDue).
		SetAmountPaid(inv.AmountPaid).
		SetAmountRemaining(inv.AmountRemaining).
		SetDescription(inv.Description).
		SetNillableDueDate(inv.DueDate).
		SetNillablePaidAt(inv.PaidAt).
		SetNillableVoidedAt(inv.VoidedAt).
		SetNillableFinalizedAt(inv.FinalizedAt).
		SetNillablePaymentIntentID(inv.PaymentIntentID).
		SetNillableInvoicePdfURL(inv.InvoicePdfUrl).
		SetAttemptCount(inv.AttemptCount).
		SetBillingReason(inv.BillingReason).
		SetMetadata(inv.Metadata).
		SetVersion(inv.Version).
		SetCreatedAt(inv.CreatedAt).
		SetUpdatedAt(inv.UpdatedAt).
		SetCreatedBy(inv.CreatedBy).
		SetUpdatedBy(inv.UpdatedBy).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to create invoice: %w", err)
	}
	// Update the input invoice with created data
	*inv = *domainInvoice.FromEnt(invoice)
	return nil
}

func (r *invoiceRepository) Get(ctx context.Context, id string) (*domainInvoice.Invoice, error) {
	client := r.client.Querier(ctx)
	inv, err := client.Invoice.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, domainInvoice.ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("failed to get invoice: %w", err)
	}
	return domainInvoice.FromEnt(inv), nil
}

func (r *invoiceRepository) Update(ctx context.Context, inv *domainInvoice.Invoice) error {
	client := r.client.Querier(ctx)
	_, err := client.Invoice.UpdateOneID(inv.ID).
		SetInvoiceStatus(string(inv.InvoiceStatus)).
		SetAmountDue(inv.AmountDue).
		SetAmountPaid(inv.AmountPaid).
		SetAmountRemaining(inv.AmountRemaining).
		SetDescription(inv.Description).
		SetNillableDueDate(inv.DueDate).
		SetNillablePaidAt(inv.PaidAt).
		SetNillableVoidedAt(inv.VoidedAt).
		SetNillableFinalizedAt(inv.FinalizedAt).
		SetNillablePaymentIntentID(inv.PaymentIntentID).
		SetNillableInvoicePdfURL(inv.InvoicePdfUrl).
		SetAttemptCount(inv.AttemptCount).
		SetBillingReason(inv.BillingReason).
		SetMetadata(inv.Metadata).
		SetVersion(inv.Version).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return domainInvoice.ErrInvoiceNotFound
		}
		return fmt.Errorf("failed to update invoice: %w", err)
	}
	return nil
}

func (r *invoiceRepository) List(ctx context.Context, filter *types.InvoiceFilter) ([]*domainInvoice.Invoice, error) {
	client := r.client.Querier(ctx)
	query := client.Invoice.Query()

	if filter != nil {
		if filter.CustomerID != "" {
			query = query.Where(invoice.CustomerIDEQ(filter.CustomerID))
		}
		if filter.SubscriptionID != "" {
			query = query.Where(invoice.SubscriptionIDEQ(filter.SubscriptionID))
		}
		if filter.WalletID != "" {
			query = query.Where(invoice.WalletIDEQ(filter.WalletID))
		}
		if len(filter.Status) > 0 {
			statuses := make([]string, len(filter.Status))
			for i, s := range filter.Status {
				statuses[i] = string(s)
			}
			query = query.Where(invoice.StatusIn(statuses...))
		}
		if filter.StartTime != nil {
			query = query.Where(invoice.CreatedAtGTE(*filter.StartTime))
		}
		if filter.EndTime != nil {
			query = query.Where(invoice.CreatedAtLTE(*filter.EndTime))
		}
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
	}

	// Always order by created_at desc to get latest invoices first
	query = query.Order(ent.Desc(invoice.FieldCreatedAt))

	invoices, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list invoices: %w", err)
	}

	result := make([]*domainInvoice.Invoice, len(invoices))
	for i, inv := range invoices {
		result[i] = domainInvoice.FromEnt(inv)
	}
	return result, nil
}

func (r *invoiceRepository) Count(ctx context.Context, filter *types.InvoiceFilter) (int, error) {
	client := r.client.Querier(ctx)
	query := client.Invoice.Query()

	if filter != nil {
		if filter.CustomerID != "" {
			query = query.Where(invoice.CustomerIDEQ(filter.CustomerID))
		}
		if filter.SubscriptionID != "" {
			query = query.Where(invoice.SubscriptionIDEQ(filter.SubscriptionID))
		}
		if filter.WalletID != "" {
			query = query.Where(invoice.WalletIDEQ(filter.WalletID))
		}
		if len(filter.Status) > 0 {
			statuses := make([]string, len(filter.Status))
			for i, s := range filter.Status {
				statuses[i] = string(s)
			}
			query = query.Where(invoice.StatusIn(statuses...))
		}
		if filter.StartTime != nil {
			query = query.Where(invoice.CreatedAtGTE(*filter.StartTime))
		}
		if filter.EndTime != nil {
			query = query.Where(invoice.CreatedAtLTE(*filter.EndTime))
		}
	}

	count, err := query.Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count invoices: %w", err)
	}
	return count, nil
}

func (r *invoiceRepository) GetByPaymentIntentID(ctx context.Context, paymentIntentID string) (*domainInvoice.Invoice, error) {
	client := r.client.Querier(ctx)
	inv, err := client.Invoice.Query().
		Where(invoice.PaymentIntentIDEQ(paymentIntentID)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, domainInvoice.ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("failed to get invoice by payment intent ID: %w", err)
	}
	return domainInvoice.FromEnt(inv), nil
}

func (r *invoiceRepository) GetPendingInvoices(ctx context.Context) ([]*domainInvoice.Invoice, error) {
	// Get all finalized invoices that are not paid and not voided
	client := r.client.Querier(ctx)
	invoices, err := client.Invoice.Query().
		Where(
			invoice.StatusEQ(string(types.InvoiceStatusFinalized)),
			invoice.PaidAtIsNil(),
			invoice.VoidedAtIsNil(),
		).
		Order(ent.Asc(invoice.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending invoices: %w", err)
	}

	result := make([]*domainInvoice.Invoice, len(invoices))
	for i, inv := range invoices {
		result[i] = domainInvoice.FromEnt(inv)
	}
	return result, nil
}
