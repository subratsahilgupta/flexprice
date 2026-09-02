package service

import (
	"testing"

	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/payment"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveGatewayCurrencyAmount(t *testing.T) {
	p := &paymentProcessor{}

	paymentObj := &payment.Payment{
		Amount:   decimal.RequireFromString("10"),
		Currency: "mac",
	}

	t.Run("nil invoice passes the payment's own currency and amount through unchanged", func(t *testing.T) {
		gotCurrency, gotAmount, err := p.resolveGatewayCurrencyAmount(paymentObj, nil)
		require.NoError(t, err)
		assert.Equal(t, "mac", gotCurrency)
		assert.True(t, gotAmount.Equal(paymentObj.Amount))
	})

	t.Run("invoice with no target currency passes through unchanged", func(t *testing.T) {
		inv := &invoice.Invoice{ID: "inv_1", Currency: "mac"}
		gotCurrency, gotAmount, err := p.resolveGatewayCurrencyAmount(paymentObj, inv)
		require.NoError(t, err)
		assert.Equal(t, "mac", gotCurrency)
		assert.True(t, gotAmount.Equal(paymentObj.Amount))
	})

	t.Run("target currency with an unfrozen (zero) rate errors instead of charging zero", func(t *testing.T) {
		inv := &invoice.Invoice{
			ID:       "inv_2",
			Currency: "mac",
			TargetCurrency: &types.TargetCurrency{
				FiatCurrencyCode: "usd",
			},
		}
		_, _, err := p.resolveGatewayCurrencyAmount(paymentObj, inv)
		require.Error(t, err)
	})

	t.Run("target currency with a frozen rate converts and rounds to the fiat currency's precision", func(t *testing.T) {
		inv := &invoice.Invoice{
			ID:       "inv_3",
			Currency: "mac",
			TargetCurrency: &types.TargetCurrency{
				FiatCurrencyCode:   "usd",
				FiatConversionRate: decimal.RequireFromString("0.105"),
			},
		}
		gotCurrency, gotAmount, err := p.resolveGatewayCurrencyAmount(paymentObj, inv)
		require.NoError(t, err)
		assert.Equal(t, "usd", gotCurrency)
		// 10 * 0.105 = 1.05, USD precision is 2 decimals
		assert.True(t, gotAmount.Equal(decimal.RequireFromString("1.05")), "got %s", gotAmount.String())
	})

	t.Run("conversion rounds to the target fiat currency's precision, not the source's", func(t *testing.T) {
		jpyPaymentObj := &payment.Payment{
			Amount:   decimal.RequireFromString("100"),
			Currency: "mac",
		}
		inv := &invoice.Invoice{
			ID:       "inv_4",
			Currency: "mac",
			TargetCurrency: &types.TargetCurrency{
				FiatCurrencyCode:   "jpy",
				FiatConversionRate: decimal.RequireFromString("1.005"),
			},
		}
		gotCurrency, gotAmount, err := p.resolveGatewayCurrencyAmount(jpyPaymentObj, inv)
		require.NoError(t, err)
		assert.Equal(t, "jpy", gotCurrency)
		// 100 * 1.005 = 100.5, JPY has 0 decimal places -> rounds to 101
		assert.True(t, gotAmount.Equal(decimal.RequireFromString("101")), "got %s", gotAmount.String())
	})
}
