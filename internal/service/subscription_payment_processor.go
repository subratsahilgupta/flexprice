package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"

	"github.com/shopspring/decimal"
)

// SubscriptionPaymentProcessor handles payment processing for subscriptions
type SubscriptionPaymentProcessor interface {
	HandlePaymentBehavior(ctx context.Context, subscription *subscription.Subscription, invoice *dto.InvoiceResponse, behavior types.PaymentBehavior) error
}

type subscriptionPaymentProcessor struct {
	*ServiceParams
}

// PaymentResult represents the result of a payment attempt
type PaymentResult struct {
	Success                    bool                `json:"success"`
	AmountPaid                 decimal.Decimal     `json:"amount_paid"`
	RemainingAmount            decimal.Decimal     `json:"remaining_amount"`
	PaymentMethods             []PaymentMethodUsed `json:"payment_methods_used"`
	RequiresManualConfirmation bool                `json:"requires_manual_confirmation"`
	Error                      error               `json:"error,omitempty"`
}

// PaymentMethodUsed represents a payment method that was used
type PaymentMethodUsed struct {
	Type   types.PaymentMethodType `json:"type"`
	ID     string                  `json:"id"`
	Amount decimal.Decimal         `json:"amount"`
	Status types.PaymentStatus     `json:"status"`
}

// NewSubscriptionPaymentProcessor creates a new subscription payment processor
func NewSubscriptionPaymentProcessor(params *ServiceParams) SubscriptionPaymentProcessor {
	return &subscriptionPaymentProcessor{
		ServiceParams: params,
	}
}

// HandlePaymentBehavior handles the payment result based on payment behavior
func (s *subscriptionPaymentProcessor) HandlePaymentBehavior(
	ctx context.Context,
	sub *subscription.Subscription,
	inv *dto.InvoiceResponse,
	behavior types.PaymentBehavior,
) error {
	s.Logger.Infow("handling payment behavior",
		"subscription_id", sub.ID,
		"invoice_id", inv.ID,
		"amount_due", inv.AmountDue,
		"collection_method", sub.CollectionMethod,
		"payment_behavior", behavior,
	)

	// Handle different collection methods
	switch types.CollectionMethod(sub.CollectionMethod) {
	case types.CollectionMethodSendInvoice:
		return s.handleSendInvoiceMethod(ctx, sub, inv)
	case types.CollectionMethodChargeAutomatically:
		return s.handleChargeAutomaticallyMethod(ctx, sub, inv, behavior)
	default:
		return ierr.NewError("unsupported collection method").
			WithHint("Collection method not supported").
			WithReportableDetails(map[string]interface{}{
				"collection_method": sub.CollectionMethod,
			}).
			Mark(ierr.ErrInvalidOperation)
	}
}

// handleSendInvoiceMethod handles send_invoice collection method
func (s *subscriptionPaymentProcessor) handleSendInvoiceMethod(
	ctx context.Context,
	sub *subscription.Subscription,
	inv *dto.InvoiceResponse,
) error {
	// For send_invoice: always activate subscription and send webhook
	// Payment behavior is ignored
	sub.SubscriptionStatus = types.SubscriptionStatusActive

	s.Logger.Infow("send_invoice method - activating subscription immediately",
		"subscription_id", sub.ID,
		"invoice_id", inv.ID,
	)

	return s.SubRepo.Update(ctx, sub)
}

// handleChargeAutomaticallyMethod handles charge_automatically collection method
func (s *subscriptionPaymentProcessor) handleChargeAutomaticallyMethod(
	ctx context.Context,
	sub *subscription.Subscription,
	inv *dto.InvoiceResponse,
	behavior types.PaymentBehavior,
) error {
	switch behavior {
	case types.PaymentBehaviorDefaultIncomplete:
		// Special case: Don't attempt payment if amount > 0
		if inv.AmountDue.GreaterThan(decimal.Zero) {
			sub.SubscriptionStatus = types.SubscriptionStatusIncomplete
			s.Logger.Infow("default_incomplete behavior - setting to incomplete",
				"subscription_id", sub.ID,
				"amount_due", inv.AmountDue,
			)
			return s.SubRepo.Update(ctx, sub)
		}
		// If amount = 0, mark as active
		sub.SubscriptionStatus = types.SubscriptionStatusActive
		return s.SubRepo.Update(ctx, sub)

	case types.PaymentBehaviorAllowIncomplete:
		return s.attemptPaymentAllowIncomplete(ctx, sub, inv)

	case types.PaymentBehaviorErrorIfIncomplete:
		return s.attemptPaymentErrorIfIncomplete(ctx, sub, inv)

	case types.PaymentBehaviorDefaultActive:
		// Default active behavior - always create active subscription without payment attempt
		s.Logger.Infow("default_active behavior - setting to active without payment attempt",
			"subscription_id", sub.ID,
			"amount_due", inv.AmountDue,
		)
		sub.SubscriptionStatus = types.SubscriptionStatusActive
		return s.SubRepo.Update(ctx, sub)

	default:
		return ierr.NewError("unsupported payment behavior").
			WithHint("Payment behavior not supported for subscription creation").
			WithReportableDetails(map[string]interface{}{
				"payment_behavior": behavior,
			}).
			Mark(ierr.ErrInvalidOperation)
	}
}

// attemptPaymentAllowIncomplete attempts payment and allows incomplete status on failure
func (s *subscriptionPaymentProcessor) attemptPaymentAllowIncomplete(
	ctx context.Context,
	sub *subscription.Subscription,
	inv *dto.InvoiceResponse,
) error {
	result := s.processPayment(ctx, sub, inv)

	// Get the latest subscription status to check if it was already activated
	// by payment reconciliation (this can happen when payment succeeds and
	// triggers subscription activation through payment service)
	latestSub, err := s.SubRepo.Get(ctx, sub.ID)
	if err != nil {
		s.Logger.Errorw("failed to get latest subscription status",
			"error", err,
			"subscription_id", sub.ID,
		)
		// Continue with original logic if we can't get latest status
		latestSub = sub
	}

	// Determine target status based on payment result
	var targetStatus types.SubscriptionStatus
	if result.Success {
		targetStatus = types.SubscriptionStatusActive
	} else {
		targetStatus = types.SubscriptionStatusIncomplete
	}

	s.Logger.Infow("allow_incomplete payment result",
		"subscription_id", sub.ID,
		"success", result.Success,
		"amount_paid", result.AmountPaid,
		"current_status", latestSub.SubscriptionStatus,
		"target_status", targetStatus,
	)

	// Only update if the subscription status needs to change
	if latestSub.SubscriptionStatus != targetStatus {
		latestSub.SubscriptionStatus = targetStatus
		return s.SubRepo.Update(ctx, latestSub)
	}

	s.Logger.Infow("subscription status already matches target, skipping update",
		"subscription_id", sub.ID,
		"status", latestSub.SubscriptionStatus,
	)

	// Update the original subscription object for consistency
	sub.SubscriptionStatus = latestSub.SubscriptionStatus
	return nil
}

// attemptPaymentErrorIfIncomplete attempts payment and returns error on failure
func (s *subscriptionPaymentProcessor) attemptPaymentErrorIfIncomplete(
	ctx context.Context,
	sub *subscription.Subscription,
	inv *dto.InvoiceResponse,
) error {
	result := s.processPayment(ctx, sub, inv)

	if result.Success {
		sub.SubscriptionStatus = types.SubscriptionStatusActive
		return s.SubRepo.Update(ctx, sub)
	}

	// Payment failed - return error to prevent subscription creation
	return ierr.NewError("payment failed").
		WithHint("Subscription creation failed due to payment failure").
		WithReportableDetails(map[string]interface{}{
			"subscription_id": sub.ID,
			"invoice_id":      inv.ID,
			"amount_due":      inv.AmountDue,
			"amount_paid":     result.AmountPaid,
		}).
		Mark(ierr.ErrInvalidOperation)
}

// processPayment processes payment sequentially (credits first, then payment method)
func (s *subscriptionPaymentProcessor) processPayment(
	ctx context.Context,
	sub *subscription.Subscription,
	inv *dto.InvoiceResponse,
) *PaymentResult {
	// Use AmountRemaining instead of AmountDue to account for any existing payments
	remainingAmount := inv.AmountRemaining

	result := &PaymentResult{
		AmountPaid:      decimal.Zero,
		RemainingAmount: remainingAmount,
		PaymentMethods:  []PaymentMethodUsed{},
	}

	s.Logger.Infow("processing payment sequentially",
		"subscription_id", sub.ID,
		"amount_due", inv.AmountDue,
		"amount_remaining", remainingAmount,
	)

	if remainingAmount.IsZero() {
		result.Success = true
		return result
	}

	// Step 1: Check if credits are available
	availableCredits := s.checkAvailableCredits(ctx, sub, inv)
	if availableCredits.GreaterThan(decimal.Zero) {
		s.Logger.Infow("credits available, processing credits payment",
			"subscription_id", sub.ID,
			"available_credits", availableCredits,
			"amount_due", inv.AmountDue,
		)

		// Pay using credits
		creditsUsed := s.processCreditsPayment(ctx, sub, inv)
		if creditsUsed.GreaterThan(decimal.Zero) {
			result.AmountPaid = result.AmountPaid.Add(creditsUsed)
			result.RemainingAmount = result.RemainingAmount.Sub(creditsUsed)
			result.PaymentMethods = append(result.PaymentMethods, PaymentMethodUsed{
				Type:   "credits",
				Amount: creditsUsed,
			})

			s.Logger.Infow("credits payment completed",
				"subscription_id", sub.ID,
				"credits_used", creditsUsed,
				"remaining_amount", result.RemainingAmount,
			)
		}
	}

	// Step 2: If all amount is paid using credits, return success
	if result.RemainingAmount.IsZero() {
		result.Success = true
		s.Logger.Infow("payment completed entirely with credits",
			"subscription_id", sub.ID,
			"total_paid", result.AmountPaid,
		)
		return result
	}

	// Step 3: If amount is remaining, use charge card
	s.Logger.Infow("remaining amount after credits, attempting card charge",
		"subscription_id", sub.ID,
		"remaining_amount", result.RemainingAmount,
	)

	cardAmountPaid := s.processPaymentMethodCharge(ctx, sub, inv, result.RemainingAmount)
	if cardAmountPaid.GreaterThan(decimal.Zero) {
		result.AmountPaid = result.AmountPaid.Add(cardAmountPaid)
		result.RemainingAmount = result.RemainingAmount.Sub(cardAmountPaid)
		result.PaymentMethods = append(result.PaymentMethods, PaymentMethodUsed{
			Type:   "card",
			Amount: cardAmountPaid,
		})

		s.Logger.Infow("card payment completed",
			"subscription_id", sub.ID,
			"card_amount_paid", cardAmountPaid,
			"remaining_amount", result.RemainingAmount,
		)
	} else {
		// Card payment failed - keep credits applied, invoice remains incomplete
		s.Logger.Infow("card payment failed, keeping credits applied",
			"subscription_id", sub.ID,
			"credits_applied", result.AmountPaid,
			"remaining_amount", result.RemainingAmount,
		)
	}

	// Step 4: Mark success if fully paid
	result.Success = result.RemainingAmount.IsZero()

	s.Logger.Infow("payment processing completed",
		"subscription_id", sub.ID,
		"success", result.Success,
		"amount_paid", result.AmountPaid,
		"remaining_amount", result.RemainingAmount,
	)

	return result
}

// processCreditsPayment processes payment using customer's credits/wallets
func (s *subscriptionPaymentProcessor) processCreditsPayment(
	ctx context.Context,
	sub *subscription.Subscription,
	inv *dto.InvoiceResponse,
) decimal.Decimal {
	s.Logger.Infow("processing credits payment",
		"subscription_id", sub.ID,
		"invoice_id", inv.ID,
		"amount_due", inv.AmountDue,
	)

	// Convert DTO invoice to domain invoice for wallet payment service
	domainInvoice := &invoice.Invoice{
		ID:              inv.ID,
		CustomerID:      inv.CustomerID,
		Currency:        inv.Currency,
		AmountDue:       inv.AmountDue,
		AmountRemaining: inv.AmountDue, // Assume full amount is remaining
	}

	// Use wallet payment service to process payment
	walletPaymentService := NewWalletPaymentService(*s.ServiceParams)
	amountPaid, err := walletPaymentService.ProcessInvoicePaymentWithWallets(ctx, domainInvoice, WalletPaymentOptions{
		Strategy:        PromotionalFirstStrategy,
		MaxWalletsToUse: 5,
		AdditionalMetadata: types.Metadata{
			"subscription_id": sub.ID,
			"payment_source":  "subscription_auto_payment",
		},
	})

	if err != nil {
		s.Logger.Errorw("credits payment failed",
			"error", err,
			"subscription_id", sub.ID,
			"invoice_id", inv.ID,
		)
		return decimal.Zero
	}

	s.Logger.Infow("credits payment completed",
		"subscription_id", sub.ID,
		"invoice_id", inv.ID,
		"amount_paid", amountPaid,
	)

	return amountPaid
}

// processPaymentMethodCharge processes payment using payment method (card, etc.)
func (s *subscriptionPaymentProcessor) processPaymentMethodCharge(
	ctx context.Context,
	sub *subscription.Subscription,
	inv *dto.InvoiceResponse,
	amount decimal.Decimal,
) decimal.Decimal {
	s.Logger.Infow("processing payment method charge",
		"subscription_id", sub.ID,
		"invoice_id", inv.ID,
		"amount", amount,
	)

	// Check if tenant has Stripe connection
	if !s.hasStripeConnection(ctx) {
		s.Logger.Warnw("no Stripe connection available for payment method charge",
			"subscription_id", sub.ID,
		)
		return decimal.Zero
	}

	// Get payment method ID
	paymentMethodID := s.getPaymentMethodID(ctx, sub)
	if paymentMethodID == "" {
		s.Logger.Warnw("no payment method available for automatic charging",
			"subscription_id", sub.ID,
		)
		return decimal.Zero
	}

	// Create payment record for card payment
	paymentService := NewPaymentService(*s.ServiceParams)
	paymentReq := &dto.CreatePaymentRequest{
		DestinationType:   types.PaymentDestinationTypeInvoice,
		DestinationID:     inv.ID,
		PaymentMethodType: types.PaymentMethodTypeCard,
		PaymentMethodID:   paymentMethodID,
		Amount:            amount,
		Currency:          inv.Currency,
		ProcessPayment:    true,
		Metadata: types.Metadata{
			"customer_id":     sub.CustomerID,
			"subscription_id": sub.ID,
			"payment_source":  "subscription_auto_payment",
		},
	}

	paymentResp, err := paymentService.CreatePayment(ctx, paymentReq)
	if err != nil {
		s.Logger.Errorw("failed to create payment record for card charge",
			"error", err,
			"subscription_id", sub.ID,
			"customer_id", sub.CustomerID,
			"payment_method_id", paymentMethodID,
			"amount", amount,
		)
		return decimal.Zero
	}

	s.Logger.Infow("created payment record for card charge",
		"subscription_id", sub.ID,
		"payment_id", paymentResp.ID,
		"amount", amount,
	)

	// Check if payment was successful
	if paymentResp.PaymentStatus == types.PaymentStatusSucceeded {
		s.Logger.Infow("payment method charge successful",
			"subscription_id", sub.ID,
			"payment_id", paymentResp.ID,
			"amount", amount,
		)
		return amount
	}

	s.Logger.Warnw("payment method charge not successful",
		"subscription_id", sub.ID,
		"payment_id", paymentResp.ID,
		"status", paymentResp.PaymentStatus,
	)
	return decimal.Zero
}

// getPaymentMethodID gets the payment method ID for the subscription
func (s *subscriptionPaymentProcessor) getPaymentMethodID(ctx context.Context, sub *subscription.Subscription) string {
	// Use subscription's payment method if set
	if sub.PaymentMethodID != nil && *sub.PaymentMethodID != "" {
		s.Logger.Infow("using subscription payment method",
			"subscription_id", sub.ID,
			"payment_method_id", *sub.PaymentMethodID,
		)
		return *sub.PaymentMethodID
	}

	// Get customer's default payment method from Stripe
	stripeService := NewStripeService(*s.ServiceParams)
	defaultPaymentMethod, err := stripeService.GetDefaultPaymentMethod(ctx, sub.CustomerID)
	if err != nil {
		s.Logger.Warnw("failed to get default payment method",
			"error", err,
			"subscription_id", sub.ID,
			"customer_id", sub.CustomerID,
		)
		return ""
	}

	if defaultPaymentMethod == nil {
		s.Logger.Warnw("customer has no default payment method",
			"subscription_id", sub.ID,
			"customer_id", sub.CustomerID,
		)
		return ""
	}

	s.Logger.Infow("using customer default payment method",
		"subscription_id", sub.ID,
		"customer_id", sub.CustomerID,
		"payment_method_id", defaultPaymentMethod.ID,
	)

	return defaultPaymentMethod.ID
}

// hasStripeConnection checks if the tenant has a Stripe connection available
func (s *subscriptionPaymentProcessor) hasStripeConnection(ctx context.Context) bool {
	conn, err := s.ConnectionRepo.GetByProvider(ctx, types.SecretProviderStripe)
	if err != nil {
		s.Logger.Debugw("no Stripe connection found",
			"error", err,
		)
		return false
	}

	if conn == nil {
		s.Logger.Debugw("Stripe connection is nil")
		return false
	}

	s.Logger.Debugw("Stripe connection found",
		"connection_id", conn.ID,
		"provider", conn.ProviderType,
	)

	return true
}

// checkAvailableCredits checks how much credits are available without consuming them
func (s *subscriptionPaymentProcessor) checkAvailableCredits(
	ctx context.Context,
	sub *subscription.Subscription,
	inv *dto.InvoiceResponse,
) decimal.Decimal {
	s.Logger.Infow("checking available credits",
		"subscription_id", sub.ID,
		"invoice_id", inv.ID,
	)

	// Get customer ID from subscription
	customerID := sub.CustomerID
	currency := inv.Currency

	// Get wallets suitable for payment
	walletPaymentService := NewWalletPaymentService(*s.ServiceParams)
	wallets, err := walletPaymentService.GetWalletsForPayment(ctx, customerID, currency, WalletPaymentOptions{
		Strategy:        PromotionalFirstStrategy,
		MaxWalletsToUse: 5,
	})
	if err != nil {
		s.Logger.Errorw("failed to get wallets for payment",
			"error", err,
			"customer_id", customerID,
			"currency", currency,
		)
		return decimal.Zero
	}

	// Calculate total available credits
	totalAvailable := decimal.Zero
	for _, w := range wallets {
		totalAvailable = totalAvailable.Add(w.Balance)
		s.Logger.Debugw("wallet balance",
			"wallet_id", w.ID,
			"wallet_type", w.WalletType,
			"balance", w.Balance,
		)
	}

	s.Logger.Infow("total available credits calculated",
		"subscription_id", sub.ID,
		"customer_id", customerID,
		"currency", currency,
		"total_available", totalAvailable,
		"wallets_count", len(wallets),
	)

	return totalAvailable
}

// ExpirePendingUpdates is a placeholder for cron job compatibility
func (s *subscriptionPaymentProcessor) ExpirePendingUpdates(ctx context.Context) error {
	// Not needed for CreateSubscription flow
	s.Logger.Infow("expire pending updates called - no-op for create subscription flow")
	return nil
}
