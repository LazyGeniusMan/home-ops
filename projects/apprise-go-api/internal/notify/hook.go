// Package notify wraps the apprise-go notification engine with a
// request-scoped, timeout-bounded sender. This file adds the outbound
// result hook: best-effort POST {"source","status":0|1,"output"} to
// APPRISE_WEBHOOK_URL (http/https only; failures logged, never surfaced).
package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Hook timeout defaults in seconds.
const (
	defaultHookConnectTimeout = 4.0
	defaultHookReadTimeout    = 4.0
	// maxHookBodyDrain caps response-body draining after the hook POST.
	maxHookBodyDrain = 64 << 10
)

// hookTemplateArgs are query keys consumed by URL handling itself
// (redirect/cto/rto), not forwarded as request params.
var hookTemplateArgs = map[string]struct{}{
	"redirect": {}, "cto": {}, "rto": {},
}

// HookPayload is the outbound result body posted to the webhook URL.
type HookPayload struct {
	// Source is the notifying client's remote address.
	Source string `json:"source"`
	// Status is 0 on success, 1 on any delivery failure.
	Status int `json:"status"`
	// Output is the result detail sent with the notification outcome.
	Output any `json:"output"`
}

// HookClient posts HookPayload values to a parsed webhook URL. The zero
// value dials with hook-specific TLS verification state; tests may
// substitute Transport to capture the request.
type HookClient struct {
	// Transport, when non-nil, performs the POST.
	Transport http.RoundTripper
	// Log receives transport warnings; nil uses slog.Default().
	Log *slog.Logger
}

// parsedHook is a validated APPRISE_WEBHOOK_URL ready to send.
type parsedHook struct {
	// endpoint is the auth/query-stripped request URL (request_url).
	endpoint string
	// params are the extra query keys forwarded as request params.
	params url.Values
	// username/password hold embedded basic-auth credentials (nil
	// password when only a user is present).
	username string
	password *string
	// connectTimeout/readTimeout bound dial and full-response reads.
	connectTimeout time.Duration
	readTimeout    time.Duration
}

// ParseHookURL validates raw as an outbound webhook URL: it must carry a
// scheme, parse cleanly, use http/https, and hold a usable host. TLS
// verification is always on: '?verify=' is rejected outright.
func ParseHookURL(raw string) (*parsedHook, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("notify: webhook URL is empty")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("notify: webhook URL is not parseable: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		return nil, fmt.Errorf("notify: webhook URL is not a valid web based URI")
	}
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("notify: webhook URL is not using the HTTP protocol")
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return nil, fmt.Errorf("notify: webhook URL is not parseable")
	}
	// Hosts that survive only as opaque path fragments are unparseable.
	if _, _, ok := parseHookHostPort(u.Host); !ok {
		return nil, fmt.Errorf("notify: webhook URL is not parseable")
	}
	query := u.Query()
	for key := range query {
		if strings.EqualFold(key, "verify") {
			return nil, fmt.Errorf("notify: webhook URL must not carry ?verify= (TLS verification is always on)")
		}
	}
	connectSecs := parseHookFloat(query.Get("cto"), defaultHookConnectTimeout)
	readSecs := parseHookFloat(query.Get("rto"), defaultHookReadTimeout)
	params := make(url.Values, len(query))
	for key, values := range query {
		if _, reserved := hookTemplateArgs[strings.ToLower(key)]; reserved {
			continue
		}
		params[key] = append([]string(nil), values...)
	}
	endpoint := &url.URL{Scheme: scheme, Host: u.Host, Path: u.EscapedPath()}
	endpointURL := endpoint.String()
	if endpointURL == scheme+"://" {
		endpointURL += u.Host
	}
	var password *string
	if u.User != nil {
		if pw, set := u.User.Password(); set {
			password = &pw
		}
	}
	username := ""
	if u.User != nil {
		username = u.User.Username()
	}
	return &parsedHook{
		endpoint:       endpointURL,
		params:         params,
		username:       username,
		password:       password,
		connectTimeout: secondsToDuration(connectSecs),
		readTimeout:    secondsToDuration(readSecs),
	}, nil
}

// SendHook posts payload to rawURL. An empty rawURL is a no-op (false).
// Validation and transport failures are logged and swallowed so the hook
// never alters the notify response.
func (c *HookClient) SendHook(ctx context.Context, rawURL string, payload HookPayload) (attempted bool) {
	log := c.Log
	if log == nil {
		log = slog.Default()
	}
	if strings.TrimSpace(rawURL) == "" {
		return false
	}
	hook, err := ParseHookURL(rawURL)
	if err != nil {
		log.Warn("notify: webhook skipped", slog.String("error", err.Error()))
		return false
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Warn("notify: webhook payload encoding failed", slog.String("error", err.Error()))
		return false
	}
	target := hook.endpoint
	if len(hook.params) > 0 {
		target += "?" + hook.params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		log.Warn("notify: webhook request build failed", slog.String("error", err.Error()))
		return false
	}
	req.Header.Set("User-Agent", "Apprise-API")
	req.Header.Set("Content-Type", "application/json")
	// No trace-context injection: this POST leaves the cluster.
	if hook.username != "" || hook.password != nil {
		password := ""
		if hook.password != nil {
			password = *hook.password
		}
		req.SetBasicAuth(hook.username, password)
	}
	transport := c.Transport
	if transport == nil {
		transport = hookTransport(hook)
	}
	client := &http.Client{Transport: transport, Timeout: hook.connectTimeout + hook.readTimeout}
	resp, err := client.Do(req)
	if err != nil {
		log.Warn("notify: webhook delivery failed", slog.String("url", hook.endpoint), slog.String("error", err.Error()))
		return true
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxHookBodyDrain))
	return true
}

// hookTransport builds the default transport for hook: dial bound by the
// connect timeout with TLS verification always on.
func hookTransport(hook *parsedHook) http.RoundTripper {
	dialer := &net.Dialer{Timeout: hook.connectTimeout}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{DialContext: dialer.DialContext, TLSClientConfig: tlsConfig}
	}
	cloned := base.Clone()
	cloned.DialContext = dialer.DialContext
	cloned.TLSClientConfig = tlsConfig
	return cloned
}

// parseHookFloat parses a timeout query value in seconds; unparseable or
// negative values fall back to the default.
func parseHookFloat(raw string, fallback float64) float64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}

// secondsToDuration converts fractional seconds to a duration.
func secondsToDuration(secs float64) time.Duration {
	return time.Duration(secs * float64(time.Second))
}

// parseHookHostPort splits a URL host[:port] authority and reports whether
// it holds a syntactically usable host with an optional numeric port.
// Userinfo must already be stripped (use u.Host, not u.netloc). Bracketed
// IPv6 literals (e.g. "[::1]:8080") split on the bracket boundary, not the
// first colon; unbracketed colons are rejected so IPv6 literals must use
// brackets (matching u.Hostname's contract). Zone IDs ("%eth0") ride along
// in the host and are validated as ordinary hostname characters.
func parseHookHostPort(authority string) (host, port string, ok bool) {
	rest := strings.TrimSpace(authority)
	if strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		if end < 0 {
			return "", "", false
		}
		host = strings.TrimSpace(rest[1:end])
		rest = rest[end+1:]
		if rest == "" {
			// Bare "[::1]": no port.
		} else if strings.HasPrefix(rest, ":") {
			port = rest[1:]
		} else {
			return "", "", false
		}
	} else {
		if strings.Contains(rest, ":") {
			// Unbracketed IPv6 literals carry colons and are rejected here
			// (callers must bracket them); host:port splits on the last
			// colon so a stray colon elsewhere fails the port check below.
			if strings.Count(rest, ":") != 1 {
				return "", "", false
			}
			host, port, _ = strings.Cut(rest, ":")
		} else {
			host = rest
		}
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return "", "", false
	}
	for i := 0; i < len(host); i++ {
		c := host[i]
		letter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		digit := c >= '0' && c <= '9'
		if letter || digit || c == '.' || c == '-' || c == '_' || c == '%' || c == ':' {
			continue
		}
		return "", "", false
	}
	if port != "" {
		for i := 0; i < len(port); i++ {
			if port[i] < '0' || port[i] > '9' {
				return "", "", false
			}
		}
	}
	return host, port, true
}
