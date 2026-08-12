package dto

import (
	"time"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/validator"
)

// SubscriptionChangeV2Request changes a subscription's plan in place: the
// subscription row survives, plan_id is swapped, and plan line items are sliced
// at the effective date. Preview and execute take this same type — that is what
// makes a quote and its execution structurally identical rather than two
// implementations kept in step by hand.
//
// Growth is deliberate. entity_policies (addon dispositions), addendum_config
// (attaching addons), checkout and idempotency_key join this type as the phases
// that implement them land.
type SubscriptionChangeV2Request struct {
	TargetPlanID      string                  `json:"target_plan_id" validate:"required"`
	ProrationBehavior types.ProrationBehavior `json:"proration_behavior" validate:"required"`

	// EntityPolicies says what happens to things already attached to the
	// subscription. The wrapper exists so coupons, tax associations and credit
	// grants can be added later without a breaking change; v0 populates addons.
	EntityPolicies *SubscriptionChangeEntityPolicies `json:"entity_policies,omitempty"`

	Metadata map[string]string `json:"metadata,omitempty"`
}

// SubscriptionChangeEntityPolicies is the extension point: one field per entity
// type, all the same shape.
type SubscriptionChangeEntityPolicies struct {
	Addons *EntityChangePolicy `json:"addons,omitempty"`
}

// EntityChangePolicy declares what a change does to one entity type's existing
// attachments.
type EntityChangePolicy struct {
	// Default applies to every active attachment of this entity type. Empty means
	// carry, which is zero operations.
	Default types.EntityDisposition `json:"default,omitempty"`

	// Overrides is keyed by the INSTANCE id — addon_associations.id, never the
	// catalogue addon_id, because one subscription can hold two concurrent
	// attachments of the same addon and only one of them may be the live one.
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

// DispositionFor resolves one attachment: a per-instance override wins over the
// default, and an absent default means carry.
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

// EntityDispositionResult reports what the change decided for one attachment,
// including the ones it left alone. The shape mirrors the request, so a preview
// result can be sent back as explicit overrides unchanged.
type EntityDispositionResult struct {
	// EntityType is the kind of attachment. v0 only emits "addon".
	EntityType string `json:"entity_type"`
	// ReferenceID is the instance id — the key an override would use.
	ReferenceID string `json:"reference_id"`
	// EntityID is the catalogue id: addon_id, coupon_id, …
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

// SubscriptionChangeV2Response reports what a change did, or would do. A preview
// and an execute of the same request at the same instant return the same money;
// only the ids differ, because preview writes nothing.
type SubscriptionChangeV2Response struct {
	Subscription     *SubscriptionResponse `json:"subscription"`
	ChangedResources ChangedResources      `json:"changed_resources"`

	ChangeType  types.SubscriptionChangeType `json:"change_type"`
	EffectiveAt time.Time                    `json:"effective_at"`
	FromPlan    PlanSummary                  `json:"from_plan"`
	ToPlan      PlanSummary                  `json:"to_plan"`

	// EntityDispositions has one entry per active attachment the change
	// considered, carried ones included.
	EntityDispositions []EntityDispositionResult `json:"entity_dispositions,omitempty"`

	Warnings []string          `json:"warnings,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}
