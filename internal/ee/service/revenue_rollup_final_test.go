package service

import (
	"context"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/revenuefact"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

// RevenueRollupFinalSuite drives FinalizeSubscriptionPeriod directly against
// the in-memory revenue_fact store and a hand-built finalized-invoice
// fixture — deliberately not going through the full billing preview engine
// (that is RevenueRollupSuite's job for RollupSubscription) so this suite can
// isolate the FINAL-flip/reconcile/non-blocking-guard behavior.
type RevenueRollupFinalSuite struct {
	testutil.BaseServiceTestSuite
	ctx   context.Context
	svc   RevenueRollupService
	store *testutil.InMemoryRevenueFactStore

	subscriptionID string
	priceID        string
	periodStart    time.Time
	periodEnd      time.Time // exclusive, matching invoice line item convention
	invoice        *invoice.Invoice
}

func TestRevenueRollupFinal(t *testing.T) {
	suite.Run(t, new(RevenueRollupFinalSuite))
}

func (s *RevenueRollupFinalSuite) SetupTest() {
	s.BaseServiceTestSuite.SetupTest()
	s.ctx = s.GetContext()
	s.store = s.GetStores().RevenueFactRepo.(*testutil.InMemoryRevenueFactStore)
	s.svc = NewRevenueRollupService(s.serviceParamsFinal())

	s.subscriptionID = "sub_final_1"
	s.priceID = "price_final_1"
	s.periodStart = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	s.periodEnd = s.periodStart.AddDate(0, 0, 30) // exclusive, per invoice line item convention
}

func (s *RevenueRollupFinalSuite) serviceParamsFinal() ServiceParams {
	stores := s.GetStores()
	return ServiceParams{
		Logger:                       s.GetLogger(),
		Config:                       s.GetConfig(),
		DB:                           s.GetDB(),
		SubRepo:                      stores.SubscriptionRepo,
		SubscriptionLineItemRepo:     stores.SubscriptionLineItemRepo,
		SubscriptionPhaseRepo:        stores.SubscriptionPhaseRepo,
		SubScheduleRepo:              stores.SubscriptionScheduleRepo,
		PlanRepo:                     stores.PlanRepo,
		PriceRepo:                    stores.PriceRepo,
		PriceUnitRepo:                stores.PriceUnitRepo,
		EventRepo:                    stores.EventRepo,
		MeterRepo:                    stores.MeterRepo,
		MeterUsageRepo:               stores.MeterUsageRepo,
		CustomerRepo:                 stores.CustomerRepo,
		InvoiceRepo:                  stores.InvoiceRepo,
		InvoiceLineItemRepo:          stores.InvoiceLineItemRepo,
		EntitlementRepo:              stores.EntitlementRepo,
		EntitlementGrantRepo:         stores.EntitlementGrantRepo,
		EnvironmentRepo:              stores.EnvironmentRepo,
		FeatureRepo:                  stores.FeatureRepo,
		AddonAssociationRepo:         stores.AddonAssociationRepo,
		TenantRepo:                   stores.TenantRepo,
		UserRepo:                     stores.UserRepo,
		AuthRepo:                     stores.AuthRepo,
		WalletRepo:                   stores.WalletRepo,
		PaymentRepo:                  stores.PaymentRepo,
		CreditNoteRepo:               stores.CreditNoteRepo,
		CreditNoteLineItemRepo:       stores.CreditNoteLineItemRepo,
		CouponRepo:                   stores.CouponRepo,
		CouponAssociationRepo:        stores.CouponAssociationRepo,
		CouponApplicationRepo:        stores.CouponApplicationRepo,
		TaxRateRepo:                  stores.TaxRateRepo,
		TaxAppliedRepo:               stores.TaxAppliedRepo,
		TaxAssociationRepo:           stores.TaxAssociationRepo,
		CreditGrantRepo:              stores.CreditGrantRepo,
		CreditGrantApplicationRepo:   stores.CreditGrantApplicationRepo,
		ConnectionRepo:               stores.ConnectionRepo,
		EntityIntegrationMappingRepo: stores.EntityIntegrationMappingRepo,
		SettingsRepo:                 stores.SettingsRepo,
		AlertLogsRepo:                stores.AlertLogsRepo,
		TaskRepo:                     stores.TaskRepo,
		SecretRepo:                   stores.SecretRepo,
		EventPublisher:               s.GetPublisher(),
		WebhookPublisher:             s.GetWebhookPublisher(),
		ProrationCalculator:          s.GetCalculator(),
		IntegrationFactory:           s.GetIntegrationFactory(),
		RevenueFactRepo:              stores.RevenueFactRepo,
	}
}

// seedProvisional writes one PROVISIONAL fixed-revenue fact row for
// s.subscriptionID/s.priceID/s.periodStart, dated on the period's own day
// grain, plus a matching finalized invoice + line item that references the
// same subscription/price/period. Mirrors the shape RollupSubscription would
// have written and FinalizeInvoice would have produced.
func (s *RevenueRollupFinalSuite) seedProvisional(ctx context.Context) {
	fact := &revenuefact.RevenueFact{
		ID:             "revfact_final_1",
		CustomerID:     "cust_final_1",
		SubscriptionID: s.subscriptionID,
		PriceID:        lo.ToPtr(s.priceID),
		RevenueSource:  types.RevenueSourceFixed,
		PeriodStart:    s.periodStart,
		PeriodEnd:      s.periodEnd.AddDate(0, 0, -1),
		Day:            s.periodStart,
		NetAmount:      decimal.NewFromInt(30),
		Currency:       "usd",
		Status:         types.FactProvisional,
	}
	s.NoError(s.store.UpsertProvisional(ctx, []*revenuefact.RevenueFact{fact}))

	inv := &invoice.Invoice{
		ID:              "inv_final_1",
		CustomerID:      "cust_final_1",
		SubscriptionID:  lo.ToPtr(s.subscriptionID),
		InvoiceType:     types.InvoiceTypeSubscription,
		InvoiceStatus:   types.InvoiceStatusFinalized,
		PaymentStatus:   types.PaymentStatusPending,
		Currency:        "usd",
		AmountDue:       decimal.NewFromInt(30),
		Subtotal:        decimal.NewFromInt(30),
		TotalDiscount:   decimal.Zero,
		AmountPaid:      decimal.Zero,
		AmountRemaining: decimal.NewFromInt(30),
		Description:     "Final flip test invoice",
		PeriodStart:     lo.ToPtr(s.periodStart),
		PeriodEnd:       lo.ToPtr(s.periodEnd),
		BillingReason:   string(types.InvoiceBillingReasonSubscriptionCycle),
		BaseModel:       types.GetDefaultBaseModel(ctx),
		LineItems: []*invoice.InvoiceLineItem{
			{
				ID:             "li_final_1",
				InvoiceID:      "inv_final_1",
				CustomerID:     "cust_final_1",
				SubscriptionID: lo.ToPtr(s.subscriptionID),
				PriceID:        lo.ToPtr(s.priceID),
				Amount:         decimal.NewFromInt(30),
				Quantity:       decimal.NewFromInt(1),
				Currency:       "usd",
				PeriodStart:    lo.ToPtr(s.periodStart),
				PeriodEnd:      lo.ToPtr(s.periodEnd),
				BaseModel:      types.GetDefaultBaseModel(ctx),
			},
		},
	}
	s.NoError(s.GetStores().InvoiceRepo.CreateWithLineItems(ctx, inv))
	s.invoice = inv
}

func (s *RevenueRollupFinalSuite) TestFinalizeSubscriptionPeriod_FlipsAndStamps() {
	s.seedProvisional(s.ctx)

	s.NoError(s.svc.FinalizeSubscriptionPeriod(s.ctx, s.invoice.ID))

	rows, err := s.store.ListBySubscriptionPeriod(s.ctx, s.subscriptionID, s.periodStart, s.periodEnd.AddDate(0, 0, -1), types.FactFinal)
	s.NoError(err)
	s.NotEmpty(rows)
	s.Equal(s.invoice.ID, lo.FromPtr(rows[0].InvoiceID))
	s.Equal("li_final_1", lo.FromPtr(rows[0].InvoiceLineItemID))
	s.Equal(types.FactFinal, rows[0].Status)

	// No more PROVISIONAL rows left for this grain — the flip moved them, it
	// did not duplicate them.
	provisional, err := s.store.ListBySubscriptionPeriod(s.ctx, s.subscriptionID, s.periodStart, s.periodEnd.AddDate(0, 0, -1), types.FactProvisional)
	s.NoError(err)
	s.Empty(provisional)

	// Sum reconciles against the invoice's Subtotal - TotalDiscount.
	sum := decimal.Zero
	for _, r := range rows {
		sum = sum.Add(r.NetAmount)
	}
	s.True(sum.Equal(s.invoice.Subtotal.Sub(s.invoice.TotalDiscount)))
}

// TestFinalizeSubscriptionPeriod_TrueupRowMatchesBySyntheticPriceID verifies
// the Task-10 seam: a commitment-trueup provisional row is written keyed on
// the stable synthetic price_id ("trueup:"+subLineItemID), while the
// finalized invoice line item carries a completely different (fresh random)
// price_id, exactly as the real billing engine assigns. The flip must still
// find and stamp the provisional row by re-deriving the same synthetic id
// from the finalized line item's metadata + sub_line_item_id — never by the
// line item's own (mismatched) price_id.
func (s *RevenueRollupFinalSuite) TestFinalizeSubscriptionPeriod_TrueupRowMatchesBySyntheticPriceID() {
	subLineItemID := "sli_final_trueup"
	syntheticPriceID := stableTrueupPriceID(subLineItemID, s.subscriptionID, false)

	fact := &revenuefact.RevenueFact{
		ID:             "revfact_final_trueup",
		CustomerID:     "cust_final_1",
		SubscriptionID: s.subscriptionID,
		SubLineItemID:  lo.ToPtr(subLineItemID),
		PriceID:        lo.ToPtr(syntheticPriceID),
		RevenueSource:  types.RevenueSourceCommitmentTrueup,
		PeriodStart:    s.periodStart,
		PeriodEnd:      s.periodEnd.AddDate(0, 0, -1),
		Day:            s.periodEnd.AddDate(0, 0, -1),
		NetAmount:      decimal.NewFromInt(50),
		Currency:       "usd",
		Status:         types.FactProvisional,
	}
	s.NoError(s.store.UpsertProvisional(s.ctx, []*revenuefact.RevenueFact{fact}))

	inv := &invoice.Invoice{
		ID:              "inv_final_trueup",
		CustomerID:      "cust_final_1",
		SubscriptionID:  lo.ToPtr(s.subscriptionID),
		InvoiceType:     types.InvoiceTypeSubscription,
		InvoiceStatus:   types.InvoiceStatusFinalized,
		PaymentStatus:   types.PaymentStatusPending,
		Currency:        "usd",
		AmountDue:       decimal.NewFromInt(50),
		Subtotal:        decimal.NewFromInt(50),
		TotalDiscount:   decimal.Zero,
		AmountRemaining: decimal.NewFromInt(50),
		BillingReason:   string(types.InvoiceBillingReasonSubscriptionCycle),
		BaseModel:       types.GetDefaultBaseModel(s.ctx),
		LineItems: []*invoice.InvoiceLineItem{
			{
				ID:                     "li_final_trueup",
				InvoiceID:              "inv_final_trueup",
				CustomerID:             "cust_final_1",
				SubscriptionID:         lo.ToPtr(s.subscriptionID),
				SubscriptionLineItemID: lo.ToPtr(subLineItemID),
				// The billing engine assigns a fresh random price_id to
				// trueup/overage lines on every compute — deliberately NOT the
				// synthetic id the provisional row was written under.
				PriceID:     lo.ToPtr(types.GenerateUUIDWithPrefix("price")),
				Amount:      decimal.NewFromInt(50),
				Quantity:    decimal.NewFromInt(1),
				Currency:    "usd",
				PeriodStart: lo.ToPtr(s.periodStart),
				PeriodEnd:   lo.ToPtr(s.periodEnd),
				Metadata:    types.Metadata{"is_commitment_trueup": "true"},
				BaseModel:   types.GetDefaultBaseModel(s.ctx),
			},
		},
	}
	s.NoError(s.GetStores().InvoiceRepo.CreateWithLineItems(s.ctx, inv))

	s.NoError(s.svc.FinalizeSubscriptionPeriod(s.ctx, inv.ID))

	rows, err := s.store.ListBySubscriptionPeriod(s.ctx, s.subscriptionID, s.periodStart, s.periodEnd.AddDate(0, 0, -1), types.FactFinal)
	s.NoError(err)
	s.NotEmpty(rows)
	s.Equal(inv.ID, lo.FromPtr(rows[0].InvoiceID))
	s.Equal("li_final_trueup", lo.FromPtr(rows[0].InvoiceLineItemID))
}

// failingRevenueFactRepo wraps the in-memory store so FlipToFinal always
// errors — used to prove FinalizeSubscriptionPeriod's caller (the async
// finalization hook) swallows the error rather than propagating it. Called,
// when non-nil, receives a signal on every FlipToFinal call — a test running
// this behind a goroutine (as performFinalizeInvoiceActions does) can select
// on it instead of guessing with a sleep.
type failingRevenueFactRepo struct {
	*testutil.InMemoryRevenueFactStore
	Called chan struct{}
}

func (f *failingRevenueFactRepo) FlipToFinal(ctx context.Context, subscriptionID, priceID string, periodStart, periodEnd time.Time, invoiceID, invoiceLineItemID string) (int, error) {
	if f.Called != nil {
		select {
		case f.Called <- struct{}{}:
		default:
		}
	}
	return 0, ierr.NewError("forced flip failure").Mark(ierr.ErrDatabase)
}

func (s *RevenueRollupFinalSuite) TestFinalizeSubscriptionPeriod_FlipErrorIsReturnedToCaller() {
	s.seedProvisional(s.ctx)

	params := s.serviceParamsFinal()
	params.RevenueFactRepo = &failingRevenueFactRepo{InMemoryRevenueFactStore: s.store}
	failingSvc := NewRevenueRollupService(params)

	err := failingSvc.FinalizeSubscriptionPeriod(s.ctx, s.invoice.ID)
	s.Error(err)
}
