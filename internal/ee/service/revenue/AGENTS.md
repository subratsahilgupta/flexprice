# Revenue Facts — `internal/ee/service/revenue`

The `revenue_facts` system: a **shadow write-path** that re-runs the billing
preview and decomposes every charge into per-day (or per-period) revenue rows,
plus the analytics read surface over those rows.

Design doc: [FLE-1257 analytics platform ERD](../../../../docs/design/2026-09-10-FLE-1257-analytics-platform-erd.md)
· Export contract: [docs/export/revenue-facts.md](../../../../docs/export/revenue-facts.md)

## Hard invariants

- **Never mutate billing.** This package re-runs `PrepareSubscriptionInvoiceRequest`
  under `ReferencePointRevenueFacts` and only ever writes `revenue_facts` rows.
  Coupon amounts are dry-run (nothing persisted, no redemption counted).
- **Reconciliation is shadow-only.** Row / line-item / invoice sums are checked
  against the engine's own amounts; a mismatch is logged
  (`revenue_reconciliation_mismatch`, `revenue_facts_drift`) — it never blocks
  a write or an invoice operation.
- **Row identity** (marginal usage rows):
  `net == usage_at_list_rate + tier_delta − entitlement_amount − line_discount − invoice_discount`.
- **Idempotency everywhere.** Upserts land on the provisional grain
  (tenant, environment, subscription, price, sub_line_item, day, revenue_source);
  recomputes bump `version` in place. FINAL rows are immutable — a void appends
  contra rows (`is_revert = true`), never edits.
- **Tenant scoping.** Every query filters tenant + environment; batch jobs walk
  only environments opted in via the `revenue_analytics_config` setting.

## Dependency rules

- This package **imports `internal/ee/service`** (for `ServiceParams` and the
  billing/price/subscription constructors). The service package must **never
  import this one**.
- The interface is `interfaces.RevenueService` (aliased here as `Service`), so
  the service layer's invoice hooks (`service/revenue_hooks.go`) can call
  finalize/void updates through the `ServiceParams.RevenueFacts` field —
  injected in `cmd/server/main.go` (`enrichServiceParams`), nil-guarded, and
  never constructed inside the service package.
- Leaf consumers (API handler, Temporal activities, main) call `revenue.New`
  directly.

This is the extraction pattern for future sub-services: implementation package
under `ee/service/<name>` importing `service`; interface in
`internal/interfaces` **only if** the service layer must call back into it,
wired as a `ServiceParams` field in main.

## File map

| File | What it holds |
|---|---|
| `rollup.go` | `Service`/`New`, the rollup (preview → decompose per line → reconcile → upsert), finalize flip + JIT invoice fallback, void revert |
| `decompose.go` | Row shapes: `previewLineItem`, `dayCharge`, marginal/period_only writers, `decompositionMode` classifier |
| `curve.go` | Daily cumulative usage curve (SUM-based, re-priced via `CalculateCost`) |
| `overage_curve.go` | Commitment split: curve halves at the boundary, tier-inverse quantity split |
| `grant_curve.go` | Entitlement-grant billing rebuilt per day from quota-crossed windows |
| `reconcile.go` | Row / line-item / invoice identity checks |
| `sweep.go` | `ReconcileBookedInvoices`: drift detection on finalized/voided invoices, `auto_correct` repair |
| `analytics.go` | `GetRevenueAnalytics`: facts → grouped, time-bucketed rows (allocation policy, adjustment breakout) |
| `source.go` | Event-source (`meter_usage.source`) share allocation for `group_by: source` |

Related but deliberately elsewhere: invoice hooks in
`service/revenue_hooks.go`, the scheduled S3 exporter in
`service/sync/export/revenue_facts_export.go`, DTOs in
`api/dto/revenue_analytics.go`, repositories in `domain/revenuefact` +
`repository/ent/revenue_fact.go` + `testutil/inmemory_revenue_fact_store.go`.

## Known limits (see ERD §14.1 / §15)

- Multi-period commitments are skipped whole (rollup-maintained prior base not
  designed yet — Q3).
- MAX / LATEST / AVG / WEIGHTED_SUM pricing and volume tiers stay whole-period:
  the curve reads a cumulative SUM, which only prices sum-shaped billing.
- Windowed (per-bucket) line commitments keep whole-period usage.

## Testing

Tests run against the real billing preview via `testutil.BaseServiceTestSuite`
(in-memory stores, no Docker). `rollup_test.go` holds the suite + worked
example; `e2e_test.go` the full lifecycle; `matrix_test.go` the pricing/
metering matrix and grants+wallet lifecycles. Any change here must keep
`Σ net_amount` reconciling to the engine preview with zero mismatch logs.
