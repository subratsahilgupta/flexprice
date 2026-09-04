package webhook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/flexprice/flexprice/internal/integration/chargebee"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type recordingInvoiceSvc struct {
	chargebee.ChargebeeInvoiceService
	flexInvoiceID string
	gotAmount     decimal.Decimal
	gotCurrency   string
}

func (s *recordingInvoiceSvc) GetFlexPriceInvoiceIDByChargebeeInvoiceID(context.Context, string) (string, error) {
	return s.flexInvoiceID, nil
}

func (s *recordingInvoiceSvc) LinkInvoiceMapping(context.Context, string, string) error { return nil }

func (s *recordingInvoiceSvc) ProcessChargebeePaymentFromWebhook(
	_ context.Context, req chargebee.ChargebeeWebhookPaymentRequest,
) error {
	s.gotAmount = req.Amount
	s.gotCurrency = req.Currency
	return nil
}

// The reconciled amount must follow the currency's own precision. A hardcoded /100
// undercharges JPY (0 decimals) by 100x and would overcharge a 3-decimal currency.
func TestPaymentSucceeded_ReconciledAmountUsesCurrencyPrecision(t *testing.T) {
	tests := []struct {
		name        string
		currency    string
		amountMinor int64
		want        string
	}{
		{"JPY has no minor unit", "JPY", 5000, "5000"},
		{"USD has two", "USD", 5000, "50"},
		{"KWD has three", "KWD", 5000, "5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invoiceSvc := &recordingInvoiceSvc{flexInvoiceID: "inv_flex_1"}
			h := NewHandler(nil, invoiceSvc, nil, logger.NewNoopLogger())

			content, err := json.Marshal(map[string]any{
				"transaction": map[string]any{
					"id": "txn_1", "amount": tt.amountMinor, "currency_code": tt.currency,
				},
				"invoice": map[string]any{
					"id":    "cb_inv_1",
					"notes": []map[string]any{{"note": chargebee.PaymentNote("pay_flex_1")}},
				},
			})
			require.NoError(t, err)

			require.NoError(t, h.handlePaymentSucceeded(context.Background(),
				&ChargebeeWebhookEvent{ID: "ev_1", EventType: string(EventPaymentSucceeded), Content: content},
				&ServiceDependencies{}))

			require.Equal(t, tt.want, invoiceSvc.gotAmount.String())
			require.Equal(t, tt.currency, invoiceSvc.gotCurrency)
		})
	}
}
