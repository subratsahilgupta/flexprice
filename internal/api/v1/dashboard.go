package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type DashboardHandler struct {
	dashboardService service.DashboardService
	logger           *logger.Logger
}

func NewDashboardHandler(
	dashboardService service.DashboardService,
	logger *logger.Logger,
) *DashboardHandler {
	return &DashboardHandler{
		dashboardService: dashboardService,
		logger:           logger,
	}
}

func (h *DashboardHandler) GetRevenues(c *gin.Context) {
	var req dto.DashboardRevenuesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).Mark(ierr.ErrValidation))
		return
	}

	// Set defaults
	if req.RevenueTrend == nil {
		req.RevenueTrend = &dto.RevenueTrendRequest{
			WindowSize:  types.DefaultWindowSize,
			WindowCount: lo.ToPtr(types.DefaultWindowCount),
		}
	}

	// Validate request
	if err := req.Validate(); err != nil {
		c.Error(err)
		return
	}

	response, err := h.dashboardService.GetRevenues(c.Request.Context(), req)
	if err != nil {
		h.logger.Errorw("failed to get dashboard revenues", "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}
