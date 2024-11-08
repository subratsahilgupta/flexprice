package router

import (
	"github.com/flexprice/flexprice/internal/api"
	"github.com/gin-gonic/gin"
)

func SetupRouter(handler *api.Handler, meterHandler *api.MeterHandler) *gin.Engine {
	router := gin.Default()

	// Existing routes
	router.POST("/events", handler.IngestEvent)
	router.GET("/usage", handler.GetUsage)

	// Meter routes
	router.POST("/meters", meterHandler.CreateMeter)
	router.GET("/meters", meterHandler.GetAllMeters)
	router.PUT("/meters/:id/disable", meterHandler.DisableMeter)

	return router
}
