package api

import (
	"net/http"
	"github.com/flexprice/flexprice/internal/meter"
	"github.com/gin-gonic/gin"
)

type MeterHandler struct {
	service meter.Service
}

func NewMeterHandler(service meter.Service) *MeterHandler {
	return &MeterHandler{service: service}
}

func (h *MeterHandler) CreateMeter(c *gin.Context) {
	var m meter.Meter
	if err := c.ShouldBindJSON(&m); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}
	if err := h.service.CreateMeter(c.Request.Context(), m); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create meter"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Meter created successfully"})
}

func (h *MeterHandler) GetAllMeters(c *gin.Context) {
	meters, err := h.service.GetAllMeters(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get meters"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"meters": meters})
}

func (h *MeterHandler) DisableMeter(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.DisableMeter(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to disable meter"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Meter disabled successfully"})
} 