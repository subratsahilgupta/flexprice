package types

import (
	"encoding/json"
	"time"
)

// OnboardingEventsMessage represents a message for generating onboarding events
type OnboardingEventsMessage struct {
	CustomerID       string      `json:"customer_id"`
	CustomerExtID    string      `json:"customer_ext_id"`
	FeatureID        string      `json:"feature_id"`
	FeatureName      string      `json:"feature_name"`
	Duration         int         `json:"duration"`
	Meters           []MeterInfo `json:"meters"`
	TenantID         string      `json:"tenant_id"`
	RequestTimestamp time.Time   `json:"request_timestamp"`
}

// MeterInfo contains the essential meter information for event generation
type MeterInfo struct {
	ID          string          `json:"id"`
	EventName   string          `json:"event_name"`
	Aggregation AggregationInfo `json:"aggregation"`
	Filters     []FilterInfo    `json:"filters"`
}

// AggregationInfo contains aggregation configuration
type AggregationInfo struct {
	Type  AggregationType `json:"type"`
	Field string          `json:"field"`
}

// FilterInfo contains filter configuration
type FilterInfo struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
}

// Marshal converts the message to JSON
func (m *OnboardingEventsMessage) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// Unmarshal converts JSON to a message
func (m *OnboardingEventsMessage) Unmarshal(data []byte) error {
	return json.Unmarshal(data, m)
}
