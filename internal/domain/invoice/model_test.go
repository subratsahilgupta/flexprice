package invoice

import (
	"testing"

	"github.com/flexprice/flexprice/ent"
)

// TestFromEnt_IsManuallyEdited verifies that the is_manually_edited lock flag
// round-trips from the ent entity into the domain model unchanged.
func TestFromEnt_IsManuallyEdited(t *testing.T) {
	tests := []struct {
		name             string
		isManuallyEdited bool
	}{
		{"true survives conversion", true},
		{"false survives conversion", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := &ent.Invoice{
				ID:               "inv_1",
				CustomerID:       "cust_1",
				Currency:         "usd",
				IsManuallyEdited: tc.isManuallyEdited,
			}

			got := FromEnt(e)

			if got.IsManuallyEdited != tc.isManuallyEdited {
				t.Fatalf("IsManuallyEdited = %v, want %v", got.IsManuallyEdited, tc.isManuallyEdited)
			}
		})
	}
}
