package refund

import (
	"context"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// Repository defines the persistence interface for Refund entities.
type Repository interface {
	Create(ctx context.Context, refund *Refund) error
	CreateBulk(ctx context.Context, refunds []*Refund) error
	Get(ctx context.Context, id string) (*Refund, error)

	// GetForUpdate row-locks the refund. Must be called inside a transaction.
	GetForUpdate(ctx context.Context, id string) (*Refund, error)

	Update(ctx context.Context, refund *Refund) error

	// Delete soft-deletes a refund (sets status to archived).
	Delete(ctx context.Context, id string) error

	List(ctx context.Context, filter *types.RefundFilter) ([]*Refund, error)
	Count(ctx context.Context, filter *types.RefundFilter) (int, error)

	// GetByIdempotencyKey looks up a refund by (tenant_id, environment_id, idempotency_key).
	GetByIdempotencyKey(ctx context.Context, key string) (*Refund, error)

	GetByGatewayRefundID(ctx context.Context, gateway, gatewayRefundID string) (*Refund, error)

	ListByInvoice(ctx context.Context, invoiceID string) ([]*Refund, error)

	// Ids with no settled refunds are absent from the returned map.
	SumSettledByPaymentIDs(ctx context.Context, paymentIDs []string) (map[string]decimal.Decimal, error)
	SumSettledByInvoiceIDs(ctx context.Context, invoiceIDs []string) (map[string]decimal.Decimal, error)

	// SumInFlightByPaymentIDs sums the requested amount of refunds that have claimed money
	// but not yet settled, so a second allocation cannot draw the same balance again.
	SumInFlightByPaymentIDs(ctx context.Context, paymentIDs []string) (map[string]decimal.Decimal, error)
}
