package utils

import "strings"

// redactedPlaceholder is what a removed secret is replaced with, so a redacted message still shows
// that something was removed rather than silently reading as if the field were absent.
const redactedPlaceholder = "[redacted]"

// RedactSecrets removes each non-empty secret from msg, leaving everything else intact.
//
// It exists for provider error messages: AWS embeds the role ARN in AccessDenied text and echoes the
// external ID when a trust-policy condition fails, and GCP embeds the workload identity audience and
// service account it tried to impersonate. Those identify the tenant's own resources and must not
// reach logs or API responses — but the rest of the message (status code, request id, which
// principal was denied, which action was refused, why) is exactly what makes a failure diagnosable,
// so only the secrets are removed and the message is otherwise passed through verbatim.
func RedactSecrets(msg string, secrets ...string) string {
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, secret, redactedPlaceholder)
	}
	return msg
}
