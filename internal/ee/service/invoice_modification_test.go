package service

import (
	"testing"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/invoice"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type InvoiceModificationServiceSuite struct {
	testutil.BaseServiceTestSuite
	service InvoiceService
}

func TestInvoiceModification(t *testing.T) {
	suite.Run(t, new(InvoiceModificationServiceSuite))
}

func (s *InvoiceModificationServiceSuite) SetupTest() {
	s.BaseServiceTestSuite.SetupTest()
	s.service = NewInvoiceService(ServiceParams{
		Logger:              s.GetLogger(),
		Config:              s.GetConfig(),
		DB:                  s.GetDB(),
		InvoiceRepo:         s.GetStores().InvoiceRepo,
		InvoiceLineItemRepo: s.GetStores().InvoiceLineItemRepo,
	})
}

func (s *InvoiceModificationServiceSuite) createDraftInvoice() *invoice.Invoice {
	ctx := s.GetContext()
	inv := &invoice.Invoice{
		ID:            types.GenerateUUIDWithPrefix(types.UUID_PREFIX_INVOICE),
		CustomerID:    "cust_test",
		InvoiceType:   types.InvoiceTypeSubscription,
		InvoiceStatus: types.InvoiceStatusDraft,
		Currency:      "usd",
		EnvironmentID: types.GetEnvironmentID(ctx),
		BaseModel:     types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().InvoiceRepo.Create(ctx, inv))
	return inv
}

func (s *InvoiceModificationServiceSuite) TestExecuteAddLineItem() {
	ctx := s.GetContext()
	inv := s.createDraftInvoice()

	resp, err := s.service.ModifyInvoice(ctx, inv.ID, dto.ExecuteInvoiceModifyRequest{
		Type: dto.InvoiceModifyTypeLineItem,
		LineItemParams: &dto.InvoiceModifyLineItemParams{
			Action: dto.InvoiceModifyLineItemActionAdd,
			Items: []dto.AddLineItemRequest{
				{
					DisplayName: "First Item",
					Amount:      decimal.NewFromInt(100),
					Quantity:    decimal.NewFromInt(1),
				},
				{
					DisplayName: "Second Item",
					Amount:      decimal.NewFromInt(50),
					Quantity:    decimal.NewFromInt(2),
				},
			},
		},
	})
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Invoice)
	s.Require().Len(resp.Invoice.LineItems, 2)
	s.True(resp.Invoice.Subtotal.Equal(decimal.NewFromInt(150)))
}

func (s *InvoiceModificationServiceSuite) TestExecuteRemoveLineItem() {
	ctx := s.GetContext()
	inv := s.createDraftInvoice()

	li := &invoice.InvoiceLineItem{
		ID:          types.GenerateUUIDWithPrefix(types.UUID_PREFIX_INVOICE_LINE_ITEM),
		InvoiceID:   inv.ID,
		CustomerID:  inv.CustomerID,
		DisplayName: lo.ToPtr("Item To Remove"),
		Amount:      decimal.NewFromInt(100),
		Quantity:    decimal.NewFromInt(1),
		Currency:    "usd",
		BaseModel:   types.GetDefaultBaseModel(ctx),
	}
	s.NoError(s.GetStores().InvoiceLineItemRepo.Create(ctx, li))

	resp, err := s.service.ModifyInvoice(ctx, inv.ID, dto.ExecuteInvoiceModifyRequest{
		Type: dto.InvoiceModifyTypeLineItem,
		LineItemParams: &dto.InvoiceModifyLineItemParams{
			Action:      dto.InvoiceModifyLineItemActionRemove,
			LineItemIDs: []string{li.ID},
		},
	})
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Invoice)
	s.Len(resp.Invoice.LineItems, 0)
}

func (s *InvoiceModificationServiceSuite) TestExecuteMarksInvoiceAsManuallyEdited() {
	ctx := s.GetContext()
	inv := s.createDraftInvoice()

	_, err := s.service.ModifyInvoice(ctx, inv.ID, dto.ExecuteInvoiceModifyRequest{
		Type: dto.InvoiceModifyTypeLineItem,
		LineItemParams: &dto.InvoiceModifyLineItemParams{
			Action: dto.InvoiceModifyLineItemActionAdd,
			Items: []dto.AddLineItemRequest{
				{
					DisplayName: "First Item",
					Amount:      decimal.NewFromInt(100),
					Quantity:    decimal.NewFromInt(1),
				},
			},
		},
	})
	s.NoError(err)

	updatedInv, err := s.GetStores().InvoiceRepo.Get(ctx, inv.ID)
	s.NoError(err)
	s.True(updatedInv.IsManuallyEdited)
}

func (s *InvoiceModificationServiceSuite) TestExecuteRejectsUnknownType() {
	ctx := s.GetContext()
	inv := s.createDraftInvoice()

	_, err := s.service.ModifyInvoice(ctx, inv.ID, dto.ExecuteInvoiceModifyRequest{
		Type: "bogus",
	})
	s.Error(err)
	s.True(ierr.IsValidation(err))
}

func (s *InvoiceModificationServiceSuite) TestExecuteRejectsUnknownAction() {
	ctx := s.GetContext()
	inv := s.createDraftInvoice()

	_, err := s.service.ModifyInvoice(ctx, inv.ID, dto.ExecuteInvoiceModifyRequest{
		Type: dto.InvoiceModifyTypeLineItem,
		LineItemParams: &dto.InvoiceModifyLineItemParams{
			Action: "bogus",
		},
	})
	s.Error(err)
	s.True(ierr.IsValidation(err))
}

func (s *InvoiceModificationServiceSuite) TestExecuteRejectsMissingLineItemParams() {
	ctx := s.GetContext()
	inv := s.createDraftInvoice()

	_, err := s.service.ModifyInvoice(ctx, inv.ID, dto.ExecuteInvoiceModifyRequest{
		Type: dto.InvoiceModifyTypeLineItem,
	})
	s.Error(err)
	s.True(ierr.IsValidation(err))
}

func (s *InvoiceModificationServiceSuite) TestExecutePropagatesUnderlyingErrors() {
	ctx := s.GetContext()
	inv := s.createDraftInvoice()
	inv.InvoiceStatus = types.InvoiceStatusFinalized
	s.NoError(s.GetStores().InvoiceRepo.Update(ctx, inv))

	_, err := s.service.ModifyInvoice(ctx, inv.ID, dto.ExecuteInvoiceModifyRequest{
		Type: dto.InvoiceModifyTypeLineItem,
		LineItemParams: &dto.InvoiceModifyLineItemParams{
			Action: dto.InvoiceModifyLineItemActionAdd,
			Items: []dto.AddLineItemRequest{
				{
					DisplayName: "Should Not Be Added",
					Amount:      decimal.NewFromInt(100),
					Quantity:    decimal.NewFromInt(1),
				},
			},
		},
	})
	s.Error(err)
	s.True(ierr.IsValidation(err))
}
