package stripe

import (
	"context"
	"errors"
	"testing"

	"github.com/flexprice/flexprice/internal/domain/connection"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/require"
)

type clientTestConnectionRepo struct {
	connection.Repository
	conn *connection.Connection
	err  error
}

func (r *clientTestConnectionRepo) GetByProvider(_ context.Context, _ types.SecretProvider) (*connection.Connection, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.conn, nil
}

func TestGetConnection_ReturnsConnection(t *testing.T) {
	want := &connection.Connection{ID: "conn_1"}
	c := NewClient(&clientTestConnectionRepo{conn: want}, nil, logger.NewNoopLogger())

	got, err := c.GetConnection(context.Background())

	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestGetConnection_PropagatesRepoError(t *testing.T) {
	repoErr := errors.New("connection not found")
	c := NewClient(&clientTestConnectionRepo{err: repoErr}, nil, logger.NewNoopLogger())

	_, err := c.GetConnection(context.Background())

	require.Error(t, err)
}
