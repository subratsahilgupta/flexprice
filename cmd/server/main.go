package main

import (
	"context"
	"encoding/json"

	"go.uber.org/fx"

	"github.com/flexprice/flexprice/internal/api"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/kafka"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/meter"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/router"
	"github.com/flexprice/flexprice/internal/storage"
	"github.com/gin-gonic/gin"
)

func main() {
	app := fx.New(
		fx.Provide(
			config.NewConfig,
			logger.NewLogger,
			postgres.NewDB,
			storage.NewClickHouseStorage,
			kafka.NewProducer,
			kafka.NewConsumer,
			meter.NewRepository,
			meter.NewService,
			api.NewMeterHandler,
			api.NewHandler,
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
	storageObj *storage.ClickHouseStorage,
	log *logger.Logger,
) {
	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				if err := r.Run(cfg.Server.Address); err != nil {
					log.Fatalf("Failed to start server: %v", err)
				}
			}()
			go consumeMessages(consumer, storageObj, cfg.Kafka.Topic, log)
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
	storageObj *storage.ClickHouseStorage,
	topic string,
	log *logger.Logger,
) {
	messages, err := consumer.Subscribe(topic)
	if err != nil {
		log.Fatalf("Failed to subscribe to topic: %v", err)
	}

	for msg := range messages {
		var event storage.Event
		log.Debugf("received message - %+v", msg)
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			log.Errorf("Failed to unmarshal event: %v : error - %v ", string(msg.Payload), err)
			continue
		}

		if err := storageObj.InsertEvent(context.Background(), event); err != nil {
			log.Errorf("Failed to insert event: %v", err)
		}

		msg.Ack()
	}
}
