package entityintegrationmapping

import (
	"context"

	"github.com/flexprice/flexprice/internal/types"
)

// Repository defines the interface for entity integration mapping data access
type Repository interface {
	Create(ctx context.Context, mapping *EntityIntegrationMapping) error
	Get(ctx context.Context, id string) (*EntityIntegrationMapping, error)
	List(ctx context.Context, filter *types.EntityIntegrationMappingFilter) ([]*EntityIntegrationMapping, error)
	Count(ctx context.Context, filter *types.EntityIntegrationMappingFilter) (int, error)
	ListAll(ctx context.Context, filter *types.EntityIntegrationMappingFilter) ([]*EntityIntegrationMapping, error)
	Update(ctx context.Context, mapping *EntityIntegrationMapping) error
	Delete(ctx context.Context, mapping *EntityIntegrationMapping) error

	// Provider-specific queries
	GetByEntityAndProvider(ctx context.Context, entityID string, entityType types.IntegrationEntityType, providerType string) (*EntityIntegrationMapping, error)
	GetByProviderEntity(ctx context.Context, providerType, providerEntityID string) (*EntityIntegrationMapping, error)
	ListByEntity(ctx context.Context, entityID string, entityType types.IntegrationEntityType) ([]*EntityIntegrationMapping, error)
	ListByProvider(ctx context.Context, providerType string) ([]*EntityIntegrationMapping, error)
}
