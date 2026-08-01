package subscription

import (
	"time"

	"github.com/flexprice/flexprice/internal/temporal/models"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	// Workflow name - must match the function name
	WorkflowMarketplaceSubscriptionFinalUsageFlush = "MarketplaceSubscriptionFinalUsageFlushWorkflow"
	// Activity name - must match the registered method name
	ActivityMarketplaceSubscriptionFinalUsageFlush = "MarketplaceSubscriptionFinalUsageFlushActivity"
)

// MarketplaceSubscriptionFinalUsageFlushWorkflow reports a cancelled subscription's final marketplace usage
// and archives its marketplace mapping. Started once per cancellation from CancelSubscription
// (post-commit, non-blocking) — not on a schedule, because AWS and GCP only accept a final report
// within roughly an hour of cancellation, far tighter than the 6h snapshot / 3h report cadence
// (ERD FLE-1106 §3, §4).
//
// A thin wrapper around a single activity, matching MarketplaceUsageReportWorkflow. The activity only
// delinks once everything reported successfully — the mapping stays published on any failure, so the
// report cron can keep retrying it. See ERD FLE-1106 §3.6.
func MarketplaceSubscriptionFinalUsageFlushWorkflow(
	ctx workflow.Context,
	input models.MarketplaceSubscriptionFinalUsageFlushWorkflowInput,
) (*models.MarketplaceSubscriptionFinalUsageFlushWorkflowResult, error) {
	log := workflow.GetLogger(ctx)
	log.Info("Starting MarketplaceSubscriptionFinalUsageFlushWorkflow", "subscription_id", input.SubscriptionID)

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    5 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    1 * time.Minute,
			MaximumAttempts:    5,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var result models.MarketplaceSubscriptionFinalUsageFlushWorkflowResult
	if err := workflow.ExecuteActivity(ctx, ActivityMarketplaceSubscriptionFinalUsageFlush, models.MarketplaceSubscriptionFinalUsageFlushActivityInput{
		SubscriptionID: input.SubscriptionID,
		CancelAt:       input.CancelAt,
		TenantID:       input.TenantID,
		EnvironmentID:  input.EnvironmentID,
	}).Get(ctx, &result); err != nil {
		log.Error("MarketplaceSubscriptionFinalUsageFlushWorkflow activity failed",
			"subscription_id", input.SubscriptionID, "error", err)
		return nil, err
	}

	log.Info("MarketplaceSubscriptionFinalUsageFlushWorkflow completed",
		"subscription_id", input.SubscriptionID,
		"final_record_id", result.FinalRecordID,
		"records_succeeded", len(result.SucceededRecordIDs),
		"records_failed", len(result.FailedRecordIDs),
		"records_skipped", len(result.SkippedRecordIDs),
		"mappings_delinked", len(result.DelinkedMappingIDs))
	return &result, nil
}
