package clickhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	clickhouse_go "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/events"
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

func (s *ClickHouseStore) Close() error {
	return s.conn.Close()
}

func (s *ClickHouseStore) InsertEvent(ctx context.Context, event *events.Event) error {
	propertiesJSON, err := json.Marshal(event.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}

	query := `
		INSERT INTO events (
			id, external_customer_id, tenant_id, event_name, timestamp, properties
		) VALUES (
			?, ?, ?, ?, ?, ?
		)
	`

	err = s.conn.Exec(ctx, query,
		event.ID,
		event.ExternalCustomerID,
		event.TenantID,
		event.EventName,
		event.Timestamp,
		string(propertiesJSON),
	)

	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	return nil
}

func (s *ClickHouseStore) GetUsage(
	ctx context.Context,
	externalCustomerID string,
	eventName string,
	propertyName string,
	aggregator events.Aggregator,
	startTime, endTime time.Time,
) (*events.AggregationResult, error) {
	query := aggregator.GetQuery(eventName, propertyName, externalCustomerID, startTime, endTime)

	var value interface{}
	switch aggregator.GetType() {
	case events.CountAggregation:
		var count uint64
		err := s.conn.QueryRow(ctx, query).Scan(&count)
		if err != nil {
			return nil, fmt.Errorf("get usage: %w", err)
		}
		value = count

	default:
		var num float64
		err := s.conn.QueryRow(ctx, query).Scan(&num)
		if err != nil {
			return nil, fmt.Errorf("get usage: %w", err)
		}
		value = num
	}

	return &events.AggregationResult{
		Value:     value,
		EventName: eventName,
		Type:      aggregator.GetType(),
	}, nil
}
