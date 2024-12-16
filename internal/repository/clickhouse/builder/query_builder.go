package builder

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/types"
)

type QueryBuilder struct {
	baseQuery    string
	filterQuery  string
	matchedQuery string
	finalQuery   string
	args         map[string]interface{}
}

func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		args: make(map[string]interface{}),
	}
}

func (qb *QueryBuilder) WithBaseFilters(ctx context.Context, params *events.UsageParams) *QueryBuilder {
	conditions := []string{
		"event_name = :event_name",
	}

	tenantID := types.GetTenantID(ctx)

	qb.args["event_name"] = params.EventName
	if tenantID != "" {
		conditions = append(conditions, "tenant_id = :tenant_id")
		qb.args["tenant_id"] = tenantID
	}

	if !params.StartTime.IsZero() {
		conditions = append(conditions, "timestamp >= :start_time")
		qb.args["start_time"] = params.StartTime.Format(time.RFC3339)
	}
	if !params.EndTime.IsZero() {
		conditions = append(conditions, "timestamp < :end_time")
		qb.args["end_time"] = params.EndTime.Format(time.RFC3339)
	}
	if params.ExternalCustomerID != "" {
		conditions = append(conditions, "external_customer_id = :external_customer_id")
		qb.args["external_customer_id"] = params.ExternalCustomerID
	}
	if params.CustomerID != "" {
		conditions = append(conditions, "customer_id = :customer_id")
		qb.args["customer_id"] = params.CustomerID
	}

	qb.baseQuery = fmt.Sprintf("WITH base_events AS (SELECT id, timestamp, properties FROM events WHERE %s)",
		strings.Join(conditions, " AND "))

	return qb
}

func (qb *QueryBuilder) WithFilterGroups(ctx context.Context, groups []events.FilterGroup) *QueryBuilder {
	if len(groups) == 0 {
		return qb
	}

	var filterConditions []string
	for _, group := range groups {
		var conditions []string
		for property, values := range group.Filters {
			quotedValues := make([]string, len(values))
			for i, v := range values {
				quotedValues[i] = fmt.Sprintf("'%s'", v)
			}
			conditions = append(conditions, fmt.Sprintf(
				"JSONExtractString(properties, '%s') IN (%s)",
				property,
				strings.Join(quotedValues, ","),
			))
		}
		filterConditions = append(filterConditions, fmt.Sprintf(
			"('%s', %d, %s)",
			group.ID,
			group.Priority,
			strings.Join(conditions, " AND "),
		))
	}

	qb.filterQuery = fmt.Sprintf(`WITH filter_matches AS (
		SELECT *,
		ARRAY[
			%s
		] as group_matches
		FROM base_events
	)`, strings.Join(filterConditions, ",\n			"))

	qb.matchedQuery = `WITH matched_events AS (
		SELECT *,
		(SELECT group_id 
		 FROM arrayJoin(group_matches) AS g 
		 WHERE g.3 = 1 
		 ORDER BY g.2 DESC, group_id 
		 LIMIT 1) as best_match_group
		FROM filter_matches
	)`

	return qb
}

func (qb *QueryBuilder) WithAggregation(ctx context.Context, aggType types.AggregationType, propertyName string) *QueryBuilder {
	var aggClause string
	switch aggType {
	case types.AggregationCount:
		aggClause = "COUNT(*)"
	case types.AggregationSum:
		aggClause = fmt.Sprintf("SUM(CAST(JSONExtractString(properties, '%s') AS Float64))", propertyName)
	case types.AggregationAvg:
		aggClause = fmt.Sprintf("AVG(CAST(JSONExtractString(properties, '%s') AS Float64))", propertyName)
	}

	qb.finalQuery = fmt.Sprintf("SELECT best_match_group as filter_group_id, %s as value FROM matched_events GROUP BY best_match_group ORDER BY best_match_group", aggClause)

	return qb
}

func (qb *QueryBuilder) Build() (string, map[string]interface{}) {
	var parts []string

	// Add base query without WITH
	if qb.baseQuery != "" {
		parts = append(parts, strings.TrimPrefix(qb.baseQuery, "WITH "))
	}

	// Add filter query without WITH
	if qb.filterQuery != "" {
		parts = append(parts, strings.TrimPrefix(qb.filterQuery, "WITH "))
	}

	// Add matched query without WITH
	if qb.matchedQuery != "" {
		parts = append(parts, strings.TrimPrefix(qb.matchedQuery, "WITH "))
	}

	// Add final query (no WITH prefix needed)
	if qb.finalQuery != "" {
		parts = append(parts, qb.finalQuery)
	}

	// Join all parts with single WITH at the start
	query := fmt.Sprintf("WITH %s", strings.Join(parts, ",\n"))

	return query, qb.args
}
