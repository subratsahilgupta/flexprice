package v1

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
)

type EventsHandler struct {
	eventService service.EventService
	log          *logger.Logger
}

func NewEventsHandler(eventService service.EventService, log *logger.Logger) *EventsHandler {
	return &EventsHandler{
		eventService: eventService,
		log:          log,
	}
}

// @Summary Ingest event
// @Description Ingest a new event into the system
// @Tags events
// @Accept json
// @Produce json
// @Param event body dto.IngestEventRequest true "Event data"
// @Success 202 {object} map[string]string "message:Event accepted for processing"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /events [post]
func (h *EventsHandler) IngestEvent(c *gin.Context) {
	ctx := c.Request.Context()
	var req dto.IngestEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Error("Failed to bind JSON", "error", err)
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request payload"})
		return
	}

	err := h.eventService.CreateEvent(ctx, &req)
	if err != nil {
		h.log.Error("Failed to ingest event", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to ingest event"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "Event accepted for processing", "event_id": req.EventID})
}

// @Summary Get usage by meter
// @Description Retrieve aggregated usage statistics using meter configuration
// @Tags events
// @Produce json
// @Param meter_id query string true "Meter ID"
// @Param external_customer_id query string false "External Customer ID"
// @Param start_time query string false "Start Time (RFC3339)"
// @Param end_time query string false "End Time (RFC3339)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /events/usage/meter [get]
func (h *EventsHandler) GetUsageByMeter(c *gin.Context) {
	ctx := c.Request.Context()
	externalCustomerID := c.Query("external_customer_id")
	meterID := c.Query("meter_id")
	startTimeStr := c.Query("start_time")
	endTimeStr := c.Query("end_time")
	windowSize := c.Query("window_size")

	if meterID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Missing required parameters: meter_id or customer_id"})
		return
	}

	startTime, endTime, err := parseStartAndEndTime(startTimeStr, endTimeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	result, err := h.eventService.GetUsageByMeter(ctx, &dto.GetUsageByMeterRequest{
		MeterID:            meterID,
		ExternalCustomerID: externalCustomerID,
		StartTime:          startTime.UTC(),
		EndTime:            endTime.UTC(),
		WindowSize:         types.WindowSize(windowSize),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// Service implementation:

// @Summary Get usage statistics
// @Description Retrieve aggregated usage statistics for events
// @Tags events
// @Produce json
// @Param external_customer_id query string false "External Customer ID"
// @Param event_name query string true "Event Name"
// @Param property_name query string false "Property Name"
// @Param aggregation_type query string false "Aggregation Type (SUM, COUNT)"
// @Param window_size query string false "Window Size (MINUTE, HOUR, DAY)"
// @Param start_time query string false "Start Time (RFC3339)"
// @Param end_time query string false "End Time (RFC3339)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /events/usage [get]
func (h *EventsHandler) GetUsage(c *gin.Context) {
	ctx := c.Request.Context()
	externalCustomerID := c.Query("external_customer_id")
	eventName := c.Query("event_name")
	propertyName := c.Query("property_name")
	aggregationType := c.Query("aggregation_type")
	startTimeStr := c.Query("start_time")
	endTimeStr := c.Query("end_time")
	windowSize := c.Query("window_size")

	if eventName == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Missing required parameters"})
		return
	}

	startTime, endTime, err := parseStartAndEndTime(startTimeStr, endTimeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	result, err := h.eventService.GetUsage(ctx, &dto.GetUsageRequest{
		ExternalCustomerID: externalCustomerID,
		EventName:          eventName,
		PropertyName:       propertyName,
		AggregationType:    aggregationType,
		StartTime:          startTime,
		EndTime:            endTime,
		WindowSize:         types.WindowSize(windowSize),
	})
	if err != nil {
		h.log.Error("Failed to get usage", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get usage"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// @Summary Get raw events
// @Description Retrieve raw events with pagination and filtering
// @Tags events
// @Produce json
// @Param external_customer_id query string false "External Customer ID"
// @Param event_name query string false "Event Name"
// @Param start_time query string false "Start Time (RFC3339)"
// @Param end_time query string false "End Time (RFC3339)"
// @Param iter_first_key query string false "Iter First Key (unix_timestamp_nanoseconds::event_id)"
// @Param iter_last_key query string false "Iter Last Key (unix_timestamp_nanoseconds::event_id)"
// @Param page_size query int false "Page Size (1-50)"
// @Success 200 {object} dto.GetEventsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /events [get]
func (h *EventsHandler) GetEvents(c *gin.Context) {
	ctx := c.Request.Context()
	externalCustomerID := c.Query("external_customer_id")
	eventName := c.Query("event_name")
	startTimeStr := c.Query("start_time")
	endTimeStr := c.Query("end_time")
	iterFirstKey := c.Query("iter_first_key")
	iterLastKey := c.Query("iter_last_key")

	pageSize := 50
	if size := c.Query("page_size"); size != "" {
		if parsed, err := strconv.Atoi(size); err == nil {
			if parsed > 0 && parsed <= 100 {
				pageSize = parsed
			}
		}
	}

	startTime, endTime, err := parseStartAndEndTime(startTimeStr, endTimeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	events, err := h.eventService.GetEvents(ctx, &dto.GetEventsRequest{
		ExternalCustomerID: externalCustomerID,
		EventName:          eventName,
		StartTime:          startTime,
		EndTime:            endTime,
		PageSize:           pageSize,
		IterFirstKey:       iterFirstKey,
		IterLastKey:        iterLastKey,
	})
	if err != nil {
		h.log.Error("Failed to get events", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get events"})
		return
	}

	c.JSON(http.StatusOK, events)
}

func parseStartAndEndTime(startTimeStr, endTimeStr string) (time.Time, time.Time, error) {
	var startTime time.Time
	var endTime time.Time
	var err error

	if startTimeStr != "" {
		startTime, err = time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	} else {
		startTime = time.Now().AddDate(0, 0, -7)
	}

	if endTimeStr != "" {
		endTime, err = time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	} else {
		endTime = time.Now()
	}

	if endTime.Before(startTime) {
		return time.Time{}, time.Time{}, errors.New("end time must be after start time")
	}

	// Ensure times are in UTC
	startTime = startTime.UTC()
	endTime = endTime.UTC()

	return startTime, endTime, nil
}
