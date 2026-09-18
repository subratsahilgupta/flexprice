package service

import (
	"context"
	"time"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

const defaultAutoChargeCooldown = time.Hour

// fetchGatewayWithAutoChargeSupport returns a gateway that can charge this customer
// off-session, or "" when none can. A provider that cannot be reached is an error,
// not an empty result: charging is refused on evidence, never on a failed read.
func fetchGatewayWithAutoChargeSupport(
	ctx context.Context,
	params ServiceParams,
	customerSvc interfaces.CustomerService,
	customerID string,
) (types.PaymentGatewayType, error) {
	if params.IntegrationFactory == nil || params.ConnectionRepo == nil {
		return "", nil
	}

	gateways, err := NewPaymentProviderResolver(params).ConfiguredGateways(ctx)
	if err != nil {
		return "", err
	}

	for _, gw := range gateways {
		switch gw {
		case types.PaymentGatewayTypeRazorpay:
			rzp, err := params.IntegrationFactory.GetRazorpayIntegration(ctx)
			if err != nil {
				return "", ierr.WithError(err).
					WithHint("The payment provider could not be reached; try again shortly").
					Mark(ierr.ErrHTTPClient)
			}
			_, tokens, err := rzp.CustomerSvc.ListConfirmedCustomerTokens(ctx, customerID)
			if err != nil {
				if ierr.IsNotFound(err) {
					continue
				}
				return "", ierr.WithError(err).
					WithHint("The payment provider could not be reached; try again shortly").
					Mark(ierr.ErrHTTPClient)
			}
			if len(tokens) > 0 {
				return gw, nil
			}
		default:
			provider, err := params.IntegrationFactory.GetPaymentMethodProvider(ctx, gw, customerSvc)
			if err != nil {
				if ierr.IsNotImplemented(err) {
					continue
				}
				return "", ierr.WithError(err).
					WithHint("The payment provider could not be reached; try again shortly").
					Mark(ierr.ErrHTTPClient)
			}

			methods, err := provider.ListSavedMethods(ctx, customerID)
			if err != nil {
				if ierr.IsNotFound(err) {
					continue
				}
				return "", ierr.WithError(err).
					WithHint("The payment provider could not be reached; try again shortly").
					Mark(ierr.ErrHTTPClient)
			}

			// Not IsDefault: the adapters charge the primary method when there is one and
			// the first valid one otherwise, so requiring a default would refuse customers
			// they can charge.
			if lo.ContainsBy(methods, func(m interfaces.ProviderPaymentMethod) bool { return m.Active }) {
				return gw, nil
			}
		}
	}

	return "", nil
}
