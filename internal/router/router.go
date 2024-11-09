package router

import (
	"github.com/flexprice/flexprice/internal/api"
	"github.com/gin-gonic/gin"
)

func SetupRouter(eventsHandler *api.EventsHandler, meterHandler *api.MeterHandler) *gin.Engine {
	router := gin.Default()

	// Existing routes
	router.POST("/events", eventsHandler.IngestEvent)
	router.GET("/usage", eventsHandler.GetUsage)

	// Meter routes
	router.POST("/meters", meterHandler.CreateMeter)
	router.GET("/meters", meterHandler.GetAllMeters)
	router.PUT("/meters/:id/disable", meterHandler.DisableMeter)

	return router
}
