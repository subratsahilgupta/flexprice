package razorpay

import (
	"context"
	"strconv"
	"strings"
	"time"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// toPaise converts a major-unit decimal (rupees) to paise for Razorpay API calls.
func toPaise(major decimal.Decimal) int64 {
	return major.Mul(decimal.NewFromInt(100)).IntPart()
}

// fromPaise converts a paise float64 (as returned by Razorpay) to a major-unit decimal.
func fromPaise(paise float64) decimal.Decimal {
	return decimal.NewFromFloat(paise).Div(decimal.NewFromInt(100))
}

func NormalizeRazorpayToken(raw map[string]interface{}) (*interfaces.ProviderPaymentMethod, error) {
	pm := &interfaces.ProviderPaymentMethod{
		GatewayMethodID:  lo.ValueOr(raw, "id", "").(string),
		ProviderMetadata: map[string]string{},
		Active:           true,
	}

	method, _ := raw["method"].(string)
	switch method {
	case "upi":
		pm.Method = types.PaymentMethodTypeUPI
		pm.UPI = razorpayUPIDetails(raw)
	case "card":
		pm.Method = types.PaymentMethodTypeCard
		pm.Card = razorpayCardDetails(raw)
	default:
		return nil, nil
	}

	if status, ok := raw["status"].(string); ok && status != "" {
		pm.Active = status == "active"
	}
	if createdAtUnix, ok := raw["created_at"].(float64); ok {
		pm.CreatedAt = time.Unix(int64(createdAtUnix), 0).UTC()
	}

	if recurring, _ := raw["recurring"].(bool); recurring {
		pm.Recurring = razorpayRecurringDetails(raw)
	}

	return pm, nil
}

func razorpayRecurringDetails(raw map[string]interface{}) *interfaces.ProviderRecurringPaymentDetails {
	details, _ := raw["recurring_details"].(map[string]interface{})
	status, _ := details["status"].(string)
	out := &interfaces.ProviderRecurringPaymentDetails{Status: toRecurringPaymentStatus(status)}

	// Razorpay declines charges on a card whose issuer no longer supports recurring,
	// even while the mandate still reads confirmed.
	if cardRecurringUnsupported(raw) && out.Status == types.RecurringPaymentStatusActive {
		out.Status = types.RecurringPaymentStatusRejected
	}

	if paise, ok := raw["max_amount"].(float64); ok && paise > 0 {
		major := fromPaise(paise)
		out.MaxAmount = &major
	}

	if expiredAtUnix, ok := raw["expired_at"].(float64); ok && expiredAtUnix > 0 {
		t := time.Unix(int64(expiredAtUnix), 0).UTC()
		out.AutoChargeableTill = &t

		if time.Now().UTC().After(t) && !lo.Contains([]types.RecurringPaymentStatus{
			types.RecurringPaymentStatusRejected, types.RecurringPaymentStatusCancelled,
		}, out.Status) {
			out.Status = types.RecurringPaymentStatusExpired
		}
	}

	return out
}

// cardRecurringUnsupported reports whether the card explicitly flags recurring as unsupported.
func cardRecurringUnsupported(raw map[string]interface{}) bool {
	card, _ := raw["card"].(map[string]interface{})
	flows, _ := card["flows"].(map[string]interface{})
	supported, ok := flows["recurring"].(bool)
	return ok && !supported
}

func razorpayCardDetails(raw map[string]interface{}) *interfaces.ProviderCardDetails {
	card, ok := raw["card"].(map[string]interface{})
	if !ok {
		return nil
	}

	last4, _ := card["last4"].(string)
	network, _ := card["network"].(string)
	return &interfaces.ProviderCardDetails{
		Brand:    network,
		Last4:    last4,
		ExpMonth: intFromAny(card["expiry_month"]),
		ExpYear:  intFromAny(card["expiry_year"]),
	}
}

func razorpayUPIDetails(raw map[string]interface{}) *interfaces.ProviderUPIDetails {
	vpa, ok := raw["vpa"].(map[string]interface{})
	if !ok {
		return nil
	}

	username, _ := vpa["username"].(string)
	handle, _ := vpa["handle"].(string)
	if username == "" || handle == "" {
		return nil
	}
	return &interfaces.ProviderUPIDetails{VPA: username + "@" + handle}
}

func intFromAny(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	}
	return 0
}

func toRecurringPaymentStatus(status string) types.RecurringPaymentStatus {
	switch status {
	case "initiated":
		return types.RecurringPaymentStatusPending
	case "confirmed":
		return types.RecurringPaymentStatusActive
	case "paused":
		return types.RecurringPaymentStatusPaused
	case "rejected":
		return types.RecurringPaymentStatusRejected
	case "cancelled":
		return types.RecurringPaymentStatusCancelled
	default:
		return types.RecurringPaymentStatusUnknown
	}
}

// SelectUsableToken applies the deterministic selection algorithm: filter for
// active, confirmed, non-expired, matching-method, under-ceiling; pick the newest.
// Exported because both the checkout-time dedup check and the auto-charge path
// need the exact same logic.
func SelectUsableToken(
	methods []*interfaces.ProviderPaymentMethod,
	preferredMethod types.PaymentMethodType,
	invoiceTotal decimal.Decimal,
) (*interfaces.ProviderPaymentMethod, bool) {
	now := time.Now().UTC()
	usable := lo.Filter(methods, func(pm *interfaces.ProviderPaymentMethod, _ int) bool {
		expiresAt, maxAmount := pm.RecurringAutoChargeableTill(), pm.RecurringMaxAmount()

		return pm.Active &&
			pm.RecurringStatus() == types.RecurringPaymentStatusActive &&
			pm.Method == preferredMethod &&
			(expiresAt == nil || !now.After(*expiresAt)) &&
			(maxAmount == nil || !maxAmount.LessThan(invoiceTotal))
	})
	if len(usable) == 0 {
		return nil, false
	}

	return lo.MaxBy(usable, func(a, b *interfaces.ProviderPaymentMethod) bool {
		return a.CreatedAt.After(b.CreatedAt)
	}), true
}

// razorpaySubscriptionMethod maps a FlexPrice PaymentMethodType to the Razorpay
// subscription_registration "method" value. Empty input defaults to "upi".
func razorpaySubscriptionMethod(pm types.PaymentMethodType) (string, error) {
	switch pm {
	case "", types.PaymentMethodTypeUPI:
		return "upi", nil
	case types.PaymentMethodTypeCard:
		return "card", nil
	default:
		return "", ierr.NewErrorf("razorpay authorization link registration does not support method %q", pm).
			WithHint("Only UPI and Card are supported for Razorpay mandate registration").
			Mark(ierr.ErrNotImplemented)
	}
}

// CreateAuthorizationLink registers a UPI Autopay or card recurring-payment
// mandate combined with the first invoice payment.
func (a *CheckoutAdapter) CreateAuthorizationLink(
	ctx context.Context,
	req interfaces.AuthorizationLinkRequest,
) (*interfaces.CheckoutProviderResponse, error) {
	method, err := razorpaySubscriptionMethod(req.PreferredMethod)
	if err != nil {
		return nil, err
	}

	c, err := a.Svc.customerSvc.EnsureCustomerSyncedToRazorpay(ctx, req.CustomerID, a.CustomerSvc)
	if err != nil {
		return nil, err
	}

	customerInfo := map[string]interface{}{
		"name": c.Name,
	}
	if c.Email != "" {
		customerInfo["email"] = c.Email
	}
	// Razorpay requires a contact number for recurring/subscription-registration links.
	if c.Contact != nil && *c.Contact != "" {
		customerInfo["contact"] = *c.Contact
	}

	data := map[string]interface{}{
		"customer":     customerInfo,
		"type":         "link",
		"amount":       toPaise(req.Amount),
		"currency":     strings.ToUpper(req.Currency),
		"description":  "Subscription authorization",
		"receipt":      req.InvoiceID,
		"email_notify": true,
		"sms_notify":   true,
		"notes": map[string]interface{}{
			"flexprice_customer_id": req.CustomerID,
			"flexprice_payment_id":  req.PaymentID,
		},
	}
	if req.ExpiresAt != nil {
		data["expire_by"] = req.ExpiresAt.Unix()
	}

	subReg := map[string]interface{}{"method": method}
	if req.MaxAmount != nil {
		subReg["max_amount"] = toPaise(*req.MaxAmount)
	}
	data["subscription_registration"] = subReg

	result, err := a.Svc.client.CreateAuthorizationLink(ctx, data)
	if err != nil {
		return nil, err
	}

	shortURL, _ := result["short_url"].(string)
	id, _ := result["id"].(string)
	if shortURL == "" || id == "" {
		return nil, ierr.NewError("razorpay authorization link response missing short_url or id").
			WithHint("Razorpay returned an unexpected response for the mandate registration link").
			WithReportableDetails(map[string]interface{}{
				"has_short_url": shortURL != "",
				"has_id":        id != "",
			}).
			Mark(ierr.ErrInternal)
	}

	return &interfaces.CheckoutProviderResponse{
		ProviderSessionID: id,
		NextAction:        types.PaymentAction{Type: types.PaymentActionTypePaymentLink, URL: shortURL},
	}, nil
}

// TryAutoChargingSavedMethod implements interfaces.CheckoutProvider by delegating
// to PaymentService.ChargeSavedToken and mapping into CheckoutProviderResponse.
func (a *CheckoutAdapter) TryAutoChargingSavedMethod(
	ctx context.Context,
	req interfaces.AuthorizationLinkRequest,
) (*interfaces.CheckoutProviderResponse, bool, error) {
	if a == nil || a.Svc == nil || a.CustomerSvc == nil {
		return nil, false, nil
	}

	custResp, err := a.CustomerSvc.GetCustomer(ctx, req.CustomerID)
	if err != nil || custResp == nil || custResp.Customer == nil {
		a.Svc.logger.Info(ctx, "checkout auto-charge: failed to load customer, falling back to auth link",
			"customer_id", req.CustomerID, "error", err)
		return nil, false, nil
	}

	result, charged, err := a.Svc.ChargeSavedToken(ctx, ChargeSavedTokenRequest{
		Customer:           custResp.Customer,
		InvoiceID:          req.InvoiceID,
		Amount:             req.Amount,
		Currency:           req.Currency,
		FlexPricePaymentID: req.PaymentID,
		PreferredMethod:    req.PreferredMethod,
	})
	if err != nil {
		return nil, false, err
	}
	if !charged || result == nil {
		return nil, false, nil
	}

	sessionID := result.RazorpayOrderID
	if sessionID == "" {
		sessionID = result.RazorpayPaymentID
	}
	return &interfaces.CheckoutProviderResponse{
		ProviderSessionID:       sessionID,
		ProviderPaymentIntentID: result.RazorpayPaymentID,
		// No NextAction — off-session charge; completion via payment webhook.
	}, true, nil
}

// HasAutoChargeableMethod implements interfaces.CheckoutProvider by checking
// if the customer has any active, non-expired recurring mandate tokens on Razorpay
// that can cover the requested amount (or any active token if amount is nil).
func (a *CheckoutAdapter) HasAutoChargeableMethod(ctx context.Context, req interfaces.HasAutoChargeableMethodRequest) (bool, error) {
	if a == nil || a.Svc == nil || a.Svc.customerSvc == nil || req.CustomerID == "" {
		return false, nil
	}

	_, tokens, err := a.Svc.customerSvc.ListCustomerTokens(ctx, req.CustomerID)
	if err != nil {
		if ierr.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	amount := decimal.Zero
	if req.Amount != nil {
		amount = *req.Amount
	}
	_, ok := selectAutoChargeToken(tokens, "", amount)
	return ok, nil
}

type PaymentMethodAdapter struct {
	CustomerSvc RazorpayCustomerService
}

func (a *PaymentMethodAdapter) ListSavedMethods(ctx context.Context, customerID string) ([]interfaces.ProviderPaymentMethod, error) {
	_, tokens, err := a.CustomerSvc.ListCustomerTokens(ctx, customerID)
	if err != nil {
		if ierr.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return lo.FilterMap(tokens, func(t *interfaces.ProviderPaymentMethod, _ int) (interfaces.ProviderPaymentMethod, bool) {
		if t == nil {
			return interfaces.ProviderPaymentMethod{}, false
		}

		return *t, true
	}), nil
}

func (a *PaymentMethodAdapter) DeleteSavedMethod(ctx context.Context, customerID, methodID string) error {
	return errMandateManagedAtCheckout()
}

func (a *PaymentMethodAdapter) SetDefaultSavedMethod(ctx context.Context, customerID, methodID string) error {
	return errMandateManagedAtCheckout()
}

func (a *PaymentMethodAdapter) CreateSetupLink(ctx context.Context, req interfaces.SetupLinkRequest) (*interfaces.SetupLinkResponse, error) {
	return nil, errMandateManagedAtCheckout()
}

func errMandateManagedAtCheckout() error {
	return ierr.NewError("razorpay saved payment methods cannot be managed here").
		WithHint("Razorpay mandates are set up at checkout").
		Mark(ierr.ErrValidation)
}
