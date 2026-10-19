package selfbilling

import (
	"context"
	"testing"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/environment"
	"github.com/flexprice/flexprice/internal/ee/service"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var activeSub = &remoteSubscription{id: "sub_us:t1", status: types.SubscriptionStatusActive}

func TestDecide(t *testing.T) {
	sub := func(status types.SubscriptionStatus) *remoteSubscription {
		return &remoteSubscription{id: "s", status: status}
	}
	trialing, active, suspended := types.TenantInternalStatusTrialing, types.TenantInternalStatusActive, types.TenantInternalStatusSuspended

	tests := []struct {
		name          string
		local         types.TenantInternalStatus
		remote        *remoteSubscription
		wantNext      types.TenantInternalStatus
		wantChanged   bool
		wantProvision bool
	}{
		{"no_remote_trialing_provisions", trialing, nil, trialing, false, true},
		{"no_remote_active_left_alone", active, nil, active, false, false},
		{"no_remote_suspended_left_alone", suspended, nil, suspended, false, false},
		{"remote_trialing_stays", trialing, sub(types.SubscriptionStatusTrialing), trialing, false, false},
		{"remote_active_activates_trialing", trialing, sub(types.SubscriptionStatusActive), active, true, false},
		{"remote_active_restores_suspended", suspended, sub(types.SubscriptionStatusActive), active, true, false},
		{"remote_active_active_noop", active, sub(types.SubscriptionStatusActive), active, false, false},
		{"incomplete_trialing_suspends", trialing, sub(types.SubscriptionStatusIncomplete), suspended, true, false},
		{"cancelled_trialing_suspends", trialing, sub(types.SubscriptionStatusCancelled), suspended, true, false},
		{"never_auto_suspend_active_cancelled", active, sub(types.SubscriptionStatusCancelled), active, false, false},
		{"never_auto_suspend_active_incomplete", active, sub(types.SubscriptionStatusIncomplete), active, false, false},
		{"suspended_cancelled_stays", suspended, sub(types.SubscriptionStatusCancelled), suspended, false, false},
		{"paused_ignored", trialing, sub(types.SubscriptionStatusPaused), trialing, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := decide(tt.local, tt.remote)
			assert.Equal(t, tt.wantNext, d.next)
			assert.Equal(t, tt.wantChanged, d.changed)
			assert.Equal(t, tt.wantProvision, d.provision)
		})
	}
}

func TestUpdateTenantStatus_ActiveUnlocksProduction(t *testing.T) {
	tests := []struct {
		name          string
		from          types.TenantInternalStatus
		prodLimit     int // 0 = leave the default
		wantProdLimit int
	}{
		{"trialing_to_active", types.TenantInternalStatusTrialing, 0, 1},
		{"suspended_to_active", types.TenantInternalStatusSuspended, 0, 1},
		{"already_active_is_repaired", types.TenantInternalStatusActive, 0, 1},
		{"higher_limit_kept", types.TenantInternalStatusTrialing, 3, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, d := newTestService(newFakeClient())
			seedTenant(t, d, "t1", tt.from)
			ctx := tenantCtx("t1")
			if tt.prodLimit > 0 {
				_, err := s.settings.UpdateSettingByKey(ctx, types.SettingKeyTenantConfig, &dto.UpdateSettingRequest{
					Value: map[string]interface{}{"production": tt.prodLimit},
				})
				require.NoError(t, err)
			}

			for i := 0; i < 2; i++ {
				got, err := d.tenants.GetByID(ctx, "t1")
				require.NoError(t, err)
				status, err := s.updateTenantStatus(ctx, got, activeSub)
				require.NoError(t, err)
				assert.Equal(t, types.TenantInternalStatusActive, status)
			}

			got, err := d.tenants.GetByID(ctx, "t1")
			require.NoError(t, err)
			assert.Equal(t, types.TenantInternalStatusActive, got.InternalStatus)

			setting, err := s.settings.GetSettingByKeyUnchecked(ctx, types.SettingKeyTenantConfig)
			require.NoError(t, err)
			assert.EqualValues(t, tt.wantProdLimit, setting.Value["production"])
			assert.Equal(t, 1, productionEnvs(t, d, "t1"))
		})
	}
}

// racingEnvStore inserts a production environment right after the first count, as a concurrent activation would.
type racingEnvStore struct {
	*testutil.InMemoryEnvironmentStore
	counts int
}

func (r *racingEnvStore) CountByType(ctx context.Context, envType types.EnvironmentType) (int, error) {
	r.counts++
	n, err := r.InMemoryEnvironmentStore.CountByType(ctx, envType)
	if r.counts == 1 {
		winner := &environment.Environment{
			ID:        "env_winner",
			Name:      "Production",
			Type:      types.EnvironmentProduction,
			BaseModel: types.GetDefaultBaseModel(ctx),
		}
		if cerr := r.InMemoryEnvironmentStore.Create(ctx, winner); cerr != nil {
			return 0, cerr
		}
	}
	return n, err
}

func TestUpdateTenantStatus_EnvCreationRaceTreatedAsDone(t *testing.T) {
	s, d := newTestService(newFakeClient())
	seedTenant(t, d, "t1", types.TenantInternalStatusTrialing)
	racing := &racingEnvStore{InMemoryEnvironmentStore: d.envs}
	s.EnvironmentRepo = racing
	s.envs = service.NewEnvironmentService(racing, service.NewEnvAccessService(s.Config), s.settings, s.ServiceParams)

	tn, err := d.tenants.GetByID(context.Background(), "t1")
	require.NoError(t, err)
	_, err = s.updateTenantStatus(tenantCtx("t1"), tn, activeSub)
	require.NoError(t, err)

	// The first read saw 0, CreateEnvironment's limit check refused ours, the recount found the winner.
	assert.Equal(t, 3, racing.counts)
	assert.Equal(t, 1, productionEnvs(t, d, "t1"))

	got, err := d.tenants.GetByID(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, types.TenantInternalStatusActive, got.InternalStatus)
}

func TestUpdateTenantStatus_NotActiveLeavesProductionLocked(t *testing.T) {
	s, d := newTestService(newFakeClient())
	seedTenant(t, d, "t1", types.TenantInternalStatusTrialing)
	tn, err := d.tenants.GetByID(context.Background(), "t1")
	require.NoError(t, err)

	status, err := s.updateTenantStatus(tenantCtx("t1"), tn, &remoteSubscription{id: "sub_us:t1", status: types.SubscriptionStatusIncomplete})
	require.NoError(t, err)

	assert.Equal(t, types.TenantInternalStatusSuspended, status)
	assert.Equal(t, 0, productionEnvs(t, d, "t1"))
}
