package events

import (
	"time"

	"github.com/google/uuid"
)

// Event represents the base event structure
type Event struct {
	ID                 string                 `json:"id" ch:"id"`
	TenantID           string                 `json:"tenant_id" ch:"tenant_id"`
	ExternalCustomerID string                 `json:"external_customer_id" ch:"external_customer_id" validate:"required"`
	EventName          string                 `json:"event_name" ch:"event_name" validate:"required"`
	Timestamp          time.Time              `json:"timestamp" ch:"timestamp,timezone('UTC')"`
	Properties         map[string]interface{} `json:"properties" ch:"properties"`
}

// NewEvent creates a new event with defaults
func NewEvent(
	eventID, tenantID, externalCustomerID, eventName string,
	timestamp time.Time,
	properties map[string]interface{},
) *Event {
	if eventID == "" {
		eventID = uuid.New().String()
	}

	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	} else {
		timestamp = timestamp.UTC()
	}

	return &Event{
		ID:                 eventID,
		TenantID:           tenantID,
		ExternalCustomerID: externalCustomerID,
		EventName:          eventName,
		Timestamp:          timestamp,
		Properties:         properties,
	}
}
