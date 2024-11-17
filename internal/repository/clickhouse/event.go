package clickhouse

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/flexprice/flexprice/internal/clickhouse"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/types"
)

type EventRepository struct {
	store *clickhouse.ClickHouseStore
}

func NewEventRepository(store *clickhouse.ClickHouseStore) events.Repository {
	return &EventRepository{store: store}
}

func (r *EventRepository) InsertEvent(ctx context.Context, event *events.Event) error {
	propertiesJSON, err := json.Marshal(event.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}

	// Adding a layer to validate the event before inserting it
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate event: %w", err)
	}

	query := `
		INSERT INTO events (
			id, external_customer_id, tenant_id, event_name, timestamp, properties
		) VALUES (
			?, ?, ?, ?, ?, ?
		)
	`

	err = r.store.GetConn().Exec(ctx, query,
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

func (r *EventRepository) GetUsage(ctx context.Context, params *events.UsageParams) (*events.AggregationResult, error) {
	aggregator := GetAggregator(params.AggregationType)
	query := aggregator.GetQuery(ctx, params)

	var value interface{}
	switch aggregator.GetType() {
	case types.AggregationCount:
		var count uint64
		err := r.store.GetConn().QueryRow(ctx, query).Scan(&count)
		if err != nil {
			return nil, fmt.Errorf("get usage: %w", err)
		}
		value = count

	default:
		var num float64
		err := r.store.GetConn().QueryRow(ctx, query).Scan(&num)
		if err != nil {
			return nil, fmt.Errorf("get usage: %w", err)
		}
		value = num
	}

	return &events.AggregationResult{
		Value:     value,
		EventName: params.EventName,
		Type:      aggregator.GetType(),
	}, nil

}
