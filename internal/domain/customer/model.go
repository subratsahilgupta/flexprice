package customer

import "github.com/flexprice/flexprice/internal/types"

type Customer struct {
	ID string `db:"id" json:"id"`

	ExternalID string `db:"external_id" json:"external_id"`

	Name string `db:"name" json:"name"`

	Email string `db:"email" json:"email"`

	types.BaseModel
}
