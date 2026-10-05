package httpclient

import (
	"bytes"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

const (
	maxLoggedBodyBytes = 4 << 10
	maxParsedBodyBytes = 64 << 10

	redactedValue = "[redacted]"
)

// sensitiveKeyParts redact any field whose normalized key contains one of them.
var sensitiveKeyParts = []string{
	"secret", "password", "passwd", "passphrase", "token", "apikey", "authorization",
	"privatekey", "credential", "cookie", "signature",
	"cvc", "cvv", "securitycode", "expiry", "cardnumber", "cardholder", "nameoncard", "accountnumber",
	"routingnumber", "iban", "sortcode", "bsb", "ifsc", "vpa", "upi", "bankaccount",
	"email", "phone", "mobile", "firstname", "lastname", "birth", "passport", "aadhaar", "ssn",
	"ipaddress", "beneficiary", "purchaser",
	"addr", "shipping", "postal", "postcode", "zipcode", "street", "billingdetails",
	"vat", "taxid", "taxinfo", "gstin", "gstno",
	"description", "note", "memo", "comment", "remark", "metadata", "customfield",
	// links that open invoices or payment pages
	"url", "pdf", "link",
}

// sensitiveKeys match exactly: "number" is a card PAN, "month"/"year" its expiry, "contact"
// a Razorpay phone, "value" a custom field's value.
var sensitiveKeys = map[string]bool{
	"number": true, "pin": true, "otp": true, "contact": true, "company": true, "value": true,
	"ip": true, "pan": true, "dob": true, "city": true, "zip": true, "line1": true,
	"line2": true, "line3": true, "objectid": true, "gst": true,
	"month": true, "year": true, "expmonth": true, "expyear": true,
}

var (
	emailPattern  = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	cardPattern   = regexp.MustCompile(`\b\d(?:[ -]?\d){12,18}\b`)
	secretPattern = regexp.MustCompile(`\b(?:sk|rk)_(?:live|test)_[A-Za-z0-9]{8,}|\bwhsec_[A-Za-z0-9+/=]{8,}|\b(?:Bearer|Basic)\s+[A-Za-z0-9._~+/=-]{8,}`)
)

// redactor redacts one call's bodies and records fields that unexpectedly held PII.
type redactor struct {
	detected []string
}

// body logs only JSON and form bodies, redacting before truncating so a cut never leaks.
func (r *redactor) body(data []byte, contentType, side string) string {
	if len(data) > maxParsedBodyBytes {
		return "[body omitted: larger than 64 KB]"
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return ""
	}

	var rendered string
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "json"):
		var doc any
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&doc); err != nil {
			return "[unparseable JSON body omitted]"
		}
		out, err := json.Marshal(r.value(doc, side))
		if err != nil {
			return "[body omitted: failed to encode]"
		}
		rendered = string(out)
	case strings.Contains(ct, "x-www-form-urlencoded"):
		values, err := url.ParseQuery(string(data))
		if err != nil {
			return "[unparseable form body omitted]"
		}
		rendered = r.form(values, side)
	default:
		return "[non-JSON body omitted]"
	}

	if len(rendered) > maxLoggedBodyBytes {
		rendered = strings.ToValidUTF8(rendered[:maxLoggedBodyBytes], "") + "…[truncated]"
	}

	return rendered
}

func (r *redactor) value(value any, path string) any {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if isSensitiveKey(key) {
				v[key] = redactedValue
				continue
			}
			v[key] = r.value(child, path+"."+key)
		}
	case []any:
		for i := range v {
			v[i] = r.value(v[i], path+"[]")
		}
	case string:
		return r.text(v, path)
	}
	return value
}

// form treats customer[billing_address][line1] as sensitive if any segment is.
func (r *redactor) form(values url.Values, side string) string {
	for key, list := range values {
		if isSensitiveFormKey(key) {
			values[key] = []string{redactedValue}
			continue
		}
		for i, item := range list {
			list[i] = r.text(item, side+"."+key)
		}
	}
	return values.Encode()
}

// text masks emails, card numbers and API keys, recording the field for the pii_detected
// alert so its key can be added to the list.
func (r *redactor) text(value, path string) string {
	if trimmed := strings.TrimSpace(value); strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return redactedValue
	}

	masked := secretPattern.ReplaceAllString(value, "[secret]")
	masked = emailPattern.ReplaceAllString(masked, "[email]")
	masked = cardPattern.ReplaceAllStringFunc(masked, func(match string) string {
		if isLuhnValid(match) {
			return "[card]"
		}
		return match
	})
	if masked != value && len(r.detected) < 20 {
		r.detected = append(r.detected, strings.TrimPrefix(path, "."))
	}
	return masked
}

func isSensitiveFormKey(key string) bool {
	segments := strings.FieldsFunc(key, func(c rune) bool { return c == '[' || c == ']' })
	for _, segment := range segments {
		// OAuth authorization code; JSON keeps "code" for provider error codes.
		if isSensitiveKey(segment) || normalizeKey(segment) == "code" {
			return true
		}
	}
	return false
}

func isSensitiveKey(key string) bool {
	normalized := normalizeKey(key)
	if sensitiveKeys[normalized] || strings.HasSuffix(normalized, "name") {
		return true
	}
	for _, part := range sensitiveKeyParts {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return false
}

// normalizeKey makes card_number, cardNumber and card-number equal.
func normalizeKey(key string) string {
	return strings.Map(func(c rune) rune {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			return c
		}
		return -1
	}, strings.ToLower(key))
}

func isLuhnValid(candidate string) bool {
	digits := strings.NewReplacer(" ", "", "-", "").Replace(candidate)
	sum := 0
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if (len(digits)-i)%2 == 0 {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return sum%10 == 0
}
