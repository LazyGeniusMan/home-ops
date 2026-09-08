package provider

import (
	"context"
	"testing"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"

	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/netbird"
)

// fakeAPI is an in-memory netbird.API.
type fakeAPI struct {
	zones   []netbird.Zone
	records map[string][]netbird.Record // zoneID -> records
	created []netbird.CreateRecord
	updated []netbird.UpdateRecord
	deleted []string // recordIDs
	seq     int
}

func (f *fakeAPI) ListZones(_ context.Context) ([]netbird.Zone, error) {
	return f.zones, nil
}

func (f *fakeAPI) ListRecords(_ context.Context, zoneID string) ([]netbird.Record, error) {
	return f.records[zoneID], nil
}

func (f *fakeAPI) CreateRecord(_ context.Context, zoneID string, rec netbird.CreateRecord) (*netbird.Record, error) {
	f.seq++
	created := &netbird.Record{ID: "new-id", Name: rec.Name, Type: rec.Type, Content: rec.Content, TTL: rec.TTL}
	f.created = append(f.created, rec)
	f.records[zoneID] = append(f.records[zoneID], *created)
	return created, nil
}

func (f *fakeAPI) UpdateRecord(_ context.Context, _, recordID string, rec netbird.UpdateRecord) (*netbird.Record, error) {
	f.updated = append(f.updated, rec)
	return &netbird.Record{ID: recordID, Name: rec.Name, Type: rec.Type, Content: rec.Content, TTL: rec.TTL}, nil
}

func (f *fakeAPI) DeleteRecord(_ context.Context, _, recordID string) error {
	f.deleted = append(f.deleted, recordID)
	return nil
}

func testFixture() *fakeAPI {
	return &fakeAPI{
		zones: []netbird.Zone{
			{ID: "zone-1", Name: "Office", Domain: "example.com", Enabled: true},
		},
		records: map[string][]netbird.Record{
			"zone-1": {
				{ID: "r1", Name: "www.example.com", Type: "A", Content: "192.168.1.1", TTL: 300},
				{ID: "r2", Name: "www.example.com", Type: "A", Content: "192.168.1.2", TTL: 300},
				{ID: "r3", Name: "api.example.com", Type: "CNAME", Content: "www.example.com", TTL: 60},
			},
		},
	}
}

func TestRecordsGroupsEntries(t *testing.T) {
	p := New(testFixture(), []string{"example.com"}, 300)
	eps, err := p.Records(context.Background())
	if err != nil {
		t.Fatalf("Records: %v", err)
	}
	if len(eps) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(eps))
	}
	byName := map[string]*endpoint.Endpoint{}
	for _, ep := range eps {
		byName[ep.DNSName] = ep
	}
	www := byName["www.example.com"]
	if www == nil {
		t.Fatalf("missing www.example.com endpoint: %v", eps)
	}
	if www.RecordType != "A" || len(www.Targets) != 2 {
		t.Errorf("unexpected www endpoint: %+v", www)
	}
	if www.RecordTTL != 300 {
		t.Errorf("unexpected TTL: %d", www.RecordTTL)
	}
	api := byName["api.example.com"]
	if api == nil || api.RecordType != "CNAME" || len(api.Targets) != 1 {
		t.Errorf("unexpected api endpoint: %+v", api)
	}
}

func TestRecordsRespectsFilter(t *testing.T) {
	p := New(testFixture(), []string{"other.net"}, 300)
	eps, err := p.Records(context.Background())
	if err != nil {
		t.Fatalf("Records: %v", err)
	}
	if len(eps) != 0 {
		t.Errorf("expected no endpoints for unmatched filter, got %v", eps)
	}
}

func TestAdjustEndpointsParity(t *testing.T) {
	p := New(testFixture(), nil, 300)
	in := []*endpoint.Endpoint{
		endpoint.NewEndpointWithTTL("WWW.EXAMPLE.COM.", "a", 0, "192.168.1.1"),
		endpoint.NewEndpoint("txt.example.com", "TXT", "hello"),
		endpoint.NewEndpointWithTTL("api.example.com", "CNAME", 60, "www.example.com"),
	}
	out, err := p.AdjustEndpoints(in)
	if err != nil {
		t.Fatalf("AdjustEndpoints: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 endpoints (TXT dropped), got %d", len(out))
	}
	if out[0].DNSName != "www.example.com" || out[0].RecordType != "A" {
		t.Errorf("not normalized: %+v", out[0])
	}
	if out[0].RecordTTL != 300 {
		t.Errorf("default TTL not applied: %d", out[0].RecordTTL)
	}
	if out[1].RecordTTL != 60 {
		t.Errorf("explicit TTL overwritten: %d", out[1].RecordTTL)
	}
}

func TestApplyChangesCreateDelete(t *testing.T) {
	f := testFixture()
	p := New(f, []string{"example.com"}, 300)
	changes := &plan.Changes{
		Create: []*endpoint.Endpoint{
			endpoint.NewEndpointWithTTL("new.example.com", "A", 300, "10.0.0.1"),
		},
		Delete: []*endpoint.Endpoint{
			endpoint.NewEndpointWithTTL("api.example.com", "CNAME", 60, "www.example.com"),
		},
	}
	if err := p.ApplyChanges(context.Background(), changes); err != nil {
		t.Fatalf("ApplyChanges: %v", err)
	}
	if len(f.created) != 1 || f.created[0].Name != "new.example.com" {
		t.Errorf("unexpected creates: %+v", f.created)
	}
	if len(f.deleted) != 1 || f.deleted[0] != "r3" {
		t.Errorf("unexpected deletes: %+v", f.deleted)
	}
}

func TestApplyChangesUpdate(t *testing.T) {
	f := testFixture()
	p := New(f, []string{"example.com"}, 300)
	changes := &plan.Changes{
		UpdateOld: []*endpoint.Endpoint{
			endpoint.NewEndpointWithTTL("www.example.com", "A", 300, "192.168.1.1", "192.168.1.2"),
		},
		UpdateNew: []*endpoint.Endpoint{
			endpoint.NewEndpointWithTTL("www.example.com", "A", 600, "192.168.1.1", "192.168.1.3"),
		},
	}
	if err := p.ApplyChanges(context.Background(), changes); err != nil {
		t.Fatalf("ApplyChanges: %v", err)
	}
	if len(f.deleted) != 1 || f.deleted[0] != "r2" {
		t.Errorf("expected r2 deleted, got %+v", f.deleted)
	}
	if len(f.created) != 1 || f.created[0].Content != "192.168.1.3" {
		t.Errorf("expected 192.168.1.3 created, got %+v", f.created)
	}
	if len(f.updated) != 1 || f.updated[0].TTL != 600 {
		t.Errorf("expected TTL refresh update, got %+v", f.updated)
	}
}

func TestGetDomainFilter(t *testing.T) {
	p := New(testFixture(), []string{"example.com"}, 300)
	if !p.GetDomainFilter().Match("www.example.com") {
		t.Error("filter should match www.example.com")
	}
	if p.GetDomainFilter().Match("other.net") {
		t.Error("filter should not match other.net")
	}
}
