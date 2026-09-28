package integrations

import (
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

// StripePaymentStatus is a Stripe PaymentIntent status.
type StripePaymentStatus string

const (
	StripePaymentStatusSucceeded             StripePaymentStatus = "succeeded"
	StripePaymentStatusRequiresPaymentMethod StripePaymentStatus = "requires_payment_method"
	StripePaymentStatusRequiresConfirmation  StripePaymentStatus = "requires_confirmation"
	StripePaymentStatusRequiresAction        StripePaymentStatus = "requires_action"
	StripePaymentStatusRequiresCapture       StripePaymentStatus = "requires_capture"
	StripePaymentStatusProcessing            StripePaymentStatus = "processing"
	StripePaymentStatusCanceled              StripePaymentStatus = "canceled"
)

// ToFlexpricePaymentStatus maps a Stripe PaymentIntent status to a FlexPrice PaymentStatus.
// The states between confirmation and settlement (requires_confirmation, requires_action,
// requires_capture, processing) are in flight rather than outcomes, so they return an
// empty status with a nil error to signal "still pending, no transition" — the same
// contract as the Razorpay mappers. requires_payment_method is not one of them: Stripe
// parks a declined intent there, so an intent read by id after a charge attempt has
// failed. Anything unrecognised is an error, so a status Stripe adds later cannot be
// silently read as pending.
func (s StripePaymentStatus) ToFlexpricePaymentStatus() (types.PaymentStatus, error) {
	switch s {
	case StripePaymentStatusSucceeded:
		return types.PaymentStatusSucceeded, nil
	case StripePaymentStatusRequiresPaymentMethod, StripePaymentStatusCanceled:
		return types.PaymentStatusFailed, nil
	case StripePaymentStatusRequiresConfirmation, StripePaymentStatusRequiresAction,
		StripePaymentStatusRequiresCapture, StripePaymentStatusProcessing:
		return "", nil
	default:
		return "", ierr.NewError("unmapped stripe payment status").
			WithReportableDetails(map[string]interface{}{
				"stripe_status": s,
			}).
			Mark(ierr.ErrInvalidOperation)
	}
}

// StripeCheckoutSessionStatus is a Stripe Checkout Session status: whether the customer
// is done with the hosted page.
type StripeCheckoutSessionStatus string

const (
	StripeCheckoutSessionStatusOpen     StripeCheckoutSessionStatus = "open"
	StripeCheckoutSessionStatusComplete StripeCheckoutSessionStatus = "complete"
	StripeCheckoutSessionStatusExpired  StripeCheckoutSessionStatus = "expired"
)

// StripeCheckoutPaymentStatus is a Checkout Session's payment_status: whether the money
// behind the page has arrived.
type StripeCheckoutPaymentStatus string

const (
	StripeCheckoutPaymentStatusPaid              StripeCheckoutPaymentStatus = "paid"
	StripeCheckoutPaymentStatusUnpaid            StripeCheckoutPaymentStatus = "unpaid"
	StripeCheckoutPaymentStatusNoPaymentRequired StripeCheckoutPaymentStatus = "no_payment_required"
)

// ToFlexpricePaymentStatus maps a Checkout Session onto a FlexPrice PaymentStatus. The
// session is the hosted page, not the money: status says whether the customer finished
// the page and payment_status whether the funds arrived, and only the pair answers "is
// this paid?". A complete session that is still unpaid is a delayed method (bank debit)
// in flight — pending, and the webhook settles it. Expired is the one terminal failure a
// session can report. Anything unrecognised is an error rather than pending, so the
// caller logs the status Stripe sent instead of quietly waiting on it.
func (s StripeCheckoutSessionStatus) ToFlexpricePaymentStatus(paymentStatus StripeCheckoutPaymentStatus) (types.PaymentStatus, error) {
	switch s {
	case StripeCheckoutSessionStatusExpired:
		return types.PaymentStatusFailed, nil
	case StripeCheckoutSessionStatusOpen:
		return "", nil
	case StripeCheckoutSessionStatusComplete:
		switch paymentStatus {
		case StripeCheckoutPaymentStatusPaid, StripeCheckoutPaymentStatusNoPaymentRequired:
			return types.PaymentStatusSucceeded, nil
		case StripeCheckoutPaymentStatusUnpaid:
			return "", nil
		default:
			return "", ierr.NewError("unmapped stripe checkout payment status").
				WithReportableDetails(map[string]interface{}{
					"stripe_session_status": s,
					"stripe_payment_status": paymentStatus,
				}).
				Mark(ierr.ErrInvalidOperation)
		}
	default:
		return "", ierr.NewError("unmapped stripe checkout session status").
			WithReportableDetails(map[string]interface{}{
				"stripe_session_status": s,
				"stripe_payment_status": paymentStatus,
			}).
			Mark(ierr.ErrInvalidOperation)
	}
}
