# Proration unification — ERD

**Date:** 2026-10-03 · **Author:** Harshit Gupta · **Branch:** `feat/proration`

**Goal:** charges, credit grants (CG) and entitlement grants (EG) all prorate with one rule. It covers calendar and anniversary billing, short first periods, short last periods and mid-period changes.

**Terms:**

- A period is written `[start, end)`: the start is included, the end is not.
- A **full period** is one regular billing period.
- **Fraction** = time used ÷ length of the full period.

Unless stated otherwise, examples use $31/month (1 day = $1 in a 31-day month), a 1000-credit monthly CG and a 100-unit monthly EG.

---

## Issues



### 1. Without multiple cadences (dates, trials, validation)


| #   | Issue                                                  | Example                                                                                                                                                         | Status                           |
| --- | ------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------- |
| 1.1 | `NextBillingDate` produces long first periods          | Annual sub, start Mar 15 2026, anchor Jun 1 2026: first period ends **Jun 1 2027**. Quarterly sub with a backdated anchor: Jan 1 → Apr 22, then the dates drift | ✅                                |
| 1.2 | `PreviousBillingDate` ignores the timezone             | Anchor Mar 1 IST gives Jan 29 IST, not Feb 1. In New York it is 1 hour off around DST                                                                           | ✅                                |
| 1.3 | Trial end is computed in UTC                           | New York: Mar 1 + 14 days = Mar 15 **01:00** local                                                                                                              | ✅                                |
| 1.4 | Calendar subs become anniversary after a trial         | Sub from Jan 15 with a 14-day trial bills Jan 29 → Feb 28, not Jan 29 → Feb 1                                                                                   | ✅                                |
| 1.5 | An addon dated before the sub start is silently moved  | —                                                                                                                                                               | ✅ rejected                       |
| 1.6 | Addon attach accepts any past date                     | Credit grants are created for every past period; no entitlement grant is created **(live)**                                                                     | ✅ addons · ⏸ line item API       |
| 1.7 | Backdating with `create_prorations` is always rejected | `dto/subscription.go:1361` compares two pointers                                                                                                                | ⏸ pointer fixed, still rejected  |
| 1.8 | A one-time item added mid-period is never billed       | With `create_prorations` the attach fails instead                                                                                                               | ✅ `create_prorations` · ⏸ `none` |




### 2. Proration (single cadence)

**2.1 Each path divides by a different period** ✅. Calendar sub from Jan 15, addon added Jan 20:


| What                | Before                                            | Correct                  |
| ------------------- | ------------------------------------------------- | ------------------------ |
| Plan charge         | 17/31                                             | 17/31                    |
| Addon charge        | 12/31                                             | 12/31                    |
| Addon CG / EG       | **12/17** (live 0.99996)                          | 12/31                    |
| Plan CG at creation | **1000**; cycle Jan 15 → Feb 15 **(live)**        | 548.39; next grant Feb 1 |
| Cancel Jan 20       | **full refund** (live: $317.63 instead of $89.63) | credit 12/31             |



| #   | Issue                                                               | Before                                        | Correct                   | Status |
| --- | ------------------------------------------------------------------- | --------------------------------------------- | ------------------------- | ------ |
| 2.2 | Anniversary sub with anchor after start (Jan 10 / Jan 20)           | $31 **(live)**                                | $10                       | ✅      |
| 2.3 | Periods 2+ under-charged (the anchor never moves)                   | $23.64 (live $26.28)                          | $31                       | ✅      |
| 2.4 | Short last period: sub end date or cancelled sub, `[Mar 1, Mar 10)` | $31                                           | $9                        | ✅      |
| 2.5 | Tiered and package prices                                           | volume **$0.00**, package $76.77              | $54.84 / $21.94           | ✅      |
| 2.6 | Addon window measured one month forward                             | Jan 31 → 1/28; New York DST $28.77 **(live)** | 1/31; $28.81              | ✅      |
| 2.7 | `none` still prorates addons dated mid-period at creation           | $19 **(live)**                                | $0 until next period      | ✅      |
| 2.8 | Grant rounding                                                      | credits stored to 8 dp, EG quota not floored  | currency precision; floor | ✅      |
| 2.9 | Plan EG at creation ignores the short first period                  | 100                                           | 54                        | ⏸      |




### 3. Multiple cadences

Scope: the item cadence is at most the sub cadence and divides it evenly.


| #   | Issue                                                      | Today                                                                                                                                                                | Correct                                                                                        | Status |
| --- | ---------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- | ------ |
| 3.1 | Quarterly item on an annual calendar stub (Mar 15 → Jan 1) | one $90 line **(live)**                                                                                                                                              | 4 windows: $17 + 3 × $90                                                                       | ✅      |
| 3.2 | Removing a monthly addon mid-quarter on a quarterly sub    | credit $100                                                                                                                                                          | $150                                                                                           | ⏳      |
| 3.3 | Cancel prorates every item against the sub's period        | monthly item on a quarterly sub: $50                                                                                                                                 | $150                                                                                           | ⏳      |
| 3.4 | Cadence checks are inconsistent                            | Mixed cadences are rejected at creation with `create_prorations` but accepted when added later. The line item API accepts longer cadences, which then break invoices | Allowed only if the item cadence is at most the sub cadence and divides it; otherwise rejected | ⏳      |


---

## Billing grid

Every date and full-period calculation runs on one grid (`types.billingPeriodGrid`): the billing dates `anchor + k × (count × period)` for any integer k, in the sub's timezone.

**1. The grid.** Monthly, anchor Jan 31. Each date is computed from the anchor, never from the previous date, so a short month does not drift the schedule.

```
 k:         -1          0           1           2           3
 ───────────┼───────────┼───────────┼───────────┼───────────┼──────▶ time
          Dec 31      Jan 31      Feb 28      Mar 31      Apr 30
                      anchor
```

**2. `billingDateAtIndex(k)`: index → date.** Month-based periods clamp to month end; daily and weekly periods add days.

```
 k = -1   Jan 31 − 1 month                  = Dec 31   (PreviousBillingDate)
 k =  1   Jan 31 + 1 month  = "Feb 31" ─clamp─▶ Feb 28
 k =  2   Jan 31 + 2 months                 = Mar 31   (back on the anchor day)
```

**3. `periodIndexAfter(t)`: date → index, and the full period around it.** Calendar monthly sub from Jan 15, anchor Feb 1.

```
 k:          -1                0                1
 ────────────┼────────▲────────┼────────────────┼──────▶ time
           Jan 1      │      Feb 1            Mar 1
                    t = Jan 15

 periodIndexAfter(Jan 15)  = 0             first billing date strictly after t
 FullBillingPeriod(Jan 15) = [dateAt(-1), dateAt(0)) = [Jan 1, Feb 1)
 serviceable period        = [Jan 15, Feb 1)  →  coefficient 17/31
```

`NextBillingDate(start)` is `dateAt(periodIndexAfter(start))`. A `t` exactly on a billing date belongs to the period that starts there.

---

## Decisions

**D1. One rule.** Fraction = seconds used ÷ seconds of the full period. This applies to plan and addon charges, credits on removal and cancel, CGs and EGs.
*Example:* an addon added Jan 20 in `[Jan 15, Feb 1)`. Charge, CG and EG all use 12/31.

**D2. The full period is the regular period, on the sub's billing schedule, that contains the window.**

- Short first period: `[Prev(end), end)`.
- Short last period: `[start, Next(start))`.

Starting a sub on day X costs the same as adding an addon on day X, and the parts of a period always add up to 1.
*Examples:*

- Calendar sub from Jan 15: **$17**.
- Anniversary sub from Jan 10 with anchor Jan 20: the full period is `[Dec 20, Jan 20)`, so **$10**.
- Calendar sub from Jan 31: **$1** (1/31). Stripe and Chargebee give 1/28; Zuora, Orb and Lago give 1/31.
- Sub ending Mar 10: **$9**.

**D3. Real seconds, with boundaries in the customer's timezone.** This is what Stripe, Recurly and Chargebee do. The DST effect is accepted: New York `[Mar 17, Apr 1)` = 0.4845, against 0.4839 counted in days.

**D4. Grants follow the charge.** A CG or EG uses the charge's fraction when `create_prorations` is set and its cadence equals the sub's. In a short first period, the grant cycle is anchored to the billing anchor. Credits round to currency precision; EG quotas round down.
*Examples:*

- 548.39 credits, with the next grant on Feb 1.
- EG quota floor(54.8) = 54.
- An annual CG on a monthly sub gets the full grant.

**D5. One flag,** `proration_behavior`**.** `none` means no partial amounts anywhere. An item that starts mid-period is free until the next period, then billed in full. This holds whether the addon is dated mid-period at creation or attached later.
*Example:* with `none`, a sub from Jan 15 pays $31 and gets 1000 credits and 100 units. An addon dated Jan 20 costs $0 until Feb 1.

**D6. The anchor must fall in** `[start, start + 1 period]`**, compared on local dates.** No backdated anchors: backdate the start instead. Stripe imports keep Stripe's anchor as-is; the date rule (D7) works from any anchor.
*Example:* monthly sub from Jan 10. Jan 20 and Feb 10 are allowed; Mar 5 and Dec 20 are rejected.

**D7. One date rule.** Billing dates are `anchor + k × period`, clamped to month end and then returning to the anchor day: Jan 31 → Feb 28 → Mar 31. Stripe, Chargebee, Recurly, Zuora and Orb work the same way. Calendar billing is this rule with the anchor at the 1st, 00:00 local. The first billing date is the anchor.

**D8. Change dates.**

- At creation: on or after the sub start.
- After creation: on or after `CurrentPeriodStart`, as in Stripe and Chargebee.

Anything earlier is rejected, not moved. The line item API is not checked yet: system rollouts (`prepare_processed_events`) backdate through it.

**D9. Backdated subs.** Backdating is unlimited, with one invoice per elapsed period (as today). Grants cover the current period only.
*Example* (`create_prorations`): an addon starts on day 10 of period 2 of 3.

- Period 1: sub only.
- Period 2: sub + addon × 20/30.
- Period 3: both in full.

**D10. Trials.**

- No proration during the trial.
- Trial end is computed in local days.
- **Calendar subs** keep the calendar anchor, and the short period gets its own invoice:
  - `[Jan 15, Jan 29)`: $0.
  - `[Jan 29, Feb 1)`: $3 plus 3/31 of the CG.
  - February: full.
- **Anniversary subs** re-anchor at trial end, so the first period is full: `[Jan 29, Feb 28)`, then `[Feb 28, Mar 29)`.

**D11. Never prorated:**

- usage charges
- commitments and minimums
- one-time items (billed once, in full)
- grants whose cadence differs from the sub's

**D12. Item cadence must be at most the sub cadence and divide it.** Anything else is rejected everywhere, e.g. an annual item on a monthly sub. Mixed cadences are allowed with `create_prorations`; each item is prorated on its own windows.
*Example:* a monthly $31 item on a calendar quarterly sub from Feb 15 bills $15.50 (14/28), then $31 per month.

**D13. Existing subscriptions keep their stored periods.** Fixes apply to new subscriptions and to future period computation.

**Out of scope:**

- plan change v1
- the quantity-change endpoint (deprecated)
- legacy entitlement overrides
- pause/resume

---



## Follow ups

1. Separate proration flags for CG and EG, falling back to the charge flag.
2. Pause/resume proration. Today the period end is only pushed out, and auto-resume may never run.
3. Customer timezone changes after subscriptions exist. Meter usage reads the customer's timezone, but subscriptions keep their own.
4. A 30/360 day-count option.
5. Commitment proration in short periods.
6. Backdated changes into invoiced periods, with adjustments (Zuora/Orb style).
7. Deferred from this work:
  - Plan EG at creation (2.9): open it through the subscription grants service, as addons do.
  - Backdating with `create_prorations` (1.7, D9).
  - Change-date check on the line item API (1.6, D8), which needs a separate path for system callers.
  - One-time advance item attached with `none` is never billed (1.8).

