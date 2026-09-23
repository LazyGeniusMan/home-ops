// Request middleware: metrics observation plus per-request slog logging.
//
// withMetrics wraps the mux: after dispatch it records method/route/status
// on httpRequestsTotal, observes duration on httpRequestDurationSeconds,
// and emits one JSON log line (method, route, status, duration). Route
// labels come from r.Pattern (the matched ServeMux pattern), falling back
// to the path only for unmatched requests — never the raw path, so no
// label carries user IDs, URLs, or unbounded values.
package server

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// withMetrics observes method/route/status/duration for every request and
// emits one JSON log line per request (method, route, status, duration).
func withMetrics(log *slog.Logger, next http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	registerMetrics()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		// r.Pattern is set by the mux after dispatch: it holds the matched
		// pattern (e.g. "/notify"), never the raw path. Unmatched requests
		// (404) fall back to the path — mux-default 404s carry no user IDs.
		route := r.Pattern
		if route == "" {
			route = r.URL.Path
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
