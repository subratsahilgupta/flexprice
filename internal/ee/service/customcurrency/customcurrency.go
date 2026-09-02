// Package customcurrency enforces and resolves a tenant's org_custom_currency_config.
package customcurrency

import (
	"context"
	"sort"
	"strings"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/domain/settings"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/utils"
	"github.com/shopspring/decimal"
)

// Service enforces and resolves the tenant's custom currency config.
type Service interface {
	// EnforceOrgCustomCurrency validates currency against the config. Empty config:
	// unchanged, today's behavior. Configured: must be a custom currency code, the
	// default fiat currency, or any fiat currency with a registered factor.
	EnforceOrgCustomCurrency(ctx context.Context, currency string) (string, error)

	// ResolveFiatCurrency picks the invoice's fiat currency: requested if the custom
	// currency has a factor for it, else the default. Empty return: not org-managed.
	ResolveFiatCurrency(ctx context.Context, customCurrency, requested string) (string, error)

	// FiatConversionRate returns the current factor from customCurrency to fiatCurrency.
	// Empty fiatCurrency: no conversion applies.
	FiatConversionRate(ctx context.Context, customCurrency, fiatCurrency string) (*decimal.Decimal, error)
}

type service struct {
	repo   settings.Repository
	logger *logger.Logger
}

// NewService builds a Service directly off the settings repository — org_custom_currency_config
// is environment-scoped, so GetByKey is the correct read; no settings-service layer needed.
func NewService(repo settings.Repository, log *logger.Logger) Service {
	return &service{repo: repo, logger: log}
}

// config reads org_custom_currency_config. Not found means unconfigured — the zero
// value of OrgCurrencyConfig already means exactly that (empty CustomCurrencies),
// so there is no separate default to merge in for this key.
func (s *service) config(ctx context.Context) (types.OrgCurrencyConfig, error) {
	setting, err := s.repo.GetByKey(ctx, types.SettingKeyOrgCustomCurrencyConfig)
	if ent.IsNotFound(err) {
		return types.OrgCurrencyConfig{}, nil
	}
	if err != nil {
		return types.OrgCurrencyConfig{}, err
	}
	return utils.ToStruct[types.OrgCurrencyConfig](setting.Value)
}

func (s *service) EnforceOrgCustomCurrency(ctx context.Context, currency string) (string, error) {
	currency = strings.ToLower(currency)

	cfg, err := s.config(ctx)
	if err != nil {
		return "", err
	}
	if len(cfg.CustomCurrencies) == 0 {
		return currency, nil
	}

	allowed := map[string]bool{cfg.DefaultFiatCurrency: true}
	for code, cur := range cfg.CustomCurrencies {
		allowed[code] = true
		for fiat := range cur.FiatConversionFactors {
			allowed[fiat] = true
		}
	}
	if allowed[currency] {
		return currency, nil
	}

	codes := make([]string, 0, len(allowed))
	for code := range allowed {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return "", ierr.NewErrorf("currency must be one of: %s", strings.Join(codes, ", ")).
		WithHint("This environment only accepts its configured custom and fiat currencies").
		Mark(ierr.ErrValidation)
}

func (s *service) ResolveFiatCurrency(ctx context.Context, customCurrency, requested string) (string, error) {
	cfg, err := s.config(ctx)
	if err != nil {
		return "", err
	}
	customCurrency = strings.ToLower(customCurrency)
	cur, ok := cfg.CustomCurrencies[customCurrency]
	if !ok {
		return "", nil
	}

	if requested != "" {
		requested = strings.ToLower(requested)
		if _, ok := cur.FiatConversionFactors[requested]; ok {
			return requested, nil
		}
		s.logger.Error(ctx, "no conversion factor for requested fiat currency, falling back to default",
			"error", "missing conversion factor",
			"custom_currency", customCurrency, "requested", requested, "default", cfg.DefaultFiatCurrency)
	}
	return cfg.DefaultFiatCurrency, nil
}

func (s *service) FiatConversionRate(ctx context.Context, customCurrency, fiatCurrency string) (*decimal.Decimal, error) {
	if fiatCurrency == "" {
		return nil, nil
	}
	cfg, err := s.config(ctx)
	if err != nil {
		return nil, err
	}
	cur, ok := cfg.CustomCurrencies[strings.ToLower(customCurrency)]
	if !ok {
		return nil, ierr.NewErrorf("currency %q is not configured", customCurrency).Mark(ierr.ErrValidation)
	}
	rate, ok := cur.FiatConversionFactors[strings.ToLower(fiatCurrency)]
	if !ok {
		return nil, ierr.NewErrorf("no conversion factor configured for %s to %s", customCurrency, fiatCurrency).Mark(ierr.ErrValidation)
	}
	return &rate, nil
}
