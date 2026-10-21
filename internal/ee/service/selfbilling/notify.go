package selfbilling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/flexprice/flexprice/internal/httpclient"
	"github.com/flexprice/flexprice/internal/types"
)

// alertTimeout bounds the detached delivery of one Zapier alert.
const alertTimeout = 30 * time.Second

type alertEvent string

const (
	alertTrialStarted    alertEvent = "trial_started"
	alertTenantActivated alertEvent = "tenant_activated"
	alertTenantSuspended alertEvent = "tenant_suspended"
)

// alert is the JSON body posted to the Zapier catch hook.
type alert struct {
	Event      alertEvent                 `json:"event"`
	TenantID   string                     `json:"tenant_id"`
	TenantName string                     `json:"tenant_name"`
	Region     string                     `json:"region"`
	FromStatus types.TenantInternalStatus `json:"from_status,omitempty"`
	ToStatus   types.TenantInternalStatus `json:"to_status,omitempty"`
	OccurredAt time.Time                  `json:"occurred_at"`
}

// notify posts the alert to the Zapier webhook in the background; delivery failures are only logged.
func (s *selfBillingService) notify(ctx context.Context, a alert) {
	hookURL := s.Config.Billing.ZapierWebhookURL
	if hookURL == "" {
		return
	}

	a.Region = s.Config.Billing.Region
	a.OccurredAt = time.Now().UTC()

	detached := context.WithoutCancel(ctx)
	go func() {
		ctx, cancel := context.WithTimeout(detached, alertTimeout)
		defer cancel()

		defer func() {
			if r := recover(); r != nil {
				s.Logger.Error(ctx, "self-billing alert panicked", "error", fmt.Errorf("panic: %v", r), "event", a.Event, "tenant_id", a.TenantID)
			}
		}()

		body, err := json.Marshal(a)
		if err != nil {
			s.Logger.Error(ctx, "failed to marshal self-billing alert", "error", err, "event", a.Event, "tenant_id", a.TenantID)
			return
		}

		_, err = httpclient.NewDefaultClient().Send(ctx, &httpclient.Request{
			Method: http.MethodPost,
			URL:    hookURL,
			Body:   body,
		})
		if httpErr, ok := httpclient.IsHTTPError(err); ok {
			s.Logger.Error(ctx, "self-billing alert rejected by Zapier", "error", "non-2xx response", "status_code", httpErr.StatusCode, "event", a.Event, "tenant_id", a.TenantID)
			return
		}
		if err != nil {
			// A transport error embeds the request URL, which is the hook's secret, so only its cause is logged.
			var urlErr *url.Error
			if errors.As(err, &urlErr) {
				err = urlErr.Err
			}
			s.Logger.Error(ctx, "failed to send self-billing alert", "error", err, "event", a.Event, "tenant_id", a.TenantID)
		}
	}()
}
