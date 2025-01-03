package ent

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/ent/invoicelineitem"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/google/uuid"
)

type invoiceLineItemRepository struct {
	client postgres.IClient
	log    *logger.Logger
}

// NewInvoiceLineItemRepository creates a new invoice line item repository
func NewInvoiceLineItemRepository(client postgres.IClient, log *logger.Logger) invoice.LineItemRepository {
	return &invoiceLineItemRepository{
		client: client,
		log:    log,
	}
}

// Create creates a new invoice line item
func (r *invoiceLineItemRepository) Create(ctx context.Context, item *invoice.InvoiceLineItem) (*invoice.InvoiceLineItem, error) {
	if item.ID == "" {
		item.ID = uuid.New().String()
	}

	client := r.client.Querier(ctx)
	entItem, err := client.InvoiceLineItem.Create().
		SetID(item.ID).
		SetTenantID(item.TenantID).
		SetInvoiceID(item.InvoiceID).
		SetCustomerID(item.CustomerID).
		SetNillableSubscriptionID(item.SubscriptionID).
		SetPriceID(item.PriceID).
		SetNillableMeterID(item.MeterID).
		SetAmount(item.Amount).
		SetQuantity(item.Quantity).
		SetCurrency(item.Currency).
		SetNillablePeriodStart(item.PeriodStart).
		SetNillablePeriodEnd(item.PeriodEnd).
		SetMetadata(item.Metadata).
		SetStatus(string(item.Status)).
		SetCreatedBy(item.CreatedBy).
		SetUpdatedBy(item.UpdatedBy).
		Save(ctx)

	if err != nil {
		return nil, fmt.Errorf("creating invoice line item: %w", err)
	}

	return item.FromEnt(entItem), nil
}

// CreateMany creates multiple invoice line items in a single transaction
func (r *invoiceLineItemRepository) CreateMany(ctx context.Context, items []*invoice.InvoiceLineItem) ([]*invoice.InvoiceLineItem, error) {
	if len(items) == 0 {
		return nil, nil
	}

	now := time.Now().UTC()
	client := r.client.Querier(ctx)
	var entItems []*ent.InvoiceLineItem
	for _, item := range items {
		if item.ID == "" {
			item.ID = uuid.New().String()
		}

		entItem, err := client.InvoiceLineItem.Create().
			SetID(item.ID).
			SetTenantID(item.TenantID).
			SetInvoiceID(item.InvoiceID).
			SetCustomerID(item.CustomerID).
			SetNillableSubscriptionID(item.SubscriptionID).
			SetPriceID(item.PriceID).
			SetNillableMeterID(item.MeterID).
			SetAmount(item.Amount).
			SetQuantity(item.Quantity).
			SetCurrency(item.Currency).
			SetNillablePeriodStart(item.PeriodStart).
			SetNillablePeriodEnd(item.PeriodEnd).
			SetMetadata(item.Metadata).
			SetStatus(string(item.Status)).
			SetCreatedBy(item.CreatedBy).
			SetUpdatedBy(item.UpdatedBy).
			SetCreatedAt(now).
			SetUpdatedAt(now).
			Save(ctx)

		if err != nil {
			return nil, fmt.Errorf("creating invoice line item: %w", err)
		}

		entItems = append(entItems, entItem)
	}

	// Convert ent items back to domain items
	result := make([]*invoice.InvoiceLineItem, len(entItems))
	for i, entItem := range entItems {
		result[i] = items[i].FromEnt(entItem)
	}

	return result, nil
}

// Get retrieves an invoice line item by ID
func (r *invoiceLineItemRepository) Get(ctx context.Context, id string) (*invoice.InvoiceLineItem, error) {
	client := r.client.Querier(ctx)
	entItem, err := client.InvoiceLineItem.Query().
		Where(invoicelineitem.ID(id)).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, invoice.ErrInvoiceLineItemNotFound
		}
		return nil, fmt.Errorf("querying invoice line item: %w", err)
	}

	var item invoice.InvoiceLineItem
	return item.FromEnt(entItem), nil
}

// GetByInvoiceID retrieves all line items for an invoice
func (r *invoiceLineItemRepository) GetByInvoiceID(ctx context.Context, invoiceID string) ([]*invoice.InvoiceLineItem, error) {
	client := r.client.Querier(ctx)
	entItems, err := client.InvoiceLineItem.Query().
		Where(invoicelineitem.InvoiceID(invoiceID)).
		All(ctx)

	if err != nil {
		return nil, fmt.Errorf("querying invoice line items: %w", err)
	}

	items := make([]*invoice.InvoiceLineItem, len(entItems))
	for i, entItem := range entItems {
		var item invoice.InvoiceLineItem
		items[i] = item.FromEnt(entItem)
	}

	return items, nil
}

// Update updates an invoice line item
func (r *invoiceLineItemRepository) Update(ctx context.Context, item *invoice.InvoiceLineItem) (*invoice.InvoiceLineItem, error) {
	client := r.client.Querier(ctx)
	entItem, err := client.InvoiceLineItem.UpdateOneID(item.ID).
		SetAmount(item.Amount).
		SetQuantity(item.Quantity).
		SetNillablePeriodStart(item.PeriodStart).
		SetNillablePeriodEnd(item.PeriodEnd).
		SetMetadata(item.Metadata).
		SetStatus(string(item.Status)).
		SetUpdatedBy(item.UpdatedBy).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, invoice.ErrInvoiceLineItemNotFound
		}
		return nil, fmt.Errorf("updating invoice line item: %w", err)
	}

	return item.FromEnt(entItem), nil
}

// Delete soft deletes an invoice line item
func (r *invoiceLineItemRepository) Delete(ctx context.Context, id string) error {
	client := r.client.Querier(ctx)
	err := client.InvoiceLineItem.UpdateOneID(id).
		SetStatus(string(types.StatusDeleted)).
		Exec(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return invoice.ErrInvoiceLineItemNotFound
		}
		return fmt.Errorf("deleting invoice line item: %w", err)
	}

	return nil
}
