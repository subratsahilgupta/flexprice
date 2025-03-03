package cron

import (
	"context"
	"net/http"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/flexprice/flexprice/internal/temporal"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

type WalletCronHandler struct {
	logger          *logger.Logger
	temporalService *temporal.Service
	walletService   service.WalletService
	tenantService   service.TenantService
}

func NewWalletCronHandler(logger *logger.Logger, temporalService *temporal.Service, walletService service.WalletService, tenantService service.TenantService) *WalletCronHandler {
	return &WalletCronHandler{
		logger:          logger,
		temporalService: temporalService,
		walletService:   walletService,
		tenantService:   tenantService,
	}
}

// ExpireCredits finds and expires credits that have passed their expiry date
func (h *WalletCronHandler) ExpireCredits(c *gin.Context) {
	h.logger.Infow("starting credit expiry cron job - %s", time.Now().UTC().Format(time.RFC3339))

	tenants, err := h.tenantService.GetAllTenants(c.Request.Context())
	if err != nil {
		h.logger.Errorw("failed to get all tenants", "error", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	// Create filter to find expired credits
	filter := &types.WalletTransactionFilter{
		Type:               lo.ToPtr(types.TransactionTypeCredit),
		TransactionStatus:  lo.ToPtr(types.TransactionStatusCompleted),
		ExpiryDateBefore:   lo.ToPtr(time.Now().UTC()),
		CreditsAvailableGT: lo.ToPtr(decimal.Zero),
	}

	response := &dto.ExpiredCreditsResponse{
		Items:   make([]*dto.ExpiredCreditsResponseItem, 0),
		Total:   0,
		Success: 0,
		Failed:  0,
	}

	for _, tenant := range tenants {
		tenantResponse := &dto.ExpiredCreditsResponseItem{
			TenantID: tenant.ID,
			Count:    0,
		}

		h.logger.Infow("tenant", "id", tenant.ID, "name", tenant.Name)
		ctx := context.WithValue(c.Request.Context(), types.CtxTenantID, tenant.ID)
		ctx = context.WithValue(ctx, types.CtxEnvironmentID, "")
		// Get transactions with expired credits
		transactions, err := h.walletService.GetWalletTransactions(ctx, "", filter)
		if err != nil {
			h.logger.Errorw("failed to list expired credits",
				"error", err,
			)
			c.Status(http.StatusInternalServerError)
			return
		}

		h.logger.Infow("found expired credits", "count", len(transactions.Items))

		// Process each expired credit
		for _, tx := range transactions.Items {
			if err := h.walletService.ExpireCredits(ctx, tx.ID); err != nil {
				h.logger.Errorw("failed to expire credits",
					"transaction_id", tx.ID,
					"error", err,
				)
				response.Failed++
				continue
			}

			tenantResponse.Count++
			response.Success++

			h.logger.Infow("expired credits successfully",
				"transaction_id", tx.ID,
				"wallet_id", tx.WalletID,
				"amount", tx.CreditsAvailable,
			)
		}

		response.Items = append(response.Items, tenantResponse)
	}

	h.logger.Infow("completed credit expiry cron job")
	c.JSON(http.StatusOK, response)
}
