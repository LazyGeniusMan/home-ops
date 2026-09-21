package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestNotifyPostStubIs501(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("POST /notify = %d, want 501 stub", rec.Code)
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
}

func TestDetailsOK(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/details", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /details = %d, want 200", rec.Code)
	}
}

func TestMetricsText(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want Prometheus text", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, "apprise_go_api_up 1") {
		t.Errorf("body = %q, want up gauge", body)
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
