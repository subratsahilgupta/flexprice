package dsl

import (
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/samber/lo"
)

type Predicate = func(*sql.Selector) // Ent aliases these under predicate.<Entity>
type OrderFunc = func(*sql.Selector) // Ent aliases these under ent.<Entity>.Asc/Desc

type FieldInfo struct {
	ColumnName string // real DB/Ent column name
}

type FieldResolver func(logical string) (*FieldInfo, error)

func BuildPredicates(filters []*types.FilterCondition, resolve FieldResolver) ([]Predicate, error) {
	out := make([]Predicate, 0, len(filters))

	for _, f := range filters {
		if f == nil {
			continue
		}
		fi, err := resolve(lo.FromPtr(f.Field))
		if err != nil {
			return nil, err
		}
		pred := predicateFromFilter(f, fi)
		if pred != nil {
			out = append(out, pred)
		}
	}
	return out, nil
}

func BuildOrders(sort []*types.SortCondition, resolve FieldResolver) ([]OrderFunc, error) {
	out := make([]OrderFunc, 0, len(sort))

	for _, s := range sort {
		if s == nil {
			continue
		}
		fi, err := resolve(s.Field)
		if err != nil {
			return nil, err
		}
		var of OrderFunc
		switch s.Direction {
		case types.SortDirectionAsc:
			of = func(sel *sql.Selector) { sel.OrderBy(sql.Asc(fi.ColumnName)) }
		case types.SortDirectionDesc:
			of = func(sel *sql.Selector) { sel.OrderBy(sql.Desc(fi.ColumnName)) }
		}
		if of != nil {
			out = append(out, of)
		}
	}
	return out, nil
}

func predicateFromFilter(f *types.FilterCondition, fi *FieldInfo) Predicate {
	if f.Operator == nil {
		return nil
	}

	switch lo.FromPtr(f.Operator) {
	case types.EQUAL:
		return func(sel *sql.Selector) { sel.Where(sql.EQ(fi.ColumnName, valueAny(f))) }

	// string operators
	case types.CONTAINS:
		if v := valueString(f); v != nil {
			return func(sel *sql.Selector) { sel.Where(sql.Contains(fi.ColumnName, *v)) }
		}
	case types.GREATER_THAN:
		if num := valueNumber(f); num != nil {
			return func(sel *sql.Selector) { sel.Where(sql.GT(fi.ColumnName, *num)) }
		}

	case types.LESS_THAN:
		if num := valueNumber(f); num != nil {
			return func(sel *sql.Selector) { sel.Where(sql.LT(fi.ColumnName, *num)) }
		}

	case types.IN:
		if arr := valueArray(f); len(arr) > 0 {
			return func(sel *sql.Selector) { sel.Where(sql.In(fi.ColumnName, toAny(arr)...)) }
		}

	case types.NOT_IN:
		if arr := valueArray(f); len(arr) > 0 {
			return func(sel *sql.Selector) { sel.Where(sql.NotIn(fi.ColumnName, toAny(arr)...)) }
		}

	case types.BEFORE:
		if t := valueDate(f); t != nil {
			return func(sel *sql.Selector) { sel.Where(sql.LT(fi.ColumnName, *t)) }
		}

	case types.AFTER:
		if t := valueDate(f); t != nil {
			return func(sel *sql.Selector) { sel.Where(sql.GT(fi.ColumnName, *t)) }
		}
	}
	return nil // fell through → invalid combi
}

func valueAny(f *types.FilterCondition) interface{} {
	// fallback when type known from column
	if s := valueString(f); s != nil {
		return *s
	}
	if n := valueNumber(f); n != nil {
		return *n
	}
	if b := valueBool(f); b != nil {
		return *b
	}
	if d := valueDate(f); d != nil {
		return *d
	}
	if a := valueArray(f); len(a) > 0 {
		return toAny(a)
	}
	return nil
}

func valueString(f *types.FilterCondition) *string  { return f.Value.String }
func valueNumber(f *types.FilterCondition) *float64 { return f.Value.Number }
func valueBool(f *types.FilterCondition) *bool      { return f.Value.Boolean }
func valueDate(f *types.FilterCondition) *time.Time { return f.Value.Date }
func valueArray(f *types.FilterCondition) []string  { return f.Value.Array }
func toAny(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
