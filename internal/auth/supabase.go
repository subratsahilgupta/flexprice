package auth

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/auth"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/golang-jwt/jwt/v4"
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
		return nil, ierr.NewError("token is required").
			Mark(ierr.ErrPermissionDenied)
	}

	// Validate the token and extract user ID
	claims, err := s.ValidateToken(ctx, req.Token)
	if err != nil {
		return nil, ierr.NewError("invalid token").
			Mark(ierr.ErrPermissionDenied)
	}

	if claims.Email != req.Email {
		return nil, ierr.NewError("email mismatch").
			Mark(ierr.ErrPermissionDenied)
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
		return nil, ierr.WithError(err).
			WithHint("Failed to get user").
			Mark(ierr.ErrPermissionDenied)
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
			return nil, ierr.NewError("unexpected signing method").
				WithHint("Unexpected signing method").
				WithReportableDetails(map[string]interface{}{
					"signing_method": token.Method.Alg(),
				}).
				Mark(ierr.ErrPermissionDenied)
		}
		return []byte(s.AuthConfig.Secret), nil
	})

	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Token parse error").
			Mark(ierr.ErrPermissionDenied)
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok || !parsedToken.Valid {
		return nil, ierr.NewError("invalid token claims").
			WithHint("Invalid token claims").
			Mark(ierr.ErrPermissionDenied)
	}

	userID, userOk := claims["sub"].(string)
	if !userOk {
		return nil, ierr.NewError("token missing user ID").
			WithHint("Token missing user ID").
			Mark(ierr.ErrPermissionDenied)
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
		return nil, ierr.NewError("token missing email").
			WithHint("Token missing email").
			Mark(ierr.ErrPermissionDenied)
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
		return ierr.WithError(err).
			WithHint("Failed to assign tenant to user").
			Mark(ierr.ErrSystem)
	}

	s.logger.Debugw("assigned tenant to user",
		"user_id", userID,
		"tenant_id", tenantID,
		"response", resp,
	)

	return nil
}

// GenerateDashboardToken generates a customer dashboard token
// Note: For Supabase, dashboard tokens use the same mechanism as Flexprice auth
func (s *supabaseAuth) GenerateDashboardToken(customerID, externalCustomerID, tenantID, environmentID string, timeoutHours int) (string, time.Time, error) {
	// Validate required parameters
	customerID = strings.TrimSpace(customerID)
	externalCustomerID = strings.TrimSpace(externalCustomerID)
	tenantID = strings.TrimSpace(tenantID)
	environmentID = strings.TrimSpace(environmentID)

	if customerID == "" {
		return "", time.Time{}, fmt.Errorf("missing required parameter: customerID")
	}
	if externalCustomerID == "" {
		return "", time.Time{}, fmt.Errorf("missing required parameter: externalCustomerID")
	}
	if tenantID == "" {
		return "", time.Time{}, fmt.Errorf("missing required parameter: tenantID")
	}
	if environmentID == "" {
		return "", time.Time{}, fmt.Errorf("missing required parameter: environmentID")
	}

	expiresAt := time.Now().Add(time.Duration(timeoutHours) * time.Hour)

	claims := jwt.MapClaims{
		"customer_id":          customerID,
		"external_customer_id": externalCustomerID,
		"tenant_id":            tenantID,
		"environment_id":       environmentID,
		"token_type":           "dashboard",
		"exp":                  expiresAt.Unix(),
		"iat":                  time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(s.AuthConfig.Secret))
	if err != nil {
		return "", time.Time{}, ierr.WithError(err).
			WithHint("Failed to sign dashboard token").
			Mark(ierr.ErrSystem)
	}

	return signedToken, expiresAt, nil
}

// ValidateDashboardToken validates a customer dashboard token
func (s *supabaseAuth) ValidateDashboardToken(ctx context.Context, token string) (*auth.DashboardClaims, error) {
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ierr.NewError("unexpected signing method").
				Mark(ierr.ErrPermissionDenied)
		}
		return []byte(s.AuthConfig.Secret), nil
	})

	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid dashboard token").
			Mark(ierr.ErrPermissionDenied)
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok || !parsedToken.Valid {
		return nil, ierr.NewError("invalid token claims").
			Mark(ierr.ErrPermissionDenied)
	}

	tokenType, ok := claims["token_type"].(string)
	if !ok || tokenType != "dashboard" {
		return nil, ierr.NewError("invalid token type claim").
			WithHint("Token type claim is missing or not a dashboard token").
			Mark(ierr.ErrPermissionDenied)
	}

	customerID, ok := claims["customer_id"].(string)
	if !ok {
		return nil, ierr.NewError("invalid customer_id claim").
			WithHint("customer_id claim is missing or has wrong type").
			Mark(ierr.ErrPermissionDenied)
	}

	externalCustomerID, ok := claims["external_customer_id"].(string)
	if !ok {
		return nil, ierr.NewError("invalid external_customer_id claim").
			WithHint("external_customer_id claim is missing or has wrong type").
			Mark(ierr.ErrPermissionDenied)
	}

	tenantIDClaim, ok := claims["tenant_id"].(string)
	if !ok {
		return nil, ierr.NewError("invalid tenant_id claim").
			WithHint("tenant_id claim is missing or has wrong type").
			Mark(ierr.ErrPermissionDenied)
	}

	environmentID, ok := claims["environment_id"].(string)
	if !ok {
		return nil, ierr.NewError("invalid environment_id claim").
			WithHint("environment_id claim is missing or has wrong type").
			Mark(ierr.ErrPermissionDenied)
	}

	if customerID == "" || externalCustomerID == "" || tenantIDClaim == "" || environmentID == "" {
		return nil, ierr.NewError("missing required claims").
			Mark(ierr.ErrPermissionDenied)
	}

	return &auth.DashboardClaims{
		CustomerID:         customerID,
		ExternalCustomerID: externalCustomerID,
		TenantID:           tenantIDClaim,
		EnvironmentID:      environmentID,
	}, nil
}
