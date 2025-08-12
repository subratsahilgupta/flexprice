package payload

import (
	"context"
	"encoding/json"
	"fmt"

	ierr "github.com/flexprice/flexprice/internal/errors"
	webhookDto "github.com/flexprice/flexprice/internal/webhook/dto"
)

type CommunicationPayloadBuilder struct {
	services *Services
}

func NewCommunicationPayloadBuilder(services *Services) PayloadBuilder {
	return &CommunicationPayloadBuilder{
		services: services,
	}
}

// BuildPayload builds the webhook payload for communication events
func (b *CommunicationPayloadBuilder) BuildPayload(ctx context.Context, eventType string, data json.RawMessage) (json.RawMessage, error) {
	var parsedPayload webhookDto.InternalCommunicationEvent

	err := json.Unmarshal(data, &parsedPayload)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Unable to unmarshal communication event payload").
			Mark(ierr.ErrInvalidOperation)
	}

	invoiceID, tenantID := parsedPayload.InvoiceID, parsedPayload.TenantID
	if invoiceID == "" || tenantID == "" {
		return nil, ierr.NewError("invalid data type for communication event").
			WithHint("Please provide a valid invoice ID and tenant ID").
			WithReportableDetails(map[string]any{
				"expected": "string",
				"got":      fmt.Sprintf("%T", data),
			}).
			Mark(ierr.ErrInvalidOperation)
	}

	// Get invoice details
	invoice, err := b.services.InvoiceService.GetInvoice(ctx, invoiceID)
	if err != nil {
		return nil, err
	}

	payload := webhookDto.NewCommunicationWebhookPayload(invoice)

	// Return the communication payload
	return json.Marshal(payload)
}
