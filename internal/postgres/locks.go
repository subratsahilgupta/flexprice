package postgres

import (
	"context"
	"fmt"
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
