package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A redirect must never be followed: the redirect target has not been through
// the caller's URL validation (VAPT SFX-2026-0203-F16).
func TestSendDoesNotFollowRedirects(t *testing.T) {
	var internalHit bool
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		internalHit = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("secret-metadata"))
	}))
	defer internal.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL, http.StatusFound)
	}))
	defer redirector.Close()

	client := NewClientWithConfig(ClientConfig{})
	resp, err := client.Send(context.Background(), &Request{
		Method: http.MethodGet,
		URL:    redirector.URL,
	})

	if err == nil {
		t.Fatalf("expected an error for a redirect response, got resp=%+v", resp)
	}
	if internalHit {
		t.Fatal("redirect was followed: the internal target received a request")
	}
}

// Non-redirect responses must still work normally.
func TestSendFollowsNoRedirectHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := NewClientWithConfig(ClientConfig{})
	resp, err := client.Send(context.Background(), &Request{
		Method: http.MethodGet,
		URL:    srv.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK || string(resp.Body) != "ok" {
		t.Fatalf("unexpected response: %d %q", resp.StatusCode, resp.Body)
	}
}
