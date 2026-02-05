package events

import (
	"fmt"
	"time"

	planActivities "github.com/flexprice/flexprice/internal/temporal/activities/plan"
	models "github.com/flexprice/flexprice/internal/temporal/models/events"
	"github.com/samber/lo"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	// Workflow name - must match the function name
	WorkflowReprocessEventsForPlan = "ReprocessEventsForPlanWorkflow"
)

// ReprocessEventsForPlanWorkflow triggers event reprocessing for missing (subscription_id, price_id, customer_id) pairs after plan price sync.
func ReprocessEventsForPlanWorkflow(ctx workflow.Context, input models.ReprocessEventsForPlanWorkflowInput) (*models.ReprocessEventsForPlanWorkflowResult, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if len(input.MissingPairs) == 0 {
		return &models.ReprocessEventsForPlanWorkflowResult{
			CustomerIDs:     nil,
			SubscriptionIDs: nil,
			PriceIDs:        nil,
			PairCount:       0,
			Message:         "No pairs to reprocess",
			CompletedAt:     workflow.Now(ctx),
		}, nil
	}

	logger := workflow.GetLogger(ctx)
	logger.Info("Starting reprocess events for plan workflow",
		"missing_pairs_count", len(input.MissingPairs))

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: time.Hour * 2,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second * 10,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute * 10,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	err := workflow.ExecuteActivity(ctx, planActivities.ActivityReprocessEventsForPlan, planActivities.ReprocessEventsForPlanInput(input)).Get(ctx, nil)
	if err != nil {
		logger.Error("Reprocess events for plan workflow failed", "error", err)
		return nil, err
	}

	result := buildReprocessEventsForPlanResult(ctx, input)
	logger.Info("Reprocess events for plan workflow completed successfully",
		"pair_count", result.PairCount,
		"customer_count", len(result.CustomerIDs))
	return result, nil
}

// buildReprocessEventsForPlanResult builds the workflow result from the input (unique IDs and summary message).
func buildReprocessEventsForPlanResult(ctx workflow.Context, input models.ReprocessEventsForPlanWorkflowInput) *models.ReprocessEventsForPlanWorkflowResult {
	customers := lo.Uniq(lo.FilterMap(input.MissingPairs, func(p models.MissingPair, _ int) (string, bool) {
		return p.CustomerID, p.CustomerID != ""
	}))
	subscriptions := lo.Uniq(lo.FilterMap(input.MissingPairs, func(p models.MissingPair, _ int) (string, bool) {
		return p.SubscriptionID, p.SubscriptionID != ""
	}))
	prices := lo.Uniq(lo.FilterMap(input.MissingPairs, func(p models.MissingPair, _ int) (string, bool) {
		return p.PriceID, p.PriceID != ""
	}))

	n := len(input.MissingPairs)
	m := len(customers)
	msg := fmt.Sprintf("Reprocessed events for %d (subscription, price, customer) pairs across %d customers", n, m)

	return &models.ReprocessEventsForPlanWorkflowResult{
		CustomerIDs:     customers,
		SubscriptionIDs: subscriptions,
		PriceIDs:        prices,
		PairCount:       n,
		Message:         msg,
		CompletedAt:     workflow.Now(ctx),
	}
}
