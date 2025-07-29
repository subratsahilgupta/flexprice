package service

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/internal/domain/coupon"
	"github.com/flexprice/flexprice/internal/domain/discount"
	"github.com/flexprice/flexprice/internal/domain/redemption"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

// DiscountEngineService provides discount application logic
type DiscountEngineService struct {
	couponRepo     coupon.Repository
	discountRepo   discount.Repository
	redemptionRepo redemption.Repository
}

// NewDiscountEngineService creates a new discount engine service
func NewDiscountEngineService(
	couponRepo coupon.Repository,
	discountRepo discount.Repository,
	redemptionRepo redemption.Repository,
) *DiscountEngineService {
	return &DiscountEngineService{
		couponRepo:     couponRepo,
		discountRepo:   discountRepo,
		redemptionRepo: redemptionRepo,
	}
}

// DiscountApplication represents the result of applying a discount
type DiscountApplication struct {
	CouponID         string             `json:"coupon_id"`
	DiscountID       string             `json:"discount_id"`
	OriginalPrice    decimal.Decimal    `json:"original_price"`
	FinalPrice       decimal.Decimal    `json:"final_price"`
	DiscountedAmount decimal.Decimal    `json:"discounted_amount"`
	DiscountType     types.DiscountType `json:"discount_type"`
	Applied          bool               `json:"applied"`
	Reason           string             `json:"reason,omitempty"`
}

// ApplyDiscountToInvoice applies available discounts to an invoice
func (s *DiscountEngineService) ApplyDiscountToInvoice(
	ctx context.Context,
	invoiceID string,
	customerID string,
	subscriptionID *string,
	lineItems []InvoiceLineItem,
) ([]DiscountApplication, error) {
	var applications []DiscountApplication

	// Get all active discounts for the subscription
	var discounts []*discount.Discount
	var err error

	if subscriptionID != nil {
		discounts, err = s.discountRepo.GetBySubscription(ctx, *subscriptionID)
		if err != nil {
			return nil, fmt.Errorf("failed to get subscription discounts: %w", err)
		}
	}

	// Apply subscription-level discounts to invoice total
	subscriptionDiscounts := s.filterSubscriptionLevelDiscounts(discounts)
	for _, d := range subscriptionDiscounts {
		app, err := s.applyDiscountToInvoice(ctx, d, invoiceID, customerID, lineItems)
		if err != nil {
			return nil, err
		}
		if app != nil {
			applications = append(applications, *app)
		}
	}

	// Apply line-item-level discounts
	lineItemDiscounts := s.filterLineItemLevelDiscounts(discounts)
	for _, d := range lineItemDiscounts {
		apps, err := s.applyDiscountToLineItems(ctx, d, invoiceID, customerID, lineItems)
		if err != nil {
			return nil, err
		}
		applications = append(applications, apps...)
	}

	return applications, nil
}

// InvoiceLineItem represents an invoice line item for discount calculation
type InvoiceLineItem struct {
	ID                     string          `json:"id"`
	SubscriptionLineItemID *string         `json:"subscription_line_item_id,omitempty"`
	Amount                 decimal.Decimal `json:"amount"`
	Currency               string          `json:"currency"`
}

func (s *DiscountEngineService) filterSubscriptionLevelDiscounts(discounts []*discount.Discount) []*discount.Discount {
	var result []*discount.Discount
	for _, d := range discounts {
		if d.IsSubscriptionLevel() {
			result = append(result, d)
		}
	}
	return result
}

func (s *DiscountEngineService) filterLineItemLevelDiscounts(discounts []*discount.Discount) []*discount.Discount {
	var result []*discount.Discount
	for _, d := range discounts {
		if d.IsLineItemLevel() {
			result = append(result, d)
		}
	}
	return result
}

func (s *DiscountEngineService) applyDiscountToInvoice(
	ctx context.Context,
	d *discount.Discount,
	invoiceID string,
	customerID string,
	lineItems []InvoiceLineItem,
) (*DiscountApplication, error) {
	// Get the coupon
	coupon, err := s.couponRepo.Get(ctx, d.CouponID)
	if err != nil {
		return nil, fmt.Errorf("failed to get coupon: %w", err)
	}

	// Check if coupon is valid
	if !coupon.IsValid() {
		return &DiscountApplication{
			CouponID:   d.CouponID,
			DiscountID: d.ID,
			Applied:    false,
			Reason:     "Coupon is not valid",
		}, nil
	}

	// Check if discount rules are satisfied
	if !s.evaluateDiscountRules(coupon, customerID, lineItems) {
		return &DiscountApplication{
			CouponID:   d.CouponID,
			DiscountID: d.ID,
			Applied:    false,
			Reason:     "Discount rules not satisfied",
		}, nil
	}

	// Calculate total invoice amount
	totalAmount := decimal.Zero
	for _, item := range lineItems {
		totalAmount = totalAmount.Add(item.Amount)
	}

	// Apply discount
	finalPrice := coupon.ApplyDiscount(totalAmount)
	discountedAmount := totalAmount.Sub(finalPrice)

	return &DiscountApplication{
		CouponID:         d.CouponID,
		DiscountID:       d.ID,
		OriginalPrice:    totalAmount,
		FinalPrice:       finalPrice,
		DiscountedAmount: discountedAmount,
		DiscountType:     coupon.Type,
		Applied:          true,
	}, nil
}

func (s *DiscountEngineService) applyDiscountToLineItems(
	ctx context.Context,
	d *discount.Discount,
	invoiceID string,
	customerID string,
	lineItems []InvoiceLineItem,
) ([]DiscountApplication, error) {
	var applications []DiscountApplication

	// Get the coupon
	coupon, err := s.couponRepo.Get(ctx, d.CouponID)
	if err != nil {
		return nil, fmt.Errorf("failed to get coupon: %w", err)
	}

	// Check if coupon is valid
	if !coupon.IsValid() {
		return []DiscountApplication{{
			CouponID:   d.CouponID,
			DiscountID: d.ID,
			Applied:    false,
			Reason:     "Coupon is not valid",
		}}, nil
	}

	// Find matching line items
	for _, item := range lineItems {
		if item.SubscriptionLineItemID != nil && *item.SubscriptionLineItemID == d.GetTargetID() {
			// Check if discount rules are satisfied for this line item
			if !s.evaluateDiscountRules(coupon, customerID, []InvoiceLineItem{item}) {
				applications = append(applications, DiscountApplication{
					CouponID:   d.CouponID,
					DiscountID: d.ID,
					Applied:    false,
					Reason:     "Discount rules not satisfied for line item",
				})
				continue
			}

			// Apply discount to line item
			finalPrice := coupon.ApplyDiscount(item.Amount)
			discountedAmount := item.Amount.Sub(finalPrice)

			applications = append(applications, DiscountApplication{
				CouponID:         d.CouponID,
				DiscountID:       d.ID,
				OriginalPrice:    item.Amount,
				FinalPrice:       finalPrice,
				DiscountedAmount: discountedAmount,
				DiscountType:     coupon.Type,
				Applied:          true,
			})
		}
	}

	return applications, nil
}

// evaluateDiscountRules evaluates if the discount rules are satisfied
func (s *DiscountEngineService) evaluateDiscountRules(
	coupon *coupon.Coupon,
	customerID string,
	lineItems []InvoiceLineItem,
) bool {
	if coupon.Rules == nil {
		return true // No rules means always apply
	}

	// Simple rule evaluation - can be extended with a proper rule engine
	rules, ok := coupon.Rules["conditions"].([]interface{})
	if !ok {
		return true
	}

	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		field, _ := ruleMap["field"].(string)
		operator, _ := ruleMap["operator"].(string)
		value := ruleMap["value"]

		if !s.evaluateRule(field, operator, value, customerID, lineItems) {
			return false
		}
	}

	return true
}

// evaluateRule evaluates a single discount rule
func (s *DiscountEngineService) evaluateRule(
	field, operator string,
	value interface{},
	customerID string,
	lineItems []InvoiceLineItem,
) bool {
	switch field {
	case "customer_id":
		return s.evaluateCustomerRule(operator, value, customerID)
	case "amount":
		return s.evaluateAmountRule(operator, value, lineItems)
	default:
		return true // Unknown field, skip rule
	}
}

func (s *DiscountEngineService) evaluateCustomerRule(operator string, value interface{}, customerID string) bool {
	expectedCustomerID, ok := value.(string)
	if !ok {
		return false
	}

	switch operator {
	case "equals":
		return customerID == expectedCustomerID
	case "not_equals":
		return customerID != expectedCustomerID
	default:
		return true
	}
}

func (s *DiscountEngineService) evaluateAmountRule(operator string, value interface{}, lineItems []InvoiceLineItem) bool {
	expectedAmount, ok := value.(float64)
	if !ok {
		return false
	}

	totalAmount := decimal.Zero
	for _, item := range lineItems {
		totalAmount = totalAmount.Add(item.Amount)
	}

	amountFloat, _ := totalAmount.Float64()

	switch operator {
	case "greater_than":
		return amountFloat > expectedAmount
	case "greater_than_or_equal":
		return amountFloat >= expectedAmount
	case "less_than":
		return amountFloat < expectedAmount
	case "less_than_or_equal":
		return amountFloat <= expectedAmount
	case "equals":
		return amountFloat == expectedAmount
	default:
		return true
	}
}

// CreateRedemption creates a redemption record for a discount application
func (s *DiscountEngineService) CreateRedemption(
	ctx context.Context,
	application DiscountApplication,
	invoiceID string,
	invoiceLineItemID *string,
) (*redemption.Redemption, error) {
	if !application.Applied {
		return nil, fmt.Errorf("cannot create redemption for unapplied discount")
	}

	// Get the coupon to create snapshot
	coupon, err := s.couponRepo.Get(ctx, application.CouponID)
	if err != nil {
		return nil, fmt.Errorf("failed to get coupon: %w", err)
	}

	// Create coupon snapshot
	couponSnapshot := map[string]interface{}{
		"id":             coupon.ID,
		"name":           coupon.Name,
		"amount_off":     coupon.AmountOff.String(),
		"percentage_off": coupon.PercentageOff.String(),
		"type":           string(coupon.Type),
		"cadence":        string(coupon.Cadence),
		"currency":       coupon.Currency,
		"rules":          coupon.Rules,
		"snapshot_at":    time.Now().UTC(),
	}

	// Create redemption
	redemption := &redemption.Redemption{
		CouponID:          application.CouponID,
		DiscountID:        application.DiscountID,
		InvoiceID:         invoiceID,
		InvoiceLineItemID: invoiceLineItemID,
		RedeemedAt:        time.Now().UTC(),
		OriginalPrice:     application.OriginalPrice,
		FinalPrice:        application.FinalPrice,
		DiscountedAmount:  application.DiscountedAmount,
		DiscountType:      application.DiscountType,
		Currency:          coupon.Currency,
		CouponSnapshot:    couponSnapshot,
	}

	// Set percentage if applicable
	if application.DiscountType == types.DiscountTypePercentage {
		percentage := coupon.PercentageOff
		redemption.DiscountPercentage = &percentage
	}

	err = s.redemptionRepo.Create(ctx, redemption)
	if err != nil {
		return nil, fmt.Errorf("failed to create redemption: %w", err)
	}

	// Increment coupon redemptions
	err = s.couponRepo.IncrementRedemptions(ctx, application.CouponID)
	if err != nil {
		return nil, fmt.Errorf("failed to increment coupon redemptions: %w", err)
	}

	return redemption, nil
}
