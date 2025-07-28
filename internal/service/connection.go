package service

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/connection"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
)

// ConnectionService defines the interface for connection operations
type ConnectionService interface {
	CreateConnection(ctx context.Context, req dto.CreateConnectionRequest) (*dto.ConnectionResponse, error)
	GetConnection(ctx context.Context, id string) (*dto.ConnectionResponse, error)
	GetConnectionByCode(ctx context.Context, connectionCode string) (*dto.ConnectionResponse, error)
	GetConnections(ctx context.Context, filter *types.ConnectionFilter) (*dto.ListConnectionsResponse, error)
	UpdateConnection(ctx context.Context, id string, req dto.UpdateConnectionRequest) (*dto.ConnectionResponse, error)
	DeleteConnection(ctx context.Context, id string) error
}

type connectionService struct {
	repo connection.Repository
	log  *logger.Logger
}

// NewConnectionService creates a new connection service
func NewConnectionService(repo connection.Repository, log *logger.Logger) ConnectionService {
	return &connectionService{
		repo: repo,
		log:  log,
	}
}

func (s *connectionService) CreateConnection(ctx context.Context, req dto.CreateConnectionRequest) (*dto.ConnectionResponse, error) {
	s.log.Debugw("creating connection",
		"connection_code", req.ConnectionCode,
		"provider_type", req.ProviderType,
	)

	// Validate the request
	if err := req.ProviderType.Validate(); err != nil {
		return nil, err
	}

	// Convert DTO to domain model
	conn := req.ToConnection()
	
	// Set required fields
	conn.ID = types.GenerateUUIDWithPrefix(types.UUID_PREFIX_CONNECTION)
	conn.TenantID = types.GetTenantID(ctx)
	conn.EnvironmentID = types.GetEnvironmentID(ctx)
	conn.Status = types.StatusPublished
	conn.CreatedAt = time.Now()
	conn.UpdatedAt = time.Now()
	conn.CreatedBy = types.GetUserID(ctx)
	conn.UpdatedBy = types.GetUserID(ctx)

	// Create the connection
	if err := s.repo.Create(ctx, conn); err != nil {
		s.log.Errorw("failed to create connection", "error", err)
		return nil, err
	}

	s.log.Infow("connection created successfully", "connection_id", conn.ID)
	return dto.ToConnectionResponse(conn), nil
}

func (s *connectionService) GetConnection(ctx context.Context, id string) (*dto.ConnectionResponse, error) {
	s.log.Debugw("getting connection", "connection_id", id)

	conn, err := s.repo.Get(ctx, id)
	if err != nil {
		s.log.Errorw("failed to get connection", "error", err, "connection_id", id)
		return nil, err
	}

	return dto.ToConnectionResponse(conn), nil
}

func (s *connectionService) GetConnectionByCode(ctx context.Context, connectionCode string) (*dto.ConnectionResponse, error) {
	s.log.Debugw("getting connection by code", "connection_code", connectionCode)

	conn, err := s.repo.GetByConnectionCode(ctx, connectionCode)
	if err != nil {
		s.log.Errorw("failed to get connection by code", "error", err, "connection_code", connectionCode)
		return nil, err
	}

	return dto.ToConnectionResponse(conn), nil
}

func (s *connectionService) GetConnections(ctx context.Context, filter *types.ConnectionFilter) (*dto.ListConnectionsResponse, error) {
	s.log.Debugw("getting connections", "filter", filter)

	if filter == nil {
		filter = types.NewConnectionFilter()
	}

	// Validate the filter
	if err := filter.Validate(); err != nil {
		return nil, err
	}

	// Get connections and count
	connections, err := s.repo.List(ctx, filter)
	if err != nil {
		s.log.Errorw("failed to list connections", "error", err)
		return nil, err
	}

	total, err := s.repo.Count(ctx, filter)
	if err != nil {
		s.log.Errorw("failed to count connections", "error", err)
		return nil, err
	}

	return &dto.ListConnectionsResponse{
		Connections: dto.ToConnectionResponses(connections),
		Total:       total,
		Limit:       filter.GetLimit(),
		Offset:      filter.GetOffset(),
	}, nil
}

func (s *connectionService) UpdateConnection(ctx context.Context, id string, req dto.UpdateConnectionRequest) (*dto.ConnectionResponse, error) {
	s.log.Debugw("updating connection", "connection_id", id)

	// Get existing connection
	conn, err := s.repo.Get(ctx, id)
	if err != nil {
		s.log.Errorw("failed to get connection for update", "error", err, "connection_id", id)
		return nil, err
	}

	// Update fields if provided
	if req.Name != "" {
		conn.Name = req.Name
	}
	if req.Description != "" {
		conn.Description = req.Description
	}
	if req.ConnectionCode != "" {
		conn.ConnectionCode = req.ConnectionCode
	}
	if req.ProviderType != "" {
		if err := req.ProviderType.Validate(); err != nil {
			return nil, err
		}
		conn.ProviderType = req.ProviderType
	}
	if req.Metadata != nil {
		conn.Metadata = req.Metadata
	}
	if req.SecretID != "" {
		conn.SecretID = req.SecretID
	}

	// Update metadata
	conn.UpdatedAt = time.Now()
	conn.UpdatedBy = types.GetUserID(ctx)

	// Save the updated connection
	if err := s.repo.Update(ctx, conn); err != nil {
		s.log.Errorw("failed to update connection", "error", err, "connection_id", id)
		return nil, err
	}

	s.log.Infow("connection updated successfully", "connection_id", id)
	return dto.ToConnectionResponse(conn), nil
}

func (s *connectionService) DeleteConnection(ctx context.Context, id string) error {
	s.log.Debugw("deleting connection", "connection_id", id)

	// Get existing connection
	conn, err := s.repo.Get(ctx, id)
	if err != nil {
		s.log.Errorw("failed to get connection for deletion", "error", err, "connection_id", id)
		return err
	}

	// Update metadata for soft delete
	conn.UpdatedAt = time.Now()
	conn.UpdatedBy = types.GetUserID(ctx)

	// Delete the connection (soft delete)
	if err := s.repo.Delete(ctx, conn); err != nil {
		s.log.Errorw("failed to delete connection", "error", err, "connection_id", id)
		return err
	}

	s.log.Infow("connection deleted successfully", "connection_id", id)
	return nil
}
