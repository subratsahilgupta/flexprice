// Code generated by ent, DO NOT EDIT.

package payment

import (
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"github.com/shopspring/decimal"
)

const (
	// Label holds the string label denoting the payment type in the database.
	Label = "payment"
	// FieldID holds the string denoting the id field in the database.
	FieldID = "id"
	// FieldTenantID holds the string denoting the tenant_id field in the database.
	FieldTenantID = "tenant_id"
	// FieldStatus holds the string denoting the status field in the database.
	FieldStatus = "status"
	// FieldCreatedAt holds the string denoting the created_at field in the database.
	FieldCreatedAt = "created_at"
	// FieldUpdatedAt holds the string denoting the updated_at field in the database.
	FieldUpdatedAt = "updated_at"
	// FieldCreatedBy holds the string denoting the created_by field in the database.
	FieldCreatedBy = "created_by"
	// FieldUpdatedBy holds the string denoting the updated_by field in the database.
	FieldUpdatedBy = "updated_by"
	// FieldEnvironmentID holds the string denoting the environment_id field in the database.
	FieldEnvironmentID = "environment_id"
	// FieldIdempotencyKey holds the string denoting the idempotency_key field in the database.
	FieldIdempotencyKey = "idempotency_key"
	// FieldDestinationType holds the string denoting the destination_type field in the database.
	FieldDestinationType = "destination_type"
	// FieldDestinationID holds the string denoting the destination_id field in the database.
	FieldDestinationID = "destination_id"
	// FieldPaymentMethodType holds the string denoting the payment_method_type field in the database.
	FieldPaymentMethodType = "payment_method_type"
	// FieldPaymentMethodID holds the string denoting the payment_method_id field in the database.
	FieldPaymentMethodID = "payment_method_id"
	// FieldPaymentGateway holds the string denoting the payment_gateway field in the database.
	FieldPaymentGateway = "payment_gateway"
	// FieldGatewayPaymentID holds the string denoting the gateway_payment_id field in the database.
	FieldGatewayPaymentID = "gateway_payment_id"
	// FieldAmount holds the string denoting the amount field in the database.
	FieldAmount = "amount"
	// FieldCurrency holds the string denoting the currency field in the database.
	FieldCurrency = "currency"
	// FieldPaymentStatus holds the string denoting the payment_status field in the database.
	FieldPaymentStatus = "payment_status"
	// FieldTrackAttempts holds the string denoting the track_attempts field in the database.
	FieldTrackAttempts = "track_attempts"
	// FieldMetadata holds the string denoting the metadata field in the database.
	FieldMetadata = "metadata"
	// FieldSucceededAt holds the string denoting the succeeded_at field in the database.
	FieldSucceededAt = "succeeded_at"
	// FieldFailedAt holds the string denoting the failed_at field in the database.
	FieldFailedAt = "failed_at"
	// FieldRefundedAt holds the string denoting the refunded_at field in the database.
	FieldRefundedAt = "refunded_at"
	// FieldRecordedAt holds the string denoting the recorded_at field in the database.
	FieldRecordedAt = "recorded_at"
	// FieldErrorMessage holds the string denoting the error_message field in the database.
	FieldErrorMessage = "error_message"
	// EdgeAttempts holds the string denoting the attempts edge name in mutations.
	EdgeAttempts = "attempts"
	// Table holds the table name of the payment in the database.
	Table = "payments"
	// AttemptsTable is the table that holds the attempts relation/edge.
	AttemptsTable = "payment_attempts"
	// AttemptsInverseTable is the table name for the PaymentAttempt entity.
	// It exists in this package in order to avoid circular dependency with the "paymentattempt" package.
	AttemptsInverseTable = "payment_attempts"
	// AttemptsColumn is the table column denoting the attempts relation/edge.
	AttemptsColumn = "payment_id"
)

// Columns holds all SQL columns for payment fields.
var Columns = []string{
	FieldID,
	FieldTenantID,
	FieldStatus,
	FieldCreatedAt,
	FieldUpdatedAt,
	FieldCreatedBy,
	FieldUpdatedBy,
	FieldEnvironmentID,
	FieldIdempotencyKey,
	FieldDestinationType,
	FieldDestinationID,
	FieldPaymentMethodType,
	FieldPaymentMethodID,
	FieldPaymentGateway,
	FieldGatewayPaymentID,
	FieldAmount,
	FieldCurrency,
	FieldPaymentStatus,
	FieldTrackAttempts,
	FieldMetadata,
	FieldSucceededAt,
	FieldFailedAt,
	FieldRefundedAt,
	FieldRecordedAt,
	FieldErrorMessage,
}

// ValidColumn reports if the column name is valid (part of the table columns).
func ValidColumn(column string) bool {
	for i := range Columns {
		if column == Columns[i] {
			return true
		}
	}
	return false
}

var (
	// TenantIDValidator is a validator for the "tenant_id" field. It is called by the builders before save.
	TenantIDValidator func(string) error
	// DefaultStatus holds the default value on creation for the "status" field.
	DefaultStatus string
	// DefaultCreatedAt holds the default value on creation for the "created_at" field.
	DefaultCreatedAt func() time.Time
	// DefaultUpdatedAt holds the default value on creation for the "updated_at" field.
	DefaultUpdatedAt func() time.Time
	// UpdateDefaultUpdatedAt holds the default value on update for the "updated_at" field.
	UpdateDefaultUpdatedAt func() time.Time
	// DefaultEnvironmentID holds the default value on creation for the "environment_id" field.
	DefaultEnvironmentID string
	// DestinationTypeValidator is a validator for the "destination_type" field. It is called by the builders before save.
	DestinationTypeValidator func(string) error
	// DestinationIDValidator is a validator for the "destination_id" field. It is called by the builders before save.
	DestinationIDValidator func(string) error
	// PaymentMethodTypeValidator is a validator for the "payment_method_type" field. It is called by the builders before save.
	PaymentMethodTypeValidator func(string) error
	// DefaultAmount holds the default value on creation for the "amount" field.
	DefaultAmount decimal.Decimal
	// CurrencyValidator is a validator for the "currency" field. It is called by the builders before save.
	CurrencyValidator func(string) error
	// PaymentStatusValidator is a validator for the "payment_status" field. It is called by the builders before save.
	PaymentStatusValidator func(string) error
	// DefaultTrackAttempts holds the default value on creation for the "track_attempts" field.
	DefaultTrackAttempts bool
)

// OrderOption defines the ordering options for the Payment queries.
type OrderOption func(*sql.Selector)

// ByID orders the results by the id field.
func ByID(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldID, opts...).ToFunc()
}

// ByTenantID orders the results by the tenant_id field.
func ByTenantID(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldTenantID, opts...).ToFunc()
}

// ByStatus orders the results by the status field.
func ByStatus(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldStatus, opts...).ToFunc()
}

// ByCreatedAt orders the results by the created_at field.
func ByCreatedAt(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldCreatedAt, opts...).ToFunc()
}

// ByUpdatedAt orders the results by the updated_at field.
func ByUpdatedAt(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldUpdatedAt, opts...).ToFunc()
}

// ByCreatedBy orders the results by the created_by field.
func ByCreatedBy(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldCreatedBy, opts...).ToFunc()
}

// ByUpdatedBy orders the results by the updated_by field.
func ByUpdatedBy(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldUpdatedBy, opts...).ToFunc()
}

// ByEnvironmentID orders the results by the environment_id field.
func ByEnvironmentID(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldEnvironmentID, opts...).ToFunc()
}

// ByIdempotencyKey orders the results by the idempotency_key field.
func ByIdempotencyKey(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldIdempotencyKey, opts...).ToFunc()
}

// ByDestinationType orders the results by the destination_type field.
func ByDestinationType(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldDestinationType, opts...).ToFunc()
}

// ByDestinationID orders the results by the destination_id field.
func ByDestinationID(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldDestinationID, opts...).ToFunc()
}

// ByPaymentMethodType orders the results by the payment_method_type field.
func ByPaymentMethodType(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldPaymentMethodType, opts...).ToFunc()
}

// ByPaymentMethodID orders the results by the payment_method_id field.
func ByPaymentMethodID(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldPaymentMethodID, opts...).ToFunc()
}

// ByPaymentGateway orders the results by the payment_gateway field.
func ByPaymentGateway(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldPaymentGateway, opts...).ToFunc()
}

// ByGatewayPaymentID orders the results by the gateway_payment_id field.
func ByGatewayPaymentID(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldGatewayPaymentID, opts...).ToFunc()
}

// ByAmount orders the results by the amount field.
func ByAmount(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldAmount, opts...).ToFunc()
}

// ByCurrency orders the results by the currency field.
func ByCurrency(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldCurrency, opts...).ToFunc()
}

// ByPaymentStatus orders the results by the payment_status field.
func ByPaymentStatus(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldPaymentStatus, opts...).ToFunc()
}

// ByTrackAttempts orders the results by the track_attempts field.
func ByTrackAttempts(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldTrackAttempts, opts...).ToFunc()
}

// BySucceededAt orders the results by the succeeded_at field.
func BySucceededAt(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldSucceededAt, opts...).ToFunc()
}

// ByFailedAt orders the results by the failed_at field.
func ByFailedAt(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldFailedAt, opts...).ToFunc()
}

// ByRefundedAt orders the results by the refunded_at field.
func ByRefundedAt(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldRefundedAt, opts...).ToFunc()
}

// ByRecordedAt orders the results by the recorded_at field.
func ByRecordedAt(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldRecordedAt, opts...).ToFunc()
}

// ByErrorMessage orders the results by the error_message field.
func ByErrorMessage(opts ...sql.OrderTermOption) OrderOption {
	return sql.OrderByField(FieldErrorMessage, opts...).ToFunc()
}

// ByAttemptsCount orders the results by attempts count.
func ByAttemptsCount(opts ...sql.OrderTermOption) OrderOption {
	return func(s *sql.Selector) {
		sqlgraph.OrderByNeighborsCount(s, newAttemptsStep(), opts...)
	}
}

// ByAttempts orders the results by attempts terms.
func ByAttempts(term sql.OrderTerm, terms ...sql.OrderTerm) OrderOption {
	return func(s *sql.Selector) {
		sqlgraph.OrderByNeighborTerms(s, newAttemptsStep(), append([]sql.OrderTerm{term}, terms...)...)
	}
}
func newAttemptsStep() *sqlgraph.Step {
	return sqlgraph.NewStep(
		sqlgraph.From(Table, FieldID),
		sqlgraph.To(AttemptsInverseTable, FieldID),
		sqlgraph.Edge(sqlgraph.O2M, false, AttemptsTable, AttemptsColumn),
	)
}
