package server

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// opsHandler serves probes, version, and metrics on metricsAddr.
func (s *Server) opsHandler() http.Handler {
	ops := http.NewServeMux()
	ops.Handle("GET /healthz", s.withOpsLogging(http.HandlerFunc(s.handleHealthz)))
	ops.Handle("GET /readyz", s.withOpsLogging(http.HandlerFunc(s.handleReadyz)))
	ops.Handle("GET /version", s.withOpsLogging(http.HandlerFunc(s.handleVersion)))
	// Default registry: includes the Go runtime and process collectors
	// registered in New plus the domain error counters.
	ops.Handle("/metrics", promhttp.Handler())
	return ops
}
