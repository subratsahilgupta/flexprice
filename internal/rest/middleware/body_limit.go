package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MaxEventIngestionBodyBytes caps a single event-ingestion request body.
//
// Set deliberately high: bulk ingestion is a supported pattern and customers
// post large batches in normal operation, so this is a backstop against a
// single request consuming unbounded memory, not a throughput control.
// Sustained volume is bounded separately by the per-pipeline consumer
// throttles (see EventProcessing.RateLimit and friends).
const MaxEventIngestionBodyBytes = 32 << 20 // 32 MiB

// BodyLimitMiddleware caps the request body at limitBytes.
//
// MaxBytesReader enforces the limit while the body is read rather than
// buffering it first, so an oversized request is cut off mid-stream instead of
// being fully materialised. The handler's existing bind-error path surfaces the
// resulting *http.MaxBytesError as a validation error (HTTP 400).
func BodyLimitMiddleware(limitBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limitBytes)
		}
		c.Next()
	}
}
