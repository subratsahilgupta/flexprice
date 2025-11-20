package subscription

import (
	"time"

	subscriptionModels "github.com/flexprice/flexprice/internal/temporal/models/subscription"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	// Workflow name - must match the function name
	WorkflowScheduleSubscriptionUpdateBillingPeriod = "ScheduleSubscriptionUpdateBillingPeriodWorkflow"
	// Activity names - must match the registered method names
	ActivityScheduleSubscriptionUpdateBillingPeriod = "ScheduleSubscriptionUpdateBillingPeriod"
)

// ScheduleSubscriptionUpdateBillingPeriodWorkflow schedules a subscription update billing period workflow
func ScheduleSubscriptionUpdateBillingPeriodWorkflow(ctx workflow.Context, input subscriptionModels.ScheduleSubscriptionUpdateBillingPeriodWorkflowInput) (*subscriptionModels.ScheduleSubscriptionUpdateBillingPeriodWorkflowResult, error) {
	// Validate input
	if err := input.Validate(); err != nil {
		return nil, err
	}

	logger := workflow.GetLogger(ctx)

	// Define activity options with extended timeouts for large batch processing
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: time.Hour * 24,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second * 10,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute * 10,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Execute the main schedule subscription update billing period activity
	var result subscriptionModels.ScheduleSubscriptionUpdateBillingPeriodWorkflowResult

	activityInput := subscriptionModels.ScheduleSubscriptionUpdateBillingPeriodWorkflowInput{
		BatchSize: input.BatchSize,
	}
	err := workflow.ExecuteActivity(ctx, ActivityScheduleSubscriptionUpdateBillingPeriod, activityInput).Get(ctx, &result)

	if err != nil {
		logger.Error("Schedule subscription update billing period workflow failed", "error", err)
		return nil, err
	}

	return &result, nil
}
