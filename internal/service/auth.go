package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	authProvider "github.com/flexprice/flexprice/internal/auth"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/auth"
	"github.com/flexprice/flexprice/internal/domain/environment"
	"github.com/flexprice/flexprice/internal/domain/tenant"
	"github.com/flexprice/flexprice/internal/domain/user"
	"github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
)

type AuthService interface {
	SignUp(ctx context.Context, req *dto.SignUpRequest) (*dto.AuthResponse, error)
	Login(ctx context.Context, req *dto.LoginRequest) (*dto.AuthResponse, error)
	OnboardNewUserWithTenant(ctx context.Context, userID, email, tenantName, tenantID string) error
}

type authService struct {
	userRepo        user.Repository
	authRepo        auth.Repository
	tenantRepo      tenant.Repository
	environmentRepo environment.Repository
	logger          *logger.Logger
	cfg             *config.Configuration
	authProvider    authProvider.Provider
	db              postgres.IClient
}

func NewAuthService(
	cfg *config.Configuration,
	userRepo user.Repository,
	authRepo auth.Repository,
	tenantRepo tenant.Repository,
	environmentRepo environment.Repository,
	logger *logger.Logger,
	db postgres.IClient,
) AuthService {
	return &authService{
		userRepo:        userRepo,
		authRepo:        authRepo,
		tenantRepo:      tenantRepo,
		environmentRepo: environmentRepo,
		logger:          logger,
		cfg:             cfg,
		authProvider:    authProvider.NewProvider(cfg),
		db:              db,
	}
}

// SignUp creates a new user and returns an auth token
func (s *authService) SignUp(ctx context.Context, req *dto.SignUpRequest) (*dto.AuthResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, errors.Wrap(errors.ErrValidation, errors.ErrCodeValidation, "invalid request")
	}

	// Check if user already exists in our system
	existingUser, err := s.userRepo.GetByEmail(ctx, req.Email)
	if existingUser != nil {
		// TODO: Check if the user is already onboarded to a tenant
		// if not, return an error
		// if yes, return the auth response with the user info
		return nil, errors.Wrap(errors.ErrAlreadyExists, errors.ErrCodeAlreadyExists, "user already exists")
	}

	// Generate a tenant ID
	tenantID := types.GenerateUUIDWithPrefix(types.UUID_PREFIX_TENANT)

	authResponse, err := s.authProvider.SignUp(ctx, authProvider.AuthRequest{
		Email:    req.Email,
		Password: req.Password,
		Token:    req.Token,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSystemError, "unable to sign up")
	}

	response := &dto.AuthResponse{
		Token:    authResponse.AuthToken,
		UserID:   authResponse.ID,
		TenantID: tenantID,
	}

	err = s.db.WithTx(ctx, func(ctx context.Context) error {
		// Create auth record
		if s.authProvider.GetProvider() == types.AuthProviderFlexprice {
			auth := auth.NewAuth(authResponse.ID, s.authProvider.GetProvider(), authResponse.ProviderToken)
			err = s.authRepo.CreateAuth(ctx, auth)
			if err != nil {
				return fmt.Errorf("unable to create auth: %w", err)
			}
		}

		err = s.OnboardNewUserWithTenant(
			ctx,
			authResponse.ID,
			req.Email,
			req.TenantName,
			response.TenantID,
		)
		if err != nil {
			return fmt.Errorf("failed to onboard tenant: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// Login authenticates a user and returns an auth token
func (s *authService) Login(ctx context.Context, req *dto.LoginRequest) (*dto.AuthResponse, error) {
	user, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("unable to get user: %w", err)
	}

	var auth *auth.Auth
	if s.authProvider.GetProvider() == types.AuthProviderFlexprice {
		auth, err = s.authRepo.GetAuthByUserID(ctx, user.ID)
		if err != nil {
			return nil, fmt.Errorf("unable to fetch user authentication channel: %w", err)
		}
	}

	authResponse, err := s.authProvider.Login(ctx, authProvider.AuthRequest{
		UserID:   user.ID,
		TenantID: user.TenantID,
		Email:    user.Email,
		Password: req.Password,
	}, auth)
	if err != nil {
		return nil, fmt.Errorf("unable to login: %w", err)
	}

	if authResponse.ID != user.ID {
		return nil, errors.Wrap(errors.ErrPermissionDenied, errors.ErrCodePermissionDenied, "user not found")
	}

	response := &dto.AuthResponse{
		Token:    authResponse.AuthToken,
		UserID:   authResponse.ID,
		TenantID: user.TenantID,
	}

	return response, nil
}

// OnboardNewUserWithTenant creates a new tenant, assigns it to the user, and sets up default environments
func (s *authService) OnboardNewUserWithTenant(ctx context.Context, userID, email, tenantName, tenantID string) error {
	// Use default tenant name if not provided
	if tenantName == "" {
		tenantName = "ACME Inc"
	}

	// Create tenant
	newTenant := &tenant.Tenant{
		ID:        tenantID,
		Name:      tenantName,
		Status:    types.StatusPublished,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.tenantRepo.Create(ctx, newTenant); err != nil {
		return errors.Wrap(err, errors.ErrCodeSystemError, "unable to create tenant")
	}

	// Create a new user without a tenant ID initially
	newUser := &user.User{
		ID:    userID,
		Email: email,
		BaseModel: types.BaseModel{
			TenantID:  tenantID,
			Status:    types.StatusPublished,
			CreatedBy: userID,
			UpdatedBy: userID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	if err := s.userRepo.Create(ctx, newUser); err != nil {
		return errors.Wrap(err, errors.ErrCodeSystemError, "unable to create user")
	}

	// Assign tenant to user in auth provider
	if err := s.authProvider.AssignUserToTenant(ctx, userID, newTenant.ID); err != nil {
		return errors.Wrap(err, errors.ErrCodeSystemError, "unable to assign tenant to user in auth provider")
	}

	// Create default environments (development, production)
	envTypes := []types.EnvironmentType{
		types.EnvironmentDevelopment,
		types.EnvironmentProduction,
	}

	for _, envType := range envTypes {
		env := &environment.Environment{
			ID:   types.GenerateUUIDWithPrefix(types.UUID_PREFIX_ENVIRONMENT),
			Name: envType.DisplayTitle(),
			Type: envType,
			BaseModel: types.BaseModel{
				TenantID:  newTenant.ID,
				Status:    types.StatusPublished,
				CreatedBy: userID,
				UpdatedBy: userID,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		}

		if err := s.environmentRepo.Create(ctx, env); err != nil {
			return errors.Wrap(err, errors.ErrCodeSystemError, "unable to create environment")
		}
	}

	// TODO: Set up dummy plans, features, etc.

	return nil
}
