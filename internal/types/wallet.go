package types

import (
	"fmt"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// WalletStatus represents the current state of a wallet
type WalletStatus string

const (
	WalletStatusActive WalletStatus = "active"
	WalletStatusFrozen WalletStatus = "frozen"
	WalletStatusClosed WalletStatus = "closed"
)

// WalletType represents the type of wallet
type WalletType string

const (
	WalletTypePromotional WalletType = "PROMOTIONAL"
	WalletTypePrePaid     WalletType = "PRE_PAID"
)

func (t WalletType) Validate() error {
	if t == "" {
		return nil
	}

	allowedValues := []string{
		string(WalletTypePromotional),
		string(WalletTypePrePaid),
	}
	if !lo.Contains(allowedValues, string(t)) {
		return fmt.Errorf("invalid wallet type: %s", t)
	}
	return nil
}

// TransactionReason represents the reason for a wallet transaction
type TransactionReason string

const (
	TransactionReasonInvoicePayment          TransactionReason = "INVOICE_PAYMENT"
	TransactionReasonFreeCredit              TransactionReason = "FREE_CREDIT_GRANT"
	TransactionReasonSubscriptionCredit      TransactionReason = "SUBSCRIPTION_CREDIT_GRANT"
	TransactionReasonPurchasedCreditInvoiced TransactionReason = "PURCHASED_CREDIT_INVOICED"
	TransactionReasonPurchasedCreditDirect   TransactionReason = "PURCHASED_CREDIT_DIRECT"
	TransactionReasonInvoiceRefund           TransactionReason = "INVOICE_REFUND"
	TransactionReasonCreditExpired           TransactionReason = "CREDIT_EXPIRED"
	TransactionReasonWalletTermination       TransactionReason = "WALLET_TERMINATION"
)

func (t TransactionReason) Validate() error {
	allowedValues := []string{
		string(TransactionReasonInvoicePayment),
		string(TransactionReasonFreeCredit),
		string(TransactionReasonSubscriptionCredit),
		string(TransactionReasonPurchasedCreditInvoiced),
		string(TransactionReasonPurchasedCreditDirect),
		string(TransactionReasonInvoiceRefund),
		string(TransactionReasonCreditExpired),
	}
	if !lo.Contains(allowedValues, string(t)) {
		return fmt.Errorf("invalid transaction reason: %s", t)
	}
	return nil
}

type WalletTxReferenceType string

const (
	WalletTxReferenceTypeInvoice  WalletTxReferenceType = "INVOICE"
	WalletTxReferenceTypePayment  WalletTxReferenceType = "PAYMENT"
	WalletTxReferenceTypeExternal WalletTxReferenceType = "EXTERNAL"
)

func (t WalletTxReferenceType) Validate() error {
	allowedValues := []string{
		string(WalletTxReferenceTypeInvoice),
		string(WalletTxReferenceTypePayment),
		string(WalletTxReferenceTypeExternal),
	}
	if !lo.Contains(allowedValues, string(t)) {
		return fmt.Errorf("invalid wallet transaction reference type: %s", t)
	}
	return nil
}

// AutoTopupTrigger represents the type of trigger for auto top-up
type AutoTopupTrigger string

const (
	// AutoTopupTriggerDisabled represents disabled auto top-up
	AutoTopupTriggerDisabled AutoTopupTrigger = "disabled"
	// AutoTopupTriggerBalanceBelowThreshold represents auto top-up when balance goes below threshold
	AutoTopupTriggerBalanceBelowThreshold AutoTopupTrigger = "balance_below_threshold"
)

func (t AutoTopupTrigger) Validate() error {
	allowedValues := []string{
		string(AutoTopupTriggerDisabled),
		string(AutoTopupTriggerBalanceBelowThreshold),
	}
	if t == "" {
		return nil
	}

	if !lo.Contains(allowedValues, string(t)) {
		return fmt.Errorf("invalid auto top-up trigger: %s", t)
	}
	return nil
}

// String returns the string representation of AutoTopupTrigger
func (t AutoTopupTrigger) String() string {
	return string(t)
}

// WalletTransactionFilter represents the filter options for wallet transactions
type WalletTransactionFilter struct {
	*QueryFilter
	*TimeRangeFilter
	WalletID                 *string            `json:"wallet_id,omitempty"`
	Type                     *TransactionType   `json:"type,omitempty"`
	TransactionStatus        *TransactionStatus `json:"transaction_status,omitempty"`
	ReferenceType            *string            `json:"reference_type,omitempty"`
	ReferenceID              *string            `json:"reference_id,omitempty"`
	ExpiryDateBefore         *time.Time         `json:"expiry_date_before,omitempty"`
	ExpiryDateAfter          *time.Time         `json:"expiry_date_after,omitempty"`
	CreditsAvailableLessThan *decimal.Decimal   `json:"credits_available_less_than,omitempty"`
	TransactionReason        *TransactionReason `json:"transaction_reason,omitempty"`
}

func NewWalletTransactionFilter() *WalletTransactionFilter {
	return &WalletTransactionFilter{
		QueryFilter: NewDefaultQueryFilter(),
	}
}

func NewNoLimitWalletTransactionFilter() *WalletTransactionFilter {
	return &WalletTransactionFilter{
		QueryFilter: NewNoLimitQueryFilter(),
	}
}

func (f WalletTransactionFilter) Validate() error {
	if f.QueryFilter != nil {
		if err := f.QueryFilter.Validate(); err != nil {
			return err
		}
	}

	if f.Type != nil {
		if err := f.Type.Validate(); err != nil {
			return err
		}
	}

	if f.TransactionStatus != nil {
		if err := f.TransactionStatus.Validate(); err != nil {
			return err
		}
	}

	if f.ReferenceType != nil && f.ReferenceID == nil || f.ReferenceID != nil && *f.ReferenceID == "" {
		return fmt.Errorf("reference_type and reference_id must be provided together")
	}

	if f.TimeRangeFilter != nil {
		if err := f.TimeRangeFilter.Validate(); err != nil {
			return err
		}
	}

	if f.ExpiryDateBefore != nil && f.ExpiryDateAfter != nil {
		if f.ExpiryDateBefore.Before(*f.ExpiryDateAfter) {
			return fmt.Errorf("expiry_date_before must be after expiry_date_after")
		}
	}

	if f.TransactionReason != nil {
		// Add validation for transaction reason if needed
	}

	return nil
}

// GetLimit implements BaseFilter interface
func (f *WalletTransactionFilter) GetLimit() int {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetLimit()
	}
	return f.QueryFilter.GetLimit()
}

// GetOffset implements BaseFilter interface
func (f *WalletTransactionFilter) GetOffset() int {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetOffset()
	}
	return f.QueryFilter.GetOffset()
}

// GetSort implements BaseFilter interface
func (f *WalletTransactionFilter) GetSort() string {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetSort()
	}
	return f.QueryFilter.GetSort()
}

// GetOrder implements BaseFilter interface
func (f *WalletTransactionFilter) GetOrder() string {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetOrder()
	}
	return f.QueryFilter.GetOrder()
}

// GetStatus implements BaseFilter interface
func (f *WalletTransactionFilter) GetStatus() string {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetStatus()
	}
	return f.QueryFilter.GetStatus()
}

// GetExpand implements BaseFilter interface
func (f *WalletTransactionFilter) GetExpand() Expand {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetExpand()
	}
	return f.QueryFilter.GetExpand()
}

func (f *WalletTransactionFilter) IsUnlimited() bool {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().IsUnlimited()
	}
	return f.QueryFilter.IsUnlimited()
}

type WalletConfigPriceType string

const (
	WalletConfigPriceTypeAll   WalletConfigPriceType = "ALL"
	WalletConfigPriceTypeUsage WalletConfigPriceType = WalletConfigPriceType(PRICE_TYPE_USAGE)
	WalletConfigPriceTypeFixed WalletConfigPriceType = WalletConfigPriceType(PRICE_TYPE_FIXED)
)

// WalletConfig represents configuration constraints for a wallet
type WalletConfig struct {
	// AllowedPriceTypes is a list of price types that are allowed for the wallet
	// nil means all price types are allowed
	AllowedPriceTypes []WalletConfigPriceType `json:"allowed_price_types,omitempty"`
}

func GetDefaultWalletConfig() *WalletConfig {
	return &WalletConfig{
		AllowedPriceTypes: []WalletConfigPriceType{WalletConfigPriceTypeAll},
	}
}

func (c WalletConfig) Validate() error {
	allowedPriceTypes := []string{
		string(WalletConfigPriceTypeAll),
		string(WalletConfigPriceTypeUsage),
		string(WalletConfigPriceTypeFixed),
	}

	if c.AllowedPriceTypes != nil {
		for _, priceType := range c.AllowedPriceTypes {
			if !lo.Contains(allowedPriceTypes, string(priceType)) {
				return fmt.Errorf("invalid price type: %s", priceType)
			}
		}
	}
	return nil
}
