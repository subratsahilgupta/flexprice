package razorpay

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/customer"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectAutoChargeToken(t *testing.T) {
	now := time.Now().UTC()
	upiToken := &interfaces.ProviderPaymentMethod{GatewayMethodID: "tok_upi", Method: types.PaymentMethodTypeUPI, Active: true, CreatedAt: now, Recurring: &interfaces.ProviderRecurringPaymentDetails{Status: types.RecurringPaymentStatusActive}}
	cardToken := &interfaces.ProviderPaymentMethod{GatewayMethodID: "tok_card", Method: types.PaymentMethodTypeCard, Active: true, CreatedAt: now, Recurring: &interfaces.ProviderRecurringPaymentDetails{Status: types.RecurringPaymentStatusActive}}
	inactiveToken := &interfaces.ProviderPaymentMethod{GatewayMethodID: "tok_inactive", Method: types.PaymentMethodTypeCard, Active: false, CreatedAt: now, Recurring: &interfaces.ProviderRecurringPaymentDetails{Status: types.RecurringPaymentStatusActive}}

	tests := []struct {
		name      string
		tokens    []*interfaces.ProviderPaymentMethod
		wantID    string
		wantFound bool
	}{
		{name: "UPI only", tokens: []*interfaces.ProviderPaymentMethod{upiToken}, wantID: "tok_upi", wantFound: true},
		{name: "Card only", tokens: []*interfaces.ProviderPaymentMethod{cardToken}, wantID: "tok_card", wantFound: true},
		{name: "both present, Card wins", tokens: []*interfaces.ProviderPaymentMethod{upiToken, cardToken}, wantID: "tok_card", wantFound: true},
		{name: "inactive token excluded", tokens: []*interfaces.ProviderPaymentMethod{inactiveToken}, wantID: "", wantFound: false},
		{name: "neither present", tokens: nil, wantID: "", wantFound: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := selectAutoChargeToken(tt.tokens, "", decimal.NewFromInt(100))
			assert.Equal(t, tt.wantFound, ok)
			if tt.wantFound {
				assert.Equal(t, tt.wantID, got.GatewayMethodID)
			}
		})
	}
}

type fakeRecurringChargeClient struct {
	RazorpayClient
	chargeErr error
	order     map[string]interface{}
	orderErr  error
}

func (c *fakeRecurringChargeClient) CreateOrder(_ context.Context, _ map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"id": "order_1"}, nil
}

func (c *fakeRecurringChargeClient) CreateRecurringPayment(_ context.Context, _ map[string]interface{}) (map[string]interface{}, error) {
	if c.chargeErr != nil {
		return nil, c.chargeErr
	}
	return map[string]interface{}{"id": "pay_1"}, nil
}

func (c *fakeRecurringChargeClient) FetchOrder(_ context.Context, _ string) (map[string]interface{}, error) {
	return c.order, c.orderErr
}

func TestChargeSavedTokenFallsBackOnlyWhenNothingWasAttempted(t *testing.T) {
	rejection := errors.New("token not eligible for recurring")
	tests := []struct {
		name        string
		client      *fakeRecurringChargeClient
		wantCharged bool
		wantErr     bool
	}{
		{
			name:        "charge submitted",
			client:      &fakeRecurringChargeClient{},
			wantCharged: true,
		},
		{
			name: "rejected before any attempt falls back",
			client: &fakeRecurringChargeClient{
				chargeErr: rejection,
				order:     map[string]interface{}{"status": "created", "attempts": float64(0)},
			},
		},
		{
			name: "attempted order keeps the error",
			client: &fakeRecurringChargeClient{
				chargeErr: rejection,
				order:     map[string]interface{}{"status": "attempted", "attempts": float64(1)},
			},
			wantErr: true,
		},
		{
			name: "unverifiable order keeps the error",
			client: &fakeRecurringChargeClient{
				chargeErr: rejection,
				orderErr:  errors.New("timeout"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &PaymentService{
				client: tt.client,
				customerSvc: &stubRazorpayCustomerSvc{tokens: []*interfaces.ProviderPaymentMethod{{
					GatewayMethodID: "token_1",
					Method:          types.PaymentMethodTypeCard,
					Active:          true,
					Recurring:       &interfaces.ProviderRecurringPaymentDetails{Status: types.RecurringPaymentStatusActive},
				}}},
				logger: logger.NewNoopLogger(),
			}

			result, charged, err := svc.ChargeSavedToken(context.Background(), ChargeSavedTokenRequest{
				Customer:  &customer.Customer{ID: "cust_1"},
				InvoiceID: "inv_1",
				Amount:    decimal.NewFromInt(100),
				Currency:  "INR",
			})

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, rejection)
				assert.False(t, charged)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantCharged, charged)
			if tt.wantCharged {
				require.NotNil(t, result)
				assert.Equal(t, "pay_1", result.RazorpayPaymentID)
			}
		})
	}
}
