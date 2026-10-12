package clickhouse

import (
	"context"

	clickhouse_go "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/analytics"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
)

const (
	defaultReportsMaxExecutionSeconds = 30
	defaultReportsMaxResultRows       = 10000
)

var _ analytics.ReportsRepository = (*ReportsRepository)(nil)

type ReportsRepository struct {
	conn   driver.Conn
	cfg    config.ReportsConfig
	logger *logger.Logger
}

func NewReportsRepository(cfg *config.Configuration, log *logger.Logger) (*ReportsRepository, error) {
	r := &ReportsRepository{cfg: cfg.Reports, logger: log}
	if r.cfg.MaxExecutionSeconds <= 0 {
		r.cfg.MaxExecutionSeconds = defaultReportsMaxExecutionSeconds
	}
	if r.cfg.MaxResultRows <= 0 {
		r.cfg.MaxResultRows = defaultReportsMaxResultRows
	}
	if !r.cfg.Enabled() {
		return r, nil
	}
	opts := cfg.ClickHouse.GetClientOptions()
	opts.Auth = clickhouse_go.Auth{
		Database: r.cfg.Database,
		Username: r.cfg.Username,
		Password: r.cfg.Password,
	}
	conn, err := clickhouse_go.Open(opts)
	if err != nil {
		return nil, ierr.WithError(err).WithHint("open reports clickhouse connection").Mark(ierr.ErrSystem)
	}
	r.conn = conn
	return r, nil
}

func (r *ReportsRepository) Enabled() bool { return r.conn != nil }

func (r *ReportsRepository) Database() string { return r.cfg.Database }

func (r *ReportsRepository) MaxRows() int { return r.cfg.MaxResultRows }

func (r *ReportsRepository) Run(ctx context.Context, q *analytics.ReportQuery) ([][]string, error) {
	if r.conn == nil {
		return nil, ierr.NewError("financial reports are not enabled").Mark(ierr.ErrPermissionDenied)
	}
	settings := clickhouse_go.Settings{
		"max_execution_time":   r.cfg.MaxExecutionSeconds,
		"max_result_rows":      r.cfg.MaxResultRows + 1,
		"result_overflow_mode": "throw",
	}
	if r.cfg.MaxRowsToRead > 0 {
		settings["max_rows_to_read"] = r.cfg.MaxRowsToRead
	}
	ctx = clickhouse_go.Context(ctx, clickhouse_go.WithParameters(q.Params), clickhouse_go.WithSettings(settings))

	rows, err := r.conn.Query(ctx, q.SQL)
	if err != nil {
		return nil, ierr.WithError(err).WithHint("run report query").Mark(ierr.ErrDatabase)
	}
	defer rows.Close()

	out := [][]string{}
	for rows.Next() {
		vals := make([]string, len(q.Columns))
		ptrs := make([]any, len(vals))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, ierr.WithError(err).WithHint("scan report row").Mark(ierr.ErrDatabase)
		}
		out = append(out, vals)
	}
	if err := rows.Err(); err != nil {
		return nil, ierr.WithError(err).WithHint("read report rows").Mark(ierr.ErrDatabase)
	}
	return out, nil
}
