// Request middleware: per-request metrics + one slog line. Route labels
// come from r.Pattern ("notfound" for unmatched), never the raw path.
// /metrics bypasses observation.
package server

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// withMetrics observes method/route/status/duration for every request and
// emits one JSON log line per request (method, route, status, duration).
// The /metrics scrape path bypasses observation entirely (no counter inc,
// no duration observe, no log line) so self-scrapes add no noise.
func withMetrics(log *slog.Logger, next http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	registerMetrics()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		// r.Pattern is set by the mux after dispatch: it holds the matched
		// pattern (e.g. "/notify"), never the raw path. Unmatched requests
		// (404, r.Pattern == "") use the bounded literal "notfound" so the
		// route label can never carry attacker-controlled paths.
		route := r.Pattern
		if route == "" {
			route = "notfound"
		}
		status := strconv.Itoa(rec.status)
		httpRequestsTotal.WithLabelValues(r.Method, route, status).Inc()
		httpRequestDurationSeconds.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
		log.InfoContext(r.Context(), "request",
			slog.String("method", r.Method),
			slog.String("route", route),
			slog.Int("status", rec.status),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

// statusRecorder captures the status code for metrics and request logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
