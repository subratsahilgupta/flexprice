package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flexprice/flexprice/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func newObservedLogger() (*logger.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	return logger.NewFromSugared(zap.New(core).Sugar()), logs
}

func TestProviderTransport_LogsOneLinePerCall(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		respBody     string
		respHeaders  map[string]string
		reqBody      string
		reqType      string
		wantLevel    zapcore.Level
		wantMessage  string
		wantRespBody string
		wantReqBody  string
	}{
		{
			name:         "success logs status, request id and bodies",
			status:       http.StatusOK,
			respBody:     `{"id":"cus_1"}`,
			respHeaders:  map[string]string{"Request-Id": "req_123", "Content-Type": "application/json"},
			reqBody:      `{"email":"a@b.com"}`,
			reqType:      "application/json",
			wantLevel:    zapcore.InfoLevel,
			wantMessage:  "provider_call.completed",
			wantRespBody: `{"id":"cus_1"}`,
			wantReqBody:  `{"email":"a@b.com"}`,
		},
		{
			name:         "4xx logs redacted bodies at info",
			status:       http.StatusPaymentRequired,
			respBody:     `{"error":{"code":"card_declined","decline_code":"insufficient_funds"}}`,
			respHeaders:  map[string]string{"Request-Id": "req_456", "Content-Type": "application/json"},
			reqBody:      "amount=100&card%5Bnumber%5D=4242424242424242&card%5Bcvc%5D=123",
			reqType:      "application/x-www-form-urlencoded",
			wantLevel:    zapcore.InfoLevel,
			wantMessage:  "provider_call.rejected",
			wantRespBody: `{"error":{"code":"card_declined","decline_code":"insufficient_funds"}}`,
			wantReqBody:  "amount=100&card%5Bnumber%5D=[redacted]&card%5Bcvc%5D=[redacted]",
		},
		{
			name:         "5xx logs at error",
			status:       http.StatusBadGateway,
			respBody:     `{"message":"upstream down"}`,
			respHeaders:  map[string]string{"Content-Type": "application/json"},
			reqBody:      `{"client_secret":"shh","amount":5}`,
			reqType:      "application/json",
			wantLevel:    zapcore.ErrorLevel,
			wantMessage:  "provider_call.failed",
			wantRespBody: `{"message":"upstream down"}`,
			wantReqBody:  `{"client_secret":"[redacted]","amount":5}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tt.respHeaders {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.respBody)
			}))
			defer srv.Close()

			log, logs := newObservedLogger()
			client := NewProviderHTTPClient(0, log, "stripe")

			req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/charges?email=a@b.com", strings.NewReader(tt.reqBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", tt.reqType)

			resp, err := client.Do(req)
			require.NoError(t, err)
			got, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			assert.Equal(t, tt.respBody, string(got), "caller must still receive the full body")

			require.Equal(t, 1, logs.Len())
			entry := logs.All()[0]
			assert.Equal(t, tt.wantLevel, entry.Level)
			assert.Equal(t, tt.wantMessage, entry.Message)

			fields := entry.ContextMap()
			assert.Equal(t, "stripe", fields["provider"])
			assert.Equal(t, "/v1/charges", fields["http_path"], "query string must not be logged")
			assert.EqualValues(t, tt.status, fields["status_code"])
			assert.Equal(t, tt.respHeaders["Request-Id"], fields["provider_request_id"])
			assert.Equal(t, tt.wantRespBody, fields["response_body"])
			assert.Equal(t, tt.wantReqBody, fields["request_body"])
		})
	}
}

type failingTransport struct{ err error }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

func TestProviderTransport_LogsTransportErrors(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantLevel   zapcore.Level
		wantMessage string
	}{
		{name: "network error", err: errors.New("connection refused"), wantLevel: zapcore.ErrorLevel, wantMessage: "provider_call.failed"},
		{name: "caller canceled", err: fmt.Errorf("dial: %w", context.Canceled), wantLevel: zapcore.InfoLevel, wantMessage: "provider_call.canceled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, logs := newObservedLogger()
			client := &http.Client{Transport: ProviderTransport(failingTransport{err: tt.err}, log, "razorpay")}

			req, err := http.NewRequest(http.MethodGet, "https://api.razorpay.com/v1/payments", nil)
			require.NoError(t, err)
			_, err = client.Do(req)
			require.Error(t, err)

			require.Equal(t, 1, logs.Len())
			assert.Equal(t, tt.wantLevel, logs.All()[0].Level)
			assert.Equal(t, tt.wantMessage, logs.All()[0].Message)
		})
	}
}

func TestProviderTransport_TruncatesLargeBodies(t *testing.T) {
	large := strings.Repeat("x", maxLoggedBodyBytes*2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, large)
	}))
	defer srv.Close()

	log, logs := newObservedLogger()
	resp, err := NewProviderHTTPClient(0, log, "zoho_books").Get(srv.URL)
	require.NoError(t, err)
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Len(t, got, len(large))
	logged := logs.All()[0].ContextMap()["response_body"].(string)
	assert.True(t, strings.HasSuffix(logged, "…[truncated]"))
	assert.Len(t, strings.TrimSuffix(logged, "…[truncated]"), maxLoggedBodyBytes)
}

func TestProviderTransport_NilLoggerOnlyTraces(t *testing.T) {
	base := failingTransport{err: errors.New("boom")}
	_, isLogging := ProviderTransport(base, nil, "stripe").(*loggingTransport)
	assert.False(t, isLogging)
}

func TestRedactJSON(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "nested card fields",
			in:   `{"source":{"type":"creditcard","number":"4111111111111111","cvc":"123","month":12}}`,
			want: `{"source":{"type":"creditcard","number":"[redacted]","cvc":"[redacted]","month":12}}`,
		},
		{
			name: "token-like keys",
			in:   `{"access_token":"abc","refresh_token":"def","api_key":"k","name":"Acme"}`,
			want: `{"access_token":"[redacted]","refresh_token":"[redacted]","api_key":"[redacted]","name":"Acme"}`,
		},
		{
			name: "truncated document still redacted",
			in:   `{"error":"bad","client_secret":"pi_123_secret_456","items":[{"na`,
			want: `{"error":"bad","client_secret":"[redacted]","items":[{"na`,
		},
		{
			name: "error codes are kept",
			in:   `{"code":"card_declined","message":"Your card was declined."}`,
			want: `{"code":"card_declined","message":"Your card was declined."}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, redactJSON(tt.in))
		})
	}
}

type ctxKey struct{}

type recordingTransport struct{ got context.Context }

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.got = req.Context()
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
}

func TestContextTransport(t *testing.T) {
	bound := context.WithValue(context.Background(), ctxKey{}, "bound")
	own := context.WithValue(context.Background(), ctxKey{}, "own")

	tests := []struct {
		name   string
		reqCtx context.Context
		want   string
	}{
		{name: "request without context gets the bound one", reqCtx: context.Background(), want: "bound"},
		{name: "request with its own context keeps it", reqCtx: own, want: "own"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordingTransport{}
			req, err := http.NewRequestWithContext(tt.reqCtx, http.MethodGet, "https://api.razorpay.com/v1/orders", nil)
			require.NoError(t, err)

			_, err = ContextTransport(bound, rec).RoundTrip(req)
			require.NoError(t, err)
			assert.Equal(t, tt.want, rec.got.Value(ctxKey{}))
		})
	}
}
