package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/entitlement"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/domain/feature"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/domain/wallet"
	"github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/publisher"
	"github.com/flexprice/flexprice/internal/types"
	webhookPublisher "github.com/flexprice/flexprice/internal/webhook/publisher"
	"github.com/shopspring/decimal"
)

// WalletService defines the interface for wallet operations
type WalletService interface {
	// CreateWallet creates a new wallet for a customer
	CreateWallet(ctx context.Context, req *dto.CreateWalletRequest) (*dto.WalletResponse, error)

	// GetWalletsByCustomerID retrieves all wallets for a customer
	GetWalletsByCustomerID(ctx context.Context, customerID string) ([]*dto.WalletResponse, error)

	// GetWalletByID retrieves a wallet by its ID and calculates real-time balance
	GetWalletByID(ctx context.Context, id string) (*dto.WalletResponse, error)

	// GetWalletTransactions retrieves transactions for a wallet with pagination
	GetWalletTransactions(ctx context.Context, walletID string, filter *types.WalletTransactionFilter) (*dto.ListWalletTransactionsResponse, error)

	// TopUpWallet adds credits to a wallet
	TopUpWallet(ctx context.Context, walletID string, req *dto.TopUpWalletRequest) (*dto.WalletResponse, error)

	// GetWalletBalance retrieves the real-time balance of a wallet
	GetWalletBalance(ctx context.Context, walletID string) (*dto.WalletBalanceResponse, error)

	// TerminateWallet terminates a wallet by closing it and debiting remaining balance
	TerminateWallet(ctx context.Context, walletID string) error

	// UpdateWallet updates a wallet
	UpdateWallet(ctx context.Context, id string, req *dto.UpdateWalletRequest) (*wallet.Wallet, error)

	// DebitWallet processes a debit operation on a wallet
	DebitWallet(ctx context.Context, req *wallet.WalletOperation) error

	// CreditWallet processes a credit operation on a wallet
	CreditWallet(ctx context.Context, req *wallet.WalletOperation) error
}

type walletService struct {
	walletRepo       wallet.Repository
	logger           *logger.Logger
	subscriptionRepo subscription.Repository
	planRepo         plan.Repository
	priceRepo        price.Repository
	eventRepo        events.Repository
	meterRepo        meter.Repository
	customerRepo     customer.Repository
	invoiceRepo      invoice.Repository
	entitlementRepo  entitlement.Repository
	featureRepo      feature.Repository
	eventPublisher   publisher.EventPublisher
	webhookPublisher webhookPublisher.WebhookPublisher
	db               postgres.IClient
	config           *config.Configuration
}

// NewWalletService creates a new instance of WalletService
func NewWalletService(
	walletRepo wallet.Repository,
	logger *logger.Logger,
	subscriptionRepo subscription.Repository,
	planRepo plan.Repository,
	priceRepo price.Repository,
	eventRepo events.Repository,
	meterRepo meter.Repository,
	customerRepo customer.Repository,
	invoiceRepo invoice.Repository,
	entitlementRepo entitlement.Repository,
	featureRepo feature.Repository,
	eventPublisher publisher.EventPublisher,
	webhookPublisher webhookPublisher.WebhookPublisher,
	db postgres.IClient,
	config *config.Configuration,
) WalletService {
	return &walletService{
		walletRepo:       walletRepo,
		logger:           logger,
		subscriptionRepo: subscriptionRepo,
		planRepo:         planRepo,
		priceRepo:        priceRepo,
		eventRepo:        eventRepo,
		meterRepo:        meterRepo,
		customerRepo:     customerRepo,
		invoiceRepo:      invoiceRepo,
		entitlementRepo:  entitlementRepo,
		featureRepo:      featureRepo,
		eventPublisher:   eventPublisher,
		webhookPublisher: webhookPublisher,
		db:               db,
		config:           config,
	}
}

func (s *walletService) CreateWallet(ctx context.Context, req *dto.CreateWalletRequest) (*dto.WalletResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Check if customer already has an active wallet
	existingWallets, err := s.walletRepo.GetWalletsByCustomerID(ctx, req.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing wallets: %w", err)
	}

	for _, w := range existingWallets {
		if w.WalletStatus == types.WalletStatusActive && w.Currency == req.Currency {
			s.logger.Warnw("customer already has an active wallet in the same currency",
				"customer_id", req.CustomerID,
				"existing_wallet_id", w.ID,
			)
			return nil, fmt.Errorf("customer already has an active wallet with ID: %s", w.ID)
		}
	}

	w := req.ToWallet(ctx)

	// Create wallet in DB and update the wallet object
	if err := s.walletRepo.CreateWallet(ctx, w); err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	s.logger.Debugw("created wallet",
		"wallet_id", w.ID,
		"customer_id", w.CustomerID,
		"currency", w.Currency,
	)

	// Convert to response DTO
	return dto.FromWallet(w), nil
}

func (s *walletService) GetWalletsByCustomerID(ctx context.Context, customerID string) ([]*dto.WalletResponse, error) {
	wallets, err := s.walletRepo.GetWalletsByCustomerID(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallets: %w", err)
	}

	response := make([]*dto.WalletResponse, len(wallets))
	for i, w := range wallets {
		response[i] = dto.FromWallet(w)
	}

	return response, nil
}

func (s *walletService) GetWalletByID(ctx context.Context, id string) (*dto.WalletResponse, error) {
	w, err := s.walletRepo.GetWalletByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	return dto.FromWallet(w), nil
}

func (s *walletService) GetWalletTransactions(ctx context.Context, walletID string, filter *types.WalletTransactionFilter) (*dto.ListWalletTransactionsResponse, error) {
	if filter == nil {
		filter = types.NewWalletTransactionFilter()
	}

	filter.WalletID = &walletID

	if err := filter.Validate(); err != nil {
		return nil, fmt.Errorf("invalid filter: %w", err)
	}

	transactions, err := s.walletRepo.ListWalletTransactions(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get transactions: %w", err)
	}

	count, err := s.walletRepo.CountWalletTransactions(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to count transactions: %w", err)
	}

	response := &dto.ListWalletTransactionsResponse{
		Items: make([]*dto.WalletTransactionResponse, len(transactions)),
		Pagination: types.NewPaginationResponse(
			count,
			filter.GetLimit(),
			filter.GetOffset(),
		),
	}

	for i, txn := range transactions {
		response.Items[i] = dto.FromWalletTransaction(txn)
	}

	return response, nil
}

// Update the TopUpWallet method to use the new processWalletOperation
func (s *walletService) TopUpWallet(ctx context.Context, walletID string, req *dto.TopUpWalletRequest) (*dto.WalletResponse, error) {
	// Create a credit operation
	creditReq := &wallet.WalletOperation{
		WalletID:          walletID,
		Type:              types.TransactionTypeCredit,
		CreditAmount:      req.Amount,
		Description:       req.Description,
		Metadata:          req.Metadata,
		TransactionReason: types.TransactionReasonFreeCredit,
		ExpiryDate:        req.ExpiryDate,
	}

	if req.PurchasedCredits {
		if req.GenerateInvoice {
			creditReq.TransactionReason = types.TransactionReasonPurchasedCreditInvoiced
		} else {
			creditReq.TransactionReason = types.TransactionReasonPurchasedCreditDirect
		}
	}

	if err := s.CreditWallet(ctx, creditReq); err != nil {
		return nil, fmt.Errorf("failed to credit wallet: %w", err)
	}

	// Get updated wallet
	return s.GetWalletByID(ctx, walletID)
}

func (s *walletService) GetWalletBalance(ctx context.Context, walletID string) (*dto.WalletBalanceResponse, error) {
	response := &dto.WalletBalanceResponse{
		RealTimeBalance: decimal.Zero,
	}

	w, err := s.walletRepo.GetWalletByID(ctx, walletID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	if w.WalletStatus != types.WalletStatusActive {
		response.Wallet = w
		response.RealTimeBalance = decimal.Zero
		response.RealTimeCreditBalance = decimal.Zero
		response.BalanceUpdatedAt = w.UpdatedAt
		return response, nil
	}

	// Get invoice summary for unpaid amounts
	invoiceService := NewInvoiceService(
		s.subscriptionRepo,
		s.planRepo,
		s.priceRepo,
		s.eventRepo,
		s.meterRepo,
		s.customerRepo,
		s.invoiceRepo,
		s.entitlementRepo,
		s.featureRepo,
		s.eventPublisher,
		s.webhookPublisher,
		s.db,
		s.logger,
		s.config,
	)

	invoiceSummary, err := invoiceService.GetCustomerInvoiceSummary(ctx, w.CustomerID, w.Currency)
	if err != nil {
		return nil, fmt.Errorf("failed to get invoice summary: %w", err)
	}

	// Get current period usage for active subscriptions
	subscriptionService := NewSubscriptionService(
		s.subscriptionRepo,
		s.planRepo,
		s.priceRepo,
		s.eventRepo,
		s.meterRepo,
		s.customerRepo,
		s.invoiceRepo,
		s.entitlementRepo,
		s.featureRepo,
		s.eventPublisher,
		s.webhookPublisher,
		s.db,
		s.logger,
		s.config,
	)

	filter := types.NewSubscriptionFilter()
	filter.CustomerID = w.CustomerID
	filter.SubscriptionStatus = []types.SubscriptionStatus{
		types.SubscriptionStatusActive,
	}

	subscriptionsResp, err := subscriptionService.ListSubscriptions(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscriptions: %w", err)
	}

	currentPeriodUsage := decimal.Zero
	for _, sub := range subscriptionsResp.Items {
		// Skip subscriptions with different currency
		if !types.IsMatchingCurrency(sub.Subscription.Currency, w.Currency) {
			continue
		}

		// Get current period usage for subscription
		usageResp, err := subscriptionService.GetUsageBySubscription(ctx, &dto.GetUsageBySubscriptionRequest{
			SubscriptionID: sub.Subscription.ID,
			StartTime:      sub.Subscription.CurrentPeriodStart,
			EndTime:        time.Now().UTC(),
			LifetimeUsage:  false, // Only get current period usage
		})
		if err != nil {
			s.logger.Errorw("failed to get current period usage",
				"wallet_id", walletID,
				"subscription_id", sub.ID,
				"error", err,
			)
			continue
		}

		if usageResp.Amount > 0 {
			currentPeriodUsage = currentPeriodUsage.Add(decimal.NewFromFloat(usageResp.Amount))
		}
	}

	// Calculate real-time balance:
	// wallet_balance - (unpaid_invoices + current_period_usage)
	// NOTE: in future, we can add a feature to allow customers to set a threshold for real-time balance
	// NOTE: in future we can restrict a wallet balance to be adjusted only for usage or fixed amount
	realTimeBalance := w.Balance.
		Sub(invoiceSummary.TotalUnpaidAmount).
		Sub(currentPeriodUsage)

	s.logger.Debugw("calculated real-time balance",
		"wallet_id", walletID,
		"current_balance", w.Balance,
		"unpaid_invoices", invoiceSummary.TotalUnpaidAmount,
		"current_period_usage", currentPeriodUsage,
		"real_time_balance", realTimeBalance,
		"currency", w.Currency,
	)

	return &dto.WalletBalanceResponse{
		Wallet:                w,
		RealTimeBalance:       realTimeBalance,
		RealTimeCreditBalance: realTimeBalance.Div(w.ConversionRate),
		BalanceUpdatedAt:      time.Now().UTC(),
		UnpaidInvoiceAmount:   invoiceSummary.TotalUnpaidAmount,
		CurrentPeriodUsage:    currentPeriodUsage,
	}, nil
}

// Update the TerminateWallet method to use the new processWalletOperation
func (s *walletService) TerminateWallet(ctx context.Context, walletID string) error {
	w, err := s.walletRepo.GetWalletByID(ctx, walletID)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}

	if w.WalletStatus == types.WalletStatusClosed {
		return fmt.Errorf("wallet is already closed")
	}

	// Use client's WithTx for atomic operations
	return s.db.WithTx(ctx, func(ctx context.Context) error {
		// Debit remaining balance if any
		if w.CreditBalance.GreaterThan(decimal.Zero) {
			debitReq := &wallet.WalletOperation{
				WalletID:          walletID,
				CreditAmount:      w.CreditBalance,
				Type:              types.TransactionTypeDebit,
				Description:       "Wallet termination - remaining balance debit",
				TransactionReason: types.TransactionReasonWalletTermination,
			}

			if err := s.processWalletOperation(ctx, debitReq); err != nil {
				return fmt.Errorf("failed to debit wallet: %w", err)
			}
		}

		// Update wallet status to closed
		if err := s.walletRepo.UpdateWalletStatus(ctx, walletID, types.WalletStatusClosed); err != nil {
			return fmt.Errorf("failed to close wallet: %w", err)
		}

		return nil
	})
}

func (s *walletService) UpdateWallet(ctx context.Context, id string, req *dto.UpdateWalletRequest) (*wallet.Wallet, error) {
	if err := req.Validate(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeValidation, "update wallet")
	}

	// Get existing wallet
	existing, err := s.walletRepo.GetWalletByID(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeNotFound, "update wallet")
	}

	// Update fields if provided
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Metadata != nil {
		existing.Metadata = *req.Metadata
	}
	if req.AutoTopupTrigger != nil {
		existing.AutoTopupTrigger = *req.AutoTopupTrigger
	}
	if req.AutoTopupMinBalance != nil {
		existing.AutoTopupMinBalance = *req.AutoTopupMinBalance
	}
	if req.AutoTopupAmount != nil {
		existing.AutoTopupAmount = *req.AutoTopupAmount
	}
	if req.Config != nil {
		existing.Config = *req.Config
	}

	// Update wallet
	if err := s.walletRepo.UpdateWallet(ctx, id, existing); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSystemError, "update wallet")
	}

	return existing, nil
}

// DebitWallet processes a debit operation on a wallet
func (s *walletService) DebitWallet(ctx context.Context, req *wallet.WalletOperation) error {
	if req.Type != types.TransactionTypeDebit {
		return fmt.Errorf("invalid transaction type")
	}

	return s.processWalletOperation(ctx, req)
}

// CreditWallet processes a credit operation on a wallet
func (s *walletService) CreditWallet(ctx context.Context, req *wallet.WalletOperation) error {
	if req.Type != types.TransactionTypeCredit {
		return fmt.Errorf("invalid transaction type")
	}

	return s.processWalletOperation(ctx, req)
}

// Wallet operations

// validateWalletOperation validates the wallet operation request
func (s *walletService) validateWalletOperation(w *wallet.Wallet, req *wallet.WalletOperation) error {
	if req.Type == "" {
		return fmt.Errorf("transaction type is required")
	}

	// Convert amount to credit amount if provided and perform credit operation
	if req.Amount.GreaterThan(decimal.Zero) {
		req.CreditAmount = req.Amount.Div(w.ConversionRate)
	} else if req.CreditAmount.GreaterThan(decimal.Zero) {
		req.Amount = req.CreditAmount.Mul(w.ConversionRate)
	} else {
		return errors.New(errors.ErrCodeInvalidOperation, "amount or credit amount is required")
	}

	if req.CreditAmount.LessThanOrEqual(decimal.Zero) {
		return errors.New(errors.ErrCodeInvalidOperation, "wallet transaction amount must be greater than 0")
	}

	return nil
}

// processDebitOperation handles the debit operation with credit selection and consumption
func (s *walletService) processDebitOperation(ctx context.Context, req *wallet.WalletOperation) error {
	// Find eligible credits with pagination
	credits, err := s.walletRepo.FindEligibleCredits(ctx, req.WalletID, req.CreditAmount, 100)
	if err != nil {
		return err
	}

	// Calculate total available balance
	var totalAvailable decimal.Decimal
	for _, c := range credits {
		totalAvailable = totalAvailable.Add(c.CreditsAvailable)
		if totalAvailable.GreaterThanOrEqual(req.CreditAmount) {
			break
		}
	}

	if totalAvailable.LessThan(req.CreditAmount) {
		return errors.New(errors.ErrCodeInvalidOperation, "insufficient balance")
	}

	// Process debit across credits
	if err := s.walletRepo.ConsumeCredits(ctx, credits, req.CreditAmount); err != nil {
		return err
	}

	return nil
}

// processWalletOperation handles both credit and debit operations
func (s *walletService) processWalletOperation(ctx context.Context, req *wallet.WalletOperation) error {
	s.logger.Debugw("Processing wallet operation", "req", req)

	return s.db.WithTx(ctx, func(ctx context.Context) error {
		// Get wallet
		w, err := s.walletRepo.GetWalletByID(ctx, req.WalletID)
		if err != nil {
			return fmt.Errorf("failed to get wallet: %w", err)
		}

		// Validate operation
		if err := s.validateWalletOperation(w, req); err != nil {
			return err
		}

		var newCreditBalance decimal.Decimal
		// For debit operations, find and consume available credits
		if req.Type == types.TransactionTypeDebit {
			newCreditBalance = w.CreditBalance.Sub(req.CreditAmount)
			// Process debit operation first
			err = s.processDebitOperation(ctx, req)
			if err != nil {
				return err
			}
		} else {
			// Process credit operation
			newCreditBalance = w.CreditBalance.Add(req.CreditAmount)
		}

		finalBalance := newCreditBalance.Mul(w.ConversionRate)

		// Create transaction record
		tx := &wallet.Transaction{
			ID:                  types.GenerateUUIDWithPrefix(types.UUID_PREFIX_WALLET_TRANSACTION),
			WalletID:            req.WalletID,
			Type:                req.Type,
			Amount:              req.Amount,
			CreditAmount:        req.CreditAmount,
			ReferenceType:       req.ReferenceType,
			ReferenceID:         req.ReferenceID,
			Description:         req.Description,
			Metadata:            req.Metadata,
			TxStatus:            types.TransactionStatusCompleted,
			TransactionReason:   req.TransactionReason,
			CreditBalanceBefore: w.CreditBalance,
			CreditBalanceAfter:  newCreditBalance,
			BaseModel:           types.GetDefaultBaseModel(ctx),
		}

		// Set credits available based on transaction type
		if req.Type == types.TransactionTypeCredit {
			tx.CreditsAvailable = req.CreditAmount
		} else {
			tx.CreditsAvailable = decimal.Zero
		}

		if req.Type == types.TransactionTypeCredit && req.ExpiryDate != nil {
			tx.ExpiryDate = parseExpiryDate(req.ExpiryDate)
		}

		if err := s.walletRepo.CreateTransaction(ctx, tx); err != nil {
			return err
		}

		// Update wallet balance
		if err := s.walletRepo.UpdateWalletBalance(ctx, req.WalletID, finalBalance, newCreditBalance); err != nil {
			return err
		}

		s.logger.Debugw("Wallet operation completed")
		return nil
	})
}

// parseExpiryDate converts YYYYMMDD integer to time.Time with end of day time
// for ex 20250101 means the credits will expire on 2025-01-01 00:00:00 UTC
// hence they will be available for use until 2024-12-31 23:59:59 UTC
func parseExpiryDate(expiryDateInt *int) *time.Time {
	if expiryDateInt == nil {
		return nil
	}

	expiryTime := time.Date(
		*expiryDateInt/10000,                   // year
		time.Month((*expiryDateInt%10000)/100), // month
		*expiryDateInt%100,                     // day
		0, 0, 0, 0,                             // Set to end of day
		time.UTC,
	)
	return &expiryTime
}
