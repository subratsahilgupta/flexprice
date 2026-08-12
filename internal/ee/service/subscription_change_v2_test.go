package service

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

// SubscriptionPlanChangeSuite covers the core swap engine: plan lines only, no
// addons. The properties under test are the ones cancel-and-recreate could not
// offer — the subscription row survives, the anchor never moves, and a service
// the target plan prices identically is left completely alone.
type SubscriptionChangeV2Suite struct {
	testutil.BaseServiceTestSuite
	svc interfaces.SubscriptionService
	td  changeV2TestData
}

type changeV2TestData struct {
	customer    *customer.Customer
	starter     *plan.Plan
	pro         *plan.Plan
	starterBase *price.Price
	proBase     *price.Price
	sub         *subscription.Subscription
	baseLine    *subscription.SubscriptionLineItem
	periodStart time.Time
	periodEnd   time.Time
}

func TestSubscriptionChangeV2(t *testing.T) {
	suite.Run(t, new(SubscriptionChangeV2Suite))
}

func (s *SubscriptionChangeV2Suite) SetupTest() {
	s.BaseServiceTestSuite.SetupTest()
	s.svc = NewSubscriptionService(s.serviceParams())
	s.setupTestData()
}

func (s *SubscriptionChangeV2Suite) TearDownTest() {
	s.BaseServiceTestSuite.TearDownTest()
}

func (s *SubscriptionChangeV2Suite) serviceParams() ServiceParams {
	st := s.GetStores()
	return ServiceParams{
		Logger:                     s.GetLogger(),
		Config:                     s.GetConfig(),
		DB:                         s.GetDB(),
		SubRepo:                    st.SubscriptionRepo,
		SubscriptionLineItemRepo:   st.SubscriptionLineItemRepo,
		SubscriptionPhaseRepo:      st.SubscriptionPhaseRepo,
		SubScheduleRepo:            st.SubscriptionScheduleRepo,
		PlanRepo:                   st.PlanRepo,
		PriceRepo:                  st.PriceRepo,
		PriceUnitRepo:              st.PriceUnitRepo,
		EventRepo:                  st.EventRepo,
		MeterRepo:                  st.MeterRepo,
		CustomerRepo:               st.CustomerRepo,
		InvoiceRepo:                st.InvoiceRepo,
		InvoiceLineItemRepo:        st.InvoiceLineItemRepo,
		EntitlementRepo:            st.EntitlementRepo,
		EnvironmentRepo:            st.EnvironmentRepo,
		FeatureRepo:                st.FeatureRepo,
		TenantRepo:                 st.TenantRepo,
		UserRepo:                   st.UserRepo,
		AuthRepo:                   st.AuthRepo,
		WalletRepo:                 st.WalletRepo,
		PaymentRepo:                st.PaymentRepo,
		CreditGrantRepo:            st.CreditGrantRepo,
		CreditGrantApplicationRepo: st.CreditGrantApplicationRepo,
		CouponRepo:                 st.CouponRepo,
		CouponAssociationRepo:      st.CouponAssociationRepo,
		CouponApplicationRepo:      st.CouponApplicationRepo,
		AddonRepo:                  st.AddonRepo,
		AddonAssociationRepo:       st.AddonAssociationRepo,
		ConnectionRepo:             st.ConnectionRepo,
		SettingsRepo:               st.SettingsRepo,
		TaxAssociationRepo:         st.TaxAssociationRepo,
		TaxRateRepo:                st.TaxRateRepo,
		AlertLogsRepo:              st.AlertLogsRepo,
		PlanPriceSyncRepo:          st.PlanPriceSyncRepo,
		EventPublisher:             s.GetPublisher(),
		WebhookPublisher:           s.GetWebhookPublisher(),
		ProrationCalculator:        s.GetCalculator(),
		IntegrationFactory:         s.GetIntegrationFactory(),
	}
}

func (s *SubscriptionChangeV2Suite) setupTestData() {
	ctx := s.GetContext()

	// The change lands at time.Now(), so the current period has to contain it —
	// ten days elapsed, twenty remaining, on a thirty-day period.
	s.td.periodStart = time.Now().UTC().Truncate(time.Hour).AddDate(0, 0, -10)
	s.td.periodEnd = s.td.periodStart.AddDate(0, 0, 30)

	s.td.customer = &customer.Customer{
		ID:         types.GenerateUUIDWithPrefix(types.UUID_PREFIX_CUSTOMER),
		ExternalID: "cust_change_v2",
		Name:       "Change V2 Co",
		BaseModel:  types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().CustomerRepo.Create(ctx, s.td.customer))

	s.td.starter = s.createPlan("Starter", "starter")
	s.td.pro = s.createPlan("Pro", "pro")

	// The base fee exists on both plans at different amounts, so a change slices
	// the line rather than leaving it alone.
	s.td.starterBase = s.createFixedPrice(s.td.starter.ID, "base_fee", 20)
	s.td.proBase = s.createFixedPrice(s.td.pro.ID, "base_fee", 50)

	s.td.sub = s.createSubscription(s.td.starter.ID)
	s.td.baseLine = s.createLineItem(s.td.sub, s.td.starterBase, s.td.starter)
}

func (s *SubscriptionChangeV2Suite) createPlan(name, lookupKey string) *plan.Plan {
	ctx := s.GetContext()
	p := &plan.Plan{
		ID:        types.GenerateUUIDWithPrefix(types.UUID_PREFIX_PLAN),
		Name:      name,
		LookupKey: lookupKey,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().PlanRepo.Create(ctx, p))
	return p
}

func (s *SubscriptionChangeV2Suite) createFixedPrice(planID, lookupKey string, amount int64) *price.Price {
	ctx := s.GetContext()
	p := &price.Price{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_PRICE),
		Amount:             decimal.NewFromInt(amount),
		Currency:           "usd",
		Type:               types.PRICE_TYPE_FIXED,
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           planID,
		LookupKey:          lookupKey,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().PriceRepo.Create(ctx, p))
	return p
}

func (s *SubscriptionChangeV2Suite) createSubscription(planID string) *subscription.Subscription {
	ctx := s.GetContext()
	sub := &subscription.Subscription{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION),
		CustomerID:         s.td.customer.ID,
		PlanID:             planID,
		SubscriptionStatus: types.SubscriptionStatusActive,
		SubscriptionType:   types.SubscriptionTypeStandalone,
		Currency:           "usd",
		BillingAnchor:      s.td.periodStart,
		StartDate:          s.td.periodStart,
		CurrentPeriodStart: s.td.periodStart,
		CurrentPeriodEnd:   s.td.periodEnd,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		BillingCycle:       types.BillingCycleAnniversary,
		Timezone:           "UTC",
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().SubscriptionRepo.Create(ctx, sub))
	return sub
}

func (s *SubscriptionChangeV2Suite) createLineItem(
	sub *subscription.Subscription,
	p *price.Price,
	pl *plan.Plan,
) *subscription.SubscriptionLineItem {
	ctx := s.GetContext()
	item := &subscription.SubscriptionLineItem{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM),
		SubscriptionID:     sub.ID,
		CustomerID:         sub.CustomerID,
		EntityID:           pl.ID,
		EntityType:         types.SubscriptionLineItemEntityTypePlan,
		PlanDisplayName:    pl.Name,
		PriceID:            p.ID,
		PriceType:          p.Type,
		Quantity:           decimal.NewFromInt(1),
		Currency:           sub.Currency,
		BillingPeriod:      p.BillingPeriod,
		BillingPeriodCount: p.BillingPeriodCount,
		InvoiceCadence:     p.InvoiceCadence,
		StartDate:          sub.CurrentPeriodStart,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().SubscriptionLineItemRepo.Create(ctx, item))
	return item
}

// recordBilled writes the invoice line a real invoice run would, so a removal
// credit has a basis to be capped against. Without it a line item reads as never
// billed, and nothing can be credited back.
func (s *SubscriptionChangeV2Suite) recordBilled(lineItemID string, amount decimal.Decimal) {
	ctx := s.GetContext()
	periodStart := s.td.periodStart
	periodEnd := s.td.periodEnd

	s.NoError(s.GetStores().InvoiceLineItemRepo.Create(ctx, &invoice.InvoiceLineItem{
		ID:                     types.GenerateUUIDWithPrefix(types.UUID_PREFIX_INVOICE_LINE_ITEM),
		InvoiceID:              types.GenerateUUIDWithPrefix(types.UUID_PREFIX_INVOICE),
		CustomerID:             s.td.customer.ID,
		SubscriptionID:         &s.td.sub.ID,
		SubscriptionLineItemID: &lineItemID,
		Amount:                 amount,
		Quantity:               decimal.NewFromInt(1),
		Currency:               "usd",
		PeriodStart:            &periodStart,
		PeriodEnd:              &periodEnd,
		BaseModel:              types.GetDefaultBaseModel(ctx),
	}))
}

func (s *SubscriptionChangeV2Suite) changeRequest(targetPlanID string, behavior types.ProrationBehavior) dto.SubscriptionChangeV2Request {
	return dto.SubscriptionChangeV2Request{
		TargetPlanID:      targetPlanID,
		ProrationBehavior: behavior,
	}
}

func (s *SubscriptionChangeV2Suite) liveLineItems() []*subscription.SubscriptionLineItem {
	ctx := s.GetContext()
	sub, err := s.GetStores().SubscriptionRepo.Get(ctx, s.td.sub.ID)
	s.Require().NoError(err)
	items, err := s.GetStores().SubscriptionLineItemRepo.ListBySubscription(ctx, sub)
	s.Require().NoError(err)

	live := make([]*subscription.SubscriptionLineItem, 0, len(items))
	for _, item := range items {
		if item.EndDate.IsZero() {
			live = append(live, item)
		}
	}
	return live
}

// ─── Swap in place ───────────────────────────────────────────────────────────

// TestExecute_UpgradeSwapsInPlace is the whole point of v2: the subscription row
// survives with its id, anchor and period bounds intact, and only plan_id and
// the plan line items move. Cancel-and-recreate could offer none of that.
func (s *SubscriptionChangeV2Suite) TestExecute_UpgradeSwapsInPlace() {
	ctx := s.GetContext()
	effectiveFrom := time.Now().UTC()

	resp, err := s.svc.ExecutePlanChange(ctx, s.td.sub.ID, s.changeRequest(s.td.pro.ID, types.ProrationBehaviorCreateProrations))
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	s.Equal(types.SubscriptionChangeTypeUpgrade, resp.ChangeType, "$20 → $50 is an upgrade")
	s.Equal(s.td.starter.ID, resp.FromPlan.ID)
	s.Equal(s.td.pro.ID, resp.ToPlan.ID)

	reloaded, err := s.GetStores().SubscriptionRepo.Get(ctx, s.td.sub.ID)
	s.Require().NoError(err)
	s.Equal(s.td.sub.ID, reloaded.ID, "the subscription row survives a plan change")
	s.Equal(s.td.pro.ID, reloaded.PlanID)
	s.True(reloaded.BillingAnchor.Equal(s.td.sub.BillingAnchor), "the anchor is the metronome; it must not move")
	s.True(reloaded.CurrentPeriodStart.Equal(s.td.sub.CurrentPeriodStart))
	s.True(reloaded.CurrentPeriodEnd.Equal(s.td.sub.CurrentPeriodEnd))

	// The old line closes exactly where the new one opens: no gap, no overlap.
	live := s.liveLineItems()
	s.Require().Len(live, 1, "exactly one live base-fee line after the swap")
	s.Equal(s.td.proBase.ID, live[0].PriceID)

	closed, err := s.GetStores().SubscriptionLineItemRepo.Get(ctx, s.td.baseLine.ID)
	s.Require().NoError(err)
	s.False(closed.EndDate.IsZero(), "the old line is closed, not deleted")
	s.True(!closed.EndDate.Before(effectiveFrom), "closed at the effective date")
	s.True(live[0].StartDate.Equal(closed.EndDate), "line items must tile with no gap or overlap")
}

// ─── The unchanged case ──────────────────────────────────────────────────────

// TestExecute_LateralIdenticalPrice_LeavesLineAlone is the case a naive
// close-and-reopen gets wrong. The service did not change, so nothing should
// move: no new line-item id, no proration entry, and no invoice made of a charge
// and a credit that cancel.
func (s *SubscriptionChangeV2Suite) TestExecute_LateralIdenticalPrice_LeavesLineAlone() {
	ctx := s.GetContext()

	// A second plan whose base fee is priced identically to Starter's.
	lateral := s.createPlan("Lateral", "lateral")
	s.createFixedPrice(lateral.ID, "base_fee", 20)

	invoicesBefore := s.countInvoices()

	resp, err := s.svc.ExecutePlanChange(ctx, s.td.sub.ID, s.changeRequest(lateral.ID, types.ProrationBehaviorCreateProrations))
	s.Require().NoError(err)
	s.Equal(types.SubscriptionChangeTypeLateral, resp.ChangeType)

	reloaded, err := s.GetStores().SubscriptionRepo.Get(ctx, s.td.sub.ID)
	s.Require().NoError(err)
	s.Equal(lateral.ID, reloaded.PlanID, "the plan still swaps")

	live := s.liveLineItems()
	s.Require().Len(live, 1)
	s.Equal(s.td.baseLine.ID, live[0].ID, "the line item keeps its id — nothing about this service changed")
	s.Equal(s.td.starterBase.ID, live[0].PriceID, "and keeps pointing at the identical price")

	s.Empty(resp.ChangedResources.LineItems, "an untouched line is not a change to report")
	s.Equal(invoicesBefore, s.countInvoices(), "net zero on an unchanged service must raise no invoice")
}

// ─── Proration behaviour ─────────────────────────────────────────────────────

// TestExecute_ProrationNone_SwapsWithoutMoney verifies that a caller who asks
// for no proration gets the service change and no money movement — the next
// regular invoice simply bills the new price.
func (s *SubscriptionChangeV2Suite) TestExecute_ProrationNone_SwapsWithoutMoney() {
	ctx := s.GetContext()
	invoicesBefore := s.countInvoices()

	_, err := s.svc.ExecutePlanChange(ctx, s.td.sub.ID, s.changeRequest(s.td.pro.ID, types.ProrationBehaviorNone))
	s.Require().NoError(err)

	reloaded, err := s.GetStores().SubscriptionRepo.Get(ctx, s.td.sub.ID)
	s.Require().NoError(err)
	s.Equal(s.td.pro.ID, reloaded.PlanID, "the service still swaps")

	live := s.liveLineItems()
	s.Require().Len(live, 1)
	s.Equal(s.td.proBase.ID, live[0].PriceID, "billed at the new price from here on")

	s.Equal(invoicesBefore, s.countInvoices(), "proration_behavior=none must not settle anything")
}

// TestExecute_DowngradeCreditsWallet covers a change that gives back more than
// it takes. A non-credit invoice cannot carry a negative total, so the net
// credit goes to the customer's wallet rather than being discarded.
func (s *SubscriptionChangeV2Suite) TestExecute_DowngradeCreditsWallet() {
	ctx := s.GetContext()

	// Start on Pro so the change is a genuine downgrade.
	s.NoError(s.GetStores().SubscriptionLineItemRepo.Delete(ctx, s.td.baseLine.ID))
	sub, err := s.GetStores().SubscriptionRepo.Get(ctx, s.td.sub.ID)
	s.Require().NoError(err)
	sub.PlanID = s.td.pro.ID
	s.Require().NoError(s.GetStores().SubscriptionRepo.Update(ctx, sub))
	s.td.baseLine = s.createLineItem(sub, s.td.proBase, s.td.pro)

	// The credit is capped at what this line was actually billed, so the period's
	// charge has to exist for a downgrade to return anything at all.
	s.recordBilled(s.td.baseLine.ID, s.td.proBase.Amount)

	resp, err := s.svc.ExecutePlanChange(ctx, s.td.sub.ID, s.changeRequest(s.td.starter.ID, types.ProrationBehaviorCreateProrations))
	s.Require().NoError(err)
	s.Equal(types.SubscriptionChangeTypeDowngrade, resp.ChangeType)

	wallets, err := s.GetStores().WalletRepo.GetWalletsByFilter(ctx, &types.WalletFilter{
		QueryFilter: types.NewDefaultQueryFilter(),
	})
	s.Require().NoError(err)
	s.Require().NotEmpty(wallets, "the residual credit must reach the customer, not be discarded")
	s.True(wallets[0].Balance.GreaterThan(decimal.Zero))
}

// ─── Preview ─────────────────────────────────────────────────────────────────

// TestPreviewWritesNothing is the property that makes preview safe to call: it
// runs resolve and compute, which are the same functions execute runs, and then
// stops before the only stage that writes.
func (s *SubscriptionChangeV2Suite) TestPreviewWritesNothing() {
	ctx := s.GetContext()
	invoicesBefore := s.countInvoices()

	resp, err := s.svc.PreviewPlanChange(ctx, s.td.sub.ID, s.changeRequest(s.td.pro.ID, types.ProrationBehaviorCreateProrations))
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	reloaded, err := s.GetStores().SubscriptionRepo.Get(ctx, s.td.sub.ID)
	s.Require().NoError(err)
	s.Equal(s.td.starter.ID, reloaded.PlanID, "preview must not swap the plan")

	live := s.liveLineItems()
	s.Require().Len(live, 1)
	s.Equal(s.td.baseLine.ID, live[0].ID, "preview must not touch line items")
	s.Equal(invoicesBefore, s.countInvoices(), "preview must not raise an invoice")
}

// TestPreviewMatchesExecute is quote parity. Preview and execute share resolve
// and compute by calling the same functions, so the money cannot drift between
// what a customer was quoted and what they are charged.
func (s *SubscriptionChangeV2Suite) TestPreviewMatchesExecute() {
	ctx := s.GetContext()
	req := s.changeRequest(s.td.pro.ID, types.ProrationBehaviorCreateProrations)

	preview, err := s.svc.PreviewPlanChange(ctx, s.td.sub.ID, req)
	s.Require().NoError(err)
	s.Require().Len(preview.ChangedResources.Invoices, 1)
	quoted := preview.ChangedResources.Invoices[0].Invoice.AmountDue

	executed, err := s.svc.ExecutePlanChange(ctx, s.td.sub.ID, req)
	s.Require().NoError(err)
	s.Require().Len(executed.ChangedResources.Invoices, 1)
	charged := executed.ChangedResources.Invoices[0].Invoice.AmountDue

	s.True(quoted.Equal(charged), "quoted %s but charged %s", quoted, charged)
	s.Equal(preview.ChangeType, executed.ChangeType)
	s.Len(preview.ChangedResources.LineItems, len(executed.ChangedResources.LineItems))
}

// ─── Preconditions ───────────────────────────────────────────────────────────

// TestPreconditionsRejectBeforeAnyWrite covers the shapes the swap engine does
// not handle. Each must fail cleanly with nothing mutated, so a caller can fall
// back to the v1 endpoint.
func (s *SubscriptionChangeV2Suite) TestPreconditionsRejectBeforeAnyWrite() {
	ctx := s.GetContext()

	tests := []struct {
		name    string
		mutate  func(*subscription.Subscription)
		target  string
		because string
	}{
		{
			name:    "already on the target plan",
			target:  s.td.starter.ID,
			because: "a no-op change is a caller mistake, not a silent success",
		},
		{
			name:    "cancelled subscription",
			mutate:  func(sub *subscription.Subscription) { sub.SubscriptionStatus = types.SubscriptionStatusCancelled },
			because: "only active or trialing subscriptions can change plan",
		},
		{
			name:    "paused subscription",
			mutate:  func(sub *subscription.Subscription) { sub.PauseStatus = types.PauseStatusActive },
			because: "a paused subscription has no live billing to prorate",
		},
		{
			name:    "hierarchy subscription",
			mutate:  func(sub *subscription.Subscription) { sub.SubscriptionType = types.SubscriptionTypeParent },
			because: "child subscriptions would need to move too",
		},
		{
			name:    "pending cancellation",
			mutate:  func(sub *subscription.Subscription) { sub.CancelAtPeriodEnd = true },
			because: "the change would race the cancellation",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			sub, err := s.GetStores().SubscriptionRepo.Get(ctx, s.td.sub.ID)
			s.Require().NoError(err)
			if tt.mutate != nil {
				tt.mutate(sub)
				s.Require().NoError(s.GetStores().SubscriptionRepo.Update(ctx, sub))
			}

			target := tt.target
			if target == "" {
				target = s.td.pro.ID
			}

			_, err = s.svc.ExecutePlanChange(ctx, s.td.sub.ID, s.changeRequest(target, types.ProrationBehaviorCreateProrations))
			s.Error(err, tt.because)

			after, getErr := s.GetStores().SubscriptionRepo.Get(ctx, s.td.sub.ID)
			s.Require().NoError(getErr)
			s.Equal(s.td.starter.ID, after.PlanID, "a rejected change must mutate nothing")

			// Restore for the next case.
			after.SubscriptionStatus = types.SubscriptionStatusActive
			after.PauseStatus = types.PauseStatusNone
			after.SubscriptionType = types.SubscriptionTypeStandalone
			after.CancelAtPeriodEnd = false
			s.Require().NoError(s.GetStores().SubscriptionRepo.Update(ctx, after))
		})
	}
}

// TestExecute_ReanchorsPriceSyncSequence ties the swap to phase 1's watermark:
// after moving plans the subscription must track its new plan's price changes.
func (s *SubscriptionChangeV2Suite) TestExecute_ReanchorsPriceSyncSequence() {
	ctx := s.GetContext()

	_, err := s.svc.ExecutePlanChange(ctx, s.td.sub.ID, s.changeRequest(s.td.pro.ID, types.ProrationBehaviorCreateProrations))
	s.Require().NoError(err)

	expected, err := s.GetStores().PlanPriceSyncRepo.CurrentPlanSequence(ctx, s.td.pro.ID)
	s.Require().NoError(err)

	reloaded, err := s.GetStores().SubscriptionRepo.Get(ctx, s.td.sub.ID)
	s.Require().NoError(err)
	s.Equal(expected, reloaded.SyncedPriceSequence,
		"the watermark must be re-anchored to the new plan, or sync will never select this sub again")
}

func (s *SubscriptionChangeV2Suite) countInvoices() int {
	ctx := s.GetContext()
	invoices, err := s.GetStores().InvoiceRepo.List(ctx, &types.InvoiceFilter{
		QueryFilter: types.NewNoLimitQueryFilter(),
	})
	s.Require().NoError(err)
	return len(invoices)
}

// TestExecute_NettedInvoiceCarriesCreditLine covers an upgrade where the old
// line was genuinely billed, so the change produces BOTH a credit for unused
// time and a charge for the new price. They net onto one invoice, and the
// credit must be visible as its own line rather than paid out separately.
func (s *SubscriptionChangeV2Suite) TestExecute_NettedInvoiceCarriesCreditLine() {
	ctx := s.GetContext()

	s.recordBilled(s.td.baseLine.ID, s.td.starterBase.Amount)

	resp, err := s.svc.ExecutePlanChange(ctx, s.td.sub.ID,
		s.changeRequest(s.td.pro.ID, types.ProrationBehaviorCreateProrations))
	s.Require().NoError(err)
	s.Require().Len(resp.ChangedResources.Invoices, 1)

	inv := resp.ChangedResources.Invoices[0].Invoice
	s.Require().NotNil(inv)

	var sawCredit bool
	for _, li := range inv.LineItems {
		if li.Amount.IsNegative() {
			sawCredit = true
		}
	}
	s.True(sawCredit, "the unused-time credit must appear as a negative line on the netted invoice")
}
