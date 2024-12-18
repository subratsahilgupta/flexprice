// In-memory price repository for testing
package testutil

import (
	"context"
	"errors"
	"sync"

	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/types"
)

type InMemoryPriceRepository struct {
	store map[string]*price.Price
	mutex sync.RWMutex
}

func NewInMemoryPriceRepository() *InMemoryPriceRepository {
	return &InMemoryPriceRepository{
		store: make(map[string]*price.Price),
	}
}

func (r *InMemoryPriceRepository) Get(ctx context.Context, id string) (*price.Price, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if p, exists := r.store[id]; exists {
		return p, nil
	}
	return nil, errors.New("price not found")
}

func (r *InMemoryPriceRepository) Create(ctx context.Context, p *price.Price) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.store[p.ID]; exists {
		return errors.New("price already exists")
	}

	r.store[p.ID] = p
	return nil
}

func (r *InMemoryPriceRepository) Update(ctx context.Context, p *price.Price) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.store[p.ID]; !exists {
		return errors.New("price not found")
	}

	r.store[p.ID] = p
	return nil
}

func (r *InMemoryPriceRepository) Delete(ctx context.Context, id string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.store[id]; !exists {
		return errors.New("price not found")
	}

	delete(r.store, id)
	return nil
}

func (r *InMemoryPriceRepository) List(ctx context.Context, filter types.Filter) ([]*price.Price, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	prices := []*price.Price{}
	for _, p := range r.store {
		prices = append(prices, p)
	}

	// Apply offset and limit
	start := filter.Offset
	end := start + filter.Limit
	if start > len(prices) {
		return []*price.Price{}, nil
	}
	if end > len(prices) {
		end = len(prices)
	}
	return prices[start:end], nil
}
