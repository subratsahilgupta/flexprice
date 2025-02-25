package types

import (
	"github.com/flexprice/flexprice/internal/errors"
	"github.com/samber/lo"
)

type SecretType string

// Secret types
const (
	SecretTypePrivateKey     SecretType = "private_key"
	SecretTypePublishableKey SecretType = "publishable_key"
	SecretTypeIntegration    SecretType = "integration"
)

func (t SecretType) Validate() error {
	allowedSecretTypes := []SecretType{SecretTypePrivateKey, SecretTypePublishableKey, SecretTypeIntegration}
	if !lo.Contains(allowedSecretTypes, t) {
		return errors.New(errors.ErrCodeValidation, "invalid secret type")
	}
	return nil
}

type SecretProvider string

// Provider types
const (
	SecretProviderFlexPrice SecretProvider = "flexprice"
	SecretProviderStripe    SecretProvider = "stripe"
	SecretProviderRazorpay  SecretProvider = "razorpay"
)

func (p SecretProvider) Validate() error {
	allowedSecretProviders := []SecretProvider{SecretProviderFlexPrice, SecretProviderStripe, SecretProviderRazorpay}
	if !lo.Contains(allowedSecretProviders, p) {
		return errors.New(errors.ErrCodeValidation, "invalid secret provider")
	}
	return nil
}

// SecretFilter defines the filter criteria for secrets
type SecretFilter struct {
	*QueryFilter
	*TimeRangeFilter

	Type     *SecretType     `json:"type,omitempty" form:"type"`
	Provider *SecretProvider `json:"provider,omitempty" form:"provider"`
	Prefix   *string         `json:"prefix,omitempty" form:"prefix"`
}

func NewSecretFilter() *SecretFilter {
	return &SecretFilter{
		QueryFilter: NewNoLimitQueryFilter(),
	}
}

func NewNoLimitSecretFilter() *SecretFilter {
	return &SecretFilter{
		QueryFilter: NewNoLimitQueryFilter(),
	}
}

func (f *SecretFilter) Validate() error {
	if f == nil {
		return nil
	}

	if f.QueryFilter == nil {
		f.QueryFilter = NewDefaultQueryFilter()
	}

	if err := f.QueryFilter.Validate(); err != nil {
		return err
	}

	if f.TimeRangeFilter != nil {
		if err := f.TimeRangeFilter.Validate(); err != nil {
			return err
		}
	}

	if !f.GetExpand().IsEmpty() {
		if err := f.GetExpand().Validate(FeatureExpandConfig); err != nil {
			return err
		}
	}

	if f.Type != nil {
		if *f.Type != "api_key" && *f.Type != "integration" {
			return errors.Wrap(errors.ErrValidation, errors.ErrCodeInvalidOperation, "invalid type")
		}
	}

	if f.Provider != nil {
		if *f.Provider != "flexprice" && *f.Provider != "stripe" {
			return errors.Wrap(errors.ErrValidation, errors.ErrCodeInvalidOperation, "invalid provider")
		}
	}

	return nil
}

func (f *SecretFilter) GetLimit() int {
	if f == nil || f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetLimit()
	}
	return f.QueryFilter.GetLimit()
}

func (f *SecretFilter) GetOffset() int {
	if f == nil || f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetOffset()
	}
	return f.QueryFilter.GetOffset()
}

func (f *SecretFilter) GetSort() string {
	if f == nil || f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetSort()
	}
	return f.QueryFilter.GetSort()
}

func (f *SecretFilter) GetStatus() string {
	if f == nil || f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetStatus()
	}
	return f.QueryFilter.GetStatus()
}

func (f *SecretFilter) GetOrder() string {
	if f == nil || f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetOrder()
	}
	return f.QueryFilter.GetOrder()
}

func (f *SecretFilter) GetExpand() Expand {
	if f == nil || f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetExpand()
	}
	return f.QueryFilter.GetExpand()
}

func (f *SecretFilter) IsUnlimited() bool {
	if f == nil || f.QueryFilter == nil {
		return NewDefaultQueryFilter().IsUnlimited()
	}
	return f.QueryFilter.IsUnlimited()
}

// SecretExpandConfig defines the allowed expand fields for secrets
var SecretExpandConfig = ExpandConfig{
	AllowedFields: []ExpandableField{},
	NestedExpands: map[ExpandableField][]ExpandableField{},
}
