package cron

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/ee/service"
	"github.com/flexprice/flexprice/internal/logger"
	cronModels "github.com/flexprice/flexprice/internal/temporal/models"
	"go.temporal.io/sdk/activity"
)

// RevenueRollupActivities wraps the periodic revenue_facts dirty-rollup entrypoint.
type RevenueRollupActivities struct {
	revenueService service.RevenueService
	logger         *logger.Logger
}

func NewRevenueRollupActivities(
	revenueService service.RevenueService,
	log *logger.Logger,
) *RevenueRollupActivities {
	return &RevenueRollupActivities{
		revenueService: revenueService,
		logger:         log,
	}
}

// RollupDirtyActivity re-upserts revenue_facts rows for every subscription with
// activity since `since`. Logic matches RevenueRollupService.RollupDirty.
func (a *RevenueRollupActivities) RollupDirtyActivity(ctx context.Context, since time.Time) (*cronModels.RevenueRollupWorkflowResult, error) {
	log := activity.GetLogger(ctx)
	log.Info("Starting revenue rollup dirty scan", "since", since)

	rolled, skipped, err := a.revenueService.RollupDirty(ctx, since)
	if err != nil {
		a.logger.Error(ctx, "revenue rollup dirty scan failed", "error", err, "since", since)
		return nil, err
	}

	a.logger.Info(ctx, "revenue rollup dirty scan completed", "rolled", rolled, "skipped", skipped, "since", since)
	log.Info("Completed revenue rollup dirty scan", "rolled", rolled, "skipped", skipped)
	return &cronModels.RevenueRollupWorkflowResult{Rolled: rolled, Skipped: skipped}, nil
}
