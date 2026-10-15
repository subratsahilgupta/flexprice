package ent

import (
	"context"
	"testing"

	"github.com/flexprice/flexprice/internal/cache"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/require"
)

// Invalidation must wait for the commit; a delete before it lets a concurrent read re-cache the old row.
func TestSubscriptionRepository_InvalidateCacheAfterCommit(t *testing.T) {
	tests := []struct {
		name            string
		inTx            bool
		finish          func(context.Context)
		wantCachedAfter bool
	}{
		{name: "no transaction deletes immediately"},
		{name: "commit deletes after commit", inTx: true, finish: types.RunPostCommitHooks},
		{name: "rollback keeps the committed row cached", inTx: true, finish: types.DiscardPostCommitHooks, wantCachedAfter: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redis := newFakeRedisCache()
			repo := &subscriptionRepository{redisCache: redis}
			ctx := entitlementCacheTestContext()
			if tt.inTx {
				ctx = types.WithPostCommitHooks(ctx)
			}
			keys := []string{
				cache.GenerateKey(ctx, cache.PrefixSubscription, "subs-1"),
				cache.GenerateKey(ctx, cache.PrefixSubscription, "subs-2"),
			}
			for _, key := range keys {
				redis.Set(ctx, key, "cached", 0)
			}

			repo.invalidateCache(ctx, "subs-1", "subs-2")

			for _, key := range keys {
				_, cached := redis.Get(ctx, key)
				require.Equal(t, tt.inTx, cached, "cache state before the transaction ends")
			}
			if tt.finish != nil {
				tt.finish(ctx)
			}
			for _, key := range keys {
				_, cached := redis.Get(ctx, key)
				require.Equal(t, tt.wantCachedAfter, cached, "cache state after the transaction ends")
			}
		})
	}
}
