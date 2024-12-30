package service

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type InvoiceServiceSuite struct {
	testutil.BaseServiceTestSuite
	service  InvoiceService
	testData struct {
		customer *customer.Customer
		plan     *plan.Plan
		meters   struct {
			apiCalls *meter.Meter
			storage  *meter.Meter
		}
		prices struct {
			apiCalls       *price.Price
			storage        *price.Price
			storageArchive *price.Price
		}
		subscription *subscription.Subscription
		now          time.Time
	}
}

func TestInvoiceService(t *testing.T) {
	suite.Run(t, new(InvoiceServiceSuite))
}

func (s *InvoiceServiceSuite) SetupTest() {
	s.BaseServiceTestSuite.SetupTest()
	s.setupService()
	s.setupTestData()
}

// TearDownTest is called after each test
func (s *InvoiceServiceSuite) TearDownTest() {
	s.BaseServiceTestSuite.TearDownTest()
}

func (s *InvoiceServiceSuite) setupService() {
	s.service = NewInvoiceService(
		s.GetStores().InvoiceRepo,
		s.GetPublisher(),
		s.GetLogger(),
		s.GetDB(),
	)
}

func (s *InvoiceServiceSuite) setupTestData() {
	// Create test customer
	s.testData.customer = &customer.Customer{
		ID:         "cust_123",
		ExternalID: "ext_cust_123",
		Name:       "Test Customer",
		Email:      "test@example.com",
		BaseModel:  types.GetDefaultBaseModel(s.GetContext()),
	}
	s.NoError(s.GetStores().CustomerRepo.Create(s.GetContext(), s.testData.customer))

	// Create test plan
	s.testData.plan = &plan.Plan{
		ID:             "plan_123",
		Name:           "Test Plan",
		Description:    "Test Plan Description",
		InvoiceCadence: types.InvoiceCadenceAdvance,
		BaseModel:      types.GetDefaultBaseModel(s.GetContext()),
	}
	s.NoError(s.GetStores().PlanRepo.Create(s.GetContext(), s.testData.plan))

	// Create test meters
	s.testData.meters.apiCalls = &meter.Meter{
		ID:        "meter_api_calls",
		Name:      "API Calls",
		EventName: "api_call",
		Aggregation: meter.Aggregation{
			Type: types.AggregationCount,
		},
		BaseModel: types.GetDefaultBaseModel(s.GetContext()),
	}
	s.NoError(s.GetStores().MeterRepo.CreateMeter(s.GetContext(), s.testData.meters.apiCalls))

	s.testData.meters.storage = &meter.Meter{
		ID:        "meter_storage",
		Name:      "Storage",
		EventName: "storage_usage",
		Aggregation: meter.Aggregation{
			Type:  types.AggregationSum,
			Field: "bytes_used",
		},
		BaseModel: types.GetDefaultBaseModel(s.GetContext()),
	}
	s.NoError(s.GetStores().MeterRepo.CreateMeter(s.GetContext(), s.testData.meters.storage))

	// Create test prices
	upTo1000 := uint64(1000)
	upTo5000 := uint64(5000)

	s.testData.prices.apiCalls = &price.Price{
		ID:                 "price_api_calls",
		Amount:             decimal.Zero,
		Currency:           "USD",
		PlanID:             s.testData.plan.ID,
		Type:               types.PRICE_TYPE_USAGE,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_TIERED,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		TierMode:           types.BILLING_TIER_SLAB,
		MeterID:            s.testData.meters.apiCalls.ID,
		Tiers: []price.PriceTier{
			{UpTo: &upTo1000, UnitAmount: decimal.NewFromFloat(0.02)},
			{UpTo: &upTo5000, UnitAmount: decimal.NewFromFloat(0.005)},
			{UpTo: nil, UnitAmount: decimal.NewFromFloat(0.01)},
		},
		BaseModel: types.GetDefaultBaseModel(s.GetContext()),
	}
	s.NoError(s.GetStores().PriceRepo.Create(s.GetContext(), s.testData.prices.apiCalls))

	s.testData.prices.storage = &price.Price{
		ID:                 "price_storage",
		Amount:             decimal.NewFromFloat(0.1),
		Currency:           "USD",
		PlanID:             s.testData.plan.ID,
		Type:               types.PRICE_TYPE_USAGE,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		MeterID:            s.testData.meters.storage.ID,
		FilterValues:       map[string][]string{"region": {"us-east-1"}, "tier": {"standard"}},
		BaseModel:          types.GetDefaultBaseModel(s.GetContext()),
	}
	s.NoError(s.GetStores().PriceRepo.Create(s.GetContext(), s.testData.prices.storage))

	s.testData.now = s.GetNow()
	s.testData.subscription = &subscription.Subscription{
		ID:                 "sub_123",
		PlanID:             s.testData.plan.ID,
		CustomerID:         s.testData.customer.ID,
		StartDate:          s.testData.now.Add(-30 * 24 * time.Hour),
		CurrentPeriodStart: s.testData.now.Add(-24 * time.Hour),
		CurrentPeriodEnd:   s.testData.now.Add(6 * 24 * time.Hour),
		Currency:           "USD",
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		SubscriptionStatus: types.SubscriptionStatusActive,
		BaseModel:          types.GetDefaultBaseModel(s.GetContext()),
	}
	s.NoError(s.GetStores().SubscriptionRepo.Create(s.GetContext(), s.testData.subscription))
}

func (s *InvoiceServiceSuite) TestCreateSubscriptionInvoice() {
	tests := []struct {
		name    string
		usage   *dto.GetUsageBySubscriptionResponse
		want    *dto.InvoiceResponse
		wantErr bool
	}{
		{
			name: "successful invoice creation with usage",
			usage: &dto.GetUsageBySubscriptionResponse{
				StartTime: s.testData.subscription.CurrentPeriodStart,
				EndTime:   s.testData.subscription.CurrentPeriodEnd,
				Amount:    15,
				Currency:  "USD",
				Charges: []*dto.SubscriptionUsageByMetersResponse{
					{
						MeterDisplayName: "API Calls",
						Quantity:         100,
						Amount:           10,
					},
					{
						MeterDisplayName: "Storage",
						Quantity:         50,
						Amount:           5,
					},
				},
			},
			want: &dto.InvoiceResponse{
				CustomerID:     s.testData.customer.ID,
				SubscriptionID: &s.testData.subscription.ID,
				InvoiceType:    types.InvoiceTypeSubscription,
				InvoiceStatus:  types.InvoiceStatusDraft,
				PaymentStatus:  types.InvoicePaymentStatusPending,
				Currency:       "USD",
				AmountDue:      decimal.NewFromFloat(15),
				BillingReason:  string(types.InvoiceBillingReasonSubscriptionCycle),
			},
			wantErr: false,
		},
		{
			name: "zero amount invoice",
			usage: &dto.GetUsageBySubscriptionResponse{
				StartTime: s.testData.subscription.CurrentPeriodStart,
				EndTime:   s.testData.subscription.CurrentPeriodEnd,
				Amount:    0,
				Currency:  "USD",
				Charges:   []*dto.SubscriptionUsageByMetersResponse{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := s.service.CreateSubscriptionInvoice(
				s.GetContext(),
				s.testData.subscription,
				tt.usage.StartTime,
				tt.usage.EndTime,
				tt.usage,
			)
			if tt.wantErr {
				s.Error(err)
				return
			}

			s.NoError(err)
			s.Equal(tt.want.CustomerID, got.CustomerID)
			s.Equal(tt.want.SubscriptionID, got.SubscriptionID)
			s.Equal(tt.want.InvoiceType, got.InvoiceType)
			s.Equal(tt.want.InvoiceStatus, got.InvoiceStatus)
			s.Equal(tt.want.PaymentStatus, got.PaymentStatus)
			s.Equal(tt.want.Currency, got.Currency)
			s.Equal(tt.want.AmountDue.Equal(got.AmountDue), true)
			s.Equal(tt.want.BillingReason, got.BillingReason)
		})
	}
}

func (s *InvoiceServiceSuite) TestFinalizeInvoice() {
	// Create a draft invoice
	draftInvoice := &invoice.Invoice{
		ID:             "inv_123",
		CustomerID:     s.testData.customer.ID,
		SubscriptionID: &s.testData.subscription.ID,
		InvoiceType:    types.InvoiceTypeSubscription,
		InvoiceStatus:  types.InvoiceStatusDraft,
		PaymentStatus:  types.InvoicePaymentStatusPending,
		Currency:       "USD",
		AmountDue:      decimal.NewFromFloat(15),
		BillingReason:  string(types.InvoiceBillingReasonSubscriptionCycle),
		BaseModel:      types.GetDefaultBaseModel(s.GetContext()),
	}
	s.NoError(s.GetStores().InvoiceRepo.Create(s.GetContext(), draftInvoice))

	tests := []struct {
		name    string
		id      string
		want    *dto.InvoiceResponse
		wantErr bool
	}{
		{
			name: "successful finalization",
			id:   draftInvoice.ID,
			want: &dto.InvoiceResponse{
				ID:             draftInvoice.ID,
				CustomerID:     draftInvoice.CustomerID,
				SubscriptionID: draftInvoice.SubscriptionID,
				InvoiceType:    draftInvoice.InvoiceType,
				InvoiceStatus:  types.InvoiceStatusFinalized,
				PaymentStatus:  draftInvoice.PaymentStatus,
				Currency:       draftInvoice.Currency,
				AmountDue:      draftInvoice.AmountDue,
				BillingReason:  draftInvoice.BillingReason,
			},
			wantErr: false,
		},
		{
			name:    "non-existent invoice",
			id:      "inv_nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := s.service.FinalizeInvoice(s.GetContext(), tt.id)
			if tt.wantErr {
				s.Error(err)
				return
			}

			s.NoError(err)
		})
	}
}

func (s *InvoiceServiceSuite) TestUpdatePaymentStatus() {
	// Create a finalized invoice
	finalizedInvoice := &invoice.Invoice{
		ID:             "inv_123",
		CustomerID:     s.testData.customer.ID,
		SubscriptionID: &s.testData.subscription.ID,
		InvoiceType:    types.InvoiceTypeSubscription,
		InvoiceStatus:  types.InvoiceStatusFinalized,
		PaymentStatus:  types.InvoicePaymentStatusPending,
		Currency:       "USD",
		AmountDue:      decimal.NewFromFloat(15),
		BillingReason:  string(types.InvoiceBillingReasonSubscriptionCycle),
		BaseModel:      types.GetDefaultBaseModel(s.GetContext()),
	}
	s.NoError(s.GetStores().InvoiceRepo.Create(s.GetContext(), finalizedInvoice))

	tests := []struct {
		name      string
		id        string
		newStatus types.InvoicePaymentStatus
		want      *dto.InvoiceResponse
		wantErr   bool
	}{
		{
			name:      "mark as paid",
			id:        finalizedInvoice.ID,
			newStatus: types.InvoicePaymentStatusSucceeded,
			want: &dto.InvoiceResponse{
				ID:             finalizedInvoice.ID,
				CustomerID:     finalizedInvoice.CustomerID,
				SubscriptionID: finalizedInvoice.SubscriptionID,
				InvoiceType:    finalizedInvoice.InvoiceType,
				InvoiceStatus:  finalizedInvoice.InvoiceStatus,
				PaymentStatus:  types.InvoicePaymentStatusSucceeded,
				Currency:       finalizedInvoice.Currency,
				AmountDue:      finalizedInvoice.AmountDue,
				BillingReason:  finalizedInvoice.BillingReason,
			},
			wantErr: false,
		},
		{
			name:      "non-existent invoice",
			id:        "inv_nonexistent",
			newStatus: types.InvoicePaymentStatusSucceeded,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := s.service.UpdatePaymentStatus(s.GetContext(), tt.id, tt.newStatus, lo.ToPtr(decimal.Zero))
			if tt.wantErr {
				s.Error(err)
				return
			}

			s.NoError(err)
		})
	}
}
