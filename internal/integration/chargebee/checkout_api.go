package chargebee

import (
	"context"
	"strings"

	"github.com/chargebee/chargebee-go/v3/actions/hostedpage"
	hostedPageModel "github.com/chargebee/chargebee-go/v3/models/hostedpage"
	"github.com/samber/lo"
)

type HostedCheckoutPageRequest struct {
	ChargebeeCustomerID string
	Currency            string
	AmountMinor         int64
	Description         string
	RedirectURL         string
	GatewayAccountID    string
	// PoNumber stamps a reference onto the invoice this page creates. Verified live:
	// Chargebee echoes it on payment_succeeded, the only thread tying that webhook
	// back to a Flexprice entity — the page's invoice is Chargebee's, so no mapping
	// of ours exists for it.
	PoNumber string
}

// CreateHostedCheckoutPage returns an invoice-scoped hosted checkout for an exact
// ad-hoc amount — unlike collect_now it charges only what we ask for.
//
// Requires One-Time Checkout enabled on the site (Checkout & Self Serve Portal →
// One time payments), else Chargebee returns one_time_checkout_not_enabled_in_hp.
// That same setting, not payment_method_save_policy, controls vaulting: this
// endpoint accepts the policy param but ignores it, leaving the card unsaved.
func (c *Client) CreateHostedCheckoutPage(
	ctx context.Context,
	req HostedCheckoutPageRequest,
) (*hostedPageModel.HostedPage, error) {
	env, err := c.env(ctx)
	if err != nil {
		return nil, err
	}

	params := &hostedPageModel.CheckoutOneTimeForItemsRequestParams{
		Customer:     &hostedPageModel.CheckoutOneTimeForItemsCustomerParams{Id: req.ChargebeeCustomerID},
		CurrencyCode: strings.ToUpper(req.Currency),
		Charges: []*hostedPageModel.CheckoutOneTimeForItemsChargeParams{{
			Amount:      lo.ToPtr(req.AmountMinor),
			Description: req.Description,
		}},
		RedirectUrl: req.RedirectURL,
	}
	if req.GatewayAccountID != "" {
		params.Card = &hostedPageModel.CheckoutOneTimeForItemsCardParams{GatewayAccountId: req.GatewayAccountID}
	}
	if req.PoNumber != "" {
		params.Invoice = &hostedPageModel.CheckoutOneTimeForItemsInvoiceParams{PoNumber: req.PoNumber}
	}

	res, err := hostedpage.CheckoutOneTimeForItems(params).RequestWithEnv(env)
	if err != nil {
		return nil, wrapAPIError(err, "Failed to create Chargebee hosted checkout page")
	}
	if res.HostedPage == nil {
		return nil, missingPayload("hosted page")
	}
	return res.HostedPage, nil
}
