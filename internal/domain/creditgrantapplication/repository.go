package creditgrantapplication

import (
	"context"

	"github.com/flexprice/flexprice/internal/types"
)

// Repository defines the interface for credit grant application data access
type Repository interface {
	Create(ctx context.Context, application *CreditGrantApplication) error
	Get(ctx context.Context, id string) (*CreditGrantApplication, error)
	List(ctx context.Context, filter *types.CreditGrantApplicationFilter) ([]*CreditGrantApplication, error)
	Count(ctx context.Context, filter *types.CreditGrantApplicationFilter) (int, error)
	ListAll(ctx context.Context, filter *types.CreditGrantApplicationFilter) ([]*CreditGrantApplication, error)
	Update(ctx context.Context, application *CreditGrantApplication) error
	Delete(ctx context.Context, application *CreditGrantApplication) error
}
