package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/proration"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

type subscriptionChangeService struct {
	ServiceParams
	subscriptionService SubscriptionService
	prorationService    proration.Service
	invoiceService      InvoiceService
	planService         PlanService
}

// NewsubscriptionChangeService creates a new subscription plan change service
func NewSubscriptionChangeService(params ServiceParams) subscription.SubscriptionChangeService {
	return &subscriptionChangeService{
		ServiceParams:       params,
		subscriptionService: NewSubscriptionService(params),
		prorationService:    NewProrationService(params),
		invoiceService:      NewInvoiceService(params),
		planService:         NewPlanService(params),
	}
}

// UpgradeSubscription upgrades a subscription to a higher plan immediately
func (s *subscriptionChangeService) UpgradeSubscription(
	ctx context.Context,
	subscriptionID string,
	req *subscription.UpgradeSubscriptionRequest,
) (*subscription.SubscriptionPlanChangeResult, error) {
	s.Logger.Infow("starting subscription upgrade",
		"subscription_id", subscriptionID,
		"target_plan_id", req.TargetPlanID,
		"proration_behavior", req.ProrationBehavior)

	// Basic validation
	if req.TargetPlanID == "" {
		return nil, ierr.NewError("target_plan_id is required").
			WithHint("Please provide a valid target plan ID").
			Mark(ierr.ErrValidation)
	}

	var result *subscription.SubscriptionPlanChangeResult
	err := s.DB.WithTx(ctx, func(txCtx context.Context) error {
		// 1. Validate subscription state and plan compatibility
		sub, targetPlan, err := s.validatePlanChange(txCtx, subscriptionID, req.TargetPlanID, "upgrade")
		if err != nil {
			return err
		}

		// 2. Calculate proration for the upgrade
		prorationResult, err := s.calculateUpgradeProration(txCtx, sub, targetPlan, req.ProrationBehavior)
		if err != nil {
			return err
		}

		// 3. Execute the upgrade
		result, err = s.executeUpgrade(txCtx, sub, targetPlan, prorationResult, req)
		if err != nil {
			return err
		}

		// 4. Create audit log
		if err := s.createAuditLog(txCtx, sub, targetPlan, "upgrade", prorationResult.NetAmount, req.Metadata); err != nil {
			s.Logger.Warnw("failed to create audit log", "error", err)
			// Don't fail the operation for audit log issues
		}

		s.Logger.Infow("subscription upgrade completed successfully",
			"subscription_id", subscriptionID,
			"target_plan_id", req.TargetPlanID,
			"proration_amount", prorationResult.NetAmount)

		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

// DowngradeSubscription downgrades a subscription to a lower plan
func (s *subscriptionChangeService) DowngradeSubscription(
	ctx context.Context,
	subscriptionID string,
	req *subscription.DowngradeSubscriptionRequest,
) (*subscription.SubscriptionPlanChangeResult, error) {
	s.Logger.Infow("starting subscription downgrade",
		"subscription_id", subscriptionID,
		"target_plan_id", req.TargetPlanID,
		"effective_at_period_end", req.EffectiveAtPeriodEnd)

	// Basic validation
	if req.TargetPlanID == "" {
		return nil, ierr.NewError("target_plan_id is required").
			WithHint("Please provide a valid target plan ID").
			Mark(ierr.ErrValidation)
	}

	if req.EffectiveAtPeriodEnd {
		return s.scheduleDowngradeAtPeriodEnd(ctx, subscriptionID, req)
	}
	return s.processImmediateDowngrade(ctx, subscriptionID, req)
}

// PreviewPlanChange previews the impact of a plan change without executing it
func (s *subscriptionChangeService) PreviewPlanChange(
	ctx context.Context,
	subscriptionID string,
	req *subscription.PreviewPlanChangeRequest,
) (*subscription.PlanChangePreviewResult, error) {
	s.Logger.Infow("previewing plan change",
		"subscription_id", subscriptionID,
		"target_plan_id", req.TargetPlanID)

	// Basic validation
	if req.TargetPlanID == "" {
		return nil, ierr.NewError("target_plan_id is required").
			WithHint("Please provide a valid target plan ID").
			Mark(ierr.ErrValidation)
	}

	// Set default effective date if not provided
	if req.EffectiveDate == nil {
		now := time.Now()
		req.EffectiveDate = &now
	}

	// Get subscription and target plan
	sub, targetPlan, err := s.validatePlanChange(ctx, subscriptionID, req.TargetPlanID, "preview")
	if err != nil {
		return nil, err
	}

	// Calculate proration preview
	prorationResult, err := s.calculateProrationPreview(ctx, sub, targetPlan, req.ProrationBehavior)
	if err != nil {
		return nil, err
	}

	// Build preview result
	preview := &subscription.PlanChangePreviewResult{
		CurrentAmount:   s.calculateCurrentSubscriptionAmount(ctx, sub),
		NewAmount:       s.calculateNewSubscriptionAmount(ctx, targetPlan),
		ProrationAmount: prorationResult.NetAmount,
		EffectiveDate:   *req.EffectiveDate,
		LineItems:       s.buildProrationLineItemPreviews(prorationResult),
	}

	s.Logger.Infow("plan change preview completed",
		"subscription_id", subscriptionID,
		"proration_amount", preview.ProrationAmount)

	return preview, nil
}

// CancelPendingPlanChange cancels any pending plan changes for a subscription
func (s *subscriptionChangeService) CancelPendingPlanChange(
	ctx context.Context,
	subscriptionID string,
) error {
	s.Logger.Infow("cancelling pending plan changes", "subscription_id", subscriptionID)

	// Get subscription
	sub, err := s.SubRepo.Get(ctx, subscriptionID)
	if err != nil {
		return ierr.NewErrorf("failed to get subscription: %v", err).
			WithHint("Check if the subscription ID is valid").
			Mark(ierr.ErrNotFound)
	}

	// For Phase 1, check metadata for pending changes
	if sub.Metadata == nil || sub.Metadata["pending_plan_id"] == "" {
		return ierr.NewError("no pending plan changes found").
			WithHint("This subscription has no pending plan changes to cancel").
			Mark(ierr.ErrNotFound)
	}

	return s.DB.WithTx(ctx, func(txCtx context.Context) error {
		// Clear pending plan fields from metadata
		delete(sub.Metadata, "pending_plan_id")
		delete(sub.Metadata, "pending_plan_change_date")
		delete(sub.Metadata, "pending_plan_change_reason")
		sub.Version++

		if err := s.SubRepo.Update(txCtx, sub); err != nil {
			return ierr.NewErrorf("failed to update subscription: %v", err).
				WithHint("Failed to clear pending plan change").
				Mark(ierr.ErrSystem)
		}

		s.Logger.Infow("pending plan change cancelled successfully", "subscription_id", subscriptionID)
		return nil
	})
}

// GetPlanChangeHistory returns the history of plan changes for a subscription
func (s *subscriptionChangeService) GetPlanChangeHistory(
	ctx context.Context,
	subscriptionID string,
) ([]*subscription.PlanChangeAuditLog, error) {
	// TODO: Implement audit log retrieval
	// For now, return empty slice as audit logs are not yet implemented
	return []*subscription.PlanChangeAuditLog{}, nil
}

// Helper methods

// validatePlanChange validates the subscription state and plan compatibility
func (s *subscriptionChangeService) validatePlanChange(
	ctx context.Context,
	subscriptionID, targetPlanID, changeType string,
) (*subscription.Subscription, *plan.Plan, error) {
	// Get subscription
	sub, err := s.SubRepo.Get(ctx, subscriptionID)
	if err != nil {
		return nil, nil, ierr.NewErrorf("failed to get subscription: %v", err).
			WithHint("Check if the subscription ID is valid").
			Mark(ierr.ErrNotFound)
	}

	// Validate subscription state
	if err := s.validateSubscriptionState(sub); err != nil {
		return nil, nil, err
	}

	// Get target plan
	targetPlan, err := s.PlanRepo.Get(ctx, targetPlanID)
	if err != nil {
		return nil, nil, ierr.NewErrorf("failed to get target plan: %v", err).
			WithHint("Check if the target plan ID is valid").
			Mark(ierr.ErrNotFound)
	}

	// Validate plan compatibility
	if err := s.validatePlanCompatibility(ctx, sub, targetPlan, changeType); err != nil {
		return nil, nil, err
	}

	return sub, targetPlan, nil
}

// validateSubscriptionState validates that the subscription can be changed
func (s *subscriptionChangeService) validateSubscriptionState(sub *subscription.Subscription) error {
	// Check subscription status
	if sub.SubscriptionStatus != types.SubscriptionStatusActive {
		return ierr.NewErrorf("subscription is not active: %s", sub.SubscriptionStatus).
			WithHint("Only active subscriptions can change plans").
			Mark(ierr.ErrValidation)
	}

	// For Phase 1, we'll use metadata to track pending changes
	// TODO: Add proper pending plan fields to schema in future phases
	if pendingPlan, exists := sub.Metadata["pending_plan_id"]; exists && pendingPlan != "" {
		return ierr.NewError("subscription has pending plan changes").
			WithHint("Cancel pending changes before making new changes").
			Mark(ierr.ErrValidation)
	}

	// Check if subscription is cancelled
	if sub.CancelledAt != nil {
		return ierr.NewError("subscription is cancelled").
			WithHint("Cancelled subscriptions cannot change plans").
			Mark(ierr.ErrValidation)
	}

	return nil
}

// validatePlanCompatibility validates that the target plan is compatible
func (s *subscriptionChangeService) validatePlanCompatibility(
	ctx context.Context,
	sub *subscription.Subscription,
	targetPlan *plan.Plan,
	changeType string,
) error {
	// Check if trying to change to the same plan
	if sub.PlanID == targetPlan.ID {
		return ierr.NewError("target plan is the same as current plan").
			WithHint("Cannot change to the same plan").
			Mark(ierr.ErrValidation)
	}

	// For Phase 1, we'll do basic validation
	// TODO: Add proper plan field validation when plan schema is available
	// Check currency compatibility would go here
	// Check billing period compatibility would go here

	// Additional validation can be added here for:
	// - Plan family compatibility
	// - Feature dependencies
	// - Geographic restrictions
	// - Customer tier restrictions

	return nil
}

// calculateUpgradeProration calculates proration for an upgrade
func (s *subscriptionChangeService) calculateUpgradeProration(
	ctx context.Context,
	sub *subscription.Subscription,
	targetPlan *plan.Plan,
	prorationBehavior types.ProrationBehavior,
) (*proration.ProrationResult, error) {
	if prorationBehavior == types.ProrationBehaviorNone {
		return &proration.ProrationResult{
			NetAmount: decimal.Zero,
		}, nil
	}

	// For Phase 1, we'll use simplified proration calculation
	// TODO: Add proper current plan comparison in future phases

	// Calculate time-based proration
	prorationParams := proration.ProrationParams{
		SubscriptionID:     sub.ID,
		Action:             types.ProrationActionUpgrade,
		CurrentPeriodStart: sub.CurrentPeriodStart,
		CurrentPeriodEnd:   sub.CurrentPeriodEnd,
		ProrationDate:      time.Now(),
		ProrationBehavior:  prorationBehavior,
		CustomerTimezone:   sub.CustomerTimezone,
		Currency:           sub.Currency,
	}

	return s.prorationService.CalculateProration(ctx, prorationParams)
}

// calculateProrationPreview calculates proration for preview purposes
func (s *subscriptionChangeService) calculateProrationPreview(
	ctx context.Context,
	sub *subscription.Subscription,
	targetPlan *plan.Plan,
	prorationBehavior types.ProrationBehavior,
) (*proration.ProrationResult, error) {
	// For preview, use the same calculation as upgrade
	return s.calculateUpgradeProration(ctx, sub, targetPlan, prorationBehavior)
}

// executeUpgrade executes the actual upgrade operation
func (s *subscriptionChangeService) executeUpgrade(
	ctx context.Context,
	sub *subscription.Subscription,
	targetPlan *plan.Plan,
	prorationResult *proration.ProrationResult,
	req *subscription.UpgradeSubscriptionRequest,
) (*subscription.SubscriptionPlanChangeResult, error) {
	// 1. Update subscription plan and related fields
	now := time.Now()
	previousPlanID := sub.PlanID // Store before updating

	// Update core subscription fields
	sub.PlanID = targetPlan.ID
	sub.UpdatedAt = now
	sub.Version++

	// Store comprehensive plan change info in metadata
	if sub.Metadata == nil {
		sub.Metadata = make(map[string]string)
	}

	// Store plan change history
	sub.Metadata["last_plan_change"] = now.Format(time.RFC3339)
	sub.Metadata["plan_change_reason"] = "upgrade"
	sub.Metadata["previous_plan_id"] = previousPlanID
	sub.Metadata["plan_change_count"] = s.incrementPlanChangeCount(sub.Metadata)

	// Store proration information
	sub.Metadata["last_proration_amount"] = prorationResult.NetAmount.String()
	sub.Metadata["last_proration_behavior"] = string(req.ProrationBehavior)

	// Store request metadata if provided
	if req.Metadata != nil {
		for key, value := range req.Metadata {
			sub.Metadata["request_"+key] = value
		}
	}

	// Update billing anchor if needed (keep existing billing cycle for upgrades)
	// The billing anchor remains the same to maintain consistent billing dates
	// This ensures customers continue to be billed on their original schedule

	// Reset usage counters for usage-based items (Stripe behavior)
	// This ensures usage tracking starts fresh with the new plan
	if err := s.resetUsageCountersForPlanChange(ctx, sub, now); err != nil {
		s.Logger.Warnw("failed to reset usage counters", "error", err, "subscription_id", sub.ID)
		// Don't fail the operation for usage counter reset issues
	}

	s.Logger.Infow("updating subscription with new plan",
		"subscription_id", sub.ID,
		"old_plan_id", previousPlanID,
		"new_plan_id", targetPlan.ID,
		"billing_anchor", sub.BillingAnchor,
		"proration_amount", prorationResult.NetAmount,
		"plan_change_count", sub.Metadata["plan_change_count"])

	if err := s.SubRepo.Update(ctx, sub); err != nil {
		return nil, ierr.NewErrorf("failed to update subscription: %v", err).
			WithHint("Failed to update subscription with new plan").
			Mark(ierr.ErrSystem)
	}

	s.Logger.Infow("subscription updated successfully",
		"subscription_id", sub.ID,
		"new_plan_id", targetPlan.ID,
		"version", sub.Version)

	// 2. Update subscription line items
	if err := s.updateSubscriptionLineItems(ctx, sub, targetPlan.ID); err != nil {
		return nil, err
	}

	// 3. Create proration invoice if needed
	var invoice *dto.InvoiceResponse
	if !prorationResult.NetAmount.IsZero() && req.ProrationBehavior == types.ProrationBehaviorCreateProrations {
		invoiceResp, err := s.createProrationInvoice(ctx, sub, prorationResult, "upgrade")
		if err != nil {
			return nil, err
		}
		invoice = invoiceResp
	}

	// 4. Handle coupons (simplified approach - deactivate line item coupons)
	if err := s.handleCouponChanges(ctx, sub); err != nil {
		s.Logger.Warnw("failed to handle coupon changes", "error", err)
		// Don't fail the operation for coupon issues
	}

	return &subscription.SubscriptionPlanChangeResult{
		Subscription:    sub,
		Invoice:         invoice,
		ProrationAmount: prorationResult.NetAmount,
		ChangeType:      "upgrade",
		EffectiveDate:   time.Now(),
		Metadata:        req.Metadata,
	}, nil
}

// scheduleDowngradeAtPeriodEnd schedules a downgrade for the end of the current period
func (s *subscriptionChangeService) scheduleDowngradeAtPeriodEnd(
	ctx context.Context,
	subscriptionID string,
	req *subscription.DowngradeSubscriptionRequest,
) (*subscription.SubscriptionPlanChangeResult, error) {
	var result *subscription.SubscriptionPlanChangeResult
	err := s.DB.WithTx(ctx, func(txCtx context.Context) error {
		// Get subscription and validate
		sub, targetPlan, err := s.validatePlanChange(txCtx, subscriptionID, req.TargetPlanID, "downgrade")
		if err != nil {
			return err
		}

		// Update subscription with pending plan change in metadata
		if sub.Metadata == nil {
			sub.Metadata = make(map[string]string)
		}
		sub.Metadata["pending_plan_id"] = req.TargetPlanID
		sub.Metadata["pending_plan_change_date"] = sub.CurrentPeriodEnd.Format(time.RFC3339)
		sub.Metadata["pending_plan_change_reason"] = "downgrade"
		sub.Version++

		if err := s.SubRepo.Update(txCtx, sub); err != nil {
			return ierr.NewErrorf("failed to update subscription: %v", err).
				WithHint("Failed to schedule downgrade").
				Mark(ierr.ErrSystem)
		}

		// Create audit log
		if err := s.createAuditLog(txCtx, sub, targetPlan, "downgrade_scheduled", decimal.Zero, req.Metadata); err != nil {
			s.Logger.Warnw("failed to create audit log", "error", err)
		}

		result = &subscription.SubscriptionPlanChangeResult{
			Subscription:    sub,
			ProrationAmount: decimal.Zero,
			ChangeType:      "downgrade_scheduled",
			EffectiveDate:   sub.CurrentPeriodEnd,
			Metadata:        req.Metadata,
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

// processImmediateDowngrade processes an immediate downgrade
func (s *subscriptionChangeService) processImmediateDowngrade(
	ctx context.Context,
	subscriptionID string,
	req *subscription.DowngradeSubscriptionRequest,
) (*subscription.SubscriptionPlanChangeResult, error) {
	// For Phase 1, immediate downgrades are not implemented
	// Return error suggesting to use period-end downgrade
	return nil, ierr.NewError("immediate downgrades are not supported").
		WithHint("Please use effective_at_period_end=true for downgrades").
		Mark(ierr.ErrValidation)
}

// Helper methods for calculations and operations

func (s *subscriptionChangeService) calculateCurrentSubscriptionAmount(
	ctx context.Context,
	sub *subscription.Subscription,
) decimal.Decimal {
	// TODO: Implement proper calculation based on current line items
	return decimal.Zero
}

func (s *subscriptionChangeService) calculateNewSubscriptionAmount(
	ctx context.Context,
	targetPlan *plan.Plan,
) decimal.Decimal {
	// TODO: Implement proper calculation based on target plan prices
	return decimal.Zero
}

func (s *subscriptionChangeService) buildProrationLineItemPreviews(
	prorationResult *proration.ProrationResult,
) []interface{} {
	// TODO: Implement proper line item preview building
	return []interface{}{}
}

func (s *subscriptionChangeService) updateSubscriptionLineItems(
	ctx context.Context,
	sub *subscription.Subscription,
	targetPlanID string,
) error {
	s.Logger.Infow("updating subscription line items",
		"subscription_id", sub.ID,
		"target_plan_id", targetPlanID)

	now := time.Now()

	// 1. Get current line items for the subscription
	currentLineItems, err := s.SubscriptionLineItemRepo.ListBySubscription(ctx, sub.ID)
	if err != nil {
		return ierr.NewErrorf("failed to get current line items: %v", err).
			WithHint("Failed to retrieve current subscription line items").
			Mark(ierr.ErrSystem)
	}

	// 2. Get prices for the new plan
	priceService := NewPriceService(s.ServiceParams)
	pricesResponse, err := priceService.GetPricesByPlanID(ctx, targetPlanID)
	if err != nil {
		return ierr.NewErrorf("failed to get prices for target plan: %v", err).
			WithHint("Failed to retrieve prices for the new plan").
			Mark(ierr.ErrSystem)
	}

	// 3. Create maps for efficient lookups
	currentPriceIDs := make(map[string]*subscription.SubscriptionLineItem)
	for _, item := range currentLineItems {
		if item.Status == types.StatusPublished {
			currentPriceIDs[item.PriceID] = item
		}
	}

	newPriceIDs := make(map[string]*dto.PriceResponse)
	for _, price := range pricesResponse.Items {
		// Only include prices that match subscription's billing period and currency
		if s.isPriceCompatibleWithSubscription(price, sub) {
			newPriceIDs[price.ID] = price
		}
	}

	// 4. End line items that are no longer in the new plan
	endedItems := make([]*subscription.SubscriptionLineItem, 0)
	for priceID, lineItem := range currentPriceIDs {
		if _, exists := newPriceIDs[priceID]; !exists {
			s.Logger.Infow("ending line item not in new plan",
				"subscription_id", sub.ID,
				"line_item_id", lineItem.ID,
				"price_id", priceID,
				"current_quantity", lineItem.Quantity)

			// Calculate any final usage charges before ending the line item
			finalUsageAmount, err := s.calculateFinalUsageAmount(ctx, lineItem, now)
			if err != nil {
				s.Logger.Warnw("failed to calculate final usage amount",
					"line_item_id", lineItem.ID,
					"error", err)
				// Continue with ending the item even if usage calculation fails
			}

			// End the line item (soft delete)
			lineItem.EndDate = now
			lineItem.Status = types.StatusArchived
			lineItem.UpdatedAt = now

			// Add comprehensive metadata for audit trail
			if lineItem.Metadata == nil {
				lineItem.Metadata = make(map[string]string)
			}
			lineItem.Metadata["removal_reason"] = "plan_change"
			lineItem.Metadata["removed_at"] = now.Format(time.RFC3339)
			lineItem.Metadata["plan_change_type"] = "upgrade"
			lineItem.Metadata["final_quantity"] = lineItem.Quantity.String()
			if finalUsageAmount != nil {
				lineItem.Metadata["final_usage_amount"] = finalUsageAmount.String()
			}

			if err := s.SubscriptionLineItemRepo.Update(ctx, lineItem); err != nil {
				return ierr.NewErrorf("failed to end line item %s: %v", lineItem.ID, err).
					WithHint("Failed to update line item status").
					Mark(ierr.ErrSystem)
			}

			endedItems = append(endedItems, lineItem)
		}
	}

	s.Logger.Infow("ended line items for plan change",
		"subscription_id", sub.ID,
		"ended_items_count", len(endedItems))

	// 5. Add new line items for prices in the new plan
	newLineItems := make([]*subscription.SubscriptionLineItem, 0)
	for priceID, price := range newPriceIDs {
		if _, exists := currentPriceIDs[priceID]; !exists {
			s.Logger.Infow("adding new line item for new plan",
				"subscription_id", sub.ID,
				"price_id", priceID,
				"price_type", price.Type,
				"price_description", price.Description,
				"price_amount", price.Amount)

			// Validate price before creating line item
			if err := s.validatePriceForLineItem(price, sub); err != nil {
				s.Logger.Errorw("price validation failed for new line item",
					"price_id", priceID,
					"error", err)
				return ierr.NewErrorf("invalid price for line item: %v", err).
					WithHint("Price validation failed for new plan").
					Mark(ierr.ErrValidation)
			}

			// Create new line item with comprehensive metadata
			lineItem := &subscription.SubscriptionLineItem{
				SubscriptionID: sub.ID,
				PriceID:        priceID,
				EntityID:       targetPlanID,
				EntityType:     types.SubscriptionLineItemEntitiyType(types.PRICE_ENTITY_TYPE_PLAN),
				StartDate:      now,
				EnvironmentID:  sub.EnvironmentID,
				Metadata: map[string]string{
					"added_reason":         "plan_change",
					"added_at":             now.Format(time.RFC3339),
					"plan_change_type":     "upgrade",
					"source_plan_id":       targetPlanID,
					"price_type":           string(price.Type),
					"price_currency":       price.Currency,
					"price_amount":         price.Amount.String(),
					"billing_period":       string(price.BillingPeriod),
					"billing_period_count": fmt.Sprintf("%d", price.BillingPeriodCount),
				},
				BaseModel: types.BaseModel{
					TenantID:  sub.TenantID,
					Status:    types.StatusPublished,
					CreatedAt: now,
					UpdatedAt: now,
				},
			}

			// Set quantity based on price type with proper initialization
			switch price.Type {
			case types.PRICE_TYPE_FIXED:
				lineItem.Quantity = decimal.NewFromInt(1)
				s.Logger.Infow("created fixed price line item",
					"line_item_id", lineItem.ID,
					"quantity", lineItem.Quantity)
			case types.PRICE_TYPE_USAGE:
				lineItem.Quantity = decimal.Zero // Usage-based starts at zero
				s.Logger.Infow("created usage-based price line item",
					"line_item_id", lineItem.ID,
					"initial_quantity", lineItem.Quantity)
			// Note: Only FIXED and USAGE price types are currently supported
			default:
				lineItem.Quantity = decimal.Zero
				s.Logger.Warnw("unknown price type, defaulting quantity to zero",
					"price_type", price.Type,
					"price_id", priceID)
			}

			newLineItems = append(newLineItems, lineItem)
		}
	}

	// 6. Create new line items in bulk
	if len(newLineItems) > 0 {
		if err := s.SubscriptionLineItemRepo.CreateBulk(ctx, newLineItems); err != nil {
			return ierr.NewErrorf("failed to create new line items: %v", err).
				WithHint("Failed to create line items for new plan").
				Mark(ierr.ErrSystem)
		}

		s.Logger.Infow("created new line items for plan change",
			"subscription_id", sub.ID,
			"new_line_items_count", len(newLineItems))
	}

	s.Logger.Infow("subscription line items updated successfully",
		"subscription_id", sub.ID,
		"target_plan_id", targetPlanID,
		"ended_items", len(currentPriceIDs)-len(newPriceIDs),
		"added_items", len(newLineItems))

	return nil
}

// isPriceCompatibleWithSubscription checks if a price is compatible with the subscription
func (s *subscriptionChangeService) isPriceCompatibleWithSubscription(
	price *dto.PriceResponse,
	sub *subscription.Subscription,
) bool {
	// Check currency compatibility
	if price.Currency != sub.Currency {
		return false
	}

	// Check billing period compatibility
	if price.BillingPeriod != sub.BillingPeriod {
		return false
	}

	// Check billing period count compatibility
	if price.BillingPeriodCount != sub.BillingPeriodCount {
		return false
	}

	return true
}

func (s *subscriptionChangeService) createProrationInvoice(
	ctx context.Context,
	sub *subscription.Subscription,
	prorationResult *proration.ProrationResult,
	changeType string,
) (*dto.InvoiceResponse, error) {
	s.Logger.Infow("creating proration invoice",
		"subscription_id", sub.ID,
		"amount", prorationResult.NetAmount,
		"change_type", changeType)

	now := time.Now()

	// Build comprehensive line items from proration result
	lineItems := make([]dto.CreateInvoiceLineItemRequest, 0)
	totalCreditAmount := decimal.Zero
	totalChargeAmount := decimal.Zero

	// Add credit line items for unused time on old plan
	for _, item := range prorationResult.CreditItems {
		totalCreditAmount = totalCreditAmount.Add(item.Amount)

		// Enhanced description for credit items
		description := fmt.Sprintf("Credit for unused time: %s (Period: %s to %s)",
			item.Description,
			item.StartDate.Format("2006-01-02"),
			item.EndDate.Format("2006-01-02"))

		lineItems = append(lineItems, dto.CreateInvoiceLineItemRequest{
			PriceID:     &item.PriceID,
			DisplayName: &description,
			Amount:      item.Amount, // Already negative
			Quantity:    item.Quantity,
			PeriodStart: &item.StartDate,
			PeriodEnd:   &item.EndDate,
			Metadata: map[string]string{
				"proration_type":    "credit",
				"change_type":       changeType,
				"original_price_id": item.PriceID,
				"period_start":      item.StartDate.Format(time.RFC3339),
				"period_end":        item.EndDate.Format(time.RFC3339),
				"item_amount":       item.Amount.String(),
				"item_quantity":     item.Quantity.String(),
				"subscription_id":   sub.ID,
			},
		})

		s.Logger.Infow("added credit line item to invoice",
			"price_id", item.PriceID,
			"amount", item.Amount,
			"quantity", item.Quantity,
			"period", fmt.Sprintf("%s to %s", item.StartDate.Format("2006-01-02"), item.EndDate.Format("2006-01-02")))
	}

	// Add charge line items for new plan prorated time
	for _, item := range prorationResult.ChargeItems {
		totalChargeAmount = totalChargeAmount.Add(item.Amount)

		// Enhanced description for charge items
		description := fmt.Sprintf("Charge for new plan: %s (Period: %s to %s)",
			item.Description,
			item.StartDate.Format("2006-01-02"),
			item.EndDate.Format("2006-01-02"))

		lineItems = append(lineItems, dto.CreateInvoiceLineItemRequest{
			PriceID:     &item.PriceID,
			DisplayName: &description,
			Amount:      item.Amount,
			Quantity:    item.Quantity,
			PeriodStart: &item.StartDate,
			PeriodEnd:   &item.EndDate,
			Metadata: map[string]string{
				"proration_type":    "charge",
				"change_type":       changeType,
				"original_price_id": item.PriceID,
				"period_start":      item.StartDate.Format(time.RFC3339),
				"period_end":        item.EndDate.Format(time.RFC3339),
				"item_amount":       item.Amount.String(),
				"item_quantity":     item.Quantity.String(),
				"subscription_id":   sub.ID,
			},
		})

		s.Logger.Infow("added charge line item to invoice",
			"price_id", item.PriceID,
			"amount", item.Amount,
			"quantity", item.Quantity,
			"period", fmt.Sprintf("%s to %s", item.StartDate.Format("2006-01-02"), item.EndDate.Format("2006-01-02")))
	}

	// Add usage-based charges if any exist
	usageLineItems, err := s.buildUsageBasedLineItems(ctx, sub, changeType, now)
	if err != nil {
		s.Logger.Warnw("failed to build usage-based line items", "error", err)
		// Continue without usage items rather than failing the entire operation
	} else {
		lineItems = append(lineItems, usageLineItems...)
		s.Logger.Infow("added usage-based line items",
			"count", len(usageLineItems),
			"subscription_id", sub.ID)
	}

	s.Logger.Infow("built invoice line items",
		"subscription_id", sub.ID,
		"total_line_items", len(lineItems),
		"total_credit_amount", totalCreditAmount,
		"total_charge_amount", totalChargeAmount,
		"net_amount", prorationResult.NetAmount)

	// If no line items (zero proration), don't create invoice
	if len(lineItems) == 0 || prorationResult.NetAmount.IsZero() {
		s.Logger.Infow("no proration invoice needed - zero amount",
			"subscription_id", sub.ID)
		return nil, nil
	}

	// Create invoice request
	invoiceReq := dto.CreateInvoiceRequest{
		CustomerID:     sub.CustomerID,
		SubscriptionID: &sub.ID,
		InvoiceType:    types.InvoiceTypeOneOff,                                 // Proration invoices are one-off
		InvoiceStatus:  &[]types.InvoiceStatus{types.InvoiceStatusFinalized}[0], // Immediately finalized
		PaymentStatus:  &[]types.PaymentStatus{types.PaymentStatusPending}[0],
		Currency:       sub.Currency,
		AmountDue:      prorationResult.NetAmount,
		Total:          prorationResult.NetAmount,
		Subtotal:       prorationResult.NetAmount,
		Description:    fmt.Sprintf("Plan %s proration for subscription %s", changeType, sub.ID),
		DueDate:        &now, // Due immediately
		BillingReason:  types.InvoiceBillingReasonSubscriptionUpdate,
		EnvironmentID:  sub.EnvironmentID,
		LineItems:      lineItems,
		Metadata: map[string]string{
			"proration_type":  "plan_change",
			"change_type":     changeType,
			"subscription_id": sub.ID,
			"created_at":      now.Format(time.RFC3339),
			"net_amount":      prorationResult.NetAmount.String(),
		},
	}

	// Set payment status based on amount
	if prorationResult.NetAmount.IsNegative() {
		// Credit invoice - mark as succeeded since it's a credit
		invoiceReq.PaymentStatus = &[]types.PaymentStatus{types.PaymentStatusSucceeded}[0]
		amountPaidAbs := prorationResult.NetAmount.Abs()
		invoiceReq.AmountPaid = &amountPaidAbs // Paid amount is positive
	} else {
		// Charge invoice - pending payment
		invoiceReq.PaymentStatus = &[]types.PaymentStatus{types.PaymentStatusPending}[0]
		zero := decimal.Zero
		invoiceReq.AmountPaid = &zero
	}

	// Create the invoice using the existing invoice service
	invoiceService := NewInvoiceService(s.ServiceParams)
	invoice, err := invoiceService.CreateInvoice(ctx, invoiceReq)
	if err != nil {
		return nil, ierr.NewErrorf("failed to create proration invoice: %v", err).
			WithHint("Failed to create invoice for plan change proration").
			Mark(ierr.ErrSystem)
	}

	s.Logger.Infow("proration invoice created successfully",
		"subscription_id", sub.ID,
		"invoice_id", invoice.ID,
		"amount", prorationResult.NetAmount,
		"line_items_count", len(lineItems),
		"change_type", changeType)

	return invoice, nil
}

func (s *subscriptionChangeService) handleCouponChanges(
	ctx context.Context,
	sub *subscription.Subscription,
) error {
	// Simplified approach: deactivate all line item coupons
	s.Logger.Infow("handling coupon changes (deactivating line item coupons)", "subscription_id", sub.ID)
	// TODO: Implement coupon deactivation logic
	return nil
}

func (s *subscriptionChangeService) createAuditLog(
	ctx context.Context,
	sub *subscription.Subscription,
	targetPlan *plan.Plan,
	changeType string,
	prorationAmount decimal.Decimal,
	metadata map[string]string,
) error {
	// TODO: Implement audit log creation
	s.Logger.Infow("creating audit log",
		"subscription_id", sub.ID,
		"change_type", changeType,
		"target_plan_id", targetPlan.ID,
		"proration_amount", prorationAmount)
	return nil
}

// incrementPlanChangeCount increments and returns the plan change count
func (s *subscriptionChangeService) incrementPlanChangeCount(metadata map[string]string) string {
	if metadata == nil {
		return "1"
	}

	currentCount := metadata["plan_change_count"]
	if currentCount == "" {
		return "1"
	}

	// Parse current count and increment
	count := 1
	if parsed, err := fmt.Sscanf(currentCount, "%d", &count); err == nil && parsed == 1 {
		count++
	}

	return fmt.Sprintf("%d", count)
}

// resetUsageCountersForPlanChange resets usage counters when plan changes
func (s *subscriptionChangeService) resetUsageCountersForPlanChange(
	ctx context.Context,
	sub *subscription.Subscription,
	resetTime time.Time,
) error {
	s.Logger.Infow("resetting usage counters for plan change",
		"subscription_id", sub.ID,
		"reset_time", resetTime)

	// Get all usage-based line items for this subscription
	lineItems, err := s.SubscriptionLineItemRepo.ListBySubscription(ctx, sub.ID)
	if err != nil {
		return ierr.NewErrorf("failed to get line items for usage reset: %v", err).
			WithHint("Failed to retrieve subscription line items").
			Mark(ierr.ErrSystem)
	}

	// Reset usage counters for usage-based items
	resetCount := 0
	for _, item := range lineItems {
		if item.Status == types.StatusPublished {
			// Get the price to check if it's usage-based
			priceService := NewPriceService(s.ServiceParams)
			price, err := priceService.GetPrice(ctx, item.PriceID)
			if err != nil {
				s.Logger.Warnw("failed to get price for usage reset",
					"price_id", item.PriceID,
					"error", err)
				continue
			}

			// Reset usage for usage-based prices
			if price.Type == types.PRICE_TYPE_USAGE {
				// Reset the quantity to zero for usage-based items
				item.Quantity = decimal.Zero
				item.UpdatedAt = resetTime

				// Add metadata about the reset
				if item.Metadata == nil {
					item.Metadata = make(map[string]string)
				}
				item.Metadata["usage_reset_at"] = resetTime.Format(time.RFC3339)
				item.Metadata["usage_reset_reason"] = "plan_change"

				if err := s.SubscriptionLineItemRepo.Update(ctx, item); err != nil {
					s.Logger.Warnw("failed to reset usage counter",
						"line_item_id", item.ID,
						"price_id", item.PriceID,
						"error", err)
					continue
				}

				resetCount++
				s.Logger.Infow("reset usage counter for line item",
					"line_item_id", item.ID,
					"price_id", item.PriceID)
			}
		}
	}

	s.Logger.Infow("usage counters reset completed",
		"subscription_id", sub.ID,
		"reset_count", resetCount)

	return nil
}

// calculateFinalUsageAmount calculates the final usage amount for a line item before ending it
func (s *subscriptionChangeService) calculateFinalUsageAmount(
	ctx context.Context,
	lineItem *subscription.SubscriptionLineItem,
	endTime time.Time,
) (*decimal.Decimal, error) {
	// Get the price to check if it's usage-based
	priceService := NewPriceService(s.ServiceParams)
	price, err := priceService.GetPrice(ctx, lineItem.PriceID)
	if err != nil {
		return nil, err
	}

	// Only calculate for usage-based prices
	if price.Type != types.PRICE_TYPE_USAGE {
		return nil, nil
	}

	// Calculate final amount based on current quantity and price
	finalAmount := lineItem.Quantity.Mul(price.Amount)

	s.Logger.Infow("calculated final usage amount",
		"line_item_id", lineItem.ID,
		"price_id", lineItem.PriceID,
		"quantity", lineItem.Quantity,
		"unit_price", price.Amount,
		"final_amount", finalAmount)

	return &finalAmount, nil
}

// validatePriceForLineItem validates that a price is suitable for creating a line item
func (s *subscriptionChangeService) validatePriceForLineItem(
	price *dto.PriceResponse,
	sub *subscription.Subscription,
) error {
	// Check if price is active
	if price.Status != types.StatusPublished {
		return ierr.NewErrorf("price %s is not active", price.ID).
			WithHint("Only active prices can be added to subscriptions").
			Mark(ierr.ErrValidation)
	}

	// Check currency compatibility
	if price.Currency != sub.Currency {
		return ierr.NewErrorf("price currency %s does not match subscription currency %s",
			price.Currency, sub.Currency).
			WithHint("Price and subscription must have the same currency").
			Mark(ierr.ErrValidation)
	}

	// Check billing period compatibility
	if price.BillingPeriod != sub.BillingPeriod {
		return ierr.NewErrorf("price billing period %s does not match subscription billing period %s",
			price.BillingPeriod, sub.BillingPeriod).
			WithHint("Price and subscription must have the same billing period").
			Mark(ierr.ErrValidation)
	}

	// Check billing period count compatibility
	if price.BillingPeriodCount != sub.BillingPeriodCount {
		return ierr.NewErrorf("price billing period count %d does not match subscription billing period count %d",
			price.BillingPeriodCount, sub.BillingPeriodCount).
			WithHint("Price and subscription must have the same billing period count").
			Mark(ierr.ErrValidation)
	}

	// Validate price amount
	if price.Amount.IsNegative() {
		return ierr.NewErrorf("price amount cannot be negative: %s", price.Amount.String()).
			WithHint("Price amounts must be zero or positive").
			Mark(ierr.ErrValidation)
	}

	return nil
}

// buildUsageBasedLineItems builds line items for usage-based charges during plan change
func (s *subscriptionChangeService) buildUsageBasedLineItems(
	ctx context.Context,
	sub *subscription.Subscription,
	changeType string,
	invoiceTime time.Time,
) ([]dto.CreateInvoiceLineItemRequest, error) {
	lineItems := make([]dto.CreateInvoiceLineItemRequest, 0)

	// Get current line items to find usage-based ones
	currentLineItems, err := s.SubscriptionLineItemRepo.ListBySubscription(ctx, sub.ID)
	if err != nil {
		return nil, err
	}

	priceService := NewPriceService(s.ServiceParams)

	for _, item := range currentLineItems {
		if item.Status != types.StatusPublished || item.Quantity.IsZero() {
			continue
		}

		// Get price to check if it's usage-based
		price, err := priceService.GetPrice(ctx, item.PriceID)
		if err != nil {
			s.Logger.Warnw("failed to get price for usage line item",
				"price_id", item.PriceID,
				"error", err)
			continue
		}

		// Only process usage-based prices with accumulated usage
		if price.Type == types.PRICE_TYPE_USAGE && !item.Quantity.IsZero() {
			usageAmount := item.Quantity.Mul(price.Amount)
			description := fmt.Sprintf("Usage charges up to plan change: %s (Quantity: %s)",
				price.Description, item.Quantity.String())

			lineItems = append(lineItems, dto.CreateInvoiceLineItemRequest{
				PriceID:     &item.PriceID,
				DisplayName: &description,
				Amount:      usageAmount,
				Quantity:    item.Quantity,
				PeriodStart: &item.StartDate,
				PeriodEnd:   &invoiceTime,
				Metadata: map[string]string{
					"line_item_type":    "usage_final",
					"change_type":       changeType,
					"original_price_id": item.PriceID,
					"usage_quantity":    item.Quantity.String(),
					"unit_price":        price.Amount.String(),
					"period_start":      item.StartDate.Format(time.RFC3339),
					"period_end":        invoiceTime.Format(time.RFC3339),
					"subscription_id":   sub.ID,
				},
			})

			s.Logger.Infow("added usage-based line item for final billing",
				"price_id", item.PriceID,
				"quantity", item.Quantity,
				"amount", usageAmount,
				"description", price.Description)
		}
	}

	return lineItems, nil
}
