package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/types"
)

func GetAggregator(aggregationType types.AggregationType) events.Aggregator {
	switch aggregationType {
	case types.AggregationCount:
		return &CountAggregator{}
	case types.AggregationSum:
		return &SumAggregator{}
	}

	// TODO: Add rest of the aggregators later
	return nil
}

func getDeduplicationKey() string {
	return "id, tenant_id, external_customer_id, customer_id, event_name"
}

// Helper function for ClickHouse datetime formatting
func formatClickHouseDateTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.000")
}

// Helper function for window size formatting
func formatWindowSize(windowSize string) string {
	switch windowSize {
	case "MINUTE":
		return "toStartOfMinute(timestamp)"
	case "HOUR":
		return "toStartOfHour(timestamp)"
	case "DAY":
		return "toStartOfDay(timestamp)"
	default:
		return "" // No windowing if windowSize is empty or invalid
	}
}

// SumAggregator implements sum aggregation
type SumAggregator struct{}

func (a *SumAggregator) GetQuery(ctx context.Context, params *events.UsageParams) string {
	windowSize := formatWindowSize(params.WindowSize)
	selectClause := ""
	windowClause := ""
	groupByClause := ""
	windowGroupBy := ""

	if windowSize != "" {
		selectClause = "window_size,"
		windowClause = fmt.Sprintf("%s AS window_size,", windowSize)
		groupByClause = "GROUP BY window_size ORDER BY window_size"
		windowGroupBy = ", window_size"
	}

	customerFilter := ""
	if params.ExternalCustomerID != "" {
		customerFilter = fmt.Sprintf("AND external_customer_id = '%s'", params.ExternalCustomerID)
	}

	// 1. First buckets by time window if specified
	// 2. Deduplicates within each window/bucket
	// 3. Sums the values
	return fmt.Sprintf(`
        SELECT 
            %s sum(value) as total
        FROM (
            SELECT
                %s anyLast(JSONExtractFloat(assumeNotNull(properties), '%s')) as value
            FROM events
            PREWHERE event_name = '%s' 
                AND tenant_id = '%s'
				%s
                AND timestamp >= toDateTime64('%s', 3)
                AND timestamp < toDateTime64('%s', 3)
            GROUP BY %s %s
        )
        %s
    `,
		selectClause,
		windowClause,
		params.PropertyName,
		params.EventName,
		types.GetTenantID(ctx),
		customerFilter,
		formatClickHouseDateTime(params.StartTime),
		formatClickHouseDateTime(params.EndTime),
		getDeduplicationKey(),
		windowGroupBy,
		groupByClause)
}

func (a *SumAggregator) GetType() types.AggregationType {
	return types.AggregationSum
}

// CountAggregator implements count aggregation
type CountAggregator struct{}

func (a *CountAggregator) GetQuery(ctx context.Context, params *events.UsageParams) string {
	windowSize := formatWindowSize(params.WindowSize)
	selectClause := ""
	groupByClause := ""

	if windowSize != "" {
		selectClause = fmt.Sprintf("%s AS window_size,", windowSize)
		groupByClause = "GROUP BY window_size ORDER BY window_size"
	}

	customerFilter := ""
	if params.ExternalCustomerID != "" {
		customerFilter = fmt.Sprintf("AND external_customer_id = '%s'", params.ExternalCustomerID)
	}

	// 1. First buckets by time window if specified
	// 2. Counts distinct events within each window/bucket
	return fmt.Sprintf(`
        SELECT 
            %s count(DISTINCT %s) as total
        FROM events
        PREWHERE event_name = '%s'
			AND tenant_id = '%s'
            %s
            AND timestamp >= toDateTime64('%s', 3)
            AND timestamp < toDateTime64('%s', 3)
        %s
    `,
		selectClause,
		getDeduplicationKey(),
		params.EventName,
		types.GetTenantID(ctx),
		customerFilter,
		formatClickHouseDateTime(params.StartTime),
		formatClickHouseDateTime(params.EndTime),
		groupByClause)
}

func (a *CountAggregator) GetType() types.AggregationType {
	return types.AggregationCount
}
