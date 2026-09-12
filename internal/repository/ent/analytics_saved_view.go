package ent

import (
	"context"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/ent/analyticssavedview"
	domainAnalytics "github.com/flexprice/flexprice/internal/domain/analytics"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/utils"
)

type analyticsSavedViewRepository struct {
	client postgres.IClient
	log    *logger.Logger
}

// NewAnalyticsSavedViewRepository creates a new ent-backed analytics.Repository.
func NewAnalyticsSavedViewRepository(client postgres.IClient, log *logger.Logger) domainAnalytics.Repository {
	return &analyticsSavedViewRepository{client: client, log: log}
}

func (r *analyticsSavedViewRepository) Create(ctx context.Context, v *domainAnalytics.SavedView) error {
	client := r.client.Writer(ctx)

	def, err := utils.ToMap(v.Definition)
	if err != nil {
		return err
	}

	r.log.Debug(ctx, "creating analytics saved view",
		"saved_view_id", v.ID,
		"tenant_id", types.GetTenantID(ctx),
		"name", v.Name,
	)

	created, err := client.AnalyticsSavedView.Create().
		SetID(v.ID).
		SetTenantID(types.GetTenantID(ctx)).
		SetEnvironmentID(types.GetEnvironmentID(ctx)).
		SetName(v.Name).
		SetVersion(v.Version).
		SetDefinition(def).
		Save(ctx)

	if err != nil {
		if ent.IsConstraintError(err) {
			return ierr.WithError(err).
				WithHint("A saved view with this ID already exists").
				WithReportableDetails(map[string]any{
					"saved_view_id": v.ID,
				}).
				Mark(ierr.ErrAlreadyExists)
		}
		return ierr.WithError(err).
			WithHint("Failed to create analytics saved view").
			Mark(ierr.ErrDatabase)
	}

	*v = *domainAnalytics.FromEnt(created)
	return nil
}

func (r *analyticsSavedViewRepository) Get(ctx context.Context, id string) (*domainAnalytics.SavedView, error) {
	client := r.client.Reader(ctx)

	r.log.Debug(ctx, "getting analytics saved view", "saved_view_id", id)

	v, err := client.AnalyticsSavedView.Query().
		Where(
			analyticssavedview.ID(id),
			analyticssavedview.TenantID(types.GetTenantID(ctx)),
			analyticssavedview.EnvironmentID(types.GetEnvironmentID(ctx)),
			analyticssavedview.Status(string(types.StatusPublished)),
		).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ierr.WithError(err).
				WithHintf("Saved view with ID %s was not found", id).
				WithReportableDetails(map[string]any{
					"saved_view_id": id,
				}).
				Mark(ierr.ErrNotFound)
		}
		return nil, ierr.WithError(err).
			WithHint("Failed to get analytics saved view").
			Mark(ierr.ErrDatabase)
	}

	return domainAnalytics.FromEnt(v), nil
}

func (r *analyticsSavedViewRepository) List(ctx context.Context) ([]*domainAnalytics.SavedView, error) {
	client := r.client.Reader(ctx)

	r.log.Debug(ctx, "listing analytics saved views",
		"tenant_id", types.GetTenantID(ctx),
		"environment_id", types.GetEnvironmentID(ctx),
	)

	views, err := client.AnalyticsSavedView.Query().
		Where(
			analyticssavedview.TenantID(types.GetTenantID(ctx)),
			analyticssavedview.EnvironmentID(types.GetEnvironmentID(ctx)),
			analyticssavedview.Status(string(types.StatusPublished)),
		).
		All(ctx)

	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to list analytics saved views").
			Mark(ierr.ErrDatabase)
	}

	result := make([]*domainAnalytics.SavedView, 0, len(views))
	for _, v := range views {
		result = append(result, domainAnalytics.FromEnt(v))
	}
	return result, nil
}
