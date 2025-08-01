package service

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/security"
	"github.com/flexprice/flexprice/internal/types"
)

// ConnectionService defines the interface for connection operations
type ConnectionService interface {
	CreateConnection(ctx context.Context, req dto.CreateConnectionRequest) (*dto.ConnectionResponse, error)
	GetConnection(ctx context.Context, id string) (*dto.ConnectionResponse, error)
	GetConnections(ctx context.Context, filter *types.ConnectionFilter) (*dto.ListConnectionsResponse, error)
	UpdateConnection(ctx context.Context, id string, req dto.UpdateConnectionRequest) (*dto.ConnectionResponse, error)
	DeleteConnection(ctx context.Context, id string) error
}

type connectionService struct {
	ServiceParams
	encryptionService security.EncryptionService
}

// NewConnectionService creates a new connection service
func NewConnectionService(params ServiceParams, encryptionService security.EncryptionService) ConnectionService {
	return &connectionService{
		ServiceParams:     params,
		encryptionService: encryptionService,
	}
}

// encryptMetadata encrypts the structured metadata
func (s *connectionService) encryptMetadata(metadata types.ConnectionMetadata) (types.ConnectionMetadata, error) {
	encryptedMetadata := metadata

	switch metadata.Type {
	case types.ConnectionMetadataTypeStripe:
		if metadata.Stripe != nil {
			encryptedPublishableKey, err := s.encryptionService.Encrypt(metadata.Stripe.PublishableKey)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			encryptedSecretKey, err := s.encryptionService.Encrypt(metadata.Stripe.SecretKey)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			encryptedWebhookSecret, err := s.encryptionService.Encrypt(metadata.Stripe.WebhookSecret)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}

			encryptedMetadata.Stripe = &types.StripeConnectionMetadata{
				PublishableKey: encryptedPublishableKey,
				SecretKey:      encryptedSecretKey,
				WebhookSecret:  encryptedWebhookSecret,
				AccountID:      metadata.Stripe.AccountID, // Account ID is not sensitive
			}
		}
	case types.ConnectionMetadataTypeRazorpay:
		if metadata.Razorpay != nil {
			encryptedKeyID, err := s.encryptionService.Encrypt(metadata.Razorpay.KeyID)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			encryptedKeySecret, err := s.encryptionService.Encrypt(metadata.Razorpay.KeySecret)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			encryptedWebhookSecret, err := s.encryptionService.Encrypt(metadata.Razorpay.WebhookSecret)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}

			encryptedMetadata.Razorpay = &types.RazorpayConnectionMetadata{
				KeyID:         encryptedKeyID,
				KeySecret:     encryptedKeySecret,
				WebhookSecret: encryptedWebhookSecret,
				AccountID:     metadata.Razorpay.AccountID, // Account ID is not sensitive
			}
		}
	case types.ConnectionMetadataTypePayPal:
		if metadata.PayPal != nil {
			encryptedClientID, err := s.encryptionService.Encrypt(metadata.PayPal.ClientID)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			encryptedClientSecret, err := s.encryptionService.Encrypt(metadata.PayPal.ClientSecret)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			encryptedWebhookID, err := s.encryptionService.Encrypt(metadata.PayPal.WebhookID)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}

			encryptedMetadata.PayPal = &types.PayPalConnectionMetadata{
				ClientID:     encryptedClientID,
				ClientSecret: encryptedClientSecret,
				WebhookID:    encryptedWebhookID,
				AccountID:    metadata.PayPal.AccountID, // Account ID is not sensitive
			}
		}
	case types.ConnectionMetadataTypeGeneric:
		// For generic metadata, encrypt string values in the data map
		if metadata.Generic != nil {
			encryptedData := make(map[string]interface{})
			for key, value := range metadata.Generic.Data {
				if strValue, ok := value.(string); ok {
					encryptedValue, err := s.encryptionService.Encrypt(strValue)
					if err != nil {
						return types.ConnectionMetadata{}, err
					}
					encryptedData[key] = encryptedValue
				} else {
					encryptedData[key] = value
				}
			}
			encryptedMetadata.Generic = &types.GenericConnectionMetadata{
				Data: encryptedData,
			}
		}
	}

	return encryptedMetadata, nil
}

// decryptMetadata decrypts the structured metadata
func (s *connectionService) decryptMetadata(metadata types.ConnectionMetadata) (types.ConnectionMetadata, error) {
	decryptedMetadata := metadata

	switch metadata.Type {
	case types.ConnectionMetadataTypeStripe:
		if metadata.Stripe != nil {
			decryptedPublishableKey, err := s.encryptionService.Decrypt(metadata.Stripe.PublishableKey)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			decryptedSecretKey, err := s.encryptionService.Decrypt(metadata.Stripe.SecretKey)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			decryptedWebhookSecret, err := s.encryptionService.Decrypt(metadata.Stripe.WebhookSecret)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}

			decryptedMetadata.Stripe = &types.StripeConnectionMetadata{
				PublishableKey: decryptedPublishableKey,
				SecretKey:      decryptedSecretKey,
				WebhookSecret:  decryptedWebhookSecret,
				AccountID:      metadata.Stripe.AccountID, // Account ID is not sensitive
			}
		}
	case types.ConnectionMetadataTypeRazorpay:
		if metadata.Razorpay != nil {
			decryptedKeyID, err := s.encryptionService.Decrypt(metadata.Razorpay.KeyID)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			decryptedKeySecret, err := s.encryptionService.Decrypt(metadata.Razorpay.KeySecret)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			decryptedWebhookSecret, err := s.encryptionService.Decrypt(metadata.Razorpay.WebhookSecret)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}

			decryptedMetadata.Razorpay = &types.RazorpayConnectionMetadata{
				KeyID:         decryptedKeyID,
				KeySecret:     decryptedKeySecret,
				WebhookSecret: decryptedWebhookSecret,
				AccountID:     metadata.Razorpay.AccountID, // Account ID is not sensitive
			}
		}
	case types.ConnectionMetadataTypePayPal:
		if metadata.PayPal != nil {
			decryptedClientID, err := s.encryptionService.Decrypt(metadata.PayPal.ClientID)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			decryptedClientSecret, err := s.encryptionService.Decrypt(metadata.PayPal.ClientSecret)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}
			decryptedWebhookID, err := s.encryptionService.Decrypt(metadata.PayPal.WebhookID)
			if err != nil {
				return types.ConnectionMetadata{}, err
			}

			decryptedMetadata.PayPal = &types.PayPalConnectionMetadata{
				ClientID:     decryptedClientID,
				ClientSecret: decryptedClientSecret,
				WebhookID:    decryptedWebhookID,
				AccountID:    metadata.PayPal.AccountID, // Account ID is not sensitive
			}
		}
	case types.ConnectionMetadataTypeGeneric:
		// For generic metadata, decrypt string values in the data map
		if metadata.Generic != nil {
			decryptedData := make(map[string]interface{})
			for key, value := range metadata.Generic.Data {
				if strValue, ok := value.(string); ok {
					decryptedValue, err := s.encryptionService.Decrypt(strValue)
					if err != nil {
						return types.ConnectionMetadata{}, err
					}
					decryptedData[key] = decryptedValue
				} else {
					decryptedData[key] = value
				}
			}
			decryptedMetadata.Generic = &types.GenericConnectionMetadata{
				Data: decryptedData,
			}
		}
	}

	return decryptedMetadata, nil
}

func (s *connectionService) CreateConnection(ctx context.Context, req dto.CreateConnectionRequest) (*dto.ConnectionResponse, error) {
	s.Logger.Debugw("creating connection",
		"name", req.Name,
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

	// Encrypt metadata
	encryptedMetadata, err := s.encryptMetadata(conn.Metadata)
	if err != nil {
		s.Logger.Errorw("failed to encrypt metadata", "error", err)
		return nil, err
	}
	conn.Metadata = encryptedMetadata

	// Create the connection
	if err := s.ConnectionRepo.Create(ctx, conn); err != nil {
		s.Logger.Errorw("failed to create connection", "error", err)
		return nil, err
	}

	s.Logger.Infow("connection created successfully", "connection_id", conn.ID)
	return dto.ToConnectionResponse(conn), nil
}

func (s *connectionService) GetConnection(ctx context.Context, id string) (*dto.ConnectionResponse, error) {
	s.Logger.Debugw("getting connection", "connection_id", id)

	conn, err := s.ConnectionRepo.Get(ctx, id)
	if err != nil {
		s.Logger.Errorw("failed to get connection", "error", err, "connection_id", id)
		return nil, err
	}

	// Decrypt metadata
	decryptedMetadata, err := s.decryptMetadata(conn.Metadata)
	if err != nil {
		s.Logger.Errorw("failed to decrypt metadata", "error", err)
		return nil, err
	}
	conn.Metadata = decryptedMetadata

	return dto.ToConnectionResponse(conn), nil
}

func (s *connectionService) GetConnections(ctx context.Context, filter *types.ConnectionFilter) (*dto.ListConnectionsResponse, error) {
	s.Logger.Debugw("getting connections", "filter", filter)

	connections, err := s.ConnectionRepo.List(ctx, filter)
	if err != nil {
		s.Logger.Errorw("failed to get connections", "error", err)
		return nil, err
	}

	// Decrypt metadata for all connections
	for _, conn := range connections {
		decryptedMetadata, err := s.decryptMetadata(conn.Metadata)
		if err != nil {
			s.Logger.Errorw("failed to decrypt metadata for connection", "error", err, "connection_id", conn.ID)
			return nil, err
		}
		conn.Metadata = decryptedMetadata
	}

	total, err := s.ConnectionRepo.Count(ctx, filter)
	if err != nil {
		s.Logger.Errorw("failed to count connections", "error", err)
		return nil, err
	}

	responses := dto.ToConnectionResponses(connections)
	return &dto.ListConnectionsResponse{
		Connections: responses,
		Total:       total,
		Limit:       filter.GetLimit(),
		Offset:      filter.GetOffset(),
	}, nil
}

func (s *connectionService) UpdateConnection(ctx context.Context, id string, req dto.UpdateConnectionRequest) (*dto.ConnectionResponse, error) {
	s.Logger.Debugw("updating connection", "connection_id", id)

	// Get existing connection
	conn, err := s.ConnectionRepo.Get(ctx, id)
	if err != nil {
		s.Logger.Errorw("failed to get connection for update", "error", err, "connection_id", id)
		return nil, err
	}

	// Update simple fields if provided
	if req.Name != "" {
		conn.Name = req.Name
	}
	if req.ProviderType != "" {
		conn.ProviderType = req.ProviderType
	}

	// Handle metadata update if provided
	if req.Metadata.Type != "" {
		// Validate the new metadata
		if err := req.Metadata.Validate(); err != nil {
			s.Logger.Errorw("invalid metadata in update request", "error", err)
			return nil, err
		}

		// Encrypt the new metadata
		encryptedMetadata, err := s.encryptMetadata(req.Metadata)
		if err != nil {
			s.Logger.Errorw("failed to encrypt metadata during update", "error", err)
			return nil, err
		}
		conn.Metadata = encryptedMetadata
	}

	conn.UpdatedAt = time.Now()
	conn.UpdatedBy = types.GetUserID(ctx)

	// Update the connection
	if err := s.ConnectionRepo.Update(ctx, conn); err != nil {
		s.Logger.Errorw("failed to update connection", "error", err, "connection_id", id)
		return nil, err
	}

	// Decrypt metadata for response
	decryptedMetadata, err := s.decryptMetadata(conn.Metadata)
	if err != nil {
		s.Logger.Errorw("failed to decrypt metadata after update", "error", err)
		return nil, err
	}
	conn.Metadata = decryptedMetadata

	s.Logger.Infow("connection updated successfully", "connection_id", conn.ID)
	return dto.ToConnectionResponse(conn), nil
}

func (s *connectionService) DeleteConnection(ctx context.Context, id string) error {
	s.Logger.Debugw("deleting connection", "connection_id", id)

	// Get existing connection
	conn, err := s.ConnectionRepo.Get(ctx, id)
	if err != nil {
		s.Logger.Errorw("failed to get connection for deletion", "error", err, "connection_id", id)
		return err
	}

	conn.UpdatedAt = time.Now()
	conn.UpdatedBy = types.GetUserID(ctx)

	// Delete the connection
	if err := s.ConnectionRepo.Delete(ctx, conn); err != nil {
		s.Logger.Errorw("failed to delete connection", "error", err, "connection_id", id)
		return err
	}

	s.Logger.Infow("connection deleted successfully", "connection_id", conn.ID)
	return nil
}
