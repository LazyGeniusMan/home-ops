package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capturingServer builds a Server whose logs go to buf, for route-label
// assertions.
func capturingServer(buf *bytes.Buffer) *Server {
	p := testServer().provider
	return New(p, slog.New(slog.NewJSONHandler(buf, nil)), "127.0.0.1:0", "127.0.0.1:0")
}

// webhookHandler mirrors the mux wiring in Run: withLogging wraps the
// webhook mux as outer middleware.
func webhookHandler(s *Server) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleNegotiate)
	mux.HandleFunc("GET /records", s.handleRecords)
	mux.HandleFunc("POST /records", s.handleApplyChanges)
	mux.HandleFunc("POST /adjustendpoints", s.handleAdjustEndpoints)
	return s.withLogging(mux)
}

// logRoutes decodes one route value per "request" log line in buf.
func logRoutes(t *testing.T, buf *bytes.Buffer) []string {
	t.Helper()
	var routes []string
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		if entry["msg"] != "request" {
			continue
		}
		route, _ := entry["route"].(string)
		routes = append(routes, route)
	}
	return routes
}

func TestWithLoggingRouteLabels(t *testing.T) {
	var buf bytes.Buffer
	s := capturingServer(&buf)
	h := webhookHandler(s)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/records", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("records status = %d", rec.Code)
	}

	routes := logRoutes(t, &buf)
	if len(routes) != 1 {
		t.Fatalf("got %d request log lines, want 1", len(routes))
	}
	if routes[0] != "GET /records" {
		t.Errorf("matched route = %q, want %q", routes[0], "GET /records")
	}
}

func TestWithLoggingCatchAllHidesRawPath(t *testing.T) {
	var buf bytes.Buffer
	s := capturingServer(&buf)
	h := webhookHandler(s)

	// GET / is a catch-all for the GET subtree, so an unknown GET path is
	// served by the negotiate handler — but the log must carry the matched
	// pattern, never the raw path.
	raw := "/totally-unknown-evil-path-12345"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, raw, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("unknown GET status = %d, want 200 (GET / catch-all)", rec.Code)
	}

	routes := logRoutes(t, &buf)
	if len(routes) != 1 {
		t.Fatalf("got %d request log lines, want 1", len(routes))
	}
	if routes[0] != "GET /" {
		t.Errorf("catch-all route = %q, want %q", routes[0], "GET /")
	}
	if strings.Contains(buf.String(), raw) {
		t.Errorf("request log leaks raw path %q", raw)
	}
}

func TestWithLoggingUnknownPathFallsBack(t *testing.T) {
	var buf bytes.Buffer
	s := capturingServer(&buf)
	h := webhookHandler(s)

	// DELETE matches no webhook pattern: the mux answers 405 with
	// r.Pattern empty, so the log must show the bounded fallback.
	raw := "/totally-unknown-evil-path-12345"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, raw, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unknown status = %d, want 405", rec.Code)
	}

	routes := logRoutes(t, &buf)
	if len(routes) != 1 {
		t.Fatalf("got %d request log lines, want 1", len(routes))
	}
	if routes[0] != "notfound" {
		t.Errorf("unknown-path route = %q, want %q", routes[0], "notfound")
	}
	if strings.Contains(buf.String(), raw) {
		t.Errorf("request log leaks raw path %q", raw)
	}
}

func TestWithOpsLoggingRouteLabels(t *testing.T) {
	var buf bytes.Buffer
	s := capturingServer(&buf)
	h := s.opsHandler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d", rec.Code)
	}

	routes := logRoutes(t, &buf)
	if len(routes) != 1 {
		t.Fatalf("got %d request log lines, want 1", len(routes))
	}
	if routes[0] != "GET /healthz" {
		t.Errorf("matched ops route = %q, want %q", routes[0], "GET /healthz")
	}
}

func TestWithOpsLoggingFallbackDirect(t *testing.T) {
	var buf bytes.Buffer
	s := capturingServer(&buf)

	// Exercise withOpsLogging with an undispatched request; it must log
	// "notfound".
	raw := "/ops-direct-evil-path-13579"
	h := s.withOpsLogging(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, raw, nil))

	routes := logRoutes(t, &buf)
	if len(routes) != 1 {
		t.Fatalf("got %d request log lines, want 1", len(routes))
	}
	if routes[0] != "notfound" {
		t.Errorf("ops fallback route = %q, want %q", routes[0], "notfound")
	}
	if strings.Contains(buf.String(), raw) {
		t.Errorf("ops request log leaks raw path %q", raw)
	}
}

func TestWithOpsLoggingUnknownPathUnlogged(t *testing.T) {
	var buf bytes.Buffer
	s := capturingServer(&buf)
	h := s.opsHandler()

	// withOpsLogging wraps each ops route (inner middleware), so an unmatched
	// path is answered 404 by the mux without emitting any request log line
	// — bounded by construction. Assert the raw path appears nowhere.
	raw := "/ops-unknown-evil-path-67890"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, raw, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown status = %d, want 404", rec.Code)
	}

	if routes := logRoutes(t, &buf); len(routes) != 0 {
		t.Errorf("unknown ops path logged routes %q, want none", routes)
	}
	if strings.Contains(buf.String(), raw) {
		t.Errorf("ops request log leaks raw path %q", raw)
	}
}
