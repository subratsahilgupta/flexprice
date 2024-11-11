package aggregation

import (
	"time"

	"github.com/flexprice/flexprice/internal/types"
)

type Aggregator interface {
	// GetQuery returns the query for this aggregation
	GetQuery(eventName string, propertyName string, externalCustomerID string, startTime time.Time, endTime time.Time) string

	// GetType returns the aggregation type
	GetType() types.AggregationType
}
