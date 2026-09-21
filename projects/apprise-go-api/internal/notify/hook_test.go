package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// errHookBoom scripts transport failure.
var errHookBoom = errors.New("hook: boom")

// captureRoundTripper records the outgoing hook request for assertions.
type captureRoundTripper struct {
	req  *http.Request
	body []byte
	err  error
}

func (c *captureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if c.err != nil {
		return nil, c.err
	}
	body, _ := io.ReadAll(req.Body)
	c.body = body
	c.req = req.Clone(req.Context())
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusOK)
	return rec.Result(), nil
}

func testHookClient(cap *captureRoundTripper) *HookClient {
	return &HookClient{
		Transport: cap,
		Log:       slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
}

func TestParseHookURLBasic(t *testing.T) {
	hook, err := ParseHookURL("https://user:pass@localhost/webhook")
	if err != nil {
		t.Fatalf("ParseHookURL() = %v", err)
	}
	if hook.endpoint != "https://localhost/webhook" {
		t.Errorf("endpoint = %q, want stripped URL", hook.endpoint)
	}
	if hook.username != "user" || hook.password == nil || *hook.password != "pass" {
		t.Errorf("auth = %q/%v, want user/pass", hook.username, hook.password)
	}
	if !hook.verify {
		t.Error("verify = false, want true default")
	}
	if len(hook.params) != 0 {
		t.Errorf("params = %v, want empty", hook.params)
	}
	if hook.connectTimeout != 4*time.Second || hook.readTimeout != 4*time.Second {
		t.Errorf("timeouts = %v/%v, want 4s/4s", hook.connectTimeout, hook.readTimeout)
	}
}

func TestParseHookURLQuery(t *testing.T) {
	hook, err := ParseHookURL("http://user@localhost/webhook/here?verify=False&key=value&cto=2.0&rto=1.0")
	if err != nil {
		t.Fatalf("ParseHookURL() = %v", err)
	}
	if hook.endpoint != "http://localhost/webhook/here" {
		t.Errorf("endpoint = %q", hook.endpoint)
	}
	if hook.username != "user" || hook.password != nil {
		t.Errorf("auth = %q/%v, want user/<nil>", hook.username, hook.password)
	}
	if hook.verify {
		t.Error("verify = true, want false")
	}
	if hook.params.Get("key") != "value" {
		t.Errorf("params = %v, want key=value", hook.params)
	}
	if hook.connectTimeout != 2*time.Second || hook.readTimeout != time.Second {
		t.Errorf("timeouts = %v/%v, want 2s/1s", hook.connectTimeout, hook.readTimeout)
	}
}

func TestParseHookURLInvalid(t *testing.T) {
	for _, raw := range []string{
		"", "invalid", "http://$#@", "invalid://hostname", "ftp://localhost/hook",
	} {
		if _, err := ParseHookURL(raw); err == nil {
			t.Errorf("ParseHookURL(%q) = nil, want error", raw)
		}
	}
}

func TestSendHookRequestShape(t *testing.T) {
	cap := &captureRoundTripper{}
	client := testHookClient(cap)
	payload := HookPayload{Source: "192.0.2.1", Status: 1, Output: map[string]any{"error": "x"}}
	if !client.SendHook(context.Background(), "https://user:pass@localhost/webhook", payload) {
		t.Fatal("SendHook() = false, want attempted")
	}
	if cap.req == nil {
		t.Fatal("no request captured")
	}
	if cap.req.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", cap.req.Method)
	}
	if got := cap.req.URL.Scheme + "://" + cap.req.URL.Host + cap.req.URL.Path; got != "https://localhost/webhook" {
		t.Errorf("URL = %s, want stripped endpoint", got)
	}
	if got := cap.req.Header.Get("User-Agent"); got != "Apprise-API" {
		t.Errorf("User-Agent = %q", got)
	}
	if got := cap.req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	user, pass, ok := cap.req.BasicAuth()
	if !ok || user != "user" || pass != "pass" {
		t.Errorf("basic auth = %q/%q/%v", user, pass, ok)
	}
	var decoded map[string]any
	if err := json.Unmarshal(cap.body, &decoded); err != nil {
		t.Fatalf("body = %v", err)
	}
	if decoded["source"] != "192.0.2.1" || decoded["status"] != float64(1) {
		t.Errorf("body = %v, want source/status", decoded)
	}
}

func TestSendHookParamsAndTimeouts(t *testing.T) {
	cap := &captureRoundTripper{}
	client := testHookClient(cap)
	client.SendHook(context.Background(),
		"http://user@localhost/webhook/here?verify=False&key=value&cto=2.0&rto=1.0",
		HookPayload{Source: "s", Status: 0})
	if cap.req == nil {
		t.Fatal("no request captured")
	}
	query := cap.req.URL.Query()
	if query.Get("key") != "value" {
		t.Errorf("query = %v, want key=value forwarded", query)
	}
	for _, reserved := range []string{"verify", "cto", "rto"} {
		if _, ok := query[reserved]; ok {
			t.Errorf("query forwards reserved key %q: %v", reserved, query)
		}
	}
	user, pass, ok := cap.req.BasicAuth()
	if !ok || user != "user" || pass != "" {
		t.Errorf("basic auth = %q/%q/%v, want user/empty", user, pass, ok)
	}
}

func TestSendHookSkipsAndSwallows(t *testing.T) {
	cap := &captureRoundTripper{}
	client := testHookClient(cap)
	// Empty and invalid URLs: not attempted, nil error contract (bool false).
	if client.SendHook(context.Background(), "", HookPayload{}) {
		t.Error("SendHook(empty) = true, want false")
	}
	if client.SendHook(context.Background(), "invalid://hostname", HookPayload{}) {
		t.Error("SendHook(invalid) = true, want false")
	}
	if cap.req != nil {
		t.Error("request captured for skipped hook")
	}
	// Transport errors are swallowed but count as attempted.
	capErr := &captureRoundTripper{err: errHookBoom}
	if !testHookClient(capErr).SendHook(context.Background(), "http://localhost", HookPayload{}) {
		t.Error("SendHook(transport error) = false, want attempted=true")
	}
}

func TestSendHookLiveServer(t *testing.T) {
	var got HookPayload
	var userAgent, contentType string
	var query url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent, contentType = r.Header.Get("User-Agent"), r.Header.Get("Content-Type")
		query = r.URL.Query()
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	client := testHookClient(&captureRoundTripper{})
	// Use the default transport path via a client without stub transport.
	live := &HookClient{Log: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	if !live.SendHook(context.Background(), srv.URL+"/h?key=value", HookPayload{Source: "s", Status: 0, Output: "out"}) {
		t.Fatal("SendHook(live) = false, want attempted")
	}
	_ = client
	if userAgent != "Apprise-API" || !strings.Contains(contentType, "application/json") {
		t.Errorf("headers = %q/%q", userAgent, contentType)
	}
	if query.Get("key") != "value" {
		t.Errorf("query = %v", query)
	}
	if got.Source != "s" || got.Status != 0 || got.Output != "out" {
		t.Errorf("payload = %+v", got)
	}
}
