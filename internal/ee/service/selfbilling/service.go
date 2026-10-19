package selfbilling

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/ee/service"
	"github.com/flexprice/flexprice/internal/interfaces"
)

// Service bills this region's tenants through the Flexprice billing tenant in Mumbai.
type Service interface {
	interfaces.SelfBillingHook
	Status(ctx context.Context, tenantID string) (*dto.SelfBillingStatusResponse, error)
}

type selfBillingService struct {
	service.ServiceParams
	client billingClient // nil when disabled
}

// New builds the self-billing service; every method is a no-op unless billing config enables self-billing.
func New(params service.ServiceParams) Service {
	s := &selfBillingService{ServiceParams: params}
	if params.Config != nil && params.Config.Billing.IsSelfBillingEnabled() {
		s.client = newRemoteClient(params.Config.Billing)
	}
	return s
}

func (s *selfBillingService) enabled() bool { return s.client != nil }

func (s *selfBillingService) region() string { return s.Config.Billing.Region }

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

func (s *selfBillingService) OnTenantCreated(ctx context.Context, tenantID, name, email string) {
	if !s.enabled() || s.Config.Billing.IsBillingTenant(tenantID) {
		return
	}
	if err := s.provision(ctx, tenantID, name, email); err != nil {
		s.Logger.Error(ctx, "self-billing provisioning failed", "error", err, "tenant_id", tenantID)
	}
}

// provision ensures the billing customer and starts its trial when it has no self-serve subscription yet.
func (s *selfBillingService) provision(ctx context.Context, tenantID, name, email string) error {
	in := customerInput{extID: externalID(s.region(), tenantID), name: name, email: email, tenantID: tenantID}
	if s.Config.Billing.TenantID != "" {
		in.legacyExtID = tenantID // Mumbai customers created by the removed in-process path
	}
	if err := s.client.EnsureCustomer(ctx, in); err != nil {
		return err
	}
	sub, err := s.client.LatestSubscription(ctx, in.extID)
	if err != nil || sub != nil {
		return err
	}
	return s.client.StartTrial(ctx, in.extID)
}
