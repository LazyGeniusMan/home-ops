// Package provider implements the ExternalDNS provider.Provider interface
// backed by NetBird DNS Custom Zones.
//
// Mapping:
//   - each NetBird zone (filtered by domain) contributes one record per
//     DNS record entry it holds;
//   - one ExternalDNS endpoint (DNSName + RecordType) maps to one or more
//     NetBird record entries (one entry per target), because the NetBird
//     records API stores a single content value per entry;
//   - record identity is tracked by NetBird record ID; lookups resolve
//     (zoneID, recordID) pairs before update/delete.
package provider

import (
	"context"
	"fmt"
	"strings"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"

	"github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/netbird"
)

// SupportedTypes are the DNS record types accepted by the NetBird records API.
var SupportedTypes = map[string]bool{"A": true, "AAAA": true, "CNAME": true}

// API is the subset of the NetBird client used by the Provider (mockable).
type API interface {
	ListZones(ctx context.Context) ([]netbird.Zone, error)
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
}

var _ provider.Provider = (*Provider)(nil)

// New builds a Provider. domainFilters restricts the served zones by domain
// suffix; an empty list serves all zones.
func New(api API, domainFilters []string, defaultTTL int64) *Provider {
	return &Provider{
		api:        api,
		filter:     endpoint.NewDomainFilter(domainFilters),
		defaultTTL: defaultTTL,
	}
}

// GetDomainFilter returns the configured domain filter for negotiation.
func (p *Provider) GetDomainFilter() endpoint.DomainFilterInterface {
	return p.filter
}

// recordRef identifies a NetBird record entry backing part of an endpoint.
type recordRef struct {
	zoneID   string
	recordID string
}

// Records lists one endpoint per (DNSName, RecordType) pair found in the
// filtered NetBird zones, grouping entries with the same name and type and
// merging their contents into targets.
func (p *Provider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	zones, err := p.api.ListZones(ctx)
	if err != nil {
		return nil, softErrorf("list zones: %v", err)
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
				return nil, softErrorf("list records for zone %q: %v", z.ID, err)
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

// AdjustEndpoints normalizes candidate endpoints for NetBird parity with
// Records: it drops unsupported record types, lower-cases names, upper-cases
// types, and fills missing TTLs with the configured default so the planner
// does not produce spurious updates.
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

// ApplyChanges creates, updates, and deletes NetBird record entries to match
// the planned changes. Updates are applied as delete + create when the
// NetBird update call cannot resolve the backing record.
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
		return nil, softErrorf("list zones: %v", err)
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
				return nil, softErrorf("list records for zone %q: %v", z.ID, err)
			}
		}
		for _, r := range records {
			k := indexKey(r.Name, r.Type, r.Content)
			idx[k] = append(idx[k], recordRef{zoneID: z.ID, recordID: r.ID})
		}
	}
	return idx, nil
}

func (p *Provider) zoneForName(ctx context.Context, dnsName string) (string, error) {
	name := strings.ToLower(strings.TrimSuffix(dnsName, "."))
	zones, err := p.api.ListZones(ctx)
	if err != nil {
		return "", softErrorf("list zones: %v", err)
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
	if bestID == "" {
		return "", fmt.Errorf("netbird: no matching zone for %q", dnsName)
	}
	return bestID, nil
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
			return softErrorf("create record %s %s: %v", ep.DNSName, target, err)
		}
	}
	return nil
}

func (p *Provider) update(ctx context.Context, idx index, old, new *endpoint.Endpoint) error {
	oldTargets := map[string]bool{}
	for _, t := range old.Targets {
		oldTargets[t] = true
	}
	// Delete entries that disappeared.
	for _, t := range old.Targets {
		stillWanted := false
		for _, nt := range new.Targets {
			if nt == t && strings.EqualFold(old.DNSName, new.DNSName) && strings.EqualFold(old.RecordType, new.RecordType) {
				stillWanted = true
				break
			}
		}
		if stillWanted {
			continue
		}
		refs := idx[indexKey(old.DNSName, old.RecordType, t)]
		if len(refs) == 0 {
			continue
		}
		if err := p.api.DeleteRecord(ctx, refs[0].zoneID, refs[0].recordID); err != nil {
			return softErrorf("delete record %s %s: %v", old.DNSName, t, err)
		}
	}
	// Create new entries and refresh TTL/content of kept ones via update.
	ttl := int64(new.RecordTTL)
	if ttl == 0 {
		ttl = p.defaultTTL
	}
	for _, t := range new.Targets {
		kept := oldTargets[t] && strings.EqualFold(old.DNSName, new.DNSName) && strings.EqualFold(old.RecordType, new.RecordType)
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
		_, err := p.api.UpdateRecord(ctx, refs[0].zoneID, refs[0].recordID, netbird.UpdateRecord{
			Name:    strings.ToLower(strings.TrimSuffix(new.DNSName, ".")),
			Type:    strings.ToUpper(new.RecordType),
			Content: t,
			TTL:     ttl,
		})
		if err != nil {
			return softErrorf("update record %s %s: %v", new.DNSName, t, err)
		}
	}
	return nil
}

func (p *Provider) delete(ctx context.Context, idx index, ep *endpoint.Endpoint) error {
	for _, t := range ep.Targets {
		for _, ref := range idx[indexKey(ep.DNSName, ep.RecordType, t)] {
			if err := p.api.DeleteRecord(ctx, ref.zoneID, ref.recordID); err != nil {
				return softErrorf("delete record %s %s: %v", ep.DNSName, t, err)
			}
		}
	}
	return nil
}

func softErrorf(format string, a ...any) error {
	return provider.NewSoftError(fmt.Errorf(format, a...))
}
