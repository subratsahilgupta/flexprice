package postgres

import (
	"context"

	"github.com/flexprice/flexprice/internal/domain/user"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
)

type userRepository struct {
	db     *postgres.DB
	logger *logger.Logger
}

func NewUserRepository(db *postgres.DB, logger *logger.Logger) user.Repository {
	return &userRepository{db: db, logger: logger}
}

func (r *userRepository) Create(ctx context.Context, user *user.User) error {
	query := `
	INSERT INTO users (id, email, tenant_id, created_at, updated_at, created_by, updated_by)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := r.db.ExecContext(
		ctx, query,
		user.ID,
		user.Email,
		user.TenantID,
		user.CreatedAt,
		user.UpdatedAt,
		user.CreatedBy,
		user.UpdatedBy,
	)
	return err
}

func (r *userRepository) GetByID(ctx context.Context, id string) (*user.User, error) {
	tenantID := types.GetTenantID(ctx)
	query := `SELECT * FROM users WHERE id = $1 AND tenant_id = $2`

	var user user.User
	err := r.db.GetContext(ctx, &user, query, id, tenantID)
	return &user, err
}

// Since we don't have a tenant id when getting a user by email, we need to get all users with that email and filter them by tenant id.
// This repo method is only exposed for the login endpoint.
func (r *userRepository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	query := `SELECT * FROM users WHERE email = $1`

	var user user.User
	err := r.db.GetContext(ctx, &user, query, email)
	return &user, err
}
