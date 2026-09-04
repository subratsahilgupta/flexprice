package types

import (
	"sort"
	"strings"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/validator"
	"github.com/shopspring/decimal"
)

// CustomCurrency is an invoice's custom-currency ledger. The invoice's own currency
// and amount columns are fiat projections of these values. Nil for fiat invoices.
//
// AmountPaid and AmountRemaining are absent: payments settle in fiat, so a stored
// copy would drift. Derive them with FromFiat.
type CustomCurrency struct {
	Code string `json:"code"`

	// fiat per 1 unit of Code; the live factor while draft, frozen at finalization
	Rate decimal.Decimal `json:"rate" swaggertype:"string"`

	Subtotal                   decimal.Decimal `json:"subtotal" swaggertype:"string"`
	TotalDiscount              decimal.Decimal `json:"total_discount" swaggertype:"string"`
	TotalTax                   decimal.Decimal `json:"total_tax" swaggertype:"string"`
	TotalPrepaidCreditsApplied decimal.Decimal `json:"total_prepaid_credits_applied" swaggertype:"string"`
	Total                      decimal.Decimal `json:"total" swaggertype:"string"`
	AmountDue                  decimal.Decimal `json:"amount_due" swaggertype:"string"`
}

// CustomCurrencyLineItem is a line item's custom-currency ledger.
type CustomCurrencyLineItem struct {
	Amount                decimal.Decimal `json:"amount" swaggertype:"string"`
	LineItemDiscount      decimal.Decimal `json:"line_item_discount" swaggertype:"string"`
	InvoiceLevelDiscount  decimal.Decimal `json:"invoice_level_discount" swaggertype:"string"`
	PrepaidCreditsApplied decimal.Decimal `json:"prepaid_credits_applied" swaggertype:"string"`
}

// ToFiat converts a custom-currency amount to fiat: fiat = custom * rate.
// Rate is fiat per 1 unit, so a mac->usd factor of 0.10 means 1 MAC = $0.10.
func (c *CustomCurrency) ToFiat(amount decimal.Decimal, fiatCurrency string) decimal.Decimal {
	converted := amount.Mul(c.Rate)
	return RoundToCurrencyPrecision(converted, fiatCurrency)
}

// FromFiat restates a fiat amount in the custom currency. Only for amounts with no
// ledger form: tax and payments. Read anything else from the ledger directly.
func (c *CustomCurrency) FromFiat(amount decimal.Decimal) decimal.Decimal {
	if !c.Rate.IsPositive() {
		return decimal.Zero
	}
	restated := amount.Div(c.Rate)
	return RoundToCurrencyPrecision(restated, c.Code)
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

// IsCustom reports whether currency is one of the tenant's custom currencies.
func (c CustomCurrencyConfig) IsCustom(currency string) bool {
	_, ok := c.CustomCurrencies[strings.ToLower(currency)]
	return ok
}

// RateFor returns the conversion factor from a custom currency to fiatCurrency,
// or zero when either is not configured.
func (c CustomCurrencyConfig) RateFor(code, fiatCurrency string) decimal.Decimal {
	cur, ok := c.CustomCurrencies[strings.ToLower(code)]
	if !ok {
		return decimal.Zero
	}
	return cur.FiatConversionFactors[strings.ToLower(fiatCurrency)]
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

// EnforceCurrency restricts a price, subscription, wallet or addon to a configured
// custom currency or the default fiat. Unconfigured tenants are unaffected.
func (c CustomCurrencyConfig) EnforceCurrency(currency string) error {
	if len(c.CustomCurrencies) == 0 {
		return nil
	}
	currency = strings.ToLower(currency)
	if c.IsCustom(currency) || currency == c.DefaultFiatCurrency {
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
