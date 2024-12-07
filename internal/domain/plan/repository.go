package plan

import (
	"context"

	"github.com/flexprice/flexprice/internal/types"
)

type Repository interface {
	Create(ctx context.Context, plan *Plan) error
	Get(ctx context.Context, id string) (*Plan, error)
	List(ctx context.Context, filter types.Filter) ([]*Plan, error)
	Update(ctx context.Context, plan *Plan) error
	Delete(ctx context.Context, id string) error
}
