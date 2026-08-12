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

	Metadata map[string]string `json:"metadata,omitempty"`
}

func (r *SubscriptionChangeV2Request) Validate() error {
	if err := validator.ValidateRequest(r); err != nil {
		return err
	}

	return r.ProrationBehavior.Validate()
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

	Warnings []string          `json:"warnings,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}
