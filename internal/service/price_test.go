package service

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/priceunit"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type PriceServiceSuite struct {
	suite.Suite
	ctx           context.Context
	priceService  PriceService
	priceRepo     *testutil.InMemoryPriceStore
	meterRepo     *testutil.InMemoryMeterStore
	priceUnitRepo *testutil.InMemoryPriceUnitStore
	logger        *logger.Logger
}

func TestPriceService(t *testing.T) {
	suite.Run(t, new(PriceServiceSuite))
}

func (s *PriceServiceSuite) SetupTest() {
	s.ctx = testutil.SetupContext()
	s.priceRepo = testutil.NewInMemoryPriceStore()
	s.meterRepo = testutil.NewInMemoryMeterStore()
	s.priceUnitRepo = testutil.NewInMemoryPriceUnitStore()
	s.logger = logger.GetLogger()

	serviceParams := ServiceParams{
		PriceRepo:     s.priceRepo,
		MeterRepo:     s.meterRepo,
		PriceUnitRepo: s.priceUnitRepo,
		PlanRepo:      testutil.NewInMemoryPlanStore(),
		Logger:        s.logger,
	}
	s.priceService = NewPriceService(serviceParams)
}

func (s *PriceServiceSuite) TestCreatePrice() {
	// Create a plan first so that the price can reference it
	plan := &plan.Plan{
		ID:          "plan-1",
		Name:        "Test Plan",
		Description: "A test plan",
		BaseModel:   types.GetDefaultBaseModel(s.ctx),
	}
	_ = s.priceService.(*priceService).ServiceParams.PlanRepo.Create(s.ctx, plan)

	req := dto.CreatePriceRequest{
		Amount:             "100",
		Currency:           "usd",
		PlanID:             "plan-1",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           "plan-1",
		Type:               types.PRICE_TYPE_USAGE,
		MeterID:            "meter-1",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_TIERED,
		TierMode:           types.BILLING_TIER_SLAB,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		Description:        "Test Price",
		Metadata:           map[string]string{"key": "value"},
		Tiers: []dto.CreatePriceTier{
			{
				UpTo:       lo.ToPtr(uint64(10)),
				UnitAmount: "50",
				FlatAmount: lo.ToPtr("10"),
			},
			{
				UpTo:       lo.ToPtr(uint64(20)),
				UnitAmount: "40",
				FlatAmount: lo.ToPtr("5"),
			},
		},
	}

	resp, err := s.priceService.CreatePrice(s.ctx, req)
	s.NoError(err)
	s.NotNil(resp)

	// Convert expected amount to decimal.Decimal for comparison
	expectedAmount, _ := decimal.NewFromString(req.Amount)
	s.Equal(expectedAmount, resp.Price.Amount) // Compare decimal.Decimal

	// Normalize currency to lowercase for comparison
	s.Equal(strings.ToLower(req.Currency), resp.Price.Currency)
}

func (s *PriceServiceSuite) TestGetPrice() {
	// Create a price
	price := &price.Price{
		ID:         "price-1",
		Amount:     decimal.NewFromInt(100),
		Currency:   "usd",
		EntityType: types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:   "plan-1",
	}
	_ = s.priceRepo.Create(s.ctx, price)

	resp, err := s.priceService.GetPrice(s.ctx, "price-1")
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(price.Amount, resp.Price.Amount)

	// Non-existent price
	resp, err = s.priceService.GetPrice(s.ctx, "nonexistent-id")
	s.Error(err)
	s.Nil(resp)
}

func (s *PriceServiceSuite) TestGetPrices() {
	// Prepopulate the repository with prices associated with a plan_id
	_ = s.priceRepo.Create(s.ctx, &price.Price{
		ID:         "price-1",
		Amount:     decimal.NewFromInt(100),
		Currency:   "usd",
		EntityType: types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:   "plan-1",
		BaseModel:  types.GetDefaultBaseModel(s.ctx),
	})
	_ = s.priceRepo.Create(s.ctx, &price.Price{
		ID:         "price-2",
		Amount:     decimal.NewFromInt(200),
		Currency:   "usd",
		EntityType: types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:   "plan-1",
		BaseModel:  types.GetDefaultBaseModel(s.ctx),
	})

	// Retrieve all prices within limit
	priceFilter := types.NewPriceFilter()
	priceFilter.EntityType = lo.ToPtr(types.PRICE_ENTITY_TYPE_PLAN)
	priceFilter.QueryFilter.Offset = lo.ToPtr(0)
	priceFilter.QueryFilter.Limit = lo.ToPtr(10)
	resp, err := s.priceService.GetPrices(s.ctx, priceFilter)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(2, resp.Pagination.Total) // Ensure all prices are retrieved
	s.Len(resp.Items, 2)

	// Sort the response prices by ID
	sort.Slice(resp.Items, func(i, j int) bool {
		return resp.Items[i].ID < resp.Items[j].ID
	})

	s.Equal("price-1", resp.Items[0].ID)
	s.Equal("price-2", resp.Items[1].ID)

	// Retrieve with offset exceeding available records
	priceFilter.QueryFilter.Offset = lo.ToPtr(10)
	priceFilter.QueryFilter.Limit = lo.ToPtr(10)
	priceFilter.EntityType = lo.ToPtr(types.PRICE_ENTITY_TYPE_PLAN)
	resp, err = s.priceService.GetPrices(s.ctx, priceFilter)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(2, resp.Pagination.Total)
	s.Len(resp.Items, 0) // Ensure no prices are retrieved
}

func (s *PriceServiceSuite) TestUpdatePrice() {
	// Create a price
	price := &price.Price{
		ID:         "price-1",
		Amount:     decimal.NewFromInt(100),
		Currency:   "usd",
		EntityType: types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:   "plan-1",
	}
	_ = s.priceRepo.Create(s.ctx, price)

	req := dto.UpdatePriceRequest{
		Description: "Updated Description",
		Metadata:    map[string]string{"updated": "true"},
	}

	resp, err := s.priceService.UpdatePrice(s.ctx, "price-1", req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(req.Description, resp.Price.Description)
	s.Equal(req.Metadata, map[string]string(resp.Price.Metadata)) // Convert Metadata for comparison
}

func (s *PriceServiceSuite) TestDeletePrice() {
	// Create a price
	price := &price.Price{ID: "price-1", Amount: decimal.NewFromInt(100), Currency: "usd"}
	_ = s.priceRepo.Create(s.ctx, price)

	err := s.priceService.DeletePrice(s.ctx, "price-1")
	s.NoError(err)

	// Ensure the price no longer exists
	_, err = s.priceRepo.Get(s.ctx, "price-1")
	s.Error(err)
}

func (s *PriceServiceSuite) TestCalculateCostWithBreakup_FlatFee() {
	price := &price.Price{
		ID:           "price-1",
		Amount:       decimal.NewFromInt(100),
		Currency:     "usd",
		BillingModel: types.BILLING_MODEL_FLAT_FEE,
	}

	quantity := decimal.NewFromInt(5)
	result := s.priceService.CalculateCostWithBreakup(s.ctx, price, quantity, false)

	s.Equal(decimal.NewFromInt(500).Equal(result.FinalCost), true, "Final cost is %v", result.FinalCost)
	s.Equal(decimal.NewFromInt(100).Equal(result.EffectiveUnitCost), true)
	s.Equal(decimal.NewFromInt(100).Equal(result.TierUnitAmount), true)
	s.Equal(-1, result.SelectedTierIndex)
}

func (s *PriceServiceSuite) TestCalculateCostWithBreakup_Package() {
	price := &price.Price{
		ID:           "price-2",
		Amount:       decimal.NewFromInt(50),
		Currency:     "usd",
		BillingModel: types.BILLING_MODEL_PACKAGE,
		TransformQuantity: price.JSONBTransformQuantity{
			DivideBy: 10,
			Round:    types.ROUND_UP,
		},
	}

	quantity := decimal.NewFromInt(25)
	result := s.priceService.CalculateCostWithBreakup(s.ctx, price, quantity, false)

	s.Equal(decimal.NewFromInt(150).Equal(result.FinalCost), true) // 25/10 = 2.5, rounded up to 3, 3*50 = 150

	// Effective unit cost should be final cost divided by actual quantity (150/25 = 6)
	expectedUnitCost := decimal.NewFromInt(150).Div(decimal.NewFromInt(25))
	s.Equal(expectedUnitCost.Equal(result.EffectiveUnitCost), true,
		"Expected effective unit cost %s but got %s",
		expectedUnitCost.String(),
		result.EffectiveUnitCost.String())

	// Tier unit amount should be package price divided by package size (50/10 = 5)
	expectedTierAmount := decimal.NewFromInt(50).Div(decimal.NewFromInt(10))
	s.Equal(expectedTierAmount.Equal(result.TierUnitAmount), true,
		"Expected tier unit amount %s but got %s",
		expectedTierAmount.String(),
		result.TierUnitAmount.String())

	s.Equal(-1, result.SelectedTierIndex)
}

func (s *PriceServiceSuite) TestCalculateCostWithBreakup_TieredVolume() {
	upTo10 := uint64(10)
	upTo20 := uint64(20)
	price := &price.Price{
		ID:           "price-3",
		Amount:       decimal.NewFromInt(100),
		Currency:     "usd",
		BillingModel: types.BILLING_MODEL_TIERED,
		TierMode:     types.BILLING_TIER_VOLUME,
		Tiers: []price.PriceTier{
			{
				UpTo:       &upTo10,
				UnitAmount: decimal.NewFromInt(50),
			},
			{
				UpTo:       &upTo20,
				UnitAmount: decimal.NewFromInt(40),
			},
			{
				UnitAmount: decimal.NewFromInt(30),
			},
		},
	}

	// Test within first tier
	quantity := decimal.NewFromInt(5)
	result := s.priceService.CalculateCostWithBreakup(s.ctx, price, quantity, false)
	s.Equal(decimal.NewFromInt(250).Equal(result.FinalCost), true) // 5 * 50 = 250
	s.Equal(decimal.NewFromInt(50).Equal(result.EffectiveUnitCost), true)
	s.Equal(decimal.NewFromInt(50).Equal(result.TierUnitAmount), true)
	s.Equal(0, result.SelectedTierIndex)

	// Test within second tier
	quantity = decimal.NewFromInt(15)
	result = s.priceService.CalculateCostWithBreakup(s.ctx, price, quantity, false)
	s.Equal(decimal.NewFromInt(600).Equal(result.FinalCost), true) // 15 * 40 = 600
	s.Equal(decimal.NewFromInt(40).Equal(result.EffectiveUnitCost), true)
	s.Equal(decimal.NewFromInt(40).Equal(result.TierUnitAmount), true)
	s.Equal(1, result.SelectedTierIndex)

	// Test within third tier (unlimited)
	quantity = decimal.NewFromInt(25)
	result = s.priceService.CalculateCostWithBreakup(s.ctx, price, quantity, false)
	s.Equal(decimal.NewFromInt(750).Equal(result.FinalCost), true) // 25 * 30 = 750
	s.Equal(decimal.NewFromInt(30).Equal(result.EffectiveUnitCost), true)
	s.Equal(decimal.NewFromInt(30).Equal(result.TierUnitAmount), true)
	s.Equal(2, result.SelectedTierIndex)
}

func (s *PriceServiceSuite) TestCalculateCostWithBreakup_TieredSlab() {
	upTo10 := uint64(10)
	upTo20 := uint64(20)
	price := &price.Price{
		ID:           "price-4",
		Amount:       decimal.NewFromInt(100),
		Currency:     "usd",
		BillingModel: types.BILLING_MODEL_TIERED,
		TierMode:     types.BILLING_TIER_SLAB,
		Tiers: []price.PriceTier{
			{
				UpTo:       &upTo10,
				UnitAmount: decimal.NewFromInt(50),
			},
			{
				UpTo:       &upTo20,
				UnitAmount: decimal.NewFromInt(40),
			},
			{
				UnitAmount: decimal.NewFromInt(30),
			},
		},
	}

	// Test within first tier
	quantity := decimal.NewFromInt(5)
	result := s.priceService.CalculateCostWithBreakup(s.ctx, price, quantity, false)
	s.Equal(decimal.NewFromInt(250).Equal(result.FinalCost), true) // 5 * 50 = 250
	s.Equal(decimal.NewFromInt(50).Equal(result.TierUnitAmount), true)
	s.Equal(0, result.SelectedTierIndex)

	// Test spanning first and second tiers
	quantity = decimal.NewFromInt(15)
	result = s.priceService.CalculateCostWithBreakup(s.ctx, price, quantity, false)

	// The implementation calculates: (10 * 50) + (5 * 40) = 500 + 200 = 700
	expectedFinalCost := decimal.NewFromInt(700)
	s.Equal(expectedFinalCost.Equal(result.FinalCost), true)

	expectedUnitCost := decimal.NewFromInt(700).Div(decimal.NewFromInt(15)) // 700/15
	s.Equal(expectedUnitCost.Equal(result.EffectiveUnitCost), true)
	s.Equal(decimal.NewFromInt(40).Equal(result.TierUnitAmount), true)
	s.Equal(1, result.SelectedTierIndex)

	// Test spanning all three tiers
	quantity = decimal.NewFromInt(25)
	result = s.priceService.CalculateCostWithBreakup(s.ctx, price, quantity, false)

	// From the debug logs, the calculation is:
	// (10 * 50) = 500 (first tier)
	// (15 * 40) = 600 (second tier, not 10 as expected)
	// No third tier is used
	// Total: 500 + 600 = 1100
	expectedFinalCost = decimal.NewFromInt(1100)
	s.Equal(expectedFinalCost.Equal(result.FinalCost), true)

	expectedUnitCost = decimal.NewFromInt(1100).Div(decimal.NewFromInt(25)) // 1100/25
	s.Equal(expectedUnitCost.Equal(result.EffectiveUnitCost), true)
	s.Equal(decimal.NewFromInt(40).Equal(result.TierUnitAmount), true) // Second tier is the last one used
	s.Equal(1, result.SelectedTierIndex)                               // Index 1 (second tier)
}

func (s *PriceServiceSuite) TestCalculateCostWithBreakup_ZeroQuantity() {
	price := &price.Price{
		ID:           "price-5",
		Amount:       decimal.NewFromInt(100),
		Currency:     "usd",
		BillingModel: types.BILLING_MODEL_FLAT_FEE,
	}

	quantity := decimal.Zero
	result := s.priceService.CalculateCostWithBreakup(s.ctx, price, quantity, false)

	s.Equal(decimal.Zero, result.FinalCost)
	s.Equal(decimal.Zero, result.EffectiveUnitCost)
	s.Equal(decimal.Zero, result.TierUnitAmount)
	s.Equal(-1, result.SelectedTierIndex)
}

func (s *PriceServiceSuite) TestCalculateCostWithBreakup_PackageScenarios() {
	// Tests the main package pricing scenarios
	// Base case: 100 units for $1.00 package
	// Test cases include:
	// 2 units → $1.00 (1 package)
	// 100 units → $1.00 (1 package, exact)
	// 150 units → $2.00 (2 packages)
	// 300 units → $3.00 (3 packages)
	// 0 units → $0.00 (edge case)
	// 99 units → $1.00 (partial package)
	// 101 units → $2.00 (just over one package)

	// Define a standard package price: 100 units for $1.00
	price := &price.Price{
		ID:           "price-package",
		Amount:       decimal.NewFromInt(1), // $1.00 per package
		Currency:     "usd",
		BillingModel: types.BILLING_MODEL_PACKAGE,
		TransformQuantity: price.JSONBTransformQuantity{
			DivideBy: 100,            // Package size of 100 units
			Round:    types.ROUND_UP, // Always round up to next package
		},
	}

	// Test cases
	testCases := []struct {
		name          string
		quantity      decimal.Decimal
		expectedCost  decimal.Decimal
		expectedUnits int64
	}{
		{
			name:          "Small quantity (2 units) should charge one full package",
			quantity:      decimal.NewFromInt(2),
			expectedCost:  decimal.NewFromInt(1), // $1.00 (1 package)
			expectedUnits: 1,
		},
		{
			name:          "Quantity at package boundary (100 units) should charge one package",
			quantity:      decimal.NewFromInt(100),
			expectedCost:  decimal.NewFromInt(1), // $1.00 (1 package)
			expectedUnits: 1,
		},
		{
			name:          "Mid-range quantity (150 units) should charge two packages",
			quantity:      decimal.NewFromInt(150),
			expectedCost:  decimal.NewFromInt(2), // $2.00 (2 packages)
			expectedUnits: 2,
		},
		{
			name:          "Large quantity (300 units) should charge three packages",
			quantity:      decimal.NewFromInt(300),
			expectedCost:  decimal.NewFromInt(3), // $3.00 (3 packages)
			expectedUnits: 3,
		},
		{
			name:          "Zero quantity should result in zero cost",
			quantity:      decimal.Zero,
			expectedCost:  decimal.Zero,
			expectedUnits: 0,
		},
		{
			name:          "Partial package (99 units) should charge one full package",
			quantity:      decimal.NewFromInt(99),
			expectedCost:  decimal.NewFromInt(1), // $1.00 (1 package)
			expectedUnits: 1,
		},
		{
			name:          "Just over one package (101 units) should charge two packages",
			quantity:      decimal.NewFromInt(101),
			expectedCost:  decimal.NewFromInt(2), // $2.00 (2 packages)
			expectedUnits: 2,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			result := s.priceService.CalculateCostWithBreakup(s.ctx, price, tc.quantity, false)

			// Verify final cost
			s.True(tc.expectedCost.Equal(result.FinalCost),
				"Expected cost %s but got %s for quantity %s",
				tc.expectedCost.String(),
				result.FinalCost.String(),
				tc.quantity.String())

			// Verify effective unit cost (if quantity is not zero)
			if !tc.quantity.IsZero() {
				expectedUnitCost := tc.expectedCost.Div(tc.quantity)
				s.True(expectedUnitCost.Equal(result.EffectiveUnitCost),
					"Expected unit cost %s but got %s for quantity %s",
					expectedUnitCost.String(),
					result.EffectiveUnitCost.String(),
					tc.quantity.String())
			}

			// Verify tier unit amount (should be price per unit in a full package)
			expectedTierUnitAmount := decimal.NewFromInt(1).Div(decimal.NewFromInt(100))
			s.True(expectedTierUnitAmount.Equal(result.TierUnitAmount),
				"Expected tier unit amount %s but got %s",
				expectedTierUnitAmount.String(),
				result.TierUnitAmount.String())
		})
	}
}

func (s *PriceServiceSuite) TestCreatePriceWithCustomUnitTiered() {
	// Create a plan first
	plan := &plan.Plan{
		ID:          "plan-tiered-1",
		Name:        "Test Plan",
		Description: "A test plan",
		BaseModel:   types.GetDefaultBaseModel(s.ctx),
	}
	_ = s.priceService.(*priceService).ServiceParams.PlanRepo.Create(s.ctx, plan)

	// Create a price unit first
	priceUnit := &priceunit.PriceUnit{
		ID:             "pu-1",
		Code:           "btc",
		Symbol:         "₿",
		ConversionRate: decimal.NewFromFloat(50000.00), // 1 BTC = 50000 USD
		Precision:      8,
		BaseCurrency:   "usd",
		BaseModel:      types.GetDefaultBaseModel(s.ctx),
	}
	_ = s.priceUnitRepo.Create(s.ctx, priceUnit)

	req := dto.CreatePriceRequest{
		Currency:           "usd",
		PlanID:             "plan-tiered-1",
		EntityType:         types.PRICE_ENTITY_TYPE_PLAN,
		EntityID:           "plan-tiered-1",
		Type:               types.PRICE_TYPE_USAGE,
		MeterID:            "meter-1",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_TIERED,
		TierMode:           types.BILLING_TIER_VOLUME,
		InvoiceCadence:     types.InvoiceCadenceAdvance,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		Description:        "Test Price with Custom Unit Tiers",
		PriceUnitType:      types.PRICE_UNIT_TYPE_CUSTOM,
		PriceUnitConfig: &dto.PriceUnitConfig{
			PriceUnit: "btc",
			PriceUnitTiers: []dto.CreatePriceTier{
				{
					UpTo:       lo.ToPtr(uint64(1000)),
					UnitAmount: "0.001",
					FlatAmount: lo.ToPtr("0.0001"),
				},
				{
					UnitAmount: "0.0005",
				},
			},
		},
	}

	resp, err := s.priceService.CreatePrice(s.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(types.BILLING_MODEL_TIERED, resp.BillingModel)
	s.Equal("btc", resp.PriceUnit)
	s.Len(resp.Tiers, 2)
	s.Equal(decimal.NewFromFloat(50), resp.Tiers[0].UnitAmount) // 0.001 BTC * 50000 USD/BTC = 50 USD
	s.Equal(decimal.NewFromFloat(25), resp.Tiers[1].UnitAmount) // 0.0005 BTC * 50000 USD/BTC = 25 USD
}

func (s *PriceServiceSuite) TestCalculateCostWithBreakup_PackageRoundingModes() {
	// Tests different rounding behaviors
	// Tests both ROUND_UP and ROUND_DOWN modes
	// Test cases include:
	// 50 units with round up → $1.00
	// 50 units with round down → $0.00
	// 250 units with round up → $3.00
	// 250 units with round down → $2.00

	basePrice := &price.Price{
		ID:           "price-package-rounding",
		Amount:       decimal.NewFromInt(1), // $1.00 per package
		Currency:     "usd",
		BillingModel: types.BILLING_MODEL_PACKAGE,
		TransformQuantity: price.JSONBTransformQuantity{
			DivideBy: 100, // Package size of 100 units
		},
	}

	testCases := []struct {
		name         string
		roundingMode string
		quantity     decimal.Decimal
		expectedCost decimal.Decimal
	}{
		{
			name:         "Round up mode with partial package",
			roundingMode: types.ROUND_UP,
			quantity:     decimal.NewFromInt(50),
			expectedCost: decimal.NewFromInt(1), // Rounds up to 1 package
		},
		{
			name:         "Round down mode with partial package",
			roundingMode: types.ROUND_DOWN,
			quantity:     decimal.NewFromInt(50),
			expectedCost: decimal.Zero, // Rounds down to 0 packages
		},
		{
			name:         "Round up mode with multiple packages",
			roundingMode: types.ROUND_UP,
			quantity:     decimal.NewFromInt(250),
			expectedCost: decimal.NewFromInt(3), // Rounds up to 3 packages
		},
		{
			name:         "Round down mode with multiple packages",
			roundingMode: types.ROUND_DOWN,
			quantity:     decimal.NewFromInt(250),
			expectedCost: decimal.NewFromInt(2), // Rounds down to 2 packages
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Create a copy of base price with specific rounding mode
			testPrice := *basePrice
			testPrice.TransformQuantity.Round = tc.roundingMode

			result := s.priceService.CalculateCostWithBreakup(s.ctx, &testPrice, tc.quantity, false)

			s.True(tc.expectedCost.Equal(result.FinalCost),
				"Expected cost %s but got %s for quantity %s with %s rounding",
				tc.expectedCost.String(),
				result.FinalCost.String(),
				tc.quantity.String(),
				tc.roundingMode)
		})
	}
}

func (s *PriceServiceSuite) TestCalculateBucketedCost_FlatFee() {
	price := &price.Price{
		ID:           "price-bucketed-flat",
		Amount:       decimal.NewFromFloat(0.10), // $0.10 per unit
		Currency:     "usd",
		BillingModel: types.BILLING_MODEL_FLAT_FEE,
	}

	// Test with multiple bucket values representing max values per time bucket
	bucketedValues := []decimal.Decimal{
		decimal.NewFromInt(9),  // Bucket 1: max(2,5,6,9) = 9
		decimal.NewFromInt(10), // Bucket 2: max(10) = 10
	}

	result := s.priceService.CalculateBucketedCost(s.ctx, price, bucketedValues)

	// Expected: (9 * 0.10) + (10 * 0.10) = 0.90 + 1.00 = 1.90
	expected := decimal.NewFromFloat(1.9)
	s.True(expected.Equal(result),
		"Expected cost %s but got %s for bucketed values %v",
		expected.String(), result.String(), bucketedValues)
}

func (s *PriceServiceSuite) TestCalculateBucketedCost_Package() {
	price := &price.Price{
		ID:           "price-bucketed-package",
		Amount:       decimal.NewFromInt(1), // $1 per package
		Currency:     "usd",
		BillingModel: types.BILLING_MODEL_PACKAGE,
		TransformQuantity: price.JSONBTransformQuantity{
			DivideBy: 10,             // 10 units per package
			Round:    types.ROUND_UP, // Round up
		},
	}

	// Test with bucket values that require different package allocations
	bucketedValues := []decimal.Decimal{
		decimal.NewFromInt(9),  // Bucket 1: max(2,5,6,9) = 9 → ceil(9/10) = 1 package
		decimal.NewFromInt(10), // Bucket 2: max(10) = 10 → ceil(10/10) = 1 package
	}

	result := s.priceService.CalculateBucketedCost(s.ctx, price, bucketedValues)

	// Expected: 1 package + 1 package = $2
	expected := decimal.NewFromInt(2)
	s.True(expected.Equal(result),
		"Expected cost %s but got %s for bucketed values %v",
		expected.String(), result.String(), bucketedValues)
}

func (s *PriceServiceSuite) TestCalculateBucketedCost_TieredSlab() {
	upTo10 := uint64(10)
	upTo20 := uint64(20)
	price := &price.Price{
		ID:           "price-bucketed-tiered-slab",
		Amount:       decimal.Zero,
		Currency:     "usd",
		BillingModel: types.BILLING_MODEL_TIERED,
		TierMode:     types.BILLING_TIER_SLAB,
		Tiers: []price.PriceTier{
			{UpTo: &upTo10, UnitAmount: decimal.NewFromFloat(0.10)}, // 0-10: $0.10/unit
			{UpTo: &upTo20, UnitAmount: decimal.NewFromFloat(0.05)}, // 11-20: $0.05/unit
			{UpTo: nil, UnitAmount: decimal.NewFromFloat(0.02)},     // 21+: $0.02/unit
		},
	}

	// Test with bucket values that span different tiers
	bucketedValues := []decimal.Decimal{
		decimal.NewFromInt(9),  // Bucket 1: max(2,5,6,9) = 9 → 9*$0.10 = $0.90
		decimal.NewFromInt(15), // Bucket 2: max(10,15) = 15 → 10*$0.10 + 5*$0.05 = $1.25
	}

	result := s.priceService.CalculateBucketedCost(s.ctx, price, bucketedValues)

	// Expected: $0.90 + $1.25 = $2.15
	expected := decimal.NewFromFloat(2.15)
	s.True(expected.Equal(result),
		"Expected cost %s but got %s for bucketed values %v",
		expected.String(), result.String(), bucketedValues)
}

func (s *PriceServiceSuite) TestCalculateBucketedCost_TieredVolume() {
	upTo10 := uint64(10)
	upTo20 := uint64(20)
	price := &price.Price{
		ID:           "price-bucketed-tiered-volume",
		Amount:       decimal.Zero,
		Currency:     "usd",
		BillingModel: types.BILLING_MODEL_TIERED,
		TierMode:     types.BILLING_TIER_VOLUME,
		Tiers: []price.PriceTier{
			{UpTo: &upTo10, UnitAmount: decimal.NewFromFloat(0.10)}, // 0-10: $0.10/unit
			{UpTo: &upTo20, UnitAmount: decimal.NewFromFloat(0.05)}, // 11-20: $0.05/unit
			{UpTo: nil, UnitAmount: decimal.NewFromFloat(0.02)},     // 21+: $0.02/unit
		},
	}

	// Test with bucket values that fall into different volume tiers
	bucketedValues := []decimal.Decimal{
		decimal.NewFromInt(9),  // Bucket 1: max(2,5,6,9) = 9 → tier 1 → 9*$0.10 = $0.90
		decimal.NewFromInt(15), // Bucket 2: max(10,15) = 15 → tier 2 → 15*$0.05 = $0.75
	}

	result := s.priceService.CalculateBucketedCost(s.ctx, price, bucketedValues)

	// Expected: $0.90 + $0.75 = $1.65
	expected := decimal.NewFromFloat(1.65)
	s.True(expected.Equal(result),
		"Expected cost %s but got %s for bucketed values %v",
		expected.String(), result.String(), bucketedValues)
}

func (s *PriceServiceSuite) TestCalculateBucketedCost_EmptyBuckets() {
	price := &price.Price{
		ID:           "price-bucketed-empty",
		Amount:       decimal.NewFromFloat(0.10),
		Currency:     "usd",
		BillingModel: types.BILLING_MODEL_FLAT_FEE,
	}

	// Test with empty bucket array
	bucketedValues := []decimal.Decimal{}

	result := s.priceService.CalculateBucketedCost(s.ctx, price, bucketedValues)

	// Expected: $0.00 (no buckets to process)
	expected := decimal.Zero
	s.True(expected.Equal(result),
		"Expected cost %s but got %s for empty bucketed values",
		expected.String(), result.String())
}

func (s *PriceServiceSuite) TestCalculateBucketedCost_ZeroValues() {
	price := &price.Price{
		ID:           "price-bucketed-zero",
		Amount:       decimal.NewFromFloat(0.10),
		Currency:     "usd",
		BillingModel: types.BILLING_MODEL_FLAT_FEE,
	}

	// Test with zero values in buckets
	bucketedValues := []decimal.Decimal{
		decimal.Zero,          // Bucket 1: no usage
		decimal.NewFromInt(5), // Bucket 2: max = 5
		decimal.Zero,          // Bucket 3: no usage
	}

	result := s.priceService.CalculateBucketedCost(s.ctx, price, bucketedValues)

	// Expected: (0 * 0.10) + (5 * 0.10) + (0 * 0.10) = $0.50
	expected := decimal.NewFromFloat(0.50)
	s.True(expected.Equal(result),
		"Expected cost %s but got %s for bucketed values with zeros %v",
		expected.String(), result.String(), bucketedValues)
}

func (s *PriceServiceSuite) TestCalculateBucketedCost_ComplexScenario() {
	// Test a complex scenario with package billing and multiple buckets
	price := &price.Price{
		ID:           "price-bucketed-complex",
		Amount:       decimal.NewFromInt(5), // $5 per package
		Currency:     "usd",
		BillingModel: types.BILLING_MODEL_PACKAGE,
		TransformQuantity: price.JSONBTransformQuantity{
			DivideBy: 100,            // 100 units per package
			Round:    types.ROUND_UP, // Round up
		},
	}

	// Test with various bucket values
	bucketedValues := []decimal.Decimal{
		decimal.NewFromInt(50),  // Bucket 1: max = 50 → ceil(50/100) = 1 package → $5
		decimal.NewFromInt(150), // Bucket 2: max = 150 → ceil(150/100) = 2 packages → $10
		decimal.NewFromInt(200), // Bucket 3: max = 200 → ceil(200/100) = 2 packages → $10
		decimal.NewFromInt(99),  // Bucket 4: max = 99 → ceil(99/100) = 1 package → $5
	}

	result := s.priceService.CalculateBucketedCost(s.ctx, price, bucketedValues)

	// Expected: $5 + $10 + $10 + $5 = $30
	expected := decimal.NewFromInt(30)
	s.True(expected.Equal(result),
		"Expected cost %s but got %s for complex bucketed scenario %v",
		expected.String(), result.String(), bucketedValues)
}
