package service

import (
	"context"
	"testing"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type EntityIntegrationMappingServiceSuite struct {
	testutil.BaseServiceTestSuite
	service EntityIntegrationMappingService
}

func TestEntityIntegrationMappingService(t *testing.T) {
	suite.Run(t, new(EntityIntegrationMappingServiceSuite))
}

func (s *EntityIntegrationMappingServiceSuite) SetupTest() {
	s.BaseServiceTestSuite.SetupTest()
	s.setupService()
}

func (s *EntityIntegrationMappingServiceSuite) setupService() {
	stores := s.GetStores()
	s.service = NewEntityIntegrationMappingService(ServiceParams{
		Logger:                       s.GetLogger(),
		Config:                       s.GetConfig(),
		DB:                           s.GetDB(),
		EntityIntegrationMappingRepo: stores.EntityIntegrationMappingRepo,
		WebhookPublisher:             s.GetWebhookPublisher(),
	})
}

func (s *EntityIntegrationMappingServiceSuite) TestCreateEntityIntegrationMapping() {
	// Test data
	req := dto.CreateEntityIntegrationMappingRequest{
		EntityID:         "cust_123",
		EntityType:       "customer",
		ProviderType:     "stripe",
		ProviderEntityID: "cus_stripe_456",
		Metadata: map[string]interface{}{
			"stripe_customer_email": "test@example.com",
		},
	}

	// Execute
	ctx := types.SetTenantID(context.Background(), "test_tenant")
	ctx = types.SetEnvironmentID(ctx, "test_env")
	ctx = types.SetUserID(ctx, "test_user")

	resp, err := s.service.CreateEntityIntegrationMapping(ctx, req)

	// Assert
	require.NoError(s.T(), err)
	assert.NotNil(s.T(), resp)
	assert.NotNil(s.T(), resp.EntityIntegrationMapping)
	assert.Equal(s.T(), req.EntityID, resp.EntityIntegrationMapping.EntityID)
	assert.Equal(s.T(), req.EntityType, resp.EntityIntegrationMapping.EntityType)
	assert.Equal(s.T(), req.ProviderType, resp.EntityIntegrationMapping.ProviderType)
	assert.Equal(s.T(), req.ProviderEntityID, resp.EntityIntegrationMapping.ProviderEntityID)
	assert.Equal(s.T(), req.Metadata, resp.EntityIntegrationMapping.Metadata)
	assert.NotEmpty(s.T(), resp.EntityIntegrationMapping.ID)
	assert.Equal(s.T(), "test_tenant", resp.EntityIntegrationMapping.TenantID)
	assert.Equal(s.T(), "test_env", resp.EntityIntegrationMapping.EnvironmentID)
}

func (s *EntityIntegrationMappingServiceSuite) TestGetEntityIntegrationMapping() {
	// Create a mapping first
	req := dto.CreateEntityIntegrationMappingRequest{
		EntityID:         "cust_123",
		EntityType:       "customer",
		ProviderType:     "stripe",
		ProviderEntityID: "cus_stripe_456",
	}

	ctx := types.SetTenantID(context.Background(), "test_tenant")
	ctx = types.SetEnvironmentID(ctx, "test_env")
	ctx = types.SetUserID(ctx, "test_user")

	created, err := s.service.CreateEntityIntegrationMapping(ctx, req)
	require.NoError(s.T(), err)

	// Get the mapping
	resp, err := s.service.GetEntityIntegrationMapping(ctx, created.EntityIntegrationMapping.ID)

	// Assert
	require.NoError(s.T(), err)
	assert.NotNil(s.T(), resp)
	assert.Equal(s.T(), created.EntityIntegrationMapping.ID, resp.EntityIntegrationMapping.ID)
	assert.Equal(s.T(), req.EntityID, resp.EntityIntegrationMapping.EntityID)
	assert.Equal(s.T(), req.EntityType, resp.EntityIntegrationMapping.EntityType)
	assert.Equal(s.T(), req.ProviderType, resp.EntityIntegrationMapping.ProviderType)
	assert.Equal(s.T(), req.ProviderEntityID, resp.EntityIntegrationMapping.ProviderEntityID)
}

func (s *EntityIntegrationMappingServiceSuite) TestGetByEntityAndProvider() {
	// Create a mapping first
	req := dto.CreateEntityIntegrationMappingRequest{
		EntityID:         "cust_123",
		EntityType:       "customer",
		ProviderType:     "stripe",
		ProviderEntityID: "cus_stripe_456",
	}

	ctx := types.SetTenantID(context.Background(), "test_tenant")
	ctx = types.SetEnvironmentID(ctx, "test_env")
	ctx = types.SetUserID(ctx, "test_user")

	_, err := s.service.CreateEntityIntegrationMapping(ctx, req)
	require.NoError(s.T(), err)

	// Get by entity and provider
	resp, err := s.service.GetByEntityAndProvider(ctx, "cust_123", "customer", "stripe")

	// Assert
	require.NoError(s.T(), err)
	assert.NotNil(s.T(), resp)
	assert.Equal(s.T(), "cust_123", resp.EntityIntegrationMapping.EntityID)
	assert.Equal(s.T(), "customer", resp.EntityIntegrationMapping.EntityType)
	assert.Equal(s.T(), "stripe", resp.EntityIntegrationMapping.ProviderType)
	assert.Equal(s.T(), "cus_stripe_456", resp.EntityIntegrationMapping.ProviderEntityID)
}

func (s *EntityIntegrationMappingServiceSuite) TestGetByProviderEntity() {
	// Create a mapping first
	req := dto.CreateEntityIntegrationMappingRequest{
		EntityID:         "cust_123",
		EntityType:       "customer",
		ProviderType:     "stripe",
		ProviderEntityID: "cus_stripe_456",
	}

	ctx := types.SetTenantID(context.Background(), "test_tenant")
	ctx = types.SetEnvironmentID(ctx, "test_env")
	ctx = types.SetUserID(ctx, "test_user")

	_, err := s.service.CreateEntityIntegrationMapping(ctx, req)
	require.NoError(s.T(), err)

	// Get by provider entity
	resp, err := s.service.GetByProviderEntity(ctx, "stripe", "cus_stripe_456")

	// Assert
	require.NoError(s.T(), err)
	assert.NotNil(s.T(), resp)
	assert.Equal(s.T(), "cust_123", resp.EntityIntegrationMapping.EntityID)
	assert.Equal(s.T(), "customer", resp.EntityIntegrationMapping.EntityType)
	assert.Equal(s.T(), "stripe", resp.EntityIntegrationMapping.ProviderType)
	assert.Equal(s.T(), "cus_stripe_456", resp.EntityIntegrationMapping.ProviderEntityID)
}
