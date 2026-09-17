package pg

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/revenuefact"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
	_ "github.com/lib/pq"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// newRealPostgresTestClient builds a real postgres.IClient backed by an
// actual Postgres instance (revenue_facts is queried with raw SQL, so a mock
// ent.Client cannot exercise it). Configuration comes from
// FLEXPRICE_TEST_POSTGRES_* env vars, mirroring
// internal/repository/ent/coupon_test.go's harness. The database must already
// have the revenue_facts table (migrations/versioned/postgres/
// 20260917000000_create_revenue_facts.sql) applied.
//
// Skips (instead of failing) when no reachable Postgres is configured, so it
// does not break `make test` / CI runs without a live database.
func newRealPostgresTestClient(t *testing.T) postgres.IClient {
	t.Helper()

	host := envOrDefault("FLEXPRICE_TEST_POSTGRES_HOST", "localhost")
	port := envOrDefault("FLEXPRICE_TEST_POSTGRES_PORT", "55432")
	user := envOrDefault("FLEXPRICE_TEST_POSTGRES_USER", "flexprice")
	password := envOrDefault("FLEXPRICE_TEST_POSTGRES_PASSWORD", "flexprice123")
	dbname := envOrDefault("FLEXPRICE_TEST_POSTGRES_DBNAME", "flexprice")
	sslmode := envOrDefault("FLEXPRICE_TEST_POSTGRES_SSLMODE", "disable")

	dsn := "host=" + host + " port=" + port + " user=" + user +
		" password=" + password + " dbname=" + dbname + " sslmode=" + sslmode

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("skipping: real Postgres test DB not reachable at %s:%s (%v)", host, port, err)
		return nil
	}
	if err := db.Ping(); err != nil {
		t.Skipf("skipping: real Postgres test DB not reachable at %s:%s (%v)", host, port, err)
		return nil
	}
	if _, err := db.Exec("SELECT 1 FROM revenue_facts LIMIT 0"); err != nil {
		t.Skipf("skipping: revenue_facts table not present (migrations not applied?): %v", err)
		return nil
	}

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := ent.NewClient(ent.Driver(drv))

	t.Cleanup(func() {
		_ = client.Close()
	})

	log, err := logger.NewLogger(&config.Configuration{
		Logging: config.LoggingConfig{Level: types.LogLevelInfo},
	})
	require.NoError(t, err)

	return postgres.NewClient(&postgres.EntClients{
		Writer:    client,
		Reader:    client,
		WriterDB:  db,
		ReaderDB:  db,
		HasReader: false,
	}, log, nil)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func newTestRevenueFactRepository(t *testing.T) revenuefact.Repository {
	t.Helper()
	client := newRealPostgresTestClient(t)
	log, err := logger.NewLogger(&config.Configuration{
		Logging: config.LoggingConfig{Level: types.LogLevelInfo},
	})
	require.NoError(t, err)
	return NewRevenueFactRepository(client, log)
}

func revenueFactTestContext(tenantID, environmentID string) context.Context {
	ctx := context.Background()
	ctx = types.SetTenantID(ctx, tenantID)
	ctx = types.SetEnvironmentID(ctx, environmentID)
	return ctx
}

func newTestRevenueFact(subscriptionID, priceID string, day time.Time, source types.RevenueSource) *revenuefact.RevenueFact {
	return &revenuefact.RevenueFact{
		ID:                types.GenerateUUIDWithPrefix("rf"),
		CustomerID:        "cust_" + subscriptionID,
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

func TestRevenueFactRepository_UpsertProvisionalIsIdempotentPerGrain(t *testing.T) {
	repo := newTestRevenueFactRepository(t)
	runID := types.GenerateUUID()
	ctx := revenueFactTestContext("tenant_"+runID, "env_"+runID)
	day := time.Now().UTC().Truncate(24 * time.Hour)

	f1 := newTestRevenueFact("sub_"+runID, "price_"+runID, day, types.RevenueSourceUsage)
	require.NoError(t, repo.UpsertProvisional(ctx, []*revenuefact.RevenueFact{f1}))

	f2 := newTestRevenueFact("sub_"+runID, "price_"+runID, day, types.RevenueSourceUsage)
	require.NoError(t, repo.UpsertProvisional(ctx, []*revenuefact.RevenueFact{f2}))

	got, err := repo.ListBySubscriptionPeriod(ctx, "sub_"+runID, day.AddDate(0, 0, -1), day.AddDate(0, 0, 1), types.FactProvisional)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.EqualValues(t, 2, got[0].Version)
	require.Equal(t, "tenant_"+runID, got[0].TenantID)
	require.Equal(t, "env_"+runID, got[0].EnvironmentID)
}

func TestRevenueFactRepository_FlipToFinal(t *testing.T) {
	repo := newTestRevenueFactRepository(t)
	runID := types.GenerateUUID()
	ctx := revenueFactTestContext("tenant_"+runID, "env_"+runID)
	day := time.Now().UTC().Truncate(24 * time.Hour)

	f := newTestRevenueFact("sub_"+runID, "price_"+runID, day, types.RevenueSourceUsage)
	require.NoError(t, repo.UpsertProvisional(ctx, []*revenuefact.RevenueFact{f}))

	n, err := repo.FlipToFinal(ctx, "sub_"+runID, "price_"+runID, day.AddDate(0, 0, -1), day.AddDate(0, 0, 1), "inv_"+runID, "inv_li_"+runID)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	finalFacts, err := repo.ListBySubscriptionPeriod(ctx, "sub_"+runID, day.AddDate(0, 0, -1), day.AddDate(0, 0, 1), types.FactFinal)
	require.NoError(t, err)
	require.Len(t, finalFacts, 1)
	require.NotNil(t, finalFacts[0].InvoiceID)
	require.Equal(t, "inv_"+runID, *finalFacts[0].InvoiceID)

	stillProvisional, err := repo.ListBySubscriptionPeriod(ctx, "sub_"+runID, day.AddDate(0, 0, -1), day.AddDate(0, 0, 1), types.FactProvisional)
	require.NoError(t, err)
	require.Empty(t, stillProvisional)
}

func TestRevenueFactRepository_ScopedByTenantAndEnvironment(t *testing.T) {
	repo := newTestRevenueFactRepository(t)
	runID := types.GenerateUUID()
	day := time.Now().UTC().Truncate(24 * time.Hour)

	ctxTenant1 := revenueFactTestContext("tenant_a_"+runID, "env_"+runID)
	ctxTenant2 := revenueFactTestContext("tenant_b_"+runID, "env_"+runID)

	f := newTestRevenueFact("sub_"+runID, "price_"+runID, day, types.RevenueSourceUsage)
	require.NoError(t, repo.UpsertProvisional(ctxTenant1, []*revenuefact.RevenueFact{f}))

	// A different tenant querying the same subscription/day must see nothing.
	got, err := repo.ListBySubscriptionPeriod(ctxTenant2, "sub_"+runID, day.AddDate(0, 0, -1), day.AddDate(0, 0, 1), types.FactProvisional)
	require.NoError(t, err)
	require.Empty(t, got)

	n, err := repo.FlipToFinal(ctxTenant2, "sub_"+runID, "price_"+runID, day.AddDate(0, 0, -1), day.AddDate(0, 0, 1), "inv_x", "inv_li_x")
	require.NoError(t, err)
	require.Equal(t, 0, n, "flipping from a different tenant must not affect another tenant's rows")
}
