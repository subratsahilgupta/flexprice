package stripe

import (
	"errors"
	"testing"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/entityintegrationmapping"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestDistributePaymentAmount(t *testing.T) {
	tests := []struct {
		name            string
		items           []paymentLinkLineItemInput
		requestedAmount decimal.Decimal
		currency        string
		want            []paymentLinkLineItemShare
	}{
		{
			name: "single item gets full amount",
			items: []paymentLinkLineItemInput{
				{PriceID: "price_1", DisplayName: "Seat fee", Amount: decimal.NewFromInt(100)},
			},
			requestedAmount: decimal.NewFromInt(100),
			currency:        "usd",
			want: []paymentLinkLineItemShare{
				{PriceID: "price_1", DisplayName: "Seat fee", UnitAmount: 10000},
			},
		},
		{
			name: "exact even split across two items",
			items: []paymentLinkLineItemInput{
				{PriceID: "price_1", DisplayName: "Seat fee", Amount: decimal.NewFromInt(50)},
				{PriceID: "price_2", DisplayName: "API calls", Amount: decimal.NewFromInt(50)},
			},
			requestedAmount: decimal.NewFromInt(100),
			currency:        "usd",
			want: []paymentLinkLineItemShare{
				{PriceID: "price_1", DisplayName: "Seat fee", UnitAmount: 5000},
				{PriceID: "price_2", DisplayName: "API calls", UnitAmount: 5000},
			},
		},
		{
			name: "uneven split, last item absorbs rounding remainder",
			items: []paymentLinkLineItemInput{
				{PriceID: "price_1", DisplayName: "A", Amount: decimal.NewFromInt(1)},
				{PriceID: "price_2", DisplayName: "B", Amount: decimal.NewFromInt(1)},
				{PriceID: "price_3", DisplayName: "C", Amount: decimal.NewFromInt(1)},
			},
			requestedAmount: decimal.NewFromInt(100),
			currency:        "usd",
			want: []paymentLinkLineItemShare{
				{PriceID: "price_1", DisplayName: "A", UnitAmount: 3333},
				{PriceID: "price_2", DisplayName: "B", UnitAmount: 3333},
				{PriceID: "price_3", DisplayName: "C", UnitAmount: 3334},
			},
		},
		{
			name: "tiny partial payment drops a would-be-zero item",
			items: []paymentLinkLineItemInput{
				{PriceID: "price_1", DisplayName: "A", Amount: decimal.NewFromInt(1)},
				{PriceID: "price_2", DisplayName: "B", Amount: decimal.NewFromInt(9999)},
			},
			requestedAmount: decimal.NewFromFloat(0.01),
			currency:        "usd",
			want: []paymentLinkLineItemShare{
				{PriceID: "price_2", DisplayName: "B", UnitAmount: 1},
			},
		},
		{
			name:            "zero requested amount returns nil",
			items:           []paymentLinkLineItemInput{{PriceID: "price_1", DisplayName: "A", Amount: decimal.NewFromInt(1)}},
			requestedAmount: decimal.Zero,
			currency:        "usd",
			want:            nil,
		},
		{
			name:            "empty input returns nil",
			items:           nil,
			requestedAmount: decimal.NewFromInt(100),
			currency:        "usd",
			want:            nil,
		},
		{
			name: "positive total that truncates to zero smallest units does not divide by zero",
			items: []paymentLinkLineItemInput{
				{PriceID: "price_1", DisplayName: "A", Amount: decimal.NewFromFloat(0.001)},
				{PriceID: "price_2", DisplayName: "B", Amount: decimal.NewFromFloat(0.001)},
			},
			requestedAmount: decimal.NewFromFloat(0.001),
			currency:        "usd",
			want:            nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := distributePaymentAmount(tt.items, tt.requestedAmount, tt.currency)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestBuildSyncedLineItems_HappyPath(t *testing.T) {
	mappingRepo := &syncTestMappingRepo{
		mappings: []*entityintegrationmapping.EntityIntegrationMapping{
			{EntityID: "price_1", ProviderEntityID: "prod_1"},
			{EntityID: "price_2", ProviderEntityID: "prod_2"},
		},
	}
	s := &PaymentService{
		logger:       logger.NewNoopLogger(),
		priceSyncSvc: NewStripePriceSyncService(nil, mappingRepo, logger.NewNoopLogger()),
	}
	invoiceResp := &dto.InvoiceResponse{
		LineItems: []*dto.InvoiceLineItemResponse{
			{InvoiceLineItem: invoice.InvoiceLineItem{ID: "li_1", PriceID: lo.ToPtr("price_1"), DisplayName: lo.ToPtr("Seat fee"), Amount: decimal.NewFromInt(50)}},
			{InvoiceLineItem: invoice.InvoiceLineItem{ID: "li_2", PriceID: lo.ToPtr("price_2"), DisplayName: lo.ToPtr("API calls"), Amount: decimal.NewFromInt(50)}},
		},
	}

	lineItems, err := s.buildSyncedLineItems(testContext(), invoiceResp, decimal.NewFromInt(100), "usd")

	require.NoError(t, err)
	require.Len(t, lineItems, 2)
	require.Equal(t, "prod_1", *lineItems[0].PriceData.Product)
	require.EqualValues(t, 5000, *lineItems[0].PriceData.UnitAmount)
	require.Equal(t, "prod_2", *lineItems[1].PriceData.Product)
	require.EqualValues(t, 5000, *lineItems[1].PriceData.UnitAmount)
}

// TestBuildSyncedLineItems_DuplicatePriceIDAcrossLineItems covers two invoice line
// items referencing the same Price (e.g. a base charge plus a usage overage). Both
// still resolve to the same Stripe Product and get their own Checkout line item. The
// dedup itself — avoiding two V1Products.Create calls when the Price is unmapped —
// needs a real Stripe client to exercise and is out of reach for this package's unit
// tests (see the file-level comment on syncTestMappingRepo); verified by code review
// against the same lo.UniqBy pattern already used in InvoiceSyncService.
func TestBuildSyncedLineItems_DuplicatePriceIDAcrossLineItems(t *testing.T) {
	mappingRepo := &syncTestMappingRepo{
		mappings: []*entityintegrationmapping.EntityIntegrationMapping{
			{EntityID: "price_1", ProviderEntityID: "prod_1"},
		},
	}
	s := &PaymentService{
		logger:       logger.NewNoopLogger(),
		priceSyncSvc: NewStripePriceSyncService(nil, mappingRepo, logger.NewNoopLogger()),
	}
	invoiceResp := &dto.InvoiceResponse{
		LineItems: []*dto.InvoiceLineItemResponse{
			{InvoiceLineItem: invoice.InvoiceLineItem{ID: "li_1", PriceID: lo.ToPtr("price_1"), DisplayName: lo.ToPtr("Base fee"), Amount: decimal.NewFromInt(50)}},
			{InvoiceLineItem: invoice.InvoiceLineItem{ID: "li_2", PriceID: lo.ToPtr("price_1"), DisplayName: lo.ToPtr("Overage"), Amount: decimal.NewFromInt(50)}},
		},
	}

	lineItems, err := s.buildSyncedLineItems(testContext(), invoiceResp, decimal.NewFromInt(100), "usd")

	require.NoError(t, err)
	require.Len(t, lineItems, 2, "both line items still get their own Checkout line item")
	require.Equal(t, "prod_1", *lineItems[0].PriceData.Product)
	require.Equal(t, "prod_1", *lineItems[1].PriceData.Product)
}

func TestBuildSyncedLineItems_MissingPriceIDRejectsRequest(t *testing.T) {
	s := &PaymentService{
		logger:       logger.NewNoopLogger(),
		priceSyncSvc: NewStripePriceSyncService(nil, &syncTestMappingRepo{}, logger.NewNoopLogger()),
	}
	invoiceResp := &dto.InvoiceResponse{
		LineItems: []*dto.InvoiceLineItemResponse{
			{InvoiceLineItem: invoice.InvoiceLineItem{ID: "li_1", PriceID: nil, DisplayName: lo.ToPtr("Manual charge"), Amount: decimal.NewFromInt(100)}},
		},
	}

	_, err := s.buildSyncedLineItems(testContext(), invoiceResp, decimal.NewFromInt(100), "usd")

	require.Error(t, err)
}

func TestBuildSyncedLineItems_SyncFailurePropagates(t *testing.T) {
	mappingRepo := &syncTestMappingRepo{listErr: errors.New("db unavailable")}
	s := &PaymentService{
		logger:       logger.NewNoopLogger(),
		priceSyncSvc: NewStripePriceSyncService(nil, mappingRepo, logger.NewNoopLogger()),
	}
	invoiceResp := &dto.InvoiceResponse{
		LineItems: []*dto.InvoiceLineItemResponse{
			{InvoiceLineItem: invoice.InvoiceLineItem{ID: "li_1", PriceID: lo.ToPtr("price_1"), DisplayName: lo.ToPtr("Seat fee"), Amount: decimal.NewFromInt(100)}},
		},
	}

	_, err := s.buildSyncedLineItems(testContext(), invoiceResp, decimal.NewFromInt(100), "usd")

	require.Error(t, err)
}

func TestBuildSyncedLineItems_ZeroAmountLineItemsSkipped(t *testing.T) {
	mappingRepo := &syncTestMappingRepo{
		mappings: []*entityintegrationmapping.EntityIntegrationMapping{
			{EntityID: "price_1", ProviderEntityID: "prod_1"},
		},
	}
	s := &PaymentService{
		logger:       logger.NewNoopLogger(),
		priceSyncSvc: NewStripePriceSyncService(nil, mappingRepo, logger.NewNoopLogger()),
	}
	invoiceResp := &dto.InvoiceResponse{
		LineItems: []*dto.InvoiceLineItemResponse{
			{InvoiceLineItem: invoice.InvoiceLineItem{ID: "li_1", PriceID: lo.ToPtr("price_1"), DisplayName: lo.ToPtr("Seat fee"), Amount: decimal.NewFromInt(100)}},
			{InvoiceLineItem: invoice.InvoiceLineItem{ID: "li_2", PriceID: nil, DisplayName: lo.ToPtr("Zeroed credit line"), Amount: decimal.Zero}},
		},
	}

	lineItems, err := s.buildSyncedLineItems(testContext(), invoiceResp, decimal.NewFromInt(100), "usd")

	require.NoError(t, err)
	require.Len(t, lineItems, 1, "the nil-PriceID item is zero-amount, so it's skipped before the PriceID check, not rejected")
}
