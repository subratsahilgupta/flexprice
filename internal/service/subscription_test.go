package service

import (
	"context"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionService_GetUsageBySubscription(t *testing.T) {
	// Setup test dependencies
	subscriptionStore := testutil.NewInMemorySubscriptionStore()
	eventStore := testutil.NewInMemoryEventStore()
	planStore := testutil.NewInMemoryPlanStore()
	priceStore := testutil.NewInMemoryPriceStore()
	meterStore := testutil.NewInMemoryMeterStore()
	customerStore := testutil.NewInMemoryCustomerStore()
	logger := logger.GetLogger()

	// Create test data
	ctx := context.Background()
	ctx = context.WithValue(ctx, types.CtxTenantID, types.DefaultTenantID)
	ctx = context.WithValue(ctx, types.CtxUserID, types.DefaultUserID)
	ctx = context.WithValue(ctx, types.CtxRequestID, uuid.New().String())

	// Create test customer
	testCustomer := &customer.Customer{
		ID:         "cust_123",
		ExternalID: "ext_cust_123",
		Name:       "Test Customer",
		Email:      "test@example.com",
		BaseModel:  types.GetDefaultBaseModel(ctx),
	}
	require.NoError(t, customerStore.Create(ctx, testCustomer))

	// Create test plan
	testPlan := &plan.Plan{
		ID:          "plan_123",
		Name:        "Test Plan",
		Description: "Test Plan Description",
		BaseModel:   types.GetDefaultBaseModel(ctx),
	}
	require.NoError(t, planStore.Create(ctx, testPlan))

	// Create test meters
	apiCallsMeter := &meter.Meter{
		ID:        "meter_api_calls",
		Name:      "API Calls",
		EventName: "api_call",
		Aggregation: meter.Aggregation{
			Type:  types.AggregationCount,
			Field: "",
		},
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	require.NoError(t, meterStore.CreateMeter(ctx, apiCallsMeter))

	storageMeter := &meter.Meter{
		ID:        "meter_storage",
		Name:      "Storage",
		EventName: "storage_usage",
		Aggregation: meter.Aggregation{
			Type:  types.AggregationSum,
			Field: "bytes_used",
		},
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	require.NoError(t, meterStore.CreateMeter(ctx, storageMeter))

	// Create test prices
	// 1. API Calls with tiers
	upTo1000 := uint64(1000)
	upTo5000 := uint64(5000)
	apiCallsPrice := &price.Price{
		ID:                 "price_api_calls",
		PlanID:             testPlan.ID,
		MeterID:            apiCallsMeter.ID,
		Type:               types.PRICE_TYPE_USAGE,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_TIERED,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		TierMode:           types.BILLING_TIER_SLAB,
		Currency:           "USD",
		Tiers: []price.PriceTier{
			{
				UpTo:       &upTo1000,
				UnitAmount: decimal.NewFromFloat(0.02),
			},
			{
				UpTo:       &upTo5000,
				UnitAmount: decimal.NewFromFloat(0.005),
			},
			{
				UpTo:       nil, // Infinity
				UnitAmount: decimal.NewFromFloat(0.01),
			},
		},
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	require.NoError(t, priceStore.Create(ctx, apiCallsPrice))

	// 2. Storage with filters
	storagePrice := &price.Price{
		ID:                 "price_storage",
		PlanID:             testPlan.ID,
		MeterID:            storageMeter.ID,
		Type:               types.PRICE_TYPE_USAGE,
		Amount:             decimal.NewFromFloat(0.1),
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		Currency:           "USD",
		FilterValues: map[string][]string{
			"region": {"us-east-1"},
			"tier":   {"standard"},
		},
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	require.NoError(t, priceStore.Create(ctx, storagePrice))

	storagePriceArchive := &price.Price{
		ID:                 "price_storage_archive",
		PlanID:             testPlan.ID,
		MeterID:            storageMeter.ID,
		Type:               types.PRICE_TYPE_USAGE,
		Amount:             decimal.NewFromFloat(0.03),
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
		BillingModel:       types.BILLING_MODEL_FLAT_FEE,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		Currency:           "USD",
		FilterValues: map[string][]string{
			"region": {"us-east-1"},
			"tier":   {"archive"},
		},
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	require.NoError(t, priceStore.Create(ctx, storagePriceArchive))

	// Create test subscription
	now := time.Now().UTC()
	testSub := &subscription.Subscription{
		ID:                 "sub_123",
		PlanID:             testPlan.ID,
		CustomerID:         testCustomer.ID,
		StartDate:          now.Add(-30 * 24 * time.Hour),
		CurrentPeriodStart: now.Add(-24 * time.Hour),
		CurrentPeriodEnd:   now.Add(6 * 24 * time.Hour),
		Currency:           "USD",
		BaseModel:          types.GetDefaultBaseModel(ctx),
	}
	require.NoError(t, subscriptionStore.Create(ctx, testSub))

	// Create test events
	// 1. API call events (1500 calls)
	for i := 0; i < 1500; i++ {
		event := &events.Event{
			ID:                 uuid.New().String(),
			TenantID:           testSub.TenantID,
			EventName:          apiCallsMeter.EventName,
			ExternalCustomerID: testCustomer.ExternalID,
			Timestamp:          now.Add(-1 * time.Hour),
			Properties:         map[string]interface{}{},
		}
		require.NoError(t, eventStore.InsertEvent(ctx, event))
	}

	// 2. Storage events with different properties
	storageEvents := []struct {
		bytes float64
		tier  string
	}{
		{bytes: 100, tier: "standard"},
		{bytes: 200, tier: "standard"},
		{bytes: 300, tier: "archive"},
	}

	for _, se := range storageEvents {
		event := &events.Event{
			ID:                 uuid.New().String(),
			TenantID:           testSub.TenantID,
			EventName:          storageMeter.EventName,
			ExternalCustomerID: testCustomer.ExternalID,
			Timestamp:          now.Add(-30 * time.Minute),
			Properties: map[string]interface{}{
				"bytes_used": se.bytes,
				"region":     "us-east-1",
				"tier":       se.tier,
			},
		}
		require.NoError(t, eventStore.InsertEvent(ctx, event))
	}

	// Create test producer
	producer := testutil.NewInMemoryMessageBroker()

	// Create service instance
	svc := NewSubscriptionService(
		subscriptionStore,
		planStore,
		priceStore,
		producer,
		eventStore,
		meterStore,
		customerStore,
		logger,
	)

	// Test cases
	tests := []struct {
		name    string
		req     *dto.GetUsageBySubscriptionRequest
		want    *dto.GetUsageBySubscriptionResponse
		wantErr bool
	}{
		{
			name: "successful usage calculation with multiple meters and filters",
			req: &dto.GetUsageBySubscriptionRequest{
				SubscriptionID: testSub.ID,
				StartTime:      now.Add(-48 * time.Hour),
				EndTime:        now,
			},
			want: &dto.GetUsageBySubscriptionResponse{
				StartTime: now.Add(-48 * time.Hour),
				EndTime:   now,
				Amount:    61.5, // Total cost calculation below
				Currency:  "USD",
				Charges: []*dto.SubscriptionUsageByMetersResponse{
					{
						MeterDisplayName: "Storage",
						Quantity:         decimal.NewFromInt(300).InexactFloat64(),
						Amount:           9, // 300 * 0.03 for archived tier
					},
					{
						MeterDisplayName: "Storage",
						Quantity:         decimal.NewFromInt(300).InexactFloat64(),
						Amount:           30, // 300 * 0.1 for standard tier
					},
					{
						MeterDisplayName: "API Calls",
						Quantity:         decimal.NewFromInt(1500).InexactFloat64(),
						Amount:           22.5, // First 1000 at 0.02 = 20, next 500 at 0.005 = 2.5
					},
				},
			},
			wantErr: false,
		},
		{
			name: "zero usage period",
			req: &dto.GetUsageBySubscriptionRequest{
				SubscriptionID: testSub.ID,
				StartTime:      now.Add(-100 * 24 * time.Hour), // Way before events
				EndTime:        now.Add(-50 * 24 * time.Hour),
			},
			want: &dto.GetUsageBySubscriptionResponse{
				StartTime: now.Add(-100 * 24 * time.Hour),
				EndTime:   now.Add(-50 * 24 * time.Hour),
				Amount:    0,
				Currency:  "USD",
				Charges:   []*dto.SubscriptionUsageByMetersResponse{},
			},
			wantErr: false,
		},
		{
			name: "default to current period when no times specified",
			req: &dto.GetUsageBySubscriptionRequest{
				SubscriptionID: testSub.ID,
			},
			want: &dto.GetUsageBySubscriptionResponse{
				StartTime: testSub.CurrentPeriodStart,
				EndTime:   testSub.CurrentPeriodEnd,
				Amount:    61.5, // Same as first test since events fall in current period
				Currency:  "USD",
			},
			wantErr: false,
		},
		{
			name: "invalid subscription ID",
			req: &dto.GetUsageBySubscriptionRequest{
				SubscriptionID: "invalid_id",
			},
			wantErr: true,
		},
		{
			name: "subscription not active",
			req: &dto.GetUsageBySubscriptionRequest{
				SubscriptionID: "sub_inactive",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.GetUsageBySubscription(ctx, tt.req)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want.StartTime.Unix(), got.StartTime.Unix())
			assert.Equal(t, tt.want.EndTime.Unix(), got.EndTime.Unix())
			assert.Equal(t, tt.want.Amount, got.Amount)
			assert.Equal(t, tt.want.Currency, got.Currency)

			if tt.want.Charges != nil {
				assert.Len(t, got.Charges, len(tt.want.Charges))
				for i, wantCharge := range tt.want.Charges {
					if wantCharge == nil {
						continue
					}

					if i >= len(got.Charges) {
						t.Errorf("got less charges than expected")
						return
					}

					gotCharge := got.Charges[i]
					assert.Equal(t, wantCharge.MeterDisplayName, gotCharge.MeterDisplayName)
					assert.Equal(t, wantCharge.Quantity, gotCharge.Quantity)
					assert.Equal(t, wantCharge.Amount, gotCharge.Amount)
				}
			}
		})
	}
}
