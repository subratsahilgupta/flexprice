package quickbooks

import (
	"context"
	"sync"
	"testing"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/entityintegrationmapping"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type fakeQBClient struct {
	QuickBooksClient
	payment *PaymentResponse
}

func (c *fakeQBClient) GetPayment(_ context.Context, _ string) (*PaymentResponse, error) {
	return c.payment, nil
}

type fakeMappingRepo struct {
	entityintegrationmapping.Repository
}

func (fakeMappingRepo) List(_ context.Context, _ *types.EntityIntegrationMappingFilter) ([]*entityintegrationmapping.EntityIntegrationMapping, error) {
	return []*entityintegrationmapping.EntityIntegrationMapping{{ID: "eim_1", EntityID: "inv_1"}}, nil
}

// fakePaymentService mimics idempotency-key dedupe and status compare-and-swap.
type fakePaymentService struct {
	interfaces.PaymentService
	mu       sync.Mutex
	byKey    map[string]*dto.PaymentResponse
	byID     map[string]*dto.PaymentResponse
	preExist []*dto.PaymentResponse
}

func newFakePaymentService() *fakePaymentService {
	return &fakePaymentService{byKey: map[string]*dto.PaymentResponse{}, byID: map[string]*dto.PaymentResponse{}}
}

func (f *fakePaymentService) ListPayments(_ context.Context, _ *types.PaymentFilter) (*dto.ListPaymentsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &dto.ListPaymentsResponse{Items: f.preExist}, nil
}

func (f *fakePaymentService) CreatePayment(_ context.Context, req *dto.CreatePaymentRequest) (*dto.PaymentResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.byKey[req.IdempotencyKey]; ok {
		return existing, nil
	}
	p := &dto.PaymentResponse{ID: "pay_" + req.IdempotencyKey, PaymentStatus: types.PaymentStatusPending}
	f.byKey[req.IdempotencyKey] = p
	f.byID[p.ID] = p
	return p, nil
}

func (f *fakePaymentService) GetPayment(_ context.Context, id string) (*dto.PaymentResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byID[id], nil
}

func (f *fakePaymentService) UpdatePayment(_ context.Context, id string, _ dto.UpdatePaymentRequest) (*dto.PaymentResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.byID[id]
	if p.PaymentStatus == types.PaymentStatusSucceeded {
		return nil, ierr.NewError("invalid transition").Mark(ierr.ErrValidation)
	}
	p.PaymentStatus = types.PaymentStatusSucceeded
	return p, nil
}

// fakeDB serializes transactions and restores payment state on error.
type fakeDB struct {
	postgres.IClient
	mu       sync.Mutex
	payments *fakePaymentService
}

func (d *fakeDB) WithTx(ctx context.Context, fn func(context.Context) error) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	saved := map[string]types.PaymentStatus{}
	savedKeys := map[string]*dto.PaymentResponse{}
	d.payments.mu.Lock()
	for id, p := range d.payments.byID {
		saved[id] = p.PaymentStatus
	}
	for k, p := range d.payments.byKey {
		savedKeys[k] = p
	}
	d.payments.mu.Unlock()

	err := fn(ctx)
	if err == nil {
		return nil
	}
	d.payments.mu.Lock()
	defer d.payments.mu.Unlock()
	for id, p := range d.payments.byID {
		if status, ok := saved[id]; ok {
			p.PaymentStatus = status
		} else {
			delete(d.payments.byID, id)
		}
	}
	d.payments.byKey = savedKeys
	return err
}

func (d *fakeDB) LockWithWait(_ context.Context, _ postgres.LockRequest) error { return nil }

type fakeInvoiceService struct {
	interfaces.InvoiceService
	mu            sync.Mutex
	reconciled    int
	failReconcile int
}

func (f *fakeInvoiceService) GetInvoice(_ context.Context, id string) (*dto.InvoiceResponse, error) {
	return &dto.InvoiceResponse{Invoice: invoice.Invoice{ID: id, Currency: "usd", AmountDue: decimal.NewFromInt(100)}}, nil
}

func (f *fakeInvoiceService) ReconcilePaymentStatus(_ context.Context, _ string, _ types.PaymentStatus, _ *decimal.Decimal) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failReconcile > 0 {
		f.failReconcile--
		return ierr.NewError("reconcile failed").Mark(ierr.ErrInternal)
	}
	f.reconciled++
	return nil
}

func TestHandleExternalPaymentFromWebhook_AppliesOnce(t *testing.T) {
	log, err := logger.NewLogger(&config.Configuration{Logging: config.LoggingConfig{Level: types.LogLevelInfo}})
	require.NoError(t, err)

	qbPayment := &PaymentResponse{
		ID: "qb_pay_1",
		Line: []PaymentLine{{
			Amount:    40,
			LinkedTxn: []LinkedTxn{{TxnId: "qb_inv_1", TxnType: "Invoice"}},
		}},
	}

	tests := []struct {
		name           string
		deliveries     int
		concurrent     bool
		legacySucceed  bool
		failReconcile  int
		wantReconciled int
	}{
		{name: "single delivery", deliveries: 1, wantReconciled: 1},
		{name: "sequential duplicate", deliveries: 3, wantReconciled: 1},
		{name: "concurrent duplicate", deliveries: 8, concurrent: true, wantReconciled: 1},
		{name: "reconcile fails then retry applies once", deliveries: 3, failReconcile: 1, wantReconciled: 1},
		{name: "legacy succeeded payment", deliveries: 2, legacySucceed: true, wantReconciled: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payments := newFakePaymentService()
			if tt.legacySucceed {
				payments.preExist = []*dto.PaymentResponse{{ID: "pay_old", PaymentStatus: types.PaymentStatusSucceeded}}
			}
			invoices := &fakeInvoiceService{failReconcile: tt.failReconcile}
			db := &fakeDB{payments: payments}
			svc := NewPaymentService(PaymentServiceParams{
				Client:                       &fakeQBClient{payment: qbPayment},
				EntityIntegrationMappingRepo: fakeMappingRepo{},
				Logger:                       log,
			})

			deliver := func() {
				require.NoError(t, svc.HandleExternalPaymentFromWebhook(context.Background(), "qb_pay_1", payments, invoices, db))
			}
			if tt.concurrent {
				var wg sync.WaitGroup
				for i := 0; i < tt.deliveries; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						deliver()
					}()
				}
				wg.Wait()
			} else {
				for i := 0; i < tt.deliveries; i++ {
					deliver()
				}
			}

			require.Equal(t, tt.wantReconciled, invoices.reconciled)
		})
	}
}

func TestHandleExternalPaymentFromWebhook_RollbackKeepsPaymentPending(t *testing.T) {
	log, err := logger.NewLogger(&config.Configuration{Logging: config.LoggingConfig{Level: types.LogLevelInfo}})
	require.NoError(t, err)

	qbPayment := &PaymentResponse{
		ID:   "qb_pay_1",
		Line: []PaymentLine{{Amount: 40, LinkedTxn: []LinkedTxn{{TxnId: "qb_inv_1", TxnType: "Invoice"}}}},
	}
	payments := newFakePaymentService()
	invoices := &fakeInvoiceService{failReconcile: 1}
	db := &fakeDB{payments: payments}
	svc := NewPaymentService(PaymentServiceParams{
		Client:                       &fakeQBClient{payment: qbPayment},
		EntityIntegrationMappingRepo: fakeMappingRepo{},
		Logger:                       log,
	})

	require.NoError(t, svc.HandleExternalPaymentFromWebhook(context.Background(), "qb_pay_1", payments, invoices, db))
	require.Zero(t, invoices.reconciled)
	require.Empty(t, payments.byID, "failed apply must leave no succeeded payment")

	require.NoError(t, svc.HandleExternalPaymentFromWebhook(context.Background(), "qb_pay_1", payments, invoices, db))
	require.Equal(t, 1, invoices.reconciled)
}
