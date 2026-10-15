//go:build pgintegration

package ent

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"testing"
	"time"

	"entgo.io/ent/dialect/sql/schema"
	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/ent/migrate"
	domainSub "github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/lib/pq"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

// FLEXPRICE_TEST_POSTGRES_DSN must be a PostgreSQL URL; each run uses an isolated schema.
func TestSubscriptionUpdate_PersistsTrialBounds(t *testing.T) {
	dsn := os.Getenv("FLEXPRICE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set FLEXPRICE_TEST_POSTGRES_DSN to run the PostgreSQL persistence test")
	}
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Contains(t, []string{"postgres", "postgresql"}, u.Scheme)
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	schemaName := types.GenerateUUIDWithPrefix("trial_test")
	_, err = db.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schemaName))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := db.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schemaName) + " CASCADE")
		require.NoError(t, err)
	})
	query := u.Query()
	query.Set("search_path", schemaName)
	u.RawQuery = query.Encode()
	client, err := ent.Open("postgres", u.String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	ctx := types.SetEnvironmentID(types.SetTenantID(context.Background(), "trial_test_tenant"), "trial_test_env")
	require.NoError(t, migrate.Create(ctx, client.Schema, []*schema.Table{migrate.SubscriptionsTable}, migrate.WithForeignKeys(false)))
	log := logger.NewNoopLogger()
	database := postgres.NewClient(&postgres.EntClients{Writer: client, Reader: client}, log, nil)
	repo := NewSubscriptionRepository(database, log, newFakeRedisCache())
	start := time.Date(2026, 10, 15, 18, 41, 41, 5412000, time.UTC)
	fixture := func(id string, status types.SubscriptionStatus, trialStart, trialEnd *time.Time) *domainSub.Subscription {
		return &domainSub.Subscription{
			ID: id, CustomerID: "customer", PlanID: "plan", Currency: "usd",
			SubscriptionStatus: status, SubscriptionType: types.SubscriptionTypeStandalone,
			StartDate: start.Add(-time.Hour), BillingAnchor: start,
			CurrentPeriodStart: start, CurrentPeriodEnd: start.AddDate(0, 1, 0),
			TrialStart: trialStart, TrialEnd: trialEnd,
			BillingCadence: types.BILLING_CADENCE_RECURRING,
			BillingPeriod:  types.BILLING_PERIOD_MONTHLY, BillingPeriodCount: 1,
			BillingCycle: types.BillingCycleAnniversary, Timezone: "UTC",
			PaymentBehavior:  string(types.PaymentBehaviorDefaultActive),
			CollectionMethod: string(types.CollectionMethodChargeAutomatically),
			PauseStatus:      types.PauseStatusNone, ProrationBehavior: types.ProrationBehaviorNone,
			EnvironmentID: types.GetEnvironmentID(ctx), BaseModel: types.GetDefaultBaseModel(ctx),
		}
	}
	for _, clear := range []bool{false, true} {
		name := "conversion"
		if clear {
			name = "clear bounds"
		}
		t.Run(name, func(t *testing.T) {
			original := fixture(types.GenerateUUIDWithPrefix("subs"), types.SubscriptionStatusTrialing,
				lo.ToPtr(start.Add(-time.Hour)), lo.ToPtr(start.AddDate(0, 0, 30)))
			require.NoError(t, repo.Create(ctx, original))
			updated := fixture(original.ID, types.SubscriptionStatusIncomplete, lo.ToPtr(start.Add(-2*time.Hour)), &start)
			if clear {
				updated = fixture(original.ID, types.SubscriptionStatusIncomplete, nil, nil)
			}
			require.NoError(t, database.WithTx(ctx, func(txCtx context.Context) error {
				return repo.Update(txCtx, updated)
			}))
			require.NoError(t, database.WithTx(ctx, func(txCtx context.Context) error {
				stored, err := repo.GetForUpdate(txCtx, original.ID)
				if err != nil {
					return err
				}
				require.Equal(t, updated.TrialStart, stored.TrialStart)
				require.Equal(t, updated.TrialEnd, stored.TrialEnd)
				require.Equal(t, updated.SubscriptionStatus, stored.SubscriptionStatus)
				return nil
			}))
		})
	}
}
