package main

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/fx"

	_ "github.com/flexprice/flexprice/docs/swagger"
	"github.com/flexprice/flexprice/internal/api"
	v1 "github.com/flexprice/flexprice/internal/api/v1"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/events"
	"github.com/flexprice/flexprice/internal/events/stores/clickhouse"
	"github.com/flexprice/flexprice/internal/kafka"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/meter"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/gin-gonic/gin"
)

// @title FlexPrice API
// @version 1.0
// @description FlexPrice API Service
// @host localhost:8080
// @BasePath /v1
// @schemes http https
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization

func init() {
	// Set UTC timezone for the entire application
	time.Local = time.UTC
}

func main() {
	app := fx.New(
		fx.Provide(
			// Core dependencies
			config.NewConfig,
			logger.NewLogger,
			postgres.NewDB,
			clickhouse.NewClickHouseStore,
			kafka.NewProducer,
			kafka.NewConsumer,

			// Domain services
			meter.NewRepository,
			meter.NewService,

			// Handlers
			provideHandlers,

			// Router
			provideRouter,
		),
		fx.Invoke(startServer),
	)
	app.Run()
}

func provideHandlers(
	producer *kafka.Producer,
	store *clickhouse.ClickHouseStore,
	logger *logger.Logger,
	meterService meter.Service,
) api.Handlers {
	return api.Handlers{
		Events: v1.NewEventsHandler(producer, store, logger),
		Meter:  v1.NewMeterHandler(meterService, logger),
	}
}

func provideRouter(handlers api.Handlers) *gin.Engine {
	return api.NewRouter(handlers)
}

func startServer(
	lifecycle fx.Lifecycle,
	r *gin.Engine,
	cfg *config.Configuration,
	consumer *kafka.Consumer,
	clickhouseObj *clickhouse.ClickHouseStore,
	log *logger.Logger,
) {
	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				if err := r.Run(cfg.Server.Address); err != nil {
					log.Fatalf("Failed to start server: %v", err)
				}
			}()
			go consumeMessages(consumer, clickhouseObj, cfg.Kafka.Topic, log)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("Shutting down server...")
			return nil
		},
	})
}

func consumeMessages(
	consumer *kafka.Consumer,
	clickhouseObj *clickhouse.ClickHouseStore,
	topic string,
	log *logger.Logger,
) {
	messages, err := consumer.Subscribe(topic)
	if err != nil {
		log.Fatalf("Failed to subscribe to topic: %v", err)
	}

	for msg := range messages {
		var event events.Event
		log.Debugf("received message - %+v", msg)
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			log.Errorf("Failed to unmarshal event: %v : error - %v ", string(msg.Payload), err)
			continue
		}

		if err := clickhouseObj.InsertEvent(context.Background(), &event); err != nil {
			log.Errorf("Failed to insert event: %v", err)
		}

		msg.Ack()
	}
}
