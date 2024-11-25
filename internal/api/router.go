package api

import (
	"github.com/flexprice/flexprice/docs/swagger"
	v1 "github.com/flexprice/flexprice/internal/api/v1"
	"github.com/flexprice/flexprice/internal/rest/middleware"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

type Handlers struct {
	Events *v1.EventsHandler
	Meter  *v1.MeterHandler
}

func NewRouter(handlers Handlers) *gin.Engine {
	router := gin.Default()
	router.Use(
		middleware.RequestIDMiddleware,
		middleware.AuthMiddleware,
	)

	// Add middleware to set swagger host dynamically
	router.Use(func(c *gin.Context) {
		if swagger.SwaggerInfo != nil {
			swagger.SwaggerInfo.Host = c.Request.Host
		}
		c.Next()
	})

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// v1 routes
	v1Group := router.Group("/v1")
	registerV1Routes(v1Group, handlers)

	return router
}

func registerV1Routes(router *gin.RouterGroup, handlers Handlers) {
	// Events routes
	events := router.Group("/events")
	{
		events.POST("", handlers.Events.IngestEvent)
		events.GET("/usage", handlers.Events.GetUsage)
	}

	// Meter routes
	meters := router.Group("/meters")
	{
		meters.POST("", handlers.Meter.CreateMeter)
		meters.GET("", handlers.Meter.GetAllMeters)
		meters.GET("/:id", handlers.Meter.GetMeter)
		meters.POST("/:id/disable", handlers.Meter.DisableMeter)
	}
}
