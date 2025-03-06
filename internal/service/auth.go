package service

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/internal/api/dto"
	authProvider "github.com/flexprice/flexprice/internal/auth"
	"github.com/flexprice/flexprice/internal/domain/auth"
	"github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/pubsub"
	"github.com/flexprice/flexprice/internal/types"
)

type AuthService interface {
	SignUp(ctx context.Context, req *dto.SignUpRequest) (*dto.AuthResponse, error)
	Login(ctx context.Context, req *dto.LoginRequest) (*dto.AuthResponse, error)
}

type authService struct {
	ServiceParams
	pubSub       pubsub.PubSub
	authProvider authProvider.Provider
}

func NewAuthService(
	params ServiceParams,
	pubSub pubsub.PubSub,
) AuthService {
	return &authService{
		ServiceParams: params,
		pubSub:        pubSub,
		authProvider:  authProvider.NewProvider(params.Config),
	}
}

// SignUp creates a new user and returns an auth token
func (s *authService) SignUp(ctx context.Context, req *dto.SignUpRequest) (*dto.AuthResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, errors.Wrap(errors.ErrValidation, errors.ErrCodeValidation, "invalid request")
	}

	// Check if user already exists in our system
	existingUser, err := s.UserRepo.GetByEmail(ctx, req.Email)
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

	err = s.DB.WithTx(ctx, func(ctx context.Context) error {
		// Create auth record
		if s.authProvider.GetProvider() == types.AuthProviderFlexprice {
			auth := auth.NewAuth(authResponse.ID, s.authProvider.GetProvider(), authResponse.ProviderToken)
			err = s.AuthRepo.CreateAuth(ctx, auth)
			if err != nil {
				return fmt.Errorf("unable to create auth: %w", err)
			}
		}

		onboardingService := NewOnboardingService(s.ServiceParams, s.pubSub)

		err = onboardingService.OnboardNewUserWithTenant(
			ctx,
			authResponse.ID,
			req.Email,
			req.TenantName,
			response.TenantID,
		)
		if err != nil {
			return fmt.Errorf("failed to onboard tenant: %w", err)
		}

		// Assign tenant to user in auth provider
		if err := s.authProvider.AssignUserToTenant(ctx, authResponse.ID, response.TenantID); err != nil {
			return errors.Wrap(err, errors.ErrCodeSystemError, "unable to assign tenant to user in auth provider")
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
	user, err := s.UserRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("unable to get user: %w", err)
	}

	var auth *auth.Auth
	if s.authProvider.GetProvider() == types.AuthProviderFlexprice {
		auth, err = s.AuthRepo.GetAuthByUserID(ctx, user.ID)
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
