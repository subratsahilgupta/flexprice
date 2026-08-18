package zoho

import (
	"context"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tPtr(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func TestFormatPeriodDescription(t *testing.T) {
	start := tPtr("2026-04-01T00:00:00Z")
	end := tPtr("2026-05-01T00:00:00Z")

	tests := []struct {
		name     string
		fallback string
		start    *time.Time
		end      *time.Time
		want     string
	}{
		{
			name:     "name and period",
			fallback: "BBNow",
			start:    start,
			end:      end,
			want:     "BBNow\n(2026-04-01 - 2026-04-30)",
		},
		{
			name:     "period only when no name",
			fallback: "",
			start:    start,
			end:      end,
			want:     "(2026-04-01 - 2026-04-30)",
		},
		{
			name:     "no period falls back to the name",
			fallback: "BBNow",
			start:    nil,
			end:      end,
			want:     "BBNow",
		},
		{
			name:     "missing end falls back to the name",
			fallback: "BBNow",
			start:    start,
			end:      nil,
			want:     "BBNow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatPeriodDescription(tt.fallback, tt.start, tt.end))
		})
	}
}

// period_end is exclusive in FlexPrice; the invoice must show the inclusive last day.
func TestInclusiveEnd(t *testing.T) {
	assert.Equal(t, "30/04/2026", inclusiveEnd(tPtr("2026-05-01T00:00:00Z")).Format(zohoDateFormat))
	assert.Equal(t, "31/12/2026", inclusiveEnd(tPtr("2027-01-01T00:00:00Z")).Format(zohoDateFormat))
}

func TestServicePeriodCustomFields(t *testing.T) {
	start := tPtr("2026-04-01T00:00:00Z")
	end := tPtr("2026-05-01T00:00:00Z")
	configured := &types.InvoiceSyncSettings{
		ServicePeriodCustomFields: &types.ServicePeriodCustomFields{
			StartFieldID: "cf_start",
			EndFieldID:   "cf_end",
		},
	}

	t.Run("populated when configured", func(t *testing.T) {
		got := servicePeriodCustomFields(configured, start, end)
		assert.Equal(t, []CustomField{
			{CustomFieldID: "cf_start", Value: "01/04/2026"},
			{CustomFieldID: "cf_end", Value: "30/04/2026"},
		}, got)
	})

	t.Run("nil when settings absent", func(t *testing.T) {
		assert.Nil(t, servicePeriodCustomFields(nil, start, end))
	})

	t.Run("nil when field IDs unset", func(t *testing.T) {
		assert.Nil(t, servicePeriodCustomFields(&types.InvoiceSyncSettings{}, start, end))
	})

	t.Run("nil when only one field ID is set", func(t *testing.T) {
		half := &types.InvoiceSyncSettings{
			ServicePeriodCustomFields: &types.ServicePeriodCustomFields{StartFieldID: "cf_start"},
		}
		assert.Nil(t, servicePeriodCustomFields(half, start, end),
			"a start date with no end date would render a broken header")
	})

	t.Run("nil when the invoice has no period", func(t *testing.T) {
		assert.Nil(t, servicePeriodCustomFields(configured, nil, nil))
	})
}

func TestServicePeriodCustomFieldsValidate(t *testing.T) {
	tests := []struct {
		name    string
		fields  *types.ServicePeriodCustomFields
		wantErr bool
	}{
		{"nil", nil, false},
		{"both empty", &types.ServicePeriodCustomFields{}, false},
		{"both set", &types.ServicePeriodCustomFields{StartFieldID: "a", EndFieldID: "b"}, false},
		{"only start", &types.ServicePeriodCustomFields{StartFieldID: "a"}, true},
		{"only end", &types.ServicePeriodCustomFields{EndFieldID: "b"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fields.Validate()
			assert.Equal(t, tt.wantErr, err != nil)
		})
	}
}

// gstCustomerRepo serves a customer carrying a GSTIN so place_of_supply resolves.
type gstCustomerRepo struct {
	customer.Repository
	metadata map[string]string
}

func (f *gstCustomerRepo) Get(_ context.Context, id string) (*customer.Customer, error) {
	return &customer.Customer{ID: id, Name: "MCGILL FOODS PRIVATE LIMITED", Metadata: f.metadata}, nil
}

func (f *gstCustomerRepo) List(_ context.Context, _ *types.CustomerFilter) ([]*customer.Customer, error) {
	return nil, nil
}

func TestSyncInvoiceSendsGSTHeaderFields(t *testing.T) {
	inv := &invoice.Invoice{
		ID:          "inv_1",
		CustomerID:  "cust_1",
		Currency:    "INR",
		IssueDate:   tPtr("2026-04-22T00:00:00Z"),
		DueDate:     tPtr("2026-05-07T00:00:00Z"),
		PeriodStart: tPtr("2026-04-01T00:00:00Z"),
		PeriodEnd:   tPtr("2026-05-01T00:00:00Z"),
		LineItems: []*invoice.InvoiceLineItem{
			func() *invoice.InvoiceLineItem {
				li := hsnLineItem("li_a", "price_a")
				li.PeriodStart = tPtr("2026-04-01T00:00:00Z")
				li.PeriodEnd = tPtr("2026-05-01T00:00:00Z")
				return li
			}(),
		},
	}

	syncConfig := &types.SyncConfig{
		InvoiceSyncSettings: &types.InvoiceSyncSettings{
			ServicePeriodCustomFields: &types.ServicePeriodCustomFields{
				StartFieldID: "cf_start",
				EndFieldID:   "cf_end",
			},
		},
	}

	client := &fakeSyncZohoClient{syncConfig: syncConfig}
	svc := &InvoiceService{
		client:       client,
		customerSvc:  fakeSyncCustomerSvc{},
		itemSyncSvc:  &capturingItemSyncSvc{},
		taxSvc:       fakeSyncTaxSvc{},
		customerRepo: &gstCustomerRepo{metadata: map[string]string{types.MetadataKeyGSTIN: "27AAMCM4148E1ZD"}},
		invoiceRepo:  &fakeSyncInvoiceRepo{inv: inv},
		priceRepo:    &fakeHSNPriceRepo{prices: map[string]*price.Price{"price_a": hsnPrice("price_a", "998415", "")}},
		mappingRepo:  &fakeSyncMappingRepo{},
		logger:       logger.NewNoopLogger(),
	}

	_, err := svc.SyncInvoiceToZoho(context.Background(), ZohoInvoiceSyncRequest{InvoiceID: "inv_1"})
	require.NoError(t, err)
	require.NotNil(t, client.createInvoiceReq)

	assert.Equal(t, "27", client.createInvoiceReq.PlaceOfSupply, "derived from the customer GSTIN")
	assert.Equal(t, []CustomField{
		{CustomFieldID: "cf_start", Value: "01/04/2026"},
		{CustomFieldID: "cf_end", Value: "30/04/2026"},
	}, client.createInvoiceReq.CustomFields)

	require.Len(t, client.createInvoiceReq.LineItems, 1)
	assert.Equal(t, "998415", client.createInvoiceReq.LineItems[0].HSNOrSAC)
	assert.Contains(t, client.createInvoiceReq.LineItems[0].Description, "(2026-04-01 - 2026-04-30)")
}

func TestSyncInvoiceOmitsGSTFieldsForNonIndianCustomer(t *testing.T) {
	inv := &invoice.Invoice{
		ID:         "inv_2",
		CustomerID: "cust_2",
		Currency:   "USD",
		LineItems:  []*invoice.InvoiceLineItem{hsnLineItem("li_a", "price_a")},
	}

	client := &fakeSyncZohoClient{}
	svc := &InvoiceService{
		client:       client,
		customerSvc:  fakeSyncCustomerSvc{},
		itemSyncSvc:  &capturingItemSyncSvc{},
		taxSvc:       fakeSyncTaxSvc{},
		customerRepo: &gstCustomerRepo{},
		invoiceRepo:  &fakeSyncInvoiceRepo{inv: inv},
		priceRepo:    &fakeHSNPriceRepo{prices: map[string]*price.Price{}},
		mappingRepo:  &fakeSyncMappingRepo{},
		logger:       logger.NewNoopLogger(),
	}

	_, err := svc.SyncInvoiceToZoho(context.Background(), ZohoInvoiceSyncRequest{InvoiceID: "inv_2"})
	require.NoError(t, err)

	assert.Empty(t, client.createInvoiceReq.PlaceOfSupply)
	assert.Empty(t, client.createInvoiceReq.CustomFields, "no custom fields when the connection has none configured")
}
