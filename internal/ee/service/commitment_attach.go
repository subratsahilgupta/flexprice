package service

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/commitment"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

// checkLineItemCommitmentAttach runs the commitment price-attach guard for a usage line item joining sub.
// Other lines cost nothing; with commitments disabled the cost is the cached settings read.
func checkLineItemCommitmentAttach(
	ctx context.Context,
	params ServiceParams,
	sub *subscription.Subscription,
	li *subscription.SubscriptionLineItem,
) error {
	if li == nil || li.PriceType != types.PRICE_TYPE_USAGE {
		return nil
	}

	svc := NewCommitmentService(params)
	enabled, err := svc.IsEnabled(ctx)
	if err != nil || !enabled {
		return err
	}

	p, err := params.PriceRepo.Get(ctx, li.PriceID)
	if err != nil {
		return err
	}

	return svc.CheckPriceAttach(ctx, sub, p, li.StartDate, lo.EmptyableToPtr(li.EndDate))
}

// rejectLegacyCommitmentInScope rejects legacy line-item commitment fields on a line that a commitment still
// in force covers, since the two would bill the same usage twice.
func rejectLegacyCommitmentInScope(
	ctx context.Context,
	params ServiceParams,
	sub *subscription.Subscription,
	li *subscription.SubscriptionLineItem,
) error {
	if li == nil || !li.HasAnyCommitment() {
		return nil
	}

	svc := NewCommitmentService(params)
	enabled, err := svc.IsEnabled(ctx)
	if err != nil || !enabled {
		return err
	}

	f := commitment.NewNoLimitFilter()
	f.SubscriptionIDs = []string{sub.ID}
	f.Statuses = []types.CommitmentStatus{types.CommitmentStatusActive, types.CommitmentStatusTerminated}
	listed, err := params.CommitmentRepo.List(ctx, f)
	if err != nil || len(listed) == 0 {
		return err
	}

	p, err := params.PriceRepo.Get(ctx, li.PriceID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, c := range listed {
		if !c.EndedBy(now) && commitmentCoversLine(c, li, p) {
			return invalidf(
				"line item %s is covered by commitment %s; legacy line-item commitment fields are not allowed",
				li.ID,
				c.ID,
			)
		}
	}

	return nil
}

func commitmentCoversLine(c *commitment.Commitment, li *subscription.SubscriptionLineItem, p *price.Price) bool {
	switch c.ScopeEntityType {
	case types.CommitmentScopeSubscription:
		return true
	case types.CommitmentScopeGroup:
		return p.GroupID == c.ScopeEntityID
	case types.CommitmentScopeMeter:
		return li.MeterID == c.ScopeEntityID
	case types.CommitmentScopePrice:
		return li.PriceID == c.ScopeEntityID
	}

	return false
}

// createSyncLineItem inserts a price-sync usage line item in the same transaction as the commitment attach
// check, under the subscription's commitments lock. A violation is logged and the line skipped, so one
// subscription never fails the whole sync; it reports whether the line was created.
func createSyncLineItem(
	ctx context.Context,
	params ServiceParams,
	sub *subscription.Subscription,
	p *price.Price,
	li *subscription.SubscriptionLineItem,
) (bool, error) {
	svc := &commitmentService{ServiceParams: params}
	var commitmentID string
	var checkErr error
	err := params.DB.WithTx(ctx, func(txCtx context.Context) error {
		commitmentID, checkErr = svc.checkPriceAttach(txCtx, sub, p, li.StartDate, lo.EmptyableToPtr(li.EndDate))
		if checkErr != nil {
			return checkErr
		}

		return params.SubscriptionLineItemRepo.Create(txCtx, li)
	})
	if checkErr != nil && ierr.IsValidation(checkErr) {
		params.Logger.Info(ctx, "price sync skipped a price that would break a commitment",
			"subscription_id", sub.ID, "price_id", p.ID, "commitment_id", commitmentID, "reason", checkErr.Error())
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}
