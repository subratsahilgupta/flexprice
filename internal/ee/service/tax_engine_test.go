package service

import (
	"context"
	"testing"

	"github.com/flexprice/flexprice/internal/domain/connection"
	"github.com/flexprice/flexprice/internal/domain/settings"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/suite"
)

// TaxEngineSelectionSuite covers which engine a tenant resolves to, which is the only place
// in the system that names a provider.
type TaxEngineSelectionSuite struct {
	testutil.BaseServiceTestSuite
	params ServiceParams
}

func TestTaxEngineSelection(t *testing.T) {
	suite.Run(t, new(TaxEngineSelectionSuite))
}

func (s *TaxEngineSelectionSuite) SetupTest() {
	s.BaseServiceTestSuite.SetupTest()
	s.WithEnvironment("env_tax_engine")
	st := s.GetStores()
	s.params = ServiceParams{
		Logger:         s.GetLogger(),
		Config:         s.GetConfig(),
		DB:             s.GetDB(),
		SettingsRepo:   st.SettingsRepo,
		ConnectionRepo: st.ConnectionRepo,
		TaxRateRepo:    st.TaxRateRepo,
		TaxAppliedRepo: st.TaxAppliedRepo,
		InvoiceRepo:    st.InvoiceRepo,
		CustomerRepo:   st.CustomerRepo,
	}
}

func (s *TaxEngineSelectionSuite) TearDownTest() {
	s.BaseServiceTestSuite.TearDownTest()
}

// seedProvider writes an invoice config naming the tax provider. A partial value map is
// enough: the settings service overlays it onto the defaults.
func (s *TaxEngineSelectionSuite) seedProvider(ctx context.Context, provider string) {
	value := map[string]interface{}{}
	if provider != "" {
		value["tax_provider"] = provider
	}
	s.Require().NoError(s.GetStores().SettingsRepo.Create(ctx, &settings.Setting{
		ID:            types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SETTING),
		Key:           types.SettingKeyInvoiceConfig,
		Value:         value,
		EnvironmentID: types.GetEnvironmentID(ctx),
		BaseModel:     types.GetDefaultBaseModel(ctx),
	}))
}

func (s *TaxEngineSelectionSuite) seedStripeConnection(ctx context.Context) {
	s.Require().NoError(s.GetStores().ConnectionRepo.Create(ctx, &connection.Connection{
		ID:            types.GenerateUUIDWithPrefix("conn"),
		Name:          "stripe",
		ProviderType:  types.SecretProviderStripe,
		EnvironmentID: types.GetEnvironmentID(ctx),
		BaseModel:     types.GetDefaultBaseModel(ctx),
	}))
}

func (s *TaxEngineSelectionSuite) TestNewTaxEngine_SettingAbsentResolvesToNative() {
	engine, err := NewTaxEngine(s.GetContext(), s.params)

	s.Require().NoError(err, "a tenant that never configured tax must keep working")
	s.Equal(types.TaxProviderFlexprice, engine.GetProvider())
	s.False(engine.GetProvider().IsExternal())
}

func (s *TaxEngineSelectionSuite) TestNewTaxEngine_SettingPresentButProviderEmptyResolvesToNative() {
	ctx := s.GetContext()
	s.seedProvider(ctx, "")

	engine, err := NewTaxEngine(ctx, s.params)

	s.Require().NoError(err)
	s.Equal(types.TaxProviderFlexprice, engine.GetProvider())
}

func (s *TaxEngineSelectionSuite) TestNewTaxEngine_FlexpriceResolvesToNative() {
	ctx := s.GetContext()
	s.seedProvider(ctx, string(types.TaxProviderFlexprice))

	engine, err := NewTaxEngine(ctx, s.params)

	s.Require().NoError(err)
	s.Equal(types.TaxProviderFlexprice, engine.GetProvider())
}

func (s *TaxEngineSelectionSuite) TestNewTaxEngine_StripeWithConnectionResolvesToStripe() {
	ctx := s.GetContext()
	s.seedProvider(ctx, string(types.TaxProviderStripe))
	s.seedStripeConnection(ctx)

	engine, err := NewTaxEngine(ctx, s.params)

	s.Require().NoError(err)
	s.Equal(types.TaxProviderStripe, engine.GetProvider())
	s.True(engine.GetProvider().IsExternal())
}

func (s *TaxEngineSelectionSuite) TestNewTaxEngine_StripeWithoutConnectionIsAnErrorNotNative() {
	ctx := s.GetContext()
	s.seedProvider(ctx, string(types.TaxProviderStripe))

	engine, err := NewTaxEngine(ctx, s.params)

	s.Require().Error(err, "falling back to native would bill Flexprice rates under a Stripe configuration")
	s.Nil(engine)
	s.True(ierr.IsValidation(err), "the tenant has to connect Stripe, so this is theirs to fix")
}

func (s *TaxEngineSelectionSuite) TestNewTaxEngine_UnknownProviderIsAnError() {
	ctx := s.GetContext()
	s.seedProvider(ctx, "avalara")

	engine, err := NewTaxEngine(ctx, s.params)

	s.Require().Error(err, "a typo must not silently change how a tenant is taxed")
	s.Nil(engine)
	s.True(ierr.IsValidation(err))
}

func (s *TaxEngineSelectionSuite) TestNewTaxEngine_EnvironmentsOfOneTenantResolveIndependently() {
	stripeCtx := types.SetEnvironmentID(s.BaseServiceTestSuite.GetContext(), "env_stripe")
	nativeCtx := types.SetEnvironmentID(s.BaseServiceTestSuite.GetContext(), "env_native")

	s.seedProvider(stripeCtx, string(types.TaxProviderStripe))
	s.seedStripeConnection(stripeCtx)
	s.seedProvider(nativeCtx, string(types.TaxProviderFlexprice))

	stripeEngine, err := NewTaxEngine(stripeCtx, s.params)
	s.Require().NoError(err)
	s.Equal(types.TaxProviderStripe, stripeEngine.GetProvider())

	nativeEngine, err := NewTaxEngine(nativeCtx, s.params)
	s.Require().NoError(err)
	s.Equal(types.TaxProviderFlexprice, nativeEngine.GetProvider())
}

func (s *TaxEngineSelectionSuite) TestNativeEngine_CommitRecordsNothing() {
	transactionID, err := (&nativeTaxEngine{ServiceParams: s.params}).Commit(s.GetContext(), nil, nil)

	s.Require().NoError(err, "native tax is filed by the tenant, so there is nothing to record")
	s.Empty(transactionID)
}
