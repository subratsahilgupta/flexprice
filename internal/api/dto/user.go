package dto

import (
	"github.com/flexprice/flexprice/internal/domain/user"
)

type UserResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func NewUserResponse(user *user.User) *UserResponse {
	return &UserResponse{
		ID:    user.ID,
		Email: user.Email,
	}
}
