package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type SecretHandler struct {
	service service.SecretService
	logger  *logger.Logger
}

func NewSecretHandler(service service.SecretService, logger *logger.Logger) *SecretHandler {
	return &SecretHandler{
		service: service,
		logger:  logger,
	}
}

// ListAPIKeys godoc
// @Summary List API keys
// @Description Get a paginated list of API keys
// @Tags secrets
// @Accept json
// @Produce json
// @Param limit query int false "Limit"
// @Param offset query int false "Offset"
// @Param status query string false "Status (published/archived)"
// @Success 200 {object} dto.ListSecretsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /secrets/api/keys [get]
func (h *SecretHandler) ListAPIKeys(c *gin.Context) {
	filter := &types.SecretFilter{
		QueryFilter: types.NewDefaultQueryFilter(),
		Type:        lo.ToPtr(types.SecretTypePrivateKey),
		Provider:    lo.ToPtr(types.SecretProviderFlexPrice),
	}

	if err := c.ShouldBindQuery(filter); err != nil {
		h.logger.Errorw("failed to bind query", "error", err)
		NewErrorResponse(c, http.StatusBadRequest, "failed to bind query", err)
		return
	}

	secrets, err := h.service.ListAPIKeys(c.Request.Context(), filter)
	if err != nil {
		h.logger.Errorw("failed to list secrets", "error", err)
		NewErrorResponse(c, http.StatusInternalServerError, "failed to list secrets", err)
		return
	}

	c.JSON(http.StatusOK, secrets)
}

// CreateAPIKey godoc
// @Summary Create a new API key
// @Description Create a new API key with the specified type and permissions
// @Tags secrets
// @Accept json
// @Produce json
// @Param request body dto.CreateAPIKeyRequest true "API key creation request"
// @Success 201 {object} dto.CreateAPIKeyResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /secrets/api/keys [post]
func (h *SecretHandler) CreateAPIKey(c *gin.Context) {
	var req dto.CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Errorw("failed to bind request", "error", err)
		NewErrorResponse(c, http.StatusBadRequest, "failed to bind request", err)
		return
	}

	secret, apiKey, err := h.service.CreateAPIKey(c.Request.Context(), &req)
	if err != nil {
		h.logger.Errorw("failed to create api key", "error", err)
		NewErrorResponse(c, http.StatusInternalServerError, "failed to create api key", err)
		return
	}

	c.JSON(http.StatusCreated, dto.CreateAPIKeyResponse{
		Secret: *dto.ToSecretResponse(secret),
		APIKey: apiKey,
	})
}

// DeleteAPIKey godoc
// @Summary Delete an API key
// @Description Delete an API key by ID
// @Tags secrets
// @Accept json
// @Produce json
// @Param id path string true "API key ID"
// @Success 204 "No Content"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /secrets/api/keys/{id} [delete]
func (h *SecretHandler) DeleteAPIKey(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		h.logger.Errorw("failed to delete api key", "error", err)
		NewErrorResponse(c, http.StatusInternalServerError, "failed to delete api key", err)
		return
	}

	c.Status(http.StatusNoContent)
}

// CreateIntegration godoc
// @Summary Create or update an integration
// @Description Create or update integration credentials
// @Tags secrets
// @Accept json
// @Produce json
// @Param provider path string true "Integration provider"
// @Param request body dto.CreateIntegrationRequest true "Integration creation request"
// @Success 201 {object} dto.SecretResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /secrets/integrations/{provider} [post]
func (h *SecretHandler) CreateIntegration(c *gin.Context) {
	provider := types.SecretProvider(c.Param("provider"))
	var req dto.CreateIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Errorw("failed to bind request", "error", err)
		NewErrorResponse(c, http.StatusBadRequest, "failed to bind request", err)
		return
	}

	req.Provider = provider
	secret, err := h.service.CreateIntegration(c.Request.Context(), &req)
	if err != nil {
		h.logger.Errorw("failed to create integration", "error", err)
		NewErrorResponse(c, http.StatusInternalServerError, "failed to create integration", err)
		return
	}

	c.JSON(http.StatusCreated, dto.ToSecretResponse(secret))
}

// GetIntegration godoc
// @Summary Get integration details
// @Description Get details of a specific integration
// @Tags secrets
// @Accept json
// @Produce json
// @Param provider path string true "Integration provider"
// @Success 200 {object} dto.SecretResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /secrets/integrations/{provider} [get]
func (h *SecretHandler) GetIntegration(c *gin.Context) {
	provider := types.SecretProvider(c.Param("provider"))
	filter := &types.SecretFilter{
		QueryFilter: types.NewDefaultQueryFilter(),
		Type:        lo.ToPtr(types.SecretTypeIntegration),
		Provider:    lo.ToPtr(provider),
	}

	secrets, err := h.service.ListIntegrations(c.Request.Context(), filter)
	if err != nil {
		h.logger.Errorw("failed to list secrets", "error", err)
		NewErrorResponse(c, http.StatusInternalServerError, "failed to list secrets", err)
		return
	}

	c.JSON(http.StatusOK, secrets.Items)
}

// DeleteIntegration godoc
// @Summary Delete an integration
// @Description Delete integration credentials
// @Tags secrets
// @Accept json
// @Produce json
// @Param provider path string true "Integration provider"
// @Success 204 "No Content"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /secrets/integrations/{provider} [delete]
func (h *SecretHandler) DeleteIntegration(c *gin.Context) {
	provider := types.SecretProvider(c.Param("provider"))
	filter := &types.SecretFilter{
		QueryFilter: types.NewDefaultQueryFilter(),
		Type:        lo.ToPtr(types.SecretTypeIntegration),
		Provider:    lo.ToPtr(provider),
	}

	secrets, err := h.service.ListIntegrations(c.Request.Context(), filter)
	if err != nil {
		h.logger.Errorw("failed to list secrets", "error", err)
		NewErrorResponse(c, http.StatusInternalServerError, "failed to list secrets", err)
		return
	}

	if len(secrets.Items) == 0 {
		h.logger.Errorw("secret not found", "error", errors.ErrNotFound)
		NewErrorResponse(c, http.StatusNotFound, "secret not found", errors.ErrNotFound)
		return
	}

	if err := h.service.Delete(c.Request.Context(), secrets.Items[0].ID); err != nil {
		h.logger.Errorw("failed to delete integration", "error", err)
		NewErrorResponse(c, http.StatusInternalServerError, "failed to delete integration", err)
		return
	}

	c.Status(http.StatusNoContent)
}
