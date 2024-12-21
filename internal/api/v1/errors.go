package v1

import "github.com/gin-gonic/gin"

// ErrorResponse represents the API error response structure
type ErrorResponse struct {
	Error  string `json:"error" example:"Invalid request payload"`
	Detail string `json:"detail" example:"Invalid request payload"`
}

func NewErrorResponse(c *gin.Context, code int, message string, err error) {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	c.AbortWithStatusJSON(code, ErrorResponse{
		Error:  message,
		Detail: detail,
	})
}
