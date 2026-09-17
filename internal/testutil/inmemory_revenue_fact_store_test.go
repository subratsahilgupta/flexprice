package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/revenuefact"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// revenueFactTestCtx sets both tenant and environment on the context, unlike
// the single-arg testCtx helper already defined in this package.
func revenueFactTestCtx(tenantID, environmentID string) context.Context {
	ctx := context.Background()
	ctx = types.SetTenantID(ctx, tenantID)
	ctx = types.SetEnvironmentID(ctx, environmentID)
	return ctx
}

func revenueFactDay(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

func sampleRevenueFact(subscriptionID, priceID string, day time.Time, source types.RevenueSource) *revenuefact.RevenueFact {
	return &revenuefact.RevenueFact{
		ID:                types.GenerateUUIDWithPrefix("rf"),
		CustomerID:        "cust_1",
		SubscriptionID:    subscriptionID,
		PriceID:           &priceID,
		RevenueSource:     source,
		PeriodStart:       day,
		PeriodEnd:         day.AddDate(0, 0, 1),
		Day:               day,
		NetAmount:         decimal.NewFromInt(100),
		DecompositionMode: types.Marginal,
		Currency:          "usd",
		Status:            types.FactProvisional,
	}
}

func TestInMemoryRevenueFactStore_UpsertIsIdempotentPerGrain(t *testing.T) {
	ctx := revenueFactTestCtx("tenant_1", "env_1")
	s := NewInMemoryRevenueFactStore()

	f := sampleRevenueFact("sub_1", "price_1", revenueFactDay("2026-09-11"), types.RevenueSourceUsage)
	require.NoError(t, s.UpsertProvisional(ctx, []*revenuefact.RevenueFact{f}))

	f2 := sampleRevenueFact("sub_1", "price_1", revenueFactDay("2026-09-11"), types.RevenueSourceUsage)
	require.NoError(t, s.UpsertProvisional(ctx, []*revenuefact.RevenueFact{f2}))

	got, err := s.ListBySubscriptionPeriod(ctx, "sub_1", revenueFactDay("2026-09-01"), revenueFactDay("2026-10-01"), types.FactProvisional)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.EqualValues(t, 2, got[0].Version)
	assert.Equal(t, "tenant_1", got[0].TenantID)
	assert.Equal(t, "env_1", got[0].EnvironmentID)
}

func TestInMemoryRevenueFactStore_UpsertDifferentGrainsDoNotCollide(t *testing.T) {
	ctx := revenueFactTestCtx("tenant_1", "env_1")
	s := NewInMemoryRevenueFactStore()

	day1 := revenueFactDay("2026-09-11")
	day2 := revenueFactDay("2026-09-12")

	require.NoError(t, s.UpsertProvisional(ctx, []*revenuefact.RevenueFact{
		sampleRevenueFact("sub_1", "price_1", day1, types.RevenueSourceUsage),
		sampleRevenueFact("sub_1", "price_1", day2, types.RevenueSourceUsage),
		sampleRevenueFact("sub_1", "price_2", day1, types.RevenueSourceUsage),
	}))

	got, err := s.ListBySubscriptionPeriod(ctx, "sub_1", revenueFactDay("2026-09-01"), revenueFactDay("2026-10-01"), types.FactProvisional)
	require.NoError(t, err)
	require.Len(t, got, 3)
	for _, f := range got {
		assert.EqualValues(t, 1, f.Version)
	}
}

func TestInMemoryRevenueFactStore_UpsertScopedByTenantAndEnvironment(t *testing.T) {
	s := NewInMemoryRevenueFactStore()
	day := revenueFactDay("2026-09-11")

	require.NoError(t, s.UpsertProvisional(revenueFactTestCtx("tenant_1", "env_1"), []*revenuefact.RevenueFact{
		sampleRevenueFact("sub_1", "price_1", day, types.RevenueSourceUsage),
	}))
	require.NoError(t, s.UpsertProvisional(revenueFactTestCtx("tenant_2", "env_1"), []*revenuefact.RevenueFact{
		sampleRevenueFact("sub_1", "price_1", day, types.RevenueSourceUsage),
	}))

	got1, err := s.ListBySubscriptionPeriod(revenueFactTestCtx("tenant_1", "env_1"), "sub_1", day, day, types.FactProvisional)
	require.NoError(t, err)
	require.Len(t, got1, 1)

	got2, err := s.ListBySubscriptionPeriod(revenueFactTestCtx("tenant_2", "env_1"), "sub_1", day, day, types.FactProvisional)
	require.NoError(t, err)
	require.Len(t, got2, 1)

	assert.NotEqual(t, got1[0].ID, got2[0].ID)
}

func TestInMemoryRevenueFactStore_FlipToFinal(t *testing.T) {
	ctx := revenueFactTestCtx("tenant_1", "env_1")
	s := NewInMemoryRevenueFactStore()

	day1 := revenueFactDay("2026-09-11")
	day2 := revenueFactDay("2026-09-12")
	day3 := revenueFactDay("2026-09-20") // outside the flip period

	require.NoError(t, s.UpsertProvisional(ctx, []*revenuefact.RevenueFact{
		sampleRevenueFact("sub_1", "price_1", day1, types.RevenueSourceUsage),
		sampleRevenueFact("sub_1", "price_1", day2, types.RevenueSourceUsage),
		sampleRevenueFact("sub_1", "price_1", day3, types.RevenueSourceUsage),
	}))

	n, err := s.FlipToFinal(ctx, "sub_1", "price_1", day1, day2, "inv_1", "inv_li_1")
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	finalFacts, err := s.ListBySubscriptionPeriod(ctx, "sub_1", day1, day3, types.FactFinal)
	require.NoError(t, err)
	require.Len(t, finalFacts, 2)
	for _, f := range finalFacts {
		require.NotNil(t, f.InvoiceID)
		assert.Equal(t, "inv_1", *f.InvoiceID)
		require.NotNil(t, f.InvoiceLineItemID)
		assert.Equal(t, "inv_li_1", *f.InvoiceLineItemID)
	}

	stillProvisional, err := s.ListBySubscriptionPeriod(ctx, "sub_1", day1, day3, types.FactProvisional)
	require.NoError(t, err)
	require.Len(t, stillProvisional, 1)
	assert.True(t, stillProvisional[0].Day.Equal(day3))
}

func TestInMemoryRevenueFactStore_FlipToFinalOnlyAffectsProvisional(t *testing.T) {
	ctx := revenueFactTestCtx("tenant_1", "env_1")
	s := NewInMemoryRevenueFactStore()
	day := revenueFactDay("2026-09-11")

	require.NoError(t, s.UpsertProvisional(ctx, []*revenuefact.RevenueFact{
		sampleRevenueFact("sub_1", "price_1", day, types.RevenueSourceUsage),
	}))

	n, err := s.FlipToFinal(ctx, "sub_1", "price_1", day, day, "inv_1", "inv_li_1")
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	// Flipping again over the same period should affect nothing more: the row is FINAL now.
	n2, err := s.FlipToFinal(ctx, "sub_1", "price_1", day, day, "inv_2", "inv_li_2")
	require.NoError(t, err)
	assert.Equal(t, 0, n2)
}
