package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/suite"
)

type EventServiceSuite struct {
	suite.Suite
	ctx        context.Context
	service    *eventService
	store      *testutil.InMemoryEventStore
	broker     *testutil.InMemoryMessageBroker
	msgChannel chan *message.Message
}

func TestEventService(t *testing.T) {
	suite.Run(t, new(EventServiceSuite))
}

func (s *EventServiceSuite) SetupTest() {
	s.ctx = context.Background()
	s.store = testutil.NewInMemoryEventStore()
	s.broker = testutil.NewInMemoryMessageBroker()
	s.service = NewEventService(s.broker, s.store, nil).(*eventService)

	// Setup message consumer
	s.msgChannel = s.broker.Subscribe()

	// Start consuming messages
	go func() {
		for msg := range s.msgChannel {
			var event events.Event
			if err := json.Unmarshal(msg.Payload, &event); err != nil {
				continue
			}
			_ = s.store.InsertEvent(s.ctx, &event)
		}
	}()
}

func (s *EventServiceSuite) TearDownTest() {
	s.broker.Close()
}

func (s *EventServiceSuite) TestCreateEvent() {
	testCases := []struct {
		name          string
		input         *dto.IngestEventRequest
		expectedError bool
		verify        func(wg *sync.WaitGroup)
	}{
		{
			name: "successful_event_creation",
			input: &dto.IngestEventRequest{
				ID:                 "test-1",
				ExternalCustomerID: "customer-1",
				EventName:          "api.request",
				Timestamp:          time.Now(),
				Properties: map[string]interface{}{
					"duration_ms": 150,
				},
			},
			expectedError: false,
			verify: func(wg *sync.WaitGroup) {
				wg.Add(1)
				go func() {
					defer wg.Done()
					time.Sleep(100 * time.Millisecond)
					s.True(s.broker.HasMessage("events", "test-1"))
					s.True(s.store.HasEvent("test-1"))
				}()
			},
		},
		{
			name: "missing_required_fields",
			input: &dto.IngestEventRequest{
				ID: "test-2",
			},
			expectedError: true,
			verify: func(wg *sync.WaitGroup) {
				s.False(s.store.HasEvent("test-2"))
				s.False(s.broker.HasMessage("events", "test-2"))
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			var wg sync.WaitGroup

			err := s.service.CreateEvent(s.ctx, tc.input)
			if tc.expectedError {
				s.Error(err)
			} else {
				s.NoError(err)
			}

			tc.verify(&wg)
			wg.Wait()
		})
	}
}

func (s *EventServiceSuite) TestGetUsage() {
	// Setup test data
	testingEvents := []*dto.IngestEventRequest{
		{
			ID:                 "evt-1",
			ExternalCustomerID: "cust-1",
			EventName:          "api.request",
			Timestamp:          time.Now().Add(-1 * time.Hour),
			Properties: map[string]interface{}{
				"duration_ms": float64(100),
			},
		},
		{
			ID:                 "evt-2",
			ExternalCustomerID: "cust-1",
			EventName:          "api.request",
			Timestamp:          time.Now().Add(-30 * time.Minute),
			Properties: map[string]interface{}{
				"duration_ms": float64(150),
			},
		},
	}

	// Insert test events directly into store
	for _, evt := range testingEvents {
		event := events.NewEvent(
			evt.ID,
			"default",
			evt.ExternalCustomerID,
			evt.EventName,
			evt.Timestamp,
			evt.Properties,
		)
		err := s.store.InsertEvent(s.ctx, event)
		s.NoError(err)
	}

	testCases := []struct {
		name          string
		request       *dto.GetUsageRequest
		expectedValue float64
		expectedError bool
	}{
		{
			name: "count_all_events",
			request: &dto.GetUsageRequest{
				ExternalCustomerID: "cust-1",
				EventName:          "api.request",
				AggregationType:    string(types.AggregationCount),
				StartTime:          time.Now().Add(-2 * time.Hour),
				EndTime:            time.Now(),
			},
			expectedValue: 2,
			expectedError: false,
		},
		{
			name: "sum_duration",
			request: &dto.GetUsageRequest{
				ExternalCustomerID: "cust-1",
				EventName:          "api.request",
				PropertyName:       "duration_ms",
				AggregationType:    string(types.AggregationSum),
				StartTime:          time.Now().Add(-2 * time.Hour),
				EndTime:            time.Now(),
			},
			expectedValue: 250,
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			result, err := s.service.GetUsage(s.ctx, tc.request)
			if tc.expectedError {
				s.Error(err)
				return
			}
			s.NoError(err)
			s.Equal(tc.expectedValue, result.Value)
		})
	}
}
