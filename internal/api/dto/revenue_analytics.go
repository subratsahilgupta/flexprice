package dto

import (
	"time"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// RevenueAnalyticsRequest queries revenue_facts as an analytics surface.
type RevenueAnalyticsRequest struct {
	// StartTime/EndTime bound the day range (inclusive days derived in UTC).
	StartTime time.Time `json:"start_time" validate:"required"`
	EndTime   time.Time `json:"end_time" validate:"required"`

	// Granularity buckets results by day, by billing period, or into one total.
	Granularity types.RevenueGranularity `json:"granularity"`

	// AllocationPolicy applies to the day granularity: billed keeps
	// whole-period amounts on their booked day; amortized spreads them evenly
	// across their period's days.
	AllocationPolicy types.RevenueAllocationPolicy `json:"allocation_policy"`

	// Status selects booked (FINAL, default) or in-progress (PROVISIONAL)
	// rows — never both in one response.
	Status types.FactStatus `json:"status"`

	// GroupBy dimensions: revenue_source, source (the event source recorded in
	// meter_usage; requires customer_ids or subscription_ids), customer_id,
	// subscription_id, price_id, meter_id, currency.
	GroupBy []string `json:"group_by"`

	// Filters restrict the rows before aggregation.
	CustomerIDs     []string `json:"customer_ids"`
	SubscriptionIDs []string `json:"subscription_ids"`
	PriceIDs        []string `json:"price_ids"`
	MeterIDs        []string `json:"meter_ids"`
	Currency        string   `json:"currency"`

	// IncludeAdjustments breaks the non-obvious components — commitment
	// true-ups, overage and revert (contra) rows — out as their own rows,
	// each labeled with its kind in adjustment_type. Off, they fold into
	// their parent buckets (true-up/overage into usage, reverts into their
	// own source) so visible rows read as plain usage/fixed. Totals are
	// identical either way.
	IncludeAdjustments bool `json:"include_adjustments"`
}

var revenueAnalyticsGroupBy = []string{"revenue_source", "source", "customer_id", "subscription_id", "price_id", "meter_id", "currency"}

// revenueAnalyticsMaxRangeDays caps the query window so one request cannot
// scan unbounded history.
const revenueAnalyticsMaxRangeDays = 90

func (r *RevenueAnalyticsRequest) Validate() error {
	if r.StartTime.IsZero() || r.EndTime.IsZero() || !r.StartTime.Before(r.EndTime) {
		return ierr.NewError("invalid time range").
			WithHint("start_time must be before end_time").
			Mark(ierr.ErrValidation)
	}
	if r.EndTime.Sub(r.StartTime) > revenueAnalyticsMaxRangeDays*24*time.Hour {
		return ierr.NewErrorf("time range exceeds %d days", revenueAnalyticsMaxRangeDays).
			WithHint("Narrow the time range or run multiple requests").
			Mark(ierr.ErrValidation)
	}
	if r.Granularity == "" {
		r.Granularity = types.RevenueGranularityDay
	}
	if err := r.Granularity.Validate(); err != nil {
		return err
	}
	if r.AllocationPolicy == "" {
		r.AllocationPolicy = types.RevenueAllocationBilled
	}
	if err := r.AllocationPolicy.Validate(); err != nil {
		return err
	}
	if r.Status == "" {
		r.Status = types.FactFinal
	}
	if err := r.Status.Validate(); err != nil {
		return err
	}
	for _, g := range r.GroupBy {
		if !lo.Contains(revenueAnalyticsGroupBy, g) {
			return ierr.NewErrorf("unsupported group_by %q", g).
				WithHint("group_by must be one of: revenue_source, source, customer_id, subscription_id, price_id, meter_id, currency").
				Mark(ierr.ErrValidation)
		}
	}
	// Source allocation reads per-subscription usage, so it needs a bounded
	// customer or subscription scope.
	if lo.Contains(r.GroupBy, "source") && len(r.CustomerIDs) == 0 && len(r.SubscriptionIDs) == 0 {
		return ierr.NewError("group_by source requires a customer or subscription filter").
			WithHint("Pass customer_ids or subscription_ids when grouping by source").
			Mark(ierr.ErrValidation)
	}
	return nil
}

// RevenueAnalyticsRow is one aggregated bucket.
type RevenueAnalyticsRow struct {
	// Group holds the requested dimensions and their values for this bucket.
	Group map[string]string `json:"group,omitempty"`

	// Day is set for day granularity; PeriodStart/PeriodEnd for period.
	Day         *time.Time `json:"day,omitempty"`
	PeriodStart *time.Time `json:"period_start,omitempty"`
	PeriodEnd   *time.Time `json:"period_end,omitempty"`

	// AdjustmentType labels rows broken out by include_adjustments:
	// "commitment_trueup", "overage" or "revert". Empty for plain rows.
	AdjustmentType string `json:"adjustment_type,omitempty"`

	NetAmount         decimal.Decimal `json:"net_amount"`
	UsageAtListRate   decimal.Decimal `json:"usage_at_list_rate"`
	TierDelta         decimal.Decimal `json:"tier_delta"`
	EntitlementAmount decimal.Decimal `json:"entitlement_amount"`
	LineDiscount      decimal.Decimal `json:"line_discount"`
	InvoiceDiscount   decimal.Decimal `json:"invoice_discount"`
	BillableQty       decimal.Decimal `json:"billable_qty"`
	EntitlementQty    decimal.Decimal `json:"entitlement_qty"`
}

// RevenueAnalyticsResponse is the aggregated result.
type RevenueAnalyticsResponse struct {
	Rows []*RevenueAnalyticsRow `json:"rows"`

	// ContainsAllocated is true when any bucket includes whole-period amounts
	// spread across days (amortized) or booked on a single day (billed) —
	// i.e. the day view carries period-shaped charges, not only true daily
	// accruals.
	ContainsAllocated bool `json:"contains_allocated"`
}
