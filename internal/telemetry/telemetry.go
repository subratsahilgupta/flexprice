package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/flexprice/flexprice/internal/buildinfo"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"go.uber.org/fx"
)

type Reporter struct {
	cfg        *config.Configuration
	log        *logger.Logger
	http       *http.Client
	instanceID string
	cloud      string
	cloudOnce  sync.Once
}

func NewReporter(cfg *config.Configuration, log *logger.Logger) *Reporter {
	hostname, _ := os.Hostname()
	r := &Reporter{
		cfg:        cfg,
		log:        log,
		http:       &http.Client{Timeout: 5 * time.Second},
		instanceID: hostname,
		cloud:      "unknown",
	}
	go r.cloudOnce.Do(func() { r.cloud = probeCloud() })
	return r
}

func RegisterHooks(lc fx.Lifecycle, r *Reporter) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if reason := r.disabledReason(); reason != "" {
				r.log.Info(ctx, "telemetry off", "reason", reason)
				return nil
			}
			r.log.Info(ctx, "telemetry on, set FLEXPRICE_TELEMETRY_DISABLED=true to opt out")
			r.init(ctx)
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

// init fires once per process start, carrying every static build/host field.
// Per-event pings after this only ever send instance_id + tenant/env/event
// name — PostHog joins them to this $set payload by distinct_id.
func (r *Reporter) init(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	p := map[string]any{
		"version":       buildinfo.Version,
		"git_sha":       buildinfo.GitSHA,
		"build_variant": buildinfo.BuildVariant,
		"region":        r.cfg.Logging.Region,
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"role":          string(r.cfg.Deployment.Mode),
		"platform":      detectPlatform(),
		"cloud":         r.cloud,
		"num_cpu":       runtime.NumCPU(),
	}
	if l := r.cfg.Telemetry.InstallLabel; l != "" {
		p["install_label"] = l
	}
	p["$set"] = map[string]any{
		"version": buildinfo.Version, "git_sha": buildinfo.GitSHA, "build_variant": buildinfo.BuildVariant,
		"region": r.cfg.Logging.Region, "os": runtime.GOOS, "arch": runtime.GOARCH, "cloud": r.cloud,
	}
	r.send(ctx, "flexprice_install_init", p)
}

// ReportEvent fires a lightweight ping for a real system event consumed off
// the webhook topic. Called from the webhook handler's processMessage, right
// after cascade — never blocks delivery, never retried.
func (r *Reporter) ReportEvent(ctx context.Context, eventName string) {
	if reason := r.disabledReason(); reason != "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	p := map[string]any{
		"tenant_id":      types.GetTenantID(ctx),
		"environment_id": types.GetEnvironmentID(ctx),
		"event_name":     eventName,
	}
	r.send(ctx, "flexprice_system_event", p)
}

var eventRoutes = map[string]string{
	"flexprice_install_init": "/v1/init",
	"flexprice_system_event": "/v1/event",
}

func (r *Reporter) send(ctx context.Context, event string, props map[string]any) {
	props["instance_id"] = r.instanceID

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
