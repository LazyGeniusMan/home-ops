// Prometheus metrics on the default registry (incl. go_*/process_*).
// Observed by the withMetrics middleware; route is the matched mux pattern,
// never the raw path.
package server

import (
	"net/http"
	"os"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/version"
	apprise "github.com/unraid/apprise-go"
)

var (
	// httpRequestsTotal counts requests by method, matched route pattern,
	// and status code. PromQL sample:
	//   sum by (route) (rate(apprise_go_api_http_requests_total[5m]))
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "apprise_go_api",
			Name:      "http_requests_total",
			Help:      "Total HTTP requests by method, route pattern, and status code.",
		},
		[]string{"method", "route", "status"},
	)

	// httpRequestDurationSeconds observes request latency by method and
	// matched route pattern. PromQL sample:
	//   histogram_quantile(0.95, sum by (le, route) (rate(apprise_go_api_http_request_duration_seconds_bucket[5m])))
	httpRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "apprise_go_api",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latency in seconds by method and route pattern.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)

	// up is 1 while the service is serving. PromQL sample:
	//   apprise_go_api_up
	upGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "apprise_go_api",
		Name:      "up",
		Help:      "1 if the service is up.",
	})

	// buildInfo carries the ldflags-injected version. PromQL sample:
	//   apprise_go_api_build_info
	buildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "apprise_go_api",
			Name:      "build_info",
			Help:      "Service build info.",
		},
		[]string{"version"},
	)

	// attachWritable is 1 when the attachment staging directory is writable
	// (TTL-cached probe, refreshed at most every attachProbeTTL). PromQL sample:
	//   apprise_go_api_attach_writable
	attachWritable = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "apprise_go_api",
		Name:      "attach_writable",
		Help:      "1 if the attachment staging directory is writable.",
	})

	// supportedServices is the number of notification service schemas
	// supported by apprise-go. PromQL sample:
	//   apprise_go_api_supported_services
	supportedServices = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "apprise_go_api",
		Name:      "supported_services",
		Help:      "Number of notification service schemas supported.",
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
		if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
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
	})
	upGauge.Set(1)
	buildInfo.WithLabelValues(version.Version).Set(1)
	supportedServices.Set(float64(len(apprise.SupportedSchemas())))
}

// metricsHandler registers metrics and serves GET /metrics via promhttp on
// the default registry.
func metricsHandler() http.Handler {
	registerMetrics()
	return promhttp.Handler()
}

// attachProbe reports whether the attachment staging directory is writable.
// An empty AttachDir resolves to os.TempDir (see internal/attach). Each call
// performs MkdirAll + CreateTemp, so callers must use cachedAttachProbe.
func attachProbe(dir string) (resolved string, canWrite bool, issue string) {
	resolved = dir
	if resolved == "" {
		resolved = os.TempDir()
	}
	if err := os.MkdirAll(resolved, 0o750); err != nil {
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
