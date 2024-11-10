package meter

import (
	"context"
	"fmt"
)

type Service interface {
	CreateMeter(ctx context.Context, meter *Meter) error
	GetMeter(ctx context.Context, id string) (*Meter, error)
	GetAllMeters(ctx context.Context) ([]*Meter, error)
	DisableMeter(ctx context.Context, id string) error
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) CreateMeter(ctx context.Context, meter *Meter) error {
	// If meter is nil, return error
	if meter == nil {
		return fmt.Errorf("meter cannot be nil")
	}

	// Use NewMeter if ID is not provided
	if meter.ID == "" {
		newMeter := NewMeter("", meter.CreatedBy)
		// Copy the provided meter fields to the new meter
		newMeter.TenantID = meter.TenantID
		newMeter.Filters = meter.Filters
		newMeter.Aggregation = meter.Aggregation
		newMeter.WindowSize = meter.WindowSize
		meter = newMeter
	}

	if err := meter.Validate(); err != nil {
		return fmt.Errorf("validate meter: %w", err)
	}

	return s.repo.CreateMeter(ctx, meter)
}

func (s *service) GetMeter(ctx context.Context, id string) (*Meter, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	return s.repo.GetMeter(ctx, id)
}

func (s *service) GetAllMeters(ctx context.Context) ([]*Meter, error) {
	return s.repo.GetAllMeters(ctx)
}

func (s *service) DisableMeter(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	return s.repo.DisableMeter(ctx, id)
}
