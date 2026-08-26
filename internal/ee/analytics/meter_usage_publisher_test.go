package analytics

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/flexprice/flexprice/internal/domain/events"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/shopspring/decimal"
)

// fakePublisher is a test double for messagePublisher: it records what was published,
// and can be told to fail. Local to this package (kafka's own fake did not move here).
type fakePublisher struct {
	mu     sync.Mutex
	topics []string
	msgs   []*message.Message
	err    error
}

func (f *fakePublisher) Publish(topic string, messages ...*message.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	for _, m := range messages {
		f.topics = append(f.topics, topic)
		f.msgs = append(f.msgs, m)
	}
	return nil
}

func (f *fakePublisher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.msgs)
}

func (f *fakePublisher) only() *message.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.msgs) != 1 {
		return nil
	}
	return f.msgs[0]
}

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

	p.PublishMeterUsage(context.Background(), []*events.MeterUsage{rec})

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

	p.PublishMeterUsage(context.Background(), []*events.MeterUsage{sampleMeterUsage()})

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

	p.PublishMeterUsage(context.Background(), []*events.MeterUsage{rec})
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

	p.PublishMeterUsage(context.Background(), []*events.MeterUsage{rec})
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

	p.PublishMeterUsage(context.Background(), []*events.MeterUsage{rec})
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
	p.PublishMeterUsage(context.Background(), []*events.MeterUsage{onEdge})
	if fake.topics[0] != "analytics.meter_usage" {
		t.Fatalf("exactly at threshold must NOT be late (main topic), got %q", fake.topics[0])
	}

	// one nanosecond over => late => lazy topic
	fake2 := &fakePublisher{}
	p2 := newAnalyticsPub(fake2)
	over := sampleMeterUsage()
	over.IngestedAt = over.Timestamp.Add(24*time.Hour + time.Nanosecond)
	p2.PublishMeterUsage(context.Background(), []*events.MeterUsage{over})
	if fake2.topics[0] != "analytics.meter_usage.lazy" {
		t.Fatalf("just over threshold must be late (lazy topic), got %q", fake2.topics[0])
	}
}

// On-time record → header late=="false" AND payload _late==false, with original record
// fields (id, meter_id) still present flat at the top level alongside the analytics fields.
func TestPublishMeterUsage_OnTimeStampsLateFalse(t *testing.T) {
	fake := &fakePublisher{}
	p := newAnalyticsPub(fake)

	rec := sampleMeterUsage()
	rec.IngestedAt = rec.Timestamp.Add(1 * time.Hour) // < 24h late

	p.PublishMeterUsage(context.Background(), []*events.MeterUsage{rec})

	msg := fake.only()
	if msg == nil {
		t.Fatal("expected exactly one message")
	}
	if got := msg.Metadata.Get("late"); got != "false" {
		t.Errorf("expected header late==\"false\", got %q", got)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(msg.Payload, &got); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if got["_late"] != false {
		t.Errorf("expected payload _late==false, got %v", got["_late"])
	}
	// original record fields must remain flat at the top level
	if got["id"] != "evt_1" {
		t.Errorf("missing/wrong flat id: %v", got["id"])
	}
	if got["meter_id"] != "meter_1" {
		t.Errorf("missing/wrong flat meter_id: %v", got["meter_id"])
	}
	// threshold basis is stamped so the consumer knows what "late" was measured against
	if got["_late_threshold_seconds"] != (24 * time.Hour).Seconds() {
		t.Errorf("expected _late_threshold_seconds==%v, got %v", (24 * time.Hour).Seconds(), got["_late_threshold_seconds"])
	}
}

// Late record → header late=="true" AND payload _late==true, AND it still routes to the
// lazy topic (existing routing behavior unchanged).
func TestPublishMeterUsage_LateStampsLateTrue(t *testing.T) {
	fake := &fakePublisher{}
	p := newAnalyticsPub(fake)

	rec := sampleMeterUsage()
	rec.IngestedAt = rec.Timestamp.Add(25 * time.Hour) // > 24h late

	p.PublishMeterUsage(context.Background(), []*events.MeterUsage{rec})

	if fake.topics[0] != "analytics.meter_usage.lazy" {
		t.Fatalf("late record must still route to lazy topic, got %q", fake.topics[0])
	}

	msg := fake.only()
	if msg == nil {
		t.Fatal("expected exactly one message")
	}
	if got := msg.Metadata.Get("late"); got != "true" {
		t.Errorf("expected header late==\"true\", got %q", got)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(msg.Payload, &got); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if got["_late"] != true {
		t.Errorf("expected payload _late==true, got %v", got["_late"])
	}
	if got["id"] != "evt_1" {
		t.Errorf("missing/wrong flat id: %v", got["id"])
	}
	if got["_late_threshold_seconds"] != (24 * time.Hour).Seconds() {
		t.Errorf("expected _late_threshold_seconds==%v, got %v", (24 * time.Hour).Seconds(), got["_late_threshold_seconds"])
	}
}

// Fire-and-forget: a publisher error must NOT propagate out of PublishMeterUsage.
func TestPublishMeterUsage_SwallowsPublisherError(t *testing.T) {
	fake := &fakePublisher{err: context.DeadlineExceeded}
	p := newAnalyticsPub(fake)

	p.PublishMeterUsage(context.Background(), []*events.MeterUsage{sampleMeterUsage()})
}

// A nil (unconfigured) publisher is a no-op, never a panic.
func TestPublishMeterUsage_NilPublisherIsNoop(t *testing.T) {
	var p *MeterUsagePublisher
	p.PublishMeterUsage(context.Background(), []*events.MeterUsage{sampleMeterUsage()})
}
