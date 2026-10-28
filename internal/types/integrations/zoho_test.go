package integrations

import (
	"testing"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestZohoPaymentGatewayProvider(t *testing.T) {
	cases := []struct {
		gateway ZohoPaymentGateway
		want    types.SecretProvider
	}{
		{ZohoPaymentGatewayStripe, types.SecretProviderStripe},
		{ZohoPaymentGatewayRazorpay, types.SecretProviderRazorpay},
		{ZohoPaymentGateway("paypal"), ""},
	}

	for _, c := range cases {
		t.Run(string(c.gateway), func(t *testing.T) {
			assert.Equal(t, c.want, c.gateway.GatewayProvider())
		})
	}
}

// A gateway listed without a provider would never find its connection and silently drop off invoices.
func TestZohoPaymentGatewaysHaveProviders(t *testing.T) {
	for _, gateway := range ZohoPaymentGateways() {
		assert.NotEmpty(t, gateway.GatewayProvider(), "gateway %q has no provider", gateway)
	}
}
