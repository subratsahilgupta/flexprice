package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// TxKey is the context key type for storing transaction
// Using a custom type instead of string prevents collisions in the context
type TxKey struct{}

// Tx extends sqlx.Tx to support nested transactions using savepoints
type Tx struct {
	*sqlx.Tx
	savepointID int
}

// GetTx retrieves a transaction from the context if it exists
// Returns the transaction and a boolean indicating if it was found
func GetTx(ctx context.Context) (*Tx, bool) {
	tx, ok := ctx.Value(TxKey{}).(*Tx)
	return tx, ok
}

// BeginTx starts a new transaction or creates a savepoint for nested transactions
//
// For top-level transactions:
// - Creates a new transaction with READ COMMITTED isolation level
// - Stores the transaction in the context
//
// For nested transactions:
// - Creates a savepoint within the existing transaction
// - Increments the savepoint ID counter
//
// Returns:
// - Updated context containing the transaction
// - Pointer to the transaction
// - Error if transaction/savepoint creation fails
func (db *DB) BeginTx(ctx context.Context) (context.Context, *Tx, error) {
	// Check if there's already a transaction in the context
	if tx, ok := GetTx(ctx); ok {
		// Create a new savepoint for nested transaction
		tx.savepointID++
		savepoint := fmt.Sprintf("sp_%d", tx.savepointID)
		_, err := tx.ExecContext(ctx, fmt.Sprintf("SAVEPOINT %s", savepoint))
		if err != nil {
			return ctx, nil, fmt.Errorf("failed to create savepoint: %w", err)
		}
		return ctx, tx, nil
	}

	// Start a new transaction with READ COMMITTED isolation level
	// This prevents dirty reads while allowing for better concurrency
	// compared to SERIALIZABLE isolation
	sqlxTx, err := db.BeginTxx(ctx, &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
		ReadOnly:  false,
	})
	if err != nil {
		return ctx, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	tx := &Tx{Tx: sqlxTx}
	ctx = context.WithValue(ctx, TxKey{}, tx)
	return ctx, tx, nil
}

// CommitTx commits the current transaction level
//
// For top-level transactions:
// - Commits the entire transaction to the database
//
// For nested transactions:
// - Releases the current savepoint
// - Decrements the savepoint ID counter
//
// Returns error if commit/release fails
func (db *DB) CommitTx(ctx context.Context) error {
	tx, ok := GetTx(ctx)
	if !ok {
		return fmt.Errorf("no transaction found in context")
	}

	if tx.savepointID > 0 {
		// Release the savepoint for nested transaction
		savepoint := fmt.Sprintf("sp_%d", tx.savepointID)
		_, err := tx.ExecContext(ctx, fmt.Sprintf("RELEASE SAVEPOINT %s", savepoint))
		tx.savepointID--
		if err != nil {
			return fmt.Errorf("failed to release savepoint: %w", err)
		}
		return nil
	}

	// Commit the main transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// RollbackTx rolls back the current transaction level
//
// For top-level transactions:
// - Rolls back the entire transaction
//
// For nested transactions:
// - Rolls back to the current savepoint
// - Decrements the savepoint ID counter
//
// Returns error if rollback fails
func (db *DB) RollbackTx(ctx context.Context) error {
	tx, ok := GetTx(ctx)
	if !ok {
		return fmt.Errorf("no transaction found in context")
	}

	if tx.savepointID > 0 {
		// Rollback to the savepoint for nested transaction
		savepoint := fmt.Sprintf("sp_%d", tx.savepointID)
		_, err := tx.ExecContext(ctx, fmt.Sprintf("ROLLBACK TO SAVEPOINT %s", savepoint))
		tx.savepointID--
		if err != nil {
			return fmt.Errorf("failed to rollback to savepoint: %w", err)
		}
		return nil
	}

	// Rollback the main transaction
	if err := tx.Rollback(); err != nil {
		if err != sql.ErrTxDone { // Ignore error if transaction was already committed
			return fmt.Errorf("failed to rollback transaction: %w", err)
		}
	}
	return nil
}

// WithTx executes a function within a transaction boundary
//
// Features:
// - Automatic transaction management (begin/commit/rollback)
// - Panic recovery with automatic rollback
// - Proper context propagation
// - Nested transaction support using savepoints
//
// Usage:
//
//	err := db.WithTx(ctx, func(ctx context.Context) error {
//	    // Use db.GetDB(ctx) to get the transaction
//	    return nil
//	})
func (db *DB) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	// Begin a new transaction or create a savepoint
	ctx, _, err := db.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Handle panics by rolling back the transaction
	defer func() {
		if p := recover(); p != nil {
			_ = db.RollbackTx(ctx) // Best effort rollback
			panic(p)               // Re-throw panic after rollback
		}
	}()

	// Execute the provided function
	if err := fn(ctx); err != nil {
		if rbErr := db.RollbackTx(ctx); rbErr != nil {
			// Combine original error with rollback error
			return fmt.Errorf("error rolling back transaction: %v (original error: %v)", rbErr, err)
		}
		return err
	}

	// Commit the transaction
	if err := db.CommitTx(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetDB returns either the transaction from the context or the main DB connection
// This should be used by repositories to ensure they're using the correct database handle
func (db *DB) GetDB(ctx context.Context) sqlx.ExtContext {
	if tx, ok := GetTx(ctx); ok {
		return tx.Tx
	}
	return db.DB
}
