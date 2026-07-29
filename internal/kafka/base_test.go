package kafka

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/config"
)

func TestGetSaramaConfigSetsOffsetRetention(t *testing.T) {
	cfg, err := GetSaramaConfig(&config.KafkaConfig{})
	if err != nil {
		t.Fatalf("GetSaramaConfig() error = %v, want nil", err)
	}

	if cfg.Consumer.Offsets.Retention != 7*24*time.Hour {
		t.Fatalf("consumer offset retention = %s, want 7 days", cfg.Consumer.Offsets.Retention)
	}
}
