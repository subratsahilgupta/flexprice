package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/entityintegrationmapping"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

type EntityIntegrationMappingService interface {
	CreateEntityIntegrationMapping(ctx context.Context, req dto.CreateEntityIntegrationMappingRequest) (*dto.EntityIntegrationMappingResponse, error)
	GetEntityIntegrationMapping(ctx context.Context, id string) (*dto.EntityIntegrationMappingResponse, error)
	GetEntityIntegrationMappings(ctx context.Context, filter *types.EntityIntegrationMappingFilter) (*dto.ListEntityIntegrationMappingsResponse, error)
	UpdateEntityIntegrationMapping(ctx context.Context, id string, req dto.UpdateEntityIntegrationMappingRequest) (*dto.EntityIntegrationMappingResponse, error)
	DeleteEntityIntegrationMapping(ctx context.Context, id string) error
	GetByEntityAndProvider(ctx context.Context, entityID string, entityType string, providerType string) (*dto.EntityIntegrationMappingResponse, error)
	GetByProviderEntity(ctx context.Context, providerType string, providerEntityID string) (*dto.EntityIntegrationMappingResponse, error)
}

type entityIntegrationMappingService struct {
	ServiceParams
}

func NewEntityIntegrationMappingService(params ServiceParams) EntityIntegrationMappingService {
	return &entityIntegrationMappingService{
		ServiceParams: params,
	}
}

func (s *entityIntegrationMappingService) CreateEntityIntegrationMapping(ctx context.Context, req dto.CreateEntityIntegrationMappingRequest) (*dto.EntityIntegrationMappingResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	mapping := req.ToEntityIntegrationMapping(ctx)

	// Validate the mapping
	if err := entityintegrationmapping.Validate(mapping); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid entity integration mapping data").
			Mark(ierr.ErrValidation)
	}

	if err := s.EntityIntegrationMappingRepo.Create(ctx, mapping); err != nil {
		// No need to wrap the error as the repository already returns properly formatted errors
		return nil, err
	}

	return &dto.EntityIntegrationMappingResponse{EntityIntegrationMapping: mapping}, nil
}

func (s *entityIntegrationMappingService) GetEntityIntegrationMapping(ctx context.Context, id string) (*dto.EntityIntegrationMappingResponse, error) {
	if id == "" {
		return nil, ierr.NewError("entity integration mapping ID is required").
			WithHint("Entity integration mapping ID is required").
			Mark(ierr.ErrValidation)
	}

	mapping, err := s.EntityIntegrationMappingRepo.Get(ctx, id)
	if err != nil {
		// No need to wrap the error as the repository already returns properly formatted errors
		return nil, err
	}

	return &dto.EntityIntegrationMappingResponse{EntityIntegrationMapping: mapping}, nil
}

func (s *entityIntegrationMappingService) GetEntityIntegrationMappings(ctx context.Context, filter *types.EntityIntegrationMappingFilter) (*dto.ListEntityIntegrationMappingsResponse, error) {
	if filter == nil {
		filter = &types.EntityIntegrationMappingFilter{
			QueryFilter: types.NewDefaultQueryFilter(),
		}
	}

	if err := filter.Validate(); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid filter parameters").
			Mark(ierr.ErrValidation)
	}

	mappings, err := s.EntityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		// No need to wrap the error as the repository already returns properly formatted errors
		return nil, err
	}

	total, err := s.EntityIntegrationMappingRepo.Count(ctx, filter)
	if err != nil {
		// No need to wrap the error as the repository already returns properly formatted errors
		return nil, err
	}

	response := make([]*dto.EntityIntegrationMappingResponse, 0, len(mappings))
	for _, m := range mappings {
		response = append(response, &dto.EntityIntegrationMappingResponse{EntityIntegrationMapping: m})
	}

	return &dto.ListEntityIntegrationMappingsResponse{
		Items:      response,
		Pagination: types.NewPaginationResponse(total, filter.GetLimit(), filter.GetOffset()),
	}, nil
}

func (s *entityIntegrationMappingService) UpdateEntityIntegrationMapping(ctx context.Context, id string, req dto.UpdateEntityIntegrationMappingRequest) (*dto.EntityIntegrationMappingResponse, error) {
	if id == "" {
		return nil, ierr.NewError("entity integration mapping ID is required").
			WithHint("Entity integration mapping ID is required").
			Mark(ierr.ErrValidation)
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Get existing mapping
	existingMapping, err := s.EntityIntegrationMappingRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Update fields if provided
	if req.EntityID != nil && *req.EntityID != "" {
		existingMapping.EntityID = *req.EntityID
	}
	if req.EntityType != nil && *req.EntityType != "" {
		existingMapping.EntityType = types.IntegrationEntityType(*req.EntityType)
	}
	if req.ProviderType != nil && *req.ProviderType != "" {
		existingMapping.ProviderType = *req.ProviderType
	}
	if req.ProviderEntityID != nil && *req.ProviderEntityID != "" {
		existingMapping.ProviderEntityID = *req.ProviderEntityID
	}
	if req.Metadata != nil {
		existingMapping.Metadata = req.Metadata
	}

	// Validate the updated mapping
	if err := entityintegrationmapping.Validate(existingMapping); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid entity integration mapping data").
			Mark(ierr.ErrValidation)
	}

	if err := s.EntityIntegrationMappingRepo.Update(ctx, existingMapping); err != nil {
		// No need to wrap the error as the repository already returns properly formatted errors
		return nil, err
	}

	return &dto.EntityIntegrationMappingResponse{EntityIntegrationMapping: existingMapping}, nil
}

func (s *entityIntegrationMappingService) DeleteEntityIntegrationMapping(ctx context.Context, id string) error {
	if id == "" {
		return ierr.NewError("entity integration mapping ID is required").
			WithHint("Entity integration mapping ID is required").
			Mark(ierr.ErrValidation)
	}

	// Get existing mapping
	mapping, err := s.EntityIntegrationMappingRepo.Get(ctx, id)
	if err != nil {
		return err
	}

	if err := s.EntityIntegrationMappingRepo.Delete(ctx, mapping); err != nil {
		// No need to wrap the error as the repository already returns properly formatted errors
		return err
	}

	return nil
}

func (s *entityIntegrationMappingService) GetByEntityAndProvider(ctx context.Context, entityID string, entityType string, providerType string) (*dto.EntityIntegrationMappingResponse, error) {
	if entityID == "" {
		return nil, ierr.NewError("entity ID is required").
			WithHint("Entity ID is required").
			Mark(ierr.ErrValidation)
	}

	if entityType == "" {
		return nil, ierr.NewError("entity type is required").
			WithHint("Entity type is required").
			Mark(ierr.ErrValidation)
	}

	if providerType == "" {
		return nil, ierr.NewError("provider type is required").
			WithHint("Provider type is required").
			Mark(ierr.ErrValidation)
	}

	// Create a filter to find the mapping
	filter := &types.EntityIntegrationMappingFilter{
		QueryFilter:  types.NewDefaultQueryFilter(),
		EntityID:     entityID,
		EntityType:   types.IntegrationEntityType(entityType),
		ProviderType: providerType,
	}

	mappings, err := s.EntityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	if len(mappings) == 0 {
		return nil, ierr.NewError("entity integration mapping not found").
			WithHint("No mapping found for the specified entity and provider").
			Mark(ierr.ErrNotFound)
	}

	// Return the first mapping found
	return &dto.EntityIntegrationMappingResponse{EntityIntegrationMapping: mappings[0]}, nil
}

func (s *entityIntegrationMappingService) GetByProviderEntity(ctx context.Context, providerType string, providerEntityID string) (*dto.EntityIntegrationMappingResponse, error) {
	if providerType == "" {
		return nil, ierr.NewError("provider type is required").
			WithHint("Provider type is required").
			Mark(ierr.ErrValidation)
	}

	if providerEntityID == "" {
		return nil, ierr.NewError("provider entity ID is required").
			WithHint("Provider entity ID is required").
			Mark(ierr.ErrValidation)
	}

	// Create a filter to find the mapping
	filter := &types.EntityIntegrationMappingFilter{
		QueryFilter:      types.NewDefaultQueryFilter(),
		ProviderType:     providerType,
		ProviderEntityID: providerEntityID,
	}

	mappings, err := s.EntityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	if len(mappings) == 0 {
		return nil, ierr.NewError("entity integration mapping not found").
			WithHint("No mapping found for the specified provider entity").
			Mark(ierr.ErrNotFound)
	}

	// Return the first mapping found
	return &dto.EntityIntegrationMappingResponse{EntityIntegrationMapping: mappings[0]}, nil
}
