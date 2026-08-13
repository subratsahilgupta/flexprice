package service

// subscription_group_discount_matrix_test.go — the discount matrix across standalone subscriptions
// and grouped-invoicing hierarchies. Each case states where the coupon is attached and which line
// items it is expected to reach; together they pin the blast radius of a price-scoped coupon,
// which is the whole risk in a group that shares plans and addon prices.

import (
	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/coupon"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

const (
	matrixAddonID      = "addon_matrix_shared"
	matrixAddonPriceID = "price_addon_matrix_shared"
	matrixAddonAmount  = 15
	matrixChildPlanFee = 30
)

// matrixGroup is the hierarchy scenarios 3-5 share: a parent on the seat plan carrying the addon
// twice, and two children on a different plan carrying it once each. Every addon line item across
// all three subscriptions sits behind one price.
type matrixGroup struct {
	parentID  string
	childIDs  []string
	addonLine map[string]int // subscription ID -> addon line items it owns
}

func (s *SubscriptionServiceSuite) seedChildPlan(planID string) *plan.Plan {
	return s.seedFixedPricePlan(planID, decimal.NewFromInt(matrixChildPlanFee), 0)
}

// createMatrixGroup builds the hierarchy. parentCoupons applies at the parent, childCoupons at
// every child — either may be empty.
func (s *SubscriptionServiceSuite) createMatrixGroup(
	suffix string,
	parentCoupons []dto.SubscriptionCouponInput,
	childCoupons []dto.SubscriptionCouponInput,
) *matrixGroup {
	ctx := s.GetContext()
	seatPlan := s.setupSeatFeePlan()
	childPlan := s.seedChildPlan("plan_matrix_child_" + suffix)

	ws1 := s.seedChildCustomer("ext_matrix_ws1_" + suffix)
	ws2 := s.seedChildCustomer("ext_matrix_ws2_" + suffix)

	childReq := func(externalID string) dto.GroupedInvoicingChildRequest {
		return dto.GroupedInvoicingChildRequest{
			PlanID:             childPlan.ID,
			ExternalCustomerID: externalID,
			SubscriptionCreationConfig: dto.SubscriptionCreationConfig{
				Addons:              []dto.AddAddonToSubscriptionRequest{{AddonID: matrixAddonID}},
				SubscriptionCoupons: childCoupons,
			},
		}
	}

	resp, err := s.service.CreateSubscription(ctx, dto.CreateSubscriptionRequest{
		CustomerID:         s.testData.customer.ID,
		PlanID:             seatPlan.ID,
		StartDate:          lo.ToPtr(s.testData.now),
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingCycle:       types.BillingCycleAnniversary,
		CollectionMethod:   lo.ToPtr(types.CollectionMethodSendInvoice),
		SubscriptionCreationConfig: dto.SubscriptionCreationConfig{
			Addons: []dto.AddAddonToSubscriptionRequest{
				{AddonID: matrixAddonID},
				{AddonID: matrixAddonID},
			},
			SubscriptionCoupons: parentCoupons,
		},
		Inheritance: &dto.SubscriptionInheritanceConfig{
			GroupedInvoicingChildrenToCreate: []dto.GroupedInvoicingChildRequest{
				childReq(ws1.ExternalID),
				childReq(ws2.ExternalID),
			},
		},
	})
	s.Require().NoError(err)

	children := s.groupedChildrenOf(resp.ID)
	s.Require().Len(children, 2)

	group := &matrixGroup{
		parentID:  resp.ID,
		childIDs:  lo.Map(children, func(c *subscription.Subscription, _ int) string { return c.ID }),
		addonLine: map[string]int{resp.ID: 2},
	}
	for _, id := range group.childIDs {
		group.addonLine[id] = 1
	}
	return group
}

// invoiceFor bills a subscription for its current period and returns the computed invoice.
func (s *SubscriptionServiceSuite) invoiceFor(subID string) *dto.InvoiceResponse {
	ctx := s.GetContext()
	sub, err := s.GetStores().SubscriptionRepo.Get(ctx, subID)
	s.Require().NoError(err)

	inv, _, err := s.createInvoiceService().CreateSubscriptionInvoice(ctx, &dto.CreateSubscriptionInvoiceRequest{
		SubscriptionID: sub.ID,
		PeriodStart:    sub.CurrentPeriodStart,
		PeriodEnd:      sub.CurrentPeriodEnd,
		ReferencePoint: types.ReferencePointPeriodStart,
	}, nil, types.InvoiceFlowManual, false)
	s.Require().NoError(err)
	s.Require().NotNil(inv)
	return inv
}

func addonLinesOf(inv *dto.InvoiceResponse) []*dto.InvoiceLineItemResponse {
	return lo.Filter(inv.LineItems, func(li *dto.InvoiceLineItemResponse, _ int) bool {
		return lo.FromPtr(li.PriceID) == matrixAddonPriceID
	})
}

// discountedLineIDs returns the IDs of every line item carrying any discount.
func discountedLineIDs(inv *dto.InvoiceResponse) []string {
	return lo.FilterMap(inv.LineItems, func(li *dto.InvoiceLineItemResponse, _ int) (string, bool) {
		total := li.LineItemDiscount.Add(li.InvoiceLevelDiscount)
		return li.ID, total.GreaterThan(decimal.Zero)
	})
}

func (s *SubscriptionServiceSuite) seedMatrixCoupon(id string, percentOff int) *coupon.Coupon {
	ctx := s.GetContext()
	c := &coupon.Coupon{
		ID:            id,
		Name:          id,
		Type:          types.CouponTypePercentage,
		Cadence:       types.CouponCadenceForever,
		PercentageOff: lo.ToPtr(decimal.NewFromInt(int64(percentOff))),
		CouponCode:    lo.ToPtr(id),
		EnvironmentID: types.GetEnvironmentID(ctx),
		BaseModel:     types.GetDefaultBaseModel(ctx),
	}
	c.Status = types.StatusPublished
	s.Require().NoError(s.GetStores().CouponRepo.Create(ctx, c))
	return c
}

// ── 1. Standalone subscription, coupon on its plan charge ──────────────────────────────────────

func (s *SubscriptionServiceSuite) TestDiscountMatrix_1_StandalonePlanCharge() {
	ctx := s.GetContext()
	seatPlan := s.setupSeatFeePlan()
	c := s.seedMatrixCoupon("coupon_matrix_1", 100)

	planPrices, err := s.GetStores().PriceRepo.List(ctx, &types.PriceFilter{
		QueryFilter: types.NewNoLimitQueryFilter(),
		EntityType:  lo.ToPtr(types.PRICE_ENTITY_TYPE_PLAN),
		EntityIDs:   []string{seatPlan.ID},
	})
	s.Require().NoError(err)
	s.Require().NotEmpty(planPrices)
	planPriceID := planPrices[0].ID

	resp, err := s.service.CreateSubscription(ctx, dto.CreateSubscriptionRequest{
		CustomerID:         s.testData.customer.ID,
		PlanID:             seatPlan.ID,
		StartDate:          lo.ToPtr(s.testData.now),
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingCycle:       types.BillingCycleAnniversary,
		SubscriptionCreationConfig: dto.SubscriptionCreationConfig{
			SubscriptionCoupons: []dto.SubscriptionCouponInput{
				{CouponCode: *c.CouponCode, PriceID: lo.ToPtr(planPriceID)},
			},
		},
	})
	s.Require().NoError(err)

	inv := s.invoiceFor(resp.ID)
	planLines := lo.Filter(inv.LineItems, func(li *dto.InvoiceLineItemResponse, _ int) bool {
		return lo.FromPtr(li.PriceID) == planPriceID
	})
	s.Require().Len(planLines, 1)
	s.True(planLines[0].Amount.Equal(planLines[0].LineItemDiscount),
		"plan charge should be fully discounted: amount %s, discount %s", planLines[0].Amount, planLines[0].LineItemDiscount)
}

// ── 2. Standalone subscription, two addons on one price, coupon sent once ──────────────────────

func (s *SubscriptionServiceSuite) TestDiscountMatrix_2_StandaloneBothAddonsFromOneCoupon() {
	ctx := s.GetContext()
	seatPlan := s.setupSeatFeePlan()
	s.seedSharedAddon(matrixAddonID, matrixAddonPriceID)
	c := s.seedMatrixCoupon("coupon_matrix_2", 100)

	resp, err := s.service.CreateSubscription(ctx, dto.CreateSubscriptionRequest{
		CustomerID:         s.testData.customer.ID,
		PlanID:             seatPlan.ID,
		StartDate:          lo.ToPtr(s.testData.now),
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingCycle:       types.BillingCycleAnniversary,
		SubscriptionCreationConfig: dto.SubscriptionCreationConfig{
			Addons: []dto.AddAddonToSubscriptionRequest{
				{AddonID: matrixAddonID},
				{AddonID: matrixAddonID},
			},
			SubscriptionCoupons: []dto.SubscriptionCouponInput{
				{CouponCode: *c.CouponCode, PriceID: lo.ToPtr(matrixAddonPriceID)},
			},
		},
	})
	s.Require().NoError(err)

	inv := s.invoiceFor(resp.ID)
	addonLines := addonLinesOf(inv)
	s.Require().Len(addonLines, 2, "both addon attachments should be charged")
	for _, li := range addonLines {
		s.True(li.Amount.Equal(li.LineItemDiscount),
			"one coupon on the price must cover both addon lines: line %s amount %s discount %s",
			li.ID, li.Amount, li.LineItemDiscount)
	}
}

// ── 3. Parent-level price coupon must reach the same price on every child ──────────────────────

func (s *SubscriptionServiceSuite) TestDiscountMatrix_3_ParentPriceCouponReachesChildren() {
	s.seedSharedAddon(matrixAddonID, matrixAddonPriceID)
	c := s.seedMatrixCoupon("coupon_matrix_3", 100)

	group := s.createMatrixGroup("s3",
		[]dto.SubscriptionCouponInput{{CouponCode: *c.CouponCode, PriceID: lo.ToPtr(matrixAddonPriceID)}},
		nil,
	)

	inv := s.invoiceFor(group.parentID)
	addonLines := addonLinesOf(inv)
	s.Require().Len(addonLines, 4, "two addon lines on the parent plus one per child")

	for _, li := range addonLines {
		s.True(li.Amount.Equal(li.LineItemDiscount),
			"parent price coupon must discount every line behind that price: line %s (sub %s) amount %s discount %s",
			li.ID, lo.FromPtr(li.SubscriptionID), li.Amount, li.LineItemDiscount)
	}
}

// ── 4. Child-level price coupon must stay on that child ────────────────────────────────────────

func (s *SubscriptionServiceSuite) TestDiscountMatrix_4_ChildPriceCouponStaysOnThatChild() {
	s.seedSharedAddon(matrixAddonID, matrixAddonPriceID)
	c := s.seedMatrixCoupon("coupon_matrix_4", 100)

	// Attached to both children by createMatrixGroup, so scope it back to one afterwards by
	// asserting per-subscription: only the lines of the children holding it may be discounted.
	group := s.createMatrixGroup("s4", nil,
		[]dto.SubscriptionCouponInput{{CouponCode: *c.CouponCode, PriceID: lo.ToPtr(matrixAddonPriceID)}},
	)

	inv := s.invoiceFor(group.parentID)

	for _, li := range addonLinesOf(inv) {
		owner := lo.FromPtr(li.SubscriptionID)
		if owner == group.parentID {
			s.True(li.LineItemDiscount.IsZero(),
				"the parent's own addon lines must be untouched by a child's coupon: line %s discount %s",
				li.ID, li.LineItemDiscount)
			continue
		}
		s.True(li.Amount.Equal(li.LineItemDiscount),
			"each child's addon line must be discounted by its own coupon: line %s (sub %s) amount %s discount %s",
			li.ID, owner, li.Amount, li.LineItemDiscount)
	}
}

// ── 5. Child subscription-level coupon — a known, deliberate gap ───────────────────────────────

// TestDiscountMatrix_5_ChildSubscriptionLevelCouponIsNotApplied documents what a child's
// subscription-level coupon does today: nothing. It has no line item to anchor to, and an
// invoice-level coupon discounts the entire invoice, so merging it would reach the parent's and
// the siblings' charges. Confining it needs invoice coupons to carry the set of lines they may
// touch, plus a distributor that lets scopes compose — deliberately out of scope for now.
//
// Discounting nothing is the safe failure: it under-discounts visibly instead of quietly
// discounting charges the coupon was never meant to reach. Price-scoped coupons (scenarios 3
// and 4) are the supported way to discount a child.
func (s *SubscriptionServiceSuite) TestDiscountMatrix_5_ChildSubscriptionLevelCouponIsNotApplied() {
	s.seedSharedAddon(matrixAddonID, matrixAddonPriceID)
	c := s.seedMatrixCoupon("coupon_matrix_5", 50)

	group := s.createMatrixGroup("s5", nil,
		[]dto.SubscriptionCouponInput{{CouponCode: *c.CouponCode}},
	)

	// The association is still recorded against each child — only the billing side declines it.
	for _, childID := range group.childIDs {
		assocs, err := s.GetStores().CouponAssociationRepo.List(s.GetContext(), &types.CouponAssociationFilter{
			QueryFilter:     types.NewNoLimitQueryFilter(),
			SubscriptionIDs: []string{childID},
			CouponIDs:       []string{c.ID},
		})
		s.NoError(err)
		s.Len(assocs, 1, "child %s should still hold the association", childID)
	}

	inv := s.invoiceFor(group.parentID)

	s.Empty(discountedLineIDs(inv), "no line may be discounted by a child's subscription-level coupon")
	s.True(inv.TotalDiscount.IsZero(), "invoice discount should be zero, got %s", inv.TotalDiscount)
}
