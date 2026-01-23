package workflows

import (
	"time"

	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/temporal/models"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	// Workflow name - must match the function name
	WorkflowPrepareProcessedEvents = "PrepareProcessedEventsWorkflow"

	// Activity names - must match registered method names
	ActivityCreateFeatureAndPrice  = "CreateFeatureAndPriceActivity"
	ActivityRolloutToSubscriptions = "RolloutToSubscriptionsActivity"
)

// PrepareProcessedEventsWorkflow creates missing feature/meter/price for an event name and optionally rolls out the plan prices to subscriptions.
func PrepareProcessedEventsWorkflow(ctx workflow.Context, input models.PrepareProcessedEventsWorkflowInput) (*models.PrepareProcessedEventsWorkflowResult, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	logger := logger.GetLogger()
	logger.Debugw("Starting PrepareProcessedEventsWorkflow",
		"event_name", input.EventName,
		"action_count", len(input.WorkflowConfig.Actions),
	)

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute * 10,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second * 5,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute * 2,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	result := &models.PrepareProcessedEventsWorkflowResult{
		EventName:       input.EventName,
		Status:          "processing",
		ActionsExecuted: 0,
		Results:         make([]models.PrepareProcessedEventsActionResult, 0, len(input.WorkflowConfig.Actions)),
	}

	// Track price_id from create_feature_and_price action for use in rollout action
	var createdPriceID string

	// Execute each action in sequence
	for i, action := range input.WorkflowConfig.Actions {
		actionType := action.GetAction()
		logger.Debugw("Executing workflow action",
			"event_name", input.EventName,
			"action_index", i,
			"action_type", actionType)

		actionResult := models.PrepareProcessedEventsActionResult{
			ActionType:  actionType,
			ActionIndex: i,
			Status:      "processing",
		}

		var err error
		switch actionType {
		case models.WorkflowActionCreateFeatureAndPrice:
			err = executeCreateFeatureAndPriceAction(ctx, input, action, &actionResult, &createdPriceID)

		case models.WorkflowActionRolloutToSubscriptions:
			err = executeRolloutToSubscriptionsAction(ctx, input, action, &actionResult, createdPriceID)

		default:
			logger.Warnw("Unknown workflow action type",
				"event_name", input.EventName,
				"action_type", actionType,
				"action_index", i)

			actionResult.Status = models.WorkflowStatusFailed
			errorMsg := "unknown workflow action type: " + string(actionType)
			actionResult.Error = &errorMsg
			result.Results = append(result.Results, actionResult)

			result.Status = models.WorkflowStatusFailed
			result.CompletedAt = workflow.Now(ctx)
			result.ErrorSummary = &errorMsg
			return result, nil
		}

		if err != nil {
			logger.Error("Workflow action failed",
				"event_name", input.EventName,
				"action_index", i,
				"action_type", actionType,
				"error", err)

			actionResult.Status = models.WorkflowStatusFailed
			errorMsg := err.Error()
			actionResult.Error = &errorMsg
			result.Results = append(result.Results, actionResult)

			result.Status = models.WorkflowStatusFailed
			result.CompletedAt = workflow.Now(ctx)
			result.ErrorSummary = &errorMsg
			return result, nil
		}

		actionResult.Status = models.WorkflowStatusCompleted
		result.Results = append(result.Results, actionResult)
		result.ActionsExecuted++

		logger.Debugw("Workflow action completed successfully",
			"event_name", input.EventName,
			"action_index", i,
			"action_type", actionType,
			"resource_id", actionResult.ResourceID)
	}

	// All actions completed successfully
	result.Status = models.WorkflowStatusCompleted
	result.CompletedAt = workflow.Now(ctx)

	logger.Info("PrepareProcessedEventsWorkflow completed successfully",
		"event_name", input.EventName,
		"actions_executed", result.ActionsExecuted)

	return result, nil
}

// executeCreateFeatureAndPriceAction executes the create feature and price action
func executeCreateFeatureAndPriceAction(
	ctx workflow.Context,
	input models.PrepareProcessedEventsWorkflowInput,
	action models.WorkflowActionConfig,
	actionResult *models.PrepareProcessedEventsActionResult,
	createdPriceID *string,
) error {
	featureAction, ok := action.(*models.CreateFeatureAndPriceActionConfig)
	if !ok {
		return temporal.NewApplicationError("invalid action config type for create_feature_and_price", "InvalidActionConfig")
	}

	if featureAction.PlanID == "" {
		return temporal.NewApplicationError("plan_id is required for create_feature_and_price action", "MissingPlanID")
	}

	activityInput := models.CreateFeatureAndPriceActivityInput{
		EventName:             input.EventName,
		TenantID:              input.TenantID,
		EnvironmentID:         input.EnvironmentID,
		FeatureAndPriceConfig: featureAction,
	}

	var activityResult models.CreateFeatureAndPriceActivityResult
	err := workflow.ExecuteActivity(ctx, ActivityCreateFeatureAndPrice, activityInput).Get(ctx, &activityResult)
	if err != nil {
		return err
	}

	// Store feature_id as the primary resource ID
	actionResult.ResourceID = activityResult.FeatureID
	actionResult.ResourceType = models.WorkflowResourceTypeFeature

	// Store price_id for rollout action to use
	*createdPriceID = activityResult.PriceID
	return nil
}

// executeRolloutToSubscriptionsAction executes the rollout to subscriptions action
func executeRolloutToSubscriptionsAction(
	ctx workflow.Context,
	input models.PrepareProcessedEventsWorkflowInput,
	action models.WorkflowActionConfig,
	actionResult *models.PrepareProcessedEventsActionResult,
	priceID string,
) error {
	rolloutAction, ok := action.(*models.RolloutToSubscriptionsActionConfig)
	if !ok {
		return temporal.NewApplicationError("invalid action config type for rollout_to_subscriptions", "InvalidActionConfig")
	}

	if rolloutAction.PlanID == "" {
		return temporal.NewApplicationError("plan_id is required for rollout_to_subscriptions action", "MissingPlanID")
	}

	if priceID == "" {
		return temporal.NewApplicationError("price_id is required for rollout_to_subscriptions action", "MissingPriceID")
	}

	activityInput := models.RolloutToSubscriptionsActivityInput{
		PlanID:         rolloutAction.PlanID,
		PriceID:        priceID,
		EventTimestamp: input.EventTimestamp,
		TenantID:       input.TenantID,
		EnvironmentID:  input.EnvironmentID,
	}

	var activityResult models.RolloutToSubscriptionsActivityResult
	err := workflow.ExecuteActivity(ctx, ActivityRolloutToSubscriptions, activityInput).Get(ctx, &activityResult)
	if err != nil {
		return err
	}

	// Store plan_id as the resource ID
	actionResult.ResourceID = rolloutAction.PlanID
	actionResult.ResourceType = models.WorkflowResourceTypePlan
	return nil
}
