package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/entitlement"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/domain/feature"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// RevenueRollupSuite drives RollupSubscription against the REAL billing preview
// engine (PrepareSubscriptionInvoiceRequest) — not a hand-rolled decomposition —
// over a worked example in the ERD §6.5 shape: a 30-day monthly subscription
// with a $30 advance fixed charge, $0.01/call usage (1200 calls/day, a
// same-meter Feature+Entitlement present but non-binding — see
// seedWorkedExample), and a $500 minimum commitment with a 2x overage factor
// and true-up enabled. Σ net_amount must reconcile to $530, and a second
// RollupSubscription call must be idempotent.
type RevenueRollupSuite struct {
	testutil.BaseServiceTestSuite
	ctx   context.Context
	svc   RevenueRollupService
	store *testutil.InMemoryRevenueFactStore

	sub         *subscription.Subscription
	periodStart time.Time
	// periodEnd is the query bound passed to ListBySubscriptionPeriod. It must
	// reach through the current period's exclusive end (see seedWorkedExample)
	// to also catch the next period's advance-billed fixed row.
	periodEnd time.Time
}

func TestRevenueRollup(t *testing.T) {
	suite.Run(t, new(RevenueRollupSuite))
}

func (s *RevenueRollupSuite) SetupTest() {
	s.BaseServiceTestSuite.SetupTest()
	s.ctx = s.GetContext()
	s.store = s.GetStores().RevenueFactRepo.(*testutil.InMemoryRevenueFactStore)
	s.svc = NewRevenueRollupService(s.serviceParams())
}

func (s *RevenueRollupSuite) TearDownTest() {
	s.BaseServiceTestSuite.TearDownTest()
}

func (s *RevenueRollupSuite) serviceParams() ServiceParams {
	stores := s.GetStores()
	return ServiceParams{
		Logger:                       s.GetLogger(),
		Config:                       s.GetConfig(),
		DB:                           s.GetDB(),
		SubRepo:                      stores.SubscriptionRepo,
		SubscriptionLineItemRepo:     stores.SubscriptionLineItemRepo,
		SubscriptionPhaseRepo:        stores.SubscriptionPhaseRepo,
		SubScheduleRepo:              stores.SubscriptionScheduleRepo,
		PlanRepo:                     stores.PlanRepo,
		PriceRepo:                    stores.PriceRepo,
		PriceUnitRepo:                stores.PriceUnitRepo,
		EventRepo:                    stores.EventRepo,
		MeterRepo:                    stores.MeterRepo,
		MeterUsageRepo:               stores.MeterUsageRepo,
		CustomerRepo:                 stores.CustomerRepo,
		InvoiceRepo:                  stores.InvoiceRepo,
		InvoiceLineItemRepo:          stores.InvoiceLineItemRepo,
		EntitlementRepo:              stores.EntitlementRepo,
		EntitlementGrantRepo:         stores.EntitlementGrantRepo,
		EnvironmentRepo:              stores.EnvironmentRepo,
		FeatureRepo:                  stores.FeatureRepo,
		AddonAssociationRepo:         stores.AddonAssociationRepo,
		TenantRepo:                   stores.TenantRepo,
		UserRepo:                     stores.UserRepo,
		AuthRepo:                     stores.AuthRepo,
		WalletRepo:                   stores.WalletRepo,
		PaymentRepo:                  stores.PaymentRepo,
		CreditNoteRepo:               stores.CreditNoteRepo,
		CreditNoteLineItemRepo:       stores.CreditNoteLineItemRepo,
		CouponRepo:                   stores.CouponRepo,
		CouponAssociationRepo:        stores.CouponAssociationRepo,
		CouponApplicationRepo:        stores.CouponApplicationRepo,
		TaxRateRepo:                  stores.TaxRateRepo,
		TaxAppliedRepo:               stores.TaxAppliedRepo,
		TaxAssociationRepo:           stores.TaxAssociationRepo,
		CreditGrantRepo:              stores.CreditGrantRepo,
		CreditGrantApplicationRepo:   stores.CreditGrantApplicationRepo,
		ConnectionRepo:               stores.ConnectionRepo,
		EntityIntegrationMappingRepo: stores.EntityIntegrationMappingRepo,
		SettingsRepo:                 stores.SettingsRepo,
		AlertLogsRepo:                stores.AlertLogsRepo,
		TaskRepo:                     stores.TaskRepo,
		SecretRepo:                   stores.SecretRepo,
		EventPublisher:               s.GetPublisher(),
		WebhookPublisher:             s.GetWebhookPublisher(),
		ProrationCalculator:          s.GetCalculator(),
		IntegrationFactory:           s.GetIntegrationFactory(),
		RevenueFactRepo:              stores.RevenueFactRepo,
	}
}

// seedWorkedExample builds a worked-example fixture in the ERD §6.5 shape —
// plan + fixed/usage prices + meter + feature + entitlement + subscription
// (with a $500/2x/true-up subscription-level commitment) — and seeds
// meter_usage at 1200 calls/day for 30 days, the real inputs the billing
// preview engine reads.
//
// The entitlement's UsageLimit is 0 rather than the design doc's illustrative
// 20 000: the real engine's subscription-level commitment/true-up is computed
// from the GROSS pre-entitlement usage amount (buildMeterUsageResponse splits
// commitment/overage before CalculateMeterUsageCharges applies any per-item
// entitlement deduction), so usage+trueup nets to exactly commitmentAmount
// minus whatever the entitlement deducted — not the "billable-after-entitlementLimit
// tops up to the commitment" shape the hand-rolled decomposition tests use.
// A non-zero entitlementLimit here would pull the $530 total off by exactly the
// deducted amount (verified empirically). UsageLimit=0 still exercises the
// real entitlement-resolution path (FeatureRepo/EntitlementRepo/
// GetAggregatedSubscriptionEntitlements) that resolveEntitlementLimit depends on,
// without perturbing the commitment math. See the task report's "concerns"
// section for the full trace.
func (s *RevenueRollupSuite) seedWorkedExample(ctx context.Context) {
	s.periodStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	periodEndExclusive := s.periodStart.AddDate(0, 0, 30)
	// ReferencePointPreview returns this period's ARREAR usage/true-up (dated
	// within [periodStart, periodEndExclusive)) together with NEXT period's
	// ADVANCE fixed charge (dated at periodEndExclusive, the next period's own
	// start) — one RollupSubscription call spans both, so the query window
	// below must reach through periodEndExclusive to catch the fixed row too.
	s.periodEnd = periodEndExclusive

	cust := &customer.Customer{
		ID:         "cust_rollup_wk",
		ExternalID: "ext_rollup_wk",
		Name:       "Rollup Worked Example",
		Email:      "rollup@example.com",
		BaseModel:  types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().CustomerRepo.Create(ctx, cust))

	pl := &plan.Plan{
		ID:        "plan_rollup_wk",
		Name:      "Rollup Worked Example Plan",
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().PlanRepo.Create(ctx, pl))

	mtr := &meter.Meter{
		ID:        "meter_rollup_wk",
		Name:      "API Calls Rollup",
		EventName: "api_call_rollup_wk",
		Aggregation: meter.Aggregation{
			Type: types.AggregationSum,
		},
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().MeterRepo.CreateMeter(ctx, mtr))

	feat := &feature.Feature{
		ID:        "feat_rollup_wk",
		Name:      "API Calls Feature Rollup",
		Type:      types.FeatureTypeMetered,
		MeterID:   mtr.ID,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().FeatureRepo.Create(ctx, feat))

	ent := &entitlement.Entitlement{
		ID:               "ent_rollup_wk",
		EntityType:       types.ENTITLEMENT_ENTITY_TYPE_PLAN,
		EntityID:         pl.ID,
		FeatureID:        feat.ID,
		FeatureType:      types.FeatureTypeMetered,
		IsEnabled:        true,
		UsageLimit:       lo.ToPtr(int64(0)),
		UsageResetPeriod: types.ENTITLEMENT_USAGE_RESET_PERIOD_MONTHLY,
		IsSoftLimit:      false,
		BaseModel:        types.GetDefaultBaseModel(ctx),
	}
	_, err := s.GetStores().EntitlementRepo.Create(ctx, ent)
	s.NoError(err)

	fixedPrice := &price.Price{
		ID:                 "price_rollup_fixed",
		Amount:             decimal.NewFromInt(30),
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           pl.ID,
		Type:               types.PRICE_TYPE_FIXED,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().PriceRepo.Create(ctx, fixedPrice))

	usagePrice := &price.Price{
		ID:                 "price_rollup_usage",
		Amount:             decimal.RequireFromString("0.01"),
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           pl.ID,
		Type:               types.PRICE_TYPE_USAGE,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		InvoiceCadence:     types.InvoiceCadenceArrear,
		MeterID:            mtr.ID,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().PriceRepo.Create(ctx, usagePrice))

	commitmentAmount := decimal.NewFromInt(500)
	overageFactor := decimal.NewFromInt(2)
	s.sub = &subscription.Subscription{
		ID:                 "sub_rollup_wk",
		PlanID:             pl.ID,
		CustomerID:         cust.ID,
		StartDate:          s.periodStart,
		BillingAnchor:      periodEndExclusive,
		CurrentPeriodStart: s.periodStart,
		CurrentPeriodEnd:   periodEndExclusive,
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		SubscriptionStatus: types.SubscriptionStatusActive,
		CommitmentAmount:   &commitmentAmount,
		OverageFactor:      &overageFactor,
		EnableTrueUp:       true,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}

	lineItems := []*subscription.SubscriptionLineItem{
		{
			ID:              types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM),
			SubscriptionID:  s.sub.ID,
			CustomerID:      s.sub.CustomerID,
			EntityID:        pl.ID,
			EntityType:      types.SubscriptionLineItemEntityTypePlan,
			PlanDisplayName: pl.Name,
			PriceID:         fixedPrice.ID,
			PriceType:       fixedPrice.Type,
			DisplayName:     "Fixed",
			Quantity:        decimal.NewFromInt(1),
			Currency:        s.sub.Currency,
			BillingPeriod:   s.sub.BillingPeriod,
			InvoiceCadence:  types.InvoiceCadenceAdvance,
			StartDate:       s.sub.StartDate,
			BaseModel:       types.GetDefaultBaseModel(ctx),
		},
		{
			ID:               types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM),
			SubscriptionID:   s.sub.ID,
			CustomerID:       s.sub.CustomerID,
			EntityID:         pl.ID,
			EntityType:       types.SubscriptionLineItemEntityTypePlan,
			PlanDisplayName:  pl.Name,
			PriceID:          usagePrice.ID,
			PriceType:        usagePrice.Type,
			MeterID:          mtr.ID,
			MeterDisplayName: mtr.Name,
			DisplayName:      "API Calls",
			Quantity:         decimal.Zero,
			Currency:         s.sub.Currency,
			BillingPeriod:    s.sub.BillingPeriod,
			InvoiceCadence:   types.InvoiceCadenceArrear,
			StartDate:        s.sub.StartDate,
			BaseModel:        types.GetDefaultBaseModel(ctx),
		},
	}
	s.NoError(s.GetStores().SubscriptionRepo.CreateWithLineItems(ctx, s.sub, lineItems))
	s.sub.LineItems = lineItems

	// Seed meter_usage: 1200 calls/day for 30 days, one bulk row per day.
	records := make([]*events.MeterUsage, 0, 30)
	for d := 0; d < 30; d++ {
		ts := s.periodStart.AddDate(0, 0, d).Add(12 * time.Hour)
		id := s.GetUUID()
		records = append(records, &events.MeterUsage{
			Event: events.Event{
				ID:                 id,
				TenantID:           types.GetTenantID(ctx),
				EnvironmentID:      types.GetEnvironmentID(ctx),
				EventName:          mtr.EventName,
				ExternalCustomerID: cust.ExternalID,
				CustomerID:         cust.ID,
				Timestamp:          ts,
				IngestedAt:         ts,
			},
			MeterID:    mtr.ID,
			QtyTotal:   decimal.NewFromInt(1200),
			UniqueHash: fmt.Sprintf("%s:%s", mtr.EventName, id),
		})
	}
	s.NoError(s.GetStores().MeterUsageRepo.BulkInsertMeterUsage(ctx, records))
}

func (s *RevenueRollupSuite) TestRollupSubscription_WorkedExampleReconciles() {
	ctx := s.ctx
	s.seedWorkedExample(ctx)

	s.NoError(s.svc.RollupSubscription(ctx, s.sub.ID))

	rows, err := s.store.ListBySubscriptionPeriod(ctx, s.sub.ID, s.periodStart, s.periodEnd, types.FactProvisional)
	s.NoError(err)
	s.NotEmpty(rows, "worked example must produce rows")

	total := decimal.Zero
	for _, r := range rows {
		total = total.Add(r.NetAmount)
	}
	s.Equal("530", total.String(), "Σ net_amount must reconcile to $530")

	// Idempotent: a second rollup of the same period bumps versions in place,
	// never duplicating rows.
	s.NoError(s.svc.RollupSubscription(ctx, s.sub.ID))
	rows2, err := s.store.ListBySubscriptionPeriod(ctx, s.sub.ID, s.periodStart, s.periodEnd, types.FactProvisional)
	s.NoError(err)
	s.Equal(len(rows), len(rows2), "idempotent recompute must not duplicate rows")

	versionByID := make(map[string]int64, len(rows))
	for _, r := range rows {
		versionByID[r.ID] = r.Version
	}
	for _, r := range rows2 {
		s.Equal(versionByID[r.ID]+1, r.Version, "recompute must bump version in place, not insert a duplicate")
	}
}

func (s *RevenueRollupSuite) TestRollupSubscription_MultiPeriodCommitmentSkipped() {
	ctx := s.ctx
	s.seedWorkedExample(ctx)

	// Turn the subscription's commitment into a multi-period one — an ANNUAL
	// commitment on a MONTHLY subscription — which the rollup must skip
	// entirely rather than attribute the true-up to a single period.
	annual := types.BILLING_PERIOD_ANNUAL
	s.sub.CommitmentDuration = &annual
	s.NoError(s.GetStores().SubscriptionRepo.Update(ctx, s.sub))

	s.NoError(s.svc.RollupSubscription(ctx, s.sub.ID))

	rows, err := s.store.ListBySubscriptionPeriod(ctx, s.sub.ID, s.periodStart, s.periodEnd, types.FactProvisional)
	s.NoError(err)
	s.Empty(rows, "multi-period commitment subscriptions must be skipped with zero rows")
}

// TestRollupSubscription_OverageSkipped covers review Fix 1: a single-period
// commitment whose usage EXCEEDS the commitment makes the engine emit a
// REDUCED within-commitment usage line plus a separate is_overage line. The
// rollup has no commitment knowledge when decomposing a usage line via the
// full curve, so it must skip the whole subscription rather than write rows
// that overstate usage.
func (s *RevenueRollupSuite) TestRollupSubscription_OverageSkipped() {
	ctx := s.ctx
	s.seedWorkedExample(ctx)

	// seedWorkedExample already seeds 1200 calls/day (36000 calls = $360,
	// under the $500 commitment). Push another 1000 calls/day so gross usage
	// (66000 calls = $660) exceeds the commitment and the engine's per-period
	// commitment split emits an is_overage line.
	extra := make([]*events.MeterUsage, 0, 30)
	for d := 0; d < 30; d++ {
		ts := s.periodStart.AddDate(0, 0, d).Add(18 * time.Hour)
		id := s.GetUUID()
		extra = append(extra, &events.MeterUsage{
			Event: events.Event{
				ID:                 id,
				TenantID:           types.GetTenantID(ctx),
				EnvironmentID:      types.GetEnvironmentID(ctx),
				EventName:          "api_call_rollup_wk",
				ExternalCustomerID: "ext_rollup_wk",
				CustomerID:         "cust_rollup_wk",
				Timestamp:          ts,
				IngestedAt:         ts,
			},
			MeterID:    "meter_rollup_wk",
			QtyTotal:   decimal.NewFromInt(1000),
			UniqueHash: fmt.Sprintf("api_call_rollup_wk_extra:%s", id),
		})
	}
	s.NoError(s.GetStores().MeterUsageRepo.BulkInsertMeterUsage(ctx, extra))

	s.NoError(s.svc.RollupSubscription(ctx, s.sub.ID))

	rows, err := s.store.ListBySubscriptionPeriod(ctx, s.sub.ID, s.periodStart, s.periodEnd, types.FactProvisional)
	s.NoError(err)
	s.Empty(rows, "a subscription with an is_overage line must be skipped with zero rows")
}

// TestRollupSubscription_BindingEntitlementLimitReconciles covers review Fix 2: with
// a REAL binding entitlement entitlementLimit (UsageLimit=20000, unlike
// seedWorkedExample's degenerate UsageLimit=0) alongside the $500 commitment
// from ERD §6.5, the total no longer equals $530 — the subscription-level
// commitment/true-up split runs on gross pre-entitlement usage, so the
// entitlementLimit genuinely perturbs the total (see the task report's Concern 4).
// This asserts RECONCILIATION-BY-CONSTRUCTION instead of a hardcoded number:
// Σ NetAmount(rows) must equal the same preview's Subtotal minus discounts,
// and zero revenue_reconciliation_mismatch logs must have fired.
func (s *RevenueRollupSuite) TestRollupSubscription_BindingEntitlementLimitReconciles() {
	ctx := s.ctx
	s.seedWorkedExample(ctx)

	ent, err := s.GetStores().EntitlementRepo.Get(ctx, "ent_rollup_wk")
	s.NoError(err)
	ent.UsageLimit = lo.ToPtr(int64(20000))
	_, err = s.GetStores().EntitlementRepo.Update(ctx, ent)
	s.NoError(err)

	// Capture logs so we can assert zero revenue_reconciliation_mismatch
	// entries fired, instead of only inferring it from the row totals.
	core, observedLogs := observer.New(zapcore.InfoLevel)
	capturingLogger := logger.NewFromSugared(zap.New(core).Sugar())
	params := s.serviceParams()
	params.Logger = capturingLogger
	svc := NewRevenueRollupService(params)

	s.NoError(svc.RollupSubscription(ctx, s.sub.ID))

	rows, err := s.store.ListBySubscriptionPeriod(ctx, s.sub.ID, s.periodStart, s.periodEnd, types.FactProvisional)
	s.NoError(err)
	s.NotEmpty(rows, "binding-entitlementLimit worked example must produce rows")

	total := decimal.Zero
	for _, r := range rows {
		total = total.Add(r.NetAmount)
	}

	// Recompute the same preview independently (RollupSubscription doesn't
	// expose the invReq it used) to get the engine's own Subtotal for this
	// exact fixture state — the reconciliation target, not a hardcoded number.
	billingSvc := NewBillingService(s.serviceParams())
	invReq, err := billingSvc.PrepareSubscriptionInvoiceRequest(ctx, &dto.PrepareSubscriptionInvoiceRequestParams{
		Subscription:   s.sub,
		PeriodStart:    s.periodStart,
		PeriodEnd:      s.periodEnd,
		ReferencePoint: types.ReferencePointPreview,
	})
	s.NoError(err)
	expected := invReq.Subtotal.Sub(invoiceDiscountTotal(invReq))
	s.False(expected.Equal(decimal.NewFromInt(530)), "sanity: the binding entitlementLimit must actually perturb the total away from the zero-entitlementLimit $530 case")

	s.True(total.Equal(expected), "Σ net_amount (%s) must reconcile to the preview's subtotal-minus-discount (%s)", total.String(), expected.String())

	for _, entry := range observedLogs.All() {
		s.NotEqual("revenue_reconciliation_mismatch", entry.Message, "unexpected reconciliation mismatch: %+v", entry.ContextMap())
	}
}

// seedFixedOnlySubscription builds a minimal customer/plan/subscription with
// a single FIXED line item — deliberately simpler than seedWorkedExample
// (no usage/meter/entitlement machinery) since TestRollupDirty_* exercises
// RollupDirty's scan/tally/error-isolation logic, not decomposition itself.
// When commitmentDuration is non-nil the subscription also carries a $500/2x
// commitment with that duration, to drive the multi-period-commitment skip.
// When createPrice is false, priceID is left dangling (never created in
// PriceRepo) so PrepareSubscriptionInvoiceRequest fails hydrating it — a
// genuine per-subscription error, not a policy skip.
func (s *RevenueRollupSuite) seedFixedOnlySubscription(
	ctx context.Context, idSuffix string, commitmentDuration *types.BillingPeriod, priceID string, createPrice bool,
) *subscription.Subscription {
	cust := &customer.Customer{
		ID:         "cust_dirty_" + idSuffix,
		ExternalID: "ext_dirty_" + idSuffix,
		Name:       "Dirty " + idSuffix,
		Email:      "dirty_" + idSuffix + "@example.com",
		BaseModel:  types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().CustomerRepo.Create(ctx, cust))

	pl := &plan.Plan{ID: "plan_dirty_" + idSuffix, Name: "Dirty Plan " + idSuffix, BaseModel: types.GetDefaultBaseModel(ctx)}
	s.NoError(s.GetStores().PlanRepo.Create(ctx, pl))

	if createPrice {
		fixedPrice := &price.Price{
			ID:                 priceID,
			Amount:             decimal.NewFromInt(30),
			Currency:           "usd",
			EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
			EntityID:           pl.ID,
			Type:               types.PRICE_TYPE_FIXED,
			BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
			BillingPeriodCount: 1,
			BillingModel:       types.BILLING_MODEL_FLAT_FEE,
			BillingCadence:     types.BILLING_CADENCE_RECURRING,
			InvoiceCadence:     types.InvoiceCadenceAdvance,
			BaseModel:          types.GetDefaultBaseModel(ctx),
		}
		s.NoError(s.GetStores().PriceRepo.Create(ctx, fixedPrice))
	}

	periodStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 0, 30)

	sub := &subscription.Subscription{
		ID:                 "sub_dirty_" + idSuffix,
		PlanID:             pl.ID,
		CustomerID:         cust.ID,
		StartDate:          periodStart,
		BillingAnchor:      periodEnd,
		CurrentPeriodStart: periodStart,
		CurrentPeriodEnd:   periodEnd,
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		SubscriptionStatus: types.SubscriptionStatusActive,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	if commitmentDuration != nil {
		commitmentAmount := decimal.NewFromInt(500)
		overageFactor := decimal.NewFromInt(2)
		sub.CommitmentAmount = &commitmentAmount
		sub.OverageFactor = &overageFactor
		sub.CommitmentDuration = commitmentDuration
		sub.EnableTrueUp = true
	}

	lineItems := []*subscription.SubscriptionLineItem{
		{
			ID:              types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM),
			SubscriptionID:  sub.ID,
			CustomerID:      sub.CustomerID,
			EntityID:        pl.ID,
			EntityType:      types.SubscriptionLineItemEntityTypePlan,
			PlanDisplayName: pl.Name,
			PriceID:         priceID,
			PriceType:       types.PRICE_TYPE_FIXED,
			DisplayName:     "Fixed",
			Quantity:        decimal.NewFromInt(1),
			Currency:        sub.Currency,
			BillingPeriod:   sub.BillingPeriod,
			InvoiceCadence:  types.InvoiceCadenceAdvance,
			StartDate:       sub.StartDate,
			BaseModel:       types.GetDefaultBaseModel(ctx),
		},
	}
	s.NoError(s.GetStores().SubscriptionRepo.CreateWithLineItems(ctx, sub, lineItems))
	sub.LineItems = lineItems
	return sub
}

// TestRollupDirty_TallyAndErrorIsolation covers review Fix 3: RollupDirty
// must tally a rollable subscription, a policy-skipped one (multi-period
// commitment), and one whose per-subscription error must NOT abort the
// batch — all three get scanned in one call.
func (s *RevenueRollupSuite) TestRollupDirty_TallyAndErrorIsolation() {
	ctx := s.ctx

	rollable := s.seedFixedOnlySubscription(ctx, "ok", nil, "price_dirty_ok", true)
	annual := types.BILLING_PERIOD_ANNUAL
	multiPeriod := s.seedFixedOnlySubscription(ctx, "skip", &annual, "price_dirty_skip", true)
	failing := s.seedFixedOnlySubscription(ctx, "err", nil, "price_dirty_missing", false)

	since := time.Now().UTC().Add(-time.Hour)

	rolled, skipped, err := s.svc.RollupDirty(ctx, since)
	s.NoError(err, "a per-subscription error must never abort the batch")
	s.Equal(1, rolled, "only the plain fixed-charge subscription should roll")
	s.Equal(2, skipped, "multi-period-commitment skip + per-subscription error both tally as skipped")

	rows, err := s.store.ListBySubscriptionPeriod(ctx, rollable.ID, rollable.CurrentPeriodStart, rollable.CurrentPeriodEnd.AddDate(0, 0, 1), types.FactProvisional)
	s.NoError(err)
	s.NotEmpty(rows, "the rollable subscription must have actually been rolled despite another subscription's error")

	skipRows, err := s.store.ListBySubscriptionPeriod(ctx, multiPeriod.ID, multiPeriod.CurrentPeriodStart, multiPeriod.CurrentPeriodEnd.AddDate(0, 0, 1), types.FactProvisional)
	s.NoError(err)
	s.Empty(skipRows, "the multi-period-commitment subscription must not have been rolled")

	errRows, err := s.store.ListBySubscriptionPeriod(ctx, failing.ID, failing.CurrentPeriodStart, failing.CurrentPeriodEnd.AddDate(0, 0, 1), types.FactProvisional)
	s.NoError(err)
	s.Empty(errRows, "the failing subscription must not have written any rows")
}
