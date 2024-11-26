package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/types"
)

type InMemoryEventStore struct {
	mu     sync.RWMutex
	events map[string]*events.Event
}

func NewInMemoryEventStore() *InMemoryEventStore {
	return &InMemoryEventStore{
		events: make(map[string]*events.Event),
	}
}

func (s *InMemoryEventStore) InsertEvent(ctx context.Context, event *events.Event) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[event.ID] = event
	return nil
}

func (s *InMemoryEventStore) GetUsage(ctx context.Context, params *events.UsageParams) (*events.AggregationResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result float64
	for _, event := range s.events {
		if event.EventName != params.EventName ||
			event.ExternalCustomerID != params.ExternalCustomerID ||
			event.Timestamp.Before(params.StartTime) ||
			event.Timestamp.After(params.EndTime) {
			continue
		}

		switch params.AggregationType {
		case types.AggregationCount:
			result++
		case types.AggregationSum:
			if params.PropertyName != "" {
				if val, ok := event.Properties[params.PropertyName]; ok {
					if numVal, ok := val.(float64); ok {
						result += numVal
					}
				}
			}
		}
	}

	return &events.AggregationResult{
		Value:     result,
		EventName: params.EventName,
		Type:      params.AggregationType,
	}, nil
}

func (s *InMemoryEventStore) GetEvents(ctx context.Context, params *events.GetEventsParams) ([]*events.Event, error) {

	return nil, nil
}

func (s *InMemoryEventStore) HasEvent(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.events[id]
	return exists
}
