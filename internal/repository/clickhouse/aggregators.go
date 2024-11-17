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

// helper function for ClickHouse datetime formatting
func formatClickHouseDateTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.000")
}

// SumAggregator implements sum aggregation
type SumAggregator struct{}

func (a *SumAggregator) GetQuery(ctx context.Context, params *events.UsageParams) string {
	return fmt.Sprintf(`
		SELECT sum(value)
		FROM (
			SELECT 
				id,
				JSONExtractFloat(assumeNotNull(properties), '%s') as value
			FROM events
			WHERE event_name = '%s'
				AND external_customer_id = '%s'
				AND timestamp >= toDateTime64('%s', 3)
				AND timestamp < toDateTime64('%s', 3)
			GROUP BY id, external_customer_id, timestamp, properties
		)
	`, params.PropertyName, params.EventName, params.ExternalCustomerID,
		formatClickHouseDateTime(params.StartTime),
		formatClickHouseDateTime(params.EndTime))
}

func (a *SumAggregator) GetType() types.AggregationType {
	return types.AggregationSum
}

// CountAggregator implements count aggregation
type CountAggregator struct{}

func (a *CountAggregator) GetQuery(ctx context.Context, params *events.UsageParams) string {
	return fmt.Sprintf(`
		SELECT count(*)
		FROM (
			SELECT id
			FROM events
			WHERE event_name = '%s'
				AND external_customer_id = '%s'
				AND timestamp >= toDateTime64('%s', 3)
				AND timestamp < toDateTime64('%s', 3)
			GROUP BY id, external_customer_id, timestamp
		)
	`, params.EventName, params.ExternalCustomerID,
		formatClickHouseDateTime(params.StartTime),
		formatClickHouseDateTime(params.EndTime))
}

func (a *CountAggregator) GetType() types.AggregationType {
	return types.AggregationCount
}
