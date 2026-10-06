package metrics_test

import (
	"context"
	"testing"

	"github.com/flexprice/flexprice/internal/metrics"
	"github.com/flexprice/flexprice/internal/metrics/metricstest"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
)

// Declared before Install runs, as in production, to exercise OTel's global delegation.
var (
	testCounter  = metrics.NewCounter("test.requests", "test", "{request}")
	testNoTenant = metrics.NewCounter("test.pretenant", "test", "{request}").WithoutTenant()
)

func ctxFor(tenantID, environmentID string) context.Context {
	ctx := context.Background()
	if tenantID != "" {
		ctx = context.WithValue(ctx, types.CtxTenantID, tenantID)
	}
	if environmentID != "" {
		ctx = context.WithValue(ctx, types.CtxEnvironmentID, environmentID)
	}
	return ctx
}

func TestLabels(t *testing.T) {
	r := metricstest.Install(t)
	provider := metrics.L(metrics.KeyProvider, "labels-test")

	testCounter.Add(ctxFor("t1", "e1"), 2, provider)
	testCounter.Add(context.Background(), 1, provider)
	testNoTenant.Add(ctxFor("t1", "e1"), 1, provider)

	assert.Equal(t, int64(2), r.Sum("test.requests", map[string]string{"provider": "labels-test", "tenant_id": "t1", "environment_id": "e1"}))
	assert.ElementsMatch(t, []metricstest.Series{{Labels: map[string]string{"provider": "labels-test"}, Value: 1}}, r.Series("test.pretenant"))
}

func TestTenantAllowlist(t *testing.T) {
	r := metricstest.Install(t)
	metrics.SetTenantAllowlist([]string{"a1:e1", "a2"})
	t.Cleanup(func() { metrics.SetTenantAllowlist(nil) })

	tests := []struct {
		name string
		ctx  context.Context
		want int64
	}{
		{"exact pair allowed", ctxFor("a1", "e1"), 1},
		{"other env blocked", ctxFor("a1", "e2"), 0},
		{"whole tenant allowed", ctxFor("a2", "e9"), 1},
		{"unlisted tenant blocked", ctxFor("a3", "e1"), 0},
		{"no tenant allowed", ctxFor("", ""), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label := metrics.L(metrics.KeyProvider, "allowlist-"+tt.name)
			testCounter.Add(tt.ctx, 1, label)
			assert.Equal(t, tt.want, r.Sum("test.requests", map[string]string{"provider": "allowlist-" + tt.name}))
		})
	}
}
