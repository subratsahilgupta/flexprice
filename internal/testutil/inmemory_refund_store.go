package testutil

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/flexprice/flexprice/internal/domain/refund"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

var _ refund.Repository = (*InMemoryRefundStore)(nil)

// InMemoryRefundStore implements refund.Repository
type InMemoryRefundStore struct {
	*InMemoryStore[*refund.Refund]
}

// NewInMemoryRefundStore creates a new in-memory refund repository
func NewInMemoryRefundStore() *InMemoryRefundStore {
	return &InMemoryRefundStore{
		InMemoryStore: NewInMemoryStore[*refund.Refund](),
	}
}

// Clear resets all stored data
func (m *InMemoryRefundStore) Clear() {
	m.InMemoryStore.Clear()
}

// Create stores a new refund
func (m *InMemoryRefundStore) Create(ctx context.Context, r *refund.Refund) error {
	if r == nil {
		return ierr.NewError("refund cannot be nil").
			WithHint("Refund cannot be nil").
			Mark(ierr.ErrValidation)
	}

	if r.ID == "" {
		return ierr.NewError("refund ID cannot be empty").
			WithHint("Refund ID cannot be empty").
			Mark(ierr.ErrValidation)
	}

	if r.EnvironmentID == "" {
		r.EnvironmentID = types.GetEnvironmentID(ctx)
	}

	return m.InMemoryStore.Create(ctx, r.ID, r)
}

// Get retrieves a refund by ID
func (m *InMemoryRefundStore) Get(ctx context.Context, id string) (*refund.Refund, error) {
	return m.InMemoryStore.Get(ctx, id)
}

// Update updates an existing refund
func (m *InMemoryRefundStore) Update(ctx context.Context, r *refund.Refund) error {
	if r == nil {
		return ierr.NewError("refund cannot be nil").
			WithHint("Refund cannot be nil").
			Mark(ierr.ErrValidation)
	}

	r.UpdatedAt = time.Now().UTC()

	return m.InMemoryStore.Update(ctx, r.ID, r)
}

// Delete removes a refund
func (m *InMemoryRefundStore) Delete(ctx context.Context, id string) error {
	return m.InMemoryStore.Delete(ctx, id)
}

// GetByIdempotencyKey retrieves a refund by idempotency key
func (m *InMemoryRefundStore) GetByIdempotencyKey(ctx context.Context, key string) (*refund.Refund, error) {
	refunds, err := m.List(ctx, &types.RefundFilter{
		QueryFilter: types.NewNoLimitQueryFilter(),
	})
	if err != nil {
		return nil, err
	}

	for _, r := range refunds {
		if r.IdempotencyKey == key {
			return r, nil
		}
	}

	return nil, ierr.NewError("refund not found").
		WithHint(fmt.Sprintf("Refund not found for idempotency key: %s", key)).
		Mark(ierr.ErrNotFound)
}

// refundFilterFn implements filter matching logic for refunds
func refundFilterFn(ctx context.Context, r *refund.Refund, filter interface{}) bool {
	if r == nil {
		return false
	}

	f, ok := filter.(*types.RefundFilter)
	if !ok {
		return true
	}

	// Check tenant ID
	if tenantID, ok := ctx.Value(types.CtxTenantID).(string); ok {
		if r.TenantID != tenantID {
			return false
		}
	}

	// Apply environment filter
	if !CheckEnvironmentFilter(ctx, r.EnvironmentID) {
		return false
	}

	// Mirrors RefundQueryOptions.ApplyStatusFilter: an empty status means published only.
	if f.GetStatus() == "" {
		if r.Status != types.StatusPublished {
			return false
		}
	} else if string(r.Status) != f.GetStatus() {
		return false
	}

	if f.PaymentIDs != nil {
		if r.PaymentID == nil || !slices.Contains(f.PaymentIDs, *r.PaymentID) {
			return false
		}
	}

	if f.CreditNoteIDs != nil {
		if r.CreditNoteID == nil || !slices.Contains(f.CreditNoteIDs, *r.CreditNoteID) {
			return false
		}
	}

	if f.InvoiceIDs != nil && !slices.Contains(f.InvoiceIDs, r.InvoiceID) {
		return false
	}

	if f.RefundDestinations != nil && !slices.Contains(f.RefundDestinations, r.RefundDestination) {
		return false
	}

	if f.OnlySettled != nil && *f.OnlySettled && !r.IsSettled() {
		return false
	}

	if f.RefundStatuses != nil && !slices.Contains(f.RefundStatuses, r.RefundStatus) {
		return false
	}

	if f.Gateway != nil {
		if r.PaymentGateway == nil || *r.PaymentGateway != *f.Gateway {
			return false
		}
	}

	// Filter by time range
	if f.TimeRangeFilter != nil {
		if f.StartTime != nil && r.CreatedAt.Before(*f.StartTime) {
			return false
		}
		if f.EndTime != nil && r.CreatedAt.After(*f.EndTime) {
			return false
		}
	}

	return true
}

// refundSortFn implements sorting logic for refunds (newest first)
func refundSortFn(i, j *refund.Refund) bool {
	if i == nil || j == nil {
		return false
	}
	return i.CreatedAt.After(j.CreatedAt)
}

// List returns a list of refunds based on the filter
func (m *InMemoryRefundStore) List(ctx context.Context, filter *types.RefundFilter) ([]*refund.Refund, error) {
	return m.InMemoryStore.List(ctx, filter, refundFilterFn, refundSortFn)
}

// Count returns the number of refunds matching the filter
func (m *InMemoryRefundStore) Count(ctx context.Context, filter *types.RefundFilter) (int, error) {
	return m.InMemoryStore.Count(ctx, filter, refundFilterFn)
}

// CreateBulk stores multiple refunds
func (m *InMemoryRefundStore) CreateBulk(ctx context.Context, refunds []*refund.Refund) error {
	for _, r := range refunds {
		if err := m.Create(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

// GetForUpdate retrieves a refund by ID; the in-memory store takes no locks
func (m *InMemoryRefundStore) GetForUpdate(ctx context.Context, id string) (*refund.Refund, error) {
	return m.Get(ctx, id)
}

// GetByGatewayRefundID retrieves a refund by gateway and gateway refund ID
func (m *InMemoryRefundStore) GetByGatewayRefundID(ctx context.Context, gateway, gatewayRefundID string) (*refund.Refund, error) {
	refunds, err := m.List(ctx, &types.RefundFilter{
		QueryFilter: types.NewNoLimitQueryFilter(),
	})
	if err != nil {
		return nil, err
	}

	for _, r := range refunds {
		if r.PaymentGateway == nil || *r.PaymentGateway != gateway {
			continue
		}
		if r.GatewayRefundID != nil && *r.GatewayRefundID == gatewayRefundID {
			return r, nil
		}
	}

	return nil, ierr.NewError("refund not found").
		WithHint(fmt.Sprintf("Refund not found for gateway refund id: %s", gatewayRefundID)).
		Mark(ierr.ErrNotFound)
}

// ListByInvoice returns all published refunds for an invoice
func (m *InMemoryRefundStore) ListByInvoice(ctx context.Context, invoiceID string) ([]*refund.Refund, error) {
	return m.List(ctx, &types.RefundFilter{
		QueryFilter: types.NewNoLimitQueryFilter(),
		InvoiceIDs:  []string{invoiceID},
	})
}

// SumSettledByPaymentIDs sums settled amounts grouped by payment ID
func (m *InMemoryRefundStore) SumSettledByPaymentIDs(ctx context.Context, paymentIDs []string) (map[string]decimal.Decimal, error) {
	return m.sumSettled(ctx, paymentIDs, func(r *refund.Refund) string {
		if r.PaymentID == nil {
			return ""
		}
		return *r.PaymentID
	})
}

// SumSettledByInvoiceIDs sums settled amounts grouped by invoice ID
func (m *InMemoryRefundStore) SumSettledByInvoiceIDs(ctx context.Context, invoiceIDs []string) (map[string]decimal.Decimal, error) {
	return m.sumSettled(ctx, invoiceIDs, func(r *refund.Refund) string {
		return r.InvoiceID
	})
}

func (m *InMemoryRefundStore) sumSettled(
	ctx context.Context,
	ids []string,
	key func(*refund.Refund) string,
) (map[string]decimal.Decimal, error) {
	result := make(map[string]decimal.Decimal, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	settled := true
	refunds, err := m.List(ctx, &types.RefundFilter{
		QueryFilter: types.NewNoLimitQueryFilter(),
		OnlySettled: &settled,
	})
	if err != nil {
		return nil, err
	}

	for _, r := range refunds {
		k := key(r)
		if k == "" || !slices.Contains(ids, k) {
			continue
		}
		result[k] = result[k].Add(r.SettledAmount)
	}

	return result, nil
}
