package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/connection"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/idempotency"
	"github.com/flexprice/flexprice/internal/security"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/client"
	"github.com/stripe/stripe-go/v79/webhook"
)

// StripeService handles Stripe integration operations
type StripeService struct {
	ServiceParams
	encryptionService security.EncryptionService
}

// NewStripeService creates a new Stripe service instance
func NewStripeService(params ServiceParams) *StripeService {
	encryptionService, err := security.NewEncryptionService(params.Config, params.Logger)
	if err != nil {
		params.Logger.Fatalw("failed to create encryption service", "error", err)
	}

	return &StripeService{
		ServiceParams:     params,
		encryptionService: encryptionService,
	}
}

// decryptConnectionMetadata decrypts the connection metadata if it's encrypted
func (s *StripeService) decryptConnectionMetadata(metadata map[string]interface{}) (map[string]interface{}, error) {
	if metadata == nil {
		return nil, nil
	}

	decryptedMetadata := make(map[string]interface{})

	// Traverse metadata by key-value pairs
	for key, value := range metadata {
		// Check if the value is encrypted (string)
		if encryptedValue, ok := value.(string); ok {
			// Decrypt the JSON string
			decryptedJSON, err := s.encryptionService.Decrypt(encryptedValue)
			if err != nil {
				return nil, err
			}

			// Deserialize back to original type
			var decryptedValue interface{}
			if err := json.Unmarshal([]byte(decryptedJSON), &decryptedValue); err != nil {
				return nil, err
			}

			decryptedMetadata[key] = decryptedValue
		} else {
			// If value is not encrypted (for backward compatibility), keep as-is
			decryptedMetadata[key] = value
		}
	}

	return decryptedMetadata, nil
}

// GetDecryptedStripeConfig gets the decrypted Stripe configuration from a connection
func (s *StripeService) GetDecryptedStripeConfig(conn *connection.Connection) (*connection.StripeConnection, error) {
	// Decrypt metadata if needed
	decryptedMetadata, err := s.decryptConnectionMetadata(conn.Metadata)
	if err != nil {
		return nil, err
	}

	// Create a temporary connection with decrypted metadata
	tempConn := &connection.Connection{
		ID:            conn.ID,
		Name:          conn.Name,
		ProviderType:  conn.ProviderType,
		Metadata:      decryptedMetadata,
		EnvironmentID: conn.EnvironmentID,
		BaseModel:     conn.BaseModel,
	}

	// Now call GetStripeConfig on the decrypted connection
	return tempConn.GetStripeConfig()
}

// CreateCustomerInStripe creates a customer in Stripe and updates our customer with Stripe ID
func (s *StripeService) CreateCustomerInStripe(ctx context.Context, customerID string) error {
	// Get our customer
	customerService := NewCustomerService(s.ServiceParams)
	ourCustomerResp, err := customerService.GetCustomer(ctx, customerID)
	if err != nil {
		return err
	}
	ourCustomer := ourCustomerResp.Customer

	// Get Stripe connection for this environment
	conn, err := s.ConnectionRepo.GetByEnvironmentAndProvider(ctx, ourCustomer.EnvironmentID, types.SecretProviderStripe)
	if err != nil {
		return ierr.NewError("failed to get Stripe connection").
			WithHint("Stripe connection not configured for this environment").
			Mark(ierr.ErrNotFound)
	}

	stripeConfig, err := s.GetDecryptedStripeConfig(conn)
	if err != nil {
		return ierr.NewError("failed to get Stripe configuration").
			WithHint("Invalid Stripe configuration").
			Mark(ierr.ErrValidation)
	}

	// Initialize Stripe client
	sc := &client.API{}
	sc.Init(stripeConfig.SecretKey, nil)

	// Check if customer already has Stripe ID
	if stripeID, exists := ourCustomer.Metadata["stripe_customer_id"]; exists && stripeID != "" {
		return ierr.NewError("customer already has Stripe ID").
			WithHint("Customer is already synced with Stripe").
			Mark(ierr.ErrAlreadyExists)
	}

	// Create customer in Stripe
	params := &stripe.CustomerParams{
		Name:  stripe.String(ourCustomer.Name),
		Email: stripe.String(ourCustomer.Email),
		Metadata: map[string]string{
			"flexprice_customer_id": ourCustomer.ID,
			"flexprice_environment": ourCustomer.EnvironmentID,
		},
	}

	// Add address if available
	if ourCustomer.AddressLine1 != "" || ourCustomer.AddressCity != "" {
		params.Address = &stripe.AddressParams{
			Line1:      stripe.String(ourCustomer.AddressLine1),
			Line2:      stripe.String(ourCustomer.AddressLine2),
			City:       stripe.String(ourCustomer.AddressCity),
			State:      stripe.String(ourCustomer.AddressState),
			PostalCode: stripe.String(ourCustomer.AddressPostalCode),
			Country:    stripe.String(ourCustomer.AddressCountry),
		}
	}

	stripeCustomer, err := sc.Customers.New(params)
	if err != nil {
		return ierr.NewError("failed to create customer in Stripe").
			WithHint("Stripe API error").
			Mark(ierr.ErrHTTPClient)
	}

	// Update our customer with Stripe ID
	updateReq := dto.UpdateCustomerRequest{
		Metadata: map[string]string{
			"stripe_customer_id": stripeCustomer.ID,
		},
	}
	// Merge with existing metadata
	if ourCustomer.Metadata != nil {
		for k, v := range ourCustomer.Metadata {
			updateReq.Metadata[k] = v
		}
	}

	_, err = customerService.UpdateCustomer(ctx, ourCustomer.ID, updateReq)
	if err != nil {
		return err
	}

	return nil
}

// CreateCustomerFromStripe creates a customer in our system from Stripe webhook data
func (s *StripeService) CreateCustomerFromStripe(ctx context.Context, stripeCustomer *stripe.Customer, environmentID string) error {
	// Create customer service instance
	customerService := NewCustomerService(s.ServiceParams)

	// Check for existing customer by external ID if flexprice_customer_id is present
	var externalID string
	if flexpriceID, exists := stripeCustomer.Metadata["flexprice_customer_id"]; exists {
		externalID = flexpriceID
		// Check if customer with this external ID already exists
		existing, err := customerService.GetCustomerByLookupKey(ctx, externalID)
		if err == nil && existing != nil {
			// Customer exists with this external ID, update with Stripe ID
			updateReq := dto.UpdateCustomerRequest{
				Metadata: map[string]string{
					"stripe_customer_id": stripeCustomer.ID,
				},
			}
			// Merge with existing metadata
			if existing.Customer.Metadata != nil {
				for k, v := range existing.Customer.Metadata {
					updateReq.Metadata[k] = v
				}
			}
			_, err = customerService.UpdateCustomer(ctx, existing.Customer.ID, updateReq)
			return err
		}
	} else {
		// Generate external ID if not present
		externalID = types.GenerateUUIDWithPrefix(types.UUID_PREFIX_CUSTOMER)
	}

	// Create new customer using DTO
	createReq := dto.CreateCustomerRequest{
		ExternalID: externalID,
		Name:       stripeCustomer.Name,
		Email:      stripeCustomer.Email,
		Metadata: map[string]string{
			"stripe_customer_id": stripeCustomer.ID,
		},
	}

	// Add address if available
	if stripeCustomer.Address != nil {
		createReq.AddressLine1 = stripeCustomer.Address.Line1
		createReq.AddressLine2 = stripeCustomer.Address.Line2
		createReq.AddressCity = stripeCustomer.Address.City
		createReq.AddressState = stripeCustomer.Address.State
		createReq.AddressPostalCode = stripeCustomer.Address.PostalCode
		createReq.AddressCountry = stripeCustomer.Address.Country
	}

	_, err := customerService.CreateCustomer(ctx, createReq)
	return err
}

// CreatePaymentLink creates a Stripe checkout session for payment
func (s *StripeService) CreatePaymentLink(ctx context.Context, req *dto.CreateStripePaymentLinkRequest) (*dto.StripePaymentLinkResponse, error) {
	s.Logger.Infow("creating stripe payment link",
		"invoice_id", req.InvoiceID,
		"customer_id", req.CustomerID,
		"amount", req.Amount.String(),
		"currency", req.Currency,
		"environment_id", req.EnvironmentID,
	)

	// Get Stripe connection for this environment
	conn, err := s.ConnectionRepo.GetByEnvironmentAndProvider(ctx, req.EnvironmentID, types.SecretProviderStripe)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe connection").
			WithHint("Stripe connection not configured for this environment").
			WithReportableDetails(map[string]interface{}{
				"environment_id": req.EnvironmentID,
			}).
			Mark(ierr.ErrNotFound)
	}

	// Get customer to verify it exists and check for Stripe customer ID
	customerService := NewCustomerService(s.ServiceParams)
	customerResp, err := customerService.GetCustomer(ctx, req.CustomerID)
	if err != nil {
		return nil, ierr.NewError("failed to get customer").
			WithHint("Customer not found").
			WithReportableDetails(map[string]interface{}{
				"customer_id": req.CustomerID,
			}).
			Mark(ierr.ErrNotFound)
	}

	// Validate invoice and check payment eligibility
	invoiceService := NewInvoiceService(s.ServiceParams)
	invoiceResp, err := invoiceService.GetInvoice(ctx, req.InvoiceID)
	if err != nil {
		return nil, ierr.NewError("failed to get invoice").
			WithHint("Invoice not found").
			WithReportableDetails(map[string]interface{}{
				"invoice_id": req.InvoiceID,
			}).
			Mark(ierr.ErrNotFound)
	}

	// Validate invoice payment status
	if invoiceResp.PaymentStatus == types.PaymentStatusSucceeded {
		return nil, ierr.NewError("invoice is already paid").
			WithHint("Cannot create payment link for an already paid invoice").
			WithReportableDetails(map[string]interface{}{
				"invoice_id":     req.InvoiceID,
				"payment_status": invoiceResp.PaymentStatus,
			}).
			Mark(ierr.ErrValidation)
	}

	if invoiceResp.InvoiceStatus == types.InvoiceStatusVoided {
		return nil, ierr.NewError("invoice is voided").
			WithHint("Cannot create payment link for a voided invoice").
			WithReportableDetails(map[string]interface{}{
				"invoice_id":     req.InvoiceID,
				"invoice_status": invoiceResp.InvoiceStatus,
			}).
			Mark(ierr.ErrValidation)
	}

	// Validate payment amount against invoice remaining balance
	if req.Amount.GreaterThan(invoiceResp.AmountRemaining) {
		return nil, ierr.NewError("payment amount exceeds invoice remaining balance").
			WithHint("Payment amount cannot be greater than the remaining balance on the invoice").
			WithReportableDetails(map[string]interface{}{
				"invoice_id":        req.InvoiceID,
				"payment_amount":    req.Amount.String(),
				"invoice_remaining": invoiceResp.AmountRemaining.String(),
				"invoice_total":     invoiceResp.AmountDue.String(),
				"invoice_paid":      invoiceResp.AmountPaid.String(),
			}).
			Mark(ierr.ErrValidation)
	}

	// Validate currency matches invoice currency
	if req.Currency != invoiceResp.Currency {
		return nil, ierr.NewError("payment currency does not match invoice currency").
			WithHint("Payment currency must match the invoice currency").
			WithReportableDetails(map[string]interface{}{
				"invoice_id":       req.InvoiceID,
				"payment_currency": req.Currency,
				"invoice_currency": invoiceResp.Currency,
			}).
			Mark(ierr.ErrValidation)
	}

	// Check if customer has Stripe customer ID, if not create one
	stripeCustomerID, exists := customerResp.Customer.Metadata["stripe_customer_id"]
	if !exists || stripeCustomerID == "" {
		// Create customer in Stripe if not exists
		if err := s.CreateCustomerInStripe(ctx, req.CustomerID); err != nil {
			return nil, ierr.NewError("failed to create customer in Stripe").
				WithHint("Unable to create customer in Stripe").
				WithReportableDetails(map[string]interface{}{
					"customer_id": req.CustomerID,
					"error":       err.Error(),
				}).
				Mark(ierr.ErrSystem)
		}
		// Get the updated customer to get the Stripe customer ID
		customerResp, err = customerService.GetCustomer(ctx, req.CustomerID)
		if err != nil {
			return nil, err
		}
		stripeCustomerID = customerResp.Customer.Metadata["stripe_customer_id"]
	}

	// Get Stripe configuration
	stripeConfig, err := s.GetDecryptedStripeConfig(conn)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe configuration").
			WithHint("Invalid Stripe configuration").
			Mark(ierr.ErrValidation)
	}

	// Initialize Stripe client
	stripeClient := &client.API{}
	stripeClient.Init(stripeConfig.SecretKey, nil)

	// Convert amount to cents (Stripe expects amounts in smallest currency unit)
	amountCents := req.Amount.Mul(decimal.NewFromInt(100)).IntPart()

	// Create line items for the checkout session
	lineItems := []*stripe.CheckoutSessionLineItemParams{
		{
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency: stripe.String(req.Currency),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name: stripe.String(fmt.Sprintf("Invoice #%s", req.InvoiceID)),
				},
				UnitAmount: stripe.Int64(amountCents),
			},
			Quantity: stripe.Int64(1),
		},
	}

	// Build metadata for the session
	metadata := map[string]string{
		"invoice_id":     req.InvoiceID,
		"customer_id":    req.CustomerID,
		"environment_id": req.EnvironmentID,
	}

	// Add custom metadata if provided
	for k, v := range req.Metadata {
		metadata[k] = v
	}

	// Provide default URLs if not provided
	successURL := req.SuccessURL
	if successURL == "" {
		successURL = "https://i.pinimg.com/originals/93/7e/0f/937e0ff78860fd29a59a9f5d4242b4f7.gif"
	}

	cancelURL := req.CancelURL
	if cancelURL == "" {
		cancelURL = "https://www.redbubble.com/i/mug/Game-over-Play-again-Yes-by-Bisams/69638823.9Q0AD?epik=dj0yJnU9MVczbWt6T1Rmc2RLWGstMDVzeXZ4eVBmVzBPd09ZSVEmcD0wJm49clVRc3FrMXhCaHByVmhPRy1FM2FzZyZ0PUFBQUFBR2lMZk1n"
	}

	// Create checkout session parameters
	params := &stripe.CheckoutSessionParams{
		LineItems:  lineItems,
		Mode:       stripe.String("payment"),
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
		Metadata:   metadata,
		Customer:   stripe.String(stripeCustomerID),
	}

	// Create payment record in database first
	paymentService := NewPaymentService(s.ServiceParams)

	// Generate idempotency key using the proper generator
	idempGen := idempotency.NewGenerator()
	idempotencyKey := idempGen.GenerateKey(idempotency.ScopePayment, map[string]interface{}{
		"invoice_id": req.InvoiceID,
		"amount":     req.Amount,
		"currency":   req.Currency,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})

	paymentReq := &dto.CreatePaymentRequest{
		IdempotencyKey:    idempotencyKey,
		DestinationType:   types.PaymentDestinationTypeInvoice,
		DestinationID:     req.InvoiceID,
		PaymentMethodType: types.PaymentMethodTypePaymentLink,
		PaymentMethodID:   "stripe_payment_link", // Temporary placeholder, will be updated with payment intent ID from webhook
		Amount:            req.Amount,
		Currency:          req.Currency,
		Metadata: types.Metadata{
			"customer_id":     req.CustomerID,
			"environment_id":  req.EnvironmentID,
			"payment_gateway": "stripe",
			"success_url":     successURL,
			"cancel_url":      cancelURL,
		},
		ProcessPayment: false, // Don't process immediately, wait for webhook
	}

	payment, err := paymentService.CreatePayment(ctx, paymentReq)
	if err != nil {
		s.Logger.Errorw("failed to create payment record",
			"error", err,
			"invoice_id", req.InvoiceID)
		return nil, ierr.NewError("failed to create payment record").
			WithHint("Payment record creation failed").
			WithReportableDetails(map[string]interface{}{
				"invoice_id": req.InvoiceID,
				"error":      err.Error(),
			}).
			Mark(ierr.ErrSystem)
	}

	// Create the checkout session
	session, err := stripeClient.CheckoutSessions.New(params)
	if err != nil {
		s.Logger.Errorw("failed to create Stripe checkout session",
			"error", err,
			"invoice_id", req.InvoiceID,
			"payment_id", payment.ID)
		// Try to delete the payment record since Stripe session creation failed
		if deleteErr := paymentService.DeletePayment(ctx, payment.ID); deleteErr != nil {
			s.Logger.Errorw("failed to delete payment record after Stripe session creation failed",
				"error", deleteErr,
				"payment_id", payment.ID)
		}
		return nil, ierr.NewError("failed to create payment link").
			WithHint("Unable to create Stripe checkout session").
			WithReportableDetails(map[string]interface{}{
				"invoice_id": req.InvoiceID,
				"error":      err.Error(),
			}).
			Mark(ierr.ErrSystem)
	}

	// Update payment record with Stripe session information
	payment.Metadata["stripe_session_id"] = session.ID
	payment.Metadata["stripe_customer_id"] = stripeCustomerID
	payment.Metadata["payment_url"] = session.URL

	// Update payment with gateway information
	paymentGateway := "stripe"
	gatewayPaymentID := session.ID

	_, err = paymentService.UpdatePayment(ctx, payment.ID, dto.UpdatePaymentRequest{
		PaymentGateway:   &paymentGateway,
		GatewayPaymentID: &gatewayPaymentID,
		Metadata:         &payment.Metadata,
	})
	if err != nil {
		s.Logger.Errorw("failed to update payment record with Stripe session info",
			"error", err,
			"payment_id", payment.ID,
			"session_id", session.ID)
		// Don't fail the entire request if metadata update fails
		// The payment record exists and Stripe session was created
	}

	response := &dto.StripePaymentLinkResponse{
		ID:         session.ID,
		PaymentURL: session.URL,
		PaymentIntentID: func() string {
			if session.PaymentIntent != nil {
				return session.PaymentIntent.ID
			}
			return ""
		}(),
		Amount:    req.Amount,
		Currency:  req.Currency,
		Status:    string(session.Status),
		CreatedAt: session.Created,
		PaymentID: func() string {
			if payment != nil {
				return payment.ID
			}
			return ""
		}(),
	}

	s.Logger.Infow("successfully created stripe payment link",
		"payment_id", response.PaymentID,
		"session_id", session.ID,
		"payment_url", session.URL,
		"invoice_id", req.InvoiceID,
		"amount", req.Amount.String(),
		"currency", req.Currency,
	)

	return response, nil
}

// ReconcilePaymentWithInvoice updates the invoice payment status and amounts when a payment succeeds
func (s *StripeService) ReconcilePaymentWithInvoice(ctx context.Context, paymentID string, paymentAmount decimal.Decimal) error {
	s.Logger.Infow("starting payment reconciliation with invoice",
		"payment_id", paymentID,
		"payment_amount", paymentAmount.String(),
	)

	// Get the payment record
	paymentService := NewPaymentService(s.ServiceParams)
	payment, err := paymentService.GetPayment(ctx, paymentID)
	if err != nil {
		s.Logger.Errorw("failed to get payment record for reconciliation",
			"error", err,
			"payment_id", paymentID,
		)
		return ierr.WithError(err).
			WithHint("Failed to get payment record for reconciliation").
			WithReportableDetails(map[string]interface{}{
				"payment_id": paymentID,
			}).
			Mark(ierr.ErrSystem)
	}

	s.Logger.Infow("got payment record for reconciliation",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"payment_amount", payment.Amount.String(),
	)

	// Get the invoice
	invoiceService := NewInvoiceService(s.ServiceParams)
	invoiceResp, err := invoiceService.GetInvoice(ctx, payment.DestinationID)
	if err != nil {
		s.Logger.Errorw("failed to get invoice for payment reconciliation",
			"error", err,
			"payment_id", paymentID,
			"invoice_id", payment.DestinationID,
		)
		return ierr.WithError(err).
			WithHint("Failed to get invoice for payment reconciliation").
			WithReportableDetails(map[string]interface{}{
				"payment_id": paymentID,
				"invoice_id": payment.DestinationID,
			}).
			Mark(ierr.ErrSystem)
	}

	s.Logger.Infow("got invoice for reconciliation",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"invoice_amount_due", invoiceResp.AmountDue.String(),
		"invoice_amount_paid", invoiceResp.AmountPaid.String(),
		"invoice_amount_remaining", invoiceResp.AmountRemaining.String(),
		"invoice_payment_status", invoiceResp.PaymentStatus,
		"invoice_status", invoiceResp.InvoiceStatus,
	)

	// Calculate new amounts
	newAmountPaid := invoiceResp.AmountPaid.Add(paymentAmount)
	newAmountRemaining := invoiceResp.AmountDue.Sub(newAmountPaid)

	// Determine payment status
	var newPaymentStatus types.PaymentStatus
	if newAmountRemaining.IsZero() || newAmountRemaining.IsNegative() {
		newPaymentStatus = types.PaymentStatusSucceeded
	} else {
		newPaymentStatus = types.PaymentStatusPending
	}

	s.Logger.Infow("calculated new amounts for reconciliation",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"payment_amount", paymentAmount.String(),
		"new_amount_paid", newAmountPaid.String(),
		"new_amount_remaining", newAmountRemaining.String(),
		"new_payment_status", newPaymentStatus,
	)

	// Update invoice payment status and amounts using reconciliation method
	s.Logger.Infow("calling invoice reconciliation",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"payment_amount", paymentAmount.String(),
		"new_payment_status", newPaymentStatus,
	)

	err = invoiceService.ReconcilePaymentStatus(ctx, payment.DestinationID, newPaymentStatus, &paymentAmount)
	if err != nil {
		s.Logger.Errorw("failed to update invoice payment status during reconciliation",
			"error", err,
			"payment_id", paymentID,
			"invoice_id", payment.DestinationID,
			"payment_amount", paymentAmount.String(),
			"new_payment_status", newPaymentStatus,
		)
		return ierr.WithError(err).
			WithHint("Failed to update invoice payment status").
			WithReportableDetails(map[string]interface{}{
				"payment_id":         paymentID,
				"invoice_id":         payment.DestinationID,
				"payment_amount":     paymentAmount.String(),
				"new_payment_status": newPaymentStatus,
			}).
			Mark(ierr.ErrSystem)
	}

	s.Logger.Infow("successfully reconciled payment with invoice",
		"payment_id", paymentID,
		"invoice_id", payment.DestinationID,
		"payment_amount", paymentAmount.String(),
		"new_payment_status", newPaymentStatus,
		"new_amount_paid", newAmountPaid.String(),
		"new_amount_remaining", newAmountRemaining.String(),
	)

	return nil
}

// ParseWebhookEvent parses a Stripe webhook event with signature verification
func (s *StripeService) ParseWebhookEvent(payload []byte, signature string, webhookSecret string) (*stripe.Event, error) {
	// Verify the webhook signature, ignoring API version mismatch
	options := webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	}
	event, err := webhook.ConstructEventWithOptions(payload, signature, webhookSecret, options)
	if err != nil {
		// Log the error using structured logging
		s.Logger.Errorw("Stripe webhook verification failed", "error", err)
		return nil, ierr.NewError("failed to verify webhook signature").
			WithHint("Invalid webhook signature or payload").
			Mark(ierr.ErrValidation)
	}
	return &event, nil
}

// VerifyWebhookSignature verifies the Stripe webhook signature
func (s *StripeService) VerifyWebhookSignature(payload []byte, signature string, webhookSecret string) error {
	_, err := webhook.ConstructEvent(payload, signature, webhookSecret)
	if err != nil {
		return ierr.NewError("failed to verify webhook signature").
			WithHint("Invalid webhook signature or payload").
			Mark(ierr.ErrValidation)
	}
	return nil
}

// GetPaymentStatus gets the payment status from Stripe checkout session
func (s *StripeService) GetPaymentStatus(ctx context.Context, sessionID string, environmentID string) (*dto.PaymentStatusResponse, error) {
	// Get Stripe connection for this environment
	conn, err := s.ConnectionRepo.GetByEnvironmentAndProvider(ctx, environmentID, types.SecretProviderStripe)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe connection").
			WithHint("Stripe connection not configured for this environment").
			WithReportableDetails(map[string]interface{}{
				"environment_id": environmentID,
			}).
			Mark(ierr.ErrNotFound)
	}

	// Get Stripe configuration
	stripeConfig, err := s.GetDecryptedStripeConfig(conn)
	if err != nil {
		return nil, ierr.NewError("failed to get Stripe configuration").
			WithHint("Invalid Stripe configuration").
			Mark(ierr.ErrValidation)
	}

	// Initialize Stripe client
	stripeClient := &client.API{}
	stripeClient.Init(stripeConfig.SecretKey, nil)

	// Get the checkout session with expanded fields
	params := &stripe.CheckoutSessionParams{
		Expand: []*string{
			stripe.String("payment_intent"),
			stripe.String("line_items"),
			stripe.String("customer"),
		},
	}
	session, err := stripeClient.CheckoutSessions.Get(sessionID, params)
	if err != nil {
		s.Logger.Errorw("failed to get Stripe checkout session",
			"error", err,
			"session_id", sessionID)
		return nil, ierr.NewError("failed to get payment status").
			WithHint("Unable to retrieve Stripe checkout session").
			WithReportableDetails(map[string]interface{}{
				"session_id": sessionID,
				"error":      err.Error(),
			}).
			Mark(ierr.ErrSystem)
	}

	// Log session details for debugging
	s.Logger.Debugw("retrieved Stripe checkout session",
		"session_id", session.ID,
		"status", session.Status,
		"has_payment_intent", session.PaymentIntent != nil,
		"has_line_items", session.LineItems != nil,
		"line_items_count", func() int {
			if session.LineItems != nil {
				return len(session.LineItems.Data)
			}
			return 0
		}(),
		"has_customer", session.Customer != nil,
	)

	// Get payment intent if available
	var paymentIntentID string
	var paymentStatus string
	var amount decimal.Decimal
	var currency string

	// First try to get data from payment intent
	if session.PaymentIntent != nil {
		paymentIntentID = session.PaymentIntent.ID
		paymentStatus = string(session.PaymentIntent.Status)
		if session.PaymentIntent.Amount > 0 {
			amount = decimal.NewFromInt(session.PaymentIntent.Amount).Div(decimal.NewFromInt(100))
		}
		if session.PaymentIntent.Currency != "" {
			currency = string(session.PaymentIntent.Currency)
		}
	}

	// If payment intent data is incomplete, try to get from session
	if paymentStatus == "" {
		paymentStatus = string(session.Status)
	}

	// If amount is still 0, try to get from line items
	if amount.IsZero() && session.LineItems != nil && len(session.LineItems.Data) > 0 {
		item := session.LineItems.Data[0]
		if item.AmountTotal > 0 {
			amount = decimal.NewFromInt(item.AmountTotal).Div(decimal.NewFromInt(100))
		}
		if item.Currency != "" && currency == "" {
			currency = string(item.Currency)
		}
	}

	// If currency is still empty, try to get from session metadata or default
	if currency == "" {
		// Check if currency is in metadata
		if session.Metadata != nil {
			if curr, exists := session.Metadata["currency"]; exists {
				currency = curr
			}
		}
		// Default to USD if still empty
		if currency == "" {
			currency = "usd"
		}
	}

	// Log extracted values for debugging
	s.Logger.Debugw("extracted payment status values",
		"session_id", session.ID,
		"payment_intent_id", paymentIntentID,
		"status", paymentStatus,
		"amount", amount.String(),
		"currency", currency,
		"customer_id", func() string {
			if session.Customer != nil {
				return session.Customer.ID
			}
			return ""
		}(),
	)

	return &dto.PaymentStatusResponse{
		SessionID:       session.ID,
		PaymentIntentID: paymentIntentID,
		Status:          paymentStatus,
		Amount:          amount,
		Currency:        currency,
		CustomerID: func() string {
			if session.Customer != nil {
				return session.Customer.ID
			}
			return ""
		}(),
		CreatedAt: session.Created,
		ExpiresAt: session.ExpiresAt,
		Metadata:  session.Metadata,
	}, nil
}
