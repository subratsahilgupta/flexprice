package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/flexprice/flexprice/internal/config"
)

type ClickHouseStorage struct {
	conn driver.Conn
}

type Event struct {
	ID         string    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	CustomerID string    `json:"customer_id"`
	FeatureID  string    `json:"feature_id"`
	Value      float64   `json:"value"`
}

func NewClickHouseStorage(config *config.Configuration) (*ClickHouseStorage, error) {
	options := config.ClickHouse.GetClientOptions()
	conn, err := clickhouse.Open(options)
	if err != nil {
		return nil, fmt.Errorf("init clickhouse client: %w", err)
	}

	return &ClickHouseStorage{conn: conn}, nil
}

func (s *ClickHouseStorage) InsertEvent(ctx context.Context, event Event) error {
	query := `
		INSERT INTO events (id, timestamp, customer_id, feature_id, value)
		VALUES (?, ?, ?, ?, ?)
	`

	err := s.conn.Exec(ctx, query,
		event.ID,
		event.Timestamp,
		event.CustomerID,
		event.FeatureID,
		event.Value,
	)

	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	return nil
}

func (s *ClickHouseStorage) GetUsage(ctx context.Context, customerID, featureID string, startTime, endTime time.Time, aggregationType string) (float64, error) {
	query := `
		SELECT {agg_func}(value) as usage
		FROM events
		WHERE customer_id = ? AND feature_id = ? AND timestamp BETWEEN ? AND ?
	`
	query = strings.Replace(query, "{agg_func}", aggregationType, 1)

	var usage float64
	err := s.conn.QueryRow(ctx, query, customerID, featureID, startTime, endTime).Scan(&usage)
	if err != nil {
		return 0, fmt.Errorf("failed to get usage: %w", err)
	}

	return usage, nil
}

func (s *ClickHouseStorage) Close() error {
	return s.conn.Close()
}
