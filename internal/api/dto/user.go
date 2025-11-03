package dto

import (
	"github.com/flexprice/flexprice/internal/domain/tenant"
	"github.com/flexprice/flexprice/internal/domain/user"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/validator"
)

// CreateUserRequest represents the request to create a new user (service accounts only)
type CreateUserRequest struct {
	Type  string   `json:"type" binding:"required" validate:"required"`              // Must be "service_account"
	Roles []string `json:"roles" binding:"required,min=1" validate:"required,min=1"` // Roles are required
}

func (r *CreateUserRequest) Validate() error {
	if err := validator.ValidateRequest(r); err != nil {
		return err
	}

	// Only service accounts can be created via API
	if r.Type != string(user.UserTypeServiceAccount) {
		return ierr.NewError("only service accounts can be created via this endpoint").
			WithHint("Regular user accounts cannot be created via API. Use type='service_account'").
			Mark(ierr.ErrValidation)
	}

	// Service accounts MUST have roles (already validated by binding tag, but double-check)
	if len(r.Roles) == 0 {
		return ierr.NewError("service accounts must have at least one role").
			WithHint("Service accounts require role assignment").
			Mark(ierr.ErrValidation)
	}

	return nil
}

// ValidateNoExtraFields checks if the raw JSON contains any fields beyond type and roles
func (r *CreateUserRequest) ValidateNoExtraFields(rawJSON map[string]interface{}) error {
	allowedFields := map[string]bool{
		"type":  true,
		"roles": true,
	}

	for field := range rawJSON {
		if !allowedFields[field] {
			return ierr.NewError("unexpected field in request").
				WithHint("Only 'type' and 'roles' fields are allowed for service account creation").
				WithReportableDetails(map[string]interface{}{
					"invalid_field": field,
				}).
				Mark(ierr.ErrValidation)
		}
	}

	return nil
}

type UserResponse struct {
	ID     string          `json:"id"`
	Email  string          `json:"email,omitempty"` // Empty for service accounts
	Type   string          `json:"type"`
	Roles  []string        `json:"roles,omitempty"`
	Tenant *TenantResponse `json:"tenant"`
}

func NewUserResponse(user *user.User, tenant *tenant.Tenant) *UserResponse {
	return &UserResponse{
		ID:     user.ID,
		Email:  user.Email,
		Type:   user.Type,
		Roles:  user.Roles,
		Tenant: NewTenantResponse(tenant),
	}
}

// ListUsersResponse is the response type for listing users with pagination
type ListUsersResponse = types.ListResponse[*UserResponse]
