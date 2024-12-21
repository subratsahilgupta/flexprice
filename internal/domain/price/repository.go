package price

import (
	"context"

	"github.com/flexprice/flexprice/internal/types"
)

type Repository interface {
	Create(ctx context.Context, price *Price) error
	Get(ctx context.Context, id string) (*Price, error)
	GetByPlanID(ctx context.Context, planID string) ([]*Price, error)
	List(ctx context.Context, filter types.Filter) ([]*Price, error)
	Update(ctx context.Context, price *Price) error
	Delete(ctx context.Context, id string) error
}
