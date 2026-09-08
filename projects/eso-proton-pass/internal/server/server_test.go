package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/LazyGeniusMan/home-ops/eso-proton-pass/internal/provider"
)

type fakeProvider struct {
	value string
	err   error
	last  string
}

func (f *fakeProvider) GetSecret(_ *http.Request, key string) (string, error) {
	f.last = key
	if f.err != nil {
		return "", f.err
	}
	// Mirror provider semantics for parity: validate then return.
	if _, _, _, verr := provider.ValidateKey(key); verr != nil {
		return "", verr
	}
	return f.value, nil
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

func TestPushReturns501TODO(t *testing.T) {
	srv := testServer(&fakeProvider{value: "x"}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/push", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "TODO") {
		t.Errorf("push body should mention TODO: %s", rec.Body.String())
	}
}
