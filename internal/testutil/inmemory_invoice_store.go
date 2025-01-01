package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/types"
)

type InMemoryInvoiceStore struct {
	mu       sync.RWMutex
	invoices map[string]*invoice.Invoice
}

func NewInMemoryInvoiceStore() *InMemoryInvoiceStore {
	return &InMemoryInvoiceStore{
		invoices: make(map[string]*invoice.Invoice),
	}
}

func (r *InMemoryInvoiceStore) Create(ctx context.Context, inv *invoice.Invoice) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if inv.ID == "" {
		return fmt.Errorf("invoice ID cannot be empty")
	}

	if _, exists := r.invoices[inv.ID]; exists {
		return fmt.Errorf("invoice with ID %s already exists", inv.ID)
	}

	// Create a deep copy to prevent external modifications
	invCopy := *inv
	r.invoices[inv.ID] = &invCopy
	return nil
}

func (r *InMemoryInvoiceStore) Get(ctx context.Context, id string) (*invoice.Invoice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	inv, exists := r.invoices[id]
	if !exists {
		return nil, fmt.Errorf("invoice not found: %s", id)
	}

	// Return a copy to prevent external modifications
	invCopy := *inv
	return &invCopy, nil
}

func (r *InMemoryInvoiceStore) Update(ctx context.Context, inv *invoice.Invoice) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.invoices[inv.ID]; !exists {
		return fmt.Errorf("invoice not found: %s", inv.ID)
	}

	// Create a deep copy and update
	invCopy := *inv
	r.invoices[inv.ID] = &invCopy
	return nil
}

func (r *InMemoryInvoiceStore) List(ctx context.Context, filter *types.InvoiceFilter) ([]*invoice.Invoice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*invoice.Invoice

	for _, inv := range r.invoices {
		if filter == nil {
			invCopy := *inv
			result = append(result, &invCopy)
			continue
		}

		if !r.matchesFilter(inv, filter) {
			continue
		}

		invCopy := *inv
		result = append(result, &invCopy)
	}

	return result, nil
}

func (r *InMemoryInvoiceStore) Count(ctx context.Context, filter *types.InvoiceFilter) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if filter == nil {
		return len(r.invoices), nil
	}

	count := 0
	for _, inv := range r.invoices {
		if r.matchesFilter(inv, filter) {
			count++
		}
	}

	return count, nil
}

func (r *InMemoryInvoiceStore) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.invoices = make(map[string]*invoice.Invoice)
}

func (r *InMemoryInvoiceStore) matchesFilter(inv *invoice.Invoice, filter *types.InvoiceFilter) bool {
	if filter.CustomerID != "" && inv.CustomerID != filter.CustomerID {
		return false
	}

	if filter.SubscriptionID != "" && (inv.SubscriptionID == nil || *inv.SubscriptionID != filter.SubscriptionID) {
		return false
	}

	if len(filter.InvoiceStatus) > 0 {
		found := false
		for _, status := range filter.InvoiceStatus {
			if inv.InvoiceStatus == status {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(filter.PaymentStatus) > 0 {
		found := false
		for _, status := range filter.PaymentStatus {
			if inv.PaymentStatus == status {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if filter.StartTime != nil && inv.CreatedAt.Before(*filter.StartTime) {
		return false
	}

	if filter.EndTime != nil && inv.CreatedAt.After(*filter.EndTime) {
		return false
	}

	return true
}
