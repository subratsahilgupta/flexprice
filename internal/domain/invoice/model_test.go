package invoice

import (
	"testing"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestInvoice_IsPaid(t *testing.T) {
	tests := []struct {
		status types.PaymentStatus
		want   bool
	}{
		{types.PaymentStatusSucceeded, true},
		{types.PaymentStatusOverpaid, true},
		{types.PaymentStatusPending, false},
		{types.PaymentStatusFailed, false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.want, (&Invoice{PaymentStatus: tt.status}).IsPaid())
		})
	}
}
