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
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type SubscriptionLineItemServiceSuite struct {
	testutil.BaseServiceTestSuite
	service  SubscriptionService
	testData struct {
		customer     *customer.Customer
		plan         *plan.Plan
		subscription *subscription.Subscription
		price        *price.Price
		lineItem     *subscription.SubscriptionLineItem
	}
}

func TestSubscriptionLineItemService(t *testing.T) {
	suite.Run(t, new(SubscriptionLineItemServiceSuite))
}

func (s *SubscriptionLineItemServiceSuite) SetupTest() {
	s.BaseServiceTestSuite.SetupTest()
	s.setupService()
	s.setupTestData()
}

func (s *SubscriptionLineItemServiceSuite) TearDownTest() {
	s.BaseServiceTestSuite.TearDownTest()
}

func (s *SubscriptionLineItemServiceSuite) setupService() {
	s.service = NewSubscriptionService(ServiceParams{
		Logger:                     s.GetLogger(),
		Config:                     s.GetConfig(),
		DB:                         s.GetDB(),
		TaxAssociationRepo:         s.GetStores().TaxAssociationRepo,
		TaxRateRepo:                s.GetStores().TaxRateRepo,
		SubRepo:                    s.GetStores().SubscriptionRepo,
		SubscriptionLineItemRepo:   s.GetStores().SubscriptionLineItemRepo,
		SubscriptionPhaseRepo:      s.GetStores().SubscriptionPhaseRepo,
		SubScheduleRepo:            s.GetStores().SubscriptionScheduleRepo,
		PlanRepo:                   s.GetStores().PlanRepo,
		PriceRepo:                  s.GetStores().PriceRepo,
		PriceUnitRepo:              s.GetStores().PriceUnitRepo,
		EventRepo:                  s.GetStores().EventRepo,
		MeterRepo:                  s.GetStores().MeterRepo,
		CustomerRepo:               s.GetStores().CustomerRepo,
		InvoiceRepo:                s.GetStores().InvoiceRepo,
		EntitlementRepo:            s.GetStores().EntitlementRepo,
		EnvironmentRepo:            s.GetStores().EnvironmentRepo,
		FeatureRepo:                s.GetStores().FeatureRepo,
		TenantRepo:                 s.GetStores().TenantRepo,
		UserRepo:                   s.GetStores().UserRepo,
		AuthRepo:                   s.GetStores().AuthRepo,
		WalletRepo:                 s.GetStores().WalletRepo,
		PaymentRepo:                s.GetStores().PaymentRepo,
		CreditGrantRepo:            s.GetStores().CreditGrantRepo,
		CreditGrantApplicationRepo: s.GetStores().CreditGrantApplicationRepo,
		CouponRepo:                 s.GetStores().CouponRepo,
		CouponAssociationRepo:      s.GetStores().CouponAssociationRepo,
		CouponApplicationRepo:      s.GetStores().CouponApplicationRepo,
		AddonRepo:                  testutil.NewInMemoryAddonStore(),
		AddonAssociationRepo:       s.GetStores().AddonAssociationRepo,
		ConnectionRepo:             s.GetStores().ConnectionRepo,
		SettingsRepo:               s.GetStores().SettingsRepo,
		EventPublisher:             s.GetPublisher(),
		WebhookPublisher:           s.GetWebhookPublisher(),
		ProrationCalculator:        s.GetCalculator(),
		FeatureUsageRepo:           s.GetStores().FeatureUsageRepo,
		IntegrationFactory:         s.GetIntegrationFactory(),
	})
}

func (s *SubscriptionLineItemServiceSuite) setupTestData() {
	ctx := s.GetContext()
	now := time.Now().UTC()
	lineItemStart := now.AddDate(0, 0, -3) // 3 days ago for effective-date tests

	s.testData.customer = &customer.Customer{
		ID:         types.GenerateUUIDWithPrefix(types.UUID_PREFIX_CUSTOMER),
		ExternalID: "ext_cust_lineitem",
		Name:       "Line Item Test Customer",
		Email:      "lineitem@example.com",
		BaseModel:  types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().CustomerRepo.Create(ctx, s.testData.customer))

	s.testData.plan = &plan.Plan{
		ID:          types.GenerateUUIDWithPrefix(types.UUID_PREFIX_PLAN),
		Name:        "Line Item Test Plan",
		Description: "Plan for line item tests",
		BaseModel:   types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().PlanRepo.Create(ctx, s.testData.plan))

	s.testData.price = &price.Price{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_PRICE),
		Amount:             decimal.NewFromInt(50),
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           s.testData.plan.ID,
		Type:               types.PRICE_TYPE_FIXED,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().PriceRepo.Create(ctx, s.testData.price))

	s.testData.subscription = &subscription.Subscription{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION),
		PlanID:             s.testData.plan.ID,
		CustomerID:         s.testData.customer.ID,
		StartDate:          now.Add(-30 * 24 * time.Hour),
		CurrentPeriodStart: now.Add(-24 * time.Hour),
		CurrentPeriodEnd:   now.Add(6 * 24 * time.Hour),
		BillingAnchor:      now.Add(-30 * 24 * time.Hour),
		Currency:           "usd",
		BillingCycle:       types.BillingCycleAnniversary,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		SubscriptionStatus: types.SubscriptionStatusActive,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().SubscriptionRepo.Create(ctx, s.testData.subscription))

	s.testData.lineItem = &subscription.SubscriptionLineItem{
		ID:              types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_LINE_ITEM),
		SubscriptionID:  s.testData.subscription.ID,
		CustomerID:      s.testData.customer.ID,
		EntityID:        s.testData.plan.ID,
		EntityType:      types.SubscriptionLineItemEntityTypePlan,
		PlanDisplayName: s.testData.plan.Name,
		PriceID:         s.testData.price.ID,
		PriceType:       s.testData.price.Type,
		DisplayName:     "Test line item",
		Quantity:        decimal.NewFromInt(1),
		Currency:        s.testData.subscription.Currency,
		BillingPeriod:   s.testData.subscription.BillingPeriod,
		InvoiceCadence:  types.InvoiceCadenceAdvance,
		StartDate:       lineItemStart,
		BaseModel:       types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().SubscriptionLineItemRepo.Create(ctx, s.testData.lineItem))
}

func (s *SubscriptionLineItemServiceSuite) TestDeleteSubscriptionLineItem_EffectiveFromBeforeStartDate() {
	ctx := s.GetContext()
	// EffectiveFrom before line item's StartDate (3 days ago)
	effectiveBefore := s.testData.lineItem.StartDate.Add(-24 * time.Hour)

	req := dto.DeleteSubscriptionLineItemRequest{
		EffectiveFrom: &effectiveBefore,
	}

	_, err := s.service.DeleteSubscriptionLineItem(ctx, s.testData.lineItem.ID, req)
	s.Error(err)
	s.Contains(err.Error(), "effective from date must be on or after start date")

	li, err := s.GetStores().SubscriptionLineItemRepo.Get(ctx, s.testData.lineItem.ID)
	s.NoError(err)
	s.True(li.EndDate.IsZero(), "line item should remain unterminated")
}

func (s *SubscriptionLineItemServiceSuite) TestDeleteSubscriptionLineItem_EffectiveFromOnOrAfterStartDate() {
	ctx := s.GetContext()
	effectiveFrom := s.testData.lineItem.StartDate.Add(24 * time.Hour) // 1 day after start

	req := dto.DeleteSubscriptionLineItemRequest{
		EffectiveFrom: &effectiveFrom,
	}

	resp, err := s.service.DeleteSubscriptionLineItem(ctx, s.testData.lineItem.ID, req)
	s.NoError(err)
	s.NotNil(resp)
	s.False(resp.SubscriptionLineItem.EndDate.IsZero())
	s.Equal(effectiveFrom.Truncate(time.Second).Unix(), resp.SubscriptionLineItem.EndDate.Truncate(time.Second).Unix())

	li, err := s.GetStores().SubscriptionLineItemRepo.Get(ctx, s.testData.lineItem.ID)
	s.NoError(err)
	s.Equal(effectiveFrom.Truncate(time.Second).Unix(), li.EndDate.Truncate(time.Second).Unix())
}

func (s *SubscriptionLineItemServiceSuite) TestUpdateSubscriptionLineItem_EffectiveFromBeforeStartDate() {
	ctx := s.GetContext()
	effectiveBefore := s.testData.lineItem.StartDate.Add(-24 * time.Hour)
	newAmount := decimal.NewFromInt(100)

	req := dto.UpdateSubscriptionLineItemRequest{
		Amount:        &newAmount,
		EffectiveFrom: &effectiveBefore,
	}

	_, err := s.service.UpdateSubscriptionLineItem(ctx, s.testData.lineItem.ID, req)
	s.Error(err)
	s.Contains(err.Error(), "effective date must be on or after line item start date")

	li, err := s.GetStores().SubscriptionLineItemRepo.Get(ctx, s.testData.lineItem.ID)
	s.NoError(err)
	s.True(li.EndDate.IsZero())
}

func (s *SubscriptionLineItemServiceSuite) TestUpdateSubscriptionLineItem_EffectiveFromBackdated() {
	ctx := s.GetContext()
	// EffectiveFrom in the past but on or after line item start (e.g. 1 day after start)
	effectiveFrom := s.testData.lineItem.StartDate.Add(24 * time.Hour)
	newAmount := decimal.NewFromInt(200)

	req := dto.UpdateSubscriptionLineItemRequest{
		Amount:        &newAmount,
		EffectiveFrom: &effectiveFrom,
	}

	resp, err := s.service.UpdateSubscriptionLineItem(ctx, s.testData.lineItem.ID, req)
	s.NoError(err)
	s.NotNil(resp)
	s.NotEqual(s.testData.lineItem.ID, resp.SubscriptionLineItem.ID, "new line item should be created")
	s.Equal(effectiveFrom.Truncate(time.Second).Unix(), resp.SubscriptionLineItem.StartDate.Truncate(time.Second).Unix())

	oldLi, err := s.GetStores().SubscriptionLineItemRepo.Get(ctx, s.testData.lineItem.ID)
	s.NoError(err)
	s.False(oldLi.EndDate.IsZero())
	s.Equal(effectiveFrom.Truncate(time.Second).Unix(), oldLi.EndDate.Truncate(time.Second).Unix())
}

func (s *SubscriptionLineItemServiceSuite) TestUpdateSubscriptionLineItem_EffectiveFromWithoutCriticalField() {
	ctx := s.GetContext()
	effectiveFrom := time.Now().UTC().Add(24 * time.Hour)

	req := dto.UpdateSubscriptionLineItemRequest{
		EffectiveFrom: &effectiveFrom,
	}

	_, err := s.service.UpdateSubscriptionLineItem(ctx, s.testData.lineItem.ID, req)
	s.Error(err)
	s.Contains(err.Error(), "effective_from requires at least one critical field")
}

func (s *SubscriptionLineItemServiceSuite) TestAddSubscriptionLineItem_Success() {
	ctx := s.GetContext()
	// Use a second price so we can add another line item
	price2 := &price.Price{
		ID:                 types.GenerateUUIDWithPrefix(types.UUID_PREFIX_PRICE),
		Amount:             decimal.NewFromInt(25),
		Currency:           "usd",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           s.testData.plan.ID,
		Type:               types.PRICE_TYPE_FIXED,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().PriceRepo.Create(ctx, price2))

	req := dto.CreateSubscriptionLineItemRequest{
		PriceID:              price2.ID,
		Quantity:             decimal.NewFromInt(2),
		SkipEntitlementCheck: true,
	}

	resp, err := s.service.AddSubscriptionLineItem(ctx, s.testData.subscription.ID, req)
	s.NoError(err)
	s.NotNil(resp)
	s.NotEmpty(resp.SubscriptionLineItem.ID)
	s.Equal(s.testData.subscription.ID, resp.SubscriptionLineItem.SubscriptionID)
	s.Equal(price2.ID, resp.SubscriptionLineItem.PriceID)
	s.True(resp.SubscriptionLineItem.Quantity.Equal(decimal.NewFromInt(2)))

	_, err = s.GetStores().SubscriptionLineItemRepo.Get(ctx, resp.SubscriptionLineItem.ID)
	s.NoError(err)
}
