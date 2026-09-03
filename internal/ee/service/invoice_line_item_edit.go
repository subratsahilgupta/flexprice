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
	// draft copy, and the modification lands on the copy. A call that still targets the
	// voided original (a retry, or a client that did not chain the returned invoice id)
	// is redirected to the replacement draft.
	inv, err := s.InvoiceRepo.Get(ctx, invoiceID)
	if err != nil {
		return nil, err
	}
	inv, err = s.followReplacementChain(ctx, inv)
	if err != nil {
		return nil, err
	}
	targetID := inv.ID
	if inv.InvoiceStatus == types.InvoiceStatusFinalized {
		// Validate the referenced line items BEFORE voiding: a bad id must fail the
		// request while the finalized invoice is still intact, not after the void.
		if err := s.validateLineItemRefsAgainstInvoice(ctx, inv, params); err != nil {
			return nil, err
		}
		draft, err := s.voidAndRecreateDraftForEdit(ctx, inv)
		if err != nil {
			return nil, err
		}
		targetID = draft.ID
	}

	var resp *dto.InvoiceResponse

	switch params.Action {
	case dto.InvoiceModifyLineItemActionAdd:
		resp, err = s.AddBulkLineItem(ctx, targetID, dto.AddBulkLineItemRequest{
			Items: params.Items,
		})
	case dto.InvoiceModifyLineItemActionUpdate:
		lineItemID, resolveErr := s.resolveLineItemIDForEdit(ctx, targetID, params.LineItemID)
		if resolveErr != nil {
			return nil, resolveErr
		}
		resp, err = s.UpdateLineItem(ctx, targetID, lineItemID, *params.Update)
	case dto.InvoiceModifyLineItemActionRemove:
		lineItemIDs := make([]string, 0, len(params.LineItemIDs))
		for _, id := range params.LineItemIDs {
			resolved, resolveErr := s.resolveLineItemIDForEdit(ctx, targetID, id)
			if resolveErr != nil {
				return nil, resolveErr
			}
			lineItemIDs = append(lineItemIDs, resolved)
		}
		resp, err = s.RemoveBulkLineItem(ctx, targetID, dto.RemoveBulkLineItemRequest{
			LineItemIDs: lineItemIDs,
		})
	default:
		return nil, ierr.NewError("unknown line item action: " + string(params.Action)).
			WithHint("valid values: add, update, remove").
			Mark(ierr.ErrValidation)
	}
	if err != nil {
		return nil, err
	}

	return &dto.InvoiceModifyResponse{Invoice: resp}, nil
}

// followReplacementChain resolves a voided invoice to its current replacement by
// following recalculated_invoice_id links — a replacement draft can itself be
// finalized, edited, and voided again, giving the chain more than one hop.
func (s *invoiceService) followReplacementChain(ctx context.Context, inv *invoice.Invoice) (*invoice.Invoice, error) {
	const maxHops = 10
	current := inv
	for range maxHops {
		if current.InvoiceStatus != types.InvoiceStatusVoided || current.RecalculatedInvoiceID == nil {
			return current, nil
		}
		next, err := s.InvoiceRepo.Get(ctx, *current.RecalculatedInvoiceID)
		if err != nil {
			return nil, err
		}
		current = next
	}
	return nil, ierr.NewError("invoice replacement chain is too deep").
		WithHintf("invoice %s has more than %d voided replacements", inv.ID, maxHops).
		Mark(ierr.ErrValidation)
}

// validateLineItemRefsAgainstInvoice checks that every line item id an update/remove
// references exists (published) on the invoice, so the void-and-recreate flow cannot
// void a finalized invoice for a request that would fail afterwards.
func (s *invoiceService) validateLineItemRefsAgainstInvoice(ctx context.Context, inv *invoice.Invoice, params *dto.InvoiceModifyLineItemParams) error {
	if params.Action != dto.InvoiceModifyLineItemActionUpdate && params.Action != dto.InvoiceModifyLineItemActionRemove {
		return nil
	}
	// List directly rather than trusting inv.LineItems: a cached or partially
	// hydrated invoice may not carry its line items.
	items, err := s.InvoiceLineItemRepo.ListByInvoiceID(ctx, inv.ID)
	if err != nil {
		return err
	}
	published := make(map[string]bool, len(items))
	for _, li := range items {
		if li.Status == types.StatusPublished {
			published[li.ID] = true
		}
	}
	refs := params.LineItemIDs
	if params.Action == dto.InvoiceModifyLineItemActionUpdate {
		refs = []string{params.LineItemID}
	}
	for _, id := range refs {
		if !published[id] {
			return ierr.NewError("line item not found").
				WithHintf("line item %s does not belong to invoice %s", id, inv.ID).
				Mark(ierr.ErrNotFound)
		}
	}
	return nil
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
// copy carrying the invoice's data — description, billing period, period start/end, due date,
// issue date, metadata, PDF URL, totals — and every published line item with its own fields
// (display name, amount, quantity, price/meter linkage, period start/end). Copied line items
// point at their originals via parent_line_item_id, so ids from the original invoice remain
// addressable. The original is linked to the copy via recalculated_invoice_id.
func (s *invoiceService) voidAndRecreateDraftForEdit(ctx context.Context, inv *invoice.Invoice) (*invoice.Invoice, error) {
	if inv.RecalculatedInvoiceID != nil {
		return nil, ierr.NewError("invoice already has a replacement").
			WithHintf("invoice %s was already voided and recreated as %s", inv.ID, *inv.RecalculatedInvoiceID).
			WithReportableDetails(map[string]any{"recalculated_invoice_id": *inv.RecalculatedInvoiceID}).
			Mark(ierr.ErrValidation)
	}

	if err := s.VoidInvoice(ctx, inv.ID, dto.InvoiceVoidRequest{}); err != nil {
		return nil, err
	}
	voided, err := s.InvoiceRepo.Get(ctx, inv.ID)
	if err != nil {
		return nil, err
	}

	draftID := types.GenerateUUIDWithPrefix(types.UUID_PREFIX_INVOICE)
	baseModel := types.GetDefaultBaseModel(ctx)

	// List directly rather than trusting voided.LineItems hydration.
	sourceItems, err := s.InvoiceLineItemRepo.ListByInvoiceID(ctx, voided.ID)
	if err != nil {
		return nil, err
	}

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
		lineItems = append(lineItems, copied)
	}

	var metadata types.Metadata
	if voided.Metadata != nil {
		metadata = lo.Assign(types.Metadata{}, voided.Metadata)
	}

	draft := &invoice.Invoice{
		ID:                     draftID,
		CustomerID:             voided.CustomerID,
		SubscriptionID:         voided.SubscriptionID,
		SubscriptionCustomerID: voided.SubscriptionCustomerID,
		InvoiceType:            voided.InvoiceType,
		InvoiceStatus:          types.InvoiceStatusDraft,
		PaymentStatus:          types.PaymentStatusPending,
		Currency:               voided.Currency,
		// Payment and credit state does not carry over: voiding refunded any captured
		// payment and applied credits back to the customer, so the draft starts clean.
		AmountPaid:                 decimal.Zero,
		TotalPrepaidCreditsApplied: decimal.Zero,
		Subtotal:                   voided.Subtotal,
		// Discount and tax stay zero: their backing records (coupon applications,
		// tax-applied rows) reference the voided invoice and are not copied. They are
		// re-derived on the draft — apply_discount recalculates from current coupon
		// associations, and taxes are recalculated when the draft is finalized.
		TotalDiscount: decimal.Zero,
		TotalTax:      decimal.Zero,
		Description:                voided.Description,
		DueDate:                    voided.DueDate,
		BillingPeriod:              voided.BillingPeriod,
		IssueDate:                  voided.IssueDate,
		PeriodStart:                voided.PeriodStart,
		PeriodEnd:                  voided.PeriodEnd,
		InvoicePDFURL:              voided.InvoicePDFURL,
		BillingReason:              voided.BillingReason,
		BillingSequence:            voided.BillingSequence,
		Metadata:                   metadata,
		EnvironmentID:              voided.EnvironmentID,
		// Deterministic key: a retried recreate resolves to the same replacement instead
		// of racing a duplicate (idempotency lookups exclude VOIDED invoices).
		IdempotencyKey:   lo.ToPtr("void_recreate-" + voided.ID),
		IsManuallyEdited: true,
		Version:          1,
		BaseModel:        baseModel,
		LineItems:        lineItems,
	}
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
		s.Logger.Error(ctx, "finalized invoice edit failed after voiding, invoice left voided with no replacement",
			"error", err, "invoice_id", voided.ID)
		return nil, err
	}

	voided.RecalculatedInvoiceID = lo.ToPtr(draft.ID)
	if err := s.InvoiceRepo.Update(ctx, voided); err != nil {
		return nil, err
	}

	s.Logger.Info(ctx, "voided finalized invoice and recreated as draft for editing",
		"original_invoice_id", voided.ID, "draft_invoice_id", draft.ID)

	return draft, nil
}
