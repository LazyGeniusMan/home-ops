// Package notify wraps the apprise-go notification engine with a
// request-scoped, timeout-bounded sender. This file adds the outbound
// result hook: after every notify, POST {"source","status":0|1,"output"} to
// APPRISE_WEBHOOK_URL (best-effort; transport errors are logged, never
// surfaced to the notify caller).
//
// The semantics mirror apprise-api api/utils.py send_webhook: http/https
// only, embedded user[:pass] as basic auth, '?verify=' controlling TLS
// verification, remaining query keys forwarded as params, '?cto='/'?rto='
// connect/read timeouts defaulting to (4.0, 4.0), and 'User-Agent:
// Apprise-API' with a JSON body.
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

// Hook timeout defaults mirror apprise URLBase socket defaults (seconds).
const (
	defaultHookConnectTimeout = 4.0
	defaultHookReadTimeout    = 4.0
	// maxHookBodyDrain caps response-body draining after the hook POST.
	maxHookBodyDrain = 64 << 10
)

// hookTemplateArgs mirrors apprise URLBase.template_args: query keys
// consumed by URL handling itself (verify/redirect/cto/rto) are not
// forwarded as request params.
var hookTemplateArgs = map[string]struct{}{
	"verify": {}, "redirect": {}, "cto": {}, "rto": {},
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
	// password when only a user is present, mirroring request_auth).
	username string
	password *string
	// verify controls TLS certificate verification ('?verify='; true
	// unless the value parses false, mirroring apprise parse_bool).
	verify bool
	// connectTimeout/readTimeout bound dial and full-response reads.
	connectTimeout time.Duration
	readTimeout    time.Duration
}

// ParseHookURL validates raw as an outbound webhook URL, mirroring the
// send_webhook gate chain: the URL must carry a scheme, parse cleanly,
// use http/https, and hold a usable host. It returns the stripped endpoint,
// forwarded params, auth, verify flag, and timeouts.
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
	// Mirror apprise parse_url strictness: hosts that survive only as
	// opaque path fragments or bare symbols (e.g. "http://$#@" where the
	// fragment swallows the tail) are unparseable.
	if _, _, ok := parseHookHostPort(u.Host); !ok {
		return nil, fmt.Errorf("notify: webhook URL is not parseable")
	}
	query := u.Query()
	verify := parseHookBool(query.Get("verify"), true)
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
		verify:         verify,
		connectTimeout: secondsToDuration(connectSecs),
		readTimeout:    secondsToDuration(readSecs),
	}, nil
}

// SendHook posts payload to rawURL. An empty rawURL is a no-op (false).
// Validation and transport failures are logged and swallowed so the hook
// never alters the notify response; the boolean reports whether the POST
// was attempted (mirroring the requests.post call-count contract).
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
// connect timeout and TLS verification per '?verify='.
func hookTransport(hook *parsedHook) http.RoundTripper {
	dialer := &net.Dialer{Timeout: hook.connectTimeout}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if !hook.verify {
		tlsConfig.InsecureSkipVerify = true
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{DialContext: dialer.DialContext, TLSClientConfig: tlsConfig}
	}
	cloned := base.Clone()
	cloned.DialContext = dialer.DialContext
	cloned.TLSClientConfig = tlsConfig
	return cloned
}

// parseHookBool parses an apprise-style bool query value, mirroring
// parse_bool: true unless the value is an explicit false spelling.
func parseHookBool(raw string, fallback bool) bool {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return fallback
	}
	switch trimmed {
	case "no", "n", "false", "f", "0", "off", "disable", "disabled", "disallow", "deny":
		return false
	default:
		return true
	}
}

// parseHookFloat parses a timeout query value in seconds; unparseable or
// negative values fall back to the default, mirroring apprise's float
// template-arg handling.
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
// Userinfo must already be stripped (use u.Host, not u.netloc).
func parseHookHostPort(authority string) (host, port string, ok bool) {
	host, port, _ = strings.Cut(authority, ":")
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" {
		return "", "", false
	}
	for i := 0; i < len(host); i++ {
		c := host[i]
		letter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		digit := c >= '0' && c <= '9'
		if letter || digit || c == '.' || c == '-' || c == '_' || c == '%' {
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
