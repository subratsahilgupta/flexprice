package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/gin-gonic/gin"
)

type SettingsHandler struct {
	service service.SettingsService
	log     *logger.Logger
}

func NewSettingsHandler(
	service service.SettingsService,
	log *logger.Logger,
) *SettingsHandler {
	return &SettingsHandler{
		service: service,
		log:     log,
	}
}

func (h *SettingsHandler) GetSettingByKey(c *gin.Context) {
	key := c.Param("key")

	resp, err := h.service.GetSettingByKey(c.Request.Context(), key)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *SettingsHandler) UpdateSettingByKey(c *gin.Context) {
	key := c.Param("key")

	var req dto.UpdateSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.service.UpdateSettingByKey(c.Request.Context(), key, &req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *SettingsHandler) DeleteSettingByKey(c *gin.Context) {
	key := c.Param("key")

	err := h.service.DeleteSettingByKey(c.Request.Context(), key)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Setting deleted successfully"})
}
