package postgres

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/internal/domain/wallet"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

type walletRepository struct {
	db     *postgres.DB
	logger *logger.Logger
}

// NewWalletRepository creates a new instance of wallet repository
func NewWalletRepository(db *postgres.DB, logger *logger.Logger) wallet.Repository {
	return &walletRepository{
		db:     db,
		logger: logger,
	}
}

// GetWalletByID retrieves a wallet by its ID
func (r *walletRepository) GetWalletByID(ctx context.Context, id string) (*wallet.Wallet, error) {
	query := `
		SELECT * FROM wallets
		WHERE id = :id 
		AND tenant_id = :tenant_id
		AND status = :status`

	params := map[string]interface{}{
		"id":        id,
		"tenant_id": types.GetTenantID(ctx),
		"status":    types.StatusPublished,
	}

	r.logger.Debug("getting wallet by id",
		"wallet_id", id,
		"tenant_id", types.GetTenantID(ctx),
	)

	// Use NamedQueryContext for named queries
	rows, err := r.db.NamedQueryContext(ctx, query, params)
	if err != nil {
		return nil, fmt.Errorf("failed to query wallet: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("wallet not found")
	}

	var w wallet.Wallet
	if err := rows.StructScan(&w); err != nil {
		return nil, fmt.Errorf("failed to scan wallet: %w", err)
	}
	return &w, nil
}

// GetWalletsByCustomerID retrieves all wallets for a customer
func (r *walletRepository) GetWalletsByCustomerID(ctx context.Context, customerID string) ([]*wallet.Wallet, error) {
	query := `
		SELECT * FROM wallets
		WHERE customer_id = :customer_id 
		AND wallet_status = :wallet_status
		AND tenant_id = :tenant_id
		AND status = :status`

	params := map[string]interface{}{
		"customer_id":   customerID,
		"wallet_status": types.WalletStatusActive,
		"tenant_id":     types.GetTenantID(ctx),
		"status":        types.StatusPublished,
	}

	r.logger.Debugw("getting active wallets by customer id",
		"customer_id", customerID,
		"tenant_id", types.GetTenantID(ctx),
	)

	rows, err := r.db.NamedQueryContext(ctx, query, params)
	if err != nil {
		return nil, fmt.Errorf("failed to query wallets: %w", err)
	}
	defer rows.Close()

	var wallets []*wallet.Wallet
	for rows.Next() {
		var w wallet.Wallet
		if err := rows.StructScan(&w); err != nil {
			return nil, fmt.Errorf("failed to scan wallet: %w", err)
		}
		wallets = append(wallets, &w)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating wallet rows: %w", err)
	}

	return wallets, nil
}

// UpdateWalletStatus updates the status of a wallet
func (r *walletRepository) UpdateWalletStatus(ctx context.Context, id string, status types.WalletStatus) error {
	query := `
		UPDATE wallets
		SET 
			wallet_status = :wallet_status,
			updated_at = NOW(),
			updated_by = :updated_by
		WHERE id = :id 
		AND tenant_id = :tenant_id
		AND status = :status`

	params := map[string]interface{}{
		"id":            id,
		"wallet_status": status,
		"updated_by":    types.GetUserID(ctx),
		"tenant_id":     types.GetTenantID(ctx),
		"status":        types.StatusPublished,
	}

	r.logger.Debug("updating wallet status",
		"wallet_id", id,
		"tenant_id", types.GetTenantID(ctx),
		"status", status,
	)

	// Use NamedExecContext for named queries
	result, err := r.db.NamedExecContext(ctx, query, params)
	if err != nil {
		return fmt.Errorf("failed to update wallet status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("wallet not found or already updated")
	}

	return nil
}

// GetTransactionByID retrieves a transaction by its ID
func (r *walletRepository) GetTransactionByID(ctx context.Context, id string) (*wallet.Transaction, error) {
	query := `
		SELECT * FROM wallet_transactions
		WHERE id = :id 
		AND tenant_id = :tenant_id
		AND status = :status`

	params := map[string]interface{}{
		"id":        id,
		"tenant_id": types.GetTenantID(ctx),
		"status":    types.StatusPublished,
	}

	r.logger.Debug("getting transaction by id",
		"transaction_id", id,
		"tenant_id", types.GetTenantID(ctx),
	)

	rows, err := r.db.NamedQueryContext(ctx, query, params)
	if err != nil {
		return nil, fmt.Errorf("failed to query transaction: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("transaction not found")
	}

	var tx wallet.Transaction
	if err := rows.StructScan(&tx); err != nil {
		return nil, fmt.Errorf("failed to scan transaction: %w", err)
	}
	return &tx, nil
}

// GetTransactionsByWalletID retrieves transactions for a wallet with pagination
func (r *walletRepository) GetTransactionsByWalletID(ctx context.Context, walletID string, limit, offset int) ([]*wallet.Transaction, error) {
	query := `
		SELECT * FROM wallet_transactions
		WHERE wallet_id = :wallet_id 
		AND tenant_id = :tenant_id
		AND status = :status
		ORDER BY created_at DESC
		LIMIT :limit OFFSET :offset`

	params := map[string]interface{}{
		"wallet_id": walletID,
		"tenant_id": types.GetTenantID(ctx),
		"status":    types.StatusPublished,
		"limit":     limit,
		"offset":    offset,
	}

	r.logger.Debug("getting transactions by wallet id",
		"wallet_id", walletID,
		"tenant_id", types.GetTenantID(ctx),
		"limit", limit,
		"offset", offset,
	)

	rows, err := r.db.NamedQueryContext(ctx, query, params)
	if err != nil {
		return nil, fmt.Errorf("failed to query transactions: %w", err)
	}
	defer rows.Close()

	var transactions []*wallet.Transaction
	for rows.Next() {
		var tx wallet.Transaction
		if err := rows.StructScan(&tx); err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}
		transactions = append(transactions, &tx)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transaction rows: %w", err)
	}

	return transactions, nil
}

// UpdateTransactionStatus updates the status of a transaction
func (r *walletRepository) UpdateTransactionStatus(ctx context.Context, id string, status types.TransactionStatus) error {
	query := `
		UPDATE wallet_transactions
		SET 
			transaction_status = :transaction_status,
			updated_at = NOW(),
			updated_by = :updated_by
		WHERE id = :id 
		AND tenant_id = :tenant_id
		AND status = :status`

	params := map[string]interface{}{
		"id":                 id,
		"transaction_status": status,
		"updated_by":         types.GetUserID(ctx),
		"tenant_id":          types.GetTenantID(ctx),
		"status":             types.StatusPublished,
	}

	r.logger.Debug("updating transaction status",
		"transaction_id", id,
		"tenant_id", types.GetTenantID(ctx),
		"status", status,
	)

	result, err := r.db.NamedExecContext(ctx, query, params)
	if err != nil {
		return fmt.Errorf("failed to update transaction status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("transaction not found")
	}

	return nil
}

// CreateWallet creates a new wallet
func (r *walletRepository) CreateWallet(ctx context.Context, w *wallet.Wallet) error {
	query := `
		INSERT INTO wallets (
			id, customer_id, currency, balance, wallet_status, metadata, tenant_id, status, created_at, updated_at, created_by, updated_by
		) VALUES (
			:id, :customer_id, :currency, :balance, :wallet_status, :metadata, :tenant_id, :status, :created_at, :updated_at, :created_by, :updated_by
		) RETURNING id, customer_id, currency, balance, wallet_status, metadata, tenant_id, status, created_at, updated_at, created_by, updated_by`

	rows, err := r.db.NamedQueryContext(ctx, query, w)
	if err != nil {
		return fmt.Errorf("failed to create wallet: %w", err)
	}
	defer rows.Close()

	// Scan the returned row into the wallet object
	if rows.Next() {
		if err := rows.StructScan(w); err != nil {
			return fmt.Errorf("failed to scan wallet: %w", err)
		}
	}

	return nil
}

// processWalletOperation handles both credit and debit operations within a transaction
func (r *walletRepository) processWalletOperation(ctx context.Context, req *wallet.WalletOperation) error {
	return r.db.WithTx(ctx, func(ctx context.Context) error {
		// Get current wallet balance with row lock
		query := `
			SELECT balance FROM wallets
			WHERE id = :id 
			AND tenant_id = :tenant_id
			AND status = :status
			AND wallet_status = :wallet_status
			FOR UPDATE`

		params := map[string]interface{}{
			"id":            req.WalletID,
			"tenant_id":     types.GetTenantID(ctx),
			"status":        types.StatusPublished,
			"wallet_status": types.WalletStatusActive, // updating only active wallets
		}

		r.logger.Debug("getting wallet balance for update",
			"wallet_id", req.WalletID,
			"tenant_id", types.GetTenantID(ctx),
		)

		var currentBalance decimal.Decimal
		rows, err := r.db.NamedQueryContext(ctx, query, params)
		if err != nil {
			return fmt.Errorf("failed to query wallet balance: %w", err)
		}
		defer rows.Close()

		if !rows.Next() {
			return fmt.Errorf("no active wallet found")
		}

		if err := rows.Scan(&currentBalance); err != nil {
			return fmt.Errorf("failed to scan balance: %w", err)
		}

		// Calculate new balance
		newBalance := currentBalance.Add(req.Amount)

		// Check if debit would result in negative balance
		if req.Type == types.TransactionTypeDebit && newBalance.LessThan(decimal.Zero) {
			return fmt.Errorf("insufficient balance")
		}

		// Update wallet balance
		updateQuery := `
			UPDATE wallets
			SET 
				balance = :balance,
				updated_at = NOW(),
				updated_by = :updated_by
			WHERE id = :id 
			AND tenant_id = :tenant_id
			AND status = :status`

		updateParams := map[string]interface{}{
			"id":         req.WalletID,
			"tenant_id":  types.GetTenantID(ctx),
			"balance":    newBalance,
			"updated_by": types.GetUserID(ctx),
			"status":     types.StatusPublished,
		}

		r.logger.Debug("updating wallet balance",
			"wallet_id", req.WalletID,
			"tenant_id", types.GetTenantID(ctx),
			"balance", newBalance,
		)

		result, err := r.db.NamedExecContext(ctx, updateQuery, updateParams)
		if err != nil {
			return fmt.Errorf("failed to update wallet balance: %w", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected: %w", err)
		}

		if rowsAffected == 0 {
			return fmt.Errorf("wallet not found or already updated")
		}

		// Create transaction record
		txQuery := `
			INSERT INTO wallet_transactions (
				tenant_id, wallet_id, type, amount, balance_before, balance_after,
				transaction_status, reference_type, reference_id, description, metadata,
				status, created_at, updated_at, created_by, updated_by
			) VALUES (
				:tenant_id, :wallet_id, :type, :amount, :balance_before, :balance_after,
				:transaction_status, :reference_type, :reference_id, :description, :metadata,
				:status, NOW(), NOW(), :created_by, :updated_by
			)`

		txParams := map[string]interface{}{
			"tenant_id":          types.GetTenantID(ctx),
			"wallet_id":          req.WalletID,
			"type":               req.Type,
			"amount":             req.Amount.Abs(), // Store absolute amount
			"balance_before":     currentBalance,
			"balance_after":      newBalance,
			"transaction_status": types.TransactionStatusCompleted,
			"reference_type":     req.ReferenceType,
			"reference_id":       req.ReferenceID,
			"description":        req.Description,
			"metadata":           types.Metadata(req.Metadata),
			"status":             types.StatusPublished,
			"created_by":         types.GetUserID(ctx),
			"updated_by":         types.GetUserID(ctx),
		}

		r.logger.Debug("creating wallet transaction",
			"wallet_id", req.WalletID,
			"tenant_id", types.GetTenantID(ctx),
			"type", req.Type,
			"amount", req.Amount,
			"balance_before", currentBalance,
			"balance_after", newBalance,
		)

		result, err = r.db.NamedExecContext(ctx, txQuery, txParams)
		if err != nil {
			return fmt.Errorf("failed to create transaction record: %w", err)
		}

		rowsAffected, err = result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected: %w", err)
		}

		if rowsAffected == 0 {
			return fmt.Errorf("failed to create transaction record")
		}

		return nil
	})
}

// CreditWallet credits a wallet with the specified amount
func (r *walletRepository) CreditWallet(ctx context.Context, req *wallet.WalletOperation) error {
	if req.Type != types.TransactionTypeCredit {
		return fmt.Errorf("invalid transaction type")
	}

	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("amount must be greater than 0")
	}

	return r.processWalletOperation(ctx, req)
}

// DebitWallet debits amount from wallet
func (r *walletRepository) DebitWallet(ctx context.Context, req *wallet.WalletOperation) error {
	if req.Type != types.TransactionTypeDebit {
		return fmt.Errorf("invalid transaction type")
	}

	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("amount must be greater than 0")
	}

	// For debit operations, make the amount negative
	req.Amount = req.Amount.Neg()
	return r.processWalletOperation(ctx, req)
}
