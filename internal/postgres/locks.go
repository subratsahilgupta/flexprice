package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/lib/pq"
)

// LockKey acquires an advisory lock based on the provided request.
// If Timeout is nil, defaults to 30 seconds. If Timeout is 0 or negative, uses fail-fast behavior.
// Auto released on tx commit/rollback.
// Must be called inside a transaction.
func (c *Client) LockKey(ctx context.Context, req types.LockRequest) error {
	tx := c.TxFromContext(ctx)
	if tx == nil {
		return fmt.Errorf("LockKey must be called inside transaction")
	}

	timeout := req.GetTimeout()

	// Handle zero or negative timeout (fail-fast)
	if timeout <= 0 {
		ok, err := c.TryLockKey(ctx, req.Key)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("lock already held (timeout: 0ms)")
		}
		return nil
	}

	// Set lock_timeout for this transaction (automatically reset on commit/rollback)
	timeoutMs := int(timeout.Milliseconds())
	_, err := tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL lock_timeout = %d", timeoutMs))
	if err != nil {
		return fmt.Errorf("failed to set lock timeout: %w", err)
	}

	// Acquire the lock (will respect lock_timeout we just set)
	_, err = tx.ExecContext(ctx, `
		SELECT pg_advisory_xact_lock(hashtext($1))
	`, req.Key)
	if err != nil {
		// Check if it's a lock timeout error using PostgreSQL error code
		if isLockTimeoutError(err) {
			return fmt.Errorf("failed to acquire lock within %v: %w", timeout, err)
		}
		return fmt.Errorf("failed to acquire lock: %w", err)
	}

	return nil
}

// isLockTimeoutError checks if the error is a PostgreSQL lock timeout error
// Uses PostgreSQL error codes for reliable detection
func isLockTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	// Check for pq.Error directly (most common case)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		// PostgreSQL error code 55P03 = query_canceled (lock timeout)
		return pqErr.Code == "55P03"
	}

	return false
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
