// Package server wires the stdlib net/http mux, stub handlers, and JSON /
// metrics helpers. G2 implements POST /notify parity, G4 the ':' remap and
// outbound webhook, G5 the full /status /details /metrics bodies.
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/config"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/notify"
)

// Server is the apprise-go-api HTTP server.
type Server struct {
	cfg    config.Config
	sender *notify.Sender
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

// Handler returns the registered mux.
func (s *Server) Handler() http.Handler { return s.mux }

// Sender exposes the notify sender (used by handlers in G2).
func (s *Server) Sender() *notify.Sender { return s.sender }

func (s *Server) routes() {
	s.mux.HandleFunc("/notify", s.handleNotify)
	s.mux.HandleFunc("/notify/", s.handleNotify)
	s.mux.HandleFunc("/status", s.handleStatus)
	s.mux.HandleFunc("/details", s.handleDetails)
	s.mux.HandleFunc("/metrics", s.handleMetrics)
}

// handleNotify is a G1 stub: POST only; full G2 parity lands later.
func (s *Server) handleNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "notify not implemented (G2)"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":            "ok",
		"stateless_storage": s.cfg.StatelessStorage,
		"stateful_mode":     s.cfg.StatefulMode,
	})
}

func (s *Server) handleDetails(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": "0.1.0"})
}

// handleMetrics emits hand-rolled Prometheus text (stdlib-only, no client_golang).
func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("# HELP apprise_go_api_up 1 if the service is up.\n# TYPE apprise_go_api_up gauge\napprise_go_api_up 1\n"))
}

// writeJSON encodes v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// negotiateFormat picks the response shape from Accept: application/json ->
// json, text/*|html -> html, else text. Full G2 negotiation lands later.
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
