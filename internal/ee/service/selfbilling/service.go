package selfbilling

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/tenant"
	"github.com/flexprice/flexprice/internal/ee/service"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/interfaces"
)

// provisionTimeout bounds the detached provisioning of a new tenant on Flexprice Cloud.
const provisionTimeout = 2 * time.Minute

var errDisabled = ierr.NewError("self-billing is not enabled").
	WithHint("Self-billing is not enabled for this deployment").
	Mark(ierr.ErrNotFound)

// Service bills this region's tenants through Flexprice Cloud.
type Service interface {
	interfaces.SelfBillingHook
	Status(ctx context.Context, tenantID string) (*dto.SelfBillingStatusResponse, error)

	// StartCheckout saves the billing details and opens the trial-end checkout of the tenant's subscription.
	StartCheckout(ctx context.Context, tenantID string, req dto.SelfBillingCheckoutRequest) (*dto.SelfBillingCheckoutResponse, error)

	// PollCheckout reports the tenant's checkout and activates the tenant once its subscription is active.
	PollCheckout(ctx context.Context, tenantID, checkoutID string) (*dto.SelfBillingCheckoutResponse, error)

	// CancelCheckout cancels the tenant's own open checkout.
	CancelCheckout(ctx context.Context, tenantID, checkoutID string) error

	// Summary returns the tenant's subscription, recent invoices and usage on Flexprice Cloud.
	Summary(ctx context.Context, tenantID string) (*dto.SelfBillingSummaryResponse, error)

	// PortalSession opens the customer portal for the tenant's own Flexprice Cloud customer.
	PortalSession(ctx context.Context, tenantID string) (*dto.SelfBillingPortalResponse, error)

	// RunHourly pushes the closed hours' usage to Flexprice Cloud, then syncs every tenant's status from it.
	RunHourly(ctx context.Context) error
}

type selfBillingService struct {
	service.ServiceParams

	client   billingClient // nil when disabled
	settings service.SettingsService
	envs     service.EnvironmentService
}

// New builds the self-billing service; every method is a no-op unless billing config enables self-billing.
func New(params service.ServiceParams) Service {
	settings := service.NewSettingsService(params)
	s := &selfBillingService{
		ServiceParams: params,
		settings:      settings,
		envs:          service.NewEnvironmentService(params.EnvironmentRepo, service.NewEnvAccessService(params.Config), settings, params),
	}
	if params.Config != nil && params.Config.Billing.IsSelfBillingEnabled() {
		s.client = newRemoteClient(params.Config.Billing)
	}
	return s
}

func (s *selfBillingService) enabled() bool {
	return s.client != nil
}

func (s *selfBillingService) extID(tenantID string) string {
	return externalID(s.Config.Billing.Region, tenantID)
}

func (s *selfBillingService) Status(ctx context.Context, tenantID string) (*dto.SelfBillingStatusResponse, error) {
	if !s.enabled() {
		return &dto.SelfBillingStatusResponse{Enabled: false}, nil
	}

	t, err := s.TenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return &dto.SelfBillingStatusResponse{Enabled: true, Status: t.InternalStatus}, nil
}

// OnTenantCreated provisions the tenant in the background so signup never waits on Flexprice Cloud.
func (s *selfBillingService) OnTenantCreated(ctx context.Context, tenantID, name, email string) {
	if !s.enabled() || s.Config.Billing.IsBillingTenant(tenantID) {
		return
	}

	s.runDetached(ctx, "provisioning", tenantID, func(ctx context.Context) error {
		return s.provision(ctx, tenantID, name, email)
	})
}

// OnTenantBillingDetailsUpdated copies the tenant's name and address to its Flexprice Cloud customer in the
// background; checkout saves the address too, so a failure here is only logged.
func (s *selfBillingService) OnTenantBillingDetailsUpdated(ctx context.Context, t *tenant.Tenant) {
	if !s.enabled() || s.Config.Billing.IsBillingTenant(t.ID) {
		return
	}

	address := t.BillingDetails.Address
	details := billingDetails{
		name:       t.Name,
		line1:      address.Line1,
		line2:      address.Line2,
		city:       address.City,
		state:      address.State,
		country:    address.Country,
		postalCode: address.PostalCode,
	}

	s.runDetached(ctx, "billing details sync", t.ID, func(ctx context.Context) error {
		return s.client.UpdateBillingDetails(ctx, s.extID(t.ID), details)
	})
}

// runDetached runs fn after the request returns, bounded by provisionTimeout; failures and panics are logged.
func (s *selfBillingService) runDetached(ctx context.Context, op, tenantID string, fn func(context.Context) error) {
	detached := context.WithoutCancel(ctx)
	go func() {
		ctx, cancel := context.WithTimeout(detached, provisionTimeout)
		defer cancel()

		defer func() {
			if r := recover(); r != nil {
				s.Logger.Error(ctx, "self-billing background task panicked", "error", fmt.Errorf("panic: %v", r), "operation", op, "tenant_id", tenantID)
			}
		}()

		if err := fn(ctx); err != nil {
			s.Logger.Error(ctx, "self-billing background task failed", "error", err, "operation", op, "tenant_id", tenantID)
		}
	}()
}

// provision ensures the Flexprice Cloud customer and starts its trial when it has no self-serve subscription yet.
func (s *selfBillingService) provision(ctx context.Context, tenantID, name, email string) error {
	in := customerInput{
		extID:    s.extID(tenantID),
		name:     name,
		email:    email,
		tenantID: tenantID,
	}

	// Customers made by the removed in-process path are keyed by the bare tenant ID.
	if s.Config.Billing.TenantID != "" {
		in.legacyExtID = tenantID
	}

	if err := s.client.EnsureCustomer(ctx, in); err != nil {
		return err
	}

	created, err := s.client.StartTrial(ctx, in.extID)
	if err != nil {
		return err
	}

	if created {
		s.notify(ctx, alert{
			Event:      alertTrialStarted,
			TenantID:   tenantID,
			TenantName: name,
		})
	}
	return nil
}

// RunHourly syncs statuses even when the usage push fails, and returns both errors joined.
func (s *selfBillingService) RunHourly(ctx context.Context) error {
	if !s.enabled() {
		return nil
	}

	pushErr := s.pushUsage(ctx, time.Now())
	syncErr := s.syncStatuses(ctx)
	return errors.Join(pushErr, syncErr)
}

// logRemoteFailure logs a Flexprice Cloud failure once, at the handler-facing method that returns it.
func (s *selfBillingService) logRemoteFailure(ctx context.Context, op, tenantID string, err error) {
	if !isRemoteFailure(err) {
		return
	}
	s.Logger.Error(ctx, "self-billing call to flexprice cloud failed", "error", err, "operation", op, "tenant_id", tenantID)
}
