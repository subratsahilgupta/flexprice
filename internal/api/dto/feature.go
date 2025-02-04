package dto

import (
	"context"
	"errors"

	"github.com/flexprice/flexprice/internal/domain/feature"
	"github.com/flexprice/flexprice/internal/types"
)

type CreateFeatureRequest struct {
	Name        string            `json:"name" binding:"required"`
	Description string            `json:"description"`
	LookupKey   string            `json:"lookup_key" binding:"required"`
	Type        types.FeatureType `json:"type" binding:"required"`
	MeterID     string            `json:"meter_id"`
	Metadata    types.Metadata    `json:"metadata"`
}

func (r *CreateFeatureRequest) Validate() error {
	if r.Name == "" {
		return errors.New("name is required")
	}

	if r.LookupKey == "" {
		return errors.New("lookup_key is required")
	}

	if err := r.Type.Validate(); err != nil {
		return err
	}

	if r.Type == types.FeatureTypeMetered {
		if r.MeterID == "" {
			return errors.New("meter_id is required for meter features")
		}
	}

	return nil
}

func (r *CreateFeatureRequest) ToFeature(ctx context.Context) (*feature.Feature, error) {
	return &feature.Feature{
		ID:          types.GenerateUUIDWithPrefix(types.UUID_PREFIX_FEATURE),
		Name:        r.Name,
		Description: r.Description,
		LookupKey:   r.LookupKey,
		Metadata:    r.Metadata,
		Type:        r.Type,
		MeterID:     r.MeterID,
		BaseModel:   types.GetDefaultBaseModel(ctx),
	}, nil
}

type UpdateFeatureRequest struct {
	Name        *string         `json:"name"`
	Description *string         `json:"description"`
	Metadata    *types.Metadata `json:"metadata"`
}

type FeatureResponse struct {
	*feature.Feature
	Meter *MeterResponse `json:"meter,omitempty"`
}

// ListFeaturesResponse represents a paginated list of features
type ListFeaturesResponse = types.ListResponse[*FeatureResponse]
