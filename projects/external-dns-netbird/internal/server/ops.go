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
	ops.Handle("/metrics", promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{}))
	return ops
}
