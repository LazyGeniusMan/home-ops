package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/external-dns/endpoint"

	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/netbird"
	nbprovider "github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/provider"
)

type fakeAPI struct {
	zones []netbird.Zone
}

func (f *fakeAPI) ListZones(context.Context) ([]netbird.Zone, error) { return f.zones, nil }
func (f *fakeAPI) ListRecords(context.Context, string) ([]netbird.Record, error) {
	return nil, nil
}
func (f *fakeAPI) CreateRecord(_ context.Context, _ string, rec netbird.CreateRecord) (*netbird.Record, error) {
	return &netbird.Record{Name: rec.Name, Type: rec.Type, Content: rec.Content, TTL: rec.TTL}, nil
}
func (f *fakeAPI) UpdateRecord(_ context.Context, _, id string, rec netbird.UpdateRecord) (*netbird.Record, error) {
	return &netbird.Record{ID: id, Name: rec.Name, Type: rec.Type, Content: rec.Content, TTL: rec.TTL}, nil
}
func (f *fakeAPI) DeleteRecord(context.Context, string, string) error { return nil }

func testServer() *Server {
	api := &fakeAPI{zones: []netbird.Zone{{
		ID: "z1", Domain: "example.com",
		Records: []netbird.Record{
			{ID: "r1", Name: "www.example.com", Type: "A", Content: "10.0.0.1", TTL: 300},
		},
	}}}
	p := nbprovider.New(api, []string{"example.com"}, 300)
	return New(p, logrus.NewEntry(logrus.New()), "127.0.0.1:0", "127.0.0.1:0")
}

func TestNegotiate(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.handleNegotiate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != MediaType {
		t.Errorf("content-type = %q, want %q", ct, MediaType)
	}
	var df endpoint.DomainFilter
	if err := json.NewDecoder(rec.Body).Decode(&df); err != nil {
		t.Fatalf("decode filter: %v", err)
	}
	if !df.Match("www.example.com") {
		t.Error("negotiated filter should match www.example.com")
	}
}

func TestRecordsHandler(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodGet, "/records", nil)
	rec := httptest.NewRecorder()
	s.handleRecords(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var eps []*endpoint.Endpoint
	if err := json.NewDecoder(rec.Body).Decode(&eps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(eps) != 1 || eps[0].DNSName != "www.example.com" || eps[0].RecordType != "A" {
		t.Errorf("unexpected endpoints: %+v", eps)
	}
}

func TestApplyChangesHandler(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodPost, "/records", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	s.handleApplyChanges(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
}

func TestApplyChangesHandlerBadJSON(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest(http.MethodPost, "/records", strings.NewReader(`{invalid`))
	rec := httptest.NewRecorder()
	s.handleApplyChanges(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdjustEndpointsHandler(t *testing.T) {
	s := testServer()
	body, _ := json.Marshal([]*endpoint.Endpoint{
		endpoint.NewEndpoint("txt.example.com", "TXT", "hello"),
		endpoint.NewEndpointWithTTL("WWW.EXAMPLE.COM.", "a", 0, "10.0.0.1"),
	})
	req := httptest.NewRequest(http.MethodPost, "/adjustendpoints", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleAdjustEndpoints(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var eps []*endpoint.Endpoint
	if err := json.NewDecoder(rec.Body).Decode(&eps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(eps) != 1 || eps[0].DNSName != "www.example.com" || eps[0].RecordTTL != 300 {
		t.Errorf("unexpected adjusted endpoints: %+v", eps)
	}
}

func TestOpsHandler(t *testing.T) {
	s := testServer()
	h := s.opsHandler()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Errorf("healthz = %d %q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "external_dns_netbird") {
		t.Error("metrics missing external_dns_netbird series")
	}
}
