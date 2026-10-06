package metrics

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// MeterName scopes app metrics so tracing views can target them.
const MeterName = "github.com/flexprice/flexprice/internal/metrics"

var meter = otel.Meter(MeterName)

// Name is a metric name, e.g. "checkout.sessions".
type Name string

type LabelKey string

const (
	KeyTenantID      LabelKey = "tenant_id"
	KeyEnvironmentID LabelKey = "environment_id"
	KeyProvider      LabelKey = "provider"
	KeyOutcome       LabelKey = "outcome"
	KeyStatus        LabelKey = "status"
	KeyMethodType    LabelKey = "method_type"
	KeyEventType     LabelKey = "event_type"
	KeyTransport     LabelKey = "transport"
)

type Label struct {
	key   LabelKey
	value string
}

func L(key LabelKey, value string) Label { return Label{key: key, value: value} }

var allowedTenants atomic.Pointer[map[string]struct{}]

// SetTenantAllowlist limits records to "tenant_id:environment_id" or "tenant_id" entries; empty allows all.
func SetTenantAllowlist(entries []string) {
	allowed := lo.Keyify(lo.FilterMap(entries, func(entry string, _ int) (string, bool) {
		entry = strings.TrimSpace(entry)
		if !strings.Contains(entry, ":") {
			entry += ":*"
		}
		return entry, entry != ":*"
	}))
	allowedTenants.Store(&allowed)
}

type Counter struct {
	instrument    metric.Int64Counter
	withoutTenant bool
}

// NewCounter declares a counter; unit is UCUM, e.g. "{request}".
func NewCounter(name Name, description, unit string) *Counter {
	instrument, _ := meter.Int64Counter(string(name), metric.WithDescription(description), metric.WithUnit(unit))
	return &Counter{instrument: instrument}
}

// WithoutTenant skips tenant labels and the allow-list, for paths where the tenant is untrusted.
func (c *Counter) WithoutTenant() *Counter {
	c.withoutTenant = true
	return c
}

func (c *Counter) Add(ctx context.Context, n int64, labels ...Label) {
	attrs := make([]attribute.KeyValue, 0, len(labels)+2)
	if tenantID := types.GetTenantID(ctx); tenantID != "" && !c.withoutTenant {
		environmentID := types.GetEnvironmentID(ctx)
		if !tenantAllowed(tenantID, environmentID) {
			return
		}

		attrs = append(attrs, attribute.String(string(KeyTenantID), tenantID), attribute.String(string(KeyEnvironmentID), environmentID))
	}

	for _, l := range labels {
		attrs = append(attrs, attribute.String(string(l.key), l.value))
	}

	c.instrument.Add(ctx, n, metric.WithAttributes(attrs...))
}

func tenantAllowed(tenantID, environmentID string) bool {
	allowed := allowedTenants.Load()
	if allowed == nil || len(*allowed) == 0 {
		return true
	}
	_, tenant := (*allowed)[tenantID+":*"]
	_, pair := (*allowed)[tenantID+":"+environmentID]
	return tenant || pair
}
