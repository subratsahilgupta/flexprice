package service

import (
	"context"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/settings"
)

// SettingsService defines the interface for managing settings operations
type SettingsService interface {

	// Key-based operations
	GetSettingByKey(ctx context.Context, key string) (*dto.SettingResponse, error)
	UpdateSettingByKey(ctx context.Context, key string, req *dto.UpdateSettingRequest) (*dto.SettingResponse, error)
	DeleteSettingByKey(ctx context.Context, key string) error
}

type settingsService struct {
	repo settings.Repository
}

func NewSettingsService(repo settings.Repository) SettingsService {
	return &settingsService{
		repo: repo,
	}
}

func (s *settingsService) GetSettingByKey(ctx context.Context, key string) (*dto.SettingResponse, error) {
	setting, err := s.repo.GetByKey(ctx, key)
	if err != nil {
		return nil, err
	}

	return dto.SettingFromDomain(setting), nil
}

func (s *settingsService) createSetting(ctx context.Context, req *dto.CreateSettingRequest) (*dto.SettingResponse, error) {
	setting := req.ToSetting(ctx)

	err := s.repo.Create(ctx, setting)
	if err != nil {
		return nil, err
	}

	return dto.SettingFromDomain(setting), nil
}

func (s *settingsService) updateSetting(ctx context.Context, setting *settings.Setting) (*dto.SettingResponse, error) {
	err := s.repo.Update(ctx, setting)
	if err != nil {
		return nil, err
	}

	return dto.SettingFromDomain(setting), nil
}

func (s *settingsService) UpdateSettingByKey(ctx context.Context, key string, req *dto.UpdateSettingRequest) (*dto.SettingResponse, error) {

	// STEP 1: Check if the setting exists
	setting, err := s.repo.GetByKey(ctx, key)

	if ent.IsNotFound(err) {
		createReq := &dto.CreateSettingRequest{
			Key:   key,
			Value: *req.Value,
		}
		return s.createSetting(ctx, createReq)
	}

	setting.Value = *req.Value
	return s.updateSetting(ctx, setting)
}

func (s *settingsService) DeleteSettingByKey(ctx context.Context, key string) error {
	err := s.repo.DeleteByKey(ctx, key)
	if err != nil {
		return err
	}
	return nil
}
