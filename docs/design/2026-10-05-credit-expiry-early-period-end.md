# Credit expiry settlement v2: periods that end early

Status: built. Linear: FLE-1498. Builds on [v1](2026-09-26-mid-period-credit-expiry.md).

## The problem

With the setting on, an expiring credit pays the usage from before its expiry on the period's open
draft (a `SUBSCRIPTION_CYCLE` invoice for the subscription's current period). The period-end run
finds that draft by its idempotency key: subscription, billing reason, period.

Flows that end or move the period early raised a different invoice and left the draft behind:
the usage before the expiry was billed again from the wallet, and the draft, holding the credits
applied at expiry, was never finalized.

Example: period 1 Oct - 1 Nov, 100 purchased + 30 free credits expiring 15 Oct, 20 of usage before
the expiry, 10 after. The expiry applies 20 to the draft. Cancelling on 20 Oct should leave 90; it
left 70.

## The rule

**The open draft follows the period.** When a flow ends or moves the period, it moves the draft's
period end (and re-keys it) so the flow bills that draft instead of a new invoice. Where the flow
raises no invoice, the draft is voided.

| Flow | Before | Now |
|---|---|---|
| Immediate cancel, `generate_invoice` | New `PRORATION` invoice, draft left behind | Draft moved to `[start, cancel]`, reason `PRORATION`; the cancel finalizes it |
| Immediate cancel, `skip` (also auto-cancel, Stripe cancel) | No invoice, draft left behind | Draft voided; applied credits refunded |
| Scheduled cancel inside the period | Period end moved, draft left behind | Draft end moved with it; moved back if the schedule is cancelled |
| Resume after a pause | Period end pushed out, draft left behind | Draft end moved with it |
| Threshold invoice | Threshold subscriptions skipped by settlement | Draft becomes the threshold invoice; the run waits while an expiring credit is pending; threshold subscriptions included |
| Plan change v2, `anchor_at_effect` | One-off invoice for the old plan's usage, draft left behind | Old period billed like an immediate cancel: draft moved to `[start, change]`, reason `PRORATION`, computed and finalized |
| Cancel at period end, plan change v2 `unchanged`, deferred plan change | Period unchanged | Unchanged: the draft is reused at period end |

Trial changes need nothing: settlement skips trialing subscriptions, so no draft exists.

## Plan change with `anchor_at_effect`

The change restarts the period at the change date, so the old period never reaches its end. It is
now billed exactly as an immediate cancel bills its cut-short period (Cancel reference point:
arrear charges up to the change), from the open draft if there is one. It is finalized inside the
change and paid after commit with the change's other invoices.

Differences from before, for every tenant:
- The old period's invoice is a subscription invoice with reason `PRORATION` (was a one-off,
  `SUBSCRIPTION_UPDATE`).
- It includes the old plan's arrear fixed charges for the cut-short period, which were billed
  nowhere before. Pricing them for a partial period is being fixed separately (immediate cancels
  bill them at the full period amount today); this flow picks that fix up automatically.
- The preview quotes the same charges.

## Building blocks

- `invoiceService.MoveCycleDraft(sub, newEnd, reason)`: finds the open cycle draft for the
  subscription's current period and sets its period end, billing reason and idempotency key, under
  the same row lock as `ComputeInvoice`. No-op without a draft.
- `invoiceService.VoidCycleDraft(sub)`: voids that draft via `VoidInvoice`.
- `InvoiceRepo.UpdateDraftInvoicePeriod`; invoice `period_end` is no longer immutable in the ent
  schema (Go-level only, no migration).
- The subscription invoice idempotency key is built by one helper, `subscriptionInvoiceKey`.
- `HasPendingExpiringCredit` checks the setting itself.

These only act when an open draft exists for the current period mid-period, which only expiry
settlement creates; tenants with the setting off see no change except the plan change rows above.

## Tests

`credit_expiry_period_end_test.go`: immediate cancel with and without an invoice, scheduled cancel
and its revert, resume, threshold, threshold waiting for a pending expiry.
`subscription_change_v2_reset_test.go`: the cut-short period billed on its open draft, keeping the
credits applied at expiry.

The in-memory invoice store's period-range filter is fixed to match the ent repository (an invoice
whose period lies inside the range); it matched the opposite before.

## Not handled

- Immediate cancel or plan change within ~2h after a credit expires: the expiry job has not applied
  the credit yet, and these user actions can't wait for it. That usage is paid from other credits.
- Backdated subscriptions (a period that ended before the subscription was created).
