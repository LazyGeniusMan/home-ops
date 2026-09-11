// Package server implements the ExternalDNS webhook provider HTTP API:
//   - GET  /                negotiate: returns the domain filter
//   - GET  /records         Records: current endpoints
//   - POST /records         ApplyChanges: apply planned changes
//   - POST /adjustendpoints AdjustEndpoints: provider-specific adjustment
//
// Media type: application/external.dns.webhook+json;version=1
// (see upstream api/webhook.yaml). Only 2xx is success; 5xx is retried by
// ExternalDNS, 4xx is a permanent failure.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"

	nbprovider "github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/provider"
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
// WebhookAddr is localhost-only (sidecar); MetricsAddr exposes /healthz and
// /metrics for probes and scraping.
type Server struct {
	provider    *nbprovider.Provider
	log         *logrus.Entry
	webhookAddr string
	metricsAddr string
	registry    *prometheus.Registry
	recordsErrs prometheus.Counter
	applyErrs   prometheus.Counter
	adjustErrs  prometheus.Counter
}

// New builds a Server.
func New(p *nbprovider.Provider, log *logrus.Entry, webhookAddr, metricsAddr string) *Server {
	reg := prometheus.NewRegistry()
	recordsErrs := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "external_dns_netbird",
		Name:      "records_errors_total",
		Help:      "Errors serving GET /records.",
	})
	applyErrs := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "external_dns_netbird",
		Name:      "apply_changes_errors_total",
		Help:      "Errors serving POST /records.",
	})
	adjustErrs := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "external_dns_netbird",
		Name:      "adjust_endpoints_errors_total",
		Help:      "Errors serving POST /adjustendpoints.",
	})
	reg.MustRegister(recordsErrs, applyErrs, adjustErrs)
	return &Server{
		provider:    p,
		log:         log,
		webhookAddr: webhookAddr,
		metricsAddr: metricsAddr,
		registry:    reg,
		recordsErrs: recordsErrs,
		applyErrs:   applyErrs,
		adjustErrs:  adjustErrs,
	}
}

// Run starts both listeners; it blocks until one of them fails.
func (s *Server) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleNegotiate)
	mux.HandleFunc("GET /records", s.handleRecords)
	mux.HandleFunc("POST /records", s.handleApplyChanges)
	mux.HandleFunc("POST /adjustendpoints", s.handleAdjustEndpoints)

	ops := s.opsHandler()

	webhookSrv := &http.Server{
		Addr:              s.webhookAddr,
		Handler:           mux,
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
		s.log.Infof("webhook API listening on %s", s.webhookAddr)
		errCh <- webhookSrv.ListenAndServe()
	}()
	go func() {
		s.log.Infof("health/metrics listening on %s", s.metricsAddr)
		errCh <- metricsSrv.ListenAndServe()
	}()
	return <-errCh
}

func (s *Server) handleNegotiate(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.provider.GetDomainFilter())
}

func (s *Server) handleRecords(w http.ResponseWriter, r *http.Request) {
	records, err := s.provider.Records(r.Context())
	if err != nil {
		s.recordsErrs.Inc()
		s.log.WithError(err).Error("records failed")
		writeError(w, err)
		return
	}
	if records == nil {
		records = []*endpoint.Endpoint{}
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *Server) handleApplyChanges(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	var changes plan.Changes
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&changes); err != nil {
		s.applyErrs.Inc()
		s.log.WithError(err).Warn("invalid changes payload")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("decode changes: %v", err)})
		return
	}
	if err := s.provider.ApplyChanges(r.Context(), &changes); err != nil {
		s.applyErrs.Inc()
		s.log.WithError(err).Error("apply changes failed")
		writeError(w, err)
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
		s.log.WithError(err).Warn("invalid adjust payload")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("decode endpoints: %v", err)})
		return
	}
	adjusted, err := s.provider.AdjustEndpoints(eps)
	if err != nil {
		s.adjustErrs.Inc()
		s.log.WithError(err).Error("adjust endpoints failed")
		writeError(w, err)
		return
	}
	if adjusted == nil {
		adjusted = []*endpoint.Endpoint{}
	}
	writeJSON(w, http.StatusOK, adjusted)
}

// writeError maps provider failures: transient (soft) errors become 5xx so
// ExternalDNS retries; permanent errors become 4xx.
func writeError(w http.ResponseWriter, err error) {
	if isSoft(err) {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", MediaType)
	w.WriteHeader(status)
	if status == http.StatusNoContent {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logrus.WithError(err).Error("encode response")
	}
}

func isSoft(err error) bool {
	return errors.Is(err, provider.SoftError)
}
