package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/wallet"
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
		return decimal.Zero, fmt.Errorf("invoice cannot be nil")
	}

	// Check if there's any amount remaining to pay
	if inv.AmountRemaining.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, nil
	}

	// Get wallets suitable for payment
	wallets, err := s.GetWalletsForPayment(ctx, inv.CustomerID, inv.Currency, options)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to get wallets for payment: %w", err)
	}

	if len(wallets) == 0 {
		s.Logger.Infow("no suitable wallets found for payment",
			"customer_id", inv.CustomerID,
			"invoice_id", inv.ID,
			"currency", inv.Currency)
		return decimal.Zero, nil
	}

	// Process payments using wallets
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

		paymentAmount := decimal.Min(remainingAmount, w.Balance)
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
		return nil, fmt.Errorf("failed to get customer wallets: %w", err)
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
