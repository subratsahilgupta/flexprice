package dto

import ierr "github.com/flexprice/flexprice/internal/errors"

// SuccessResponse represents a generic success response
type SuccessResponse struct {
	Message string `json:"message"`
}

// Address represents a physical address
type Address struct {
	Line1      string `json:"address_line1" validate:"omitempty,max=255"`
	Line2      string `json:"address_line2" validate:"omitempty,max=255"`
	City       string `json:"address_city" validate:"omitempty,max=100"`
	State      string `json:"address_state" validate:"omitempty,max=100"`
	PostalCode string `json:"address_postal_code" validate:"omitempty,max=20"`
	Country    string `json:"address_country" validate:"omitempty,len=2,iso3166_1_alpha2"`
}

// ValidateAddressCountry validates the country code format
func ValidateAddressCountry(country string) bool {
	if country == "" {
		return true
	}
	// Check if country code is exactly 2 characters
	if len(country) != 2 {
		return false
	}
	// TODO: Add validation against ISO 3166-1 alpha-2 codes
	return true
}

// ValidateAddress validates all address fields
func ValidateAddress(address Address) error {
	if !ValidateAddressCountry(address.Country) {
		return ierr.NewError("invalid country code format").
			WithHint("Country code must be 2 characters").
			Mark(ierr.ErrValidation)
	}

	// Validate field lengths
	if len(address.Line1) > 255 {
		return ierr.NewError("address line 1 too long").
			WithHint("Address line 1 must be less than 255 characters").
			Mark(ierr.ErrValidation)
	}
	if len(address.Line2) > 255 {
		return ierr.NewError("address line 2 too long").
			WithHint("Address line 2 must be less than 255 characters").
			Mark(ierr.ErrValidation)
	}
	if len(address.City) > 100 {
		return ierr.NewError("city name too long").
			WithHint("City name must be less than 100 characters").
			Mark(ierr.ErrValidation)
	}
	if len(address.State) > 100 {
		return ierr.NewError("state name too long").
			WithHint("State name must be less than 100 characters").
			Mark(ierr.ErrValidation)
	}
	if len(address.PostalCode) > 20 {
		return ierr.NewError("postal code too long").
			WithHint("Postal code must be less than 20 characters").
			Mark(ierr.ErrValidation)
	}
	return nil
}
