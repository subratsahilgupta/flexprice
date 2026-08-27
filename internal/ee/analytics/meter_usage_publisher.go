package analytics

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/kafka"
	"github.com/flexprice/flexprice/internal/logger"
)

// messagePublisher is the minimal publish surface this package needs. *kafka.Producer
// satisfies it. Declared locally so the analytics package does not depend on kafka's
// internal interface — it only needs "publish a message to a topic".
type messagePublisher interface {
	Publish(topic string, messages ...*message.Message) error
}

// MeterUsagePublisher is the exported dependency the service depends on. The concrete impl
// (*meterUsagePublisher) stays private per repo convention; consumers use this interface.
type MeterUsagePublisher interface {
	PublishMeterUsage(ctx context.Context, records []*events.MeterUsage)
}

// meterUsagePublisher publishes meter_usage records to the analytics meter_usage topic AFTER
// they are written to ClickHouse. It is ADDITIVE and FIRE-AND-FORGET: ClickHouse stays
// authoritative, so a publish failure here is logged and swallowed — it must never fail the
// caller or affect billing. Gated on cfg.Analytics.Enabled: when the feed is off (or the topic
// is unset) the fx provider returns an untyped-nil interface, and every method is a no-op.
// Recency (late/on-time) is handled downstream by the dedup worker's two-cadence design on the
// ClickHouse side — every record here publishes to the single main topic.
type meterUsagePublisher struct {
	publisher messagePublisher
	topic     string
	logger    *logger.Logger
}

// NewMeterUsagePublisher is the fx provider. It reuses the local-cluster producer that
// already carries source events. Gated: unless the analytics feed is enabled AND a main topic is
// configured AND a producer exists, it returns an untyped-nil interface, mirroring the presence
// gating used for the KafkaSecondary dual-write path. Returning untyped nil (not a typed-nil
// *meterUsagePublisher) keeps the caller's `!= nil` guard FALSE so the method is never called.
func NewMeterUsagePublisher(primaryProducer *kafka.Producer, cfg *config.Configuration, logger *logger.Logger) MeterUsagePublisher {
	if !cfg.Analytics.Enabled || cfg.Analytics.MeterUsageTopic == "" || primaryProducer == nil {
		return nil
	}
	return &meterUsagePublisher{
		publisher: primaryProducer,
		topic:     cfg.Analytics.MeterUsageTopic,
		logger:    logger,
	}
}

// PublishMeterUsage publishes each record as one JSON message keyed by event id (the lake's
// dedup key) to the single main topic. ingested_at is stamped when unset so the lake has a
// version column. Fire-and-forget: per-record errors are logged, never returned.
func (p *meterUsagePublisher) PublishMeterUsage(ctx context.Context, records []*events.MeterUsage) {
	// nil publisher (feed disabled) ⇒ no-op.
	if p == nil || p.publisher == nil {
		return
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
				Error("analytics meter_usage marshal failed")
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
			).Error("analytics meter_usage publish failed")
		}
	}
}
