package main

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/fx"

	"github.com/flexprice/flexprice/internal/api"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/events"
	"github.com/flexprice/flexprice/internal/events/stores/clickhouse"
	"github.com/flexprice/flexprice/internal/kafka"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/meter"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/router"
	"github.com/gin-gonic/gin"
)

func init() {
	// Set UTC timezone for the entire application
	time.Local = time.UTC
}

func main() {
	app := fx.New(
		fx.Provide(
			// Key components
			config.NewConfig,
			logger.NewLogger,

			// Databases
			postgres.NewDB,
			clickhouse.NewClickHouseStore,

			// Kafka
			kafka.NewProducer,
			kafka.NewConsumer,

			// Repositories
			meter.NewRepository,

			// Services
			meter.NewService,

			// Handlers
			api.NewEventsHandler,
			api.NewMeterHandler,

			// Router
			router.SetupRouter,
		),
		fx.Invoke(startServer),
	)

	app.Run()
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
