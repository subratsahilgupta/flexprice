package service

import (
	"testing"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/user"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/stretchr/testify/suite"
)

type AuthServiceSuite struct {
	testutil.BaseServiceTestSuite
	authService AuthService
	userRepo    *testutil.InMemoryUserStore
}

func TestAuthService(t *testing.T) {
	suite.Run(t, new(AuthServiceSuite))
}

func (s *AuthServiceSuite) SetupTest() {
	s.BaseServiceTestSuite.SetupTest()
	s.setupService()
	s.setupTestData()
}

func (s *AuthServiceSuite) setupService() {
	s.userRepo = s.GetStores().UserRepo.(*testutil.InMemoryUserStore)

	s.authService = NewAuthService(
		s.GetConfig(),
		s.userRepo,
		s.GetStores().AuthRepo,
		s.GetStores().TenantRepo,
		s.GetStores().EnvironmentRepo,
		s.GetLogger(),
		s.GetDB(),
	)
}

func (s *AuthServiceSuite) setupTestData() {
	// Clear any existing data
	s.BaseServiceTestSuite.ClearStores()
}

func (s *AuthServiceSuite) TestSignUp() {
	testCases := []struct {
		name          string
		req           *dto.SignUpRequest
		setupFunc     func()
		expectedError bool
	}{
		{
			name: "successful_signup",
			req: &dto.SignUpRequest{
				Email:    "test@example.com",
				Password: "securepassword",
			},
			setupFunc:     nil,
			expectedError: false,
		},
		{
			name: "duplicate_email",
			req: &dto.SignUpRequest{
				Email:    "existing@example.com",
				Password: "securepassword",
			},
			setupFunc: func() {
				// Create an existing user to trigger a duplicate scenario
				_ = s.userRepo.Create(s.GetContext(), &user.User{
					ID:    "user-1",
					Email: "existing@example.com",
				})
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			if tc.setupFunc != nil {
				tc.setupFunc()
			}

			resp, err := s.authService.SignUp(s.GetContext(), tc.req)

			if tc.expectedError {
				s.Error(err)
				s.Nil(resp)
			} else {
				s.NoError(err)
				s.NotNil(resp)
				// We used a real provider, so check that token exists (not necessarily 'auth-token' as before)
				s.NotEmpty(resp.Token)
			}
		})
	}
}
