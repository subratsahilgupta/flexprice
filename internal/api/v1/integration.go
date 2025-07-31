package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/gin-gonic/gin"
)

// IntegrationHandler handles integration API requests
type IntegrationHandler struct {
	IntegrationService service.IntegrationService
	logger             *logger.Logger
}

// NewIntegrationHandler creates a new integration handler
func NewIntegrationHandler(
	integrationService service.IntegrationService,
	logger *logger.Logger,
) *IntegrationHandler {
	return &IntegrationHandler{
		IntegrationService: integrationService,
		logger:             logger,
	}
}

// SyncCustomerToProviders godoc
// @Summary Sync customer to all available providers
// @Description Sync a customer to all available payment providers for the current tenant
// @Tags Integration
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param customer_id path string true "Customer ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} errors.ErrorResponse
// @Failure 401 {object} errors.ErrorResponse
// @Failure 404 {object} errors.ErrorResponse
// @Failure 500 {object} errors.ErrorResponse
// @Router /integration/sync-customer/{customer_id} [post]
func (h *IntegrationHandler) SyncCustomerToProviders(c *gin.Context) {
	customerID := c.Param("customer_id")
	if customerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Customer ID is required",
		})
		return
	}

	ctx := c.Request.Context()
	err := h.IntegrationService.SyncCustomerToProviders(ctx, customerID)
	if err != nil {
		h.logger.Errorw("failed to sync customer to providers",
			"customer_id", customerID,
			"error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to sync customer to providers",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Customer sync initiated successfully",
		"customer_id": customerID,
	})
}

// GetAvailableProviders godoc
// @Summary Get available providers
// @Description Get all available payment providers for the current tenant
// @Tags Integration
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} dto.ListConnectionsResponse
// @Failure 401 {object} errors.ErrorResponse
// @Failure 500 {object} errors.ErrorResponse
// @Router /integration/providers [get]
func (h *IntegrationHandler) GetAvailableProviders(c *gin.Context) {
	ctx := c.Request.Context()
	providers, err := h.IntegrationService.GetAvailableProviders(ctx)
	if err != nil {
		h.logger.Errorw("failed to get available providers", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get available providers",
		})
		return
	}

	// Convert to response format
	connections := make([]dto.ConnectionResponse, 0, len(providers))
	for _, provider := range providers {
		connections = append(connections, dto.ConnectionResponse{
			ID:            provider.ID,
			Name:          provider.Name,
			ProviderType:  provider.ProviderType,
			Metadata:      provider.Metadata,
			EnvironmentID: provider.EnvironmentID,
			TenantID:      provider.TenantID,
			Status:        provider.Status,
			CreatedAt:     provider.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:     provider.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			CreatedBy:     provider.CreatedBy,
			UpdatedBy:     provider.UpdatedBy,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"providers": connections,
		"total":     len(connections),
	})
}
