package analytics

import (
	"context"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/utils"
)

// SavedView is a persisted, named analytics view definition.
type SavedView struct {
	ID         string
	Name       string
	Version    int
	Definition ViewDefinition
	types.BaseModel
}

// Repository persists and retrieves saved analytics views.
type Repository interface {
	Create(ctx context.Context, v *SavedView) error
	Get(ctx context.Context, id string) (*SavedView, error)
	List(ctx context.Context) ([]*SavedView, error)
}

// FromEnt converts an ent AnalyticsSavedView row into the domain SavedView.
func FromEnt(e *ent.AnalyticsSavedView) *SavedView {
	if e == nil {
		return nil
	}

	def, _ := utils.ToStruct[ViewDefinition](e.Definition)

	return &SavedView{
		ID:         e.ID,
		Name:       e.Name,
		Version:    e.Version,
		Definition: def,
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
