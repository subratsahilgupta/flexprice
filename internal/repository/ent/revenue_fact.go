package ent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/flexprice/flexprice/ent"
	entrevenuefact "github.com/flexprice/flexprice/ent/revenuefact"
	"github.com/flexprice/flexprice/internal/domain/revenuefact"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
)

// revenueFactInsertColumns is the column list of the revenue_facts table in the
// order UpsertProvisional binds its args. Shared by the INSERT column list so
// the placeholders and args never drift apart.
var revenueFactInsertColumns = []string{
	"id", "tenant_id", "environment_id", "customer_id", "subscription_id", "sub_line_item_id", "price_id", "meter_id",
	"aggregation_type", "revenue_source", "period_start", "period_end", "day", "service_start", "service_end",
	"recognition_method", "usage_at_list_rate", "tier_delta", "entitlement_amount", "line_discount", "invoice_discount",
	"net_amount", "billable_qty", "entitlement_qty", "decomposition_mode", "currency", "status", "is_revert",
	"invoice_id", "invoice_line_item_id", "lock_adjusted_day", "computed_at", "version",
}

// revenueFactMutableColumns are the columns re-written by UpsertProvisional's
// DO UPDATE SET clause: every column except the provisional grain
// (tenant_id, environment_id, subscription_id, price_id, day, revenue_source),
// the primary key (id), and version (bumped separately).
var revenueFactMutableColumns = []string{
	"customer_id", "sub_line_item_id", "meter_id", "aggregation_type",
	"period_start", "period_end", "service_start", "service_end",
	"recognition_method", "usage_at_list_rate", "tier_delta", "entitlement_amount",
	"line_discount", "invoice_discount", "net_amount", "billable_qty", "entitlement_qty",
	"decomposition_mode", "currency", "status", "is_revert",
	"invoice_id", "invoice_line_item_id", "lock_adjusted_day", "computed_at",
}

// enumStrPtr nil-safely converts a nullable string-enum pointer to a *string so
// the SQL driver binds it consistently with the non-pointer enum args (which are
// cast via string(...)).
func enumStrPtr[T ~string](p *T) *string {
	if p == nil {
		return nil
	}
	s := string(*p)
	return &s
}

var revenueFactInsertColumnList = strings.Join(revenueFactInsertColumns, ", ")

type revenueFactRepository struct {
	client postgres.IClient
	log    *logger.Logger
}

// NewRevenueFactRepository creates an ent-backed revenuefact.Repository.
func NewRevenueFactRepository(client postgres.IClient, log *logger.Logger) revenuefact.Repository {
	return &revenueFactRepository{client: client, log: log}
}

// UpsertProvisional inserts/updates facts on the provisional grain (tenant,
// environment, subscription, price, day, revenue_source), bumping version on
// conflict. The partial-index ON CONFLICT with a version bump is not
// expressible through ent, so it runs as raw SQL over the ent writer
// connection. tenant_id, environment_id and status are always set from
// ctx/PROVISIONAL — never trusted from the input facts.
func (r *revenueFactRepository) UpsertProvisional(ctx context.Context, facts []*revenuefact.RevenueFact) error {
	if len(facts) == 0 {
		return nil
	}

	// The provisional-grain unique index includes price_id, and Postgres never
	// treats two NULLs as equal in a unique constraint, so a NULL price_id
	// would silently defeat ON CONFLICT dedup and double-write on re-roll. All
	// slice-1 revenue sources always sit on a price, so require it up front,
	// before any DB access.
	for _, f := range facts {
		if f.PriceID == nil || *f.PriceID == "" {
			return ierr.NewError("revenue fact requires a non-empty price_id").
				WithHint("Provisional revenue facts must carry a non-empty price_id").
				Mark(ierr.ErrValidation)
		}
	}

	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)
	now := time.Now().UTC()

	span := StartRepositorySpan(ctx, "revenue_fact", "upsert_provisional", map[string]interface{}{
		"tenant_id":      tenantID,
		"environment_id": environmentID,
		"fact_count":     len(facts),
	})
	defer FinishSpan(span)

	numCols := len(revenueFactInsertColumns)
	args := make([]interface{}, 0, len(facts)*numCols)
	valueGroups := make([]string, 0, len(facts))

	for i, f := range facts {
		computedAt := f.ComputedAt
		if computedAt.IsZero() {
			computedAt = now
		}
		version := f.Version
		if version == 0 {
			version = 1
		}

		placeholders := make([]string, numCols)
		base := i * numCols
		for j := 0; j < numCols; j++ {
			placeholders[j] = fmt.Sprintf("$%d", base+j+1)
		}
		valueGroups = append(valueGroups, "("+strings.Join(placeholders, ", ")+")")

		args = append(args,
			f.ID,
			tenantID,
			environmentID,
			f.CustomerID,
			f.SubscriptionID,
			f.SubLineItemID,
			f.PriceID,
			f.MeterID,
			enumStrPtr(f.AggregationType),
			string(f.RevenueSource),
			f.PeriodStart,
			f.PeriodEnd,
			f.Day,
			f.ServiceStart,
			f.ServiceEnd,
			enumStrPtr(f.RecognitionMethod),
			f.UsageAtListRate,
			f.TierDelta,
			f.EntitlementAmount,
			f.LineDiscount,
			f.InvoiceDiscount,
			f.NetAmount,
			f.BillableQty,
			f.EntitlementQty,
			string(f.DecompositionMode),
			f.Currency,
			string(types.FactProvisional),
			f.IsRevert,
			f.InvoiceID,
			f.InvoiceLineItemID,
			f.LockAdjustedDay,
			computedAt,
			version,
		)
	}

	setClauses := make([]string, 0, len(revenueFactMutableColumns)+1)
	for _, col := range revenueFactMutableColumns {
		setClauses = append(setClauses, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
	}
	setClauses = append(setClauses, "version = revenue_facts.version + 1")

	query := fmt.Sprintf(
		`INSERT INTO revenue_facts (%s) VALUES %s
		ON CONFLICT (tenant_id, environment_id, subscription_id, price_id, day, revenue_source)
		WHERE status = 'PROVISIONAL'
		DO UPDATE SET %s`,
		revenueFactInsertColumnList,
		strings.Join(valueGroups, ", "),
		strings.Join(setClauses, ", "),
	)

	if _, err := r.client.Writer(ctx).ExecContext(ctx, query, args...); err != nil {
		SetSpanError(span, err)
		r.log.Error(ctx, "upsert provisional revenue facts failed",
			"error", err,
			"tenant_id", tenantID,
			"environment_id", environmentID,
			"fact_count", len(facts),
		)
		return ierr.WithError(err).
			WithHint("Failed to upsert provisional revenue facts").
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	return nil
}

// FlipToFinal converts PROVISIONAL rows for a subscription/price/period to
// FINAL, stamping the given invoice line item. Returns the rows affected.
func (r *revenueFactRepository) FlipToFinal(ctx context.Context, subscriptionID, priceID string, periodStart, periodEnd time.Time, invoiceID, invoiceLineItemID string) (int, error) {
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	span := StartRepositorySpan(ctx, "revenue_fact", "flip_to_final", map[string]interface{}{
		"tenant_id":       tenantID,
		"environment_id":  environmentID,
		"subscription_id": subscriptionID,
		"price_id":        priceID,
		"invoice_id":      invoiceID,
	})
	defer FinishSpan(span)

	// UpsertProvisional rejects empty/NULL price_id up front, so provisional
	// rows always carry a non-empty price_id; a plain equality match suffices.
	n, err := r.client.Writer(ctx).RevenueFact.Update().
		Where(
			entrevenuefact.TenantID(tenantID),
			entrevenuefact.EnvironmentID(environmentID),
			entrevenuefact.SubscriptionID(subscriptionID),
			entrevenuefact.PriceID(priceID),
			entrevenuefact.DayGTE(periodStart),
			entrevenuefact.DayLTE(periodEnd),
			entrevenuefact.StatusEQ(types.FactProvisional),
		).
		SetStatus(types.FactFinal).
		SetInvoiceID(invoiceID).
		SetInvoiceLineItemID(invoiceLineItemID).
		Save(ctx)
	if err != nil {
		SetSpanError(span, err)
		r.log.Error(ctx, "flip revenue facts to final failed",
			"error", err,
			"tenant_id", tenantID,
			"environment_id", environmentID,
			"subscription_id", subscriptionID,
			"price_id", priceID,
			"invoice_id", invoiceID,
		)
		return 0, ierr.WithError(err).
			WithHint("Failed to flip revenue facts to final").
			Mark(ierr.ErrDatabase)
	}

	SetSpanSuccess(span)
	return n, nil
}

// ListBySubscriptionPeriod lists facts for a subscription within a period, filtered by status.
func (r *revenueFactRepository) ListBySubscriptionPeriod(ctx context.Context, subscriptionID string, periodStart, periodEnd time.Time, status types.FactStatus) ([]*revenuefact.RevenueFact, error) {
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	span := StartRepositorySpan(ctx, "revenue_fact", "list_by_subscription_period", map[string]interface{}{
		"tenant_id":       tenantID,
		"environment_id":  environmentID,
		"subscription_id": subscriptionID,
		"status":          string(status),
	})
	defer FinishSpan(span)

	rows, err := r.client.Reader(ctx).RevenueFact.Query().
		Where(
			entrevenuefact.TenantID(tenantID),
			entrevenuefact.EnvironmentID(environmentID),
			entrevenuefact.SubscriptionID(subscriptionID),
			entrevenuefact.DayGTE(periodStart),
			entrevenuefact.DayLTE(periodEnd),
			entrevenuefact.StatusEQ(status),
		).
		Order(ent.Asc(entrevenuefact.FieldDay)).
		All(ctx)
	if err != nil {
		SetSpanError(span, err)
		r.log.Error(ctx, "list revenue facts by subscription period failed",
			"error", err,
			"tenant_id", tenantID,
			"environment_id", environmentID,
			"subscription_id", subscriptionID,
		)
		return nil, ierr.WithError(err).
			WithHint("Failed to list revenue facts").
			Mark(ierr.ErrDatabase)
	}

	facts := make([]*revenuefact.RevenueFact, 0, len(rows))
	for _, row := range rows {
		facts = append(facts, revenueFactFromEnt(row))
	}

	SetSpanSuccess(span)
	return facts, nil
}

// revenueFactFromEnt maps a generated ent.RevenueFact to the domain model.
func revenueFactFromEnt(e *ent.RevenueFact) *revenuefact.RevenueFact {
	return &revenuefact.RevenueFact{
		ID:                e.ID,
		TenantID:          e.TenantID,
		EnvironmentID:     e.EnvironmentID,
		CustomerID:        e.CustomerID,
		SubscriptionID:    e.SubscriptionID,
		SubLineItemID:     e.SubLineItemID,
		PriceID:           e.PriceID,
		MeterID:           e.MeterID,
		AggregationType:   e.AggregationType,
		RevenueSource:     e.RevenueSource,
		PeriodStart:       e.PeriodStart,
		PeriodEnd:         e.PeriodEnd,
		Day:               e.Day,
		ServiceStart:      e.ServiceStart,
		ServiceEnd:        e.ServiceEnd,
		RecognitionMethod: e.RecognitionMethod,
		UsageAtListRate:   e.UsageAtListRate,
		TierDelta:         e.TierDelta,
		EntitlementAmount: e.EntitlementAmount,
		LineDiscount:      e.LineDiscount,
		InvoiceDiscount:   e.InvoiceDiscount,
		NetAmount:         e.NetAmount,
		BillableQty:       e.BillableQty,
		EntitlementQty:    e.EntitlementQty,
		DecompositionMode: e.DecompositionMode,
		Currency:          e.Currency,
		Status:            e.Status,
		IsRevert:          e.IsRevert,
		InvoiceID:         e.InvoiceID,
		InvoiceLineItemID: e.InvoiceLineItemID,
		LockAdjustedDay:   e.LockAdjustedDay,
		ComputedAt:        e.ComputedAt,
		Version:           e.Version,
	}
}
