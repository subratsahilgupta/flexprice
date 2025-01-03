package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/types"
)

// InMemoryInvoiceLineItemStore implements invoice.LineItemRepository for testing
type InMemoryInvoiceLineItemStore struct {
	sync.RWMutex
	items map[string]*invoice.InvoiceLineItem
}

// NewInMemoryInvoiceLineItemStore creates a new in-memory invoice line item store
func NewInMemoryInvoiceLineItemStore() invoice.LineItemRepository {
	return &InMemoryInvoiceLineItemStore{
		items: make(map[string]*invoice.InvoiceLineItem),
	}
}

func (s *InMemoryInvoiceLineItemStore) Create(ctx context.Context, item *invoice.InvoiceLineItem) (*invoice.InvoiceLineItem, error) {
	s.Lock()
	defer s.Unlock()

	if item.ID == "" {
		item.ID = types.GenerateUUIDWithPrefix(types.UUID_PREFIX_INVOICE_LINE_ITEM)
	}

	s.items[item.ID] = item
	return item, nil
}

func (s *InMemoryInvoiceLineItemStore) CreateMany(ctx context.Context, items []*invoice.InvoiceLineItem) ([]*invoice.InvoiceLineItem, error) {
	s.Lock()
	defer s.Unlock()

	for _, item := range items {
		if item.ID == "" {
			item.ID = types.GenerateUUIDWithPrefix(types.UUID_PREFIX_INVOICE_LINE_ITEM)
		}
		s.items[item.ID] = item
	}
	return items, nil
}

func (s *InMemoryInvoiceLineItemStore) Get(ctx context.Context, id string) (*invoice.InvoiceLineItem, error) {
	s.RLock()
	defer s.RUnlock()

	item, exists := s.items[id]
	if !exists {
		return nil, fmt.Errorf("invoice line item not found: %s", id)
	}
	return item, nil
}

func (s *InMemoryInvoiceLineItemStore) GetByInvoiceID(ctx context.Context, invoiceID string) ([]*invoice.InvoiceLineItem, error) {
	s.RLock()
	defer s.RUnlock()

	var items []*invoice.InvoiceLineItem
	for _, item := range s.items {
		if item.InvoiceID == invoiceID && item.Status != types.StatusDeleted {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *InMemoryInvoiceLineItemStore) Update(ctx context.Context, item *invoice.InvoiceLineItem) (*invoice.InvoiceLineItem, error) {
	s.Lock()
	defer s.Unlock()

	if _, exists := s.items[item.ID]; !exists {
		return nil, fmt.Errorf("invoice line item not found: %s", item.ID)
	}

	s.items[item.ID] = item
	return item, nil
}

func (s *InMemoryInvoiceLineItemStore) Delete(ctx context.Context, id string) error {
	s.Lock()
	defer s.Unlock()

	item, exists := s.items[id]
	if !exists {
		return fmt.Errorf("invoice line item not found: %s", id)
	}

	item.Status = types.StatusDeleted
	return nil
}

func (s *InMemoryInvoiceLineItemStore) Clear() {
	s.Lock()
	defer s.Unlock()
	s.items = make(map[string]*invoice.InvoiceLineItem)
}
