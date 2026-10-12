package analytics

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

// ReportColumn describes one output column.
type ReportColumn struct {
	Name string
	Type types.ColumnType
	Role types.ColumnRole
}

// ReportQuery is compiled SQL plus parameters.
type ReportQuery struct {
	SQL     string
	Params  map[string]string
	Columns []ReportColumn
}

const (
	chEpoch      = "toDateTime64(0, 6)"
	chTimeLayout = "2006-01-02 15:04:05.000000"
)

var chIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var billingGrainFn = map[types.Grain]string{
	"":               "toStartOfDay",
	types.GrainDay:   "toStartOfDay",
	types.GrainWeek:  "toMonday",
	types.GrainMonth: "toStartOfMonth",
}

func BuildBillingQuery(rv *ResolvedView, db, tenantID, environmentID string, maxRows int) (*ReportQuery, error) {
	if tenantID == "" || environmentID == "" {
		return nil, ierr.NewError("tenant and environment are required").Mark(ierr.ErrPermissionDenied)
	}
	if !chIdentifier.MatchString(db) {
		return nil, ierr.NewErrorf("invalid reports database %q", db).Mark(ierr.ErrSystem)
	}
	if rv == nil || len(rv.Metrics) != 1 {
		return nil, ierr.NewError("billing views take exactly one metric").Mark(ierr.ErrValidation)
	}
	bm, ok := LookupBillingMetric(rv.Metrics[0])
	if !ok {
		return nil, ierr.NewErrorf("unknown billing metric %q", rv.Metrics[0]).Mark(ierr.ErrValidation)
	}
	grainFn, ok := billingGrainFn[rv.Time.Grain]
	if !ok {
		return nil, ierr.NewErrorf("grain %q not supported for billing", rv.Time.Grain).Mark(ierr.ErrValidation)
	}

	dims := slices.Clone(rv.Dimensions)
	if bm.Currency && !slices.Contains(dims, "currency") {
		dims = append(dims, "currency")
	}

	params := map[string]string{
		"tenant_id":      tenantID,
		"environment_id": environmentID,
		"from":           rv.Time.From.UTC().Format(chTimeLayout),
		"to":             rv.Time.To.UTC().Format(chTimeLayout),
	}
	timeExpr := fmt.Sprintf("nullIf(%s, %s)", bm.TimeCol, chEpoch)

	var selects, groups []string
	var cols []ReportColumn
	if rv.Shape == types.ShapeTimeseries {
		selects = append(selects, fmt.Sprintf("ifNull(formatDateTime(toDateTime(%s(%s, 'UTC'), 'UTC'), '%%Y-%%m-%%dT%%H:%%i:%%SZ'), '') AS bucket", grainFn, timeExpr))
		groups = append(groups, "bucket")
		cols = append(cols, ReportColumn{Name: "bucket", Type: types.ColumnTypeDatetime, Role: types.ColumnRoleDimension})
	}
	for _, d := range dims {
		col, ok := bm.Dimensions[d]
		if !ok {
			return nil, ierr.NewErrorf("illegal dimension %q for metric %q", d, bm.Metric).Mark(ierr.ErrValidation)
		}
		selects = append(selects, fmt.Sprintf("ifNull(toString(%s), '') AS d_%s", col, d))
		groups = append(groups, "d_"+d)
		cols = append(cols, ReportColumn{Name: d, Type: types.ColumnTypeString, Role: types.ColumnRoleDimension})
	}
	selects = append(selects, fmt.Sprintf("ifNull(toString(%s), '') AS m_value", bm.Measure))
	cols = append(cols, ReportColumn{Name: string(bm.Metric), Type: types.ColumnTypeDecimal, Role: types.ColumnRoleMetric})

	where := []string{
		"tenant_id = {tenant_id:String}",
		"environment_id = {environment_id:String}",
		"_peerdb_is_deleted = 0",
		"status = 'published'",
	}
	where = append(where, bm.Where...)
	where = append(where,
		timeExpr+" >= toDateTime64({from:String}, 6, 'UTC')",
		timeExpr+" < toDateTime64({to:String}, 6, 'UTC')",
	)
	for i, f := range rv.Filters {
		if f == nil {
			continue
		}
		col, ok := bm.Dimensions[f.Field]
		if !ok {
			return nil, ierr.NewErrorf("illegal filter field %q for metric %q", f.Field, bm.Metric).Mark(ierr.ErrValidation)
		}
		if len(f.Value) == 0 {
			where = append(where, "1 = 0")
			continue
		}
		placeholders := make([]string, len(f.Value))
		for j, v := range f.Value {
			name := fmt.Sprintf("f%d_%d", i, j)
			params[name] = v
			placeholders[j] = "{" + name + ":String}"
		}
		where = append(where, fmt.Sprintf("%s IN (%s)", col, strings.Join(placeholders, ", ")))
	}

	var b strings.Builder
	b.WriteString("SELECT " + strings.Join(selects, ", "))
	b.WriteString(" FROM " + db + "." + bm.Table + " FINAL")
	b.WriteString(" WHERE " + strings.Join(where, " AND "))
	if len(groups) > 0 {
		b.WriteString(" GROUP BY " + strings.Join(groups, ", "))
	}
	switch rv.Shape {
	case types.ShapeTimeseries, types.ShapeScalar:
		b.WriteString(" ORDER BY " + strings.Join(groups, ", "))
	case types.ShapeBreakdown:
		b.WriteString(" ORDER BY " + bm.Measure + " DESC")
	}
	// One extra row detects overflow.
	if maxRows > 0 {
		fmt.Fprintf(&b, " LIMIT %d", maxRows+1)
	}

	return &ReportQuery{SQL: b.String(), Params: params, Columns: cols}, nil
}
