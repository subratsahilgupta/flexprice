---
feature: invoice-draft-editing
version: v1
spec_hash: ""  # set after file is finalized
status: draft
created_at: 2026-08-03
---

# Spec — Invoice Draft Editing

Allow direct, in-place editing of a draft invoice: line item name/quantity/amount, adding or removing line items, and applying or removing coupons and taxes ad hoc — for both subscription-generated and one-off draft invoices.

## Problem

Today, nothing in the system allows editing an invoice once created. `UpdateInvoice` only touches `invoice_pdf_url`, `due_date`, and `metadata`. There is no endpoint to mutate line items, and coupons/taxes only ever get attached to an invoice as a side effect of `ComputeInvoice`/`FinalizeInvoice` resolving standing subscription-level config — never as a direct, invoice-scoped action. Support and billing ops teams need to correct a draft invoice by hand (fix a wrong quantity, rename a line item, add a one-time credit, waive a tax) before it's finalized and synced to Stripe, without voiding and recreating the whole invoice.

## Competitive context (informs the design, not a requirement)

Every platform researched (Stripe, Orb, Metronome, Lago) treats "draft = mutable, finalized = locked" as the entire safety net, and none use a preview-before-commit step for edits — changes apply directly. Orb ships the closest analog ("Invoice Adjustments": edit line items, add fixed fees/credits, change discounts, draft-only, immediate recalculation) but only via dashboard, not a documented public API, and explicitly does **not** protect adjustments from being silently discarded when the subscription driving the invoice is recomputed/regenerated. Our own `DailyDraftAndComputeActivity` (a daily, opt-in Temporal schedule that re-runs `ComputeInvoice` on every active subscription's current-period draft, purely for real-time usage visibility) has the exact same clobber risk — so this spec includes an explicit lock mechanism none of the researched competitors document having.

## Scope

**In scope:**
- Editing an existing line item's `display_name`, `quantity`, `amount` (independently — no derived recalculation between them) on a draft invoice, subscription-generated or one-off.
- Adding a new manual line item to an existing draft invoice.
- Removing a line item from a draft invoice (soft-delete).
- Applying a coupon to a draft invoice or a specific line item on it, as an ad-hoc, invoice-scoped action.
- Removing a previously ad-hoc-applied coupon.
- Applying a tax rate to a draft invoice, as an ad-hoc, invoice-scoped action.
- Removing a previously ad-hoc-applied tax.
- Locking a draft invoice's **line items** out of automatic/explicit recompute once they've been manually added, edited, or removed.
- Making coupon/tax recompute additive-aware, so ad-hoc coupon/tax applications survive a `ComputeInvoice` run without needing a lock.

**Out of scope:**
- Editing finalized, voided, or skipped invoices (existing status guards are unchanged).
- Any preview/dry-run mode for these edits — direct execution only, matching every platform researched.
- A dedicated audit trail table (existing structured logging is sufficient for v1).
- An "unlock" / force-recompute escape hatch for a line-item-locked invoice — `void` → `RecalculateInvoice` (creates a replacement invoice) remains the only way back to a fully system-computed state.
- Any change to how coupons/taxes resolve for invoices that have **not** had an ad-hoc coupon/tax applied (standing subscription-level `CouponAssociation`/`TaxAssociation` + compute-time resolution is untouched).
- Editing quantity/amount recalculating each other automatically (each is an independent override).
- Suppressing a *standing subscription-level* coupon/tax association going forward. Removing an ad-hoc `CouponApplication`/`TaxApplied` record only deletes that record — if a subscription-level `CouponAssociation`/`TaxAssociation` independently resolves to the same coupon/tax, it can still reapply on the next compute. See Known limitations.

## Data model changes

- `Invoice`: add `has_manual_edits bool` (default `false`). Set only by line-item mutations (add/edit/remove); coupon/tax ad-hoc mutations do **not** set it. This is the single interaction point with the existing compute pipeline.
- `InvoiceLineItem`: remove `Immutable()` from `display_name` so renames are possible. `amount`/`quantity` require no schema change — already mutable.
- No new tables. `CouponApplication.coupon_association_id` and `TaxApplied.tax_association_id` are already nullable, and `TaxApplied` already has a generic `entity_type`/`entity_id` (already supporting `invoice`). Ad-hoc coupon/tax application creates these records directly, with the association field left `nil`, entirely reusing existing entities.
- Line item removal reuses the existing `InvoiceRepo.RemoveLineItems` (sets `status = deleted`) — no new soft-delete mechanism.

## API surface

All endpoints below: draft-only (`invoice_status == DRAFT`, else reject), `write` permission scope (same tier as other draft mutations, not the stricter `delete` tier used for finalize/void), scoped to tenant + environment like every other endpoint in the codebase.

| Method & path | Behavior |
|---|---|
| `POST /invoices/:id/line-items` | Add a new manual line item (`display_name`, `quantity`, `amount`) |
| `PUT /invoices/:id/line-items/:line_item_id` | Edit `display_name`/`quantity`/`amount` on an existing line item (any origin) |
| `DELETE /invoices/:id/line-items/:line_item_id` | Soft-delete a line item |
| `POST /invoices/:id/coupons` | Apply a coupon ad hoc (invoice-level or scoped to one line item) |
| `DELETE /invoices/:id/coupons/:coupon_application_id` | Remove an ad-hoc coupon application |
| `POST /invoices/:id/taxes` | Apply a tax rate ad hoc |
| `DELETE /invoices/:id/taxes/:tax_applied_id` | Remove an ad-hoc tax application |

Every mutation is one transaction: validate draft status → apply the change → recalculate `subtotal`/`total`/`total_tax`/`total_discount`/`amount_due` from the full current set of line items + applied coupons/taxes → persist. The three line-item endpoints additionally set `has_manual_edits = true` if not already set; the four coupon/tax endpoints do not touch that flag.

## Recompute lock behavior

`has_manual_edits` — set only by a line-item add/edit/remove — is checked, and short-circuits with an info-level log (no error), at the top of:
- `ComputeInvoice` (`internal/ee/service/invoice.go:408`) — covers the explicit `POST /invoices/:id/compute` API call, the once-per-period-rollover call from `ProcessInvoiceWorkflow`, and every invocation from the daily `DailyDraftAndComputeActivity` schedule. This is what stops `reconcileLineItems` from re-deriving line items from usage over a manual edit.
- `RecalculateInvoiceV2` (`internal/ee/service/invoice.go:3253`) — covers the explicit `POST /invoices/:id/recalculate-v2` call, for the same reason.

Both become permanent no-ops for that invoice once locked. There is no unlock path in v1 (see Scope).

**Finalize is deliberately not gated by this lock.** `performFinalizeInvoiceActions` never re-derives line items — it only applies prepaid credits (additive against whatever the current line-item amounts are) and resolves/applies taxes (see below) against the current subtotal. Once coupon/tax application is made additive-aware, finalize's existing behavior is already correct on top of a manually-edited invoice without any special-casing: it finalizes using whatever state the invoice is actually in.

**Coupon/tax additive-awareness (required change to existing code, not just new code).** `applyCouponsToInvoice` and `applyTaxesToInvoice`/`ApplyTaxesOnInvoice` (`internal/ee/service/invoice.go` and `internal/ee/service/tax.go`), invoked both from `ComputeInvoice` (for invoices that are not line-item-locked) and from `performFinalizeInvoiceActions` (for every invoice), must be updated to fold in existing ad-hoc `CouponApplication`/`TaxApplied` records (`*_association_id = nil`, tied directly to the invoice) into `TotalDiscount`/`TotalTax` **in addition to** whatever subscription-level associations resolve that round, rather than overwriting those totals from subscription-resolved data alone. This is what lets an ad-hoc coupon/tax survive a `ComputeInvoice` run without needing to set `has_manual_edits`.

## Validation & guardrails

- Draft-only status gate on every new endpoint, checked under row lock (existing `GetForUpdate` pattern) to prevent a race with a concurrent finalize.
- Non-negative amount/quantity and currency-match validation carried over unchanged from `InvoiceLineItem.Validate()`.
- Tenant/environment scoping on every read/write, consistent with the rest of the codebase.
- Coupon/tax application reuses the existing discount/tax calculation logic (`applyCouponsToInvoice`/`applyTaxesToInvoice` machinery) to compute `OriginalPrice`/`FinalPrice`/`DiscountedAmount` or `TaxableAmount`/`TaxAmount` — not reimplemented.

## Acceptance criteria (EARS notation)

> WHEN = trigger condition · THE SYSTEM SHALL = required behavior

**CR-01 — Draft-only gate**
WHEN any edit endpoint (line item add/edit/remove, coupon add/remove, tax add/remove) is called on an invoice whose status is not `DRAFT`, THE SYSTEM SHALL reject the request with an invalid-state error and make no changes.

**CR-02 — Atomic apply + recompute**
WHEN any edit endpoint successfully applies its change, THE SYSTEM SHALL recalculate `subtotal`/`total`/`total_tax`/`total_discount`/`amount_due` from the current full set of line items and applied coupons/taxes, and persist both the edit and the recalculated totals within a single transaction.

**CR-03 — Manual edit lock is set only by line-item mutations**
WHEN a line item is added, edited, or removed on a draft invoice, THE SYSTEM SHALL set `has_manual_edits = true` on the invoice if not already set. Applying or removing an ad-hoc coupon or tax SHALL NOT set this flag.

**CR-04 — Recompute no-ops on line-item-locked invoices**
WHEN `ComputeInvoice` or `RecalculateInvoiceV2` is invoked — by API call, Temporal workflow, or scheduled job — on an invoice with `has_manual_edits = true`, THE SYSTEM SHALL return success without altering any line item, coupon application, tax applied record, or total, and SHALL log the skip at info level with the invoice ID.

**CR-04a — Finalize is unaffected by the lock**
WHEN `FinalizeInvoice` is called on a draft invoice, regardless of `has_manual_edits`, THE SYSTEM SHALL proceed with its existing behavior (invoice numbering, prepaid credit application, tax resolution via CR-04b, status transition to `FINALIZED`) using the invoice's current line items and totals as-is — finalize never re-derives line items and is therefore never blocked by this lock.

**CR-04b — Coupon/tax application is additive-aware**
WHEN `applyCouponsToInvoice` or `applyTaxesToInvoice`/`ApplyTaxesOnInvoice` runs (from `ComputeInvoice` on a non-locked invoice, or from `FinalizeInvoice` on any invoice), THE SYSTEM SHALL include existing ad-hoc `CouponApplication`/`TaxApplied` records (association field `nil`) for that invoice in the resulting `TotalDiscount`/`TotalTax`, in addition to whatever subscription-level associations resolve that round — never overwriting their contribution.

**CR-05 — Quantity/amount independence**
WHEN a user edits a line item's `quantity`, THE SYSTEM SHALL NOT automatically recalculate that line item's `amount` from a unit price, or vice versa; each is an independently stored override.

**CR-06 — Line item rename preserves identity fields**
WHEN a user edits a line item's `display_name`, THE SYSTEM SHALL persist the new name without altering `price_id`, `meter_id`, `subscription_line_item_id`, or any other pricing-context field.

**CR-07 — Line item removal is non-destructive**
WHEN a user removes a line item, THE SYSTEM SHALL soft-delete it via the existing `RemoveLineItems` mechanism (`status = deleted`) rather than hard-deleting the row.

**CR-08 — Ad-hoc coupon application**
WHEN a user applies a coupon to a draft invoice, THE SYSTEM SHALL create a `CouponApplication` record scoped directly to the invoice (and optionally one line item) with `coupon_association_id = nil`, without creating or modifying any subscription-level `CouponAssociation`.

**CR-09 — Ad-hoc tax application**
WHEN a user applies a tax rate to a draft invoice, THE SYSTEM SHALL create a `TaxApplied` record with `entity_type = invoice`, `entity_id = <invoice_id>`, `tax_association_id = nil`, without creating or modifying any subscription/customer-level `TaxAssociation`.

**CR-10 — Coupon/tax removal reverses totals**
WHEN a user removes a previously ad-hoc-applied coupon or tax, THE SYSTEM SHALL remove that application record and recompute totals as if it had never been applied.

**CR-11 — Existing validation preserved**
WHEN any line item amount/quantity is set via edit or add, THE SYSTEM SHALL enforce the existing non-negative and currency-match validation defined on `InvoiceLineItem.Validate()`.

**CR-12 — Tenant isolation**
WHEN any edit mutation is processed, THE SYSTEM SHALL scope all reads/writes to the caller's `tenant_id` + `environment_id`, returning not-found for a cross-tenant invoice ID.

**CR-13 — Concurrency safety**
WHEN two edit requests target the same invoice concurrently, THE SYSTEM SHALL serialize them via row-level locking (existing `GetForUpdate` pattern) so totals are never corrupted by a lost update.

## Failure-mode considerations

These new endpoints are synchronous CRUD mutations, not event consumers or Temporal activities, so the AGENTS.md event-processing invariants (idempotent handlers, event-timestamp ordering, backfill correctness) apply only where this feature touches existing async paths:

| Failure mode | How this spec addresses it |
|---|---|
| **Retries** | CR-04 guarantees `ComputeInvoice`/`RecalculateInvoiceV2` remain safe to retry (as Temporal activities) after an invoice is locked — a retry sees the same locked state and no-ops identically. |
| **Tenant isolation** | CR-12: every mutation scoped to tenant + environment. |
| **Concurrency** | CR-13: row-level lock prevents lost updates from concurrent edits or an edit racing a finalize/compute. |
| **Duplicate client retries on POST endpoints** (add line item / apply coupon / apply tax) | Accepted risk for v1 — no idempotency key. A retried POST could create a duplicate line item or coupon application. Flagged as a candidate follow-up if it proves to be a real-world problem; not blocking for v1 since these are low-frequency, human-initiated actions with immediate visual feedback (unlike high-volume event ingestion). |
| **Event ordering / backfill** | Not applicable — no usage-event stream is read by these endpoints. |

## Non-requirements

- Does NOT support editing finalized, voided, or skipped invoices.
- Does NOT provide a preview/dry-run mode for any edit.
- Does NOT persist a dedicated audit trail (structured logging only).
- Does NOT provide a way to unlock a line-item-locked invoice back into automatic recompute; void + recalculate is the only path back to a system-computed invoice.
- Does NOT change coupon/tax resolution behavior for invoices that have never had an ad-hoc coupon/tax applied.

## Known limitations (accepted for v1)

- **Removing an ad-hoc coupon/tax does not suppress a standing subscription-level association resolving to the same coupon/tax.** `CR-10` deletes the ad-hoc `CouponApplication`/`TaxApplied` record; it does not create a "never apply this again" rule. If the customer's subscription independently carries a `CouponAssociation`/`TaxAssociation` that resolves to that same coupon/tax rate, a later `ComputeInvoice` run (on a non-line-item-locked invoice) or `FinalizeInvoice` can reapply it. This only affects *removal* of something that's also standing subscription config — adding an ad-hoc coupon/tax, and editing/adding/removing line items, are fully protected as specified above.
