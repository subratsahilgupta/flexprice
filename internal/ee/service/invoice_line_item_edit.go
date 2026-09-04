package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// recalculateTotalsFromLineItems re-derives totals from already-applied discount/tax;
// callers must pass only published line items, and discount/tax are not recomputed here.
func (s *invoiceService) recalculateTotalsFromLineItems(inv *invoice.Invoice, lineItems []*invoice.InvoiceLineItem) {
	subtotal := decimal.Zero
	for _, li := range lineItems {
		subtotal = subtotal.Add(li.Amount)
	}
	inv.Subtotal = subtotal

	// Discount-first-then-tax: total = subtotal - prepaid credits - discount + tax
	inv.Total = inv.Subtotal.Sub(inv.TotalPrepaidCreditsApplied).Sub(inv.TotalDiscount).Add(inv.TotalTax)
	if inv.Total.IsNegative() {
		inv.Total = decimal.Zero
	}
	inv.AmountDue = inv.Total
	inv.AmountRemaining = inv.Total.Sub(inv.AmountPaid)
	if inv.AmountRemaining.IsNegative() {
		inv.AmountRemaining = decimal.Zero
	}
}

func (s *invoiceService) UpdateLineItem(ctx context.Context, invoiceID, lineItemID string, req dto.UpdateLineItemRequest) (*dto.InvoiceResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	var lockedInv *invoice.Invoice
	var publishedLineItems []*invoice.InvoiceLineItem

	err := s.DB.WithTx(ctx, func(txCtx context.Context) error {
		inv, err := s.InvoiceRepo.GetForUpdate(txCtx, invoiceID)
		if err != nil {
			return err
		}
		if inv.InvoiceStatus != types.InvoiceStatusDraft {
			return ierr.NewError("invoice is not in draft status").
				WithHint("invoice must be in draft status to be edited").
				Mark(ierr.ErrValidation)
		}
		lockedInv = inv

		existingItem, err := s.InvoiceLineItemRepo.Get(txCtx, lineItemID)
		if err != nil {
			return err
		}
		if existingItem.InvoiceID != invoiceID {
			return ierr.NewError("line item not found").
				WithHintf("line item %s does not belong to invoice %s", lineItemID, invoiceID).
				Mark(ierr.ErrNotFound)
		}
		if existingItem.Status != types.StatusPublished {
			// Editing an already-archived/deleted row would branch the lineage
			// chain instead of extending it, or resurrect a removed item.
			return ierr.NewError("line item is not editable").
				WithHintf("line item %s has status %s and is not the current version", lineItemID, existingItem.Status).
				Mark(ierr.ErrValidation)
		}

		builder := invoice.NewInvoiceLineItemBuilder(existingItem).
			WithID(types.GenerateUUIDWithPrefix(types.UUID_PREFIX_INVOICE_LINE_ITEM)).
			WithParentLineItemID(lo.ToPtr(existingItem.ID)).
			WithBaseModel(types.GetDefaultBaseModel(txCtx))

		if req.DisplayName != nil {
			builder = builder.WithDisplayName(req.DisplayName)
		}
		if req.Amount != nil {
			builder = builder.WithAmount(*req.Amount)
		}
		if req.Quantity != nil {
			builder = builder.WithQuantity(*req.Quantity)
		}

		newItem := builder.Build()
		if err := newItem.Validate(); err != nil {
			return err
		}

		existingItem.Status = types.StatusArchived
		if err := s.InvoiceLineItemRepo.Update(txCtx, existingItem); err != nil {
			return err
		}

		if err := s.InvoiceLineItemRepo.Create(txCtx, newItem); err != nil {
			return err
		}

		remaining := make([]*invoice.InvoiceLineItem, 0, len(lockedInv.LineItems))
		for _, li := range lockedInv.LineItems {
			if li.ID == existingItem.ID {
				continue
			}
			remaining = append(remaining, li)
		}
		publishedLineItems = append(remaining, newItem)

		s.recalculateTotalsFromLineItems(lockedInv, publishedLineItems)
		lockedInv.IsManuallyEdited = true

		return s.InvoiceRepo.Update(txCtx, lockedInv)
	})
	if err != nil {
		return nil, err
	}

	lockedInv.LineItems = publishedLineItems
	return dto.NewInvoiceResponse(lockedInv), nil
}

func (s *invoiceService) AddBulkLineItem(ctx context.Context, invoiceID string, req dto.AddBulkLineItemRequest) (*dto.InvoiceResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	var lockedInv *invoice.Invoice
	var publishedLineItems []*invoice.InvoiceLineItem

	err := s.DB.WithTx(ctx, func(txCtx context.Context) error {
		inv, err := s.InvoiceRepo.GetForUpdate(txCtx, invoiceID)
		if err != nil {
			return err
		}
		if inv.InvoiceStatus != types.InvoiceStatusDraft {
			return ierr.NewError("invoice is not in draft status").
				WithHint("invoice must be in draft status to be edited").
				Mark(ierr.ErrValidation)
		}
		lockedInv = inv

		newItems := lo.Map(req.Items, func(item dto.AddLineItemRequest, _ int) *invoice.InvoiceLineItem {
			return invoice.NewInvoiceLineItemBuilder(nil).
				WithID(types.GenerateUUIDWithPrefix(types.UUID_PREFIX_INVOICE_LINE_ITEM)).
				WithInvoiceID(inv.ID).
				WithCustomerID(inv.CustomerID).
				WithEnvironmentID(inv.EnvironmentID).
				WithDisplayName(lo.ToPtr(item.DisplayName)).
				WithAmount(item.Amount).
				WithQuantity(item.Quantity).
				WithCurrency(inv.Currency).
				WithBaseModel(types.GetDefaultBaseModel(txCtx)).
				Build()
		})
		for _, newItem := range newItems {
			if err := newItem.Validate(); err != nil {
				return err
			}
		}

		if err := s.InvoiceLineItemRepo.CreateBulk(txCtx, newItems); err != nil {
			return err
		}

		publishedLineItems = make([]*invoice.InvoiceLineItem, 0, len(lockedInv.LineItems)+len(newItems))
		publishedLineItems = append(publishedLineItems, lockedInv.LineItems...)
		publishedLineItems = append(publishedLineItems, newItems...)

		s.recalculateTotalsFromLineItems(lockedInv, publishedLineItems)
		lockedInv.IsManuallyEdited = true

		return s.InvoiceRepo.Update(txCtx, lockedInv)
	})
	if err != nil {
		return nil, err
	}

	lockedInv.LineItems = publishedLineItems
	return dto.NewInvoiceResponse(lockedInv), nil
}

func (s *invoiceService) RemoveBulkLineItem(ctx context.Context, invoiceID string, req dto.RemoveBulkLineItemRequest) (*dto.InvoiceResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	var lockedInv *invoice.Invoice
	var publishedLineItems []*invoice.InvoiceLineItem

	err := s.DB.WithTx(ctx, func(txCtx context.Context) error {
		inv, err := s.InvoiceRepo.GetForUpdate(txCtx, invoiceID)
		if err != nil {
			return err
		}
		if inv.InvoiceStatus != types.InvoiceStatusDraft {
			return ierr.NewError("invoice is not in draft status").
				WithHint("invoice must be in draft status to be edited").
				Mark(ierr.ErrValidation)
		}
		lockedInv = inv

		removedIDs := make(map[string]bool, len(req.LineItemIDs))
		for _, lineItemID := range req.LineItemIDs {
			existingItem, err := s.InvoiceLineItemRepo.Get(txCtx, lineItemID)
			if err != nil {
				return err
			}
			if existingItem.InvoiceID != invoiceID {
				return ierr.NewError("line item not found").
					WithHintf("line item %s does not belong to invoice %s", lineItemID, invoiceID).
					Mark(ierr.ErrNotFound)
			}
			if existingItem.Status != types.StatusPublished {
				return ierr.NewError("line item is not removable").
					WithHintf("line item %s has status %s and is not the current version", lineItemID, existingItem.Status).
					Mark(ierr.ErrValidation)
			}
			removedIDs[lineItemID] = true
		}

		if err := s.InvoiceRepo.RemoveLineItems(txCtx, invoiceID, req.LineItemIDs); err != nil {
			return err
		}

		remaining := make([]*invoice.InvoiceLineItem, 0, len(lockedInv.LineItems))
		for _, li := range lockedInv.LineItems {
			if removedIDs[li.ID] {
				continue
			}
			remaining = append(remaining, li)
		}
		publishedLineItems = remaining

		s.recalculateTotalsFromLineItems(lockedInv, publishedLineItems)
		lockedInv.IsManuallyEdited = true

		return s.InvoiceRepo.Update(txCtx, lockedInv)
	})
	if err != nil {
		return nil, err
	}

	lockedInv.LineItems = publishedLineItems
	return dto.NewInvoiceResponse(lockedInv), nil
}

func (s *invoiceService) ModifyInvoice(ctx context.Context, invoiceID string, req dto.ExecuteInvoiceModifyRequest) (*dto.InvoiceModifyResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	switch req.Type {
	case dto.InvoiceModifyTypeLineItem:
		return s.executeLineItemModification(ctx, invoiceID, req.LineItemParams)
	default:
		return nil, ierr.NewError("unknown modification type: " + string(req.Type)).
			WithHint("valid values: line_item").
			Mark(ierr.ErrValidation)
	}
}

func (s *invoiceService) executeLineItemModification(ctx context.Context, invoiceID string, params *dto.InvoiceModifyLineItemParams) (*dto.InvoiceModifyResponse, error) {
	// Finalized-invoice edit flow: the finalized invoice is voided and recreated as a
	// draft copy, and the modification lands on the copy. Hard stop for voided ids: a
	// call that still targets a voided original is rejected (the error names the
	// replacement) — clients must chain the invoice id returned by a successful edit.
	inv, err := s.InvoiceRepo.Get(ctx, invoiceID)
	if err != nil {
		return nil, err
	}
	if err := rejectVoidedInvoiceEdit(inv); err != nil {
		return nil, err
	}

	if inv.InvoiceStatus != types.InvoiceStatusFinalized {
		resp, err := s.applyLineItemAction(ctx, inv.ID, params)
		if err != nil {
			return nil, err
		}
		return &dto.InvoiceModifyResponse{Invoice: resp}, nil
	}

	// Void, recreate, and edit in ONE transaction: any failure rolls back the void
	// and the copy together, so the finalized invoice is never left voided by a
	// request that could not complete.
	var resp *dto.InvoiceResponse
	err = s.DB.WithTx(ctx, func(txCtx context.Context) error {
		locked, err := s.InvoiceRepo.GetForUpdate(txCtx, inv.ID)
		if err != nil {
			return err
		}
		if locked.InvoiceStatus != types.InvoiceStatusFinalized {
			return ierr.NewError("invoice is no longer finalized").
				WithHintf("invoice %s changed to %s concurrently, retry the edit", locked.ID, locked.InvoiceStatus).
				Mark(ierr.ErrValidation)
		}

		draft, idMap, err := s.voidAndRecreateDraftForEdit(txCtx, locked)
		if err != nil {
			return err
		}

		translated, err := translateLineItemParams(params, idMap, draft.ID)
		if err != nil {
			return err
		}
		resp, err = s.applyLineItemAction(txCtx, draft.ID, translated)
		return err
	})
	if err != nil {
		return nil, err
	}

	return &dto.InvoiceModifyResponse{Invoice: resp}, nil
}

// applyLineItemAction dispatches one line-item modification against the given invoice.
func (s *invoiceService) applyLineItemAction(ctx context.Context, invoiceID string, params *dto.InvoiceModifyLineItemParams) (*dto.InvoiceResponse, error) {
	switch params.Action {
	case dto.InvoiceModifyLineItemActionAdd:
		return s.AddBulkLineItem(ctx, invoiceID, dto.AddBulkLineItemRequest{
			Items: params.Items,
		})
	case dto.InvoiceModifyLineItemActionUpdate:
		lineItemID, err := s.resolveLineItemIDForEdit(ctx, invoiceID, params.LineItemID)
		if err != nil {
			return nil, err
		}
		return s.UpdateLineItem(ctx, invoiceID, lineItemID, *params.Update)
	case dto.InvoiceModifyLineItemActionRemove:
		lineItemIDs := make([]string, 0, len(params.LineItemIDs))
		for _, id := range params.LineItemIDs {
			resolved, err := s.resolveLineItemIDForEdit(ctx, invoiceID, id)
			if err != nil {
				return nil, err
			}
			lineItemIDs = append(lineItemIDs, resolved)
		}
		return s.RemoveBulkLineItem(ctx, invoiceID, dto.RemoveBulkLineItemRequest{
			LineItemIDs: lineItemIDs,
		})
	default:
		return nil, ierr.NewError("unknown line item action: " + string(params.Action)).
			WithHint("valid values: add, update, remove").
			Mark(ierr.ErrValidation)
	}
}

// translateLineItemParams maps line item ids from the voided original onto the draft copy
// using the copy's old->new id map. Remove ids that no longer exist are dropped; a targeted
// update of an unknown id errors (the transaction rolls the void back).
func translateLineItemParams(params *dto.InvoiceModifyLineItemParams, idMap map[string]string, draftID string) (*dto.InvoiceModifyLineItemParams, error) {
	translated := *params
	switch params.Action {
	case dto.InvoiceModifyLineItemActionUpdate:
		newID, ok := idMap[params.LineItemID]
		if !ok {
			return nil, ierr.NewError("line item not found").
				WithHintf("line item %s does not exist on invoice %s", params.LineItemID, draftID).
				Mark(ierr.ErrNotFound)
		}
		translated.LineItemID = newID
	case dto.InvoiceModifyLineItemActionRemove:
		ids := make([]string, 0, len(params.LineItemIDs))
		for _, id := range params.LineItemIDs {
			if newID, ok := idMap[id]; ok {
				ids = append(ids, newID)
			}
		}
		if len(ids) == 0 {
			return nil, ierr.NewError("line items not found").
				WithHintf("none of the requested line items exist on invoice %s", draftID).
				Mark(ierr.ErrNotFound)
		}
		translated.LineItemIDs = ids
	}
	return &translated, nil
}

// rejectVoidedInvoiceEdit hard-stops edits that target a voided invoice. When the
// invoice was voided by the edit flow, the error names its replacement so the caller
// can retry against the current draft; redirecting silently would let a stale caller
// stack a second edit onto a replacement it has never seen.
func rejectVoidedInvoiceEdit(inv *invoice.Invoice) error {
	if inv.InvoiceStatus != types.InvoiceStatusVoided {
		return nil
	}
	if inv.RecalculatedInvoiceID != nil {
		return ierr.NewError("invoice is voided and was replaced by " + *inv.RecalculatedInvoiceID).
			WithHintf("invoice %s was voided and replaced by %s — retry the edit against the replacement", inv.ID, *inv.RecalculatedInvoiceID).
			WithReportableDetails(map[string]any{"recalculated_invoice_id": *inv.RecalculatedInvoiceID}).
			Mark(ierr.ErrValidation)
	}
	return ierr.NewError("invoice is voided").
		WithHintf("invoice %s is voided and can no longer be edited", inv.ID).
		Mark(ierr.ErrValidation)
}

// resolveLineItemIDForEdit maps a line item id onto the given invoice. Ids already on the
// invoice pass through; an id from the voided original of a recreated draft resolves to the
// copied line item via parent_line_item_id lineage, so clients can keep referencing the ids
// they read before the void-and-recreate.
func (s *invoiceService) resolveLineItemIDForEdit(ctx context.Context, invoiceID, lineItemID string) (string, error) {
	li, err := s.InvoiceLineItemRepo.Get(ctx, lineItemID)
	if err == nil && li.InvoiceID == invoiceID {
		return lineItemID, nil
	}

	items, listErr := s.InvoiceLineItemRepo.ListByInvoiceID(ctx, invoiceID)
	if listErr != nil {
		return "", listErr
	}
	for _, item := range items {
		if item.ParentLineItemID != nil && *item.ParentLineItemID == lineItemID && item.Status == types.StatusPublished {
			return item.ID, nil
		}
	}
	if err != nil {
		return "", err
	}
	return lineItemID, nil
}

// voidAndRecreateDraftForEdit implements the finalized-invoice edit flow: it voids the
// finalized invoice (refunding any captured payment or applied credits) and creates a DRAFT
// copy carrying the invoice's data and every published line item. Copied line items point at
// their originals via parent_line_item_id and the returned map translates old ids to new
// ones. Discounts are re-derived on the copy from the subscription's current coupon
// associations; taxes are recalculated when the draft is finalized. The original links to
// the copy via recalculated_invoice_id. Callers run this inside a transaction so a failed
// edit rolls the void back.
func (s *invoiceService) voidAndRecreateDraftForEdit(ctx context.Context, inv *invoice.Invoice) (*invoice.Invoice, map[string]string, error) {
	voided, err := s.VoidInvoice(ctx, inv.ID, dto.InvoiceVoidRequest{})
	if err != nil {
		return nil, nil, err
	}

	draftID := types.GenerateUUIDWithPrefix(types.UUID_PREFIX_INVOICE)
	baseModel := types.GetDefaultBaseModel(ctx)

	// List directly rather than trusting voided.LineItems hydration.
	sourceItems, err := s.InvoiceLineItemRepo.ListByInvoiceID(ctx, voided.ID)
	if err != nil {
		return nil, nil, err
	}

	idMap := make(map[string]string, len(sourceItems))
	lineItems := make([]*invoice.InvoiceLineItem, 0, len(sourceItems))
	for _, li := range sourceItems {
		if li.Status != types.StatusPublished {
			continue
		}
		copied := invoice.NewInvoiceLineItemBuilder(li).
			WithID(types.GenerateUUIDWithPrefix(types.UUID_PREFIX_INVOICE_LINE_ITEM)).
			WithInvoiceID(draftID).
			WithParentLineItemID(lo.ToPtr(li.ID)).
			WithBaseModel(baseModel).
			Build()
		idMap[li.ID] = copied.ID
		lineItems = append(lineItems, copied)
	}

	draft := voided.CopyForDraftEdit(draftID, baseModel)
	draft.LineItems = lineItems
	if len(lineItems) > 0 {
		s.recalculateTotalsFromLineItems(draft, lineItems)
	} else {
		// A finalized invoice can carry value without line items (e.g. a one-off created
		// with only amounts). Recomputing from zero copied items would erase that value
		// after the original was already voided — keep the copied subtotal instead.
		draft.Total = draft.Subtotal
		draft.AmountDue = draft.Total
		draft.AmountRemaining = draft.Total
	}

	if err := s.InvoiceRepo.CreateWithLineItems(ctx, draft); err != nil {
		s.Logger.Error(ctx, "finalized invoice edit failed after voiding",
			"error", err, "invoice_id", voided.ID)
		return nil, nil, err
	}

	// Carry the discount over by re-deriving it from the subscription's current coupon
	// associations — this creates real coupon-application records for the copy instead
	// of copying totals whose backing records reference the voided invoice.
	if draft.SubscriptionID != nil {
		if err := s.recalculateDiscountOnInvoice(ctx, draft); err != nil {
			return nil, nil, err
		}
		if err := s.InvoiceRepo.Update(ctx, draft); err != nil {
			return nil, nil, err
		}
	}

	voided.RecalculatedInvoiceID = lo.ToPtr(draft.ID)
	if err := s.InvoiceRepo.Update(ctx, voided); err != nil {
		return nil, nil, err
	}

	s.Logger.Info(ctx, "voided finalized invoice and recreated as draft for editing",
		"original_invoice_id", voided.ID, "draft_invoice_id", draft.ID)

	return draft, idMap, nil
}
