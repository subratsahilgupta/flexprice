package wallet

import (
	"time"

	"github.com/flexprice/flexprice/internal/errors"
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

func (w *WalletOperation) Validate() error {
	if err := w.Type.Validate(); err != nil {
		return errors.New(errors.ErrCodeValidation, "invalid transaction type")
	}

	if w.Amount.IsZero() && w.CreditAmount.IsZero() {
		return errors.New(errors.ErrCodeValidation, "amount or credit_amount must be provided")
	}

	if err := w.TransactionReason.Validate(); err != nil {
		return err
	}

	if w.ExpiryDate != nil {
		expiryTime := types.ParseYYYYMMDDToDate(w.ExpiryDate)
		if expiryTime == nil {
			return errors.New(errors.ErrCodeValidation, "invalid expiry date")
		}

		if expiryTime.Before(time.Now().UTC()) {
			return errors.New(errors.ErrCodeValidation, "expiry date cannot be in the past")
		}
	}

	return nil
}
