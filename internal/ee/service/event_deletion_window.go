package service

import (
	"context"
	"sync"
	"time"

	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/types"
)

// EventDeletionWindowService answers whether an arriving event belongs to a
// deletion in progress, so the consumer drops it instead of writing a row the
// deletion's mutation can no longer reach.
//
// Shared by every consumer that writes events or meter_usage. The DELETING rows
// are cached per tenant and environment for suppressionCacheTTL, because a
// per-event Postgres read would be the ingestion bottleneck.
type EventDeletionWindowService interface {
	// IsSuppressed must be consulted before anything else the consumer does for
	// the event: a dropped event fires no webhook, publishes nothing, runs no
	// side effects and takes no dedup lock.
	IsSuppressed(ctx context.Context, externalCustomerID string, timestamp time.Time) bool
}

type eventDeletionWindowService struct {
	ServiceParams

	mu sync.Mutex
	// windows is keyed by "tenant:environment"; the fence read is tenant-scoped
	// and one consumer process serves many tenants.
	windows map[string]*deletionWindow
}

// deletionWindow is one tenant's cached fence. At most one request per customer,
// which the unique partial index guarantees.
type deletionWindow struct {
	loadedAt   time.Time
	byCustomer map[string]*events.DeletionRequest
}

func NewEventDeletionWindowService(params ServiceParams) EventDeletionWindowService {
	return &eventDeletionWindowService{
		ServiceParams: params,
		windows:       make(map[string]*deletionWindow),
	}
}

func (s *eventDeletionWindowService) IsSuppressed(ctx context.Context, externalCustomerID string, timestamp time.Time) bool {
	if s.EventDeletionRequestRepo == nil || externalCustomerID == "" {
		return false
	}
	return s.currentWindow(ctx).byCustomer[externalCustomerID].IncludesTimestamp(timestamp)
}

// currentWindow returns the ctx tenant's window, reloading it when stale. A failed
// read yields an empty window, so a Postgres blip writes events rather than
// dropping them.
func (s *eventDeletionWindowService) currentWindow(ctx context.Context) *deletionWindow {
	key := types.GetTenantID(ctx) + ":" + types.GetEnvironmentID(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()

	window, ok := s.windows[key]
	if ok && time.Since(window.loadedAt) < suppressionCacheTTL {
		return window
	}

	requests, err := s.EventDeletionRequestRepo.ListAll(ctx, inProgressDeletionFilter(nil))
	if err != nil {
		// loadedAt advances even on failure, so a down database does not become one
		// query per event.
		window = &deletionWindow{loadedAt: time.Now()}
		s.windows[key] = window
		s.Logger.Error(ctx, "failed to load the event deletion suppression window",
			"error", err,
			"tenant_id", types.GetTenantID(ctx),
			"environment_id", types.GetEnvironmentID(ctx),
		)
		return window
	}

	byCustomer := make(map[string]*events.DeletionRequest, len(requests))
	for _, request := range requests {
		byCustomer[request.ExternalCustomerID] = request
	}
	window = &deletionWindow{loadedAt: time.Now(), byCustomer: byCustomer}
	s.windows[key] = window

	if len(byCustomer) > 0 {
		s.Logger.Info(ctx, "event deletion suppression window loaded",
			"customer_count", len(byCustomer),
			"tenant_id", types.GetTenantID(ctx),
			"environment_id", types.GetEnvironmentID(ctx),
		)
	}
	return window
}
