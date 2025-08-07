package clickhouse

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

func GetAggregator(aggregationType types.AggregationType) events.Aggregator {
	switch aggregationType {
	case types.AggregationCount:
		return &CountAggregator{}
	case types.AggregationSum:
		return &SumAggregator{}
	case types.AggregationAvg:
		return &AvgAggregator{}
	case types.AggregationCountUnique:
		return &CountUniqueAggregator{}
	case types.AggregationLatest:
		return &LatestAggregator{}
	case types.AggregationSumWithMultiplier:
		return &SumWithMultiAggregator{}
	case types.AggregationMax:
		return &MaxAggregator{}
	}
	return nil
}

func getDeduplicationKey() string {
	return "id"
}

func formatClickHouseDateTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.000")
}

func formatWindowSize(windowSize types.WindowSize) string {
	switch windowSize {
	case types.WindowSizeMinute:
		return "toStartOfMinute(timestamp)"
	case types.WindowSizeHour:
		return "toStartOfHour(timestamp)"
	case types.WindowSizeDay:
		return "toStartOfDay(timestamp)"
	case types.WindowSizeWeek:
		return "toStartOfWeek(timestamp)"
	case types.WindowSize15Min:
		return "toStartOfInterval(timestamp, INTERVAL 15 MINUTE)"
	case types.WindowSize30Min:
		return "toStartOfInterval(timestamp, INTERVAL 30 MINUTE)"
	case types.WindowSize3Hour:
		return "toStartOfInterval(timestamp, INTERVAL 3 HOUR)"
	case types.WindowSize6Hour:
		return "toStartOfInterval(timestamp, INTERVAL 6 HOUR)"
	case types.WindowSize12Hour:
		return "toStartOfInterval(timestamp, INTERVAL 12 HOUR)"
	default:
		return ""
	}
}

func buildFilterConditions(filters map[string][]string) string {
	if len(filters) == 0 {
		return ""
	}

	var conditions []string
	for key, values := range filters {
		if len(values) == 0 {
			continue
		}

		quotedValues := make([]string, len(values))
		for i, v := range values {
			quotedValues[i] = fmt.Sprintf("'%s'", v)
		}

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

func buildTimeConditions(params *events.UsageParams) string {
	conditions := parseTimeConditions(params)

	if len(conditions) == 0 {
		return ""
	}

	return "AND " + strings.Join(conditions, " AND ")
}

func parseTimeConditions(params *events.UsageParams) []string {
	var conditions []string

	if !params.StartTime.IsZero() {
		conditions = append(conditions,
			fmt.Sprintf("timestamp >= toDateTime64('%s', 3)",
				formatClickHouseDateTime(params.StartTime)))
	}

	if !params.EndTime.IsZero() {
		conditions = append(conditions,
			fmt.Sprintf("timestamp < toDateTime64('%s', 3)",
				formatClickHouseDateTime(params.EndTime)))
	}

	return conditions
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

	externalCustomerFilter := ""
	if params.ExternalCustomerID != "" {
		externalCustomerFilter = fmt.Sprintf("AND external_customer_id = '%s'", params.ExternalCustomerID)
	}

	customerFilter := ""
	if params.CustomerID != "" {
		customerFilter = fmt.Sprintf("AND customer_id = '%s'", params.CustomerID)
	}

	filterConditions := buildFilterConditions(params.Filters)
	timeConditions := buildTimeConditions(params)

	return fmt.Sprintf(`
        SELECT 
            %s sum(value) as total
        FROM (
            SELECT
                %s anyLast(JSONExtractFloat(assumeNotNull(properties), '%s')) as value
            FROM events
            PREWHERE tenant_id = '%s'
				AND environment_id = '%s'
				AND event_name = '%s'
				%s
				%s
                %s
                %s
            GROUP BY %s %s
        )
        %s
    `,
		selectClause,
		windowClause,
		params.PropertyName,
		types.GetTenantID(ctx),
		types.GetEnvironmentID(ctx),
		params.EventName,
		externalCustomerFilter,
		customerFilter,
		filterConditions,
		timeConditions,
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

	externalCustomerFilter := ""
	if params.ExternalCustomerID != "" {
		externalCustomerFilter = fmt.Sprintf("AND external_customer_id = '%s'", params.ExternalCustomerID)
	}

	customerFilter := ""
	if params.CustomerID != "" {
		customerFilter = fmt.Sprintf("AND customer_id = '%s'", params.CustomerID)
	}

	filterConditions := buildFilterConditions(params.Filters)
	timeConditions := buildTimeConditions(params)

	return fmt.Sprintf(`
        SELECT 
            %s count(DISTINCT %s) as total
        FROM events
        PREWHERE tenant_id = '%s'
			AND environment_id = '%s'
			AND event_name = '%s'
			%s
			%s
            %s
            %s
        %s
    `,
		selectClause,
		getDeduplicationKey(),
		types.GetTenantID(ctx),
		types.GetEnvironmentID(ctx),
		params.EventName,
		externalCustomerFilter,
		customerFilter,
		filterConditions,
		timeConditions,
		groupByClause)
}

func (a *CountAggregator) GetType() types.AggregationType {
	return types.AggregationCount
}

// CountUniqueAggregator implements count unique aggregation
type CountUniqueAggregator struct{}

func (a *CountUniqueAggregator) GetQuery(ctx context.Context, params *events.UsageParams) string {
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

	externalCustomerFilter := ""
	if params.ExternalCustomerID != "" {
		externalCustomerFilter = fmt.Sprintf("AND external_customer_id = '%s'", params.ExternalCustomerID)
	}

	customerFilter := ""
	if params.CustomerID != "" {
		customerFilter = fmt.Sprintf("AND customer_id = '%s'", params.CustomerID)
	}

	filterConditions := buildFilterConditions(params.Filters)
	timeConditions := buildTimeConditions(params)

	return fmt.Sprintf(`
        SELECT 
            %s count(DISTINCT property_value) as total
        FROM (
            SELECT
                %s JSONExtractString(assumeNotNull(properties), '%s') as property_value
            FROM events
            PREWHERE tenant_id = '%s'
				AND environment_id = '%s'
				AND event_name = '%s'
				%s
				%s
                %s
                %s
            GROUP BY %s, property_value %s
        )
        %s
    `,
		selectClause,
		windowClause,
		params.PropertyName,
		types.GetTenantID(ctx),
		types.GetEnvironmentID(ctx),
		params.EventName,
		externalCustomerFilter,
		customerFilter,
		filterConditions,
		timeConditions,
		getDeduplicationKey(),
		windowGroupBy,
		groupByClause)
}

func (a *CountUniqueAggregator) GetType() types.AggregationType {
	return types.AggregationCountUnique
}

// AvgAggregator implements avg aggregation
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

	externalCustomerFilter := ""
	if params.ExternalCustomerID != "" {
		externalCustomerFilter = fmt.Sprintf("AND external_customer_id = '%s'", params.ExternalCustomerID)
	}

	customerFilter := ""
	if params.CustomerID != "" {
		customerFilter = fmt.Sprintf("AND customer_id = '%s'", params.CustomerID)
	}

	filterConditions := buildFilterConditions(params.Filters)
	timeConditions := buildTimeConditions(params)

	return fmt.Sprintf(`
        SELECT 
            %s avg(value) as total
        FROM (
            SELECT
                %s anyLast(JSONExtractFloat(assumeNotNull(properties), '%s')) as value
            FROM events
            PREWHERE tenant_id = '%s'
				AND environment_id = '%s'
				AND event_name = '%s' 
				%s
				%s
				%s
                %s
            GROUP BY %s %s
        )
        %s
    `,
		selectClause,
		windowClause,
		params.PropertyName,
		types.GetTenantID(ctx),
		types.GetEnvironmentID(ctx),
		params.EventName,
		externalCustomerFilter,
		customerFilter,
		filterConditions,
		timeConditions,
		getDeduplicationKey(),
		windowGroupBy,
		groupByClause)
}

func (a *AvgAggregator) GetType() types.AggregationType {
	return types.AggregationAvg
}

// LatestAggregator implements latest value aggregation
type LatestAggregator struct{}

func (a *LatestAggregator) GetQuery(ctx context.Context, params *events.UsageParams) string {
	windowSize := formatWindowSize(params.WindowSize)
	windowClause := ""
	groupByClause := ""

	if windowSize != "" {
		windowClause = fmt.Sprintf("%s AS window_size,", windowSize)
		groupByClause = "GROUP BY window_size ORDER BY window_size"
	}

	externalCustomerFilter := ""
	if params.ExternalCustomerID != "" {
		externalCustomerFilter = fmt.Sprintf("AND external_customer_id = '%s'", params.ExternalCustomerID)
	}

	customerFilter := ""
	if params.CustomerID != "" {
		customerFilter = fmt.Sprintf("AND customer_id = '%s'", params.CustomerID)
	}

	filterConditions := buildFilterConditions(params.Filters)
	timeConditions := buildTimeConditions(params)

	return fmt.Sprintf(`
        SELECT 
            %s argMax(JSONExtractFloat(assumeNotNull(properties), '%s'), timestamp) as total
        FROM 
			events	PREWHERE tenant_id = '%s'
                AND environment_id = '%s'
                AND event_name = '%s'
                %s
                %s
                %s
                %s
        %s
    `,
		windowClause,
		params.PropertyName,
		types.GetTenantID(ctx),
		types.GetEnvironmentID(ctx),
		params.EventName,
		externalCustomerFilter,
		customerFilter,
		filterConditions,
		timeConditions,
		groupByClause)
}

func (a *LatestAggregator) GetType() types.AggregationType {
	return types.AggregationLatest
}

// SumWithMultiAggregator implements sum with multiplier aggregation
type SumWithMultiAggregator struct{}

func (a *SumWithMultiAggregator) GetQuery(ctx context.Context, params *events.UsageParams) string {
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

	externalCustomerFilter := ""
	if params.ExternalCustomerID != "" {
		externalCustomerFilter = fmt.Sprintf("AND external_customer_id = '%s'", params.ExternalCustomerID)
	}

	customerFilter := ""
	if params.CustomerID != "" {
		customerFilter = fmt.Sprintf("AND customer_id = '%s'", params.CustomerID)
	}

	filterConditions := buildFilterConditions(params.Filters)
	timeConditions := buildTimeConditions(params)

	multiplier := decimal.NewFromInt(1)
	if params.Multiplier != nil {
		multiplier = *params.Multiplier
	}

	return fmt.Sprintf(`
        SELECT 
            %s (sum(value) * %f) as total
        FROM (
            SELECT
                %s anyLast(JSONExtractFloat(assumeNotNull(properties), '%s')) as value
            FROM events
            PREWHERE tenant_id = '%s'
				AND environment_id = '%s'
				AND event_name = '%s'
				%s
				%s
                %s
                %s
            GROUP BY %s %s
        )
        %s
    `,
		selectClause,
		multiplier.InexactFloat64(),
		windowClause,
		params.PropertyName,
		types.GetTenantID(ctx),
		types.GetEnvironmentID(ctx),
		params.EventName,
		externalCustomerFilter,
		customerFilter,
		filterConditions,
		timeConditions,
		getDeduplicationKey(),
		windowGroupBy,
		groupByClause)
}

func (a *SumWithMultiAggregator) GetType() types.AggregationType {
	return types.AggregationSumWithMultiplier
}

// MaxAggregator implements max aggregation
type MaxAggregator struct{}

func (a *MaxAggregator) GetQuery(ctx context.Context, params *events.UsageParams) string {
	// If bucket_size is specified, use windowed aggregation
	if params.BucketSize != "" {
		return a.getWindowedQuery(ctx, params)
	}
	// Otherwise use simple MAX aggregation
	return a.getNonWindowedQuery(ctx, params)
}

func (a *MaxAggregator) getNonWindowedQuery(ctx context.Context, params *events.UsageParams) string {
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

	externalCustomerFilter := ""
	if params.ExternalCustomerID != "" {
		externalCustomerFilter = fmt.Sprintf("AND external_customer_id = '%s'", params.ExternalCustomerID)
	}

	customerFilter := ""
	if params.CustomerID != "" {
		customerFilter = fmt.Sprintf("AND customer_id = '%s'", params.CustomerID)
	}

	filterConditions := buildFilterConditions(params.Filters)
	timeConditions := buildTimeConditions(params)

	return fmt.Sprintf(`
		SELECT 
			%s max(value) as total
		FROM (
			SELECT
				%s anyLast(JSONExtractFloat(assumeNotNull(properties), '%s')) as value
			FROM events
			PREWHERE tenant_id = '%s'
				AND environment_id = '%s'
				AND event_name = '%s'
				%s
				%s
				%s
				%s
			GROUP BY %s %s
		)
		%s
	`,
		selectClause,
		windowClause,
		params.PropertyName,
		types.GetTenantID(ctx),
		types.GetEnvironmentID(ctx),
		params.EventName,
		externalCustomerFilter,
		customerFilter,
		filterConditions,
		timeConditions,
		getDeduplicationKey(),
		windowGroupBy,
		groupByClause)
}

func (a *MaxAggregator) getWindowedQuery(ctx context.Context, params *events.UsageParams) string {
	bucketWindow := fmt.Sprintf("toStartOfInterval(timestamp, INTERVAL 1 %s)", strings.ToUpper(string(params.BucketSize)))

	externalCustomerFilter := ""
	if params.ExternalCustomerID != "" {
		externalCustomerFilter = fmt.Sprintf("AND external_customer_id = '%s'", params.ExternalCustomerID)
	}

	customerFilter := ""
	if params.CustomerID != "" {
		customerFilter = fmt.Sprintf("AND customer_id = '%s'", params.CustomerID)
	}

	filterConditions := buildFilterConditions(params.Filters)
	timeConditions := buildTimeConditions(params)

	// First get max values per bucket, then get the max across all buckets
	return fmt.Sprintf(`
		WITH bucket_maxes AS (
			SELECT
				%s as bucket_start,
				max(JSONExtractFloat(assumeNotNull(properties), '%s')) as bucket_max
			FROM events
			PREWHERE tenant_id = '%s'
				AND environment_id = '%s'
				AND event_name = '%s'
				%s
				%s
				%s
				%s
			GROUP BY bucket_start
			ORDER BY bucket_start
		)
		SELECT
			(SELECT sum(bucket_max) FROM bucket_maxes) as total,
			bucket_start as timestamp,
			bucket_max as value
		FROM bucket_maxes
		ORDER BY bucket_start
	`,
		bucketWindow,
		params.PropertyName,
		types.GetTenantID(ctx),
		types.GetEnvironmentID(ctx),
		params.EventName,
		externalCustomerFilter,
		customerFilter,
		filterConditions,
		timeConditions)
}

func (a *MaxAggregator) GetType() types.AggregationType {
	return types.AggregationMax
}

// // buildFilterGroupsQuery builds a query that matches events to the most specific filter group
// func buildFilterGroupsQuery(params *events.UsageWithFiltersParams) string {
//     var queryBuilder strings.Builder

//     // Base query with CTE for filter matches
//     queryBuilder.WriteString(fmt.Sprintf(`
//         WITH
//         base_events AS (
//             SELECT
//                 %s,
//                 timestamp,
//                 properties
//             FROM events
//             WHERE event_name = $1
//             %s
//         ),
//         filter_matches AS (
//             SELECT
//                 *,
//                 -- Calculate matches for each filter group
//                 ARRAY[
//     `, getDeduplicationKey(), buildTimeAndCustomerConditions(params.UsageParams)))

//     // Generate array of filter group matches
//     for i, group := range params.FilterGroups {
//         if i > 0 {
//             queryBuilder.WriteString(",")
//         }

//         // Default group case (no filters)
//         if len(group.Filters) == 0 {
//             queryBuilder.WriteString(fmt.Sprintf(`
//                 (%d, %d, 1)  -- group_id, priority, always matches
//             `, group.ID, group.Priority))
//             continue
//         }

//         // Build filter conditions
//         var conditions []string
//         for property, values := range group.Filters {
//             quotedValues := make([]string, len(values))
//             for i, v := range values {
//                 quotedValues[i] = fmt.Sprintf("'%s'", v)
//             }
//             conditions = append(conditions, fmt.Sprintf(
//                 "JSONExtractString(properties, '%s') IN (%s)",
//                 property,
//                 strings.Join(quotedValues, ","),
//             ))
//         }

//         queryBuilder.WriteString(fmt.Sprintf(`
//             (%d, %d, %s)  -- group_id, priority, filter conditions
//         `, group.ID, group.Priority, strings.Join(conditions, " AND ")))
//     }

//     // Complete filter_matches CTE and add matched_events CTE
//     queryBuilder.WriteString(`
//         ] as group_matches
//         ),
//         matched_events AS (
//             SELECT
//                 *,
//                 -- Select group with highest priority that matches
//                 (
//                     SELECT group_id
//                     FROM arrayJoin(group_matches) AS g
//                     WHERE g.3 = 1  -- where matches = true
//                     ORDER BY g.2 DESC, group_id  -- order by priority desc, then group_id
//                     LIMIT 1
//                 ) as best_match_group
//             FROM base_events
//         )
//     `)

//     // Add final aggregation based on params
//     queryBuilder.WriteString(`
//         SELECT
//             best_match_group as filter_group_id,
//     `)

//     // Add aggregation function
//     switch params.AggregationType {
//     case types.AggregationCount:
//         queryBuilder.WriteString(`
//             COUNT(*) as value
//         `)
//     case types.AggregationSum:
//         queryBuilder.WriteString(fmt.Sprintf(`
//             SUM(CAST(JSONExtractString(properties, '%s') AS Float64)) as value
//         `, params.PropertyName))
//     case types.AggregationAvg:
//         queryBuilder.WriteString(fmt.Sprintf(`
//             AVG(CAST(JSONExtractString(properties, '%s') AS Float64)) as value
//         `, params.PropertyName))
//     }

//     // Add grouping and ordering
//     queryBuilder.WriteString(`
//         FROM matched_events
//         GROUP BY best_match_group
//         ORDER BY best_match_group
//     `)

//     return queryBuilder.String()
// }

// // buildTimeAndCustomerConditions builds the WHERE clause for time and customer filters
// func buildTimeAndCustomerConditions(params *events.UsageParams) string {
//     var conditions []string

//     // Add time range conditions
//     if !params.StartTime.IsZero() {
//         conditions = append(conditions,
//             fmt.Sprintf("AND timestamp >= '%s'", params.StartTime.Format(time.RFC3339)))
//     }
//     if !params.EndTime.IsZero() {
//         conditions = append(conditions,
//             fmt.Sprintf("AND timestamp < '%s'", params.EndTime.Format(time.RFC3339)))
//     }

//     // Add customer conditions
//     if params.ExternalCustomerID != "" {
//         conditions = append(conditions,
//             fmt.Sprintf("AND external_customer_id = '%s'", params.ExternalCustomerID))
//     }
//     if params.CustomerID != "" {
//         conditions = append(conditions,
//             fmt.Sprintf("AND customer_id = '%s'", params.CustomerID))
//     }

//     return strings.Join(conditions, " ")
// }
