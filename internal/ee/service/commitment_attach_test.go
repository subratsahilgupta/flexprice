package service

import (
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/addon"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

// quantityCommitmentOnAPIOnlyGroup creates a quantity commitment scoped to the group holding only the API meter.
func (s *CommitmentServiceSuite) quantityCommitmentOnAPIOnlyGroup() {
	req := validReq(s.fx.sub.ID, "")
	req.Scope = dto.CommitmentScopeInput{EntityType: types.CommitmentScopeGroup, EntityID: s.fx.groupAPIOnly.ID}
	_, err := s.svc.Create(s.GetContext(), req)
	s.Require().NoError(err)
}

func (s *CommitmentServiceSuite) TestAddonAttach_RejectsPriceBreakingCommitment() {
	ctx := s.GetContext()
	s.quantityCommitmentOnAPIOnlyGroup()

	s.Require().NoError(s.GetStores().AddonRepo.Create(ctx, &addon.Addon{
		ID:        "addon_storage",
		LookupKey: "addon_storage",
		Name:      "Storage",
		BaseModel: types.GetDefaultBaseModel(ctx),
	}))
	addonPrice := *s.fx.priceOtherMeterInGroup
	addonPrice.ID, addonPrice.EntityType, addonPrice.EntityID = "price_addon_storage", types.PRICE_ENTITY_TYPE_ADDON, "addon_storage"
	s.Require().NoError(s.GetStores().PriceRepo.Create(ctx, &addonPrice))

	start := s.fx.sub.StartDate
	_, err := NewSubscriptionService(s.fullParams()).AddAddonToSubscription(ctx, &dto.AddAddonRequest{
		SubscriptionID: s.fx.sub.ID,
		AddAddonToSubscriptionRequest: dto.AddAddonToSubscriptionRequest{
			AddonID:   "addon_storage",
			Cadence:   types.AddonCadenceRecurring,
			StartDate: &start,
		},
	})
	s.requireValidation(err, "different meter")
}

func (s *CommitmentServiceSuite) TestPlanSync_SkipsPriceBreakingCommitment() {
	ctx := s.GetContext()
	s.quantityCommitmentOnAPIOnlyGroup()
	sub, err := s.GetStores().SubscriptionRepo.Get(ctx, s.fx.sub.ID)
	s.Require().NoError(err)
	sub.SubscriptionType = types.SubscriptionTypeStandalone
	s.Require().NoError(s.GetStores().SubscriptionRepo.Update(ctx, sub))
	s.Require().NoError(s.GetStores().PlanRepo.Create(ctx, &plan.Plan{
		ID:            "plan_cmt",
		Name:          "Plan",
		EnvironmentID: types.GetEnvironmentID(ctx),
		BaseModel:     types.GetDefaultBaseModel(ctx),
	}))

	// A second meter would break the single-meter commitment; another API price joins it fine.
	storage := *s.fx.priceOtherMeterInGroup
	storage.ID, storage.Sequence = "price_plan_storage_new", 1
	api := *s.fx.priceAPI
	api.ID, api.GroupID, api.Sequence = "price_plan_api_new", s.fx.groupAPIOnly.ID, 2
	s.Require().NoError(s.GetStores().PriceRepo.Create(ctx, &storage))
	s.Require().NoError(s.GetStores().PriceRepo.Create(ctx, &api))

	params := s.fullParams()
	params.PlanPriceSyncRepo = s.GetStores().PlanPriceSyncRepo
	_, err = NewPlanService(params).SyncPlanPricesV2(ctx, "plan_cmt")
	s.Require().NoError(err)

	_, items, err := s.GetStores().SubscriptionRepo.GetWithLineItems(ctx, s.fx.sub.ID)
	s.Require().NoError(err)
	priceIDs := lo.Map(items, func(li *subscription.SubscriptionLineItem, _ int) string { return li.PriceID })
	s.Contains(priceIDs, api.ID)
	s.NotContains(priceIDs, storage.ID)
	s.NotContains(priceIDs, s.fx.priceOtherMeterInGroup.ID)
}

func (s *CommitmentServiceSuite) TestLineItemUpdate_RejectsLegacyCommitmentInScope() {
	req := validReq(s.fx.sub.ID, "")
	req.Scope = dto.CommitmentScopeInput{EntityType: types.CommitmentScopeMeter, EntityID: s.fx.meterAPI.ID}
	_, err := s.svc.Create(s.GetContext(), req)
	s.Require().NoError(err)

	_, err = NewSubscriptionService(s.fullParams()).UpdateSubscriptionLineItem(s.GetContext(), s.fx.lineAPI.ID,
		dto.UpdateSubscriptionLineItemRequest{
			CommitmentAmount:        lo.ToPtr(dec("50")),
			CommitmentType:          types.COMMITMENT_TYPE_AMOUNT,
			CommitmentOverageFactor: lo.ToPtr(dec("1.5")),
			EffectiveFrom:           lo.ToPtr(s.fx.sub.StartDate.AddDate(0, 1, 0)),
		})
	s.requireValidation(err, "legacy line-item commitment")

	stored, err := s.GetStores().SubscriptionLineItemRepo.Get(s.GetContext(), s.fx.lineAPI.ID)
	s.Require().NoError(err)
	s.False(stored.HasAnyCommitment())
}

func (s *CommitmentServiceSuite) TestLineItemAdd_RejectsLegacyCommitmentInScope() {
	req := validReq(s.fx.sub.ID, "")
	req.Scope = dto.CommitmentScopeInput{EntityType: types.CommitmentScopeSubscription, EntityID: s.fx.sub.ID}
	req.Type, req.CommittedRate, req.Value = types.COMMITMENT_TYPE_AMOUNT, nil, dec("100")
	_, err := s.svc.Create(s.GetContext(), req)
	s.Require().NoError(err)

	_, err = NewSubscriptionService(s.fullParams()).AddSubscriptionLineItem(s.GetContext(), s.fx.sub.ID,
		dto.CreateSubscriptionLineItemRequest{
			PriceID:          s.fx.priceOtherMeterInGroup.ID,
			CommitmentAmount: lo.ToPtr(dec("50")),
			CommitmentType:   types.COMMITMENT_TYPE_AMOUNT,
		})
	s.requireValidation(err, "legacy line-item commitment")
}

func (s *CommitmentServiceSuite) TestCheckPriceAttach_AllowsLineEndingBeforeCommitmentStart() {
	march := time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)
	req := validReq(s.fx.sub.ID, "")
	req.Scope = dto.CommitmentScopeInput{EntityType: types.CommitmentScopeGroup, EntityID: s.fx.groupAPIOnly.ID}
	req.StartDate = march
	_, err := s.svc.Create(s.GetContext(), req)
	s.Require().NoError(err)

	// A second meter would break the single-meter commitment, but this line is gone before it starts.
	start := s.fx.sub.StartDate
	s.Require().NoError(s.svc.CheckPriceAttach(s.GetContext(), s.fx.sub, s.fx.priceOtherMeterInGroup, start, &march))

	err = s.svc.CheckPriceAttach(s.GetContext(), s.fx.sub, s.fx.priceOtherMeterInGroup, start, lo.ToPtr(march.AddDate(0, 0, 1)))
	s.requireValidation(err, "different meter")
}

func (s *CommitmentServiceSuite) TestCheckPriceAttach_GuardsTerminatedCommitmentUntilCutoff() {
	req := validReq(s.fx.sub.ID, "")
	req.Scope = dto.CommitmentScopeInput{EntityType: types.CommitmentScopeGroup, EntityID: s.fx.groupAPIOnly.ID}
	created, err := s.svc.Create(s.GetContext(), req)
	s.Require().NoError(err)
	cutoff := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	_, err = s.svc.Terminate(s.GetContext(), created.ID, dto.TerminateCommitmentRequest{EffectiveAt: &cutoff})
	s.Require().NoError(err)

	err = s.svc.CheckPriceAttach(s.GetContext(), s.fx.sub, s.fx.priceOtherMeterInGroup, s.fx.sub.StartDate, nil)
	s.requireValidation(err, "different meter")

	s.Require().NoError(s.svc.CheckPriceAttach(s.GetContext(), s.fx.sub, s.fx.priceOtherMeterInGroup, cutoff, nil))
}
