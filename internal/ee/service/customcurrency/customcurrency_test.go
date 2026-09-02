package customcurrency

import (
	"context"
	"testing"

	domainSettings "github.com/flexprice/flexprice/internal/domain/settings"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/utils"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestService builds a Service backed by an in-memory settings repo, optionally
// seeded with an org_custom_currency_config value.
func newTestService(t *testing.T, ctx context.Context, config *types.OrgCurrencyConfig) Service {
	t.Helper()
	repo := testutil.NewInMemorySettingsStore()
	if config != nil {
		require.NoError(t, config.Validate())
		value, err := utils.ToMap(config)
		require.NoError(t, err)
		require.NoError(t, repo.Create(ctx, &domainSettings.Setting{
			ID:            types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SETTING),
			Key:           types.SettingKeyOrgCustomCurrencyConfig,
			Value:         value,
			EnvironmentID: types.GetEnvironmentID(ctx),
			BaseModel: types.BaseModel{
				TenantID: types.GetTenantID(ctx),
				Status:   types.StatusPublished,
			},
		}))
	}
	return NewService(repo, logger.NewNoopLogger())
}

func macConfig() *types.OrgCurrencyConfig {
	return &types.OrgCurrencyConfig{
		CustomCurrencies: map[string]types.CustomCurrency{
			"mac": {
				Name:   "MoEngage AI Credits",
				Symbol: "MAC",
				FiatConversionFactors: map[string]decimal.Decimal{
					"usd": decimal.RequireFromString("0.1"),
					"inr": decimal.RequireFromString("8.5"),
				},
			},
		},
		DefaultFiatCurrency: "usd",
	}
}

func TestEnforceOrgCustomCurrency_EmptyConfig_PassesThroughLowercased(t *testing.T) {
	ctx := testutil.SetupContext()
	svc := newTestService(t, ctx, nil)

	got, err := svc.EnforceOrgCustomCurrency(ctx, "USD")
	require.NoError(t, err)
	assert.Equal(t, "usd", got)
}

func TestEnforceOrgCustomCurrency_Configured(t *testing.T) {
	ctx := testutil.SetupContext()
	svc := newTestService(t, ctx, macConfig())

	tests := []struct {
		name     string
		currency string
		wantOK   bool
		want     string
	}{
		{name: "the custom currency itself", currency: "MAC", wantOK: true, want: "mac"},
		{name: "the default fiat currency", currency: "usd", wantOK: true, want: "usd"},
		{name: "a fiat currency with a registered factor", currency: "INR", wantOK: true, want: "inr"},
		{name: "an unregistered fiat currency", currency: "eur", wantOK: false},
		{name: "a random unrelated code", currency: "xyz", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.EnforceOrgCustomCurrency(ctx, tt.currency)
			if tt.wantOK {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestResolveFiatCurrency(t *testing.T) {
	ctx := testutil.SetupContext()
	svc := newTestService(t, ctx, macConfig())

	t.Run("unconfigured custom currency returns empty, no error", func(t *testing.T) {
		got, err := svc.ResolveFiatCurrency(ctx, "usd", "")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("no requested currency falls back to default", func(t *testing.T) {
		got, err := svc.ResolveFiatCurrency(ctx, "mac", "")
		require.NoError(t, err)
		assert.Equal(t, "usd", got)
	})

	t.Run("requested currency with a registered factor is honored", func(t *testing.T) {
		got, err := svc.ResolveFiatCurrency(ctx, "MAC", "INR")
		require.NoError(t, err)
		assert.Equal(t, "inr", got)
	})

	t.Run("requested currency without a registered factor falls back to default", func(t *testing.T) {
		got, err := svc.ResolveFiatCurrency(ctx, "mac", "eur")
		require.NoError(t, err)
		assert.Equal(t, "usd", got)
	})
}

func TestFiatConversionRate(t *testing.T) {
	ctx := testutil.SetupContext()
	svc := newTestService(t, ctx, macConfig())

	t.Run("empty fiat currency means no conversion applies", func(t *testing.T) {
		rate, err := svc.FiatConversionRate(ctx, "mac", "")
		require.NoError(t, err)
		assert.Nil(t, rate)
	})

	t.Run("unknown custom currency errors", func(t *testing.T) {
		_, err := svc.FiatConversionRate(ctx, "notacurrency", "usd")
		require.Error(t, err)
	})

	t.Run("no factor for the requested fiat currency errors", func(t *testing.T) {
		_, err := svc.FiatConversionRate(ctx, "mac", "eur")
		require.Error(t, err)
	})

	t.Run("valid pair returns the configured rate", func(t *testing.T) {
		rate, err := svc.FiatConversionRate(ctx, "MAC", "USD")
		require.NoError(t, err)
		require.NotNil(t, rate)
		assert.True(t, rate.Equal(decimal.RequireFromString("0.1")))
	})
}

func TestFiatConversionRate_UnconfiguredTenant(t *testing.T) {
	ctx := testutil.SetupContext()
	svc := newTestService(t, ctx, nil)

	_, err := svc.FiatConversionRate(ctx, "mac", "usd")
	require.Error(t, err, "a tenant with no custom currency config has no known currencies")
}
