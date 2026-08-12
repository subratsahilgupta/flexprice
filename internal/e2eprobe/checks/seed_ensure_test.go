package checks

import (
	"context"
	"testing"

	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/e2eprobe"
	"github.com/flexprice/flexprice/internal/logger"
	itypes "github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/go-sdk/v2/models/dtos"
	"github.com/flexprice/go-sdk/v2/models/types"
)

func TestSeedEnsure(t *testing.T) {
	tests := []struct {
		name                    string
		setup                   func(fc *fakeClient)
		wantErr                 bool
		wantFeaturesCreated     int
		wantCustomersCreated    int
		wantPlansCreated        int
		wantPricesCreated       int
		wantSubsCreated         int
		wantWalletsCreated      int
		wantPersistentCustomers int
		wantPreFundedCustomers  int
		wantMeterIDs            int
		wantFeatureIDs          int
		wantPlanIDs             int
		wantSubIDs              int
	}{
		{
			name: "AllPresent: features pre-exist, customers present, plan pre-exists",
			setup: func(fc *fakeClient) {
				// Pre-populate 11 features with lookup keys and meter IDs.
				for _, spec := range seedFeatureSpecs {
					lk := spec.lookupKey
					mID := "meter_" + spec.eventName
					fc.features.features = append(fc.features.features, types.FeatureResponse{
						ID:        strPtr("feat_" + spec.lookupKey),
						LookupKey: &lk,
						MeterID:   &mID,
					})
				}
				// Pre-populate 10 customers.
				for i := 0; i < 10; i++ {
					ext := persistentExternalCustomerID(i)
					fc.customers.byExt[ext] = "cust_" + ext
				}
				// Pre-populate 1 plan.
				lk := e2eprobePlanLookupKey
				planID := "plan_existing"
				fc.plans.plans = append(fc.plans.plans, types.PlanResponse{
					ID:        &planID,
					LookupKey: &lk,
				})
				// subs and wallets are empty — they'll be created
			},
			wantErr:                 false,
			wantFeaturesCreated:     0,  // all 11 found via Query
			wantCustomersCreated:    1,  // 10 pre-populated; alert canary still needs creating
			wantPlansCreated:        0,  // plan found via Query
			wantPricesCreated:       12, // base + 11 usage prices
			wantSubsCreated:         11, // 10 persistent + 1 alert canary
			wantWalletsCreated:      4,  // 3 pre-funded + 1 alert canary
			wantPersistentCustomers: 11, // 10 persistent + 1 alert canary
			wantPreFundedCustomers:  3,
			wantMeterIDs:            11,
			wantFeatureIDs:          11,
			wantPlanIDs:             1,
			wantSubIDs:              11,
		},
		{
			name: "CreatesMissing: all empty, all entities created",
			setup: func(fc *fakeClient) {
				// Customers not found on initial lookup.
				fc.customers.getErr = errNotFound
			},
			wantErr:                 false,
			wantFeaturesCreated:     11,
			wantCustomersCreated:    11, // 10 persistent + 1 alert canary
			wantPlansCreated:        1,
			wantPricesCreated:       12, // base + 11 usage
			wantSubsCreated:         11, // 10 persistent + 1 alert canary
			wantWalletsCreated:      4,  // 3 pre-funded + 1 alert canary
			wantPersistentCustomers: 11, // 10 persistent + 1 alert canary
			wantPreFundedCustomers:  3,
			wantMeterIDs:            11,
			wantFeatureIDs:          11,
			wantPlanIDs:             1,
			wantSubIDs:              11,
		},
		{
			name: "PartialExisting: features exist but plan/subs/wallets don't",
			setup: func(fc *fakeClient) {
				// Pre-populate 11 features.
				for _, spec := range seedFeatureSpecs {
					lk := spec.lookupKey
					mID := "meter_" + spec.eventName
					fc.features.features = append(fc.features.features, types.FeatureResponse{
						ID:        strPtr("feat_" + spec.lookupKey),
						LookupKey: &lk,
						MeterID:   &mID,
					})
				}
				// Customers all exist.
				for i := 0; i < 10; i++ {
					ext := persistentExternalCustomerID(i)
					fc.customers.byExt[ext] = "cust_" + ext
				}
				// Plan and everything below are missing.
			},
			wantErr:                 false,
			wantFeaturesCreated:     0,
			wantCustomersCreated:    1,  // alert canary still needs creating
			wantPlansCreated:        1,
			wantPricesCreated:       12,
			wantSubsCreated:         11, // 10 persistent + 1 alert canary
			wantWalletsCreated:      4,  // 3 pre-funded + 1 alert canary
			wantPersistentCustomers: 11, // 10 persistent + 1 alert canary
			wantPreFundedCustomers:  3,
			wantMeterIDs:            11,
			wantFeatureIDs:          11,
			wantPlanIDs:             1,
			wantSubIDs:              11,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fc := newFakeClient()
			reg := e2eprobe.NewRegistry()
			tc.setup(fc)
			s := NewSeedEnsure(fc, reg, "run-1", nil)
			err := s.Run(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("Run() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}

			if len(fc.features.created) != tc.wantFeaturesCreated {
				t.Errorf("features.created = %d, want %d", len(fc.features.created), tc.wantFeaturesCreated)
			}
			if len(fc.customers.created) != tc.wantCustomersCreated {
				t.Errorf("customers.created = %d, want %d", len(fc.customers.created), tc.wantCustomersCreated)
			}
			if len(fc.plans.created) != tc.wantPlansCreated {
				t.Errorf("plans.created = %d, want %d", len(fc.plans.created), tc.wantPlansCreated)
			}
			if len(fc.prices.created) != tc.wantPricesCreated {
				t.Errorf("prices.created = %d, want %d", len(fc.prices.created), tc.wantPricesCreated)
			}
			if len(fc.subs.created) != tc.wantSubsCreated {
				t.Errorf("subs.created = %d, want %d", len(fc.subs.created), tc.wantSubsCreated)
			}
			if len(fc.wallets.created) != tc.wantWalletsCreated {
				t.Errorf("wallets.created = %d, want %d", len(fc.wallets.created), tc.wantWalletsCreated)
			}

			got := reg.Seeds()
			if len(got.PersistentCustomerIDs) != tc.wantPersistentCustomers {
				t.Errorf("PersistentCustomerIDs = %d, want %d", len(got.PersistentCustomerIDs), tc.wantPersistentCustomers)
			}
			if len(got.PreFundedCustomerIDs) != tc.wantPreFundedCustomers {
				t.Errorf("PreFundedCustomerIDs = %d, want %d", len(got.PreFundedCustomerIDs), tc.wantPreFundedCustomers)
			}
			if len(got.MeterIDs) != tc.wantMeterIDs {
				t.Errorf("MeterIDs = %d, want %d", len(got.MeterIDs), tc.wantMeterIDs)
			}
			if len(got.FeatureIDs) != tc.wantFeatureIDs {
				t.Errorf("FeatureIDs = %d, want %d", len(got.FeatureIDs), tc.wantFeatureIDs)
			}
			if len(got.PlanIDs) != tc.wantPlanIDs {
				t.Errorf("PlanIDs = %d, want %d", len(got.PlanIDs), tc.wantPlanIDs)
			}
			if len(got.PersistentSubIDs) != tc.wantSubIDs {
				t.Errorf("PersistentSubIDs = %d, want %d", len(got.PersistentSubIDs), tc.wantSubIDs)
			}
		})
	}
}

func TestSeedEnsure_BucketedFeaturesProvisioned(t *testing.T) {
	fc := newFakeClient()
	reg := e2eprobe.NewRegistry()
	lg, _ := logger.NewLogger(&config.Configuration{Logging: config.LoggingConfig{Level: itypes.LogLevelInfo}})
	s := NewSeedEnsure(fc, reg, "test-run", lg)

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	seeds := reg.Seeds()
	wantKeys := []string{"e2eprobe_max_15min_feature", "e2eprobe_sum_hour_feature", "e2eprobe_max_day_feature"}
	for _, k := range wantKeys {
		if _, ok := seeds.BucketedFeatureIDs[k]; !ok {
			t.Errorf("BucketedFeatureIDs missing key %q; got keys %v", k, keysOf(seeds.BucketedFeatureIDs))
		}
	}
}

func keysOf(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

func TestSeedEnsure_CouponProvisioning(t *testing.T) {
	fc := newFakeClient()
	reg := e2eprobe.NewRegistry()
	lg, _ := logger.NewLogger(&config.Configuration{Logging: config.LoggingConfig{Level: itypes.LogLevelInfo}})
	s := NewSeedEnsure(fc, reg, "test-run", lg)

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("first Run() unexpected error: %v", err)
	}
	seeds := reg.Seeds()
	if seeds.SharedCouponID == "" {
		t.Fatalf("SharedCouponID empty after seed run")
	}
	if seeds.SharedCouponCode != "E2EPROBE_COUPON_10PCT" {
		t.Errorf("SharedCouponCode = %q, want E2EPROBE_COUPON_10PCT", seeds.SharedCouponCode)
	}
	if len(fc.coupons.created) != 1 {
		t.Fatalf("coupon created %d times on first run; want 1", len(fc.coupons.created))
	}
	if got := fc.coupons.created[0]; got.Type != types.CouponTypePercentage || got.Cadence != types.CouponCadenceOnce {
		t.Errorf("coupon Type/Cadence = %v/%v; want percentage/once", got.Type, got.Cadence)
	}
	if got := fc.coupons.created[0]; got.PercentageOff == nil || *got.PercentageOff != "10" {
		t.Errorf("coupon PercentageOff = %v, want \"10\"", got.PercentageOff)
	}

	// Second run must not create another (idempotent).
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("second Run() unexpected error: %v", err)
	}
	if len(fc.coupons.created) != 1 {
		t.Errorf("coupon created %d times across 2 runs; want 1 (idempotency broken)", len(fc.coupons.created))
	}
}

func TestSeedEnsure_TaxRateProvisioning(t *testing.T) {
	fc := newFakeClient()
	reg := e2eprobe.NewRegistry()
	lg, _ := logger.NewLogger(&config.Configuration{Logging: config.LoggingConfig{Level: itypes.LogLevelInfo}})
	s := NewSeedEnsure(fc, reg, "test-run", lg)

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("first Run() unexpected error: %v", err)
	}
	seeds := reg.Seeds()
	if seeds.SharedTaxRateID == "" {
		t.Fatalf("SharedTaxRateID empty after seed run")
	}
	if seeds.SharedTaxRateCode != "E2EPROBE_TAX_10PCT" {
		t.Errorf("SharedTaxRateCode = %q, want E2EPROBE_TAX_10PCT", seeds.SharedTaxRateCode)
	}
	if len(fc.taxRates.created) != 1 {
		t.Fatalf("tax rate created %d times on first run; want 1", len(fc.taxRates.created))
	}
	got := fc.taxRates.created[0]
	if got.Code != "E2EPROBE_TAX_10PCT" {
		t.Errorf("tax rate Code = %q, want E2EPROBE_TAX_10PCT", got.Code)
	}
	if got.PercentageValue == nil || *got.PercentageValue != "10" {
		t.Errorf("tax rate PercentageValue = %v, want \"10\"", got.PercentageValue)
	}
	if got.Scope == nil || *got.Scope != types.TaxRateScopeExternal {
		t.Errorf("tax rate Scope = %v, want EXTERNAL", got.Scope)
	}

	// Simulate the second run finding the existing rate via List.
	codeCopy := "E2EPROBE_TAX_10PCT"
	idCopy := seeds.SharedTaxRateID
	fc.taxRates.listResp = &dtos.GetTaxRatesResponse{
		TaxRateResponses: []types.TaxRateResponse{{ID: &idCopy, Code: &codeCopy}},
	}
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("second Run() unexpected error: %v", err)
	}
	if len(fc.taxRates.created) != 1 {
		t.Errorf("tax rate created %d times across 2 runs; want 1 (idempotency broken)", len(fc.taxRates.created))
	}
}

func TestSeedEnsure_PlanEntitlementsProvisioned(t *testing.T) {
	fc := newFakeClient()
	reg := e2eprobe.NewRegistry()
	lg, _ := logger.NewLogger(&config.Configuration{Logging: config.LoggingConfig{Level: itypes.LogLevelInfo}})
	s := NewSeedEnsure(fc, reg, "test-run", lg)

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	seeds := reg.Seeds()
	// 8 non-bucketed metered features get plan-level entitlements.
	if len(seeds.PlanEntitlementIDs) != 8 {
		t.Errorf("PlanEntitlementIDs = %d, want 8 (one per non-bucketed feature); got %v", len(seeds.PlanEntitlementIDs), seeds.PlanEntitlementIDs)
	}
	// Every created entitlement carries usage_limit=100, is_soft_limit=true,
	// reset_period=MONTHLY, is_enabled=true.
	if len(fc.entitlements.created) != 8 {
		t.Errorf("entitlements Create called %d times, want 8", len(fc.entitlements.created))
	}
	for i, req := range fc.entitlements.created {
		if req.UsageLimit == nil || *req.UsageLimit != 100 {
			t.Errorf("entitlement[%d] UsageLimit = %v, want 100", i, req.UsageLimit)
		}
		if req.IsSoftLimit == nil || !*req.IsSoftLimit {
			t.Errorf("entitlement[%d] IsSoftLimit = %v, want true", i, req.IsSoftLimit)
		}
		if req.IsEnabled == nil || !*req.IsEnabled {
			t.Errorf("entitlement[%d] IsEnabled = %v, want true", i, req.IsEnabled)
		}
		if req.UsageResetPeriod == nil || *req.UsageResetPeriod != types.EntitlementUsageResetPeriodMonthly {
			t.Errorf("entitlement[%d] UsageResetPeriod = %v, want Monthly", i, req.UsageResetPeriod)
		}
		if req.FeatureType != types.FeatureTypeMetered {
			t.Errorf("entitlement[%d] FeatureType = %v, want Metered", i, req.FeatureType)
		}
	}
}

func TestSeedEnsure_CommitmentAndCouponOnPersistentSubs(t *testing.T) {
	fc := newFakeClient()
	reg := e2eprobe.NewRegistry()
	lg, _ := logger.NewLogger(&config.Configuration{Logging: config.LoggingConfig{Level: itypes.LogLevelInfo}})
	s := NewSeedEnsure(fc, reg, "test-run", lg)

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if len(fc.subs.created) == 0 {
		t.Fatalf("no subs created")
	}
	// Every persistent sub must carry commitment fields.
	for i, req := range fc.subs.created {
		if req.CommitmentAmount == nil || *req.CommitmentAmount != "5.00" {
			t.Errorf("sub[%d] CommitmentAmount = %v, want \"5.00\"", i, req.CommitmentAmount)
		}
		if req.CommitmentDuration == nil || *req.CommitmentDuration != types.BillingPeriodMonthly {
			t.Errorf("sub[%d] CommitmentDuration = %v, want MONTHLY", i, req.CommitmentDuration)
		}
		if req.OverageFactor == nil || *req.OverageFactor != "1.5" {
			t.Errorf("sub[%d] OverageFactor = %v, want \"1.5\"", i, req.OverageFactor)
		}
	}

	// Persistent cust #1's sub must carry the shared coupon at sub-create.
	found := false
	for _, req := range fc.subs.created {
		if req.ExternalCustomerID != nil && *req.ExternalCustomerID == "e2eprobe-cust-persistent-1" {
			if len(req.SubscriptionCoupons) != 1 || req.SubscriptionCoupons[0].CouponCode != "E2EPROBE_COUPON_10PCT" {
				t.Errorf("cust-persistent-1 sub SubscriptionCoupons = %+v, want [{CouponCode: E2EPROBE_COUPON_10PCT}]", req.SubscriptionCoupons)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("no sub found for e2eprobe-cust-persistent-1")
	}

	// Other persistent subs must NOT carry the coupon (only cust #1).
	for _, req := range fc.subs.created {
		if req.ExternalCustomerID == nil || *req.ExternalCustomerID == "e2eprobe-cust-persistent-1" {
			continue
		}
		if len(req.SubscriptionCoupons) != 0 {
			t.Errorf("sub for %s carries SubscriptionCoupons = %+v, want none", *req.ExternalCustomerID, req.SubscriptionCoupons)
		}
	}
}
