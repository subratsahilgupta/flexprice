package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/dto"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/meter"
	"github.com/gin-gonic/gin"
)

type MeterHandler struct {
	service meter.Service
	log     *logger.Logger
}

func NewMeterHandler(service meter.Service, log *logger.Logger) *MeterHandler {
	return &MeterHandler{service: service, log: log}
}

// @Summary Create meter
// @Description Create a new meter with the specified configuration
// @Tags meters
// @Accept json
// @Produce json
// @Param meter body dto.CreateMeterRequest true "Meter configuration"
// @Success 201 {object} dto.MeterResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /meters [post]
func (h *MeterHandler) CreateMeter(c *gin.Context) {
	var req dto.CreateMeterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request payload"})
		return
	}

	meter := req.ToMeter(c.GetString("user_id"))
	if err := h.service.CreateMeter(c.Request.Context(), meter); err != nil {
		h.log.Error("Failed to create meter", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create meter"})
		return
	}

	c.JSON(http.StatusCreated, dto.ToMeterResponse(meter))
}

// @Summary List meters
// @Description Get all active meters
// @Tags meters
// @Produce json
// @Success 200 {array} dto.MeterResponse
// @Failure 500 {object} ErrorResponse
// @Router /meters [get]
func (h *MeterHandler) GetAllMeters(c *gin.Context) {
	meters, err := h.service.GetAllMeters(c.Request.Context())
	if err != nil {
		h.log.Error("Failed to get meters", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get meters"})
		return
	}

	response := make([]*dto.MeterResponse, len(meters))
	for i, m := range meters {
		response[i] = dto.ToMeterResponse(m)
	}
	c.JSON(http.StatusOK, response)
}

// @Summary Get meter
// @Description Get a specific meter by ID
// @Tags meters
// @Produce json
// @Param id path string true "Meter ID"
// @Success 200 {object} dto.MeterResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /meters/{id} [get]
func (h *MeterHandler) GetMeter(c *gin.Context) {
	id := c.Param("id")
	meter, err := h.service.GetMeter(c.Request.Context(), id)
	if err != nil {
		h.log.Error("Failed to get meter", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get meter"})
		return
	}
	c.JSON(http.StatusOK, dto.ToMeterResponse(meter))
}

// @Summary Disable meter
// @Description Disable an existing meter
// @Tags meters
// @Produce json
// @Param id path string true "Meter ID"
// @Success 200 {object} map[string]string "message:Meter disabled successfully"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /meters/{id}/disable [post]
func (h *MeterHandler) DisableMeter(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.DisableMeter(c.Request.Context(), id); err != nil {
		h.log.Error("Failed to disable meter", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to disable meter"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Meter disabled successfully"})
}
