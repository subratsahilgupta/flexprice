package selfbilling

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	cockroach "github.com/cockroachdb/errors"
	"github.com/flexprice/flexprice/internal/config"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	sdkerrors "github.com/flexprice/go-sdk/v2/models/errors"
	sdktypes "github.com/flexprice/go-sdk/v2/models/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	pathCustomers            = "/customers"           // as CreateCustomer / UpdateCustomer build it
	pathCustomerByExternalID = "/customers/external/" // as GetCustomerByExternalID builds it
	pathSubscriptionsSuffix  = "/subscriptions"       // as GetSubscriptionsForCustomer appends it
	pathSubscriptions        = "/subscriptions"       // as CreateSubscription builds it
)

// notFoundBody is the JSON error envelope the Flexprice API returns for a 404.
const notFoundBody = `{"code":"not_found","message":"customer not found","http_status_code":404}`

type cannedResponse struct {
	status int
	body   string
}

// fakeCloud serves just the Flexprice Cloud endpoints the remote client touches.
type fakeCloud struct {
	mu sync.Mutex

	byExtID map[string]string           // external_id -> customer id
	subs    map[string][]map[string]any // external_id -> subscriptions
	failAll *cannedResponse             // when set, every request gets this response

	created       []string
	rekeyedTo     []string
	subscriptions []map[string]any // CreateSubscription request bodies
}

func newFakeCloud(existing map[string]string) *fakeCloud {
	return &fakeCloud{
		byExtID: existing,
		subs:    map[string][]map[string]any{},
	}
}

func (f *fakeCloud) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if f.failAll != nil {
			w.WriteHeader(f.failAll.status)
			_, _ = w.Write([]byte(f.failAll.body))
			return
		}

		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, pathCustomerByExternalID) && strings.HasSuffix(r.URL.Path, pathSubscriptionsSuffix):
			ext := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, pathCustomerByExternalID), pathSubscriptionsSuffix)
			if _, ok := f.byExtID[ext]; !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(notFoundBody))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": f.subs[ext]})

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, pathCustomerByExternalID):
			ext := strings.TrimPrefix(r.URL.Path, pathCustomerByExternalID)
			id, ok := f.byExtID[ext]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(notFoundBody))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "external_id": ext})

		case r.Method == http.MethodPost && r.URL.Path == pathCustomers:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			ext := body["external_id"].(string)
			f.byExtID[ext] = "cust_" + ext
			f.created = append(f.created, ext)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "cust_" + ext, "external_id": ext})

		case r.Method == http.MethodPut && r.URL.Path == pathCustomers:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			old := r.URL.Query().Get("external_customer_id")
			id := f.byExtID[old]
			newExt := body["external_id"].(string)
			f.byExtID[newExt] = id
			delete(f.byExtID, old)
			f.rekeyedTo = append(f.rekeyedTo, newExt)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "external_id": newExt})

		case r.Method == http.MethodPost && r.URL.Path == pathSubscriptions:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.subscriptions = append(f.subscriptions, body)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "sub_new"})

		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
}

func newTestRemoteClient(t *testing.T, f *fakeCloud) *remoteClient {
	t.Helper()
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	return newRemoteClient(config.BillingConfig{
		Region:  "in",
		APIHost: srv.URL,
		APIKey:  "sk",
		PlanID:  "plan_1",
	})
}

func TestEnsureCustomer(t *testing.T) {
	tests := []struct {
		name        string
		existing    map[string]string // external_id -> customer id on Flexprice Cloud
		legacyExtID string            // set only in the region hosting the billing tenant
		wantCreated []string
		wantRenamed []string
		wantFinal   map[string]string
	}{
		{
			name:        "legacy_bare_id_is_renamed",
			existing:    map[string]string{"t1": "cust_legacy"},
			legacyExtID: "t1",
			wantRenamed: []string{"in:t1"},
			wantFinal:   map[string]string{"in:t1": "cust_legacy"},
		},
		{
			name:        "already_renamed_is_noop",
			existing:    map[string]string{"in:t1": "cust_legacy"},
			legacyExtID: "t1",
			wantFinal:   map[string]string{"in:t1": "cust_legacy"},
		},
		{
			name:        "not_found_anywhere_is_created",
			existing:    map[string]string{},
			legacyExtID: "t1",
			wantCreated: []string{"in:t1"},
			wantFinal:   map[string]string{"in:t1": "cust_in:t1"},
		},
		{
			name:        "other_region_never_looks_up_legacy",
			existing:    map[string]string{"t1": "cust_other"},
			wantCreated: []string{"in:t1"},
			wantFinal:   map[string]string{"t1": "cust_other", "in:t1": "cust_in:t1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeCloud(tt.existing)
			c := newTestRemoteClient(t, f)
			in := customerInput{extID: "in:t1", legacyExtID: tt.legacyExtID, name: "Acme", tenantID: "t1"}

			require.NoError(t, c.EnsureCustomer(context.Background(), in))
			require.NoError(t, c.EnsureCustomer(context.Background(), in), "second call must be a no-op")

			assert.Equal(t, tt.wantCreated, f.created, "never duplicate; create at most once")
			assert.Equal(t, tt.wantRenamed, f.rekeyedTo, "rename at most once")
			assert.Equal(t, tt.wantFinal, f.byExtID)
		})
	}
}

func TestStartTrial(t *testing.T) {
	tests := []struct {
		name     string
		existing []map[string]any
		wantSubs []map[string]any
	}{
		{
			name: "creates_monthly_usd_subscription_on_the_plan",
			wantSubs: []map[string]any{{
				"external_customer_id": "in:t1",
				"plan_id":              "plan_1",
				"currency":             "usd",
				"billing_period":       "MONTHLY",
			}},
		},
		{
			name:     "subscription_created_meanwhile_is_not_duplicated",
			existing: []map[string]any{{"id": "s1", "plan_id": "plan_1", "subscription_status": "trialing"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeCloud(map[string]string{"in:t1": "cust_1"})
			f.subs["in:t1"] = tt.existing
			c := newTestRemoteClient(t, f)

			require.NoError(t, c.StartTrial(context.Background(), "in:t1"))

			require.Len(t, f.subscriptions, len(tt.wantSubs))
			for i, want := range tt.wantSubs {
				for k, v := range want {
					assert.Equal(t, v, f.subscriptions[i][k], k)
				}
			}
		})
	}
}

func TestLatestSubscription(t *testing.T) {
	at := func(h int) string {
		return time.Date(2026, 10, 1, h, 0, 0, 0, time.UTC).Format(time.RFC3339)
	}
	sub := func(id, plan string, status types.SubscriptionStatus, created int) map[string]any {
		return map[string]any{"id": id, "plan_id": plan, "subscription_status": status, "created_at": at(created)}
	}
	active, trialing, cancelled := types.SubscriptionStatusActive, types.SubscriptionStatusTrialing, types.SubscriptionStatusCancelled

	tests := []struct {
		name       string
		subs       []map[string]any
		wantID     string
		wantStatus types.SubscriptionStatus
	}{
		{"no_subscriptions", nil, "", ""},
		{"newest_live_wins", []map[string]any{sub("s1", "plan_1", active, 1), sub("s2", "plan_1", trialing, 2)}, "s2", trialing},
		{"live_beats_newer_cancelled", []map[string]any{sub("s1", "plan_1", active, 1), sub("s2", "plan_1", cancelled, 2)}, "s1", active},
		{"only_cancelled_is_returned", []map[string]any{sub("s1", "plan_1", cancelled, 1)}, "s1", cancelled},
		{"newest_cancelled_when_all_cancelled", []map[string]any{sub("s1", "plan_1", cancelled, 1), sub("s2", "plan_1", cancelled, 3)}, "s2", cancelled},
		{"other_plans_ignored", []map[string]any{sub("s1", "plan_other", active, 5)}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeCloud(map[string]string{"in:t1": "cust_1"})
			f.subs["in:t1"] = tt.subs
			c := newTestRemoteClient(t, f)

			got, err := c.LatestSubscription(context.Background(), "in:t1")
			require.NoError(t, err)

			if tt.wantID == "" {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.wantID, got.id)
			assert.Equal(t, tt.wantStatus, got.status)
		})
	}
}

func TestLatestSubscription_UnknownCustomerIsNil(t *testing.T) {
	c := newTestRemoteClient(t, newFakeCloud(map[string]string{}))

	got, err := c.LatestSubscription(context.Background(), "in:missing")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestRemoteClient_MapsCloudErrors(t *testing.T) {
	tests := []struct {
		name     string
		resp     cannedResponse
		wantKind func(error) bool
		wantHint string
	}{
		{
			name:     "json_400_is_validation",
			resp:     cannedResponse{http.StatusBadRequest, `{"code":"validation_error","message":"external id is malformed","http_status_code":400}`},
			wantKind: ierr.IsValidation,
			wantHint: "external id is malformed",
		},
		{
			name:     "json_409_is_already_exists",
			resp:     cannedResponse{http.StatusConflict, `{"code":"conflict","message":"customer exists","http_status_code":409}`},
			wantKind: ierr.IsAlreadyExists,
			wantHint: "customer exists",
		},
		{
			name:     "json_500_is_http_client",
			resp:     cannedResponse{http.StatusInternalServerError, `{"code":"internal_error","message":"boom","http_status_code":500}`},
			wantKind: ierr.IsHTTPClient,
			wantHint: "Flexprice Cloud is unavailable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeCloud(map[string]string{})
			f.failAll = &tt.resp
			c := newTestRemoteClient(t, f)

			err := c.EnsureCustomer(context.Background(), customerInput{extID: "in:t1", name: "Acme", tenantID: "t1"})
			require.Error(t, err)
			assert.True(t, tt.wantKind(err), "got %v", err)
			assert.Contains(t, cockroach.FlattenHints(err), tt.wantHint)
			assert.Equal(t, tt.resp.status, httpStatus(err), "the SDK status stays reachable")
		})
	}
}

func TestCloudErr(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantKind func(error) bool
	}{
		{"not_found", sdkerrors.NewAPIError("x", http.StatusNotFound, "", nil), ierr.IsNotFound},
		{"conflict", sdkerrors.NewAPIError("x", http.StatusConflict, `{"message":"exists"}`, nil), ierr.IsAlreadyExists},
		{"unauthorized_is_validation", sdkerrors.NewAPIError("x", http.StatusUnauthorized, `{"message":"bad key"}`, nil), ierr.IsValidation},
		{"server_error_is_http_client", sdkerrors.NewAPIError("x", http.StatusBadGateway, "", nil), ierr.IsHTTPClient},
		{"transport_error_is_http_client", errors.New("dial tcp: connection refused"), ierr.IsHTTPClient},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cloudErr(tt.err)
			assert.True(t, tt.wantKind(got), "got %v", got)
			assert.Equal(t, isNotFound(tt.err), isNotFound(got), "not-found branching survives the mapping")
		})
	}
	assert.NoError(t, cloudErr(nil))
}

func TestIsNotFound(t *testing.T) {
	resp := func(code int) sdktypes.HTTPMetadata {
		return sdktypes.HTTPMetadata{Response: &http.Response{StatusCode: code}}
	}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain_error", errors.New("boom"), false},
		{"api_error_404", sdkerrors.NewAPIError("x", http.StatusNotFound, "", nil), true},
		{"api_error_500", sdkerrors.NewAPIError("x", http.StatusInternalServerError, "", nil), false},
		{"error_response_404", &sdkerrors.ErrorResponse{HTTPMeta: resp(http.StatusNotFound)}, true},
		{"error_response_400", &sdkerrors.ErrorResponse{HTTPMeta: resp(http.StatusBadRequest)}, false},
		{"error_response_without_http_meta", &sdkerrors.ErrorResponse{}, false},
		{"validation_error_404", sdkerrors.NewResponseValidationError("x", http.StatusNotFound, "", nil, errors.New("bad")), true},
		{"validation_error_500", sdkerrors.NewResponseValidationError("x", http.StatusInternalServerError, "", nil, errors.New("bad")), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isNotFound(tt.err))
		})
	}
}

func TestIsNotFound_SDKJSON404(t *testing.T) {
	c := newTestRemoteClient(t, newFakeCloud(map[string]string{}))

	_, err := c.sdk.Customers.GetCustomerByExternalID(context.Background(), "missing")
	require.Error(t, err)

	var errResp *sdkerrors.ErrorResponse
	assert.True(t, errors.As(err, &errResp), "a JSON 404 surfaces as ErrorResponse, got %T", err)
	assert.True(t, isNotFound(err))
	assert.Equal(t, "customer not found", lo.FromPtr(errResp.Message))
}
