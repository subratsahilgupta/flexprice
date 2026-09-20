package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/revenuefact"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// GetRevenueAnalytics aggregates revenue_facts into the requested buckets:
// grouped by the given dimensions, at day/period/total granularity, with
// whole-period amounts placed per the allocation policy. Denied unless the
// tenant opted in via settings.
func (s *revenueService) GetRevenueAnalytics(ctx context.Context, req *dto.RevenueAnalyticsRequest) (*dto.RevenueAnalyticsResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if err := s.requireRevenueAnalyticsEnabled(ctx); err != nil {
		return nil, err
	}

	agg := newRevenueAggregation(req)
	const pageSize = 5000
	offset := 0
	for {
		facts, err := s.RevenueFactRepo.ListFacts(ctx, revenuefact.FactsFilter{
			DayStart:        req.StartTime,
			DayEnd:          req.EndTime,
			Status:          req.Status,
			CustomerIDs:     req.CustomerIDs,
			SubscriptionIDs: req.SubscriptionIDs,
			PriceIDs:        req.PriceIDs,
			MeterIDs:        req.MeterIDs,
			Currency:        req.Currency,
			Limit:           pageSize,
			Offset:          offset,
		})
		if err != nil {
			return nil, err
		}
		for _, f := range facts {
			agg.add(f)
		}
		if len(facts) < pageSize {
			break
		}
		offset += pageSize
	}

	return agg.response(), nil
}

// revenueAggregation folds facts into buckets keyed by (time bucket, group
// values, adjustment).
type revenueAggregation struct {
	req     *dto.RevenueAnalyticsRequest
	buckets map[string]*dto.RevenueAnalyticsRow
	// containsAllocated flips when a whole-period amount lands in a day view.
	containsAllocated bool
}

func newRevenueAggregation(req *dto.RevenueAnalyticsRequest) *revenueAggregation {
	return &revenueAggregation{req: req, buckets: map[string]*dto.RevenueAnalyticsRow{}}
}

// adjustmentRow reports whether f is one of the non-obvious components —
// commitment true-up, overage, or a revert (contra) row.
func adjustmentRow(f *revenuefact.RevenueFact) bool {
	return f.IsRevert ||
		f.RevenueSource == types.RevenueSourceCommitmentTrueup ||
		f.RevenueSource == types.RevenueSourceOverage
}

// displaySource is the source a fact shows under. When adjustments fold
// (default), true-up and overage count as usage so visible rows read as plain
// usage/fixed; reverts keep their own (mapped) source.
func (a *revenueAggregation) displaySource(f *revenuefact.RevenueFact) string {
	src := f.RevenueSource
	if !a.req.IncludeAdjustments &&
		(src == types.RevenueSourceCommitmentTrueup || src == types.RevenueSourceOverage) {
		src = types.RevenueSourceUsage
	}
	return string(src)
}

func (a *revenueAggregation) add(f *revenuefact.RevenueFact) {
	group := map[string]string{}
	for _, g := range a.req.GroupBy {
		switch g {
		case "revenue_source":
			group[g] = a.displaySource(f)
		case "customer_id":
			group[g] = f.CustomerID
		case "subscription_id":
			group[g] = f.SubscriptionID
		case "price_id":
			group[g] = lo.FromPtr(f.PriceID)
		case "meter_id":
			group[g] = lo.FromPtr(f.MeterID)
		case "currency":
			group[g] = f.Currency
		}
	}
	adjustment := a.req.IncludeAdjustments && adjustmentRow(f)

	switch a.req.Granularity {
	case types.RevenueGranularityTotal:
		a.fold(group, adjustment, nil, nil, nil, f, decimal.NewFromInt(1))
	case types.RevenueGranularityPeriod:
		ps, pe := f.PeriodStart, f.PeriodEnd
		a.fold(group, adjustment, nil, &ps, &pe, f, decimal.NewFromInt(1))
	default: // day
		if f.DecompositionMode == types.Marginal || a.req.AllocationPolicy == types.RevenueAllocationBilled {
			if f.DecompositionMode == types.PeriodOnly {
				a.containsAllocated = true
			}
			day := f.Day
			a.fold(group, adjustment, &day, nil, nil, f, decimal.NewFromInt(1))
			return
		}
		// Amortized: spread the whole-period row evenly across its period's
		// days, remainder on the last day so sums stay exact.
		a.containsAllocated = true
		days := int(f.PeriodEnd.Sub(f.PeriodStart).Hours()/24) + 1
		if days < 1 {
			days = 1
		}
		weights := make([]decimal.Decimal, days)
		for i := range weights {
			weights[i] = decimal.NewFromInt(1)
		}
		shares := spreadAmount(decimal.NewFromInt(1), weights)
		for i, share := range shares {
			day := f.PeriodStart.AddDate(0, 0, i)
			a.fold(group, adjustment, &day, nil, nil, f, share)
		}
	}
}

// fold adds fraction x of f's metrics into the bucket for (group, adjustment,
// time bucket).
func (a *revenueAggregation) fold(group map[string]string, adjustment bool, day, periodStart, periodEnd *time.Time, f *revenuefact.RevenueFact, fraction decimal.Decimal) {
	keyParts := make([]string, 0, len(a.req.GroupBy)+3)
	for _, g := range a.req.GroupBy {
		keyParts = append(keyParts, group[g])
	}
	if day != nil {
		keyParts = append(keyParts, day.Format("2006-01-02"))
	}
	if periodStart != nil {
		keyParts = append(keyParts, periodStart.Format("2006-01-02"), periodEnd.Format("2006-01-02"))
	}
	if adjustment {
		keyParts = append(keyParts, "adjustment")
	}
	key := strings.Join(keyParts, "|")

	row, ok := a.buckets[key]
	if !ok {
		row = &dto.RevenueAnalyticsRow{Day: day, PeriodStart: periodStart, PeriodEnd: periodEnd, Adjustment: adjustment}
		if len(group) > 0 {
			row.Group = group
		}
		a.buckets[key] = row
	}
	row.NetAmount = row.NetAmount.Add(f.NetAmount.Mul(fraction))
	row.UsageAtListRate = row.UsageAtListRate.Add(f.UsageAtListRate.Mul(fraction))
	row.TierDelta = row.TierDelta.Add(f.TierDelta.Mul(fraction))
	row.EntitlementAmount = row.EntitlementAmount.Add(f.EntitlementAmount.Mul(fraction))
	row.LineDiscount = row.LineDiscount.Add(f.LineDiscount.Mul(fraction))
	row.InvoiceDiscount = row.InvoiceDiscount.Add(f.InvoiceDiscount.Mul(fraction))
	row.BillableQty = row.BillableQty.Add(f.BillableQty.Mul(fraction))
	row.EntitlementQty = row.EntitlementQty.Add(f.EntitlementQty.Mul(fraction))
}

func (a *revenueAggregation) response() *dto.RevenueAnalyticsResponse {
	rows := make([]*dto.RevenueAnalyticsRow, 0, len(a.buckets))
	for _, row := range a.buckets {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		ti, tj := rowSortTime(rows[i]), rowSortTime(rows[j])
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return rowSortKey(rows[i]) < rowSortKey(rows[j])
	})
	return &dto.RevenueAnalyticsResponse{Rows: rows, ContainsAllocated: a.containsAllocated}
}

func rowSortTime(r *dto.RevenueAnalyticsRow) time.Time {
	if r.Day != nil {
		return *r.Day
	}
	if r.PeriodStart != nil {
		return *r.PeriodStart
	}
	return time.Time{}
}

func rowSortKey(r *dto.RevenueAnalyticsRow) string {
	parts := make([]string, 0, len(r.Group)+1)
	keys := make([]string, 0, len(r.Group))
	for k := range r.Group {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts = append(parts, r.Group[k])
	}
	if r.Adjustment {
		parts = append(parts, "zz_adjustment")
	}
	return strings.Join(parts, "|")
}
