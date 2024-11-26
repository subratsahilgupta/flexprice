package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/errors"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/kafka"
	"github.com/flexprice/flexprice/internal/repository/clickhouse"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/go-playground/validator/v10"
)

type EventService interface {
	CreateEvent(ctx context.Context, createEventRequest *dto.IngestEventRequest) error
	GetUsage(ctx context.Context, getUsageRequest *dto.GetUsageRequest) (*events.AggregationResult, error)
	GetUsageByMeter(ctx context.Context, getUsageByMeterRequest *dto.GetUsageByMeterRequest) (*events.AggregationResult, error)
	GetEvents(ctx context.Context, req *dto.GetEventsRequest) (*dto.GetEventsResponse, error)
}

type eventService struct {
	producer  kafka.MessageProducer
	eventRepo events.Repository
	meterRepo meter.Repository
	validator *validator.Validate
}

func NewEventService(producer kafka.MessageProducer, eventRepo events.Repository, meterRepo meter.Repository) EventService {
	return &eventService{
		producer:  producer,
		eventRepo: eventRepo,
		meterRepo: meterRepo,
		validator: validator.New(),
	}
}

func (s *eventService) CreateEvent(ctx context.Context, createEventRequest *dto.IngestEventRequest) error {
	if err := s.validator.Struct(createEventRequest); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	tenantID := types.GetTenantID(ctx)
	event := events.NewEvent(
		createEventRequest.EventName,
		tenantID,
		createEventRequest.ExternalCustomerID,
		createEventRequest.Properties,
		createEventRequest.Timestamp,
		createEventRequest.EventID,
		createEventRequest.CustomerID,
		createEventRequest.Source,
	)

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if err := s.producer.PublishWithID("events", payload, event.ID); err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	// Return the event ID to the client
	createEventRequest.EventID = event.ID
	return nil
}

func (s *eventService) GetUsage(ctx context.Context, getUsageRequest *dto.GetUsageRequest) (*events.AggregationResult, error) {
	aggType := types.AggregationType(getUsageRequest.AggregationType)
	aggregator := clickhouse.GetAggregator(aggType)
	if aggregator == nil {
		return nil, fmt.Errorf("invalid aggregation type: %s", getUsageRequest.AggregationType)
	}

	result, err := s.eventRepo.GetUsage(
		ctx,
		&events.UsageParams{
			ExternalCustomerID: getUsageRequest.ExternalCustomerID,
			EventName:          getUsageRequest.EventName,
			PropertyName:       getUsageRequest.PropertyName,
			AggregationType:    aggType,
			WindowSize:         getUsageRequest.WindowSize,
			StartTime:          getUsageRequest.StartTime,
			EndTime:            getUsageRequest.EndTime,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage: %w", err)
	}

	return result, nil
}

func (s *eventService) GetUsageByMeter(ctx context.Context, getUsageByMeterRequest *dto.GetUsageByMeterRequest) (*events.AggregationResult, error) {
	meter, err := s.meterRepo.GetMeter(ctx, getUsageByMeterRequest.MeterID)
	if err != nil {
		return nil, errors.NewAttributeNotFoundError("meter")
	}

	if meter.EventName == "" {
		return nil, errors.NewAttributeNotFoundError("event_name")
	}

	if meter.Aggregation.Field == "" {
		return nil, errors.NewAttributeNotFoundError("aggregation_field")
	}

	if meter.WindowSize == "" {
		return nil, errors.NewAttributeNotFoundError("window_size")
	}

	usageRequest := &dto.GetUsageRequest{
		ExternalCustomerID: getUsageByMeterRequest.ExternalCustomerID,
		EventName:          meter.EventName,
		PropertyName:       meter.Aggregation.Field,
		AggregationType:    string(meter.Aggregation.Type),
		WindowSize:         string(meter.WindowSize),
		StartTime:          getUsageByMeterRequest.StartTime,
		EndTime:            getUsageByMeterRequest.EndTime,
	}
	return s.GetUsage(ctx, usageRequest)
}

func (s *eventService) GetEvents(ctx context.Context, req *dto.GetEventsRequest) (*dto.GetEventsResponse, error) {
	// Max page size is 50
	if req.PageSize <= 0 || req.PageSize >= 50 {
		req.PageSize = 50
	}

	iterFirst, err := parseEventIteratorToStruct(req.IterFirstKey)
	if err != nil {
		return nil, fmt.Errorf("invalid before cursor key: %w", err)
	}

	iterLast, err := parseEventIteratorToStruct(req.IterLastKey)
	if err != nil {
		return nil, fmt.Errorf("invalid after cursor key: %w", err)
	}

	eventsList, err := s.eventRepo.GetEvents(ctx, &events.GetEventsParams{
		ExternalCustomerID: req.ExternalCustomerID,
		EventName:          req.EventName,
		StartTime:          req.StartTime,
		EndTime:            req.EndTime,
		IterFirst:          iterFirst,
		IterLast:           iterLast,
		PageSize:           req.PageSize + 1,
	})
	if err != nil {
		return nil, fmt.Errorf("get events: %w", err)
	}

	hasMore := len(eventsList) > req.PageSize
	if hasMore {
		eventsList = eventsList[:req.PageSize]
	}

	response := &dto.GetEventsResponse{
		Events:  make([]dto.Event, len(eventsList)),
		HasMore: hasMore,
	}

	// Set first and next cursors
	if len(eventsList) > 0 {
		firstEvent := eventsList[0]
		lastEvent := eventsList[len(eventsList)-1]
		response.IterFirstKey = createEventIteratorKey(firstEvent.Timestamp, firstEvent.ID)

		if hasMore {
			response.IterLastKey = createEventIteratorKey(lastEvent.Timestamp, lastEvent.ID)
		}
	}

	// Convert events to DTO
	for i, event := range eventsList {
		response.Events[i] = dto.Event{
			ID:                 event.ID,
			ExternalCustomerID: event.ExternalCustomerID,
			CustomerID:         event.CustomerID,
			EventName:          event.EventName,
			Timestamp:          event.Timestamp,
			Properties:         event.Properties,
			Source:             event.Source,
		}
	}

	return response, nil
}

func parseEventIteratorToStruct(key string) (*events.EventIterator, error) {
	if key == "" {
		return nil, nil
	}

	// sample cursor "timestamp_id::event_id"
	parts := strings.Split(key, "::")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid cursor key format")
	}

	timestamp, err := time.Parse(time.RFC3339, parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp: %w", err)
	}

	cursor := &events.EventIterator{
		Timestamp: timestamp,
		ID:        parts[1],
	}

	return cursor, nil
}

func createEventIteratorKey(timestamp time.Time, id string) string {
	// sample cursor timestamp_id::event_id
	return fmt.Sprintf("%s::%s", timestamp.Format(time.RFC3339), id)
}
