package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/LazyGeniusMan/home-ops/projects/eso-proton-pass/internal/provider"
)

type fakeProvider struct {
	value   string
	err     error
	last    string
	ready   error
	gets    int
	readies int
}

func (f *fakeProvider) GetSecret(_ *http.Request, key string) (string, error) {
	f.last = key
	f.gets++
	if f.err != nil {
		return "", f.err
	}
	// Mirror provider semantics for parity: validate then return.
	if _, _, _, verr := provider.ValidateKey(key); verr != nil {
		return "", verr
	}
	return f.value, nil
}

func (f *fakeProvider) Ready(_ context.Context) error {
	f.readies++
	return f.ready
}

func testServer(f *fakeProvider) *Server {
	return NewWithProvider(f, slog.New(slog.NewTextHandler(os.Stderr, nil)))
}

func TestGetPullPath(t *testing.T) {
	f := &fakeProvider{value: "s3cret"}
	srv := testServer(f).Handler()
	req := httptest.NewRequest(http.MethodGet, "/get?key=pass://vault/item/password", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var got getResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Value != "s3cret" {
		t.Errorf("value = %q", got.Value)
	}
	if f.last != "pass://vault/item/password" {
		t.Errorf("provider got key %q", f.last)
	}
}

func TestGetMissingKey(t *testing.T) {
	srv := testServer(&fakeProvider{value: "x"}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/get", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestGetNotFoundMaps404(t *testing.T) {
	srv := testServer(&fakeProvider{err: provider.ErrNotFound}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/get?key=pass://v/i/f", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestGetBackendErrorMaps502(t *testing.T) {
	srv := testServer(&fakeProvider{err: errors.New("boom")}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/get?key=pass://v/i/f", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
	// Error envelope must not echo the secret or backend detail.
	if strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("error body leaks backend detail: %s", rec.Body.String())
	}
}

// TestStatusCodeOfTable checks sentinel/wrapped errors map to HTTP codes
// (400/404/422/502 + 500 fallback), mirroring the apprise StatusCodeOf
// table shape.
func TestStatusCodeOfTable(t *testing.T) {
	wrapped := func(err error) error { return fmt.Errorf("layer: %w", err) }
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, http.StatusOK},
		{"bad request", errBadRequest, http.StatusBadRequest},
		{"missing key", errMissingKey, http.StatusBadRequest},
		{"missing remoteref", errMissingRemoteRefKey, http.StatusBadRequest},
		{"invalid key", fmt.Errorf("x: %w", provider.ErrInvalidKey), http.StatusBadRequest},
		{"not found", provider.ErrNotFound, http.StatusNotFound},
		{"wrapped not found", wrapped(provider.ErrNotFound), http.StatusNotFound},
		{"unprocessable", provider.ErrUnprocessable, http.StatusUnprocessableEntity},
		{"wrapped unprocessable", wrapped(provider.ErrUnprocessable), http.StatusUnprocessableEntity},
		{"upstream", provider.ErrUpstream, http.StatusBadGateway},
		{"wrapped upstream", wrapped(provider.ErrUpstream), http.StatusBadGateway},
		{"plain backend", errors.New("boom"), http.StatusBadGateway},
		{"push", provider.ErrPushUnimplemented, http.StatusNotImplemented},
		{"method", errMethodNotAllowed, http.StatusMethodNotAllowed},
		{"status coder", &statusErr{code: http.StatusTooManyRequests, msg: "m", err: errors.New("x")}, http.StatusTooManyRequests},
		{"plain unknown", errUnmappedTestOnly(), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusCodeOf(tc.err); got != tc.want {
				t.Errorf("statusCodeOf = %d, want %d", got, tc.want)
			}
		})
	}
}

// statusErr is a StatusCode()-carrying error mirroring the apprise
// StatusError shape (typed status, %w-unwrappable cause).
type statusErr struct {
	code int
	msg  string
	err  error
}

func (e *statusErr) Error() string   { return e.msg + ": " + e.err.Error() }
func (e *statusErr) Unwrap() error   { return e.err }
func (e *statusErr) StatusCode() int { return e.code }

// TestHandlerErrorTable drives the envelope end to end: sentinel/wrapped
// errors produce the mapped status with a lowercase, punctuation-free
// message and no backend detail.
func TestHandlerErrorTable(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantBody string
	}{
		{"not found", provider.ErrNotFound, http.StatusNotFound, "secret not found"},
		{"wrapped not found", fmt.Errorf("get: %w", provider.ErrNotFound), http.StatusNotFound, "secret not found"},
		{"invalid key", fmt.Errorf("x: %w", provider.ErrInvalidKey), http.StatusBadRequest, "invalid key: want pass://{vault}/{item}/{field}"},
		{"unprocessable", provider.ErrUnprocessable, http.StatusUnprocessableEntity, "unprocessable reference"},
		{"upstream", fmt.Errorf("resolve: %w", provider.ErrUpstream), http.StatusBadGateway, "failed to resolve secret"},
		{"backend detail", errors.New("execboom token=abc path=/etc/x"), http.StatusBadGateway, "failed to resolve secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := testServer(&fakeProvider{err: tc.err}).Handler()
			req := httptest.NewRequest(http.MethodGet, "/get?key=pass://v/i/f", nil)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			var body map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body["error"] != tc.wantBody {
				t.Errorf("error = %q, want %q", body["error"], tc.wantBody)
			}
			for _, leak := range []string{"execboom", "abc", "/etc/x"} {
				if strings.Contains(body["error"], leak) {
					t.Errorf("envelope leaks %q: %q", leak, body["error"])
				}
			}
		})
	}
}

func TestGetPostJSONBody(t *testing.T) {
	srv := testServer(&fakeProvider{value: "v"}).Handler()
	body := strings.NewReader(`{"remoteRef": {"key": "pass://vault/item/field"}}`)
	req := httptest.NewRequest(http.MethodPost, "/get", body)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestValidateHealthMetrics(t *testing.T) {
	srv := testServer(&fakeProvider{value: "x"}).Handler()
	for _, tc := range []struct{ method, path string }{
		{http.MethodHead, "/"},
		{http.MethodGet, "/"},
		{http.MethodGet, "/healthz"},
		{http.MethodGet, "/metrics"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s %s: status = %d, want 200", tc.method, tc.path, rec.Code)
		}
	}
}

// TestHealthzZeroDownstreamCalls pins the liveness contract: static JSON,
// no provider interaction, so kubelet probes never wedge on the backend.
func TestHealthzZeroDownstreamCalls(t *testing.T) {
	f := &fakeProvider{value: "x"}
	srv := testServer(f).Handler()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("healthz decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("healthz status = %q, want ok", body["status"])
	}
	if f.gets != 0 || f.readies != 0 {
		t.Errorf("healthz made %d gets + %d readies, want 0 downstream calls", f.gets, f.readies)
	}
}

// TestReadyzUpAndDown checks the readiness probe: 200 when reachable, 503
// with the failing key (and no backend detail) when stubbed down.
func TestReadyzUpAndDown(t *testing.T) {
	up := &fakeProvider{value: "x"}
	h := testServer(up).Handler()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("readyz up status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("readyz up decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("readyz up status = %q, want ok", body["status"])
	}
	if up.readies != 1 {
		t.Errorf("readyz up made %d probes, want 1", up.readies)
	}

	down := &fakeProvider{ready: errors.New("connection refused at /run/session token=abc")}
	h = testServer(down).Handler()
	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz down status = %d, want 503", rec.Code)
	}
	body = nil
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("readyz down decode: %v", err)
	}
	if body["status"] != "not_ready" || body["failing"] != "pass-cli" {
		t.Errorf("readyz down body = %v, want status/not_ready + failing", body)
	}
	for _, leak := range []string{"refused", "/run/session", "abc"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Errorf("readyz leaks %q: %s", leak, rec.Body.String())
		}
	}
}

// TestMetricsExposesRuntimeSeries checks /metrics carries the domain
// request/latency series plus go_*/process_* runtime series and the
// build_info gauge, with no unbounded (raw-path) labels.
func TestMetricsExposesRuntimeSeries(t *testing.T) {
	f := &fakeProvider{value: "x"}
	srv := testServer(f).Handler()
	// Generate one observation per route so all series exist.
	for _, path := range []string{"/get?key=pass://v/i/f", "/healthz", "/readyz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
	}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"eso_proton_pass_http_requests_total",
		"eso_proton_pass_http_request_duration_seconds_bucket",
		"go_goroutines",
		"process_cpu_seconds_total",
		`eso_proton_pass_build_info{version=`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
	// Route labels must be registered patterns, never raw secret paths.
	if strings.Contains(body, "pass://") {
		t.Errorf("metrics leak raw path in labels")
	}
}

func TestPushReturns501(t *testing.T) {
	srv := testServer(&fakeProvider{value: "x"}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/push", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "push is not implemented (pull-only provider)") {
		t.Errorf("push body should state pull-only: %s", rec.Body.String())
	}
}
