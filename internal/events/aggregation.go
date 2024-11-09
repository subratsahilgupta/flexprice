package events

import "time"

// AggregationType represents different types of aggregations
type AggregationType string

const (
	CountAggregation       AggregationType = "COUNT"
	SumAggregation         AggregationType = "SUM"
	MaxAggregation         AggregationType = "MAX"
	UniqueCountAggregation AggregationType = "UNIQUE_COUNT"
	LatestAggregation      AggregationType = "LATEST"
)

// Aggregator defines how to aggregate events
type Aggregator interface {
	// GetQuery returns the ClickHouse query for this aggregation
	GetQuery(eventName string, propertyName string, externalCustomerID string, startTime time.Time, endTime time.Time) string

	// GetType returns the aggregation type
	GetType() AggregationType
}

// AggregationResult represents the result of an aggregation
type AggregationResult struct {
	Value     interface{}     `json:"value"`
	EventName string          `json:"event_name"`
	Type      AggregationType `json:"type"`
}
