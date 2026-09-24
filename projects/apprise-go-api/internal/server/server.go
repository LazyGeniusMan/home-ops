// Package server wires the stdlib net/http mux and registers routes.
// handler.go serves POST /notify parity, sender.go decodes payloads,
// validation.go validates fields, errors.go holds the error contract,
// health.go serves liveness/readiness probes, metrics.go holds the
// Prometheus collectors, and middleware.go observes requests.
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/config"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/notify"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/version"
	apprise "github.com/unraid/apprise-go"
)

// Server is the apprise-go-api HTTP server.
type Server struct {
	cfg    config.Config
	sender senderIface
	log    *slog.Logger
	mux    *http.ServeMux
}

// New wires dependencies and registers routes.
func New(cfg config.Config, sender *notify.Sender, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	if sender == nil {
		sender = notify.New(time.Duration(cfg.CallTimeoutSecs) * time.Second)
	}
	s := &Server{cfg: cfg, sender: sender, log: log, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Sender exposes the notify sender used by the handlers.
func (s *Server) Sender() senderIface { return s.sender }

func (s *Server) routes() {
	s.mux.HandleFunc("/notify", s.serveNotifyRoot)
	s.mux.HandleFunc("/notify/", s.serveNotifySub)
	s.mux.HandleFunc("/status", s.handleStatus)
	s.mux.HandleFunc("/details", s.handleDetails)
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/readyz", s.handleReadyz)
	s.mux.Handle("/metrics", metricsHandler())
}

// Handler returns the registered mux wrapped in the request metrics +
// logging middleware. Route labels come from r.Pattern (the matched mux
// pattern), never the raw path, so no label carries user IDs, URLs, or
// unbounded values.
func (s *Server) Handler() http.Handler { return withMetrics(s.log, s.mux) }

// serveNotifyRoot serves POST /notify exactly (stateless-only).
func (s *Server) serveNotifyRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/notify" {
		http.NotFound(w, r)
		return
	}
	s.serveNotify(w, r)
}

// serveNotifySub serves POST /notify/ exactly; any deeper path (keyed
// /notify/{KEY}, stateful /add/, /cfg, ...) is 404 — the service is
// stateless-only and exposes no per-key storage routes.
func (s *Server) serveNotifySub(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/notify/" {
		http.NotFound(w, r)
		return
	}
	s.serveNotify(w, r)
}

// handleStatus reports service health plus attach/config-lock flags. The
// attach writability lookup is served from the TTL cache (attachWritable);
// failures surface as ATTACH_PERMISSION_ISSUE. /status never probes per
// scrape — the cached gauge feeds /metrics.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	dir, canWrite, issue := cachedAttachProbe(s.cfg.AttachDir)
	body := map[string]any{
		"status":            "ok",
		"version":           version.Version,
		"stateful_mode":     s.cfg.StatefulMode,
		"stateless_storage": s.cfg.StatelessStorage,
		"attach_dir":        dir,
		"can_write_attach":  canWrite,
		"config_lock":       false,
	}
	if issue != "" {
		body["attach_permission_issue"] = issue
	}
	writeJSON(w, http.StatusOK, body)
}

// handleDetails returns the stateless service catalog: the notification
// service schemas supported by apprise-go plus the stateless-only route
// table. It carries no persistence key by design (stateless-only). This is
// a domain endpoint, not a health probe — use /healthz and /readyz for
// probes.
func (s *Server) handleDetails(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	schemas := apprise.SupportedSchemas()
	sorted := append([]string(nil), schemas...)
	sort.Strings(sorted)
	writeJSON(w, http.StatusOK, map[string]any{
		"version":           version.Version,
		"stateful_mode":     s.cfg.StatefulMode,
		"stateless_storage": s.cfg.StatelessStorage,
		"service_count":     len(sorted),
		"services":          sorted,
		"routes":            []string{"POST /notify", "GET /status", "GET /details", "GET /metrics", "GET /healthz", "GET /readyz"},
	})
}

// writeJSON encodes v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// negotiateFormat picks the response shape from Accept: application/json ->
// json, text/*|html -> html, else text.
func negotiateFormat(accept string) string {
	a := strings.ToLower(accept)
	switch {
	case strings.Contains(a, "application/json"):
		return "json"
	case strings.Contains(a, "text/html"), strings.HasPrefix(strings.TrimSpace(a), "text/"):
		return "html"
	default:
		return "text"
	}
}
