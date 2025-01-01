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
	client postgres.IClient
	logger *logger.Logger
}

// NewInvoiceRepository creates a new invoice repository
func NewInvoiceRepository(client postgres.IClient, logger *logger.Logger) domainInvoice.Repository {
	return &invoiceRepository{
		client: client,
		logger: logger,
	}
}

func (r *invoiceRepository) Create(ctx context.Context, inv *domainInvoice.Invoice) error {
	client := r.client.Querier(ctx)
	invoice, err := client.Invoice.Create().
		SetID(inv.ID).
		SetTenantID(inv.TenantID).
		SetCustomerID(inv.CustomerID).
		SetNillableSubscriptionID(inv.SubscriptionID).
		SetInvoiceType(string(inv.InvoiceType)).
		SetInvoiceStatus(string(inv.InvoiceStatus)).
		SetPaymentStatus(string(inv.PaymentStatus)).
		SetCurrency(inv.Currency).
		SetAmountDue(inv.AmountDue).
		SetAmountPaid(inv.AmountPaid).
		SetAmountRemaining(inv.AmountRemaining).
		SetDescription(inv.Description).
		SetNillableDueDate(inv.DueDate).
		SetNillablePaidAt(inv.PaidAt).
		SetNillableVoidedAt(inv.VoidedAt).
		SetNillableFinalizedAt(inv.FinalizedAt).
		SetNillableInvoicePdfURL(inv.InvoicePDFURL).
		SetBillingReason(inv.BillingReason).
		SetMetadata(inv.Metadata).
		SetVersion(inv.Version).
		SetStatus(string(inv.Status)).
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

	// Use predicate-based update for optimistic locking
	query := client.Invoice.Update().
		Where(
			invoice.ID(inv.ID),
			invoice.TenantID(types.GetTenantID(ctx)),
			invoice.Status(string(types.StatusPublished)),
			invoice.Version(inv.Version), // Version check for optimistic locking
		)

	// Set all fields
	query.
		SetInvoiceStatus(string(inv.InvoiceStatus)).
		SetPaymentStatus(string(inv.PaymentStatus)).
		SetAmountDue(inv.AmountDue).
		SetAmountPaid(inv.AmountPaid).
		SetAmountRemaining(inv.AmountRemaining).
		SetDescription(inv.Description).
		SetNillableDueDate(inv.DueDate).
		SetNillablePaidAt(inv.PaidAt).
		SetNillableVoidedAt(inv.VoidedAt).
		SetNillableFinalizedAt(inv.FinalizedAt).
		SetNillableInvoicePdfURL(inv.InvoicePDFURL).
		SetBillingReason(string(inv.BillingReason)).
		SetMetadata(inv.Metadata).
		SetUpdatedAt(inv.UpdatedAt).
		SetUpdatedBy(inv.UpdatedBy).
		AddVersion(1) // Increment version atomically

	// Execute update
	n, err := query.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update invoice: %w", err)
	}
	if n == 0 {
		// No rows were updated - either record doesn't exist or version mismatch
		exists, err := client.Invoice.Query().
			Where(
				invoice.ID(inv.ID),
				invoice.TenantID(types.GetTenantID(ctx)),
			).
			Exist(ctx)
		if err != nil {
			return fmt.Errorf("failed to check invoice existence: %w", err)
		}
		if !exists {
			return domainInvoice.ErrInvoiceNotFound
		}
		// Record exists but version mismatch
		return domainInvoice.NewVersionConflictError(inv.ID, inv.Version, inv.Version+1)
	}

	return nil
}

func (r *invoiceRepository) List(ctx context.Context, filter *types.InvoiceFilter) ([]*domainInvoice.Invoice, error) {
	client := r.client.Querier(ctx)
	query := client.Invoice.Query()

	if filter != nil {
		query = ToEntQuery(ctx, filter, query)
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
	}

	// Always order by created_at desc for consistency
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
		query = ToEntQuery(ctx, filter, query)
	}

	count, err := query.Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count invoices: %w", err)
	}
	return count, nil
}

// helper functions

// Add a helper function to parse the InvoiceFilter struct to relevant ent base *ent.InvoiceQuery
func ToEntQuery(ctx context.Context, f *types.InvoiceFilter, query *ent.InvoiceQuery) *ent.InvoiceQuery {
	query.Where(
		invoice.TenantID(types.GetTenantID(ctx)),
		invoice.Status(string(types.StatusPublished)),
	)
	if f.CustomerID != "" {
		query = query.Where(invoice.CustomerID(f.CustomerID))
	}
	if f.SubscriptionID != "" {
		query = query.Where(invoice.SubscriptionID(f.SubscriptionID))
	}
	if f.InvoiceType != "" {
		query = query.Where(invoice.InvoiceType(string(f.InvoiceType)))
	}
	if len(f.InvoiceStatus) > 0 {
		invoiceStatuses := make([]string, len(f.InvoiceStatus))
		for i, status := range f.InvoiceStatus {
			invoiceStatuses[i] = string(status)
		}
		query = query.Where(invoice.InvoiceStatusIn(invoiceStatuses...))
	}
	if len(f.PaymentStatus) > 0 {
		paymentStatuses := make([]string, len(f.PaymentStatus))
		for i, status := range f.PaymentStatus {
			paymentStatuses[i] = string(status)
		}
		query = query.Where(invoice.PaymentStatusIn(paymentStatuses...))
	}
	if f.StartTime != nil {
		query = query.Where(invoice.CreatedAtGTE(*f.StartTime))
	}
	if f.EndTime != nil {
		query = query.Where(invoice.CreatedAtLTE(*f.EndTime))
	}
	return query
}
