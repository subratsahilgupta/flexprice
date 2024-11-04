package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	"github.com/flexprice/flexprice/internal/api"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/kafka"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/meter"
	"github.com/flexprice/flexprice/internal/storage"
	"github.com/gin-gonic/gin"
)

func main() {
	// Initialize logger
	logger.Init()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize ClickHouse storage
	clickhouseStorage, err := storage.NewClickHouseStorage(cfg.ClickHouse.GetClientOptions())
	if err != nil {
		logger.Log.Fatalf("Failed to create ClickHouse storage: %v", err)
	}
	defer clickhouseStorage.Close()

	// Initialize Kafka producer
	kafkaProducer, err := kafka.NewProducer(cfg.Kafka.Brokers)
	if err != nil {
		logger.Log.Fatalf("Failed to create Kafka producer: %v", err)
	}
	defer kafkaProducer.Close()

	// Initialize Kafka consumer
	kafkaConsumer, err := kafka.NewConsumer(cfg.Kafka.Brokers, cfg.Kafka.ConsumerGroup)
	if err != nil {
		logger.Log.Fatalf("Failed to create Kafka consumer: %v", err)
	}
	defer kafkaConsumer.Close()

	// Initialize meter registry
	meterRegistry := meter.NewMeterRegistry()

	// Set up API handlers
	handler := api.NewHandler(kafkaProducer, meterRegistry, clickhouseStorage)

	// Set up Gin router
	router := gin.Default()
	router.POST("/events", handler.IngestEvent)
	router.GET("/usage", handler.GetUsage)

	// Start the server
	go func() {
		if err := router.Run(cfg.Server.Address); err != nil {
			logger.Log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Start consuming messages
	go consumeMessages(kafkaConsumer, clickhouseStorage, cfg.Kafka.Topic)

	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Log.Info("Shutting down server...")
}

func consumeMessages(consumer *kafka.Consumer, storageObj *storage.ClickHouseStorage, topic string) {
	messages, err := consumer.Subscribe(topic)
	if err != nil {
		logger.Log.Fatalf("Failed to subscribe to topic: %v", err)
	}

	for msg := range messages {
		var event storage.Event
		logger.Log.Debugf("received message - %+v", msg)
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			logger.Log.Errorf("Failed to unmarshal event: %v : error - %v ", string(msg.Payload), err)
			continue
		}

		if err := storageObj.InsertEvent(context.Background(), event); err != nil {
			logger.Log.Errorf("Failed to insert event: %v", err)
		}

		msg.Ack()
	}
}
