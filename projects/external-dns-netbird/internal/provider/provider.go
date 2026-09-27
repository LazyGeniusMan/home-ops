// Package provider implements the ExternalDNS provider backed by NetBird
// DNS Custom Zones: one endpoint (DNSName + type) maps to one NetBird entry
// per target; identity tracked by NetBird record ID.
package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"

	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/netbird"
)

// SupportedTypes are the DNS record types accepted by the NetBird records API.
var SupportedTypes = map[string]bool{"A": true, "AAAA": true, "CNAME": true}

// ErrNoMatchingZone marks permanent zone-resolution failures (server
// maps to HTTP 422, no retry).
var ErrNoMatchingZone = errors.New("netbird: no matching zone")

// API is the NetBird client subset used by the Provider (mockable).
type API interface {
	ListZones(ctx context.Context) ([]netbird.Zone, error)
	CreateZone(ctx context.Context, req netbird.CreateZoneRequest) (*netbird.Zone, error)
	ListRecords(ctx context.Context, zoneID string) ([]netbird.Record, error)
	CreateRecord(ctx context.Context, zoneID string, rec netbird.CreateRecord) (*netbird.Record, error)
	UpdateRecord(ctx context.Context, zoneID, recordID string, rec netbird.UpdateRecord) (*netbird.Record, error)
	DeleteRecord(ctx context.Context, zoneID, recordID string) error
}

// Provider is an ExternalDNS provider backed by NetBird DNS Custom Zones.
type Provider struct {
	provider.BaseProvider
	api        API
	filter     *endpoint.DomainFilter
	defaultTTL int64
	autoCreate bool
}

var _ provider.Provider = (*Provider)(nil)

// New builds a Provider; empty domainFilters serves all zones.
// autoCreate gates zone auto-creation (NETBIRD_AUTO_CREATE): when false,
// names without a matching zone are a permanent error instead of a
// CreateZone call.
func New(api API, domainFilters []string, defaultTTL int64, autoCreate ...bool) *Provider {
	create := true
	if len(autoCreate) > 0 {
		create = autoCreate[0]
	}
	return &Provider{
		api:        api,
		filter:     endpoint.NewDomainFilter(domainFilters),
		defaultTTL: defaultTTL,
		autoCreate: create,
	}
}

// GetDomainFilter returns the configured domain filter for negotiation.
func (p *Provider) GetDomainFilter() endpoint.DomainFilterInterface {
	return p.filter
}

// Ping is the /readyz probe: a single cheap authenticated read (ListZones)
// with results discarded. Only API reachability matters, so unlike Records
// it never fans out to N×ListRecords per zone — one SaaS call per probe,
// safe under the kubelet's 10s probe period.
func (p *Provider) Ping(ctx context.Context) error {
	if _, err := p.api.ListZones(ctx); err != nil {
		return softOrHard("list zones", err)
	}
	return nil
}

// recordRef identifies a NetBird record entry backing part of an endpoint.
type recordRef struct {
	zoneID   string
	recordID string
}

// Records lists one endpoint per (DNSName, RecordType) pair, grouping
// same-name/type entries and merging their contents into targets.
func (p *Provider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	zones, err := p.api.ListZones(ctx)
	if err != nil {
		return nil, softOrHard("list zones", err)
	}
	type key struct {
		dnsName string
		recType string
	}
	grouped := map[key][]netbird.Record{}
	ttls := map[key]int64{}
	order := []key{}
	for _, z := range zones {
		if !p.filter.Match(z.Domain) {
			continue
		}
		records := z.Records
		if records == nil {
			records, err = p.api.ListRecords(ctx, z.ID)
			if err != nil {
				return nil, softOrHard("list records", err)
			}
		}
		for _, r := range records {
			k := key{dnsName: strings.ToLower(strings.TrimSuffix(r.Name, ".")), recType: strings.ToUpper(r.Type)}
			if _, ok := grouped[k]; !ok {
				order = append(order, k)
				ttls[k] = r.TTL
			}
			grouped[k] = append(grouped[k], r)
		}
	}
	endpoints := make([]*endpoint.Endpoint, 0, len(grouped))
	for _, k := range order {
		targets := make([]string, 0, len(grouped[k]))
		for _, r := range grouped[k] {
			targets = append(targets, r.Content)
		}
		endpoints = append(endpoints, endpoint.NewEndpointWithTTL(k.dnsName, k.recType, endpoint.TTL(ttls[k]), targets...))
	}
	return endpoints, nil
}

// AdjustEndpoints normalizes candidates to match Records: drops
// unsupported types, normalizes case, fills missing TTLs so the planner
// sees no spurious diffs.
func (p *Provider) AdjustEndpoints(endpoints []*endpoint.Endpoint) ([]*endpoint.Endpoint, error) {
	adjusted := make([]*endpoint.Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		recType := strings.ToUpper(ep.RecordType)
		if !SupportedTypes[recType] {
			continue
		}
		out := *ep
		out.Targets = append(endpoint.Targets(nil), ep.Targets...)
		out.DNSName = strings.ToLower(strings.TrimSuffix(out.DNSName, "."))
		out.RecordType = recType
		if out.RecordTTL == 0 {
			out.RecordTTL = endpoint.TTL(p.defaultTTL)
		}
		adjusted = append(adjusted, &out)
	}
	return adjusted, nil
}

// ApplyChanges creates, updates, and deletes record entries to match the
// plan (update falls back to delete + create when unresolvable).
func (p *Provider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	index, err := p.buildIndex(ctx)
	if err != nil {
		return err
	}
	for _, ep := range changes.Create {
		if err := p.create(ctx, ep); err != nil {
			return err
		}
	}
	for i, old := range changes.UpdateOld {
		if i >= len(changes.UpdateNew) {
			break
		}
		if err := p.update(ctx, index, old, changes.UpdateNew[i]); err != nil {
			return err
		}
	}
	for _, ep := range changes.Delete {
		if err := p.delete(ctx, index, ep); err != nil {
			return err
		}
	}
	return nil
}

// index maps (dnsname, type, target) to the backing NetBird record entries.
type index map[string][]recordRef

func indexKey(dnsName, recType, target string) string {
	return strings.ToLower(strings.TrimSuffix(dnsName, ".")) + "\x00" + strings.ToUpper(recType) + "\x00" + target
}

func (p *Provider) buildIndex(ctx context.Context) (index, error) {
	zones, err := p.api.ListZones(ctx)
	if err != nil {
		return nil, softOrHard("list zones", err)
	}
	idx := index{}
	for _, z := range zones {
		if !p.filter.Match(z.Domain) {
			continue
		}
		records := z.Records
		if records == nil {
			records, err = p.api.ListRecords(ctx, z.ID)
			if err != nil {
				return nil, softOrHard("list records", err)
			}
		}
		for _, r := range records {
			k := indexKey(r.Name, r.Type, r.Content)
			idx[k] = append(idx[k], recordRef{zoneID: z.ID, recordID: r.ID})
		}
	}
	return idx, nil
}

// zoneForName resolves the longest-suffix zone, auto-creating the
// DOMAIN_FILTER candidate when autoCreate is true. Misses outside the
// filter are permanent; API failures map soft (transient) or hard
// (permanent 4xx) via softOrHard. NetBird never deletes DNS entries on its
// own: a name with no matching zone is a permanent error (or an explicit
// auto-create), so stale records are intentionally retained rather than
// garbage-collected.
func (p *Provider) zoneForName(ctx context.Context, dnsName string) (string, error) {
	name := strings.ToLower(strings.TrimSuffix(dnsName, "."))
	zones, err := p.api.ListZones(ctx)
	if err != nil {
		return "", softOrHard("list zones", err)
	}
	bestID := ""
	bestLen := -1
	for _, z := range zones {
		if !p.filter.Match(z.Domain) {
			continue
		}
		domain := strings.ToLower(strings.TrimSuffix(z.Domain, "."))
		if name == domain || strings.HasSuffix(name, "."+domain) {
			if len(domain) > bestLen {
				bestLen = len(domain)
				bestID = z.ID
			}
		}
	}
	if bestID != "" {
		return bestID, nil
	}
	candidate := longestFilterSuffix(p.filter, name)
	if candidate == "" {
		return "", fmt.Errorf("%w for %q", ErrNoMatchingZone, dnsName)
	}
	// Re-check before creating; a zone outside the filter is a permanent miss.
	for _, z := range zones {
		if strings.EqualFold(strings.TrimSuffix(z.Domain, "."), candidate) {
			if !p.filter.Match(z.Domain) {
				return "", fmt.Errorf("%w for %q (zone %q outside domain filter)", ErrNoMatchingZone, dnsName, z.Domain)
			}
			return z.ID, nil
		}
	}
	if !p.filter.Match(candidate) {
		return "", fmt.Errorf("%w for %q (candidate zone %q outside domain filter)", ErrNoMatchingZone, dnsName, candidate)
	}
	// Auto-creation is opt-out via NETBIRD_AUTO_CREATE=false: without it a
	// missing zone is a permanent error (no surprise zones from typos).
	if !p.autoCreate {
		return "", fmt.Errorf("%w for %q (zone auto-creation disabled)", ErrNoMatchingZone, dnsName)
	}
	created, err := p.api.CreateZone(ctx, netbird.CreateZoneRequest{
		Name:               candidate,
		Domain:             candidate,
		EnableSearchDomain: false,
		DistributionGroups: []string{},
	})
	if err != nil {
		return "", softOrHard("create zone", err)
	}
	if created == nil || created.ID == "" {
		return "", softError("create zone", errEmptyZoneResponse)
	}
	return created.ID, nil
}

// errEmptyZoneResponse marks a zone-create call that returned no zone ID.
var errEmptyZoneResponse = errors.New("netbird: empty zone response")

// longestFilterSuffix picks the auto-create zone domain for dnsName: the
// longest matching filter entry, else the immediate parent domain ("" for
// single-label names).
func longestFilterSuffix(filter *endpoint.DomainFilter, dnsName string) string {
	best := ""
	if filter != nil {
		for _, f := range filter.Filters {
			domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(f), "."))
			if domain == "" {
				continue
			}
			if dnsName == domain || strings.HasSuffix(dnsName, "."+domain) {
				if len(domain) > len(best) {
					best = domain
				}
			}
		}
	}
	if best != "" {
		return best
	}
	// No filter configured (or no filter entry is a suffix): derive the
	// immediate parent domain so single-record hostnames self-heal.
	if i := strings.Index(dnsName, "."); i > 0 && i < len(dnsName)-1 {
		return dnsName[i+1:]
	}
	return ""
}

func (p *Provider) create(ctx context.Context, ep *endpoint.Endpoint) error {
	zoneID, err := p.zoneForName(ctx, ep.DNSName)
	if err != nil {
		return err
	}
	ttl := int64(ep.RecordTTL)
	if ttl == 0 {
		ttl = p.defaultTTL
	}
	for _, target := range ep.Targets {
		_, err := p.api.CreateRecord(ctx, zoneID, netbird.CreateRecord{
			Name:    strings.ToLower(strings.TrimSuffix(ep.DNSName, ".")),
			Type:    strings.ToUpper(ep.RecordType),
			Content: target,
			TTL:     ttl,
		})
		if err != nil {
			return softOrHard("create record", err)
		}
	}
	return nil
}

func (p *Provider) update(ctx context.Context, idx index, old, new *endpoint.Endpoint) error {
	oldTargets := map[string]bool{}
	for _, t := range old.Targets {
		oldTargets[t] = true
	}
	// NetBird stores one entry per target, so one endpoint maps to N
	// refs; delete/update paths must iterate ALL refs for a key, not just
	// refs[0]. A key with refs in more than one zone is a split-brain the
	// index cannot resolve safely, so it fails instead of half-applying.
	mismatch := func(refs []recordRef) error {
		zones := map[string]struct{}{}
		for _, ref := range refs {
			zones[ref.zoneID] = struct{}{}
		}
		if len(zones) > 1 {
			return fmt.Errorf("netbird: update record failed: %q has entries in %d zones", old.DNSName, len(zones))
		}
		return nil
	}
	renamed := !strings.EqualFold(old.DNSName, new.DNSName) || !strings.EqualFold(old.RecordType, new.RecordType)
	// Delete entries that disappeared (or every old entry on rename: the
	// name/type key changes, so re-resolve under the old key).
	for _, t := range old.Targets {
		stillWanted := !renamed
		if stillWanted {
			stillWanted = false
			for _, nt := range new.Targets {
				if nt == t {
					stillWanted = true
					break
				}
			}
		}
		if stillWanted {
			continue
		}
		refs := idx[indexKey(old.DNSName, old.RecordType, t)]
		if len(refs) == 0 {
			continue
		}
		if err := mismatch(refs); err != nil {
			return err
		}
		for _, ref := range refs {
			if err := p.api.DeleteRecord(ctx, ref.zoneID, ref.recordID); err != nil {
				return softOrHard("delete record", err)
			}
		}
	}
	// Create new entries and refresh TTL/content of kept ones via update.
	ttl := int64(new.RecordTTL)
	if ttl == 0 {
		ttl = p.defaultTTL
	}
	for _, t := range new.Targets {
		kept := oldTargets[t] && !renamed
		if !kept {
			if err := p.create(ctx, endpoint.NewEndpointWithTTL(new.DNSName, new.RecordType, endpoint.TTL(ttl), t)); err != nil {
				return err
			}
			continue
		}
		refs := idx[indexKey(old.DNSName, old.RecordType, t)]
		if len(refs) == 0 {
			continue
		}
		if err := mismatch(refs); err != nil {
			return err
		}
		for _, ref := range refs {
			_, err := p.api.UpdateRecord(ctx, ref.zoneID, ref.recordID, netbird.UpdateRecord{
				Name:    strings.ToLower(strings.TrimSuffix(new.DNSName, ".")),
				Type:    strings.ToUpper(new.RecordType),
				Content: t,
				TTL:     ttl,
			})
			if err != nil {
				return softOrHard("update record", err)
			}
		}
	}
	return nil
}

func (p *Provider) delete(ctx context.Context, idx index, ep *endpoint.Endpoint) error {
	for _, t := range ep.Targets {
		for _, ref := range idx[indexKey(ep.DNSName, ep.RecordType, t)] {
			if err := p.api.DeleteRecord(ctx, ref.zoneID, ref.recordID); err != nil {
				return softOrHard("delete record", err)
			}
		}
	}
	return nil
}

// opError names a provider operation for soft-error messages.
type opError struct {
	op  string
	err error
}

func (e *opError) Error() string { return "netbird: " + e.op + " failed" }

func (e *opError) Unwrap() error { return e.err }

// softError wraps a NetBird failure for external-dns interop as a
// transient error. The message carries only the operation name (lowercase,
// no identifiers or backend detail — see internal/server/errors.go); the
// cause stays reachable via errors.As/Is for the Retryable check.
func softError(op string, err error) error {
	return provider.NewSoftError(&opError{op: op, err: err})
}

// apiStatus reports the NetBird API status code when err carries one.
func apiStatus(err error) (int, bool) {
	var apiErr *netbird.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode, true
	}
	return 0, false
}

// softOrHard maps a NetBird failure to soft (transient: transport errors,
// 429/5xx) or hard (permanent: other 4xx) so ExternalDNS retries only what
// can succeed. Unknown shapes are soft (retry-safe default).
func softOrHard(op string, err error) error {
	if code, ok := apiStatus(err); ok {
		if code == http.StatusTooManyRequests || (code >= 500 && code <= 599) {
			return softError(op, err)
		}
		return fmt.Errorf("netbird: %s failed", op)
	}
	return softError(op, err)
}
