package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/gin-gonic/gin"
)

// OnboardingHandler handles onboarding-related API endpoints
type OnboardingHandler struct {
	onboardingService service.OnboardingService
	log               *logger.Logger
}

// NewOnboardingHandler creates a new onboarding handler
func NewOnboardingHandler(onboardingService service.OnboardingService, log *logger.Logger) *OnboardingHandler {
	return &OnboardingHandler{
		onboardingService: onboardingService,
		log:               log,
	}
}

func (h *OnboardingHandler) GenerateEvents(c *gin.Context) {
	ctx := c.Request.Context()
	var req dto.OnboardingEventsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Error("Failed to bind JSON", "error", err)
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request payload"})
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Set default duration if not provided
	if req.Duration <= 0 {
		h.log.Debugw("setting default duration",
			"customer_id", req.CustomerID,
			"feature_id", req.FeatureID,
			"default_duration", 60,
		)
		req.Duration = 60
	}

	response, err := h.onboardingService.GenerateEvents(ctx, &req)
	if err != nil {
		h.log.Error("Failed to generate events", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to generate events"})
		return
	}

	c.JSON(http.StatusAccepted, response)
}
