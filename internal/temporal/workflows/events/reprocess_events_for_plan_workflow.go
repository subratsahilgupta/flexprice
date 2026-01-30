package events

import (
	"time"

	models "github.com/flexprice/flexprice/internal/temporal/models/events"
	planActivities "github.com/flexprice/flexprice/internal/temporal/activities/plan"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	// Workflow name - must match the function name
	WorkflowReprocessEventsForPlan = "ReprocessEventsForPlanWorkflow"
)

// ReprocessEventsForPlanWorkflow triggers event reprocessing for missing (subscription_id, price_id, customer_id) pairs after plan price sync.
func ReprocessEventsForPlanWorkflow(ctx workflow.Context, input models.ReprocessEventsForPlanWorkflowInput) error {
	if err := input.Validate(); err != nil {
		return err
	}
	if len(input.MissingPairs) == 0 {
		return nil
	}

	logger := workflow.GetLogger(ctx)
	logger.Info("Starting reprocess events for plan workflow",
		"missing_pairs_count", len(input.MissingPairs))

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: time.Hour * 2,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second * 10,
			BackoffCoefficient:  2.0,
			MaximumInterval:    time.Minute * 10,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	activityInput := planActivities.ReprocessEventsForPlanInput{
		MissingPairs:  input.MissingPairs,
		TenantID:      input.TenantID,
		EnvironmentID: input.EnvironmentID,
		UserID:        input.UserID,
	}

	err := workflow.ExecuteActivity(ctx, planActivities.ActivityReprocessEventsForPlan, activityInput).Get(ctx, nil)
	if err != nil {
		logger.Error("Reprocess events for plan workflow failed", "error", err)
		return err
	}

	logger.Info("Reprocess events for plan workflow completed successfully")
	return nil
}
