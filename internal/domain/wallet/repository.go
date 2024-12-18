package wallet

import (
	"context"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// Repository defines the interface for wallet operations
type Repository interface {
	// Wallet operations
	GetWalletByID(ctx context.Context, id string) (*Wallet, error)
	GetWalletsByCustomerID(ctx context.Context, customerID string) ([]*Wallet, error)
	UpdateWalletStatus(ctx context.Context, id string, status types.WalletStatus) error

	// Transaction operations
	GetTransactionByID(ctx context.Context, id string) (*Transaction, error)
	GetTransactionsByWalletID(ctx context.Context, walletID string, limit, offset int) ([]*Transaction, error)
	UpdateTransactionStatus(ctx context.Context, id string, status types.TransactionStatus) error

	// Atomic operations that require transaction support
	CreateWallet(ctx context.Context, w *Wallet) error
	CreditWallet(ctx context.Context, req *WalletOperation) error
	DebitWallet(ctx context.Context, req *WalletOperation) error
}

// WalletOperation represents the request to credit or debit a wallet
type WalletOperation struct {
	WalletID      string
	Type          types.TransactionType
	Amount        decimal.Decimal
	ReferenceType string
	ReferenceID   string
	Description   string
	Metadata      map[string]string
}
