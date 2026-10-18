package telemetry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/ent/customer"
	"github.com/flexprice/flexprice/ent/environment"
	"github.com/flexprice/flexprice/ent/invoice"
	"github.com/flexprice/flexprice/ent/plan"
	"github.com/flexprice/flexprice/ent/subscription"
	"github.com/flexprice/flexprice/ent/tenant"
	"github.com/flexprice/flexprice/ent/wallet"
	"github.com/flexprice/flexprice/internal/buildinfo"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/google/uuid"
	"go.uber.org/fx"
)

const (
	heartbeatEvery = time.Hour
	snapshotEvery  = 24 * time.Hour
	topTenants     = 20
)

type Reporter struct {
	cfg       *config.Configuration
	db        postgres.IClient
	log       *logger.Logger
	http      *http.Client
	processID string
	cancel    context.CancelFunc
}

func NewReporter(cfg *config.Configuration, db postgres.IClient, log *logger.Logger) *Reporter {
	return &Reporter{
		cfg:       cfg,
		db:        db,
		log:       log,
		http:      &http.Client{Timeout: 5 * time.Second},
		processID: uuid.NewString(),
	}
}

func (r *Reporter) FireOnce(ctx context.Context) {
	r.heartbeat(ctx)
	r.snapshot(ctx)
}

func RegisterHooks(lc fx.Lifecycle, r *Reporter) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if reason := r.disabledReason(); reason != "" {
				r.log.Infow("telemetry off", "reason", reason)
				return nil
			}
			r.log.Infow("telemetry on, set FLEXPRICE_TELEMETRY_DISABLED=true to opt out")
			runCtx, cancel := context.WithCancel(context.Background())
			r.cancel = cancel
			go r.run(runCtx)
			return nil
		},
		OnStop: func(context.Context) error {
			if r.cancel != nil {
				r.cancel()
			}
			return nil
		},
	})
}

func (r *Reporter) disabledReason() string {
	switch {
	case r.cfg.Telemetry.Disabled:
		return "FLEXPRICE_TELEMETRY_DISABLED"
	case isTruthy(os.Getenv("DO_NOT_TRACK")):
		return "DO_NOT_TRACK"
	case r.cfg.Telemetry.APIKey == "":
		return "no destination configured"
	}
	return ""
}

func isTruthy(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "1" || s == "true"
}

func (r *Reporter) run(ctx context.Context) {
	sendsSnapshot := r.cfg.Deployment.Mode == types.ModeAPI || r.cfg.Deployment.Mode == types.ModeLocal

	r.heartbeat(ctx)
	if sendsSnapshot {
		r.snapshot(ctx)
	}

	hb := time.NewTicker(heartbeatEvery)
	defer hb.Stop()
	snap := time.NewTicker(snapshotEvery)
	defer snap.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-hb.C:
			r.heartbeat(ctx)
		case <-snap.C:
			if sendsSnapshot {
				r.snapshot(ctx)
			}
		}
	}
}

func (r *Reporter) base(instanceID string) map[string]any {
	p := map[string]any{
		"instance_id": instanceID,
		"version":     buildinfo.Version,
		"git_sha":     buildinfo.GitSHA,
		"region":      r.cfg.Logging.Region,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
	}
	if l := r.cfg.Telemetry.InstallLabel; l != "" {
		p["install_label"] = l
	}
	return p
}

func (r *Reporter) heartbeat(ctx context.Context) {
	id, err := r.instanceID(ctx)
	if err != nil {
		r.log.Debugw("telemetry heartbeat skipped", "error", err)
		return
	}
	p := r.base(id)
	p["process_id"] = r.processID
	p["role"] = string(r.cfg.Deployment.Mode)
	p["platform"] = detectPlatform()
	p["cloud"] = detectCloud()
	p["num_cpu"] = runtime.NumCPU()
	r.send(ctx, "flexprice_install_heartbeat", id, p)
}

func (r *Reporter) snapshot(ctx context.Context) {
	id, err := r.instanceID(ctx)
	if err != nil {
		r.log.Debugw("telemetry snapshot skipped", "error", err)
		return
	}
	p := r.base(id)
	if err := r.addCounts(ctx, id, p); err != nil {
		r.log.Debugw("telemetry snapshot skipped", "error", err)
		return
	}
	// Same-day retries and replicas collapse to one event.
	p["$insert_id"] = hash(id, "snapshot", time.Now().UTC().Format("2006-01-02"))
	r.send(ctx, "flexprice_install_snapshot", id, p)
}

func (r *Reporter) instanceID(ctx context.Context) (string, error) {
	first, err := r.db.Reader(ctx).Tenant.Query().
		Order(ent.Asc(tenant.FieldCreatedAt), ent.Asc(tenant.FieldID)).
		FirstID(ctx)
	if err != nil {
		return "", err
	}
	return hash("flexprice-instance", first), nil
}

func (r *Reporter) addCounts(ctx context.Context, instanceID string, p map[string]any) error {
	c := r.db.Reader(ctx)
	pub := string(types.StatusPublished)
	since := time.Now().Add(-30 * 24 * time.Hour)

	tenants, err := c.Tenant.Query().Where(tenant.StatusEQ(pub)).Count(ctx)
	if err != nil {
		return err
	}
	envs, err := c.Environment.Query().Where(environment.StatusEQ(pub)).Count(ctx)
	if err != nil {
		return err
	}
	plans, err := c.Plan.Query().Where(plan.StatusEQ(pub)).Count(ctx)
	if err != nil {
		return err
	}

	customers, err := byTenant(ctx, c.Customer.Query().Where(customer.StatusEQ(pub)).
		GroupBy(customer.FieldTenantID).Aggregate(ent.Count()))
	if err != nil {
		return err
	}
	subs, err := byTenant(ctx, c.Subscription.Query().
		Where(subscription.StatusEQ(pub), subscription.SubscriptionStatusEQ(types.SubscriptionStatusActive)).
		GroupBy(subscription.FieldTenantID).Aggregate(ent.Count()))
	if err != nil {
		return err
	}
	invoices, err := byTenant(ctx, c.Invoice.Query().Where(invoice.StatusEQ(pub), invoice.CreatedAtGT(since)).
		GroupBy(invoice.FieldTenantID).Aggregate(ent.Count()))
	if err != nil {
		return err
	}
	wallets, err := byTenant(ctx, c.Wallet.Query().Where(wallet.StatusEQ(pub)).
		GroupBy(wallet.FieldTenantID).Aggregate(ent.Count()))
	if err != nil {
		return err
	}

	ids := make([]string, 0, len(customers))
	for id := range customers {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return customers[ids[i]] > customers[ids[j]] })

	rows := []map[string]string{}
	for i, id := range ids {
		if i == topTenants {
			break
		}
		rows = append(rows, map[string]string{
			"tenant_hash":          hash(instanceID, id)[:16],
			"customers":            bucket(customers[id]),
			"active_subscriptions": bucket(subs[id]),
			"invoices_30d":         bucket(invoices[id]),
			"wallets":              bucket(wallets[id]),
		})
	}

	p["tenant_count"] = tenants
	p["environments"] = bucket(envs)
	p["plans"] = bucket(plans)
	p["tenants"] = rows
	p["tenants_omitted"] = max(0, len(ids)-topTenants)
	return nil
}

func byTenant(ctx context.Context, s interface {
	Scan(context.Context, any) error
}) (map[string]int, error) {
	var rows []struct {
		TenantID string `json:"tenant_id"`
		Count    int    `json:"count"`
	}
	if err := s.Scan(ctx, &rows); err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[r.TenantID] = r.Count
	}
	return out, nil
}

func (r *Reporter) send(ctx context.Context, event, distinctID string, props map[string]any) {
	props["$process_person_profile"] = false
	body := map[string]any{
		"api_key":     r.cfg.Telemetry.APIKey,
		"event":       event,
		"distinct_id": distinctID,
		"properties":  props,
	}
	if r.cfg.Telemetry.DryRun {
		body["api_key"] = "<redacted>"
		b, _ := json.Marshal(body)
		r.log.Infow("telemetry dry run", "payload", string(b))
		return
	}
	b, err := json.Marshal(body)
	if err != nil {
		return
	}
	host := strings.TrimRight(r.cfg.Telemetry.Host, "/")
	if host == "" {
		host = "https://us.i.posthog.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/capture/", bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.http.Do(req)
	if err != nil {
		r.log.Debugw("telemetry send failed", "event", event, "error", err)
		return
	}
	defer resp.Body.Close()
	r.log.Debugw("telemetry sent", "event", event, "status", resp.StatusCode)
}

func hash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return hex.EncodeToString(sum[:])
}

func bucket(n int) string {
	switch {
	case n <= 0:
		return "0"
	case n < 10:
		return "1-10"
	case n < 100:
		return "10-100"
	case n < 1_000:
		return "100-1k"
	case n < 10_000:
		return "1k-10k"
	case n < 100_000:
		return "10k-100k"
	case n < 1_000_000:
		return "100k-1M"
	}
	return "1M+"
}

func detectPlatform() string {
	switch {
	case os.Getenv("KUBERNETES_SERVICE_HOST") != "":
		return "kubernetes"
	case os.Getenv("ECS_CONTAINER_METADATA_URI_V4") != "":
		return "ecs"
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "docker"
	}
	return "binary"
}

func detectCloud() string {
	var vendor strings.Builder
	for _, f := range []string{"sys_vendor", "product_name"} {
		if b, err := os.ReadFile("/sys/class/dmi/id/" + f); err == nil {
			vendor.Write(b)
		}
	}
	v := strings.ToLower(vendor.String())
	switch {
	case strings.Contains(v, "amazon"):
		return "aws"
	case strings.Contains(v, "google"):
		return "gcp"
	case strings.Contains(v, "microsoft"):
		return "azure"
	}
	return "unknown"
}

