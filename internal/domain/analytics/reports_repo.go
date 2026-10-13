package analytics

import "context"

// ReportsRepository runs compiled report queries.
type ReportsRepository interface {
	Enabled() bool
	Database() string
	// UsageDatabase falls back to Database.
	UsageDatabase() string
	MaxRows() int
	Run(ctx context.Context, q *ReportQuery) ([][]string, error)
}
