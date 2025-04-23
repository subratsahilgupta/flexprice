package postgres

import (
	"context"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/logger"
	sentryService "github.com/flexprice/flexprice/internal/sentry"
)

// SentryClient wraps the standard postgres client with Sentry monitoring
type SentryClient struct {
	client IClient
	sentry *sentryService.Service
	logger *logger.Logger
}

// NewSentryClient creates a new Sentry-instrumented Postgres client
func NewSentryClient(client IClient, sentry *sentryService.Service, logger *logger.Logger) IClient {
	return &SentryClient{
		client: client,
		sentry: sentry,
		logger: logger,
	}
}

// WithTx wraps the given function in a transaction with Sentry span tracking
func (c *SentryClient) WithTx(ctx context.Context, fn func(context.Context) error) error {
	span, spanCtx := c.sentry.StartDBSpan(ctx, "postgres.transaction", map[string]interface{}{
		"operation": "transaction",
	})
	defer span.Finish()

	// Use the original client's WithTx but with the new span context
	return c.client.WithTx(spanCtx, fn)
}

// TxFromContext returns the transaction from context if it exists
func (c *SentryClient) TxFromContext(ctx context.Context) *ent.Tx {
	return c.client.TxFromContext(ctx)
}

// Querier returns the current transaction client if in a transaction, or the regular client
// This method wraps the client with span tracking
func (c *SentryClient) Querier(ctx context.Context) *ent.Client {
	// Start a span for this database operation
	operation := getOperationNameFromStack()
	span, spanCtx := c.sentry.StartDBSpan(ctx, operation, map[string]interface{}{
		"client_type": "ent",
	})

	// For most operations, the span will be short-lived as we're just getting the client
	// Transaction-level spans are handled in WithTx
	// Repository operations should ideally create their own spans for complex operations
	span.Finish()

	// Return the client from the original implementation
	return c.client.Querier(spanCtx)
}

// getOperationNameFromStack attempts to extract a meaningful operation name from the stack
// This is a best-effort function that tries to determine what repository operation is being performed
func getOperationNameFromStack() string {
	// Default operation name
	return "postgres.query"
}
