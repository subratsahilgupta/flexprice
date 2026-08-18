package service

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/golang-jwt/jwt/v4"
)

type pylonClaims struct {
	Email             string `json:"email"`
	Name              string `json:"name,omitempty"`
	AccountExternalID string `json:"account_external_id,omitempty"`
	ContactExternalID string `json:"contact_external_id,omitempty"`
	jwt.RegisteredClaims
}

func (s *userService) CreateSupportChatToken(ctx context.Context) (*dto.SupportChatTokenResponse, error) {
	if s.cfg == nil || s.cfg.Pylon.AppID == "" || s.cfg.Pylon.IdentitySecret == "" {
		return nil, ierr.NewError("pylon identity verification is not configured").
			WithHint("Set pylon.app_id and pylon.identity_secret").
			Mark(ierr.ErrInternal)
	}

	user, err := s.GetUserInfo(ctx)
	if err != nil {
		return nil, err
	}

	if user.Email == "" {
		return nil, ierr.NewError("user has no email").
			WithHint("Support chat identity verification requires a user with an email").
			Mark(ierr.ErrValidation)
	}

	now := time.Now().UTC()
	expiresAt := now.Add(s.cfg.Pylon.GetTokenTTL())

	claims := pylonClaims{
		Email:             user.Email,
		Name:              user.Name,
		ContactExternalID: user.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			Audience:  jwt.ClaimStrings{s.cfg.Pylon.AppID},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	if user.Tenant != nil {
		claims.AccountExternalID = user.Tenant.ID
		if claims.Name == "" {
			claims.Name = user.Tenant.Name
		}
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.Pylon.IdentitySecret))
	if err != nil {
		s.logger.Error(ctx, "failed to sign support chat token", "error", err, "user_id", user.ID)
		return nil, ierr.WithError(err).
			WithHint("Failed to sign the support chat token").
			Mark(ierr.ErrInternal)
	}

	return &dto.SupportChatTokenResponse{
		Token:     token,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	}, nil
}
