package tenant

import (
	"time"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/types"
)

// Tenant represents an organization or group within the system.
type Tenant struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Status    types.Status `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// FromEnt converts an ent Tenant to a domain Tenant
func FromEnt(e *ent.Tenant) *Tenant {
	if e == nil {
		return nil
	}

	return &Tenant{
		ID:        e.ID,
		Name:      e.Name,
		Status:    types.Status(e.Status),
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}

// FromEntList converts a list of ent Tenants to domain Tenants
func FromEntList(tenants []*ent.Tenant) []*Tenant {
	if tenants == nil {
		return nil
	}

	result := make([]*Tenant, len(tenants))
	for i, t := range tenants {
		result[i] = FromEnt(t)
	}

	return result
}
