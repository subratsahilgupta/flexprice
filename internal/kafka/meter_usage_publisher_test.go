package kafka

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/shopspring/decimal"
)

func sampleMeterUsage() *events.MeterUsage {
	mu := &events.MeterUsage{
		MeterID:    "meter_1",
		QtyTotal:   decimal.NewFromInt(5),
		UniqueHash: "hash_1",
	}
	mu.ID = "evt_1"
	mu.TenantID = "tenant_1"
	mu.EnvironmentID = "env_1"
	mu.EventName = "api_call"
	mu.Timestamp = time.Unix(1700000000, 0).UTC()
	return mu
}

// newAnalyticsPub builds a MeterUsagePublisher over a fake, on the analytics topics with a
// 24h late threshold (the config default).
func newAnalyticsPub(pub messagePublisher) *MeterUsagePublisher {
	return &MeterUsagePublisher{
		publisher:     pub,
		topic:         "analytics.meter_usage",
		lazyTopic:     "analytics.meter_usage.lazy",
		lateThreshold: 24 * time.Hour,
		logger:        logger.NewNoopLogger(),
	}
}

func TestPublishMeterUsage_MarshalsRecordToMainTopic(t *testing.T) {
	fake := &fakePublisher{}
	p := newAnalyticsPub(fake)

	rec := sampleMeterUsage()
	rec.IngestedAt = rec.Timestamp // on-time: ingested == timestamp

	if err := p.PublishMeterUsage(context.Background(), []*events.MeterUsage{rec}); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if fake.callCount() != 1 {
		t.Fatalf("expected 1 publish, got %d", fake.callCount())
	}
	if fake.topics[0] != "analytics.meter_usage" {
		t.Fatalf("expected topic analytics.meter_usage, got %q", fake.topics[0])
	}

	msg := fake.only()
	if msg == nil {
		t.Fatal("expected exactly one message")
	}
	if msg.UUID != "evt_1" {
		t.Fatalf("expected message UUID evt_1 (dedup key), got %q", msg.UUID)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(msg.Payload, &got); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if got["id"] != "evt_1" {
		t.Errorf("missing/wrong id: %v", got["id"])
	}
	if got["meter_id"] != "meter_1" {
		t.Errorf("missing/wrong meter_id: %v", got["meter_id"])
	}
	if got["ingested_at"] == nil || got["ingested_at"] == "0001-01-01T00:00:00Z" {
		t.Errorf("ingested_at must be stamped for versioning, got %v", got["ingested_at"])
	}
}

// A zero ingested_at is stamped, and (now - old timestamp) exceeds the threshold, so the
// record routes to the lazy topic.
func TestPublishMeterUsage_StampsIngestedAt(t *testing.T) {
	fake := &fakePublisher{}
	p := newAnalyticsPub(fake)

	if err := p.PublishMeterUsage(context.Background(), []*events.MeterUsage{sampleMeterUsage()}); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	msg := fake.only()
	if msg == nil {
		t.Fatal("expected exactly one message")
	}
	var got map[string]interface{}
	_ = json.Unmarshal(msg.Payload, &got)
	if got["ingested_at"] == nil || got["ingested_at"] == "0001-01-01T00:00:00Z" {
		t.Errorf("ingested_at must be stamped, got %v", got["ingested_at"])
	}
}

// On-time record (ingested within threshold of timestamp) → main topic.
func TestPublishMeterUsage_OnTimeGoesToMainTopic(t *testing.T) {
	fake := &fakePublisher{}
	p := newAnalyticsPub(fake)

	rec := sampleMeterUsage()
	rec.IngestedAt = rec.Timestamp.Add(1 * time.Hour) // < 24h late

	_ = p.PublishMeterUsage(context.Background(), []*events.MeterUsage{rec})
	if fake.topics[0] != "analytics.meter_usage" {
		t.Fatalf("on-time record must go to main topic, got %q", fake.topics[0])
	}
}

// Late record (ingested - timestamp > threshold) → lazy topic.
func TestPublishMeterUsage_LateGoesToLazyTopic(t *testing.T) {
	fake := &fakePublisher{}
	p := newAnalyticsPub(fake)

	rec := sampleMeterUsage()
	rec.IngestedAt = rec.Timestamp.Add(25 * time.Hour) // > 24h late

	_ = p.PublishMeterUsage(context.Background(), []*events.MeterUsage{rec})
	if fake.topics[0] != "analytics.meter_usage.lazy" {
		t.Fatalf("late record must go to lazy topic, got %q", fake.topics[0])
	}
}

// Late record but lazy topic unset → falls back to the main topic (never dropped).
func TestPublishMeterUsage_LateFallsBackWhenLazyUnset(t *testing.T) {
	fake := &fakePublisher{}
	p := newAnalyticsPub(fake)
	p.lazyTopic = "" // no lazy topic configured

	rec := sampleMeterUsage()
	rec.IngestedAt = rec.Timestamp.Add(25 * time.Hour) // > 24h late

	_ = p.PublishMeterUsage(context.Background(), []*events.MeterUsage{rec})
	if fake.topics[0] != "analytics.meter_usage" {
		t.Fatalf("late record with no lazy topic must fall back to main, got %q", fake.topics[0])
	}
}

// Boundary: exactly at the threshold is NOT late (routing uses strictly greater-than).
func TestPublishMeterUsage_ThresholdBoundary(t *testing.T) {
	fake := &fakePublisher{}
	p := newAnalyticsPub(fake)

	// exactly 24h => not late => main topic
	onEdge := sampleMeterUsage()
	onEdge.IngestedAt = onEdge.Timestamp.Add(24 * time.Hour)
	_ = p.PublishMeterUsage(context.Background(), []*events.MeterUsage{onEdge})
	if fake.topics[0] != "analytics.meter_usage" {
		t.Fatalf("exactly at threshold must NOT be late (main topic), got %q", fake.topics[0])
	}

	// one nanosecond over => late => lazy topic
	fake2 := &fakePublisher{}
	p2 := newAnalyticsPub(fake2)
	over := sampleMeterUsage()
	over.IngestedAt = over.Timestamp.Add(24*time.Hour + time.Nanosecond)
	_ = p2.PublishMeterUsage(context.Background(), []*events.MeterUsage{over})
	if fake2.topics[0] != "analytics.meter_usage.lazy" {
		t.Fatalf("just over threshold must be late (lazy topic), got %q", fake2.topics[0])
	}
}

// Fire-and-forget: a publisher error must NOT propagate out of PublishMeterUsage.
func TestPublishMeterUsage_SwallowsPublisherError(t *testing.T) {
	fake := &fakePublisher{err: context.DeadlineExceeded}
	p := newAnalyticsPub(fake)

	if err := p.PublishMeterUsage(context.Background(), []*events.MeterUsage{sampleMeterUsage()}); err != nil {
		t.Fatalf("publish error must be swallowed (fire-and-forget), got %v", err)
	}
}

// A nil (unconfigured) publisher is a no-op, never a panic.
func TestPublishMeterUsage_NilPublisherIsNoop(t *testing.T) {
	var p *MeterUsagePublisher
	if err := p.PublishMeterUsage(context.Background(), []*events.MeterUsage{sampleMeterUsage()}); err != nil {
		t.Fatalf("nil publisher must be a no-op, got %v", err)
	}
}
