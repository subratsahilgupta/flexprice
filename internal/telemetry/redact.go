package telemetry

import "encoding/json"

// redactedFields lists JSON keys stripped from every system_event payload
// before it leaves the process, regardless of event type. Webhook payload
// shapes vary per event and nest arbitrarily (Customer inside Invoice inside
// Subscription, etc), so this walks the parsed JSON generically by key name
// rather than hardcoding a shape per event type. New sensitive fields on any
// webhook DTO are covered automatically as long as the key name matches.
var redactedFields = map[string]bool{
	// customer PII
	"name":                true,
	"email":               true,
	"address_line1":       true,
	"address_line2":       true,
	"address_city":        true,
	"address_state":       true,
	"address_postal_code": true,
	"address_country":     true,
	"phone":               true,
	"metadata":            true,

	// financial amounts and balances
	"amount":                        true,
	"amount_due":                    true,
	"amount_paid":                   true,
	"amount_remaining":              true,
	"subtotal":                      true,
	"total":                         true,
	"total_tax":                     true,
	"total_discount":                true,
	"total_prepaid_credits_applied": true,
	"settled_amount":                true,
	"total_amount":                  true,
	"balance":                       true,
	"credit_balance":                true,
	"credit_balance_before":         true,
	"credit_balance_after":          true,
	"credit_amount":                 true,
	"current_balance":               true,
	"current_spend":                 true,
	"threshold":                     true,
	"conversion_rate":               true,
	"currency":                      true,

	// payment/gateway details and freeform event properties
	"payment_gateway":    true,
	"gateway_refund_id":  true,
	"hosted_invoice_url": true,
	"properties":         true,
}

// redactPayload walks a parsed JSON value and removes any object key present
// in redactedFields, at any nesting depth, in place.
func redactPayload(v any) {
	switch val := v.(type) {
	case map[string]any:
		for k := range val {
			if redactedFields[k] {
				delete(val, k)
				continue
			}
			redactPayload(val[k])
		}
	case []any:
		for _, item := range val {
			redactPayload(item)
		}
	}
}

// redactEventPayload returns a copy of raw with sensitive fields stripped.
// Non-object payloads and invalid JSON pass through unchanged, since this
// only needs to strip known object keys, not validate shape.
func redactEventPayload(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return raw
	}
	redactPayload(parsed)
	b, err := json.Marshal(parsed)
	if err != nil {
		return raw
	}
	return json.RawMessage(b)
}
