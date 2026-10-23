package export

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/wallet"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestWalletLedgerCSVRow(t *testing.T) {
	d := func(v string) decimal.Decimal { return decimal.RequireFromString(v) }
	created := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	paid := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		row  wallet.WalletLedgerExportData
		want WalletLedgerCSV
	}{
		{
			name: "purchase carries what was paid and when it was paid",
			row: wallet.WalletLedgerExportData{
				Type: types.TransactionTypeCredit, Credits: d("500"), AmountPaid: lo.ToPtr(d("450.004")),
				TopUpAmountRemainingBefore: d("0"), TopUpAmountRemainingAfter: d("450.004"),
				EffectiveAt: paid, CreatedAt: created,
			},
			want: WalletLedgerCSV{Credits: "500", AmountPaid: "450", TopUpAmountUsed: "", TopUpAmountRemainingAfter: "450", EffectiveAt: "2026-10-03T12:00:00Z", CreatedAt: "2026-10-01T00:00:00Z"},
		},
		{
			name: "adjustment has no paid amount",
			row:  wallet.WalletLedgerExportData{Type: types.TransactionTypeCredit, Credits: d("40")},
			want: WalletLedgerCSV{Credits: "40", AmountPaid: ""},
		},
		{
			name: "debit is negative and takes its share of the batch",
			row: wallet.WalletLedgerExportData{
				Type: types.TransactionTypeDebit, Credits: d("200"),
				WalletTxnCredits: lo.ToPtr(d("500")), WalletTxnAmountPaid: lo.ToPtr(d("450")),
				InvoicePeriodStart: lo.ToPtr(created), InvoicePeriodEnd: lo.ToPtr(paid),
			},
			want: WalletLedgerCSV{Credits: "-200", AmountPaid: "", TopUpAmountUsed: "180", InvoicePeriodStart: "2026-10-01T00:00:00Z", InvoicePeriodEnd: "2026-10-03T12:00:00Z"},
		},
		{
			name: "debit from credits with no paid amount",
			row:  wallet.WalletLedgerExportData{Type: types.TransactionTypeDebit, Credits: d("5")},
			want: WalletLedgerCSV{Credits: "-5", TopUpAmountUsed: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.row.Currency = "usd"
			got := toWalletLedgerCSV(&tt.row)
			assert.Equal(t, string(tt.row.Type), got.Type)
			assert.Equal(t, tt.want.Credits, got.Credits)
			assert.Equal(t, tt.want.AmountPaid, got.AmountPaid)
			assert.Equal(t, tt.want.TopUpAmountUsed, got.TopUpAmountUsed)
			assert.Equal(t, tt.want.InvoicePeriodStart, got.InvoicePeriodStart)
			assert.Equal(t, tt.want.InvoicePeriodEnd, got.InvoicePeriodEnd)
			if tt.want.TopUpAmountRemainingAfter != "" {
				assert.Equal(t, tt.want.TopUpAmountRemainingAfter, got.TopUpAmountRemainingAfter)
			}
			if tt.want.EffectiveAt != "" {
				assert.Equal(t, tt.want.EffectiveAt, got.EffectiveAt)
				assert.Equal(t, tt.want.CreatedAt, got.CreatedAt)
			}
		})
	}
}
