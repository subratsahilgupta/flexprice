package service

import (
	"github.com/shopspring/decimal"
)

// splitCurveAtCommitment divides one usage line item's daily curve into the
// engine's two halves: usage billed within the commitment (its normal line's
// amount) and usage billed as overage (its overage line's amount). Days until
// the item's cumulative charge reaches normalAmount are normal; the excess
// accrues on the overage curve, scaled so each half sums exactly to its
// engine line. List-rate value moves with the quantity (a unit's value never
// counts in both halves), and TierDelta absorbs the overage factor and any
// engine rounding, keeping the row identity exact on every day.
func splitCurveAtCommitment(curve []dayCharge, normalAmount, overageAmount, rate decimal.Decimal) (normal, overage []dayCharge) {
	if len(curve) == 0 {
		return nil, nil
	}

	// Pre-factor overage value per day: how much of the raw charge exceeds
	// the within-commitment amount.
	last := curve[len(curve)-1]
	prefactorTotal := last.CumulativeCharge.Sub(normalAmount)
	if prefactorTotal.IsNegative() {
		prefactorTotal = decimal.Zero
	}
	overageScale := decimal.Zero
	if prefactorTotal.IsPositive() && !overageAmount.IsZero() {
		overageScale = overageAmount.Div(prefactorTotal)
	}

	normal = make([]dayCharge, len(curve))
	overage = make([]dayCharge, len(curve))
	for i, dc := range curve {
		prefactor := dc.CumulativeCharge.Sub(normalAmount)
		if prefactor.IsNegative() {
			prefactor = decimal.Zero
		}
		overQty := decimal.Zero
		if rate.IsPositive() {
			overQty = prefactor.Div(rate)
		}

		normalCharge := dc.CumulativeCharge
		if normalCharge.GreaterThan(normalAmount) {
			normalCharge = normalAmount
		}
		normalList := dc.CumulativeGrossQty.Mul(rate).Sub(prefactor)
		normal[i] = dayCharge{
			Day:                      dc.Day,
			CumulativeCharge:         normalCharge,
			CumulativeGrossQty:       dc.CumulativeGrossQty,
			CumulativeBillableQty:    dc.CumulativeBillableQty.Sub(overQty),
			CumulativeEntitlementQty: dc.CumulativeEntitlementQty,
			UsageAtListRate:          normalList,
			TierDelta:                normalCharge.Sub(normalList).Add(dc.CumulativeEntitlementQty.Mul(rate)),
		}

		overCharge := prefactor.Mul(overageScale)
		overage[i] = dayCharge{
			Day:                   dc.Day,
			CumulativeCharge:      overCharge,
			CumulativeBillableQty: overQty,
			UsageAtListRate:       prefactor,
			TierDelta:             overCharge.Sub(prefactor),
		}
	}

	// Pin both endings to the engine's own amounts so marginal rows sum with
	// zero residual under rounding.
	normal[len(normal)-1].CumulativeCharge = normalAmount
	normal[len(normal)-1].TierDelta = normalAmount.
		Sub(normal[len(normal)-1].UsageAtListRate).
		Add(normal[len(normal)-1].CumulativeEntitlementQty.Mul(rate))
	overage[len(overage)-1].CumulativeCharge = overageAmount
	overage[len(overage)-1].TierDelta = overageAmount.Sub(overage[len(overage)-1].UsageAtListRate)

	return normal, overage
}
