package clickhouse

import (
	"fmt"

	clickhouse_go "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/flexprice/flexprice/internal/config"
)

type ClickHouseStore struct {
	conn driver.Conn
}

func NewClickHouseStore(config *config.Configuration) (*ClickHouseStore, error) {
	options := config.ClickHouse.GetClientOptions()
	conn, err := clickhouse_go.Open(options)
	if err != nil {
		return nil, fmt.Errorf("init clickhouse client: %w", err)
	}

	return &ClickHouseStore{conn: conn}, nil
}

func (s *ClickHouseStore) GetConn() driver.Conn {
	return s.conn
}

func (s *ClickHouseStore) Close() error {
	return s.conn.Close()
}
