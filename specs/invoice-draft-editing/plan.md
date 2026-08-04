---
derived_from_spec: specs/invoice-draft-editing/spec.md
derived_from_sha: ""  # set to spec_hash once spec is finalized
created_at: 2026-08-03
---

# Plan — Invoice Draft Editing

## Architecture

Seven new endpoints under `/invoices/:id/...` for line-item and ad-hoc coupon/tax mutation, plus two schema fields, plus two required changes to *existing* compute/finalize code so ad-hoc coupons/taxes survive recompute without needing a lock (per spec's CR-04b). New service logic lives in three new files in `internal/ee/service/` (methods on the existing `*invoiceService` receiver — Go allows this across files in one package), keeping `invoice.go` itself unchanged in size:

- `internal/ee/service/invoice_line_item_edit.go` — `AddLineItem`, `UpdateLineItem`, `RemoveLineItem` + a shared totals-recalculation helper. These three are the only methods that set `is_manually_edited`.
- `internal/ee/service/invoice_coupon_edit.go` — `ApplyAdHocCoupon`, `RemoveAdHocCoupon`.
- `internal/ee/service/invoice_tax_edit.go` — `ApplyAdHocTax`, `RemoveAdHocTax`.

Existing files get targeted edits, not rewrites: `invoice.go` (lock guards in `ComputeInvoice`/`RecalculateInvoiceV2`, additive-aware fix in `applyCouponsToInvoice`), `tax.go` (additive-aware fix in `applyTaxesToInvoice`/`RecalculateTaxesOnInvoice`), and the ent schemas/repository for the two schema changes.

## Deliberate deviations from general house style

- **No builder pattern for the new line-item edit methods.** AGENTS.md's coding style guide asks for private-fields + builders on new structs, but `Invoice`/`InvoiceLineItem` are plain public-field structs with no builder anywhere in the codebase (verified: no `NewInvoiceBuilder`/`NewInvoiceLineItemBuilder` exists). Introducing a builder for just this feature would make these two structs inconsistent with themselves. Follow the existing plain-struct convention.
- **Tax application stays invoice-level only, not line-item-level**, consistent with the spec (CR-09 never claimed line-item granularity for taxes, unlike coupons in CR-08). This isn't a new decision — it's confirmed necessary because `types.TaxRateEntityType` only has `customer`/`subscription`/`invoice`/`tenant` constants; there is no line-item-level entity type today, and adding one is out of scope.

## Affected modules

| Module | Change |
|---|---|
| `ent/schema/invoice.go` | Add `is_manually_edited bool` field (`Optional().Default(false).Comment(...)`) |
| `ent/schema/invoice_line_item.go` | Add `parent_line_item_id` field (`Optional().Nillable()`, self-referential string ID — no edge/foreign-key needed, plain string like other cross-entity ID refs in this schema, e.g. `subscription_line_item_id`). `display_name` stays untouched — see plan note below. |
| `internal/domain/invoice/model.go` | Add `IsManuallyEdited bool` to `Invoice` struct + `FromEnt` |
| `internal/domain/invoice/line_item.go` | Add `ParentLineItemID *string` to `InvoiceLineItem` struct + `FromEnt` |
| `internal/repository/ent/invoice.go` | `Update` (invoice-level) persists `IsManuallyEdited` |
| `internal/ee/service/invoice_line_item_edit.go` | New file: `AddLineItem`, `UpdateLineItem`, `RemoveLineItem`, shared totals helper |
| `internal/ee/service/invoice_coupon_edit.go` | New file: `ApplyAdHocCoupon`, `RemoveAdHocCoupon` |
| `internal/ee/service/invoice_tax_edit.go` | New file: `ApplyAdHocTax`, `RemoveAdHocTax` |
| `internal/ee/service/invoice.go` | `ComputeInvoice` (~408) + `RecalculateInvoiceV2` (~3253): add `is_manually_edited` no-op guard. `applyCouponsToInvoice` (~4420): fold in ad-hoc `CouponApplication`s |
| `internal/ee/service/tax.go` | `applyTaxesToInvoice`-equivalent tax logic: fold in ad-hoc `TaxApplied` records |
| `internal/api/dto/invoice.go` | New request DTOs: `AddLineItemRequest`, `UpdateLineItemRequest`, `ApplyCouponRequest`, `ApplyTaxRequest` |
| `internal/api/v1/invoice.go` | 7 new handlers |
| `internal/api/router.go` | 7 new route registrations inside existing `invoices := v1Private.Group("/invoices")` block |
| _(none)_ | No `migrations/postgres/` file needed — both new columns are simple additive Ent-native fields, applied via `make migrate-ent`'s live schema diff, same as prior precedent (`0a2fb87e0`). See T-02/T-04. |

## Key design points

### Totals recalculation helper (used by all three line-item methods)
A private helper, e.g. `recalculateTotalsFromLineItems(ctx, inv, lineItems) error`, computes `Subtotal` (sum of line item amounts), then re-derives `Total`/`AmountDue`/`AmountRemaining` factoring in existing `TotalDiscount`/`TotalTax`/`TotalPrepaidCreditsApplied` (same arithmetic already used in `applyTaxesToInvoice`: `Total = Subtotal - TotalPrepaidCreditsApplied - TotalDiscount + TotalTax`, floored at zero). This does **not** recompute discount/tax from scratch — CR-02 only requires recalculating from the *current* set of line items + already-applied coupons/taxes, not re-deriving new ones.

### `is_manually_edited` lock guard placement
- `ComputeInvoice` (`internal/ee/service/invoice.go:408`): add the check immediately after the existing `if inv.InvoiceStatus != Draft && != Skipped { return early }` guard (~line 417-419): `if inv.IsManuallyEdited { log.Info(...); return inv, false, nil }`.
- `RecalculateInvoiceV2` (`internal/ee/service/invoice.go:3253`): same shape, added right after its existing `if inv.InvoiceStatus != Draft { return err }` guard (~line 3263).
- Both checks read the invoice's current `IsManuallyEdited` — since `ComputeInvoice` already re-reads the invoice under `GetForUpdate` inside its transaction (~line 499), the freshest guard should be placed there, not just on the pre-lock read, to avoid a race where a line-item edit lands between the initial read and the lock.

### Additive-aware coupon/tax fix (CR-04b — the one change to *existing* logic this feature requires)
- `applyCouponsToInvoice` (`internal/ee/service/invoice.go:4420`) currently does `inv.TotalDiscount = couponResult.TotalDiscountAmount` (full overwrite). Change to: fetch ad-hoc coupon applications via `CouponApplicationRepo.List(ctx, &types.CouponApplicationFilter{InvoiceIDs: []string{inv.ID}})`, filter to those with `CouponAssociationID == ""` (ad-hoc — no standing association), sum their `DiscountedAmount`, and set `inv.TotalDiscount = couponResult.TotalDiscountAmount.Add(adHocDiscountTotal)`.
- Tax equivalent: fetch ad-hoc tax-applied records via `taxService.ListTaxApplied(ctx, &types.TaxAppliedFilter{EntityType: types.TaxRateEntityTypeInvoice, EntityID: inv.ID})`, filter to those with `TaxAssociationID == nil` (ad-hoc), sum their `TaxAmount`, and add to whatever `applyTaxesToInvoice`/`RecalculateTaxesOnInvoice` compute from subscription-resolved rates.
- This same additive logic must run in **both** call sites: `ComputeInvoice` (for non-locked invoices) and `performFinalizeInvoiceActions`/`RecalculateTaxesOnInvoice` (for every invoice, per CR-04a) — factor it into one shared private helper each (e.g. `sumAdHocCouponDiscounts(ctx, invoiceID) (decimal.Decimal, error)` and `sumAdHocTaxAmounts(ctx, invoiceID) (decimal.Decimal, error)`) called from both places rather than duplicating the fetch+filter+sum logic.

### Line item add/edit/remove — which repository methods to use
- **Add**: use `LineItemRepository.Create(ctx, item)` (`internal/domain/invoice/line_item_repository.go`), not the invoice-level `AddLineItems` edge method — that one is bulk/edge and marked as a legacy candidate for removal in a `TODO` comment.
- **Remove**: use the existing `InvoiceRepo.RemoveLineItems(ctx, invoiceID, []string{lineItemID})` (`internal/repository/ent/invoice.go:369`) — already does the soft-delete (`SetStatus(StatusDeleted)`) that CR-07 requires. Don't switch to `LineItemRepository.Delete` — its soft/hard-delete semantics weren't verified during research, and `RemoveLineItems`'s behavior already is.
- **Edit (archive-and-replace, per CR-06)**: this is NOT an in-place update of the edited fields. Two calls: (1) `LineItemRepository.Update(ctx, oldItem)` with `oldItem.Status = types.StatusArchived` — the existing `Update` method already sets `Status` in its chain, no repository change needed for this half; (2) `LineItemRepository.Create(ctx, newItem)` where `newItem` is a copy of `oldItem`'s fields with the requested changes applied and `ParentLineItemID = &oldItem.ID` — `Create` already supports setting `DisplayName` (Ent's `Immutable()` only blocks `Update`, not `Create`), so no schema change to `display_name` is needed either. Both calls happen inside the same transaction as the invoice row lock.

### Ad-hoc coupon/tax service call shape
- Coupon: `couponService.ApplyDiscount(ctx, dto.ApplyDiscountRequest{CouponID, OriginalPrice: <invoice.Subtotal or lineItem.Amount>, Currency: inv.Currency})` (`internal/ee/service/coupon.go:142`) computes the discount; then construct and persist a `CouponApplication` directly via its repository `Create` (`internal/domain/coupon_application/repository.go`), with `CouponAssociationID` left as zero-value/empty (ad-hoc) and `InvoiceLineItemID` set only for line-item-scoped coupons.
- Tax: fetch the chosen `TaxRate`, build a `*dto.TaxRateResponse`, compute the amount (the private `calculateTaxAmount` in `internal/ee/service/tax.go:1065` is same-package-callable since the new file lives in `internal/ee/service` too), then call the existing public `taxService.CreateTaxApplied(ctx, dto.CreateTaxAppliedRequest{TaxRateID, EntityType: types.TaxRateEntityTypeInvoice, EntityID: inv.ID, TaxableAmount, TaxAmount, Currency, TaxAssociationID: nil})` (`internal/ee/service/tax.go:350`).
- Removal: `couponApplicationRepo.Delete(ctx, id)` / `taxService.DeleteTaxApplied(ctx, id)` — both already exist. After either, re-run the totals helper (subtract the removed amount, or simpler: re-derive `TotalDiscount`/`TotalTax` from the remaining set of applications, same as the additive-aware fetch above).

### Permission scope
All 7 routes use `write(types.EntityInvoice, types.ActionWrite)` (the existing shorthand at `internal/api/router.go:113`), matching the rest of the draft-mutation routes (`compute`, `void`, `finalize` all use the same `write(...)` middleware — despite `finalize`/`void`'s swagger docs marking `@x-scope "delete"` for SDK/MCP categorization purposes, the actual Gin permission middleware is identical `ActionWrite` for all of them). Follow suit: swagger `@x-scope "write"` on all 7 new handlers, consistent with the decision that these are draft-only, reversible edits.

## Risks

- **`is_manually_edited`/`parent_line_item_id` naming collision risk**: none found for either — no existing field with these names, and no existing "parent" concept on `InvoiceLineItem` to conflict with.
- **Additive-aware fix touches invoice.go/tax.go paths used by every subscription invoice compute**, not just ones with ad-hoc coupons/taxes. Must verify with a test that an invoice with **zero** ad-hoc records behaves identically to today (sum = 0, no regression) — this is the highest-risk change in the whole feature since it's a modification to hot, existing, billing-critical code.
- **Line-item-scoped ad-hoc coupons don't get re-pointed across an edit** (documented in spec's Known limitations) — a coupon's `InvoiceLineItemID` keeps referencing the archived predecessor after that line item is edited. Doesn't affect totals (see spec), only a cosmetic traceability gap. Not fixed in v1.
- **Line-item-scoped tax was considered and explicitly rejected** for this iteration (see Deliberate deviations) — if a future request needs it, that's a new `TaxRateEntityType` constant plus resolver support, out of scope here.
- **`make migrate-ent-dry-run` for T-02/T-04 requires a live Postgres connection** (`cmd/migrate/postgres.go` always opens a real DB, no offline diff mode) — unavailable in a sandboxed execution environment with no Docker access. This sanity check must run in CI or on a developer machine with DB access before the two new columns ship to production. Not a blocker for the rest of this plan (Ent's auto-migration will apply them at deploy time regardless), but the review step itself is deferred, not skipped.

## Decisions carried over from spec (recap, not new)

- `is_manually_edited` set only by line-item add/edit/remove, never by coupon/tax ad-hoc actions.
- Finalize is never gated by the lock (it doesn't re-derive line items).
- No unlock/force-recompute escape hatch; void + recalculate is the only way back.
- No audit trail table; structured logging only.
- No idempotency key on the new POST endpoints (accepted risk, low-frequency human-initiated actions).
- 7 separate REST endpoints, not a single batch/modify endpoint (explicit decision after discussion — matches this codebase's and every researched competitor's convention).
- Line item edits are archive-and-replace, not in-place update — gives free per-edit history via existing `CreatedBy`/`CreatedAt`/`UpdatedBy`/`UpdatedAt` fields without a dedicated audit table, and mirrors `reconcileLineItems`'s existing archive-then-insert shape.
