package selfbilling

import (
	"context"
	"sync"
	"testing"

	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/tenant"
	"github.com/flexprice/flexprice/internal/ee/service"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClient is an in-memory Flexprice Cloud keyed by customer external ID.
type fakeClient struct {
	mu sync.Mutex

	customers     map[string]bool
	legacyExtIDs  map[string]string // extID -> legacyExtID passed to EnsureCustomer
	subs          map[string]*remoteSubscription
	trialsStarted int
	err           error
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		customers:    map[string]bool{},
		legacyExtIDs: map[string]string{},
		subs:         map[string]*remoteSubscription{},
	}
}

func (f *fakeClient) EnsureCustomer(_ context.Context, in customerInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.customers[in.extID] = true
	f.legacyExtIDs[in.extID] = in.legacyExtID
	return f.err
}

func (f *fakeClient) StartTrial(_ context.Context, extID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.subs[extID] != nil {
		return f.err
	}
	f.trialsStarted++
	f.subs[extID] = &remoteSubscription{id: "sub_" + extID, status: types.SubscriptionStatusTrialing}
	return f.err
}

func (f *fakeClient) LatestSubscription(_ context.Context, extID string) (*remoteSubscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.subs[extID], f.err
}

// recordingClient records the context provisioning ran with and can panic on the first call.
type recordingClient struct {
	*fakeClient
	ctxErr      error
	hasDeadline bool
	panic       bool
}

func (r *recordingClient) EnsureCustomer(ctx context.Context, in customerInput) error {
	r.ctxErr = ctx.Err()
	_, r.hasDeadline = ctx.Deadline()
	if r.panic {
		panic("flexprice cloud client blew up")
	}
	return r.fakeClient.EnsureCustomer(ctx, in)
}

// newTestService returns an enabled service whose detached provisioning runs inline.
func newTestService(client billingClient) (*selfBillingService, *testutil.InMemoryTenantStore) {
	tenants := testutil.NewInMemoryTenantStore()
	params := service.ServiceParams{
		Logger:     logger.NewNoopLogger(),
		TenantRepo: tenants,
		Config: &config.Configuration{Billing: config.BillingConfig{
			Enabled:      true,
			Region:       "us",
			APIHost:      "https://cloud.test/v1",
			APIKey:       "sk",
			PlanID:       "plan_1",
			DashboardURL: "https://us.app.test",
		}},
	}

	s := &selfBillingService{
		ServiceParams: params,
		client:        client,
		spawn:         func(run func()) { run() },
	}
	return s, tenants
}

func TestNew_DisabledIsNoop(t *testing.T) {
	svc := New(service.ServiceParams{Logger: logger.NewNoopLogger(), Config: &config.Configuration{}})

	st, err := svc.Status(context.Background(), "tenant_1")
	require.NoError(t, err)
	assert.False(t, st.Enabled)

	assert.NotPanics(t, func() {
		svc.OnTenantCreated(context.Background(), "tenant_1", "Acme", "a@acme.io")
	})
}

func TestStatus_Enabled(t *testing.T) {
	s, tenants := newTestService(newFakeClient())
	seeded := &tenant.Tenant{ID: "t1", Name: "Acme", InternalStatus: types.TenantInternalStatusTrialing}
	require.NoError(t, tenants.Create(context.Background(), seeded))

	st, err := s.Status(context.Background(), "t1")
	require.NoError(t, err)
	assert.True(t, st.Enabled)
	assert.Equal(t, types.TenantInternalStatusTrialing, st.Status)
}

func TestOnTenantCreated_Provisions(t *testing.T) {
	tests := []struct {
		name          string
		tenantID      string
		billingTenant string
		existingSub   bool
		wantCustomer  bool
		wantTrial     bool
		wantLegacyID  string
	}{
		{
			name:         "new_tenant_gets_customer_and_trial",
			tenantID:     "t1",
			wantCustomer: true,
			wantTrial:    true,
		},
		{
			name:         "existing_subscription_no_second_trial",
			tenantID:     "t1",
			existingSub:  true,
			wantCustomer: true,
		},
		{
			name:          "billing_tenant_skipped",
			tenantID:      "t_billing",
			billingTenant: "t_billing",
		},
		{
			name:          "billing_tenant_region_looks_up_legacy_bare_id",
			tenantID:      "t1",
			billingTenant: "t_billing",
			wantCustomer:  true,
			wantTrial:     true,
			wantLegacyID:  "t1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := newFakeClient()
			s, _ := newTestService(fc)
			s.Config.Billing.TenantID = tt.billingTenant
			if tt.existingSub {
				fc.subs["us:"+tt.tenantID] = &remoteSubscription{id: "old", status: types.SubscriptionStatusActive}
			}

			s.OnTenantCreated(context.Background(), tt.tenantID, "Acme", "a@acme.io")

			assert.Equal(t, tt.wantCustomer, fc.customers["us:"+tt.tenantID])
			assert.Equal(t, tt.wantTrial, fc.trialsStarted == 1)
			assert.Equal(t, tt.wantLegacyID, fc.legacyExtIDs["us:"+tt.tenantID])
		})
	}
}

func TestOnTenantCreated_RunsDetached(t *testing.T) {
	rc := &recordingClient{fakeClient: newFakeClient()}
	s, _ := newTestService(rc)
	var pending func()
	s.spawn = func(run func()) { pending = run }

	ctx, cancel := context.WithCancel(context.Background())
	s.OnTenantCreated(ctx, "t1", "Acme", "a@acme.io")
	require.NotNil(t, pending, "provisioning is handed off, not run inline")
	assert.False(t, rc.customers["us:t1"], "the caller is not blocked on Flexprice Cloud")

	cancel() // the request finishing must not abort provisioning
	pending()

	assert.True(t, rc.customers["us:t1"])
	assert.Equal(t, 1, rc.trialsStarted)
	require.NoError(t, rc.ctxErr)
	assert.True(t, rc.hasDeadline, "provisioning runs under its own timeout")
}

func TestOnTenantCreated_PanicIsRecovered(t *testing.T) {
	rc := &recordingClient{fakeClient: newFakeClient(), panic: true}
	s, _ := newTestService(rc)

	assert.NotPanics(t, func() {
		s.OnTenantCreated(context.Background(), "t1", "Acme", "a@acme.io")
	})
	assert.Equal(t, 0, rc.trialsStarted)
}
