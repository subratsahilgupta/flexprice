package service

import (
	"testing"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type SubscriptionChangeServiceTestSuite struct {
	testutil.BaseServiceTestSuite
	subscriptionChangeService SubscriptionChangeService
	subscriptionService       SubscriptionService
	planService               PlanService
	priceService              PriceService
}

func (s *SubscriptionChangeServiceTestSuite) SetupTest() {
	s.BaseServiceTestSuite.SetupTest()
	s.setupServices()
}

func (s *SubscriptionChangeServiceTestSuite) TearDownTest() {
	s.BaseServiceTestSuite.TearDownTest()
}

func (s *SubscriptionChangeServiceTestSuite) setupServices() {
	serviceParams := ServiceParams{
		Logger:                     s.GetLogger(),
		Config:                     s.GetConfig(),
		DB:                         s.GetDB(),
		AuthRepo:                   s.GetStores().AuthRepo,
		UserRepo:                   s.GetStores().UserRepo,
		EventRepo:                  s.GetStores().EventRepo,
		MeterRepo:                  s.GetStores().MeterRepo,
		PriceRepo:                  s.GetStores().PriceRepo,
		CustomerRepo:               s.GetStores().CustomerRepo,
		PlanRepo:                   s.GetStores().PlanRepo,
		SubRepo:                    s.GetStores().SubscriptionRepo,
		WalletRepo:                 s.GetStores().WalletRepo,
		TenantRepo:                 s.GetStores().TenantRepo,
		InvoiceRepo:                s.GetStores().InvoiceRepo,
		FeatureRepo:                s.GetStores().FeatureRepo,
		EntitlementRepo:            s.GetStores().EntitlementRepo,
		PaymentRepo:                s.GetStores().PaymentRepo,
		SecretRepo:                 s.GetStores().SecretRepo,
		EnvironmentRepo:            s.GetStores().EnvironmentRepo,
		TaskRepo:                   s.GetStores().TaskRepo,
		CreditGrantRepo:            s.GetStores().CreditGrantRepo,
		CreditGrantApplicationRepo: s.GetStores().CreditGrantApplicationRepo,
		CouponRepo:                 s.GetStores().CouponRepo,
		CouponAssociationRepo:      s.GetStores().CouponAssociationRepo,
		CouponApplicationRepo:      s.GetStores().CouponApplicationRepo,
		SettingsRepo:               s.GetStores().SettingsRepo,
		EventPublisher:             s.GetPublisher(),
		WebhookPublisher:           s.GetWebhookPublisher(),
		ProrationCalculator:        s.GetCalculator(), // Use the correct method from BaseServiceTestSuite
	}

	s.subscriptionChangeService = NewSubscriptionChangeService(serviceParams)
	s.subscriptionService = NewSubscriptionService(serviceParams)
	s.planService = NewPlanService(serviceParams)
	s.priceService = NewPriceService(serviceParams)
}

func (s *SubscriptionChangeServiceTestSuite) createTestPlan(name string, amount decimal.Decimal) *plan.Plan {
	ctx := s.GetContext()

	planReq := dto.CreatePlanRequest{
		Name:        name,
		Description: "Test plan for subscription changes",
	}

	planResponse, err := s.planService.CreatePlan(ctx, planReq)
	require.NoError(s.T(), err)

	// Create a price for the plan
	priceReq := dto.CreatePriceRequest{
		Amount:         amount.String(),
		Currency:       "usd",
		Type:           types.PRICE_TYPE_FIXED,
		BillingModel:   types.BILLING_MODEL_FLAT_FEE,
		BillingCadence: types.BILLING_CADENCE_RECURRING,
		BillingPeriod:  types.BILLING_PERIOD_MONTHLY,
		EntityType:     types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:       planResponse.Plan.ID,
	}

	_, err = s.priceService.CreatePrice(ctx, priceReq)
	require.NoError(s.T(), err)

	return planResponse.Plan
}

func (s *SubscriptionChangeServiceTestSuite) createTestSubscription(planID, customerID string) *subscription.Subscription {
	ctx := s.GetContext()

	subReq := dto.CreateSubscriptionRequest{
		CustomerID:         customerID,
		PlanID:             planID,
		Currency:           "usd",
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingCycle:       types.BillingCycleAnniversary,
	}

	subResponse, err := s.subscriptionService.CreateSubscription(ctx, subReq)
	require.NoError(s.T(), err)

	// Get the subscription with line items
	sub, _, err := s.GetStores().SubscriptionRepo.GetWithLineItems(ctx, subResponse.Subscription.ID)
	require.NoError(s.T(), err)

	return sub
}

// func (s *SubscriptionChangeServiceTestSuite) TestPreviewSubscriptionUpgrade() {
// 	ctx := s.GetContext()

// 	// Create test data
// 	customer := s.CreateTestCustomer()
// 	basicPlan := s.createTestPlan("Basic", decimal.NewFromFloat(10.00))
// 	premiumPlan := s.createTestPlan("Premium", decimal.NewFromFloat(20.00))
// 	testSub := s.createTestSubscription(basicPlan.ID, customer.ID)

// 	// Create preview request
// 	req := dto.SubscriptionChangePreviewRequest{
// 		SubscriptionChangeRequest: dto.SubscriptionChangeRequest{
// 			TargetPlanID:      premiumPlan.ID,
// 			ProrationBehavior: types.ProrationBehaviorCreateProrations,
// 		},
// 	}

// 	// Test preview
// 	response, err := s.subscriptionChangeService.PreviewSubscriptionChange(ctx, testSub.ID, req)

// 	// Assertions
// 	require.NoError(s.T(), err)
// 	assert.NotNil(s.T(), response)
// 	assert.Equal(s.T(), testSub.ID, response.SubscriptionID)
// 	assert.Equal(s.T(), basicPlan.ID, response.CurrentPlan.ID)
// 	assert.Equal(s.T(), premiumPlan.ID, response.TargetPlan.ID)
// 	assert.Equal(s.T(), types.SubscriptionChangeTypeUpgrade, response.ChangeType)
// 	assert.NotNil(s.T(), response.ProrationDetails)
// 	assert.NotNil(s.T(), response.ImmediateInvoicePreview)
// 	assert.NotNil(s.T(), response.NextInvoicePreview)
// }

// func (s *SubscriptionChangeServiceTestSuite) TestPreviewSubscriptionDowngrade() {
// 	ctx := s.GetContext()

// 	// Create test data
// 	customer := s.CreateTestCustomer()
// 	basicPlan := s.createTestPlan("Basic", decimal.NewFromFloat(10.00))
// 	premiumPlan := s.createTestPlan("Premium", decimal.NewFromFloat(20.00))
// 	testSub := s.createTestSubscription(premiumPlan.ID, customer.ID)

// 	// Create preview request
// 	req := dto.SubscriptionChangePreviewRequest{
// 		SubscriptionChangeRequest: dto.SubscriptionChangeRequest{
// 			TargetPlanID:      basicPlan.ID,
// 			ProrationBehavior: types.ProrationBehaviorCreateProrations,
// 		},
// 	}

// 	// Test preview
// 	response, err := s.subscriptionChangeService.PreviewSubscriptionChange(ctx, testSub.ID, req)

// 	// Assertions
// 	require.NoError(s.T(), err)
// 	assert.NotNil(s.T(), response)
// 	assert.Equal(s.T(), testSub.ID, response.SubscriptionID)
// 	assert.Equal(s.T(), premiumPlan.ID, response.CurrentPlan.ID)
// 	assert.Equal(s.T(), basicPlan.ID, response.TargetPlan.ID)
// 	assert.Equal(s.T(), types.SubscriptionChangeTypeDowngrade, response.ChangeType)
// 	assert.Contains(s.T(), response.Warnings, "This is a downgrade. You may lose access to certain features.")
// }

// func (s *SubscriptionChangeServiceTestSuite) TestPreviewSubscriptionLateral() {
// 	ctx := s.GetContext()

// 	// Create test data
// 	customer := s.CreateTestCustomer()
// 	plan1 := s.createTestPlan("Plan A", decimal.NewFromFloat(15.00))
// 	plan2 := s.createTestPlan("Plan B", decimal.NewFromFloat(15.00))
// 	testSub := s.createTestSubscription(plan1.ID, customer.ID)

// 	// Create preview request
// 	req := dto.SubscriptionChangePreviewRequest{
// 		SubscriptionChangeRequest: dto.SubscriptionChangeRequest{
// 			TargetPlanID:      plan2.ID,
// 			ProrationBehavior: types.ProrationBehaviorCreateProrations,
// 		},
// 	}

// 	// Test preview
// 	response, err := s.subscriptionChangeService.PreviewSubscriptionChange(ctx, testSub.ID, req)

// 	// Assertions
// 	require.NoError(s.T(), err)
// 	assert.NotNil(s.T(), response)
// 	assert.Equal(s.T(), types.SubscriptionChangeTypeLateral, response.ChangeType)
// }

// func (s *SubscriptionChangeServiceTestSuite) TestExecuteSubscriptionUpgrade() {
// 	ctx := s.GetContext()

// 	// Create test data
// 	customer := s.CreateTestCustomer()
// 	basicPlan := s.createTestPlan("Basic", decimal.NewFromFloat(10.00))
// 	premiumPlan := s.createTestPlan("Premium", decimal.NewFromFloat(20.00))
// 	testSub := s.createTestSubscription(basicPlan.ID, customer.ID)
// 	originalSubID := testSub.ID

// 	// Create execute request
// 	req := dto.SubscriptionChangeRequest{
// 		TargetPlanID:      premiumPlan.ID,
// 		ProrationBehavior: types.ProrationBehaviorCreateProrations,
// 		InvoiceNow:        &[]bool{true}[0],
// 	}

// 	// Test execution
// 	response, err := s.subscriptionChangeService.ExecuteSubscriptionChange(ctx, testSub.ID, req)

// 	// Assertions
// 	require.NoError(s.T(), err)
// 	assert.NotNil(s.T(), response)
// 	assert.Equal(s.T(), types.SubscriptionChangeTypeUpgrade, response.ChangeType)
// 	assert.Equal(s.T(), originalSubID, response.OldSubscription.ID)
// 	assert.NotEqual(s.T(), originalSubID, response.NewSubscription.ID)
// 	assert.Equal(s.T(), types.SubscriptionStatusCancelled, response.OldSubscription.Status)
// 	assert.Equal(s.T(), types.SubscriptionStatusActive, response.NewSubscription.Status)
// 	assert.Equal(s.T(), premiumPlan.ID, response.NewSubscription.PlanID)

// 	// Verify old subscription is archived
// 	oldSub, err := s.GetStores().SubscriptionRepo.Get(ctx, originalSubID)
// 	require.NoError(s.T(), err)
// 	assert.Equal(s.T(), types.SubscriptionStatusCancelled, oldSub.SubscriptionStatus)
// 	assert.NotNil(s.T(), oldSub.CancelledAt)

// 	// Verify new subscription exists
// 	newSub, err := s.GetStores().SubscriptionRepo.Get(ctx, response.NewSubscription.ID)
// 	require.NoError(s.T(), err)
// 	assert.Equal(s.T(), types.SubscriptionStatusActive, newSub.SubscriptionStatus)
// 	assert.Equal(s.T(), premiumPlan.ID, newSub.PlanID)
// 	assert.Equal(s.T(), customer.ID, newSub.CustomerID)
// }

// func (s *SubscriptionChangeServiceTestSuite) TestExecuteSubscriptionChangeWithoutProration() {
// 	ctx := s.GetContext()

// 	// Create test data
// 	customer := s.CreateTestCustomer()
// 	basicPlan := s.createTestPlan("Basic", decimal.NewFromFloat(10.00))
// 	premiumPlan := s.createTestPlan("Premium", decimal.NewFromFloat(20.00))
// 	testSub := s.createTestSubscription(basicPlan.ID, customer.ID)

// 	// Create execute request without proration
// 	req := dto.SubscriptionChangeRequest{
// 		TargetPlanID:      premiumPlan.ID,
// 		ProrationBehavior: types.ProrationBehaviorNone,
// 		InvoiceNow:        &[]bool{false}[0],
// 	}

// 	// Test execution
// 	response, err := s.subscriptionChangeService.ExecuteSubscriptionChange(ctx, testSub.ID, req)

// 	// Assertions
// 	require.NoError(s.T(), err)
// 	assert.NotNil(s.T(), response)
// 	assert.Nil(s.T(), response.ProrationApplied)
// 	assert.Nil(s.T(), response.Invoice)
// }

// func (s *SubscriptionChangeServiceTestSuite) TestPreviewSubscriptionChangeValidation() {
// 	ctx := s.GetContext()

// 	// Test with invalid subscription ID
// 	req := dto.SubscriptionChangePreviewRequest{
// 		SubscriptionChangeRequest: dto.SubscriptionChangeRequest{
// 			TargetPlanID:      "invalid-plan-id",
// 			ProrationBehavior: types.ProrationBehaviorCreateProrations,
// 		},
// 	}

// 	_, err := s.subscriptionChangeService.PreviewSubscriptionChange(ctx, "invalid-sub-id", req)
// 	assert.Error(s.T(), err)
// }

// func (s *SubscriptionChangeServiceTestSuite) TestExecuteSubscriptionChangeValidation() {
// 	ctx := s.GetContext()

// 	// Test with invalid subscription ID
// 	req := dto.SubscriptionChangeRequest{
// 		TargetPlanID:      "invalid-plan-id",
// 		ProrationBehavior: types.ProrationBehaviorCreateProrations,
// 	}

// 	_, err := s.subscriptionChangeService.ExecuteSubscriptionChange(ctx, "invalid-sub-id", req)
// 	assert.Error(s.T(), err)
// }

// func (s *SubscriptionChangeServiceTestSuite) TestValidateSubscriptionForChange() {
// 	ctx := s.GetContext()

// 	// Create test data
// 	customer := s.CreateTestCustomer()
// 	basicPlan := s.createTestPlan("Basic", decimal.NewFromFloat(10.00))
// 	testSub := s.createTestSubscription(basicPlan.ID, customer.ID)

// 	// Test with active subscription (should pass)
// 	testSub.SubscriptionStatus = types.SubscriptionStatusActive
// 	err := s.subscriptionChangeService.(*subscriptionChangeService).validateSubscriptionForChange(testSub)
// 	assert.NoError(s.T(), err)

// 	// Test with cancelled subscription (should fail)
// 	testSub.SubscriptionStatus = types.SubscriptionStatusCancelled
// 	err = s.subscriptionChangeService.(*subscriptionChangeService).validateSubscriptionForChange(testSub)
// 	assert.Error(s.T(), err)

// 	// Test with paused subscription (should fail)
// 	testSub.SubscriptionStatus = types.SubscriptionStatusPaused
// 	err = s.subscriptionChangeService.(*subscriptionChangeService).validateSubscriptionForChange(testSub)
// 	assert.Error(s.T(), err)
// }

// func (s *SubscriptionChangeServiceTestSuite) TestDetermineChangeType() {
// 	ctx := s.GetContext()

// 	// Create test plans with different prices
// 	basicPlan := s.createTestPlan("Basic", decimal.NewFromFloat(10.00))
// 	premiumPlan := s.createTestPlan("Premium", decimal.NewFromFloat(20.00))
// 	samePricePlan := s.createTestPlan("Alternative", decimal.NewFromFloat(10.00))

// 	service := s.subscriptionChangeService.(*subscriptionChangeService)

// 	// Test upgrade
// 	changeType, err := service.determineChangeType(ctx, basicPlan, premiumPlan)
// 	require.NoError(s.T(), err)
// 	assert.Equal(s.T(), types.SubscriptionChangeTypeUpgrade, changeType)

// 	// Test downgrade
// 	changeType, err = service.determineChangeType(ctx, premiumPlan, basicPlan)
// 	require.NoError(s.T(), err)
// 	assert.Equal(s.T(), types.SubscriptionChangeTypeDowngrade, changeType)

// 	// Test lateral change
// 	changeType, err = service.determineChangeType(ctx, basicPlan, samePricePlan)
// 	require.NoError(s.T(), err)
// 	assert.Equal(s.T(), types.SubscriptionChangeTypeLateral, changeType)

// 	// Test same plan
// 	changeType, err = service.determineChangeType(ctx, basicPlan, basicPlan)
// 	require.NoError(s.T(), err)
// 	assert.Equal(s.T(), types.SubscriptionChangeTypeLateral, changeType)
// }

// func (s *SubscriptionChangeServiceTestSuite) TestCalculatePeriodEnd() {
// 	service := s.subscriptionChangeService.(*subscriptionChangeService)
// 	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

// 	// Test daily
// 	end := service.calculatePeriodEnd(start, types.BILLING_PERIOD_DAILY, 7)
// 	expected := start.AddDate(0, 0, 7)
// 	assert.Equal(s.T(), expected, end)

// 	// Test weekly
// 	end = service.calculatePeriodEnd(start, types.BILLING_PERIOD_WEEKLY, 2)
// 	expected = start.AddDate(0, 0, 14)
// 	assert.Equal(s.T(), expected, end)

// 	// Test monthly
// 	end = service.calculatePeriodEnd(start, types.BILLING_PERIOD_MONTHLY, 3)
// 	expected = start.AddDate(0, 3, 0)
// 	assert.Equal(s.T(), expected, end)

// 	// Test annual
// 	end = service.calculatePeriodEnd(start, types.BILLING_PERIOD_ANNUAL, 1)
// 	expected = start.AddDate(1, 0, 0)
// 	assert.Equal(s.T(), expected, end)
// }

// func (s *SubscriptionChangeServiceTestSuite) TestGenerateWarnings() {
// 	service := s.subscriptionChangeService.(*subscriptionChangeService)

// 	// Create test subscription
// 	testSub := &subscription.Subscription{
// 		TrialEnd: &[]time.Time{time.Now().Add(24 * time.Hour)}[0],
// 	}

// 	// Create test plan
// 	testPlan := &plan.Plan{
// 		Name: "Test Plan",
// 	}

// 	// Test downgrade warnings
// 	warnings := service.generateWarnings(testSub, testPlan, types.SubscriptionChangeTypeDowngrade, types.ProrationBehaviorCreateProrations)
// 	assert.Contains(s.T(), warnings, "This is a downgrade. You may lose access to certain features.")
// 	assert.Contains(s.T(), warnings, "Changing plans during trial period may end your trial immediately.")
// 	assert.Contains(s.T(), warnings, "Proration charges or credits will be applied to your next invoice.")

// 	// Test upgrade warnings (no downgrade warning)
// 	warnings = service.generateWarnings(testSub, testPlan, types.SubscriptionChangeTypeUpgrade, types.ProrationBehaviorNone)
// 	assert.NotContains(s.T(), warnings, "This is a downgrade. You may lose access to certain features.")
// 	assert.Contains(s.T(), warnings, "Changing plans during trial period may end your trial immediately.")
// 	assert.NotContains(s.T(), warnings, "Proration charges or credits will be applied to your next invoice.")
// }

func TestSubscriptionChangeServiceTestSuite(t *testing.T) {
	suite.Run(t, new(SubscriptionChangeServiceTestSuite))
}
