package service

import (
	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/fxrate"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// seedTenantRate creates a published tenant-scope fx rate for the pair.
func (s *WalletServiceSuite) seedTenantRate(from, to, rate string) {
	s.NoError(s.GetStores().FXRateRepo.Create(s.GetContext(), &fxrate.FXRate{
		ID:            types.GenerateUUIDWithPrefix(types.UUID_PREFIX_FX_RATE),
		Scope:         types.FXRateScopeTenant,
		ScopeID:       types.FXRateScopeIDTenant,
		FromCurrency:  from,
		ToCurrency:    to,
		Rate:          decimal.RequireFromString(rate),
		EnvironmentID: types.GetEnvironmentID(s.GetContext()),
		BaseModel:     types.GetDefaultBaseModel(s.GetContext()),
	}))
}

func (s *WalletServiceSuite) setCustomerBillingCurrency(billing string) {
	s.testData.customer.BillingCurrency = lo.ToPtr(billing)
	s.NoError(s.GetStores().CustomerRepo.Update(s.GetContext(), s.testData.customer))
}

// A missing rate fails the top-up instead of leaving a pending credit (the in-memory WithTx doesn't
// roll back, so this asserts the error).
func (s *WalletServiceSuite) TestTopUpWallet_CrossCurrency_MissingRate_Errors() {
	s.seedAutoComplete(false)
	s.setCustomerBillingCurrency("inr") // wallet is usd; no usd->inr rate seeded

	_, err := s.service.TopUpWallet(s.GetContext(), s.testData.wallet.ID, &dto.TopUpWalletRequest{
		CreditsToAdd:      decimal.NewFromInt(100),
		TransactionReason: types.TransactionReasonPurchasedCreditInvoiced,
		IdempotencyKey:    lo.ToPtr("topup-xcur-norate"),
	})
	s.Require().Error(err, "a cross-currency top-up with no rate must fail inside the tx")
}

// With a rate, the top-up invoice converts in the top-up tx; the wallet credit keeps its own currency.
func (s *WalletServiceSuite) TestTopUpWallet_CrossCurrency_ConvertsInvoiceInsideTx() {
	s.seedAutoComplete(false)
	s.setCustomerBillingCurrency("inr")
	s.seedTenantRate("usd", "inr", "83")

	resp, err := s.service.TopUpWallet(s.GetContext(), s.testData.wallet.ID, &dto.TopUpWalletRequest{
		CreditsToAdd:      decimal.NewFromInt(100),
		TransactionReason: types.TransactionReasonPurchasedCreditInvoiced,
		IdempotencyKey:    lo.ToPtr("topup-xcur-rate"),
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp.InvoiceID)

	// Invoice converted to the billing currency, at the frozen rate.
	inv, err := s.GetStores().InvoiceRepo.Get(s.GetContext(), *resp.InvoiceID)
	s.Require().NoError(err)
	s.Equal("inr", inv.Currency, "top-up invoice must be issued in the billing currency")
	s.Require().NotNil(inv.FxConversion)
	s.Equal("usd", inv.FxConversion.ChargeCurrency)
	s.True(decimal.RequireFromString("8300").Equal(inv.AmountDue), "amount_due got %s", inv.AmountDue)

	// Pending wallet credit stays in the wallet's own currency — conversion never touches it.
	s.Require().NotNil(resp.WalletTransaction)
	s.Equal("usd", resp.WalletTransaction.Currency)
	s.True(decimal.NewFromInt(100).Equal(resp.WalletTransaction.CreditAmount), "credits got %s", resp.WalletTransaction.CreditAmount)
}

// A pay-first top-up draft is converted inside the top-up tx, since finalize won't see it.
func (s *WalletServiceSuite) TestTopUpWallet_PayFirstCrossCurrency_ConvertsDraftInsideTx() {
	s.setCustomerBillingCurrency("inr")
	s.seedTenantRate("usd", "inr", "83")

	ws := s.service.(*walletService)
	_, invoiceID, err := ws.handlePurchasedCreditInvoicedTransaction(
		s.GetContext(), s.testData.wallet.ID, lo.ToPtr("topup-payfirst-rate"),
		&dto.TopUpWalletRequest{
			CreditsToAdd:      decimal.NewFromInt(100),
			TransactionReason: types.TransactionReasonPurchasedCreditInvoiced,
			Checkout:          s.checkoutParamsRazorpay(),
			IdempotencyKey:    lo.ToPtr("topup-payfirst-rate"),
		})
	s.Require().NoError(err)

	inv, err := s.GetStores().InvoiceRepo.Get(s.GetContext(), invoiceID)
	s.Require().NoError(err)
	s.Equal(types.InvoiceStatusDraft, inv.InvoiceStatus, "pay-first invoice stays DRAFT until checkout completes")
	s.Equal("inr", inv.Currency, "pay-first draft must be converted inside the top-up tx (before checkout)")
	s.Require().NotNil(inv.FxConversion, "draft must carry fx_conversion after the in-tx conversion")
	s.Equal("usd", inv.FxConversion.ChargeCurrency)
}

// A pay-first top-up with no rate fails inside the top-up tx rather than later at checkout.
func (s *WalletServiceSuite) TestTopUpWallet_PayFirstCrossCurrency_MissingRate_Errors() {
	s.setCustomerBillingCurrency("inr") // no usd->inr rate

	ws := s.service.(*walletService)
	_, _, err := ws.handlePurchasedCreditInvoicedTransaction(
		s.GetContext(), s.testData.wallet.ID, lo.ToPtr("topup-payfirst-norate"),
		&dto.TopUpWalletRequest{
			CreditsToAdd:      decimal.NewFromInt(100),
			TransactionReason: types.TransactionReasonPurchasedCreditInvoiced,
			Checkout:          s.checkoutParamsRazorpay(),
			IdempotencyKey:    lo.ToPtr("topup-payfirst-norate"),
		})
	s.Require().Error(err, "a pay-first cross-currency top-up with no rate must fail inside the top-up tx")
}
