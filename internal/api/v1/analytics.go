package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/analytics"
	"github.com/flexprice/flexprice/internal/ee/service"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
)

// AnalyticsHandler handles ad-hoc and saved-view analytics query endpoints.
type AnalyticsHandler struct {
	svc service.AnalyticsService
	log *logger.Logger
}

func NewAnalyticsHandler(svc service.AnalyticsService, log *logger.Logger) *AnalyticsHandler {
	return &AnalyticsHandler{svc: svc, log: log}
}

// Query runs an ad-hoc analytics query against a view definition.
// @Summary Run an ad-hoc analytics query
// @Description Resolves the given view definition against the supplied variables and executes it.
// @ID queryAnalytics
// @Tags Analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body dto.AnalyticsQueryRequest true "Analytics query request"
// @Success 200 {object} service.QueryResult
// @Failure 400 {object} ierr.ErrorResponse "Invalid request"
// @Failure 500 {object} ierr.ErrorResponse "Server error"
// @x-scope "read"
// @Router /analytics/query [post]
func (h *AnalyticsHandler) Query(c *gin.Context) {
	ctx := c.Request.Context()

	var req dto.AnalyticsQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Please check the request payload").
			Mark(ierr.ErrValidation))
		return
	}

	if err := req.Validate(); err != nil {
		c.Error(err)
		return
	}

	res, err := h.svc.ExecuteView(ctx, req.Definition, req.Variables)
	if err != nil {
		h.log.Error(ctx, "failed to execute analytics query", "error", err)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, res)
}

// CreateView creates a new saved analytics view.
// @Summary Create a saved analytics view
// @Description Persists a named view definition that can later be queried by ID.
// @ID createAnalyticsSavedView
// @Tags Analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body dto.CreateSavedViewRequest true "Saved view request"
// @Success 201 {object} analytics.SavedView
// @Failure 400 {object} ierr.ErrorResponse "Invalid request"
// @Failure 500 {object} ierr.ErrorResponse "Server error"
// @Router /analytics/views [post]
func (h *AnalyticsHandler) CreateView(c *gin.Context) {
	ctx := c.Request.Context()

	var req dto.CreateSavedViewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Please check the request payload").
			Mark(ierr.ErrValidation))
		return
	}

	if err := req.Validate(); err != nil {
		c.Error(err)
		return
	}

	v := &analytics.SavedView{
		ID:         types.GenerateUUIDWithPrefix(types.UUID_PREFIX_ANALYTICS_SAVED_VIEW),
		Name:       req.Name,
		Definition: req.Definition,
	}

	if err := h.svc.CreateView(ctx, v); err != nil {
		h.log.Error(ctx, "failed to create analytics saved view", "error", err, "saved_view_id", v.ID)
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, v)
}

// QuerySavedView executes a previously saved analytics view by ID.
// @Summary Query a saved analytics view
// @Description Resolves the saved view's definition against the supplied variables and executes it.
// @ID queryAnalyticsSavedView
// @Tags Analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Saved view ID"
// @Param request body dto.SavedViewQueryRequest true "Saved view query request"
// @Success 200 {object} service.QueryResult
// @Failure 400 {object} ierr.ErrorResponse "Invalid request"
// @Failure 404 {object} ierr.ErrorResponse "Saved view not found"
// @Failure 500 {object} ierr.ErrorResponse "Server error"
// @x-scope "read"
// @Router /analytics/views/{id}/query [post]
func (h *AnalyticsHandler) QuerySavedView(c *gin.Context) {
	ctx := c.Request.Context()

	id := c.Param("id")
	if id == "" {
		c.Error(ierr.NewError("saved view id is required").
			WithHint("Provide a valid saved view id").
			Mark(ierr.ErrValidation))
		return
	}

	var req dto.SavedViewQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Please check the request payload").
			Mark(ierr.ErrValidation))
		return
	}

	if err := req.Validate(); err != nil {
		c.Error(err)
		return
	}

	res, err := h.svc.QuerySavedView(ctx, id, req.Variables)
	if err != nil {
		h.log.Error(ctx, "failed to query analytics saved view", "error", err, "saved_view_id", id)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, res)
}
