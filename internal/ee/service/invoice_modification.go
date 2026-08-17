package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
)

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
	var (
		resp *dto.InvoiceResponse
		err  error
	)

	switch params.Action {
	case dto.InvoiceModifyLineItemActionAdd:
		resp, err = s.AddBulkLineItem(ctx, invoiceID, dto.AddBulkLineItemRequest{
			Items: params.Items,
		})
	case dto.InvoiceModifyLineItemActionRemove:
		resp, err = s.RemoveBulkLineItem(ctx, invoiceID, dto.RemoveBulkLineItemRequest{
			LineItemIDs: params.LineItemIDs,
		})
	default:
		return nil, ierr.NewError("unknown line item action: " + string(params.Action)).
			WithHint("valid values: add, remove").
			Mark(ierr.ErrValidation)
	}
	if err != nil {
		return nil, err
	}

	return &dto.InvoiceModifyResponse{Invoice: resp}, nil
}
