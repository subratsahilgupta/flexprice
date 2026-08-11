package middleware

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestBodyLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const limit = 64

	tests := []struct {
		name          string
		body          string
		wantTooLarge  bool
		wantBytesRead int
	}{
		{
			name:          "body under the limit is read in full",
			body:          strings.Repeat("a", limit-1),
			wantTooLarge:  false,
			wantBytesRead: limit - 1,
		},
		{
			name:          "body exactly at the limit is allowed",
			body:          strings.Repeat("a", limit),
			wantTooLarge:  false,
			wantBytesRead: limit,
		},
		{
			name:         "body over the limit is rejected",
			body:         strings.Repeat("a", limit+1),
			wantTooLarge: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var readErr error
			var n int

			router := gin.New()
			router.POST("/", BodyLimitMiddleware(limit), func(c *gin.Context) {
				b, err := io.ReadAll(c.Request.Body)
				readErr = err
				n = len(b)
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(tt.body))
			router.ServeHTTP(httptest.NewRecorder(), req)

			if tt.wantTooLarge {
				var maxErr *http.MaxBytesError
				assert.True(t, errors.As(readErr, &maxErr),
					"expected *http.MaxBytesError, got %v", readErr)
				return
			}

			assert.NoError(t, readErr)
			assert.Equal(t, tt.wantBytesRead, n)
		})
	}
}

// A nil body must not panic — GET-style requests reach the middleware too.
func TestBodyLimitMiddleware_NilBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/", BodyLimitMiddleware(64), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Body = nil
	w := httptest.NewRecorder()

	assert.NotPanics(t, func() { router.ServeHTTP(w, req) })
	assert.Equal(t, http.StatusOK, w.Code)
}
