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

func (s *ApplyDiscountToInvoiceSuite) TestCalledTwiceDoesNotDoubleDiscount() {
	ctx := s.GetContext()

	periodStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	sub := &subscription.Subscription{
		ID: "sub_discount_2", CustomerID: s.testData.customer.ID, Currency: "usd",
		SubscriptionStatus: types.SubscriptionStatusActive,
		CurrentPeriodStart: periodStart, CurrentPeriodEnd: periodEnd,
		BillingAnchor: periodStart, StartDate: periodStart,
		BillingPeriod: types.BILLING_PERIOD_MONTHLY, BillingPeriodCount: 1,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().SubscriptionRepo.Create(ctx, sub))

	c := s.createPercentageCoupon("coupon_discount_2", 10)
	assoc := &coupon_association.CouponAssociation{
		ID: "assoc_discount_2", CouponID: c.ID, SubscriptionID: sub.ID,
		StartDate: periodStart, EnvironmentID: types.GetEnvironmentID(ctx), Coupon: c,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().CouponAssociationRepo.Create(ctx, assoc))

	inv := s.createSubscriptionDraftInvoice("inv_discount_2", sub, periodStart, periodEnd, decimal.NewFromInt(100), "price_2")

	first, err := s.service.ApplyDiscountToInvoice(ctx, inv.ID)
	s.NoError(err)
	second, err := s.service.ApplyDiscountToInvoice(ctx, inv.ID)
	s.NoError(err)

	s.True(first.TotalDiscount.Equal(second.TotalDiscount), "first %s vs second %s", first.TotalDiscount, second.TotalDiscount)
	s.True(second.TotalDiscount.Equal(decimal.NewFromInt(10)))
	s.Require().Len(second.LineItems, 1)
	s.True(second.LineItems[0].LineItemDiscount.Equal(decimal.Zero), "invoice-level discount must not leak into line_item_discount")
	s.True(second.LineItems[0].InvoiceLevelDiscount.Equal(decimal.NewFromInt(10)))

	filter := types.NewNoLimitCouponApplicationFilter()
	filter.InvoiceIDs = []string{inv.ID}
	apps, err := s.GetStores().CouponApplicationRepo.List(ctx, filter)
	s.NoError(err)
	s.Len(apps, 1, "second call must not create a duplicate CouponApplication row")
}

// TestSubscriptionInvoiceWithNoAssociationsIsNoOp is distinct from TestNoSubscriptionIsNoOp:
// this invoice DOES have a subscription and a period, so resolveCurrentInvoiceCoupons goes
// through selectSubscriptionCoupons (not the early nil-subscription return) and must itself
// produce an empty result when there are simply no CouponAssociation rows to find.
func (s *ApplyDiscountToInvoiceSuite) TestSubscriptionInvoiceWithNoAssociationsIsNoOp() {
	ctx := s.GetContext()

	periodStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	sub := &subscription.Subscription{
		ID: "sub_discount_6", CustomerID: s.testData.customer.ID, Currency: "usd",
		SubscriptionStatus: types.SubscriptionStatusActive,
		CurrentPeriodStart: periodStart, CurrentPeriodEnd: periodEnd,
		BillingAnchor: periodStart, StartDate: periodStart,
		BillingPeriod: types.BILLING_PERIOD_MONTHLY, BillingPeriodCount: 1,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().SubscriptionRepo.Create(ctx, sub))

	inv := s.createSubscriptionDraftInvoice("inv_discount_6", sub, periodStart, periodEnd, decimal.NewFromInt(100), "price_6")

	resp, err := s.service.ApplyDiscountToInvoice(ctx, inv.ID)
	s.NoError(err)
	s.True(resp.TotalDiscount.IsZero())
	s.True(resp.Total.Equal(decimal.NewFromInt(100)))
}

func (s *ApplyDiscountToInvoiceSuite) TestAssociationRemovedResetsToZero() {
	ctx := s.GetContext()

	periodStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	sub := &subscription.Subscription{
		ID: "sub_discount_3", CustomerID: s.testData.customer.ID, Currency: "usd",
		SubscriptionStatus: types.SubscriptionStatusActive,
		CurrentPeriodStart: periodStart, CurrentPeriodEnd: periodEnd,
		BillingAnchor: periodStart, StartDate: periodStart,
		BillingPeriod: types.BILLING_PERIOD_MONTHLY, BillingPeriodCount: 1,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().SubscriptionRepo.Create(ctx, sub))

	c := s.createPercentageCoupon("coupon_discount_3", 10)
	assoc := &coupon_association.CouponAssociation{
		ID: "assoc_discount_3", CouponID: c.ID, SubscriptionID: sub.ID,
		StartDate: periodStart, EnvironmentID: types.GetEnvironmentID(ctx), Coupon: c,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().CouponAssociationRepo.Create(ctx, assoc))

	inv := s.createSubscriptionDraftInvoice("inv_discount_3", sub, periodStart, periodEnd, decimal.NewFromInt(100), "price_3")

	first, err := s.service.ApplyDiscountToInvoice(ctx, inv.ID)
	s.NoError(err)
	s.True(first.TotalDiscount.Equal(decimal.NewFromInt(10)))

	// Association ends before the invoice's period -> no longer active.
	expired := periodStart.Add(-time.Second)
	assoc.EndDate = &expired
	s.NoError(s.GetStores().CouponAssociationRepo.Update(ctx, assoc))

	second, err := s.service.ApplyDiscountToInvoice(ctx, inv.ID)
	s.NoError(err)
	s.True(second.TotalDiscount.IsZero(), "want 0, got %s", second.TotalDiscount)
	s.True(second.Total.Equal(decimal.NewFromInt(100)))

	filter := types.NewNoLimitCouponApplicationFilter()
	filter.InvoiceIDs = []string{inv.ID}
	apps, err := s.GetStores().CouponApplicationRepo.List(ctx, filter)
	s.NoError(err)
	s.Empty(apps, "stale coupon application from the first call must be cleaned up")
}

func (s *ApplyDiscountToInvoiceSuite) TestPreservesExistingTax() {
	ctx := s.GetContext()

	periodStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	sub := &subscription.Subscription{
		ID: "sub_discount_4", CustomerID: s.testData.customer.ID, Currency: "usd",
		SubscriptionStatus: types.SubscriptionStatusActive,
		CurrentPeriodStart: periodStart, CurrentPeriodEnd: periodEnd,
		BillingAnchor: periodStart, StartDate: periodStart,
		BillingPeriod: types.BILLING_PERIOD_MONTHLY, BillingPeriodCount: 1,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().SubscriptionRepo.Create(ctx, sub))

	c := s.createPercentageCoupon("coupon_discount_4", 10)
	assoc := &coupon_association.CouponAssociation{
		ID: "assoc_discount_4", CouponID: c.ID, SubscriptionID: sub.ID,
		StartDate: periodStart, EnvironmentID: types.GetEnvironmentID(ctx), Coupon: c,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().CouponAssociationRepo.Create(ctx, assoc))

	inv := s.createSubscriptionDraftInvoice("inv_discount_4", sub, periodStart, periodEnd, decimal.NewFromInt(100), "price_4")

	// Simulate RecalculateTaxesOnInvoice having already run: 8% tax on the pre-discount subtotal.
	inv.TotalTax = decimal.NewFromInt(8)
	inv.Total = inv.Subtotal.Add(inv.TotalTax)
	inv.AmountDue = inv.Total
	s.NoError(s.GetStores().InvoiceRepo.Update(ctx, inv))

	resp, err := s.service.ApplyDiscountToInvoice(ctx, inv.ID)
	s.NoError(err)
	// Subtotal(100) - Discount(10) + Tax(8) = 98, not Subtotal - Discount = 90.
	s.True(resp.Total.Equal(decimal.NewFromInt(98)), "want 98, got %s", resp.Total)
	s.True(resp.TotalTax.Equal(decimal.NewFromInt(8)), "tax must be untouched")
}

func (s *ApplyDiscountToInvoiceSuite) TestLineItemLevelCoupon() {
	ctx := s.GetContext()

	periodStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	sub := &subscription.Subscription{
		ID: "sub_discount_5", CustomerID: s.testData.customer.ID, Currency: "usd",
		SubscriptionStatus: types.SubscriptionStatusActive,
		CurrentPeriodStart: periodStart, CurrentPeriodEnd: periodEnd,
		BillingAnchor: periodStart, StartDate: periodStart,
		BillingPeriod: types.BILLING_PERIOD_MONTHLY, BillingPeriodCount: 1,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().SubscriptionRepo.Create(ctx, sub))

	sli := &subscription.SubscriptionLineItem{
		ID: "sli_discount_5", SubscriptionID: sub.ID, CustomerID: s.testData.customer.ID,
		PriceID: "price_5", Quantity: decimal.NewFromInt(1),
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().SubscriptionLineItemRepo.Create(ctx, sli))

	c := s.createPercentageCoupon("coupon_discount_5", 20)
	assoc := &coupon_association.CouponAssociation{
		ID: "assoc_discount_5", CouponID: c.ID, SubscriptionID: sub.ID,
		SubscriptionLineItemID: &sli.ID,
		StartDate:              periodStart, EnvironmentID: types.GetEnvironmentID(ctx), Coupon: c,
		BaseModel: types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().CouponAssociationRepo.Create(ctx, assoc))

	inv := s.createSubscriptionDraftInvoice("inv_discount_5", sub, periodStart, periodEnd, decimal.NewFromInt(50), "price_5")

	first, err := s.service.ApplyDiscountToInvoice(ctx, inv.ID)
	s.NoError(err)
	s.Require().Len(first.LineItems, 1)
	s.True(first.LineItems[0].LineItemDiscount.Equal(decimal.NewFromInt(10)), "want 10, got %s", first.LineItems[0].LineItemDiscount)
	s.True(first.TotalDiscount.Equal(decimal.NewFromInt(10)))

	second, err := s.service.ApplyDiscountToInvoice(ctx, inv.ID)
	s.NoError(err)
	s.Require().Len(second.LineItems, 1)
	s.True(second.LineItems[0].LineItemDiscount.Equal(decimal.NewFromInt(10)), "must not double on repeat call, got %s", second.LineItems[0].LineItemDiscount)
}
