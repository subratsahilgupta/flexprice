package service

import (
	"context"
	"testing"

	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// testPriceServiceParams builds the minimal ServiceParams CalculateCost needs
// (it only touches s.Logger on the priceService receiver).
func testPriceServiceParams(t *testing.T) ServiceParams {
	t.Helper()
	log := logger.NewNoopLogger()
	return ServiceParams{
		Logger:        log,
		DB:            testutil.NewMockPostgresClient(log),
		PriceRepo:     testutil.NewInMemoryPriceStore(),
		MeterRepo:     testutil.NewInMemoryMeterStore(),
		PlanRepo:      testutil.NewInMemoryPlanStore(),
		PriceUnitRepo: testutil.NewInMemoryPriceUnitStore(),
		AddonRepo:     testutil.NewInMemoryAddonStore(),
		SubRepo:       testutil.NewInMemorySubscriptionStore(),
	}
}

// seamFlatPrice returns a USAGE, FLAT_FEE price priced at unitAmount per unit.
func seamFlatPrice(t *testing.T, unitAmount string) *price.Price {
	t.Helper()
	return &price.Price{
		ID:           "price_flat_seam_test",
		Amount:       decimal.RequireFromString(unitAmount),
		Currency:     "usd",
		Type:         types.PRICE_TYPE_USAGE,
		BillingModel: types.BILLING_MODEL_FLAT_FEE,
	}
}

// seamGraduatedPrice returns a USAGE, TIERED/SLAB price from ordered {upTo, unitAmount}
// pairs. An upTo of "0" marks the final, open-ended tier (UpTo = nil).
func seamGraduatedPrice(t *testing.T, tiers [][2]string) *price.Price {
	t.Helper()
	priceTiers := make(price.JSONBTiers, 0, len(tiers))
	for _, tier := range tiers {
		upToStr, unitAmountStr := tier[0], tier[1]
		var upTo *uint64
		if upToStr != "0" {
			v := decimal.RequireFromString(upToStr).BigInt().Uint64()
			upTo = &v
		}
		priceTiers = append(priceTiers, price.PriceTier{
			UpTo:       upTo,
			UnitAmount: decimal.RequireFromString(unitAmountStr),
		})
	}

	return &price.Price{
		ID:           "price_graduated_seam_test",
		Currency:     "usd",
		Type:         types.PRICE_TYPE_USAGE,
		BillingModel: types.BILLING_MODEL_TIERED,
		TierMode:     types.BILLING_TIER_SLAB,
		Tiers:        priceTiers,
	}
}

// TestApproachC_MarginalPrefixSumEqualsPeriodCharge is the go/no-go
// characterization test for revenue_facts Phase 2 "Approach C": daily revenue
// decomposition via CalculateCost deltas on cumulative-through-day quantities.
//
// It asserts that summing the marginal CalculateCost deltas across a
// monotonically increasing cumulative-quantity series equals the single
// full-period CalculateCost charge, for both a flat and a graduated (SLAB)
// price. If this holds, per-day revenue facts can be derived by re-running
// the pure pricing function on each day's cumulative-through-day quantity
// without any new pricing logic (Approach C). If it fails (e.g. rounding
// drift), Task 7 must fall back to Approach B (loop the full preview with
// AsOf=D per day).
func TestApproachC_MarginalPrefixSumEqualsPeriodCharge(t *testing.T) {
	ctx := context.Background()
	ps := NewPriceService(testPriceServiceParams(t))

	cases := []struct {
		name string
		p    *price.Price
	}{
		{"flat", seamFlatPrice(t, "0.01")},
		{"graduated", seamGraduatedPrice(t, [][2]string{{"1000", "0.01"}, {"0", "0.008"}})},
	}

	cum := []string{"0", "500", "2000", "5000", "20000", "40000", "60000"}
	epsilon := decimal.RequireFromString("0.000001")

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var marginalSum decimal.Decimal
			prev := decimal.Zero
			for _, q := range cum[1:] {
				d := decimal.RequireFromString(q)
				cur := ps.CalculateCost(ctx, c.p, d)
				marginalSum = marginalSum.Add(cur.Sub(prev))
				prev = cur
			}

			period := ps.CalculateCost(ctx, c.p, decimal.RequireFromString(cum[len(cum)-1]))

			residual := marginalSum.Sub(period).Abs()
			assert.True(t, residual.LessThan(epsilon),
				"marginal prefix sum %s != period charge %s (residual %s)", marginalSum, period, residual)
		})
	}
}
