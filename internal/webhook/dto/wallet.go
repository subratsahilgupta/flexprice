package webhookDto

import (
	"github.com/flexprice/flexprice/internal/api/dto"
)

type InternalWalletEvent struct {
	EventType string `json:"event_type"`
	WalletID  string `json:"wallet_id"`
	TenantID  string `json:"tenant_id"`
}

type InternalTransactionEvent struct {
	EventType     string `json:"event_type"`
	TransactionID string `json:"transaction_id"`
	TenantID      string `json:"tenant_id"`
}

// WalletWebhookPayload represents the detailed payload for wallet webhooks
type WalletWebhookPayload struct {
	Wallet *dto.WalletResponse `json:"wallet"`
}

type TransactionWebhookPayload struct {
	Transaction *dto.WalletTransactionResponse `json:"transaction"`
	Wallet      *dto.WalletResponse            `json:"wallet"`
}

func NewWalletWebhookPayload(wallet *dto.WalletResponse) *WalletWebhookPayload {
	return &WalletWebhookPayload{
		Wallet: wallet,
	}
}

func NewTransactionWebhookPayload(transaction *dto.WalletTransactionResponse, wallet *dto.WalletResponse) *TransactionWebhookPayload {
	return &TransactionWebhookPayload{
		Transaction: transaction,
		Wallet:      wallet,
	}
}
