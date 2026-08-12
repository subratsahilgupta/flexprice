# Plan Change v2 — v0 Implementation Plan

Status: **Proposed**
Date: 2026-08-12
Design: [2026-08-11-plan-change-v2-erd.md](2026-08-11-plan-change-v2-erd.md) — that document is the
design; this one is the build order. Section references (§) point at it.

---

## 1. What v0 is

**Simple plan change + addons.** Swap `subscriptions.plan_id` in place, slice plan line items at the
effective date, keep the anchor. Addons carry by default, can be dropped, and new ones can be
attached in the same request.

**In v0**


| Capability                   | Detail                                                         |
| ---------------------------- | -------------------------------------------------------------- |
| Same-interval plan change    | immediate only                                                 |
| Preview + execute            | identical request type, one settlement path                    |
| Addon dispositions           | `carry` (default) / `drop`, keyed by `addon_associations.id`   |
| Attach addons at change time | `addendum_config.addons`                                       |
| One netted invoice           | charges − credits, credits as negative lines, zero → no invoice |

**Not in v0** — each 4xx's or is deferred, none is silently half-done:

- Interval / cadence / period-count / billing-cycle / currency change → 4xx, hint names v1
- Hierarchy subscriptions, phases, paused subscriptions → 4xx
- `change_at = end_of_period` → 4xx in v0, arrives with the scheduler collapse
- `checkout` field → reserved on the request, 400 if set; payment gating comes later
- Tier offset for split usage lines (§6) — restart accepted, test 26 pins it
- Fixed-arrear addon drop charge (§3) — bills nothing today, test 11 pins it
- Coupon / tax / credit-grant / price / entitlement dispositions — all CARRY, zero operations
- Stripe inbound repoint, `subscription_associations` table, v1 deprecation

---

## 2. Phases


| Phase | Name                   | Ships alone? | Depends on |
| ----- | ---------------------- | ------------ | ---------- |
| 0     | Credit basis, on v1    | yes          | —          |
| 1     | Swap enablement        | yes          | —          |
| 2     | Core swap engine       | no           | 0, 1       |
| 3     | Addon dispositions     | no           | 2          |
| 4     | API surface + SDK      | no           | 2, 3       |
| 5     | Verification + rollout | —            | 4          |


Phase 0 is entirely v1-side and invisible to callers. Phase 1 is schema only. Neither depends on the
other, so both can start immediately and in parallel with design review of phase 2.

---

## 3. Phase 0 — Credit basis, on v1

**Goal:** fix the one money bug that swap-in-place compounds, while restart still masks it. Nothing
here changes the v1 API.

Scope is deliberately just this. The two addon columns previously listed here are **not**
prerequisites for the swap: `credit_grants.addon_association_id` only matters once addons can be
dropped, so it moved to phase 3, and `addon_associations.quantity` is hygiene — the `addon_quantity`
metadata is written in two places and read in none, while proration already uses the line item's
`Quantity` — so it moved to §10.

Credit is computed from the list price and the cap never binds (§10).

- Populate `SubscriptionLineItemID` in `buildChargeLineItem`
  ([line_item_proration.go:221](../../internal/ee/service/line_item_proration.go#L221)). Regular
  invoices already set it ([billing.go:289](../../internal/ee/service/billing.go#L289)); proration
  invoices leave it NULL, so the join below would see only regular invoices. **Do this first** —
  everything else in 0.1 depends on it.
- Implement `getOriginalAmountPaidForLineItem` and `getPreviousCreditsForLineItem`, already stubbed
  commented-out at [proration.go:441,454](../../internal/ee/service/proration.go#L441). Both join
  `invoice_line_items` on the immutable `subscription_line_item_id` FK.
- Replace `originalAmountPaid := price.Amount.Mul(item.Quantity)`
  ([proration.go:451](../../internal/ee/service/proration.go#L451)) and the same expression at
  [line_item_proration.go:210](../../internal/ee/service/line_item_proration.go#L210).

**Exit:** a second proration credit in one period is bounded by what was actually invoiced. Unit test
covering two consecutive changes; the cap in `capCreditAmount`
([calculator.go:179](../../internal/domain/proration/calculator.go#L179)) now binds.

---

## 4. Phase 1 — Swap enablement

**Goal:** make an in-place plan change physically possible. No endpoint yet.

- `ent/schema/subscription.go:46` — drop `.Immutable()` on `plan_id`; add it to the repository
  `Update` field list. `make generate-ent && make generate-migration`.
- **Reset `synced_price_sequence` whenever `plan_id` changes.** The watermark is only meaningful
  relative to a plan, and the discovery filter is `synced_price_sequence < TargetSeq`
  ([planpricesync/repository.go:92](../../internal/domain/planpricesync/repository.go#L92)) — a
  carried value marks the sub permanently in-sync with a plan it never reconciled against. Set it to
  the target plan's current max in the same update.
- `SubRepo.GetForUpdate` mirroring
  [invoice/repository.go:15](../../internal/domain/invoice/repository.go#L15).

**Exit:** a service-layer test mutates `plan_id` inside a transaction under a row lock, and the
plan-price sync then picks the subscription up for the new plan. Test 27.

---

## 5. Phase 2 — Core swap engine, plan lines only

**Goal:** the whole change works end to end for a plan with no addons. This is the phase with the
real design content.

### 5.1 Shape — resolve / compute / settle

Three stages, in this order. Preview and execute share the first two **by calling the same
functions**, not by having parallel implementations — that is what makes quote parity structural
rather than a thing to keep in sync. It is also what v1 got wrong.

```
resolve(ctx, sub, req)   -> planChangeRequest    // no writes, no money
compute(ctx, request)    -> planChangeQuote      // no writes; line deltas + proration + net
settle(ctx, request, quote, opts)                // the only stage that writes
```

- **resolve** — load the subscription and target plan, run every precondition, apply the line key
  and four-case resolver (5.2), resolve entity dispositions (phase 3). Output describes the whole
  intended change.
- **compute** — proration entries, netted total, the `ChangedResources` shape. Pure.
- **settle** — one of:

| Settler      | When                              | Phase |
| ------------ | --------------------------------- | ----- |
| `preview`    | preview endpoint — renders, writes nothing | 2 |
| `payLater`   | execute, no checkout              | 2     |
| `payFirst`   | execute, checkout present and net > 0 | later |

The branch sits at the top of settle and nowhere else, mirroring the existing quantity-change path
([subscription_modification.go:333](../../internal/ee/service/subscription_modification.go#L333)):

```go
if checkout != nil && quote.NetAmount().GreaterThan(decimal.Zero) {
    return s.settlePayFirst(ctx, request, quote, checkout)   // phase: later
}
return s.settlePayLater(ctx, request, quote)
```

**Two constraints that must hold from day one, or adding checkout later means rewriting resolve:**

1. **`planChangeRequest` must be plain serializable data** — ids and values, no live pointers, no
   loaded entities. `settlePayFirst` persists the resolved request onto the checkout session and
   replays it on payment success ([subscription_modification_quantity.go:759](../../internal/ee/service/subscription_modification_quantity.go#L759)),
   so it must survive a round trip through JSON and re-execution minutes later.
2. **`resolve` and `compute` write nothing.** Preview is exactly execute minus settle. If either
   stage mutates, preview stops being safe and pay-first stops being replayable.

v0 ships `preview` and `payLater`. `payFirst` is a third settler added later — a new file, no change
to resolve or compute. Until then the request's `checkout` field 400s.

### 5.2 Line key (§6)

One exported function, two consumers — the engine and the response. Do not write two.

```
USAGE     -> (meter_id, sorted filter_values)
FIXED     -> lookup_key, when both prices have a non-empty one
otherwise -> price_id
```

### 5.3 Four-case resolver (§6)

| Current line            | Target line | Action                                  |
| ----------------------- | ----------- | --------------------------------------- |
| paired, identical price | —           | **no-op** — row untouched, no entry      |
| paired, different price | —           | close at `effective_at`, open successor |
| paired with nothing     | —           | close at `effective_at`                 |
| —                       | unpaired    | open at `effective_at`                  |

Row 1 is not an optimisation; it is what makes test 2 pass and what stops an unchanged service from
getting a new id on every change.

### 5.4 Preconditions

All 4xx before any write, per §2 — enforced in `resolve`, so preview rejects exactly what execute
rejects. Every hint names the v1 endpoint.

### 5.5 Single netted settlement

- Call `LineItemProrationService.Compute`, never `Apply` — `Apply` returns only `error` and settles
  charges and credits independently.
- Sum `NetAmount` across **all** entries rather than bucketing each into charge or credit
  ([line_item_proration.go:124](../../internal/ee/service/line_item_proration.go#L124)).
- Credits become negative invoice lines. Zero net with no entry moved → no invoice.
- Residual credit beyond the invoice total goes to the wallet (test 5). No new rule needed:
  `TopUpWalletForProratedCharge` ([wallet.go:2896](../../internal/ee/service/wallet.go#L2896)) already
  finds an active **prepaid** wallet matching the currency and creates one when there is none.
- **One transaction** covers closing line items, opening successors, mutating `plan_id` +
  `synced_price_sequence`, and creating the invoice — so the change either fully happens or fully
  does not. Everything external (payment attempt, provider sync, webhook publish) happens **after
  commit** and must be independently retryable. This is the failure that produced
  *"removal was persisted and the credit is UNISSUED"*
  ([subscription.go:5672](../../internal/ee/service/subscription.go#L5672)); v0 does not repeat it.

### 5.6 Endpoints

`preview` and `execute` are the same `resolve` + `compute` with a different settler (5.1). Preview
returns the `effective_at` it computed.

**Exit:** tests 1–7, 18–21, 24, 25, 27 green. `go test -race ./internal/ee/service -run TestSubscriptionChangeV2`.

---

## 6. Phase 3 — Addon dispositions

**Goal:** carry / drop / attach.

### 6.1 Types (`internal/types/`)

`EntityDisposition` (`carry`, `drop`) with `ValidateFor`, `SubscriptionChangeEntityType` (`addon`),
`DispositionReason` (`default`, `explicit_override`, `forced`). Per §2.

### 6.2 Resolver

Runs inside `resolve` (5.1). Precedence: default → per-instance override → forced. Emits one
`EntityDispositionResult` per active attachment, including carried ones.

An override key naming an unknown or inactive association is **not** an error: skip it and add a
`Warnings` entry naming the key. Rationale — a caller round-tripping a preview result into an
execute can legitimately race an association that ended in between, and failing the whole change for
a stale key is worse than ignoring it.

### 6.3 `credit_grants.addon_association_id`

Needed here, not earlier: it only matters once a plan change can drop an addon.

- New nullable immutable column, `ent/schema/creditgrant.go`.
- Backfill from `addon_id` where the subscription has exactly one association for that addon, NULL
  otherwise.
- Scope `CancelFutureSubscriptionGrants`
  ([subscription.go:5643](../../internal/ee/service/subscription.go#L5643)) on the association when
  set, falling back to `addon_id` when NULL.

**Exit:** in the overlap window of §5, dropping attachment A leaves B's grants alone. Test 16.

### 6.4 Drop inside a plan change (§3)

- Use `Compute`, fold entries into the phase-2 settlement. Removes the best-effort wallet top-up
  whose failure logs *"removal was persisted and the credit is UNISSUED"*
  ([subscription.go:5672](../../internal/ee/service/subscription.go#L5672)).
- `EffectiveDate` defaults to `effective_at`, not `sub.CurrentPeriodEnd`.
- Cancel future credit grants scoped on `addon_association_id` (6.3).
- **Guard:** do not close the entitlement grant while another active association of the same
  `addon_id` remains — they can share one grant row
  ([entitlement_grant.go:118](../../ent/schema/entitlement_grant.go#L118)).
- `proration_behavior: none` **with a drop is legal**, not a 4xx. The service stops, no money moves;
  a customer asking for no proration gets none.

### 6.5 Attach — `addendum_config.addons`

Reuses `AddAddonToSubscription`. Adding an already-attached addon stacks a second attachment by
design (§8); to replace, drop the existing one in the same request.

**Exit:** tests 8–10, 12–17 green. Test 11 asserts the known fixed-arrear gap.

---

## 7. Phase 4 — API surface

- DTOs in `internal/api/dto/subscription_change_v2.go` per §2. The attach wrapper is
  `AddendumConfig` / `addendum_config` throughout — the doc previously said `new_config` in places,
  and the wire name must be settled before anything reaches an SDK.
- Routes in `internal/api/router.go`:
  `POST /subscriptions/:id/change/v2/preview` (`@x-scope "read"`),
  `POST /subscriptions/:id/change/v2/execute` (`@x-scope "write"`).
  Both handlers do the same thing — bind, delegate, respond — and differ only in the settler they
  ask for (5.1). No branching logic in the handler.
- `IdempotencyKey` → new `idempotency.ScopePlanChange`, keyed on
  `(subscription_id, target_plan_id, effective_at, caller key)`
  ([generator.go](../../internal/idempotency/generator.go)). This, not the
  `target_plan_id == plan_id` precondition, is what makes a replay safe.
- `ChangedLineItem.LineKey` — additive optional field, keep the existing `swaggertype` and `enums`
  tags. A plan change never emits `updated`.
- Webhook `subscription.plan_changed` with `{from_plan_id, to_plan_id, effective_at}`, plus
  `subscription.updated`. **Never** `cancelled` then `created`.
- `make swagger && make sdk-all`.

**Exit:** tests 22, 23 green; swagger and SDK generation clean; v1 still callable and **not** marked
deprecated.

---

## 8. Phase 5 — Verification and rollout

- Full §12 table, with 11 and 26 asserting the two accepted limitations so the later fixes have
  failing tests to flip.
- Parity harness: same scenario through v1 and v2, compare money. Expect a deliberate difference for
  anniversary billing, where v1 moves the anchor
  ([subscription_change.go:855](../../internal/ee/service/subscription_change.go#L855)) and v2 does
  not.
- E2E per §12: `make run-local`, `make migrate-local`, Starter and Pro with matching line keys, one
  fixed-advance and one usage-arrear addon, assert **one invoice not three**.
- `make test && make lint-ci`.

---

## 9. Decisions — settled


| Question                                             | Decision                                                                         | Where |
| ---------------------------------------------------- | -------------------------------------------------------------------------------- | ----- |
| Wallet for residual credit, and when none exists     | No new rule — existing behaviour: active prepaid wallet matching currency, created if absent | 5.5 |
| Override key naming an unknown/inactive association  | Skip it, add a `Warnings` entry. Not an error                                    | 6.2   |
| `proration_behavior: none` + addon drop              | Legal. Service stops, no money moves                                             | 6.4   |
| Settlement failing after line items are written      | One transaction for all DB writes; external effects after commit, retryable      | 5.5   |


---

## 10. Backlog — deliberately not in v0

- `addon_associations.quantity`. The `addon_quantity` line-item metadata is a literal `"1"` written
  at [subscription.go:5743](../../internal/ee/service/subscription.go#L5743) and
  [subscription_line_item.go:511](../../internal/api/dto/subscription_line_item.go#L511) and **read
  nowhere**; proration uses the line item's real `Quantity`. Hygiene, not correctness.
- Tier offset for split usage lines, plus the VOLUME-mode policy (§6).
- Fixed-arrear addon drop charge (§3) — needs a new `Old* × (1 − coefficient)` branch in
  `shouldIssueCharge`, a design task rather than a flag.
- `payFirst` settler and checkout gating (5.1).
- `change_at = end_of_period` and the scheduler collapse.
- Stripe inbound repoint — needs a v2 equivalent of `OpeningInvoiceAdjustmentAmount` first.
