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
	"strings"
	"sync"
	"time"

	"github.com/flexprice/flexprice/ent/environment"
	"github.com/flexprice/flexprice/ent/invoice"
	"github.com/flexprice/flexprice/ent/plan"
	"github.com/flexprice/flexprice/ent/tenant"
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
)

type Reporter struct {
	cfg        *config.Configuration
	db         postgres.IClient
	log        *logger.Logger
	http       *http.Client
	processID  string
	instanceID string
	cloud      string
	cloudOnce  sync.Once
	cancel     context.CancelFunc
	done       chan struct{}
}

func NewReporter(cfg *config.Configuration, db postgres.IClient, log *logger.Logger) *Reporter {
	hostname, _ := os.Hostname()
	r := &Reporter{
		cfg:        cfg,
		db:         db,
		log:        log,
		http:       &http.Client{Timeout: 5 * time.Second},
		processID:  uuid.NewString(),
		instanceID: hostname,
		cloud:      "unknown",
	}
	go r.cloudOnce.Do(func() { r.cloud = probeCloud() })
	return r
}

func (r *Reporter) FireOnce(ctx context.Context) {
	r.heartbeat(ctx)
	r.snapshot(ctx)
}

func RegisterHooks(lc fx.Lifecycle, r *Reporter) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if reason := r.disabledReason(); reason != "" {
				r.log.Info(ctx, "telemetry off", "reason", reason)
				return nil
			}
			r.log.Info(ctx, "telemetry on, set FLEXPRICE_TELEMETRY_DISABLED=true to opt out")
			runCtx, cancel := context.WithCancel(context.Background())
			r.cancel = cancel
			r.done = make(chan struct{})
			go func() {
				defer close(r.done)
				r.run(runCtx)
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if r.cancel == nil {
				return nil
			}
			r.cancel()
			select {
			case <-r.done:
			case <-ctx.Done():
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

func (r *Reporter) base() map[string]any {
	p := map[string]any{
		"instance_id":   r.instanceID,
		"version":       buildinfo.Version,
		"git_sha":       buildinfo.GitSHA,
		"build_variant": buildinfo.BuildVariant,
		"region":        r.cfg.Logging.Region,
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
	}
	if l := r.cfg.Telemetry.InstallLabel; l != "" {
		p["install_label"] = l
	}
	return p
}

func (r *Reporter) heartbeat(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	p := r.base()
	p["process_id"] = r.processID
	p["role"] = string(r.cfg.Deployment.Mode)
	p["platform"] = detectPlatform()
	p["cloud"] = r.cloud
	p["num_cpu"] = runtime.NumCPU()
	r.send(ctx, "flexprice_install_heartbeat", r.instanceID, p)
}

func (r *Reporter) snapshot(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	p := r.base()
	if err := r.addCounts(ctx, p); err != nil {
		r.log.Debug(ctx, "telemetry snapshot skipped", "error", err)
		return
	}
	p["$insert_id"] = hash(r.instanceID, "snapshot", time.Now().UTC().Format("2006-01-02"))
	r.send(ctx, "flexprice_install_snapshot", r.instanceID, p)
}

func (r *Reporter) addCounts(ctx context.Context, p map[string]any) error {
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
	invoices, err := c.Invoice.Query().Where(invoice.StatusEQ(pub), invoice.CreatedAtGT(since)).Count(ctx)
	if err != nil {
		return err
	}

	p["tenant_count"] = tenants
	p["environments"] = bucket(envs)
	p["plans"] = bucket(plans)
	p["invoices_30d"] = bucket(invoices)
	return nil
}

var eventRoutes = map[string]string{
	"flexprice_install_heartbeat": "/v1/heartbeat",
	"flexprice_install_snapshot":  "/v1/snapshot",
}

func (r *Reporter) send(ctx context.Context, event string, _ string, props map[string]any) {
	if r.cfg.Telemetry.DryRun {
		b, _ := json.Marshal(props)
		r.log.Info(ctx, "telemetry dry-run", "event", event, "payload", string(b))
		return
	}
	b, err := json.Marshal(props)
	if err != nil {
		return
	}
	path := eventRoutes[event]
	if path == "" {
		return
	}
	host := r.cfg.Telemetry.Host
	if host == "" {
		host = "https://telemetry.flexprice.io"
	}
	host = strings.TrimRight(host, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+path, bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.http.Do(req)
	if err != nil {
		r.log.Debug(ctx, "telemetry send failed", "event", event, "error", err)
		return
	}
	defer resp.Body.Close()
	r.log.Debug(ctx, "telemetry sent", "event", event, "status", resp.StatusCode)
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

func probeCloud() string {
	var vendor strings.Builder
	for _, f := range []string{"sys_vendor", "product_name"} {
		if b, err := os.ReadFile("/sys/class/dmi/id/" + f); err == nil {
			vendor.Write(b)
		}
	}
	if v := strings.ToLower(vendor.String()); v != "" {
		switch {
		case strings.Contains(v, "amazon"):
			return "aws"
		case strings.Contains(v, "google"):
			return "gcp"
		case strings.Contains(v, "microsoft"):
			return "azure"
		}
	}

	type candidate struct {
		name    string
		url     string
		headers map[string]string
	}
	candidates := []candidate{
		{"aws", "http://169.254.169.254/latest/meta-data/", nil},
		{"gcp", "http://169.254.169.254/computeMetadata/v1/", map[string]string{"Metadata-Flavor": "Google"}},
		{"azure", "http://169.254.169.254/metadata/instance", map[string]string{"Metadata": "true"}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	winner := make(chan string, len(candidates))
	client := &http.Client{Timeout: 2 * time.Second}

	for _, c := range candidates {
		c := c
		go func() {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
			if err != nil {
				return
			}
			for k, v := range c.headers {
				req.Header.Set(k, v)
			}
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			resp.Body.Close()
			if resp.StatusCode < 500 {
				winner <- c.name
			}
		}()
	}

	select {
	case cloud := <-winner:
		return cloud
	case <-ctx.Done():
		return "unknown"
	}
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
