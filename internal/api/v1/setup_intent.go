package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/gin-gonic/gin"
)

type SetupIntentHandler struct {
	stripeService *service.StripeService
	log           *logger.Logger
}

func NewSetupIntentHandler(stripeService *service.StripeService, log *logger.Logger) *SetupIntentHandler {
	return &SetupIntentHandler{
		stripeService: stripeService,
		log:           log,
	}
}

// @Summary Create a Setup Intent session
// @Description Create a Setup Intent with checkout session for saving payment methods (supports multiple payment providers)
// @Tags Setup Intents
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param setup_intent body dto.CreateSetupIntentRequest true "Setup Intent configuration"
// @Success 201 {object} dto.SetupIntentResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /setup-intents/sessions [post]
func (h *SetupIntentHandler) CreateSetupIntentSession(c *gin.Context) {
	var req dto.CreateSetupIntentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Error("Failed to bind JSON", "error", err)
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	// Validate the request
	if err := req.Validate(); err != nil {
		h.log.Error("Setup Intent request validation failed", "error", err)
		c.Error(err)
		return
	}

	resp, err := h.stripeService.SetupIntent(c.Request.Context(), &req)
	if err != nil {
		h.log.Error("Failed to create Setup Intent", "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, resp)
}
