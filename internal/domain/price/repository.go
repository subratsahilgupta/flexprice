package price

import (
	"context"

	"github.com/flexprice/flexprice/internal/types"
)

type Repository interface {
	CreatePrice(ctx context.Context, price *Price) error
	GetPrice(ctx context.Context, id string) (*Price, error)
	GetPrices(ctx context.Context, filter types.Filter) ([]*Price, error)
	UpdatePrice(ctx context.Context, price *Price) error
	UpdatePriceStatus(ctx context.Context, id string, status types.Status) error
}
