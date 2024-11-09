package meter

import "context"

type Service interface {
	CreateMeter(ctx context.Context, meter Meter) error
	GetAllMeters(ctx context.Context) ([]Meter, error)
	DisableMeter(ctx context.Context, id string) error
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) CreateMeter(ctx context.Context, meter Meter) error {
	// Add business logic if needed
	return s.repo.CreateMeter(ctx, meter)
}

func (s *service) GetMeter(ctx context.Context, id string) (Meter, error) {
	return s.repo.GetMeter(ctx, id)
}

func (s *service) GetAllMeters(ctx context.Context) ([]Meter, error) {
	return s.repo.GetAllMeters(ctx)
}

func (s *service) DisableMeter(ctx context.Context, id string) error {
	return s.repo.DisableMeter(ctx, id)
}
