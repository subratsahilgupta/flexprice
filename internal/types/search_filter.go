package types

import (
	"time"

	"github.com/samber/lo"

	ierr "github.com/flexprice/flexprice/internal/errors"
)

// filtering options
type DataType string

const (
	DataTypeString DataType = "STRING"
	DataTypeNumber DataType = "NUMBER"
	DataTypeDate   DataType = "DATE"
	DataTypeArray  DataType = "ARRAY"
)

type FilterOperatorType string

const (
	// equal
	EQUAL FilterOperatorType = "EQUAL"

	// string
	CONTAINS FilterOperatorType = "CONTAINS"
	// TODO: add these operators
	// NOT_CONTAINS FilterOperatorType = "NOT_CONTAINS"
	// STARTS_WITH  FilterOperatorType = "STARTS_WITH"
	// ENDS_WITH    FilterOperatorType = "ENDS_WITH"

	// number
	GREATER_THAN FilterOperatorType = "GREATER_THAN"
	LESS_THAN    FilterOperatorType = "LESS_THAN"

	// array
	IS_ANY_OF     FilterOperatorType = "IS_ANY_OF"
	IS_NOT_ANY_OF FilterOperatorType = "IS_NOT_ANY_OF"

	// date
	BEFORE FilterOperatorType = "BEFORE"
	AFTER  FilterOperatorType = "AFTER"
)

type FilterCondition struct {
	Field        string             `json:"field" form:"field"`
	Operator     FilterOperatorType `json:"operator" form:"operator"`
	DataType     DataType           `json:"data_type" form:"data_type"`
	ValueString  string             `json:"value_string" form:"value_string"`
	ValueNumber  float64            `json:"value_number" form:"value_number"`
	ValueArray   []interface{}      `json:"value_array" form:"value_array"`
	ValueDate    time.Time          `json:"value_date" form:"value_date"`
	ValueBoolean bool               `json:"value_boolean" form:"value_boolean"`
}

func (f *FilterCondition) Validate() error {

	// check for empty fields
	if f.Field == "" {
		return ierr.NewError("field is required").
			WithHint("Field is required").
			Mark(ierr.ErrValidation)
	}

	if f.Operator == "" {
		return ierr.NewError("operator is required").
			WithHint("Operator is required").
			Mark(ierr.ErrValidation)
	}

	if f.DataType == "" {
		return ierr.NewError("data_type is required").
			WithHint("Data type is required").
			Mark(ierr.ErrValidation)
	}

	if f.DataType == DataTypeString {
		if f.ValueString == "" {
			return ierr.NewError("value_string is required").
				WithHint("Value string is required").
				Mark(ierr.ErrValidation)
		}
	}

	if f.DataType == DataTypeNumber {
		if f.ValueNumber == 0 {
			return ierr.NewError("value_number is required").
				WithHint("Value number is required").
				Mark(ierr.ErrValidation)
		}
	}

	if f.DataType == DataTypeArray {
		if f.ValueArray == nil {
			return ierr.NewError("value_array is required").
				WithHint("Value array is required").
				Mark(ierr.ErrValidation)
		}
	}

	if f.DataType == DataTypeDate {
		if f.ValueDate == (time.Time{}) {
			return ierr.NewError("value_date is required").
				WithHint("Value date is required").
				Mark(ierr.ErrValidation)
		}
	}

	return nil
}

// sorting options
type SortDirection string

const (
	SortDirectionAsc  SortDirection = "ASC"
	SortDirectionDesc SortDirection = "DESC"
)

type SortCondition struct {
	Field     string        `json:"field" form:"field"`
	Direction SortDirection `json:"direction" form:"direction"`
	DataType  DataType      `json:"data_type" form:"data_type"`
}

func (s *SortCondition) Validate() error {
	if s.Field == "" {
		return ierr.NewError("field is required").
			WithHint("Field is required").
			Mark(ierr.ErrValidation)
	}

	if s.Direction == "" {
		return ierr.NewError("direction is required").
			WithHint("Direction is required").
			Mark(ierr.ErrValidation)
	}

	if s.DataType == "" {
		return ierr.NewError("data_type is required").
			WithHint("Data type is required").
			Mark(ierr.ErrValidation)
	}

	allowedDataTypes := []DataType{DataTypeDate, DataTypeNumber, DataTypeString}

	if !lo.Contains(allowedDataTypes, s.DataType) {
		return ierr.NewError("data_type is invalid").
			WithHint("Data type is invalid").
			Mark(ierr.ErrValidation)
	}

	return nil
}
