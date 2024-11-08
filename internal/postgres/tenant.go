package postgres

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

func (db *DB) QueryWithTenant(ctx context.Context, tenantID string, query string, args ...interface{}) (*sqlx.Rows, error) {
	tenantQuery := fmt.Sprintf("%s WHERE tenant_id = $1", query)
	return db.QueryxContext(ctx, tenantQuery, append([]interface{}{tenantID}, args...)...)
}
