// Package server wires the stdlib net/http mux, stub handlers, and JSON /
// metrics helpers. G2 implements POST /notify parity, G4 the ':' remap and
// outbound webhook, G5 the full /status /details /metrics bodies.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/config"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/notify"
	apprise "github.com/unraid/apprise-go"
)

// version is the service version reported by /details and /metrics.
const version = "0.1.0"

// Server is the apprise-go-api HTTP server.
type Server struct {
	cfg    config.Config
	sender senderIface
	log    *slog.Logger
	mux    *http.ServeMux
}

// senderIface is the notify.Sender contract the handlers depend on. The
// concrete *notify.Sender satisfies it; tests substitute fakes without
// changing production wiring.
type senderIface interface {
	Send(ctx context.Context, req notify.Request) (notify.Result, error)
	Timeout() time.Duration
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
func (s *Server) Sender() senderIface { return s.sender }

func (s *Server) routes() {
	s.mux.HandleFunc("/notify", s.serveNotifyRoot)
	s.mux.HandleFunc("/notify/", s.serveNotifySub)
	s.mux.HandleFunc("/status", s.handleStatus)
	s.mux.HandleFunc("/details", s.handleDetails)
	s.mux.HandleFunc("/metrics", s.handleMetrics)
}

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

// attachProbe reports whether the attachment staging directory is writable.
// An empty AttachDir resolves to os.TempDir (see internal/attach).
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

// handleStatus reports service health plus attach/config-lock flags. The
// writability probe creates (and removes) a temp file in the resolved
// attach dir; a failure surfaces as ATTACH_PERMISSION_ISSUE.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	dir, canWrite, issue := attachProbe(s.cfg.AttachDir)
	body := map[string]any{
		"status":            "ok",
		"version":           version,
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
// table. It carries no persistence key by design (stateless-only).
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
		"version":           version,
		"stateful_mode":     s.cfg.StatefulMode,
		"stateless_storage": s.cfg.StatelessStorage,
		"service_count":     len(sorted),
		"services":          sorted,
		"routes":            []string{"POST /notify", "GET /status", "GET /details", "GET /metrics"},
	})
}

// handleMetrics emits hand-rolled Prometheus text (stdlib-only, no client_golang).
func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	_, canWrite, _ := attachProbe(s.cfg.AttachDir)
	up := `# HELP apprise_go_api_up 1 if the service is up.
# TYPE apprise_go_api_up gauge
apprise_go_api_up 1
`
	build := `# HELP apprise_go_api_build_info Service build info.
# TYPE apprise_go_api_build_info gauge
apprise_go_api_build_info{version="` + version + `"} 1
`
	attach := `# HELP apprise_go_api_attach_writable 1 if the attachment staging directory is writable.
# TYPE apprise_go_api_attach_writable gauge
`
	attachVal := "0"
	if canWrite {
		attachVal = "1"
	}
	services := `# HELP apprise_go_api_supported_services Number of notification service schemas supported.
# TYPE apprise_go_api_supported_services gauge
`
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "%s%s%sapprise_go_api_attach_writable %s\n%sapprise_go_api_supported_services %d\n",
		up, build, attach, attachVal, services, len(apprise.SupportedSchemas()))
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
