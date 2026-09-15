package service

import (
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
)

// dimensionValue reads dim's value off a detailed usage-analytics item. dim
// is the ORIGINAL dimension name the view requested (pre alias-translation —
// see translateDimensions), so "customer_id" and "external_customer_id" both
// resolve to item.ExternalCustomerID.
//
// Money guard: this reads only usage-facing fields off the item
// (MeterID/Source/ExternalCustomerID/Properties) — never Subtotal,
// TotalCost, TotalDiscount, Currency, or CommitmentInfo. Phase-1 is
// usage-only and must never surface money (see analytics_shaper_test.go).
func dimensionValue(item dto.UsageAnalyticItem, dim string) string {
	switch {
	case dim == "meter_id":
		return item.MeterID
	case dim == "source":
		return item.Source
	case dim == "customer_id" || dim == "external_customer_id":
		return item.ExternalCustomerID
	case strings.HasPrefix(dim, "properties."):
		return item.Properties[strings.TrimPrefix(dim, "properties.")]
	default:
		return ""
	}
}

// shapeBreakdown converts detailed usage-analytics items into a
// dimension+metric table: one row per item, one column per requested
// dimension plus usage_quantity. item.TotalUsage is already the meter's
// resolved aggregation value (SUM/MAX/LATEST/COUNT_UNIQUE — see
// buildMeterUsageAggregationColumns), so no per-aggregation routing is
// needed here.
//
// Money guard: only item.TotalUsage (usage) is read for the metric column —
// never item.Subtotal/TotalCost/TotalDiscount/Currency/CommitmentInfo.
func shapeBreakdown(items []dto.UsageAnalyticItem, dims []string) dto.AnalyticsQueryResult {
	cols := make([]*dto.AnalyticsColumn, 0, len(dims)+1)
	for _, d := range dims {
		cols = append(cols, &dto.AnalyticsColumn{Name: d, Type: dto.ColumnTypeString, Role: dto.ColumnRoleDimension})
	}
	cols = append(cols, &dto.AnalyticsColumn{Name: "usage_quantity", Type: dto.ColumnTypeDecimal, Role: dto.ColumnRoleMetric})

	rows := make([][]string, 0, len(items))
	for _, it := range items {
		row := make([]string, 0, len(dims)+1)
		for _, d := range dims {
			row = append(row, dimensionValue(it, d))
		}
		row = append(row, it.TotalUsage.String())
		rows = append(rows, row)
	}
	return dto.AnalyticsQueryResult{Columns: cols, Rows: rows, Meta: dto.AnalyticsQueryMeta{QuerySource: "meter_usage"}}
}

// shapeTimeseries converts detailed usage-analytics items' time-bucketed
// points into a window_start(+dims)+usage_quantity table: one row per
// (item, point) pair. point.Usage is the bucket's resolved aggregation
// value, mirroring item.TotalUsage for breakdown. No cross-bucket total is
// computed in meta — summing bucket values is only correct for additive
// aggregations (e.g. SUM), not MAX/LATEST/COUNT_UNIQUE.
//
// Money guard: only p.Usage (usage) and p.Timestamp are read off each point
// — never p.Subtotal/Discount/Cost or the commitment-bucket cost fields.
func shapeTimeseries(items []dto.UsageAnalyticItem, dims []string) dto.AnalyticsQueryResult {
	cols := make([]*dto.AnalyticsColumn, 0, len(dims)+2)
	cols = append(cols, &dto.AnalyticsColumn{Name: "window_start", Type: dto.ColumnTypeDatetime, Role: dto.ColumnRoleDimension})
	for _, d := range dims {
		cols = append(cols, &dto.AnalyticsColumn{Name: d, Type: dto.ColumnTypeString, Role: dto.ColumnRoleDimension})
	}
	cols = append(cols, &dto.AnalyticsColumn{Name: "usage_quantity", Type: dto.ColumnTypeDecimal, Role: dto.ColumnRoleMetric})

	rows := make([][]string, 0)
	for _, it := range items {
		for _, p := range it.Points {
			row := make([]string, 0, len(dims)+2)
			row = append(row, p.Timestamp.Format(time.RFC3339))
			for _, d := range dims {
				row = append(row, dimensionValue(it, d))
			}
			row = append(row, p.Usage.String())
			rows = append(rows, row)
		}
	}
	return dto.AnalyticsQueryResult{
		Columns: cols,
		Rows:    rows,
		Meta:    dto.AnalyticsQueryMeta{QuerySource: "meter_usage"},
	}
}
