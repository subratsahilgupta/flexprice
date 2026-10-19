package selfbilling

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/tenant"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/utils"
)

const (
	checkoutSuccessPath = "/billing/checkout/success"
	checkoutFailurePath = "/billing/checkout/failure"
)

// StartCheckout only checks out a trialing subscription; an active one activates and an incomplete one pays in the portal.
func (s *selfBillingService) StartCheckout(ctx context.Context, tenantID string, req dto.SelfBillingCheckoutRequest) (*dto.SelfBillingCheckoutResponse, error) {
	if !s.enabled() {
		return nil, errDisabled
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	t, err := s.TenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	extID := s.extID(tenantID)
	if err := s.provision(ctx, tenantID, t.Name, req.Email); err != nil {
		return nil, err
	}

	if err := s.client.UpdateBillingDetails(ctx, extID, toBillingDetails(req)); err != nil {
		return nil, err
	}

	sub, err := s.client.LatestSubscription(ctx, extID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, ierr.NewError("flexprice cloud subscription not found").
			WithHint("Your billing subscription could not be found; contact support").
			Mark(ierr.ErrNotFound)
	}

	switch sub.status {
	case types.SubscriptionStatusTrialing:
	case types.SubscriptionStatusActive:
		return s.completedCheckout(ctx, tenantID, "")
	case types.SubscriptionStatusIncomplete:
		return s.payInPortal(ctx, t)
	default:
		return nil, ierr.NewErrorf("flexprice cloud subscription %s is %s", sub.id, sub.status).
			WithHint("Your billing subscription cannot be paid for here; contact support").
			WithReportableDetails(map[string]any{
				"subscription_id": sub.id,
				"status":          sub.status,
			}).
			Mark(ierr.ErrInvalidOperation)
	}

	co, err := s.client.StartTrialEndCheckout(ctx, checkoutInput{
		extID:          extID,
		subscriptionID: sub.id,
		successURL:     s.Config.Billing.DashboardURL + checkoutSuccessPath,
		failureURL:     s.Config.Billing.DashboardURL + checkoutFailurePath,
	})
	if ierr.IsAlreadyExists(err) {
		return s.checkoutInProgress(ctx, t, sub.id, err)
	}
	if err != nil {
		return nil, err
	}

	// A $0 first period ends the trial with nothing to pay; activate once Flexprice Cloud reports it active.
	if co.status == types.CheckoutStatusCompleted {
		active, err := s.subscriptionActive(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		if active {
			return s.completedCheckout(ctx, tenantID, co.id)
		}
	}

	s.Logger.Info(ctx, "self-billing checkout started", "tenant_id", tenantID, "checkout_id", co.id, "subscription_id", sub.id)
	return toCheckoutResponse(co, t.InternalStatus), nil
}

// PollCheckout activates on a completed checkout only while the Flexprice Cloud subscription is active.
func (s *selfBillingService) PollCheckout(ctx context.Context, tenantID, checkoutID string) (*dto.SelfBillingCheckoutResponse, error) {
	if !s.enabled() {
		return nil, errDisabled
	}

	co, err := s.ownedCheckout(ctx, tenantID, checkoutID)
	if err != nil {
		return nil, err
	}

	if co.status == types.CheckoutStatusCompleted {
		active, err := s.subscriptionActive(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		if active {
			if err := s.activate(ctx, tenantID); err != nil {
				return nil, err
			}
		}
	}

	t, err := s.TenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	return toCheckoutResponse(co, t.InternalStatus), nil
}

func (s *selfBillingService) CancelCheckout(ctx context.Context, tenantID, checkoutID string) error {
	if !s.enabled() {
		return errDisabled
	}

	if _, err := s.ownedCheckout(ctx, tenantID, checkoutID); err != nil {
		return err
	}
	return s.client.CancelCheckout(ctx, checkoutID)
}

func (s *selfBillingService) completedCheckout(ctx context.Context, tenantID, checkoutID string) (*dto.SelfBillingCheckoutResponse, error) {
	if err := s.activate(ctx, tenantID); err != nil {
		return nil, err
	}

	return &dto.SelfBillingCheckoutResponse{
		CheckoutID: checkoutID,
		Status:     types.CheckoutStatusCompleted,
		Terminal:   true,
		Tenant:     types.TenantInternalStatusActive,
	}, nil
}

// payInPortal sends a tenant whose trial ended unpaid to the customer portal to pay the open invoice.
func (s *selfBillingService) payInPortal(ctx context.Context, t *tenant.Tenant) (*dto.SelfBillingCheckoutResponse, error) {
	p, err := s.client.PortalSession(ctx, s.extID(t.ID))
	if err != nil {
		return nil, err
	}

	return &dto.SelfBillingCheckoutResponse{
		URL:         p.url,
		Status:      types.CheckoutStatusPending,
		PayInPortal: true,
		Tenant:      t.InternalStatus,
	}, nil
}

// checkoutInProgress resumes the tenant's open trial-end checkout, else reports the conflict.
func (s *selfBillingService) checkoutInProgress(ctx context.Context, t *tenant.Tenant, subscriptionID string, cause error) (*dto.SelfBillingCheckoutResponse, error) {
	if id := conflictCheckoutID(cause); id != "" {
		co, err := s.ownedCheckout(ctx, t.ID, id)
		if err == nil && !co.terminal && co.url != "" {
			return toCheckoutResponse(co, t.InternalStatus), nil
		}
	}

	// A fresh error, so Flexprice Cloud's own 409 hint does not replace ours.
	return nil, ierr.NewErrorf("self-billing checkout already in progress: %v", cause).
		WithHint("a checkout is already in progress; finish or cancel it").
		WithReportableDetails(map[string]any{"subscription_id": subscriptionID}).
		Mark(ierr.ErrAlreadyExists)
}

// ownedCheckout loads a checkout session and hides one that belongs to another tenant.
func (s *selfBillingService) ownedCheckout(ctx context.Context, tenantID, checkoutID string) (*remoteCheckout, error) {
	co, err := s.client.GetCheckout(ctx, checkoutID)
	if err != nil {
		return nil, err
	}

	if co.extID != s.extID(tenantID) {
		return nil, ierr.NewError("checkout session not found").
			WithHint("Checkout session not found").
			Mark(ierr.ErrNotFound)
	}
	return co, nil
}

// subscriptionActive reports whether the tenant's Flexprice Cloud subscription is active, the source of truth for payment.
func (s *selfBillingService) subscriptionActive(ctx context.Context, tenantID string) (bool, error) {
	sub, err := s.client.LatestSubscription(ctx, s.extID(tenantID))
	if err != nil {
		return false, err
	}
	return sub != nil && sub.status == types.SubscriptionStatusActive, nil
}

// activate marks the tenant active, allows one production environment and creates it. Safe to repeat.
func (s *selfBillingService) activate(ctx context.Context, tenantID string) error {
	ctx = types.SetTenantID(ctx, tenantID)

	t, err := s.TenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return err
	}

	if t.InternalStatus != types.TenantInternalStatusActive {
		activated := tenant.NewBuilder(t).WithInternalStatus(types.TenantInternalStatusActive).Build()
		if err := s.TenantRepo.Update(ctx, activated); err != nil {
			return err
		}
		s.Logger.Info(ctx, "self-billing tenant activated", "tenant_id", tenantID, "from", t.InternalStatus)
	}

	if err := s.raiseProductionLimit(ctx); err != nil {
		return err
	}
	return s.ensureProductionEnv(ctx)
}

// raiseProductionLimit allows at least one production environment and keeps a higher limit.
func (s *selfBillingService) raiseProductionLimit(ctx context.Context) error {
	setting, err := s.settings.GetSettingByKeyUnchecked(ctx, types.SettingKeyTenantConfig)
	if err != nil {
		return err
	}

	cfg, err := utils.ToStruct[types.TenantConfig](setting.Value)
	if err == nil && cfg.Production >= 1 {
		return nil
	}

	_, err = s.settings.UpdateSettingByKey(ctx, types.SettingKeyTenantConfig, &dto.UpdateSettingRequest{
		Value: map[string]interface{}{"production": 1},
	})
	return err
}

func (s *selfBillingService) ensureProductionEnv(ctx context.Context) error {
	n, err := s.EnvironmentRepo.CountByType(ctx, types.EnvironmentProduction)
	if err != nil || n > 0 {
		return err
	}

	_, err = s.envs.CreateEnvironment(ctx, dto.CreateEnvironmentRequest{
		Name: types.EnvironmentProduction.DisplayTitle(),
		Type: string(types.EnvironmentProduction),
	})
	if err == nil {
		return nil
	}

	// A concurrent activation may have created it first, so the limit refused ours.
	if n, cerr := s.EnvironmentRepo.CountByType(ctx, types.EnvironmentProduction); cerr == nil && n > 0 {
		return nil
	}
	return err
}

func toCheckoutResponse(co *remoteCheckout, status types.TenantInternalStatus) *dto.SelfBillingCheckoutResponse {
	return &dto.SelfBillingCheckoutResponse{
		CheckoutID: co.id,
		URL:        co.url,
		Status:     co.status,
		Terminal:   co.terminal,
		Tenant:     status,
	}
}

func toBillingDetails(req dto.SelfBillingCheckoutRequest) billingDetails {
	return billingDetails{
		name:       req.Name,
		email:      req.Email,
		line1:      req.Line1,
		line2:      req.Line2,
		city:       req.City,
		state:      req.State,
		country:    req.Country,
		postalCode: req.PostalCode,
		taxID:      req.TaxID,
	}
}
