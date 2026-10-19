package dto

import "github.com/flexprice/flexprice/internal/types"

// SelfBillingStatusResponse reports whether self-billing is on and the tenant's internal status.
type SelfBillingStatusResponse struct {
	Enabled bool                       `json:"enabled"`
	Status  types.TenantInternalStatus `json:"status,omitempty"`
}
