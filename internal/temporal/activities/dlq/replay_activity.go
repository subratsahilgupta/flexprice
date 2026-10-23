package dlq

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/ee/service"
	ierr "github.com/flexprice/flexprice/internal/errors"
	dlqengine "github.com/flexprice/flexprice/internal/kafka/dlq"
	models "github.com/flexprice/flexprice/internal/temporal/models/dlq"
	"go.temporal.io/sdk/activity"
)

// ReplayDLQActivities contains the DLQ replay activity.
type ReplayDLQActivities struct {
	dlqReplayService service.DLQReplayService
}

// NewReplayDLQActivities creates a new ReplayDLQActivities instance.
func NewReplayDLQActivities(dlqReplayService service.DLQReplayService) *ReplayDLQActivities {
	return &ReplayDLQActivities{dlqReplayService: dlqReplayService}
}

// ReplayDLQ drains one DLQ topic back to origin.
// This method will be registered as "ReplayDLQ" in Temporal.
func (a *ReplayDLQActivities) ReplayDLQ(ctx context.Context, input models.ReplayDLQWorkflowInput) (*models.ReplayDLQWorkflowResult, error) {
	logger := activity.GetLogger(ctx)

	if err := input.Validate(); err != nil {
		return nil, err
	}

	// Heartbeat so a dead worker is detected within HeartbeatTimeout instead of StartToCloseTimeout.
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				activity.RecordHeartbeat(ctx)
			}
		}
	}()

	summary, err := a.dlqReplayService.ReplayDLQ(ctx, dlqengine.Options{
		SourceTopic:    input.SourceTopic,
		TargetOverride: input.TargetOverride,
		Group:          input.Group,
		SinceMs:        input.SinceMs,
		FromStart:      input.FromStart,
		Max:            input.Max,
		MaxReplays:     input.MaxReplays,
		DryRun:         input.DryRun,
	})
	if err != nil {
		logger.Error("DLQ replay activity failed",
			"source_topic", input.SourceTopic,
			"error", err)
		return nil, ierr.WithError(err).
			WithHint("Failed to replay DLQ topic").
			WithReportableDetails(map[string]interface{}{
				"source_topic": input.SourceTopic,
			}).
			Mark(ierr.ErrInternal)
	}

	logger.Info("Completed DLQ replay activity",
		"source_topic", input.SourceTopic,
		"scanned", summary.Scanned,
		"replayed", summary.Replayed,
		"skipped", summary.Skipped,
		"quarantined", summary.Quarantined)

	return &models.ReplayDLQWorkflowResult{
		Scanned:     summary.Scanned,
		Replayed:    summary.Replayed,
		Skipped:     summary.Skipped,
		Quarantined: summary.Quarantined,
		ByTarget:    summary.ByTarget,
		ByReason:    summary.ByReason,
	}, nil
}
