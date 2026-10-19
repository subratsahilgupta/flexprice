package selfbilling

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/config"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	flexprice "github.com/flexprice/go-sdk/v2"
	sdkerrors "github.com/flexprice/go-sdk/v2/models/errors"
	sdktypes "github.com/flexprice/go-sdk/v2/models/types"
	"github.com/samber/lo"
)

const (
	remoteTimeout = 15 * time.Second

	// trialCurrency and trialBillingPeriod are the self-serve plan's defaults on Flexprice Cloud.
	trialCurrency      = "usd"
	trialBillingPeriod = sdktypes.BillingPeriodMonthly
)

type remoteClient struct {
	sdk    *flexprice.Flexprice
	planID string
}

func newRemoteClient(cfg config.BillingConfig) *remoteClient {
	sdk := flexprice.New(
		flexprice.WithServerURL(cfg.APIHost),
		flexprice.WithSecurity(cfg.APIKey),
		flexprice.WithTimeout(remoteTimeout),
	)

	return &remoteClient{
		sdk:    sdk,
		planID: cfg.PlanID,
	}
}

// EnsureCustomer finds or creates the customer; a legacy bare-tenant-id customer is re-keyed, never duplicated.
func (c *remoteClient) EnsureCustomer(ctx context.Context, in customerInput) error {
	_, err := c.sdk.Customers.GetCustomerByExternalID(ctx, in.extID)
	if !isNotFound(err) {
		return cloudErr(err)
	}

	if in.legacyExtID != "" {
		_, err := c.sdk.Customers.GetCustomerByExternalID(ctx, in.legacyExtID)
		if err == nil {
			rekey := sdktypes.UpdateCustomerRequest{ExternalID: lo.ToPtr(in.extID)}
			_, err = c.sdk.Customers.UpdateCustomer(ctx, rekey, nil, lo.ToPtr(in.legacyExtID))
			return cloudErr(err)
		}
		if !isNotFound(err) {
			return cloudErr(err)
		}
	}

	_, err = c.sdk.Customers.CreateCustomer(ctx, sdktypes.CreateCustomerRequest{
		ExternalID: in.extID,
		Name:       in.name,
		Email:      lo.EmptyableToPtr(in.email),
		Metadata:   map[string]string{"tenant_id": in.tenantID},
	})
	return cloudErr(err)
}

// StartTrial re-checks the subscription right before creating it, since CreateSubscription takes no idempotency key.
func (c *remoteClient) StartTrial(ctx context.Context, extID string) error {
	sub, err := c.LatestSubscription(ctx, extID)
	if err != nil || sub != nil {
		return err
	}

	_, err = c.sdk.Subscriptions.CreateSubscription(ctx, sdktypes.CreateSubscriptionRequest{
		ExternalCustomerID: lo.ToPtr(extID),
		PlanID:             c.planID,
		Currency:           trialCurrency,
		BillingPeriod:      trialBillingPeriod,
	})
	return cloudErr(err)
}

func (c *remoteClient) LatestSubscription(ctx context.Context, extID string) (*remoteSubscription, error) {
	resp, err := c.sdk.Customers.GetSubscriptionsForCustomer(ctx, extID, nil)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, cloudErr(err)
	}

	var live, cancelled *sdktypes.SubscriptionResponse
	newer := func(s, than *sdktypes.SubscriptionResponse) bool {
		return than == nil || lo.FromPtr(s.CreatedAt).After(lo.FromPtr(than.CreatedAt))
	}

	for i := range resp.ListSubscriptionsResponse.Items {
		s := &resp.ListSubscriptionsResponse.Items[i]
		if lo.FromPtr(s.PlanID) != c.planID {
			continue
		}

		if lo.FromPtr(s.SubscriptionStatus) == sdktypes.SubscriptionStatusCancelled {
			if newer(s, cancelled) {
				cancelled = s
			}
			continue
		}

		if newer(s, live) {
			live = s
		}
	}

	latest := lo.Ternary(live != nil, live, cancelled)
	if latest == nil {
		return nil, nil
	}
	return toRemoteSubscription(latest), nil
}

// toRemoteSubscription maps the SDK subscription onto local types; Flexprice Cloud shares our status values.
func toRemoteSubscription(s *sdktypes.SubscriptionResponse) *remoteSubscription {
	return &remoteSubscription{
		id:     lo.FromPtr(s.ID),
		status: types.SubscriptionStatus(lo.FromPtr(s.SubscriptionStatus)),
	}
}

// cloudErr classifies a Flexprice Cloud SDK failure; the SDK error stays in the chain.
func cloudErr(err error) error {
	if err == nil {
		return nil
	}

	status := httpStatus(err)
	hint := remoteMessage(err)

	var kind error
	switch {
	case status == http.StatusNotFound:
		kind = ierr.ErrNotFound
	case status == http.StatusConflict:
		kind = ierr.ErrAlreadyExists
	case status >= 400 && status < 500:
		kind = ierr.ErrValidation
	default:
		kind = ierr.ErrHTTPClient
		hint = "Flexprice Cloud is unavailable; try again shortly"
	}

	if hint == "" {
		hint = "Flexprice Cloud rejected the request"
	}

	return ierr.WithError(err).
		WithMessagef("flexprice cloud call failed with status %d", status).
		WithHint(hint).
		Mark(kind)
}

// remoteMessage returns the user-facing message in a Flexprice Cloud error body, if any.
func remoteMessage(err error) string {
	var errResp *sdkerrors.ErrorResponse
	if errors.As(err, &errResp) {
		return strings.TrimSpace(lo.FromPtr(errResp.Message))
	}

	var body string
	var apiErr *sdkerrors.APIError
	var validationErr *sdkerrors.ResponseValidationError
	switch {
	case errors.As(err, &apiErr):
		body = apiErr.Body
	case errors.As(err, &validationErr):
		body = validationErr.Body
	default:
		return ""
	}

	var parsed struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(body), &parsed) != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Message)
}

// isNotFound reports whether an SDK call failed with HTTP 404, whichever error type the SDK used for it.
func isNotFound(err error) bool {
	return httpStatus(err) == http.StatusNotFound
}

// httpStatus returns the HTTP status of a failed SDK call, or 0 when err did not come from an HTTP response.
func httpStatus(err error) int {
	if err == nil {
		return 0
	}

	var apiErr *sdkerrors.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode
	}

	var errResp *sdkerrors.ErrorResponse
	if errors.As(err, &errResp) {
		if errResp.HTTPMeta.Response == nil {
			return 0
		}
		return errResp.HTTPMeta.Response.StatusCode
	}

	var validationErr *sdkerrors.ResponseValidationError
	if errors.As(err, &validationErr) {
		return validationErr.StatusCode
	}
	return 0
}
