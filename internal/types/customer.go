package types

import (
	"fmt"
	"strings"

	ierr "github.com/flexprice/flexprice/internal/errors"
)

// ContactPerson is a person at the customer; the one marked primary is the main contact.
type ContactPerson struct {
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

// ValidateContactPersons checks the list allows at most one primary and no repeated ids,
// and gives new entries an id.
func ValidateContactPersons(persons []*ContactPerson) error {
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
			p.ID = GenerateUUIDWithPrefix(UUID_PREFIX_CONTACT_PERSON)
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

// CustomerFilter represents filters for customer queries
type CustomerFilter struct {
	*QueryFilter
	*TimeRangeFilter
	*MetadataFilter

	Filters     []*FilterCondition `json:"filters,omitempty" form:"filters" validate:"omitempty"`
	Sort        []*SortCondition   `json:"sort,omitempty" form:"sort" validate:"omitempty"`
	CustomerIDs []string           `json:"customer_ids,omitempty" form:"customer_ids" validate:"omitempty"`
	ExternalIDs []string           `json:"external_ids,omitempty" form:"external_ids" validate:"omitempty"`
	ExternalID  string             `json:"external_id,omitempty" form:"external_id" validate:"omitempty"`
	Email       string             `json:"email,omitempty" form:"email" validate:"omitempty,email"`
}

// NewCustomerFilter creates a new CustomerFilter with default values
func NewCustomerFilter() *CustomerFilter {
	return &CustomerFilter{
		QueryFilter: NewDefaultQueryFilter(),
	}
}

// NewNoLimitCustomerFilter creates a new CustomerFilter with no pagination limits
func NewNoLimitCustomerFilter() *CustomerFilter {
	return &CustomerFilter{
		QueryFilter: NewNoLimitQueryFilter(),
	}
}

// Validate validates the customer filter
func (f CustomerFilter) Validate() error {
	if f.QueryFilter != nil {
		if err := f.QueryFilter.Validate(); err != nil {
			return err
		}
	}

	if f.TimeRangeFilter != nil {
		if err := f.TimeRangeFilter.Validate(); err != nil {
			return err
		}
	}

	if f.MetadataFilter != nil {
		if err := f.MetadataFilter.Validate(); err != nil {
			return err
		}
	}

	if f.Email != "" && !IsValidEmail(f.Email) {
		return ierr.NewError("invalid email").
			WithHint("Please provide a valid email").
			Mark(ierr.ErrValidation)
	}

	return nil
}

// GetLimit implements BaseFilter interface
func (f *CustomerFilter) GetLimit() int {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetLimit()
	}
	return f.QueryFilter.GetLimit()
}

// GetOffset implements BaseFilter interface
func (f *CustomerFilter) GetOffset() int {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetOffset()
	}
	return f.QueryFilter.GetOffset()
}

// GetSort implements BaseFilter interface
func (f *CustomerFilter) GetSort() string {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetSort()
	}
	return f.QueryFilter.GetSort()
}

// GetOrder implements BaseFilter interface
func (f *CustomerFilter) GetOrder() string {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetOrder()
	}
	return f.QueryFilter.GetOrder()
}

// GetStatus implements BaseFilter interface.
// An unset query status defaults to published so internal lists hide archived customers.
// Search requests send a filters array (possibly empty); return empty then so
// ApplyStatusFilter includes published+archived and DSL can narrow further.
func (f *CustomerFilter) GetStatus() string {
	if f == nil {
		return string(StatusPublished)
	}
	if f.QueryFilter != nil && f.QueryFilter.Status != nil {
		if status := string(*f.QueryFilter.Status); status != "" {
			return status
		}
	}
	if f.Filters != nil {
		return ""
	}
	return string(StatusPublished)
}

// GetExpand implements BaseFilter interface
func (f *CustomerFilter) GetExpand() Expand {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetExpand()
	}
	return f.QueryFilter.GetExpand()
}

func (f *CustomerFilter) IsUnlimited() bool {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().IsUnlimited()
	}
	return f.QueryFilter.IsUnlimited()
}

// Common validation rules for IDs
func validateID(id string, idType string) error {

	// Sample Constraints for customer id and external customer id
	// 1. Cannot contain invalid characters % and space

	invalidChars := []string{"%", " "}
	for _, char := range invalidChars {
		if strings.Contains(id, char) {
			return ierr.NewError(fmt.Sprintf("invalid %s", idType)).
				WithHint(fmt.Sprintf("Please provide a valid %s - cannot contain: %s", idType, char)).
				Mark(ierr.ErrValidation)
		}
	}

	return nil
}

// ValidateCustomerID validates the customer id
func ValidateCustomerID(id string) error {

	if strings.HasPrefix(id, "_") || strings.HasSuffix(id, "_") {
		return ierr.NewError("invalid customer id").
			WithHint("Please provide a valid customer id").
			Mark(ierr.ErrValidation)
	}

	return validateID(id, "customer id")
}

// ValidateExternalCustomerID validates the external customer id
func ValidateExternalCustomerID(id string) error {
	return validateID(id, "external customer id")
}
