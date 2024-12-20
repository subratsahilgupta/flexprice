package service

import (
	"context"
	"testing"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/tenant"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/stretchr/testify/suite"
)

type TenantServiceSuite struct {
	suite.Suite
	ctx           context.Context
	tenantService *TenantService
	tenantRepo    *testutil.InMemoryTenantRepository
}

func TestTenantService(t *testing.T) {
	suite.Run(t, new(TenantServiceSuite))
}

func (s *TenantServiceSuite) SetupTest() {
	// Initialize context and repository
	s.ctx = testutil.SetupContext()
	s.tenantRepo = testutil.NewInMemoryTenantStore()

	// Create the tenantService with the repository
	s.tenantService = NewTenantService(s.tenantRepo)
}

func (s *TenantServiceSuite) TestGetTenantByID() {
	testCases := []struct {
		name          string
		setup         func(ctx context.Context)
		tenantID      string
		expectedError bool
		expectedName  string
	}{
		{
			name: "tenant_found",
			setup: func(ctx context.Context) {
				_ = s.tenantRepo.Create(ctx, &tenant.Tenant{
					ID:   "tenant-1",
					Name: "Test Tenant",
				})
			},
			tenantID:      "tenant-1",
			expectedError: false,
			expectedName:  "Test Tenant",
		},
		{
			name:          "tenant_not_found",
			setup:         nil,
			tenantID:      "nonexistent-id",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Reset repository and service for each test
			s.tenantRepo = testutil.NewInMemoryTenantStore()
			s.tenantService = NewTenantService(s.tenantRepo)

			// Create a context for the test
			ctx := testutil.SetupContext()

			// Execute setup function if provided
			if tc.setup != nil {
				tc.setup(ctx)
			}

			// Call the service method
			resp, err := s.tenantService.GetTenantByID(ctx, tc.tenantID)

			// Assert results
			if tc.expectedError {
				s.Error(err)
				s.Nil(resp)
			} else {
				s.NoError(err)
				s.NotNil(resp)
				s.Equal(tc.tenantID, resp.ID)
				s.Equal(tc.expectedName, resp.Name)
			}
		})
	}
}

func (s *TenantServiceSuite) TestCreateTenant() {
	testCases := []struct {
		name          string
		request       dto.CreateTenantRequest
		expectedError bool
		expectedName  string
	}{
		{
			name: "valid_tenant",
			request: dto.CreateTenantRequest{
				Name: "New Tenant",
			},
			expectedError: false,
			expectedName:  "New Tenant",
		},
		{
			name: "invalid_tenant",
			request: dto.CreateTenantRequest{
				Name: "",
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Reset repository and service for each test
			s.tenantRepo = testutil.NewInMemoryTenantStore()
			s.tenantService = NewTenantService(s.tenantRepo)

			// Create a context for the test
			ctx := testutil.SetupContext()

			// Call the service method
			resp, err := s.tenantService.CreateTenant(ctx, tc.request)

			// Assert results
			if tc.expectedError {
				s.Error(err)
				s.Nil(resp)
			} else {
				s.NoError(err)
				s.NotNil(resp)
				s.Equal(tc.expectedName, resp.Name)
			}
		})
	}
}
