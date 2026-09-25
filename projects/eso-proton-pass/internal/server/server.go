// Package server exposes the ESO webhook HTTP surface: GET|POST /get
// (pull → 200 {"value"}), HEAD|GET / (validate), /healthz, /readyz,
// /metrics, POST /push (→ 501).
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/LazyGeniusMan/home-ops/projects/eso-proton-pass/internal/provider"
	"github.com/LazyGeniusMan/home-ops/projects/eso-proton-pass/internal/version"
)

// httpRequestsTotal counts webhook requests by method, route pattern, and
// status code. Route is the registered pattern (never the raw path) so
// label cardinality stays bounded.
// PromQL: sum by (route) (rate(eso_proton_pass_http_requests_total[5m]))
var httpRequestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "eso_proton_pass",
		Name:      "http_requests_total",
		Help:      "Webhook requests handled, by method, route pattern, and status code.",
	},
	[]string{"method", "route", "status"},
)

// httpRequestDurationSeconds observes webhook request latency by method and
// route pattern (DefBuckets keep bucket cardinality small).
// PromQL: histogram_quantile(0.95, sum by (le, route) (rate(eso_proton_pass_http_request_duration_seconds_bucket[5m])))
var httpRequestDurationSeconds = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: "eso_proton_pass",
		Name:      "http_request_duration_seconds",
		Help:      "Webhook request latency in seconds, by method and route pattern.",
		Buckets:   prometheus.DefBuckets,
	},
	[]string{"method", "route"},
)

// up is 1 while the process serves traffic.
// PromQL: eso_proton_pass_up
var up = prometheus.NewGauge(prometheus.GaugeOpts{
	Namespace: "eso_proton_pass",
	Name:      "up",
	Help:      "1 while the process is serving traffic.",
})

// buildInfo reports the ldflags-injected release version as
// eso_proton_pass_build_info{version="..."} == 1.
// PromQL: eso_proton_pass_build_info
var buildInfo = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: "eso_proton_pass",
		Name:      "build_info",
		Help:      "Build metadata; value is always 1, release version in the version label.",
	},
	[]string{"version"},
)

// registerDefaultCollectorsOnce registers Go/process collectors exactly once.
var registerDefaultCollectorsOnce sync.Once

// registerCollector tolerates AlreadyRegisteredError so construction stays
// safe when the default registry already carries the collector.
func registerCollector(c prometheus.Collector) {
	if err := prometheus.Register(c); err != nil {
		if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
			panic(err)
		}
	}
}

// registerOrReuseVec registers c on the default registry, returning the
// already registered collector when New runs more than once in a single
// process (e.g. one Server per test) so every Server's metrics stay live.
func registerOrReuseVec(c *prometheus.CounterVec) *prometheus.CounterVec {
	if err := prometheus.Register(c); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(*prometheus.CounterVec); ok {
				return existing
			}
		}
		panic(err)
	}
	return c
}

// registerOrReuseHist registers c like registerOrReuseVec for histograms.
func registerOrReuseHist(c *prometheus.HistogramVec) *prometheus.HistogramVec {
	if err := prometheus.Register(c); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(*prometheus.HistogramVec); ok {
				return existing
			}
		}
		panic(err)
	}
	return c
}

// registerOrReuseGauge registers c like registerOrReuseVec for gauges.
func registerOrReuseGauge(c prometheus.Gauge) prometheus.Gauge {
	if err := prometheus.Register(c); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(prometheus.Gauge); ok {
				return existing
			}
		}
		panic(err)
	}
	return c
}

// registerOrReuseGaugeVec registers c like registerOrReuseVec for gauge vecs.
func registerOrReuseGaugeVec(c *prometheus.GaugeVec) *prometheus.GaugeVec {
	if err := prometheus.Register(c); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(*prometheus.GaugeVec); ok {
				return existing
			}
		}
		panic(err)
	}
	return c
}

// Server is the HTTP front end for the provider.
type Server struct {
	prov    Provider
	logger  *slog.Logger
	started time.Time

	requestsTotal *prometheus.CounterVec
	duration      *prometheus.HistogramVec
}

// Provider is the subset of provider.Provider used by the server.
type Provider interface {
	GetSecret(r *http.Request, key string) (string, error)
	Ready(ctx context.Context) error
}

// New builds a Server and registers domain and Go/process collectors for
// /metrics. Tests substitute fakes via NewWithProvider.
func New(p *provider.Provider, logger *slog.Logger) *Server {
	registerDefaultCollectorsOnce.Do(func() {
		registerCollector(collectors.NewGoCollector())
		registerCollector(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	})
	requestsTotal := registerOrReuseVec(httpRequestsTotal)
	duration := registerOrReuseHist(httpRequestDurationSeconds)
	upGauge := registerOrReuseGauge(up)
	upGauge.Set(1)
	build := registerOrReuseGaugeVec(buildInfo)
	build.WithLabelValues(version.Version).Set(1)
	return &Server{
		prov:          &httpProvider{p: p},
		logger:        logger,
		started:       time.Now(),
		requestsTotal: requestsTotal,
		duration:      duration,
	}
}

// httpProvider adapts *provider.Provider to the request-scoped interface.
type httpProvider struct {
	p *provider.Provider
}

func (h *httpProvider) GetSecret(r *http.Request, key string) (string, error) {
	return h.p.GetSecret(r.Context(), key)
}

func (h *httpProvider) Ready(ctx context.Context) error {
	return h.p.Ready(ctx)
}

// NewWithProvider builds a Server over a custom Provider (tests).
// Metrics (incl. up/build_info) reuse the shared registry collectors.
func NewWithProvider(p Provider, logger *slog.Logger) *Server {
	registerDefaultCollectorsOnce.Do(func() {
		registerCollector(collectors.NewGoCollector())
		registerCollector(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	})
	upGauge := registerOrReuseGauge(up)
	upGauge.Set(1)
	build := registerOrReuseGaugeVec(buildInfo)
	build.WithLabelValues(version.Version).Set(1)
	return &Server{
		prov:          p,
		logger:        logger,
		started:       time.Now(),
		requestsTotal: registerOrReuseVec(httpRequestsTotal),
		duration:      registerOrReuseHist(httpRequestDurationSeconds),
	}
}

// Handler returns the mux with all routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// Routes omit methods and dispatch on r.Method; metrics use literal
	// patterns (never raw paths) to bound cardinality.
	mux.HandleFunc("/get", s.withMetrics("/get", s.handleGetDispatch))
	mux.HandleFunc("/", s.withMetrics("/", s.handleValidate))
	mux.HandleFunc("/healthz", s.withMetrics("/healthz", s.handleHealthz))
	mux.HandleFunc("/readyz", s.withMetrics("/readyz", s.handleReadyz))
	mux.HandleFunc("/push", s.withMetrics("/push", s.handlePush))
	mux.Handle("/metrics", promhttp.Handler())
	return s.withLogging(mux)
}

type getResponse struct {
	Value string `json:"value"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// handleGetDispatch routes /get by method: GET with ?key= or POST with a
// JSON {"remoteRef": {"key": ...}} body.
func (s *Server) handleGetDispatch(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGet(w, r)
	case http.MethodPost:
		s.handleGetPost(w, r)
	default:
		s.writeError(w, r, errMethodNotAllowed)
	}
}

// handleGet serves GET /get?key=pass://...
func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		s.writeError(w, r, errMissingKey)
		return
	}
	value, err := s.prov.GetSecret(r, key)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, getResponse{Value: value})
}

type getPostBody struct {
	RemoteRef struct {
		Key string `json:"key"`
	} `json:"remoteRef"`
}

// handleGetPost serves POST /get with {"remoteRef": {"key": ...}}.
func (s *Server) handleGetPost(w http.ResponseWriter, r *http.Request) {
	var body getPostBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, r, errBadRequest)
		return
	}
	if body.RemoteRef.Key == "" {
		s.writeError(w, r, errMissingRemoteRefKey)
		return
	}
	value, err := s.prov.GetSecret(r, body.RemoteRef.Key)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, getResponse{Value: value})
}

// handleValidate serves HEAD / and GET / for ESO store validation.
func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.writeError(w, r, errMethodNotAllowed)
		return
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleHealthz is the liveness probe: static JSON, zero downstream calls.
// It never touches the provider, so kubelet liveness checks cannot wedge on
// pass-cli.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readyzTimeout bounds the pass-cli reachability probe so readiness checks
// fail fast instead of hanging a full exec timeout.
const readyzTimeout = 5 * time.Second

// handleReadyz probes pass-cli reachability without resolving a secret (no
// key material leaves the process). Reachable → 200 {"status":"ok"};
// otherwise 503 {"status":"not_ready","failing":"pass-cli"}. The underlying
// error is debug-logged server-side and never exposed.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readyzTimeout)
	defer cancel()
	if err := s.prov.Ready(ctx); err != nil {
		s.logger.DebugContext(r.Context(), "readyz probe failed", slog.Any("err", err))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status":  "not_ready",
			"failing": "pass-cli",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handlePush rejects pushes: pull-only provider.
func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, r, errMethodNotAllowedPush)
		return
	}
	s.writeError(w, r, provider.ErrPushUnimplemented)
}

// withMetrics records per-request counters and latency. Route is the
// registered mux pattern passed by Handler (never the raw path), so label
// cardinality stays bounded.
func (s *Server) withMetrics(route string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next(rec, r)
		s.requestsTotal.WithLabelValues(r.Method, route, strconv.Itoa(rec.status)).Inc()
		s.duration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	}
}

// withLogging logs one line per request with the matched mux pattern as
// route.
func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		route := r.Pattern
		if route == "" {
			route = "unknown"
		}
		s.logger.InfoContext(r.Context(), "request",
			slog.String("method", r.Method),
			slog.String("route", route),
			slog.Int("status", rec.status),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

// statusRecorder captures the status code for request logging and metrics.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
