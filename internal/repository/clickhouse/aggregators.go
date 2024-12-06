package clickhouse

import (
	"context"
	"fmt"
	"strings"
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
	case types.AggregationAvg:
		return &AvgAggregator{}
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
func formatWindowSize(windowSize types.WindowSize) string {
	switch windowSize {
	case types.WindowSizeMinute:
		return "toStartOfMinute(timestamp)"
	case types.WindowSizeHour:
		return "toStartOfHour(timestamp)"
	case types.WindowSizeDay:
		return "toStartOfDay(timestamp)"
	default:
		return "" // No windowing if windowSize is empty or invalid
	}
}

// buildFilterConditions creates the WHERE clause for filters
func buildFilterConditions(filters map[string][]string) string {
	if len(filters) == 0 {
		return ""
	}

	var conditions []string
	for key, values := range filters {
		if len(values) == 0 {
			continue
		}

		// Create an array of quoted values
		quotedValues := make([]string, len(values))
		for i, v := range values {
			quotedValues[i] = fmt.Sprintf("'%s'", v)
		}

		// Use JSONExtractString for property comparison
		conditions = append(conditions, fmt.Sprintf(
			"JSONExtractString(properties, '%s') IN (%s)",
			key,
			strings.Join(quotedValues, ","),
		))
	}

	if len(conditions) == 0 {
		return ""
	}

	return "AND " + strings.Join(conditions, " AND ")
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

	filterConditions := buildFilterConditions(params.Filters)

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
		filterConditions,
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

	filterConditions := buildFilterConditions(params.Filters)

	return fmt.Sprintf(`
        SELECT 
            %s count(DISTINCT %s) as total
        FROM events
        PREWHERE event_name = '%s'
            AND tenant_id = '%s'
            %s
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
		filterConditions,
		formatClickHouseDateTime(params.StartTime),
		formatClickHouseDateTime(params.EndTime),
		groupByClause)
}

func (a *CountAggregator) GetType() types.AggregationType {
	return types.AggregationCount
}

type AvgAggregator struct{}

func (a *AvgAggregator) GetQuery(ctx context.Context, params *events.UsageParams) string {
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

	filterConditions := buildFilterConditions(params.Filters)

	return fmt.Sprintf(`
        SELECT 
            %s avg(value) as total
        FROM (
            SELECT
                %s anyLast(JSONExtractFloat(assumeNotNull(properties), '%s')) as value
            FROM events
            PREWHERE event_name = '%s' 
                AND tenant_id = '%s'
                %s
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
		filterConditions,
		formatClickHouseDateTime(params.StartTime),
		formatClickHouseDateTime(params.EndTime),
		getDeduplicationKey(),
		windowGroupBy,
		groupByClause)
}

func (a *AvgAggregator) GetType() types.AggregationType {
	return types.AggregationAvg
}
