package razorpay

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/customer"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRazorpaySubscriptionMethod(t *testing.T) {
	tests := []struct {
		name    string
		input   types.PaymentMethodType
		want    string
		wantErr bool
	}{
		{name: "empty defaults to upi", input: "", want: "upi", wantErr: false},
		{name: "UPI maps to upi", input: types.PaymentMethodTypeUPI, want: "upi", wantErr: false},
		{name: "Card maps to card", input: types.PaymentMethodTypeCard, want: "card", wantErr: false},
		{name: "unsupported method errors", input: types.PaymentMethodTypeACH, want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := razorpaySubscriptionMethod(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.True(t, ierr.IsNotImplemented(err), "expected ErrNotImplemented-marked error")
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

type stubRazorpayCustomerSvc struct {
	tokens []*interfaces.ProviderPaymentMethod
	err    error
}

func (s *stubRazorpayCustomerSvc) EnsureCustomerSyncedToRazorpay(ctx context.Context, customerID string, customerService interfaces.CustomerService) (*customer.Customer, error) {
	return nil, nil
}
func (s *stubRazorpayCustomerSvc) SyncCustomerToRazorpay(ctx context.Context, flexpriceCustomer *customer.Customer) (string, error) {
	return "", nil
}
func (s *stubRazorpayCustomerSvc) GetRazorpayCustomerID(ctx context.Context, customerID string) (string, error) {
	return "", nil
}
func (s *stubRazorpayCustomerSvc) UpdateRazorpayCustomerNotes(ctx context.Context, razorpayCustomerID string, notes map[string]interface{}) error {
	return nil
}
func (s *stubRazorpayCustomerSvc) ListConfirmedCustomerTokens(ctx context.Context, customerID string) (string, []*interfaces.ProviderPaymentMethod, error) {
	if s.err != nil {
		return "", nil, s.err
	}
	return "cust_rzp_1", s.tokens, nil
}

func TestRazorpayHasAutoChargeableMethod(t *testing.T) {
	ctx := context.Background()

	t.Run("nil adapter or service returns false, nil", func(t *testing.T) {
		var nilAdapter *CheckoutAdapter
		has, err := nilAdapter.HasAutoChargeableMethod(ctx, interfaces.HasAutoChargeableMethodRequest{CustomerID: "cust_1"})
		assert.NoError(t, err)
		assert.False(t, has)

		adapterWithoutSvc := &CheckoutAdapter{}
		has, err = adapterWithoutSvc.HasAutoChargeableMethod(ctx, interfaces.HasAutoChargeableMethodRequest{CustomerID: "cust_1"})
		assert.NoError(t, err)
		assert.False(t, has)

		adapterWithoutCustomerSvc := &CheckoutAdapter{Svc: &PaymentService{}}
		has, err = adapterWithoutCustomerSvc.HasAutoChargeableMethod(ctx, interfaces.HasAutoChargeableMethodRequest{CustomerID: "cust_1"})
		assert.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("returns true when customer has confirmed recurring mandate token with nil amount", func(t *testing.T) {
		adapter := &CheckoutAdapter{
			Svc: &PaymentService{
				customerSvc: &stubRazorpayCustomerSvc{
					tokens: []*interfaces.ProviderPaymentMethod{
						{GatewayMethodID: "token_123", Method: types.PaymentMethodTypeUPI, Active: true},
					},
				},
			},
		}
		has, err := adapter.HasAutoChargeableMethod(ctx, interfaces.HasAutoChargeableMethodRequest{CustomerID: "cust_1"})
		assert.NoError(t, err)
		assert.True(t, has)
	})

	t.Run("returns true when amount is within mandate ceiling", func(t *testing.T) {
		maxAmount := decimal.NewFromInt(1000)
		amount := decimal.NewFromInt(500)
		adapter := &CheckoutAdapter{
			Svc: &PaymentService{
				customerSvc: &stubRazorpayCustomerSvc{
					tokens: []*interfaces.ProviderPaymentMethod{
						{GatewayMethodID: "token_123", Method: types.PaymentMethodTypeUPI, Active: true, Recurring: &interfaces.ProviderRecurringPaymentDetails{MaxAmount: &maxAmount}},
					},
				},
			},
		}
		has, err := adapter.HasAutoChargeableMethod(ctx, interfaces.HasAutoChargeableMethodRequest{CustomerID: "cust_1", Amount: &amount})
		assert.NoError(t, err)
		assert.True(t, has)
	})

	t.Run("returns false when amount exceeds mandate ceiling", func(t *testing.T) {
		maxAmount := decimal.NewFromInt(500)
		amount := decimal.NewFromInt(1000)
		adapter := &CheckoutAdapter{
			Svc: &PaymentService{
				customerSvc: &stubRazorpayCustomerSvc{
					tokens: []*interfaces.ProviderPaymentMethod{
						{GatewayMethodID: "token_123", Method: types.PaymentMethodTypeUPI, Active: true, Recurring: &interfaces.ProviderRecurringPaymentDetails{MaxAmount: &maxAmount}},
					},
				},
			},
		}
		has, err := adapter.HasAutoChargeableMethod(ctx, interfaces.HasAutoChargeableMethodRequest{CustomerID: "cust_1", Amount: &amount})
		assert.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("returns false when all confirmed tokens are expired", func(t *testing.T) {
		past := time.Now().Add(-24 * time.Hour).UTC()
		adapter := &CheckoutAdapter{
			Svc: &PaymentService{
				customerSvc: &stubRazorpayCustomerSvc{
					tokens: []*interfaces.ProviderPaymentMethod{
						{GatewayMethodID: "token_123", Method: types.PaymentMethodTypeUPI, Active: true, Recurring: &interfaces.ProviderRecurringPaymentDetails{AutoChargeableTill: &past}},
					},
				},
			},
		}
		has, err := adapter.HasAutoChargeableMethod(ctx, interfaces.HasAutoChargeableMethodRequest{CustomerID: "cust_1"})
		assert.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("returns false when no confirmed tokens exist", func(t *testing.T) {
		adapter := &CheckoutAdapter{
			Svc: &PaymentService{
				customerSvc: &stubRazorpayCustomerSvc{
					tokens: []*interfaces.ProviderPaymentMethod{},
				},
			},
		}
		has, err := adapter.HasAutoChargeableMethod(ctx, interfaces.HasAutoChargeableMethodRequest{CustomerID: "cust_1"})
		assert.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("returns false, nil when customer is not found", func(t *testing.T) {
		adapter := &CheckoutAdapter{
			Svc: &PaymentService{
				customerSvc: &stubRazorpayCustomerSvc{
					err: ierr.NewError("customer not found").Mark(ierr.ErrNotFound),
				},
			},
		}
		has, err := adapter.HasAutoChargeableMethod(ctx, interfaces.HasAutoChargeableMethodRequest{CustomerID: "cust_1"})
		assert.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("returns false when all confirmed tokens are inactive", func(t *testing.T) {
		adapter := &CheckoutAdapter{
			Svc: &PaymentService{
				customerSvc: &stubRazorpayCustomerSvc{
					tokens: []*interfaces.ProviderPaymentMethod{
						{GatewayMethodID: "token_123", Method: types.PaymentMethodTypeUPI, Active: false},
					},
				},
			},
		}
		has, err := adapter.HasAutoChargeableMethod(ctx, interfaces.HasAutoChargeableMethodRequest{CustomerID: "cust_1"})
		assert.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("returns false, err when a real error occurs", func(t *testing.T) {
		adapter := &CheckoutAdapter{
			Svc: &PaymentService{
				customerSvc: &stubRazorpayCustomerSvc{
					err: ierr.NewError("api failure").Mark(ierr.ErrHTTPClient),
				},
			},
		}
		has, err := adapter.HasAutoChargeableMethod(ctx, interfaces.HasAutoChargeableMethodRequest{CustomerID: "cust_1"})
		assert.Error(t, err)
		assert.True(t, ierr.IsHTTPClient(err))
		assert.False(t, has)
	})
}

func TestNormalizeRazorpayToken(t *testing.T) {
	t.Run("confirmed token sets Active to true and maps fields", func(t *testing.T) {
		raw := map[string]interface{}{
			"id":     "tok_123",
			"method": "upi",
			"recurring_details": map[string]interface{}{
				"status": "confirmed",
			},
			"max_amount": float64(1500000),
			"created_at": float64(1700000000),
		}
		pm, err := NormalizeRazorpayToken(raw)
		assert.NoError(t, err)
		assert.NotNil(t, pm)
		assert.True(t, pm.Active)
		assert.Equal(t, "tok_123", pm.GatewayMethodID)
		assert.Equal(t, types.PaymentMethodTypeUPI, pm.Method)
		assert.NotNil(t, pm.RecurringMaxAmount())
		assert.Equal(t, "15000", pm.RecurringMaxAmount().String())
	})

	t.Run("lapsed mandate reports expired recurring status", func(t *testing.T) {
		raw := map[string]interface{}{
			"id":                "tok_old",
			"method":            "card",
			"recurring_details": map[string]interface{}{"status": "confirmed"},
			"expired_at":        float64(time.Now().Add(-time.Hour).Unix()),
		}
		pm, err := NormalizeRazorpayToken(raw)
		require.NoError(t, err)
		require.NotNil(t, pm)
		assert.Equal(t, types.RecurringPaymentStatusExpired, pm.Recurring.Status)
	})

	t.Run("live mandate keeps active recurring status", func(t *testing.T) {
		raw := map[string]interface{}{
			"id":                "tok_live",
			"method":            "card",
			"recurring_details": map[string]interface{}{"status": "confirmed"},
			"expired_at":        float64(time.Now().Add(time.Hour).Unix()),
		}
		pm, err := NormalizeRazorpayToken(raw)
		require.NoError(t, err)
		require.NotNil(t, pm)
		assert.Equal(t, types.RecurringPaymentStatusActive, pm.Recurring.Status)
	})

	t.Run("non-confirmed token returns nil, nil", func(t *testing.T) {
		raw := map[string]interface{}{
			"id":     "tok_456",
			"method": "card",
			"recurring_details": map[string]interface{}{
				"status": "rejected",
			},
		}
		pm, err := NormalizeRazorpayToken(raw)
		assert.NoError(t, err)
		assert.Nil(t, pm)
	})
}

func TestPaymentMethodAdapterListSavedMethods(t *testing.T) {
	ctx := context.Background()

	t.Run("returns tokens", func(t *testing.T) {
		a := &PaymentMethodAdapter{CustomerSvc: &stubRazorpayCustomerSvc{tokens: []*interfaces.ProviderPaymentMethod{
			{GatewayMethodID: "token_1", Method: types.PaymentMethodTypeCard, Active: true},
		}}}
		got, err := a.ListSavedMethods(ctx, "cust_1")
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "token_1", got[0].GatewayMethodID)
	})

	t.Run("unsynced customer has nothing saved", func(t *testing.T) {
		a := &PaymentMethodAdapter{CustomerSvc: &stubRazorpayCustomerSvc{err: ierr.NewError("not synced").Mark(ierr.ErrNotFound)}}
		got, err := a.ListSavedMethods(ctx, "cust_1")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("gateway failure propagates", func(t *testing.T) {
		a := &PaymentMethodAdapter{CustomerSvc: &stubRazorpayCustomerSvc{err: errors.New("boom")}}
		_, err := a.ListSavedMethods(ctx, "cust_1")
		assert.Error(t, err)
	})
}

func TestPaymentMethodAdapterRejectsManagement(t *testing.T) {
	ctx := context.Background()
	a := &PaymentMethodAdapter{CustomerSvc: &stubRazorpayCustomerSvc{}}

	assert.True(t, ierr.IsValidation(a.DeleteSavedMethod(ctx, "cust_1", "token_1")))
	assert.True(t, ierr.IsValidation(a.SetDefaultSavedMethod(ctx, "cust_1", "token_1")))
	_, err := a.CreateSetupLink(ctx, interfaces.SetupLinkRequest{CustomerID: "cust_1"})
	assert.True(t, ierr.IsValidation(err))
}
