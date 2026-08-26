package analytics

import (
	"context"
	"encoding/json"
	"strconv"
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

// meterUsagePublisher publishes meter_usage records to the analytics meter_usage topics
// AFTER they are written to ClickHouse. It is ADDITIVE and FIRE-AND-FORGET: ClickHouse stays
// authoritative, so a publish failure here is logged and swallowed — it must never fail the
// caller or affect billing. Gated on cfg.Analytics.Enabled: when the feed is off (or the main
// topic is unset) the fx provider returns an untyped-nil interface, and every method is a no-op.
//
// Routing per record: on-time records go to the main topic; late records (ingested_at -
// timestamp > lateThreshold) go to the lazy topic. If the lazy topic is unset, late records
// fall back to the main topic (never dropped).
type meterUsagePublisher struct {
	publisher     messagePublisher
	topic         string
	lazyTopic     string
	lateThreshold time.Duration
	logger        *logger.Logger
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
		publisher:     primaryProducer,
		topic:         cfg.Analytics.MeterUsageTopic,
		lazyTopic:     cfg.Analytics.MeterUsageLazyTopic,
		lateThreshold: cfg.Analytics.LateThreshold,
		logger:        logger,
	}
}

// PublishMeterUsage publishes each record as one JSON message keyed by event id (the lake's
// dedup key). ingested_at is stamped when unset so the lake has a version column. Late records
// route to the lazy topic (falling back to the main topic when it is unset). Fire-and-forget:
// per-record errors are logged, never returned.
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

		// Late events (ingested well after their timestamp) route to the lazy topic. When no lazy
		// topic is configured, late records fall back to the main topic so they are never dropped.
		// Compute lateness ONCE here: it both drives routing and is stamped as an explicit signal
		// (header + payload) so downstream need not infer it from the topic name.
		late := record.IngestedAt.Sub(record.Timestamp) > p.lateThreshold
		topic := p.topic
		if late && p.lazyTopic != "" {
			topic = p.lazyTopic
		}

		// Marshal a lightweight wrapper: *events.MeterUsage embeds flat (its json-tagged fields
		// stay top-level), plus two analytics-only fields prefixed "_" to keep them distinct.
		// The domain struct is left untouched (shared with the CH write path).
		payload, err := json.Marshal(struct {
			*events.MeterUsage
			Late                 bool    `json:"_late"`
			LateThresholdSeconds float64 `json:"_late_threshold_seconds"`
		}{
			MeterUsage:           record,
			Late:                 late,
			LateThresholdSeconds: p.lateThreshold.Seconds(),
		})
		if err != nil {
			p.logger.Ctx(ctx).With("event_id", record.ID, "error", err).
				Error("analytics meter_usage marshal failed")
			continue
		}

		msg := message.NewMessage(record.ID, payload)
		msg.Metadata.Set("tenant_id", record.TenantID)
		msg.Metadata.Set("environment_id", record.EnvironmentID)
		msg.Metadata.Set("late", strconv.FormatBool(late))

		if err := p.publisher.Publish(topic, msg); err != nil {
			p.logger.Ctx(ctx).With(
				"event_id", record.ID,
				"tenant_id", record.TenantID,
				"topic", topic,
				"error", err,
			).Error("analytics meter_usage publish failed")
		}
	}
}
