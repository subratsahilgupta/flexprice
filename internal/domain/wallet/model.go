package wallet

import (
	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// Wallet represents a credit wallet for a customer
type Wallet struct {
	ID                  string                 `db:"id" json:"id"`
	CustomerID          string                 `db:"customer_id" json:"customer_id"`
	Currency            string                 `db:"currency" json:"currency"`
	Balance             decimal.Decimal        `db:"balance" json:"balance"`
	CreditBalance       decimal.Decimal        `db:"credit_balance" json:"credit_balance"`
	WalletStatus        types.WalletStatus     `db:"wallet_status" json:"wallet_status"`
	Name                string                 `json:"name,omitempty"`
	Description         string                 `db:"description" json:"description"`
	Metadata            types.Metadata         `db:"metadata" json:"metadata"`
	AutoTopupTrigger    types.AutoTopupTrigger `db:"auto_topup_trigger" json:"auto_topup_trigger"`
	AutoTopupMinBalance decimal.Decimal        `db:"auto_topup_min_balance" json:"auto_topup_min_balance"`
	AutoTopupAmount     decimal.Decimal        `db:"auto_topup_amount" json:"auto_topup_amount"`
	WalletType          types.WalletType       `db:"wallet_type" json:"wallet_type"`
	Config              types.WalletConfig     `db:"config" json:"config"`
	ConversionRate      decimal.Decimal        `db:"conversion_rate" json:"conversion_rate"`
	EnvironmentID       string                 `db:"environment_id" json:"environment_id"`
	types.BaseModel
}

func (w *Wallet) TableName() string {
	return "wallets"
}

func (w *Wallet) Validate() error {
	if w.ConversionRate.LessThanOrEqual(decimal.Zero) {
		return errors.New(errors.ErrCodeValidation, "conversion rate must be greater than 0")
	}

	if !w.Balance.Equal(w.CreditBalance.Mul(w.ConversionRate)) {
		return errors.New(errors.ErrCodeValidation, "balance and credit balance do not match")
	}

	return nil
}

// ApplyConversionRate applies the conversion rate to the wallet
// so for conversion rate of 2 means 1 credit = 2 dollars (assuming USD)
// and similarly for conversion rate of 0.5 means 1 dollar = 0.5 credits
func (w *Wallet) ApplyConversionRate(rate decimal.Decimal) *Wallet {
	w.Balance = w.CreditBalance.Mul(rate)
	return w
}

// FromEnt converts an ent wallet to a domain wallet
func FromEnt(e *ent.Wallet) *Wallet {
	if e == nil {
		return nil
	}

	var autoTopupMinBalance, autoTopupAmount decimal.Decimal
	if e.AutoTopupMinBalance != nil {
		autoTopupMinBalance = *e.AutoTopupMinBalance
	}
	if e.AutoTopupAmount != nil {
		autoTopupAmount = *e.AutoTopupAmount
	}

	return &Wallet{
		ID:                  e.ID,
		CustomerID:          e.CustomerID,
		Currency:            e.Currency,
		Balance:             e.Balance,
		CreditBalance:       e.CreditBalance,
		WalletStatus:        types.WalletStatus(e.WalletStatus),
		Name:                e.Name,
		Description:         e.Description,
		Metadata:            e.Metadata,
		AutoTopupTrigger:    types.AutoTopupTrigger(lo.FromPtr(e.AutoTopupTrigger)),
		AutoTopupMinBalance: autoTopupMinBalance,
		AutoTopupAmount:     autoTopupAmount,
		WalletType:          types.WalletType(e.WalletType),
		Config:              e.Config,
		ConversionRate:      e.ConversionRate,
		EnvironmentID:       e.EnvironmentID,
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

// FromEntList converts a list of ent wallets to domain wallets
func FromEntList(es []*ent.Wallet) []*Wallet {
	if es == nil {
		return nil
	}

	wallets := make([]*Wallet, len(es))
	for i, e := range es {
		wallets[i] = FromEnt(e)
	}
	return wallets
}
