package service

import (
	"strings"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/shopspring/decimal"
)

// dimensionValue reads dim's value off a detailed usage-analytics item. dim
// is the ORIGINAL dimension name the view requested (pre alias-translation —
// see translateDimensions), so "customer_id" and "external_customer_id" both
// resolve to item.ExternalCustomerID.
func dimensionValue(item dto.UsageAnalyticItem, dim string) any {
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
		return nil
	}
}

// shapeBreakdown converts detailed usage-analytics items into a
// dimension+metric table: one row per item, one column per requested
// dimension plus usage_quantity. item.TotalUsage is already the meter's
// resolved aggregation value (SUM/MAX/LATEST/COUNT_UNIQUE — see
// buildMeterUsageAggregationColumns), so no per-aggregation routing is
// needed here.
func shapeBreakdown(items []dto.UsageAnalyticItem, dims []string) dto.AnalyticsQueryResult {
	cols := make([]*dto.AnalyticsColumn, 0, len(dims)+1)
	for _, d := range dims {
		cols = append(cols, &dto.AnalyticsColumn{Name: d, Type: "string", Role: "dimension"})
	}
	cols = append(cols, &dto.AnalyticsColumn{Name: "usage_quantity", Type: "decimal", Role: "metric"})

	rows := make([][]any, 0, len(items))
	for _, it := range items {
		row := make([]any, 0, len(dims)+1)
		for _, d := range dims {
			row = append(row, dimensionValue(it, d))
		}
		row = append(row, it.TotalUsage.String())
		rows = append(rows, row)
	}
	return dto.AnalyticsQueryResult{Columns: cols, Rows: rows, Meta: map[string]any{"query_source": "meter_usage"}}
}

// shapeTimeseries converts detailed usage-analytics items' time-bucketed
// points into a window_start(+dims)+usage_quantity table: one row per
// (item, point) pair. point.Usage is the bucket's resolved aggregation
// value, mirroring item.TotalUsage for breakdown.
func shapeTimeseries(items []dto.UsageAnalyticItem, dims []string) dto.AnalyticsQueryResult {
	cols := make([]*dto.AnalyticsColumn, 0, len(dims)+2)
	cols = append(cols, &dto.AnalyticsColumn{Name: "window_start", Type: "datetime", Role: "dimension"})
	for _, d := range dims {
		cols = append(cols, &dto.AnalyticsColumn{Name: d, Type: "string", Role: "dimension"})
	}
	cols = append(cols, &dto.AnalyticsColumn{Name: "usage_quantity", Type: "decimal", Role: "metric"})

	rows := make([][]any, 0)
	total := decimal.Zero
	for _, it := range items {
		for _, p := range it.Points {
			row := make([]any, 0, len(dims)+2)
			row = append(row, p.Timestamp)
			for _, d := range dims {
				row = append(row, dimensionValue(it, d))
			}
			row = append(row, p.Usage.String())
			rows = append(rows, row)
			total = total.Add(p.Usage)
		}
	}
	return dto.AnalyticsQueryResult{
		Columns: cols,
		Rows:    rows,
		Meta:    map[string]any{"query_source": "meter_usage", "total": total.String()},
	}
}
