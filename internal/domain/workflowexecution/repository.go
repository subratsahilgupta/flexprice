package workflowexecution

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/types"
)

// Repository defines the interface for workflow execution data access
type Repository interface {
	Create(ctx context.Context, exec *WorkflowExecution) error
	Get(ctx context.Context, workflowID, runID string) (*WorkflowExecution, error)
	List(ctx context.Context, filter *types.WorkflowExecutionFilter) ([]*WorkflowExecution, error)
	Count(ctx context.Context, filter *types.WorkflowExecutionFilter) (int, error)
	ListAll(ctx context.Context, filter *types.WorkflowExecutionFilter) ([]*WorkflowExecution, error)
	Delete(ctx context.Context, id string) error
	UpdateStatus(ctx context.Context, workflowID, runID string, status types.WorkflowExecutionStatus, errorMessage string, endTime *time.Time, durationMs *int64) error
}
