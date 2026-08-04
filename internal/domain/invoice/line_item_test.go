package invoice

import (
	"testing"

	"github.com/flexprice/flexprice/ent"
	"github.com/samber/lo"
)

// TestFromEnt_ParentLineItemID verifies that the parent_line_item_id lineage
// pointer round-trips from the ent entity into the domain model unchanged,
// both when unset (the common case) and when set (a line item created by
// editing an existing one).
func TestFromEnt_ParentLineItemID(t *testing.T) {
	tests := []struct {
		name             string
		parentLineItemID *string
	}{
		{"nil for a line item that was never edited", nil},
		{"set for a line item created by editing an existing one", lo.ToPtr("li_original")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := &ent.InvoiceLineItem{
				ID:               "li_1",
				InvoiceID:        "inv_1",
				CustomerID:       "cust_1",
				Currency:         "usd",
				ParentLineItemID: tc.parentLineItemID,
			}

			got := new(InvoiceLineItem).FromEnt(e)

			if tc.parentLineItemID == nil {
				if got.ParentLineItemID != nil {
					t.Fatalf("ParentLineItemID = %v, want nil", got.ParentLineItemID)
				}
				return
			}

			if got.ParentLineItemID == nil || *got.ParentLineItemID != *tc.parentLineItemID {
				t.Fatalf("ParentLineItemID = %v, want %v", got.ParentLineItemID, *tc.parentLineItemID)
			}
		})
	}
}
