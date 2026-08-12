package service

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// Plan change, swap-in-place: the subscription row survives with its id, billing
// anchor and period bounds intact; only plan_id and the plan-derived line items
// move. Cancel-and-recreate could offer none of that.
//
// Three stages, and Preview is exactly Execute minus the last one:
//
//	resolve  — decide everything. Reads only.
//	compute  — price the decision. Reads only.
//	settle   — the sole stage that writes.
//
// Preview and Execute call the same resolve and compute, so a quote and its
// execution cannot drift apart. That is also the seam a pay-first checkout slots
// into later: a third settler, not a change to the first two stages.

// ─── Stage 1: resolve ────────────────────────────────────────────────────────

// lineMove pairs a line item with the price that governs it, for one side of the
// slice: a line being closed, or one being opened.
type lineMove struct {
	lineItem *subscription.SubscriptionLineItem
	price    *price.Price
}

// planChangeRequest is the fully resolved intent: every decision made, no money
// computed, nothing written.
type planChangeRequest struct {
	sub         *subscription.Subscription
	fromPlan    *plan.Plan
	toPlan      *plan.Plan
	effectiveAt time.Time
	behavior    types.ProrationBehavior

	// Services the target plan prices identically to the current one. These are
	// left completely alone — not closed and reopened — so an unchanged service
	// keeps its line-item id and its usage window.
	unchanged []*subscription.SubscriptionLineItem
	closing   []lineMove
	opening   []lineMove

	changeType types.SubscriptionChangeType
}

// resolvePlanChange loads the target plan, rejects everything the swap engine
// cannot honour, and works out which line items move.
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

	// Prices the target plan can actually offer this subscription: currency and
	// billing period have to line up, which is why an interval change is a 4xx.
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
	if err := s.planLineMoves(ctx, r, targetPrices, toPlan); err != nil {
		return nil, err
	}

	r.changeType = planChangeTypeFor(r)
	return r, nil
}

// checkPlanChangePreconditions rejects, before any write, every shape the swap
// engine does not handle. Each hint names the v1 endpoint as the way forward.
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

	for _, item := range sub.LineItems {
		if item.SubscriptionPhaseID != nil {
			return fail("subscription has phases",
				"Phased subscriptions are not supported by this endpoint. Use the v1 change endpoint.",
				map[string]any{"subscription_id": sub.ID})
		}
	}

	return nil
}

// planLineMoves works out which plan line items close, which open, and which are
// left alone.
//
// A target price that bills identically to a live line is treated as the same
// service continuing: the row is not touched. Slicing it would give an unchanged
// service a new line-item id, split its usage window, and emit a charge and a
// credit that cancel. Everything else closes and its replacements open.
func (s *subscriptionService) planLineMoves(
	ctx context.Context,
	r *planChangeRequest,
	targetPrices []*dto.PriceResponse,
	toPlan *dto.PlanResponse,
) error {
	priceSvc := NewPriceService(s.ServiceParams)

	current := make([]lineMove, 0, len(r.sub.LineItems))
	for _, item := range r.sub.LineItems {
		if item.EntityType != types.SubscriptionLineItemEntityTypePlan || !item.EndDate.IsZero() {
			continue
		}
		p, err := priceSvc.GetPrice(ctx, item.PriceID)
		if err != nil {
			return err
		}
		current = append(current, lineMove{lineItem: item, price: p.Price})
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
		r.opening = append(r.opening, lineMove{lineItem: item, price: target.Price})
	}

	return nil
}

// buildPlanChangeLineItem constructs (without persisting) the line item for a
// target price, starting at the effective date.
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

// billsIdentically reports whether two prices produce the same bill for the same
// usage. Only then is leaving a line item untouched correct.
// TODO: Handle tiered prices correctly.
func billsIdentically(a, b *price.Price) bool {
	if a == nil || b == nil {
		return false
	}

	if len(a.Tiers) > 0 || len(b.Tiers) > 0 {
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

// planChangeTypeFor compares recurring value rather than the prorated net, so the
// answer does not depend on when in the period the change lands, or on whether
// proration was requested at all.
func planChangeTypeFor(r *planChangeRequest) types.SubscriptionChangeType {
	oldTotal, newTotal := decimal.Zero, decimal.Zero
	for _, move := range r.closing {
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

// ─── Stage 2: compute ────────────────────────────────────────────────────────

// computePlanChange prices the resolved change. Charges and credits are netted
// across every entry rather than settled as two independent totals, so a change
// that gives back as much as it takes produces nothing at all.
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

// ─── Stage 3: settle ─────────────────────────────────────────────────────────

// PreviewPlanChange returns the money and line-item movements a change would
// produce, writing nothing.
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
	resp.ChangedResources.Invoices = planChangePreviewInvoices(r, quote)

	return resp, nil
}

// ExecutePlanChange applies the change. Every database write happens in one
// transaction, so the change either fully happens or fully does not — the
// subscription is never left swapped but unbilled.
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

	return resp, nil
}

// loadSubscriptionForPlanChange reads the subscription and its line items.
// Execute takes a row lock first, so two concurrent changes serialise on the
// subscription instead of both reading the same state and both writing line
// items. The lock must be the first read: a decision made before it is a
// decision made on state another writer can still change.
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

// applyPlanChangeLineItems slices the plan lines at the effective date. Lines
// left unchanged are not touched at all.
func (s *subscriptionService) applyPlanChangeLineItems(ctx context.Context, r *planChangeRequest) error {
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

// applyPlanSwap mutates plan_id and re-anchors the plan-price watermark. That
// watermark only means anything relative to one plan, so carrying the old value
// would hide the subscription from plan-price sync for good.
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

// settlePlanChange turns the quote into money. Charges and credits net onto one
// invoice so the credit is visible as its own line rather than being paid out
// separately; a net credit goes to the wallet, since a non-credit invoice cannot
// carry a negative total.
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

// ─── Response ────────────────────────────────────────────────────────────────

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
		Metadata:     req.Metadata,
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

// planChangePreviewInvoices renders the money a change would move without
// creating anything, so a caller sees the amount rather than an empty response.
func planChangePreviewInvoices(r *planChangeRequest, quote *LineItemProrationSummary) []dto.ChangedInvoice {
	if r.behavior != types.ProrationBehaviorCreateProrations || quote.NetAmount().IsZero() {
		return nil
	}
	return []dto.ChangedInvoice{{
		Action: dto.ChangedInvoiceActionCreated,
		Status: dto.ChangedInvoiceStatusPreview,
		Invoice: &dto.InvoiceResponse{
			Invoice: invoice.Invoice{
				AmountDue: quote.NetAmount(),
				Total:     quote.NetAmount(),
				Currency:  r.sub.Currency,
			},
		},
	}}
}

func planChangeIdempotencyKey(r *planChangeRequest) string {
	return "plan_change_" + r.sub.ID + "_" + r.toPlan.ID + "_" +
		r.effectiveAt.UTC().Format(time.RFC3339Nano)
}
