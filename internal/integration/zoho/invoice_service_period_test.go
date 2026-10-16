package zoho

import (
	"context"
	"testing"

	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncInvoiceResolvesServicePeriodFromSentItems(t *testing.T) {
	item := func(id, start, end string) *invoice.InvoiceLineItem {
		li := hsnLineItem(id, "price_a")
		if start != "" {
			li.PeriodStart = tPtr(start)
		}
		if end != "" {
			li.PeriodEnd = tPtr(end)
		}
		return li
	}
	const sept = "2026-09-12T00:00:00Z"
	const oct = "2026-10-12T00:00:00Z"
	const nov = "2026-11-12T00:00:00Z"
	tests := []struct {
		name               string
		items              func() []*invoice.InvoiceLineItem
		wantStart, wantEnd string
		count              int
	}{
		{"advance invoice with discounted services and skipped anchors", func() []*invoice.InvoiceLineItem {
			a, b := item("blinkit", oct, nov), item("instamart", oct, nov)
			a.LineItemDiscount, b.LineItemDiscount = a.Amount, b.Amount
			anchor := item("anchor", "2026-01-01T00:00:00Z", "2027-01-01T00:00:00Z")
			anchor.Amount = decimal.Zero
			return []*invoice.InvoiceLineItem{nil, anchor, a, b}
		}, "2026-10-12", "2026-11-11", 2},
		{"mixed periods independent of item order", func() []*invoice.InvoiceLineItem {
			return []*invoice.InvoiceLineItem{item("advance", oct, nov), item("arrears", sept, oct)}
		}, "2026-09-12", "2026-11-11", 2},
		{"arrears", func() []*invoice.InvoiceLineItem { return []*invoice.InvoiceLineItem{item("arrears", sept, oct)} }, "2026-09-12", "2026-10-11", 1},
		{"one time", func() []*invoice.InvoiceLineItem { return []*invoice.InvoiceLineItem{item("one-time", oct, oct)} }, "2026-10-12", "2026-10-12", 1},
		{"no dates does not fall back to billing cycle", func() []*invoice.InvoiceLineItem { return []*invoice.InvoiceLineItem{item("undated", "", "")} }, "", "", 1},
		{"missing price mapping still sent", func() []*invoice.InvoiceLineItem {
			li := item("custom", oct, nov)
			li.PriceID = nil
			return []*invoice.InvoiceLineItem{li}
		}, "2026-10-12", "2026-11-11", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv := &invoice.Invoice{ID: "inv_advance", CustomerID: "cust_1", Currency: "INR", PeriodStart: tPtr(sept), PeriodEnd: tPtr(oct), LineItems: tt.items()}
			client := &fakeSyncZohoClient{syncConfig: &types.SyncConfig{InvoiceSyncSettings: &types.InvoiceSyncSettings{
				ZohoInvoiceSyncSettings: types.ZohoInvoiceSyncSettings{ServicePeriodCustomFields: &types.ServicePeriodCustomFields{StartFieldID: "cf_start", EndFieldID: "cf_end"}},
			}}}
			svc := &InvoiceService{client: client, customerSvc: fakeSyncCustomerSvc{}, itemSyncSvc: &capturingItemSyncSvc{}, taxSvc: fakeSyncTaxSvc{}, customerRepo: &gstCustomerRepo{}, invoiceRepo: &fakeSyncInvoiceRepo{inv: inv}, priceRepo: &fakeHSNPriceRepo{prices: map[string]*price.Price{}}, mappingRepo: &fakeSyncMappingRepo{}, logger: logger.NewNoopLogger()}
			_, err := svc.SyncInvoiceToZoho(context.Background(), ZohoInvoiceSyncRequest{InvoiceID: inv.ID})
			require.NoError(t, err)
			require.NotNil(t, client.createInvoiceReq)
			require.Len(t, client.createInvoiceReq.LineItems, tt.count)
			if tt.wantStart == "" {
				assert.Empty(t, client.createInvoiceReq.CustomFields)
			} else {
				assert.Equal(t, []CustomField{{APIName: "cf_start", Value: tt.wantStart}, {APIName: "cf_end", Value: tt.wantEnd}}, client.createInvoiceReq.CustomFields)
			}
			assert.Equal(t, tPtr(sept), inv.PeriodStart)
			assert.Equal(t, tPtr(oct), inv.PeriodEnd)
		})
	}
}
