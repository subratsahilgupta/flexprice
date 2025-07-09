package custompricingunit

import (
	"context"
)

// CustomPricingUnitRepository defines the interface for custom pricing unit persistence
type CustomPricingUnitRepository interface {
	// Create creates a new custom pricing unit
	Create(ctx context.Context, unit *CustomPricingUnit) error

	// GetByID retrieves a custom pricing unit by ID
	GetByID(ctx context.Context, id string) (*CustomPricingUnit, error)

	// GetByCode retrieves a custom pricing unit by code
	GetByCode(ctx context.Context, code string) (*CustomPricingUnit, error)

	// List returns a list of custom pricing units with optional filtering
	List(ctx context.Context, filter *CustomPricingUnitFilter) ([]*CustomPricingUnit, error)

	// Update updates an existing custom pricing unit
	Update(ctx context.Context, unit *CustomPricingUnit) error

	// Delete deletes a custom pricing unit
	Delete(ctx context.Context, id string) error

	// ExistsByCode checks if a custom pricing unit exists with the given code
	ExistsByCode(ctx context.Context, code string) (bool, error)

	// IsInUse checks if a custom pricing unit is being used by any prices
	IsInUse(ctx context.Context, id string) (bool, error)
}
