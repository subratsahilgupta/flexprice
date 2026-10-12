package testutil

import (
	"context"

	domainAnalytics "github.com/flexprice/flexprice/internal/domain/analytics"
)

var _ domainAnalytics.ReportsRepository = (*InMemoryReportsRepo)(nil)

// InMemoryReportsRepo returns canned report rows.
type InMemoryReportsRepo struct {
	On   bool
	DB   string
	Max  int
	Rows [][]string
	Err  error
	Last *domainAnalytics.ReportQuery
}

func (r *InMemoryReportsRepo) Enabled() bool { return r.On }

func (r *InMemoryReportsRepo) Database() string { return r.DB }

func (r *InMemoryReportsRepo) MaxRows() int { return r.Max }

func (r *InMemoryReportsRepo) Run(_ context.Context, q *domainAnalytics.ReportQuery) ([][]string, error) {
	r.Last = q
	return r.Rows, r.Err
}
