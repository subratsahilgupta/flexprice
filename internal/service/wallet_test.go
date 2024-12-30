package service

import (
	"context"
	"testing"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/wallet"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type WalletServiceSuite struct {
	suite.Suite
	ctx           context.Context
	walletService *walletService
	walletRepo    *testutil.InMemoryWalletRepository
	logger        *logger.Logger
	client        postgres.IClient
}

func TestWalletService(t *testing.T) {
	suite.Run(t, new(WalletServiceSuite))
}

func (s *WalletServiceSuite) SetupTest() {
	s.ctx = testutil.SetupContext()
	s.walletRepo = testutil.NewInMemoryWalletStore()

	// Initialize logger with test config
	cfg := &config.Configuration{
		Logging: config.LoggingConfig{
			Level: types.LogLevelDebug,
		},
	}
	var err error
	s.logger, err = logger.NewLogger(cfg)
	if err != nil {
		s.T().Fatalf("failed to create logger: %v", err)
	}

	// Initialize mock postgres client
	s.client = testutil.NewMockPostgresClient(s.logger)

	s.walletService = &walletService{
		walletRepo: s.walletRepo,
		logger:     s.logger,
		client:     s.client,
	}
}

func (s *WalletServiceSuite) TestCreateWallet() {
	req := &dto.CreateWalletRequest{
		CustomerID: "customer-1",
		Currency:   "USD",
		Metadata:   types.Metadata{"key": "value"},
	}

	resp, err := s.walletService.CreateWallet(s.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(req.CustomerID, resp.CustomerID)
	s.Equal(req.Currency, resp.Currency)
	s.Equal(decimal.Zero, resp.Balance)
}

func (s *WalletServiceSuite) TestGetWalletByID() {
	w := &wallet.Wallet{
		ID:           "wallet-1",
		CustomerID:   "customer-1",
		Currency:     "USD",
		Balance:      decimal.NewFromInt(1000),
		WalletStatus: types.WalletStatusActive,
	}
	_ = s.walletRepo.CreateWallet(s.ctx, w)

	resp, err := s.walletService.GetWalletByID(s.ctx, "wallet-1")
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(w.CustomerID, resp.CustomerID)
	s.Equal(w.Currency, resp.Currency)
	s.Equal(w.Balance, resp.Balance)
}

func (s *WalletServiceSuite) TestGetWalletsByCustomerID() {
	_ = s.walletRepo.CreateWallet(s.ctx, &wallet.Wallet{ID: "wallet-1", CustomerID: "customer-1", Currency: "USD", Balance: decimal.NewFromInt(1000)})
	_ = s.walletRepo.CreateWallet(s.ctx, &wallet.Wallet{ID: "wallet-2", CustomerID: "customer-1", Currency: "EUR", Balance: decimal.NewFromInt(500)})

	resp, err := s.walletService.GetWalletsByCustomerID(s.ctx, "customer-1")
	s.NoError(err)
	s.Len(resp, 2)
}

func (s *WalletServiceSuite) TestTopUpWallet() {
	w := &wallet.Wallet{
		ID:           "wallet-1",
		CustomerID:   "customer-1",
		Currency:     "USD",
		Balance:      decimal.NewFromInt(1000),
		WalletStatus: types.WalletStatusActive,
	}
	_ = s.walletRepo.CreateWallet(s.ctx, w)

	topUpReq := &dto.TopUpWalletRequest{
		Amount: decimal.NewFromInt(500),
	}
	resp, err := s.walletService.TopUpWallet(s.ctx, "wallet-1", topUpReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(decimal.NewFromInt(1500), resp.Balance)
}

func (s *WalletServiceSuite) TestTerminateWallet() {
	// Create a wallet
	w := &wallet.Wallet{
		ID:           "wallet-1",
		CustomerID:   "customer-1",
		Currency:     "USD",
		Balance:      decimal.NewFromInt(100),
		WalletStatus: types.WalletStatusActive,
	}
	_ = s.walletRepo.CreateWallet(s.ctx, w)

	// Terminate the wallet
	err := s.walletService.TerminateWallet(s.ctx, "wallet-1")
	s.NoError(err)

	// Verify the wallet status
	updatedWallet, _ := s.walletRepo.GetWalletByID(s.ctx, "wallet-1")
	s.Equal(types.WalletStatusClosed, updatedWallet.WalletStatus)
	s.Equal(decimal.NewFromInt(0).Equal(updatedWallet.Balance), true)

	// Verify transaction creation
	transactions, _ := s.walletRepo.GetTransactionsByWalletID(s.ctx, "wallet-1", 10, 0)
	s.Len(transactions, 1)
	s.Equal(types.TransactionTypeDebit, transactions[0].Type)
	s.Equal(decimal.NewFromInt(100).Equal(transactions[0].Amount), true)
}
