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

// Name is a metric name; every name needs an entry in catalog.
type Name string

const (
	CheckoutSessions   Name = "checkout.sessions"
	PaymentTransitions Name = "payment.transitions"
	PaymentAttempts    Name = "payment.attempts"
	WebhookDeliveries  Name = "webhook.outbound.deliveries"
	APIErrors          Name = "api.errors"
	InvoiceTransitions Name = "invoice.transitions"
	RefundTransitions  Name = "refund.transitions"
	GatewayWebhooks    Name = "gateway.webhooks"
)

// catalog maps every metric to its description.
var catalog = map[Name]string{
	CheckoutSessions:   "Checkout sessions by status reached",
	PaymentTransitions: "Payment status changes by new status, including creation",
	PaymentAttempts:    "Gateway charge attempts by status",
	WebhookDeliveries:  "Outbound deliveries of subscribed webhook events by outcome",
	APIErrors:          "API error responses by error code",
	InvoiceTransitions: "Invoice status changes by new status, including draft creation",
	RefundTransitions:  "Refund status changes by new status, including creation",
	GatewayWebhooks:    "Inbound gateway webhooks by processing outcome; unhandled types are labelled other",
}

// counters is built once at init and only read afterwards, so concurrent lookups are safe.
var counters = lo.MapValues(catalog, func(description string, name Name) metric.Int64Counter {
	c, _ := meter.Int64Counter(string(name), metric.WithDescription(description))
	return c
})

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
	KeyErrorCode     LabelKey = "error_code"
	KeyAction        LabelKey = "action"
	KeyChargeMode    LabelKey = "charge_mode"
	KeyCheckout      LabelKey = "checkout"
	KeyInvoiceType   LabelKey = "invoice_type"
	KeyBillingReason LabelKey = "billing_reason"
	KeyDestination   LabelKey = "destination"
	KeyReason        LabelKey = "reason"
)

type Label struct {
	key   LabelKey
	value string
}

func L(key LabelKey, value string) Label { return Label{key: key, value: value} }

// RecordCounter increments the counter name by n; names missing from catalog are ignored.
func RecordCounter(ctx context.Context, name Name, n int64, labels ...Label) {
	c, ok := counters[name]
	if !ok {
		return
	}

	attrs := make([]attribute.KeyValue, 0, len(labels)+2)
	for _, l := range labels {
		attrs = append(attrs, attribute.String(string(l.key), l.value))
	}

	// Identity labels go last: OTel keeps the last value for a repeated key, so callers cannot override them.
	if tenantID := types.GetTenantID(ctx); tenantID != "" {
		environmentID := types.GetEnvironmentID(ctx)
		if !tenantAllowed(tenantID, environmentID) {
			return
		}

		attrs = append(attrs, attribute.String(string(KeyTenantID), tenantID), attribute.String(string(KeyEnvironmentID), environmentID))
	}

	// Inside a DB transaction, record only once it commits.
	add := func() { c.Add(ctx, n, metric.WithAttributes(attrs...)) }
	if !types.RegisterPostCommit(ctx, add) {
		add()
	}
}

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

func tenantAllowed(tenantID, environmentID string) bool {
	allowed := allowedTenants.Load()
	if allowed == nil || len(*allowed) == 0 {
		return true
	}
	_, tenant := (*allowed)[tenantID+":*"]
	_, pair := (*allowed)[tenantID+":"+environmentID]
	return tenant || pair
}
