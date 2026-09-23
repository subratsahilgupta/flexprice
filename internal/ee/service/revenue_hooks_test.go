package service

import (
	"context"
	"testing"
	"time"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noopRevenueFacts satisfies interfaces.RevenueService without touching a DB.
type noopRevenueFacts struct{}

func (noopRevenueFacts) RollupSubscription(context.Context, string) error { return nil }
func (noopRevenueFacts) RollupDirty(context.Context, time.Time) (int, int, error) {
	return 0, 0, nil
}
func (noopRevenueFacts) FinalizeSubscriptionPeriod(context.Context, string) error { return nil }
func (noopRevenueFacts) RevertInvoiceFacts(context.Context, string) error         { return nil }
func (noopRevenueFacts) ReconcileBookedInvoices(context.Context, time.Time) (int, int, int, error) {
	return 0, 0, 0, nil
}
func (noopRevenueFacts) GetRevenueAnalytics(context.Context, *dto.RevenueAnalyticsRequest) (*dto.RevenueAnalyticsResponse, error) {
	return nil, nil
}

// TestAsyncRevenueFactsUpdate_DropsCallerTransaction: three of the four hook
// call sites fire from inside an open transaction. Carrying that transaction
// into a goroutine that outlives the request puts two goroutines on one
// Postgres connection and keeps using it after the commit — the wire protocol
// desynchronizes and the poisoned connection goes back to the pool, so the
// next unrelated request fails instead. The detached work must run on its own
// connection, while keeping the tenant scope it needs.
func TestAsyncRevenueFactsUpdate_DropsCallerTransaction(t *testing.T) {
	ctx := types.SetTenantID(context.Background(), "tenant_hook")
	ctx = types.SetEnvironmentID(ctx, "env_hook")
	ctx = context.WithValue(ctx, types.CtxDBTransaction, &ent.Tx{})
	require.NotNil(t, ctx.Value(types.CtxDBTransaction), "the caller is inside a transaction")

	seen := make(chan context.Context, 1)
	params := ServiceParams{Logger: logger.NewNoopLogger(), RevenueFacts: noopRevenueFacts{}}

	asyncRevenueFactsUpdate(ctx, params, "test", "inv_hook",
		func(asyncCtx context.Context, _ interfaces.RevenueService) error {
			seen <- asyncCtx
			return nil
		})

	select {
	case got := <-seen:
		assert.Nil(t, got.Value(types.CtxDBTransaction),
			"detached work must not reuse the request's transaction")
		assert.Equal(t, "tenant_hook", types.GetTenantID(got), "tenant scope survives")
		assert.Equal(t, "env_hook", types.GetEnvironmentID(got), "environment scope survives")
	case <-time.After(5 * time.Second):
		t.Fatal("the detached update never ran")
	}
}
