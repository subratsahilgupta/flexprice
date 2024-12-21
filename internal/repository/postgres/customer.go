package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
)

type customerRepository struct {
	db     *postgres.DB
	logger *logger.Logger
}

func NewCustomerRepository(db *postgres.DB, logger *logger.Logger) customer.Repository {
	return &customerRepository{db: db, logger: logger}
}

func (r *customerRepository) Create(ctx context.Context, customer *customer.Customer) error {
	query := `
		INSERT INTO customers (
			id, tenant_id, external_id, name, email, created_at, updated_at, created_by, updated_by
		) VALUES (
			:id, :tenant_id, :external_id, :name, :email, :created_at, :updated_at, :created_by, :updated_by
		)`

	r.logger.Debug("creating customer",
		"customer_id", customer.ID,
		"tenant_id", customer.TenantID,
	)

	_, err := r.db.NamedExecContext(ctx, query, customer)
	return err
}

func (r *customerRepository) Get(ctx context.Context, id string) (*customer.Customer, error) {
	var c customer.Customer
	rows, err := r.db.NamedQueryContext(ctx, "SELECT * FROM customers WHERE id = :id AND tenant_id = :tenant_id", map[string]interface{}{
		"id":        id,
		"tenant_id": types.GetTenantID(ctx),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get customer: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("customer not found")
	}

	if err := rows.StructScan(&c); err != nil {
		return nil, fmt.Errorf("failed to scan customer: %w", err)
	}

	return &c, nil
}

func (r *customerRepository) List(ctx context.Context, filter types.Filter) ([]*customer.Customer, error) {
	var customers []*customer.Customer
	query := `
		SELECT * FROM customers WHERE tenant_id = :tenant_id ORDER BY created_at DESC LIMIT :limit OFFSET :offset`

	rows, err := r.db.NamedQueryContext(ctx, query, map[string]interface{}{
		"tenant_id": types.GetTenantID(ctx),
		"limit":     filter.Limit,
		"offset":    filter.Offset,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list customers: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var c customer.Customer
		if err := rows.StructScan(&c); err != nil {
			return nil, fmt.Errorf("failed to scan customer: %w", err)
		}
		customers = append(customers, &c)
	}

	return customers, nil
}

func (r *customerRepository) Update(ctx context.Context, customer *customer.Customer) error {
	query := `
		UPDATE customers SET
			external_id = :external_id,
			name = :name,
			email = :email,
			updated_at = :updated_at,
			updated_by = :updated_by
		WHERE id = :id AND tenant_id = :tenant_id`

	r.logger.Debug("updating customer",
		"customer_id", customer.ID,
		"tenant_id", customer.TenantID,
	)

	_, err := r.db.NamedExecContext(ctx, query, customer)
	return err
}

func (r *customerRepository) Delete(ctx context.Context, id string) error {
	query := `
		UPDATE customers SET
			status = :status,
			updated_at = :updated_at,
			updated_by = :updated_by
		WHERE id = :id AND tenant_id = :tenant_id`

	r.logger.Debug("deleting customer",
		"customer_id", id,
		"tenant_id", types.GetTenantID(ctx),
	)

	_, err := r.db.NamedExecContext(ctx, query, map[string]interface{}{
		"id":         id,
		"tenant_id":  types.GetTenantID(ctx),
		"status":     types.StatusDeleted,
		"updated_by": types.GetUserID(ctx),
		"updated_at": time.Now().UTC(),
	})
	return err
}
