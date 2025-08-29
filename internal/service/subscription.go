package service

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/addonassociation"
	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	webhookDto "github.com/flexprice/flexprice/internal/webhook/dto"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

type SubscriptionService interface {
	CreateSubscription(ctx context.Context, req dto.CreateSubscriptionRequest) (*dto.SubscriptionResponse, error)
	GetSubscription(ctx context.Context, id string) (*dto.SubscriptionResponse, error)
	CancelSubscription(ctx context.Context, id string, cancelAtPeriodEnd bool) error
	ActivateIncompleteSubscription(ctx context.Context, subscriptionID string) error
	ListSubscriptions(ctx context.Context, filter *types.SubscriptionFilter) (*dto.ListSubscriptionsResponse, error)
	GetUsageBySubscription(ctx context.Context, req *dto.GetUsageBySubscriptionRequest) (*dto.GetUsageBySubscriptionResponse, error)
	UpdateBillingPeriods(ctx context.Context) (*dto.SubscriptionUpdatePeriodResponse, error)

	// Pause-related methods
	PauseSubscription(ctx context.Context, subscriptionID string, req *dto.PauseSubscriptionRequest) (*dto.PauseSubscriptionResponse, error)
	ResumeSubscription(ctx context.Context, subscriptionID string, req *dto.ResumeSubscriptionRequest) (*dto.ResumeSubscriptionResponse, error)
	GetPause(ctx context.Context, pauseID string) (*subscription.SubscriptionPause, error)
	ListPauses(ctx context.Context, subscriptionID string) (*dto.ListSubscriptionPausesResponse, error)
	CalculatePauseImpact(ctx context.Context, subscriptionID string, req *dto.PauseSubscriptionRequest) (*types.BillingImpactDetails, error)
	CalculateResumeImpact(ctx context.Context, subscriptionID string, req *dto.ResumeSubscriptionRequest) (*types.BillingImpactDetails, error)

	// Schedule-related methods
	CreateSubscriptionSchedule(ctx context.Context, req *dto.CreateSubscriptionScheduleRequest) (*dto.SubscriptionScheduleResponse, error)
	GetSubscriptionSchedule(ctx context.Context, id string) (*dto.SubscriptionScheduleResponse, error)
	GetScheduleBySubscriptionID(ctx context.Context, subscriptionID string) (*dto.SubscriptionScheduleResponse, error)
	UpdateSubscriptionSchedule(ctx context.Context, id string, req *dto.UpdateSubscriptionScheduleRequest) (*dto.SubscriptionScheduleResponse, error)
	AddSchedulePhase(ctx context.Context, scheduleID string, req *dto.AddSchedulePhaseRequest) (*dto.SubscriptionScheduleResponse, error)
	AddSubscriptionPhase(ctx context.Context, subscriptionID string, req *dto.AddSchedulePhaseRequest) (*dto.SubscriptionScheduleResponse, error)

	// Coupon-related methods
	ApplyCouponsToSubscriptionWithLineItems(ctx context.Context, subscriptionID string, subscriptionCoupons []string, lineItemCoupons map[string][]string, lineItems []*subscription.SubscriptionLineItem) error

	ValidateAndFilterPricesForSubscription(ctx context.Context, entityID string, entityType types.PriceEntityType, subscription *subscription.Subscription) ([]*dto.PriceResponse, error)

	// Addon management for subscriptions
	AddAddonToSubscription(ctx context.Context, subscriptionID string, req *dto.AddAddonToSubscriptionRequest) (*addonassociation.AddonAssociation, error)
	RemoveAddonFromSubscription(ctx context.Context, subscriptionID string, addonID string, reason string) error

	// Auto-cancellation methods
	ProcessAutoCancellationSubscriptions(ctx context.Context) error
}

type subscriptionService struct {
	ServiceParams
}

func NewSubscriptionService(params ServiceParams) SubscriptionService {
	return &subscriptionService{
		ServiceParams: params,
	}
}

func (s *subscriptionService) CreateSubscription(ctx context.Context, req dto.CreateSubscriptionRequest) (*dto.SubscriptionResponse, error) {
	// Handle default values
	if req.BillingCycle == "" {
		req.BillingCycle = types.BillingCycleAnniversary
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Get customer based on the provided IDs
	var customer *customer.Customer
	var err error

	// Case- CustomerID is present - use it directly (ignore ExternalCustomerID if present)
	if req.CustomerID != "" {
		customer, err = s.CustomerRepo.Get(ctx, req.CustomerID)
		if err != nil {
			return nil, err
		}
	} else {
		// Case- Only ExternalCustomerID is present
		customer, err = s.CustomerRepo.GetByLookupKey(ctx, req.ExternalCustomerID)
		if err != nil {
			return nil, err
		}
		// Set the CustomerID from the found customer
		req.CustomerID = customer.ID
	}

	if customer.Status != types.StatusPublished {
		return nil, ierr.NewError("customer is not active").
			WithHint("The customer must be active to create a subscription").
			WithReportableDetails(map[string]interface{}{
				"customer_id": req.CustomerID,
				"status":      customer.Status,
			}).
			Mark(ierr.ErrValidation)
	}

	plan, err := s.PlanRepo.Get(ctx, req.PlanID)
	if err != nil {
		return nil, err
	}

	if plan.Status != types.StatusPublished {
		return nil, ierr.NewError("plan is not active").
			WithHint("The plan must be active to create a subscription").
			WithReportableDetails(map[string]interface{}{
				"plan_id": req.PlanID,
				"status":  plan.Status,
			}).
			Mark(ierr.ErrValidation)
	}

	sub := req.ToSubscription(ctx)

	// Validate and filter prices for the plan using the reusable method
	validPrices, err := s.ValidateAndFilterPricesForSubscription(
		ctx,
		plan.ID,
		types.PRICE_ENTITY_TYPE_PLAN,
		sub,
	)
	if err != nil {
		return nil, err
	}

	// Create price map for line item creation
	priceMap := make(map[string]*dto.PriceResponse, len(validPrices))
	for _, p := range validPrices {
		priceMap[p.Price.ID] = p
	}

	// Ensure start date is in UTC format
	// Note: StartDate is now guaranteed to be set (either from request or defaulted in DTO validation)
	sub.StartDate = sub.StartDate.UTC()
	now := time.Now().UTC()

	// Set start date and ensure it's in UTC
	// TODO: handle when start date is in the past and there are
	// multiple billing periods in the past so in this case we need to keep
	// the current period start as now only and handle past periods in proration
	if sub.StartDate.IsZero() {
		sub.StartDate = now
	} else {
		sub.StartDate = sub.StartDate.UTC()
	}

	if sub.BillingCycle == types.BillingCycleCalendar {
		sub.BillingAnchor = types.CalculateCalendarBillingAnchor(sub.StartDate, sub.BillingPeriod)
	} else {
		// default to start date for anniversary billing
		sub.BillingAnchor = sub.StartDate
	}

	if sub.BillingPeriodCount == 0 {
		sub.BillingPeriodCount = 1
	}

	// Calculate the first billing period end date
	nextBillingDate, err := types.NextBillingDate(sub.StartDate, sub.BillingAnchor, sub.BillingPeriodCount, sub.BillingPeriod, sub.EndDate)
	if err != nil {
		return nil, err
	}

	sub.CurrentPeriodStart = sub.StartDate
	sub.CurrentPeriodEnd = nextBillingDate

	// Convert line items
	lineItems := make([]*subscription.SubscriptionLineItem, 0, len(validPrices))
	for _, price := range validPrices {
		lineItems = append(lineItems, &subscription.SubscriptionLineItem{
			PriceID:       price.ID,
			EnvironmentID: types.GetEnvironmentID(ctx),
			BaseModel:     types.GetDefaultBaseModel(ctx),
		})
	}

	// Convert line items
	for _, item := range lineItems {
		price, ok := priceMap[item.PriceID]
		if !ok {
			return nil, ierr.NewError("failed to get price %s: price not found").
				WithHint("Ensure all prices are valid and available").
				WithReportableDetails(map[string]interface{}{
					"price_id": item.PriceID,
				}).
				Mark(ierr.ErrDatabase)
		}

		if price.Type == types.PRICE_TYPE_USAGE && price.Meter != nil {
			item.MeterID = price.Meter.ID
			item.MeterDisplayName = price.Meter.Name
			item.DisplayName = price.Meter.Name
			item.Quantity = decimal.Zero
		} else {
			item.DisplayName = plan.Name
			if item.Quantity.IsZero() {
				item.Quantity = decimal.NewFromInt(1)
			}
		}

		if item.ID == "" {
			item.ID = types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM)
		}

		item.SubscriptionID = sub.ID
		item.PriceType = price.Type
		item.EntityID = plan.ID
		item.EntityType = types.SubscriptionLineItemEntitiyTypePlan
		item.PlanDisplayName = plan.Name
		item.CustomerID = sub.CustomerID
		item.Currency = sub.Currency
		item.BillingPeriod = sub.BillingPeriod
		item.InvoiceCadence = price.InvoiceCadence
		item.TrialPeriod = price.TrialPeriod
		item.PriceUnitID = price.PriceUnitID
		item.PriceUnit = price.PriceUnit
		item.StartDate = sub.StartDate
		if sub.EndDate != nil {
			item.EndDate = *sub.EndDate
		}
	}

	// Process price overrides if provided
	if len(req.OverrideLineItems) > 0 {
		err = s.ProcessSubscriptionPriceOverrides(ctx, sub, req.OverrideLineItems, lineItems, priceMap)
		if err != nil {
			return nil, err
		}
	}

	sub.LineItems = lineItems

	s.Logger.Infow("creating subscription",
		"customer_id", sub.CustomerID,
		"plan_id", sub.PlanID,
		"start_date", sub.StartDate,
		"billing_anchor", sub.BillingAnchor,
		"current_period_start", sub.CurrentPeriodStart,
		"current_period_end", sub.CurrentPeriodEnd,
		"valid_prices", len(validPrices),
		"num_line_items", len(sub.LineItems),
		"has_phases", len(req.Phases) > 0)

	// Create response object
	response := &dto.SubscriptionResponse{Subscription: sub}

	invoiceService := NewInvoiceService(s.ServiceParams)
	var invoice *dto.InvoiceResponse
	err = s.DB.WithTx(ctx, func(ctx context.Context) error {
		// Create subscription with line items
		err = s.SubRepo.CreateWithLineItems(ctx, sub, sub.LineItems)
		if err != nil {
			return err
		}

		// Handle addons if provided
		if len(req.Addons) > 0 {
			err = s.handleSubscriptionAddons(ctx, sub, req.Addons)
			if err != nil {
				return err
			}
		}

		creditGrantRequests := make([]dto.CreateCreditGrantRequest, 0)

		// check if user has overidden the plan credit grants, if so add them to the request
		if req.CreditGrants != nil {
			creditGrantRequests = append(creditGrantRequests, req.CreditGrants...)
		} else {
			// if user has not overidden the plan credit grants, add the plan credit grants to the request
			creditGrantService := NewCreditGrantService(s.ServiceParams)
			planCreditGrants, err := creditGrantService.GetCreditGrantsByPlan(ctx, plan.ID)
			if err != nil {
				return err
			}

			s.Logger.Infow("plan has credit grants", "plan_id", plan.ID, "credit_grants_count", len(planCreditGrants.Items))
			// if plan has credit grants, add them to the request
			if len(planCreditGrants.Items) > 0 {
				for _, cg := range planCreditGrants.Items {
					creditGrantRequests = append(creditGrantRequests, dto.CreateCreditGrantRequest{
						Name:                   cg.Name,
						Scope:                  types.CreditGrantScopeSubscription,
						Credits:                cg.Credits,
						Cadence:                cg.Cadence,
						ExpirationType:         cg.ExpirationType,
						Priority:               cg.Priority,
						SubscriptionID:         lo.ToPtr(sub.ID),
						Period:                 cg.Period,
						PlanID:                 &plan.ID,
						ExpirationDuration:     cg.ExpirationDuration,
						ExpirationDurationUnit: cg.ExpirationDurationUnit,
						Metadata:               cg.Metadata,
						PeriodCount:            cg.PeriodCount,
					})
				}
			}
		}

		// handle credit grants
		err = s.handleCreditGrants(ctx, sub, creditGrantRequests)
		if err != nil {
			return err
		}

		// Create subscription schedule if phases are provided
		if len(req.Phases) > 0 {
			schedule, err := s.createScheduleFromPhases(ctx, sub, req.Phases)
			if err != nil {
				return err
			}

			// Include the schedule in the response
			if schedule != nil {
				response.Schedule = dto.SubscriptionScheduleResponseFromDomain(schedule)
			}
		}

		// handle tax rate linking
		err = s.handleTaxRateLinking(ctx, sub, req)
		if err != nil {
			return err
		}
		// Apply coupons to the subscription
		if err := s.ApplyCouponsToSubscriptionWithLineItems(ctx, sub.ID, req.Coupons, req.LineItemCoupons, lineItems); err != nil {
			return err
		}

		// Create invoice for the subscription (in case it has advance charges)
		invoice, err = invoiceService.CreateSubscriptionInvoice(ctx, &dto.CreateSubscriptionInvoiceRequest{
			SubscriptionID: sub.ID,
			PeriodStart:    sub.CurrentPeriodStart,
			PeriodEnd:      sub.CurrentPeriodEnd,
			ReferencePoint: types.ReferencePointPeriodStart,
		})

		return err
	})
	if err != nil {
		return nil, err
	}

	// if the subscription is created with incomplete status, but it doesn't create an invoice, we need to mark it as active
	// This applies regardless of collection method - if there's no invoice to pay, the subscription should be active
	if sub.SubscriptionStatus == types.SubscriptionStatusIncomplete && (invoice == nil || invoice.PaymentStatus == types.PaymentStatusSucceeded) {
		sub.SubscriptionStatus = types.SubscriptionStatusActive
		err = s.SubRepo.Update(ctx, sub)
		if err != nil {
			return nil, err
		}
	}

	// Update response to ensure it has the latest subscription data
	response.Subscription = sub

	s.publishInternalWebhookEvent(ctx, types.WebhookEventSubscriptionCreated, sub.ID)
	return response, nil
}

func (s *subscriptionService) handleTaxRateLinking(ctx context.Context, sub *subscription.Subscription, req dto.CreateSubscriptionRequest) error {
	taxService := NewTaxService(s.ServiceParams)

	// if tax overrides are provided, link them to the subscription
	if len(req.TaxRateOverrides) > 0 {
		err := taxService.LinkTaxRatesToEntity(ctx, dto.LinkTaxRateToEntityRequest{
			EntityType:       types.TaxRateEntityTypeSubscription,
			EntityID:         sub.ID,
			TaxRateOverrides: req.TaxRateOverrides,
		})
		if err != nil {
			return err
		}
	}

	// If no tax rate overrides are provided, link the customer's tax association to the subscription
	if req.TaxRateOverrides == nil {
		filter := types.NewNoLimitTaxAssociationFilter()
		filter.EntityType = types.TaxRateEntityTypeCustomer
		filter.EntityID = sub.CustomerID
		filter.AutoApply = lo.ToPtr(true)
		tenantTaxAssociations, err := taxService.ListTaxAssociations(ctx, filter)
		if err != nil {
			return err
		}

		err = taxService.LinkTaxRatesToEntity(ctx, dto.LinkTaxRateToEntityRequest{
			EntityType:              types.TaxRateEntityTypeSubscription,
			EntityID:                sub.ID,
			ExistingTaxAssociations: tenantTaxAssociations.Items,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// processSubscriptionPriceOverrides handles creating subscription-scoped prices for overrides
func (s *subscriptionService) ProcessSubscriptionPriceOverrides(
	ctx context.Context,
	sub *subscription.Subscription,
	overrideRequests []dto.OverrideLineItemRequest,
	lineItems []*subscription.SubscriptionLineItem,
	priceMap map[string]*dto.PriceResponse,
) error {
	if len(overrideRequests) == 0 {
		return nil
	}

	s.Logger.Infow("processing price overrides for subscription",
		"subscription_id", sub.ID,
		"override_count", len(overrideRequests))

	// Create a map from price ID to line item for quick lookup
	lineItemsByPriceID := make(map[string]*subscription.SubscriptionLineItem)
	for _, item := range lineItems {
		lineItemsByPriceID[item.PriceID] = item
	}

	// Process each override request
	for _, override := range overrideRequests {
		// Validate that the price exists in the plan
		originalPrice, exists := priceMap[override.PriceID]
		if !exists {
			return ierr.NewError("price not found in plan").
				WithHint("Override price must be a valid price from the selected plan").
				WithReportableDetails(map[string]interface{}{
					"price_id": override.PriceID,
					"plan_id":  sub.PlanID,
				}).
				Mark(ierr.ErrValidation)
		}

		// Find the corresponding line item
		lineItem, exists := lineItemsByPriceID[override.PriceID]
		if !exists {
			return ierr.NewError("line item not found for price").
				WithHint("Could not find line item for the specified price").
				WithReportableDetails(map[string]interface{}{
					"price_id": override.PriceID,
				}).
				Mark(ierr.ErrInternal)
		}

		priceUnitType := originalPrice.PriceUnitType
		if priceUnitType == "" {
			priceUnitType = types.PRICE_UNIT_TYPE_FIAT
		}

		// Create subscription-scoped price clone
		overriddenPrice := &price.Price{
			ID:                     types.GenerateUUIDWithPrefix(types.UUID_PREFIX_PRICE),
			Amount:                 originalPrice.Amount,
			Currency:               originalPrice.Currency,
			DisplayAmount:          originalPrice.DisplayAmount,
			Type:                   originalPrice.Type,
			BillingPeriod:          originalPrice.BillingPeriod,
			BillingPeriodCount:     originalPrice.BillingPeriodCount,
			BillingModel:           originalPrice.BillingModel,
			BillingCadence:         originalPrice.BillingCadence,
			InvoiceCadence:         originalPrice.InvoiceCadence,
			TrialPeriod:            originalPrice.TrialPeriod,
			TierMode:               originalPrice.TierMode,
			Tiers:                  originalPrice.Tiers,
			MeterID:                originalPrice.MeterID,
			LookupKey:              "",
			Description:            originalPrice.Description,
			PriceUnitID:            originalPrice.PriceUnitID,
			PriceUnit:              originalPrice.PriceUnit,
			PriceUnitType:          priceUnitType,
			ConversionRate:         originalPrice.ConversionRate,
			DisplayPriceUnitAmount: originalPrice.DisplayPriceUnitAmount,
			PriceUnitAmount:        originalPrice.PriceUnitAmount,
			PriceUnitTiers:         originalPrice.PriceUnitTiers,
			TransformQuantity:      originalPrice.TransformQuantity,
			Metadata:               originalPrice.Metadata,
			EnvironmentID:          originalPrice.EnvironmentID,
			EntityType:             types.PRICE_ENTITY_TYPE_SUBSCRIPTION,
			ParentPriceID:          originalPrice.ID,
			EntityID:               sub.ID,
			BaseModel:              types.GetDefaultBaseModel(ctx),
		}

		// Apply overrides
		if override.Amount != nil {
			overriddenPrice.Amount = *override.Amount
			overriddenPrice.DisplayAmount = overriddenPrice.GetDisplayAmount()
		}

		// Update line item quantity if specified
		if override.Quantity != nil {
			lineItem.Quantity = *override.Quantity
		}

		// Validate the overridden price
		if err := overriddenPrice.Validate(); err != nil {
			return ierr.WithError(err).
				WithHint("Failed to validate overridden price").
				WithReportableDetails(map[string]interface{}{
					"original_price_id": override.PriceID,
					"override_price_id": overriddenPrice.ID,
				}).
				Mark(ierr.ErrValidation)
		}

		// Create the subscription-scoped price
		if err := s.PriceRepo.Create(ctx, overriddenPrice); err != nil {
			return ierr.WithError(err).
				WithHint("Failed to create subscription-scoped price").
				WithReportableDetails(map[string]interface{}{
					"original_price_id": override.PriceID,
					"override_price_id": overriddenPrice.ID,
				}).
				Mark(ierr.ErrDatabase)
		}

		// Update the line item to reference the new subscription-scoped price
		lineItem.PriceID = overriddenPrice.ID

		s.Logger.Infow("created subscription-scoped price override",
			"subscription_id", sub.ID,
			"original_price_id", override.PriceID,
			"override_price_id", overriddenPrice.ID,
			"amount_override", override.Amount != nil,
			"quantity_override", override.Quantity != nil)
	}

	return nil
}

// handleCreditGrants handles creating and applying credit grants for a subscription
func (s *subscriptionService) handleCreditGrants(
	ctx context.Context,
	subscription *subscription.Subscription,
	creditGrantRequests []dto.CreateCreditGrantRequest,
) error {
	if len(creditGrantRequests) == 0 {
		return nil
	}

	creditGrantService := NewCreditGrantService(s.ServiceParams)

	s.Logger.Infow("processing credit grants for subscription",
		"subscription_id", subscription.ID,
		"credit_grants_count", len(creditGrantRequests))

	// Create and apply credit grants
	for _, grantReq := range creditGrantRequests {
		// Ensure subscription ID is set and scope is SUBSCRIPTION
		grantReq.SubscriptionID = &subscription.ID
		grantReq.Scope = types.CreditGrantScopeSubscription

		// Create credit grant in DB
		createdGrant, err := creditGrantService.CreateCreditGrant(ctx, grantReq)
		if err != nil {
			return ierr.WithError(err).
				WithHint("Failed to create credit grant for subscription").
				WithReportableDetails(map[string]interface{}{
					"subscription_id": subscription.ID,
					"grant_name":      grantReq.Name,
				}).
				Mark(ierr.ErrDatabase)
		}

		// Apply the credit grant using the new simplified method
		metadata := types.Metadata{
			"created_during": "subscription_creation",
			"grant_name":     createdGrant.Name,
		}

		err = creditGrantService.ApplyCreditGrant(
			ctx,
			createdGrant.CreditGrant,
			subscription,
			metadata,
		)

		if err != nil {
			return ierr.WithError(err).
				WithHint("Failed to apply credit grant for subscription").
				WithReportableDetails(map[string]interface{}{
					"subscription_id": subscription.ID,
					"grant_id":        createdGrant.ID,
					"grant_name":      createdGrant.Name,
				}).
				Mark(ierr.ErrDatabase)
		}

		s.Logger.Infow("successfully processed credit grant for subscription",
			"subscription_id", subscription.ID,
			"grant_id", createdGrant.ID,
			"grant_name", createdGrant.Name,
			"amount", createdGrant.Credits)
	}

	return nil
}

func (s *subscriptionService) GetSubscription(ctx context.Context, id string) (*dto.SubscriptionResponse, error) {
	// Get subscription with line items
	subscription, _, err := s.SubRepo.GetWithLineItems(ctx, id)
	if err != nil {
		return nil, err
	}

	response := &dto.SubscriptionResponse{Subscription: subscription}

	// if subscription pause status is not none, get all pauses
	if subscription.PauseStatus != types.PauseStatusNone {
		pauses, err := s.SubRepo.ListPauses(ctx, id)
		if err != nil {
			return nil, err
		}
		response.Pauses = pauses
	}

	// expand plan
	planService := NewPlanService(s.ServiceParams)

	plan, err := planService.GetPlan(ctx, subscription.PlanID)
	if err != nil {
		return nil, err
	}
	response.Plan = plan

	// expand customer
	customerService := NewCustomerService(s.ServiceParams)
	customer, err := customerService.GetCustomer(ctx, subscription.CustomerID)
	if err != nil {
		return nil, err
	}
	response.Customer = customer

	// Try to get schedule if exists
	schedule, err := s.GetScheduleBySubscriptionID(ctx, id)
	if err == nil && schedule != nil {
		response.Schedule = schedule
	}

	// expand coupon associations
	couponAssociationService := NewCouponAssociationService(s.ServiceParams)
	couponAssociations, err := couponAssociationService.GetCouponAssociationsBySubscription(ctx, id)
	if err != nil {
		s.Logger.Errorw("failed to get coupon associations for subscription",
			"subscription_id", id,
			"error", err)
	} else {
		response.CouponAssociations = couponAssociations
	}

	return response, nil
}

func (s *subscriptionService) CancelSubscription(ctx context.Context, id string, cancelAtPeriodEnd bool) error {
	subscription, _, err := s.SubRepo.GetWithLineItems(ctx, id)
	if err != nil {
		return err
	}

	if subscription.SubscriptionStatus == types.SubscriptionStatusCancelled {
		return ierr.NewError("subscription is already cancelled").
			WithHint("The subscription is already cancelled").
			WithReportableDetails(map[string]interface{}{
				"subscription_id": id,
			}).
			Mark(ierr.ErrValidation)
	}

	now := time.Now().UTC()
	subscription.CancelledAt = &now
	if cancelAtPeriodEnd {
		subscription.CancelAtPeriodEnd = cancelAtPeriodEnd
		subscription.CancelAt = lo.ToPtr(subscription.CurrentPeriodEnd)
	} else {
		subscription.SubscriptionStatus = types.SubscriptionStatusCancelled
		subscription.CancelAt = nil
	}

	err = s.DB.WithTx(ctx, func(ctx context.Context) error {

		// cancel future credit grant applications
		creditGrantService := NewCreditGrantService(s.ServiceParams)
		err = creditGrantService.CancelFutureCreditGrantsOfSubscription(ctx, subscription.ID)
		if err != nil {
			return err
		}

		err = s.SubRepo.Update(ctx, subscription)
		if err != nil {
			return err
		}

		return nil
	})

	// Publish webhook event
	s.publishInternalWebhookEvent(ctx, types.WebhookEventSubscriptionUpdated, subscription.ID)
	s.publishInternalWebhookEvent(ctx, types.WebhookEventSubscriptionCancelled, subscription.ID)

	return nil
}

func (s *subscriptionService) ListSubscriptions(ctx context.Context, filter *types.SubscriptionFilter) (*dto.ListSubscriptionsResponse, error) {
	planService := NewPlanService(s.ServiceParams)

	if filter == nil {
		filter = types.NewSubscriptionFilter()
	}

	if filter.GetLimit() == 0 {
		filter.Limit = lo.ToPtr(types.GetDefaultFilter().Limit)
	}

	if filter.QueryFilter == nil {
		filter.QueryFilter = types.NewDefaultQueryFilter()
	}

	subscriptions, err := s.SubRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	count, err := s.SubRepo.Count(ctx, filter)
	if err != nil {
		return nil, err
	}

	response := &dto.ListSubscriptionsResponse{
		Items: make([]*dto.SubscriptionResponse, len(subscriptions)),
		Pagination: types.NewPaginationResponse(
			count,
			filter.GetLimit(),
			filter.GetOffset(),
		),
	}

	// Collect unique plan IDs
	planIDMap := make(map[string]*dto.PlanResponse, 0)
	for _, sub := range subscriptions {
		planIDMap[sub.PlanID] = nil
	}

	// Get plans in bulk
	planFilter := types.NewNoLimitPlanFilter()
	planFilter.EntityIDs = lo.Keys(planIDMap)
	if filter != nil && filter.Expand != nil {
		planFilter.Expand = filter.Expand // pass on the filters to next layer
	}
	planResponse, err := planService.GetPlans(ctx, planFilter)
	if err != nil {
		return nil, err
	}

	// Build plan map for quick lookup
	for _, plan := range planResponse.Items {
		planIDMap[plan.Plan.ID] = plan
	}

	// Build response with plans
	for i, sub := range subscriptions {
		response.Items[i] = &dto.SubscriptionResponse{
			Subscription: sub,
			Plan:         planIDMap[sub.PlanID],
		}
	}

	// Include schedules if requested in expand
	if filter.Expand != nil && filter.GetExpand().Has(types.ExpandSchedule) {
		s.addSchedulesToSubscriptionResponses(ctx, response.Items)
	}

	return response, nil
}

// addSchedulesToSubscriptionResponses adds schedule information to subscription responses if available
func (s *subscriptionService) addSchedulesToSubscriptionResponses(ctx context.Context, items []*dto.SubscriptionResponse) {
	// If repository doesn't support schedules, return early
	if s.SubscriptionScheduleRepo == nil {
		s.Logger.Debugw("subscription schedule repository is not configured, skipping schedule expansion")
		return
	}

	// Group subscriptions by ID for faster lookup
	subMap := make(map[string]*dto.SubscriptionResponse, len(items))
	for _, sub := range items {
		subMap[sub.ID] = sub
	}

	// Collect all subscription IDs
	subscriptionIDs := lo.Keys(subMap)

	// In a real implementation, we would get schedules in a single query
	// For now, we'll do individual lookups
	for _, subscriptionID := range subscriptionIDs {
		sub := subMap[subscriptionID]

		// Try to get schedule if exists
		schedule, err := s.SubscriptionScheduleRepo.GetBySubscriptionID(ctx, subscriptionID)
		if err != nil || schedule == nil {
			continue
		}

		// Add schedule to subscription response
		sub.Schedule = dto.SubscriptionScheduleResponseFromDomain(schedule)
	}
}

func (s *subscriptionService) GetUsageBySubscription(ctx context.Context, req *dto.GetUsageBySubscriptionRequest) (*dto.GetUsageBySubscriptionResponse, error) {
	response := &dto.GetUsageBySubscriptionResponse{}

	eventService := NewEventService(s.EventRepo, s.MeterRepo, s.EventPublisher, s.Logger, s.Config)
	priceService := NewPriceService(s.ServiceParams)

	// Get subscription with line items
	subscription, lineItems, err := s.SubRepo.GetWithLineItems(ctx, req.SubscriptionID)
	if err != nil {
		return nil, err
	}

	// Get customer
	customer, err := s.CustomerRepo.Get(ctx, subscription.CustomerID)
	if err != nil {
		return nil, err
	}

	usageStartTime := req.StartTime
	if usageStartTime.IsZero() {
		usageStartTime = subscription.CurrentPeriodStart
	}

	// TODO: handle this to honour line item level end time
	usageEndTime := req.EndTime
	if usageEndTime.IsZero() {
		usageEndTime = subscription.CurrentPeriodEnd
	}

	if req.LifetimeUsage {
		usageStartTime = time.Time{}
		usageEndTime = time.Now().UTC()
	}

	// Collect all price IDs
	priceIDs := make([]string, 0, len(lineItems))
	for _, item := range lineItems {
		if item.PriceType != types.PRICE_TYPE_USAGE {
			continue
		}
		if item.MeterID == "" {
			continue
		}
		priceIDs = append(priceIDs, item.PriceID)
	}

	// Fetch all prices in one call
	priceFilter := types.NewNoLimitPriceFilter()
	priceFilter.PriceIDs = priceIDs
	priceFilter.Expand = lo.ToPtr(string(types.ExpandMeters))
	pricesList, err := priceService.GetPrices(ctx, priceFilter)
	if err != nil {
		return nil, err
	}

	// Build price map for quick lookup
	priceMap := make(map[string]*price.Price, len(pricesList.Items))
	meterMap := make(map[string]*dto.MeterResponse, len(pricesList.Items))
	// Pre-fetch all meter display names
	meterDisplayNames := make(map[string]string)

	for _, p := range pricesList.Items {
		priceMap[p.ID] = p.Price
		meterMap[p.Price.MeterID] = p.Meter
		if p.Meter != nil {
			meterDisplayNames[p.Price.MeterID] = p.Meter.Name
		}
	}

	totalCost := decimal.Zero

	s.Logger.Debugw("calculating usage for subscription",
		"subscription_id", req.SubscriptionID,
		"start_time", usageStartTime,
		"end_time", usageEndTime,
		"metered_line_items", len(priceIDs))

	// Performance optimization: Get distinct event names for this customer
	// to filter out meters that have no events, reducing processing from potentially
	// 400-500 meters down to only 5-7 that have actual usage
	distinctEventNames, err := s.EventRepo.GetDistinctEventNames(ctx, customer.ExternalID, usageStartTime, usageEndTime)
	if err != nil {
		s.Logger.Warnw("failed to get distinct event names, proceeding without optimization",
			"error", err,
			"external_customer_id", customer.ExternalID)
		distinctEventNames = nil // Fallback: process all meters if optimization fails
	}

	// Create a map for fast event name lookup
	eventNameExists := make(map[string]bool, len(distinctEventNames))
	for _, eventName := range distinctEventNames {
		eventNameExists[eventName] = true
	}

	s.Logger.Debugw("distinct event names optimization",
		"external_customer_id", customer.ExternalID,
		"total_distinct_events", len(distinctEventNames),
		"total_line_items", len(lineItems),
		"distinct_event_names", distinctEventNames)

	meterUsageRequests := make([]*dto.GetUsageByMeterRequest, 0, len(lineItems))
	for _, lineItem := range lineItems {
		if lineItem.PriceType != types.PRICE_TYPE_USAGE {
			continue
		}

		if lineItem.MeterID == "" {
			continue
		}

		meter := meterMap[lineItem.MeterID]
		if meter == nil {
			continue
		}

		if len(distinctEventNames) == 0 {
			// skip all usage items if distinct event names is nil
			// which means there is no event data in the database
			// this is a fallback to ensure that we don't process all meters
			// if the event data is not available

			s.Logger.Debugw("skipping meter as there are no events",
				"meter_id", lineItem.MeterID,
				"event_name", meter.EventName,
				"customer_id", customer.ID,
				"external_customer_id", customer.ExternalID,
				"subscription_id", req.SubscriptionID)
			continue
		}

		// Performance optimization: Skip meters that don't have any events for this customer
		// Only skip if we successfully got distinct event names (not nil) and the event doesn't exist
		if distinctEventNames != nil && !eventNameExists[meter.EventName] {
			s.Logger.Debugw("skipping meter with no events",
				"meter_id", lineItem.MeterID,
				"event_name", meter.EventName,
				"customer_id", customer.ID,
				"external_customer_id", customer.ExternalID,
				"subscription_id", req.SubscriptionID)
			continue
		}

		meterID := lineItem.MeterID
		usageRequest := &dto.GetUsageByMeterRequest{
			MeterID:            meterID,
			PriceID:            lineItem.PriceID,
			Meter:              meter.ToMeter(),
			ExternalCustomerID: customer.ExternalID,
			StartTime:          usageStartTime,
			EndTime:            usageEndTime,
			Filters:            make(map[string][]string),
		}

		for _, filter := range meter.Filters {
			usageRequest.Filters[filter.Key] = filter.Values
		}
		meterUsageRequests = append(meterUsageRequests, usageRequest)
	}

	s.Logger.Infow("performance optimization results",
		"subscription_id", req.SubscriptionID,
		"external_customer_id", customer.ExternalID,
		"total_line_items", len(lineItems),
		"total_usage_line_items", len(priceIDs),
		"meters_with_events", len(meterUsageRequests),
		"optimization_enabled", distinctEventNames != nil,
		"meters_skipped", len(priceIDs)-len(meterUsageRequests))

	usageMap, err := eventService.BulkGetUsageByMeter(ctx, meterUsageRequests)
	if err != nil {
		return nil, err
	}

	s.Logger.Debugw("fetched usage for meters",
		"meter_ids", lo.Keys(usageMap),
		"total_usage_count", len(usageMap),
		"subscription_id", req.SubscriptionID)

	// Store usage charges for later sorting and processing
	var usageCharges []*dto.SubscriptionUsageByMetersResponse

	// First pass: calculate normal costs and build initial charge objects
	// Note: we are iterating over the meterUsageRequests and not the usageMap
	// This is because the usageMap is a map of meterID to usage and we want to iterate over the meterUsageRequests
	// as there can be multiple requests for the same meterID with different priceIDs
	// Ideally this will not be the case and we will have a single request per meterID
	// TODO: should add validation to ensure that same subscription does not have multiple line items with the same meterID
	for _, request := range meterUsageRequests {
		meterID := request.MeterID
		priceID := request.PriceID
		usage, ok := usageMap[priceID]

		if !ok {
			continue
		}

		// Get price by price ID and check if it exists
		priceObj, priceExists := priceMap[usage.PriceID]
		if !priceExists || priceObj == nil {
			return nil, ierr.NewError("price not found").
				WithHint("The price for the meter was not found").
				WithReportableDetails(map[string]interface{}{
					"meter_id":        meterID,
					"price_id":        usage.PriceID,
					"subscription_id": req.SubscriptionID,
				}).
				Mark(ierr.ErrNotFound)
		}

		meterDisplayName := ""
		if meter, ok := meterDisplayNames[meterID]; ok {
			meterDisplayName = meter
		}

		// For bucketed max, we need to handle array of values
		var cost decimal.Decimal
		var quantity decimal.Decimal

		// Get meter info
		meterInfo := meterMap[meterID]
		if priceObj.MeterID != "" && meterInfo != nil && meterInfo.ToMeter().IsBucketedMaxMeter() {
			// For bucketed max, use the array of values
			bucketedValues := make([]decimal.Decimal, len(usage.Results))
			for i, result := range usage.Results {
				bucketedValues[i] = result.Value
			}
			cost = priceService.CalculateBucketedCost(ctx, priceObj, bucketedValues)

			// Calculate quantity as sum of all bucket maxes
			quantity = decimal.Zero
			for _, bucketValue := range bucketedValues {
				quantity = quantity.Add(bucketValue)
			}
		} else {
			// For all other cases, use the single value
			quantity = usage.Value
			cost = priceService.CalculateCost(ctx, priceObj, quantity)
		}

		s.Logger.Debugw("calculated usage for meter",
			"meter_id", meterID,
			"quantity", quantity,
			"cost", cost,
			"meter_display_name", meterDisplayName,
			"subscription_id", req.SubscriptionID,
			"usage", usage,
			"price", priceObj,
		)

		charge := createChargeResponse(
			priceObj,
			quantity,
			cost,
			meterDisplayName,
		)

		if charge == nil {
			continue
		}

		usageCharges = append(usageCharges, charge)
		totalCost = totalCost.Add(cost)
	}

	// Apply commitment logic if set on the subscription
	hasCommitment := false

	commitmentAmount := lo.FromPtr(subscription.CommitmentAmount)
	overageFactor := lo.FromPtr(subscription.OverageFactor)

	// Check if commitment amount is greater than zero
	if commitmentAmount.GreaterThan(decimal.Zero) {
		// Check if overage factor is greater than 1.0
		oneDecimal := decimal.NewFromInt(1)
		hasCommitment = overageFactor.GreaterThan(oneDecimal)
	}

	// Default values assuming no commitment/overage
	commitmentFloat, _ := commitmentAmount.Float64()
	overageFactorFloat, _ := overageFactor.Float64()
	response.CommitmentAmount = commitmentFloat
	response.OverageFactor = overageFactorFloat
	response.HasOverage = false

	// Initialize charges list with enough capacity
	response.Charges = make([]*dto.SubscriptionUsageByMetersResponse, 0, len(usageCharges)*2)

	// If using commitment-based pricing, process charges with overage logic
	if hasCommitment {
		// First, filter charges to only include usage-based charges for commitment calculations
		// Fixed charges are not subject to commitment/overage
		var usageOnlyCharges []*dto.SubscriptionUsageByMetersResponse
		var fixedCharges []*dto.SubscriptionUsageByMetersResponse

		for _, charge := range usageCharges {
			if charge.Price != nil && charge.Price.Type == types.PRICE_TYPE_USAGE {
				usageOnlyCharges = append(usageOnlyCharges, charge)
			} else {
				// Add fixed charges directly to the response without overage calculation
				fixedCharges = append(fixedCharges, charge)
			}
		}

		// Add all fixed charges directly to the response
		response.Charges = append(response.Charges, fixedCharges...)

		// Track remaining commitment and process each usage charge
		remainingCommitment := commitmentAmount
		totalOverageAmount := decimal.Zero

		for _, charge := range usageOnlyCharges {
			// Get charge amount as decimal for precise calculations
			chargeAmount := decimal.NewFromFloat(charge.Amount)
			pricePerUnit := decimal.Zero
			if charge.Price != nil && charge.Price.BillingModel == types.BILLING_MODEL_FLAT_FEE {
				pricePerUnit = charge.Price.Amount
			} else if charge.Quantity > 0 {
				pricePerUnit = chargeAmount.Div(decimal.NewFromFloat(charge.Quantity))
			}

			// Normal price covers all of this charge
			if remainingCommitment.GreaterThanOrEqual(chargeAmount) {
				charge.IsOverage = false
				remainingCommitment = remainingCommitment.Sub(chargeAmount)
				response.Charges = append(response.Charges, charge)
				continue
			}

			// Charge needs to be split between normal and overage
			if remainingCommitment.GreaterThan(decimal.Zero) {
				// Calculate exact quantity that can be covered by remaining commitment
				var normalQuantityDecimal decimal.Decimal

				if !pricePerUnit.IsZero() {
					normalQuantityDecimal = remainingCommitment.Div(pricePerUnit)

					// Round down to ensure we don't exceed commitment
					normalQuantityDecimal = normalQuantityDecimal.Floor()
				}

				// Calculate the normal amount based on the normal quantity
				normalAmountDecimal := normalQuantityDecimal.Mul(pricePerUnit)

				// Create the normal charge
				if normalQuantityDecimal.GreaterThan(decimal.Zero) {
					normalCharge := *charge // Create a copy
					normalCharge.Quantity = normalQuantityDecimal.InexactFloat64()
					normalCharge.Amount = price.FormatAmountToFloat64WithPrecision(normalAmountDecimal, subscription.Currency)
					normalCharge.DisplayAmount = price.FormatAmountToStringWithPrecision(normalAmountDecimal, subscription.Currency)
					normalCharge.IsOverage = false
					response.Charges = append(response.Charges, &normalCharge)
				}

				// Calculate overage quantity and amount
				overageQuantityDecimal := decimal.NewFromFloat(charge.Quantity).Sub(normalQuantityDecimal)

				// Create the overage charge only if there's actual overage
				if overageQuantityDecimal.GreaterThan(decimal.Zero) {
					overageAmountDecimal := overageQuantityDecimal.Mul(pricePerUnit).Mul(overageFactor)
					totalOverageAmount = totalOverageAmount.Add(overageAmountDecimal)

					overageCharge := *charge // Create a copy
					overageCharge.Quantity = overageQuantityDecimal.InexactFloat64()
					overageCharge.Amount = price.FormatAmountToFloat64WithPrecision(overageAmountDecimal, subscription.Currency)
					overageCharge.DisplayAmount = price.FormatAmountToStringWithPrecision(overageAmountDecimal, subscription.Currency)
					overageCharge.IsOverage = true
					overageCharge.OverageFactor = overageFactorFloat
					response.Charges = append(response.Charges, &overageCharge)
					response.HasOverage = true
				}

				// Update remaining commitment (should be zero or very close to it due to rounding)
				remainingCommitment = remainingCommitment.Sub(normalAmountDecimal)
				continue
			}

			// Charge is entirely in overage
			overageAmountDecimal := chargeAmount.Mul(overageFactor)
			totalOverageAmount = totalOverageAmount.Add(overageAmountDecimal)

			charge.Amount = price.FormatAmountToFloat64WithPrecision(overageAmountDecimal, subscription.Currency)
			charge.DisplayAmount = overageAmountDecimal.StringFixed(6)
			charge.IsOverage = true
			charge.OverageFactor = overageFactorFloat
			response.Charges = append(response.Charges, charge)
			response.HasOverage = true
		}

		// Calculate final amounts for response
		commitmentUtilized := commitmentAmount.Sub(remainingCommitment)
		commitmentUtilizedFloat, _ := commitmentUtilized.Float64()
		overageAmountFloat, _ := totalOverageAmount.Float64()
		response.CommitmentUtilized = commitmentUtilizedFloat
		response.OverageAmount = overageAmountFloat

		// Update total cost with commitment + overage calculation
		totalCost = commitmentUtilized.Add(totalOverageAmount)
	} else {
		// Without commitment, just use the original charges
		response.Charges = usageCharges
	}

	response.StartTime = usageStartTime
	response.EndTime = usageEndTime
	response.Amount = price.FormatAmountToFloat64WithPrecision(totalCost, subscription.Currency)
	response.Currency = subscription.Currency
	response.DisplayAmount = price.GetDisplayAmountWithPrecision(totalCost, subscription.Currency)
	return response, nil
}

// UpdateBillingPeriods updates the current billing periods for all active subscriptions
// This should be run every 15 minutes to ensure billing periods are up to date
// TODO: move to billing service
func (s *subscriptionService) UpdateBillingPeriods(ctx context.Context) (*dto.SubscriptionUpdatePeriodResponse, error) {
	const batchSize = 100
	now := time.Now().UTC()

	s.Logger.Infow("starting billing period updates",
		"current_time", now)

	response := &dto.SubscriptionUpdatePeriodResponse{
		Items:        make([]*dto.SubscriptionUpdatePeriodResponseItem, 0),
		TotalFailed:  0,
		TotalSuccess: 0,
		StartAt:      now,
	}

	offset := 0
	for {
		filter := &types.SubscriptionFilter{
			QueryFilter: &types.QueryFilter{
				Limit:  lo.ToPtr(batchSize),
				Offset: lo.ToPtr(offset),
				Status: lo.ToPtr(types.StatusPublished),
			},
			SubscriptionStatus: []types.SubscriptionStatus{types.SubscriptionStatusActive},
			TimeRangeFilter: &types.TimeRangeFilter{
				EndTime: &now,
			},
		}

		subs, err := s.SubRepo.ListAllTenant(ctx, filter)
		if err != nil {
			return response, err
		}

		s.Logger.Infow("processing subscription batch",
			"batch_size", len(subs),
			"offset", offset)

		if len(subs) == 0 {
			break // No more subscriptions to process
		}

		// Process each subscription in the batch
		for _, sub := range subs {
			// update context to include the tenant id
			ctx = context.WithValue(ctx, types.CtxTenantID, sub.TenantID)
			ctx = context.WithValue(ctx, types.CtxEnvironmentID, sub.EnvironmentID)
			ctx = context.WithValue(ctx, types.CtxUserID, sub.CreatedBy)

			item := &dto.SubscriptionUpdatePeriodResponseItem{
				SubscriptionID: sub.ID,
				PeriodStart:    sub.CurrentPeriodStart,
				PeriodEnd:      sub.CurrentPeriodEnd,
			}
			err = s.processSubscriptionPeriod(ctx, sub, now)
			if err != nil {
				s.Logger.Errorw("failed to process subscription period",
					"subscription_id", sub.ID,
					"error", err)

				response.TotalFailed++
				item.Error = err.Error()
			} else {
				item.Success = true
				response.TotalSuccess++
			}

			response.Items = append(response.Items, item)
		}

		offset += len(subs)
		if len(subs) < batchSize {
			break // No more subscriptions to fetch
		}
	}

	return response, nil
}

/// Helpers

// we get each subscription picked by the cron where the current period end is before now
// and we process the subscription period to create invoices for the passed period
// and decide next period start and end or cancel the subscription if it has ended
func (s *subscriptionService) processSubscriptionPeriod(ctx context.Context, sub *subscription.Subscription, now time.Time) error {
	// Skip processing for paused subscriptions
	if sub.SubscriptionStatus == types.SubscriptionStatusPaused {
		s.Logger.Infow("skipping period processing for paused subscription",
			"subscription_id", sub.ID)
		return nil
	}

	// Check for scheduled pauses that should be activated
	if sub.PauseStatus == types.PauseStatusScheduled && sub.ActivePauseID != nil {
		pause, err := s.SubRepo.GetPause(ctx, *sub.ActivePauseID)
		if err != nil {
			return err
		}

		// If this is a period-end pause and we're at period end, activate it
		if pause.PauseMode == types.PauseModePeriodEnd && !now.Before(sub.CurrentPeriodEnd) {
			sub.SubscriptionStatus = types.SubscriptionStatusPaused
			pause.PauseStatus = types.PauseStatusActive

			// Update the subscription and pause
			if err := s.SubRepo.Update(ctx, sub); err != nil {
				return err
			}

			if err := s.SubRepo.UpdatePause(ctx, pause); err != nil {
				return err
			}

			s.Logger.Infow("activated period-end pause",
				"subscription_id", sub.ID,
				"pause_id", pause.ID)

			// Skip further processing
			return nil
		}

		// If this is a scheduled pause and we've reached the start date, activate it
		if pause.PauseMode == types.PauseModeScheduled && !now.Before(pause.PauseStart) {
			sub.SubscriptionStatus = types.SubscriptionStatusPaused
			pause.PauseStatus = types.PauseStatusActive

			// Update the subscription and pause
			if err := s.SubRepo.Update(ctx, sub); err != nil {
				return err
			}

			if err := s.SubRepo.UpdatePause(ctx, pause); err != nil {
				return err
			}

			s.Logger.Infow("activated scheduled pause",
				"subscription_id", sub.ID,
				"pause_id", pause.ID)

			// Skip further processing
			return nil
		}
	}

	// Check for auto-resume based on pause end date
	if sub.SubscriptionStatus == types.SubscriptionStatusPaused && sub.ActivePauseID != nil {
		pause, err := s.SubRepo.GetPause(ctx, *sub.ActivePauseID)
		if err != nil {
			return err
		}

		// If this pause has an end date and we've reached it, auto-resume
		if pause.PauseEnd != nil && !now.Before(*pause.PauseEnd) {
			// Calculate the pause duration
			pauseDuration := now.Sub(pause.PauseStart)

			// Update the pause record
			pause.PauseStatus = types.PauseStatusCompleted
			pause.ResumedAt = &now

			// Update the subscription
			sub.SubscriptionStatus = types.SubscriptionStatusActive
			sub.PauseStatus = types.PauseStatusNone
			sub.ActivePauseID = nil

			// Adjust the billing period by the pause duration
			sub.CurrentPeriodEnd = sub.CurrentPeriodEnd.Add(pauseDuration)

			// Update the subscription and pause
			if err := s.SubRepo.Update(ctx, sub); err != nil {
				return err
			}

			if err := s.SubRepo.UpdatePause(ctx, pause); err != nil {
				return err
			}

			s.Logger.Infow("auto-resumed subscription",
				"subscription_id", sub.ID,
				"pause_id", pause.ID,
				"pause_duration", pauseDuration)

			// Continue with normal processing
		} else {
			// Still paused, skip processing
			s.Logger.Infow("skipping period processing for paused subscription",
				"subscription_id", sub.ID)
			return nil
		}
	}

	// TODO: Check if subscription has ended and should be cancelled

	// Initialize services
	invoiceService := NewInvoiceService(s.ServiceParams)

	currentStart := sub.CurrentPeriodStart
	currentEnd := sub.CurrentPeriodEnd

	// Start with current period
	var periods []struct {
		start time.Time
		end   time.Time
	}
	periods = append(periods, struct {
		start time.Time
		end   time.Time
	}{
		start: currentStart,
		end:   currentEnd,
	})

	// isLastPeriod := false
	// if sub.EndDate != nil && currentEnd.Equal(*sub.EndDate) {
	// 	isLastPeriod = true
	// }

	// Generate periods but respect subscription end date
	for currentEnd.Before(now) {
		nextStart := currentEnd
		nextEnd, err := types.NextBillingDate(nextStart, sub.BillingAnchor, sub.BillingPeriodCount, sub.BillingPeriod, sub.EndDate)
		if err != nil {
			s.Logger.Errorw("failed to calculate next billing date",
				"subscription_id", sub.ID,
				"current_end", currentEnd,
				"process_up_to", now,
				"error", err)
			return err
		}

		periods = append(periods, struct {
			start time.Time
			end   time.Time
		}{
			start: nextStart,
			end:   nextEnd,
		})

		// in case of end date reached or next end is equal to current end, we break the loop
		// nextEnd will be equal to currentEnd in case of end date reached
		if nextEnd.Equal(currentEnd) {
			s.Logger.Infow("stopped period generation - reached subscription end date",
				"subscription_id", sub.ID,
				"end_date", sub.EndDate,
				"final_period_end", currentEnd)
			break
		}

		currentEnd = nextEnd
	}

	if len(periods) == 1 {
		s.Logger.Debugw("no transitions needed for subscription",
			"subscription_id", sub.ID,
			"current_period_start", sub.CurrentPeriodStart,
			"current_period_end", sub.CurrentPeriodEnd,
			"process_up_to", now)
		return nil
	}

	// Use db's WithTx for atomic operations
	err := s.DB.WithTx(ctx, func(ctx context.Context) error {
		// Process all periods except the last one (which becomes the new current period)
		for i := 0; i < len(periods)-1; i++ {
			period := periods[i]

			// Create a single invoice for both arrear and advance charges at period end
			inv, err := invoiceService.CreateSubscriptionInvoice(ctx, &dto.CreateSubscriptionInvoiceRequest{
				SubscriptionID: sub.ID,
				PeriodStart:    period.start,
				PeriodEnd:      period.end,
				ReferencePoint: types.ReferencePointPeriodEnd,
			})
			if err != nil {
				return err
			}

			// Check for cancellation at this period end
			if sub.CancelAtPeriodEnd && sub.CancelAt != nil && !sub.CancelAt.After(period.end) {
				sub.SubscriptionStatus = types.SubscriptionStatusCancelled
				sub.CancelledAt = sub.CancelAt
				break
			}

			// Check if this period end matches the subscription end date
			if sub.EndDate != nil && period.end.Equal(*sub.EndDate) {
				sub.SubscriptionStatus = types.SubscriptionStatusCancelled
				sub.CancelledAt = sub.EndDate
				s.Logger.Infow("will cancel subscription at end of this period",
					"subscription_id", sub.ID,
					"period_end", period.end,
					"end_date", *sub.EndDate)
				break
			}

			if inv == nil {
				s.Logger.Errorw("skipping period as no invoice was created",
					"subscription_id", sub.ID,
					"period_start", period.start,
					"period_end", period.end,
					"period_index", i)
				continue
			}

			s.Logger.Infow("created invoice for period",
				"subscription_id", sub.ID,
				"invoice_id", inv.ID,
				"period_start", period.start,
				"period_end", period.end,
				"period_index", i)
		}

		// Update to the new current period (last period)
		newPeriod := periods[len(periods)-1]
		sub.CurrentPeriodStart = newPeriod.start
		sub.CurrentPeriodEnd = newPeriod.end

		// Final cancellation check
		if sub.CancelAtPeriodEnd && sub.CancelAt != nil && !sub.CancelAt.After(newPeriod.end) {
			sub.SubscriptionStatus = types.SubscriptionStatusCancelled
			sub.CancelledAt = sub.CancelAt
		}

		// Check if the new period end matches the subscription end date
		if sub.EndDate != nil && newPeriod.end.Equal(*sub.EndDate) {
			sub.SubscriptionStatus = types.SubscriptionStatusCancelled
			sub.CancelledAt = sub.EndDate
			s.Logger.Infow("subscription will be cancelled at new period end (end date reached)",
				"subscription_id", sub.ID,
				"new_period_end", newPeriod.end,
				"end_date", *sub.EndDate)
		}

		// Update the subscription
		if err := s.SubRepo.Update(ctx, sub); err != nil {
			return err
		}

		s.Logger.Infow("completed subscription period processing",
			"subscription_id", sub.ID,
			"original_period_start", periods[0].start,
			"original_period_end", periods[0].end,
			"new_period_start", sub.CurrentPeriodStart,
			"new_period_end", sub.CurrentPeriodEnd,
			"process_up_to", now,
			"periods_processed", len(periods)-1,
			"has_end_date", sub.EndDate != nil)

		return nil
	})

	if err != nil {
		s.Logger.Errorw("failed to process subscription period",
			"subscription_id", sub.ID,
			"error", err)
		return err
	}

	return nil
}

func createChargeResponse(priceObj *price.Price, quantity decimal.Decimal, cost decimal.Decimal, meterDisplayName string) *dto.SubscriptionUsageByMetersResponse {
	if priceObj == nil {
		return nil
	}

	finalAmount := price.FormatAmountToFloat64WithPrecision(cost, priceObj.Currency)

	return &dto.SubscriptionUsageByMetersResponse{
		Amount:           finalAmount,
		Currency:         priceObj.Currency,
		DisplayAmount:    price.GetDisplayAmountWithPrecision(cost, priceObj.Currency),
		Quantity:         quantity.InexactFloat64(),
		MeterID:          priceObj.MeterID,
		MeterDisplayName: meterDisplayName,
		Price:            priceObj,
	}
}

// filterValidPricesForSubscription filters prices that are valid for a subscription
// This utility function can be used for both plans and addons
func filterValidPricesForSubscription(prices []*dto.PriceResponse, subscription *subscription.Subscription) []*dto.PriceResponse {
	var validPrices []*dto.PriceResponse
	for _, p := range prices {
		if types.IsMatchingCurrency(p.Price.Currency, subscription.Currency) &&
			p.Price.BillingPeriod == subscription.BillingPeriod &&
			p.Price.BillingPeriodCount == subscription.BillingPeriodCount {
			validPrices = append(validPrices, p)
		}
	}
	return validPrices
}

// ValidateAndFilterPricesForSubscription validates and filters prices for a subscription
// This method follows the same validation pattern as plans and can be reused for addons
func (s *subscriptionService) ValidateAndFilterPricesForSubscription(
	ctx context.Context,
	entityID string,
	entityType types.PriceEntityType,
	subscription *subscription.Subscription,
) ([]*dto.PriceResponse, error) {
	// Get prices for the entity (plan or addon)
	priceService := NewPriceService(s.ServiceParams)

	var pricesResponse *dto.ListPricesResponse
	var err error

	if entityType == types.PRICE_ENTITY_TYPE_PLAN {
		pricesResponse, err = priceService.GetPricesByPlanID(ctx, entityID)
	} else if entityType == types.PRICE_ENTITY_TYPE_ADDON {
		pricesResponse, err = priceService.GetPricesByAddonID(ctx, entityID)
	}

	if err != nil {
		return nil, err
	}

	if len(pricesResponse.Items) == 0 {
		return nil, ierr.NewError("no prices found for entity").
			WithHint("The entity must have at least one price to create a subscription").
			WithReportableDetails(map[string]interface{}{
				"entity_id":   entityID,
				"entity_type": entityType,
			}).
			Mark(ierr.ErrValidation)
	}

	// Filter prices for subscription that are valid for the entity
	validPrices := filterValidPricesForSubscription(pricesResponse.Items, subscription)
	if len(validPrices) == 0 {
		return nil, ierr.NewError("no valid prices found for subscription").
			WithHint("No prices match the subscription criteria").
			WithReportableDetails(map[string]interface{}{
				"entity_id":       entityID,
				"entity_type":     entityType,
				"billing_period":  subscription.BillingPeriod,
				"billing_cadence": subscription.BillingCadence,
				"currency":        subscription.Currency,
			}).
			Mark(ierr.ErrValidation)
	}

	return validPrices, nil
}

// PauseSubscription pauses a subscription
func (s *subscriptionService) PauseSubscription(
	ctx context.Context,
	subscriptionID string,
	req *dto.PauseSubscriptionRequest,
) (*dto.PauseSubscriptionResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Get the subscription
	sub, lineItems, err := s.SubRepo.GetWithLineItems(ctx, subscriptionID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get subscription for pausing").
			Mark(ierr.ErrNotFound)
	}
	sub.LineItems = lineItems

	// Validate subscription can be paused
	if sub.SubscriptionStatus != types.SubscriptionStatusActive {
		return nil, ierr.NewError("invalid subscription status").
			WithHint("Subscription is not active").
			WithReportableDetails(map[string]any{
				"status": sub.SubscriptionStatus,
			}).
			Mark(ierr.ErrValidation)
	}

	// Calculate pause start and end
	pauseStart, pauseEnd, err := s.calculatePauseStartEnd(req, sub)
	if err != nil {
		return nil, err
	}

	// Use the unified billing impact calculator
	impact, err := s.calculateBillingImpact(ctx, sub, lineItems, *pauseStart, pauseEnd, false, nil)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to calculate billing impact").
			Mark(ierr.ErrValidation)
	}

	// If this is a dry run, return the impact without making changes
	if req.DryRun {
		return &dto.PauseSubscriptionResponse{
			BillingImpact: impact,
			DryRun:        true,
		}, nil
	}

	// Create the pause record and update the subscription
	sub, pause, err := s.executePause(ctx, sub, req, pauseStart, pauseEnd)
	if err != nil {
		return nil, err
	}

	response := dto.NewSubscriptionPauseResponse(sub, pause)
	response.BillingImpact = impact

	// Return the response
	// Publish webhook event
	s.publishInternalWebhookEvent(ctx, types.WebhookEventSubscriptionUpdated, subscriptionID)
	s.publishInternalWebhookEvent(ctx, types.WebhookEventSubscriptionPaused, subscriptionID)
	return response, nil
}

// executePause creates the pause record and updates the subscription
func (s *subscriptionService) executePause(
	ctx context.Context,
	sub *subscription.Subscription,
	req *dto.PauseSubscriptionRequest,
	pauseStart *time.Time,
	pauseEnd *time.Time,
) (*subscription.Subscription, *subscription.SubscriptionPause, error) {
	// Set pause status based on mode
	pauseStatus := types.PauseStatusActive
	if req.PauseMode == types.PauseModeScheduled || req.PauseMode == types.PauseModePeriodEnd {
		pauseStatus = types.PauseStatusScheduled
	}

	// Create the pause record
	pause := &subscription.SubscriptionPause{
		ID:                  types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_PAUSE),
		SubscriptionID:      sub.ID,
		PauseStatus:         pauseStatus,
		PauseMode:           req.PauseMode,
		ResumeMode:          types.ResumeModeAuto, // Default to auto resume if pause end is set
		PauseStart:          *pauseStart,
		PauseEnd:            pauseEnd,
		ResumedAt:           nil,
		OriginalPeriodStart: sub.CurrentPeriodStart,
		OriginalPeriodEnd:   sub.CurrentPeriodEnd,
		Reason:              req.Reason,
		Metadata:            req.Metadata,
		EnvironmentID:       sub.EnvironmentID,
		BaseModel:           types.GetDefaultBaseModel(ctx),
	}

	// Update the subscription
	sub.PauseStatus = pauseStatus
	sub.ActivePauseID = lo.ToPtr(pause.ID)

	// Only change subscription status to paused for immediate pauses
	if req.PauseMode == types.PauseModeImmediate {
		sub.SubscriptionStatus = types.SubscriptionStatusPaused
	}

	// Execute the transaction
	err := s.DB.WithTx(ctx, func(txCtx context.Context) error {
		// Create the pause record
		if err := s.SubRepo.CreatePause(txCtx, pause); err != nil {
			return err
		}

		// Update the subscription
		if err := s.SubRepo.Update(txCtx, sub); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	return sub, pause, nil
}

// ResumeSubscription resumes a paused subscription
func (s *subscriptionService) ResumeSubscription(
	ctx context.Context,
	subscriptionID string,
	req *dto.ResumeSubscriptionRequest,
) (*dto.ResumeSubscriptionResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Get the subscription with its pauses
	_, pauses, err := s.SubRepo.GetWithPauses(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	// get the line items
	sub, lineItems, err := s.SubRepo.GetWithLineItems(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	sub.LineItems = lineItems
	sub.Pauses = pauses

	// Validate subscription can be resumed
	if sub.SubscriptionStatus != types.SubscriptionStatusPaused &&
		sub.PauseStatus != types.PauseStatusScheduled {
		return nil, ierr.NewError("invalid subscription status").
			WithHint("Subscription is not paused").
			WithReportableDetails(map[string]any{
				"status": sub.SubscriptionStatus,
			}).
			Mark(ierr.ErrValidation)
	}

	if sub.ActivePauseID == nil {
		return nil, ierr.NewError("invalid subscription status").
			WithHint("Subscription has no active pause").
			Mark(ierr.ErrValidation)
	}

	// Find the active pause
	var activePause *subscription.SubscriptionPause
	for _, p := range pauses {
		if p.ID == *sub.ActivePauseID {
			activePause = p
			break
		}
	}

	if activePause == nil {
		return nil, ierr.NewError("invalid subscription status").
			WithHint("Active pause not found").
			Mark(ierr.ErrValidation)
	}

	// Use the unified billing impact calculator
	impact, err := s.calculateBillingImpact(ctx, sub, lineItems, activePause.PauseStart, activePause.PauseEnd, true, activePause)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to calculate billing impact").
			Mark(ierr.ErrValidation)
	}

	// If this is a dry run, return the impact without making changes
	if req.DryRun {
		return &dto.ResumeSubscriptionResponse{
			BillingImpact: impact,
			DryRun:        true,
		}, nil
	}

	// Resume the subscription
	sub, activePause, err = s.executeResume(ctx, sub, activePause, req)
	if err != nil {
		return nil, err
	}

	// Publish webhook event
	s.publishInternalWebhookEvent(ctx, types.WebhookEventSubscriptionUpdated, subscriptionID)
	s.publishInternalWebhookEvent(ctx, types.WebhookEventSubscriptionResumed, subscriptionID)

	// Return the response
	return &dto.ResumeSubscriptionResponse{
		Subscription: &dto.SubscriptionResponse{
			Subscription: sub,
		},
		Pause: &dto.SubscriptionPauseResponse{
			SubscriptionPause: activePause,
		},
		BillingImpact: impact,
		DryRun:        false,
	}, nil
}

// executeResume updates the subscription and pause record for a resume operation
func (s *subscriptionService) executeResume(
	ctx context.Context,
	sub *subscription.Subscription,
	activePause *subscription.SubscriptionPause,
	req *dto.ResumeSubscriptionRequest,
) (*subscription.Subscription, *subscription.SubscriptionPause, error) {
	// Update the pause record
	now := time.Now()
	activePause.PauseStatus = types.PauseStatusCompleted
	activePause.ResumeMode = req.ResumeMode
	activePause.ResumedAt = &now
	activePause.Metadata = req.Metadata
	activePause.UpdatedBy = types.GetUserID(ctx)

	// Calculate the pause duration
	pauseDuration := now.Sub(activePause.PauseStart)

	// Update the subscription
	sub.PauseStatus = types.PauseStatusNone
	sub.ActivePauseID = nil

	// Only change subscription status if it was paused
	if sub.SubscriptionStatus == types.SubscriptionStatusPaused {
		sub.SubscriptionStatus = types.SubscriptionStatusActive
	}

	// Adjust the billing period by the pause duration
	sub.CurrentPeriodEnd = sub.CurrentPeriodEnd.Add(pauseDuration)

	// Execute the transaction
	err := s.DB.WithTx(ctx, func(txCtx context.Context) error {
		// Update the pause record
		if err := s.SubRepo.UpdatePause(txCtx, activePause); err != nil {
			return err
		}

		// Update the subscription
		if err := s.SubRepo.Update(txCtx, sub); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	return sub, activePause, nil
}

// GetPause gets a subscription pause by ID
func (s *subscriptionService) GetPause(ctx context.Context, pauseID string) (*subscription.SubscriptionPause, error) {
	pause, err := s.SubRepo.GetPause(ctx, pauseID)
	if err != nil {
		return nil, err
	}
	return pause, nil
}

// ListPauses lists all pauses for a subscription
func (s *subscriptionService) ListPauses(ctx context.Context, subscriptionID string) (*dto.ListSubscriptionPausesResponse, error) {
	pauses, err := s.SubRepo.ListPauses(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	return dto.NewListSubscriptionPausesResponse(pauses), nil
}

// CalculatePauseImpact calculates the billing impact of pausing a subscription
func (s *subscriptionService) CalculatePauseImpact(
	ctx context.Context,
	subscriptionID string,
	req *dto.PauseSubscriptionRequest,
) (*types.BillingImpactDetails, error) {
	// Get the subscription
	sub, lineItems, err := s.SubRepo.GetWithLineItems(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}

	// Validate subscription can be paused
	if sub.SubscriptionStatus != types.SubscriptionStatusActive {
		return nil, ierr.NewError("invalid subscription status").
			WithHint("Subscription is not active").
			WithReportableDetails(map[string]any{
				"status": sub.SubscriptionStatus,
			}).
			Mark(ierr.ErrValidation)
	}

	// Calculate pause start and end
	pauseStart, pauseEnd, err := s.calculatePauseStartEnd(req, sub)
	if err != nil {
		return nil, err
	}

	// Use the unified billing impact calculator
	return s.calculateBillingImpact(ctx, sub, lineItems, *pauseStart, pauseEnd, false, nil)
}

// CalculateResumeImpact calculates the billing impact of resuming a subscription
func (s *subscriptionService) CalculateResumeImpact(
	ctx context.Context,
	subscriptionID string,
	req *dto.ResumeSubscriptionRequest,
) (*types.BillingImpactDetails, error) {
	// Get the subscription with its pauses
	_, pauses, err := s.SubRepo.GetWithPauses(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}

	// get the line items
	sub, lineItems, err := s.SubRepo.GetWithLineItems(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	sub.LineItems = lineItems
	sub.Pauses = pauses

	// Validate subscription can be resumed
	if sub.SubscriptionStatus != types.SubscriptionStatusPaused &&
		sub.PauseStatus != types.PauseStatusScheduled {
		return nil, ierr.NewError("invalid subscription status").
			WithHint("Subscription is not paused").
			WithReportableDetails(map[string]any{
				"status": sub.SubscriptionStatus,
			}).
			Mark(ierr.ErrValidation)
	}

	if sub.ActivePauseID == nil {
		return nil, ierr.NewError("invalid subscription status").
			WithHint("Subscription has no active pause").
			Mark(ierr.ErrValidation)
	}

	// Find the active pause
	var activePause *subscription.SubscriptionPause
	for _, p := range pauses {
		if p.ID == *sub.ActivePauseID {
			activePause = p
			break
		}
	}

	if activePause == nil {
		return nil, ierr.NewError("invalid subscription status").
			WithHint("Active pause not found").
			Mark(ierr.ErrValidation)
	}

	// Use the unified billing impact calculator
	return s.calculateBillingImpact(ctx, sub, lineItems, activePause.PauseStart, activePause.PauseEnd, true, activePause)
}

// Pause subscription helper methods

// calculatePauseStartEnd calculates the pause start and end dates based on the pause mode
// requested input and the subscription's current period end date.
// TODO: add a config check for max pause duration and make it configurable for each tenant
func (s *subscriptionService) calculatePauseStartEnd(req *dto.PauseSubscriptionRequest, sub *subscription.Subscription) (*time.Time, *time.Time, error) {
	now := time.Now().UTC()

	// First lets handle pause_start date based on pause mode
	var pauseStart, pauseEnd *time.Time
	switch req.PauseMode {
	case types.PauseModeImmediate:
		pauseStart = &now
	case types.PauseModeScheduled:
		pauseStart = req.PauseStart
	case types.PauseModePeriodEnd:
		pauseStart = lo.ToPtr(sub.CurrentPeriodEnd)
	default:
		return nil, nil, ierr.NewError("invalid pause mode").
			WithHint("Invalid pause mode").
			WithReportableDetails(map[string]any{
				"pauseMode": req.PauseMode,
			}).
			Mark(ierr.ErrValidation)
	}

	if pauseStart == nil || pauseStart.IsZero() {
		return nil, nil, ierr.NewError("invalid pause start date").
			WithHint("Pause start date is required").
			Mark(ierr.ErrValidation)
	}

	if req.PauseDays != nil {
		pauseEnd = lo.ToPtr(pauseStart.AddDate(0, 0, *req.PauseDays))
	} else if req.PauseEnd != nil {
		pauseEnd = req.PauseEnd
	}

	if pauseEnd == nil || pauseEnd.IsZero() || pauseEnd.Before(*pauseStart) {
		return nil, nil, ierr.NewError("invalid pause end date").
			WithHint("Pause end date is not valid").
			WithReportableDetails(map[string]any{
				"pauseStart": pauseStart,
				"pauseEnd":   pauseEnd,
			}).
			Mark(ierr.ErrValidation)
	}

	return pauseStart, pauseEnd, nil
}

// calculateBillingImpact calculates the billing impact of pause/resume operations
func (s *subscriptionService) calculateBillingImpact(
	_ context.Context,
	sub *subscription.Subscription,
	lineItems []*subscription.SubscriptionLineItem,
	pauseStart time.Time,
	pauseEnd *time.Time,
	isResume bool,
	activePause *subscription.SubscriptionPause,
) (*types.BillingImpactDetails, error) {
	// Initialize impact details
	impact := &types.BillingImpactDetails{}

	// Get subscription configuration for billing model (advance vs. arrears)
	// TODO: handle this when we implement add ons with one time charges
	var invoiceCadence types.InvoiceCadence
	for _, li := range lineItems {
		if li.PriceType == types.PRICE_TYPE_FIXED {
			invoiceCadence = li.InvoiceCadence
			break
		}
	}

	// TODO: need to handle this better for cases with no fixed prices
	if invoiceCadence == "" {
		invoiceCadence = types.InvoiceCadenceArrear
	}

	// Set original period information
	if isResume && activePause != nil {
		impact.OriginalPeriodStart = &activePause.OriginalPeriodStart
		impact.OriginalPeriodEnd = &activePause.OriginalPeriodEnd
	} else {
		impact.OriginalPeriodStart = &sub.CurrentPeriodStart
		impact.OriginalPeriodEnd = &sub.CurrentPeriodEnd
	}

	now := time.Now()

	if isResume {
		// Resume impact calculation
		if activePause == nil {
			return nil, ierr.NewError("missing active pause").
				WithHint("Cannot calculate resume impact without active pause").
				Mark(ierr.ErrValidation)
		}

		// Calculate pause duration
		pauseDuration := now.Sub(activePause.PauseStart)
		impact.PauseDurationDays = int(pauseDuration.Hours() / 24)

		// Set next billing date to now for immediate resumes
		impact.NextBillingDate = &now

		// Calculate adjusted period dates
		adjustedStart := now
		adjustedEnd := activePause.OriginalPeriodEnd.Add(pauseDuration)
		impact.AdjustedPeriodStart = &adjustedStart
		impact.AdjustedPeriodEnd = &adjustedEnd

		// Calculate next billing amount based on billing model
		if invoiceCadence == types.InvoiceCadenceAdvance {
			// For advance billing, calculate the prorated amount for the resumed period
			// This is a simplified calculation - in a real implementation, you would
			// need to consider the subscription's line items, pricing, etc.
			totalPeriodDuration := activePause.OriginalPeriodEnd.Sub(activePause.OriginalPeriodStart)
			remainingDuration := adjustedEnd.Sub(now)
			if totalPeriodDuration > 0 {
				remainingRatio := float64(remainingDuration) / float64(totalPeriodDuration)
				impact.NextBillingAmount = decimal.NewFromFloat(100.00 * remainingRatio) // Placeholder value
			}
		} else {
			// For arrears billing, no immediate charge on resume
			impact.NextBillingAmount = decimal.Zero
		}
	} else {
		// Pause impact calculation

		// Calculate the current period adjustment (credit for unused time)
		if invoiceCadence == types.InvoiceCadenceAdvance {
			// For advance billing, calculate credit for unused portion
			totalPeriodDuration := sub.CurrentPeriodEnd.Sub(sub.CurrentPeriodStart)
			unusedDuration := sub.CurrentPeriodEnd.Sub(pauseStart)
			if totalPeriodDuration > 0 {
				unusedRatio := float64(unusedDuration) / float64(totalPeriodDuration)
				// Negative value indicates a credit to the customer
				impact.PeriodAdjustmentAmount = decimal.NewFromFloat(-100.00 * unusedRatio) // Placeholder value
			}
		} else {
			// For arrears billing, calculate charge for used portion
			totalPeriodDuration := sub.CurrentPeriodEnd.Sub(sub.CurrentPeriodStart)
			usedDuration := pauseStart.Sub(sub.CurrentPeriodStart)
			if totalPeriodDuration > 0 {
				usedRatio := float64(usedDuration) / float64(totalPeriodDuration)
				impact.PeriodAdjustmentAmount = decimal.NewFromFloat(100.00 * usedRatio) // Placeholder value
			}
		}

		// Calculate pause duration and next billing date
		if pauseEnd != nil {
			pauseDuration := pauseEnd.Sub(pauseStart)
			impact.PauseDurationDays = int(pauseDuration.Hours() / 24)
			impact.NextBillingDate = pauseEnd

			// Calculate adjusted period dates
			adjustedStart := pauseStart
			adjustedEnd := sub.CurrentPeriodEnd.Add(pauseDuration)
			impact.AdjustedPeriodStart = &adjustedStart
			impact.AdjustedPeriodEnd = &adjustedEnd
		} else {
			// For indefinite pauses, use a default of 30 days for estimation
			defaultPauseDays := 30
			impact.PauseDurationDays = defaultPauseDays
			estimatedEnd := pauseStart.AddDate(0, 0, defaultPauseDays)
			impact.NextBillingDate = &estimatedEnd

			// Calculate adjusted period dates
			adjustedStart := pauseStart
			adjustedEnd := sub.CurrentPeriodEnd.AddDate(0, 0, defaultPauseDays)
			impact.AdjustedPeriodStart = &adjustedStart
			impact.AdjustedPeriodEnd = &adjustedEnd
		}
	}

	return impact, nil
}

func (s *subscriptionService) publishInternalWebhookEvent(ctx context.Context, eventName string, subscriptionID string) {

	eventPayload := webhookDto.InternalSubscriptionEvent{
		SubscriptionID: subscriptionID,
		TenantID:       types.GetTenantID(ctx),
	}

	webhookPayload, err := json.Marshal(eventPayload)

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

// CreateSubscriptionSchedule creates a subscription schedule
func (s *subscriptionService) CreateSubscriptionSchedule(ctx context.Context, req *dto.CreateSubscriptionScheduleRequest) (*dto.SubscriptionScheduleResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Verify subscription exists
	sub, _, err := s.SubRepo.GetWithLineItems(ctx, req.SubscriptionID)
	if err != nil {
		return nil, err
	}

	// Check if a schedule already exists for the subscription
	if s.SubscriptionScheduleRepo == nil {
		return nil, ierr.NewError("subscription repository does not support schedules").
			WithHint("Schedule functionality is not supported").
			Mark(ierr.ErrInternal)
	}

	// Check if a schedule already exists
	existingSchedule, err := s.SubscriptionScheduleRepo.GetBySubscriptionID(ctx, req.SubscriptionID)
	if err == nil && existingSchedule != nil {
		return nil, ierr.NewError("subscription already has a schedule").
			WithHint("A subscription can only have one schedule").
			WithReportableDetails(map[string]interface{}{
				"subscription_id": req.SubscriptionID,
				"schedule_id":     existingSchedule.ID,
			}).
			Mark(ierr.ErrAlreadyExists)
	}

	// Create the schedule
	schedule := &subscription.SubscriptionSchedule{
		ID:                types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_SCHEDULE),
		SubscriptionID:    req.SubscriptionID,
		ScheduleStatus:    types.ScheduleStatusActive,
		CurrentPhaseIndex: 0,
		EndBehavior:       req.EndBehavior,
		StartDate:         sub.StartDate,
		Metadata:          types.Metadata{},
		EnvironmentID:     types.GetEnvironmentID(ctx),
		BaseModel:         types.GetDefaultBaseModel(ctx),
	}

	// Create phases
	phases := make([]*subscription.SchedulePhase, 0, len(req.Phases))
	for i, phaseInput := range req.Phases {
		// Convert line items to the domain model type
		lineItems := make([]types.SchedulePhaseLineItem, 0, len(phaseInput.LineItems))
		for _, item := range phaseInput.LineItems {
			lineItems = append(lineItems, types.SchedulePhaseLineItem{
				PriceID:     item.PriceID,
				Quantity:    item.Quantity,
				DisplayName: item.DisplayName,
				Metadata:    types.Metadata(item.Metadata),
			})
		}

		// Convert credit grants to the domain model type
		creditGrants := make([]types.SchedulePhaseCreditGrant, 0, len(phaseInput.CreditGrants))
		for _, grant := range phaseInput.CreditGrants {
			creditGrants = append(creditGrants, types.SchedulePhaseCreditGrant{
				Name:                   grant.Name,
				Scope:                  grant.Scope,
				PlanID:                 grant.PlanID,
				Credits:                grant.Credits,
				Cadence:                grant.Cadence,
				Period:                 grant.Period,
				PeriodCount:            grant.PeriodCount,
				ExpirationType:         grant.ExpirationType,
				ExpirationDuration:     grant.ExpirationDuration,
				ExpirationDurationUnit: grant.ExpirationDurationUnit,
				Priority:               grant.Priority,
				Metadata:               grant.Metadata,
			})
		}

		phase := &subscription.SchedulePhase{
			ID:               types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_SCHEDULE_PHASE),
			ScheduleID:       schedule.ID,
			PhaseIndex:       i,
			StartDate:        phaseInput.StartDate,
			EndDate:          phaseInput.EndDate,
			CommitmentAmount: &phaseInput.CommitmentAmount,
			OverageFactor:    &phaseInput.OverageFactor,
			LineItems:        lineItems,
			CreditGrants:     creditGrants,
			Metadata:         phaseInput.Metadata,
			EnvironmentID:    types.GetEnvironmentID(ctx),
			BaseModel:        types.GetDefaultBaseModel(ctx),
		}
		phases = append(phases, phase)
	}

	// Create the schedule with phases
	err = s.SubscriptionScheduleRepo.CreateWithPhases(ctx, schedule, phases)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to create subscription schedule").
			WithReportableDetails(map[string]interface{}{
				"subscription_id": req.SubscriptionID,
				"phase_count":     len(phases),
			}).
			Mark(ierr.ErrDatabase)
	}

	// Set the phases to the schedule before returning
	schedule.Phases = phases
	return dto.SubscriptionScheduleResponseFromDomain(schedule), nil
}

// GetSubscriptionSchedule gets a subscription schedule by ID
func (s *subscriptionService) GetSubscriptionSchedule(ctx context.Context, id string) (*dto.SubscriptionScheduleResponse, error) {
	if s.SubscriptionScheduleRepo == nil {
		return nil, ierr.NewError("subscription repository does not support schedules").
			WithHint("Schedule functionality is not supported").
			Mark(ierr.ErrInternal)
	}

	schedule, err := s.SubscriptionScheduleRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return dto.SubscriptionScheduleResponseFromDomain(schedule), nil
}

// GetScheduleBySubscriptionID gets a subscription schedule by subscription ID
func (s *subscriptionService) GetScheduleBySubscriptionID(ctx context.Context, subscriptionID string) (*dto.SubscriptionScheduleResponse, error) {
	// If repository doesn't support schedules, return nil instead of error
	// This allows graceful fallback for backward compatibility
	if s.SubscriptionScheduleRepo == nil {
		s.Logger.Warnw("subscription schedule repository is not configured",
			"subscription_id", subscriptionID)
		return nil, nil
	}

	schedule, err := s.SubscriptionScheduleRepo.GetBySubscriptionID(ctx, subscriptionID)
	if err != nil {
		// Not found is a valid response - the subscription may not have a schedule
		if ierr.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	if schedule == nil {
		return nil, nil
	}

	return dto.SubscriptionScheduleResponseFromDomain(schedule), nil
}

// UpdateSubscriptionSchedule updates a subscription schedule
func (s *subscriptionService) UpdateSubscriptionSchedule(ctx context.Context, id string, req *dto.UpdateSubscriptionScheduleRequest) (*dto.SubscriptionScheduleResponse, error) {
	if s.SubscriptionScheduleRepo == nil {
		return nil, ierr.NewError("subscription repository does not support schedules").
			WithHint("Schedule functionality is not supported").
			Mark(ierr.ErrInternal)
	}

	// Get the current schedule
	schedule, err := s.SubscriptionScheduleRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Update the fields
	if req.Status != "" {
		schedule.ScheduleStatus = req.Status
	}

	if req.EndBehavior != "" {
		schedule.EndBehavior = req.EndBehavior
	}

	// Update in the database
	if err := s.SubscriptionScheduleRepo.Update(ctx, schedule); err != nil {
		return nil, err
	}

	// Get fresh data
	updatedSchedule, err := s.SubscriptionScheduleRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return dto.SubscriptionScheduleResponseFromDomain(updatedSchedule), nil
}

// createScheduleFromPhases creates a schedule and its phases for a subscription
func (s *subscriptionService) createScheduleFromPhases(ctx context.Context, sub *subscription.Subscription, phaseInputs []dto.SubscriptionSchedulePhaseInput) (*subscription.SubscriptionSchedule, error) {
	// Create the schedule
	schedule := &subscription.SubscriptionSchedule{
		ID:                types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_SCHEDULE),
		SubscriptionID:    sub.ID,
		ScheduleStatus:    types.ScheduleStatusActive,
		CurrentPhaseIndex: 0,
		EndBehavior:       types.EndBehaviorRelease,
		StartDate:         sub.StartDate,
		Metadata:          types.Metadata{},
		EnvironmentID:     types.GetEnvironmentID(ctx),
		BaseModel:         types.GetDefaultBaseModel(ctx),
	}

	// Create phases
	phases := make([]*subscription.SchedulePhase, 0, len(phaseInputs))
	for i, phaseInput := range phaseInputs {
		// Convert line items to the domain model type
		lineItems := make([]types.SchedulePhaseLineItem, 0, len(phaseInput.LineItems))
		for _, item := range phaseInput.LineItems {
			lineItems = append(lineItems, types.SchedulePhaseLineItem{
				PriceID:     item.PriceID,
				Quantity:    item.Quantity,
				DisplayName: item.DisplayName,
				Metadata:    item.Metadata,
			})
		}

		// Convert credit grants to the domain model type
		creditGrants := make([]types.SchedulePhaseCreditGrant, 0, len(phaseInput.CreditGrants))
		for _, grant := range phaseInput.CreditGrants {
			creditGrants = append(creditGrants, types.SchedulePhaseCreditGrant{
				Name:                   grant.Name,
				Scope:                  grant.Scope,
				PlanID:                 grant.PlanID,
				Credits:                grant.Credits,
				Cadence:                grant.Cadence,
				Period:                 grant.Period,
				PeriodCount:            grant.PeriodCount,
				ExpirationType:         grant.ExpirationType,
				ExpirationDuration:     grant.ExpirationDuration,
				ExpirationDurationUnit: grant.ExpirationDurationUnit,
				Priority:               grant.Priority,
				Metadata:               grant.Metadata,
			})
		}

		phase := &subscription.SchedulePhase{
			ID:               types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_SCHEDULE_PHASE),
			ScheduleID:       schedule.ID,
			PhaseIndex:       i,
			StartDate:        phaseInput.StartDate,
			EndDate:          phaseInput.EndDate,
			CommitmentAmount: &phaseInput.CommitmentAmount,
			OverageFactor:    &phaseInput.OverageFactor,
			LineItems:        lineItems,
			CreditGrants:     creditGrants,
			Metadata:         phaseInput.Metadata,
			EnvironmentID:    types.GetEnvironmentID(ctx),
			BaseModel:        types.GetDefaultBaseModel(ctx),
		}
		phases = append(phases, phase)
	}

	// Create the schedule with phases
	err := s.SubscriptionScheduleRepo.CreateWithPhases(ctx, schedule, phases)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to create subscription schedule").
			WithReportableDetails(map[string]interface{}{
				"subscription_id": sub.ID,
				"phase_count":     len(phases),
			}).
			Mark(ierr.ErrDatabase)
	}

	// Set the phases to the schedule before returning
	schedule.Phases = phases
	return schedule, nil
}

// AddSchedulePhase adds a new phase to an existing subscription schedule
func (s *subscriptionService) AddSchedulePhase(ctx context.Context, scheduleID string, req *dto.AddSchedulePhaseRequest) (*dto.SubscriptionScheduleResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	if s.SubscriptionScheduleRepo == nil {
		return nil, ierr.NewError("subscription repository does not support schedules").
			WithHint("Schedule functionality is not supported").
			Mark(ierr.ErrInternal)
	}

	// Get the existing schedule with its phases
	schedule, err := s.SubscriptionScheduleRepo.Get(ctx, scheduleID)
	if err != nil {
		return nil, err
	}

	// Get the subscription to validate against its dates
	existingSubscription, _, err := s.SubRepo.GetWithLineItems(ctx, schedule.SubscriptionID)
	if err != nil {
		return nil, err
	}

	// Load existing phases
	existingPhases, err := s.SubscriptionScheduleRepo.ListPhases(ctx, scheduleID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to list existing phases").
			Mark(ierr.ErrDatabase)
	}

	// Validate that the new phase's start date is not before subscription start date
	if req.Phase.StartDate.Before(existingSubscription.StartDate) {
		return nil, ierr.NewError("phase start date cannot be before subscription start date").
			WithHint("The phase must start on or after the subscription start date").
			WithReportableDetails(map[string]interface{}{
				"subscription_start_date": existingSubscription.StartDate,
				"phase_start_date":        req.Phase.StartDate,
			}).
			Mark(ierr.ErrValidation)
	}

	// If subscription has an end date, validate the phase doesn't extend beyond it
	if existingSubscription.EndDate != nil && req.Phase.EndDate != nil && req.Phase.EndDate.After(*existingSubscription.EndDate) {
		return nil, ierr.NewError("phase end date cannot be after subscription end date").
			WithHint("The phase must end on or before the subscription end date").
			WithReportableDetails(map[string]interface{}{
				"subscription_end_date": existingSubscription.EndDate,
				"phase_end_date":        req.Phase.EndDate,
			}).
			Mark(ierr.ErrValidation)
	}

	// Sort phases by start date
	sort.Slice(existingPhases, func(i, j int) bool {
		return existingPhases[i].StartDate.Before(existingPhases[j].StartDate)
	})

	// SIMPLIFIED APPROACH: Only allow adding phases at the end of existing phases
	if len(existingPhases) > 0 {
		lastPhase := existingPhases[len(existingPhases)-1]

		// Check if the last phase has an end date
		if lastPhase.EndDate == nil {
			return nil, ierr.NewError("cannot add phase after an open-ended phase").
				WithHint("The last phase must have an end date to add a new phase").
				Mark(ierr.ErrValidation)
		}

		// Verify the new phase starts after the last existing phase ends
		if !req.Phase.StartDate.After(*lastPhase.EndDate) {
			return nil, ierr.NewError("new phase must start after the end of the last phase").
				WithHint("Phase cannot overlap with existing phases. Add phases only at the end of the schedule").
				WithReportableDetails(map[string]interface{}{
					"last_phase_end_date":  lastPhase.EndDate,
					"new_phase_start_date": req.Phase.StartDate,
				}).
				Mark(ierr.ErrValidation)
		}
	}

	// Create the new phase
	newPhase := &subscription.SchedulePhase{
		ID:               types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_SCHEDULE_PHASE),
		ScheduleID:       scheduleID,
		PhaseIndex:       len(existingPhases), // Add as the next phase
		StartDate:        req.Phase.StartDate,
		EndDate:          req.Phase.EndDate,
		CommitmentAmount: &req.Phase.CommitmentAmount,
		OverageFactor:    &req.Phase.OverageFactor,
		Metadata:         req.Phase.Metadata,
		EnvironmentID:    types.GetEnvironmentID(ctx),
		BaseModel:        types.GetDefaultBaseModel(ctx),
	}

	// Convert line items
	if len(req.Phase.LineItems) > 0 {
		lineItems := make([]types.SchedulePhaseLineItem, 0, len(req.Phase.LineItems))
		for _, item := range req.Phase.LineItems {
			lineItems = append(lineItems, types.SchedulePhaseLineItem{
				PriceID:     item.PriceID,
				Quantity:    item.Quantity,
				DisplayName: item.DisplayName,
				Metadata:    types.Metadata(item.Metadata),
			})
		}
		newPhase.LineItems = lineItems
	}

	// Convert credit grants
	if len(req.Phase.CreditGrants) > 0 {
		creditGrants := make([]types.SchedulePhaseCreditGrant, 0, len(req.Phase.CreditGrants))
		for _, grant := range req.Phase.CreditGrants {
			creditGrants = append(creditGrants, types.SchedulePhaseCreditGrant{
				Name:                   grant.Name,
				Scope:                  grant.Scope,
				PlanID:                 grant.PlanID,
				Credits:                grant.Credits,
				Cadence:                grant.Cadence,
				Period:                 grant.Period,
				PeriodCount:            grant.PeriodCount,
				ExpirationType:         grant.ExpirationType,
				ExpirationDuration:     grant.ExpirationDuration,
				ExpirationDurationUnit: grant.ExpirationDurationUnit,
				Priority:               grant.Priority,
				Metadata:               grant.Metadata,
			})
		}
		newPhase.CreditGrants = creditGrants
	}

	// Create the new phase
	err = s.SubscriptionScheduleRepo.CreatePhase(ctx, newPhase)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to add phase to subscription schedule").
			WithReportableDetails(map[string]interface{}{
				"schedule_id": scheduleID,
			}).
			Mark(ierr.ErrDatabase)
	}

	// Update the schedule with the latest phase count
	schedule.UpdatedAt = time.Now()
	schedule.UpdatedBy = types.GetUserID(ctx)
	err = s.SubscriptionScheduleRepo.Update(ctx, schedule)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to update subscription schedule").
			WithReportableDetails(map[string]interface{}{
				"schedule_id": scheduleID,
			}).
			Mark(ierr.ErrDatabase)
	}

	// Get the updated schedule to return in the response
	updatedSchedule, err := s.SubscriptionScheduleRepo.Get(ctx, scheduleID)
	if err != nil {
		return nil, err
	}

	return dto.SubscriptionScheduleResponseFromDomain(updatedSchedule), nil
}

// AddSubscriptionPhase adds a new phase to a subscription, creating a schedule if needed
// This is more user-friendly than AddSchedulePhase as it works directly with subscription IDs
func (s *subscriptionService) AddSubscriptionPhase(ctx context.Context, subscriptionID string, req *dto.AddSchedulePhaseRequest) (*dto.SubscriptionScheduleResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	if s.SubscriptionScheduleRepo == nil {
		return nil, ierr.NewError("subscription repository does not support schedules").
			WithHint("Schedule functionality is not supported").
			Mark(ierr.ErrInternal)
	}

	// Get the subscription
	existingSubscription, lineItems, err := s.SubRepo.GetWithLineItems(ctx, subscriptionID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get subscription").
			WithReportableDetails(map[string]interface{}{
				"subscription_id": subscriptionID,
			}).
			Mark(ierr.ErrNotFound)
	}

	// Validate that the new phase's start date is not before subscription start date
	if req.Phase.StartDate.Before(existingSubscription.StartDate) {
		return nil, ierr.NewError("phase start date cannot be before subscription start date").
			WithHint("The phase must start on or after the subscription start date").
			WithReportableDetails(map[string]interface{}{
				"subscription_start_date": existingSubscription.StartDate,
				"phase_start_date":        req.Phase.StartDate,
			}).
			Mark(ierr.ErrValidation)
	}

	if req.Phase.EndDate != nil && existingSubscription.EndDate != nil && req.Phase.StartDate.After(*existingSubscription.EndDate) {
		return nil, ierr.NewError("phase start date cannot be after subscription end date").
			WithHint("The phase must start before the subscription end date").
			WithReportableDetails(map[string]interface{}{
				"subscription_end_date": existingSubscription.EndDate,
				"phase_start_date":      req.Phase.StartDate,
			}).
			Mark(ierr.ErrValidation)
	}

	if existingSubscription.EndDate != nil && req.Phase.EndDate != nil && req.Phase.EndDate.After(*existingSubscription.EndDate) {
		return nil, ierr.NewError("phase end date cannot be after subscription end date").
			WithHint("The phase must end on or before the subscription end date").
			WithReportableDetails(map[string]interface{}{
				"subscription_end_date": existingSubscription.EndDate,
				"phase_end_date":        req.Phase.EndDate,
			}).
			Mark(ierr.ErrValidation)
	}

	// Check for existing schedule
	schedule, err := s.SubscriptionScheduleRepo.GetBySubscriptionID(ctx, subscriptionID)
	if err != nil && !ierr.IsNotFound(err) {
		// Error other than "not found"
		return nil, ierr.WithError(err).
			WithHint("Failed to check for existing schedule").
			Mark(ierr.ErrDatabase)
	}

	// No schedule exists, we need to create one
	if schedule == nil || err != nil {
		s.Logger.Infow("creating new schedule for subscription",
			"subscription_id", subscriptionID)

		// Create a schedule with initial phase from subscription start to new phase start
		initialPhases := []dto.SubscriptionSchedulePhaseInput{}

		// Only add initial phase if new phase doesn't start exactly at subscription start
		if !req.Phase.StartDate.Equal(existingSubscription.StartDate) {
			// Build a default initial phase based on subscription's current values
			initialPhase := dto.SubscriptionSchedulePhaseInput{
				BillingCycle:     existingSubscription.BillingCycle,
				StartDate:        existingSubscription.StartDate,
				EndDate:          &req.Phase.StartDate,
				CommitmentAmount: lo.FromPtr(existingSubscription.CommitmentAmount),
				OverageFactor:    lo.FromPtr(existingSubscription.OverageFactor),
				Metadata:         map[string]string{"created_by": "system", "reason": "auto-created-initial-phase"},
			}

			// Add line items from subscription
			for _, item := range lineItems {
				initialPhase.LineItems = append(initialPhase.LineItems, dto.SubscriptionLineItemRequest{
					PriceID:     item.PriceID,
					Quantity:    item.Quantity,
					DisplayName: item.DisplayName,
					Metadata:    item.Metadata,
				})
			}

			initialPhases = append(initialPhases, initialPhase)
		}

		// Add the new phase
		initialPhases = append(initialPhases, req.Phase)

		// Create the schedule with both phases
		createReq := &dto.CreateSubscriptionScheduleRequest{
			SubscriptionID: subscriptionID,
			EndBehavior:    types.EndBehaviorRelease,
			Phases:         initialPhases,
		}

		// Create the schedule
		return s.CreateSubscriptionSchedule(ctx, createReq)
	}

	// Schedule exists, add the phase to it
	return s.AddSchedulePhase(ctx, schedule.ID, req)
}

// TODO: This is not used anywhere
// HandleSubscriptionStateChange handles subscription state changes for credit grants
func (s *subscriptionService) HandleSubscriptionStateChange(ctx context.Context, subscriptionID string, oldStatus, newStatus types.SubscriptionStatus) error {
	s.Logger.Infow("handling subscription state change for credit grants",
		"subscription_id", subscriptionID,
		"old_status", oldStatus,
		"new_status", newStatus)

	switch {
	case newStatus == types.SubscriptionStatusActive && oldStatus != types.SubscriptionStatusActive:
		return s.handleSubscriptionActivation(ctx, subscriptionID)

	case newStatus == types.SubscriptionStatusCancelled:
		return s.handleSubscriptionCancellation(ctx, subscriptionID)

	case newStatus == types.SubscriptionStatusPaused:
		return s.handleSubscriptionPause(ctx, subscriptionID)

	case oldStatus == types.SubscriptionStatusPaused && newStatus == types.SubscriptionStatusActive:
		return s.handleSubscriptionResume(ctx, subscriptionID)

	default:
		s.Logger.Debugw("no action required for subscription state change",
			"subscription_id", subscriptionID,
			"old_status", oldStatus,
			"new_status", newStatus)
	}

	return nil
}

func (s *subscriptionService) handleSubscriptionActivation(ctx context.Context, subscriptionID string) error {
	// Process any deferred credits and trigger immediate processing for newly active subscription
	return nil
}

func (s *subscriptionService) handleSubscriptionCancellation(ctx context.Context, subscriptionID string) error {
	// Future: Cancel scheduled applications if we implement full application tracking
	s.Logger.Infow("subscription cancelled, future recurring grants will not be processed", "subscription_id", subscriptionID)
	return nil
}

func (s *subscriptionService) handleSubscriptionPause(ctx context.Context, subscriptionID string) error {
	// Future: Defer scheduled applications if we implement full application tracking
	s.Logger.Infow("subscription paused, recurring grants will be deferred", "subscription_id", subscriptionID)
	return nil
}

func (s *subscriptionService) handleSubscriptionResume(ctx context.Context, subscriptionID string) error {
	// Process any missed recurring grants
	return nil
}

// ApplyCouponsToSubscriptionWithLineItems applies both subscription-level and line item-level coupons to a subscription
func (s *subscriptionService) ApplyCouponsToSubscriptionWithLineItems(ctx context.Context, subscriptionID string, subscriptionCoupons []string, lineItemCoupons map[string][]string, lineItems []*subscription.SubscriptionLineItem) error {
	if len(subscriptionCoupons) == 0 && len(lineItemCoupons) == 0 {
		return nil
	}

	s.Logger.Infow("handling subscription and line item coupon associations",
		"subscription_id", subscriptionID,
		"subscription_coupon_count", len(subscriptionCoupons),
		"line_item_coupon_count", len(lineItemCoupons))

	// Create coupon service instance
	couponAssociationService := NewCouponAssociationService(s.ServiceParams)

	// Step 1: Apply subscription-level coupons
	if len(subscriptionCoupons) > 0 {
		err := couponAssociationService.ApplyCouponToSubscription(ctx, subscriptionCoupons, subscriptionID)
		if err != nil {
			return ierr.WithError(err).
				WithHint("Failed to apply subscription-level coupons").
				WithReportableDetails(map[string]interface{}{
					"subscription_id": subscriptionID,
					"coupon_ids":      subscriptionCoupons,
				}).
				Mark(ierr.ErrInternal)
		}

		s.Logger.Infow("successfully applied subscription-level coupons",
			"subscription_id", subscriptionID,
			"coupon_count", len(subscriptionCoupons))
	}

	// Step 2: Apply line item-level coupons
	if len(lineItemCoupons) > 0 {
		priceIDToLineItem := make(map[string]*subscription.SubscriptionLineItem)
		for _, lineItem := range lineItems {
			priceIDToLineItem[lineItem.PriceID] = lineItem
		}

		for priceID, couponIDs := range lineItemCoupons {
			lineItem, ok := priceIDToLineItem[priceID]
			if !ok {
				return ierr.NewError("line item not found").
					WithHint("Please provide a valid price ID").
					WithReportableDetails(map[string]interface{}{
						"price_id": priceID,
					}).
					Mark(ierr.ErrValidation)
			}

			err := couponAssociationService.ApplyCouponToSubscriptionLineItem(ctx, couponIDs, subscriptionID, lineItem.ID)
			if err != nil {
				return ierr.WithError(err).
					WithHint("Failed to apply line item coupons").
					WithReportableDetails(map[string]interface{}{
						"subscription_id": subscriptionID,
						"price_id":        priceID,
						"line_item_id":    lineItem.ID,
						"coupon_ids":      couponIDs,
					}).
					Mark(ierr.ErrInternal)
			}
		}

		s.Logger.Infow("successfully applied line item coupons",
			"subscription_id", subscriptionID,
			"line_item_count", len(lineItemCoupons))
	}

	s.Logger.Infow("successfully applied all coupons to subscription",
		"subscription_id", subscriptionID,
		"subscription_coupon_count", len(subscriptionCoupons),
		"line_item_coupon_count", len(lineItemCoupons))

	return nil
}

// handleSubscriptionAddons processes addons for a subscription
func (s *subscriptionService) handleSubscriptionAddons(
	ctx context.Context,
	subscription *subscription.Subscription,
	addonRequests []dto.AddAddonToSubscriptionRequest,
) error {
	if len(addonRequests) == 0 {
		return nil
	}

	s.Logger.Infow("processing addons for subscription",
		"subscription_id", subscription.ID,
		"addons_count", len(addonRequests))

	// Validate and create subscription addons
	for _, addonReq := range addonRequests {
		// Validate the addon request
		if err := addonReq.Validate(); err != nil {
			return ierr.WithError(err).
				WithHint("Invalid addon request").
				WithReportableDetails(map[string]interface{}{
					"subscription_id": subscription.ID,
					"addon_id":        addonReq.AddonID,
				}).
				Mark(ierr.ErrValidation)
		}

		// Get addon to ensure it's valid and active
		addonService := NewAddonService(s.ServiceParams)
		addonResponse, err := addonService.GetAddon(ctx, addonReq.AddonID)
		if err != nil {
			return ierr.WithError(err).
				WithHint("Addon not found").
				WithReportableDetails(map[string]interface{}{
					"subscription_id": subscription.ID,
					"addon_id":        addonReq.AddonID,
				}).
				Mark(ierr.ErrNotFound)
		}

		if addonResponse.Addon.Status != types.StatusPublished {
			return ierr.NewError("addon is not active").
				WithHint("The addon must be active to add to a subscription").
				WithReportableDetails(map[string]interface{}{
					"subscription_id": subscription.ID,
					"addon_id":        addonReq.AddonID,
					"status":          addonResponse.Addon.Status,
				}).
				Mark(ierr.ErrValidation)
		}

		// Validate and filter prices for the addon using the same pattern as plans
		validPrices, err := s.ValidateAndFilterPricesForSubscription(
			ctx,
			addonReq.AddonID,
			types.PRICE_ENTITY_TYPE_ADDON,
			subscription,
		)
		if err != nil {
			return ierr.WithError(err).
				WithHint("Failed to validate addon prices").
				WithReportableDetails(map[string]interface{}{
					"subscription_id": subscription.ID,
					"addon_id":        addonReq.AddonID,
				}).
				Mark(ierr.ErrValidation)
		}

		s.Logger.Infow("validated addon prices for subscription",
			"subscription_id", subscription.ID,
			"addon_id", addonReq.AddonID,
			"valid_prices_count", len(validPrices))

		// Create subscription addon using the validated prices
		subscriptionAddon, err := s.addAddonToSubscription(ctx, subscription, lo.ToPtr(addonReq))
		if err != nil {
			return ierr.WithError(err).
				WithHint("Failed to add addon to subscription").
				WithReportableDetails(map[string]interface{}{
					"subscription_id": subscription.ID,
					"addon_id":        addonReq.AddonID,
				}).
				Mark(ierr.ErrNotFound)
		}

		s.Logger.Infow("created subscription addon",
			"subscription_id", subscription.ID,
			"addon_id", subscriptionAddon.AddonID,
			"subscription_addon_id", subscriptionAddon.ID,
			"start_date", subscriptionAddon.StartDate,
			"end_date", subscriptionAddon.EndDate,
		)
	}

	return nil
}

// AddAddonToSubscription adds an addon to a subscription
// This is the public facing method for adding an addon to a subscription
func (s *subscriptionService) AddAddonToSubscription(
	ctx context.Context,
	subID string,
	req *dto.AddAddonToSubscriptionRequest,
) (*addonassociation.AddonAssociation, error) {

	sub, lineItems, err := s.SubRepo.GetWithLineItems(ctx, subID)
	if err != nil {
		return nil, err
	}
	sub.LineItems = lineItems

	return s.addAddonToSubscription(ctx, sub, req)
}

// addAddonToSubscription adds an addon to a subscription
func (s *subscriptionService) addAddonToSubscription(
	ctx context.Context,
	sub *subscription.Subscription,
	req *dto.AddAddonToSubscriptionRequest,
) (*addonassociation.AddonAssociation, error) {
	// Validate request
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Get addon via addon service to reuse validations
	addonService := NewAddonService(s.ServiceParams)
	a, err := addonService.GetAddon(ctx, req.AddonID)
	if err != nil {
		return nil, err
	}

	if a.Addon.Status != types.StatusPublished {
		return nil, ierr.NewError("addon is not published").
			WithHint("Cannot add inactive addon to subscription").
			Mark(ierr.ErrValidation)
	}

	// Check if sub exists and is active
	if sub.SubscriptionStatus != types.SubscriptionStatusActive {
		return nil, ierr.NewError("subscription is not active").
			WithHint("Cannot add addon to inactive subscription").
			Mark(ierr.ErrValidation)
	}

	// Check if addon is already added to subscription only for single instance addons
	if a.Addon.Type == types.AddonTypeOnetime {
		filter := types.NewAddonAssociationFilter()
		filter.AddonIDs = []string{req.AddonID}
		filter.EntityIDs = []string{sub.ID}
		filter.EntityType = lo.ToPtr(types.AddonAssociationEntityTypeSubscription)
		filter.Limit = lo.ToPtr(1)

		existingAddons, err := s.AddonAssociationRepo.List(ctx, filter)
		if err != nil {
			return nil, err
		}

		if len(existingAddons) > 0 {
			return nil, ierr.NewError("addon is already added to subscription").
				WithHint("Cannot add addon to subscription that already has an active instance").
				Mark(ierr.ErrValidation)
		}
	}

	// Validate and filter prices for the addon using the same pattern as plans
	validPrices, err := s.ValidateAndFilterPricesForSubscription(ctx, req.AddonID, types.PRICE_ENTITY_TYPE_ADDON, sub)
	if err != nil {
		return nil, err
	}

	// Create subscription addon
	addonAssociation := req.ToAddonAssociation(
		ctx,
		sub.ID,
		types.AddonAssociationEntityTypeSubscription,
	)

	// Track existing meter IDs
	meterIDMap := make(map[string]bool)
	for _, item := range sub.LineItems {
		if item.MeterID != "" {
			meterIDMap[item.MeterID] = true
		}
	}

	// Check for meter ID conflicts in new addon prices
	for _, price := range validPrices {
		if price.Price.MeterID != "" {
			if meterIDMap[price.Price.MeterID] {
				return nil, ierr.NewError("duplicate meter ID found in addon prices").
					WithHint("Each price must have a unique meter ID across the subscription").
					WithReportableDetails(map[string]interface{}{
						"subscription_id": sub.ID,
						"addon_id":        req.AddonID,
						"meter_id":        price.Price.MeterID,
					}).
					Mark(ierr.ErrValidation)
			}
			meterIDMap[price.Price.MeterID] = true
		}
	}

	// Create line items for the addon using validated prices
	lineItems := make([]*subscription.SubscriptionLineItem, 0, len(validPrices))
	for _, priceResponse := range validPrices {
		lineItem := s.createLineItemFromPrice(ctx, priceResponse, sub, req.AddonID, a.Addon.Name)
		lineItems = append(lineItems, lineItem)
	}

	err = s.DB.WithTx(ctx, func(ctx context.Context) error {
		// Create subscription addon
		err = s.AddonAssociationRepo.Create(ctx, addonAssociation)
		if err != nil {
			return err
		}

		// Create line items for the addon
		for _, lineItem := range lineItems {
			err = s.SubscriptionLineItemRepo.Create(ctx, lineItem)
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	s.Logger.Infow("added addon to subscription",
		"subscription_id", sub.ID,
		"addon_id", req.AddonID,
		"prices_count", len(validPrices),
		"line_items_count", len(lineItems),
	)

	return addonAssociation, nil
}

// RemoveAddonFromSubscription removes an addon from a subscription
func (s *subscriptionService) RemoveAddonFromSubscription(
	ctx context.Context,
	subscriptionID string,
	addonID string,
	reason string,
) error {
	// Get subscription addon
	filter := types.NewAddonAssociationFilter()
	filter.AddonIDs = []string{addonID}
	filter.EntityIDs = []string{subscriptionID}
	filter.EntityType = lo.ToPtr(types.AddonAssociationEntityTypeSubscription)

	subscriptionAddons, err := s.AddonAssociationRepo.List(ctx, filter)
	if err != nil {
		return err
	}

	var targetAddon *addonassociation.AddonAssociation
	for _, sa := range subscriptionAddons {
		if sa.AddonStatus == types.AddonStatusActive {
			targetAddon = sa
			break
		}
	}

	if targetAddon == nil {
		return ierr.NewError("addon not found on subscription").
			WithHint("Addon is not active on this subscription").
			Mark(ierr.ErrNotFound)
	}

	// Update addon status to cancelled and delete line items in a transaction
	now := time.Now()
	targetAddon.AddonStatus = types.AddonStatusCancelled
	targetAddon.CancellationReason = reason
	targetAddon.CancelledAt = &now
	targetAddon.EndDate = &now

	err = s.DB.WithTx(ctx, func(ctx context.Context) error {
		// Update subscription addon
		err = s.AddonAssociationRepo.Update(ctx, targetAddon)
		if err != nil {
			return err
		}

		// End the corresponding line items for this addon (soft delete approach)
		subscription, err := s.SubRepo.Get(ctx, subscriptionID)
		if err != nil {
			return err
		}

		lineItemsEnded := 0
		for _, lineItem := range subscription.LineItems {
			// Debug logging to understand line item matching
			s.Logger.Infow("checking line item for addon removal",
				"subscription_id", subscriptionID,
				"addon_id", addonID,
				"line_item_id", lineItem.ID,
				"line_item_metadata", lineItem.Metadata)

			// Check metadata for addon_id
			metadataMatch := lineItem.Metadata != nil && lineItem.Metadata["addon_id"] == addonID

			if metadataMatch {
				s.Logger.Infow("found matching line item for addon removal",
					"subscription_id", subscriptionID,
					"addon_id", addonID,
					"line_item_id", lineItem.ID,
					"metadata_match", metadataMatch)

				// End the line item (soft delete approach like Togai)
				lineItem.EndDate = now
				lineItem.Status = types.StatusDeleted

				// Add metadata for audit trail
				if lineItem.Metadata == nil {
					lineItem.Metadata = make(map[string]string)
				}
				lineItem.Metadata["removal_reason"] = reason
				lineItem.Metadata["removed_at"] = now.Format(time.RFC3339)
				lineItem.Metadata["removed_by"] = types.GetUserID(ctx)

				err = s.SubscriptionLineItemRepo.Update(ctx, lineItem)
				if err != nil {
					s.Logger.Errorw("failed to end line item for addon",
						"subscription_id", subscriptionID,
						"addon_id", addonID,
						"line_item_id", lineItem.ID,
						"error", err)
					return err
				}
				lineItemsEnded++
			}
		}

		s.Logger.Infow("ended line items for addon removal",
			"subscription_id", subscriptionID,
			"addon_id", addonID,
			"line_items_ended", lineItemsEnded,
			"removal_reason", reason)

		return nil
	})

	if err != nil {
		return err
	}

	s.Logger.Infow("removed addon from subscription",
		"subscription_id", subscriptionID,
		"addon_id", addonID,
	)

	return nil
}

// createLineItemFromPrice creates a subscription line item from a price for addon additions
func (s *subscriptionService) createLineItemFromPrice(ctx context.Context, priceResponse *dto.PriceResponse, sub *subscription.Subscription, addonID, addonName string) *subscription.SubscriptionLineItem {
	price := priceResponse.Price

	lineItem := &subscription.SubscriptionLineItem{
		ID:             types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM),
		SubscriptionID: sub.ID,
		CustomerID:     sub.CustomerID,
		EntityID:       addonID,
		EntityType:     types.SubscriptionLineItemEntitiyTypeAddon,
		PriceID:        price.ID,
		PriceType:      price.Type,
		Currency:       sub.Currency,
		BillingPeriod:  price.BillingPeriod,
		InvoiceCadence: price.InvoiceCadence,
		TrialPeriod:    0,
		StartDate:      time.Now(),
		EndDate:        time.Time{},
		Metadata: map[string]string{
			"addon_id":        addonID,
			"subscription_id": sub.ID,
			"addon_quantity":  "1",
			"addon_status":    string(types.AddonStatusActive),
		},
		EnvironmentID: sub.EnvironmentID,
		BaseModel:     types.GetDefaultBaseModel(ctx),
	}

	// Set price-related fields
	if price.Type == types.PRICE_TYPE_USAGE && price.MeterID != "" && priceResponse.Meter != nil {
		lineItem.MeterID = price.MeterID
		lineItem.MeterDisplayName = priceResponse.Meter.Name
		lineItem.DisplayName = priceResponse.Meter.Name
		lineItem.Quantity = decimal.Zero
	} else {
		lineItem.DisplayName = addonName
		lineItem.Quantity = decimal.NewFromInt(1)
	}

	return lineItem
}

// ActivateIncompleteSubscription activates a subscription that is in incomplete status
// after the first invoice has been successfully paid
func (s *subscriptionService) ActivateIncompleteSubscription(ctx context.Context, subscriptionID string) error {
	s.Logger.Infow("activating incomplete subscription", "subscription_id", subscriptionID)

	// Get the subscription
	sub, err := s.SubRepo.Get(ctx, subscriptionID)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get subscription").
			WithReportableDetails(map[string]interface{}{
				"subscription_id": subscriptionID,
			}).
			Mark(ierr.ErrDatabase)
	}

	// Check if subscription is in incomplete status
	if sub.SubscriptionStatus != types.SubscriptionStatusIncomplete {
		// If the subscription is not in incomplete status, do nothing
		return nil
	}

	// Update subscription status to active
	sub.SubscriptionStatus = types.SubscriptionStatusActive

	// Update the subscription in database
	err = s.SubRepo.Update(ctx, sub)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to update subscription status").
			WithReportableDetails(map[string]interface{}{
				"subscription_id": subscriptionID,
			}).
			Mark(ierr.ErrDatabase)
	}

	s.Logger.Infow("successfully activated incomplete subscription",
		"subscription_id", subscriptionID,
		"previous_status", types.SubscriptionStatusIncomplete,
		"new_status", types.SubscriptionStatusActive)

	// Publish webhook event for subscription activation
	s.publishInternalWebhookEvent(ctx, types.WebhookEventSubscriptionActivated, subscriptionID)

	return nil
}

// SubscriptionConfig represents the configuration for subscription auto-cancellation
type SubscriptionConfig struct {
	GracePeriodDays         int  `json:"grace_period_days"`
	AutoCancellationEnabled bool `json:"auto_cancellation_enabled"`
}

// GetSubscriptionConfig retrieves the subscription configuration from settings
func (s *subscriptionService) GetSubscriptionConfig(ctx context.Context) (*SubscriptionConfig, error) {
	setting, err := s.SettingsRepo.GetByKey(ctx, "subscription_config")
	if err != nil {
		// If setting not found, return default
		if ierr.IsNotFound(err) {
			s.Logger.Infow("subscription config not found, returning default",
				"tenant_id", types.GetTenantID(ctx), "environment_id", types.GetEnvironmentID(ctx))
			return &SubscriptionConfig{
				GracePeriodDays:         1,
				AutoCancellationEnabled: false,
			}, nil
		}
		return nil, err
	}

	// Extract configuration from the setting value
	config := &SubscriptionConfig{
		GracePeriodDays:         3,
		AutoCancellationEnabled: false,
	}

	// Extract grace_period_days
	if gracePeriodDaysRaw, exists := setting.Value["grace_period_days"]; exists {
		switch v := gracePeriodDaysRaw.(type) {
		case float64:
			config.GracePeriodDays = int(v)
		case int:
			config.GracePeriodDays = v
		}
	}

	// Extract auto_cancellation_enabled
	if autoCancellationEnabledRaw, exists := setting.Value["auto_cancellation_enabled"]; exists {
		if autoCancellationEnabled, ok := autoCancellationEnabledRaw.(bool); ok {
			config.AutoCancellationEnabled = autoCancellationEnabled
		}
	}

	return config, nil
}

// isEligibleForAutoCancellation checks if an active subscription is eligible for auto-cancellation
func (s *subscriptionService) isEligibleForAutoCancellation(ctx context.Context, sub *subscription.Subscription, config *SubscriptionConfig) bool {
	// Check if subscription is active
	if sub.SubscriptionStatus != types.SubscriptionStatusActive {
		return false
	}

	now := time.Now().UTC()

	// Query for unpaid invoices - remove TimeRangeFilter since it filters by period_end, not due_date
	filter := &types.InvoiceFilter{
		SubscriptionID: sub.ID,
		PaymentStatus: []types.PaymentStatus{
			types.PaymentStatusPending,
			types.PaymentStatusFailed,
		},
		// Remove TimeRangeFilter as it filters by period_end, not due_date
	}

	s.Logger.Debugw("fetching unpaid invoices for auto-cancellation eligibility",
		"subscription_id", sub.ID,
		"payment_statuses", filter.PaymentStatus)

	invoices, err := s.InvoiceRepo.List(ctx, filter)
	if err != nil {
		s.Logger.Errorw("failed to fetch invoices for auto-cancellation",
			"subscription_id", sub.ID,
			"error", err)
		return false
	}

	s.Logger.Debugw("found invoices for subscription", 
		"subscription_id", sub.ID,
		"invoice_count", len(invoices))

	// Check each invoice for eligibility criteria
	for _, inv := range invoices {
		// Skip invalid due dates
		if inv.DueDate == nil {
			s.Logger.Warnw("invoice has invalid due date, skipping",
				"subscription_id", sub.ID,
				"invoice_id", inv.ID)
			continue
		}

		// Check amount_remaining (must have outstanding amount)
		if !inv.AmountRemaining.GreaterThan(decimal.Zero) {
			s.Logger.Debugw("invoice has no remaining amount, skipping",
				"subscription_id", sub.ID,
				"invoice_id", inv.ID,
				"amount_remaining", inv.AmountRemaining)
			continue
		}

		// Calculate grace period end time: due_date + grace_period_days
		gracePeriodEndTime := inv.DueDate.AddDate(0, 0, config.GracePeriodDays)

		s.Logger.Debugw("evaluating invoice for auto-cancellation",
			"subscription_id", sub.ID,
			"invoice_id", inv.ID,
			"due_date", inv.DueDate,
			"amount_remaining", inv.AmountRemaining,
			"grace_period_days", config.GracePeriodDays,
			"grace_period_end_time", gracePeriodEndTime,
			"current_time", now,
			"is_past_grace_period", now.After(gracePeriodEndTime))

		// Check if current time is past grace period end
		if now.After(gracePeriodEndTime) {
			s.Logger.Infow("subscription eligible for auto-cancellation",
				"subscription_id", sub.ID,
				"invoice_id", inv.ID,
				"amount_remaining", inv.AmountRemaining,
				"due_date", inv.DueDate,
				"grace_period_end_time", gracePeriodEndTime,
				"current_time", now,
				"payment_status", inv.PaymentStatus)
			return true
		} else {
			s.Logger.Debugw("invoice not past grace period yet",
				"subscription_id", sub.ID,
				"invoice_id", inv.ID,
				"due_date", inv.DueDate,
				"grace_period_end_time", gracePeriodEndTime,
				"days_until_grace_expires", gracePeriodEndTime.Sub(now).Hours()/24)
		}
	}

	s.Logger.Debugw("subscription not eligible for auto-cancellation",
		"subscription_id", sub.ID,
		"reason", "no invoices past grace period")

	return false
}

// ProcessAutoCancellationSubscriptions processes subscriptions that are eligible for auto-cancellation
func (s *subscriptionService) ProcessAutoCancellationSubscriptions(ctx context.Context) error {
	s.Logger.Infow("starting auto-cancellation processing")

	// Get subscription configuration
	config, err := s.GetSubscriptionConfig(ctx)
	if err != nil {
		s.Logger.Errorw("failed to get subscription config", "error", err)
		return err
	}

	// Early return if auto-cancellation is disabled at tenant level
	if !config.AutoCancellationEnabled {
		s.Logger.Infow("auto-cancellation is disabled for tenant, skipping",
			"tenant_id", types.GetTenantID(ctx),
			"environment_id", types.GetEnvironmentID(ctx))
		return nil
	}

	// Get ONLY ACTIVE subscriptions (skip cancelled, paused, trialing, etc.)
	filter := &types.SubscriptionFilter{
		SubscriptionStatus: []types.SubscriptionStatus{types.SubscriptionStatusActive},
	}

	s.Logger.Infow("fetching ONLY ACTIVE subscriptions for auto-cancellation",
		"subscription_status_filter", "ACTIVE")

	subscriptions, err := s.SubRepo.List(ctx, filter)
	if err != nil {
		s.Logger.Errorw("failed to get subscriptions for auto-cancellation", "error", err)
		return err
	}

	s.Logger.Infow("found subscriptions for auto-cancellation check",
		"total_subscriptions", len(subscriptions))

	// Log subscription count for monitoring
	s.Logger.Infow("found subscriptions for auto-cancellation processing",
		"total_subscriptions", len(subscriptions))

	canceledCount := 0
	failedCount := 0

	// Process each subscription
	for _, sub := range subscriptions {

		if s.isEligibleForAutoCancellation(ctx, sub, config) {
			s.Logger.Infow("auto-cancelling subscription",
				"subscription_id", sub.ID,
				"grace_period_days", config.GracePeriodDays)

			// Cancel the subscription
			if err := s.CancelSubscription(ctx, sub.ID, false); err != nil {
				s.Logger.Errorw("failed to auto-cancel subscription",
					"subscription_id", sub.ID,
					"error", err)
				failedCount++
				continue
			}

			canceledCount++

			// Log audit trail (PRD requirement)
			s.Logger.Infow("successfully auto-canceled subscription",
				"subscription_id", sub.ID,
				"reason", "grace_period_expired",
				"grace_period_days", config.GracePeriodDays,
				"canceled_by", "auto_cancellation_system",
				"tenant_id", sub.TenantID,
				"environment_id", sub.EnvironmentID)
		}
	}

	s.Logger.Infow("completed auto-cancellation processing",
		"total_subscriptions", len(subscriptions),
		"canceled_count", canceledCount,
		"failed_count", failedCount)

	return nil
}
