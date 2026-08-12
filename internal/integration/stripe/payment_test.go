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
			{InvoiceLineItem: invoice.InvoiceLineItem{ID: "li_2", PriceID: lo.ToPtr("price_2"), DisplayName: lo.ToPtr("API calls"), Amount: decimal.NewFromInt(30)}},
		},
	}

	lineItems, err := s.buildSyncedLineItems(testContext(), invoiceResp, "usd")

	require.NoError(t, err)
	require.Len(t, lineItems, 2)
	require.Equal(t, "prod_1", *lineItems[0].PriceData.Product)
	require.EqualValues(t, 5000, *lineItems[0].PriceData.UnitAmount)
	require.Equal(t, "prod_2", *lineItems[1].PriceData.Product)
	require.EqualValues(t, 3000, *lineItems[1].PriceData.UnitAmount)
}

func TestBuildSyncedLineItems_MissingPriceIDFallsBackForThatItem(t *testing.T) {
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
			{InvoiceLineItem: invoice.InvoiceLineItem{ID: "li_1", PriceID: lo.ToPtr("price_1"), DisplayName: lo.ToPtr("Seat fee"), Amount: decimal.NewFromInt(50)}},
			{InvoiceLineItem: invoice.InvoiceLineItem{ID: "li_2", PriceID: nil, DisplayName: lo.ToPtr("Manual charge"), Amount: decimal.NewFromInt(20)}},
		},
	}

	lineItems, err := s.buildSyncedLineItems(testContext(), invoiceResp, "usd")

	require.NoError(t, err)
	require.Len(t, lineItems, 2)
	require.Equal(t, "prod_1", *lineItems[0].PriceData.Product)
	require.Nil(t, lineItems[1].PriceData.Product)
	require.Equal(t, "Manual charge", *lineItems[1].PriceData.ProductData.Name)
	require.EqualValues(t, 2000, *lineItems[1].PriceData.UnitAmount)
}

func TestBuildSyncedLineItems_AllMissingPriceIDReturnsNilForFullFallback(t *testing.T) {
	s := &PaymentService{
		logger:       logger.NewNoopLogger(),
		priceSyncSvc: NewStripePriceSyncService(nil, &syncTestMappingRepo{}, logger.NewNoopLogger()),
	}
	invoiceResp := &dto.InvoiceResponse{
		LineItems: []*dto.InvoiceLineItemResponse{
			{InvoiceLineItem: invoice.InvoiceLineItem{ID: "li_1", PriceID: nil, DisplayName: lo.ToPtr("Manual charge"), Amount: decimal.NewFromInt(100)}},
		},
	}

	lineItems, err := s.buildSyncedLineItems(testContext(), invoiceResp, "usd")

	require.NoError(t, err)
	require.Nil(t, lineItems, "no Price-backed items to sync, so the caller falls back to the ad-hoc lump-sum item")
}

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

	lineItems, err := s.buildSyncedLineItems(testContext(), invoiceResp, "usd")

	require.NoError(t, err)
	require.Len(t, lineItems, 2, "both line items still get their own Checkout line item")
	require.Equal(t, "prod_1", *lineItems[0].PriceData.Product)
	require.Equal(t, "prod_1", *lineItems[1].PriceData.Product)
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

	_, err := s.buildSyncedLineItems(testContext(), invoiceResp, "usd")

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

	lineItems, err := s.buildSyncedLineItems(testContext(), invoiceResp, "usd")

	require.NoError(t, err)
	require.Len(t, lineItems, 1)
}
