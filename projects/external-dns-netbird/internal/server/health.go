// Health and readiness probes plus the build version endpoint.
//
// GET /healthz is the liveness probe: it returns {"status":"ok"} with zero
// downstream calls (no NetBird API traffic).
//
// GET /readyz probes NetBird API reachability with a bounded Records call
// and returns {"status":"ok"} when reachable, or 503
// {"status":"not_ready","failing":"netbird-api"} when not. The underlying
// error is logged server-side at debug level and never exposed, so probe
// responses carry no traces, tokens, or paths.
//
// GET /version reports the ldflags-injected release version
// (internal/version.Version; "dev" for local builds).
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/version"
)

const readyzTimeout = 5 * time.Second

// handleHealthz is the liveness probe: static JSON, zero downstream calls.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeOpsJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleVersion reports the ldflags-injected release version.
func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeOpsJSON(w, http.StatusOK, map[string]string{"version": version.Version})
}

// handleReadyz probes NetBird API reachability via a bounded Records call
// (a cheap authenticated read over the provider's mockable API surface).
// Only reachability matters, so results are discarded.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readyzTimeout)
	defer cancel()
	if _, err := s.provider.Records(ctx); err != nil {
		s.log.DebugContext(r.Context(), "readyz probe failed", slog.Any("err", err))
		writeOpsJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status":  "not_ready",
			"failing": "netbird-api",
		})
		return
	}
	writeOpsJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// withOpsLogging emits one JSON line per ops request (method, route,
// status, duration). It mirrors the Batch-A withLogging shape for the ops
// listener without touching the webhook middleware. The route is the matched
// ServeMux pattern (r.Pattern, e.g. "GET /healthz" — the method prefix from
// the "METHOD /path" pattern form is kept; it is fine for logs), falling
// back to the bounded literal "notfound" when no pattern matched, never the
// raw path.
func (s *Server) withOpsLogging(next http.Handler) http.Handler {
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

// writeOpsJSON encodes v as application/json. Ops responses must not use
// the webhook MediaType (application/external.dns.webhook+json;version=1),
// which is reserved for the ExternalDNS webhook negotiation.
func writeOpsJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
