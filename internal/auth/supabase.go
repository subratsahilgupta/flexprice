package auth

import (
	"context"
	"fmt"
	"log"

	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/auth"
	"github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/golang-jwt/jwt"
	"github.com/nedpals/supabase-go"
)

type supabaseAuth struct {
	AuthConfig config.AuthConfig
	client     *supabase.Client
	logger     *logger.Logger
}

func NewSupabaseAuth(cfg *config.Configuration) Provider {
	supabaseUrl := cfg.Auth.Supabase.BaseURL
	adminApiKey := cfg.Auth.Supabase.ServiceKey

	client := supabase.CreateClient(supabaseUrl, adminApiKey)
	if client == nil {
		log.Fatalf("failed to create Supabase client")
	}

	logger, _ := logger.NewLogger(cfg)

	return &supabaseAuth{
		AuthConfig: cfg.Auth,
		client:     client,
		logger:     logger,
	}
}

func (s *supabaseAuth) GetProvider() types.AuthProvider {
	return types.AuthProviderSupabase
}

// SignUp is not used directly for Supabase as users sign up through the Supabase UI
// This method is kept for compatibility with the Provider interface
func (s *supabaseAuth) SignUp(ctx context.Context, req AuthRequest) (*AuthResponse, error) {
	// For Supabase, we don't directly sign up users through this method
	// Instead, we validate the token and get user info
	// For Supabase, we validate the token and extract user info
	if req.Token == "" {
		return nil, errors.Wrap(errors.ErrValidation, errors.ErrCodeValidation, "token is required")
	}

	// Validate the token and extract user ID
	claims, err := s.ValidateToken(ctx, req.Token)
	if err != nil {
		return nil, errors.Wrap(errors.ErrPermissionDenied, errors.ErrCodePermissionDenied, "invalid token")
	}

	if claims.Email != req.Email {
		return nil, errors.Wrap(errors.ErrPermissionDenied, errors.ErrCodePermissionDenied, "email mismatch")
	}

	// Create auth response with the token
	authResponse := &AuthResponse{
		ProviderToken: claims.UserID,
		AuthToken:     req.Token,
		ID:            claims.UserID,
	}

	return authResponse, nil
}

// Login validates the token and returns user info
func (s *supabaseAuth) Login(ctx context.Context, req AuthRequest, userAuthInfo *auth.Auth) (*AuthResponse, error) {
	user, err := s.client.Auth.SignIn(ctx, supabase.UserCredentials{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &AuthResponse{
		ProviderToken: user.User.ID,
		AuthToken:     user.AccessToken,
		ID:            user.User.ID,
	}, nil
}

func (s *supabaseAuth) ValidateToken(ctx context.Context, token string) (*auth.Claims, error) {
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.AuthConfig.Secret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("token parse error: %w", err)
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok || !parsedToken.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	userID, userOk := claims["sub"].(string)
	if !userOk {
		return nil, fmt.Errorf("token missing user ID")
	}

	// Get tenant_id from app_metadata
	var tenantID string
	if appMetadata, ok := claims["app_metadata"].(map[string]interface{}); ok {
		if tid, ok := appMetadata["tenant_id"].(string); ok {
			tenantID = tid
		}
	}

	email, ok := claims["email"].(string)
	if !ok {
		return nil, fmt.Errorf("token missing email")
	}

	return &auth.Claims{
		UserID:   userID,
		TenantID: tenantID,
		Email:    email,
	}, nil
}

func (s *supabaseAuth) AssignUserToTenant(ctx context.Context, userID string, tenantID string) error {
	// Use Supabase Admin API to update user's app_metadata
	params := supabase.AdminUserParams{
		AppMetadata: map[string]interface{}{
			"tenant_id": tenantID,
		},
	}

	resp, err := s.client.Admin.UpdateUser(ctx, userID, params)
	if err != nil {
		return fmt.Errorf("failed to assign tenant to user: %w", err)
	}

	s.logger.Debugw("assigned tenant to user",
		"user_id", userID,
		"tenant_id", tenantID,
		"response", resp,
	)

	return nil
}
