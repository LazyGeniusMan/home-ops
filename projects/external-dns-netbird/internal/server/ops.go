package server

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// opsHandler serves liveness/readiness probes and Prometheus metrics.
func (s *Server) opsHandler() http.Handler {
	ops := http.NewServeMux()
	ops.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	// Default registry: includes the Go runtime and process collectors
	// registered in New plus the domain error counters.
	ops.Handle("/metrics", promhttp.Handler())
	return ops
}
