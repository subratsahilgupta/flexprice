package cron

import (
	"context"
	"testing"
	"time"

	cronModels "github.com/flexprice/flexprice/internal/temporal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func rollupDirtyStub(_ context.Context, _ time.Time) (*cronModels.RevenueRollupWorkflowResult, error) {
	return nil, nil
}

func TestRevenueRollupWorkflow_Success(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	expected := &cronModels.RevenueRollupWorkflowResult{Rolled: 5, Skipped: 1}
	input := cronModels.RevenueRollupInput{Interval: 2 * time.Hour}

	env.RegisterActivityWithOptions(rollupDirtyStub, activity.RegisterOptions{
		Name: ActivityRollupDirty,
	})

	var capturedSince time.Time
	env.OnActivity(ActivityRollupDirty, mock.Anything, mock.MatchedBy(func(since time.Time) bool {
		capturedSince = since
		return true
	})).Return(expected, nil)

	beforeStart := time.Now()
	env.ExecuteWorkflow(RevenueRollupWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)

	// The activity must be invoked with the run window: since = workflow start - Interval.
	require.False(t, capturedSince.IsZero(), "RollupDirtyActivity must receive a non-zero since")
	require.WithinDuration(t, beforeStart.Add(-input.Interval), capturedSince, 5*time.Second)
}

func TestRevenueRollupWorkflow_DefaultInterval(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	env.RegisterActivityWithOptions(rollupDirtyStub, activity.RegisterOptions{
		Name: ActivityRollupDirty,
	})

	var capturedSince time.Time
	env.OnActivity(ActivityRollupDirty, mock.Anything, mock.MatchedBy(func(since time.Time) bool {
		capturedSince = since
		return true
	})).Return(&cronModels.RevenueRollupWorkflowResult{}, nil)

	beforeStart := time.Now()
	// Zero Interval must fall back to defaultRevenueRollupInterval (1h).
	env.ExecuteWorkflow(RevenueRollupWorkflow, cronModels.RevenueRollupInput{})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	require.WithinDuration(t, beforeStart.Add(-defaultRevenueRollupInterval), capturedSince, 5*time.Second)
}

func TestRevenueRollupWorkflow_ActivityError(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	env.RegisterActivityWithOptions(rollupDirtyStub, activity.RegisterOptions{
		Name: ActivityRollupDirty,
	})
	env.OnActivity(ActivityRollupDirty, mock.Anything, mock.Anything).
		Return(nil, assert.AnError)

	env.ExecuteWorkflow(RevenueRollupWorkflow, cronModels.RevenueRollupInput{Interval: time.Hour})

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
}
