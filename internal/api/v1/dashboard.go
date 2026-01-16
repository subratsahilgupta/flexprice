package v1

import (
	"net/http"
	"strconv"

	"github.com/flexprice/flexprice/internal/api/dto"
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
	// Get window_count from query parameter only
	windowCount := c.Query("window_count")
	if windowCount == "" {
		windowCount = strconv.Itoa(types.DefaultWindowCount)
	}

	// Build request with window_count from query param
	req := dto.DashboardRevenuesRequest{
		RevenueTrend: &dto.RevenueTrendRequest{
			WindowCount: lo.ToPtr(lo.Must(strconv.Atoi(windowCount))),
		},
	}

	response, err := h.dashboardService.GetRevenues(c.Request.Context(), req)
	if err != nil {
		h.logger.Errorw("failed to get dashboard revenues", "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}
