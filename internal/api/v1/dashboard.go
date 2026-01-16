package v1

import (
	"net/http"
	"strconv"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/gin-gonic/gin"
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
	// Get window_count from query parameter only
	windowCountStr := c.Query("window_count")
	windowCount := 3 // Default to 3 if not provided
	if windowCountStr != "" {
		wc, err := strconv.Atoi(windowCountStr)
		if err != nil || wc <= 0 {
			c.Error(ierr.NewError("invalid window_count query parameter").
				WithHint("window_count must be a positive integer").
				Mark(ierr.ErrValidation))
			return
		}
		windowCount = wc
	}

	// Build request with window_count from query param
	req := dto.DashboardRevenuesRequest{
		RevenueTrend: &dto.RevenueTrendRequest{
			WindowCount: &windowCount,
		},
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
