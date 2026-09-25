// Health probes plus the build version endpoint. /healthz: static ok;
// /readyz: bounded NetBird API probe (503 when unreachable); /version:
// ldflags-injected release version.
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/tracing"
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

// withOpsLogging logs one line per ops request with the matched pattern as
// route.
func (s *Server) withOpsLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.log.InfoContext(r.Context(), "request",
			append([]any{
				slog.String("method", r.Method),
				slog.String("route", requestRoute(r)),
				slog.Int("status", rec.status),
				slog.Duration("duration", time.Since(start)),
			}, tracing.TraceAttrs(r.Context())...)...,
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
