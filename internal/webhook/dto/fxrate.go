package webhookDto

import (
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/types"
)

// InternalFXRateEvent is the compact payload published to the system-events topic.
type InternalFXRateEvent struct {
	FXRateID string `json:"fx_rate_id"`
	TenantID string `json:"tenant_id"`
}

// FXRate is the outbound webhook representation of an FX rate.
type FXRate struct {
	ID            string            `json:"id"`
	EnvironmentID string            `json:"environment_id"`
	Scope         string            `json:"scope"`
	ScopeID       string            `json:"scope_id"`
	FromCurrency  string            `json:"from_currency"`
	ToCurrency    string            `json:"to_currency"`
	Rate          string            `json:"rate"`
	Source        string            `json:"source"`
	StartDate     *time.Time        `json:"start_date,omitempty"`
	EndDate       *time.Time        `json:"end_date,omitempty"`
	Status        string            `json:"status"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

func NewFXRate(resp *dto.FXRateResponse) *FXRate {
	if resp == nil || resp.FXRate == nil {
		return nil
	}
	return &FXRate{
		ID:            resp.ID,
		EnvironmentID: resp.EnvironmentID,
		Scope:         string(resp.Scope),
		ScopeID:       resp.ScopeID,
		FromCurrency:  resp.FromCurrency,
		ToCurrency:    resp.ToCurrency,
		Rate:          resp.Rate.String(),
		Source:        string(resp.Source),
		StartDate:     resp.StartDate,
		EndDate:       resp.EndDate,
		Status:        string(resp.Status),
		Metadata:      resp.Metadata,
	}
}

type FXRateWebhookPayload struct {
	EventType types.WebhookEventName `json:"event_type"`
	FXRate    *FXRate                `json:"fx_rate"`
}

func NewFXRateWebhookPayload(rate *dto.FXRateResponse, eventType types.WebhookEventName) *FXRateWebhookPayload {
	return &FXRateWebhookPayload{EventType: eventType, FXRate: NewFXRate(rate)}
}
