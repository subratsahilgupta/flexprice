package invoice

import (
	"context"

	"github.com/flexprice/flexprice/internal/types"
)

// Repository defines the interface for invoice persistence operations
type Repository interface {
	// Create creates a new invoice
	Create(ctx context.Context, invoice *Invoice) error

	// Get retrieves an invoice by ID
	Get(ctx context.Context, id string) (*Invoice, error)

	// Update updates an existing invoice
	Update(ctx context.Context, invoice *Invoice) error

	// List retrieves invoices based on filter criteria
	List(ctx context.Context, filter *types.InvoiceFilter) ([]*Invoice, error)

	// Count returns the total count of invoices based on filter criteria
	Count(ctx context.Context, filter *types.InvoiceFilter) (int, error)
}
