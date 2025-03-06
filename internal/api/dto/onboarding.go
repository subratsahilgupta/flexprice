package dto

import (
	"time"

	"github.com/go-playground/validator/v10"
)

// OnboardingEventsRequest represents the request to generate events for onboarding
type OnboardingEventsRequest struct {
	CustomerID string `json:"customer_id" binding:"required" validate:"required" example:"cus_01HKG8QWERTY123"`
	FeatureID  string `json:"feature_id" binding:"required" validate:"required" example:"feat_01HKG8QWERTY123"`
	Duration   int    `json:"duration"  validate:"omitempty,min=1,max=600" default:"60" example:"60"`
}

// Validate validates the OnboardingEventsRequest
func (r *OnboardingEventsRequest) Validate() error {
	return validator.New().Struct(r)
}

// OnboardingEventsResponse represents the response for onboarding events generation
type OnboardingEventsResponse struct {
	Message   string    `json:"message"`
	StartedAt time.Time `json:"started_at"`
	Duration  int       `json:"duration"`
	Count     int       `json:"count"`
}
