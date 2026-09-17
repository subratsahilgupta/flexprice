package service

import (
	"context"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// curveTestOpts is the config assembled by the curveXxx functional options
// below, consumed by buildTestCurveInput.
type curveTestOpts struct {
	perDay    decimal.Decimal
	allowance decimal.Decimal
	price     *price.Price
	days      int
}

type curveTestOpt func(*curveTestOpts)

// curvePerDay sets the flat quantity of usage fired on each day of the period.
func curvePerDay(qty int64) curveTestOpt {
	return func(o *curveTestOpts) { o.perDay = decimal.NewFromInt(qty) }
}

// curveAllowance sets the entitlement quantity consumed earliest-first.
func curveAllowance(qty int64) curveTestOpt {
	return func(o *curveTestOpts) { o.allowance = decimal.NewFromInt(qty) }
}

// curveFlatRate builds a USAGE, FLAT_FEE price priced at unitAmount per unit.
func curveFlatRate(unitAmount string) curveTestOpt {
	return func(o *curveTestOpts) {
		o.price = &price.Price{
			ID:           "price_curve_flat_test",
			Amount:       decimal.RequireFromString(unitAmount),
			Currency:     "usd",
			Type:         types.PRICE_TYPE_USAGE,
			BillingModel: types.BILLING_MODEL_FLAT_FEE,
		}
	}
}

// curveDays sets how many days of usage to fire and the period length.
func curveDays(n int) curveTestOpt {
	return func(o *curveTestOpts) { o.days = n }
}

// curvePrice sets an arbitrary price fixture directly (e.g. a graduated/SLAB
// price built inline by the caller, rather than via curveFlatRate).
func curvePrice(p *price.Price) curveTestOpt {
	return func(o *curveTestOpts) { o.price = p }
}

// buildTestCurveInput seeds an in-memory meter-usage store with `days` daily
// records of `perDay` quantity, and returns the LineItemPricingInput for the
// resulting period (BuildUsageCurve reads usage back out via
// GetCumulativeDailyUsage, exactly like the real ClickHouse-backed path).
func buildTestCurveInput(t *testing.T, ctx context.Context, store *testutil.InMemoryMeterUsageStore, opts ...curveTestOpt) LineItemPricingInput {
	t.Helper()

	cfg := curveTestOpts{days: 1}
	for _, opt := range opts {
		opt(&cfg)
	}
	require.NotNil(t, cfg.price, "a price option (e.g. curveFlatRate) is required")

	const meterID = "meter_curve_test"
	periodStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	records := make([]*events.MeterUsage, 0, cfg.days)
	for d := 0; d < cfg.days; d++ {
		ts := periodStart.AddDate(0, 0, d).Add(time.Hour)
		id := types.GenerateUUIDWithPrefix("mu_curve")
		records = append(records, &events.MeterUsage{
			Event: events.Event{
				ID:            id,
				TenantID:      types.GetTenantID(ctx),
				EnvironmentID: types.GetEnvironmentID(ctx),
				EventName:     "curve_test_event",
				Timestamp:     ts,
				IngestedAt:    ts,
				Properties:    map[string]interface{}{},
			},
			MeterID:    meterID,
			QtyTotal:   cfg.perDay,
			UniqueHash: id,
		})
	}
	require.NoError(t, store.BulkInsertMeterUsage(ctx, records))

	return LineItemPricingInput{
		Price:       cfg.price,
		MeterID:     meterID,
		PeriodStart: periodStart,
		PeriodEnd:   periodStart.AddDate(0, 0, cfg.days),
		Allowance:   cfg.allowance,
	}
}

// curveMarginal returns the per-day delta at slice index idx: got[idx] minus
// got[idx-1] (or got[0] alone when idx==0). Note idx is a slice position, not
// a 1-based day number — index 10 is the 11th day (Day fields are 0-indexed
// from period start).
func curveMarginal(got []DayCharge, idx int) decimal.Decimal {
	if idx == 0 {
		return got[0].CumulativeCharge
	}
	return got[idx].CumulativeCharge.Sub(got[idx-1].CumulativeCharge)
}

// TestBuildUsageCurve_AllowanceThenFlatRate is the brief's headline test:
// 2000/day usage, a 20000 allowance, a flat $0.01/unit price, over 30 days.
// Days 1-10 stay fully inside the allowance (charge == 0); day 11 is the
// first day billed usage crosses the allowance, so its marginal charge is
// 2000 * $0.01 = $20; by day 30, 20 billed days * $20 = $400 cumulative.
func TestBuildUsageCurve_AllowanceThenFlatRate(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewInMemoryMeterUsageStore()
	svc := NewRevenueCurveService(ServiceParams{
		Logger:         logger.NewNoopLogger(),
		MeterUsageRepo: store,
		PriceRepo:      testutil.NewInMemoryPriceStore(),
		MeterRepo:      testutil.NewInMemoryMeterStore(),
		PlanRepo:       testutil.NewInMemoryPlanStore(),
		PriceUnitRepo:  testutil.NewInMemoryPriceUnitStore(),
		AddonRepo:      testutil.NewInMemoryAddonStore(),
		SubRepo:        testutil.NewInMemorySubscriptionStore(),
	})

	curve := buildTestCurveInput(t, ctx, store,
		curvePerDay(2000),
		curveAllowance(20000),
		curveFlatRate("0.01"),
		curveDays(30),
	)

	got, err := svc.BuildUsageCurve(ctx, curve)
	require.NoError(t, err)
	require.Len(t, got, 30)

	// (a) days fully inside the allowance have zero cumulative charge.
	assert.True(t, got[9].CumulativeCharge.IsZero(), "day 10 still within allowance")

	// (b) after the allowance is exhausted, charge rises at the flat rate.
	assert.Equal(t, "20", curveMarginal(got, 10).String(), "day 11 marginal charge")

	// (c) marginal deltas sum to the period charge (20 billed days * $20).
	var marginalSum decimal.Decimal
	for i := range got {
		marginalSum = marginalSum.Add(curveMarginal(got, i))
	}
	assert.Equal(t, "400", got[29].CumulativeCharge.String(), "period cumulative charge")
	assert.True(t, marginalSum.Equal(got[29].CumulativeCharge), "marginal sum must equal period charge")

	// (d) entitlement is consumed earliest-first as a function of gross usage.
	assert.Equal(t, "20000", got[9].CumulativeEntitlementQty.String(), "allowance fully consumed by day 10")
	assert.Equal(t, "20000", got[29].CumulativeEntitlementQty.String(), "entitlement consumption caps at the allowance")
	assert.Equal(t, "40000", got[29].CumulativeBillableQty.String())
	assert.Equal(t, "60000", got[29].CumulativeGrossQty.String())

	// tier1_rate is the flat $0.01 rate; flat pricing has zero tier delta.
	assert.Equal(t, "600", got[29].UsageAtListRate.String(), "gross qty x list rate")
	assert.True(t, got[29].TierDelta.IsZero(), "flat pricing has no tier delta")
}

// TestBuildUsageCurve_GraduatedPrice exercises a SLAB-tiered price (proven
// additive by the Approach-C seam test) with no allowance: cumulative charge
// on each day must equal CalculateCost run directly on that day's cumulative
// quantity, and TierDelta must reflect the deviation from a flat tier1 charge.
func TestBuildUsageCurve_GraduatedPrice(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewInMemoryMeterUsageStore()
	params := ServiceParams{
		Logger:         logger.NewNoopLogger(),
		MeterUsageRepo: store,
		PriceRepo:      testutil.NewInMemoryPriceStore(),
		MeterRepo:      testutil.NewInMemoryMeterStore(),
		PlanRepo:       testutil.NewInMemoryPlanStore(),
		PriceUnitRepo:  testutil.NewInMemoryPriceUnitStore(),
		AddonRepo:      testutil.NewInMemoryAddonStore(),
		SubRepo:        testutil.NewInMemorySubscriptionStore(),
	}
	svc := NewRevenueCurveService(params)
	priceSvc := NewPriceService(params)

	tier1 := decimal.RequireFromString("0.01")
	tier2 := decimal.RequireFromString("0.008")
	upTo := uint64(1000)
	graduated := &price.Price{
		ID:           "price_curve_graduated_test",
		Currency:     "usd",
		Type:         types.PRICE_TYPE_USAGE,
		BillingModel: types.BILLING_MODEL_TIERED,
		TierMode:     types.BILLING_TIER_SLAB,
		Tiers: price.JSONBTiers{
			{UpTo: &upTo, UnitAmount: tier1},
			{UpTo: nil, UnitAmount: tier2},
		},
	}

	curve := buildTestCurveInput(t, ctx, store,
		curvePerDay(500),
		curveDays(5),
		curvePrice(graduated),
	)

	got, err := svc.BuildUsageCurve(ctx, curve)
	require.NoError(t, err)
	require.Len(t, got, 5)

	for i, day := range got {
		cumQty := decimal.NewFromInt(int64((i + 1) * 500))
		wantCharge := priceSvc.CalculateCost(ctx, graduated, cumQty)
		assert.True(t, day.CumulativeCharge.Equal(wantCharge),
			"day %d: got %s want %s", i, day.CumulativeCharge, wantCharge)
		assert.True(t, day.CumulativeGrossQty.Equal(cumQty))
		assert.True(t, day.CumulativeBillableQty.Equal(cumQty), "no allowance: billable == gross")
		assert.True(t, day.CumulativeEntitlementQty.IsZero())

		wantListRate := cumQty.Mul(tier1)
		assert.True(t, day.UsageAtListRate.Equal(wantListRate))
		wantTierDelta := wantCharge.Sub(cumQty.Mul(tier1))
		assert.True(t, day.TierDelta.Equal(wantTierDelta))
	}

	// Day 3 crosses the 1000-unit tier boundary (cum qty 1500): TierDelta
	// must be non-zero once usage spills into the second, cheaper tier.
	assert.False(t, got[2].TierDelta.IsZero(), "tier delta should be non-zero past the tier boundary")
}
