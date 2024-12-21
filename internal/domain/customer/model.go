package customer

import "github.com/flexprice/flexprice/internal/types"

type Customer struct {
	// ID is the unique identifier for the customer
	ID string `db:"id" json:"id"`

	// ExternalID is the external identifier for the customer
	ExternalID string `db:"external_id" json:"external_id"`

	// Name is the name of the customer
	Name string `db:"name" json:"name"`

	// Email is the email of the customer
	Email string `db:"email" json:"email"`

	types.BaseModel
}
