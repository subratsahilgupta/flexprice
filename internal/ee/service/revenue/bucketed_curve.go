package revenue

// Bucketed pricing charges each window on its own quantity, so a cumulative
// curve cannot reproduce it: 5 units/day on a $1-per-10 package with daily
// buckets bills $1 every day, while re-pricing the running total bills $1
// once. This file rebuilds the daily shape the way the engine charges it —
// price each window, then group windows into days.

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/ee/service"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// bucketedCurveInput describes one bucketed usage line item to split per day.
type bucketedCurveInput struct {
	Price *price.Price
	Meter *meter.Meter
	Sub   *subscription.Subscription

	// PeriodStart/PeriodEnd bound the half-open billing window [start, end).
	PeriodStart time.Time
	PeriodEnd   time.Time

	// EngineAmount is the charge the billing engine computed for the line —
	// the daily shape always scales to sum exactly to it.
	EngineAmount decimal.Decimal

	ExternalCustomerIDs []string
	Timezone            string
}

// bucketedDayGrain reports whether this pair's windows nest inside days. Week
// and month windows span days, so their charge cannot be attributed to one.
func bucketedDayGrain(p *price.Price, m *meter.Meter) bool {
	if !price.IsBucketed(p, m) {
		return false
	}
	switch price.ResolveBucketSize(p, m) {
	case types.WindowSizeWeek, types.WindowSizeMonth:
		return false
	default:
		return true
	}
}

// buildBucketedCurve prices each of the line's windows the way the engine does
// and accumulates them into a daily curve. ok=false means the shape is
// unknowable (no windows carry usage) and the caller falls back to period_only.
//
// Windows are cut in UTC — the convention the billing engine's bucketed reads
// use — while revenue days follow the subscription's timezone. A window is
// attributed to the day holding its start, so an offset that does not divide
// the window size can shift one boundary window by a day; the period total is
// unaffected, since the curve is scaled to the engine's own amount.
func (s *revenueService) buildBucketedCurve(ctx context.Context, in bucketedCurveInput) ([]dayCharge, bool, error) {
	aggType := types.AggregationMax
	groupBy := price.BucketedGroupBy(in.Price, in.Meter)
	if price.IsBucketedSum(in.Price, in.Meter) {
		aggType = types.AggregationSum
		groupBy = ""
	}
	var paramsGroupBy []string
	if groupBy != "" {
		paramsGroupBy = []string{"properties." + groupBy}
	}

	usage, err := s.MeterUsageRepo.GetUsageForBucketedMeters(ctx, &events.MeterUsageQueryParams{
		TenantID:            types.GetTenantID(ctx),
		EnvironmentID:       types.GetEnvironmentID(ctx),
		ExternalCustomerIDs: in.ExternalCustomerIDs,
		MeterID:             in.Meter.ID,
		StartTime:           in.PeriodStart,
		EndTime:             in.PeriodEnd,
		AggregationType:     aggType,
		WindowSize:          price.ResolveBucketSize(in.Price, in.Meter),
		BillingAnchor:       &in.Sub.BillingAnchor,
		GroupBy:             paramsGroupBy,
		UseFinal:            true,
	})
	if err != nil {
		return nil, false, err
	}
	if usage == nil || len(usage.Results) == 0 {
		return nil, false, nil
	}

	// Each window is priced on its own quantity — per group when the price is
	// tiered and the meter groups, which is how the engine charges it too.
	priceSvc := service.NewPriceService(s.ServiceParams)
	loc := timezoneLocation(in.Timezone)
	chargeByDay := make(map[string]decimal.Decimal, len(usage.Results))
	qtyByDay := make(map[string]decimal.Decimal, len(usage.Results))
	measured := decimal.Zero
	for _, r := range usage.Results {
		key := r.WindowSize.In(loc).Format(dayKeyLayout)
		charge := priceSvc.CalculateCost(ctx, in.Price, r.Value)
		chargeByDay[key] = chargeByDay[key].Add(charge)
		qtyByDay[key] = qtyByDay[key].Add(r.Value)
		measured = measured.Add(charge)
	}

	// The engine prices from its own snapshot and rounds per line, so scale
	// the measured shape onto its amount; TierDelta carries the difference.
	scale := decimal.NewFromInt(1)
	switch {
	case in.EngineAmount.IsZero():
		scale = decimal.Zero
	case measured.IsZero():
		return nil, false, nil
	default:
		scale = in.EngineAmount.Div(measured)
	}

	startDay, endDay := localDayWalk(in.PeriodStart, in.PeriodEnd, loc)
	if tomorrow := todayEnd(loc); tomorrow.Before(endDay) {
		endDay = tomorrow
	}
	if !startDay.Before(endDay) {
		return nil, false, nil
	}

	rate := listRate(in.Price)
	curve := make([]dayCharge, 0)
	runCharge, runQty := decimal.Zero, decimal.Zero
	for cur := startDay; cur.Before(endDay); cur = cur.AddDate(0, 0, 1) {
		key := cur.Format(dayKeyLayout)
		runCharge = runCharge.Add(chargeByDay[key].Mul(scale))
		runQty = runQty.Add(qtyByDay[key])
		curve = append(curve, dayCharge{
			Day:                   time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, time.UTC),
			CumulativeCharge:      runCharge,
			CumulativeGrossQty:    runQty,
			CumulativeBillableQty: runQty,
			UsageAtListRate:       runQty.Mul(rate),
			TierDelta:             runCharge.Sub(runQty.Mul(rate)),
		})
	}
	if len(curve) == 0 {
		return nil, false, nil
	}

	// Pin the final cumulative to the engine amount so the marginal rows sum
	// with zero residual under the scaling division.
	last := &curve[len(curve)-1]
	last.CumulativeCharge = in.EngineAmount
	last.TierDelta = in.EngineAmount.Sub(last.UsageAtListRate)
	return curve, true, nil
}

// todayEnd is the local midnight starting tomorrow — the walk bound that keeps
// an open period from booking days that have not happened.
func todayEnd(loc *time.Location) time.Time {
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
}
