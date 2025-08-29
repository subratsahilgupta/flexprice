package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/proration"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// SubscriptionChangeService handles subscription plan changes (upgrades/downgrades)
type SubscriptionChangeService interface {
	// PreviewSubscriptionChange shows the impact of changing subscription plan
	PreviewSubscriptionChange(ctx context.Context, subscriptionID string, req dto.SubscriptionChangePreviewRequest) (*dto.SubscriptionChangePreviewResponse, error)

	// ExecuteSubscriptionChange performs the actual subscription plan change
	ExecuteSubscriptionChange(ctx context.Context, subscriptionID string, req dto.SubscriptionChangeRequest) (*dto.SubscriptionChangeExecuteResponse, error)
}

type subscriptionChangeService struct {
	serviceParams       ServiceParams
	invoiceService      InvoiceService
	prorationService    proration.Service
	subscriptionService SubscriptionService
}

// NewSubscriptionChangeService creates a new subscription change service
func NewSubscriptionChangeService(serviceParams ServiceParams) SubscriptionChangeService {
	return &subscriptionChangeService{
		serviceParams:       serviceParams,
		invoiceService:      NewInvoiceService(serviceParams),
		prorationService:    NewProrationService(serviceParams),
		subscriptionService: NewSubscriptionService(serviceParams),
	}
}

// PreviewSubscriptionChange shows the impact of changing subscription plan
func (s *subscriptionChangeService) PreviewSubscriptionChange(
	ctx context.Context,
	subscriptionID string,
	req dto.SubscriptionChangePreviewRequest,
) (*dto.SubscriptionChangePreviewResponse, error) {
	logger := s.serviceParams.Logger.With(
		zap.String("subscription_id", subscriptionID),
		zap.String("target_plan_id", req.TargetPlanID),
	)

	logger.Info("previewing subscription change")

	// Validate the request
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Get current subscription with line items
	currentSub, lineItems, err := s.serviceParams.SubRepo.GetWithLineItems(ctx, subscriptionID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve subscription").
			Mark(ierr.ErrDatabase)
	}

	// Get current plan
	currentPlan, err := s.serviceParams.PlanRepo.Get(ctx, currentSub.PlanID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve current plan").
			Mark(ierr.ErrDatabase)
	}

	// Get target plan
	targetPlan, err := s.serviceParams.PlanRepo.Get(ctx, req.TargetPlanID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve target plan").
			Mark(ierr.ErrDatabase)
	}

	// Validate plans are in same environment
	if currentPlan.EnvironmentID != targetPlan.EnvironmentID {
		return nil, ierr.NewError("plans must be in the same environment").
			WithHint("Cannot change between plans in different environments").
			Mark(ierr.ErrValidation)
	}

	// Check if subscription is in a valid state for changes
	if err := s.validateSubscriptionForChange(currentSub); err != nil {
		return nil, err
	}

	// Determine change type and validate it's allowed
	changeType, err := s.determineChangeType(ctx, currentPlan, targetPlan)
	if err != nil {
		return nil, err
	}

	// Calculate effective date
	effectiveDate := time.Now()
	if req.EffectiveDate != nil {
		effectiveDate = *req.EffectiveDate
	}
	if req.PreviewDate != nil {
		effectiveDate = *req.PreviewDate
	}

	// Calculate proration if needed
	var prorationDetails *dto.ProrationDetails
	if req.ProrationBehavior == types.ProrationBehaviorCreateProrations {
		prorationDetails, err = s.calculateProrationPreview(ctx, currentSub, lineItems, targetPlan, effectiveDate)
		if err != nil {
			logger.Error("failed to calculate proration preview", zap.Error(err))
			return nil, err
		}
	}

	// Calculate immediate invoice preview
	immediateInvoice, err := s.calculateImmediateInvoicePreview(ctx, currentSub, targetPlan, prorationDetails, effectiveDate)
	if err != nil {
		logger.Error("failed to calculate immediate invoice preview", zap.Error(err))
		return nil, err
	}

	// Calculate next invoice preview
	nextInvoice, err := s.calculateNextInvoicePreview(ctx, currentSub, targetPlan, effectiveDate)
	if err != nil {
		logger.Error("failed to calculate next invoice preview", zap.Error(err))
		return nil, err
	}

	// Calculate new billing cycle
	newBillingCycle, err := s.calculateNewBillingCycle(currentSub, targetPlan, req.BillingCycleAnchor, effectiveDate)
	if err != nil {
		logger.Error("failed to calculate new billing cycle", zap.Error(err))
		return nil, err
	}

	// Generate warnings
	warnings := s.generateWarnings(currentSub, targetPlan, changeType, req.ProrationBehavior)

	response := &dto.SubscriptionChangePreviewResponse{
		SubscriptionID: subscriptionID,
		CurrentPlan: dto.PlanSummary{
			ID:          currentPlan.ID,
			Name:        currentPlan.Name,
			LookupKey:   currentPlan.LookupKey,
			Description: currentPlan.Description,
		},
		TargetPlan: dto.PlanSummary{
			ID:          targetPlan.ID,
			Name:        targetPlan.Name,
			LookupKey:   targetPlan.LookupKey,
			Description: targetPlan.Description,
		},
		ChangeType:              changeType,
		ProrationDetails:        prorationDetails,
		ImmediateInvoicePreview: immediateInvoice,
		NextInvoicePreview:      nextInvoice,
		EffectiveDate:           effectiveDate,
		NewBillingCycle:         *newBillingCycle,
		Warnings:                warnings,
		Metadata:                req.Metadata,
	}

	logger.Info("subscription change preview completed successfully")
	return response, nil
}

// ExecuteSubscriptionChange performs the actual subscription plan change
func (s *subscriptionChangeService) ExecuteSubscriptionChange(
	ctx context.Context,
	subscriptionID string,
	req dto.SubscriptionChangeRequest,
) (*dto.SubscriptionChangeExecuteResponse, error) {
	logger := s.serviceParams.Logger.With(
		zap.String("subscription_id", subscriptionID),
		zap.String("target_plan_id", req.TargetPlanID),
	)

	logger.Info("executing subscription change")

	// Validate the request
	if err := req.Validate(); err != nil {
		return nil, err
	}

	var response *dto.SubscriptionChangeExecuteResponse

	// Execute the change within a transaction
	err := s.serviceParams.DB.WithTx(ctx, func(txCtx context.Context) error {
		// Get current subscription with line items
		currentSub, lineItems, err := s.serviceParams.SubRepo.GetWithLineItems(txCtx, subscriptionID)
		if err != nil {
			return ierr.WithError(err).
				WithHint("Failed to retrieve subscription").
				Mark(ierr.ErrDatabase)
		}

		// Get current and target plans
		currentPlan, err := s.serviceParams.PlanRepo.Get(txCtx, currentSub.PlanID)
		if err != nil {
			return ierr.WithError(err).
				WithHint("Failed to retrieve current plan").
				Mark(ierr.ErrDatabase)
		}

		targetPlan, err := s.serviceParams.PlanRepo.Get(txCtx, req.TargetPlanID)
		if err != nil {
			return ierr.WithError(err).
				WithHint("Failed to retrieve target plan").
				Mark(ierr.ErrDatabase)
		}

		// Validate the change
		if err := s.validateSubscriptionForChange(currentSub); err != nil {
			return err
		}

		// Determine change type
		changeType, err := s.determineChangeType(txCtx, currentPlan, targetPlan)
		if err != nil {
			return err
		}

		// Calculate effective date
		effectiveDate := time.Now()
		if req.EffectiveDate != nil {
			effectiveDate = *req.EffectiveDate
		}

		// Execute the change based on type
		result, err := s.executeChange(txCtx, currentSub, lineItems, targetPlan, changeType, req, effectiveDate)
		if err != nil {
			return err
		}

		response = result
		return nil
	})

	if err != nil {
		logger.Error("failed to execute subscription change", zap.Error(err))
		return nil, err
	}

	logger.Info("subscription change executed successfully",
		zap.String("old_subscription_id", response.OldSubscription.ID),
		zap.String("new_subscription_id", response.NewSubscription.ID),
	)

	return response, nil
}

// validateSubscriptionForChange checks if subscription can be changed
func (s *subscriptionChangeService) validateSubscriptionForChange(sub *subscription.Subscription) error {
	// Check subscription status
	switch sub.SubscriptionStatus {
	case types.SubscriptionStatusActive, types.SubscriptionStatusTrialing:
		// These are valid states for changes
	case types.SubscriptionStatusPaused:
		return ierr.NewError("cannot change paused subscription").
			WithHint("Resume the subscription before changing plans").
			Mark(ierr.ErrValidation)
	case types.SubscriptionStatusCancelled:
		return ierr.NewError("cannot change cancelled subscription").
			WithHint("Cancelled subscriptions cannot be changed").
			Mark(ierr.ErrValidation)
	default:
		return ierr.NewError("subscription not in valid state for changes").
			WithHint("Subscription must be active or trialing").
			WithReportableDetails(map[string]any{
				"current_status": sub.SubscriptionStatus,
			}).
			Mark(ierr.ErrValidation)
	}

	return nil
}

// determineChangeType determines if this is an upgrade, downgrade, or lateral change
func (s *subscriptionChangeService) determineChangeType(
	ctx context.Context,
	currentPlan *plan.Plan,
	targetPlan *plan.Plan,
) (types.SubscriptionChangeType, error) {
	// For now, we'll use a simple comparison based on plan names or could be enhanced
	// with pricing comparison logic in the future

	// If same plan, it's a lateral change (though this might not make sense)
	if currentPlan.ID == targetPlan.ID {
		return types.SubscriptionChangeTypeLateral, nil
	}

	// Get prices for both plans to compare
	priceService := NewPriceService(s.serviceParams)

	currentPricesResponse, err := priceService.GetPricesByPlanID(ctx, currentPlan.ID)
	if err != nil {
		return "", ierr.WithError(err).
			WithHint("Failed to get current plan prices").
			Mark(ierr.ErrDatabase)
	}

	targetPricesResponse, err := priceService.GetPricesByPlanID(ctx, targetPlan.ID)
	if err != nil {
		return "", ierr.WithError(err).
			WithHint("Failed to get target plan prices").
			Mark(ierr.ErrDatabase)
	}

	// Extract price objects from response
	currentPrices := make([]*price.Price, len(currentPricesResponse.Items))
	for i, item := range currentPricesResponse.Items {
		currentPrices[i] = item.Price
	}

	targetPrices := make([]*price.Price, len(targetPricesResponse.Items))
	for i, item := range targetPricesResponse.Items {
		targetPrices[i] = item.Price
	}

	// Simple comparison based on total plan value
	// In a real implementation, this would be more sophisticated
	currentValue := s.calculatePlanValue(currentPrices)
	targetValue := s.calculatePlanValue(targetPrices)

	if targetValue.GreaterThan(currentValue) {
		return types.SubscriptionChangeTypeUpgrade, nil
	} else if targetValue.LessThan(currentValue) {
		return types.SubscriptionChangeTypeDowngrade, nil
	}

	return types.SubscriptionChangeTypeLateral, nil
}

// calculatePlanValue calculates a simple total value for plan comparison
func (s *subscriptionChangeService) calculatePlanValue(prices []*price.Price) decimal.Decimal {
	total := decimal.Zero
	for _, p := range prices {
		if !p.Amount.IsZero() {
			total = total.Add(p.Amount)
		}
	}
	return total
}

// calculateProrationPreview calculates proration amounts for preview
func (s *subscriptionChangeService) calculateProrationPreview(
	ctx context.Context,
	currentSub *subscription.Subscription,
	lineItems []*subscription.SubscriptionLineItem,
	targetPlan *plan.Plan,
	effectiveDate time.Time,
) (*dto.ProrationDetails, error) {
	// Get prices for the current subscription line items
	priceIDs := make([]string, len(lineItems))
	for i, item := range lineItems {
		priceIDs[i] = item.PriceID
	}

	// Fetch prices using price service
	priceService := NewPriceService(s.serviceParams)
	priceFilter := types.NewNoLimitPriceFilter()
	priceFilter.PriceIDs = priceIDs
	pricesResponse, err := priceService.GetPrices(ctx, priceFilter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get prices for proration calculation").
			Mark(ierr.ErrDatabase)
	}

	// Build prices map required by proration service
	pricesMap := make(map[string]*price.Price)
	for _, priceResp := range pricesResponse.Items {
		pricesMap[priceResp.Price.ID] = priceResp.Price
	}

	// Use existing proration service to calculate
	params := proration.SubscriptionProrationParams{
		Subscription:  currentSub,
		Prices:        pricesMap,
		BillingCycle:  currentSub.BillingCycle,
		ProrationMode: types.ProrationModeActive,
	}

	result, err := s.prorationService.CalculateSubscriptionProration(ctx, params)
	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	// Calculate totals
	creditAmount := decimal.Zero
	chargeAmount := decimal.Zero

	for _, lineResult := range result.LineItemResults {
		for _, credit := range lineResult.CreditItems {
			creditAmount = creditAmount.Add(credit.Amount)
		}
		for _, charge := range lineResult.ChargeItems {
			chargeAmount = chargeAmount.Add(charge.Amount)
		}
	}

	netAmount := chargeAmount.Sub(creditAmount)

	// Calculate days
	daysUsed := int(effectiveDate.Sub(currentSub.CurrentPeriodStart).Hours() / 24)
	daysRemaining := int(currentSub.CurrentPeriodEnd.Sub(effectiveDate).Hours() / 24)

	return &dto.ProrationDetails{
		CreditAmount:       creditAmount,
		CreditDescription:  "Credit for unused time on current plan",
		ChargeAmount:       chargeAmount,
		ChargeDescription:  fmt.Sprintf("Charge for new plan from %s", effectiveDate.Format("2006-01-02")),
		NetAmount:          netAmount,
		ProrationDate:      effectiveDate,
		CurrentPeriodStart: currentSub.CurrentPeriodStart,
		CurrentPeriodEnd:   currentSub.CurrentPeriodEnd,
		DaysUsed:           daysUsed,
		DaysRemaining:      daysRemaining,
		Currency:           currentSub.Currency,
	}, nil
}

// calculateImmediateInvoicePreview calculates what would be invoiced immediately
func (s *subscriptionChangeService) calculateImmediateInvoicePreview(
	ctx context.Context,
	currentSub *subscription.Subscription,
	targetPlan *plan.Plan,
	prorationDetails *dto.ProrationDetails,
	effectiveDate time.Time,
) (*dto.InvoicePreview, error) {
	if prorationDetails == nil {
		return nil, nil
	}

	// Create line items for the preview
	lineItems := []dto.InvoiceLineItemPreview{}

	// Add credit line item if there's a credit
	if prorationDetails.CreditAmount.GreaterThan(decimal.Zero) {
		lineItems = append(lineItems, dto.InvoiceLineItemPreview{
			Description: prorationDetails.CreditDescription,
			Amount:      prorationDetails.CreditAmount.Neg(),
			Quantity:    decimal.NewFromInt(1),
			UnitPrice:   prorationDetails.CreditAmount.Neg(),
			IsProration: true,
			PeriodStart: &prorationDetails.CurrentPeriodStart,
			PeriodEnd:   &effectiveDate,
		})
	}

	// Add charge line item if there's a charge
	if prorationDetails.ChargeAmount.GreaterThan(decimal.Zero) {
		lineItems = append(lineItems, dto.InvoiceLineItemPreview{
			Description: prorationDetails.ChargeDescription,
			Amount:      prorationDetails.ChargeAmount,
			Quantity:    decimal.NewFromInt(1),
			UnitPrice:   prorationDetails.ChargeAmount,
			IsProration: true,
			PeriodStart: &effectiveDate,
			PeriodEnd:   &prorationDetails.CurrentPeriodEnd,
		})
	}

	subtotal := prorationDetails.NetAmount
	// For simplicity, assume no taxes in preview
	taxAmount := decimal.Zero
	total := subtotal.Add(taxAmount)

	return &dto.InvoicePreview{
		Subtotal:  subtotal,
		TaxAmount: taxAmount,
		Total:     total,
		Currency:  currentSub.Currency,
		LineItems: lineItems,
		DueDate:   &effectiveDate,
	}, nil
}

// calculateNextInvoicePreview calculates how the next regular invoice would be affected
func (s *subscriptionChangeService) calculateNextInvoicePreview(
	ctx context.Context,
	currentSub *subscription.Subscription,
	targetPlan *plan.Plan,
	effectiveDate time.Time,
) (*dto.InvoicePreview, error) {
	// Get target plan prices
	priceService := NewPriceService(s.serviceParams)
	targetPricesResponse, err := priceService.GetPricesByPlanID(ctx, targetPlan.ID)
	if err != nil {
		return nil, err
	}

	// Extract price objects from response
	targetPrices := make([]*price.Price, len(targetPricesResponse.Items))
	for i, item := range targetPricesResponse.Items {
		targetPrices[i] = item.Price
	}

	// Calculate what the next invoice would look like with the new plan
	lineItems := []dto.InvoiceLineItemPreview{}
	subtotal := decimal.Zero

	for _, p := range targetPrices {
		if !p.Amount.IsZero() && p.Amount.GreaterThan(decimal.Zero) {
			amount := p.Amount
			description := fmt.Sprintf("%s - %s", targetPlan.Name, p.Description)
			if p.Description == "" {
				description = fmt.Sprintf("%s - Price", targetPlan.Name)
			}
			lineItems = append(lineItems, dto.InvoiceLineItemPreview{
				Description: description,
				Amount:      amount,
				Quantity:    decimal.NewFromInt(1),
				UnitPrice:   amount,
				IsProration: false,
			})
			subtotal = subtotal.Add(amount)
		}
	}

	// For simplicity, assume no taxes in preview
	taxAmount := decimal.Zero
	total := subtotal.Add(taxAmount)

	return &dto.InvoicePreview{
		Subtotal:  subtotal,
		TaxAmount: taxAmount,
		Total:     total,
		Currency:  currentSub.Currency,
		LineItems: lineItems,
	}, nil
}

// calculateNewBillingCycle calculates the new billing cycle information
func (s *subscriptionChangeService) calculateNewBillingCycle(
	currentSub *subscription.Subscription,
	targetPlan *plan.Plan,
	billingCycleAnchor types.BillingCycleAnchor,
	effectiveDate time.Time,
) (*dto.BillingCycleInfo, error) {
	// Get target plan prices to determine billing cadence
	// For simplicity, we'll use the current subscription's billing info
	// In a real implementation, this would be derived from the target plan

	newPeriodStart := effectiveDate
	newBillingAnchor := currentSub.BillingAnchor

	// Adjust based on billing cycle anchor
	switch billingCycleAnchor {
	case types.BillingCycleAnchorReset:
		newBillingAnchor = effectiveDate
	case types.BillingCycleAnchorImmediate:
		newPeriodStart = effectiveDate
		newBillingAnchor = effectiveDate
	case types.BillingCycleAnchorUnchanged:
		// Keep current billing anchor
	}

	// Calculate new period end based on billing period
	newPeriodEnd := s.calculatePeriodEnd(newPeriodStart, currentSub.BillingPeriod, currentSub.BillingPeriodCount)

	return &dto.BillingCycleInfo{
		PeriodStart:        newPeriodStart,
		PeriodEnd:          newPeriodEnd,
		BillingAnchor:      newBillingAnchor,
		BillingCadence:     currentSub.BillingCadence,
		BillingPeriod:      currentSub.BillingPeriod,
		BillingPeriodCount: currentSub.BillingPeriodCount,
	}, nil
}

// calculatePeriodEnd calculates the end of a billing period
func (s *subscriptionChangeService) calculatePeriodEnd(
	start time.Time,
	period types.BillingPeriod,
	count int,
) time.Time {
	switch period {
	case types.BILLING_PERIOD_DAILY:
		return start.AddDate(0, 0, count)
	case types.BILLING_PERIOD_WEEKLY:
		return start.AddDate(0, 0, count*7)
	case types.BILLING_PERIOD_MONTHLY:
		return start.AddDate(0, count, 0)
	case types.BILLING_PERIOD_ANNUAL:
		return start.AddDate(count, 0, 0)
	case types.BILLING_PERIOD_QUARTER:
		return start.AddDate(0, count*3, 0)
	case types.BILLING_PERIOD_HALF_YEAR:
		return start.AddDate(0, count*6, 0)
	default:
		return start.AddDate(0, 1, 0) // Default to 1 month
	}
}

// generateWarnings generates warnings about the subscription change
func (s *subscriptionChangeService) generateWarnings(
	currentSub *subscription.Subscription,
	targetPlan *plan.Plan,
	changeType types.SubscriptionChangeType,
	prorationBehavior types.ProrationBehavior,
) []string {
	warnings := []string{}

	// Warning for downgrades
	if changeType == types.SubscriptionChangeTypeDowngrade {
		warnings = append(warnings, "This is a downgrade. You may lose access to certain features.")
	}

	// Warning for trial subscriptions
	if currentSub.TrialEnd != nil && currentSub.TrialEnd.After(time.Now()) {
		warnings = append(warnings, "Changing plans during trial period may end your trial immediately.")
	}

	// Warning about proration
	if prorationBehavior == types.ProrationBehaviorCreateProrations {
		warnings = append(warnings, "Proration charges or credits will be applied to your next invoice.")
	}

	return warnings
}

// executeChange performs the actual subscription change
func (s *subscriptionChangeService) executeChange(
	ctx context.Context,
	currentSub *subscription.Subscription,
	lineItems []*subscription.SubscriptionLineItem,
	targetPlan *plan.Plan,
	changeType types.SubscriptionChangeType,
	req dto.SubscriptionChangeRequest,
	effectiveDate time.Time,
) (*dto.SubscriptionChangeExecuteResponse, error) {
	logger := s.serviceParams.Logger.With(
		zap.String("subscription_id", currentSub.ID),
		zap.String("target_plan_id", targetPlan.ID),
		zap.String("change_type", string(changeType)),
	)

	// Step 1: Calculate proration if needed
	var prorationDetails *dto.ProrationDetails
	if req.ProrationBehavior == types.ProrationBehaviorCreateProrations {
		var err error
		prorationDetails, err = s.calculateProrationPreview(ctx, currentSub, lineItems, targetPlan, effectiveDate)
		if err != nil {
			return nil, err
		}
	}

	// Step 2: Archive the old subscription
	archivedSub, err := s.archiveSubscription(ctx, currentSub, effectiveDate)
	if err != nil {
		return nil, err
	}

	// Step 3: Create new subscription
	newSub, err := s.createNewSubscription(ctx, currentSub, targetPlan, req, effectiveDate)
	if err != nil {
		return nil, err
	}

	// Step 4: Generate invoice if needed
	var invoice *dto.InvoiceResponse
	if req.InvoiceNow != nil && *req.InvoiceNow && prorationDetails != nil && prorationDetails.NetAmount.GreaterThan(decimal.Zero) {
		invoice, err = s.generateChangeInvoice(ctx, newSub, prorationDetails, effectiveDate)
		if err != nil {
			logger.Error("failed to generate change invoice", zap.Error(err))
			// Don't fail the entire operation for invoice generation issues
			// but log the error
		}
	}

	response := &dto.SubscriptionChangeExecuteResponse{
		OldSubscription: dto.SubscriptionSummary{
			ID:                 archivedSub.ID,
			Status:             archivedSub.SubscriptionStatus,
			PlanID:             archivedSub.PlanID,
			CurrentPeriodStart: archivedSub.CurrentPeriodStart,
			CurrentPeriodEnd:   archivedSub.CurrentPeriodEnd,
			BillingAnchor:      archivedSub.BillingAnchor,
			CreatedAt:          archivedSub.CreatedAt,
			ArchivedAt:         archivedSub.CancelledAt,
		},
		NewSubscription: dto.SubscriptionSummary{
			ID:                 newSub.ID,
			Status:             newSub.SubscriptionStatus,
			PlanID:             newSub.PlanID,
			CurrentPeriodStart: newSub.CurrentPeriodStart,
			CurrentPeriodEnd:   newSub.CurrentPeriodEnd,
			BillingAnchor:      newSub.BillingAnchor,
			CreatedAt:          newSub.CreatedAt,
		},
		ChangeType:       changeType,
		Invoice:          invoice,
		ProrationApplied: prorationDetails,
		EffectiveDate:    effectiveDate,
		Metadata:         req.Metadata,
	}

	return response, nil
}

// archiveSubscription marks the current subscription as cancelled/archived
func (s *subscriptionChangeService) archiveSubscription(
	ctx context.Context,
	currentSub *subscription.Subscription,
	effectiveDate time.Time,
) (*subscription.Subscription, error) {
	// Update subscription status to cancelled
	currentSub.SubscriptionStatus = types.SubscriptionStatusCancelled
	currentSub.CancelledAt = &effectiveDate
	currentSub.UpdatedAt = time.Now()
	currentSub.UpdatedBy = types.GetUserID(ctx)

	err := s.serviceParams.SubRepo.Update(ctx, currentSub)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to archive old subscription").
			Mark(ierr.ErrDatabase)
	}

	return currentSub, nil
}

// createNewSubscription creates a new subscription with the target plan using the existing subscription service
func (s *subscriptionChangeService) createNewSubscription(
	ctx context.Context,
	currentSub *subscription.Subscription,
	targetPlan *plan.Plan,
	req dto.SubscriptionChangeRequest,
	effectiveDate time.Time,
) (*subscription.Subscription, error) {
	// Determine start date based on billing cycle anchor behavior
	startDate := effectiveDate
	billingCycle := currentSub.BillingCycle

	// Adjust billing behavior if requested
	// if req.BillingCycleAnchor == types.BillingCycleAnchorReset {
	// 	// Reset billing cycle means we want the billing anchor to be the effective date
	// 	billingCycle = types.BillingCycleAnniversary
	// 	startDate = effectiveDate
	// } else if req.BillingCycleAnchor == types.BillingCycleAnchorImmediate {
	// 	// Immediate billing means we want to bill right away
	// 	billingCycle = types.BillingCycleAnniversary
	// 	startDate = effectiveDate
	// }
	// For unchanged, we keep the current billing cycle and use effective date as start

	// Build create subscription request based on current subscription but with new plan
	createSubReq := dto.CreateSubscriptionRequest{
		CustomerID:         currentSub.CustomerID,
		PlanID:             targetPlan.ID,
		Currency:           currentSub.Currency,
		LookupKey:          currentSub.LookupKey, // Keep same lookup key for continuity
		BillingCadence:     currentSub.BillingCadence,
		BillingPeriod:      currentSub.BillingPeriod,
		BillingPeriodCount: currentSub.BillingPeriodCount,
		BillingCycle:       billingCycle,
		StartDate:          &startDate,
		Metadata:           req.Metadata,
		ProrationMode:      types.ProrationModeActive, // Enable proration for new subscription
		CustomerTimezone:   currentSub.CustomerTimezone,
	}

	// Set trial end if provided
	if req.TrialEnd != nil {
		createSubReq.TrialEnd = req.TrialEnd
	}

	// Use the existing subscription service to create the new subscription
	response, err := s.subscriptionService.CreateSubscription(ctx, createSubReq)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to create new subscription using subscription service").
			Mark(ierr.ErrDatabase)
	}

	// Get the created subscription with line items
	newSub, _, err := s.serviceParams.SubRepo.GetWithLineItems(ctx, response.Subscription.ID)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve newly created subscription").
			Mark(ierr.ErrDatabase)
	}

	return newSub, nil
}

// generateChangeInvoice generates an invoice for the subscription change
func (s *subscriptionChangeService) generateChangeInvoice(
	ctx context.Context,
	newSub *subscription.Subscription,
	prorationDetails *dto.ProrationDetails,
	effectiveDate time.Time,
) (*dto.InvoiceResponse, error) {
	// Create invoice request for the proration
	invoiceReq := dto.CreateInvoiceRequest{
		CustomerID:     newSub.CustomerID,
		SubscriptionID: &newSub.ID,
		InvoiceType:    types.InvoiceTypeSubscription,
		BillingReason:  types.InvoiceBillingReasonSubscriptionUpdate,
		Description:    fmt.Sprintf("Subscription change - %s", effectiveDate.Format("2006-01-02")),
		Currency:       newSub.Currency,
		PeriodStart:    &prorationDetails.CurrentPeriodStart,
		PeriodEnd:      &prorationDetails.CurrentPeriodEnd,
		Subtotal:       prorationDetails.NetAmount,
		AmountDue:      prorationDetails.NetAmount,
		Metadata: map[string]string{
			"subscription_change": "true",
			"effective_date":      effectiveDate.Format(time.RFC3339),
		},
	}

	// Add line items for proration
	lineItems := []dto.CreateInvoiceLineItemRequest{}

	if prorationDetails.CreditAmount.GreaterThan(decimal.Zero) {
		displayName := prorationDetails.CreditDescription
		lineItems = append(lineItems, dto.CreateInvoiceLineItemRequest{
			DisplayName: &displayName,
			Amount:      prorationDetails.CreditAmount.Neg(),
			Quantity:    decimal.NewFromInt(1),
			PeriodStart: &prorationDetails.CurrentPeriodStart,
			PeriodEnd:   &effectiveDate,
		})
	}

	if prorationDetails.ChargeAmount.GreaterThan(decimal.Zero) {
		displayName := prorationDetails.ChargeDescription
		lineItems = append(lineItems, dto.CreateInvoiceLineItemRequest{
			DisplayName: &displayName,
			Amount:      prorationDetails.ChargeAmount,
			Quantity:    decimal.NewFromInt(1),
			PeriodStart: &effectiveDate,
			PeriodEnd:   &prorationDetails.CurrentPeriodEnd,
		})
	}

	invoiceReq.LineItems = lineItems

	// Create the invoice
	return s.invoiceService.CreateInvoice(ctx, invoiceReq)
}
