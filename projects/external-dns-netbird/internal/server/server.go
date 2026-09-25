// Package server implements the ExternalDNS webhook provider HTTP API
// (media type application/external.dns.webhook+json;version=1): 2xx is
// success, 5xx retried, 4xx permanent.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"

	nbprovider "github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/provider"
	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/version"
)

// MediaType is the webhook content type and version negotiated with ExternalDNS.
const MediaType = "application/external.dns.webhook+json;version=1"

const (
	maxBodyBytes   = 32 << 20 // 32 MiB, matches ExternalDNS --webhook-provider-max-body-size default
	readTimeout    = 30 * time.Second
	writeTimeout   = 60 * time.Second
	headerTimeout  = 10 * time.Second
	maxHeaderBytes = 1 << 20 // 1 MiB
)

// Server serves the webhook provider API plus health/metrics endpoints.
// WebhookAddr is localhost-only (sidecar); MetricsAddr exposes /healthz,
// /readyz, /version, and /metrics for probes and scraping.
type Server struct {
	provider    *nbprovider.Provider
	log         *slog.Logger
	webhookAddr string
	metricsAddr string
	recordsErrs prometheus.Counter
	applyErrs   prometheus.Counter
	adjustErrs  prometheus.Counter
}

// buildInfo reports the ldflags-injected release version as
// external_dns_netbird_build_info{version="..."} == 1.
// PromQL: external_dns_netbird_build_info
var buildInfo = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: "external_dns_netbird",
		Name:      "build_info",
		Help:      "Build metadata; value is always 1, release version in the version label.",
	},
	[]string{"version"},
)

func init() {
	registerCollector(buildInfo)
}

// registerDefaultCollectorsOnce registers Go/process collectors exactly once.
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

// registerOrReuse registers c on the default registry, returning the already
// registered collector when New() runs more than once in a single process
// (e.g. one Server per test) so every Server's counters stay live.
func registerOrReuse(c prometheus.Counter) prometheus.Counter {
	if err := prometheus.Register(c); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(prometheus.Counter); ok {
				return existing
			}
		}
		panic(err)
	}
	return c
}

// New builds a Server and registers domain and Go/process collectors.
func New(p *nbprovider.Provider, log *slog.Logger, webhookAddr, metricsAddr string) *Server {
	registerDefaultCollectorsOnce.Do(func() {
		registerCollector(collectors.NewGoCollector())
		registerCollector(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	})
	recordsErrs := registerOrReuse(prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "external_dns_netbird",
		Name:      "records_errors_total",
		Help:      "Errors serving GET /records.",
	}))
	applyErrs := registerOrReuse(prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "external_dns_netbird",
		Name:      "apply_changes_errors_total",
		Help:      "Errors serving POST /records.",
	}))
	adjustErrs := registerOrReuse(prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "external_dns_netbird",
		Name:      "adjust_endpoints_errors_total",
		Help:      "Errors serving POST /adjustendpoints.",
	}))
	// Build version gauge: external_dns_netbird_build_info{version="..."} == 1.
	// Surface via build_info (documented) — no separate /version sampler
	// needed beyond the ops endpoint.
	buildInfo.WithLabelValues(version.Version).Set(1)
	return &Server{
		provider:    p,
		log:         log,
		webhookAddr: webhookAddr,
		metricsAddr: metricsAddr,
		recordsErrs: recordsErrs,
		applyErrs:   applyErrs,
		adjustErrs:  adjustErrs,
	}
}

// shutdownTimeout bounds graceful drain of both listeners on SIGTERM
// (docker stop): in-flight webhook applies finish before the process exits.
const shutdownTimeout = 10 * time.Second

// Run starts both listeners (webhook + ops) and blocks until the context
// is cancelled or one listener fails. On context cancellation it shuts
// both servers down gracefully with shutdownTimeout, draining in-flight
// requests; docker stop therefore drains instead of hard-killing.
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleNegotiate)
	mux.HandleFunc("GET /records", s.handleRecords)
	mux.HandleFunc("POST /records", s.handleApplyChanges)
	mux.HandleFunc("POST /adjustendpoints", s.handleAdjustEndpoints)

	ops := s.opsHandler()

	webhookSrv := &http.Server{
		Addr:              s.webhookAddr,
		Handler:           s.withLogging(mux),
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		ReadHeaderTimeout: headerTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	metricsSrv := &http.Server{
		Addr:              s.metricsAddr,
		Handler:           ops,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		ReadHeaderTimeout: headerTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}

	errCh := make(chan error, 2)
	go func() {
		s.log.InfoContext(ctx, "webhook API listening", slog.String("addr", s.webhookAddr))
		if err := webhookSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	go func() {
		s.log.InfoContext(ctx, "health/metrics listening", slog.String("addr", s.metricsAddr))
		if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		s.log.Info("shutting down", slog.String("reason", ctx.Err().Error()))
	case err := <-errCh:
		// One listener failed: shut the other down gracefully before
		// returning so no listener is orphaned.
		if err != nil {
			s.log.Error("listener failed, shutting down", slog.Any("err", err))
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = webhookSrv.Shutdown(shutdownCtx)
		_ = metricsSrv.Shutdown(shutdownCtx)
		s.log.Info("drained")
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(2)
	var webhookErr, metricsErr error
	go func() {
		defer wg.Done()
		webhookErr = webhookSrv.Shutdown(shutdownCtx)
	}()
	go func() {
		defer wg.Done()
		metricsErr = metricsSrv.Shutdown(shutdownCtx)
	}()
	wg.Wait()
	s.log.Info("drained")
	return errors.Join(webhookErr, metricsErr)
}

func (s *Server) handleNegotiate(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, s.provider.GetDomainFilter())
}

func (s *Server) handleRecords(w http.ResponseWriter, r *http.Request) {
	records, err := s.provider.Records(r.Context())
	if err != nil {
		s.recordsErrs.Inc()
		s.log.ErrorContext(r.Context(), "records failed", slog.Any("err", err))
		s.writeError(w, err)
		return
	}
	if records == nil {
		records = []*endpoint.Endpoint{}
	}
	s.writeJSON(w, http.StatusOK, records)
}

func (s *Server) handleApplyChanges(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	var changes plan.Changes
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&changes); err != nil {
		s.applyErrs.Inc()
		s.log.WarnContext(r.Context(), "invalid changes payload", slog.Any("err", err))
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("decode changes: %v", err)})
		return
	}
	if err := s.provider.ApplyChanges(r.Context(), &changes); err != nil {
		s.applyErrs.Inc()
		s.log.ErrorContext(r.Context(), "apply changes failed", slog.Any("err", err))
		s.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdjustEndpoints(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	var eps []*endpoint.Endpoint
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&eps); err != nil {
		s.adjustErrs.Inc()
		s.log.WarnContext(r.Context(), "invalid adjust payload", slog.Any("err", err))
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("decode endpoints: %v", err)})
		return
	}
	adjusted, err := s.provider.AdjustEndpoints(eps)
	if err != nil {
		s.adjustErrs.Inc()
		s.log.ErrorContext(r.Context(), "adjust endpoints failed", slog.Any("err", err))
		s.writeError(w, err)
		return
	}
	if adjusted == nil {
		adjusted = []*endpoint.Endpoint{}
	}
	s.writeJSON(w, http.StatusOK, adjusted)
}

// writeError maps failures to status codes with a sanitized {"error"}
// envelope.
func (s *Server) writeError(w http.ResponseWriter, err error) {
	s.writeJSON(w, statusCodeOf(err), map[string]string{"error": publicError(err)})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", MediaType)
	w.WriteHeader(status)
	if status == http.StatusNoContent {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.log.Error("encode response", slog.Any("err", err))
	}
}

// withLogging logs one line per webhook request with the matched pattern
// as route.
func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.log.InfoContext(r.Context(), "request",
			slog.String("method", r.Method),
			slog.String("route", requestRoute(r)),
			slog.Int("status", rec.status),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

// requestRoute returns the matched ServeMux pattern or "notfound".
func requestRoute(r *http.Request) string {
	if r.Pattern != "" {
		return r.Pattern
	}
	return "notfound"
}

// statusRecorder captures the status code for request logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
