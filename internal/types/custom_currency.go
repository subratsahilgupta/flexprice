package types

import (
	"sort"
	"strings"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/validator"
	"github.com/shopspring/decimal"
)

// CustomCurrency is the custom-currency equivalent of an invoice whose own
// currency and amounts are always fiat. Nil when the tenant has none.
type CustomCurrency struct {
	CustomCurrencyCode string `json:"custom_currency_code"`
	// frozen at finalization; amount_due = custom_currency_amount * rate
	CustomConversionRate decimal.Decimal `json:"custom_conversion_rate" swaggertype:"string"`
	// the invoice total as computed, before conversion to fiat
	CustomCurrencyAmount decimal.Decimal `json:"custom_currency_amount" swaggertype:"string"`
}

// ToFiat converts an amount denominated in the custom currency into fiatCurrency.
//
//	fiat = custom * rate
//	custom = fiat / rate
//
// rate is fiat per 1 unit of custom currency, matching PriceUnit
// (fiat_amount = price_unit_amount * conversion_rate) and Wallet
// (amount = credits * conversion_rate).
//
// So a mac→usd factor of 10 means 1 MAC = 10 USD, hence 1 USD = 0.1 MAC.
// To make 1 USD = 10 MAC instead, store the factor as 0.1.
func (c *CustomCurrency) ToFiat(amtInCustomCurr decimal.Decimal, fiatCurrency string) decimal.Decimal {
	converted := amtInCustomCurr.Mul(c.CustomConversionRate)
	return RoundToCurrencyPrecision(converted, fiatCurrency)
}

// CustomCurrencyConfig is the tenant's custom currencies. Empty: no enforcement.
type CustomCurrencyConfig struct {
	// keyed by currency code; dive validates each entry's own tags
	CustomCurrencies map[string]CustomCurrencyDefinition `json:"custom_currencies" validate:"dive"`
	// currency invoices are denominated in; every custom currency needs a factor for it
	DefaultFiatCurrency string `json:"default_fiat_currency"`
}

// CustomCurrencyDefinition is one tenant-defined currency — never change its key once referenced.
type CustomCurrencyDefinition struct {
	Name                  string                     `json:"name" validate:"required"`
	Symbol                string                     `json:"symbol" validate:"required"`
	FiatConversionFactors map[string]decimal.Decimal `json:"fiat_conversion_factors" validate:"required,min=1"`
}

// Validate implements SettingConfig. Pointer receiver: it lowercases codes in place.
func (c *CustomCurrencyConfig) Validate() error {
	if len(c.CustomCurrencies) == 0 {
		return nil
	}

	if c.DefaultFiatCurrency == "" {
		return ierr.NewError("default_fiat_currency is required when custom_currencies is set").
			WithHint("default_fiat_currency is required when custom_currencies is set").
			Mark(ierr.ErrValidation)
	}
	c.DefaultFiatCurrency = strings.ToLower(c.DefaultFiatCurrency)
	defaultCode := c.DefaultFiatCurrency

	normalized := make(map[string]CustomCurrencyDefinition, len(c.CustomCurrencies))
	for key, cur := range c.CustomCurrencies {
		code := strings.ToLower(key)
		// Map order is undefined, so two keys normalizing to the same code would
		// silently keep whichever landed last.
		if _, exists := normalized[code]; exists {
			return ierr.NewErrorf("duplicate custom currency code %q", code).
				WithHintf("Custom currency codes are case-insensitive; %q is defined more than once", code).
				Mark(ierr.ErrValidation)
		}
		if code == defaultCode {
			return ierr.NewErrorf("default_fiat_currency %q cannot also be a custom currency code", defaultCode).
				WithHintf("default_fiat_currency %q cannot also be a custom currency code", defaultCode).
				Mark(ierr.ErrValidation)
		}

		factors := make(map[string]decimal.Decimal, len(cur.FiatConversionFactors))
		for fiat, rate := range cur.FiatConversionFactors {
			if _, exists := factors[strings.ToLower(fiat)]; exists {
				return ierr.NewErrorf("custom currency %q: duplicate conversion factor for %q", code, strings.ToLower(fiat)).
					WithHintf("Fiat currency codes are case-insensitive; %q is defined more than once", strings.ToLower(fiat)).
					Mark(ierr.ErrValidation)
			}
			if rate.LessThanOrEqual(decimal.Zero) {
				return ierr.NewErrorf("custom currency %q: conversion factor for %q must be positive", code, fiat).
					WithHintf("custom currency %q: conversion factor for %q must be positive", code, fiat).
					Mark(ierr.ErrValidation)
			}
			factors[strings.ToLower(fiat)] = rate
		}
		if _, ok := factors[defaultCode]; !ok {
			return ierr.NewErrorf("custom currency %q must have a conversion factor for the default fiat currency %q", code, defaultCode).
				WithHintf("custom currency %q must have a conversion factor for the default fiat currency %q", code, defaultCode).
				Mark(ierr.ErrValidation)
		}

		cur.FiatConversionFactors = factors
		normalized[code] = cur
	}
	c.CustomCurrencies = normalized

	return validator.ValidateRequest(c)
}

// EnforceCurrency restricts an entity — price, subscription, wallet, addon — to one of
// the tenant's custom currencies or its default fiat currency. Unconfigured tenants are
// unaffected. Fiat stays allowed because invoices are denominated in it, and a wallet
// must share the invoice's currency to pay it.
func (c CustomCurrencyConfig) EnforceCurrency(currency string) error {
	if len(c.CustomCurrencies) == 0 {
		return nil
	}
	currency = strings.ToLower(currency)
	if _, ok := c.CustomCurrencies[currency]; ok || currency == c.DefaultFiatCurrency {
		return nil
	}

	allowed := make([]string, 0, len(c.CustomCurrencies)+1)
	allowed = append(allowed, c.DefaultFiatCurrency)
	for code := range c.CustomCurrencies {
		allowed = append(allowed, code)
	}
	sort.Strings(allowed)
	return ierr.NewErrorf("currency must be one of: %s", strings.Join(allowed, ", ")).
		WithHint("This environment only accepts its configured custom currencies and its default fiat currency").
		Mark(ierr.ErrValidation)
}
