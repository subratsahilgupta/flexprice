package service

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/repository/postgres"
)

type MeterService interface {
	CreateMeter(ctx context.Context, meter *meter.Meter) error
	GetMeter(ctx context.Context, id string) (*meter.Meter, error)
	GetAllMeters(ctx context.Context) ([]*meter.Meter, error)
	DisableMeter(ctx context.Context, id string) error
}

type meterService struct {
	meterRepo postgres.MeterRepository
}

func NewMeterService(meterRepo meter.Repository) MeterService {
	return &meterService{meterRepo: meterRepo}
}

func (s *meterService) CreateMeter(ctx context.Context, meterObject *meter.Meter) error {
	// If meter is nil, return error
	if meterObject == nil {
		return fmt.Errorf("meter cannot be nil")
	}

	// Use NewMeter if ID is not provided
	if meterObject.ID == "" {
		newMeter := meter.NewMeter("", meterObject.CreatedBy)
		newMeter.TenantID = meterObject.TenantID
		newMeter.EventName = meterObject.EventName
		newMeter.Aggregation = meterObject.Aggregation
		newMeter.WindowSize = meterObject.WindowSize
		meterObject = newMeter
	}

	if err := meterObject.Validate(); err != nil {
		return fmt.Errorf("validate meter: %w", err)
	}

	return s.meterRepo.CreateMeter(ctx, meterObject)
}

func (s *meterService) GetMeter(ctx context.Context, id string) (*meter.Meter, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	return s.meterRepo.GetMeter(ctx, id)
}

func (s *meterService) GetAllMeters(ctx context.Context) ([]*meter.Meter, error) {
	return s.meterRepo.GetAllMeters(ctx)
}

func (s *meterService) DisableMeter(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	return s.meterRepo.DisableMeter(ctx, id)
}
