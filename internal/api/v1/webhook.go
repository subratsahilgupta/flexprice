package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
)

// WebhookHandler handles webhook-related endpoints
type WebhookHandler struct {
	config      *config.Webhook
	svixService service.SvixService
	logger      *logger.Logger
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(
	cfg *config.Configuration,
	svixService service.SvixService,
	logger *logger.Logger,
) *WebhookHandler {
	return &WebhookHandler{
		config:      &cfg.Webhook,
		svixService: svixService,
		logger:      logger,
	}
}

// GetDashboardURL handles the GET /webhooks/dashboard endpoint
func (h *WebhookHandler) GetDashboardURL(c *gin.Context) {
	if !h.config.Svix.Enabled {
		c.JSON(http.StatusOK, gin.H{
			"url":          "",
			"svix_enabled": false,
		})
		return
	}

	tenantID := types.GetTenantID(c.Request.Context())
	environmentID := types.GetEnvironmentID(c.Request.Context())

	// Get or create Svix application
	appID, err := h.svixService.GetOrCreateApplication(c.Request.Context(), tenantID, environmentID)
	if err != nil {
		h.logger.Errorw("failed to get/create Svix application",
			"error", err,
			"tenant_id", tenantID,
			"environment_id", environmentID,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get webhook dashboard",
		})
		return
	}

	// Get dashboard URL
	url, err := h.svixService.GetDashboardURL(c.Request.Context(), appID)
	if err != nil {
		h.logger.Errorw("failed to get Svix dashboard URL",
			"error", err,
			"tenant_id", tenantID,
			"environment_id", environmentID,
			"app_id", appID,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get webhook dashboard",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":          url,
		"svix_enabled": true,
	})
}
