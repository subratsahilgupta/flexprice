package auth

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/auth"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/golang-jwt/jwt"
)

type supabaseAuth struct {
	AuthConfig config.AuthConfig
}

func NewSupabaseAuth(cfg *config.Configuration) Provider {
	return &supabaseAuth{
		AuthConfig: cfg.Auth,
	}
}

func (s *supabaseAuth) GetProvider() types.AuthProvider {
	return types.AuthProviderSupabase
}

func (s *supabaseAuth) SignUp(ctx context.Context, req AuthRequest) (*AuthResponse, error) {
	// Delegate to Supabase Auth API
	// Implementation depends on Supabase SDK or REST API
	return nil, fmt.Errorf("use UI for signup")
}

func (s *supabaseAuth) Login(ctx context.Context, req AuthRequest, userAuthInfo *auth.Auth) (*AuthResponse, error) {
	// TODO: implement login by integrating with Supabase SDK
	return nil, fmt.Errorf("use UI for login")
}

func (s *supabaseAuth) ValidateToken(ctx context.Context, token string) (*auth.Claims, error) {
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		secret := s.AuthConfig.Secret
		return []byte(secret), nil
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

	// TODO: set this later when we have tenant support
	tenantID, tenantOk := claims["tenant_id"].(string)
	if !tenantOk {
		tenantID = types.DefaultTenantID
	}

	return &auth.Claims{UserID: userID, TenantID: tenantID}, nil
}
