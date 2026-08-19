package subscription

import (
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

func newPlanChangeSchedule() *SubscriptionSchedule {
	return &SubscriptionSchedule{
		ID:           "sched_123",
		ScheduleType: types.SubscriptionScheduleChangeTypePlanChange,
	}
}

func TestPlanChangeV2Config_RoundTrip(t *testing.T) {
	s := newPlanChangeSchedule()
	config := &PlanChangeV2Configuration{
		TargetPlanID:      "plan_target",
		ProrationBehavior: types.ProrationBehaviorNone,
		AddonPolicy: &EntityChangePolicyConfig{
			DefaultBehaviour: types.EntityChangeBehaviourCarry,
			Overrides: map[string]types.EntityChangeBehaviour{
				"addon_assoc_1": types.EntityChangeBehaviourDrop,
			},
		},
		IdempotencyKey: lo.ToPtr("idem_1"),
		ChangeMetadata: map[string]string{"source": "api"},
	}

	if err := s.SetPlanChangeV2Config(config); err != nil {
		t.Fatalf("SetPlanChangeV2Config: %v", err)
	}
	if config.Version != PlanChangeConfigVersionV2 {
		t.Fatalf("expected version to be set to %q, got %q", PlanChangeConfigVersionV2, config.Version)
	}

	got, err := s.GetPlanChangeV2Config()
	if err != nil {
		t.Fatalf("GetPlanChangeV2Config: %v", err)
	}
	if got.Version != PlanChangeConfigVersionV2 {
		t.Fatalf("version = %q, want %q", got.Version, PlanChangeConfigVersionV2)
	}
	if got.TargetPlanID != "plan_target" || got.ProrationBehavior != types.ProrationBehaviorNone {
		t.Fatalf("unexpected config: %+v", got)
	}
	if got.AddonPolicy == nil || got.AddonPolicy.DefaultBehaviour != types.EntityChangeBehaviourCarry {
		t.Fatalf("unexpected addon policy: %+v", got.AddonPolicy)
	}
	if got.AddonPolicy.Overrides["addon_assoc_1"] != types.EntityChangeBehaviourDrop {
		t.Fatalf("unexpected overrides: %+v", got.AddonPolicy.Overrides)
	}
	if lo.FromPtr(got.IdempotencyKey) != "idem_1" || got.ChangeMetadata["source"] != "api" {
		t.Fatalf("unexpected config: %+v", got)
	}
}

func TestPlanChangeV2Result_RoundTrip(t *testing.T) {
	s := newPlanChangeSchedule()

	result, err := s.GetPlanChangeV2Result()
	if err != nil || result != nil {
		t.Fatalf("expected nil result with no error, got %+v / %v", result, err)
	}

	effective := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	if err := s.SetPlanChangeV2Result(&PlanChangeV2Result{
		SubscriptionID: "sub_1",
		FromPlanID:     "plan_a",
		ToPlanID:       "plan_b",
		ChangeType:     "upgrade",
		EffectiveDate:  effective,
	}); err != nil {
		t.Fatalf("SetPlanChangeV2Result: %v", err)
	}

	got, err := s.GetPlanChangeV2Result()
	if err != nil {
		t.Fatalf("GetPlanChangeV2Result: %v", err)
	}
	if got.SubscriptionID != "sub_1" || got.FromPlanID != "plan_a" || got.ToPlanID != "plan_b" {
		t.Fatalf("unexpected result: %+v", got)
	}
	if !got.EffectiveDate.Equal(effective) {
		t.Fatalf("effective date = %v, want %v", got.EffectiveDate, effective)
	}
}

func TestPlanChangeV2Config_WrongScheduleType(t *testing.T) {
	s := &SubscriptionSchedule{ScheduleType: types.SubscriptionScheduleChangeTypeCancellation}

	if err := s.SetPlanChangeV2Config(&PlanChangeV2Configuration{}); err != ErrInvalidScheduleType {
		t.Fatalf("SetPlanChangeV2Config err = %v, want ErrInvalidScheduleType", err)
	}
	if _, err := s.GetPlanChangeV2Config(); err != ErrInvalidScheduleType {
		t.Fatalf("GetPlanChangeV2Config err = %v, want ErrInvalidScheduleType", err)
	}
	if err := s.SetPlanChangeV2Result(&PlanChangeV2Result{}); err != ErrInvalidScheduleType {
		t.Fatalf("SetPlanChangeV2Result err = %v, want ErrInvalidScheduleType", err)
	}
	if _, err := s.GetPlanChangeV2Result(); err != ErrInvalidScheduleType {
		t.Fatalf("GetPlanChangeV2Result err = %v, want ErrInvalidScheduleType", err)
	}
	if s.IsPlanChangeV2() {
		t.Fatal("IsPlanChangeV2() = true for a cancellation schedule")
	}
}

func TestIsPlanChangeV2(t *testing.T) {
	v1 := newPlanChangeSchedule()
	if err := v1.SetPlanChangeConfig(&PlanChangeConfiguration{
		TargetPlanID:       "plan_target",
		ProrationBehavior:  types.ProrationBehaviorCreateProrations,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		BillingPeriod:      types.BILLING_PERIOD_MONTHLY,
		BillingPeriodCount: 1,
	}); err != nil {
		t.Fatalf("SetPlanChangeConfig: %v", err)
	}
	if v1.IsPlanChangeV2() {
		t.Fatal("IsPlanChangeV2() = true for a v1 config blob")
	}

	v2 := newPlanChangeSchedule()
	if err := v2.SetPlanChangeV2Config(&PlanChangeV2Configuration{TargetPlanID: "plan_target"}); err != nil {
		t.Fatalf("SetPlanChangeV2Config: %v", err)
	}
	if !v2.IsPlanChangeV2() {
		t.Fatal("IsPlanChangeV2() = false for a v2 config blob")
	}

	empty := newPlanChangeSchedule()
	if empty.IsPlanChangeV2() {
		t.Fatal("IsPlanChangeV2() = true for an empty configuration")
	}

	malformed := newPlanChangeSchedule()
	malformed.Configuration = []byte("not json")
	if malformed.IsPlanChangeV2() {
		t.Fatal("IsPlanChangeV2() = true for a malformed configuration")
	}
}
