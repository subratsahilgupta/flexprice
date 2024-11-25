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

	windowClause := ""
	groupByClause := ""

	if windowSize != "" {
		windowClause = fmt.Sprintf("%s AS window_size,", windowSize)
		groupByClause = "GROUP BY window_size"
	}

	return fmt.Sprintf(`
        SELECT %s SUM(value) AS total_value
        FROM (
            SELECT JSONExtractFloat(assumeNotNull(properties), '%s') AS value, timestamp
            FROM events
            WHERE event_name = '%s'
              AND external_customer_id = '%s'
              AND timestamp >= toDateTime64('%s', 3)
              AND timestamp < toDateTime64('%s', 3)
        )
        %s
    `, windowClause, params.PropertyName, params.EventName, params.ExternalCustomerID,
		formatClickHouseDateTime(params.StartTime),
		formatClickHouseDateTime(params.EndTime),
		groupByClause)
}

func (a *SumAggregator) GetType() types.AggregationType {
	return types.AggregationSum
}

// CountAggregator implements count aggregation
type CountAggregator struct{}

func (a *CountAggregator) GetQuery(ctx context.Context, params *events.UsageParams) string {
	windowSize := formatWindowSize(params.WindowSize)
	windowClause := ""
	groupByClause := ""

	if windowSize != "" {
		windowClause = fmt.Sprintf("%s AS window_size,", windowSize)
		groupByClause = "GROUP BY window_size"
	}

	return fmt.Sprintf(`
        SELECT %s COUNT(*) AS total_count
        FROM (
            SELECT id, timestamp
            FROM events
            WHERE event_name = '%s'
              AND external_customer_id = '%s'
              AND timestamp >= toDateTime64('%s', 3)
              AND timestamp < toDateTime64('%s', 3)
            GROUP BY id, external_customer_id, timestamp
        )
        %s
    `, windowClause, params.EventName, params.ExternalCustomerID,
		formatClickHouseDateTime(params.StartTime),
		formatClickHouseDateTime(params.EndTime),
		groupByClause)
}

func (a *CountAggregator) GetType() types.AggregationType {
	return types.AggregationCount
}
