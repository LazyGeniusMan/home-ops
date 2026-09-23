// Tests for the liveness and readiness probes plus the attach-dir TTL
// cache: /healthz performs zero downstream calls, /readyz reports 200 vs
// 503 with the failing dependency named, and the writability probe runs at
// most once per TTL window (no per-scrape side effects).
package server

import (
	"context"
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

	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/config"
	"github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/notify"
)

// countingNotifySender records Send invocations without delivering: /healthz
// must make zero downstream calls.
type countingNotifySender struct{ calls int }

func (f *countingNotifySender) Send(_ context.Context, _ notify.Request) (notify.Result, error) {
	f.calls++
	return notify.Result{}, nil
}

func (f *countingNotifySender) Timeout() time.Duration { return time.Second }

func TestHealthzNoDownstreamCalls(t *testing.T) {
	resetAttachCache()
	fake := &countingNotifySender{}
	cfg := config.Config{StatelessStorage: "no", StatefulMode: "disabled", CallTimeoutSecs: 30}
	s := New(cfg, notify.New(time.Second), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	s.sender = fake
	start := time.Now()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if elapsed := time.Since(start); elapsed >= 50*time.Millisecond {
		t.Errorf("GET /healthz took %v, want <50ms", elapsed)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET /healthz body is not JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("GET /healthz body = %v, want {status:ok}", body)
	}
	if fake.calls != 0 {
		t.Errorf("sender calls = %d, want 0 (liveness must not touch downstream)", fake.calls)
	}
}

func TestReadyzReadyVsNotReady(t *testing.T) {
	resetAttachCache()
	newServer := func(dir string) *Server {
		cfg := config.Config{StatelessStorage: "no", StatefulMode: "disabled", CallTimeoutSecs: 30, AttachDir: dir}
		return New(cfg, notify.New(time.Second), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	}
	// Ready: writable dir → 200 {"status":"ok"}.
	s := newServer(t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /readyz (ready) = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET /readyz body is not JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("GET /readyz body = %v, want {status:ok}", body)
	}
	// Not ready: unwritable dir → 503 {"status":"not_ready","failing":...}.
	resetAttachCache()
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s = newServer(filepath.Join(blocker, "child"))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /readyz (not ready) = %d, want 503", rec.Code)
	}
	body = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET /readyz 503 body is not JSON: %v", err)
	}
	if body["status"] != "not_ready" {
		t.Errorf("GET /readyz 503 body status = %v, want not_ready", body["status"])
	}
	failing, ok := body["failing"].(string)
	if !ok || failing == "" {
		t.Errorf("GET /readyz 503 body failing = %v, want named dependency", body["failing"])
	}
	raw, _ := json.Marshal(body)
	if strings.Contains(string(raw), "Trace") || strings.Contains(string(raw), "/tmp/") {
		t.Errorf("GET /readyz 503 body leaks internals: %s", raw)
	}
}

// TestAttachProbeCached asserts the writability probe runs at most once per
// TTL window: repeated /status calls after removing the dir still report
// the cached writable result (no per-scrape MkdirAll+CreateTemp).
func TestAttachProbeCached(t *testing.T) {
	resetAttachCache()
	dir := t.TempDir()
	cfg := config.Config{StatelessStorage: "no", StatefulMode: "disabled", CallTimeoutSecs: 30, AttachDir: dir}
	s := New(cfg, notify.New(time.Second), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /status = %d, want 200", rec.Code)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/status", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET /status body is not JSON: %v", err)
	}
	if got := body["can_write_attach"]; got != true {
		t.Errorf("can_write_attach = %v, want cached true (no re-probe within TTL)", got)
	}
}
