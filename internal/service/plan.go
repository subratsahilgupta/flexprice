package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/creditgrant"
	"github.com/flexprice/flexprice/internal/domain/entitlement"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/repository/ent"
	"github.com/flexprice/flexprice/internal/types"
	webhookDto "github.com/flexprice/flexprice/internal/webhook/dto"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

type SyncPlanPricesResponse struct {
	Message                string `json:"message"`
	PlanID                 string `json:"plan_id"`
	PlanName               string `json:"plan_name"`
	SynchronizationSummary struct {
		SubscriptionsProcessed int `json:"subscriptions_processed"`
		PricesAdded            int `json:"prices_added"`
		PricesRemoved          int `json:"prices_removed"`
		PricesSkipped          int `json:"prices_skipped"`
	} `json:"synchronization_summary"`
}

type PlanService interface {
	CreatePlan(ctx context.Context, req dto.CreatePlanRequest) (*dto.CreatePlanResponse, error)
	GetPlan(ctx context.Context, id string) (*dto.PlanResponse, error)
	GetPlans(ctx context.Context, filter *types.PlanFilter) (*dto.ListPlansResponse, error)
	UpdatePlan(ctx context.Context, id string, req dto.UpdatePlanRequest) (*dto.PlanResponse, error)
	DeletePlan(ctx context.Context, id string) error
	SyncPlanPrices(ctx context.Context, id string) (*SyncPlanPricesResponse, error)
}

type planService struct {
	ServiceParams
}

func NewPlanService(
	params ServiceParams,
) PlanService {
	return &planService{
		ServiceParams: params,
	}
}

func (s *planService) CreatePlan(ctx context.Context, req dto.CreatePlanRequest) (*dto.CreatePlanResponse, error) {
	// Validate request
	if err := req.Validate(); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid plan data provided").
			Mark(ierr.ErrValidation)
	}

	plan := req.ToPlan(ctx)

	// Start a transaction to create plan, prices, and entitlements
	err := s.DB.WithTx(ctx, func(ctx context.Context) error {
		// 1. Create the plan
		if err := s.PlanRepo.Create(ctx, plan); err != nil {
			return err
		}

		// 2. Create prices in bulk if present
		if len(req.Prices) > 0 {
			prices := make([]*price.Price, len(req.Prices))
			for i, priceReq := range req.Prices {
				price, err := priceReq.ToPrice(ctx)
				if err != nil {
					return ierr.WithError(err).
						WithHint("Failed to create price").
						Mark(ierr.ErrValidation)
				}
				price.PlanID = plan.ID
				prices[i] = price
			}

			// Create prices in bulk
			if err := s.PriceRepo.CreateBulk(ctx, prices); err != nil {
				return err
			}
		}

		// 3. Create entitlements in bulk if present
		// TODO: add feature validations - maybe by cerating a bulk create method
		// in the entitlement service that can own this for create and updates
		if len(req.Entitlements) > 0 {
			entitlements := make([]*entitlement.Entitlement, len(req.Entitlements))
			for i, entReq := range req.Entitlements {
				ent := entReq.ToEntitlement(ctx, plan.ID)
				entitlements[i] = ent
			}

			// Create entitlements in bulk
			if _, err := s.EntitlementRepo.CreateBulk(ctx, entitlements); err != nil {
				return err
			}

			// Publish webhook events for each entitlement
			for _, ent := range entitlements {
				s.publishEntitlementWebhookEvent(ctx, types.WebhookEventEntitlementCreated, ent.ID)
			}
		}

		// 4. Create credit grants in bulk if present
		if len(req.CreditGrants) > 0 {

			creditGrants := make([]*creditgrant.CreditGrant, len(req.CreditGrants))
			for i, creditGrantReq := range req.CreditGrants {
				creditGrant := creditGrantReq.ToCreditGrant(ctx)
				creditGrant.PlanID = &plan.ID
				creditGrant.Scope = types.CreditGrantScopePlan
				// Clear subscription_id for plan-scoped credit grants
				creditGrant.SubscriptionID = nil
				creditGrants[i] = creditGrant
			}

			// validate credit grants
			for _, creditGrant := range creditGrants {
				if err := creditGrant.Validate(); err != nil {
					return ierr.WithError(err).
						WithHint("Invalid credit grant data provided").
						WithReportableDetails(map[string]any{
							"credit_grant": creditGrant,
						}).
						Mark(ierr.ErrValidation)
				}
			}

			// Create credit grants in bulk
			if _, err := s.CreditGrantRepo.CreateBulk(ctx, creditGrants); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	response := &dto.CreatePlanResponse{Plan: plan}

	return response, nil
}

func (s *planService) GetPlan(ctx context.Context, id string) (*dto.PlanResponse, error) {
	if id == "" {
		return nil, ierr.NewError("plan ID is required").
			WithHint("Please provide a valid plan ID").
			Mark(ierr.ErrValidation)
	}

	plan, err := s.PlanRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	priceService := NewPriceService(s.PriceRepo, s.MeterRepo, s.Logger)
	entitlementService := NewEntitlementService(s.ServiceParams)

	pricesResponse, err := priceService.GetPricesByPlanID(ctx, plan.ID)
	if err != nil {
		s.Logger.Errorw("failed to fetch prices for plan", "plan_id", plan.ID, "error", err)
		return nil, err
	}

	entitlements, err := entitlementService.GetPlanEntitlements(ctx, plan.ID)
	if err != nil {
		s.Logger.Errorw("failed to fetch entitlements for plan", "plan_id", plan.ID, "error", err)
		return nil, err
	}

	creditGrants, err := NewCreditGrantService(s.ServiceParams).GetCreditGrantsByPlan(ctx, plan.ID)
	if err != nil {
		s.Logger.Errorw("failed to fetch credit grants for plan", "plan_id", plan.ID, "error", err)
		return nil, err
	}

	response := &dto.PlanResponse{
		Plan:         plan,
		Prices:       pricesResponse.Items,
		Entitlements: entitlements.Items,
		CreditGrants: creditGrants.Items,
	}
	return response, nil
}

func (s *planService) GetPlans(ctx context.Context, filter *types.PlanFilter) (*dto.ListPlansResponse, error) {
	if filter == nil {
		filter = types.NewPlanFilter()
	}

	if err := filter.Validate(); err != nil {
		return nil, err
	}

	// Fetch plans
	plans, err := s.PlanRepo.List(ctx, filter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve plans").
			Mark(ierr.ErrDatabase)
	}

	// Get count
	count, err := s.PlanRepo.Count(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Build response
	response := &dto.ListPlansResponse{
		Items: make([]*dto.PlanResponse, len(plans)),
		Pagination: types.NewPaginationResponse(
			count,
			filter.GetLimit(),
			filter.GetOffset(),
		),
	}

	if len(plans) == 0 {
		return response, nil
	}

	for i, plan := range plans {
		response.Items[i] = &dto.PlanResponse{Plan: plan}
	}

	// Expand entitlements and prices if requested
	planIDs := lo.Map(plans, func(plan *plan.Plan, _ int) string {
		return plan.ID
	})

	// Create maps for storing expanded data
	pricesByPlanID := make(map[string][]*dto.PriceResponse)
	entitlementsByPlanID := make(map[string][]*dto.EntitlementResponse)
	creditGrantsByPlanID := make(map[string][]*dto.CreditGrantResponse)

	priceService := NewPriceService(s.PriceRepo, s.MeterRepo, s.Logger)
	entitlementService := NewEntitlementService(s.ServiceParams)

	// If prices or entitlements expansion is requested, fetch them in bulk
	// Fetch prices if requested
	if filter.GetExpand().Has(types.ExpandPrices) {
		priceFilter := types.NewNoLimitPriceFilter().
			WithPlanIDs(planIDs).
			WithStatus(types.StatusPublished)

		// If meters should be expanded, propagate the expansion to prices
		if filter.GetExpand().Has(types.ExpandMeters) {
			priceFilter = priceFilter.WithExpand(string(types.ExpandMeters))
		}

		prices, err := priceService.GetPrices(ctx, priceFilter)
		if err != nil {
			return nil, err
		}

		for _, p := range prices.Items {
			pricesByPlanID[p.PlanID] = append(pricesByPlanID[p.PlanID], p)
		}
	}

	// Fetch entitlements if requested
	if filter.GetExpand().Has(types.ExpandEntitlements) {
		entFilter := types.NewNoLimitEntitlementFilter().
			WithPlanIDs(planIDs).
			WithStatus(types.StatusPublished)

		// If features should be expanded, propagate the expansion to entitlements
		if filter.GetExpand().Has(types.ExpandFeatures) {
			entFilter = entFilter.WithExpand(string(types.ExpandFeatures))
		}

		entitlements, err := entitlementService.ListEntitlements(ctx, entFilter)
		if err != nil {
			return nil, err
		}

		for _, e := range entitlements.Items {
			entitlementsByPlanID[e.PlanID] = append(entitlementsByPlanID[e.PlanID], e)
		}
	}

	// Fetch credit grants if requested
	if filter.GetExpand().Has(types.ExpandCreditGrant) {

		for _, planID := range planIDs {
			creditGrants, err := s.CreditGrantRepo.GetByPlan(ctx, planID)
			if err != nil {
				return nil, err
			}

			for _, cg := range creditGrants {
				creditGrantsByPlanID[lo.FromPtr(cg.PlanID)] = append(creditGrantsByPlanID[lo.FromPtr(cg.PlanID)], &dto.CreditGrantResponse{CreditGrant: cg})
			}
		}
	}

	// Build response with expanded fields
	for i, plan := range plans {

		// Add prices if available
		if prices, ok := pricesByPlanID[plan.ID]; ok {
			response.Items[i].Prices = prices
		}

		// Add entitlements if available
		if entitlements, ok := entitlementsByPlanID[plan.ID]; ok {
			response.Items[i].Entitlements = entitlements
		}

		// Add credit grants if available
		if creditGrants, ok := creditGrantsByPlanID[plan.ID]; ok {
			response.Items[i].CreditGrants = creditGrants
		}
	}

	return response, nil
}

func (s *planService) UpdatePlan(ctx context.Context, id string, req dto.UpdatePlanRequest) (*dto.PlanResponse, error) {
	if id == "" {
		return nil, ierr.NewError("plan ID is required").
			WithHint("Plan ID is required").
			Mark(ierr.ErrValidation)
	}

	// Get the existing plan
	planResponse, err := s.GetPlan(ctx, id)
	if err != nil {
		return nil, err
	}

	plan := planResponse.Plan

	// Update plan fields if provided
	if req.Name != nil {
		plan.Name = *req.Name
	}
	if req.Description != nil {
		plan.Description = *req.Description
	}
	if req.LookupKey != nil {
		plan.LookupKey = *req.LookupKey
	}

	// Start a transaction for updating plan, prices, and entitlements
	err = s.DB.WithTx(ctx, func(ctx context.Context) error {
		// 1. Update the plan
		if err := s.PlanRepo.Update(ctx, plan); err != nil {
			return err
		}

		// 2. Handle prices
		if len(req.Prices) > 0 {
			// Create maps for tracking
			reqPriceMap := make(map[string]dto.UpdatePlanPriceRequest)
			for _, reqPrice := range req.Prices {
				if reqPrice.ID != "" {
					reqPriceMap[reqPrice.ID] = reqPrice
				}
			}

			// Track prices to delete
			pricesToDelete := make([]string, 0)

			// Handle existing prices
			for _, price := range planResponse.Prices {
				if reqPrice, ok := reqPriceMap[price.ID]; ok {
					// Update existing price
					price.Description = reqPrice.Description
					price.Metadata = reqPrice.Metadata
					price.LookupKey = reqPrice.LookupKey
					if err := s.PriceRepo.Update(ctx, price.Price); err != nil {
						return err
					}
				} else {
					// Delete price not in request
					pricesToDelete = append(pricesToDelete, price.ID)
				}
			}

			// Delete prices in bulk
			if len(pricesToDelete) > 0 {
				if err := s.PriceRepo.DeleteBulk(ctx, pricesToDelete); err != nil {
					return err
				}
			}

			// Create new prices
			newPrices := make([]*price.Price, 0)
			for _, reqPrice := range req.Prices {
				if reqPrice.ID == "" {
					newPrice, err := reqPrice.ToPrice(ctx)
					if err != nil {
						return ierr.WithError(err).
							WithHint("Failed to create price").
							Mark(ierr.ErrValidation)
					}
					newPrice.PlanID = plan.ID
					newPrices = append(newPrices, newPrice)
				}
			}

			if len(newPrices) > 0 {
				if err := s.PriceRepo.CreateBulk(ctx, newPrices); err != nil {
					return err
				}
			}
		}

		// 3. Handle entitlements
		if len(req.Entitlements) > 0 {
			// Create maps for tracking
			reqEntMap := make(map[string]dto.UpdatePlanEntitlementRequest)
			for _, reqEnt := range req.Entitlements {
				if reqEnt.ID != "" {
					reqEntMap[reqEnt.ID] = reqEnt
				}
			}

			// Track entitlements to delete
			entsToDelete := make([]string, 0)

			// Handle existing entitlements
			for _, ent := range planResponse.Entitlements {
				if reqEnt, ok := reqEntMap[ent.ID]; ok {
					// Update existing entitlement
					ent.IsEnabled = reqEnt.IsEnabled
					ent.UsageLimit = reqEnt.UsageLimit
					ent.UsageResetPeriod = reqEnt.UsageResetPeriod
					ent.IsSoftLimit = reqEnt.IsSoftLimit
					ent.StaticValue = reqEnt.StaticValue
					if _, err := s.EntitlementRepo.Update(ctx, ent.Entitlement); err != nil {
						return err
					}
				} else {
					// Delete entitlement not in request
					entsToDelete = append(entsToDelete, ent.ID)
				}
			}

			// Delete entitlements in bulk
			if len(entsToDelete) > 0 {
				if err := s.EntitlementRepo.DeleteBulk(ctx, entsToDelete); err != nil {
					return err
				}
			}

			// Create new entitlements
			newEntitlements := make([]*entitlement.Entitlement, 0)
			for _, reqEnt := range req.Entitlements {
				if reqEnt.ID == "" {
					ent := reqEnt.ToEntitlement(ctx, plan.ID)
					newEntitlements = append(newEntitlements, ent)
				}
			}

			if len(newEntitlements) > 0 {
				if _, err := s.EntitlementRepo.CreateBulk(ctx, newEntitlements); err != nil {
					return err
				}

				// Publish webhook events for each new entitlement
				for _, ent := range newEntitlements {
					s.publishEntitlementWebhookEvent(ctx, types.WebhookEventEntitlementCreated, ent.ID)
				}
			}
		}

		// 4. Handle credit grants
		if len(req.CreditGrants) > 0 {
			// Create maps for tracking
			reqCreditGrantMap := make(map[string]dto.UpdatePlanCreditGrantRequest)
			for _, reqCreditGrant := range req.CreditGrants {
				if reqCreditGrant.ID != "" {
					reqCreditGrantMap[reqCreditGrant.ID] = reqCreditGrant
				}
			}

			// Track credit grants to delete
			creditGrantsToDelete := make([]string, 0)

			// Handle existing credit grants
			for _, cg := range planResponse.CreditGrants {
				if reqCreditGrant, ok := reqCreditGrantMap[cg.ID]; ok {
					// Update existing credit grant using the UpdateCreditGrant method
					if reqCreditGrant.CreateCreditGrantRequest != nil {
						updateReq := dto.UpdateCreditGrantRequest{
							Name:     lo.ToPtr(reqCreditGrant.Name),
							Metadata: lo.ToPtr(reqCreditGrant.Metadata),
						}
						updateReq.UpdateCreditGrant(cg.CreditGrant, ctx)
					}
					if _, err := s.CreditGrantRepo.Update(ctx, cg.CreditGrant); err != nil {
						return err
					}
				} else {
					// Delete credit grant not in request
					creditGrantsToDelete = append(creditGrantsToDelete, cg.ID)
				}
			}

			// Delete credit grants in bulk
			if len(creditGrantsToDelete) > 0 {
				if err := s.CreditGrantRepo.DeleteBulk(ctx, creditGrantsToDelete); err != nil {
					return err
				}
			}

			// Create new credit grants
			newCreditGrants := make([]*creditgrant.CreditGrant, 0)
			for _, reqCreditGrant := range req.CreditGrants {
				if reqCreditGrant.ID == "" {
					// Use the embedded CreateCreditGrantRequest
					createReq := *reqCreditGrant.CreateCreditGrantRequest
					createReq.Scope = types.CreditGrantScopePlan
					createReq.PlanID = &plan.ID

					newCreditGrant := createReq.ToCreditGrant(ctx)
					// Clear subscription_id for plan-scoped credit grants
					newCreditGrant.SubscriptionID = nil
					newCreditGrants = append(newCreditGrants, newCreditGrant)
				}
			}

			if len(newCreditGrants) > 0 {
				if _, err := s.CreditGrantRepo.CreateBulk(ctx, newCreditGrants); err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.GetPlan(ctx, id)
}

func (s *planService) DeletePlan(ctx context.Context, id string) error {

	if id == "" {
		return ierr.NewError("plan ID is required").
			WithHint("Plan ID is required").
			Mark(ierr.ErrValidation)
	}

	// check if plan exists
	plan, err := s.PlanRepo.Get(ctx, id)
	if err != nil {
		return err
	}

	subscriptionFilters := types.NewDefaultQueryFilter()
	subscriptionFilters.Status = lo.ToPtr(types.StatusPublished)
	subscriptionFilters.Limit = lo.ToPtr(1)
	subscriptions, err := s.SubRepo.List(ctx, &types.SubscriptionFilter{
		QueryFilter:             subscriptionFilters,
		PlanID:                  id,
		SubscriptionStatusNotIn: []types.SubscriptionStatus{types.SubscriptionStatusCancelled},
	})
	if err != nil {
		return err
	}

	if len(subscriptions) > 0 {
		return ierr.NewError("plan is still associated with subscriptions").
			WithHint("Please remove the active subscriptions before deleting this plan.").
			WithReportableDetails(map[string]interface{}{
				"plan_id": id,
			}).
			Mark(ierr.ErrInvalidOperation)
	}

	err = s.PlanRepo.Delete(ctx, plan)
	if err != nil {
		return err
	}
	return nil
}

func (s *planService) SyncPlanPrices(ctx context.Context, id string) (*SyncPlanPricesResponse, error) {
	if id == "" {
		return nil, ierr.NewError("plan ID is required").
			WithHint("Plan ID is required").
			Mark(ierr.ErrValidation)
	}

	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	// Get the plan to be synced
	p, err := s.PlanRepo.Get(ctx, id)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get plan").
			Mark(ierr.ErrDatabase)
	}

	if p.TenantID != tenantID {
		return nil, ierr.NewError("plan does not belong to the specified tenant").
			WithHint("Plan does not belong to the specified tenant").
			WithReportableDetails(map[string]interface{}{
				"plan_id": id,
			}).
			Mark(ierr.ErrInvalidOperation)
	}

	s.Logger.Infow("Found plan", "plan_id", id, "plan_name", p.Name)

	// Get all prices for the plan
	priceFilter := types.NewNoLimitPriceFilter()
	priceFilter = priceFilter.WithStatus(types.StatusPublished)
	priceFilter = priceFilter.WithPlanIDs([]string{id})

	prices, err := s.PriceRepo.List(ctx, priceFilter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to list prices for plan").
			Mark(ierr.ErrDatabase)
	}

	// Filter prices for the plan and environment
	planPrices := make([]*price.Price, 0)
	meterMap := make(map[string]*meter.Meter)
	for _, price := range prices {
		if price.PlanID == id && price.TenantID == tenantID && price.EnvironmentID == environmentID {
			planPrices = append(planPrices, price)
			if price.MeterID != "" {
				meterMap[price.MeterID] = nil
			}
		}
	}

	meterFilter := types.NewNoLimitMeterFilter()
	meterFilter.MeterIDs = lo.Keys(meterMap)
	meters, err := s.MeterRepo.List(ctx, meterFilter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to list meters").
			Mark(ierr.ErrDatabase)
	}

	for _, meter := range meters {
		meterMap[meter.ID] = meter
	}

	if len(planPrices) == 0 {
		return nil, ierr.NewError("no active prices found for this plan").
			WithHint("No active prices found for this plan").
			WithReportableDetails(map[string]interface{}{
				"plan_id": id,
			}).
			Mark(ierr.ErrInvalidOperation)
	}

	s.Logger.Infow("Found prices for plan", "plan_id", id, "price_count", len(planPrices))

	// Set up filter for subscriptions
	subscriptionFilter := &types.SubscriptionFilter{}
	subscriptionFilter.PlanID = id
	subscriptionFilter.SubscriptionStatus = []types.SubscriptionStatus{
		types.SubscriptionStatusActive,
		types.SubscriptionStatusTrialing,
	}

	// Get all active subscriptions for this plan
	subs, err := s.SubRepo.ListAll(ctx, subscriptionFilter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to list subscriptions").
			Mark(ierr.ErrDatabase)
	}

	s.Logger.Infow("Found active subscriptions using plan", "plan_id", id, "subscription_count", len(subs))

	totalAdded := 0
	totalRemoved := 0
	totalSkipped := 0

	// Get line item repository from ent package
	lineItemRepo := ent.NewSubscriptionLineItemRepository(s.DB)

	// Iterate through each subscription
	for _, sub := range subs {
		time.Sleep(100 * time.Millisecond)
		if sub.TenantID != tenantID || sub.EnvironmentID != environmentID {
			s.Logger.Infow("Skipping subscription - not in the specified tenant/environment", "subscription_id", sub.ID)
			continue
		}

		// filter the eligible price ids for this subscription by currency and period
		eligiblePriceList := make([]*price.Price, 0, len(planPrices))
		for _, p := range planPrices {
			if types.IsMatchingCurrency(p.Currency, sub.Currency) &&
				p.BillingPeriod == sub.BillingPeriod &&
				p.BillingPeriodCount == sub.BillingPeriodCount {
				eligiblePriceList = append(eligiblePriceList, p)
			}
		}

		// Get existing line items for the subscription
		lineItems, err := lineItemRepo.ListBySubscription(ctx, sub.ID)
		if err != nil {
			s.Logger.Infow("Failed to get line items for subscription", "subscription_id", sub.ID, "error", err)
			continue
		}

		// Create maps for fast lookups
		existingPriceIDs := make(map[string]*subscription.SubscriptionLineItem)
		for _, item := range lineItems {
			if item.PlanID == id && item.Status == types.StatusPublished {
				existingPriceIDs[item.PriceID] = item
			}
		}

		addedCount := 0
		removedCount := 0
		skippedCount := 0

		// Map to track which prices we've processed
		processedPrices := make(map[string]bool)

		// Add missing prices from the plan
		for _, pr := range eligiblePriceList {
			processedPrices[pr.ID] = true

			// Check if the subscription already has this price
			_, exists := existingPriceIDs[pr.ID]
			if exists {
				skippedCount++
				continue
			}

			// Create a new line item for the subscription
			newLineItem := &subscription.SubscriptionLineItem{
				ID:              types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM),
				SubscriptionID:  sub.ID,
				CustomerID:      sub.CustomerID,
				PlanID:          id,
				PlanDisplayName: p.Name,
				PriceID:         pr.ID,
				PriceType:       pr.Type,
				MeterID:         pr.MeterID,
				Currency:        pr.Currency,
				BillingPeriod:   pr.BillingPeriod,
				InvoiceCadence:  pr.InvoiceCadence,
				TrialPeriod:     pr.TrialPeriod,
				StartDate:       sub.StartDate, // Use subscription's start date
				Metadata:        map[string]string{"added_by": "plan_sync_api"},
				EnvironmentID:   environmentID,
				BaseModel: types.BaseModel{
					TenantID:  tenantID,
					Status:    types.StatusPublished,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					CreatedBy: types.DefaultUserID,
					UpdatedBy: types.DefaultUserID,
				},
			}

			if pr.Type == types.PRICE_TYPE_USAGE && pr.MeterID != "" {
				newLineItem.MeterID = pr.MeterID
				newLineItem.MeterDisplayName = meterMap[pr.MeterID].Name
				newLineItem.DisplayName = meterMap[pr.MeterID].Name
				newLineItem.Quantity = decimal.Zero
			} else {
				newLineItem.DisplayName = p.Name
				newLineItem.Quantity = decimal.NewFromInt(1)
			}

			err = lineItemRepo.Create(ctx, newLineItem)
			if err != nil {
				s.Logger.Infow("Failed to create line item for subscription", "subscription_id", sub.ID, "error", err)
				continue
			}

			s.Logger.Infow("Added price to subscription", "subscription_id", sub.ID, "price_id", pr.ID)
			addedCount++
		}

		// Remove prices that are no longer in the plan
		for priceID, item := range existingPriceIDs {
			if !processedPrices[priceID] {
				// Mark the line item as deleted
				item.Status = types.StatusDeleted
				item.UpdatedAt = time.Now()

				err = lineItemRepo.Update(ctx, item)
				if err != nil {
					s.Logger.Infow("Failed to delete line item for subscription", "subscription_id", sub.ID, "error", err)
					continue
				}

				s.Logger.Infow("Removed price from subscription", "subscription_id", sub.ID, "price_id", priceID)
				removedCount++
			}
		}

		s.Logger.Infow("Subscription", "subscription_id", sub.ID, "added_count", addedCount, "removed_count", removedCount, "skipped_count", skippedCount)

		totalAdded += addedCount
		totalRemoved += removedCount
		totalSkipped += skippedCount
	}

	// Update response with final statistics
	response := &SyncPlanPricesResponse{
		Message:  "Plan prices synchronized successfully",
		PlanID:   id,
		PlanName: p.Name,
		SynchronizationSummary: struct {
			SubscriptionsProcessed int `json:"subscriptions_processed"`
			PricesAdded            int `json:"prices_added"`
			PricesRemoved          int `json:"prices_removed"`
			PricesSkipped          int `json:"prices_skipped"`
		}{
			SubscriptionsProcessed: len(subs),
			PricesAdded:            totalAdded,
			PricesRemoved:          totalRemoved,
			PricesSkipped:          totalSkipped,
		},
	}

	s.Logger.Infow("Plan sync completed", "total_added", totalAdded, "total_removed", totalRemoved, "total_skipped", totalSkipped)

	return response, nil
}

func (s *planService) publishEntitlementWebhookEvent(ctx context.Context, eventName string, entitlementID string) {
	webhookPayload, err := json.Marshal(webhookDto.InternalEntitlementEvent{
		EntitlementID: entitlementID,
		TenantID:      types.GetTenantID(ctx),
	})
	if err != nil {
		s.Logger.Errorw("failed to marshal webhook payload", "error", err)
		return
	}

	webhookEvent := &types.WebhookEvent{
		ID:            types.GenerateUUIDWithPrefix(types.UUID_PREFIX_WEBHOOK_EVENT),
		EventName:     eventName,
		TenantID:      types.GetTenantID(ctx),
		EnvironmentID: types.GetEnvironmentID(ctx),
		UserID:        types.GetUserID(ctx),
		Timestamp:     time.Now().UTC(),
		Payload:       json.RawMessage(webhookPayload),
	}
	if err := s.WebhookPublisher.PublishWebhook(ctx, webhookEvent); err != nil {
		s.Logger.Errorf("failed to publish %s event: %v", webhookEvent.EventName, err)
	}
}
