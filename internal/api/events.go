package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/flexprice/flexprice/internal/events"
	"github.com/flexprice/flexprice/internal/events/stores/clickhouse"
	"github.com/flexprice/flexprice/internal/kafka"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/gin-gonic/gin"
)

type EventsHandler struct {
	producer *kafka.Producer
	store    *clickhouse.ClickHouseStore
	log      *logger.Logger
}

func NewEventsHandler(
	producer *kafka.Producer,
	store *clickhouse.ClickHouseStore,
	log *logger.Logger,
) *EventsHandler {
	return &EventsHandler{
		producer: producer,
		store:    store,
		log:      log,
	}
}

func (h *EventsHandler) IngestEvent(c *gin.Context) {
	var eventRequest struct {
		ID                 string                 `json:"id"`
		ExternalCustomerID string                 `json:"external_customer_id" binding:"required"`
		TenantID           string                 `json:"tenant_id"`
		EventName          string                 `json:"event_name" binding:"required"`
		Timestamp          time.Time              `json:"timestamp"`
		Properties         map[string]interface{} `json:"properties"`
	}

	if err := c.ShouldBindJSON(&eventRequest); err != nil {
		h.log.Error("Failed to bind JSON", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	event := events.NewEvent(
		eventRequest.ID,
		eventRequest.TenantID,
		eventRequest.ExternalCustomerID,
		eventRequest.EventName,
		eventRequest.Timestamp,
		eventRequest.Properties,
	)

	payload, err := json.Marshal(event)
	if err != nil {
		h.log.Error("Failed to marshal event", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process event"})
		return
	}

	if err := h.producer.PublishWithID("events", payload, event.ID); err != nil {
		h.log.Error("Failed to publish event", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process event"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "Event accepted for processing", "event_id": event.ID})
}

func (h *EventsHandler) GetUsage(c *gin.Context) {
	// TODO: Add tenant_id to the query later
	externalCustomerID := c.Query("external_customer_id")
	eventName := c.Query("event_name")
	propertyName := c.Query("property_name")
	aggregationType := c.Query("aggregation_type")
	startTimeStr := c.Query("start_time")
	endTimeStr := c.Query("end_time")

	if externalCustomerID == "" || eventName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required parameters"})
		return
	}

	if startTimeStr == "" || endTimeStr == "" {
		// Default to last 7 days
		endTimeStr = time.Now().Format(time.RFC3339)
		startTimeStr = time.Now().AddDate(0, 0, -7).Format(time.RFC3339)
	}

	// Parse times
	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start_time format"})
		return
	}

	endTime, err := time.Parse(time.RFC3339, endTimeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid end_time format"})
		return
	}

	// Ensure times are in UTC
	startTime = startTime.UTC()
	endTime = endTime.UTC()

	// Get appropriate aggregator
	aggType := events.AggregationType(aggregationType)
	aggregator := clickhouse.GetAggregator(aggType)
	if aggregator == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid aggregation_type"})
		return
	}

	result, err := h.store.GetUsage(
		c.Request.Context(),
		externalCustomerID,
		eventName,
		propertyName,
		aggregator,
		startTime,
		endTime,
	)

	if err != nil {
		h.log.Error("Failed to get usage", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get usage"})
		return
	}

	c.JSON(http.StatusOK, result)
}
