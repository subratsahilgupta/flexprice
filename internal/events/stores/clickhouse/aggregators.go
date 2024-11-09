package clickhouse

import (
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/events"
)

func GetAggregator(aggregationType events.AggregationType) events.Aggregator {
	switch aggregationType {
	case events.CountAggregation:
		return &CountAggregator{}
	case events.SumAggregation:
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

func (a *SumAggregator) GetQuery(eventName, propertyName, externalCustomerID string, startTime, endTime time.Time) string {
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
	`, propertyName, eventName, externalCustomerID,
		formatClickHouseDateTime(startTime),
		formatClickHouseDateTime(endTime))
}

func (a *SumAggregator) GetType() events.AggregationType {
	return events.SumAggregation
}

// CountAggregator implements count aggregation
type CountAggregator struct{}

func (a *CountAggregator) GetQuery(eventName, _, externalCustomerID string, startTime, endTime time.Time) string {
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
	`, eventName, externalCustomerID,
		formatClickHouseDateTime(startTime),
		formatClickHouseDateTime(endTime))
}

func (a *CountAggregator) GetType() events.AggregationType {
	return events.CountAggregation
}
