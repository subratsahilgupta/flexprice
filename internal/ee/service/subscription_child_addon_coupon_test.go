package service

// subscription_child_addon_coupon_test.go — a coupon attached at creation can name the line item
// it discounts via price_id. The map used to resolve that was built from plan-derived line items
// before addons, extra line items and price overrides existed, so an addon's price never matched.
// Worse, a miss was not an error: the coupon silently became subscription-level and discounted
// every charge on the subscription instead of the one line the caller asked for.

import (
	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/addon"
	"github.com/flexprice/flexprice/internal/domain/coupon"
	"github.com/flexprice/flexprice/internal/domain/coupon_association"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// seedSharedAddon publishes an addon with one recurring FIXED price, the shape both grouped
// children attach so their line items collide on a single price ID.
func (s *SubscriptionServiceSuite) seedSharedAddon(addonID, priceID string) {
	ctx := s.GetContext()

	s.Require().NoError(s.GetStores().AddonRepo.Create(ctx, &addon.Addon{
		ID:        addonID,
		LookupKey: addonID,
		Name:      "Shared Child Addon",
		BaseModel: types.GetDefaultBaseModel(ctx),
	}))

	s.Require().NoError(s.GetStores().PriceRepo.Create(ctx, &price.Price{
		ID:                 priceID,
		Amount:             decimal.NewFromInt(15),
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_ADDON,
		EntityID:           addonID,
		Type:               types.PRICE_TYPE_FIXED,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}))
}

// seedFullDiscountCoupon publishes an unlimited 100%-off coupon. Redemptions are consumed per
// association, so a coupon shared across N children needs headroom for N.
func (s *SubscriptionServiceSuite) seedFullDiscountCoupon(couponID string) *coupon.Coupon {
	ctx := s.GetContext()
	c := &coupon.Coupon{
		ID:            couponID,
		Name:          "Full Discount",
		Type:          types.CouponTypePercentage,
		Cadence:       types.CouponCadenceForever,
		PercentageOff: lo.ToPtr(decimal.NewFromInt(100)),
		CouponCode:    lo.ToPtr(couponID),
		EnvironmentID: types.GetEnvironmentID(ctx),
		BaseModel:     types.GetDefaultBaseModel(ctx),
	}
	c.Status = types.StatusPublished
	s.Require().NoError(s.GetStores().CouponRepo.Create(ctx, c))
	return c
}

// TestCreateSubscription_ChildAddonPriceCoupon_ScopesToEachChildsAddonLineItem is the shape the
// feature exists for: two grouped children, the same addon on both, one coupon discounting that
// addon on each. Each child's association must point at that child's own addon line item.
func (s *SubscriptionServiceSuite) TestCreateSubscription_ChildAddonPriceCoupon_ScopesToEachChildsAddonLineItem() {
	ctx := s.GetContext()
	seatPlan := s.setupSeatFeePlan()

	addonID := "addon_shared_children"
	addonPriceID := "price_addon_shared_children"
	s.seedSharedAddon(addonID, addonPriceID)
	c := s.seedFullDiscountCoupon("coupon_child_addon_full")

	ws1 := s.seedChildCustomer("ext_ws1_addon_coupon")
	ws2 := s.seedChildCustomer("ext_ws2_addon_coupon")

	childReq := func(externalID string) dto.GroupedInvoicingChildRequest {
		return dto.GroupedInvoicingChildRequest{
			PlanID:             seatPlan.ID,
			ExternalCustomerID: externalID,
			SubscriptionCreationConfig: dto.SubscriptionCreationConfig{
				Addons: []dto.AddAddonToSubscriptionRequest{{AddonID: addonID}},
				SubscriptionCoupons: []dto.SubscriptionCouponInput{
					{CouponCode: *c.CouponCode, PriceID: lo.ToPtr(addonPriceID)},
				},
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

	for _, child := range children {
		addonLineItems := s.addonLineItemsFor(child.ID, addonID)
		s.Require().Len(addonLineItems, 1, "child %s should carry one addon line item", child.ID)

		assocs, err := s.GetStores().CouponAssociationRepo.List(ctx, &types.CouponAssociationFilter{
			QueryFilter:     types.NewNoLimitQueryFilter(),
			SubscriptionIDs: []string{child.ID},
			CouponIDs:       []string{c.ID},
		})
		s.NoError(err)
		s.Require().Len(assocs, 1, "child %s should hold exactly one association", child.ID)
		s.Equal(
			addonLineItems[0].ID,
			lo.FromPtr(assocs[0].SubscriptionLineItemID),
			"the coupon must target this child's addon line item, not fall back to subscription level",
		)
	}

	// The parent itself carries no addon and must not pick up the discount.
	parentAssocs, err := s.GetStores().CouponAssociationRepo.List(ctx, &types.CouponAssociationFilter{
		QueryFilter:     types.NewNoLimitQueryFilter(),
		SubscriptionIDs: []string{resp.ID},
		CouponIDs:       []string{c.ID},
	})
	s.NoError(err)
	s.Empty(parentAssocs, "the parent must not be associated with a coupon its children own")
}

// TestCreateSubscription_AddonAttachedTwice_CouponCoversEveryMatchingLineItem covers the case a
// price_id cannot disambiguate: one addon attached twice yields two line items behind one price.
// SubscriptionCouponInput can only name a price, so the coupon has to cover both — refusing would
// leave the shape with no way to express its discount at all.
func (s *SubscriptionServiceSuite) TestCreateSubscription_AddonAttachedTwice_CouponCoversEveryMatchingLineItem() {
	ctx := s.GetContext()
	seatPlan := s.setupSeatFeePlan()

	addonID := "addon_attached_twice"
	addonPriceID := "price_addon_attached_twice"
	s.seedSharedAddon(addonID, addonPriceID)
	c := s.seedFullDiscountCoupon("coupon_addon_twice")

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
				{AddonID: addonID},
				{AddonID: addonID},
			},
			SubscriptionCoupons: []dto.SubscriptionCouponInput{
				{CouponCode: *c.CouponCode, PriceID: lo.ToPtr(addonPriceID)},
			},
		},
	})
	s.Require().NoError(err)

	addonLineItems := s.addonLineItemsFor(resp.ID, addonID)
	s.Require().Len(addonLineItems, 2, "both attachments should produce their own line item")

	assocs, err := s.GetStores().CouponAssociationRepo.List(ctx, &types.CouponAssociationFilter{
		QueryFilter:     types.NewNoLimitQueryFilter(),
		SubscriptionIDs: []string{resp.ID},
		CouponIDs:       []string{c.ID},
	})
	s.NoError(err)
	s.Require().Len(assocs, 2, "one association per matching line item")

	targeted := lo.Map(assocs, func(a *coupon_association.CouponAssociation, _ int) string {
		return lo.FromPtr(a.SubscriptionLineItemID)
	})
	s.ElementsMatch(
		lo.Map(addonLineItems, func(li *subscription.SubscriptionLineItem, _ int) string { return li.ID }),
		targeted,
		"every line item behind the price must be covered, none twice",
	)
}

// TestCreateSubscription_SubscriptionCoupon_UnknownPriceIDIsRejected pins the behaviour change: an
// unresolvable price_id used to be logged and downgraded to a subscription-level coupon, quietly
// discounting every charge instead of one line. A caller naming a price that isn't there is now told.
func (s *SubscriptionServiceSuite) TestCreateSubscription_SubscriptionCoupon_UnknownPriceIDIsRejected() {
	ctx := s.GetContext()
	seatPlan := s.setupSeatFeePlan()
	c := s.seedFullDiscountCoupon("coupon_unknown_price")

	_, err := s.service.CreateSubscription(ctx, dto.CreateSubscriptionRequest{
		CustomerID:         s.testData.customer.ID,
		PlanID:             seatPlan.ID,
		StartDate:          lo.ToPtr(s.testData.now),
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingCycle:       types.BillingCycleAnniversary,
		SubscriptionCreationConfig: dto.SubscriptionCreationConfig{
			SubscriptionCoupons: []dto.SubscriptionCouponInput{
				{CouponCode: *c.CouponCode, PriceID: lo.ToPtr("price_does_not_exist")},
			},
		},
	})
	s.Require().Error(err)
	s.True(ierr.IsValidation(err), "expected a validation error, got %v", err)
}

// seedLimitedCoupon publishes a 100%-off coupon capped at maxRedemptions.
func (s *SubscriptionServiceSuite) seedLimitedCoupon(couponID string, maxRedemptions int) *coupon.Coupon {
	c := s.seedFullDiscountCoupon(couponID)
	c.MaxRedemptions = lo.ToPtr(maxRedemptions)
	s.Require().NoError(s.GetStores().CouponRepo.Update(s.GetContext(), c))
	return c
}

// TestCreateSubscription_CouponWithoutRedemptionHeadroom_IsRejected covers the cost of fanning
// out: each line item becomes its own association and consumes its own redemption, so a
// single-use coupon cannot cover an addon attached twice. CouponValidationService re-reads the
// coupon before each association, so the second one is refused there.
func (s *SubscriptionServiceSuite) TestCreateSubscription_CouponWithoutRedemptionHeadroom_IsRejected() {
	ctx := s.GetContext()
	seatPlan := s.setupSeatFeePlan()

	addonID := "addon_headroom"
	addonPriceID := "price_addon_headroom"
	s.seedSharedAddon(addonID, addonPriceID)
	c := s.seedLimitedCoupon("coupon_headroom_one", 1)

	_, err := s.service.CreateSubscription(ctx, dto.CreateSubscriptionRequest{
		CustomerID:         s.testData.customer.ID,
		PlanID:             seatPlan.ID,
		StartDate:          lo.ToPtr(s.testData.now),
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingCycle:       types.BillingCycleAnniversary,
		SubscriptionCreationConfig: dto.SubscriptionCreationConfig{
			Addons: []dto.AddAddonToSubscriptionRequest{
				{AddonID: addonID},
				{AddonID: addonID},
			},
			SubscriptionCoupons: []dto.SubscriptionCouponInput{
				{CouponCode: *c.CouponCode, PriceID: lo.ToPtr(addonPriceID)},
			},
		},
	})
	s.Require().Error(err)
	s.True(ierr.IsValidation(err), "expected a validation error, got %v", err)
	s.Contains(err.Error(), "redemption", "the error must name the redemption shortfall")
}

// TestCreateSubscription_CouponWithHeadroomForEveryChild_Succeeds is the counterpart: the same
// coupon across two grouped children needs two redemptions, and having exactly that many is enough.
func (s *SubscriptionServiceSuite) TestCreateSubscription_CouponWithHeadroomForEveryChild_Succeeds() {
	ctx := s.GetContext()
	seatPlan := s.setupSeatFeePlan()

	addonID := "addon_headroom_children"
	addonPriceID := "price_addon_headroom_children"
	s.seedSharedAddon(addonID, addonPriceID)
	c := s.seedLimitedCoupon("coupon_headroom_two", 2)

	ws1 := s.seedChildCustomer("ext_ws1_headroom")
	ws2 := s.seedChildCustomer("ext_ws2_headroom")

	childReq := func(externalID string) dto.GroupedInvoicingChildRequest {
		return dto.GroupedInvoicingChildRequest{
			PlanID:             seatPlan.ID,
			ExternalCustomerID: externalID,
			SubscriptionCreationConfig: dto.SubscriptionCreationConfig{
				Addons: []dto.AddAddonToSubscriptionRequest{{AddonID: addonID}},
				SubscriptionCoupons: []dto.SubscriptionCouponInput{
					{CouponCode: *c.CouponCode, PriceID: lo.ToPtr(addonPriceID)},
				},
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
		Inheritance: &dto.SubscriptionInheritanceConfig{
			GroupedInvoicingChildrenToCreate: []dto.GroupedInvoicingChildRequest{
				childReq(ws1.ExternalID),
				childReq(ws2.ExternalID),
			},
		},
	})
	s.Require().NoError(err)
	s.Require().Len(s.groupedChildrenOf(resp.ID), 2)

	updated, err := s.GetStores().CouponRepo.Get(ctx, c.ID)
	s.NoError(err)
	s.Equal(2, updated.TotalRedemptions, "one redemption per child association")
}

// TestCreateSubscription_SubscriptionCoupon_PlanPriceStillResolves guards the path that already
// worked: a plan price must keep resolving to its line item now that the map is rebuilt from
// persisted line items rather than the plan-derived ones.
func (s *SubscriptionServiceSuite) TestCreateSubscription_SubscriptionCoupon_PlanPriceStillResolves() {
	ctx := s.GetContext()
	seatPlan := s.setupSeatFeePlan()
	c := s.seedFullDiscountCoupon("coupon_plan_price")

	planPrices, err := s.GetStores().PriceRepo.List(ctx, &types.PriceFilter{
		QueryFilter: types.NewNoLimitQueryFilter(),
		EntityType:  lo.ToPtr(types.PRICE_ENTITY_TYPE_PLAN),
		EntityIDs:   []string{seatPlan.ID},
	})
	s.NoError(err)
	s.Require().NotEmpty(planPrices)

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
				{CouponCode: *c.CouponCode, PriceID: lo.ToPtr(planPrices[0].ID)},
			},
		},
	})
	s.Require().NoError(err)

	assocs, err := s.GetStores().CouponAssociationRepo.List(ctx, &types.CouponAssociationFilter{
		QueryFilter:     types.NewNoLimitQueryFilter(),
		SubscriptionIDs: []string{resp.ID},
		CouponIDs:       []string{c.ID},
	})
	s.NoError(err)
	s.Require().Len(assocs, 1)
	s.NotNil(assocs[0].SubscriptionLineItemID, "a plan price_id must still scope the coupon to its line item")
}
