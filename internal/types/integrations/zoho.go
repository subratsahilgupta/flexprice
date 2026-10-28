package integrations

import (
	"github.com/flexprice/flexprice/internal/types"
)

// ZohoPaymentGateway is a Zoho Books online payment gateway backed by a Flexprice connection.
type ZohoPaymentGateway string

const (
	ZohoPaymentGatewayStripe   ZohoPaymentGateway = "stripe"
	ZohoPaymentGatewayRazorpay ZohoPaymentGateway = "razorpay"
)

// ZohoPaymentGateways lists the gateways attached to synced Zoho invoices when connected.
func ZohoPaymentGateways() []ZohoPaymentGateway {
	return []ZohoPaymentGateway{ZohoPaymentGatewayStripe, ZohoPaymentGatewayRazorpay}
}

// GatewayProvider returns the provider of the Flexprice connection that backs the gateway.
func (g ZohoPaymentGateway) GatewayProvider() types.SecretProvider {
	switch g {
	case ZohoPaymentGatewayStripe:
		return types.SecretProviderStripe
	case ZohoPaymentGatewayRazorpay:
		return types.SecretProviderRazorpay
	default:
		return ""
	}
}
