package postgres

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/internal/domain/auth"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
)

type authRepository struct {
	db     *postgres.DB
	logger *logger.Logger
}

func NewAuthRepository(db *postgres.DB, logger *logger.Logger) auth.Repository {
	return &authRepository{db: db, logger: logger}
}

func (r *authRepository) CreateAuth(ctx context.Context, auth *auth.Auth) error {
	if !r.ValidateProvider(auth.Provider) {
		return fmt.Errorf("invalid provider")
	}

	query := `INSERT INTO auths (user_id, provider, token, status, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.db.ExecContext(ctx, query, auth.UserID, auth.Provider, auth.Token, auth.Status, auth.CreatedAt, auth.UpdatedAt)
	return err
}

func (r *authRepository) GetAuthByUserID(ctx context.Context, userID string) (*auth.Auth, error) {
	query := `SELECT * FROM auths WHERE user_id = $1`
	var auth auth.Auth
	err := r.db.GetContext(ctx, &auth, query, userID)
	return &auth, err
}

func (r *authRepository) UpdateAuth(ctx context.Context, auth *auth.Auth) error {
	if !r.ValidateProvider(auth.Provider) {
		return fmt.Errorf("invalid provider")
	}

	query := `UPDATE auths SET provider = $1, token = $2, status = $3, updated_at = $4 WHERE user_id = $5`
	_, err := r.db.ExecContext(ctx, query, auth.Provider, auth.Token, auth.Status, auth.UpdatedAt, auth.UserID)
	return err
}

func (r *authRepository) DeleteAuth(ctx context.Context, userID string) error {
	query := `DELETE FROM auths WHERE user_id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

// Auth provider to be used only for Flexprice or other self implemented providers
func (r *authRepository) ValidateProvider(provider types.AuthProvider) bool {
	return provider == types.AuthProviderFlexprice
}
