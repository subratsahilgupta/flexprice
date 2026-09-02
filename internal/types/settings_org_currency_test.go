package types

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrgCurrencyConfig_Validate(t *testing.T) {
	dec := decimal.RequireFromString

	tests := []struct {
		name    string
		config  OrgCurrencyConfig
		wantErr string // substring, empty means no error expected
	}{
		{
			name:   "empty custom currencies is valid, no enforcement",
			config: OrgCurrencyConfig{},
		},
		{
			name: "missing default_fiat_currency",
			config: OrgCurrencyConfig{
				CustomCurrencies: map[string]CustomCurrency{
					"mac": {Name: "Credits", Symbol: "MAC", FiatConversionFactors: map[string]decimal.Decimal{"usd": dec("0.1")}},
				},
			},
			wantErr: "default_fiat_currency is required",
		},
		{
			name: "default_fiat_currency also a custom currency code",
			config: OrgCurrencyConfig{
				CustomCurrencies: map[string]CustomCurrency{
					"usd": {Name: "Credits", Symbol: "USD", FiatConversionFactors: map[string]decimal.Decimal{"usd": dec("0.1")}},
				},
				DefaultFiatCurrency: "usd",
			},
			wantErr: "cannot also be a custom currency code",
		},
		{
			name: "non-positive conversion factor",
			config: OrgCurrencyConfig{
				CustomCurrencies: map[string]CustomCurrency{
					"mac": {Name: "Credits", Symbol: "MAC", FiatConversionFactors: map[string]decimal.Decimal{"usd": dec("0")}},
				},
				DefaultFiatCurrency: "usd",
			},
			wantErr: "must be positive",
		},
		{
			name: "negative conversion factor",
			config: OrgCurrencyConfig{
				CustomCurrencies: map[string]CustomCurrency{
					"mac": {Name: "Credits", Symbol: "MAC", FiatConversionFactors: map[string]decimal.Decimal{"usd": dec("-1")}},
				},
				DefaultFiatCurrency: "usd",
			},
			wantErr: "must be positive",
		},
		{
			name: "missing conversion factor for default fiat currency",
			config: OrgCurrencyConfig{
				CustomCurrencies: map[string]CustomCurrency{
					"mac": {Name: "Credits", Symbol: "MAC", FiatConversionFactors: map[string]decimal.Decimal{"inr": dec("10")}},
				},
				DefaultFiatCurrency: "usd",
			},
			wantErr: "must have a conversion factor for the default fiat currency",
		},
		{
			name: "missing required name",
			config: OrgCurrencyConfig{
				CustomCurrencies: map[string]CustomCurrency{
					"mac": {Symbol: "MAC", FiatConversionFactors: map[string]decimal.Decimal{"usd": dec("0.1")}},
				},
				DefaultFiatCurrency: "usd",
			},
			wantErr: "validation",
		},
		{
			name: "missing required symbol",
			config: OrgCurrencyConfig{
				CustomCurrencies: map[string]CustomCurrency{
					"mac": {Name: "Credits", FiatConversionFactors: map[string]decimal.Decimal{"usd": dec("0.1")}},
				},
				DefaultFiatCurrency: "usd",
			},
			wantErr: "validation",
		},
		{
			name: "valid single currency",
			config: OrgCurrencyConfig{
				CustomCurrencies: map[string]CustomCurrency{
					"mac": {Name: "Credits", Symbol: "MAC", FiatConversionFactors: map[string]decimal.Decimal{"usd": dec("0.1"), "inr": dec("8.5")}},
				},
				DefaultFiatCurrency: "usd",
			},
		},
		{
			name: "valid multiple currencies",
			config: OrgCurrencyConfig{
				CustomCurrencies: map[string]CustomCurrency{
					"mac": {Name: "Credits A", Symbol: "MAC", FiatConversionFactors: map[string]decimal.Decimal{"usd": dec("0.1")}},
					"fxp": {Name: "Credits B", Symbol: "FXP", FiatConversionFactors: map[string]decimal.Decimal{"usd": dec("0.5")}},
				},
				DefaultFiatCurrency: "usd",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestOrgCurrencyConfig_Validate_NormalizesToLowercase(t *testing.T) {
	config := OrgCurrencyConfig{
		CustomCurrencies: map[string]CustomCurrency{
			"MAC": {Name: "Credits", Symbol: "MAC", FiatConversionFactors: map[string]decimal.Decimal{"USD": decimal.RequireFromString("0.1")}},
		},
		DefaultFiatCurrency: "USD",
	}

	require.NoError(t, config.Validate())

	assert.Equal(t, "usd", config.DefaultFiatCurrency)
	_, hasUpper := config.CustomCurrencies["MAC"]
	assert.False(t, hasUpper, "uppercase key should have been normalized away")
	cur, hasLower := config.CustomCurrencies["mac"]
	require.True(t, hasLower, "expected lowercase key after normalization")
	_, hasFactorUpper := cur.FiatConversionFactors["USD"]
	assert.False(t, hasFactorUpper, "factor key should have been lowercased")
	_, hasFactorLower := cur.FiatConversionFactors["usd"]
	assert.True(t, hasFactorLower)
}
