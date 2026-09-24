package attach

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// InternalToken is the reserved SSRF deny-list token. When present in the
// reject list it resolves each attachment host and blocks loopback,
// private, link-local, reserved, unspecified, multicast, and CGN
// addresses — including ones reached via DNS or alternate IP encodings,
// not just literal matches. Opt-in, never part of the default list.
const InternalToken = "internal"

// resolveTimeout bounds a single attachment-host DNS resolution, mirroring
// Python's _RESOLVE_TIMEOUT_SEC.
const resolveTimeout = 5 * time.Second

// resolveHost resolves host to IP addresses. It is a variable so tests can
// stub DNS without network access.
var resolveHost = func(host string) ([]netip.Addr, error) {
	if addr, err := parseIPLiteral(host); err == nil {
		return []netip.Addr{addr}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), resolveTimeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		if addr, err := netip.ParseAddr(ip.IP.String()); err == nil {
			out = append(out, addr)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("attach: no usable addresses for %q", host)
	}
	return out, nil
}

// cgnSharedV4 is 100.64.0.0/10 (RFC 6598 carrier-grade NAT shared space),
// which netip.Addr.IsPrivate does not cover.
var cgnSharedV4 = netip.MustParsePrefix("100.64.0.0/10")

// reservedNets mirrors Python ipaddress is_reserved for the common
// documentation/benchmark/relay ranges beyond the
// IsPrivate/IsLoopback/link-local/multicast/unspecified predicates.
var reservedNets = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("255.255.255.255/32"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("fec0::/10"),
}

// isBlockedAddress reports whether addr must never be reachable from an
// attachment fetch: loopback, private, link-local, reserved, unspecified,
// multicast, or CGN shared space. IPv4-mapped IPv6 addresses are classified
// by their embedded IPv4 address.
func isBlockedAddress(addr netip.Addr) bool {
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	if addr.IsPrivate() ||
		addr.IsLoopback() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return true
	}
	if addr.Is4() && cgnSharedV4.Contains(addr) {
		return true
	}
	for _, p := range reservedNets {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// parseIPLiteral parses a literal IP, including bracketed IPv6 ("[::1]")
// and inet_aton-style alternate IPv4 encodings (decimal "2130706433",
// octal "0177.0.0.1", hex "0x7f.0.0.1", short forms "127.1") that resolvers
// accept but netip.ParseAddr rejects.
func parseIPLiteral(host string) (netip.Addr, error) {
	literal := host
	if strings.HasPrefix(literal, "[") && strings.HasSuffix(literal, "]") {
		literal = literal[1 : len(literal)-1]
	}
	if addr, err := netip.ParseAddr(literal); err == nil {
		return addr, nil
	}
	if addr, ok := parseIPv4Alt(literal); ok {
		return addr, nil
	}
	return netip.Addr{}, fmt.Errorf("attach: not an IP literal: %q", host)
}

// parseIPv4Alt parses inet_aton-style IPv4 forms: 1-4 dot-separated parts,
// each decimal, octal (leading 0), or hex (leading 0x), with the last part
// filling the remaining bytes.
func parseIPv4Alt(s string) (netip.Addr, bool) {
	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 4 {
		return netip.Addr{}, false
	}
	nums := make([]uint64, len(parts))
	for i, p := range parts {
		if p == "" {
			return netip.Addr{}, false
		}
		base := 10
		digits := p
		if strings.HasPrefix(p, "0x") || strings.HasPrefix(p, "0X") {
			base, digits = 16, p[2:]
		} else if len(p) > 1 && strings.HasPrefix(p, "0") {
			base, digits = 8, p[1:]
		}
		if digits == "" {
			return netip.Addr{}, false
		}
		v, err := strconv.ParseUint(digits, base, 64)
		if err != nil {
			return netip.Addr{}, false
		}
		nums[i] = v
	}
	var b [4]byte
	switch len(nums) {
	case 1:
		if nums[0] > 0xffffffff {
			return netip.Addr{}, false
		}
		b[0], b[1], b[2], b[3] = byte(nums[0]>>24), byte(nums[0]>>16), byte(nums[0]>>8), byte(nums[0])
	case 2:
		if nums[0] > 0xff || nums[1] > 0xffffff {
			return netip.Addr{}, false
		}
		b[0], b[1], b[2], b[3] = byte(nums[0]), byte(nums[1]>>16), byte(nums[1]>>8), byte(nums[1])
	case 3:
		if nums[0] > 0xff || nums[1] > 0xff || nums[2] > 0xffff {
			return netip.Addr{}, false
		}
		b[0], b[1], b[2], b[3] = byte(nums[0]), byte(nums[1]), byte(nums[2]>>8), byte(nums[2])
	default:
		for i, v := range nums {
			if v > 0xff {
				return netip.Addr{}, false
			}
			b[i] = byte(v)
		}
	}
	return netip.AddrFrom4(b), true
}

// isInternalTarget resolves host and reports whether any resulting address
// is blocked. A host that cannot be resolved (including on timeout) is
// treated as internal/blocked since a destination that cannot be
// classified cannot be proven safe.
func isInternalTarget(host string) bool {
	addrs, err := resolveHost(host)
	if err != nil || len(addrs) == 0 {
		return true
	}
	for _, addr := range addrs {
		if isBlockedAddress(addr) {
			return true
		}
	}
	return false
}

// ruleKind classifies a compiled allow/reject rule.
type ruleKind int

const (
	// ruleInternal resolves and IP-classifies the host at match time.
	ruleInternal ruleKind = iota
	// ruleURL matches against the full URL (scheme-pinned when explicit).
	ruleURL
	// ruleHost matches against the host (or host:port) only.
	ruleHost
)

// rule is one compiled allow/reject entry. URL rules without a port pin
// additionally record noPort so matching can reject URLs carrying a port
// (Go's regexp has no lookahead).
type rule struct {
	re     *regexp.Regexp
	kind   ruleKind
	noPort bool
}

// matchURL reports whether a URL rule matches rawURL, enforcing the
// no-explicit-port constraint when the rule carries no port.
func (r rule) matchURL(rawURL string) bool {
	if !r.re.MatchString(rawURL) {
		return false
	}
	if r.noPort {
		if u, err := url.Parse(rawURL); err == nil && u.Port() != "" {
			return false
		}
	}
	return true
}

// Policy is the SSRF allow/reject filter for remote attachment URLs,
// mirroring Python's AppriseURLFilter. Deny rules are always processed
// before allow rules; a URL matching a deny rule is rejected; otherwise it
// is allowed only when it matches an allow rule.
type Policy struct {
	allow []rule
	deny  []rule
}

// NewPolicy compiles allow/reject lists. Entries are separated by commas
// and/or whitespace. Each entry may be a full URL (http:// or https://,
// pinning the scheme), a URL without scheme (matches both), a plain
// hostname or IP, or the reserved "internal" token. '*' matches anything,
// '?' matches a single host char ([A-Za-z0-9_-]) or path char (any
// non-slash). A trailing '*' is implied so rules operate as prefix matches.
func NewPolicy(allowList, denyList string) *Policy {
	return &Policy{allow: parseRuleList(allowList), deny: parseRuleList(denyList)}
}

// IsAllowed reports whether rawURL passes the DENY-first, allow-second
// policy. Non http(s) URLs, unparseable URLs, and URLs without a host are
// never allowed.
func (p *Policy) IsAllowed(rawURL string) bool {
	if p == nil {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if scheme := strings.ToLower(parsed.Scheme); scheme != "http" && scheme != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return false
	}
	netloc := host
	if port := parsed.Port(); port != "" {
		netloc = host + ":" + port
	}
	for _, r := range p.deny {
		switch r.kind {
		case ruleInternal:
			if isInternalTarget(host) {
				return false
			}
		case ruleURL:
			if r.matchURL(rawURL) {
				return false
			}
		case ruleHost:
			if r.re.MatchString(netloc) {
				return false
			}
		}
	}
	// "internal" has no meaning as a positive match; it is ignored here.
	for _, r := range p.allow {
		switch r.kind {
		case ruleInternal:
			continue
		case ruleURL:
			if r.matchURL(rawURL) {
				return true
			}
		case ruleHost:
			if r.re.MatchString(netloc) {
				return true
			}
		}
	}
	return false
}

// parseRuleList splits a list on whitespace/commas and compiles each token.
func parseRuleList(list string) []rule {
	var out []rule
	for _, token := range strings.FieldsFunc(strings.ToLower(list), func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) {
		if token == "" {
			continue
		}
		if token == InternalToken {
			out = append(out, rule{kind: ruleInternal})
			continue
		}
		switch {
		case strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://"):
			re, noPort := compileURLToken(token)
			out = append(out, rule{re: re, kind: ruleURL, noPort: noPort})
		case strings.Contains(token, "/"):
			re, noPort := compileURLToken("https?://" + token)
			out = append(out, rule{re: re, kind: ruleURL, noPort: noPort})
		default:
			out = append(out, rule{re: compileHostToken(token), kind: ruleHost})
		}
	}
	return out
}

// compileHostToken compiles a host-based token (no "/") into a regex
// matched against the URL's host (or host:port) part.
func compileHostToken(token string) *regexp.Regexp {
	return regexp.MustCompile("(?i)^" + wildcardToRegex(token, true) + "$")
}

// compileURLToken compiles a URL token with an explicit scheme, or an
// implicit token prefixed with "https?://" (matching both schemes). It
// reports noPort=true when the rule carries no port, in which case the
// caller must reject URLs with an explicit port (Go regexp lacks the
// negative lookahead Python uses).
func compileURLToken(token string) (*regexp.Regexp, bool) {
	scheme := "https?"
	switch {
	case strings.HasPrefix(token, "http://"):
		scheme, token = "http", token[len("http://"):]
	case strings.HasPrefix(token, "https://"):
		scheme, token = "https", token[len("https://"):]
	case strings.HasPrefix(token, "https?://"):
		token = token[len("https?://"):]
	}
	netloc, path := token, ""
	if i := strings.Index(token, "/"); i >= 0 {
		netloc, path = token[:i], token[i:]
	}
	host, port := netloc, ""
	portSpecified := false
	if i := strings.LastIndex(netloc, ":"); i >= 0 && !strings.Contains(netloc[i:], "]") {
		host, port, portSpecified = netloc[:i], netloc[i+1:], true
	}
	var b strings.Builder
	b.WriteString("(?i)^" + scheme + "://")
	b.WriteString(wildcardToRegex(host, true))
	noPort := false
	if portSpecified {
		b.WriteString(":" + regexp.QuoteMeta(port))
	} else {
		// No port in the rule: the URL must not carry one either
		// (enforced by rule.matchURL; Go regexp has no lookahead).
		noPort = true
	}
	switch {
	case path == "" || path == "/":
		b.WriteString("(/.*)?")
	case strings.HasSuffix(path, "*"):
		b.WriteString(wildcardToRegex(path[:len(path)-1], false) + "([^/]+/?)")
	case strings.HasSuffix(path, "/"):
		b.WriteString(wildcardToRegex(strings.TrimSuffix(path, "/"), false) + "(/.*)?")
	default:
		b.WriteString(wildcardToRegex(path, false) + "($|/.*)")
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String()), noPort
}

// wildcardToRegex converts '*'/'?' wildcards: '*' becomes '.*' for hosts
// and '[^/]+/?' for paths; '?' becomes '[A-Za-z0-9_-]' for hosts and '[^/]'
// for paths; everything else is escaped.
func wildcardToRegex(pattern string, isHost bool) string {
	var b strings.Builder
	for _, c := range pattern {
		switch c {
		case '*':
			if isHost {
				b.WriteString(".*")
			} else {
				b.WriteString("[^/]+/?")
			}
		case '?':
			if isHost {
				b.WriteString("[A-Za-z0-9_-]")
			} else {
				b.WriteString("[^/]")
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}
