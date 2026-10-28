package zoho

import (
	"context"
	"errors"
	"testing"

	"github.com/flexprice/flexprice/internal/domain/connection"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/types/integrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGatewayConnectionRepo serves published connections for the given providers.
type fakeGatewayConnectionRepo struct {
	connection.Repository
	providers []types.SecretProvider
	err       error
	calls     int
}

func (f *fakeGatewayConnectionRepo) GetByProvider(_ context.Context, provider types.SecretProvider) (*connection.Connection, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	for _, p := range f.providers {
		if p == provider {
			return &connection.Connection{ProviderType: provider}, nil
		}
	}
	return nil, ierr.NewError("connection not found").Mark(ierr.ErrNotFound)
}

func TestPaymentOptions(t *testing.T) {
	attachEnabled := &types.InvoiceSyncSettings{
		ZohoInvoiceSyncSettings: types.ZohoInvoiceSyncSettings{AttachPaymentGateways: true},
	}
	stripeOnly := &PaymentOptions{PaymentGateways: []PaymentGateway{
		{Configured: true, GatewayName: integrations.ZohoPaymentGatewayStripe},
	}}
	stripeAndRazorpay := &PaymentOptions{PaymentGateways: []PaymentGateway{
		{Configured: true, GatewayName: integrations.ZohoPaymentGatewayStripe},
		{Configured: true, GatewayName: integrations.ZohoPaymentGatewayRazorpay},
	}}

	cases := []struct {
		name          string
		settings      *types.InvoiceSyncSettings
		providers     []types.SecretProvider
		repoErr       error
		want          *PaymentOptions
		wantErr       bool
		wantRepoCalls bool
	}{
		{
			name:      "no settings attaches nothing",
			settings:  nil,
			providers: []types.SecretProvider{types.SecretProviderStripe},
		},
		{
			name:      "toggle off attaches nothing",
			settings:  &types.InvoiceSyncSettings{},
			providers: []types.SecretProvider{types.SecretProviderStripe},
		},
		{
			name:          "toggle on with stripe connected attaches stripe",
			settings:      attachEnabled,
			providers:     []types.SecretProvider{types.SecretProviderStripe},
			want:          stripeOnly,
			wantRepoCalls: true,
		},
		{
			name:          "toggle on with stripe and razorpay connected attaches both",
			settings:      attachEnabled,
			providers:     []types.SecretProvider{types.SecretProviderRazorpay, types.SecretProviderStripe},
			want:          stripeAndRazorpay,
			wantRepoCalls: true,
		},
		{
			name:      "toggle on with only razorpay connected attaches razorpay",
			settings:  attachEnabled,
			providers: []types.SecretProvider{types.SecretProviderRazorpay},
			want: &PaymentOptions{PaymentGateways: []PaymentGateway{
				{Configured: true, GatewayName: integrations.ZohoPaymentGatewayRazorpay},
			}},
			wantRepoCalls: true,
		},
		{
			name:          "toggle on with stripe and paddle connected attaches only stripe",
			settings:      attachEnabled,
			providers:     []types.SecretProvider{types.SecretProviderPaddle, types.SecretProviderStripe},
			want:          stripeOnly,
			wantRepoCalls: true,
		},
		{
			name:          "toggle on with only paddle connected attaches nothing",
			settings:      attachEnabled,
			providers:     []types.SecretProvider{types.SecretProviderPaddle},
			wantRepoCalls: true,
		},
		{
			name:          "toggle on with no connections attaches nothing",
			settings:      attachEnabled,
			wantRepoCalls: true,
		},
		{
			name:          "connection lookup failure is returned",
			settings:      attachEnabled,
			repoErr:       errors.New("db unavailable"),
			wantErr:       true,
			wantRepoCalls: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeGatewayConnectionRepo{providers: c.providers, err: c.repoErr}
			svc := &InvoiceService{connectionRepo: repo, logger: logger.NewNoopLogger()}

			got, err := svc.paymentOptions(context.Background(), "inv_1", c.settings)

			assert.Equal(t, c.wantRepoCalls, repo.calls > 0)
			if c.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}
