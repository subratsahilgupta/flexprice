package service

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/addonassociation"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/idempotency"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

type lineItemChange struct {
	lineItem *subscription.SubscriptionLineItem
	price    *price.Price

	// Set only for dropped addon lines (needed to close the association).
	association *addonassociation.AddonAssociation
}

func (c lineItemChange) isAddon() bool {
	return c.lineItem != nil &&
		c.lineItem.EntityType == types.SubscriptionLineItemEntityTypeAddon &&
		c.association != nil
}

type planChangeRequest struct {
	sub         *subscription.Subscription
	fromPlan    *plan.Plan
	toPlan      *plan.Plan
	effectiveAt time.Time
	behavior    types.ProrationBehavior

	// Prices that bill identically keep their line-item id / usage window.
	unchanged []*subscription.SubscriptionLineItem
	closing   []lineItemChange
	opening   []lineItemChange

	dispositions []dto.EntityDispositionResult
	warnings     []string

	changeType types.SubscriptionChangeType
}

func (s *subscriptionService) resolvePlanChange(
	ctx context.Context,
	sub *subscription.Subscription,
	req dto.SubscriptionChangeV2Request,
	effectiveAt time.Time,
) (*planChangeRequest, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	planSvc := NewPlanService(s.ServiceParams)
	toPlan, err := planSvc.GetPlan(ctx, req.TargetPlanID)
	if err != nil {
		return nil, err
	}
	fromPlan, err := planSvc.GetPlan(ctx, sub.PlanID)
	if err != nil {
		return nil, err
	}

	if err := s.checkPlanChangePreconditions(sub, req); err != nil {
		return nil, err
	}

	targetPrices, err := s.ValidateAndFilterPricesForSubscription(
		ctx, toPlan.Plan.ID, types.PRICE_ENTITY_TYPE_PLAN, sub, nil,
	)
	if err != nil {
		return nil, err
	}
	if len(targetPrices) == 0 {
		return nil, ierr.NewError("target plan has no prices compatible with this subscription").
			WithHint("The target plan must have prices matching the subscription's currency and billing period. Use the v1 change endpoint to change interval or currency.").
			WithReportableDetails(map[string]any{
				"subscription_id": sub.ID,
				"target_plan_id":  toPlan.Plan.ID,
				"currency":        sub.Currency,
				"billing_period":  sub.BillingPeriod,
			}).
			Mark(ierr.ErrValidation)
	}

	r := &planChangeRequest{
		sub:         sub,
		fromPlan:    fromPlan.Plan,
		toPlan:      toPlan.Plan,
		effectiveAt: effectiveAt,
		behavior:    req.ProrationBehavior,
	}
	if err := s.resolveLineItems(ctx, r, targetPrices, toPlan); err != nil {
		return nil, err
	}

	if err := s.resolveAddonDispositions(ctx, r, req); err != nil {
		return nil, err
	}

	r.changeType = planChangeTypeFor(r)
	return r, nil
}

func (s *subscriptionService) checkPlanChangePreconditions(
	sub *subscription.Subscription,
	req dto.SubscriptionChangeV2Request,
) error {
	fail := func(msg, hint string, details map[string]any) error {
		return ierr.NewError(msg).WithHint(hint).WithReportableDetails(details).Mark(ierr.ErrValidation)
	}

	if sub.PlanID == req.TargetPlanID {
		return fail("subscription is already on the target plan",
			"The subscription is already on this plan",
			map[string]any{"subscription_id": sub.ID, "plan_id": sub.PlanID})
	}

	if sub.SubscriptionStatus != types.SubscriptionStatusActive &&
		sub.SubscriptionStatus != types.SubscriptionStatusTrialing {
		return fail("subscription is not active",
			"Only active or trialing subscriptions can change plan",
			map[string]any{"subscription_id": sub.ID, "subscription_status": sub.SubscriptionStatus})
	}

	if sub.PauseStatus == types.PauseStatusActive || sub.PauseStatus == types.PauseStatusScheduled {
		return fail("subscription is paused",
			"Resume the subscription before changing its plan",
			map[string]any{"subscription_id": sub.ID, "pause_status": sub.PauseStatus})
	}

	if sub.SubscriptionType != types.SubscriptionTypeStandalone {
		return fail("subscription is part of a hierarchy",
			"Hierarchy subscriptions are not supported by this endpoint. Use the v1 change endpoint.",
			map[string]any{"subscription_id": sub.ID, "subscription_type": sub.SubscriptionType})
	}

	if sub.CancelAtPeriodEnd || sub.CancelAt != nil {
		return fail("subscription has a pending cancellation",
			"Clear the pending cancellation before changing plan",
			map[string]any{"subscription_id": sub.ID})
	}

	// TODO: Handle mixed billing periods correctly.
	if req.ProrationBehavior == types.ProrationBehaviorCreateProrations && sub.HasMixedBillingPeriods() {
		return fail("proration is not supported for subscriptions with mixed billing periods",
			"Set proration_behavior to 'none' to change this subscription's plan",
			map[string]any{"subscription_id": sub.ID})
	}

	for _, item := range sub.LineItems {
		if item.SubscriptionPhaseID != nil {
			return fail("subscription has phases",
				"Phased subscriptions are not supported by this endpoint. Use the v1 change endpoint.",
				map[string]any{"subscription_id": sub.ID})
		}
	}

	return nil
}

// Leave lines alone when the target price bills identically; otherwise close/open.
func (s *subscriptionService) resolveLineItems(
	ctx context.Context,
	r *planChangeRequest,
	targetPrices []*dto.PriceResponse,
	toPlan *dto.PlanResponse,
) error {
	priceSvc := NewPriceService(s.ServiceParams)

	current := make([]lineItemChange, 0, len(r.sub.LineItems))
	for _, item := range r.sub.LineItems {
		if item.EntityType != types.SubscriptionLineItemEntityTypePlan || !item.EndDate.IsZero() {
			continue
		}
		p, err := priceSvc.GetPrice(ctx, item.PriceID)
		if err != nil {
			return err
		}
		current = append(current, lineItemChange{lineItem: item, price: p.Price})
	}

	matchedTarget := make([]bool, len(targetPrices))
	matchedCurrent := make([]bool, len(current))

	for i, live := range current {
		for j, target := range targetPrices {
			if matchedTarget[j] || !billsIdentically(live.price, target.Price) {
				continue
			}

			matchedTarget[j], matchedCurrent[i] = true, true
			r.unchanged = append(r.unchanged, live.lineItem)
			break
		}
	}

	for i, live := range current {
		if !matchedCurrent[i] {
			r.closing = append(r.closing, live)
		}
	}

	subResp := &dto.SubscriptionResponse{Subscription: r.sub}
	for j, target := range targetPrices {
		if matchedTarget[j] {
			continue
		}
		item, err := buildPlanChangeLineItem(ctx, subResp, target, toPlan, r.effectiveAt)
		if err != nil {
			return err
		}
		r.opening = append(r.opening, lineItemChange{lineItem: item, price: target.Price})
	}

	return nil
}

func buildPlanChangeLineItem(
	ctx context.Context,
	subResp *dto.SubscriptionResponse,
	target *dto.PriceResponse,
	toPlan *dto.PlanResponse,
	effectiveAt time.Time,
) (*subscription.SubscriptionLineItem, error) {
	lineItemReq := &dto.CreateSubscriptionLineItemRequest{
		PriceID:   target.Price.ID,
		StartDate: &effectiveAt,
	}
	if err := lineItemReq.Validate(target.Price, subResp.Subscription); err != nil {
		return nil, err
	}

	item := lineItemReq.ToSubscriptionLineItem(ctx, dto.LineItemParams{
		Subscription: subResp,
		Price:        target,
		Plan:         toPlan,
		EntityType:   types.SubscriptionLineItemEntityTypePlan,
	})

	item.BillingPeriodCount = target.Price.BillingPeriodCount
	return item, nil
}

// billsIdentically reports whether two prices bill the same way, which is the
// only case where leaving a line item untouched is correct.
//
// It is deliberately conservative: a false "identical" leaves a line on the old
// price forever, which is worse than the churn of slicing one that did not need
// it. So anything it cannot compare completely returns false.
//
//   - USAGE prices never match. Separating them needs filter_values, which is
//     not on the domain price model, so two prices on one meter split by filter
//     (region=us vs region=eu) would compare equal and be treated as one charge.
//   - Tiered prices never match; comparing ladders is more subtlety than a
//     shortcut is worth.
func billsIdentically(a, b *price.Price) bool {
	if a == nil || b == nil {
		return false
	}

	if a.Type == types.PRICE_TYPE_USAGE || b.Type == types.PRICE_TYPE_USAGE {
		return false
	}

	if len(a.Tiers) > 0 || len(b.Tiers) > 0 {
		return false
	}

	// PACKAGE prices at the same amount still differ if they bundle a different
	// quantity per package.
	if a.TransformQuantity != b.TransformQuantity {
		return false
	}

	return a.Amount.Equal(b.Amount) &&
		a.Type == b.Type &&
		a.MeterID == b.MeterID &&
		a.BillingModel == b.BillingModel &&
		a.InvoiceCadence == b.InvoiceCadence &&
		a.BillingPeriod == b.BillingPeriod &&
		a.BillingPeriodCount == b.BillingPeriodCount
}

// Uses recurring amounts (not prorated net) so timing/proration don't change the type.
func planChangeTypeFor(r *planChangeRequest) types.SubscriptionChangeType {
	oldTotal, newTotal := decimal.Zero, decimal.Zero
	for _, move := range r.closing {
		if move.isAddon() {
			continue
		}
		oldTotal = oldTotal.Add(move.price.Amount)
	}
	for _, move := range r.opening {
		newTotal = newTotal.Add(move.price.Amount)
	}

	switch {
	case newTotal.GreaterThan(oldTotal):
		return types.SubscriptionChangeTypeUpgrade
	case newTotal.LessThan(oldTotal):
		return types.SubscriptionChangeTypeDowngrade
	default:
		return types.SubscriptionChangeTypeLateral
	}
}

func (s *subscriptionService) resolveAddonDispositions(
	ctx context.Context,
	r *planChangeRequest,
	req dto.SubscriptionChangeV2Request,
) error {
	var policy *dto.EntityChangePolicy
	if req.EntityPolicies != nil {
		policy = req.EntityPolicies.Addons
	}

	associations, err := s.activeAddonAssociations(ctx, r.sub.ID)
	if err != nil {
		return err
	}

	seen := make(map[string]bool, len(associations))
	for _, association := range associations {
		seen[association.ID] = true

		disposition := policy.DispositionFor(association.ID)
		r.dispositions = append(r.dispositions, dto.EntityDispositionResult{
			EntityType:  "addon",
			ReferenceID: association.ID,
			EntityID:    association.AddonID,
			Disposition: disposition,
		})

		if disposition != types.EntityDispositionDrop {
			continue
		}

		if err := s.closeAddonLineItems(ctx, r, association); err != nil {
			return err
		}
	}

	// Stale override keys (e.g. preview→execute race) warn instead of failing.
	if policy != nil {
		for referenceID := range policy.Overrides {
			if !seen[referenceID] {
				r.warnings = append(r.warnings,
					"ignored addon disposition for unknown or inactive association "+referenceID)
			}
		}
	}

	return nil
}

func (s *subscriptionService) activeAddonAssociations(
	ctx context.Context,
	subscriptionID string,
) ([]*addonassociation.AddonAssociation, error) {
	resp, err := NewAddonService(s.ServiceParams).GetActiveAddonAssociation(ctx, dto.GetActiveAddonAssociationRequest{
		EntityID:   subscriptionID,
		EntityType: types.AddonAssociationEntityTypeSubscription,
	})
	if err != nil {
		return nil, err
	}

	associations := make([]*addonassociation.AddonAssociation, 0, len(resp.Items))
	for _, item := range resp.Items {
		if item.AddonAssociation != nil {
			associations = append(associations, item.AddonAssociation)
		}
	}
	return associations, nil
}

func (s *subscriptionService) closeAddonLineItems(
	ctx context.Context,
	r *planChangeRequest,
	association *addonassociation.AddonAssociation,
) error {
	filter := types.NewSubscriptionLineItemFilter()
	filter.SubscriptionIDs = []string{r.sub.ID}
	filter.AddonAssociationIDs = []string{association.ID}

	items, err := s.SubscriptionLineItemRepo.List(ctx, filter)
	if err != nil {
		return err
	}

	priceSvc := NewPriceService(s.ServiceParams)
	for _, item := range items {
		if !item.EndDate.IsZero() {
			continue
		}
		p, err := priceSvc.GetPrice(ctx, item.PriceID)
		if err != nil {
			return err
		}
		r.closing = append(r.closing, lineItemChange{
			lineItem:    item,
			price:       p.Price,
			association: association,
		})
	}
	return nil
}

func (s *subscriptionService) applyDroppedAddons(ctx context.Context, r *planChangeRequest) error {
	closed := make(map[string]bool)
	for _, change := range r.closing {
		if !change.isAddon() || closed[change.association.ID] {
			continue
		}
		closed[change.association.ID] = true

		association := change.association
		association.EndDate = lo.ToPtr(r.effectiveAt)
		association.CancelledAt = lo.ToPtr(r.effectiveAt)
		association.AddonStatus = types.AddonStatusCancelled
		if err := s.AddonAssociationRepo.Update(ctx, association); err != nil {
			return err
		}

		// KNOWN LIMITATION: grants are tagged by addon_id only, so concurrent
		// attachments of the same addon cannot be cancelled independently.
		if err := NewCreditGrantService(s.ServiceParams).CancelFutureSubscriptionGrants(ctx, dto.CancelFutureSubscriptionGrantsRequest{
			SubscriptionID: r.sub.ID,
			AddonID:        lo.ToPtr(association.AddonID),
			EffectiveDate:  lo.ToPtr(r.effectiveAt),
		}); err != nil {
			return err
		}
	}

	return nil
}

func (s *subscriptionService) computePlanChange(
	ctx context.Context,
	r *planChangeRequest,
) (*LineItemProrationSummary, error) {
	entries := make([]LineItemProrationEntry, 0, len(r.closing)+len(r.opening))
	for _, move := range r.closing {
		entries = append(entries, LineItemProrationEntry{
			LineItem: move.lineItem,
			Price:    move.price,
			Action:   types.ProrationActionRemoveItem,
		})
	}
	for _, move := range r.opening {
		entries = append(entries, LineItemProrationEntry{
			LineItem:    move.lineItem,
			Price:       move.price,
			Action:      types.ProrationActionAddItem,
			NewQuantity: move.lineItem.Quantity,
		})
	}

	return NewLineItemProrationService(s.ServiceParams).Compute(ctx, LineItemProrationRequest{
		Subscription:  r.sub,
		Entries:       entries,
		EffectiveDate: r.effectiveAt,
		Behavior:      r.behavior,
		Reason:        "plan change",
	})
}

func (s *subscriptionService) PreviewPlanChange(
	ctx context.Context,
	subscriptionID string,
	req dto.SubscriptionChangeV2Request,
) (*dto.SubscriptionChangeV2Response, error) {
	sub, err := s.loadSubscriptionForPlanChange(ctx, subscriptionID, false)
	if err != nil {
		return nil, err
	}

	r, err := s.resolvePlanChange(ctx, sub, req, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	quote, err := s.computePlanChange(ctx, r)
	if err != nil {
		return nil, err
	}

	resp := buildPlanChangeResponse(r, req)
	resp.ChangedResources.LineItems = planChangeChangedLineItems(r)
	resp.ChangedResources.Invoices = s.planChangePreviewInvoices(ctx, r, quote)
	return resp, nil
}

func (s *subscriptionService) ExecutePlanChange(
	ctx context.Context,
	subscriptionID string,
	req dto.SubscriptionChangeV2Request,
) (*dto.SubscriptionChangeV2Response, error) {
	var resp *dto.SubscriptionChangeV2Response

	err := s.DB.WithTx(ctx, func(txCtx context.Context) error {
		sub, err := s.loadSubscriptionForPlanChange(txCtx, subscriptionID, true)
		if err != nil {
			return err
		}

		r, err := s.resolvePlanChange(txCtx, sub, req, time.Now().UTC())
		if err != nil {
			return err
		}

		quote, err := s.computePlanChange(txCtx, r)
		if err != nil {
			return err
		}

		if err := s.applyPlanChangeLineItems(txCtx, r); err != nil {
			return err
		}

		if err := s.applyDroppedAddons(txCtx, r); err != nil {
			return err
		}

		if err := s.applyPlanSwap(txCtx, r); err != nil {
			return err
		}

		changedInvoices, err := s.settlePlanChange(txCtx, r, quote)
		if err != nil {
			return err
		}

		resp = buildPlanChangeResponse(r, req)
		resp.ChangedResources.LineItems = planChangeChangedLineItems(r)
		resp.ChangedResources.Invoices = changedInvoices
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.Logger.Info(ctx, "subscription plan changed",
		"subscription_id", subscriptionID,
		"from_plan_id", resp.FromPlan.ID,
		"to_plan_id", resp.ToPlan.ID,
		"change_type", resp.ChangeType)

	s.attemptPlanChangePayment(ctx, resp)

	// After commit so webhook failure cannot roll back a completed change.
	s.publishSystemEvent(ctx, types.WebhookEventSubscriptionPlanChanged, subscriptionID)
	s.publishSystemEvent(ctx, types.WebhookEventSubscriptionUpdated, subscriptionID)

	return resp, nil
}

func (s *subscriptionService) attemptPlanChangePayment(ctx context.Context, resp *dto.SubscriptionChangeV2Response) {
	invoiceSvc := NewInvoiceService(s.ServiceParams)
	for _, changed := range resp.ChangedResources.Invoices {
		if changed.Invoice == nil || changed.Invoice.ID == "" {
			continue
		}

		if err := invoiceSvc.AttemptPayment(ctx, changed.Invoice.ID); err != nil {
			s.Logger.Info(ctx, "plan change invoice created but payment attempt failed; invoice remains collectable",
				"error", err, "invoice_id", changed.Invoice.ID)
		}
	}
}

// forUpdate takes the row lock as the first read so concurrent changes serialize.
func (s *subscriptionService) loadSubscriptionForPlanChange(
	ctx context.Context,
	subscriptionID string,
	forUpdate bool,
) (*subscription.Subscription, error) {
	var (
		sub *subscription.Subscription
		err error
	)
	if forUpdate {
		sub, err = s.SubRepo.GetForUpdate(ctx, subscriptionID)
	} else {
		sub, err = s.SubRepo.Get(ctx, subscriptionID)
	}
	if err != nil {
		return nil, err
	}

	lineItems, err := s.SubscriptionLineItemRepo.ListBySubscription(ctx, sub)
	if err != nil {
		return nil, err
	}
	sub.LineItems = lineItems
	return sub, nil
}

func (s *subscriptionService) applyPlanChangeLineItems(ctx context.Context, r *planChangeRequest) error {
	// A line left alone still belongs to the subscription, and the subscription is
	// moving. Without this its entity_id keeps naming the old plan and its
	// plan_display_name — copied onto every future invoice line — keeps showing
	// the old plan's name to the customer.
	for _, item := range r.unchanged {
		updated := subscription.NewSubscriptionLineItemBuilder(item).
			WithOwningPlan(r.toPlan.ID, r.toPlan.Name).
			Build()
		if err := s.SubscriptionLineItemRepo.Update(ctx, updated); err != nil {
			return err
		}
	}

	for _, move := range r.closing {
		move.lineItem.EndDate = r.effectiveAt
		if err := s.SubscriptionLineItemRepo.Update(ctx, move.lineItem); err != nil {
			return err
		}
	}

	if len(r.opening) == 0 {
		return nil
	}
	toCreate := make([]*subscription.SubscriptionLineItem, 0, len(r.opening))
	for _, move := range r.opening {
		toCreate = append(toCreate, move.lineItem)
	}
	return s.SubscriptionLineItemRepo.CreateBulk(ctx, toCreate)
}

// Re-anchor synced_price_sequence; the old plan's watermark is meaningless on the new plan.
func (s *subscriptionService) applyPlanSwap(ctx context.Context, r *planChangeRequest) error {
	r.sub.PlanID = r.toPlan.ID
	if err := s.SubRepo.Update(ctx, r.sub); err != nil {
		return err
	}

	targetSeq, err := s.PlanPriceSyncRepo.CurrentPlanSequence(ctx, r.toPlan.ID)
	if err != nil {
		return err
	}

	return s.PlanPriceSyncRepo.ReanchorSubSyncedSequence(ctx, r.sub.ID, targetSeq)
}

// Net charge → one invoice (credits as lines); net credit → wallet (invoice totals can't go negative).
func (s *subscriptionService) settlePlanChange(
	ctx context.Context,
	r *planChangeRequest,
	quote *LineItemProrationSummary,
) ([]dto.ChangedInvoice, error) {
	if r.behavior != types.ProrationBehaviorCreateProrations {
		return nil, nil
	}

	switch {
	case quote.NetAmount().GreaterThan(decimal.Zero):
		inv, err := s.createPlanChangeInvoice(ctx, r, quote)
		if err != nil {
			return nil, err
		}

		return []dto.ChangedInvoice{{
			ID:      inv.ID,
			Action:  dto.ChangedInvoiceActionCreated,
			Status:  dto.ChangedInvoiceStatusFromPaymentStatus(inv.PaymentStatus),
			Invoice: inv,
		}}, nil

	case quote.NetAmount().LessThan(decimal.Zero):
		walletSvc := NewWalletService(s.ServiceParams)
		if _, err := walletSvc.TopUpWalletForProratedCharge(
			ctx, r.sub.GetInvoicingCustomerID(), quote.NetAmount().Abs(), r.sub.Currency, planChangeIdempotencyKey(r),
		); err != nil {
			return nil, err
		}

		return nil, nil
	default:
		return nil, nil
	}
}

func (s *subscriptionService) createPlanChangeInvoice(
	ctx context.Context,
	r *planChangeRequest,
	quote *LineItemProrationSummary,
) (*dto.InvoiceResponse, error) {
	lineItems := make([]dto.CreateInvoiceLineItemRequest, 0,
		len(quote.ChargeLineItems)+len(quote.CreditLineItems))
	lineItems = append(lineItems, quote.ChargeLineItems...)
	lineItems = append(lineItems, quote.CreditLineItems...)

	billingPeriod := string(r.sub.BillingPeriod)
	periodEnd := r.sub.CurrentPeriodEnd
	idempotencyKey := planChangeIdempotencyKey(r)

	return NewInvoiceService(s.ServiceParams).CreateInvoice(ctx, dto.CreateInvoiceRequest{
		CustomerID:     r.sub.GetInvoicingCustomerID(),
		SubscriptionID: &r.sub.ID,
		InvoiceType:    types.InvoiceTypeOneOff,
		Currency:       r.sub.Currency,
		BillingReason:  types.InvoiceBillingReasonSubscriptionUpdate,
		AmountDue:      quote.NetAmount(),
		Total:          quote.NetAmount(),
		Subtotal:       quote.NetAmount(),
		PeriodStart:    &r.effectiveAt,
		PeriodEnd:      &periodEnd,
		BillingPeriod:  &billingPeriod,
		LineItems:      lineItems,
		IdempotencyKey: &idempotencyKey,
	})
}

func buildPlanChangeResponse(
	r *planChangeRequest,
	req dto.SubscriptionChangeV2Request,
) *dto.SubscriptionChangeV2Response {
	return &dto.SubscriptionChangeV2Response{
		Subscription: &dto.SubscriptionResponse{Subscription: r.sub},
		ChangeType:   r.changeType,
		EffectiveAt:  r.effectiveAt,
		FromPlan:     planChangeSummary(r.fromPlan),
		ToPlan:       planChangeSummary(r.toPlan),

		EntityDispositions: r.dispositions,
		Warnings:           r.warnings,
		Metadata:           req.Metadata,
	}
}

func planChangeSummary(p *plan.Plan) dto.PlanSummary {
	if p == nil {
		return dto.PlanSummary{}
	}
	return dto.PlanSummary{
		ID:          p.ID,
		Name:        p.Name,
		LookupKey:   p.LookupKey,
		Description: p.Description,
	}
}

func planChangeChangedLineItems(r *planChangeRequest) []dto.ChangedLineItem {
	items := make([]dto.ChangedLineItem, 0, len(r.closing)+len(r.opening))
	for _, move := range r.closing {
		endDate := r.effectiveAt
		items = append(items, dto.ChangedLineItem{
			ID:           move.lineItem.ID,
			PriceID:      move.lineItem.PriceID,
			Quantity:     move.lineItem.Quantity,
			StartDate:    &move.lineItem.StartDate,
			EndDate:      &endDate,
			ChangeAction: dto.ChangedLineItemActionEnded,
		})
	}
	for _, move := range r.opening {
		startDate := move.lineItem.StartDate
		items = append(items, dto.ChangedLineItem{
			ID:           move.lineItem.ID,
			PriceID:      move.lineItem.PriceID,
			Quantity:     move.lineItem.Quantity,
			StartDate:    &startDate,
			ChangeAction: dto.ChangedLineItemActionCreated,
		})
	}
	return items
}

func (s *subscriptionService) planChangePreviewInvoices(
	ctx context.Context,
	r *planChangeRequest,
	quote *LineItemProrationSummary,
) []dto.ChangedInvoice {
	if r.behavior != types.ProrationBehaviorCreateProrations || quote.NetAmount().IsZero() {
		return nil
	}

	subtotal := quote.NetAmount()
	tax := s.previewTaxFor(ctx, r, subtotal)

	return []dto.ChangedInvoice{{
		Action: dto.ChangedInvoiceActionCreated,
		Status: dto.ChangedInvoiceStatusPreview,
		Invoice: &dto.InvoiceResponse{
			Invoice: invoice.Invoice{
				Subtotal:  subtotal,
				TotalTax:  tax,
				Total:     subtotal.Add(tax),
				AmountDue: subtotal.Add(tax),
				Currency:  r.sub.Currency,
			},
		},
	}}
}

func (s *subscriptionService) previewTaxFor(
	ctx context.Context,
	r *planChangeRequest,
	subtotal decimal.Decimal,
) decimal.Decimal {
	if !subtotal.IsPositive() {
		return decimal.Zero
	}

	taxSvc := NewTaxService(s.ServiceParams)
	periodEnd := r.sub.CurrentPeriodEnd

	rates, err := taxSvc.PrepareTaxRatesForInvoice(ctx, dto.CreateInvoiceRequest{
		CustomerID:     r.sub.GetInvoicingCustomerID(),
		SubscriptionID: &r.sub.ID,
		Currency:       r.sub.Currency,
		PeriodStart:    &r.effectiveAt,
		PeriodEnd:      &periodEnd,
	})
	if err != nil {
		s.Logger.Info(ctx, "could not resolve tax rates for plan change preview; quoting untaxed",
			"error", err, "subscription_id", r.sub.ID)
		return decimal.Zero
	}
	if len(rates) == 0 {
		return decimal.Zero
	}

	result, err := taxSvc.ApplyTaxesOnInvoice(ctx, &invoice.Invoice{
		ID:       r.sub.ID,
		Currency: r.sub.Currency,
		Subtotal: subtotal,
	}, rates)
	if err != nil {
		s.Logger.Info(ctx, "could not compute tax for plan change preview; quoting untaxed",
			"error", err, "subscription_id", r.sub.ID)
		return decimal.Zero
	}

	return result.TotalTaxAmount
}

// Hash to fit invoices.idempotency_key (varchar(100)).
func planChangeIdempotencyKey(r *planChangeRequest) string {
	return idempotency.NewGenerator().GenerateKey(idempotency.ScopePlanChange, map[string]interface{}{
		"subscription_id": r.sub.ID,
		"target_plan_id":  r.toPlan.ID,
		"effective_at":    r.effectiveAt.UTC().Format(time.RFC3339Nano),
	})
}
