// Package server exposes the HTTP surface consumed by the ESO generic
// webhook provider:
//
//	GET  /get?key=pass://vault/item/field  pull path → 200 {"value": "..."}
//	POST /get {"remoteRef":{"key":"..."}}   pull path (JSON body variant)
//	HEAD /  | GET /                         validate path → 200
//	GET  /healthz                            liveness → 200 {"status":"ok"}
//	GET  /metrics                            Prometheus metrics (text)
//	POST /push                               → 501 TODO (pull-only)
//
// All pull responses use the {"value": ...} envelope so ESO can extract the
// secret with result.jsonPath "$.value".
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/LazyGeniusMan/home-ops/eso-proton-pass/internal/provider"
)

// Server is the HTTP front end for the provider.
type Server struct {
	prov    Provider
	logger  *slog.Logger
	started time.Time

	requestsTotal  atomic.Uint64
	requestsFailed atomic.Uint64
}

// Provider is the subset of provider.Provider used by the server.
type Provider interface {
	GetSecret(r *http.Request, key string) (string, error)
}

// New builds a Server. The provider dependency is the concrete
// provider.Provider; it is adapted so tests can substitute fakes via
// NewWithProvider.
func New(p *provider.Provider, logger *slog.Logger) *Server {
	return &Server{prov: &httpProvider{p: p}, logger: logger, started: time.Now()}
}

// httpProvider adapts *provider.Provider to the request-scoped interface.
type httpProvider struct {
	p *provider.Provider
}

func (h *httpProvider) GetSecret(r *http.Request, key string) (string, error) {
	return h.p.GetSecret(r.Context(), key)
}

// NewWithProvider builds a Server over a custom Provider (tests).
func NewWithProvider(p Provider, logger *slog.Logger) *Server {
	return &Server{prov: p, logger: logger, started: time.Now()}
}

// Handler returns the mux with all routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// NB: method-specific patterns like "HEAD /" conflict with subtrees such
	// as "GET /get" in Go 1.22+ ServeMux, so routes are registered without
	// methods and dispatch on r.Method inside each handler.
	mux.HandleFunc("/get", s.handleGetDispatch)
	mux.HandleFunc("/", s.handleValidate)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/push", s.handlePush)
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
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed: use GET or POST"})
	}
}

// handleGet serves GET /get?key=pass://...
func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		s.count(false)
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "missing ?key=pass://{vault}/{item}/{field}"})
		return
	}
	value, err := s.prov.GetSecret(r, key)
	if err != nil {
		s.count(false)
		if errors.Is(err, provider.ErrNotFound) {
			// 404 lets ESO apply the ExternalSecret deletionPolicy.
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "secret not found"})
			return
		}
		s.logger.Error("get failed", slog.String("error", err.Error()))
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to resolve secret"})
		return
	}
	s.count(true)
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
		s.count(false)
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body: want {\"remoteRef\": {\"key\": \"pass://...\"}}"})
		return
	}
	if body.RemoteRef.Key == "" {
		s.count(false)
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "missing remoteRef.key"})
		return
	}
	r.URL.RawQuery = "key=" + body.RemoteRef.Key
	// Reuse the GET path by forwarding the extracted key.
	value, err := s.prov.GetSecret(r, body.RemoteRef.Key)
	if err != nil {
		s.count(false)
		if errors.Is(err, provider.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "secret not found"})
			return
		}
		s.logger.Error("get failed", slog.String("error", err.Error()))
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to resolve secret"})
		return
	}
	s.count(true)
	writeJSON(w, http.StatusOK, getResponse{Value: value})
}

// handleValidate serves HEAD / and GET / for ESO store validation.
func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	s.count(true)
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleHealthz serves the liveness probe.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleMetrics serves minimal Prometheus-format counters plus uptime.
func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP eso_proton_pass_requests_total Total webhook requests handled.\n")
	fmt.Fprintf(w, "# TYPE eso_proton_pass_requests_total counter\n")
	fmt.Fprintf(w, "eso_proton_pass_requests_total %d\n", s.requestsTotal.Load())
	fmt.Fprintf(w, "# HELP eso_proton_pass_requests_failed_total Total failed webhook requests.\n")
	fmt.Fprintf(w, "# TYPE eso_proton_pass_requests_failed_total counter\n")
	fmt.Fprintf(w, "eso_proton_pass_requests_failed_total %d\n", s.requestsFailed.Load())
	fmt.Fprintf(w, "# HELP eso_proton_pass_uptime_seconds Seconds since process start.\n")
	fmt.Fprintf(w, "# TYPE eso_proton_pass_uptime_seconds gauge\n")
	fmt.Fprintf(w, "eso_proton_pass_uptime_seconds %d\n", int64(time.Since(s.started).Seconds()))
}

// handlePush rejects pushes: pull-only provider (TODO).
func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed: use POST"})
		return
	}
	s.count(false)
	writeJSON(w, http.StatusNotImplemented, errorResponse{Error: provider.ErrPushUnimplemented.Error()})
}

func (s *Server) count(ok bool) {
	s.requestsTotal.Add(1)
	if !ok {
		s.requestsFailed.Add(1)
	}
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Info("request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Duration("elapsed", time.Since(start)),
		)
	})
}
