package prepareprocessedevents

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/flexprice/flexprice/internal/temporal/models"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
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
	logger := a.serviceParams.Logger
	logger.Debugw("Starting CreateFeatureAndPriceActivity",
		"event_name", input.EventName,
		"plan_id", input.FeatureAndPriceConfig.PlanID)

	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Ensure context has tenant/env/user for BaseModel defaults
	ctx = types.SetTenantID(ctx, input.TenantID)
	ctx = types.SetEnvironmentID(ctx, input.EnvironmentID)

	cfg := input.FeatureAndPriceConfig

	featureService := service.NewFeatureService(a.serviceParams)
	priceService := service.NewPriceService(a.serviceParams)

	// Convert workflow config to DTOs - returns slice of DTOs (one per feature)
	dtos, err := cfg.ToDTO(&models.WorkflowActionParams{
		EventName:       input.EventName,
		EventProperties: input.EventProperties,
	})
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to convert feature and price config to DTOs").
			Mark(ierr.ErrInternal)
	}

	// Handle multiple features - ToDTO returns []CreateFeatureAndPriceDTOs
	dtosList, ok := dtos.([]models.CreateFeatureAndPriceDTOs)
	if !ok {
		return nil, ierr.NewError("failed to convert to []CreateFeatureAndPriceDTOs").
			WithHint("Failed to convert DTOs to []CreateFeatureAndPriceDTOs").
			Mark(ierr.ErrInternal)
	}

	results := make([]models.FeaturePriceResult, 0, len(dtosList))

	// Create each feature and price
	for i, featureAndPriceDTOs := range dtosList {
		featureResp, err := featureService.CreateFeature(ctx, *featureAndPriceDTOs.Feature)
		if err != nil {
			return nil, ierr.WithError(err).
				WithHint("Failed to create feature and meter via workflow").
				WithReportableDetails(map[string]interface{}{
					"event_name":    input.EventName,
					"feature_index": i,
					"feature_name":  featureAndPriceDTOs.Feature.Name,
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
					"event_name":    input.EventName,
					"feature_index": i,
					"feature_id":    featureResp.Feature.ID,
					"plan_id":       cfg.PlanID,
					"meter_id":      featureResp.Feature.MeterID,
				}).
				Mark(ierr.ErrInternal)
		}

		results = append(results, models.FeaturePriceResult{
			FeatureID: featureResp.Feature.ID,
			MeterID:   featureResp.Feature.MeterID,
			PriceID:   priceResp.ID,
		})

		logger.Infow("Created feature and price",
			"event_name", input.EventName,
			"feature_index", i,
			"feature_id", featureResp.Feature.ID,
			"meter_id", featureResp.Feature.MeterID,
			"price_id", priceResp.ID,
			"feature_name", featureResp.Feature.Name,
		)
	}

	logger.Debugw("CreateFeatureAndPriceActivity completed",
		"event_name", input.EventName,
		"features_created", len(results),
	)

	return &models.CreateFeatureAndPriceActivityResult{
		Features: results,
		PlanID:   cfg.PlanID,
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
		"subscription_count", len(subsResponse.Items),
		"subscription_ids", func() []string {
			ids := make([]string, 0, len(subsResponse.Items))
			for _, sub := range subsResponse.Items {
				ids = append(ids, sub.ID)
			}
			return ids
		}())

	lineItemsCreated := 0
	lineItemsFailed := 0

	// Create line item for each subscription
	for _, subResp := range subsResponse.Items {
		createReq := dto.CreateSubscriptionLineItemRequest{
			PriceID:              input.PriceID,
			StartDate:            &input.EventTimestamp, // Use event timestamp as StartDate
			Quantity:             decimal.Zero,          // Usage prices have zero quantity
			SkipEntitlementCheck: true,                  // Skip entitlement check for workflow-created line items
			Metadata: map[string]string{
				"added_by":      "prepare_processed_events_workflow",
				"workflow_type": "rollout_to_subscriptions",
			},
		}

		lineItemResp, err := subscriptionService.AddSubscriptionLineItem(ctx, subResp.ID, createReq)
		if err != nil {
			logger.Errorw("Failed to create line item for subscription",
				"subscription_id", subResp.ID,
				"price_id", input.PriceID,
				"plan_id", input.PlanID,
				"error", err)
			lineItemsFailed++
			continue
		}

		lineItemsCreated++
		logger.Debugw("Successfully created line item for subscription",
			"subscription_id", subResp.ID,
			"price_id", input.PriceID,
			"plan_id", input.PlanID,
			"line_item_id", lineItemResp.SubscriptionLineItem.ID,
			"line_item_entity_id", lineItemResp.SubscriptionLineItem.EntityID,
			"line_item_entity_type", lineItemResp.SubscriptionLineItem.EntityType,
			"line_item_status", lineItemResp.SubscriptionLineItem.Status,
			"start_date", input.EventTimestamp)
	}

	logger.Debugw("RolloutToSubscriptionsActivity completed",
		"plan_id", input.PlanID,
		"price_id", input.PriceID,
		"line_items_created", lineItemsCreated,
		"line_items_failed", lineItemsFailed)

	return &models.RolloutToSubscriptionsActivityResult{
		LineItemsCreated: lineItemsCreated,
		LineItemsFailed:  lineItemsFailed,
	}, nil
}
