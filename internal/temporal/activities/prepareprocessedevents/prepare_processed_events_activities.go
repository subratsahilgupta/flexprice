package prepareprocessedevents

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/flexprice/flexprice/internal/temporal/models"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"go.temporal.io/sdk/activity"
)

// PrepareProcessedEventsActivities contains Temporal activities used by PrepareProcessedEventsWorkflow.
type PrepareProcessedEventsActivities struct {
	serviceParams service.ServiceParams
}

func NewPrepareProcessedEventsActivities(serviceParams service.ServiceParams) *PrepareProcessedEventsActivities {
	return &PrepareProcessedEventsActivities{serviceParams: serviceParams}
}

// CreateFeatureAndPriceActivity creates a metered feature (with meter) and a plan-scoped price for that meter.
// It gets defaults from settings (which already merges defaults).
func (a *PrepareProcessedEventsActivities) CreateFeatureAndPriceActivity(
	ctx context.Context,
	input models.CreateFeatureAndPriceActivityInput,
) (*models.CreateFeatureAndPriceActivityResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Starting CreateFeatureAndPriceActivity", "event_name", input.EventName, "plan_id", input.FeatureAndPriceConfig.PlanID)

	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Ensure context has tenant/env/user for BaseModel defaults
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	cfg := input.FeatureAndPriceConfig

	featureService := service.NewFeatureService(a.serviceParams)
	priceService := service.NewPriceService(a.serviceParams)

	// Convert workflow config to DTOs
	dtos, err := cfg.ToDTO(&models.WorkflowActionParams{
		EventName: input.EventName,
	})
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to convert feature and price config to DTOs").
			Mark(ierr.ErrInternal)
	}

	featureAndPriceDTOs, ok := dtos.(*models.CreateFeatureAndPriceDTOs)
	if !ok {
		return nil, ierr.NewError("failed to convert to CreateFeatureAndPriceDTOs").Mark(ierr.ErrInternal)
	}

	featureResp, err := featureService.CreateFeature(ctx, *featureAndPriceDTOs.Feature)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to create feature and meter via workflow").
			WithReportableDetails(map[string]interface{}{
				"event_name": input.EventName,
			}).
			Mark(ierr.ErrInternal)
	}

	if featureResp == nil || featureResp.Feature == nil || featureResp.Feature.MeterID == "" {
		return nil, ierr.NewError("feature created but meter_id missing").
			WithHint("Feature creation did not return a valid meter_id").
			WithReportableDetails(map[string]interface{}{
				"event_name": input.EventName,
				"feature_id": func() string {
					if featureResp != nil && featureResp.Feature != nil {
						return featureResp.Feature.ID
					}
					return ""
				}(),
			}).
			Mark(ierr.ErrInternal)
	}

	// Set meter_id on price DTO and create price
	featureAndPriceDTOs.Price.MeterID = featureResp.Feature.MeterID
	priceResp, err := priceService.CreatePrice(ctx, *featureAndPriceDTOs.Price)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to create price via workflow").
			WithReportableDetails(map[string]interface{}{
				"event_name": input.EventName,
				"plan_id":    cfg.PlanID,
				"meter_id":   featureResp.Feature.MeterID,
			}).
			Mark(ierr.ErrInternal)
	}

	logger.Info("CreateFeatureAndPriceActivity completed",
		"event_name", input.EventName,
		"feature_id", featureResp.Feature.ID,
		"meter_id", featureResp.Feature.MeterID,
		"price_id", priceResp.ID,
	)

	return &models.CreateFeatureAndPriceActivityResult{
		FeatureID: featureResp.Feature.ID,
		MeterID:   featureResp.Feature.MeterID,
		PriceID:   priceResp.ID,
		PlanID:    cfg.PlanID,
	}, nil
}

// RolloutToSubscriptionsActivity creates line items for all subscriptions associated with the plan
// It directly creates line items with event timestamp as StartDate - no currency/billing period matching
func (a *PrepareProcessedEventsActivities) RolloutToSubscriptionsActivity(
	ctx context.Context,
	input models.RolloutToSubscriptionsActivityInput,
) (*models.RolloutToSubscriptionsActivityResult, error) {
	logger := a.serviceParams.Logger
	logger.Debugw("Starting RolloutToSubscriptionsActivity",
		"plan_id", input.PlanID,
		"price_id", input.PriceID,
		"event_timestamp", input.EventTimestamp)

	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Ensure context has tenant/env/user
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	subscriptionService := service.NewSubscriptionService(a.serviceParams)

	// Get all active subscriptions for this plan
	subscriptionFilter := &types.SubscriptionFilter{
		PlanID: input.PlanID,
		SubscriptionStatus: []types.SubscriptionStatus{
			types.SubscriptionStatusActive,
			types.SubscriptionStatusTrialing,
		},
	}
	subsResponse, err := subscriptionService.ListSubscriptions(ctx, subscriptionFilter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to list subscriptions for plan").
			Mark(ierr.ErrDatabase)
	}

	logger.Debugw("Found subscriptions for plan",
		"plan_id", input.PlanID,
		"subscription_count", len(subsResponse.Items))

	lineItemsCreated := 0
	lineItemsFailed := 0

	// Create line item for each subscription
	for _, subResp := range subsResponse.Items {
		createReq := dto.CreateSubscriptionLineItemRequest{
			PriceID:   input.PriceID,
			StartDate: &input.EventTimestamp, // Use event timestamp as StartDate
			Metadata: map[string]string{
				"added_by":      "prepare_processed_events_workflow",
				"workflow_type": "rollout_to_subscriptions",
			},
			Quantity: decimal.Zero, // Usage prices have zero quantity
		}

		_, err := subscriptionService.AddSubscriptionLineItem(ctx, subResp.ID, createReq)
		if err != nil {
			logger.Error("Failed to create line item for subscription",
				"subscription_id", subResp.ID,
				"price_id", input.PriceID,
				"error", err)
			lineItemsFailed++
			continue
		}

		lineItemsCreated++
		logger.Debugw("Created line item for subscription",
			"subscription_id", subResp.ID,
			"price_id", input.PriceID,
			"start_date", input.EventTimestamp)
	}

	logger.Info("RolloutToSubscriptionsActivity completed",
		"plan_id", input.PlanID,
		"price_id", input.PriceID,
		"line_items_created", lineItemsCreated,
		"line_items_failed", lineItemsFailed)

	return &models.RolloutToSubscriptionsActivityResult{
		LineItemsCreated: lineItemsCreated,
		LineItemsFailed:  lineItemsFailed,
	}, nil
}
