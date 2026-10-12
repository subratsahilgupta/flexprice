package contact

import (
	"strings"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

// Person is a person at the customer; the one marked primary is the main contact.
type Person struct {
	ID          string `json:"id,omitempty"`
	IsPrimary   bool   `json:"is_primary,omitempty"`
	Salutation  string `json:"salutation,omitempty" validate:"omitempty,max=25"`
	FirstName   string `json:"first_name" validate:"required,max=100"`
	LastName    string `json:"last_name,omitempty" validate:"omitempty,max=100"`
	Email       string `json:"email,omitempty" validate:"omitempty,email"`
	Mobile      string `json:"mobile,omitempty" validate:"omitempty,max=20"`
	Designation string `json:"designation,omitempty" validate:"omitempty,max=100"`
	Department  string `json:"department,omitempty" validate:"omitempty,max=100"`
}

// ValidatePersons checks the list allows at most one primary and no repeated ids,
// and gives new entries an id.
func ValidatePersons(persons []*Person) error {
	seen := make(map[string]bool, len(persons))
	people := make(map[string]bool, len(persons))
	primaries := 0
	for _, p := range persons {
		person := strings.ToLower(p.Email + "|" + p.FirstName + "|" + p.LastName)
		if people[person] {
			return ierr.NewError("duplicate contact person").
				WithHint("Contact persons must differ in email or name").
				Mark(ierr.ErrValidation)
		}
		people[person] = true
		if p.IsPrimary {
			primaries++
		}
		if p.ID == "" {
			p.ID = types.GenerateUUIDWithPrefix(types.UUID_PREFIX_CONTACT_PERSON)
		}
		if seen[p.ID] {
			return ierr.NewError("duplicate contact person id").
				WithHintf("Contact person id %s appears more than once", p.ID).
				Mark(ierr.ErrValidation)
		}
		seen[p.ID] = true
	}
	if primaries > 1 {
		return ierr.NewError("more than one primary contact person").
			WithHint("Mark at most one contact person as primary").
			Mark(ierr.ErrValidation)
	}
	return nil
}
