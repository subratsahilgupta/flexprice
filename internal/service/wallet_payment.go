package service

import (
	"context"
	"sort"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/wallet"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// WalletPaymentStrategy defines the strategy for selecting wallets for payment
type WalletPaymentStrategy string

const (
	// PromotionalFirstStrategy prioritizes promotional wallets before prepaid wallets
	PromotionalFirstStrategy WalletPaymentStrategy = "promotional_first"
	// PrepaidFirstStrategy prioritizes prepaid wallets before promotional wallets
	PrepaidFirstStrategy WalletPaymentStrategy = "prepaid_first"
	// BalanceOptimizedStrategy selects wallets to minimize leftover balances
	BalanceOptimizedStrategy WalletPaymentStrategy = "balance_optimized"
)

// WalletPaymentOptions defines options for wallet payment processing
type WalletPaymentOptions struct {
	// Strategy determines the order in which wallets are selected
	Strategy WalletPaymentStrategy
	// MaxWalletsToUse limits the number of wallets to use (0 means no limit)
	MaxWalletsToUse int
	// AdditionalMetadata to include in payment requests
	AdditionalMetadata types.Metadata
}

// DefaultWalletPaymentOptions returns the default options for wallet payments
func DefaultWalletPaymentOptions() WalletPaymentOptions {
	return WalletPaymentOptions{
		Strategy:           PromotionalFirstStrategy,
		MaxWalletsToUse:    0,
		AdditionalMetadata: types.Metadata{},
	}
}

// WalletPaymentService defines the interface for wallet payment operations
type WalletPaymentService interface {
	// ProcessInvoicePaymentWithWallets attempts to pay an invoice using available wallets
	ProcessInvoicePaymentWithWallets(ctx context.Context, inv *invoice.Invoice, options WalletPaymentOptions) (decimal.Decimal, error)

	// GetWalletsForPayment retrieves and filters wallets suitable for payment
	GetWalletsForPayment(ctx context.Context, customerID string, currency string, options WalletPaymentOptions) ([]*wallet.Wallet, error)
}

type walletPaymentService struct {
	ServiceParams
}

// NewWalletPaymentService creates a new wallet payment service
func NewWalletPaymentService(params ServiceParams) WalletPaymentService {
	return &walletPaymentService{
		ServiceParams: params,
	}
}

// ProcessInvoicePaymentWithWallets attempts to pay an invoice using available wallets
func (s *walletPaymentService) ProcessInvoicePaymentWithWallets(
	ctx context.Context,
	inv *invoice.Invoice,
	options WalletPaymentOptions,
) (decimal.Decimal, error) {
	if inv == nil {
		return decimal.Zero, ierr.NewError("invoice cannot be nil").
			Mark(ierr.ErrInvalidOperation)
	}

	// Check if there's any amount remaining to pay
	if inv.AmountRemaining.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, nil
	}

	// Get wallets suitable for payment
	wallets, err := s.GetWalletsForPayment(ctx, inv.CustomerID, inv.Currency, options)
	if err != nil {
		return decimal.Zero, err
	}

	if len(wallets) == 0 {
		s.Logger.Infow("no suitable wallets found for payment",
			"customer_id", inv.CustomerID,
			"invoice_id", inv.ID,
			"currency", inv.Currency)
		return decimal.Zero, nil
	}

	// Calculate price type breakdown for the invoice using existing line items
	priceTypeAmounts := s.calculatePriceTypeAmounts(inv.LineItems)

	s.Logger.Infow("calculated price type breakdown for invoice",
		"invoice_id", inv.ID,
		"price_type_amounts", priceTypeAmounts)

	// Process payments using wallets with price type restrictions
	remainingAmount := inv.AmountRemaining
	initialAmount := inv.AmountRemaining
	paymentService := NewPaymentService(s.ServiceParams)

	// Limit the number of wallets if specified
	maxWallets := len(wallets)
	if options.MaxWalletsToUse > 0 && options.MaxWalletsToUse < maxWallets {
		maxWallets = options.MaxWalletsToUse
	}

	for i, w := range wallets {
		if i >= maxWallets || remainingAmount.IsZero() {
			break
		}

		// Calculate how much this wallet can pay based on its price type restrictions
		allowedAmount := s.calculateAllowedPaymentAmount(w, priceTypeAmounts, remainingAmount)
		if allowedAmount.IsZero() {
			s.Logger.Infow("wallet cannot pay any amount due to price type restrictions",
				"wallet_id", w.ID,
				"wallet_config", w.Config,
				"price_type_amounts", priceTypeAmounts)
			continue
		}

		paymentAmount := decimal.Min(allowedAmount, w.Balance)
		if paymentAmount.IsZero() {
			continue
		}

		// Create payment request
		metadata := types.Metadata{
			"wallet_type": string(w.WalletType),
			"wallet_id":   w.ID,
		}

		// Add additional metadata if provided
		for k, v := range options.AdditionalMetadata {
			metadata[k] = v
		}

		paymentReq := dto.CreatePaymentRequest{
			Amount:            paymentAmount,
			Currency:          inv.Currency,
			PaymentMethodType: types.PaymentMethodTypeCredits,
			PaymentMethodID:   w.ID,
			DestinationType:   types.PaymentDestinationTypeInvoice,
			DestinationID:     inv.ID,
			ProcessPayment:    true,
			Metadata:          metadata,
		}

		_, err := paymentService.CreatePayment(ctx, &paymentReq)
		if err != nil {
			s.Logger.Errorw("failed to create credits payment",
				"error", err,
				"invoice_id", inv.ID,
				"wallet_id", w.ID,
				"wallet_type", w.WalletType)
			continue
		}

		remainingAmount = remainingAmount.Sub(paymentAmount)

		// Update the price type amounts to reflect what was paid
		s.updatePriceTypeAmountsAfterPayment(priceTypeAmounts, paymentAmount, &w.Config)
	}

	amountPaid := initialAmount.Sub(remainingAmount)

	if !amountPaid.IsZero() {
		s.Logger.Infow("payment processed using wallets",
			"invoice_id", inv.ID,
			"amount_paid", amountPaid,
			"remaining_amount", remainingAmount)
	} else {
		s.Logger.Infow("no payments processed using wallets",
			"invoice_id", inv.ID,
			"amount", initialAmount)
	}

	return amountPaid, nil
}

// GetWalletsForPayment retrieves and filters wallets suitable for payment
func (s *walletPaymentService) GetWalletsForPayment(
	ctx context.Context,
	customerID string,
	currency string,
	options WalletPaymentOptions,
) ([]*wallet.Wallet, error) {
	// Get all wallets for the customer
	wallets, err := s.WalletRepo.GetWalletsByCustomerID(ctx, customerID)
	if err != nil {
		return nil, err
	}

	// Filter active wallets with matching currency
	activeWallets := make([]*wallet.Wallet, 0)
	for _, w := range wallets {
		if w.WalletStatus == types.WalletStatusActive &&
			types.IsMatchingCurrency(w.Currency, currency) &&
			w.Balance.GreaterThan(decimal.Zero) {
			activeWallets = append(activeWallets, w)
		}
	}

	// Sort wallets based on the selected strategy
	sortedWallets := s.sortWalletsByStrategy(activeWallets, options.Strategy)

	return sortedWallets, nil
}

// sortWalletsByStrategy sorts wallets based on the specified strategy
func (s *walletPaymentService) sortWalletsByStrategy(
	wallets []*wallet.Wallet,
	strategy WalletPaymentStrategy,
) []*wallet.Wallet {
	result := make([]*wallet.Wallet, 0, len(wallets))
	if len(wallets) == 0 {
		return result
	}

	// Copy wallets to avoid modifying the original slice
	result = append(result, wallets...)

	if strategy == "" {
		strategy = PromotionalFirstStrategy
	}

	// Sort by balance (smallest first to minimize leftover balances)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Balance.LessThan(result[j].Balance)
	})

	switch strategy {
	case PromotionalFirstStrategy:
		// First separate wallets by type
		sort.Slice(result, func(i, j int) bool {
			return result[i].WalletType == types.WalletTypePromotional
		})
	case PrepaidFirstStrategy:
		sort.Slice(result, func(i, j int) bool {
			return result[i].WalletType == types.WalletTypePrePaid
		})
	case BalanceOptimizedStrategy:
		// Sort by balance (smallest first to minimize leftover balances)
		sort.Slice(result, func(i, j int) bool {
			return result[i].Balance.LessThan(result[j].Balance)
		})
	default:
		// Default to promotional first if strategy is not recognized
		return s.sortWalletsByStrategy(wallets, PromotionalFirstStrategy)
	}

	return result
}

// calculatePriceTypeAmounts calculates the total amount for each price type in the invoice
func (s *walletPaymentService) calculatePriceTypeAmounts(lineItems []*invoice.InvoiceLineItem) map[string]decimal.Decimal {
	priceTypeAmounts := map[string]decimal.Decimal{
		string(types.PRICE_TYPE_USAGE): decimal.Zero,
		string(types.PRICE_TYPE_FIXED): decimal.Zero,
	}

	for _, item := range lineItems {
		if item.PriceType != nil {
			priceType := *item.PriceType
			if amount, exists := priceTypeAmounts[priceType]; exists {
				priceTypeAmounts[priceType] = amount.Add(item.Amount)
			} else {
				// If it's not USAGE or FIXED, treat it as FIXED by default
				priceTypeAmounts[string(types.PRICE_TYPE_FIXED)] = priceTypeAmounts[string(types.PRICE_TYPE_FIXED)].Add(item.Amount)
			}
		} else {
			// If price type is nil, treat it as FIXED by default
			priceTypeAmounts[string(types.PRICE_TYPE_FIXED)] = priceTypeAmounts[string(types.PRICE_TYPE_FIXED)].Add(item.Amount)
		}
	}

	return priceTypeAmounts
}

// calculateAllowedPaymentAmount calculates how much a wallet can pay based on its price type restrictions
func (s *walletPaymentService) calculateAllowedPaymentAmount(
	w *wallet.Wallet,
	priceTypeAmounts map[string]decimal.Decimal,
	remainingAmount decimal.Decimal,
) decimal.Decimal {
	// If wallet has no allowed price types, use default (ALL)
	if w.Config.AllowedPriceTypes == nil || len(w.Config.AllowedPriceTypes) == 0 {
		// Return the minimum of remaining amount and wallet balance
		return decimal.Min(remainingAmount, w.Balance)
	}

	allowedAmount := decimal.Zero

	for _, allowedPriceType := range w.Config.AllowedPriceTypes {
		switch allowedPriceType {
		case types.WalletConfigPriceTypeAll:
			// If ALL is allowed, wallet can pay the full remaining amount (up to its balance)
			return decimal.Min(remainingAmount, w.Balance)
		case types.WalletConfigPriceTypeUsage:
			// Add the remaining USAGE amount
			usageAmount := priceTypeAmounts[string(types.PRICE_TYPE_USAGE)]
			allowedAmount = allowedAmount.Add(usageAmount)
		case types.WalletConfigPriceTypeFixed:
			// Add the remaining FIXED amount
			fixedAmount := priceTypeAmounts[string(types.PRICE_TYPE_FIXED)]
			allowedAmount = allowedAmount.Add(fixedAmount)
		}
	}

	// Return the minimum of allowed amount, remaining amount, and wallet balance
	return decimal.Min(decimal.Min(allowedAmount, remainingAmount), w.Balance)
}

// updatePriceTypeAmountsAfterPayment updates the price type amounts after a payment is made
func (s *walletPaymentService) updatePriceTypeAmountsAfterPayment(
	priceTypeAmounts map[string]decimal.Decimal,
	paymentAmount decimal.Decimal,
	walletConfig *types.WalletConfig,
) {
	// If wallet has no allowed price types, deduct proportionally
	if walletConfig.AllowedPriceTypes == nil || len(walletConfig.AllowedPriceTypes) == 0 {
		s.deductProportionally(priceTypeAmounts, paymentAmount)
		return
	}

	// Check if ALL is allowed
	for _, allowedPriceType := range walletConfig.AllowedPriceTypes {
		if allowedPriceType == types.WalletConfigPriceTypeAll {
			s.deductProportionally(priceTypeAmounts, paymentAmount)
			return
		}
	}

	// Deduct from specific price types based on wallet config
	remainingPayment := paymentAmount

	// First, try to deduct from USAGE if allowed
	for _, allowedPriceType := range walletConfig.AllowedPriceTypes {
		if allowedPriceType == types.WalletConfigPriceTypeUsage && remainingPayment.GreaterThan(decimal.Zero) {
			usageAmount := priceTypeAmounts[string(types.PRICE_TYPE_USAGE)]
			deductAmount := decimal.Min(usageAmount, remainingPayment)
			priceTypeAmounts[string(types.PRICE_TYPE_USAGE)] = usageAmount.Sub(deductAmount)
			remainingPayment = remainingPayment.Sub(deductAmount)
		}
	}

	// Then, try to deduct from FIXED if allowed
	for _, allowedPriceType := range walletConfig.AllowedPriceTypes {
		if allowedPriceType == types.WalletConfigPriceTypeFixed && remainingPayment.GreaterThan(decimal.Zero) {
			fixedAmount := priceTypeAmounts[string(types.PRICE_TYPE_FIXED)]
			deductAmount := decimal.Min(fixedAmount, remainingPayment)
			priceTypeAmounts[string(types.PRICE_TYPE_FIXED)] = fixedAmount.Sub(deductAmount)
			remainingPayment = remainingPayment.Sub(deductAmount)
		}
	}
}

// deductProportionally deducts payment amount proportionally from all price types
func (s *walletPaymentService) deductProportionally(
	priceTypeAmounts map[string]decimal.Decimal,
	paymentAmount decimal.Decimal,
) {
	totalAmount := decimal.Zero
	for _, amount := range priceTypeAmounts {
		totalAmount = totalAmount.Add(amount)
	}

	if totalAmount.IsZero() {
		return
	}

	// Deduct proportionally from each price type
	for priceType, amount := range priceTypeAmounts {
		if amount.GreaterThan(decimal.Zero) {
			proportion := amount.Div(totalAmount)
			deductAmount := paymentAmount.Mul(proportion)
			priceTypeAmounts[priceType] = amount.Sub(deductAmount)
		}
	}
}
