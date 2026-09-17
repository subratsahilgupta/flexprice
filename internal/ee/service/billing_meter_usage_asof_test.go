package service

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/stretchr/testify/assert"
)

func TestResolveAsOf(t *testing.T) {
	d := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, d, resolveAsOf(&dto.PrepareSubscriptionInvoiceRequestParams{AsOf: d}))

	got := resolveAsOf(&dto.PrepareSubscriptionInvoiceRequestParams{}) // zero
	assert.WithinDuration(t, time.Now().UTC(), got, time.Minute)
}
