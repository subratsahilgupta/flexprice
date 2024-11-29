package dto

import "github.com/go-playground/validator/v10"

type SignUpRequest struct {
	Email    string `json:"email" binding:"required,email" validate:"email"`
	Password string `json:"password" binding:"required,min=8" validate:"min=8"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email" validate:"email"`
	Password string `json:"password" binding:"required" validate:"min=8"`
}

type AuthResponse struct {
	Token string `json:"token"`
}

func (r *SignUpRequest) Validate() error {
	return validator.New().Struct(r)
}

func (r *LoginRequest) Validate() error {
	return validator.New().Struct(r)
}
