// Code generated by ent, DO NOT EDIT.

package ent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/schema/field"
	"github.com/flexprice/flexprice/ent/predicate"
	"github.com/flexprice/flexprice/ent/wallettransaction"
	"github.com/shopspring/decimal"
)

// WalletTransactionUpdate is the builder for updating WalletTransaction entities.
type WalletTransactionUpdate struct {
	config
	hooks    []Hook
	mutation *WalletTransactionMutation
}

// Where appends a list predicates to the WalletTransactionUpdate builder.
func (wtu *WalletTransactionUpdate) Where(ps ...predicate.WalletTransaction) *WalletTransactionUpdate {
	wtu.mutation.Where(ps...)
	return wtu
}

// SetStatus sets the "status" field.
func (wtu *WalletTransactionUpdate) SetStatus(s string) *WalletTransactionUpdate {
	wtu.mutation.SetStatus(s)
	return wtu
}

// SetNillableStatus sets the "status" field if the given value is not nil.
func (wtu *WalletTransactionUpdate) SetNillableStatus(s *string) *WalletTransactionUpdate {
	if s != nil {
		wtu.SetStatus(*s)
	}
	return wtu
}

// SetUpdatedAt sets the "updated_at" field.
func (wtu *WalletTransactionUpdate) SetUpdatedAt(t time.Time) *WalletTransactionUpdate {
	wtu.mutation.SetUpdatedAt(t)
	return wtu
}

// SetUpdatedBy sets the "updated_by" field.
func (wtu *WalletTransactionUpdate) SetUpdatedBy(s string) *WalletTransactionUpdate {
	wtu.mutation.SetUpdatedBy(s)
	return wtu
}

// SetNillableUpdatedBy sets the "updated_by" field if the given value is not nil.
func (wtu *WalletTransactionUpdate) SetNillableUpdatedBy(s *string) *WalletTransactionUpdate {
	if s != nil {
		wtu.SetUpdatedBy(*s)
	}
	return wtu
}

// ClearUpdatedBy clears the value of the "updated_by" field.
func (wtu *WalletTransactionUpdate) ClearUpdatedBy() *WalletTransactionUpdate {
	wtu.mutation.ClearUpdatedBy()
	return wtu
}

// SetType sets the "type" field.
func (wtu *WalletTransactionUpdate) SetType(s string) *WalletTransactionUpdate {
	wtu.mutation.SetType(s)
	return wtu
}

// SetNillableType sets the "type" field if the given value is not nil.
func (wtu *WalletTransactionUpdate) SetNillableType(s *string) *WalletTransactionUpdate {
	if s != nil {
		wtu.SetType(*s)
	}
	return wtu
}

// SetAmount sets the "amount" field.
func (wtu *WalletTransactionUpdate) SetAmount(d decimal.Decimal) *WalletTransactionUpdate {
	wtu.mutation.SetAmount(d)
	return wtu
}

// SetNillableAmount sets the "amount" field if the given value is not nil.
func (wtu *WalletTransactionUpdate) SetNillableAmount(d *decimal.Decimal) *WalletTransactionUpdate {
	if d != nil {
		wtu.SetAmount(*d)
	}
	return wtu
}

// SetCreditAmount sets the "credit_amount" field.
func (wtu *WalletTransactionUpdate) SetCreditAmount(d decimal.Decimal) *WalletTransactionUpdate {
	wtu.mutation.SetCreditAmount(d)
	return wtu
}

// SetNillableCreditAmount sets the "credit_amount" field if the given value is not nil.
func (wtu *WalletTransactionUpdate) SetNillableCreditAmount(d *decimal.Decimal) *WalletTransactionUpdate {
	if d != nil {
		wtu.SetCreditAmount(*d)
	}
	return wtu
}

// SetCreditBalanceBefore sets the "credit_balance_before" field.
func (wtu *WalletTransactionUpdate) SetCreditBalanceBefore(d decimal.Decimal) *WalletTransactionUpdate {
	wtu.mutation.SetCreditBalanceBefore(d)
	return wtu
}

// SetNillableCreditBalanceBefore sets the "credit_balance_before" field if the given value is not nil.
func (wtu *WalletTransactionUpdate) SetNillableCreditBalanceBefore(d *decimal.Decimal) *WalletTransactionUpdate {
	if d != nil {
		wtu.SetCreditBalanceBefore(*d)
	}
	return wtu
}

// SetCreditBalanceAfter sets the "credit_balance_after" field.
func (wtu *WalletTransactionUpdate) SetCreditBalanceAfter(d decimal.Decimal) *WalletTransactionUpdate {
	wtu.mutation.SetCreditBalanceAfter(d)
	return wtu
}

// SetNillableCreditBalanceAfter sets the "credit_balance_after" field if the given value is not nil.
func (wtu *WalletTransactionUpdate) SetNillableCreditBalanceAfter(d *decimal.Decimal) *WalletTransactionUpdate {
	if d != nil {
		wtu.SetCreditBalanceAfter(*d)
	}
	return wtu
}

// SetReferenceType sets the "reference_type" field.
func (wtu *WalletTransactionUpdate) SetReferenceType(s string) *WalletTransactionUpdate {
	wtu.mutation.SetReferenceType(s)
	return wtu
}

// SetNillableReferenceType sets the "reference_type" field if the given value is not nil.
func (wtu *WalletTransactionUpdate) SetNillableReferenceType(s *string) *WalletTransactionUpdate {
	if s != nil {
		wtu.SetReferenceType(*s)
	}
	return wtu
}

// ClearReferenceType clears the value of the "reference_type" field.
func (wtu *WalletTransactionUpdate) ClearReferenceType() *WalletTransactionUpdate {
	wtu.mutation.ClearReferenceType()
	return wtu
}

// SetReferenceID sets the "reference_id" field.
func (wtu *WalletTransactionUpdate) SetReferenceID(s string) *WalletTransactionUpdate {
	wtu.mutation.SetReferenceID(s)
	return wtu
}

// SetNillableReferenceID sets the "reference_id" field if the given value is not nil.
func (wtu *WalletTransactionUpdate) SetNillableReferenceID(s *string) *WalletTransactionUpdate {
	if s != nil {
		wtu.SetReferenceID(*s)
	}
	return wtu
}

// ClearReferenceID clears the value of the "reference_id" field.
func (wtu *WalletTransactionUpdate) ClearReferenceID() *WalletTransactionUpdate {
	wtu.mutation.ClearReferenceID()
	return wtu
}

// SetDescription sets the "description" field.
func (wtu *WalletTransactionUpdate) SetDescription(s string) *WalletTransactionUpdate {
	wtu.mutation.SetDescription(s)
	return wtu
}

// SetNillableDescription sets the "description" field if the given value is not nil.
func (wtu *WalletTransactionUpdate) SetNillableDescription(s *string) *WalletTransactionUpdate {
	if s != nil {
		wtu.SetDescription(*s)
	}
	return wtu
}

// ClearDescription clears the value of the "description" field.
func (wtu *WalletTransactionUpdate) ClearDescription() *WalletTransactionUpdate {
	wtu.mutation.ClearDescription()
	return wtu
}

// SetMetadata sets the "metadata" field.
func (wtu *WalletTransactionUpdate) SetMetadata(m map[string]string) *WalletTransactionUpdate {
	wtu.mutation.SetMetadata(m)
	return wtu
}

// ClearMetadata clears the value of the "metadata" field.
func (wtu *WalletTransactionUpdate) ClearMetadata() *WalletTransactionUpdate {
	wtu.mutation.ClearMetadata()
	return wtu
}

// SetTransactionStatus sets the "transaction_status" field.
func (wtu *WalletTransactionUpdate) SetTransactionStatus(s string) *WalletTransactionUpdate {
	wtu.mutation.SetTransactionStatus(s)
	return wtu
}

// SetNillableTransactionStatus sets the "transaction_status" field if the given value is not nil.
func (wtu *WalletTransactionUpdate) SetNillableTransactionStatus(s *string) *WalletTransactionUpdate {
	if s != nil {
		wtu.SetTransactionStatus(*s)
	}
	return wtu
}

// SetCreditsAvailable sets the "credits_available" field.
func (wtu *WalletTransactionUpdate) SetCreditsAvailable(d decimal.Decimal) *WalletTransactionUpdate {
	wtu.mutation.SetCreditsAvailable(d)
	return wtu
}

// SetNillableCreditsAvailable sets the "credits_available" field if the given value is not nil.
func (wtu *WalletTransactionUpdate) SetNillableCreditsAvailable(d *decimal.Decimal) *WalletTransactionUpdate {
	if d != nil {
		wtu.SetCreditsAvailable(*d)
	}
	return wtu
}

// SetPriority sets the "priority" field.
func (wtu *WalletTransactionUpdate) SetPriority(i int) *WalletTransactionUpdate {
	wtu.mutation.ResetPriority()
	wtu.mutation.SetPriority(i)
	return wtu
}

// SetNillablePriority sets the "priority" field if the given value is not nil.
func (wtu *WalletTransactionUpdate) SetNillablePriority(i *int) *WalletTransactionUpdate {
	if i != nil {
		wtu.SetPriority(*i)
	}
	return wtu
}

// AddPriority adds i to the "priority" field.
func (wtu *WalletTransactionUpdate) AddPriority(i int) *WalletTransactionUpdate {
	wtu.mutation.AddPriority(i)
	return wtu
}

// ClearPriority clears the value of the "priority" field.
func (wtu *WalletTransactionUpdate) ClearPriority() *WalletTransactionUpdate {
	wtu.mutation.ClearPriority()
	return wtu
}

// Mutation returns the WalletTransactionMutation object of the builder.
func (wtu *WalletTransactionUpdate) Mutation() *WalletTransactionMutation {
	return wtu.mutation
}

// Save executes the query and returns the number of nodes affected by the update operation.
func (wtu *WalletTransactionUpdate) Save(ctx context.Context) (int, error) {
	wtu.defaults()
	return withHooks(ctx, wtu.sqlSave, wtu.mutation, wtu.hooks)
}

// SaveX is like Save, but panics if an error occurs.
func (wtu *WalletTransactionUpdate) SaveX(ctx context.Context) int {
	affected, err := wtu.Save(ctx)
	if err != nil {
		panic(err)
	}
	return affected
}

// Exec executes the query.
func (wtu *WalletTransactionUpdate) Exec(ctx context.Context) error {
	_, err := wtu.Save(ctx)
	return err
}

// ExecX is like Exec, but panics if an error occurs.
func (wtu *WalletTransactionUpdate) ExecX(ctx context.Context) {
	if err := wtu.Exec(ctx); err != nil {
		panic(err)
	}
}

// defaults sets the default values of the builder before save.
func (wtu *WalletTransactionUpdate) defaults() {
	if _, ok := wtu.mutation.UpdatedAt(); !ok {
		v := wallettransaction.UpdateDefaultUpdatedAt()
		wtu.mutation.SetUpdatedAt(v)
	}
}

// check runs all checks and user-defined validators on the builder.
func (wtu *WalletTransactionUpdate) check() error {
	if v, ok := wtu.mutation.GetType(); ok {
		if err := wallettransaction.TypeValidator(v); err != nil {
			return &ValidationError{Name: "type", err: fmt.Errorf(`ent: validator failed for field "WalletTransaction.type": %w`, err)}
		}
	}
	return nil
}

func (wtu *WalletTransactionUpdate) sqlSave(ctx context.Context) (n int, err error) {
	if err := wtu.check(); err != nil {
		return n, err
	}
	_spec := sqlgraph.NewUpdateSpec(wallettransaction.Table, wallettransaction.Columns, sqlgraph.NewFieldSpec(wallettransaction.FieldID, field.TypeString))
	if ps := wtu.mutation.predicates; len(ps) > 0 {
		_spec.Predicate = func(selector *sql.Selector) {
			for i := range ps {
				ps[i](selector)
			}
		}
	}
	if value, ok := wtu.mutation.Status(); ok {
		_spec.SetField(wallettransaction.FieldStatus, field.TypeString, value)
	}
	if value, ok := wtu.mutation.UpdatedAt(); ok {
		_spec.SetField(wallettransaction.FieldUpdatedAt, field.TypeTime, value)
	}
	if wtu.mutation.CreatedByCleared() {
		_spec.ClearField(wallettransaction.FieldCreatedBy, field.TypeString)
	}
	if value, ok := wtu.mutation.UpdatedBy(); ok {
		_spec.SetField(wallettransaction.FieldUpdatedBy, field.TypeString, value)
	}
	if wtu.mutation.UpdatedByCleared() {
		_spec.ClearField(wallettransaction.FieldUpdatedBy, field.TypeString)
	}
	if wtu.mutation.EnvironmentIDCleared() {
		_spec.ClearField(wallettransaction.FieldEnvironmentID, field.TypeString)
	}
	if value, ok := wtu.mutation.GetType(); ok {
		_spec.SetField(wallettransaction.FieldType, field.TypeString, value)
	}
	if value, ok := wtu.mutation.Amount(); ok {
		_spec.SetField(wallettransaction.FieldAmount, field.TypeOther, value)
	}
	if value, ok := wtu.mutation.CreditAmount(); ok {
		_spec.SetField(wallettransaction.FieldCreditAmount, field.TypeOther, value)
	}
	if value, ok := wtu.mutation.CreditBalanceBefore(); ok {
		_spec.SetField(wallettransaction.FieldCreditBalanceBefore, field.TypeOther, value)
	}
	if value, ok := wtu.mutation.CreditBalanceAfter(); ok {
		_spec.SetField(wallettransaction.FieldCreditBalanceAfter, field.TypeOther, value)
	}
	if value, ok := wtu.mutation.ReferenceType(); ok {
		_spec.SetField(wallettransaction.FieldReferenceType, field.TypeString, value)
	}
	if wtu.mutation.ReferenceTypeCleared() {
		_spec.ClearField(wallettransaction.FieldReferenceType, field.TypeString)
	}
	if value, ok := wtu.mutation.ReferenceID(); ok {
		_spec.SetField(wallettransaction.FieldReferenceID, field.TypeString, value)
	}
	if wtu.mutation.ReferenceIDCleared() {
		_spec.ClearField(wallettransaction.FieldReferenceID, field.TypeString)
	}
	if value, ok := wtu.mutation.Description(); ok {
		_spec.SetField(wallettransaction.FieldDescription, field.TypeString, value)
	}
	if wtu.mutation.DescriptionCleared() {
		_spec.ClearField(wallettransaction.FieldDescription, field.TypeString)
	}
	if value, ok := wtu.mutation.Metadata(); ok {
		_spec.SetField(wallettransaction.FieldMetadata, field.TypeJSON, value)
	}
	if wtu.mutation.MetadataCleared() {
		_spec.ClearField(wallettransaction.FieldMetadata, field.TypeJSON)
	}
	if value, ok := wtu.mutation.TransactionStatus(); ok {
		_spec.SetField(wallettransaction.FieldTransactionStatus, field.TypeString, value)
	}
	if wtu.mutation.ExpiryDateCleared() {
		_spec.ClearField(wallettransaction.FieldExpiryDate, field.TypeTime)
	}
	if value, ok := wtu.mutation.CreditsAvailable(); ok {
		_spec.SetField(wallettransaction.FieldCreditsAvailable, field.TypeOther, value)
	}
	if wtu.mutation.IdempotencyKeyCleared() {
		_spec.ClearField(wallettransaction.FieldIdempotencyKey, field.TypeString)
	}
	if value, ok := wtu.mutation.Priority(); ok {
		_spec.SetField(wallettransaction.FieldPriority, field.TypeInt, value)
	}
	if value, ok := wtu.mutation.AddedPriority(); ok {
		_spec.AddField(wallettransaction.FieldPriority, field.TypeInt, value)
	}
	if wtu.mutation.PriorityCleared() {
		_spec.ClearField(wallettransaction.FieldPriority, field.TypeInt)
	}
	if n, err = sqlgraph.UpdateNodes(ctx, wtu.driver, _spec); err != nil {
		if _, ok := err.(*sqlgraph.NotFoundError); ok {
			err = &NotFoundError{wallettransaction.Label}
		} else if sqlgraph.IsConstraintError(err) {
			err = &ConstraintError{msg: err.Error(), wrap: err}
		}
		return 0, err
	}
	wtu.mutation.done = true
	return n, nil
}

// WalletTransactionUpdateOne is the builder for updating a single WalletTransaction entity.
type WalletTransactionUpdateOne struct {
	config
	fields   []string
	hooks    []Hook
	mutation *WalletTransactionMutation
}

// SetStatus sets the "status" field.
func (wtuo *WalletTransactionUpdateOne) SetStatus(s string) *WalletTransactionUpdateOne {
	wtuo.mutation.SetStatus(s)
	return wtuo
}

// SetNillableStatus sets the "status" field if the given value is not nil.
func (wtuo *WalletTransactionUpdateOne) SetNillableStatus(s *string) *WalletTransactionUpdateOne {
	if s != nil {
		wtuo.SetStatus(*s)
	}
	return wtuo
}

// SetUpdatedAt sets the "updated_at" field.
func (wtuo *WalletTransactionUpdateOne) SetUpdatedAt(t time.Time) *WalletTransactionUpdateOne {
	wtuo.mutation.SetUpdatedAt(t)
	return wtuo
}

// SetUpdatedBy sets the "updated_by" field.
func (wtuo *WalletTransactionUpdateOne) SetUpdatedBy(s string) *WalletTransactionUpdateOne {
	wtuo.mutation.SetUpdatedBy(s)
	return wtuo
}

// SetNillableUpdatedBy sets the "updated_by" field if the given value is not nil.
func (wtuo *WalletTransactionUpdateOne) SetNillableUpdatedBy(s *string) *WalletTransactionUpdateOne {
	if s != nil {
		wtuo.SetUpdatedBy(*s)
	}
	return wtuo
}

// ClearUpdatedBy clears the value of the "updated_by" field.
func (wtuo *WalletTransactionUpdateOne) ClearUpdatedBy() *WalletTransactionUpdateOne {
	wtuo.mutation.ClearUpdatedBy()
	return wtuo
}

// SetType sets the "type" field.
func (wtuo *WalletTransactionUpdateOne) SetType(s string) *WalletTransactionUpdateOne {
	wtuo.mutation.SetType(s)
	return wtuo
}

// SetNillableType sets the "type" field if the given value is not nil.
func (wtuo *WalletTransactionUpdateOne) SetNillableType(s *string) *WalletTransactionUpdateOne {
	if s != nil {
		wtuo.SetType(*s)
	}
	return wtuo
}

// SetAmount sets the "amount" field.
func (wtuo *WalletTransactionUpdateOne) SetAmount(d decimal.Decimal) *WalletTransactionUpdateOne {
	wtuo.mutation.SetAmount(d)
	return wtuo
}

// SetNillableAmount sets the "amount" field if the given value is not nil.
func (wtuo *WalletTransactionUpdateOne) SetNillableAmount(d *decimal.Decimal) *WalletTransactionUpdateOne {
	if d != nil {
		wtuo.SetAmount(*d)
	}
	return wtuo
}

// SetCreditAmount sets the "credit_amount" field.
func (wtuo *WalletTransactionUpdateOne) SetCreditAmount(d decimal.Decimal) *WalletTransactionUpdateOne {
	wtuo.mutation.SetCreditAmount(d)
	return wtuo
}

// SetNillableCreditAmount sets the "credit_amount" field if the given value is not nil.
func (wtuo *WalletTransactionUpdateOne) SetNillableCreditAmount(d *decimal.Decimal) *WalletTransactionUpdateOne {
	if d != nil {
		wtuo.SetCreditAmount(*d)
	}
	return wtuo
}

// SetCreditBalanceBefore sets the "credit_balance_before" field.
func (wtuo *WalletTransactionUpdateOne) SetCreditBalanceBefore(d decimal.Decimal) *WalletTransactionUpdateOne {
	wtuo.mutation.SetCreditBalanceBefore(d)
	return wtuo
}

// SetNillableCreditBalanceBefore sets the "credit_balance_before" field if the given value is not nil.
func (wtuo *WalletTransactionUpdateOne) SetNillableCreditBalanceBefore(d *decimal.Decimal) *WalletTransactionUpdateOne {
	if d != nil {
		wtuo.SetCreditBalanceBefore(*d)
	}
	return wtuo
}

// SetCreditBalanceAfter sets the "credit_balance_after" field.
func (wtuo *WalletTransactionUpdateOne) SetCreditBalanceAfter(d decimal.Decimal) *WalletTransactionUpdateOne {
	wtuo.mutation.SetCreditBalanceAfter(d)
	return wtuo
}

// SetNillableCreditBalanceAfter sets the "credit_balance_after" field if the given value is not nil.
func (wtuo *WalletTransactionUpdateOne) SetNillableCreditBalanceAfter(d *decimal.Decimal) *WalletTransactionUpdateOne {
	if d != nil {
		wtuo.SetCreditBalanceAfter(*d)
	}
	return wtuo
}

// SetReferenceType sets the "reference_type" field.
func (wtuo *WalletTransactionUpdateOne) SetReferenceType(s string) *WalletTransactionUpdateOne {
	wtuo.mutation.SetReferenceType(s)
	return wtuo
}

// SetNillableReferenceType sets the "reference_type" field if the given value is not nil.
func (wtuo *WalletTransactionUpdateOne) SetNillableReferenceType(s *string) *WalletTransactionUpdateOne {
	if s != nil {
		wtuo.SetReferenceType(*s)
	}
	return wtuo
}

// ClearReferenceType clears the value of the "reference_type" field.
func (wtuo *WalletTransactionUpdateOne) ClearReferenceType() *WalletTransactionUpdateOne {
	wtuo.mutation.ClearReferenceType()
	return wtuo
}

// SetReferenceID sets the "reference_id" field.
func (wtuo *WalletTransactionUpdateOne) SetReferenceID(s string) *WalletTransactionUpdateOne {
	wtuo.mutation.SetReferenceID(s)
	return wtuo
}

// SetNillableReferenceID sets the "reference_id" field if the given value is not nil.
func (wtuo *WalletTransactionUpdateOne) SetNillableReferenceID(s *string) *WalletTransactionUpdateOne {
	if s != nil {
		wtuo.SetReferenceID(*s)
	}
	return wtuo
}

// ClearReferenceID clears the value of the "reference_id" field.
func (wtuo *WalletTransactionUpdateOne) ClearReferenceID() *WalletTransactionUpdateOne {
	wtuo.mutation.ClearReferenceID()
	return wtuo
}

// SetDescription sets the "description" field.
func (wtuo *WalletTransactionUpdateOne) SetDescription(s string) *WalletTransactionUpdateOne {
	wtuo.mutation.SetDescription(s)
	return wtuo
}

// SetNillableDescription sets the "description" field if the given value is not nil.
func (wtuo *WalletTransactionUpdateOne) SetNillableDescription(s *string) *WalletTransactionUpdateOne {
	if s != nil {
		wtuo.SetDescription(*s)
	}
	return wtuo
}

// ClearDescription clears the value of the "description" field.
func (wtuo *WalletTransactionUpdateOne) ClearDescription() *WalletTransactionUpdateOne {
	wtuo.mutation.ClearDescription()
	return wtuo
}

// SetMetadata sets the "metadata" field.
func (wtuo *WalletTransactionUpdateOne) SetMetadata(m map[string]string) *WalletTransactionUpdateOne {
	wtuo.mutation.SetMetadata(m)
	return wtuo
}

// ClearMetadata clears the value of the "metadata" field.
func (wtuo *WalletTransactionUpdateOne) ClearMetadata() *WalletTransactionUpdateOne {
	wtuo.mutation.ClearMetadata()
	return wtuo
}

// SetTransactionStatus sets the "transaction_status" field.
func (wtuo *WalletTransactionUpdateOne) SetTransactionStatus(s string) *WalletTransactionUpdateOne {
	wtuo.mutation.SetTransactionStatus(s)
	return wtuo
}

// SetNillableTransactionStatus sets the "transaction_status" field if the given value is not nil.
func (wtuo *WalletTransactionUpdateOne) SetNillableTransactionStatus(s *string) *WalletTransactionUpdateOne {
	if s != nil {
		wtuo.SetTransactionStatus(*s)
	}
	return wtuo
}

// SetCreditsAvailable sets the "credits_available" field.
func (wtuo *WalletTransactionUpdateOne) SetCreditsAvailable(d decimal.Decimal) *WalletTransactionUpdateOne {
	wtuo.mutation.SetCreditsAvailable(d)
	return wtuo
}

// SetNillableCreditsAvailable sets the "credits_available" field if the given value is not nil.
func (wtuo *WalletTransactionUpdateOne) SetNillableCreditsAvailable(d *decimal.Decimal) *WalletTransactionUpdateOne {
	if d != nil {
		wtuo.SetCreditsAvailable(*d)
	}
	return wtuo
}

// SetPriority sets the "priority" field.
func (wtuo *WalletTransactionUpdateOne) SetPriority(i int) *WalletTransactionUpdateOne {
	wtuo.mutation.ResetPriority()
	wtuo.mutation.SetPriority(i)
	return wtuo
}

// SetNillablePriority sets the "priority" field if the given value is not nil.
func (wtuo *WalletTransactionUpdateOne) SetNillablePriority(i *int) *WalletTransactionUpdateOne {
	if i != nil {
		wtuo.SetPriority(*i)
	}
	return wtuo
}

// AddPriority adds i to the "priority" field.
func (wtuo *WalletTransactionUpdateOne) AddPriority(i int) *WalletTransactionUpdateOne {
	wtuo.mutation.AddPriority(i)
	return wtuo
}

// ClearPriority clears the value of the "priority" field.
func (wtuo *WalletTransactionUpdateOne) ClearPriority() *WalletTransactionUpdateOne {
	wtuo.mutation.ClearPriority()
	return wtuo
}

// Mutation returns the WalletTransactionMutation object of the builder.
func (wtuo *WalletTransactionUpdateOne) Mutation() *WalletTransactionMutation {
	return wtuo.mutation
}

// Where appends a list predicates to the WalletTransactionUpdate builder.
func (wtuo *WalletTransactionUpdateOne) Where(ps ...predicate.WalletTransaction) *WalletTransactionUpdateOne {
	wtuo.mutation.Where(ps...)
	return wtuo
}

// Select allows selecting one or more fields (columns) of the returned entity.
// The default is selecting all fields defined in the entity schema.
func (wtuo *WalletTransactionUpdateOne) Select(field string, fields ...string) *WalletTransactionUpdateOne {
	wtuo.fields = append([]string{field}, fields...)
	return wtuo
}

// Save executes the query and returns the updated WalletTransaction entity.
func (wtuo *WalletTransactionUpdateOne) Save(ctx context.Context) (*WalletTransaction, error) {
	wtuo.defaults()
	return withHooks(ctx, wtuo.sqlSave, wtuo.mutation, wtuo.hooks)
}

// SaveX is like Save, but panics if an error occurs.
func (wtuo *WalletTransactionUpdateOne) SaveX(ctx context.Context) *WalletTransaction {
	node, err := wtuo.Save(ctx)
	if err != nil {
		panic(err)
	}
	return node
}

// Exec executes the query on the entity.
func (wtuo *WalletTransactionUpdateOne) Exec(ctx context.Context) error {
	_, err := wtuo.Save(ctx)
	return err
}

// ExecX is like Exec, but panics if an error occurs.
func (wtuo *WalletTransactionUpdateOne) ExecX(ctx context.Context) {
	if err := wtuo.Exec(ctx); err != nil {
		panic(err)
	}
}

// defaults sets the default values of the builder before save.
func (wtuo *WalletTransactionUpdateOne) defaults() {
	if _, ok := wtuo.mutation.UpdatedAt(); !ok {
		v := wallettransaction.UpdateDefaultUpdatedAt()
		wtuo.mutation.SetUpdatedAt(v)
	}
}

// check runs all checks and user-defined validators on the builder.
func (wtuo *WalletTransactionUpdateOne) check() error {
	if v, ok := wtuo.mutation.GetType(); ok {
		if err := wallettransaction.TypeValidator(v); err != nil {
			return &ValidationError{Name: "type", err: fmt.Errorf(`ent: validator failed for field "WalletTransaction.type": %w`, err)}
		}
	}
	return nil
}

func (wtuo *WalletTransactionUpdateOne) sqlSave(ctx context.Context) (_node *WalletTransaction, err error) {
	if err := wtuo.check(); err != nil {
		return _node, err
	}
	_spec := sqlgraph.NewUpdateSpec(wallettransaction.Table, wallettransaction.Columns, sqlgraph.NewFieldSpec(wallettransaction.FieldID, field.TypeString))
	id, ok := wtuo.mutation.ID()
	if !ok {
		return nil, &ValidationError{Name: "id", err: errors.New(`ent: missing "WalletTransaction.id" for update`)}
	}
	_spec.Node.ID.Value = id
	if fields := wtuo.fields; len(fields) > 0 {
		_spec.Node.Columns = make([]string, 0, len(fields))
		_spec.Node.Columns = append(_spec.Node.Columns, wallettransaction.FieldID)
		for _, f := range fields {
			if !wallettransaction.ValidColumn(f) {
				return nil, &ValidationError{Name: f, err: fmt.Errorf("ent: invalid field %q for query", f)}
			}
			if f != wallettransaction.FieldID {
				_spec.Node.Columns = append(_spec.Node.Columns, f)
			}
		}
	}
	if ps := wtuo.mutation.predicates; len(ps) > 0 {
		_spec.Predicate = func(selector *sql.Selector) {
			for i := range ps {
				ps[i](selector)
			}
		}
	}
	if value, ok := wtuo.mutation.Status(); ok {
		_spec.SetField(wallettransaction.FieldStatus, field.TypeString, value)
	}
	if value, ok := wtuo.mutation.UpdatedAt(); ok {
		_spec.SetField(wallettransaction.FieldUpdatedAt, field.TypeTime, value)
	}
	if wtuo.mutation.CreatedByCleared() {
		_spec.ClearField(wallettransaction.FieldCreatedBy, field.TypeString)
	}
	if value, ok := wtuo.mutation.UpdatedBy(); ok {
		_spec.SetField(wallettransaction.FieldUpdatedBy, field.TypeString, value)
	}
	if wtuo.mutation.UpdatedByCleared() {
		_spec.ClearField(wallettransaction.FieldUpdatedBy, field.TypeString)
	}
	if wtuo.mutation.EnvironmentIDCleared() {
		_spec.ClearField(wallettransaction.FieldEnvironmentID, field.TypeString)
	}
	if value, ok := wtuo.mutation.GetType(); ok {
		_spec.SetField(wallettransaction.FieldType, field.TypeString, value)
	}
	if value, ok := wtuo.mutation.Amount(); ok {
		_spec.SetField(wallettransaction.FieldAmount, field.TypeOther, value)
	}
	if value, ok := wtuo.mutation.CreditAmount(); ok {
		_spec.SetField(wallettransaction.FieldCreditAmount, field.TypeOther, value)
	}
	if value, ok := wtuo.mutation.CreditBalanceBefore(); ok {
		_spec.SetField(wallettransaction.FieldCreditBalanceBefore, field.TypeOther, value)
	}
	if value, ok := wtuo.mutation.CreditBalanceAfter(); ok {
		_spec.SetField(wallettransaction.FieldCreditBalanceAfter, field.TypeOther, value)
	}
	if value, ok := wtuo.mutation.ReferenceType(); ok {
		_spec.SetField(wallettransaction.FieldReferenceType, field.TypeString, value)
	}
	if wtuo.mutation.ReferenceTypeCleared() {
		_spec.ClearField(wallettransaction.FieldReferenceType, field.TypeString)
	}
	if value, ok := wtuo.mutation.ReferenceID(); ok {
		_spec.SetField(wallettransaction.FieldReferenceID, field.TypeString, value)
	}
	if wtuo.mutation.ReferenceIDCleared() {
		_spec.ClearField(wallettransaction.FieldReferenceID, field.TypeString)
	}
	if value, ok := wtuo.mutation.Description(); ok {
		_spec.SetField(wallettransaction.FieldDescription, field.TypeString, value)
	}
	if wtuo.mutation.DescriptionCleared() {
		_spec.ClearField(wallettransaction.FieldDescription, field.TypeString)
	}
	if value, ok := wtuo.mutation.Metadata(); ok {
		_spec.SetField(wallettransaction.FieldMetadata, field.TypeJSON, value)
	}
	if wtuo.mutation.MetadataCleared() {
		_spec.ClearField(wallettransaction.FieldMetadata, field.TypeJSON)
	}
	if value, ok := wtuo.mutation.TransactionStatus(); ok {
		_spec.SetField(wallettransaction.FieldTransactionStatus, field.TypeString, value)
	}
	if wtuo.mutation.ExpiryDateCleared() {
		_spec.ClearField(wallettransaction.FieldExpiryDate, field.TypeTime)
	}
	if value, ok := wtuo.mutation.CreditsAvailable(); ok {
		_spec.SetField(wallettransaction.FieldCreditsAvailable, field.TypeOther, value)
	}
	if wtuo.mutation.IdempotencyKeyCleared() {
		_spec.ClearField(wallettransaction.FieldIdempotencyKey, field.TypeString)
	}
	if value, ok := wtuo.mutation.Priority(); ok {
		_spec.SetField(wallettransaction.FieldPriority, field.TypeInt, value)
	}
	if value, ok := wtuo.mutation.AddedPriority(); ok {
		_spec.AddField(wallettransaction.FieldPriority, field.TypeInt, value)
	}
	if wtuo.mutation.PriorityCleared() {
		_spec.ClearField(wallettransaction.FieldPriority, field.TypeInt)
	}
	_node = &WalletTransaction{config: wtuo.config}
	_spec.Assign = _node.assignValues
	_spec.ScanValues = _node.scanValues
	if err = sqlgraph.UpdateNode(ctx, wtuo.driver, _spec); err != nil {
		if _, ok := err.(*sqlgraph.NotFoundError); ok {
			err = &NotFoundError{wallettransaction.Label}
		} else if sqlgraph.IsConstraintError(err) {
			err = &ConstraintError{msg: err.Error(), wrap: err}
		}
		return nil, err
	}
	wtuo.mutation.done = true
	return _node, nil
}
