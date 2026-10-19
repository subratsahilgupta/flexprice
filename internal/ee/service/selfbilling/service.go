package selfbilling

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/ee/service"
	"github.com/flexprice/flexprice/internal/interfaces"
)

// provisionTimeout bounds the detached provisioning of a new tenant on Flexprice Cloud.
const provisionTimeout = 2 * time.Minute

// Service bills this region's tenants through Flexprice Cloud.
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

func (s *selfBillingService) enabled() bool {
	return s.client != nil
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

	detached := context.WithoutCancel(ctx)
	go func() {
		ctx, cancel := context.WithTimeout(detached, provisionTimeout)
		defer cancel()

		defer func() {
			if r := recover(); r != nil {
				s.Logger.Error(ctx, "self-billing provisioning panicked", "error", fmt.Errorf("panic: %v", r), "tenant_id", tenantID)
			}
		}()

		if err := s.provision(ctx, tenantID, name, email); err != nil {
			s.Logger.Error(ctx, "self-billing provisioning failed", "error", err, "tenant_id", tenantID)
		}
	}()
}

// provision ensures the Flexprice Cloud customer and starts its trial when it has no self-serve subscription yet.
func (s *selfBillingService) provision(ctx context.Context, tenantID, name, email string) error {
	in := customerInput{
		extID:    externalID(s.Config.Billing.Region, tenantID),
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
	return s.client.StartTrial(ctx, in.extID)
}
