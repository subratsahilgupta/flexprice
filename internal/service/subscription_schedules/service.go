package subscription_schedules

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/types"
	"go.uber.org/zap"
)

// SubscriptionChangeExecutor defines the interface for executing subscription changes
type SubscriptionChangeExecutor interface {
	ExecuteSubscriptionChangeInternal(ctx context.Context, subscriptionID string, req dto.SubscriptionChangeRequest) (*dto.SubscriptionChangeExecuteResponse, error)
}

// Service handles all subscription schedule operations
type Service struct {
	scheduleRepo     subscription.SubscriptionScheduleRepository
	subscriptionRepo subscription.Repository
	changeExecutor   SubscriptionChangeExecutor
	logger           *zap.Logger
}

// NewService creates a new subscription schedule service
func NewService(
	scheduleRepo subscription.SubscriptionScheduleRepository,
	subscriptionRepo subscription.Repository,
	logger *zap.Logger,
) *Service {
	return &Service{
		scheduleRepo:     scheduleRepo,
		subscriptionRepo: subscriptionRepo,
		changeExecutor:   nil, // Will be set via SetChangeExecutor to avoid circular dependency
		logger:           logger,
	}
}

// SetChangeExecutor sets the subscription change executor (to avoid circular dependency)
func (s *Service) SetChangeExecutor(executor SubscriptionChangeExecutor) {
	s.changeExecutor = executor
}

// SchedulePlanChange schedules a plan change at period end
func (s *Service) SchedulePlanChange(
	ctx context.Context,
	subscriptionID string,
	config *subscription.PlanChangeConfiguration,
) (*subscription.SubscriptionSchedule, error) {
	// Get subscription to calculate period end
	sub, err := s.subscriptionRepo.Get(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	// Validate subscription is active
	if sub.SubscriptionStatus != types.SubscriptionStatusActive {
		return nil, fmt.Errorf("subscription must be active to schedule changes")
	}

	// Check for existing pending schedule
	existing, err := s.scheduleRepo.GetPendingBySubscriptionAndType(
		ctx,
		subscriptionID,
		types.SubscriptionScheduleChangeTypePlanChange,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing schedules: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("a plan change is already scheduled for this subscription")
	}

	// Create schedule
	schedule := &subscription.SubscriptionSchedule{
		ID:             types.GenerateUUIDWithPrefix(types.UUID_PREFIX_SUBSCRIPTION_SCHEDULE),
		SubscriptionID: subscriptionID,
		ScheduleType:   types.SubscriptionScheduleChangeTypePlanChange,
		ScheduledAt:    sub.CurrentPeriodEnd,
		Status:         types.ScheduleStatusPending,
		TenantID:       sub.TenantID,
		EnvironmentID:  sub.EnvironmentID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		CreatedBy:      types.GetUserID(ctx),
		UpdatedBy:      types.GetUserID(ctx),
		StatusColumn:   types.StatusPublished,
	}

	// Set configuration
	if err := schedule.SetPlanChangeConfig(config); err != nil {
		return nil, fmt.Errorf("failed to set configuration: %w", err)
	}

	// Save to database
	if err := s.scheduleRepo.Create(ctx, schedule); err != nil {
		return nil, fmt.Errorf("failed to create schedule: %w", err)
	}

	s.logger.Info("plan change scheduled in database",
		zap.String("schedule_id", schedule.ID),
		zap.String("subscription_id", subscriptionID),
		zap.Time("scheduled_at", schedule.ScheduledAt),
		zap.String("target_plan_id", config.TargetPlanID),
	)

	return schedule, nil
}

// ScheduleCancellation creates a schedule for subscription cancellation

// Cancel cancels a pending schedule
func (s *Service) Cancel(ctx context.Context, scheduleID string) error {
	schedule, err := s.scheduleRepo.Get(ctx, scheduleID)
	if err != nil {
		return fmt.Errorf("failed to get schedule: %w", err)
	}

	if !schedule.CanBeCancelled() {
		return fmt.Errorf("schedule cannot be cancelled (status: %s)", schedule.Status)
	}

	// NEW: Restore subscription state based on schedule type
	if err := s.restoreSubscriptionState(ctx, schedule); err != nil {
		s.logger.Warn("failed to restore subscription state",
			zap.String("schedule_id", scheduleID),
			zap.Error(err),
		)
		// Don't fail cancellation if restoration fails (logged for investigation)
	}

	schedule.Status = types.ScheduleStatusCancelled
	schedule.UpdatedAt = time.Now()
	schedule.UpdatedBy = types.GetUserID(ctx)

	if err := s.scheduleRepo.Update(ctx, schedule); err != nil {
		return fmt.Errorf("failed to cancel schedule: %w", err)
	}

	s.logger.Info("schedule cancelled in database",
		zap.String("schedule_id", scheduleID),
		zap.String("subscription_id", schedule.SubscriptionID),
		zap.String("schedule_type", string(schedule.ScheduleType)),
	)

	return nil
}

// CancelBySubscriptionAndType cancels a pending schedule by subscription ID and schedule type
func (s *Service) CancelBySubscriptionAndType(
	ctx context.Context,
	subscriptionID string,
	scheduleType types.SubscriptionScheduleChangeType,
) error {
	// Get the pending schedule for this subscription and type
	schedule, err := s.scheduleRepo.GetPendingBySubscriptionAndType(ctx, subscriptionID, scheduleType)
	if err != nil {
		return fmt.Errorf("failed to get pending schedule: %w", err)
	}

	if schedule == nil {
		return fmt.Errorf("no pending %s schedule found for subscription %s", scheduleType, subscriptionID)
	}

	// Use the existing Cancel method
	return s.Cancel(ctx, schedule.ID)
}

// CancelPendingForSubscription cancels all pending schedules for a subscription
func (s *Service) CancelPendingForSubscription(ctx context.Context, subscriptionID string) error {
	schedules, err := s.scheduleRepo.GetBySubscriptionID(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("failed to get schedules: %w", err)
	}

	for _, schedule := range schedules {
		if schedule.CanBeCancelled() {
			if err := s.Cancel(ctx, schedule.ID); err != nil {
				s.logger.Warn("failed to cancel schedule",
					zap.String("schedule_id", schedule.ID),
					zap.Error(err),
				)
			}
		}
	}

	return nil
}

// Get retrieves a schedule by ID
func (s *Service) Get(ctx context.Context, scheduleID string) (*subscription.SubscriptionSchedule, error) {
	return s.scheduleRepo.Get(ctx, scheduleID)
}

// GetBySubscriptionID retrieves all schedules for a subscription
func (s *Service) GetBySubscriptionID(ctx context.Context, subscriptionID string) ([]*subscription.SubscriptionSchedule, error) {
	return s.scheduleRepo.GetBySubscriptionID(ctx, subscriptionID)
}

// GetPendingBySubscriptionAndType retrieves a pending schedule by subscription ID and type
func (s *Service) GetPendingBySubscriptionAndType(
	ctx context.Context,
	subscriptionID string,
	scheduleType types.SubscriptionScheduleChangeType,
) (*subscription.SubscriptionSchedule, error) {
	return s.scheduleRepo.GetPendingBySubscriptionAndType(ctx, subscriptionID, scheduleType)
}

// List retrieves schedules based on filter
func (s *Service) List(ctx context.Context, filter *types.SubscriptionScheduleFilter) ([]*subscription.SubscriptionSchedule, error) {
	return s.scheduleRepo.List(ctx, filter)
}

// ExecuteSchedule executes a scheduled change (called by Temporal worker)
func (s *Service) ExecuteSchedule(ctx context.Context, scheduleID string) error {
	// Mark as executing
	if err := s.MarkAsExecuting(ctx, scheduleID); err != nil {
		return err
	}

	schedule, err := s.scheduleRepo.Get(ctx, scheduleID)
	if err != nil {
		return fmt.Errorf("failed to get schedule: %w", err)
	}

	// Validate it's still pending
	if schedule.Status != types.ScheduleStatusExecuting {
		return fmt.Errorf("schedule is not executing (status: %s)", schedule.Status)
	}

	// Execute based on type
	var executionError error
	var result interface{}

	switch schedule.ScheduleType {
	case types.SubscriptionScheduleChangeTypePlanChange:
		result, executionError = s.executePlanChange(ctx, schedule)
	default:
		executionError = fmt.Errorf("unsupported schedule type: %s", schedule.ScheduleType)
	}

	// Update status based on result
	if executionError != nil {
		if err := s.MarkAsFailed(ctx, scheduleID, executionError.Error()); err != nil {
			return fmt.Errorf("failed to mark as failed: %w (original error: %v)", err, executionError)
		}
		return executionError
	}

	// Mark as executed
	if err := s.MarkAsExecuted(ctx, scheduleID, result); err != nil {
		return fmt.Errorf("failed to mark as executed: %w", err)
	}

	return nil
}

// executePlanChange executes a plan change schedule
func (s *Service) executePlanChange(
	ctx context.Context,
	schedule *subscription.SubscriptionSchedule,
) (*subscription.PlanChangeResult, error) {
	config, err := schedule.GetPlanChangeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to parse plan change configuration: %w", err)
	}

	// Get current subscription
	sub, err := s.subscriptionRepo.Get(ctx, schedule.SubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	// Validate subscription state
	if sub.SubscriptionStatus != types.SubscriptionStatusActive {
		return nil, fmt.Errorf("subscription is not active (status: %s)", sub.SubscriptionStatus)
	}

	if sub.CancelAtPeriodEnd || sub.CancelledAt != nil {
		return nil, fmt.Errorf("subscription is cancelled or scheduled for cancellation")
	}

	if sub.PlanID == config.TargetPlanID {
		return nil, fmt.Errorf("subscription is already on target plan %s", config.TargetPlanID)
	}

	s.logger.Info("executing plan change",
		zap.String("schedule_id", schedule.ID),
		zap.String("subscription_id", schedule.SubscriptionID),
		zap.String("from_plan", sub.PlanID),
		zap.String("to_plan", config.TargetPlanID),
	)

	// Execute the subscription change if executor is available
	if s.changeExecutor == nil {
		s.logger.Warn("change executor not set, cannot execute plan change",
			zap.String("schedule_id", schedule.ID),
		)
		return nil, fmt.Errorf("subscription change executor not configured")
	}

	// Build change request from configuration
	changeRequest := dto.SubscriptionChangeRequest{
		TargetPlanID:       config.TargetPlanID,
		ProrationBehavior:  config.ProrationBehavior,
		BillingCadence:     config.BillingCadence,
		BillingPeriod:      config.BillingPeriod,
		BillingPeriodCount: config.BillingPeriodCount,
		BillingCycle:       config.BillingCycle,
		Metadata:           config.ChangeMetadata,
	}

	// Execute the change
	changeResponse, err := s.changeExecutor.ExecuteSubscriptionChangeInternal(ctx, schedule.SubscriptionID, changeRequest)
	if err != nil {
		s.logger.Error("failed to execute subscription change",
			zap.String("schedule_id", schedule.ID),
			zap.String("subscription_id", schedule.SubscriptionID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to execute subscription change: %w", err)
	}

	// Build result
	result := &subscription.PlanChangeResult{
		OldSubscriptionID: schedule.SubscriptionID,
		NewSubscriptionID: changeResponse.NewSubscription.ID,
		ChangeType:        string(changeResponse.ChangeType),
		EffectiveDate:     time.Now(),
	}

	s.logger.Info("plan change executed successfully",
		zap.String("schedule_id", schedule.ID),
		zap.String("subscription_id", schedule.SubscriptionID),
	)

	return result, nil
}

// executeCancellation executes a scheduled cancellation

// MarkAsExecuting updates schedule status to executing (called by Temporal worker)
func (s *Service) MarkAsExecuting(ctx context.Context, scheduleID string) error {
	schedule, err := s.scheduleRepo.Get(ctx, scheduleID)
	if err != nil {
		return fmt.Errorf("failed to get schedule: %w", err)
	}

	if schedule.Status != types.ScheduleStatusPending {
		return fmt.Errorf("schedule is not pending (status: %s)", schedule.Status)
	}

	schedule.Status = types.ScheduleStatusExecuting
	schedule.UpdatedAt = time.Now()

	return s.scheduleRepo.Update(ctx, schedule)
}

// MarkAsExecuted updates schedule status after successful execution
func (s *Service) MarkAsExecuted(ctx context.Context, scheduleID string, result interface{}) error {
	schedule, err := s.scheduleRepo.Get(ctx, scheduleID)
	if err != nil {
		return fmt.Errorf("failed to get schedule: %w", err)
	}

	now := time.Now()
	schedule.Status = types.ScheduleStatusExecuted
	schedule.ExecutedAt = &now
	schedule.UpdatedAt = now

	// Store result based on type
	if schedule.ScheduleType == types.SubscriptionScheduleChangeTypePlanChange {
		if planResult, ok := result.(*subscription.PlanChangeResult); ok {
			if err := schedule.SetPlanChangeResult(planResult); err != nil {
				s.logger.Warn("failed to store execution result", zap.Error(err))
			}
		}
	}

	// Update database first
	if err := s.scheduleRepo.Update(ctx, schedule); err != nil {
		return err
	}

	return nil
}

// MarkAsFailed updates schedule status after failed execution
func (s *Service) MarkAsFailed(ctx context.Context, scheduleID string, errorMsg string) error {
	schedule, err := s.scheduleRepo.Get(ctx, scheduleID)
	if err != nil {
		return fmt.Errorf("failed to get schedule: %w", err)
	}

	now := time.Now()
	schedule.Status = types.ScheduleStatusFailed
	schedule.ExecutedAt = &now
	schedule.ErrorMessage = &errorMsg
	schedule.UpdatedAt = now

	// Update database first
	if err := s.scheduleRepo.Update(ctx, schedule); err != nil {
		return err
	}

	return nil
}

// restoreSubscriptionState restores subscription to its pre-schedule state
func (s *Service) restoreSubscriptionState(
	ctx context.Context,
	schedule *subscription.SubscriptionSchedule,
) error {

	switch schedule.ScheduleType {
	case types.SubscriptionScheduleChangeTypePlanChange:
		// For plan change: just cancel schedule, subscription remains unchanged
		s.logger.Info("plan change schedule cancelled, no state restoration needed",
			zap.String("schedule_id", schedule.ID),
		)
		return nil

	case types.SubscriptionScheduleChangeTypeCancellation:
		// For cancellation: restore subscription fields that were changed
		return s.restoreCancellationState(ctx, schedule)

	default:
		// Other types: no restoration needed
		return nil
	}
}

// restoreCancellationState restores subscription state when a cancellation schedule is cancelled
func (s *Service) restoreCancellationState(
	ctx context.Context,
	schedule *subscription.SubscriptionSchedule,
) error {
	// Parse the cancellation configuration to get original state
	config, err := schedule.GetCancellationConfig()
	if err != nil {
		return fmt.Errorf("failed to parse cancellation configuration: %w", err)
	}

	// Get the subscription
	sub, err := s.subscriptionRepo.Get(ctx, schedule.SubscriptionID)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	s.logger.Info("restoring subscription state after cancellation schedule cancellation",
		zap.String("schedule_id", schedule.ID),
		zap.String("subscription_id", sub.ID),
		zap.Bool("current_cancel_at_period_end", sub.CancelAtPeriodEnd),
	)

	// Restore the original state
	sub.CancelAtPeriodEnd = config.OriginalCancelAtPeriodEnd
	sub.CancelAt = config.OriginalCancelAt
	sub.EndDate = config.OriginalEndDate
	sub.CancelledAt = config.OriginalCancelledAt

	// Update the subscription
	if err := s.subscriptionRepo.Update(ctx, sub); err != nil {
		return fmt.Errorf("failed to restore subscription state: %w", err)
	}

	s.logger.Info("subscription state restored successfully",
		zap.String("schedule_id", schedule.ID),
		zap.String("subscription_id", sub.ID),
		zap.Bool("restored_cancel_at_period_end", sub.CancelAtPeriodEnd),
	)

	return nil
}
