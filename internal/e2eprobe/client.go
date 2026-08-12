package e2eprobe

import (
	"context"

	flexprice "github.com/flexprice/go-sdk/v2"
	"github.com/flexprice/go-sdk/v2/models/dtos"
	"github.com/flexprice/go-sdk/v2/models/types"
)

type Client interface {
	Customers() CustomerOps
	Plans() PlanOps
	Prices() PriceOps
	Features() FeatureOps
	Subscriptions() SubscriptionOps
	Wallets() WalletOps
	Events() EventOps
	Invoices() InvoiceOps
	NewAsyncEventClient() AsyncEventClient
	Entitlements() EntitlementOps
	Coupons() CouponOps
	CouponAssociations() CouponAssociationOps
	TaxRates() TaxRateOps
	TaxAssociations() TaxAssociationOps
}

type CustomerOps interface {
	Create(ctx context.Context, req types.CreateCustomerRequest) (*dtos.CreateCustomerResponse, error)
	GetByExternalID(ctx context.Context, externalID string) (*dtos.GetCustomerByExternalIDResponse, error)
	Get(ctx context.Context, id string) (*dtos.GetCustomerResponse, error)
	GetEntitlements(ctx context.Context, id string) (*dtos.GetCustomerEntitlementsResponse, error)
	GetUsageSummary(ctx context.Context, req dtos.GetCustomerUsageSummaryRequest) (*dtos.GetCustomerUsageSummaryResponse, error)
	Update(ctx context.Context, body types.UpdateCustomerRequest, id, externalID *string) (*dtos.UpdateCustomerResponse, error)
	Delete(ctx context.Context, id string) (*dtos.DeleteCustomerResponse, error)
	Query(ctx context.Context, filter types.CustomerFilter) (*dtos.QueryCustomerResponse, error)
}

type PlanOps interface {
	Create(ctx context.Context, req types.CreatePlanRequest) (*dtos.CreatePlanResponse, error)
	Query(ctx context.Context, filter types.PlanFilter) (*dtos.QueryPlanResponse, error)
	Get(ctx context.Context, id string) (*dtos.GetPlanResponse, error)
}

type PriceOps interface {
	Create(ctx context.Context, req types.CreatePriceRequest) (*dtos.CreatePriceResponse, error)
	Query(ctx context.Context, filter types.PriceFilter) (*dtos.QueryPriceResponse, error)
}

type FeatureOps interface {
	Create(ctx context.Context, req types.CreateFeatureRequest) (*dtos.CreateFeatureResponse, error)
	Query(ctx context.Context, filter types.FeatureFilter) (*dtos.QueryFeatureResponse, error)
}

type SubscriptionOps interface {
	Create(ctx context.Context, req types.CreateSubscriptionRequest) (*dtos.CreateSubscriptionResponse, error)
	Get(ctx context.Context, id string) (*dtos.GetSubscriptionResponse, error)
	Cancel(ctx context.Context, id string, body types.CancelSubscriptionRequest) (*dtos.CancelSubscriptionResponse, error)
	Query(ctx context.Context, filter types.SubscriptionFilter) (*dtos.QuerySubscriptionResponse, error)
	ActivateSubscription(ctx context.Context, id string, body types.ActivateDraftSubscriptionRequest) (*dtos.ActivateSubscriptionResponse, error)
	GetEntitlements(ctx context.Context, id string, featureIDs []string) (*dtos.GetSubscriptionEntitlementsResponse, error)
	GetUsage(ctx context.Context, req types.GetUsageBySubscriptionRequest) (*dtos.GetSubscriptionUsageResponse, error)
	CreateLineItem(ctx context.Context, id string, body types.CreateSubscriptionLineItemRequest) (*dtos.CreateSubscriptionLineItemResponse, error)
	UpdateLineItem(ctx context.Context, id string, body types.UpdateSubscriptionLineItemRequest) (*dtos.UpdateSubscriptionLineItemResponse, error)
}

type WalletOps interface {
	Create(ctx context.Context, req types.CreateWalletRequest) (*dtos.CreateWalletResponse, error)
	Query(ctx context.Context, filter types.WalletFilter) (*dtos.QueryWalletResponse, error)
	GetWalletsByCustomerID(ctx context.Context, customerID string) (*dtos.GetWalletsByCustomerIDResponse, error)
	GetBalance(ctx context.Context, id string) (*dtos.GetWalletBalanceResponse, error)
	TopUp(ctx context.Context, id string, body types.TopUpWalletRequest) (*dtos.TopUpWalletResponse, error)
}

type EventOps interface {
	Ingest(ctx context.Context, req types.IngestEventRequest) (*dtos.IngestEventResponse, error)
	GetUsageAnalytics(ctx context.Context, req types.GetUsageAnalyticsRequest) (*dtos.GetUsageAnalyticsResponse, error)
	// ListRaw queries the raw events table. The probe uses this to verify
	// synchronously-ingested events actually landed before polling the
	// aggregation pipeline — separating "ingest dropped" from "aggregation
	// dropped" in failure attribution.
	ListRaw(ctx context.Context, req types.GetEventsRequest) (*dtos.ListRawEventsResponse, error)
}

type InvoiceOps interface {
	Query(ctx context.Context, filter types.InvoiceFilter) (*dtos.QueryInvoiceResponse, error)
	Get(ctx context.Context, id string) (*dtos.GetInvoiceResponse, error)
	GetPreview(ctx context.Context, req types.GetPreviewInvoiceRequest) (*dtos.GetInvoicePreviewResponse, error)
}

type EntitlementOps interface {
	Create(ctx context.Context, req types.CreateEntitlementRequest) (*dtos.CreateEntitlementResponse, error)
	Query(ctx context.Context, req types.EntitlementFilter) (*dtos.QueryEntitlementResponse, error)
	Delete(ctx context.Context, id string) (*dtos.DeleteEntitlementResponse, error)
}

type CouponOps interface {
	Create(ctx context.Context, req types.CreateCouponRequest) (*dtos.CreateCouponResponse, error)
	Query(ctx context.Context, req types.CouponFilter) (*dtos.QueryCouponResponse, error)
	GetByCode(ctx context.Context, code string) (*dtos.GetCouponByCodeResponse, error)
	Delete(ctx context.Context, id string) (*dtos.DeleteCouponResponse, error)
}

type CouponAssociationOps interface {
	List(ctx context.Context, req dtos.ListCouponAssociationsRequest) (*dtos.ListCouponAssociationsResponse, error)
}

type TaxRateOps interface {
	Create(ctx context.Context, req types.CreateTaxRateRequest) (*dtos.CreateTaxRateResponse, error)
	Get(ctx context.Context, id string) (*dtos.GetTaxRateResponse, error)
	List(ctx context.Context, req dtos.GetTaxRatesRequest) (*dtos.GetTaxRatesResponse, error)
	Delete(ctx context.Context, id string) (*dtos.DeleteTaxRateResponse, error)
}

type TaxAssociationOps interface {
	Create(ctx context.Context, req types.CreateTaxAssociationRequest) (*dtos.CreateTaxAssociationResponse, error)
	List(ctx context.Context, entityType, entityID, externalCustomerID, taxRateID *string) (*dtos.ListTaxAssociationsResponse, error)
	Delete(ctx context.Context, id string) (*dtos.DeleteTaxAssociationResponse, error)
}

type AsyncEventClient interface {
	Enqueue(eventName, externalCustomerID string, properties map[string]any) error
	EnqueueWithOptions(opts flexprice.EventOptions) error
	Flush() error
	Close() error
}

func NewSDKClient(apiHost, apiKey string) Client {
	sdk := flexprice.New(
		flexprice.WithServerURL(apiHost),
		flexprice.WithSecurity(apiKey),
	)
	return &sdkClient{sdk: sdk}
}

type sdkClient struct {
	sdk *flexprice.Flexprice
}

func (c *sdkClient) Customers() CustomerOps         { return customerOps{c.sdk.Customers} }
func (c *sdkClient) Plans() PlanOps                 { return planOps{c.sdk.Plans} }
func (c *sdkClient) Prices() PriceOps               { return priceOps{c.sdk.Prices} }
func (c *sdkClient) Features() FeatureOps           { return featureOps{c.sdk.Features} }
func (c *sdkClient) Subscriptions() SubscriptionOps { return subscriptionOps{c.sdk.Subscriptions} }
func (c *sdkClient) Wallets() WalletOps             { return walletOps{c.sdk.Wallets} }
func (c *sdkClient) Events() EventOps               { return eventOps{c.sdk.Events} }
func (c *sdkClient) Invoices() InvoiceOps           { return invoiceOps{c.sdk.Invoices} }
func (c *sdkClient) NewAsyncEventClient() AsyncEventClient {
	return c.sdk.NewAsyncClient()
}
func (c *sdkClient) Entitlements() EntitlementOps             { return entitlementOps{c.sdk.Entitlements} }
func (c *sdkClient) Coupons() CouponOps                       { return couponOps{c.sdk.Coupons} }
func (c *sdkClient) CouponAssociations() CouponAssociationOps { return couponAssociationOps{c.sdk.CouponAssociations} }
func (c *sdkClient) TaxRates() TaxRateOps                     { return taxRateOps{c.sdk.TaxRates} }
func (c *sdkClient) TaxAssociations() TaxAssociationOps       { return taxAssociationOps{c.sdk.TaxAssociations} }

// --- adapters ---

type customerOps struct{ s *flexprice.Customers }

func (o customerOps) Create(ctx context.Context, req types.CreateCustomerRequest) (*dtos.CreateCustomerResponse, error) {
	return o.s.CreateCustomer(ctx, req)
}
func (o customerOps) GetByExternalID(ctx context.Context, externalID string) (*dtos.GetCustomerByExternalIDResponse, error) {
	return o.s.GetCustomerByExternalID(ctx, externalID)
}
func (o customerOps) Get(ctx context.Context, id string) (*dtos.GetCustomerResponse, error) {
	return o.s.GetCustomer(ctx, id)
}
func (o customerOps) GetEntitlements(ctx context.Context, id string) (*dtos.GetCustomerEntitlementsResponse, error) {
	return o.s.GetCustomerEntitlements(ctx, id)
}
func (o customerOps) GetUsageSummary(ctx context.Context, req dtos.GetCustomerUsageSummaryRequest) (*dtos.GetCustomerUsageSummaryResponse, error) {
	return o.s.GetCustomerUsageSummary(ctx, req)
}
func (o customerOps) Update(ctx context.Context, body types.UpdateCustomerRequest, id, externalID *string) (*dtos.UpdateCustomerResponse, error) {
	return o.s.UpdateCustomer(ctx, body, id, externalID)
}
func (o customerOps) Delete(ctx context.Context, id string) (*dtos.DeleteCustomerResponse, error) {
	return o.s.DeleteCustomer(ctx, id)
}
func (o customerOps) Query(ctx context.Context, filter types.CustomerFilter) (*dtos.QueryCustomerResponse, error) {
	return o.s.QueryCustomer(ctx, filter)
}

type planOps struct{ s *flexprice.Plans }

func (o planOps) Create(ctx context.Context, req types.CreatePlanRequest) (*dtos.CreatePlanResponse, error) {
	return o.s.CreatePlan(ctx, req)
}
func (o planOps) Query(ctx context.Context, f types.PlanFilter) (*dtos.QueryPlanResponse, error) {
	return o.s.QueryPlan(ctx, f)
}
func (o planOps) Get(ctx context.Context, id string) (*dtos.GetPlanResponse, error) {
	return o.s.GetPlan(ctx, id)
}

type priceOps struct{ s *flexprice.Prices }

func (o priceOps) Create(ctx context.Context, req types.CreatePriceRequest) (*dtos.CreatePriceResponse, error) {
	return o.s.CreatePrice(ctx, req)
}
func (o priceOps) Query(ctx context.Context, f types.PriceFilter) (*dtos.QueryPriceResponse, error) {
	return o.s.QueryPrice(ctx, f)
}

type featureOps struct{ s *flexprice.Features }

func (o featureOps) Create(ctx context.Context, req types.CreateFeatureRequest) (*dtos.CreateFeatureResponse, error) {
	return o.s.CreateFeature(ctx, req)
}
func (o featureOps) Query(ctx context.Context, f types.FeatureFilter) (*dtos.QueryFeatureResponse, error) {
	return o.s.QueryFeature(ctx, f)
}

type subscriptionOps struct{ s *flexprice.Subscriptions }

func (o subscriptionOps) Create(ctx context.Context, req types.CreateSubscriptionRequest) (*dtos.CreateSubscriptionResponse, error) {
	return o.s.CreateSubscription(ctx, req)
}
func (o subscriptionOps) Get(ctx context.Context, id string) (*dtos.GetSubscriptionResponse, error) {
	return o.s.GetSubscription(ctx, id)
}
func (o subscriptionOps) Cancel(ctx context.Context, id string, body types.CancelSubscriptionRequest) (*dtos.CancelSubscriptionResponse, error) {
	return o.s.CancelSubscription(ctx, id, body)
}
func (o subscriptionOps) Query(ctx context.Context, f types.SubscriptionFilter) (*dtos.QuerySubscriptionResponse, error) {
	return o.s.QuerySubscription(ctx, f)
}
func (o subscriptionOps) ActivateSubscription(ctx context.Context, id string, body types.ActivateDraftSubscriptionRequest) (*dtos.ActivateSubscriptionResponse, error) {
	return o.s.ActivateSubscription(ctx, id, body)
}
func (o subscriptionOps) GetEntitlements(ctx context.Context, id string, featureIDs []string) (*dtos.GetSubscriptionEntitlementsResponse, error) {
	return o.s.GetSubscriptionEntitlements(ctx, id, featureIDs)
}
func (o subscriptionOps) GetUsage(ctx context.Context, req types.GetUsageBySubscriptionRequest) (*dtos.GetSubscriptionUsageResponse, error) {
	return o.s.GetSubscriptionUsage(ctx, req)
}
func (o subscriptionOps) CreateLineItem(ctx context.Context, id string, body types.CreateSubscriptionLineItemRequest) (*dtos.CreateSubscriptionLineItemResponse, error) {
	return o.s.CreateSubscriptionLineItem(ctx, id, body)
}
func (o subscriptionOps) UpdateLineItem(ctx context.Context, id string, body types.UpdateSubscriptionLineItemRequest) (*dtos.UpdateSubscriptionLineItemResponse, error) {
	return o.s.UpdateSubscriptionLineItem(ctx, id, body)
}

type walletOps struct{ s *flexprice.Wallets }

func (o walletOps) Create(ctx context.Context, req types.CreateWalletRequest) (*dtos.CreateWalletResponse, error) {
	return o.s.CreateWallet(ctx, req)
}
func (o walletOps) Query(ctx context.Context, f types.WalletFilter) (*dtos.QueryWalletResponse, error) {
	return o.s.QueryWallet(ctx, f)
}
func (o walletOps) GetWalletsByCustomerID(ctx context.Context, customerID string) (*dtos.GetWalletsByCustomerIDResponse, error) {
	return o.s.GetWalletsByCustomerID(ctx, customerID)
}

// GetBalance passes nil for the optional expand parameter (not exposed in the interface).
func (o walletOps) GetBalance(ctx context.Context, id string) (*dtos.GetWalletBalanceResponse, error) {
	return o.s.GetWalletBalance(ctx, id, nil)
}
func (o walletOps) TopUp(ctx context.Context, id string, body types.TopUpWalletRequest) (*dtos.TopUpWalletResponse, error) {
	return o.s.TopUpWallet(ctx, id, body)
}

type eventOps struct{ s *flexprice.Events }

func (o eventOps) Ingest(ctx context.Context, req types.IngestEventRequest) (*dtos.IngestEventResponse, error) {
	return o.s.IngestEvent(ctx, req)
}
func (o eventOps) GetUsageAnalytics(ctx context.Context, req types.GetUsageAnalyticsRequest) (*dtos.GetUsageAnalyticsResponse, error) {
	return o.s.GetUsageAnalytics(ctx, req)
}
func (o eventOps) ListRaw(ctx context.Context, req types.GetEventsRequest) (*dtos.ListRawEventsResponse, error) {
	return o.s.ListRawEvents(ctx, req)
}

type invoiceOps struct{ s *flexprice.Invoices }

func (o invoiceOps) Query(ctx context.Context, f types.InvoiceFilter) (*dtos.QueryInvoiceResponse, error) {
	return o.s.QueryInvoice(ctx, f)
}

// Get passes nil for optional expandBySource and groupBy parameters (not exposed in the interface).
func (o invoiceOps) Get(ctx context.Context, id string) (*dtos.GetInvoiceResponse, error) {
	return o.s.GetInvoice(ctx, id, nil, nil)
}
func (o invoiceOps) GetPreview(ctx context.Context, req types.GetPreviewInvoiceRequest) (*dtos.GetInvoicePreviewResponse, error) {
	return o.s.GetInvoicePreview(ctx, req)
}

type entitlementOps struct{ s *flexprice.Entitlements }

func (o entitlementOps) Create(ctx context.Context, req types.CreateEntitlementRequest) (*dtos.CreateEntitlementResponse, error) {
	return o.s.CreateEntitlement(ctx, req)
}
func (o entitlementOps) Query(ctx context.Context, f types.EntitlementFilter) (*dtos.QueryEntitlementResponse, error) {
	return o.s.QueryEntitlement(ctx, f)
}
func (o entitlementOps) Delete(ctx context.Context, id string) (*dtos.DeleteEntitlementResponse, error) {
	return o.s.DeleteEntitlement(ctx, id)
}

type couponOps struct{ s *flexprice.Coupons }

func (o couponOps) Create(ctx context.Context, req types.CreateCouponRequest) (*dtos.CreateCouponResponse, error) {
	return o.s.CreateCoupon(ctx, req)
}
func (o couponOps) Query(ctx context.Context, f types.CouponFilter) (*dtos.QueryCouponResponse, error) {
	return o.s.QueryCoupon(ctx, f)
}
func (o couponOps) GetByCode(ctx context.Context, code string) (*dtos.GetCouponByCodeResponse, error) {
	return o.s.GetCouponByCode(ctx, code)
}
func (o couponOps) Delete(ctx context.Context, id string) (*dtos.DeleteCouponResponse, error) {
	return o.s.DeleteCoupon(ctx, id)
}

type couponAssociationOps struct{ s *flexprice.CouponAssociations }

func (o couponAssociationOps) List(ctx context.Context, req dtos.ListCouponAssociationsRequest) (*dtos.ListCouponAssociationsResponse, error) {
	return o.s.ListCouponAssociations(ctx, req)
}

type taxRateOps struct{ s *flexprice.TaxRates }

func (o taxRateOps) Create(ctx context.Context, req types.CreateTaxRateRequest) (*dtos.CreateTaxRateResponse, error) {
	return o.s.CreateTaxRate(ctx, req)
}
func (o taxRateOps) Get(ctx context.Context, id string) (*dtos.GetTaxRateResponse, error) {
	return o.s.GetTaxRate(ctx, id)
}
func (o taxRateOps) List(ctx context.Context, req dtos.GetTaxRatesRequest) (*dtos.GetTaxRatesResponse, error) {
	return o.s.GetTaxRates(ctx, req)
}
func (o taxRateOps) Delete(ctx context.Context, id string) (*dtos.DeleteTaxRateResponse, error) {
	return o.s.DeleteTaxRate(ctx, id)
}

type taxAssociationOps struct{ s *flexprice.TaxAssociations }

func (o taxAssociationOps) Create(ctx context.Context, req types.CreateTaxAssociationRequest) (*dtos.CreateTaxAssociationResponse, error) {
	return o.s.CreateTaxAssociation(ctx, req)
}
func (o taxAssociationOps) List(ctx context.Context, entityType, entityID, externalCustomerID, taxRateID *string) (*dtos.ListTaxAssociationsResponse, error) {
	return o.s.ListTaxAssociations(ctx, entityType, entityID, externalCustomerID, taxRateID)
}
func (o taxAssociationOps) Delete(ctx context.Context, id string) (*dtos.DeleteTaxAssociationResponse, error) {
	return o.s.DeleteTaxAssociation(ctx, id)
}
