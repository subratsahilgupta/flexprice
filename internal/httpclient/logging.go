package httpclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/logger"
)

// maxLoggedBodyBytes caps each request/response body written to a log line. (4 KB)
const maxLoggedBodyBytes = 4 << 10
const redactedValue = "[redacted]"

// providerRequestIDHeaders are the response headers providers use to identify a request
// in their own logs, in lookup order.
var providerRequestIDHeaders = []string{
	"Request-Id",               // Stripe
	"X-Request-Id",             // generic
	"Intuit_tid",               // QuickBooks
	"X-Hubspot-Correlation-Id", // HubSpot
}

// sensitiveKeyParts redact any body field whose lowercased key contains one of them.
var sensitiveKeyParts = []string{
	"secret", "password", "token", "api_key", "apikey", "authorization",
	"private_key", "card_number", "cvc", "cvv",
}

// sensitiveKeys redact body fields whose lowercased key is exactly one of them. "number"
// is the card PAN in Moyasar and Stripe card params.
var sensitiveKeys = map[string]bool{"number": true, "pin": true}

var jsonStringOrNumberField = regexp.MustCompile(`"([^"\\]{1,64})"\s*:\s*("(?:[^"\\]|\\.)*"|-?\d[\d.eE+-]*)`)

// loggingTransport writes one structured log line per outbound call to a third-party
// provider, with the request and response bodies redacted and truncated.
type loggingTransport struct {
	base     http.RoundTripper
	logger   *logger.Logger
	provider string
}

// ProviderTransport instruments base for calls to a third-party provider: an OTel
// CLIENT span plus one log line per call. A nil log skips the logging layer.
func ProviderTransport(base http.RoundTripper, log *logger.Logger, provider string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if log == nil {
		return OtelTransport(base)
	}
	// Logging sits inside the OTel transport so its lines carry the CLIENT span id.
	return OtelTransport(&loggingTransport{base: base, logger: log, provider: provider})
}

// NewProviderHTTPClient is NewOtelHTTPClient with per-call provider logging.
func NewProviderHTTPClient(timeout time.Duration, log *logger.Logger, provider string) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		Transport:     ProviderTransport(nil, log, provider),
		CheckRedirect: RejectRedirects,
	}
}

// NewProviderClient is NewDefaultClient with per-call provider logging.
func NewProviderClient(log *logger.Logger, provider string) Client {
	return &DefaultClient{client: NewProviderHTTPClient(30*time.Second, log, provider)}
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	ctx := req.Context()
	durationMS := time.Since(start).Milliseconds()

	if err != nil {
		if errors.Is(err, context.Canceled) {
			t.logger.Info(ctx, "provider_call.canceled",
				"provider", t.provider,
				"http_method", req.Method,
				"http_host", req.URL.Host,
				"http_path", req.URL.Path,
				"duration_ms", durationMS,
			)
			return resp, err
		}
		t.logger.Error(ctx, "provider_call.failed",
			"error", err.Error(),
			"provider", t.provider,
			"http_method", req.Method,
			"http_host", req.URL.Host,
			"http_path", req.URL.Path,
			"duration_ms", durationMS,
			"request_body", requestBody(req),
		)
		return resp, err
	}

	requestID := providerRequestID(resp.Header)
	reqBody := requestBody(req)
	respBody := peekResponseBody(resp)
	if resp.StatusCode < http.StatusBadRequest {
		t.logger.Info(ctx, "provider_call.completed",
			"provider", t.provider,
			"http_method", req.Method,
			"http_host", req.URL.Host,
			"http_path", req.URL.Path,
			"status_code", resp.StatusCode,
			"duration_ms", durationMS,
			"provider_request_id", requestID,
			"request_body", reqBody,
			"response_body", respBody,
		)
		return resp, nil
	}

	if resp.StatusCode >= http.StatusInternalServerError {
		t.logger.Error(ctx, "provider_call.failed",
			"error", fmt.Sprintf("%s returned HTTP %d", t.provider, resp.StatusCode),
			"provider", t.provider,
			"http_method", req.Method,
			"http_host", req.URL.Host,
			"http_path", req.URL.Path,
			"status_code", resp.StatusCode,
			"duration_ms", durationMS,
			"provider_request_id", requestID,
			"request_body", reqBody,
			"response_body", respBody,
		)
		return resp, nil
	}

	// 4xx is logged at Info: many are expected outcomes (not found, declined) that the
	// caller handles, and Error would mark the span as an exception.
	t.logger.Info(ctx, "provider_call.rejected",
		"provider", t.provider,
		"http_method", req.Method,
		"http_host", req.URL.Host,
		"http_path", req.URL.Path,
		"status_code", resp.StatusCode,
		"duration_ms", durationMS,
		"provider_request_id", requestID,
		"retry_after", resp.Header.Get("Retry-After"),
		"request_body", reqBody,
		"response_body", respBody,
	)
	return resp, nil
}

func providerRequestID(h http.Header) string {
	for _, name := range providerRequestIDHeaders {
		if v := h.Get(name); v != "" {
			return v
		}
	}
	return ""
}

// requestBody re-reads the request body through GetBody, which net/http sets for
// in-memory bodies; streamed bodies are not logged.
func requestBody(req *http.Request) string {
	if req.GetBody == nil || req.ContentLength == 0 {
		return ""
	}
	if !isLoggableContent(req.Header) {
		return "[non-text body omitted]"
	}
	body, err := req.GetBody()
	if err != nil {
		return ""
	}
	defer body.Close()
	data, truncated := readCapped(body)
	return formatBody(data, truncated, req.Header.Get("Content-Type"))
}

// peekResponseBody reads the head of resp.Body for logging and puts it back so the
// caller still sees the complete body.
func peekResponseBody(resp *http.Response) string {
	if resp.Body == nil || resp.Body == http.NoBody {
		return ""
	}
	if !isLoggableContent(resp.Header) {
		return "[non-text body omitted]"
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxLoggedBodyBytes+1))
	resp.Body = &replayedBody{Reader: io.MultiReader(bytes.NewReader(data), resp.Body), Closer: resp.Body}
	logged, truncated := capBody(data)
	return formatBody(logged, truncated, resp.Header.Get("Content-Type"))
}

type replayedBody struct {
	io.Reader
	io.Closer
}

func readCapped(r io.Reader) ([]byte, bool) {
	data, _ := io.ReadAll(io.LimitReader(r, maxLoggedBodyBytes+1))
	return capBody(data)
}

func capBody(data []byte) ([]byte, bool) {
	if len(data) > maxLoggedBodyBytes {
		return data[:maxLoggedBodyBytes], true
	}
	return data, false
}

func isLoggableContent(h http.Header) bool {
	if enc := h.Get("Content-Encoding"); enc != "" && enc != "identity" {
		return false
	}
	ct := strings.ToLower(h.Get("Content-Type"))
	if ct == "" {
		return true
	}
	for _, textual := range []string{"json", "xml", "text/", "x-www-form-urlencoded"} {
		if strings.Contains(ct, textual) {
			return true
		}
	}
	return false
}

func formatBody(data []byte, truncated bool, contentType string) string {
	var body string
	if strings.Contains(strings.ToLower(contentType), "x-www-form-urlencoded") {
		body = redactForm(string(data))
	} else {
		body = redactJSON(string(data))
	}
	if truncated {
		body += "…[truncated]"
	}
	return body
}

// redactJSON replaces string and number values of sensitive keys. It works on
// truncated documents, which a JSON decoder would reject.
func redactJSON(body string) string {
	return jsonStringOrNumberField.ReplaceAllStringFunc(body, func(field string) string {
		key := jsonStringOrNumberField.FindStringSubmatch(field)[1]
		if !isSensitiveKey(key) {
			return field
		}
		return fmt.Sprintf("%q:%q", key, redactedValue)
	})
}

// redactForm handles form bodies, including bracketed keys such as card[number].
func redactForm(body string) string {
	pairs := strings.Split(body, "&")
	for i, pair := range pairs {
		rawKey, _, found := strings.Cut(pair, "=")
		if !found {
			continue
		}
		key, err := url.QueryUnescape(rawKey)
		if err != nil {
			key = rawKey
		}
		if open := strings.LastIndex(key, "["); open >= 0 {
			key = strings.TrimSuffix(key[open+1:], "]")
		}
		if isSensitiveKey(key) {
			pairs[i] = rawKey + "=" + redactedValue
		}
	}
	return strings.Join(pairs, "&")
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(key)
	if sensitiveKeys[key] {
		return true
	}
	for _, part := range sensitiveKeyParts {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}
