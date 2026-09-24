package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/config"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/notify"
)

func testServer() *Server {
	cfg := config.Config{StatelessStorage: "no", StatefulMode: "disabled", CallTimeoutSecs: 30}
	return New(cfg, notify.New(time.Second), slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func TestNotifyGetIs405(t *testing.T) {
	s := testServer()
	for _, path := range []string{"/notify", "/notify/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s = %d, want 405", path, rec.Code)
		}
		if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
			t.Errorf("GET %s Allow = %q, want POST", path, allow)
		}
	}
}

func TestNotifyPostIsImplemented(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusNotImplemented {
		t.Error("POST /notify = 501, want implemented handler")
	}
}

func TestStatusOK(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET /status body is not JSON: %v", err)
	}
	for _, key := range []string{"status", "can_write_attach", "attach_dir", "config_lock", "stateful_mode", "stateless_storage"} {
		if _, ok := body[key]; !ok {
			t.Errorf("GET /status body missing key %q: %v", key, body)
		}
	}
	if got := body["can_write_attach"]; got != true {
		t.Errorf("can_write_attach = %v, want true (temp dir probe)", got)
	}
	if _, bad := body["attach_permission_issue"]; bad {
		t.Errorf("attach_permission_issue present on writable dir: %v", body)
	}
}

func TestStatusUnwritableAttachDir(t *testing.T) {
	cfg := config.Config{StatelessStorage: "no", StatefulMode: "disabled", CallTimeoutSecs: 30, AttachDir: filepath.Join(t.TempDir(), "missing-parent", "child")}
	// Block creation: plant a regular file where the parent dir would go.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.AttachDir = filepath.Join(blocker, "child")
	s := New(cfg, notify.New(time.Second), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET /status body is not JSON: %v", err)
	}
	if got := body["can_write_attach"]; got != false {
		t.Errorf("can_write_attach = %v, want false", got)
	}
	if got := body["attach_permission_issue"]; got != "ATTACH_PERMISSION_ISSUE" {
		t.Errorf("attach_permission_issue = %v, want ATTACH_PERMISSION_ISSUE", got)
	}
}

func TestDetailsOK(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/details", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /details = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET /details body is not JSON: %v", err)
	}
	raw, _ := json.Marshal(body)
	if strings.Contains(string(raw), "SECRET") || strings.Contains(string(raw), "secret") {
		t.Errorf("GET /details leaks a secret-looking key: %s", raw)
	}
	svcs, ok := body["services"].([]any)
	if !ok || len(svcs) == 0 {
		t.Fatalf("GET /details services = %v, want non-empty catalog", body["services"])
	}
	if n, ok := body["service_count"].(float64); !ok || int(n) != len(svcs) {
		t.Errorf("service_count = %v, want %d", body["service_count"], len(svcs))
	}
	found := false
	for _, v := range svcs {
		if v == "json" {
			found = true
		}
	}
	if !found {
		t.Errorf("services catalog missing %q: %v", "json", svcs)
	}
}

func TestMetricsPrometheus(t *testing.T) {
	resetAttachCache()
	s := testServer()
	// Generate request series before scraping.
	req := httptest.NewRequest(http.MethodGet, "/details", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /details = %d, want 200", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want Prometheus text", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"apprise_go_api_up 1",
		"apprise_go_api_build_info",
		"apprise_go_api_attach_writable 1",
		"apprise_go_api_supported_services",
		"apprise_go_api_http_requests_total",
		"apprise_go_api_http_request_duration_seconds_bucket",
		"go_goroutines",
		"process_cpu_seconds_total",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics body missing %q", want)
		}
	}
}

// TestMetricsRouteLabelIsPattern asserts the request middleware labels the
// route with the matched mux pattern, never user-controlled path content:
// keyed /notify/{KEY} 404s must not create per-key series.
func TestMetricsRouteLabelIsPattern(t *testing.T) {
	resetAttachCache()
	s := testServer()
	req := httptest.NewRequest(http.MethodPost, "/notify/somekey", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /notify/{KEY} = %d, want 404", rec.Code)
	}
	// WithLabelValues creates the series on read; assert it stays zero —
	// the middleware never observes the raw keyed path.
	if got := testutil.ToFloat64(httpRequestsTotal.WithLabelValues(http.MethodPost, "/notify/somekey", "404")); got != 0 {
		t.Errorf("http_requests_total{route=/notify/somekey} = %v, want 0 (route must be the matched pattern)", got)
	}
}

func TestStatelessOnlyRoutesAre404(t *testing.T) {
	s := testServer()
	for _, path := range []string{"/cfg", "/add/", "/json", "/notify/somekey", "/notify/somekey/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404 (stateless-only)", path, rec.Code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/notify/somekey", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("POST /notify/{KEY} = %d, want 404 (stateless-only)", rec.Code)
	}
}

func TestNegotiateFormat(t *testing.T) {
	if got := negotiateFormat("application/json"); got != "json" {
		t.Errorf("negotiateFormat(json) = %q, want json", got)
	}
	if got := negotiateFormat("text/html"); got != "html" {
		t.Errorf("negotiateFormat(html) = %q, want html", got)
	}
	if got := negotiateFormat(""); got != "text" {
		t.Errorf("negotiateFormat(empty) = %q, want text", got)
	}
}

func TestNewNilDefaults(t *testing.T) {
	cfg := config.Config{CallTimeoutSecs: 30}
	s := New(cfg, nil, nil)
	if s.Handler() == nil || s.Sender() == nil {
		t.Error("New(nil) should default logger and sender")
	}
}
