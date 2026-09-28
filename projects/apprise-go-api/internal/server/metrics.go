// Prometheus metrics on the default registry (incl. go_*/process_*).
// Observed by the withMetrics middleware; route is the matched mux pattern,
// never the raw path.
package server

import (
	"errors"
	"net/http"
	"os"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/notify"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/version"
	apprise "github.com/unraid/apprise-go"
)

var (
	// httpRequestsTotal counts requests by method, route pattern, and status.
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "apprise_go_api",
			Name:      "http_requests_total",
			Help:      "Total HTTP requests by method, route pattern, and status code.",
		},
		[]string{"method", "route", "status"},
	)

	// httpRequestDurationSeconds observes request latency by method and route.
	httpRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "apprise_go_api",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latency in seconds by method and route pattern.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)

	// up is 1 while the service is serving.
	upGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "apprise_go_api",
		Name:      "up",
		Help:      "1 if the service is up.",
	})

	// buildInfo carries the ldflags-injected version.
	buildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "apprise_go_api",
			Name:      "build_info",
			Help:      "Service build info.",
		},
		[]string{"version"},
	)

	// attachWritable is 1 when the attachment staging dir is writable (TTL-cached).
	attachWritable = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "apprise_go_api",
		Name:      "attach_writable",
		Help:      "1 if the attachment staging directory is writable.",
	})

	// supportedServices is the number of apprise-go service schemas.
	supportedServices = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "apprise_go_api",
		Name:      "supported_services",
		Help:      "Number of notification service schemas supported.",
	})

	// sendTimeoutsTotal counts Send calls that gave up on the per-call timeout.
	sendTimeoutsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "apprise_go_api",
		Name:      "send_timeouts_total",
		Help:      "Total notify Send calls that timed out.",
	})

	// sendInFlight tracks concurrent Send calls (bounded by maxInFlight).
	sendInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "apprise_go_api",
		Name:      "send_in_flight",
		Help:      "Current in-flight notify Send calls.",
	})

	registerMetricsOnce sync.Once
)

// registerDefaultCollectorsOnce registers stdlib runtime/process
// collectors exactly once (tolerates AlreadyRegistered in shared test
// processes).
var registerDefaultCollectorsOnce sync.Once

// registerCollector tolerates AlreadyRegisteredError so New() stays safe when
// the default registry already carries the collector (shared test process).
func registerCollector(c prometheus.Collector) {
	if err := prometheus.Register(c); err != nil {
		var already prometheus.AlreadyRegisteredError
		if !errors.As(err, &already) {
			panic(err)
		}
	}
}

// registerMetrics registers the domain metrics on the default registry once
// and refreshes the constant gauges (up, build_info, supported_services).
func registerMetrics() {
	registerMetricsOnce.Do(func() {
		registerDefaultCollectorsOnce.Do(func() {
			registerCollector(collectors.NewGoCollector())
			registerCollector(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
		})
		registerCollector(httpRequestsTotal)
		registerCollector(httpRequestDurationSeconds)
		registerCollector(upGauge)
		registerCollector(buildInfo)
		registerCollector(attachWritable)
		registerCollector(supportedServices)
		registerCollector(sendTimeoutsTotal)
		registerCollector(sendInFlight)
	})
	upGauge.Set(1)
	buildInfo.WithLabelValues(version.Version).Set(1)
	supportedServices.Set(float64(len(apprise.SupportedSchemas())))
	refreshSendGauges()
}

// lastTimeouts tracks the notify timeout count already exported so the
// counter only moves forward by the delta on each refresh.
var lastTimeouts uint64

// refreshSendGauges syncs the send pipeline gauges from the notify package
// counters. It runs on every registerMetrics call (every request via the
// middleware) so the timeout counter and in-flight gauge never go stale.
func refreshSendGauges() {
	if cur := notify.ReportTimeouts(); cur != lastTimeouts {
		if cur > lastTimeouts {
			sendTimeoutsTotal.Add(float64(cur - lastTimeouts))
		}
		lastTimeouts = cur
	}
	sendInFlight.Set(float64(notify.ReportInFlight()))
}

// metricsHandler registers metrics and serves GET /metrics via promhttp on
// the default registry.
func metricsHandler() http.Handler {
	registerMetrics()
	return promhttp.Handler()
}

// attachProbe reports whether the attachment staging directory is writable.
// An empty AttachDir resolves to os.TempDir (see internal/attach). The
// probe never creates directories: the staging path deliberately refuses to
// auto-create on the request path (see stageStream), so a probe that called
// MkdirAll would mask a misconfigured mount and report healthy while every
// request fails. A missing dir is NOT_WRITABLE_ISSUE, not healthy.
func attachProbe(dir string) (resolved string, canWrite bool, issue string) {
	resolved = dir
	if resolved == "" {
		resolved = os.TempDir()
	}
	st, err := os.Stat(resolved)
	if err != nil || !st.IsDir() {
		return resolved, false, "ATTACH_PERMISSION_ISSUE"
	}
	f, err := os.CreateTemp(resolved, ".writability-*")
	if err != nil {
		return resolved, false, "ATTACH_PERMISSION_ISSUE"
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return resolved, true, ""
}
