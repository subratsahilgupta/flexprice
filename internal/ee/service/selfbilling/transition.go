package selfbilling

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/tenant"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/utils"
)

type decision struct {
	next      types.TenantInternalStatus
	changed   bool
	provision bool
}

// decide maps the Flexprice Cloud subscription onto the local status. Active tenants are never auto-suspended.
func decide(local types.TenantInternalStatus, remote *remoteSubscription) decision {
	keep := decision{next: local}
	if remote == nil {
		keep.provision = local == types.TenantInternalStatusTrialing
		return keep
	}

	switch remote.status {
	case types.SubscriptionStatusActive:
		if local == types.TenantInternalStatusActive {
			return keep
		}
		return decision{next: types.TenantInternalStatusActive, changed: true}

	case types.SubscriptionStatusIncomplete, types.SubscriptionStatusCancelled:
		if local != types.TenantInternalStatusTrialing {
			return keep
		}
		return decision{next: types.TenantInternalStatusSuspended, changed: true}

	default:
		return keep
	}
}

// updateTenantStatus moves the tenant to the status decide allows; an active tenant always gets a production environment. Safe to repeat.
func (s *selfBillingService) updateTenantStatus(ctx context.Context, t *tenant.Tenant, remote *remoteSubscription) (types.TenantInternalStatus, error) {
	ctx = types.SetTenantID(ctx, t.ID)

	d := decide(t.InternalStatus, remote)
	if d.changed {
		updated := tenant.NewBuilder(t).WithInternalStatus(d.next).Build()
		if err := s.TenantRepo.Update(ctx, updated); err != nil {
			return "", err
		}
		s.Logger.Info(ctx, "self-billing tenant status updated", "tenant_id", t.ID, "from", t.InternalStatus, "to", d.next)
	}

	if d.next != types.TenantInternalStatusActive {
		return d.next, nil
	}

	if err := s.raiseProductionLimit(ctx); err != nil {
		return "", err
	}
	if err := s.ensureProductionEnv(ctx); err != nil {
		return "", err
	}
	return d.next, nil
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
