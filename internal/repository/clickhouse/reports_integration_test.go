package clickhouse

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/domain/analytics"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportsRepository_RunsBillingQuery(t *testing.T) {
	address := chEnvOrDefault("FLEXPRICE_TEST_CLICKHOUSE_ADDRESS", "localhost:9000")
	username := chEnvOrDefault("FLEXPRICE_TEST_CLICKHOUSE_USERNAME", "flexprice")
	password := chEnvOrDefault("FLEXPRICE_TEST_CLICKHOUSE_PASSWORD", "flexprice123")
	db := fmt.Sprintf("reports_it_%d", time.Now().UnixNano())

	admin := newRealClickHouseTestStore(t)
	ctx := context.Background()
	require.NoError(t, admin.GetConn().Exec(ctx, "CREATE DATABASE "+db))
	t.Cleanup(func() { _ = admin.GetConn().Exec(context.Background(), "DROP DATABASE IF EXISTS "+db) })
	require.NoError(t, admin.GetConn().Exec(ctx, `CREATE TABLE `+db+`.invoices (
		id String, tenant_id String, environment_id String, status String,
		invoice_status String, invoice_type String, customer_id String, currency String,
		total Decimal(20, 9), finalized_at DateTime64(6),
		period_start DateTime64(6) DEFAULT finalized_at, issue_date DateTime64(6) DEFAULT toDateTime64(0, 6),
		_peerdb_is_deleted UInt8, _peerdb_version Int64
	) ENGINE = ReplacingMergeTree(_peerdb_version) ORDER BY id`))
	baseCols := `(id, tenant_id, environment_id, status, invoice_status, invoice_type, customer_id, currency, total, finalized_at, _peerdb_is_deleted, _peerdb_version)`
	ins := `INSERT INTO ` + db + `.invoices ` + baseCols + ` VALUES `
	require.NoError(t, admin.GetConn().Exec(ctx, ins+
		`('i1','t1','e1','published','FINALIZED','SUBSCRIPTION','c1','usd',10,'2026-09-05 00:00:00',0,1),`+
		`('i1','t1','e1','published','FINALIZED','SUBSCRIPTION','c1','usd',10,'2026-09-05 00:00:00',0,2),`+
		`('i2','t1','e1','published','FINALIZED','SUBSCRIPTION','c2','inr',5,'2026-09-06 00:00:00',0,1),`+
		`('i3','t1','e1','published','DRAFT','SUBSCRIPTION','c1','usd',99,'2026-09-06 00:00:00',0,1),`+
		`('i4','t2','e1','published','FINALIZED','SUBSCRIPTION','c9','usd',77,'2026-09-06 00:00:00',0,1),`+
		`('i5','t1','e1','published','FINALIZED','SUBSCRIPTION','c1','usd',33,'1970-01-01 00:00:00',0,1),`+
		`('i6','t1','e1','published','FINALIZED','SUBSCRIPTION','c1','usd',44,'2026-09-07 00:00:00',1,1),`+
		`('i7','t1','e1','published','FINALIZED','SUBSCRIPTION','c3','usd',70,'2026-08-10 00:00:00',0,1),`+
		`('i7','t1','e1','published','VOIDED','SUBSCRIPTION','c3','usd',70,'2026-08-10 00:00:00',0,2),`+
		`('i8','t1','e1','published','FINALIZED','SUBSCRIPTION','c1','usd',2,'2026-08-20 00:00:00',0,1),`+
		`('i9','t1','e1','published','FINALIZED','SUBSCRIPTION','c4','usd',4,'2026-08-15 00:00:00',0,1)`))
	// Finalized later than the period it bills for.
	require.NoError(t, admin.GetConn().Exec(ctx, `INSERT INTO `+db+`.invoices
		(id, tenant_id, environment_id, status, invoice_status, invoice_type, customer_id, currency, total, finalized_at, period_start, issue_date, _peerdb_is_deleted, _peerdb_version) VALUES
		('i10','t1','e1','published','FINALIZED','SUBSCRIPTION','c5','usd',100,'2026-10-02 00:00:00','2026-09-15 00:00:00','2026-09-15 00:00:00',0,1),
		('i11','t1','e1','published','FINALIZED','ONE_OFF','c6','usd',7,'2026-10-03 00:00:00','1970-01-01 00:00:00','2026-08-25 00:00:00',0,1),
		('i12','t1','e1','published','FINALIZED','SUBSCRIPTION','c7','usd',50,'2026-09-10 00:00:00','2026-07-01 00:00:00','2026-07-01 00:00:00',0,1)`))

	cfg := &config.Configuration{
		ClickHouse: config.ClickHouseConfig{Address: address, Username: username, Password: password, Database: "default", MaxMemoryUsage: 1},
		Reports:    config.ReportsConfig{Database: db, Username: username, Password: password},
	}
	repo, err := NewReportsRepository(cfg, nil)
	require.NoError(t, err)
	require.True(t, repo.Enabled())

	var filters []*analytics.Filter
	run := func(shape types.Shape, grain types.Grain, dims ...string) [][]string {
		rv := &analytics.ResolvedView{
			Family:     types.FamilyBilling,
			Shape:      shape,
			Metrics:    []types.Metric{types.MetricInvoiceTotal},
			Dimensions: dims,
			Filters:    filters,
			Time: analytics.TimeSpec{
				From:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
				To:    time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
				Grain: grain,
			},
		}
		q, err := analytics.BuildBillingQuery(rv, repo.Database(), "t1", "e1", 100)
		require.NoError(t, err)
		rows, err := repo.Run(ctx, q)
		require.NoError(t, err)
		return rows
	}

	// Dedup, status, tenant, epoch filters; period_start attribution.
	assert.Equal(t, [][]string{{"inr", "5"}, {"usd", "123"}}, run(types.ShapeScalar, ""))

	// i10 follows period_start, i11 falls back to issue_date.
	assert.Equal(t, [][]string{
		{"2026-08-01T00:00:00Z", "usd", "13"},
		{"2026-09-01T00:00:00Z", "inr", "5"},
		{"2026-09-01T00:00:00Z", "usd", "110"},
	}, run(types.ShapeTimeseries, types.GrainMonth))

	assert.Equal(t, [][]string{
		{"c5", "usd", "100"},
		{"c1", "usd", "12"},
		{"c6", "usd", "7"},
		{"c2", "inr", "5"},
		{"c4", "usd", "4"},
	}, run(types.ShapeBreakdown, "", "customer_id"))

	filters = []*analytics.Filter{{Field: "customer_id", Op: types.IN, Value: []string{"c2"}}}
	assert.Equal(t, [][]string{{"inr", "5"}}, run(types.ShapeScalar, ""))
	filters = nil

	weekly := run(types.ShapeTimeseries, types.GrainWeek)
	require.NotEmpty(t, weekly)
	for _, r := range weekly {
		assert.True(t, strings.HasSuffix(r[0], "T00:00:00Z"), r[0])
		bucket, err := time.Parse(time.RFC3339, r[0])
		require.NoError(t, err)
		assert.Equal(t, time.Monday, bucket.Weekday())
	}

	runCmp := func(shape types.Shape, grain types.Grain, compare string, dims ...string) [][]string {
		rv := &analytics.ResolvedView{
			Family: types.FamilyBilling, Shape: shape, Metrics: []types.Metric{types.MetricInvoiceTotal}, Dimensions: dims,
			Time:    analytics.TimeSpec{From: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Grain: grain},
			Compare: compare,
		}
		q, err := analytics.BuildBillingQuery(rv, repo.Database(), "t1", "e1", 100)
		require.NoError(t, err)
		rows, err := repo.Run(ctx, q)
		require.NoError(t, err)
		return rows
	}

	assert.Equal(t, [][]string{{"inr", "5", "0"}, {"usd", "110", "13"}}, runCmp(types.ShapeScalar, "", "previous_period"))
	assert.Equal(t, [][]string{
		{"2026-09-01T00:00:00Z", "inr", "5", "0"},
		{"2026-09-01T00:00:00Z", "usd", "110", "13"},
	}, runCmp(types.ShapeTimeseries, types.GrainMonth, "previous_period"))
	assert.Contains(t, runCmp(types.ShapeTimeseries, types.GrainDay, "previous_period"), []string{"2026-09-20T00:00:00Z", "usd", "0", "2"})
	byCustomer := runCmp(types.ShapeBreakdown, "", "previous_period", "customer_id")
	assert.Contains(t, byCustomer, []string{"c1", "usd", "10", "2"})
	assert.Contains(t, byCustomer, []string{"c4", "usd", "0", "4"})
}

func TestReportsRepository_GenericReports(t *testing.T) {
	address := chEnvOrDefault("FLEXPRICE_TEST_CLICKHOUSE_ADDRESS", "localhost:9000")
	username := chEnvOrDefault("FLEXPRICE_TEST_CLICKHOUSE_USERNAME", "flexprice")
	password := chEnvOrDefault("FLEXPRICE_TEST_CLICKHOUSE_PASSWORD", "flexprice123")
	db := fmt.Sprintf("reports_gen_%d", time.Now().UnixNano())

	admin := newRealClickHouseTestStore(t)
	ctx := context.Background()
	require.NoError(t, admin.GetConn().Exec(ctx, "CREATE DATABASE "+db))
	t.Cleanup(func() { _ = admin.GetConn().Exec(context.Background(), "DROP DATABASE IF EXISTS "+db) })
	require.NoError(t, admin.GetConn().Exec(ctx, `CREATE TABLE `+db+`.invoices (
		id String, tenant_id String, environment_id String, status String,
		invoice_status String, payment_status String, customer_id String, currency String,
		amount_remaining Decimal(20, 9), amount_paid Decimal(20, 9),
		finalized_at DateTime64(6), due_date DateTime64(6), issue_date DateTime64(6), paid_at DateTime64(6),
		_peerdb_is_deleted UInt8, _peerdb_version Int64
	) ENGINE = ReplacingMergeTree(_peerdb_version) ORDER BY id`))
	require.NoError(t, admin.GetConn().Exec(ctx, `CREATE TABLE `+db+`.wallet_transactions (
		id String, tenant_id String, environment_id String, status String,
		type String, transaction_status String, transaction_reason String,
		customer_id String, currency String, amount Decimal(20, 9), created_at DateTime64(6),
		_peerdb_is_deleted UInt8, _peerdb_version Int64
	) ENGINE = ReplacingMergeTree(_peerdb_version) ORDER BY id`))

	epoch := "1970-01-01 00:00:00"
	future := time.Now().UTC().AddDate(0, 0, 10).Format("2006-01-02 15:04:05")
	d10 := time.Now().UTC().AddDate(0, 0, -10).Format("2006-01-02 15:04:05")
	d100 := time.Now().UTC().AddDate(0, 0, -100).Format("2006-01-02 15:04:05")
	inv := func(id, invStatus, pay, cust string, remaining, paid int, due, issue, paidAt string) string {
		return fmt.Sprintf("('%s','t1','e1','published','%s','%s','%s','usd',%d,%d,'2026-09-01 00:00:00','%s','%s','%s',0,1)",
			id, invStatus, pay, cust, remaining, paid, due, issue, paidAt)
	}
	require.NoError(t, admin.GetConn().Exec(ctx, `INSERT INTO `+db+`.invoices VALUES `+strings.Join([]string{
		inv("a1", "FINALIZED", "PENDING", "c1", 30, 0, epoch, epoch, epoch),
		inv("a2", "FINALIZED", "PENDING", "c1", 70, 0, future, epoch, epoch),
		inv("a3", "FINALIZED", "SUCCEEDED", "c1", 0, 5, d10, epoch, epoch),
		inv("a4", "DRAFT", "PENDING", "c1", 500, 0, d10, epoch, epoch),
		inv("a5", "FINALIZED", "PENDING", "c2", 11, 0, d10, epoch, epoch),
		inv("a6", "FINALIZED", "PENDING", "c2", 13, 0, d100, epoch, epoch),
		inv("p1", "FINALIZED", "SUCCEEDED", "c1", 0, 100, epoch, "2026-09-01 00:00:00", "2026-09-11 00:00:00"),
		inv("p2", "FINALIZED", "SUCCEEDED", "c1", 0, 50, epoch, "2026-09-05 00:00:00", "2026-09-25 00:00:00"),
		inv("p3", "FINALIZED", "OVERPAID", "c2", 0, 30, epoch, epoch, "2026-09-13 00:00:00"),
	}, ",")))

	tx := func(id, typ, st, reason string, amount int) string {
		return fmt.Sprintf("('%s','t1','e1','published','%s','%s','%s','c1','usd',%d,'2026-09-10 00:00:00',0,1)", id, typ, st, reason, amount)
	}
	require.NoError(t, admin.GetConn().Exec(ctx, `INSERT INTO `+db+`.wallet_transactions VALUES `+strings.Join([]string{
		tx("w1", "credit", "completed", "PURCHASED_CREDIT_DIRECT", 50),
		tx("w2", "credit", "completed", "FREE_CREDIT_GRANT", 10),
		tx("w3", "credit", "failed", "PURCHASED_CREDIT_INVOICED", 99),
		tx("w4", "credit", "completed", "CREDIT_NOTE", 7),
		tx("w5", "debit", "completed", "INVOICE_PAYMENT", 12),
		tx("w6", "debit", "completed", "CREDIT_EXPIRED", 5),
	}, ",")))

	cfg := &config.Configuration{
		ClickHouse: config.ClickHouseConfig{Address: address, Username: username, Password: password, Database: "default", MaxMemoryUsage: 1},
		Reports:    config.ReportsConfig{Database: db, Username: username, Password: password},
	}
	repo, err := NewReportsRepository(cfg, nil)
	require.NoError(t, err)

	run := func(metric types.Metric, shape types.Shape, dims ...string) [][]string {
		rv := &analytics.ResolvedView{
			Family: types.FamilyBilling, Shape: shape, Metrics: []types.Metric{metric}, Dimensions: dims,
			Time: analytics.TimeSpec{
				From: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
			},
		}
		q, err := analytics.BuildBillingQuery(rv, repo.Database(), "t1", "e1", 100)
		require.NoError(t, err)
		rows, err := repo.Run(ctx, q)
		require.NoError(t, err)
		return rows
	}

	// Snapshot ignores the window.
	assert.Equal(t, [][]string{{"usd", "124"}}, run(types.MetricTotalReceivables, types.ShapeScalar))

	aging := map[string]string{}
	for _, r := range run(types.MetricReceivablesAging, types.ShapeBreakdown, "aging_bucket") {
		aging[r[0]] = r[2]
	}
	assert.Equal(t, map[string]string{"No due date": "30", "Current": "70", "1-30": "11", "90+": "13"}, aging)

	assert.Equal(t, [][]string{{"usd", "15"}}, run(types.MetricDaysToPay, types.ShapeScalar))

	assert.ElementsMatch(t, [][]string{
		{"FREE_CREDIT_GRANT", "usd", "10"},
		{"PURCHASED_CREDIT_DIRECT", "usd", "50"},
	}, run(types.MetricCreditTopups, types.ShapeBreakdown, "transaction_reason"))

	assert.Equal(t, [][]string{{"usd", "12"}}, run(types.MetricCreditConsumption, types.ShapeScalar))

	assert.Equal(t, [][]string{{"usd", "90"}}, run(types.MetricARPU, types.ShapeScalar))
}

func TestReportsRepository_CustomReports(t *testing.T) {
	address := chEnvOrDefault("FLEXPRICE_TEST_CLICKHOUSE_ADDRESS", "localhost:9000")
	username := chEnvOrDefault("FLEXPRICE_TEST_CLICKHOUSE_USERNAME", "flexprice")
	password := chEnvOrDefault("FLEXPRICE_TEST_CLICKHOUSE_PASSWORD", "flexprice123")
	db := fmt.Sprintf("reports_cus_%d", time.Now().UnixNano())

	admin := newRealClickHouseTestStore(t)
	ctx := context.Background()
	require.NoError(t, admin.GetConn().Exec(ctx, "CREATE DATABASE "+db))
	t.Cleanup(func() { _ = admin.GetConn().Exec(context.Background(), "DROP DATABASE IF EXISTS "+db) })
	require.NoError(t, admin.GetConn().Exec(ctx, `CREATE TABLE `+db+`.invoices (
		id String, tenant_id String, environment_id String, status String,
		invoice_status String, currency String, total Decimal(20, 9), finalized_at DateTime64(6),
		period_start DateTime64(6), issue_date DateTime64(6),
		_peerdb_is_deleted UInt8, _peerdb_version Int64
	) ENGINE = ReplacingMergeTree(_peerdb_version) ORDER BY id`))
	require.NoError(t, admin.GetConn().Exec(ctx, `CREATE TABLE `+db+`.credit_notes (
		id String, tenant_id String, environment_id String, status String,
		credit_note_status String, currency String, total_amount Decimal(20, 8),
		created_at DateTime64(6), finalized_at DateTime64(6),
		_peerdb_is_deleted UInt8, _peerdb_version Int64
	) ENGINE = ReplacingMergeTree(_peerdb_version) ORDER BY id`))
	require.NoError(t, admin.GetConn().Exec(ctx, `CREATE TABLE `+db+`.subscriptions (
		id String, tenant_id String, environment_id String, status String,
		customer_id String, currency String, subscription_status String,
		start_date DateTime64(6), end_date DateTime64(6), cancelled_at DateTime64(6),
		_peerdb_is_deleted UInt8, _peerdb_version Int64
	) ENGINE = ReplacingMergeTree(_peerdb_version) ORDER BY id`))

	require.NoError(t, admin.GetConn().Exec(ctx, `INSERT INTO `+db+`.invoices VALUES
		('i1','t1','e1','published','FINALIZED','usd',100,'2026-09-10 00:00:00','2026-09-10 00:00:00','2026-09-10 00:00:00',0,1),
		('i2','t1','e1','published','DRAFT','usd',500,'2026-09-10 00:00:00','2026-09-10 00:00:00','2026-09-10 00:00:00',0,1),
		('i3','t1','e1','published','FINALIZED','usd',30,'2026-10-05 00:00:00','2026-09-20 00:00:00','2026-09-20 00:00:00',0,1),
		('i4','t1','e1','published','FINALIZED','usd',8,'2026-09-02 00:00:00','1970-01-01 00:00:00','2026-10-08 00:00:00',0,1)`))
	require.NoError(t, admin.GetConn().Exec(ctx, `INSERT INTO `+db+`.credit_notes VALUES
		('n1','t1','e1','published','FINALIZED','usd',20,'2026-09-12 00:00:00','1970-01-01 00:00:00',0,1),
		('n2','t1','e1','published','FINALIZED','usd',15,'2026-10-03 00:00:00','1970-01-01 00:00:00',0,1),
		('n3','t1','e1','published','DRAFT','usd',99,'2026-09-12 00:00:00','1970-01-01 00:00:00',0,1)`))

	epoch := "1970-01-01 00:00:00"
	sub := func(id, cust, st, start, end, cancelled string) string {
		return fmt.Sprintf("('%s','t1','e1','published','%s','usd','%s','%s','%s','%s',0,1)", id, cust, st, start, end, cancelled)
	}
	require.NoError(t, admin.GetConn().Exec(ctx, `INSERT INTO `+db+`.subscriptions VALUES `+strings.Join([]string{
		sub("s1", "c1", "active", "2026-01-01 00:00:00", epoch, epoch),
		sub("s2", "c1", "active", "2026-08-15 00:00:00", epoch, epoch),
		sub("s3", "c2", "trialing", "2026-08-20 00:00:00", epoch, epoch),
		sub("s4", "c3", "cancelled", "2026-01-01 00:00:00", epoch, "2026-09-15 00:00:00"),
		sub("s5", "c4", "active", "2026-10-05 00:00:00", epoch, epoch),
		sub("s6", "c5", "cancelled", "2026-01-01 00:00:00", "2026-09-20 00:00:00", epoch),
		sub("s7", "c6", "cancelled", "2026-01-01 00:00:00", epoch, epoch),
		sub("s8", "c7", "cancelled", "2026-01-01 00:00:00", epoch, "2026-07-15 00:00:00"),
	}, ",")))

	cfg := &config.Configuration{
		ClickHouse: config.ClickHouseConfig{Address: address, Username: username, Password: password, Database: "default", MaxMemoryUsage: 1},
		Reports:    config.ReportsConfig{Database: db, Username: username, Password: password},
	}
	repo, err := NewReportsRepository(cfg, nil)
	require.NoError(t, err)

	run := func(metric types.Metric, shape types.Shape, from, to time.Time) [][]string {
		rv := &analytics.ResolvedView{
			Family: types.FamilyBilling, Shape: shape, Metrics: []types.Metric{metric},
			Time: analytics.TimeSpec{From: from, To: to, Grain: types.GrainMonth},
		}
		q, err := analytics.BuildBillingQuery(rv, repo.Database(), "t1", "e1", 100)
		require.NoError(t, err)
		rows, err := repo.Run(ctx, q)
		require.NoError(t, err)
		return rows
	}
	sep := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	oct := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	assert.Equal(t, [][]string{
		{"2026-09-01T00:00:00Z", "usd", "110"},
		{"2026-10-01T00:00:00Z", "usd", "-7"},
	}, run(types.MetricNetBilling, types.ShapeTimeseries, sep, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)))

	nov := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	// c1, c2 open; c3 cancelled_at, c5 end_date mid-window.
	assert.Equal(t, [][]string{{"usd", "4"}}, run(types.MetricActiveCustomers, types.ShapeScalar, sep, oct))
	// Cancelled and ended subs drop out; c4 starts.
	assert.Equal(t, [][]string{{"usd", "3"}}, run(types.MetricActiveCustomers, types.ShapeScalar, oct, nov))
	// Both bounded intervals ended before window.
	assert.Equal(t, [][]string{{"usd", "2"}}, run(types.MetricActiveCustomers, types.ShapeScalar, late, oct))
}
