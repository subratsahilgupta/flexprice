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
		h.log.Errorw("invalid request payload", "error", err)
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request payload"})
		return
	}

	// Set default duration if not provided
	if req.Duration <= 0 {
		h.log.Debugw("setting default duration",
			"customer_id", req.CustomerID,
			"feature_id", req.FeatureID,
			"subscription_id", req.SubscriptionID,
			"default_duration", 60,
		)
		req.Duration = 60
	}

	// Log which mode we're using
	if req.SubscriptionID != "" {
		h.log.Infow("generating events using subscription ID",
			"subscription_id", req.SubscriptionID,
			"duration", req.Duration,
		)

		// If both subscription ID and customer/feature IDs are provided, log a warning
		if req.CustomerID != "" && req.FeatureID != "" {
			h.log.Warnw("both subscription ID and customer/feature IDs provided, using subscription ID",
				"subscription_id", req.SubscriptionID,
				"customer_id", req.CustomerID,
				"feature_id", req.FeatureID,
			)
		}
	} else {
		h.log.Infow("generating events using customer and feature IDs",
			"customer_id", req.CustomerID,
			"feature_id", req.FeatureID,
			"duration", req.Duration,
		)
	}

	response, err := h.onboardingService.GenerateEvents(ctx, &req)
	if err != nil {
		h.log.Errorw("Failed to generate events",
			"error", err,
			"subscription_id", req.SubscriptionID,
			"customer_id", req.CustomerID,
			"feature_id", req.FeatureID,
		)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to generate events"})
		return
	}

	c.JSON(http.StatusAccepted, response)
}
