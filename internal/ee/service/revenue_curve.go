package service

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/domain/price"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// dayKeyLayout is a location-independent calendar-day key. Map keys can't be
// raw time.Time here: GetCumulativeDailyUsage implementations (ClickHouse and
// in-memory) each resolve their own *time.Location for the same IANA name, so
// two time.Time values for the same instant/zone aren't guaranteed to be
// == comparable (time.Time equality includes the loc pointer).
const dayKeyLayout = "2006-01-02"

// DayCharge is one calendar day's CUMULATIVE-through-that-day revenue
// decomposition for a single usage line item, produced by BuildUsageCurve
// (Approach C). Every field is a running total through Day, not a per-day
// marginal value — the rollup derives marginal(D) = cur.X - prev.X per field.
type DayCharge struct {
	Day time.Time

	// CumulativeCharge is CalculateCost(price, CumulativeBillableQty) — the
	// actual engine charge through this day, net of the entitlement limit.
	CumulativeCharge decimal.Decimal
	// CumulativeGrossQty is the raw cumulative metered quantity through this
	// day, before the entitlement limit is applied.
	CumulativeGrossQty decimal.Decimal
	// CumulativeBillableQty is max(0, CumulativeGrossQty - EntitlementLimit).
	CumulativeBillableQty decimal.Decimal
	// CumulativeEntitlementQty is min(CumulativeGrossQty, EntitlementLimit) —
	// the entitlement consumed earliest-first, purely as a function of gross usage.
	CumulativeEntitlementQty decimal.Decimal
	// UsageAtListRate is CumulativeGrossQty x tier1_rate: gross usage priced at
	// the list rate, before any entitlement giveback.
	UsageAtListRate decimal.Decimal
	// TierDelta is CumulativeCharge - (CumulativeBillableQty x tier1_rate) —
	// the deviation from a flat list-rate charge caused by graduated tiering
	// on the billed (post-entitlement) quantity. Zero for flat pricing, since
	// tier1_rate is then the only rate.
	TierDelta decimal.Decimal
}

// LineItemPricingInput is what BuildUsageCurve needs to price one usage line
// item's cumulative usage curve over a billing period. Tenant/environment are
// read from ctx, not carried here.
type LineItemPricingInput struct {
	Price   *price.Price
	MeterID string

	PeriodStart time.Time
	// PeriodEnd is exclusive: the period is the half-open window
	// [PeriodStart, PeriodEnd), matching CumulativeDailyUsageParams.
	PeriodEnd time.Time

	// EntitlementLimit is the entitlement quantity consumed earliest-first before
	// billing starts. Negative values are treated as zero.
	EntitlementLimit decimal.Decimal

	// Timezone is the IANA name used to bucket usage into calendar days.
	// Empty falls back to UTC.
	Timezone string
}

type revenueCurveService struct {
	ServiceParams
}

// NewRevenueCurveService constructs the Approach-C cumulative usage curve
// helper: per-day cumulative charge derivation for revenue-facts decomposition.
func NewRevenueCurveService(params ServiceParams) *revenueCurveService {
	return &revenueCurveService{ServiceParams: params}
}

// BuildUsageCurve returns one DayCharge per calendar day in li's period, each
// carrying cumulative-through-that-day charge/quantities (Approach C).
//
// Mechanism: read the cumulative gross usage curve once via
// GetCumulativeDailyUsage, then for each day D re-run CalculateCost on
// billable(D) = max(0, gross(D) - EntitlementLimit). CalculateCost is exact and
// additive over monotonic cumulative prefixes for flat/graduated pricing
// (proven by the Approach-C seam test in revenue_curve_seam_test.go), so no
// extra pricing logic or ClickHouse read is needed beyond the single call.
func (s *revenueCurveService) BuildUsageCurve(ctx context.Context, li LineItemPricingInput) ([]DayCharge, error) {
	if li.Price == nil {
		return nil, ierr.NewError("price is required").
			WithHint("BuildUsageCurve requires a non-nil price").
			Mark(ierr.ErrValidation)
	}
	if li.MeterID == "" {
		return nil, ierr.NewError("meter id is required").
			WithHint("BuildUsageCurve requires a meter id").
			Mark(ierr.ErrValidation)
	}
	if !li.PeriodStart.Before(li.PeriodEnd) {
		return nil, ierr.NewError("invalid period").
			WithHint("period start must be before period end").
			Mark(ierr.ErrValidation)
	}

	loc := time.UTC
	if li.Timezone != "" {
		if l, err := time.LoadLocation(li.Timezone); err == nil {
			loc = l
		}
	}

	points, err := s.MeterUsageRepo.GetCumulativeDailyUsage(ctx, &events.CumulativeDailyUsageParams{
		TenantID:      types.GetTenantID(ctx),
		EnvironmentID: types.GetEnvironmentID(ctx),
		MeterID:       li.MeterID,
		StartTime:     li.PeriodStart,
		EndTime:       li.PeriodEnd,
		UseFinal:      true,
		Timezone:      li.Timezone,
	})
	if err != nil {
		return nil, err
	}

	cumByDay := make(map[string]decimal.Decimal, len(points))
	for _, p := range points {
		cumByDay[p.Day.Format(dayKeyLayout)] = p.CumulativeQty
	}

	entitlementLimit := li.EntitlementLimit
	if entitlementLimit.IsNegative() {
		entitlementLimit = decimal.Zero
	}
	tier1Rate := listRate(li.Price)
	priceSvc := NewPriceService(s.ServiceParams)

	startLocal := li.PeriodStart.In(loc)
	cur := time.Date(startLocal.Year(), startLocal.Month(), startLocal.Day(), 0, 0, 0, 0, loc)

	curve := make([]DayCharge, 0)
	runningGross := decimal.Zero
	for cur.Before(li.PeriodEnd) {
		// A day with no new usage carries forward the prior running total —
		// the cumulative curve is only ever produced for days that have
		// usage rows, but every calendar day in the period needs a DayCharge.
		if qty, ok := cumByDay[cur.Format(dayKeyLayout)]; ok {
			runningGross = qty
		}

		billable := runningGross.Sub(entitlementLimit)
		if billable.IsNegative() {
			billable = decimal.Zero
		}
		entitlementQty := entitlementLimit
		if runningGross.LessThan(entitlementLimit) {
			entitlementQty = runningGross
		}

		charge := priceSvc.CalculateCost(ctx, li.Price, billable)

		curve = append(curve, DayCharge{
			Day:                      time.Date(cur.Year(), cur.Month(), cur.Day(), 0, 0, 0, 0, time.UTC),
			CumulativeCharge:         charge,
			CumulativeGrossQty:       runningGross,
			CumulativeBillableQty:    billable,
			CumulativeEntitlementQty: entitlementQty,
			UsageAtListRate:          runningGross.Mul(tier1Rate),
			TierDelta:                charge.Sub(billable.Mul(tier1Rate)),
		})

		cur = cur.AddDate(0, 0, 1)
	}

	return curve, nil
}

// listRate is the tier1/list rate used for UsageAtListRate and TierDelta: the
// first tier's unit amount for tiered pricing, or the flat unit amount
// otherwise (a flat price has one rate, which doubles as its own tier1).
func listRate(p *price.Price) decimal.Decimal {
	if len(p.Tiers) > 0 {
		return p.Tiers[0].UnitAmount
	}
	return p.Amount
}
