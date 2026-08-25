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

// newLakePub builds a MeterUsagePublisher over a fake, on the lake topic.
func newLakePub(pub messagePublisher) *MeterUsagePublisher {
	return &MeterUsagePublisher{
		publisher: pub,
		topic:     "analytics.meter_usage",
		logger:    logger.NewNoopLogger(),
	}
}

func TestPublishMeterUsage_MarshalsRecordToLakeTopic(t *testing.T) {
	fake := &fakePublisher{}
	p := newLakePub(fake)

	if err := p.PublishMeterUsage(context.Background(), []*events.MeterUsage{sampleMeterUsage()}); err != nil {
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
	// The lake needs id + ingested_at (version) + meter_id.
	if got["id"] != "evt_1" {
		t.Errorf("missing/wrong id: %v", got["id"])
	}
	if got["meter_id"] != "meter_1" {
		t.Errorf("missing/wrong meter_id: %v", got["meter_id"])
	}
	if got["ingested_at"] == nil || got["ingested_at"] == "0001-01-01T00:00:00Z" {
		t.Errorf("ingested_at must be stamped for lake versioning, got %v", got["ingested_at"])
	}
}

// Fire-and-forget: a publisher error must NOT propagate out of PublishMeterUsage.
func TestPublishMeterUsage_SwallowsPublisherError(t *testing.T) {
	fake := &fakePublisher{err: context.DeadlineExceeded}
	p := newLakePub(fake)

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
