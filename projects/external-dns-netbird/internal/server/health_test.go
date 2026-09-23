package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/netbird"
	nbprovider "github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/provider"
	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/version"
)

// countingAPI wraps fakeAPI and counts downstream calls so the healthz
// zero-call assertion is meaningful.
type countingAPI struct {
	*fakeAPI
	calls int
}

func (c *countingAPI) ListZones(ctx context.Context) ([]netbird.Zone, error) {
	c.calls++
	return c.fakeAPI.ListZones(ctx)
}

func (c *countingAPI) ListRecords(ctx context.Context, zoneID string) ([]netbird.Record, error) {
	c.calls++
	return c.fakeAPI.ListRecords(ctx, zoneID)
}

func TestHealthzZeroDownstreamCalls(t *testing.T) {
	api := &countingAPI{fakeAPI: &fakeAPI{zones: []netbird.Zone{{
		ID: "z1", Domain: "example.com",
		Records: []netbird.Record{
			{ID: "r1", Name: "www.example.com", Type: "A", Content: "10.0.0.1", TTL: 300},
		},
	}}}}
	p := nbprovider.New(api, []string{"example.com"}, 300)
	s := New(p, testLogger(), "127.0.0.1:0", "127.0.0.1:0")
	h := s.opsHandler()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("healthz content-type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("healthz decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("healthz status = %q, want ok", body["status"])
	}
	if api.calls != 0 {
		t.Errorf("healthz made %d downstream calls, want 0", api.calls)
	}
}

// errAPI fails every call to stub NetBird API downtime.
type errAPI struct{ fakeAPI }

func (e *errAPI) ListZones(context.Context) ([]netbird.Zone, error) {
	return nil, errors.New("netbird: connection refused")
}

func TestReadyzUpAndDown(t *testing.T) {
	up := testServer()
	h := up.opsHandler()
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

	down := New(
		nbprovider.New(&errAPI{}, []string{"example.com"}, 300),
		testLogger(), "127.0.0.1:0", "127.0.0.1:0",
	)
	hd := down.opsHandler()
	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec = httptest.NewRecorder()
	hd.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz down status = %d, want 503", rec.Code)
	}
	var downBody map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&downBody); err != nil {
		t.Fatalf("readyz down decode: %v", err)
	}
	if downBody["status"] != "not_ready" {
		t.Errorf("readyz down status = %q, want not_ready", downBody["status"])
	}
	if downBody["failing"] == "" {
		t.Error("readyz down response missing failing key")
	}
	if strings.Contains(rec.Body.String(), "refused") {
		t.Error("readyz down response leaks backend detail")
	}
}

func TestVersionEndpoint(t *testing.T) {
	s := testServer()
	h := s.opsHandler()
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("version status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("version decode: %v", err)
	}
	if body["version"] != version.Version {
		t.Errorf("version = %q, want %q", body["version"], version.Version)
	}
	if version.Version == "" {
		t.Error("version must not be empty (want dev for local builds)")
	}
}
