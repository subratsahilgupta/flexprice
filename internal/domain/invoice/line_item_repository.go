package invoice

import (
	"context"
)

// LineItemRepository defines the interface for invoice line item persistence operations
type LineItemRepository interface {
	// Create creates a new invoice line item
	Create(ctx context.Context, item *InvoiceLineItem) (*InvoiceLineItem, error)

	// CreateMany creates multiple invoice line items in a single transaction
	CreateMany(ctx context.Context, items []*InvoiceLineItem) ([]*InvoiceLineItem, error)

	// Get retrieves an invoice line item by ID
	Get(ctx context.Context, id string) (*InvoiceLineItem, error)

	// GetByInvoiceID retrieves all line items for an invoice
	GetByInvoiceID(ctx context.Context, invoiceID string) ([]*InvoiceLineItem, error)

	// Update updates an invoice line item
	Update(ctx context.Context, item *InvoiceLineItem) (*InvoiceLineItem, error)

	// Delete soft deletes an invoice line item
	Delete(ctx context.Context, id string) error
}
