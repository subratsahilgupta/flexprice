package ent

import (
	"context"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/config"
	domainGrant "github.com/flexprice/flexprice/internal/domain/entitlementgrant"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CloseWindow is raw SQL, so its monotonic `valid_to > $1` guard and its
// deliberate hands-off treatment of usage/grant_status/last_computed_at can only
// be verified against a real Postgres. Skips when none is reachable — see
// newRealPostgresTestClient in coupon_test.go.
func newTestEntitlementGrantRepository(t *testing.T) domainGrant.Repository {
	t.Helper()
	client := newRealPostgresTestClient(t)
	log, err := logger.NewLogger(&config.Configuration{
		Logging: config.LoggingConfig{Level: types.LogLevelInfo},
	})
	require.NoError(t, err)
	return NewEntitlementGrantRepository(client, log)
}

func testGrantContext() context.Context {
	ctx := context.Background()
	ctx = types.SetTenantID(ctx, types.DefaultTenantID)
	ctx = types.SetEnvironmentID(ctx, "env_eg_topup_test")
	ctx = context.WithValue(ctx, types.CtxUserID, types.DefaultUserID)
	return ctx
}

func newTestGrant(ctx context.Context, mutate func(*domainGrant.EntitlementGrant)) *domainGrant.EntitlementGrant {
	// valid_from is unique per grant so repeat runs never collide on the
	// (slot, valid_from) index.
	validFrom := time.Now().UTC().Truncate(time.Microsecond)
	g := &domainGrant.EntitlementGrant{
		ID:                  types.GenerateUUID(),
		EntitlementConfigID: "ent_topup_test",
		CustomerID:          "cust_topup_test",
		SubscriptionID:      "sub_topup_test",
		ScopeEntityType:     types.EntitlementGrantScopeFeature,
		ScopeEntityID:       "feat_topup_test",
		Measure:             types.EntitlementGrantMeasureQuantity,
		Quota:               decimal.NewFromInt(1000),
		Usage:               decimal.Zero,
		ValidFrom:           validFrom,
		ValidTo:             validFrom.Add(30 * 24 * time.Hour),
		GrantStatus:         types.EntitlementGrantStatusActive,
		EnvironmentID:       types.GetEnvironmentID(ctx),
		BaseModel:           types.GetDefaultBaseModel(ctx),
	}
	g.Status = types.StatusPublished
	if mutate != nil {
		mutate(g)
	}
	return g
}

func createTestGrant(t *testing.T, repo domainGrant.Repository, ctx context.Context, mutate func(*domainGrant.EntitlementGrant)) *domainGrant.EntitlementGrant {
	t.Helper()
	g, err := repo.Create(ctx, newTestGrant(ctx, mutate))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Delete(context.WithValue(ctx, types.CtxUserID, types.DefaultUserID), g.ID) })
	return g
}

func TestCloseWindow_ShortensValidTo(t *testing.T) {
	repo := newTestEntitlementGrantRepository(t)
	ctx := testGrantContext()
	g := createTestGrant(t, repo, ctx, nil)

	boundary := g.ValidFrom.Add(time.Hour)
	require.NoError(t, repo.CloseWindow(ctx, g.ID, boundary))

	after, err := repo.Get(ctx, g.ID)
	require.NoError(t, err)
	assert.True(t, after.ValidTo.Equal(boundary),
		"expected valid_to %s, got %s", boundary, after.ValidTo)
}

// The guard is what makes a replayed or out-of-order close safe: coverage that
// was already given up can never be handed back.
func TestCloseWindow_NeverExtends(t *testing.T) {
	repo := newTestEntitlementGrantRepository(t)
	ctx := testGrantContext()
	g := createTestGrant(t, repo, ctx, nil)

	boundary := g.ValidFrom.Add(time.Hour)
	require.NoError(t, repo.CloseWindow(ctx, g.ID, boundary))
	require.NoError(t, repo.CloseWindow(ctx, g.ID, g.ValidFrom.Add(10*time.Hour)))

	after, err := repo.Get(ctx, g.ID)
	require.NoError(t, err)
	assert.True(t, after.ValidTo.Equal(boundary),
		"a later boundary must not extend the window: %s", after.ValidTo)
}

// last_computed_at < valid_to is what keeps a closed row in the evaluator's
// unfinalized set for its final refresh, so the close must not stamp it — nor
// touch the usage/status the evaluator owns.
func TestCloseWindow_LeavesSnapshotFieldsAlone(t *testing.T) {
	repo := newTestEntitlementGrantRepository(t)
	ctx := testGrantContext()
	g := createTestGrant(t, repo, ctx, nil)

	computedAt := g.ValidFrom.Add(30 * time.Minute)
	require.NoError(t, repo.UpdateSnapshot(ctx, domainGrant.NewEntitlementGrantBuilder(g).
		WithUsage(decimal.NewFromInt(120)).
		WithLastComputedAt(&computedAt).
		Build()))

	require.NoError(t, repo.CloseWindow(ctx, g.ID, g.ValidFrom.Add(time.Hour)))

	after, err := repo.Get(ctx, g.ID)
	require.NoError(t, err)
	assert.True(t, after.Usage.Equal(decimal.NewFromInt(120)), "close must not touch usage")
	assert.True(t, after.Quota.Equal(g.Quota), "close must not touch quota")
	assert.Equal(t, types.EntitlementGrantStatusActive, after.GrantStatus,
		"close must leave grant_status to the evaluator")
	require.NotNil(t, after.LastComputedAt)
	assert.True(t, after.LastComputedAt.Equal(computedAt),
		"close must not stamp last_computed_at, or the row finalizes early and loses tail usage")
	assert.True(t, after.LastComputedAt.Before(after.ValidTo),
		"closed row must stay unfinalized so it still owes a final refresh")
}

func TestCloseWindow_IsTenantScoped(t *testing.T) {
	repo := newTestEntitlementGrantRepository(t)
	ctx := testGrantContext()
	g := createTestGrant(t, repo, ctx, nil)

	otherTenant := types.SetTenantID(ctx, "tenant_other")
	require.NoError(t, repo.CloseWindow(otherTenant, g.ID, g.ValidFrom.Add(time.Hour)))

	after, err := repo.Get(ctx, g.ID)
	require.NoError(t, err)
	assert.True(t, after.ValidTo.Equal(g.ValidTo),
		"another tenant must not be able to close this window")
}

// Phase 3 derives exhaustion each tick, so the snapshot must be able to CLEAR
// quota_crossed_at. SetNillable silently skips a nil, which would strand a
// stale overage window on a grant that is back under quota.
func TestUpdateSnapshot_ClearsQuotaCrossedAt(t *testing.T) {
	repo := newTestEntitlementGrantRepository(t)
	ctx := testGrantContext()

	crossed := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Microsecond)
	g := createTestGrant(t, repo, ctx, func(g *domainGrant.EntitlementGrant) {
		g.QuotaCrossedAt = &crossed
		g.GrantStatus = types.EntitlementGrantStatusExhausted
		g.Usage = decimal.NewFromInt(1500)
	})
	require.NotNil(t, g.QuotaCrossedAt)

	cleared := domainGrant.NewEntitlementGrantBuilder(g).
		WithUsage(decimal.NewFromInt(900)).
		WithGrantStatus(types.EntitlementGrantStatusActive).
		WithQuotaCrossedAt(nil).
		Build()
	require.NoError(t, repo.UpdateSnapshot(ctx, cleared))

	after, err := repo.Get(ctx, g.ID)
	require.NoError(t, err)
	assert.Nil(t, after.QuotaCrossedAt, "snapshot must clear quota_crossed_at, not skip the nil")
	assert.Equal(t, types.EntitlementGrantStatusActive, after.GrantStatus)
	assert.True(t, after.Usage.Equal(decimal.NewFromInt(900)))
}

func TestUpdateSnapshot_SetsQuotaCrossedAt(t *testing.T) {
	repo := newTestEntitlementGrantRepository(t)
	ctx := testGrantContext()
	g := createTestGrant(t, repo, ctx, nil)
	require.Nil(t, g.QuotaCrossedAt)

	at := time.Now().UTC().Truncate(time.Microsecond)
	crossed := domainGrant.NewEntitlementGrantBuilder(g).
		WithUsage(decimal.NewFromInt(1500)).
		WithGrantStatus(types.EntitlementGrantStatusExhausted).
		WithQuotaCrossedAt(&at).
		Build()
	require.NoError(t, repo.UpdateSnapshot(ctx, crossed))

	after, err := repo.Get(ctx, g.ID)
	require.NoError(t, err)
	require.NotNil(t, after.QuotaCrossedAt)
	assert.True(t, after.QuotaCrossedAt.UTC().Equal(at))
	assert.Equal(t, types.EntitlementGrantStatusExhausted, after.GrantStatus)
}

// The snapshot writes usage/status/crossing only — a tick must not roll back the
// quota a grant was opened with, or wipe its proration audit trail.
func TestUpdateSnapshot_LeavesQuotaAndMetadataAlone(t *testing.T) {
	repo := newTestEntitlementGrantRepository(t)
	ctx := testGrantContext()
	g := createTestGrant(t, repo, ctx, func(g *domainGrant.EntitlementGrant) {
		g.Quota = decimal.NewFromInt(1300)
		g.Metadata = types.Metadata{"proration.assoc_a1.coefficient": "0.5"}
	})

	require.NoError(t, repo.UpdateSnapshot(ctx, domainGrant.NewEntitlementGrantBuilder(g).
		WithUsage(decimal.NewFromInt(900)).
		WithGrantStatus(types.EntitlementGrantStatusActive).
		Build()))

	after, err := repo.Get(ctx, g.ID)
	require.NoError(t, err)
	assert.True(t, after.Quota.Equal(decimal.NewFromInt(1300)), "quota rolled back to %s", after.Quota)
	assert.Equal(t, "0.5", after.Metadata["proration.assoc_a1.coefficient"])
}
