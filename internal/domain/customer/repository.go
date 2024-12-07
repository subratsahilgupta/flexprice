package customer

import (
	"context"

	"github.com/flexprice/flexprice/internal/types"
)

type Repository interface {
	Create(ctx context.Context, customer *Customer) error
	Get(ctx context.Context, id string) (*Customer, error)
	List(ctx context.Context, filter types.Filter) ([]*Customer, error)
	Update(ctx context.Context, customer *Customer) error
	Delete(ctx context.Context, id string) error
}
