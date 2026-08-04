package ent

import (
	"context"
	"testing"

	"github.com/flexprice/flexprice/internal/config"
	domainInvoice "github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// newTestInvoiceRepository builds an invoiceRepository backed by a real
// Postgres instance (see newRealPostgresTestClient in coupon_test.go). This
// is required because we are testing internal/repository/ent's Ent-backed
// Update method directly, not the domain.Repository interface in the
// abstract - an in-memory testutil implementation would not exercise this
// file's SQL update chain at all.
func newTestInvoiceRepository(t *testing.T) domainInvoice.Repository {
	t.Helper()
	client := newRealPostgresTestClient(t)
	log, err := logger.NewLogger(&config.Configuration{
		Logging: config.LoggingConfig{Level: types.LogLevelInfo},
	})
	require.NoError(t, err)
	return NewInvoiceRepository(client, log, noopRedisCache{})
}

func testInvoiceContext() context.Context {
	ctx := context.Background()
	ctx = types.SetTenantID(ctx, types.DefaultTenantID)
	ctx = context.WithValue(ctx, types.CtxUserID, types.DefaultUserID)
	return ctx
}

func newTestInvoice(ctx context.Context) *domainInvoice.Invoice {
	zero := decimal.Zero
	return &domainInvoice.Invoice{
		ID:              types.GenerateUUIDWithPrefix(types.UUID_PREFIX_INVOICE),
		CustomerID:      types.GenerateUUIDWithPrefix(types.UUID_PREFIX_CUSTOMER),
		InvoiceType:     types.InvoiceTypeOneOff,
		InvoiceStatus:   types.InvoiceStatusDraft,
		PaymentStatus:   types.PaymentStatusPending,
		Currency:        "usd",
		AmountDue:       zero,
		AmountPaid:      zero,
		AmountRemaining: zero,
		Subtotal:        zero,
		Total:           zero,
		TotalDiscount:   zero,
		TotalTax:        zero,
		EnvironmentID:   types.GetEnvironmentID(ctx),
		BaseModel:       types.GetDefaultBaseModel(ctx),
	}
}

// TestInvoiceRepository_Update_PersistsIsManuallyEdited verifies that
// setting IsManuallyEdited on the domain invoice struct and calling
// Update persists the flag, i.e. it round-trips through a re-fetch (CR-03).
func TestInvoiceRepository_Update_PersistsIsManuallyEdited(t *testing.T) {
	repo := newTestInvoiceRepository(t)
	ctx := testInvoiceContext()

	inv := newTestInvoice(ctx)
	require.NoError(t, repo.Create(ctx, inv))

	got, err := repo.Get(ctx, inv.ID)
	require.NoError(t, err)
	require.False(t, got.IsManuallyEdited, "expected default is_manually_edited to be false")

	got.IsManuallyEdited = true
	require.NoError(t, repo.Update(ctx, got))

	reloaded, err := repo.Get(ctx, inv.ID)
	require.NoError(t, err)
	require.True(t, reloaded.IsManuallyEdited, "is_manually_edited should persist true after Update")
}
