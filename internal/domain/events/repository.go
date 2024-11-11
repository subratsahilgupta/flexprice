package events

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/types"
)

type Repository interface {
	InsertEvent(ctx context.Context, event *Event) error
	GetUsage(ctx context.Context, params *UsageParams) (*AggregationResult, error)
}

type UsageParams struct {
	ExternalCustomerID string
	EventName          string
	PropertyName       string
	AggregationType    types.AggregationType
	StartTime          time.Time
	EndTime            time.Time
}

type AggregationResult struct {
	Value     interface{}           `json:"value"`
	EventName string                `json:"event_name"`
	Type      types.AggregationType `json:"type"`
}
