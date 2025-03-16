package tenant

import (
	"encoding/json"
	"time"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/types"
)

// Tenant represents an organization or group within the system.
type Tenant struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Status         types.Status   `json:"status"`
	BillingDetails BillingDetails `json:"billing_details"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// Address represents a physical address
type Address struct {
	Line1      string `json:"address_line1"`
	Line2      string `json:"address_line2"`
	City       string `json:"address_city"`
	State      string `json:"address_state"`
	PostalCode string `json:"address_postal_code"`
	Country    string `json:"address_country"`
}

// BillingDetails contains tenant billing information
type BillingDetails struct {
	Email     string  `json:"email"`
	HelpEmail string  `json:"help_email"`
	Phone     string  `json:"phone"`
	Address   Address `json:"address"`
}

func (b *BillingDetails) ToMap() map[string]interface{} {
	bytes, err := json.Marshal(b)
	if err != nil {
		return nil
	}

	var m map[string]interface{}
	err = json.Unmarshal(bytes, &m)
	if err != nil {
		return m // return empty map if error
	}
	return m
}

// FromEnt converts an ent Tenant to a domain Tenant
func FromEnt(e *ent.Tenant) *Tenant {
	if e == nil {
		return nil
	}

	billingDetails := BillingDetails{}
	if e.BillingDetails != nil {
		// marshal the billing details
		bytes, err := json.Marshal(e.BillingDetails)
		if err != nil {
			return nil
		}

		err = json.Unmarshal(bytes, &billingDetails)
		if err != nil {
			return nil
		}
	}

	return &Tenant{
		ID:             e.ID,
		Name:           e.Name,
		Status:         types.Status(e.Status),
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
		BillingDetails: billingDetails,
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
