package analytics

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

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
	// TopN: LIMIT is the view limit.
	TopN bool
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

// PreviousWindow: shiftMonths is 0 for rolling windows.
func PreviousWindow(from, to time.Time, compare string) (time.Time, time.Time, int, error) {
	var pf, pt time.Time
	var shift int
	switch compare {
	case "previous_period":
		if from.UTC().Day() == 1 && from.Equal(from.Truncate(24*time.Hour)) {
			n := 1
			for from.AddDate(0, n, 0).Before(to) {
				n++
			}
			pf, pt, shift = from.AddDate(0, -n, 0), shiftBackClamped(to, n), n
		} else {
			pf, pt = from.Add(-to.Sub(from)), from
		}
	case "previous_year":
		pf, pt, shift = from.AddDate(-1, 0, 0), to.AddDate(-1, 0, 0), 12
	default:
		return time.Time{}, time.Time{}, 0, ierr.NewErrorf("unsupported compare %q", compare).
			WithHint("compare must be previous_period or previous_year").
			Mark(ierr.ErrValidation)
	}
	if pt.After(from) {
		pt = from
	}
	return pf, pt, shift, nil
}

// shiftBackClamped overflows to the next month's first day.
func shiftBackClamped(t time.Time, months int) time.Time {
	t = t.UTC()
	first := time.Date(t.Year(), t.Month()-time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	if t.Day() > first.AddDate(0, 1, -1).Day() {
		return first.AddDate(0, 1, 0)
	}
	return time.Date(first.Year(), first.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
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
	if bm.Build != nil {
		return bm.Build(BuildInput{
			RV:     rv,
			DB:     db,
			Params: params,
			Scope: func(alias string) string {
				return fmt.Sprintf("%[1]s.tenant_id = {tenant_id:String} AND %[1]s.environment_id = {environment_id:String} AND %[1]s._peerdb_is_deleted = 0 AND %[1]s.status = '%s'", alias, types.StatusPublished)
			},
			TimeCond: func(col string) string {
				e := fmt.Sprintf("nullIf(%s, %s)", col, chEpoch)
				return fmt.Sprintf("%s >= toDateTime64({from:String}, 6, 'UTC') AND %s < toDateTime64({to:String}, 6, 'UTC')", e, e)
			},
			MaxRows: maxRows,
		})
	}
	if bm.Snapshot && (rv.Compare != "" || rv.Shape == types.ShapeTimeseries) {
		return nil, ierr.NewErrorf("metric %q is a snapshot", bm.Metric).
			WithHint("snapshot metrics support neither compare nor timeseries").
			Mark(ierr.ErrValidation)
	}
	if rv.Compare != "" && bm.Agg == "" {
		return nil, ierr.NewErrorf("metric %q does not support compare", bm.Metric).
			WithHint("remove compare for this metric").
			Mark(ierr.ErrValidation)
	}
	timeExpr := fmt.Sprintf("nullIf(%s, %s)", bm.TimeCol, chEpoch)
	curCond := fmt.Sprintf("%s >= toDateTime64({from:String}, 6, 'UTC') AND %s < toDateTime64({to:String}, 6, 'UTC')", timeExpr, timeExpr)
	bucketExpr := timeExpr
	measure := bm.Measure
	prevMeasure := ""
	timeWhere := []string{
		timeExpr + " >= toDateTime64({from:String}, 6, 'UTC')",
		timeExpr + " < toDateTime64({to:String}, 6, 'UTC')",
	}
	if rv.Compare != "" {
		pf, pt, shiftMonths, err := PreviousWindow(rv.Time.From, rv.Time.To, rv.Compare)
		if err != nil {
			return nil, err
		}
		params["prev_from"] = pf.UTC().Format(chTimeLayout)
		params["prev_to"] = pt.UTC().Format(chTimeLayout)
		prevCond := fmt.Sprintf("%s >= toDateTime64({prev_from:String}, 6, 'UTC') AND %s < toDateTime64({prev_to:String}, 6, 'UTC')", timeExpr, timeExpr)
		shifted := fmt.Sprintf("addYears(%s, 1)", timeExpr)
		switch {
		case rv.Compare == "previous_period" && shiftMonths > 0:
			params["shift_m"] = strconv.Itoa(shiftMonths)
			shifted = fmt.Sprintf("addMonths(%s, {shift_m:Int32})", timeExpr)
		case rv.Compare == "previous_period":
			params["shift_s"] = strconv.FormatInt(int64(rv.Time.To.Sub(rv.Time.From).Seconds()), 10)
			shifted = fmt.Sprintf("addSeconds(%s, {shift_s:Int64})", timeExpr)
		}
		bucketExpr = fmt.Sprintf("if(%s, %s, %s)", curCond, timeExpr, shifted)
		measure = fmt.Sprintf("%sIf(%s, %s)", bm.Agg, bm.Arg, curCond)
		prevMeasure = fmt.Sprintf("%sIf(%s, %s)", bm.Agg, bm.Arg, prevCond)
		timeWhere = []string{fmt.Sprintf("((%s) OR (%s))", curCond, prevCond)}
	}

	var selects, groups []string
	var cols []ReportColumn
	if rv.Shape == types.ShapeTimeseries {
		selects = append(selects, fmt.Sprintf("ifNull(formatDateTime(toDateTime(%s(%s, 'UTC'), 'UTC'), '%%Y-%%m-%%dT%%H:%%i:%%SZ'), '') AS bucket", grainFn, bucketExpr))
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
	selects = append(selects, fmt.Sprintf("ifNull(toString(%s), '') AS m_value", measure))
	cols = append(cols, ReportColumn{Name: string(bm.Metric), Type: types.ColumnTypeDecimal, Role: types.ColumnRoleMetric})
	if prevMeasure != "" {
		selects = append(selects, fmt.Sprintf("ifNull(toString(%s), '') AS m_prev", prevMeasure))
		cols = append(cols, ReportColumn{Name: string(bm.Metric) + "_prev", Type: types.ColumnTypeDecimal, Role: types.ColumnRoleMetric})
	}

	where := []string{
		"tenant_id = {tenant_id:String}",
		"environment_id = {environment_id:String}",
		"_peerdb_is_deleted = 0",
		"status = '" + string(types.StatusPublished) + "'",
	}
	where = append(where, bm.Where...)
	if !bm.Snapshot {
		where = append(where, timeWhere...)
	}
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
	if rv.Shape == types.ShapeBreakdown {
		b.WriteString(" ORDER BY " + measure + " DESC")
	} else if len(groups) > 0 {
		b.WriteString(" ORDER BY " + strings.Join(groups, ", "))
	}
	topN := billingTopN(rv, bm.Metric)
	switch {
	case topN && maxRows > 0:
		fmt.Fprintf(&b, " LIMIT %d", min(rv.Limit, maxRows))
	case topN:
		fmt.Fprintf(&b, " LIMIT %d", rv.Limit)
	case maxRows > 0:
		// One extra row detects overflow.
		fmt.Fprintf(&b, " LIMIT %d", maxRows+1)
	}

	return &ReportQuery{SQL: b.String(), Params: params, Columns: cols, TopN: topN}, nil
}

// billingTopN: SQL order equals view order.
func billingTopN(rv *ResolvedView, metric types.Metric) bool {
	if rv.Shape != types.ShapeBreakdown || rv.Limit <= 0 {
		return false
	}
	switch len(rv.Sort) {
	case 0:
		return true
	case 1:
		s := rv.Sort[0]
		return s != nil && s.Field == string(metric) && s.Dir == types.SortDirectionDesc
	}
	return false
}
