package postgres

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/internal/types"
)

// LockKey uses transaction-scoped advisory lock.
// WAITs until lock is acquired.
// Auto released on tx commit/rollback.
// Must be called inside a transaction.
func (c *Client) LockKey(ctx context.Context, key string) error {
	tx := c.TxFromContext(ctx)
	if tx == nil {
		return fmt.Errorf("LockKey must be called inside transaction")
	}

	_, err := tx.ExecContext(ctx, `
		SELECT pg_advisory_xact_lock(hashtext($1))
	`, key)
	return err
}

// TryLockKey tries acquiring advisory lock immediately.
// Returns ok=false if lock is already held.
// Auto released on tx commit/rollback.
// Must be called inside a transaction.
func (c *Client) TryLockKey(ctx context.Context, key string) (bool, error) {
	tx := c.TxFromContext(ctx)
	if tx == nil {
		return false, fmt.Errorf("TryLockKey must be called inside transaction")
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT pg_try_advisory_xact_lock(hashtext($1))
	`, key)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	if !rows.Next() {
		return false, nil
	}

	var ok bool
	if err := rows.Scan(&ok); err != nil {
		return false, err
	}

	return ok, nil
}

// LockRowForUpdate locks a row using FOR UPDATE (WAIT mode).
// Blocks until the row is available for locking.
// Must be called inside a transaction. Lock is automatically released on commit/rollback.
func (c *Client) LockRowForUpdate(ctx context.Context, tableName types.TableName, id any) error {
	tx := c.TxFromContext(ctx)
	if tx == nil {
		return fmt.Errorf("LockRowForUpdate must be called inside transaction")
	}

	query := fmt.Sprintf("SELECT id FROM %s WHERE id = $1 FOR UPDATE", tableName)
	_, err := tx.ExecContext(ctx, query, id)
	return err
}

// LockRowForUpdateNowait locks a row using FOR UPDATE NOWAIT (fail-fast mode).
// Returns error if row is already locked.
// Must be called inside a transaction. Lock is automatically released on commit/rollback.
func (c *Client) LockRowForUpdateNowait(ctx context.Context, tableName types.TableName, id any) error {
	tx := c.TxFromContext(ctx)
	if tx == nil {
		return fmt.Errorf("LockRowForUpdateNowait must be called inside transaction")
	}

	query := fmt.Sprintf("SELECT id FROM %s WHERE id = $1 FOR UPDATE NOWAIT", tableName)
	_, err := tx.ExecContext(ctx, query, id)
	return err
}
