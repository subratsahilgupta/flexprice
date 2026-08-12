package checks

import (
	"context"

	"github.com/flexprice/flexprice/internal/e2eprobe"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/go-sdk/v2/models/types"
)

// PersistentBillingInvariantsProbe queries the latest cycle invoice for
// persistent cust #0 (tax-attached) and cust #1 (coupon-attached), asserting
// that each invoice contains the seeded tax rate / coupon association.
//
// This complements cycle-invoice-probe (which checks freshness) with a
// content check — catching divergence where preview flows correctly attach
// the tax / coupon but the real cycle invoicing code path silently drops it.
//
// Soft-skips when the seed hasn't run, when persistent customers haven't
// been provisioned, or when no invoice exists yet for the customer.
type PersistentBillingInvariantsProbe struct {
	client e2eprobe.Client
	reg    e2eprobe.Registry
	runID  string
	lg     *logger.Logger
}

func NewPersistentBillingInvariantsProbe(c e2eprobe.Client, r e2eprobe.Registry, runID string, lg *logger.Logger) *PersistentBillingInvariantsProbe {
	return &PersistentBillingInvariantsProbe{client: c, reg: r, runID: runID, lg: lg}
}

func (p *PersistentBillingInvariantsProbe) Name() string        { return "persistent-billing-invariants-probe" }
func (p *PersistentBillingInvariantsProbe) Kind() e2eprobe.Kind { return e2eprobe.KindProbe }

func (p *PersistentBillingInvariantsProbe) Run(ctx context.Context) error {
	seeds := p.reg.Seeds()
	if len(seeds.PersistentCustomerIDs) < 2 {
		return nil
	}

	// Persistent cust #0 → tax invariant.
	if seeds.SharedTaxRateID != "" {
		cust0 := seeds.PersistentCustomerIDs[0]
		inv, err := p.latestInvoice(ctx, cust0)
		if err != nil {
			return e2eprobe.Errorf(map[string]string{"step": "query_invoice_cust0", "external_customer_id": cust0}, "query invoice: %w", err)
		}
		if inv != nil {
			if !invoiceHasTaxRate(inv, seeds.SharedTaxRateID) {
				return e2eprobe.Errorf(map[string]string{"step": "assert_tax_present_cust0", "external_customer_id": cust0, "tax_rate_id": seeds.SharedTaxRateID}, "latest invoice for cust #0 does not include tax rate %s", seeds.SharedTaxRateID)
			}
		}
	}

	// Persistent cust #1 → coupon invariant.
	if seeds.SharedCouponID != "" {
		cust1 := seeds.PersistentCustomerIDs[1]
		inv, err := p.latestInvoice(ctx, cust1)
		if err != nil {
			return e2eprobe.Errorf(map[string]string{"step": "query_invoice_cust1", "external_customer_id": cust1}, "query invoice: %w", err)
		}
		if inv != nil {
			if !invoiceHasCoupon(inv, seeds.SharedCouponID) {
				return e2eprobe.Errorf(map[string]string{"step": "assert_coupon_present_cust1", "external_customer_id": cust1, "coupon_id": seeds.SharedCouponID}, "latest invoice for cust #1 does not include coupon %s", seeds.SharedCouponID)
			}
		}
	}
	return nil
}

func (p *PersistentBillingInvariantsProbe) latestInvoice(ctx context.Context, extID string) (*types.InvoiceResponse, error) {
	limit := int64(1)
	resp, err := p.client.Invoices().Query(ctx, types.InvoiceFilter{
		ExternalCustomerID: &extID,
		Limit:              &limit,
	})
	if err != nil {
		return nil, err
	}
	if resp.ListInvoicesResponse == nil || len(resp.ListInvoicesResponse.Items) == 0 {
		return nil, nil // no invoice yet — soft skip at caller
	}
	inv := resp.ListInvoicesResponse.Items[0]
	return &inv, nil
}

func invoiceHasTaxRate(inv *types.InvoiceResponse, taxRateID string) bool {
	for _, tx := range inv.Taxes {
		if tx.TaxRateID != nil && *tx.TaxRateID == taxRateID {
			return true
		}
	}
	return false
}

func invoiceHasCoupon(inv *types.InvoiceResponse, couponID string) bool {
	for _, ca := range inv.CouponApplications {
		if ca.CouponID != nil && *ca.CouponID == couponID {
			return true
		}
	}
	return false
}
