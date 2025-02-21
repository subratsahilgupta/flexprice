package wallet

import (
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// WalletOperation represents the request to credit or debit a wallet
type WalletOperation struct {
	WalletID string                `json:"wallet_id"`
	Type     types.TransactionType `json:"type"`
	// Amount is the amount of the transaction in the wallet's currency
	Amount decimal.Decimal `json:"amount"`
	// CreditAmount is the amount of the transaction in the wallet's credit currency
	// If both are provided, Amount is used
	CreditAmount      decimal.Decimal         `json:"credit_amount"`
	ReferenceType     string                  `json:"reference_type,omitempty"`
	ReferenceID       string                  `json:"reference_id,omitempty"`
	Description       string                  `json:"description,omitempty"`
	Metadata          types.Metadata          `json:"metadata,omitempty"`
	ExpiryDate        *int                    `json:"expiry_date,omitempty"` // YYYYMMDD format
	TransactionReason types.TransactionReason `json:"transaction_reason,omitempty"`
}
