package selfbilling

import (
	"context"
	"sync"
)

type fakeClient struct {
	mu            sync.Mutex
	customers     map[string]bool
	legacyExtIDs  map[string]string // extID -> legacyExtID passed to EnsureCustomer
	subs          map[string]*remoteSubscription
	trialsStarted int
	err           error
}

func newFakeClient() *fakeClient {
	return &fakeClient{customers: map[string]bool{}, legacyExtIDs: map[string]string{}, subs: map[string]*remoteSubscription{}}
}

func (f *fakeClient) EnsureCustomer(_ context.Context, in customerInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.customers[in.extID] = true
	f.legacyExtIDs[in.extID] = in.legacyExtID
	return f.err
}

func (f *fakeClient) StartTrial(_ context.Context, ext string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.trialsStarted++
	f.subs[ext] = &remoteSubscription{id: "sub_" + ext, status: "trialing"}
	return f.err
}

func (f *fakeClient) LatestSubscription(_ context.Context, ext string) (*remoteSubscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.subs[ext], f.err
}
