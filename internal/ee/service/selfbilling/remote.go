package selfbilling

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/flexprice/flexprice/internal/config"
	ierr "github.com/flexprice/flexprice/internal/errors"
	flexprice "github.com/flexprice/go-sdk/v2"
	sdkerrors "github.com/flexprice/go-sdk/v2/models/errors"
	sdktypes "github.com/flexprice/go-sdk/v2/models/types"
	"github.com/samber/lo"
)

type remoteClient struct {
	sdk    *flexprice.Flexprice
	planID string
}

func newRemoteClient(cfg config.BillingConfig) *remoteClient {
	return &remoteClient{
		sdk:    flexprice.New(flexprice.WithServerURL(cfg.APIHost), flexprice.WithSecurity(cfg.APIKey), flexprice.WithTimeout(15*time.Second)),
		planID: cfg.PlanID,
	}
}

// EnsureCustomer finds or creates the customer; a legacy bare-tenant-id customer is re-keyed, never duplicated.
func (c *remoteClient) EnsureCustomer(ctx context.Context, in customerInput) error {
	_, err := c.sdk.Customers.GetCustomerByExternalID(ctx, in.extID)
	if !isNotFound(err) {
		return err
	}
	if in.legacyExtID != "" {
		_, err := c.sdk.Customers.GetCustomerByExternalID(ctx, in.legacyExtID)
		if err == nil {
			_, err = c.sdk.Customers.UpdateCustomer(ctx, sdktypes.UpdateCustomerRequest{ExternalID: lo.ToPtr(in.extID)}, nil, lo.ToPtr(in.legacyExtID))
			return err
		}
		if !isNotFound(err) {
			return err
		}
	}
	_, err = c.sdk.Customers.CreateCustomer(ctx, sdktypes.CreateCustomerRequest{
		ExternalID: in.extID,
		Name:       in.name,
		Email:      lo.EmptyableToPtr(in.email),
		Metadata:   map[string]string{"tenant_id": in.tenantID},
	})
	return err
}

// StartTrial subscribes the customer to the self-serve plan; the plan's prices carry the trial days.
func (c *remoteClient) StartTrial(ctx context.Context, extID string) error {
	resp, err := c.sdk.Plans.GetPlan(ctx, c.planID)
	if err != nil {
		return err
	}
	prices := resp.PlanResponse.Prices
	if len(prices) == 0 {
		return ierr.NewError("self-serve plan has no prices").Mark(ierr.ErrInvalidOperation)
	}
	fixed, ok := lo.Find(prices, func(p sdktypes.PriceResponse) bool {
		return lo.FromPtr(p.Type) == sdktypes.PriceTypeFixed
	})
	if !ok {
		fixed = prices[0]
	}
	_, err = c.sdk.Subscriptions.CreateSubscription(ctx, sdktypes.CreateSubscriptionRequest{
		ExternalCustomerID: lo.ToPtr(extID),
		PlanID:             c.planID,
		Currency:           lo.FromPtr(fixed.Currency),
		BillingPeriod:      lo.FromPtr(fixed.BillingPeriod),
	})
	return err
}

func (c *remoteClient) LatestSubscription(ctx context.Context, extID string) (*remoteSubscription, error) {
	resp, err := c.sdk.Customers.GetSubscriptionsForCustomer(ctx, extID, nil)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
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

func toRemoteSubscription(s *sdktypes.SubscriptionResponse) *remoteSubscription {
	return &remoteSubscription{id: lo.FromPtr(s.ID), status: string(lo.FromPtr(s.SubscriptionStatus)), trialEnd: s.TrialEnd}
}

// isNotFound reports whether an SDK call failed with HTTP 404, whichever error type the SDK used for it.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *sdkerrors.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound
	}
	var errResp *sdkerrors.ErrorResponse
	if errors.As(err, &errResp) {
		return errResp.HTTPMeta.Response != nil && errResp.HTTPMeta.Response.StatusCode == http.StatusNotFound
	}
	var validationErr *sdkerrors.ResponseValidationError
	if errors.As(err, &validationErr) {
		return validationErr.StatusCode == http.StatusNotFound
	}
	return false
}
