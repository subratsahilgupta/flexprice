package ent

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/ent"
	entAuth "github.com/flexprice/flexprice/ent/auth"
	domainAuth "github.com/flexprice/flexprice/internal/domain/auth"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
)

type authRepository struct {
	client postgres.IClient
	logger *logger.Logger
}

// NewAuthRepository creates a new auth repository
func NewAuthRepository(client postgres.IClient, logger *logger.Logger) domainAuth.Repository {
	return &authRepository{
		client: client,
		logger: logger,
	}
}

// CreateAuth creates a new auth record
func (r *authRepository) CreateAuth(ctx context.Context, auth *domainAuth.Auth) error {
	if !r.ValidateProvider(auth.Provider) {
		return fmt.Errorf("invalid provider")
	}

	r.logger.Debugw("creating auth", "user_id", auth.UserID, "provider", auth.Provider)

	client := r.client.Querier(ctx)
	_, err := client.Auth.
		Create().
		SetUserID(auth.UserID).
		SetProvider(string(auth.Provider)).
		SetToken(auth.Token).
		SetStatus(string(auth.Status)).
		SetCreatedAt(auth.CreatedAt).
		SetUpdatedAt(auth.UpdatedAt).
		Save(ctx)

	if err != nil {
		return fmt.Errorf("failed to create auth: %w", err)
	}

	return nil
}

// GetAuthByUserID retrieves an auth record by user ID
func (r *authRepository) GetAuthByUserID(ctx context.Context, userID string) (*domainAuth.Auth, error) {
	client := r.client.Querier(ctx)
	auth, err := client.Auth.
		Query().
		Where(
			entAuth.UserID(userID),
		).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("auth not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get auth: %w", err)
	}

	return domainAuth.FromEnt(auth), nil
}

// UpdateAuth updates an auth record
func (r *authRepository) UpdateAuth(ctx context.Context, auth *domainAuth.Auth) error {
	if !r.ValidateProvider(auth.Provider) {
		return fmt.Errorf("invalid provider")
	}

	r.logger.Debugw("updating auth", "user_id", auth.UserID, "provider", auth.Provider)

	client := r.client.Querier(ctx)
	_, err := client.Auth.
		Update().
		Where(entAuth.UserID(auth.UserID)).
		SetProvider(string(auth.Provider)).
		SetToken(auth.Token).
		SetStatus(string(auth.Status)).
		SetUpdatedAt(auth.UpdatedAt).
		Save(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("auth not found: %w", err)
		}
		return fmt.Errorf("failed to update auth: %w", err)
	}

	return nil
}

// DeleteAuth deletes an auth record
func (r *authRepository) DeleteAuth(ctx context.Context, userID string) error {
	r.logger.Debugw("deleting auth", "user_id", userID)

	client := r.client.Querier(ctx)
	_, err := client.Auth.
		Delete().
		Where(entAuth.UserID(userID)).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to delete auth: %w", err)
	}

	return nil
}

// Auth provider to be used only for Flexprice or other self implemented providers
func (r *authRepository) ValidateProvider(provider types.AuthProvider) bool {
	return provider == types.AuthProviderFlexprice
}
