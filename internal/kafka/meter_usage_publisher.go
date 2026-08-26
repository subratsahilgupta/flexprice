package kafka

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/logger"
)

// MeterUsagePublisher publishes meter_usage records to the analytics lake topic
// (cfg.Kafka.LakeTopic) AFTER they are written to ClickHouse. It is ADDITIVE and
// FIRE-AND-FORGET: ClickHouse stays authoritative, so a publish failure here is logged
// and swallowed — it must never fail the caller or affect billing. Presence-gated: when the
// lake topic is not configured the fx provider returns nil, and every method is a no-op.
type MeterUsagePublisher struct {
	publisher messagePublisher
	topic     string
	logger    *logger.Logger
}

// NewMeterUsagePublisher is the fx provider. It reuses the local-cluster producer that already
// carries source events. Presence-based: an empty cfg.Kafka.LakeTopic ⇒ nil publisher (no-op),
// mirroring the KafkaSecondary dual-write gating.
func NewMeterUsagePublisher(primaryProducer *Producer, cfg *config.Configuration, logger *logger.Logger) *MeterUsagePublisher {
	if cfg.Kafka.LakeTopic == "" || primaryProducer == nil {
		return nil
	}
	return &MeterUsagePublisher{
		publisher: primaryProducer,
		topic:     cfg.Kafka.LakeTopic,
		logger:    logger,
	}
}

// PublishMeterUsage publishes each record to the lake topic as one JSON message keyed by
// event id (the lake's dedup key). ingested_at is stamped when unset so the lake has a version
// column. Fire-and-forget: per-record errors are logged, never returned.
func (p *MeterUsagePublisher) PublishMeterUsage(ctx context.Context, records []*events.MeterUsage) error {
	// nil publisher (lake not configured) ⇒ no-op.
	if p == nil || p.publisher == nil {
		return nil
	}

	for _, record := range records {
		// The lake dedups on (id, ingested_at). BulkInsert relies on the ClickHouse DEFAULT for
		// ingested_at, so records reach here with it zero — stamp it now to give the lake a version.
		if record.IngestedAt.IsZero() {
			record.IngestedAt = time.Now().UTC()
		}

		payload, err := json.Marshal(record)
		if err != nil {
			p.logger.Ctx(ctx).With("event_id", record.ID, "error", err).
				Error("lake meter_usage marshal failed")
			continue
		}

		msg := message.NewMessage(record.ID, payload)
		msg.Metadata.Set("tenant_id", record.TenantID)
		msg.Metadata.Set("environment_id", record.EnvironmentID)

		if err := p.publisher.Publish(p.topic, msg); err != nil {
			p.logger.Ctx(ctx).With(
				"event_id", record.ID,
				"tenant_id", record.TenantID,
				"topic", p.topic,
				"error", err,
			).Error("lake meter_usage publish failed")
		}
	}
	return nil
}
