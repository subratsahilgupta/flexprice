package dto

import (
	"time"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/validator"
)

type SubscriptionChangeV2Request struct {
	TargetPlanID      string                  `json:"target_plan_id" validate:"required"`
	ProrationBehavior types.ProrationBehavior `json:"proration_behavior" validate:"required"`

	EntityPolicies *SubscriptionChangeEntityPolicies `json:"entity_policies,omitempty"`

	Metadata map[string]string `json:"metadata,omitempty"`
}

type SubscriptionChangeEntityPolicies struct {
	Addons *EntityChangePolicy `json:"addons,omitempty"`
}

type EntityChangePolicy struct {
	Default types.EntityDisposition `json:"default,omitempty"`

	// Overrides is keyed by addon_associations.id (instance), not catalogue addon_id.
	Overrides map[string]types.EntityDisposition `json:"overrides,omitempty"`
}

func (p *EntityChangePolicy) Validate() error {
	if p == nil {
		return nil
	}
	if err := p.Default.Validate(); err != nil {
		return err
	}
	for _, disposition := range p.Overrides {
		if err := disposition.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (p *EntityChangePolicy) DispositionFor(referenceID string) types.EntityDisposition {
	if p == nil {
		return types.EntityDispositionCarry
	}
	if d, ok := p.Overrides[referenceID]; ok && d != "" {
		return d
	}
	if p.Default != "" {
		return p.Default
	}
	return types.EntityDispositionCarry
}

type EntityDispositionResult struct {
	EntityType  string                  `json:"entity_type"`
	ReferenceID string                  `json:"reference_id"`
	EntityID    string                  `json:"entity_id"`
	Disposition types.EntityDisposition `json:"disposition"`
}

func (r *SubscriptionChangeV2Request) Validate() error {
	if err := validator.ValidateRequest(r); err != nil {
		return err
	}

	if err := r.ProrationBehavior.Validate(); err != nil {
		return err
	}

	if r.EntityPolicies != nil {
		return r.EntityPolicies.Addons.Validate()
	}
	return nil
}

type SubscriptionChangeV2Response struct {
	Subscription     *SubscriptionResponse `json:"subscription"`
	ChangedResources ChangedResources      `json:"changed_resources"`

	ChangeType  types.SubscriptionChangeType `json:"change_type"`
	EffectiveAt time.Time                    `json:"effective_at"`
	FromPlan    PlanSummary                  `json:"from_plan"`
	ToPlan      PlanSummary                  `json:"to_plan"`

	EntityDispositions []EntityDispositionResult `json:"entity_dispositions,omitempty"`

	Warnings []string          `json:"warnings,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}
