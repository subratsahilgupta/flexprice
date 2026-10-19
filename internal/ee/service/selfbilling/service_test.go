package selfbilling

import (
	"context"
	"sync"
	"testing"
	"time"

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

// recordingClient records the context provisioning ran with, signals each call, and can panic.
type recordingClient struct {
	*fakeClient
	called      chan struct{}
	ctxErr      error
	hasDeadline bool
	panic       bool
}

func newRecordingClient() *recordingClient {
	return &recordingClient{fakeClient: newFakeClient(), called: make(chan struct{}, 1)}
}

func (r *recordingClient) EnsureCustomer(ctx context.Context, in customerInput) error {
	r.fakeClient.mu.Lock()
	r.ctxErr = ctx.Err()
	_, r.hasDeadline = ctx.Deadline()
	r.fakeClient.mu.Unlock()

	r.called <- struct{}{}
	if r.panic {
		panic("flexprice cloud client blew up")
	}
	return r.fakeClient.EnsureCustomer(ctx, in)
}

// trials returns the number of trials started so far.
func (f *fakeClient) trials() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.trialsStarted
}

// hasCustomer reports whether the customer was ensured.
func (f *fakeClient) hasCustomer(extID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.customers[extID]
}

// newTestService returns a service with self-billing enabled against client.
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

func TestProvision(t *testing.T) {
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

			require.NoError(t, s.provision(context.Background(), tt.tenantID, "Acme", "a@acme.io"))

			assert.Equal(t, tt.wantCustomer, fc.customers["us:"+tt.tenantID])
			assert.Equal(t, tt.wantTrial, fc.trials() == 1)
			assert.Equal(t, tt.wantLegacyID, fc.legacyExtIDs["us:"+tt.tenantID])
		})
	}
}

func TestOnTenantCreated_BillingTenantSkipped(t *testing.T) {
	rc := newRecordingClient()
	s, _ := newTestService(rc)
	s.Config.Billing.TenantID = "t_billing"

	s.OnTenantCreated(context.Background(), "t_billing", "Acme", "a@acme.io")

	assert.Never(t, func() bool { return len(rc.called) > 0 }, 50*time.Millisecond, 10*time.Millisecond)
}

func TestOnTenantCreated_RunsDetached(t *testing.T) {
	rc := newRecordingClient()
	s, _ := newTestService(rc)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the request finishing must not abort provisioning
	s.OnTenantCreated(ctx, "t1", "Acme", "a@acme.io")

	require.Eventually(t, func() bool { return rc.trials() == 1 }, 2*time.Second, 5*time.Millisecond)
	assert.True(t, rc.hasCustomer("us:t1"))

	rc.fakeClient.mu.Lock()
	defer rc.fakeClient.mu.Unlock()
	require.NoError(t, rc.ctxErr)
	assert.True(t, rc.hasDeadline, "provisioning runs under its own timeout")
}

func TestOnTenantCreated_PanicIsRecovered(t *testing.T) {
	rc := newRecordingClient()
	rc.panic = true
	s, _ := newTestService(rc)

	assert.NotPanics(t, func() {
		s.OnTenantCreated(context.Background(), "t1", "Acme", "a@acme.io")
	})

	require.Eventually(t, func() bool { return len(rc.called) == 1 }, 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, 0, rc.trials())
}
