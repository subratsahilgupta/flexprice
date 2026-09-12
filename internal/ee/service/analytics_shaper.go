package service

import (
	"strings"

	"github.com/flexprice/flexprice/internal/domain/events"
)

// Column describes one column of a shaped QueryResult.
type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Role     string `json:"role"` // "dimension" | "metric"
	Currency string `json:"currency,omitempty"`
}

// QueryResult is the visualization-agnostic shape all analytics queries render into.
type QueryResult struct {
	Columns []Column       `json:"columns"`
	Rows    [][]any        `json:"rows"`
	Meta    map[string]any `json:"meta"`
}

// ShapeBreakdown converts detailed meter_usage results into a dimension+metric
// table: one row per item, one column per requested dimension plus usage_quantity.
func ShapeBreakdown(items []events.MeterUsageDetailedResult, dims []string) QueryResult {
	cols := make([]Column, 0, len(dims)+1)
	for _, d := range dims {
		cols = append(cols, Column{Name: d, Type: "string", Role: "dimension"})
	}
	cols = append(cols, Column{Name: "usage_quantity", Type: "decimal", Role: "metric"})

	rows := make([][]any, 0, len(items))
	for _, it := range items {
		row := make([]any, 0, len(dims)+1)
		for _, d := range dims {
			key := strings.TrimPrefix(d, "properties.")
			row = append(row, it.Properties[key])
		}
		row = append(row, it.TotalUsage.String())
		rows = append(rows, row)
	}
	return QueryResult{Columns: cols, Rows: rows, Meta: map[string]any{"query_source": "meter_usage"}}
}

// ShapeTimeseries converts an aggregated meter_usage result's time-bucketed
// points into a window_start+usage_quantity table.
func ShapeTimeseries(res *events.MeterUsageAggregationResult) QueryResult {
	cols := []Column{
		{Name: "window_start", Type: "datetime", Role: "dimension"},
		{Name: "usage_quantity", Type: "decimal", Role: "metric"},
	}
	rows := make([][]any, 0, len(res.Points))
	for _, p := range res.Points {
		rows = append(rows, []any{p.WindowStart, p.Value.String()})
	}
	return QueryResult{Columns: cols, Rows: rows, Meta: map[string]any{"query_source": "meter_usage", "total": res.TotalValue.String()}}
}
