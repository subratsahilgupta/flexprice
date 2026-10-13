package testutil

import (
	"context"
	"sort"
	"time"

	"github.com/flexprice/flexprice/internal/domain/addonassociation"
	"github.com/flexprice/flexprice/internal/domain/addonpricesync"
	"github.com/flexprice/flexprice/internal/domain/price"
	"github.com/flexprice/flexprice/internal/domain/subscription"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

// InMemoryAddonPriceSyncStore implements addonpricesync.Repository against the
// other in-memory stores. It mirrors internal/repository/ent/addon_price_sync.go
// closely enough for service-level unit tests — keyed on the addon association,
// with the watermark on association.SyncedPriceSequence.
type InMemoryAddonPriceSyncStore struct {
	priceStore    *InMemoryPriceStore
	subStore      *InMemorySubscriptionStore
	lineItemStore *InMemorySubscriptionLineItemStore
	assocStore    *InMemoryAddonAssociationStore
}

func NewInMemoryAddonPriceSyncStore(
	priceStore *InMemoryPriceStore,
	subStore *InMemorySubscriptionStore,
	lineItemStore *InMemorySubscriptionLineItemStore,
	assocStore *InMemoryAddonAssociationStore,
) *InMemoryAddonPriceSyncStore {
	return &InMemoryAddonPriceSyncStore{
		priceStore:    priceStore,
		subStore:      subStore,
		lineItemStore: lineItemStore,
		assocStore:    assocStore,
	}
}

// Clear is a no-op — this store owns no state, only refs to other stores.
func (r *InMemoryAddonPriceSyncStore) Clear() {}

// --- snapshot helpers -------------------------------------------------------

func isAddonUsagePrice(p *price.Price, addonID string) bool {
	return p != nil &&
		p.Status == types.StatusPublished &&
		p.EntityType == types.PRICE_ENTITY_TYPE_ADDON &&
		p.EntityID == addonID &&
		p.Type == types.PRICE_TYPE_USAGE
}

func (r *InMemoryAddonPriceSyncStore) addonUsagePrices(ctx context.Context, addonID string) []*price.Price {
	if r.priceStore == nil {
		return nil
	}
	r.priceStore.mu.RLock()
	defer r.priceStore.mu.RUnlock()
	var out []*price.Price
	for _, p := range r.priceStore.items {
		if isAddonUsagePrice(p, addonID) && scopedByCtx(ctx, p.TenantID, p.EnvironmentID) {
			out = append(out, p)
		}
	}
	return out
}

func (r *InMemoryAddonPriceSyncStore) subByID() map[string]*subscription.Subscription {
	out := make(map[string]*subscription.Subscription)
	if r.subStore == nil {
		return out
	}
	r.subStore.mu.RLock()
	defer r.subStore.mu.RUnlock()
	for id, s := range r.subStore.items {
		out[id] = s
	}
	return out
}

// activeRecurringAssociations returns active, recurring, published associations
// to addonID on active/trialing published subscriptions, scoped by ctx.
func (r *InMemoryAddonPriceSyncStore) activeRecurringAssociations(ctx context.Context, addonID string) []*addonassociation.AddonAssociation {
	if r.assocStore == nil {
		return nil
	}
	subs := r.subByID()

	r.assocStore.mu.RLock()
	defer r.assocStore.mu.RUnlock()

	var out []*addonassociation.AddonAssociation
	for _, aa := range r.assocStore.items {
		if aa == nil || aa.Status != types.StatusPublished {
			continue
		}
		if aa.EntityType != types.AddonAssociationEntityTypeSubscription {
			continue
		}
		if aa.AddonID != addonID || aa.AddonStatus != types.AddonStatusActive {
			continue
		}
		if aa.EndDate != nil {
			continue
		}
		if !scopedByCtx(ctx, aa.TenantID, aa.EnvironmentID) {
			continue
		}
		sub := subs[aa.EntityID]
		if sub == nil || sub.Status != types.StatusPublished {
			continue
		}
		if sub.SubscriptionStatus != types.SubscriptionStatusActive &&
			sub.SubscriptionStatus != types.SubscriptionStatusTrialing {
			continue
		}
		out = append(out, aa)
	}
	return out
}

// --- addonpricesync.Repository implementation -------------------------------

// CurrentAddonSequence returns max(prices.sequence) for the addon's published
// usage prices. 0 when none qualify.
func (r *InMemoryAddonPriceSyncStore) CurrentAddonSequence(ctx context.Context, addonID string) (int64, error) {
	if addonID == "" {
		return 0, ierr.NewError("addon_id is required").Mark(ierr.ErrValidation)
	}
	var maxSeq int64
	for _, p := range r.addonUsagePrices(ctx, addonID) {
		if p.Sequence > maxSeq {
			maxSeq = p.Sequence
		}
	}
	return maxSeq, nil
}

// ListBehindAssociations returns active recurring associations whose watermark is
// below TargetSeq, ordered by (synced_price_sequence, id) and limited.
func (r *InMemoryAddonPriceSyncStore) ListBehindAssociations(
	ctx context.Context,
	p addonpricesync.ListBehindAssociationsParams,
) ([]addonpricesync.AddonAssociationRef, error) {
	if p.AddonID == "" {
		return nil, ierr.NewError("addon_id is required").Mark(ierr.ErrValidation)
	}

	var behind []*addonassociation.AddonAssociation
	for _, aa := range r.activeRecurringAssociations(ctx, p.AddonID) {
		if aa.SyncedPriceSequence < p.TargetSeq {
			behind = append(behind, aa)
		}
	}
	sort.Slice(behind, func(i, j int) bool {
		if behind[i].SyncedPriceSequence != behind[j].SyncedPriceSequence {
			return behind[i].SyncedPriceSequence < behind[j].SyncedPriceSequence
		}
		return behind[i].ID < behind[j].ID
	})
	if p.Limit > 0 && len(behind) > p.Limit {
		behind = behind[:p.Limit]
	}

	refs := make([]addonpricesync.AddonAssociationRef, 0, len(behind))
	for _, aa := range behind {
		refs = append(refs, addonpricesync.AddonAssociationRef{
			AssociationID:  aa.ID,
			SubscriptionID: aa.EntityID,
		})
	}
	return refs, nil
}

// TerminateEndedAddonLineItems end-dates live addon usage line items belonging to
// the given associations whose catalog price has been ended. Idempotent.
func (r *InMemoryAddonPriceSyncStore) TerminateEndedAddonLineItems(
	ctx context.Context,
	p addonpricesync.TerminateEndedAddonLineItemsParams,
) (int, error) {
	if p.AddonID == "" {
		return 0, ierr.NewError("addon_id is required").Mark(ierr.ErrValidation)
	}
	if len(p.AssociationIDs) == 0 || r.lineItemStore == nil {
		return 0, nil
	}

	endedPrice := make(map[string]*price.Price)
	for _, pp := range r.addonUsagePrices(ctx, p.AddonID) {
		if pp.EndDate != nil {
			endedPrice[pp.ID] = pp
		}
	}
	if len(endedPrice) == 0 {
		return 0, nil
	}

	assocSet := make(map[string]struct{}, len(p.AssociationIDs))
	for _, id := range p.AssociationIDs {
		assocSet[id] = struct{}{}
	}

	r.lineItemStore.mu.Lock()
	defer r.lineItemStore.mu.Unlock()

	updated := 0
	for _, li := range r.lineItemStore.items {
		if li == nil || li.Status != types.StatusPublished {
			continue
		}
		if li.EntityType != types.SubscriptionLineItemEntityTypeAddon {
			continue
		}
		if !li.EndDate.IsZero() || li.AddonAssociationID == nil {
			continue
		}
		if _, ok := assocSet[*li.AddonAssociationID]; !ok {
			continue
		}
		pp, ok := endedPrice[li.PriceID]
		if !ok || pp.EndDate == nil {
			continue
		}
		if !scopedByCtx(ctx, li.TenantID, li.EnvironmentID) {
			continue
		}
		endDate := *pp.EndDate
		if !li.StartDate.IsZero() && li.StartDate.After(endDate) {
			endDate = li.StartDate
		}
		li.EndDate = endDate
		li.UpdatedAt = time.Now().UTC()
		if uid := types.GetUserID(ctx); uid != "" {
			li.UpdatedBy = uid
		}
		updated++
	}
	return updated, nil
}

// StampAssociations sets synced_price_sequence on the given associations.
// Forward-only: never lowers an existing higher value.
func (r *InMemoryAddonPriceSyncStore) StampAssociations(
	ctx context.Context,
	p addonpricesync.StampAssociationsParams,
) (int, error) {
	if len(p.AssociationIDs) == 0 || r.assocStore == nil {
		return 0, nil
	}

	r.assocStore.mu.Lock()
	defer r.assocStore.mu.Unlock()

	updated := 0
	for _, id := range p.AssociationIDs {
		aa, ok := r.assocStore.items[id]
		if !ok || aa == nil {
			continue
		}
		if !scopedByCtx(ctx, aa.TenantID, aa.EnvironmentID) {
			continue
		}
		if aa.SyncedPriceSequence >= p.TargetSeq {
			continue
		}
		aa.SyncedPriceSequence = p.TargetSeq
		aa.UpdatedAt = time.Now().UTC()
		if uid := types.GetUserID(ctx); uid != "" {
			aa.UpdatedBy = uid
		}
		updated++
	}
	return updated, nil
}

// CountUnsyncedAssociations returns how many active recurring associations are
// behind targetSeq.
func (r *InMemoryAddonPriceSyncStore) CountUnsyncedAssociations(ctx context.Context, addonID string, targetSeq int64) (int64, error) {
	if addonID == "" {
		return 0, ierr.NewError("addon_id is required").Mark(ierr.ErrValidation)
	}
	var count int64
	for _, aa := range r.activeRecurringAssociations(ctx, addonID) {
		if aa.SyncedPriceSequence < targetSeq {
			count++
		}
	}
	return count, nil
}
