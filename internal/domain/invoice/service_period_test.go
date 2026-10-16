package invoice

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveServicePeriod(t *testing.T) {
	date := func(month time.Month, day int) *time.Time {
		v := time.Date(2026, month, day, 0, 0, 0, 0, time.UTC)
		return &v
	}
	sept, oct, nov := date(9, 12), date(10, 12), date(11, 12)
	tests := []struct {
		name       string
		items      []*InvoiceLineItem
		start, end *time.Time
	}{
		{"empty", nil, nil, nil},
		{"advance", []*InvoiceLineItem{{PeriodStart: oct, PeriodEnd: nov}}, oct, nov},
		{"mixed unordered", []*InvoiceLineItem{{PeriodStart: oct, PeriodEnd: nov}, {PeriodStart: sept, PeriodEnd: oct}}, sept, nov},
		{"one time", []*InvoiceLineItem{{PeriodStart: oct, PeriodEnd: oct}}, oct, oct},
		{"invalid", []*InvoiceLineItem{nil, {}, {PeriodStart: sept}, {PeriodEnd: nov}, {PeriodStart: nov, PeriodEnd: sept}, {PeriodStart: &time.Time{}, PeriodEnd: nov}}, nil, nil},
		{"invalid alongside valid", []*InvoiceLineItem{{PeriodStart: nov, PeriodEnd: sept}, {PeriodStart: oct, PeriodEnd: nov}}, oct, nov},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := ResolveServicePeriod(tt.items)
			assert.Equal(t, tt.start, start)
			assert.Equal(t, tt.end, end)
			if start != nil {
				require.NotSame(t, tt.start, start)
				require.NotSame(t, tt.end, end)
			}
		})
	}
}
