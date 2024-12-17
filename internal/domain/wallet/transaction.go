package wallet

import (
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// Transaction represents a wallet transaction
type Transaction struct {
	ID            string                  `db:"id" json:"id"`
	WalletID      string                  `db:"wallet_id" json:"wallet_id"`
	Type          types.TransactionType   `db:"type" json:"type"`
	Amount        decimal.Decimal         `db:"amount" json:"amount"`
	BalanceBefore decimal.Decimal         `db:"balance_before" json:"balance_before"`
	BalanceAfter  decimal.Decimal         `db:"balance_after" json:"balance_after"`
	TxStatus      types.TransactionStatus `db:"transaction_status" json:"transaction_status"`
	ReferenceType string                  `db:"reference_type" json:"reference_type"`
	ReferenceID   string                  `db:"reference_id" json:"reference_id"`
	Description   string                  `db:"description" json:"description"`
	Metadata      types.Metadata          `db:"metadata" json:"metadata"`
	types.BaseModel
}

func (t *Transaction) TableName() string {
	return "wallet_transactions"
}
