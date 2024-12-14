// In-memory customer repository for testing
package testutil

import (
	"context"
	"errors"
	"sync"

	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/types"
)

type InMemoryCustomerRepository struct {
	store map[string]*customer.Customer
	mutex sync.RWMutex
}

func NewInMemoryCustomerRepository() *InMemoryCustomerRepository {
	return &InMemoryCustomerRepository{
		store: make(map[string]*customer.Customer),
	}
}

func (r *InMemoryCustomerRepository) Get(ctx context.Context, id string) (*customer.Customer, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if c, exists := r.store[id]; exists {
		return c, nil
	}
	return nil, errors.New("customer not found")
}

func (r *InMemoryCustomerRepository) Create(ctx context.Context, c *customer.Customer) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.store[c.ID]; exists {
		return errors.New("customer already exists")
	}

	r.store[c.ID] = c
	return nil
}

func (r *InMemoryCustomerRepository) Update(ctx context.Context, c *customer.Customer) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.store[c.ID]; !exists {
		return errors.New("customer not found")
	}

	r.store[c.ID] = c
	return nil
}

func (r *InMemoryCustomerRepository) Delete(ctx context.Context, id string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.store[id]; !exists {
		return errors.New("customer not found")
	}

	delete(r.store, id)
	return nil
}

func (r *InMemoryCustomerRepository) List(ctx context.Context, filter types.Filter) ([]*customer.Customer, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	customers := []*customer.Customer{}
	for _, c := range r.store {
		customers = append(customers, c)
	}

	// Apply offset and limit
	start := filter.Offset
	end := start + filter.Limit

	if start > len(customers) {
		return []*customer.Customer{}, nil // No customers in this range
	} else if end > len(customers) {
		end = len(customers) // Adjust the end index to avoid out-of-bounds
	}

	return customers[start:end], nil
}
