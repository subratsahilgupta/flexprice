package ent

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/ent"
	entUser "github.com/flexprice/flexprice/ent/user"
	domainUser "github.com/flexprice/flexprice/internal/domain/user"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
)

type userRepository struct {
	client postgres.IClient
	logger *logger.Logger
}

// NewUserRepository creates a new user repository
func NewUserRepository(client postgres.IClient, logger *logger.Logger) domainUser.Repository {
	return &userRepository{
		client: client,
		logger: logger,
	}
}

// Create creates a new user
func (r *userRepository) Create(ctx context.Context, user *domainUser.User) error {
	r.logger.Debugw("creating user", "user_id", user.ID, "email", user.Email, "tenant_id", user.TenantID)

	client := r.client.Querier(ctx)
	_, err := client.User.
		Create().
		SetID(user.ID).
		SetEmail(user.Email).
		SetTenantID(user.TenantID).
		SetStatus(string(user.Status)).
		SetCreatedBy(user.CreatedBy).
		SetUpdatedBy(user.UpdatedBy).
		SetCreatedAt(user.CreatedAt).
		SetUpdatedAt(user.UpdatedAt).
		Save(ctx)

	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetByID retrieves a user by ID
func (r *userRepository) GetByID(ctx context.Context, id string) (*domainUser.User, error) {
	tenantID, ok := ctx.Value(types.CtxTenantID).(string)
	if !ok {
		return nil, fmt.Errorf("tenant ID not found in context")
	}

	client := r.client.Querier(ctx)
	user, err := client.User.
		Query().
		Where(
			entUser.ID(id),
			entUser.TenantID(tenantID),
		).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("user not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return domainUser.FromEnt(user), nil
}

// GetByEmail retrieves a user by email
func (r *userRepository) GetByEmail(ctx context.Context, email string) (*domainUser.User, error) {
	client := r.client.Querier(ctx)

	// For login, we don't have tenant ID in context, so we just search by email
	query := client.User.Query().Where(entUser.Email(email))

	// If tenant ID is in context, add it to the query
	if tenantID, ok := ctx.Value(types.CtxTenantID).(string); ok {
		query = query.Where(entUser.TenantID(tenantID))
	}

	user, err := query.Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("user not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return domainUser.FromEnt(user), nil
}
