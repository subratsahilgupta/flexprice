// In-memory plan repository for testing
package testutil

import (
	"context"
	"errors"
	"sync"

	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/types"
)

type InMemoryPlanRepository struct {
	store map[string]*plan.Plan
	mutex sync.RWMutex
}

func NewInMemoryPlanRepository() *InMemoryPlanRepository {
	return &InMemoryPlanRepository{
		store: make(map[string]*plan.Plan),
	}
}

func (r *InMemoryPlanRepository) Get(ctx context.Context, id string) (*plan.Plan, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if p, exists := r.store[id]; exists {
		return p, nil
	}
	return nil, errors.New("plan not found")
}

func (r *InMemoryPlanRepository) Create(ctx context.Context, p *plan.Plan) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.store[p.ID]; exists {
		return errors.New("plan already exists")
	}

	r.store[p.ID] = p
	return nil
}

func (r *InMemoryPlanRepository) Update(ctx context.Context, p *plan.Plan) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.store[p.ID]; !exists {
		return errors.New("plan not found")
	}

	r.store[p.ID] = p
	return nil
}

func (r *InMemoryPlanRepository) Delete(ctx context.Context, id string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.store[id]; !exists {
		return errors.New("plan not found")
	}

	delete(r.store, id)
	return nil
}

func (r *InMemoryPlanRepository) List(ctx context.Context, filter types.Filter) ([]*plan.Plan, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	plans := []*plan.Plan{}
	for _, p := range r.store {
		plans = append(plans, p)
	}

	// Apply offset and limit
	start := filter.Offset
	end := start + filter.Limit
	if start > len(plans) {
		return []*plan.Plan{}, nil
	}
	if end > len(plans) {
		end = len(plans)
	}
	return plans[start:end], nil
}
