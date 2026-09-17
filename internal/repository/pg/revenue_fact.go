// Package pg holds repositories that issue hand-written SQL directly against
// the raw *sql.DB connections (as opposed to internal/repository/ent, which
// goes through ent). Ent's automatic tenant/environment interceptors do NOT
// run here, so every query in this package MUST filter/set tenant_id and
// environment_id explicitly from ctx.
package pg

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/domain/revenuefact"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
)

// revenueFactColumns is the full column list of the revenue_facts table, in
// the order scanRevenueFact expects. Shared by the INSERT column list and the
// SELECT projection so the two never drift apart.
var revenueFactColumns = []string{
	"id", "tenant_id", "environment_id", "customer_id", "subscription_id", "sub_line_item_id", "price_id", "meter_id",
	"aggregation_type", "revenue_source", "period_start", "period_end", "day", "service_start", "service_end",
	"recognition_method", "usage_at_list_rate", "tier_delta", "entitlement_credit", "line_discount", "invoice_discount",
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
	"recognition_method", "usage_at_list_rate", "tier_delta", "entitlement_credit",
	"line_discount", "invoice_discount", "net_amount", "billable_qty", "entitlement_qty",
	"decomposition_mode", "currency", "status", "is_revert",
	"invoice_id", "invoice_line_item_id", "lock_adjusted_day", "computed_at",
}

var revenueFactColumnList = strings.Join(revenueFactColumns, ", ")

type revenueFactRepository struct {
	client postgres.IClient
	log    *logger.Logger
}

// NewRevenueFactRepository creates a raw-SQL-backed revenuefact.Repository.
// Uses the *sql.DB connections postgres.IClient already holds for ent
// (WriterDB/ReaderDB) rather than opening a separate pool.
func NewRevenueFactRepository(client postgres.IClient, log *logger.Logger) revenuefact.Repository {
	return &revenueFactRepository{client: client, log: log}
}

// UpsertProvisional inserts/updates facts on the provisional grain (tenant,
// environment, subscription, price, day, revenue_source), bumping version on
// conflict. tenant_id, environment_id and status are always set from
// ctx/PROVISIONAL — never trusted from the input facts — since raw SQL
// bypasses ent's tenant/environment interceptors.
func (r *revenueFactRepository) UpsertProvisional(ctx context.Context, facts []*revenuefact.RevenueFact) error {
	if len(facts) == 0 {
		return nil
	}

	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)
	now := time.Now().UTC()

	numCols := len(revenueFactColumns)
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
			f.AggregationType,
			string(f.RevenueSource),
			f.PeriodStart,
			f.PeriodEnd,
			f.Day,
			f.ServiceStart,
			f.ServiceEnd,
			f.RecognitionMethod,
			f.UsageAtListRate,
			f.TierDelta,
			f.EntitlementCredit,
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
		revenueFactColumnList,
		strings.Join(valueGroups, ", "),
		strings.Join(setClauses, ", "),
	)

	db := r.client.WriterDB(ctx)
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
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

	return nil
}

// FlipToFinal converts PROVISIONAL rows for a subscription/price/period to
// FINAL, stamping the given invoice line item. Returns the rows affected.
func (r *revenueFactRepository) FlipToFinal(ctx context.Context, subscriptionID, priceID string, periodStart, periodEnd time.Time, invoiceID, invoiceLineItemID string) (int, error) {
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	// price_id is nullable; treat an empty priceID as matching NULL rows
	// (e.g. fixed-revenue facts that carry no price) via the OR clause below,
	// since price_id = '' would otherwise never match NULL.
	const query = `
		UPDATE revenue_facts
		SET status = 'FINAL', invoice_id = $1, invoice_line_item_id = $2
		WHERE tenant_id = $3
		  AND environment_id = $4
		  AND subscription_id = $5
		  AND (price_id = $6 OR (price_id IS NULL AND $6 = ''))
		  AND day BETWEEN $7 AND $8
		  AND status = 'PROVISIONAL'
	`

	db := r.client.WriterDB(ctx)
	res, err := db.ExecContext(ctx, query,
		invoiceID, invoiceLineItemID, tenantID, environmentID, subscriptionID, priceID, periodStart, periodEnd,
	)
	if err != nil {
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

	rows, err := res.RowsAffected()
	if err != nil {
		r.log.Error(ctx, "reading rows affected for flip to final failed",
			"error", err,
			"subscription_id", subscriptionID,
		)
		return 0, ierr.WithError(err).
			WithHint("Failed to read rows affected for flip to final").
			Mark(ierr.ErrDatabase)
	}

	return int(rows), nil
}

// ListBySubscriptionPeriod lists facts for a subscription within a period, filtered by status.
func (r *revenueFactRepository) ListBySubscriptionPeriod(ctx context.Context, subscriptionID string, periodStart, periodEnd time.Time, status types.FactStatus) ([]*revenuefact.RevenueFact, error) {
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	query := fmt.Sprintf(`
		SELECT %s
		FROM revenue_facts
		WHERE tenant_id = $1
		  AND environment_id = $2
		  AND subscription_id = $3
		  AND day BETWEEN $4 AND $5
		  AND status = $6
		ORDER BY day ASC
	`, revenueFactColumnList)

	db := r.client.ReaderDB(ctx)
	rows, err := db.QueryContext(ctx, query, tenantID, environmentID, subscriptionID, periodStart, periodEnd, string(status))
	if err != nil {
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
	defer rows.Close()

	facts := make([]*revenuefact.RevenueFact, 0)
	for rows.Next() {
		f, err := scanRevenueFact(rows)
		if err != nil {
			r.log.Error(ctx, "scan revenue fact row failed",
				"error", err,
				"subscription_id", subscriptionID,
			)
			return nil, ierr.WithError(err).
				WithHint("Failed to scan revenue fact row").
				Mark(ierr.ErrDatabase)
		}
		facts = append(facts, f)
	}
	if err := rows.Err(); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed while iterating revenue fact rows").
			Mark(ierr.ErrDatabase)
	}

	return facts, nil
}

// rowScanner is implemented by both *sql.Rows and *sql.Row.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanRevenueFact scans one row in revenueFactColumns order into a RevenueFact.
func scanRevenueFact(row rowScanner) (*revenuefact.RevenueFact, error) {
	var f revenuefact.RevenueFact
	var (
		subLineItemID     sql.NullString
		priceID           sql.NullString
		meterID           sql.NullString
		aggregationType   sql.NullString
		serviceStart      sql.NullTime
		serviceEnd        sql.NullTime
		recognitionMethod sql.NullString
		invoiceID         sql.NullString
		invoiceLineItemID sql.NullString
		lockAdjustedDay   sql.NullTime
		revenueSource     string
		decompositionMode string
		status            string
	)

	if err := row.Scan(
		&f.ID,
		&f.TenantID,
		&f.EnvironmentID,
		&f.CustomerID,
		&f.SubscriptionID,
		&subLineItemID,
		&priceID,
		&meterID,
		&aggregationType,
		&revenueSource,
		&f.PeriodStart,
		&f.PeriodEnd,
		&f.Day,
		&serviceStart,
		&serviceEnd,
		&recognitionMethod,
		&f.UsageAtListRate,
		&f.TierDelta,
		&f.EntitlementCredit,
		&f.LineDiscount,
		&f.InvoiceDiscount,
		&f.NetAmount,
		&f.BillableQty,
		&f.EntitlementQty,
		&decompositionMode,
		&f.Currency,
		&status,
		&f.IsRevert,
		&invoiceID,
		&invoiceLineItemID,
		&lockAdjustedDay,
		&f.ComputedAt,
		&f.Version,
	); err != nil {
		return nil, err
	}

	f.RevenueSource = types.RevenueSource(revenueSource)
	f.DecompositionMode = types.DecompositionMode(decompositionMode)
	f.Status = types.FactStatus(status)

	if subLineItemID.Valid {
		f.SubLineItemID = &subLineItemID.String
	}
	if priceID.Valid {
		f.PriceID = &priceID.String
	}
	if meterID.Valid {
		f.MeterID = &meterID.String
	}
	if aggregationType.Valid {
		f.AggregationType = &aggregationType.String
	}
	if recognitionMethod.Valid {
		f.RecognitionMethod = &recognitionMethod.String
	}
	if serviceStart.Valid {
		f.ServiceStart = &serviceStart.Time
	}
	if serviceEnd.Valid {
		f.ServiceEnd = &serviceEnd.Time
	}
	if invoiceID.Valid {
		f.InvoiceID = &invoiceID.String
	}
	if invoiceLineItemID.Valid {
		f.InvoiceLineItemID = &invoiceLineItemID.String
	}
	if lockAdjustedDay.Valid {
		f.LockAdjustedDay = &lockAdjustedDay.Time
	}

	return &f, nil
}
