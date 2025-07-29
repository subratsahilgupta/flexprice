package redemption

import (
	"context"
)

// Repository defines the interface for redemption data access
type Repository interface {
	Create(ctx context.Context, redemption *Redemption) error
	Get(ctx context.Context, id string) (*Redemption, error)
	Update(ctx context.Context, redemption *Redemption) error
	Delete(ctx context.Context, id string) error
	GetByInvoice(ctx context.Context, invoiceID string) ([]*Redemption, error)
	GetByInvoiceLineItem(ctx context.Context, invoiceLineItemID string) ([]*Redemption, error)
}
