package testutil

import (
	"context"

	"github.com/flexprice/flexprice/internal/domain/connection"
	"github.com/flexprice/flexprice/internal/types"
)

type InMemoryConnectionStore struct{}

func NewInMemoryConnectionStore() *InMemoryConnectionStore {
	return &InMemoryConnectionStore{}
}

func (s *InMemoryConnectionStore) Create(ctx context.Context, c *connection.Connection) error {
	return nil
}
func (s *InMemoryConnectionStore) Get(ctx context.Context, id string) (*connection.Connection, error) {
	return nil, nil
}
func (s *InMemoryConnectionStore) GetByConnectionCode(ctx context.Context, code string) (*connection.Connection, error) {
	return nil, nil
}
func (s *InMemoryConnectionStore) GetByEnvironmentAndProvider(ctx context.Context, environmentID string, provider types.SecretProvider) (*connection.Connection, error) {
	return &connection.Connection{
		ID:            "dummy-connection-id",
		EnvironmentID: environmentID,
		ProviderType:  provider,
		Metadata: map[string]interface{}{
			"publishable_key": "pk_test_dummy",
			"secret_key":      "sk_test_dummy",
			"webhook_secret":  "whsec_dummy",
		},
	}, nil
}
func (s *InMemoryConnectionStore) List(ctx context.Context, filter *types.ConnectionFilter) ([]*connection.Connection, error) {
	return nil, nil
}
func (s *InMemoryConnectionStore) Count(ctx context.Context, filter *types.ConnectionFilter) (int, error) {
	return 0, nil
}
func (s *InMemoryConnectionStore) Update(ctx context.Context, c *connection.Connection) error {
	return nil
}
func (s *InMemoryConnectionStore) Delete(ctx context.Context, c *connection.Connection) error {
	return nil
}
func (s *InMemoryConnectionStore) Clear() {}
