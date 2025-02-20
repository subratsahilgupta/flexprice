package testutil

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/domain/wallet"
	"github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

type InMemoryWalletStore struct {
	wallets      *InMemoryStore[*wallet.Wallet]
	transactions *InMemoryStore[*wallet.Transaction]
}

func NewInMemoryWalletStore() *InMemoryWalletStore {
	return &InMemoryWalletStore{
		wallets:      NewInMemoryStore[*wallet.Wallet](),
		transactions: NewInMemoryStore[*wallet.Transaction](),
	}
}

// walletFilterFn implements filtering logic for wallets
func walletFilterFn(ctx context.Context, w *wallet.Wallet, filter interface{}) bool {
	if w == nil {
		return false
	}

	// Check tenant ID
	if tenantID, ok := ctx.Value(types.CtxTenantID).(string); ok {
		if w.TenantID != tenantID {
			return false
		}
	}

	// Check wallet status
	if w.Status != types.StatusPublished {
		return false
	}

	// Check wallet status is active
	if w.WalletStatus != types.WalletStatusActive {
		return false
	}

	return true
}

// transactionFilterFn implements filtering logic for transactions
func transactionFilterFn(ctx context.Context, t *wallet.Transaction, filter interface{}) bool {
	if t == nil {
		return false
	}

	f, ok := filter.(*types.WalletTransactionFilter)
	if !ok {
		return true // No filter applied
	}

	// Check tenant ID
	if tenantID, ok := ctx.Value(types.CtxTenantID).(string); ok {
		if t.TenantID != tenantID {
			return false
		}
	}

	// Filter by status
	if f.Status != nil && t.Status != *f.Status {
		return false
	}

	// Filter by wallet ID
	if f.WalletID != nil && t.WalletID != *f.WalletID {
		return false
	}

	// Filter by transaction type
	if f.Type != nil && t.Type != *f.Type {
		return false
	}

	// Filter by transaction status
	if f.TransactionStatus != nil && t.TxStatus != *f.TransactionStatus {
		return false
	}

	// Filter by reference type and ID
	if f.ReferenceType != nil && f.ReferenceID != nil {
		if t.ReferenceType != *f.ReferenceType || t.ReferenceID != *f.ReferenceID {
			return false
		}
	}

	// Filter by time range
	if f.TimeRangeFilter != nil {
		if f.StartTime != nil && t.CreatedAt.Before(*f.StartTime) {
			return false
		}
		if f.EndTime != nil && t.CreatedAt.After(*f.EndTime) {
			return false
		}
	}

	if f.AmountUsedLessThan != nil && t.AmountUsed.GreaterThan(*f.AmountUsedLessThan) {
		return false
	}

	if f.ExpiryDateBefore != nil && t.ExpiryDate != nil && t.ExpiryDate.After(*f.ExpiryDateBefore) {
		return false
	}

	if f.ExpiryDateAfter != nil && t.ExpiryDate != nil && t.ExpiryDate.Before(*f.ExpiryDateAfter) {
		return false
	}

	if f.TransactionReason != nil && t.TransactionReason != *f.TransactionReason {
		return false
	}

	return true
}

// transactionSortFn implements sorting logic for transactions
func transactionSortFn(i, j *wallet.Transaction) bool {
	if i == nil || j == nil {
		return false
	}
	return i.CreatedAt.After(j.CreatedAt)
}

func (s *InMemoryWalletStore) CreateWallet(ctx context.Context, w *wallet.Wallet) error {
	if w == nil {
		return fmt.Errorf("wallet cannot be nil")
	}

	// Set default status if not set
	if w.Status == "" {
		w.Status = types.StatusPublished
	}
	if w.WalletStatus == "" {
		w.WalletStatus = types.WalletStatusActive
	}

	return s.wallets.Create(ctx, w.ID, w)
}

func (s *InMemoryWalletStore) GetWalletByID(ctx context.Context, id string) (*wallet.Wallet, error) {
	return s.wallets.Get(ctx, id)
}

func (s *InMemoryWalletStore) GetWalletsByCustomerID(ctx context.Context, customerID string) ([]*wallet.Wallet, error) {
	// Create a filter function that checks customer ID
	filterFn := func(ctx context.Context, w *wallet.Wallet, filter interface{}) bool {
		if !walletFilterFn(ctx, w, filter) {
			return false
		}
		return w.CustomerID == customerID
	}

	// List all wallets with the customer ID filter
	return s.wallets.List(ctx, nil, filterFn, nil)
}

func (s *InMemoryWalletStore) UpdateWalletStatus(ctx context.Context, id string, status types.WalletStatus) error {
	w, err := s.GetWalletByID(ctx, id)
	if err != nil {
		return err
	}

	w.WalletStatus = status
	return s.wallets.Update(ctx, id, w)
}

func (s *InMemoryWalletStore) CreditWallet(ctx context.Context, req *wallet.WalletOperation) error {
	if req.Type != types.TransactionTypeCredit {
		return fmt.Errorf("invalid transaction type")
	}

	if req.CreditAmount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("amount must be greater than 0")
	}

	return s.performWalletOperation(ctx, req)
}

func (s *InMemoryWalletStore) DebitWallet(ctx context.Context, req *wallet.WalletOperation) error {
	if req.Type != types.TransactionTypeDebit {
		return fmt.Errorf("invalid transaction type")
	}

	if req.CreditAmount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("amount must be greater than 0")
	}
	return s.performWalletOperation(ctx, req)
}

func (s *InMemoryWalletStore) performWalletOperation(ctx context.Context, req *wallet.WalletOperation) error {
	w, err := s.GetWalletByID(ctx, req.WalletID)
	if err != nil {
		return err
	}

	// Convert amount to credit amount if provided and perform credit operation
	if req.Amount.GreaterThan(decimal.Zero) {
		req.CreditAmount = req.Amount.Div(w.ConversionRate)
	} else if req.CreditAmount.GreaterThan(decimal.Zero) {
		req.Amount = req.CreditAmount.Mul(w.ConversionRate)
	} else {
		return errors.New(errors.ErrCodeInvalidOperation, "amount or credit amount is required")
	}

	// Calculate new balance
	var newCreditBalance decimal.Decimal
	if req.Type == types.TransactionTypeCredit {
		newCreditBalance = w.CreditBalance.Add(req.CreditAmount)
	} else if req.Type == types.TransactionTypeDebit {
		newCreditBalance = w.CreditBalance.Sub(req.CreditAmount)
		if newCreditBalance.LessThan(decimal.Zero) {
			return fmt.Errorf("insufficient balance: current=%s, requested=%s", w.CreditBalance, req.CreditAmount)
		}
	} else {
		return fmt.Errorf("invalid transaction type")
	}

	// final balance
	finalBalance := newCreditBalance.Mul(w.ConversionRate)

	// Create a transaction
	txn := &wallet.Transaction{
		ID:                  fmt.Sprintf("txn-%s-%d", req.WalletID, time.Now().UnixNano()),
		WalletID:            req.WalletID,
		Type:                req.Type,
		Amount:              req.Amount,
		CreditAmount:        req.CreditAmount,
		BalanceBefore:       w.Balance,
		BalanceAfter:        finalBalance,
		CreditBalanceBefore: w.CreditBalance,
		CreditBalanceAfter:  newCreditBalance,
		TxStatus:            types.TransactionStatusCompleted,
		Description:         req.Description,
		Metadata:            req.Metadata,
		BaseModel:           types.GetDefaultBaseModel(ctx),
	}

	if err := s.transactions.Create(ctx, txn.ID, txn); err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	w.Balance = finalBalance
	w.CreditBalance = newCreditBalance
	return s.wallets.Update(ctx, w.ID, w)
}

func (s *InMemoryWalletStore) GetTransactionByID(ctx context.Context, id string) (*wallet.Transaction, error) {
	return s.transactions.Get(ctx, id)
}

func (s *InMemoryWalletStore) ListWalletTransactions(ctx context.Context, f *types.WalletTransactionFilter) ([]*wallet.Transaction, error) {
	return s.transactions.List(ctx, f, transactionFilterFn, transactionSortFn)
}

func (s *InMemoryWalletStore) ListAllWalletTransactions(ctx context.Context, f *types.WalletTransactionFilter) ([]*wallet.Transaction, error) {
	// Create a copy of the filter without pagination
	filterCopy := *f
	filterCopy.QueryFilter.Limit = nil
	return s.transactions.List(ctx, &filterCopy, transactionFilterFn, transactionSortFn)
}

func (s *InMemoryWalletStore) CountWalletTransactions(ctx context.Context, f *types.WalletTransactionFilter) (int, error) {
	return s.transactions.Count(ctx, f, transactionFilterFn)
}

func (s *InMemoryWalletStore) UpdateTransactionStatus(ctx context.Context, id string, status types.TransactionStatus) error {
	txn, err := s.GetTransactionByID(ctx, id)
	if err != nil {
		return err
	}

	txn.TxStatus = status
	return s.transactions.Update(ctx, id, txn)
}

func (s *InMemoryWalletStore) Clear() {
	s.wallets.Clear()
	s.transactions.Clear()
}

// UpdateWallet updates a wallet in the in-memory store
func (s *InMemoryWalletStore) UpdateWallet(ctx context.Context, id string, w *wallet.Wallet) error {
	// Check if wallet exists and belongs to tenant
	existing, err := s.wallets.Get(ctx, id)
	if err != nil || existing.TenantID != types.GetTenantID(ctx) || existing.Status != types.StatusPublished {
		return errors.New(errors.ErrCodeNotFound, "wallet not found")
	}

	// Update fields if provided
	if w.Name != "" {
		existing.Name = w.Name
	}
	if w.Description != "" {
		existing.Description = w.Description
	}
	if w.Metadata != nil {
		existing.Metadata = w.Metadata
	}
	if w.AutoTopupTrigger != "" {
		existing.AutoTopupTrigger = w.AutoTopupTrigger
	}
	if !w.AutoTopupMinBalance.IsZero() {
		existing.AutoTopupMinBalance = w.AutoTopupMinBalance
	}
	if !w.AutoTopupAmount.IsZero() {
		existing.AutoTopupAmount = w.AutoTopupAmount
	}

	// Update metadata
	existing.UpdatedBy = types.GetUserID(ctx)
	existing.UpdatedAt = time.Now().UTC()

	// Save back to store
	if err := s.wallets.Update(ctx, id, existing); err != nil {
		return errors.Wrap(err, errors.ErrCodeSystemError, "failed to update wallet")
	}
	*w = *existing

	return nil
}
