package service

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type FixedChargeBillingSuite struct {
	testutil.BaseServiceTestSuite
	service BillingService
}

func TestFixedChargeBilling(t *testing.T) {
	suite.Run(t, new(FixedChargeBillingSuite))
}

func (s *FixedChargeBillingSuite) SetupTest() {
	s.BaseServiceTestSuite.SetupTest()
	s.service = NewBillingService(ServiceParams{
		Logger:                   s.GetLogger(),
		Config:                   s.GetConfig(),
		DB:                       s.GetDB(),
		SubRepo:                  s.GetStores().SubscriptionRepo,
		SubscriptionLineItemRepo: s.GetStores().SubscriptionLineItemRepo,
		PlanRepo:                 s.GetStores().PlanRepo,
		PriceRepo:                s.GetStores().PriceRepo,
		EventRepo:                s.GetStores().EventRepo,
		MeterRepo:                s.GetStores().MeterRepo,
		CustomerRepo:             s.GetStores().CustomerRepo,
		InvoiceRepo:              s.GetStores().InvoiceRepo,
		EntitlementRepo:          s.GetStores().EntitlementRepo,
		EnvironmentRepo:          s.GetStores().EnvironmentRepo,
		FeatureRepo:              s.GetStores().FeatureRepo,
		TenantRepo:               s.GetStores().TenantRepo,
		UserRepo:                 s.GetStores().UserRepo,
		AuthRepo:                 s.GetStores().AuthRepo,
		WalletRepo:               s.GetStores().WalletRepo,
		PaymentRepo:              s.GetStores().PaymentRepo,
		CouponAssociationRepo:    s.GetStores().CouponAssociationRepo,
		CouponRepo:               s.GetStores().CouponRepo,
		CouponApplicationRepo:    s.GetStores().CouponApplicationRepo,
		AddonAssociationRepo:     s.GetStores().AddonAssociationRepo,
		TaxRateRepo:              s.GetStores().TaxRateRepo,
		TaxAssociationRepo:       s.GetStores().TaxAssociationRepo,
		TaxAppliedRepo:           s.GetStores().TaxAppliedRepo,
		SettingsRepo:             s.GetStores().SettingsRepo,
		EventPublisher:           s.GetPublisher(),
		WebhookPublisher:         s.GetWebhookPublisher(),
		ProrationCalculator:      s.GetCalculator(),
		AlertLogsRepo:            s.GetStores().AlertLogsRepo,
	})
}

func (s *FixedChargeBillingSuite) TearDownTest() {
	s.BaseServiceTestSuite.TearDownTest()
}

// seedCustomerAndPlan creates an Aruba customer and a plan, returns both.
func (s *FixedChargeBillingSuite) seedCustomerAndPlan(custID, planID string) (*customer.Customer, *plan.Plan) {
	ctx := s.GetContext()
	cust := &customer.Customer{
		ID:         custID,
		ExternalID: "ext_" + custID,
		Name:       "Aruba",
		Email:      "aruba@example.com",
		BaseModel:  types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().CustomerRepo.Create(ctx, cust))

	pl := &plan.Plan{
		ID:        planID,
		Name:      "Aruba Plan " + planID,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().PlanRepo.Create(ctx, pl))
	return cust, pl
}

// seedPrice creates a price in PriceRepo and returns it.
func (s *FixedChargeBillingSuite) seedPrice(p *price.Price) *price.Price {
	s.NoError(s.GetStores().PriceRepo.Create(s.GetContext(), p))
	return p
}

// seedSubscriptionWithLineItem creates a subscription and one line item, returns the subscription
// (with LineItems populated in memory).
func (s *FixedChargeBillingSuite) seedSubscriptionWithLineItem(
	sub *subscription.Subscription,
	li *subscription.SubscriptionLineItem,
) *subscription.Subscription {
	ctx := s.GetContext()
	s.NoError(s.GetStores().SubscriptionRepo.CreateWithLineItems(ctx, sub, []*subscription.SubscriptionLineItem{li}))
	sub.LineItems = []*subscription.SubscriptionLineItem{li}
	return sub
}

// seedSubscriptionWithLineItems creates a subscription and multiple line items.
func (s *FixedChargeBillingSuite) seedSubscriptionWithLineItems(
	sub *subscription.Subscription,
	lis []*subscription.SubscriptionLineItem,
) *subscription.Subscription {
	ctx := s.GetContext()
	s.NoError(s.GetStores().SubscriptionRepo.CreateWithLineItems(ctx, sub, lis))
	sub.LineItems = lis
	return sub
}

// fixedPeriod returns a standard monthly period: Jan 1 – Feb 1 2025.
func fixedPeriod() (time.Time, time.Time) {
	start := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, time.February, 1, 0, 0, 0, 0, time.UTC)
	return start, end
}

func (s *FixedChargeBillingSuite) TestFlatFee_Advance_Monthly() {
	ctx := s.GetContext()
	periodStart, periodEnd := fixedPeriod()

	_, pl := s.seedCustomerAndPlan("cust_ff_adv", "plan_ff_adv")

	p := s.seedPrice(&price.Price{
		ID:                 "price_ff_adv",
		Amount:             decimal.NewFromInt(100),
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           pl.ID,
		Type:               types.PRICE_TYPE_FIXED,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	})

	sub := &subscription.Subscription{
		ID:                 "sub_ff_adv",
		PlanID:             pl.ID,
		CustomerID:         "cust_ff_adv",
		StartDate:          periodStart,
		BillingAnchor:      periodStart,
		CurrentPeriodStart: periodStart,
		CurrentPeriodEnd:   periodEnd,
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		SubscriptionStatus: types.SubscriptionStatusActive,
		Timezone:           "UTC",
		ProrationBehavior:  types.ProrationBehaviorNone,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	li := &subscription.SubscriptionLineItem{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM),
		SubscriptionID:     sub.ID,
		CustomerID:         sub.CustomerID,
		EntityID:           pl.ID,
		EntityType:         types.SubscriptionLineItemEntityTypePlan,
		PriceID:            p.ID,
		PriceType:          types.PRICE_TYPE_FIXED,
		DisplayName:        "Flat Fee",
		Quantity:           decimal.NewFromInt(3),
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		StartDate:          periodStart,
		EndDate:            periodEnd,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	s.seedSubscriptionWithLineItem(sub, li)

	result, err := s.service.CalculateFixedCharges(ctx, &dto.CalculateFixedChargesParams{
		Subscription: sub,
		PeriodStart:  periodStart,
		PeriodEnd:    periodEnd,
	})
	s.NoError(err)
	s.Require().Len(result.LineItems, 1, "expected 1 line item for flat fee advance")
	s.True(result.LineItems[0].Amount.Equal(decimal.NewFromInt(300)),
		"expected $100 × 3 = $300, got %s", result.LineItems[0].Amount)
	s.True(result.TotalAmount.Equal(decimal.NewFromInt(300)))
}

func (s *FixedChargeBillingSuite) TestFlatFee_Arrear_Monthly() {
	ctx := s.GetContext()
	periodStart, periodEnd := fixedPeriod()

	_, pl := s.seedCustomerAndPlan("cust_ff_arr", "plan_ff_arr")

	p := s.seedPrice(&price.Price{
		ID:                 "price_ff_arr",
		Amount:             decimal.NewFromInt(100),
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           pl.ID,
		Type:               types.PRICE_TYPE_FIXED,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		InvoiceCadence:     types.InvoiceCadenceArrear,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	})

	sub := &subscription.Subscription{
		ID:                 "sub_ff_arr",
		PlanID:             pl.ID,
		CustomerID:         "cust_ff_arr",
		StartDate:          periodStart,
		BillingAnchor:      periodStart,
		CurrentPeriodStart: periodStart,
		CurrentPeriodEnd:   periodEnd,
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		SubscriptionStatus: types.SubscriptionStatusActive,
		Timezone:           "UTC",
		ProrationBehavior:  types.ProrationBehaviorNone,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	li := &subscription.SubscriptionLineItem{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM),
		SubscriptionID:     sub.ID,
		CustomerID:         sub.CustomerID,
		EntityID:           pl.ID,
		EntityType:         types.SubscriptionLineItemEntityTypePlan,
		PriceID:            p.ID,
		PriceType:          types.PRICE_TYPE_FIXED,
		DisplayName:        "Flat Fee Arrear",
		Quantity:           decimal.NewFromInt(3),
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		InvoiceCadence:     types.InvoiceCadenceArrear,
		StartDate:          periodStart,
		EndDate:            periodEnd,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	s.seedSubscriptionWithLineItem(sub, li)

	result, err := s.service.CalculateFixedCharges(ctx, &dto.CalculateFixedChargesParams{
		Subscription: sub,
		PeriodStart:  periodStart,
		PeriodEnd:    periodEnd,
	})
	s.NoError(err)
	s.Require().Len(result.LineItems, 1, "expected 1 arrear line item")
	s.True(result.LineItems[0].Amount.Equal(decimal.NewFromInt(300)),
		"expected $100 × 3 = $300 for arrear, got %s", result.LineItems[0].Amount)
	s.True(result.TotalAmount.Equal(decimal.NewFromInt(300)))
}

func (s *FixedChargeBillingSuite) TestPackage_Advance_Monthly() {
	ctx := s.GetContext()
	periodStart, periodEnd := fixedPeriod()

	_, pl := s.seedCustomerAndPlan("cust_pkg_adv", "plan_pkg_adv")

	// $50 per package of 10 units; quantity=25 → ceil(25/10)=3 packages → $150
	p := s.seedPrice(&price.Price{
		ID:                 "price_pkg_adv",
		Amount:             decimal.NewFromInt(50),
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           pl.ID,
		Type:               types.PRICE_TYPE_FIXED,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_PACKAGE,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		TransformQuantity: price.JSONBTransformQuantity{
			DivideBy: 10,
			Round:    types.ROUND_UP,
		},
		BaseModel: types.GetDefaultBaseModel(ctx),
	})

	sub := &subscription.Subscription{
		ID:                 "sub_pkg_adv",
		PlanID:             pl.ID,
		CustomerID:         "cust_pkg_adv",
		StartDate:          periodStart,
		BillingAnchor:      periodStart,
		CurrentPeriodStart: periodStart,
		CurrentPeriodEnd:   periodEnd,
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		SubscriptionStatus: types.SubscriptionStatusActive,
		Timezone:           "UTC",
		ProrationBehavior:  types.ProrationBehaviorNone,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	li := &subscription.SubscriptionLineItem{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM),
		SubscriptionID:     sub.ID,
		CustomerID:         sub.CustomerID,
		EntityID:           pl.ID,
		EntityType:         types.SubscriptionLineItemEntityTypePlan,
		PriceID:            p.ID,
		PriceType:          types.PRICE_TYPE_FIXED,
		DisplayName:        "Package Fee",
		Quantity:           decimal.NewFromInt(25),
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		StartDate:          periodStart,
		EndDate:            periodEnd,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	s.seedSubscriptionWithLineItem(sub, li)

	result, err := s.service.CalculateFixedCharges(ctx, &dto.CalculateFixedChargesParams{
		Subscription: sub,
		PeriodStart:  periodStart,
		PeriodEnd:    periodEnd,
	})
	s.NoError(err)
	s.Require().Len(result.LineItems, 1, "expected 1 package line item")
	// ceil(25/10) = 3 packages × $50 = $150
	s.True(result.LineItems[0].Amount.Equal(decimal.NewFromInt(150)),
		"expected ceil(25/10)=3 packages × $50 = $150, got %s", result.LineItems[0].Amount)
	s.True(result.TotalAmount.Equal(decimal.NewFromInt(150)))
}

func (s *FixedChargeBillingSuite) TestPackage_Arrear_Monthly() {
	ctx := s.GetContext()
	periodStart, periodEnd := fixedPeriod()

	_, pl := s.seedCustomerAndPlan("cust_pkg_arr", "plan_pkg_arr")

	p := s.seedPrice(&price.Price{
		ID:                 "price_pkg_arr",
		Amount:             decimal.NewFromInt(50),
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           pl.ID,
		Type:               types.PRICE_TYPE_FIXED,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_PACKAGE,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		InvoiceCadence:     types.InvoiceCadenceArrear,
		TransformQuantity: price.JSONBTransformQuantity{
			DivideBy: 10,
			Round:    types.ROUND_UP,
		},
		BaseModel: types.GetDefaultBaseModel(ctx),
	})

	sub := &subscription.Subscription{
		ID:                 "sub_pkg_arr",
		PlanID:             pl.ID,
		CustomerID:         "cust_pkg_arr",
		StartDate:          periodStart,
		BillingAnchor:      periodStart,
		CurrentPeriodStart: periodStart,
		CurrentPeriodEnd:   periodEnd,
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		SubscriptionStatus: types.SubscriptionStatusActive,
		Timezone:           "UTC",
		ProrationBehavior:  types.ProrationBehaviorNone,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	li := &subscription.SubscriptionLineItem{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM),
		SubscriptionID:     sub.ID,
		CustomerID:         sub.CustomerID,
		EntityID:           pl.ID,
		EntityType:         types.SubscriptionLineItemEntityTypePlan,
		PriceID:            p.ID,
		PriceType:          types.PRICE_TYPE_FIXED,
		DisplayName:        "Package Fee Arrear",
		Quantity:           decimal.NewFromInt(25),
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		InvoiceCadence:     types.InvoiceCadenceArrear,
		StartDate:          periodStart,
		EndDate:            periodEnd,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	s.seedSubscriptionWithLineItem(sub, li)

	result, err := s.service.CalculateFixedCharges(ctx, &dto.CalculateFixedChargesParams{
		Subscription: sub,
		PeriodStart:  periodStart,
		PeriodEnd:    periodEnd,
	})
	s.NoError(err)
	s.Require().Len(result.LineItems, 1, "expected 1 arrear package line item")
	s.True(result.LineItems[0].Amount.Equal(decimal.NewFromInt(150)),
		"expected $150 for arrear package, got %s", result.LineItems[0].Amount)
	s.True(result.TotalAmount.Equal(decimal.NewFromInt(150)))
}

func (s *FixedChargeBillingSuite) TestTieredSlab_Advance_Monthly() {
	ctx := s.GetContext()
	periodStart, periodEnd := fixedPeriod()

	_, pl := s.seedCustomerAndPlan("cust_slab_adv", "plan_slab_adv")

	upTo10 := uint64(10)
	upTo50 := uint64(50)
	// Tiers: 0–10 @ $5/unit, 11–50 @ $3/unit; quantity=20
	// SLAB: (10×$5) + (10×$3) = $50 + $30 = $80
	p := s.seedPrice(&price.Price{
		ID:                 "price_slab_adv",
		Amount:             decimal.Zero,
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           pl.ID,
		Type:               types.PRICE_TYPE_FIXED,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_TIERED,
		TierMode:           types.BILLING_TIER_SLAB,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		Tiers: price.JSONBTiers{
			{UpTo: &upTo10, UnitAmount: decimal.NewFromInt(5)},
			{UpTo: &upTo50, UnitAmount: decimal.NewFromInt(3)},
		},
		BaseModel: types.GetDefaultBaseModel(ctx),
	})

	sub := &subscription.Subscription{
		ID:                 "sub_slab_adv",
		PlanID:             pl.ID,
		CustomerID:         "cust_slab_adv",
		StartDate:          periodStart,
		BillingAnchor:      periodStart,
		CurrentPeriodStart: periodStart,
		CurrentPeriodEnd:   periodEnd,
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		SubscriptionStatus: types.SubscriptionStatusActive,
		Timezone:           "UTC",
		ProrationBehavior:  types.ProrationBehaviorNone,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	li := &subscription.SubscriptionLineItem{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM),
		SubscriptionID:     sub.ID,
		CustomerID:         sub.CustomerID,
		EntityID:           pl.ID,
		EntityType:         types.SubscriptionLineItemEntityTypePlan,
		PriceID:            p.ID,
		PriceType:          types.PRICE_TYPE_FIXED,
		DisplayName:        "Tiered Slab Fee",
		Quantity:           decimal.NewFromInt(20),
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		StartDate:          periodStart,
		EndDate:            periodEnd,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	s.seedSubscriptionWithLineItem(sub, li)

	result, err := s.service.CalculateFixedCharges(ctx, &dto.CalculateFixedChargesParams{
		Subscription: sub,
		PeriodStart:  periodStart,
		PeriodEnd:    periodEnd,
	})
	s.NoError(err)
	s.Require().Len(result.LineItems, 1, "expected 1 tiered slab line item")
	// (10×$5) + (10×$3) = $80
	s.True(result.LineItems[0].Amount.Equal(decimal.NewFromInt(80)),
		"expected SLAB (10×$5)+(10×$3)=$80, got %s", result.LineItems[0].Amount)
	s.True(result.TotalAmount.Equal(decimal.NewFromInt(80)))
}

func (s *FixedChargeBillingSuite) TestTieredVolume_Advance_Monthly() {
	ctx := s.GetContext()
	periodStart, periodEnd := fixedPeriod()

	_, pl := s.seedCustomerAndPlan("cust_vol_adv", "plan_vol_adv")

	upTo10 := uint64(10)
	upTo50 := uint64(50)
	// VOLUME: all 20 units priced at matching tier → tier 2 ($3) → 20×$3 = $60
	p := s.seedPrice(&price.Price{
		ID:                 "price_vol_adv",
		Amount:             decimal.Zero,
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           pl.ID,
		Type:               types.PRICE_TYPE_FIXED,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_TIERED,
		TierMode:           types.BILLING_TIER_VOLUME,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		Tiers: price.JSONBTiers{
			{UpTo: &upTo10, UnitAmount: decimal.NewFromInt(5)},
			{UpTo: &upTo50, UnitAmount: decimal.NewFromInt(3)},
		},
		BaseModel: types.GetDefaultBaseModel(ctx),
	})

	sub := &subscription.Subscription{
		ID:                 "sub_vol_adv",
		PlanID:             pl.ID,
		CustomerID:         "cust_vol_adv",
		StartDate:          periodStart,
		BillingAnchor:      periodStart,
		CurrentPeriodStart: periodStart,
		CurrentPeriodEnd:   periodEnd,
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		SubscriptionStatus: types.SubscriptionStatusActive,
		Timezone:           "UTC",
		ProrationBehavior:  types.ProrationBehaviorNone,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	li := &subscription.SubscriptionLineItem{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM),
		SubscriptionID:     sub.ID,
		CustomerID:         sub.CustomerID,
		EntityID:           pl.ID,
		EntityType:         types.SubscriptionLineItemEntityTypePlan,
		PriceID:            p.ID,
		PriceType:          types.PRICE_TYPE_FIXED,
		DisplayName:        "Tiered Volume Fee",
		Quantity:           decimal.NewFromInt(20),
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		StartDate:          periodStart,
		EndDate:            periodEnd,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	s.seedSubscriptionWithLineItem(sub, li)

	result, err := s.service.CalculateFixedCharges(ctx, &dto.CalculateFixedChargesParams{
		Subscription: sub,
		PeriodStart:  periodStart,
		PeriodEnd:    periodEnd,
	})
	s.NoError(err)
	s.Require().Len(result.LineItems, 1, "expected 1 tiered volume line item")
	// VOLUME: all 20 units at tier-2 rate $3 → $60
	s.True(result.LineItems[0].Amount.Equal(decimal.NewFromInt(60)),
		"expected VOLUME 20×$3=$60, got %s", result.LineItems[0].Amount)
	s.True(result.TotalAmount.Equal(decimal.NewFromInt(60)))
}

func (s *FixedChargeBillingSuite) TestFlatFee_Annual_Advance() {
	ctx := s.GetContext()
	periodStart := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	_, pl := s.seedCustomerAndPlan("cust_ann_adv", "plan_ann_adv")

	p := s.seedPrice(&price.Price{
		ID:                 "price_ann_adv",
		Amount:             decimal.NewFromInt(1200),
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           pl.ID,
		Type:               types.PRICE_TYPE_FIXED,
		BillingPeriod:      types.BILLING_PERIOD_ANNUAL,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	})

	sub := &subscription.Subscription{
		ID:                 "sub_ann_adv",
		PlanID:             pl.ID,
		CustomerID:         "cust_ann_adv",
		StartDate:          periodStart,
		BillingAnchor:      periodStart,
		CurrentPeriodStart: periodStart,
		CurrentPeriodEnd:   periodEnd,
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_ANNUAL,
		BillingPeriodCount: 1,
		SubscriptionStatus: types.SubscriptionStatusActive,
		Timezone:           "UTC",
		ProrationBehavior:  types.ProrationBehaviorNone,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	li := &subscription.SubscriptionLineItem{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM),
		SubscriptionID:     sub.ID,
		CustomerID:         sub.CustomerID,
		EntityID:           pl.ID,
		EntityType:         types.SubscriptionLineItemEntityTypePlan,
		PriceID:            p.ID,
		PriceType:          types.PRICE_TYPE_FIXED,
		DisplayName:        "Annual Fee",
		Quantity:           decimal.NewFromInt(1),
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_ANNUAL,
		BillingPeriodCount: 1,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		StartDate:          periodStart,
		EndDate:            periodEnd,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	s.seedSubscriptionWithLineItem(sub, li)

	result, err := s.service.CalculateFixedCharges(ctx, &dto.CalculateFixedChargesParams{
		Subscription: sub,
		PeriodStart:  periodStart,
		PeriodEnd:    periodEnd,
	})
	s.NoError(err)
	s.Require().Len(result.LineItems, 1, "expected 1 annual line item")
	s.True(result.LineItems[0].Amount.Equal(decimal.NewFromInt(1200)),
		"expected full annual amount $1200, got %s", result.LineItems[0].Amount)
	s.True(result.TotalAmount.Equal(decimal.NewFromInt(1200)))
}

func (s *FixedChargeBillingSuite) TestMixedPlan_FlatFeeAndTieredSlab_Advance() {
	ctx := s.GetContext()
	periodStart, periodEnd := fixedPeriod()

	_, pl := s.seedCustomerAndPlan("cust_mix", "plan_mix")

	// Price 1: FLAT_FEE $100 × 1 = $100
	pFlat := s.seedPrice(&price.Price{
		ID:                 "price_mix_flat",
		Amount:             decimal.NewFromInt(100),
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           pl.ID,
		Type:               types.PRICE_TYPE_FIXED,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	})

	upTo10 := uint64(10)
	upTo50 := uint64(50)
	// Price 2: TIERED SLAB, quantity=20 → (10×$5)+(10×$3) = $80
	pTiered := s.seedPrice(&price.Price{
		ID:                 "price_mix_tiered",
		Amount:             decimal.Zero,
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           pl.ID,
		Type:               types.PRICE_TYPE_FIXED,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_TIERED,
		TierMode:           types.BILLING_TIER_SLAB,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		Tiers: price.JSONBTiers{
			{UpTo: &upTo10, UnitAmount: decimal.NewFromInt(5)},
			{UpTo: &upTo50, UnitAmount: decimal.NewFromInt(3)},
		},
		BaseModel: types.GetDefaultBaseModel(ctx),
	})

	sub := &subscription.Subscription{
		ID:                 "sub_mix",
		PlanID:             pl.ID,
		CustomerID:         "cust_mix",
		StartDate:          periodStart,
		BillingAnchor:      periodStart,
		CurrentPeriodStart: periodStart,
		CurrentPeriodEnd:   periodEnd,
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		SubscriptionStatus: types.SubscriptionStatusActive,
		Timezone:           "UTC",
		ProrationBehavior:  types.ProrationBehaviorNone,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	liFlat := &subscription.SubscriptionLineItem{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM),
		SubscriptionID:     sub.ID,
		CustomerID:         sub.CustomerID,
		EntityID:           pl.ID,
		EntityType:         types.SubscriptionLineItemEntityTypePlan,
		PriceID:            pFlat.ID,
		PriceType:          types.PRICE_TYPE_FIXED,
		DisplayName:        "Flat Fee",
		Quantity:           decimal.NewFromInt(1),
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		StartDate:          periodStart,
		EndDate:            periodEnd,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	liTiered := &subscription.SubscriptionLineItem{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM),
		SubscriptionID:     sub.ID,
		CustomerID:         sub.CustomerID,
		EntityID:           pl.ID,
		EntityType:         types.SubscriptionLineItemEntityTypePlan,
		PriceID:            pTiered.ID,
		PriceType:          types.PRICE_TYPE_FIXED,
		DisplayName:        "Tiered Fee",
		Quantity:           decimal.NewFromInt(20),
		Currency:           "usd",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		StartDate:          periodStart,
		EndDate:            periodEnd,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	s.seedSubscriptionWithLineItems(sub, []*subscription.SubscriptionLineItem{liFlat, liTiered})

	result, err := s.service.CalculateFixedCharges(ctx, &dto.CalculateFixedChargesParams{
		Subscription: sub,
		PeriodStart:  periodStart,
		PeriodEnd:    periodEnd,
	})
	s.NoError(err)
	s.Require().Len(result.LineItems, 2, "expected 2 line items for mixed plan")

	var flatAmt, tieredAmt decimal.Decimal
	for _, item := range result.LineItems {
		switch lo.FromPtr(item.PriceID) {
		case pFlat.ID:
			flatAmt = item.Amount
		case pTiered.ID:
			tieredAmt = item.Amount
		default:
			s.Fail("unexpected PriceID in line items: %s", lo.FromPtr(item.PriceID))
		}
	}
	s.True(flatAmt.Equal(decimal.NewFromInt(100)), "flat fee line item should be $100, got %s", flatAmt)
	s.True(tieredAmt.Equal(decimal.NewFromInt(80)), "tiered slab line item should be $80, got %s", tieredAmt)
	s.True(result.TotalAmount.Equal(decimal.NewFromInt(180)), "total should be $180, got %s", result.TotalAmount)
}

// FixedChargeProrationSuite covers proration of fixed charges by CalculateFixedCharges across
// calendar and anniversary stubs, later periods, timezones and proration behaviours.
type FixedChargeProrationSuite struct {
	testutil.BaseServiceTestSuite
	params  ServiceParams
	billing BillingService
}

func TestFixedChargeProration(t *testing.T) {
	suite.Run(t, new(FixedChargeProrationSuite))
}

func (s *FixedChargeProrationSuite) SetupTest() {
	s.BaseServiceTestSuite.SetupTest()
	stores := s.GetStores()
	s.params = ServiceParams{
		Logger:                   s.GetLogger(),
		Config:                   s.GetConfig(),
		DB:                       s.GetDB(),
		SubRepo:                  stores.SubscriptionRepo,
		SubscriptionLineItemRepo: stores.SubscriptionLineItemRepo,
		PlanRepo:                 stores.PlanRepo,
		PriceRepo:                stores.PriceRepo,
		PriceUnitRepo:            stores.PriceUnitRepo,
		EventRepo:                stores.EventRepo,
		MeterRepo:                stores.MeterRepo,
		CustomerRepo:             stores.CustomerRepo,
		InvoiceRepo:              stores.InvoiceRepo,
		InvoiceLineItemRepo:      stores.InvoiceLineItemRepo,
		EntitlementRepo:          stores.EntitlementRepo,
		EnvironmentRepo:          stores.EnvironmentRepo,
		FeatureRepo:              stores.FeatureRepo,
		TenantRepo:               stores.TenantRepo,
		UserRepo:                 stores.UserRepo,
		AuthRepo:                 stores.AuthRepo,
		WalletRepo:               stores.WalletRepo,
		PaymentRepo:              stores.PaymentRepo,
		CouponRepo:               stores.CouponRepo,
		CouponAssociationRepo:    stores.CouponAssociationRepo,
		CouponApplicationRepo:    stores.CouponApplicationRepo,
		AddonAssociationRepo:     stores.AddonAssociationRepo,
		TaxRateRepo:              stores.TaxRateRepo,
		TaxAssociationRepo:       stores.TaxAssociationRepo,
		TaxAppliedRepo:           stores.TaxAppliedRepo,
		SettingsRepo:             stores.SettingsRepo,
		EventPublisher:           s.GetPublisher(),
		WebhookPublisher:         s.GetWebhookPublisher(),
		ProrationCalculator:      s.GetCalculator(),
		AlertLogsRepo:            stores.AlertLogsRepo,
	}
	s.billing = NewBillingService(s.params)
}

func (s *FixedChargeProrationSuite) TearDownTest() {
	s.BaseServiceTestSuite.TearDownTest()
}

// pvAt returns local midnight of the given date in tz, as a UTC instant.
func pvAt(y int, m time.Month, day int, tz string) time.Time {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		panic(err)
	}
	return time.Date(y, m, day, 0, 0, 0, 0, loc).UTC()
}

func pvUTC(y int, m time.Month, day int) time.Time { return pvAt(y, m, day, "UTC") }

type pvItemSpec struct {
	key        string
	period     types.BillingPeriod
	amount     string
	qty        int64
	model      types.BillingModel
	tierMode   types.BillingTier
	tiers      []price.PriceTier
	transform  *price.TransformQuantity
	start      time.Time // zero = sub start
	end        time.Time
	entityType types.SubscriptionLineItemEntityType
}

type pvSubSpec struct {
	cycle    types.BillingCycle
	period   types.BillingPeriod
	start    time.Time
	anchor   time.Time // zero = derive as production does
	tz       string
	behavior types.ProrationBehavior
	endDate  *time.Time
	items    []pvItemSpec
}

type pvSub struct {
	sub   *subscription.Subscription
	items map[string]*subscription.SubscriptionLineItem
}

func (s *FixedChargeProrationSuite) build(spec pvSubSpec) *pvSub {
	ctx := s.GetContext()
	id := types.GenerateUUIDWithPrefix("pv")
	tz := lo.Ternary(spec.tz == "", "UTC", spec.tz)

	cust := &customer.Customer{ID: "cust_" + id, ExternalID: "ext_" + id, Name: "PV", BaseModel: types.GetDefaultBaseModel(ctx)}
	s.Require().NoError(s.GetStores().CustomerRepo.Create(ctx, cust))
	pl := &plan.Plan{ID: "plan_" + id, Name: "PV Plan", BaseModel: types.GetDefaultBaseModel(ctx)}
	s.Require().NoError(s.GetStores().PlanRepo.Create(ctx, pl))

	// Mirror subscription creation (subscription.go: anchor + first period end).
	anchor := spec.anchor
	if anchor.IsZero() {
		if spec.cycle == types.BillingCycleCalendar {
			anchor = types.CalculateCalendarBillingAnchor(spec.start, spec.period, tz)
		} else {
			anchor = spec.start
		}
	}
	periodEnd, err := types.NextBillingDate(&types.NextBillingDateParams{
		CurrentPeriodStart: spec.start, BillingAnchor: anchor, Unit: 1,
		Period: spec.period, SubscriptionEndDate: spec.endDate, Timezone: tz,
	})
	s.Require().NoError(err)

	sub := &subscription.Subscription{
		ID: "sub_" + id, PlanID: pl.ID, CustomerID: cust.ID,
		StartDate: spec.start, BillingAnchor: anchor, EndDate: spec.endDate,
		CurrentPeriodStart: spec.start, CurrentPeriodEnd: periodEnd,
		Currency: "usd", BillingPeriod: spec.period, BillingPeriodCount: 1,
		BillingCycle: spec.cycle, SubscriptionStatus: types.SubscriptionStatusActive,
		Timezone: tz, ProrationBehavior: spec.behavior,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}

	out := &pvSub{sub: sub, items: map[string]*subscription.SubscriptionLineItem{}}
	lineItems := make([]*subscription.SubscriptionLineItem, 0, len(spec.items))
	for _, it := range spec.items {
		model := lo.Ternary(it.model == "", types.BILLING_MODEL_FLAT_FEE, it.model)
		entityType := lo.Ternary(it.entityType == "", types.SubscriptionLineItemEntityTypePlan, it.entityType)
		entityID := pl.ID
		priceEntity := types.PRICE_ENTITY_TYPE_PLAN
		if entityType == types.SubscriptionLineItemEntityTypeAddon {
			entityID = "addon_" + id + "_" + it.key
			priceEntity = types.PRICE_ENTITY_TYPE_ADDON
		}
		amount := decimal.Zero
		if it.amount != "" {
			amount = decimal.RequireFromString(it.amount)
		}
		pr := &price.Price{
			ID: "price_" + id + "_" + it.key, Amount: amount, Currency: "usd",
			EntityType: priceEntity, EntityID: entityID, Type: types.PRICE_TYPE_FIXED,
			BillingPeriod: it.period, BillingPeriodCount: 1, BillingModel: model,
			TierMode: it.tierMode, Tiers: it.tiers,
			BillingCadence: types.BILLING_CADENCE_RECURRING, InvoiceCadence: types.InvoiceCadenceAdvance,
			BaseModel: types.GetDefaultBaseModel(ctx),
		}
		if it.transform != nil {
			pr.TransformQuantity = price.JSONBTransformQuantity(*it.transform)
		}
		s.Require().NoError(s.GetStores().PriceRepo.Create(ctx, pr))

		qty := lo.Ternary(it.qty == 0, int64(1), it.qty)
		start := lo.Ternary(it.start.IsZero(), spec.start, it.start)
		li := &subscription.SubscriptionLineItem{
			ID: "li_" + id + "_" + it.key, SubscriptionID: sub.ID, CustomerID: cust.ID,
			EntityID: entityID, EntityType: entityType,
			PriceID: pr.ID, PriceType: types.PRICE_TYPE_FIXED, DisplayName: it.key,
			Quantity: decimal.NewFromInt(qty), Currency: "usd",
			BillingPeriod: it.period, BillingPeriodCount: 1,
			InvoiceCadence: types.InvoiceCadenceAdvance, StartDate: start, EndDate: it.end,
			BaseModel: types.GetDefaultBaseModel(ctx),
		}
		lineItems = append(lineItems, li)
		out.items[it.key] = li
	}

	s.Require().NoError(s.GetStores().SubscriptionRepo.CreateWithLineItems(ctx, sub, lineItems))
	sub.LineItems = lineItems
	return out
}

// setCurrent rolls the subscription's current period the way production does on period advance:
// CurrentPeriodStart/End move, BillingAnchor is left untouched.
func (sc *pvSub) setCurrent(start, end time.Time) {
	sc.sub.CurrentPeriodStart = start
	sc.sub.CurrentPeriodEnd = end
}

// charge returns the invoice total and per-item totals for [start, end).
func (s *FixedChargeProrationSuite) charge(sc *pvSub, start, end time.Time) (decimal.Decimal, map[string]decimal.Decimal) {
	res, err := s.billing.CalculateFixedCharges(s.GetContext(), &dto.CalculateFixedChargesParams{
		Subscription: sc.sub,
		PeriodStart:  start,
		PeriodEnd:    end,
	})
	s.Require().NoError(err)
	perItem := map[string]decimal.Decimal{}
	for key, li := range sc.items {
		total := decimal.Zero
		for _, line := range res.LineItems {
			if line.SubscriptionLineItemID != nil && *line.SubscriptionLineItemID == li.ID {
				total = total.Add(line.Amount)
			}
		}
		perItem[key] = total
	}
	return res.TotalAmount, perItem
}

// current charges the subscription's own current period.
func (s *FixedChargeProrationSuite) current(sc *pvSub) (decimal.Decimal, map[string]decimal.Decimal) {
	return s.charge(sc, sc.sub.CurrentPeriodStart, sc.sub.CurrentPeriodEnd)
}

func (s *FixedChargeProrationSuite) expect(label, want string, got decimal.Decimal) {
	expected := decimal.RequireFromString(want)
	ok := got.Sub(expected).Abs().LessThanOrEqual(decimal.NewFromFloat(0.01))
	s.True(ok, "%s: expected %s, got %s", label, want, got.StringFixed(2))
}

func flat(key string, period types.BillingPeriod, amount string) pvItemSpec {
	return pvItemSpec{key: key, period: period, amount: amount}
}

// 1. Calendar monthly start Jan 15: stub [Jan15,Feb1) over [Jan1,Feb1) = 17/31.
func (s *FixedChargeProrationSuite) TestCalendarMonthlyStub() {
	sc := s.build(pvSubSpec{
		cycle: types.BillingCycleCalendar, period: types.BILLING_PERIOD_MONTHLY,
		start: pvUTC(2026, time.January, 15), behavior: types.ProrationBehaviorCreateProrations,
		items: []pvItemSpec{flat("plan", types.BILLING_PERIOD_MONTHLY, "31")},
	})
	s.Equal(pvUTC(2026, time.February, 1), sc.sub.CurrentPeriodEnd)
	total, _ := s.current(sc)
	s.expect("S01 calendar monthly stub Jan15-Feb1", "17.00", total)
}

// 2. Same sub, later periods must bill 1x.
func (s *FixedChargeProrationSuite) TestCalendarMonthlyLaterPeriods() {
	spec := pvSubSpec{
		cycle: types.BillingCycleCalendar, period: types.BILLING_PERIOD_MONTHLY,
		start: pvUTC(2026, time.January, 15), behavior: types.ProrationBehaviorCreateProrations,
		items: []pvItemSpec{flat("plan", types.BILLING_PERIOD_MONTHLY, "31")},
	}
	s.Run("period2_current_rolled", func() {
		sc := s.build(spec)
		sc.setCurrent(pvUTC(2026, time.February, 1), pvUTC(2026, time.March, 1))
		total, _ := s.current(sc)
		s.expect("S02a calendar monthly period 2 Feb1-Mar1 (current rolled)", "31.00", total)
	})
	s.Run("period3_current_rolled", func() {
		sc := s.build(spec)
		sc.setCurrent(pvUTC(2026, time.March, 1), pvUTC(2026, time.April, 1))
		total, _ := s.current(sc)
		s.expect("S02b calendar monthly period 3 Mar1-Apr1 (current rolled)", "31.00", total)
	})
	s.Run("period2_as_next_period_advance", func() {
		// Period-end invoice of period 1: advance for [Feb1,Mar1) while current is still [Jan15,Feb1).
		sc := s.build(spec)
		total, _ := s.charge(sc, pvUTC(2026, time.February, 1), pvUTC(2026, time.March, 1))
		s.expect("S02c calendar monthly period 2 as next-period advance", "31.00", total)
	})
}

// 3. Anniversary with anchor ahead of start: stub [Jan10,Jan20) over [Dec20,Jan20) = 10/31.
func (s *FixedChargeProrationSuite) TestAnniversaryAnchorAhead() {
	spec := pvSubSpec{
		cycle: types.BillingCycleAnniversary, period: types.BILLING_PERIOD_MONTHLY,
		start: pvUTC(2026, time.January, 10), anchor: pvUTC(2026, time.January, 20),
		behavior: types.ProrationBehaviorCreateProrations,
		items:    []pvItemSpec{flat("plan", types.BILLING_PERIOD_MONTHLY, "31")},
	}
	s.Run("period1_stub", func() {
		sc := s.build(spec)
		s.Equal(pvUTC(2026, time.January, 20), sc.sub.CurrentPeriodEnd)
		total, _ := s.current(sc)
		s.expect("S03a anniversary anchor-ahead stub Jan10-Jan20", "10.00", total)
	})
	s.Run("period2_full", func() {
		sc := s.build(spec)
		sc.setCurrent(pvUTC(2026, time.January, 20), pvUTC(2026, time.February, 20))
		total, _ := s.current(sc)
		s.expect("S03b anniversary anchor-ahead period 2 Jan20-Feb20", "31.00", total)
	})
}

// 4. Anniversary sub anchored Jan 10; addon added Jan 25. Period 2 addon must be full.
func (s *FixedChargeProrationSuite) TestAnniversaryAddonMidPeriodThenFullPeriod() {
	spec := pvSubSpec{
		cycle: types.BillingCycleAnniversary, period: types.BILLING_PERIOD_MONTHLY,
		start: pvUTC(2026, time.January, 10), behavior: types.ProrationBehaviorCreateProrations,
		items: []pvItemSpec{
			flat("plan", types.BILLING_PERIOD_MONTHLY, "31"),
			{key: "addon", period: types.BILLING_PERIOD_MONTHLY, amount: "31",
				start: pvUTC(2026, time.January, 25), entityType: types.SubscriptionLineItemEntityTypeAddon},
		},
	}
	s.Run("period1_addon_partial", func() {
		sc := s.build(spec)
		_, per := s.current(sc)
		s.expect("S04a anniversary period 1 addon from Jan25 (16/31)", "16.00", per["addon"])
		s.expect("S04a anniversary period 1 plan", "31.00", per["plan"])
	})
	s.Run("period2_addon_full", func() {
		sc := s.build(spec)
		sc.setCurrent(pvUTC(2026, time.February, 10), pvUTC(2026, time.March, 10))
		_, per := s.current(sc)
		s.expect("S04b anniversary period 2 Feb10-Mar10 addon", "31.00", per["addon"])
		s.expect("S04b anniversary period 2 Feb10-Mar10 plan", "31.00", per["plan"])
	})
}

// 5. Calendar quarterly start Feb 15: 45/90 of the Jan1-Apr1 quarter, then full.
func (s *FixedChargeProrationSuite) TestCalendarQuarterly() {
	spec := pvSubSpec{
		cycle: types.BillingCycleCalendar, period: types.BILLING_PERIOD_QUARTER,
		start: pvUTC(2026, time.February, 15), behavior: types.ProrationBehaviorCreateProrations,
		items: []pvItemSpec{flat("plan", types.BILLING_PERIOD_QUARTER, "90")},
	}
	s.Run("period1_stub", func() {
		sc := s.build(spec)
		s.Equal(pvUTC(2026, time.April, 1), sc.sub.CurrentPeriodEnd)
		total, _ := s.current(sc)
		s.expect("S05a calendar quarterly stub Feb15-Apr1", "45.00", total)
	})
	s.Run("period2_full", func() {
		sc := s.build(spec)
		sc.setCurrent(pvUTC(2026, time.April, 1), pvUTC(2026, time.July, 1))
		total, _ := s.current(sc)
		s.expect("S05b calendar quarterly period 2 Apr1-Jul1", "90.00", total)
	})
}

// 6. Calendar annual start Mar 15 2026: 292/365.
func (s *FixedChargeProrationSuite) TestCalendarAnnual() {
	sc := s.build(pvSubSpec{
		cycle: types.BillingCycleCalendar, period: types.BILLING_PERIOD_ANNUAL,
		start: pvUTC(2026, time.March, 15), behavior: types.ProrationBehaviorCreateProrations,
		items: []pvItemSpec{flat("plan", types.BILLING_PERIOD_ANNUAL, "365")},
	})
	s.Equal(pvUTC(2027, time.January, 1), sc.sub.CurrentPeriodEnd)
	total, _ := s.current(sc)
	s.expect("S06 calendar annual stub Mar15-Jan1 (292/365)", "292.00", total)
}

// 8. Timezone-aware calendar stubs.
func (s *FixedChargeProrationSuite) TestTimezoneCalendarStub() {
	s.Run("asia_kolkata", func() {
		tz := "Asia/Kolkata"
		sc := s.build(pvSubSpec{
			cycle: types.BillingCycleCalendar, period: types.BILLING_PERIOD_MONTHLY, tz: tz,
			start: pvAt(2026, time.February, 15, tz), behavior: types.ProrationBehaviorCreateProrations,
			items: []pvItemSpec{flat("plan", types.BILLING_PERIOD_MONTHLY, "31")},
		})
		s.Equal(pvAt(2026, time.March, 1, tz), sc.sub.CurrentPeriodEnd)
		total, _ := s.current(sc)
		s.expect("S08a Asia/Kolkata calendar monthly stub Feb15-Mar1 (14/28)", "15.50", total)
	})
	s.Run("america_new_york_dst", func() {
		tz := "America/New_York"
		sc := s.build(pvSubSpec{
			cycle: types.BillingCycleCalendar, period: types.BILLING_PERIOD_MONTHLY, tz: tz,
			start: pvAt(2026, time.March, 17, tz), behavior: types.ProrationBehaviorCreateProrations,
			items: []pvItemSpec{flat("plan", types.BILLING_PERIOD_MONTHLY, "31")},
		})
		s.Equal(pvAt(2026, time.April, 1, tz), sc.sub.CurrentPeriodEnd)
		total, _ := s.current(sc)
		// Seconds-based: 15 days / (31 days - 1h DST) = 360/743 -> 31*360/743 = 15.02.
		s.expect("S08b America/New_York calendar monthly stub Mar17-Apr1 (360h/743h)", "15.02", total)
	})
}

// 9. Subscription end date or cancellation mid-period: last period [Mar1, Mar10) = 9/31.
func (s *FixedChargeProrationSuite) TestEndDateMidPeriod() {
	endDate := pvUTC(2026, time.March, 10)
	spec := pvSubSpec{
		cycle: types.BillingCycleCalendar, period: types.BILLING_PERIOD_MONTHLY,
		start: pvUTC(2026, time.January, 1), behavior: types.ProrationBehaviorCreateProrations,
		endDate: &endDate,
		items:   []pvItemSpec{flat("plan", types.BILLING_PERIOD_MONTHLY, "31")},
	}
	s.Run("current_period_clamped_to_end_date", func() {
		sc := s.build(spec)
		periodEnd, err := types.NextBillingDate(&types.NextBillingDateParams{
			CurrentPeriodStart: pvUTC(2026, time.March, 1), BillingAnchor: sc.sub.BillingAnchor, Unit: 1,
			Period: types.BILLING_PERIOD_MONTHLY, SubscriptionEndDate: &endDate, Timezone: "UTC",
		})
		s.Require().NoError(err)
		s.Equal(endDate, periodEnd, "last period should be clamped to end date")
		sc.setCurrent(pvUTC(2026, time.March, 1), periodEnd)
		total, _ := s.current(sc)
		s.expect("S09a end date Mar10, last period Mar1-Mar10 (9/31)", "9.00", total)
	})
	s.Run("line_item_end_date_full_window", func() {
		spec2 := spec
		spec2.items = []pvItemSpec{{key: "plan", period: types.BILLING_PERIOD_MONTHLY, amount: "31", end: endDate}}
		sc := s.build(spec2)
		sc.setCurrent(pvUTC(2026, time.March, 1), pvUTC(2026, time.April, 1))
		total, _ := s.current(sc)
		s.expect("S09b item end Mar10 in window Mar1-Apr1 (9/31)", "9.00", total)
	})
	s.Run("cancelled_sub_last_period", func() {
		sc := s.build(spec)
		sc.sub.SubscriptionStatus = types.SubscriptionStatusCancelled
		sc.sub.CancelledAt = lo.ToPtr(endDate)
		sc.setCurrent(pvUTC(2026, time.March, 1), endDate)
		total, _ := s.current(sc)
		s.expect("S09c cancelled at Mar10, last period Mar1-Mar10 (9/31)", "9.00", total)
	})
}

// 10. Tier-aware money: CalculateCost(price, qty) x coefficient on a 17/31 stub.
func (s *FixedChargeProrationSuite) TestTieredAndPackageStub() {
	base := pvSubSpec{
		cycle: types.BillingCycleCalendar, period: types.BILLING_PERIOD_MONTHLY,
		start: pvUTC(2026, time.January, 15), behavior: types.ProrationBehaviorCreateProrations,
	}
	s.Run("volume_tiers", func() {
		spec := base
		spec.items = []pvItemSpec{{
			key: "plan", period: types.BILLING_PERIOD_MONTHLY, qty: 10,
			model: types.BILLING_MODEL_TIERED, tierMode: types.BILLING_TIER_VOLUME,
			tiers: []price.PriceTier{
				{UpTo: lo.ToPtr(uint64(10)), UnitAmount: decimal.NewFromInt(10)},
				{UpTo: nil, UnitAmount: decimal.NewFromInt(8)},
			},
		}}
		sc := s.build(spec)
		total, _ := s.current(sc)
		// CalculateCost = 10 x $10 = $100; x 17/31 = 54.84.
		s.expect("S10a volume tiers qty10 ($100) on 17/31 stub", "54.84", total)
	})
	s.Run("volume_tiers_qty12", func() {
		spec := base
		spec.items = []pvItemSpec{{
			key: "plan", period: types.BILLING_PERIOD_MONTHLY, qty: 12,
			model: types.BILLING_MODEL_TIERED, tierMode: types.BILLING_TIER_VOLUME,
			tiers: []price.PriceTier{
				{UpTo: lo.ToPtr(uint64(10)), UnitAmount: decimal.NewFromInt(10)},
				{UpTo: nil, UnitAmount: decimal.NewFromInt(8)},
			},
		}}
		sc := s.build(spec)
		total, _ := s.current(sc)
		// CalculateCost = 12 x $8 = $96; x 17/31 = 52.65.
		s.expect("S10b volume tiers qty12 ($96) on 17/31 stub", "52.65", total)
	})
	s.Run("package", func() {
		spec := base
		spec.items = []pvItemSpec{{
			key: "plan", period: types.BILLING_PERIOD_MONTHLY, qty: 7, amount: "20",
			model:     types.BILLING_MODEL_PACKAGE,
			transform: &price.TransformQuantity{DivideBy: 5, Round: types.ROUND_UP},
		}}
		sc := s.build(spec)
		total, _ := s.current(sc)
		// CalculateCost = ceil(7/5)=2 packages x $20 = $40; x 17/31 = 21.94.
		s.expect("S10c package 5/unit $20 qty7 ($40) on 17/31 stub", "21.94", total)
	})
}

// 12. proration_behavior=none: no proration at all.
func (s *FixedChargeProrationSuite) TestProrationNone() {
	s.Run("calendar_monthly_stub", func() {
		sc := s.build(pvSubSpec{
			cycle: types.BillingCycleCalendar, period: types.BILLING_PERIOD_MONTHLY,
			start: pvUTC(2026, time.January, 15), behavior: types.ProrationBehaviorNone,
			items: []pvItemSpec{flat("plan", types.BILLING_PERIOD_MONTHLY, "31")},
		})
		total, _ := s.current(sc)
		s.expect("S12a none: calendar monthly stub Jan15-Feb1", "31.00", total)
	})
	s.Run("anniversary_anchor_ahead", func() {
		sc := s.build(pvSubSpec{
			cycle: types.BillingCycleAnniversary, period: types.BILLING_PERIOD_MONTHLY,
			start: pvUTC(2026, time.January, 10), anchor: pvUTC(2026, time.January, 20),
			behavior: types.ProrationBehaviorNone,
			items:    []pvItemSpec{flat("plan", types.BILLING_PERIOD_MONTHLY, "31")},
		})
		total, _ := s.current(sc)
		s.expect("S12b none: anniversary anchor-ahead stub Jan10-Jan20", "31.00", total)
	})
}

// D7: addon attached Oct 3 00:00 New York on a calendar monthly sub. The divisor is the local
// [Oct 1, Nov 1) month (744h), not a month counted forward from the attach across the DST change.
func (s *FixedChargeProrationSuite) TestAttachBeforeDSTChangeUsesFullPeriod() {
	ctx := s.GetContext()
	tz := "America/New_York"
	attachAt := pvAt(2026, time.October, 3, tz)
	sc := s.build(pvSubSpec{
		cycle: types.BillingCycleCalendar, period: types.BILLING_PERIOD_MONTHLY, tz: tz,
		start: pvAt(2026, time.October, 1, tz), behavior: types.ProrationBehaviorCreateProrations,
		items: []pvItemSpec{
			flat("plan", types.BILLING_PERIOD_MONTHLY, "31"),
			{key: "addon", period: types.BILLING_PERIOD_MONTHLY, amount: "31", start: attachAt,
				entityType: types.SubscriptionLineItemEntityTypeAddon},
		},
	})
	addonPrice, err := s.GetStores().PriceRepo.Get(ctx, sc.items["addon"].PriceID)
	s.Require().NoError(err)

	quote, err := NewLineItemProrationService(s.params).Compute(ctx, LineItemProrationRequest{
		Subscription: sc.sub,
		Entries: []LineItemProrationEntry{{
			LineItem: sc.items["addon"], NewPrice: addonPrice, NewQuantity: decimal.NewFromInt(1),
			Action: types.ProrationActionAddItem,
		}},
		EffectiveDate: attachAt,
		Behavior:      types.ProrationBehaviorCreateProrations,
	})
	s.Require().NoError(err)
	s.expect("S13a New York attach Oct 3 (29/31)", "29.00", quote.TotalChargeAmount)

	_, per := s.current(sc)
	s.expect("S13b New York addon on the opening invoice (29/31)", "29.00", per["addon"])
}
