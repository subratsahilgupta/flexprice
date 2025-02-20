package wallet

import (
	"time"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// Transaction represents a wallet transaction
type Transaction struct {
	ID                string                  `db:"id" json:"id"`
	WalletID          string                  `db:"wallet_id" json:"wallet_id"`
	Type              types.TransactionType   `db:"type" json:"type"`
	Amount            decimal.Decimal         `db:"amount" json:"amount"`
	BalanceBefore     decimal.Decimal         `db:"balance_before" json:"balance_before"`
	BalanceAfter      decimal.Decimal         `db:"balance_after" json:"balance_after"`
	TxStatus          types.TransactionStatus `db:"transaction_status" json:"transaction_status"`
	ReferenceType     string                  `db:"reference_type" json:"reference_type"`
	ReferenceID       string                  `db:"reference_id" json:"reference_id"`
	Description       string                  `db:"description" json:"description"`
	Metadata          types.Metadata          `db:"metadata" json:"metadata"`
	ExpiryDate        *time.Time              `db:"expiry_date" json:"expiry_date"`
	AmountUsed        decimal.Decimal         `db:"amount_used" json:"amount_used"`
	TransactionReason types.TransactionReason `db:"transaction_reason" json:"transaction_reason"`
	types.BaseModel
}

func (t *Transaction) TableName() string {
	return "wallet_transactions"
}

// ToEnt converts a domain transaction to an ent transaction
func (t *Transaction) ToEnt() *ent.WalletTransaction {
	return &ent.WalletTransaction{
		ID:                t.ID,
		WalletID:          t.WalletID,
		Type:              string(t.Type),
		Amount:            t.Amount,
		BalanceBefore:     t.BalanceBefore,
		BalanceAfter:      t.BalanceAfter,
		TransactionStatus: string(t.TxStatus),
		ReferenceType:     t.ReferenceType,
		ReferenceID:       t.ReferenceID,
		Description:       t.Description,
		Metadata:          t.Metadata,
		ExpiryDate:        t.ExpiryDate,
		AmountUsed:        t.AmountUsed,
		TransactionReason: string(t.TransactionReason),
		TenantID:          t.TenantID,
		Status:            string(t.Status),
		CreatedBy:         t.CreatedBy,
		UpdatedBy:         t.UpdatedBy,
		CreatedAt:         t.CreatedAt,
		UpdatedAt:         t.UpdatedAt,
	}
}

// FromEnt converts an ent transaction to a domain transaction
func TransactionFromEnt(e *ent.WalletTransaction) *Transaction {
	if e == nil {
		return nil
	}

	return &Transaction{
		ID:                e.ID,
		WalletID:          e.WalletID,
		Type:              types.TransactionType(e.Type),
		Amount:            e.Amount,
		BalanceBefore:     e.BalanceBefore,
		BalanceAfter:      e.BalanceAfter,
		TxStatus:          types.TransactionStatus(e.TransactionStatus),
		ReferenceType:     e.ReferenceType,
		ReferenceID:       e.ReferenceID,
		Description:       e.Description,
		Metadata:          types.Metadata(e.Metadata),
		ExpiryDate:        e.ExpiryDate,
		AmountUsed:        e.AmountUsed,
		TransactionReason: types.TransactionReason(e.TransactionReason),
		BaseModel: types.BaseModel{
			TenantID:  e.TenantID,
			Status:    types.Status(e.Status),
			CreatedBy: e.CreatedBy,
			UpdatedBy: e.UpdatedBy,
			CreatedAt: e.CreatedAt,
			UpdatedAt: e.UpdatedAt,
		},
	}
}

// TransactionListFromMap converts a list of maps to domain transactions
func TransactionListFromEnt(dataList []*ent.WalletTransaction) []*Transaction {
	if dataList == nil {
		return nil
	}

	transactions := make([]*Transaction, len(dataList))
	for i, data := range dataList {
		transactions[i] = TransactionFromEnt(data)
	}
	return transactions
}
