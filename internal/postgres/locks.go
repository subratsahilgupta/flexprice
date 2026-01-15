package postgres

import (
	"context"
	"fmt"
	"sort"
	"strings"

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

// GenerateLockKey generates a lock key from a scope and parameters.
// Automatically extracts tenant_id and environment_id from context and includes them in the key.
// Reuses the same pattern as idempotency.GenerateKey for consistency.
// The key is a deterministic string that Postgres will hash internally.
func GenerateLockKey(ctx context.Context, scope types.LockScope, params map[string]interface{}) string {
	// Extract tenant and environment IDs from context
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	// Create a merged params map that includes context values
	// User-provided params override context values if same key is provided
	mergedParams := make(map[string]interface{})

	// Add tenant_id from context if present
	if tenantID != "" {
		mergedParams["tenant_id"] = tenantID
	}

	// Add environment_id from context if present
	if environmentID != "" {
		mergedParams["environment_id"] = environmentID
	}

	// Merge user-provided params (these override context values if same key)
	for k, v := range params {
		mergedParams[k] = v
	}

	// Sort params for consistent ordering (same as idempotency generator)
	keys := make([]string, 0, len(mergedParams))
	for k := range mergedParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build string in format: scope:key1=value1:key2=value2:...
	// Same format as idempotency generator, but without hashing
	// (Postgres hashtext() will hash it internally)
	var b strings.Builder
	b.WriteString(string(scope))
	for _, k := range keys {
		b.WriteString(fmt.Sprintf(":%s=%v", k, mergedParams[k]))
	}

	return b.String()
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
