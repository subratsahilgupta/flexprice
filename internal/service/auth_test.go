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
	stores := s.GetStores()
	s.userRepo = stores.UserRepo.(*testutil.InMemoryUserStore)
	pubSub := testutil.NewInMemoryPubSub()

	s.authService = NewAuthService(ServiceParams{
		Logger:           s.GetLogger(),
		Config:           s.GetConfig(),
		DB:               s.GetDB(),
		SubRepo:          stores.SubscriptionRepo,
		PlanRepo:         stores.PlanRepo,
		PriceRepo:        stores.PriceRepo,
		EventRepo:        stores.EventRepo,
		MeterRepo:        stores.MeterRepo,
		CustomerRepo:     stores.CustomerRepo,
		InvoiceRepo:      stores.InvoiceRepo,
		EntitlementRepo:  stores.EntitlementRepo,
		EnvironmentRepo:  stores.EnvironmentRepo,
		FeatureRepo:      stores.FeatureRepo,
		TenantRepo:       stores.TenantRepo,
		UserRepo:         stores.UserRepo,
		AuthRepo:         stores.AuthRepo,
		EventPublisher:   s.GetPublisher(),
		WebhookPublisher: s.GetWebhookPublisher(),
	}, pubSub)
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
