package service

import (
	"context"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/coupon"
	"github.com/flexprice/flexprice/internal/domain/coupon_association"
	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type ApplyDiscountToInvoiceSuite struct {
	testutil.BaseServiceTestSuite
	service  InvoiceService
	testData struct {
		customer *customer.Customer
	}
}

func TestApplyDiscountToInvoice(t *testing.T) {
	suite.Run(t, new(ApplyDiscountToInvoiceSuite))
}

func (s *ApplyDiscountToInvoiceSuite) SetupTest() {
	s.BaseServiceTestSuite.SetupTest()
	s.setupService()
	s.setupTestData()
}

func (s *ApplyDiscountToInvoiceSuite) GetContext() context.Context {
	return types.SetEnvironmentID(s.BaseServiceTestSuite.GetContext(), "env_test")
}

func (s *ApplyDiscountToInvoiceSuite) setupService() {
	s.service = NewInvoiceService(ServiceParams{
		Logger:                     s.GetLogger(),
		Config:                     s.GetConfig(),
		DB:                         s.GetDB(),
		SubRepo:                    s.GetStores().SubscriptionRepo,
		SubscriptionLineItemRepo:   s.GetStores().SubscriptionLineItemRepo,
		PlanRepo:                   s.GetStores().PlanRepo,
		PriceRepo:                  s.GetStores().PriceRepo,
		EventRepo:                  s.GetStores().EventRepo,
		MeterRepo:                  s.GetStores().MeterRepo,
		CustomerRepo:               s.GetStores().CustomerRepo,
		InvoiceRepo:                s.GetStores().InvoiceRepo,
		InvoiceLineItemRepo:        s.GetStores().InvoiceLineItemRepo,
		EntitlementRepo:            s.GetStores().EntitlementRepo,
		EnvironmentRepo:            s.GetStores().EnvironmentRepo,
		FeatureRepo:                s.GetStores().FeatureRepo,
		AddonAssociationRepo:       s.GetStores().AddonAssociationRepo,
		TenantRepo:                 s.GetStores().TenantRepo,
		UserRepo:                   s.GetStores().UserRepo,
		AuthRepo:                   s.GetStores().AuthRepo,
		WalletRepo:                 s.GetStores().WalletRepo,
		PaymentRepo:                s.GetStores().PaymentRepo,
		CreditNoteRepo:             s.GetStores().CreditNoteRepo,
		CouponRepo:                 s.GetStores().CouponRepo,
		CouponAssociationRepo:      s.GetStores().CouponAssociationRepo,
		CouponApplicationRepo:      s.GetStores().CouponApplicationRepo,
		EventPublisher:             s.GetPublisher(),
		WebhookPublisher:           s.GetWebhookPublisher(),
		CreditGrantRepo:            s.GetStores().CreditGrantRepo,
		CreditGrantApplicationRepo: s.GetStores().CreditGrantApplicationRepo,
		CreditNoteLineItemRepo:     s.GetStores().CreditNoteLineItemRepo,
		TaxRateRepo:                s.GetStores().TaxRateRepo,
		TaxAppliedRepo:             s.GetStores().TaxAppliedRepo,
		TaxAssociationRepo:         s.GetStores().TaxAssociationRepo,
		SettingsRepo:               s.GetStores().SettingsRepo,
		AlertLogsRepo:              s.GetStores().AlertLogsRepo,
		WalletBalanceAlertPubSub:   types.WalletBalanceAlertPubSub{PubSub: testutil.NewInMemoryPubSub()},
	})
}

func (s *ApplyDiscountToInvoiceSuite) setupTestData() {
	s.BaseServiceTestSuite.ClearStores()
	s.testData.customer = &customer.Customer{
		ID:         "cust_discount_test",
		ExternalID: "ext_cust_discount_test",
		Name:       "Discount Test Customer",
		BaseModel:  types.GetDefaultBaseModel(s.GetContext()),
	}
	s.NoError(s.GetStores().CustomerRepo.Create(s.GetContext(), s.testData.customer))
}

// createOneOffDraftInvoice creates a subscription-less draft invoice with one line item,
// persisted via CreateWithLineItems so InvoiceLineItemRepo reads see it too.
func (s *ApplyDiscountToInvoiceSuite) createOneOffDraftInvoice(id string, amount decimal.Decimal) *invoice.Invoice {
	li := &invoice.InvoiceLineItem{
		ID:               s.GetUUID(),
		InvoiceID:        id,
		CustomerID:       s.testData.customer.ID,
		Amount:           amount,
		Quantity:         decimal.NewFromInt(1),
		Currency:         "usd",
		LineItemDiscount: decimal.Zero,
		BaseModel:        types.GetDefaultBaseModel(s.GetContext()),
	}
	inv := &invoice.Invoice{
		ID:            id,
		CustomerID:    s.testData.customer.ID,
		Currency:      "usd",
		Subtotal:      amount,
		Total:         amount,
		AmountDue:     amount,
		InvoiceType:   types.InvoiceTypeOneOff,
		InvoiceStatus: types.InvoiceStatusDraft,
		BaseModel:     types.GetDefaultBaseModel(s.GetContext()),
		LineItems:     []*invoice.InvoiceLineItem{li},
	}
	s.NoError(s.GetStores().InvoiceRepo.CreateWithLineItems(s.GetContext(), inv))
	return inv
}

// createSubscriptionDraftInvoice creates a subscription + a draft invoice for it (one line
// item, PriceID set so line-item-level coupon matching can be exercised), scoped to
// [periodStart, periodEnd).
func (s *ApplyDiscountToInvoiceSuite) createSubscriptionDraftInvoice(
	invID string, sub *subscription.Subscription, periodStart, periodEnd time.Time, amount decimal.Decimal, priceID string,
) *invoice.Invoice {
	li := &invoice.InvoiceLineItem{
		ID:               s.GetUUID(),
		InvoiceID:        invID,
		CustomerID:       s.testData.customer.ID,
		SubscriptionID:   &sub.ID,
		Amount:           amount,
		Quantity:         decimal.NewFromInt(1),
		Currency:         "usd",
		PriceID:          lo.ToPtr(priceID),
		LineItemDiscount: decimal.Zero,
		BaseModel:        types.GetDefaultBaseModel(s.GetContext()),
	}
	inv := &invoice.Invoice{
		ID:             invID,
		CustomerID:     s.testData.customer.ID,
		SubscriptionID: &sub.ID,
		Currency:       "usd",
		Subtotal:       amount,
		Total:          amount,
		AmountDue:      amount,
		PeriodStart:    &periodStart,
		PeriodEnd:      &periodEnd,
		InvoiceType:    types.InvoiceTypeSubscription,
		InvoiceStatus:  types.InvoiceStatusDraft,
		BaseModel:      types.GetDefaultBaseModel(s.GetContext()),
		LineItems:      []*invoice.InvoiceLineItem{li},
	}
	s.NoError(s.GetStores().InvoiceRepo.CreateWithLineItems(s.GetContext(), inv))
	return inv
}

// createPercentageCoupon creates a simple forever-cadence percentage-off coupon.
func (s *ApplyDiscountToInvoiceSuite) createPercentageCoupon(id string, percentOff int64) *coupon.Coupon {
	c := &coupon.Coupon{
		ID:            id,
		Name:          "Test Coupon " + id,
		Type:          types.CouponTypePercentage,
		PercentageOff: lo.ToPtr(decimal.NewFromInt(percentOff)),
		Cadence:       types.CouponCadenceForever,
		Currency:      "usd",
		EnvironmentID: types.GetEnvironmentID(s.GetContext()),
		BaseModel:     types.GetDefaultBaseModel(s.GetContext()),
	}
	s.NoError(s.GetStores().CouponRepo.Create(s.GetContext(), c))
	return c
}

func (s *ApplyDiscountToInvoiceSuite) TestNoSubscriptionIsNoOp() {
	ctx := s.GetContext()
	inv := s.createOneOffDraftInvoice("inv_no_sub", decimal.NewFromInt(100))

	resp, err := s.service.ApplyDiscountToInvoice(ctx, inv.ID)
	s.NoError(err)
	s.Require().NotNil(resp)
	s.True(resp.TotalDiscount.IsZero())
	s.True(resp.Total.Equal(decimal.NewFromInt(100)))

	filter := types.NewNoLimitCouponApplicationFilter()
	filter.InvoiceIDs = []string{inv.ID}
	apps, err := s.GetStores().CouponApplicationRepo.List(ctx, filter)
	s.NoError(err)
	s.Empty(apps)
}

func (s *ApplyDiscountToInvoiceSuite) TestRejectsNonDraftInvoice() {
	ctx := s.GetContext()
	inv := s.createOneOffDraftInvoice("inv_finalized", decimal.NewFromInt(100))
	inv.InvoiceStatus = types.InvoiceStatusFinalized
	s.NoError(s.GetStores().InvoiceRepo.Update(ctx, inv))

	_, err := s.service.ApplyDiscountToInvoice(ctx, inv.ID)
	s.Error(err)
	s.Contains(err.Error(), "not in draft status")
}

func (s *ApplyDiscountToInvoiceSuite) TestAppliesInvoiceLevelCoupon() {
	ctx := s.GetContext()

	periodStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	sub := &subscription.Subscription{
		ID: "sub_discount_1", CustomerID: s.testData.customer.ID, Currency: "usd",
		SubscriptionStatus: types.SubscriptionStatusActive,
		CurrentPeriodStart: periodStart, CurrentPeriodEnd: periodEnd,
		BillingAnchor: periodStart, StartDate: periodStart,
		BillingPeriod: types.BILLING_PERIOD_MONTHLY, BillingPeriodCount: 1,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().SubscriptionRepo.Create(ctx, sub))

	c := s.createPercentageCoupon("coupon_discount_1", 10)
	assoc := &coupon_association.CouponAssociation{
		ID: "assoc_discount_1", CouponID: c.ID, SubscriptionID: sub.ID,
		StartDate: periodStart, EnvironmentID: types.GetEnvironmentID(ctx), Coupon: c,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().CouponAssociationRepo.Create(ctx, assoc))

	inv := s.createSubscriptionDraftInvoice("inv_discount_1", sub, periodStart, periodEnd, decimal.NewFromInt(100), "price_1")

	resp, err := s.service.ApplyDiscountToInvoice(ctx, inv.ID)
	s.NoError(err)
	s.Require().NotNil(resp)
	s.True(resp.TotalDiscount.Equal(decimal.NewFromInt(10)), "want 10, got %s", resp.TotalDiscount)
	s.True(resp.Total.Equal(decimal.NewFromInt(90)), "want 90, got %s", resp.Total)

	filter := types.NewNoLimitCouponApplicationFilter()
	filter.InvoiceIDs = []string{inv.ID}
	apps, err := s.GetStores().CouponApplicationRepo.List(ctx, filter)
	s.NoError(err)
	s.Len(apps, 1)
	s.Equal(c.ID, apps[0].CouponID)
}
