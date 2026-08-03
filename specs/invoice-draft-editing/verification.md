---
derived_from_spec: specs/invoice-draft-editing/spec.md
derived_from_sha: ""  # set to spec_hash once spec finalized
created_at: 2026-08-03
---

# Verification — Invoice Draft Editing

Each acceptance criterion maps to at least one test. If a criterion has no test, the feature is not done.

---

## CR-01 — Draft-only gate

**Test:** `TestLineItemEndpoints_RejectNonDraft` / `TestCouponTaxEndpoints_RejectNonDraft`
- Setup: one finalized invoice, one voided invoice, one draft invoice with a line item.
- Action: call each of the 7 new service methods (`AddLineItem`, `UpdateLineItem`, `RemoveLineItem`, `ApplyAdHocCoupon`, `RemoveAdHocCoupon`, `ApplyAdHocTax`, `RemoveAdHocTax`) against the finalized and voided invoices.
- Assert: each call returns an `ierr.ErrValidation`-marked error, no DB row is created/changed; the same calls against the draft invoice succeed.
- Type: service unit test.

---

## CR-02 — Atomic apply + recompute

**Test:** `TestRecalculateInvoiceTotals_Arithmetic`
- Setup: line items summing to $100, existing `TotalDiscount = $10`, `TotalTax = $5`, `TotalPrepaidCreditsApplied = $0`.
- Action: call `recalculateInvoiceTotals`.
- Assert: `Subtotal = $100`, `Total = $95`, `AmountDue = $95`, `AmountRemaining = $95 - AmountPaid`.
- Edge case: discount + tax combination that would drive `Total` negative → assert floored at `$0`.
- Type: service unit test (pure function, no DB).

**Test:** `TestAddLineItem_RecomputesTotalsInSameTransaction`
- Setup: draft invoice with one $50 line item.
- Action: `AddLineItem` with a $30 line item.
- Assert: within the same call, `Subtotal = $80` and the new line item both persisted; a DB read immediately after shows both, never an intermediate state with only one applied.
- Type: service integration test (real DB).

---

## CR-03 — Manual edit lock is set only by line-item mutations

**Test:** `TestLineItemMutations_SetIsManuallyEdited`
- Action: call `AddLineItem`, then separately (fresh invoice) `UpdateLineItem`, then separately `RemoveLineItem`.
- Assert: `IsManuallyEdited == true` after each, on a previously-`false` invoice.

**Test:** `TestCouponTaxMutations_DoNotSetIsManuallyEdited`
- Action: call `ApplyAdHocCoupon`, `RemoveAdHocCoupon`, `ApplyAdHocTax`, `RemoveAdHocTax` on a fresh invoice (`IsManuallyEdited == false`).
- Assert: `IsManuallyEdited` remains `false` after every one of these four calls.
- Type: service unit tests.

---

## CR-04 — Recompute no-ops on line-item-locked invoices

**Test:** `TestComputeInvoice_NoOpsWhenManuallyEdited`
- Setup: subscription draft invoice with known line items and usage data; set `IsManuallyEdited = true`.
- Action: call `ComputeInvoice`.
- Assert: line items, `Subtotal`/`Total`/etc. unchanged after the call; no error returned; an info-level log line is emitted containing the invoice ID.
- Type: service integration test.

**Test:** `TestRecalculateInvoiceV2_NoOpsWhenManuallyEdited`
- Same shape, calling `RecalculateInvoiceV2` instead.

---

## CR-04a — Finalize is unaffected by the lock

**Test:** `TestFinalizeInvoice_ProceedsRegardlessOfManualEdits`
- Setup: two subscription draft invoices, identical usage/line items, one with `IsManuallyEdited = true` (after a line-item edit), one with `IsManuallyEdited = false`.
- Action: call `FinalizeInvoice` on both.
- Assert: both transition to `FINALIZED`, both get an invoice number, both run prepaid-credit application and tax resolution (CR-04b) — no behavioral branch on `IsManuallyEdited` inside finalize itself.
- Type: service integration test.

---

## CR-04b — Coupon/tax application is additive-aware

**Test:** `TestApplyCouponsToInvoice_ZeroAdHocRegression` *(regression, must pass unmodified)*
- Setup: subscription invoice with only subscription-resolved coupons, no ad-hoc `CouponApplication` records.
- Action: call `applyCouponsToInvoice`.
- Assert: `TotalDiscount` identical to pre-change behavior — confirms the additive fix doesn't alter the zero-ad-hoc case.

**Test:** `TestApplyCouponsToInvoice_AdditiveWithAdHoc`
- Setup: same invoice, plus one ad-hoc `CouponApplication` (`CouponAssociationID = ""`) with `DiscountedAmount = $10`, plus subscription-resolved coupons contributing $15.
- Action: call `applyCouponsToInvoice`.
- Assert: `TotalDiscount = $25` (sum of both), not just $15.

**Test:** `TestApplyTaxesToInvoice_ZeroAdHocRegression` *(regression)*
- Existing tests `TestRecalculateTaxesOnInvoice_WithSubscriptionTax`, `TestRecalculateTaxesOnInvoice_NoTaxUnchanged`, `TestRecalculateTaxesOnInvoice_FinalizeNoDoubleTax` (`internal/ee/service/invoice_apply_taxes_draft_test.go`) must pass unmodified after the additive-aware change.

**Test:** `TestApplyTaxesToInvoice_AdditiveWithAdHoc`
- Setup: invoice with one ad-hoc `TaxApplied` record (`TaxAssociationID = nil`, `TaxAmount = $8`) plus subscription-resolved tax rates contributing $12.
- Assert: `TotalTax = $20`.
- Type: all service integration tests (real DB).

---

## CR-05 — Quantity/amount independence

**Test:** `TestUpdateLineItem_QuantityAmountIndependent`
- Setup: line item with `Amount = $100`, `Quantity = 10`.
- Action: `UpdateLineItem` with only `Quantity = 20`.
- Assert: new row has `Quantity = 20`, `Amount = $100` (unchanged — not recalculated as `$200`).
- Action (separate case): `UpdateLineItem` with only `Amount = $250`.
- Assert: new row has `Amount = $250`, `Quantity = 10` (unchanged).
- Type: service unit test.

---

## CR-06 — Line item edit is archive-and-replace, preserving identity fields

**Test:** `TestUpdateLineItem_ArchivesOldCreatesNew`
- Setup: line item with `PriceID`, `MeterID`, `SubscriptionLineItemID` all set.
- Action: `UpdateLineItem` changing `DisplayName`.
- Assert: old row's `Status == archived`; new row exists with the new `DisplayName` and identical `PriceID`/`MeterID`/`SubscriptionLineItemID`; old row's ID ≠ new row's ID.
- Type: service integration test.

---

## CR-06a — Line item lineage supports multiple edits

**Test:** `TestUpdateLineItem_ChainsLineageAcrossMultipleEdits`
- Action: create line item (v1) → edit it (v2, `ParentLineItemID = v1.ID`) → edit v2 (v3).
- Assert: `v3.ParentLineItemID == v2.ID` (not `v1.ID`); walking `ParentLineItemID` from v3 reaches v2 then v1.
- Type: service integration test.

---

## CR-07 — Line item removal is non-destructive

**Test:** `TestRemoveLineItem_SoftDeletes`
- Action: `RemoveLineItem`.
- Assert: row still exists in DB with `Status == deleted`; excluded from `ListByInvoiceID`'s default (published-only) results and from recalculated totals.
- Type: service integration test.

---

## CR-08 — Ad-hoc coupon application

**Test:** `TestApplyAdHocCoupon_InvoiceLevel`
- Action: `ApplyAdHocCoupon` with `LineItemID = nil`.
- Assert: `CouponApplication` created with `InvoiceID` set, `InvoiceLineItemID = nil`, `CouponAssociationID = ""`; no `CouponAssociation` row created or modified.

**Test:** `TestApplyAdHocCoupon_LineItemLevel`
- Action: `ApplyAdHocCoupon` with `LineItemID` set to an existing line item.
- Assert: discount computed against that line item's `Amount.Sub(LineItemDiscount)`, not the invoice subtotal; `CouponApplication.InvoiceLineItemID` set accordingly.
- Type: service integration tests.

---

## CR-09 — Ad-hoc tax application

**Test:** `TestApplyAdHocTax_CreatesInvoiceScopedRecord`
- Action: `ApplyAdHocTax`.
- Assert: `TaxApplied` created with `EntityType == invoice`, `EntityID == <invoice_id>`, `TaxAssociationID == nil`; no `TaxAssociation` row created or modified.
- Type: service integration test.

---

## CR-10 — Coupon/tax removal reverses totals

**Test:** `TestRemoveAdHocCoupon_RevertsDiscount`
- Setup: invoice with `TotalDiscount = $25` (from one ad-hoc + one subscription-resolved coupon, per CR-04b's test).
- Action: `RemoveAdHocCoupon` on the ad-hoc one.
- Assert: `CouponApplication` row gone; `TotalDiscount` recomputed to $15 (as if the ad-hoc one never applied).

**Test:** `TestRemoveAdHocTax_RevertsTax`
- Same shape for `TaxApplied`/`TotalTax`.
- Type: service integration tests.

---

## CR-11 — Existing validation preserved

**Test:** `TestAddLineItem_RejectsNegativeAmountOrQuantity`, `TestUpdateLineItem_RejectsNegativeAmountOrQuantity`
- Action: call with `Amount = -$10` or `Quantity = -1`.
- Assert: rejected with the same validation error `InvoiceLineItem.Validate()` already produces for negative values.
- Type: service unit tests.

---

## CR-12 — Tenant isolation

**Test:** `TestEditEndpoints_TenantIsolation`
- Setup: two tenants (T1, T2); T1 has a draft invoice.
- Action: call each of the 7 service methods with T2's tenant context against T1's invoice ID.
- Assert: not-found error for every call; no cross-tenant read or write occurs.
- Type: service integration test.

---

## CR-13 — Concurrency safety

**Test:** `TestConcurrentLineItemEdits_Serialize`
- Setup: single draft invoice.
- Action: two goroutines call `UpdateLineItem` on the same line item simultaneously with different values.
- Assert: both complete without error (serialized via `GetForUpdate`, not lost-update); final state reflects one edit fully, then the other fully — not a merge of partial writes; invoice `Total` is internally consistent with whichever edit landed last.
- Type: service integration test with real DB row locking.

---

## Coverage gate
Before merge, all tests above must be green (`make test`). Missing test = block merge. `make lint-ci` must also pass (LL006: every new `Error(...)` log call needs a literal `"error"` key).
