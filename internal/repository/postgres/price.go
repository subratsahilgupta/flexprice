package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
)

type priceRepository struct {
	db *postgres.DB
}

func NewPriceRepository(db *postgres.DB) price.Repository {
	return &priceRepository{db: db}
}

func (r *priceRepository) CreatePrice(ctx context.Context, price *price.Price) error {
	query := `
		INSERT INTO prices (
			id, tenant_id, amount, currency, external_id, external_source,
			billing_period, billing_period_count, billing_model, billing_cadence,
			billing_country_code, tier_mode, tiers, transform, lookup_key,
			description, metadata, status, created_at, updated_at, created_by, updated_by
		) VALUES (
			:id, :tenant_id, :amount, :currency, :external_id, :external_source,
			:billing_period, :billing_period_count, :billing_model, :billing_cadence,
			:billing_country_code, :tier_mode, :tiers, :transform, :lookup_key,
			:description, :metadata, :status, :created_at, :updated_at, :created_by, :updated_by
		)`

	_, err := r.db.NamedExecContext(ctx, query, price)
	return err
}

func (r *priceRepository) GetPrice(ctx context.Context, id string) (*price.Price, error) {
	var p price.Price
	query := `SELECT * FROM prices WHERE id = :id AND tenant_id = :tenant_id`

	err := r.db.GetContext(ctx, &p, query, map[string]interface{}{
		"id":        id,
		"tenant_id": ctx.Value("tenant_id"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get price: %w", err)
	}
	return &p, nil
}

func (r *priceRepository) GetPrices(ctx context.Context, filter types.Filter) ([]*price.Price, error) {
	var prices []*price.Price
	query := `
		SELECT * FROM prices 
		WHERE tenant_id = :tenant_id 
		ORDER BY created_at DESC 
		LIMIT :limit OFFSET :offset`

	err := r.db.SelectContext(
		ctx,
		&prices,
		query,
		ctx.Value("tenant_id"),
		filter.Limit,
		filter.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list prices: %w", err)
	}

	return prices, nil
}

func (r *priceRepository) UpdatePrice(ctx context.Context, price *price.Price) error {
	price.UpdatedAt = time.Now()
	price.UpdatedBy = types.GetUserID(ctx)

	query := `
		UPDATE prices SET 
			description = :description,
			metadata = :metadata,
			status = :status,
			updated_at = :updated_at,
			updated_by = :updated_by
		WHERE id = :id AND tenant_id = :tenant_id`

	result, err := r.db.NamedExecContext(ctx, query, price)
	if err != nil {
		return fmt.Errorf("failed to update price: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("price not found")
	}
	return nil
}

func (r *priceRepository) UpdatePriceStatus(ctx context.Context, id string, status types.Status) error {
	query := `
		UPDATE prices SET 
			status = :status,
			updated_at = :updated_at,
			updated_by = :updated_by
		WHERE id = :id AND tenant_id = :tenant_id`

	_, err := r.db.NamedExecContext(ctx, query, map[string]interface{}{
		"id":         id,
		"status":     status,
		"updated_at": time.Now(),
		"updated_by": ctx.Value("user_id"),
	})
	if err != nil {
		return fmt.Errorf("failed to update price status: %w", err)
	}
	return nil
}
