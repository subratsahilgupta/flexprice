package tenant

import (
	"context"
)

type Repository interface {
	Create(ctx context.Context, tenant *Tenant) error
	GetById(ctx context.Context, id string) (*Tenant, error)
}
