package analytics

import (
	"context"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/types"
)

// View is a persisted, named analytics view definition.
type View struct {
	ID            string
	Name          string
	Version       int
	Definition    *types.ViewDefinition
	EnvironmentID string
	types.BaseModel
}

// Repository persists and retrieves analytics views.
type Repository interface {
	Create(ctx context.Context, v *View) error
	Get(ctx context.Context, id string) (*View, error)
	List(ctx context.Context) ([]*View, error)
}

// FromEnt converts an ent AnalyticsView row into the domain View. Ent owns
// the jsonb (un)marshal of the typed definition field, so no manual decoding
// here.
func FromEnt(e *ent.AnalyticsView) *View {
	if e == nil {
		return nil
	}

	return &View{
		ID:            e.ID,
		Name:          e.Name,
		Version:       e.Version,
		Definition:    &e.Definition,
		EnvironmentID: e.EnvironmentID,
		BaseModel: types.BaseModel{
			TenantID:  e.TenantID,
			Status:    types.Status(e.Status),
			CreatedAt: e.CreatedAt,
			UpdatedAt: e.UpdatedAt,
			CreatedBy: e.CreatedBy,
			UpdatedBy: e.UpdatedBy,
		},
	}
}
