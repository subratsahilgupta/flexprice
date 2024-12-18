package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/domain/wallet"
	"github.com/flexprice/flexprice/internal/kafka"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
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
	GetWalletTransactions(ctx context.Context, walletID string, filter types.Filter) (*dto.WalletTransactionsResponse, error)

	// TopUpWallet adds credits to a wallet
	TopUpWallet(ctx context.Context, walletID string, req *dto.TopUpWalletRequest) (*dto.WalletResponse, error)

	// GetWalletBalance retrieves the real-time balance of a wallet
	GetWalletBalance(ctx context.Context, walletID string) (*dto.WalletBalanceResponse, error)
}

type walletService struct {
	walletRepo       wallet.Repository
	logger           *logger.Logger
	subscriptionRepo subscription.Repository
	planRepo         plan.Repository
	priceRepo        price.Repository
	producer         kafka.MessageProducer
	eventRepo        events.Repository
	meterRepo        meter.Repository
	customerRepo     customer.Repository
}

// NewWalletService creates a new instance of WalletService
func NewWalletService(
	walletRepo wallet.Repository,
	logger *logger.Logger,
	subscriptionRepo subscription.Repository,
	planRepo plan.Repository,
	priceRepo price.Repository,
	producer kafka.MessageProducer,
	eventRepo events.Repository,
	meterRepo meter.Repository,
	customerRepo customer.Repository,
) WalletService {
	return &walletService{
		walletRepo:       walletRepo,
		logger:           logger,
		subscriptionRepo: subscriptionRepo,
		planRepo:         planRepo,
		priceRepo:        priceRepo,
		producer:         producer,
		eventRepo:        eventRepo,
		meterRepo:        meterRepo,
		customerRepo:     customerRepo,
	}
}

func (s *walletService) CreateWallet(ctx context.Context, req *dto.CreateWalletRequest) (*dto.WalletResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
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
	return &dto.WalletResponse{
		ID:           w.ID,
		CustomerID:   w.CustomerID,
		Currency:     w.Currency,
		Balance:      w.Balance,
		WalletStatus: w.WalletStatus,
		Metadata:     w.Metadata,
		CreatedAt:    w.CreatedAt,
		UpdatedAt:    w.UpdatedAt,
	}, nil
}

func (s *walletService) GetWalletsByCustomerID(ctx context.Context, customerID string) ([]*dto.WalletResponse, error) {
	wallets, err := s.walletRepo.GetWalletsByCustomerID(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallets: %w", err)
	}

	response := make([]*dto.WalletResponse, len(wallets))
	for i, w := range wallets {
		response[i] = &dto.WalletResponse{
			ID:           w.ID,
			CustomerID:   w.CustomerID,
			Currency:     w.Currency,
			Balance:      w.Balance,
			WalletStatus: w.WalletStatus,
			Metadata:     w.Metadata,
			CreatedAt:    w.CreatedAt,
			UpdatedAt:    w.UpdatedAt,
		}
	}

	return response, nil
}

func (s *walletService) GetWalletByID(ctx context.Context, id string) (*dto.WalletResponse, error) {
	w, err := s.walletRepo.GetWalletByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	return &dto.WalletResponse{
		ID:           w.ID,
		CustomerID:   w.CustomerID,
		Currency:     w.Currency,
		Balance:      w.Balance,
		WalletStatus: w.WalletStatus,
		Metadata:     w.Metadata,
		CreatedAt:    w.CreatedAt,
		UpdatedAt:    w.UpdatedAt,
	}, nil
}

func (s *walletService) GetWalletTransactions(ctx context.Context, walletID string, filter types.Filter) (*dto.WalletTransactionsResponse, error) {
	transactions, err := s.walletRepo.GetTransactionsByWalletID(ctx, walletID, filter.Limit, filter.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get transactions: %w", err)
	}

	response := &dto.WalletTransactionsResponse{
		Transactions: make([]*dto.WalletTransactionResponse, len(transactions)),
		Filter:       filter,
		// TODO: Add total count from repository
	}

	for i, txn := range transactions {
		response.Transactions[i] = &dto.WalletTransactionResponse{
			ID:                txn.ID,
			WalletID:          txn.WalletID,
			Type:              string(txn.Type),
			Amount:            txn.Amount,
			BalanceBefore:     txn.BalanceBefore,
			BalanceAfter:      txn.BalanceAfter,
			TransactionStatus: txn.TxStatus,
			ReferenceType:     txn.ReferenceType,
			ReferenceID:       txn.ReferenceID,
			Description:       txn.Description,
			Metadata:          txn.Metadata,
			CreatedAt:         txn.CreatedAt,
		}
	}

	return response, nil
}

func (s *walletService) TopUpWallet(ctx context.Context, walletID string, req *dto.TopUpWalletRequest) (*dto.WalletResponse, error) {
	// Create a credit operation
	creditReq := &wallet.WalletOperation{
		WalletID:    walletID,
		Type:        types.TransactionTypeCredit,
		Amount:      req.Amount,
		Description: req.Description,
		Metadata:    req.Metadata,
	}

	if err := s.walletRepo.CreditWallet(ctx, creditReq); err != nil {
		return nil, fmt.Errorf("failed to credit wallet: %w", err)
	}

	// Get updated wallet
	return s.GetWalletByID(ctx, walletID)
}

func (s *walletService) GetWalletBalance(ctx context.Context, walletID string) (*dto.WalletBalanceResponse, error) {
	w, err := s.walletRepo.GetWalletByID(ctx, walletID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}

	subscriptionService := NewSubscriptionService(
		s.subscriptionRepo,
		s.planRepo,
		s.priceRepo,
		s.producer,
		s.eventRepo,
		s.meterRepo,
		s.customerRepo,
		s.logger,
	)

	filter := &types.SubscriptionFilter{
		CustomerID:         w.CustomerID,
		Status:             types.StatusPublished,
		SubscriptionStatus: types.SubscriptionStatusActive,
	}

	subscriptionsResp, err := subscriptionService.ListSubscriptions(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscriptions: %w", err)
	}

	totalPendingCharges := decimal.Zero
	for _, sub := range subscriptionsResp.Subscriptions {
		usageResp, err := subscriptionService.GetUsageBySubscription(ctx, &dto.GetUsageBySubscriptionRequest{
			SubscriptionID: sub.Subscription.ID,
			StartTime:      sub.Subscription.CurrentPeriodStart,
			EndTime:        time.Now().UTC(),
		})
		if err != nil {
			s.logger.Errorw("failed to get subscription usage",
				"subscription_id", sub.Subscription.ID,
				"error", err,
			)
			continue
		}

		if usageResp.Amount > 0 {
			totalPendingCharges = totalPendingCharges.Add(decimal.NewFromFloat(usageResp.Amount))
		}
	}

	realTimeBalance := w.Balance.Sub(totalPendingCharges)

	s.logger.Debugw("calculated real-time balance",
		"wallet_id", walletID,
		"current_balance", w.Balance,
		"total_pending_charges", totalPendingCharges,
		"real_time_balance", realTimeBalance,
	)

	return &dto.WalletBalanceResponse{
		RealTimeBalance:  realTimeBalance,
		BalanceUpdatedAt: time.Now().UTC(),
		Wallet:           w,
	}, nil
}
