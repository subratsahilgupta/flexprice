package workflows

import (
	"time"

	"github.com/flexprice/flexprice/internal/temporal/models"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	// Workflow name - must match the function name
	WorkflowSubscriptionScheduler = "SubscriptionSchedulerWorkflow"

	// Activity names for scheduler
	ActivityFetchSubscriptionBatch       = "FetchSubscriptionBatch"
	ActivityEnqueueSubscriptionWorkflows = "EnqueueSubscriptionWorkflows"
)

// SubscriptionSchedulerWorkflow is the entry point workflow triggered on a cron schedule
// It fetches all subscriptions in batches and enqueues independent workflows for each subscription
func SubscriptionSchedulerWorkflow(ctx workflow.Context, input models.SubscriptionSchedulerWorkflowInput) error {
	logger := workflow.GetLogger(ctx)

	// Validate input
	if err := input.Validate(); err != nil {
		return err
	}

	logger.Info("Starting subscription scheduler workflow", "batch_size", input.BatchSize)

	// Define activity options with extended timeouts for batch fetching
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	offset := 0
	batchSize := input.BatchSize
	if batchSize <= 0 {
		batchSize = 100 // Default batch size
	}

	totalEnqueued := 0

	// Fetch subscriptions in batches and enqueue workflows
	for {
		// Fetch batch of subscription IDs
		fetchInput := models.FetchSubscriptionBatchInput{
			TenantID:      workflow.GetInfo(ctx).WorkflowExecution.ID, // TODO: Get from context
			EnvironmentID: workflow.GetInfo(ctx).WorkflowExecution.ID, // TODO: Get from context
			BatchSize:     batchSize,
			Offset:        offset,
		}

		var fetchOutput models.FetchSubscriptionBatchOutput
		err := workflow.ExecuteActivity(ctx, ActivityFetchSubscriptionBatch, fetchInput).Get(ctx, &fetchOutput)
		if err != nil {
			logger.Error("Failed to fetch subscription batch", "error", err, "offset", offset)
			return err
		}

		// Check if we have any subscriptions in this batch
		if len(fetchOutput.SubscriptionIDs) == 0 {
			logger.Info("Completed processing all subscriptions", "total_enqueued", totalEnqueued)
			break
		}

		logger.Info("Enqueueing subscription workflows",
			"count", len(fetchOutput.SubscriptionIDs),
			"offset", offset,
			"batch_size", batchSize)

		// Enqueue independent workflows for each subscription
		enqueueInput := models.EnqueueSubscriptionWorkflowsInput{
			SubscriptionIDs: fetchOutput.SubscriptionIDs,
			TenantID:        fetchInput.TenantID,
			EnvironmentID:   fetchInput.EnvironmentID,
		}

		err = workflow.ExecuteActivity(ctx, ActivityEnqueueSubscriptionWorkflows, enqueueInput).Get(ctx, nil)
		if err != nil {
			logger.Error("Failed to enqueue subscription workflows", "error", err, "offset", offset)
			// Continue processing even if enqueuing fails for this batch
			// We log the error and move to next batch
		} else {
			totalEnqueued += len(fetchOutput.SubscriptionIDs)
		}

		offset += batchSize
	}

	logger.Info("Subscription scheduler workflow completed successfully", "total_enqueued", totalEnqueued)
	return nil
}
