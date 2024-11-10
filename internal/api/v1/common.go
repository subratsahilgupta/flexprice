package v1

// ErrorResponse represents the API error response structure
type ErrorResponse struct {
    Error string `json:"error" example:"Invalid request payload"`
} 