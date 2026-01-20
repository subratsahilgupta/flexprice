package types

import ierr "github.com/flexprice/flexprice/internal/errors"

type FailurePointType string

const (
	FailurePointTypeCustomerLookup             FailurePointType = "customer_lookup"
	FailurePointTypeMeterLookup                FailurePointType = "meter_lookup"
	FailurePointTypePriceLookup                FailurePointType = "price_lookup"
	FailurePointTypeSubscriptionLineItemLookup FailurePointType = "subscription_line_item_lookup"
)

type FailurePoint struct {
	FailurePointType FailurePointType    `json:"failure_point_type"`
	Error            *ierr.ErrorResponse `json:"error,omitempty"`
}

type DebugTrackerStatus string

const (
	DebugTrackerStatusUnprocessed DebugTrackerStatus = "unprocessed"
	DebugTrackerStatusNotFound    DebugTrackerStatus = "not_found"
	DebugTrackerStatusFound       DebugTrackerStatus = "found"
	DebugTrackerStatusError       DebugTrackerStatus = "error"
)

type EventProcessingStatusType string

const (
	EventProcessingStatusTypeProcessed  EventProcessingStatusType = "processed"
	EventProcessingStatusTypeProcessing EventProcessingStatusType = "processing"
	EventProcessingStatusTypeFailed     EventProcessingStatusType = "failed"
)
